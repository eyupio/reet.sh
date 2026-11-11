package terminalbroker

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
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
	// ErrReattachNoConversation means the row has no native session id to load.
	ErrReattachNoConversation = errors.New("This chat has no saved conversation to reconnect.")
	// ErrReattachArchived means the chat was archived: its transcript is gone,
	// so it is not reconnected.
	ErrReattachArchived = errors.New("This chat was archived, so its history is gone.")
	// ErrReattachConflict means the machine refused with a conflict that is
	// not a start in flight: another session owns the conversation.
	ErrReattachConflict = errors.New("This conversation is open in another session.")
	// ErrReattachStarting means the machine is still starting this chat (an
	// earlier reattach, a create or a restore): a retry soon may succeed.
	ErrReattachStarting = errors.New("Still reconnecting. Try again in a moment.")
	// ErrReattachFailed means the machine could not reopen the chat, or did
	// not confirm it running. Trying again may help.
	ErrReattachFailed = errors.New("The machine did not bring this chat back. Try again, or start a new chat.")
)

// reattachCallTimeout bounds a reattach's acknowledgement: the machine's
// start is bounded by 60 s, and the controller waits that long and a margin.
const reattachCallTimeout = 60*time.Second + 15*time.Second

// reattachKey names one in-flight ReattachChat: the session and whether the
// caller confirmed trust. Trust is part of the key so a trusting caller never
// joins a call that did not confirm it, and never lends its trust to one.
type reattachKey struct {
	chatKey
	trust bool
}

// reattachCall is one in-flight ReattachChat; callers for the same session
// and trust join it and get its result.
type reattachCall struct {
	done chan struct{}
	err  error
	// joined counts the callers waiting on this call besides its leader.
	joined int
}

// reattachJoined reports how many callers wait on a session's in-flight
// reattach with that trust besides the one running it.
func (b *Broker) reattachJoined(uid int64, session string, trust bool) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c := b.reattaching[reattachKey{chatKey{uid, session}, trust}]; c != nil {
		return c.joined
	}
	return 0
}

// ReattachChat reconnects a lost or exited chat in place: the machine starts
// it again with session/load under the same reet session id, and the row
// returns to running with its transcript. No row is created, and the row is
// left as it was on any error. Concurrent calls for one session and trust
// share a single machine request and its result.
//
// trust confirms that the person trusts the chat's folder, which lets the
// machine load OpenCode plugins there. It is sent only for this call and is
// never stored by the controller (the machine keeps a started chat's trust in
// its restart record, see docs/structured-sessions.md): only a person's
// explicit request may set it, never an automatic attempt.
func (b *Broker) ReattachChat(ctx context.Context, uid int64, session string, trust bool) error {
	k := reattachKey{chatKey{uid, session}, trust}
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
		b.reattaching = map[reattachKey]*reattachCall{}
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
	c.err = b.reattachChat(ctx, uid, session, trust)
	return c.err
}

func (b *Broker) reattachChat(ctx context.Context, uid int64, session string, trust bool) error {
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return err
	}
	// Which agents can reconnect in place is the machine's descriptor, checked below, not a list here.
	if !s.Structured || !terminalproto.ReviewedChatTool(s.Tool) {
		return ErrACPUnsupported
	}
	if b.st.ChatOffFor(uid, s.Tool) {
		return ErrChatOff
	}
	// An archived chat's transcript is gone: say so before the ended checks,
	// whose answer offers a Continue with history there is nothing for.
	archived, err := b.st.TerminalArchivedFor(uid, session)
	if err != nil {
		return err
	}
	if archived {
		return ErrReattachArchived
	}
	switch s.State {
	case terminalproto.Lost, terminalproto.Exited:
	case terminalproto.Ended:
		// A person ended it: continuing is a deliberate new row.
		return ErrACPClosed
	default:
		return ErrReattachNotClosed
	}
	// A person asked to end it. Rows ended before ended chats were recorded
	// as ended are stored exited, or lost when the machine went away before
	// it answered; the end operation refuses them all the same.
	ended, err := b.st.TerminalEndRequestedFor(uid, s.AgentID, session)
	if err != nil {
		return err
	}
	if ended {
		return ErrACPClosed
	}
	if !terminalproto.ValidACPSessionIDFor(s.Tool, s.ACPSessionID) {
		return ErrReattachNoConversation
	}
	// Reconnecting a YOLO chat is a YOLO launch the account must still allow.
	if s.Yolo && !b.yoloAllowed() {
		return ErrYoloNotAllowed
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
	if err != nil || caps.Generation != p.Generation {
		return ErrOffline
	}
	chat, ok := caps.Capabilities.ChatAgent(s.Tool)
	supported := caps.Capabilities.ChatReattachFor(s.Tool) && ok && chat.History
	if !supported {
		return ErrReattachUnsupported
	}
	in := terminalproto.ChatReattach{Tool: s.Tool, Dir: s.Dir, Yolo: s.Yolo, ACPSessionID: s.ACPSessionID, Trust: trust}
	if caps.Capabilities.ACPLiveControls {
		in.Effort = s.Effort
	}
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
	// The reply's OpenCode session id is not recorded: the machine loaded
	// the stored one, and the store only ever fills an empty id, so the
	// stored id always wins.
	if _, err := reattachAck(reply); err != nil {
		return err
	}
	if err := b.st.ObserveTerminalStateFor(uid, s.AgentID, session, p.Generation, terminalproto.Running, nil); err != nil {
		// The row changed meanwhile (ended, or a new generation); the next
		// reconcile settles it from what the machine reports.
		return ErrReattachFailed
	}
	if fresh, err := b.st.TerminalSessionFor(uid, session); err == nil {
		s = fresh
	} else {
		s.State = terminalproto.Running
	}
	// The adapter loads the conversation in its start mode whatever mode the chat was left in, so the mode kept here goes back to it.
	_ = b.st.RecordTerminalAgentModeFor(uid, session, b.startAgentMode(uid, s.AgentID, s.Tool))
	b.restoreChat(uid, s)
	b.startChatQueue(uid, s.ID)
	_ = b.st.AddEventFor(uid, "machine", "chat-reattach", "", map[string]string{
		"machine": s.AgentID, "tool": s.Tool, "session": session, "trust": strconv.FormatBool(trust),
	})
	return nil
}

// reattachAck maps the machine's answer to a reattach. Only a running session
// is success; a session still starting (the answer to a resent request id) or
// gone is a retryable failure. The machine encodes
// terminalhost.ErrOperationConflict as terminalproto.Conflict, with the
// detail terminalproto.ConflictStarting while the chat is still starting.
// YOLO refused and capacity keep the create path's errors, and ACP refusals
// keep their own.
func reattachAck(reply terminalproto.Message) (*terminalproto.SessionChanged, error) {
	switch body := reply.Body.(type) {
	case *terminalproto.SessionChanged:
		if body.State == terminalproto.Running {
			return body, nil
		}
	case *terminalproto.Result:
		switch body.Code {
		case terminalproto.Conflict:
			if body.Detail == terminalproto.ConflictStarting {
				return nil, ErrReattachStarting
			}
			return nil, ErrReattachConflict
		case terminalproto.YoloRefused:
			// YOLO is off on the machine: a retry cannot help.
			return nil, ErrYoloRefused
		case terminalproto.Capacity:
			return nil, store.ErrTerminalCapacity
		}
		if refusal := acpRefusal(body.Code, body.Detail); refusal != nil {
			return nil, refusal
		}
	}
	return nil, ErrReattachFailed
}
