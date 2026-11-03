package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// ErrReattachUnsupported means the machine did not negotiate
	// chat-reattach-v1 for the tool, or cannot load its history, so it was not
	// asked.
	ErrReattachUnsupported = errors.New("Update reet on this machine to reconnect this chat.")
	// ErrReattachNotClosed means the row is not lost or exited: the chat is
	// still starting, running or ending.
	ErrReattachNotClosed = errors.New("This chat is still running.")
	// ErrReattachNoConversation means the row has no OpenCode session id to
	// load.
	ErrReattachNoConversation = errors.New("This chat has no saved conversation to reconnect.")
	// ErrReattachConflict means the machine refused with a conflict: another
	// session owns the conversation, or a start for this one is in flight.
	ErrReattachConflict = errors.New("This conversation is open in another session.")
	// ErrReattachFailed means the machine could not reopen the chat, or did
	// not confirm it running. Trying again may help.
	ErrReattachFailed = errors.New("The chat could not be reconnected. Try again, or start a new chat.")
)

// reattachCallTimeout bounds a reattach's acknowledgement: the machine's
// start is bounded by 60 s, and the controller waits that long and a margin.
const reattachCallTimeout = 60*time.Second + 15*time.Second

// reattachCall is one in-flight ReattachChat; callers for the same session
// join it and get its result.
type reattachCall struct {
	done chan struct{}
	err  error
	// joined counts the callers waiting on this call besides its leader.
	joined int
}

// reattachJoined reports how many callers wait on a session's in-flight
// reattach besides the one running it.
func (b *Broker) reattachJoined(uid int64, session string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c := b.reattaching[chatKey{uid, session}]; c != nil {
		return c.joined
	}
	return 0
}

// ReattachChat reconnects a lost or exited OpenCode chat in place: the
// machine starts it again with session/load under the same reet session id,
// and the row returns to running with its transcript. No row is created, and
// the row is left as it was on any error. Concurrent calls for one session
// share a single machine request and its result.
func (b *Broker) ReattachChat(ctx context.Context, uid int64, session string) error {
	k := chatKey{uid, session}
	b.mu.Lock()
	if c := b.reattaching[k]; c != nil {
		c.joined++
		b.mu.Unlock()
		select {
		case <-c.done:
			return c.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if b.reattaching == nil {
		b.reattaching = map[chatKey]*reattachCall{}
	}
	c := &reattachCall{done: make(chan struct{}), err: ErrReattachFailed}
	b.reattaching[k] = c
	b.mu.Unlock()
	// Release the slot however the call ends, so a later call runs again.
	defer func() {
		b.mu.Lock()
		delete(b.reattaching, k)
		b.mu.Unlock()
		close(c.done)
	}()
	c.err = b.reattachChat(ctx, uid, session)
	return c.err
}

func (b *Broker) reattachChat(ctx context.Context, uid int64, session string) error {
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return err
	}
	if !s.Structured || s.Tool != "opencode" {
		return ErrACPUnsupported
	}
	switch s.State {
	case terminalproto.Lost, terminalproto.Exited:
	case terminalproto.Ended:
		// A person ended it: continuing is a deliberate new row.
		return ErrACPClosed
	default:
		return ErrReattachNotClosed
	}
	if !terminalproto.ValidACPSessionIDFor(s.Tool, s.ACPSessionID) {
		return ErrReattachNoConversation
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
	if err != nil || caps.Generation != p.Generation {
		return ErrOffline
	}
	if chat, ok := caps.Capabilities.ChatAgent(s.Tool); !caps.Capabilities.ChatReattachFor(s.Tool) || !ok || !chat.History {
		return ErrReattachUnsupported
	}
	in := terminalproto.ChatReattach{Tool: s.Tool, Dir: s.Dir, Yolo: s.Yolo, ACPSessionID: s.ACPSessionID}
	if terminalproto.ValidModel(s.Model) {
		in.Model = s.Model
	}
	if terminalproto.ValidTitle(s.Title) {
		in.Title = s.Title
	}
	// The machine streams the post-load status and interrupted-tool closes
	// before it answers: the ring must exist first. It is never closed here,
	// because the transcript is kept.
	b.chatLog(uid, session)
	// Once sent, the reattach runs on the machine whether or not the browser
	// stays, so a cancelled request must not abandon the answer. A fresh
	// request id every call: a reused one is answered from the machine's
	// receipt of the earlier attempt.
	ctx = context.WithoutCancel(ctx)
	reply, err := p.callWithin(ctx, terminalproto.Message{RequestID: ID(), SessionID: session, Body: &in}, reattachCallTimeout)
	if err != nil {
		if errors.Is(err, ErrOffline) {
			return ErrOffline
		}
		return ErrReattachFailed
	}
	changed, err := reattachAck(reply)
	if err != nil {
		return err
	}
	if err := b.st.ObserveTerminalStateFor(uid, s.AgentID, session, p.Generation, terminalproto.Running, nil); err != nil {
		// The row changed meanwhile (ended, or a new generation); the next
		// reconcile settles it from what the machine reports.
		return ErrReattachFailed
	}
	if changed.ACPSessionID != "" && changed.ACPSessionID != s.ACPSessionID {
		// The store keeps the first id it recorded; this only fills a gap.
		_ = b.st.RecordTerminalACPSessionFor(uid, s.AgentID, session, changed.ACPSessionID)
	}
	if fresh, err := b.st.TerminalSessionFor(uid, session); err == nil {
		s = fresh
	} else {
		s.State = terminalproto.Running
	}
	b.restoreChat(uid, s)
	_ = b.st.AddEventFor(uid, "machine", "chat-reattach", "", map[string]string{
		"machine": s.AgentID, "tool": s.Tool, "session": session,
	})
	return nil
}

// reattachAck maps the machine's answer to a reattach. Only a running session
// is success; a session still starting (the answer to a resent request id) or
// gone is a retryable failure. The machine encodes
// terminalhost.ErrOperationConflict as terminalproto.Conflict (chatResult
// falls through to result), and ACP refusals keep their own errors.
func reattachAck(reply terminalproto.Message) (*terminalproto.SessionChanged, error) {
	switch body := reply.Body.(type) {
	case *terminalproto.SessionChanged:
		if body.State == terminalproto.Running {
			return body, nil
		}
	case *terminalproto.Result:
		if body.Code == terminalproto.Conflict {
			return nil, ErrReattachConflict
		}
		if refusal := acpRefusal(body.Code, body.Detail); refusal != nil {
			return nil, refusal
		}
	}
	return nil, ErrReattachFailed
}
