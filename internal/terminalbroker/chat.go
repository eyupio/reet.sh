package terminalbroker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	ErrACPUnsupported = errors.New("that machine does not support OpenCode chats")
	ErrACPVersion     = errors.New("the machine needs OpenCode 1.19.5 or newer for chats")
	ErrACPBusy        = errors.New("the chat is already working")
	ErrACPClosed      = errors.New("the chat is not running")
	// ErrACPResumeNotFound means the requested OpenCode session is not one of
	// this account's closed chats on that machine.
	ErrACPResumeNotFound = errors.New("that chat history is not available")
	// ErrACPHistoryUnsupported means the machine's connector predates
	// acp-history-v1, so it cannot report or resume an OpenCode session id.
	ErrACPHistoryUnsupported = errors.New("that machine cannot reopen chat history")
)

// ACPTrustError means the folder has OpenCode plugins and the start did not
// carry the person's trust confirmation.
type ACPTrustError struct{ Detail string }

func (e *ACPTrustError) Error() string { return "this folder has OpenCode plugins" }

// ACPUnavailableError carries the machine's plain sentence when OpenCode could
// not start for a reason other than version or trust.
type ACPUnavailableError struct{ Detail string }

func (e *ACPUnavailableError) Error() string { return "OpenCode could not start the chat" }

func acpRefusal(code terminalproto.ResultCode, detail string) error {
	switch code {
	case terminalproto.ACPUnavailable:
		if detail == "" {
			return ErrACPUnsupported
		}
		return &ACPUnavailableError{Detail: detail}
	case terminalproto.ACPVersion:
		return ErrACPVersion
	case terminalproto.ACPTrustRequired:
		return &ACPTrustError{Detail: detail}
	}
	return nil
}

type chatKey struct {
	uid     int64
	session string
}

// ChatEvent is one bounded event in a chat's controller-side ring. Seq is
// assigned by the controller; the machine's protocol has no sequence number.
type ChatEvent struct {
	Seq int64 `json:"seq"`
	terminalproto.ACPEvent
}

type chatLog struct {
	mu        sync.Mutex
	events    []ChatEvent
	seq       int64
	bytes     int
	truncated bool
	subs      map[int]chan ChatEvent
	nextSub   int
}

const (
	maxChatEvents = 1000
	maxChatBytes  = 256 << 10
	maxChatSubs   = 32
)

func newChatLog() *chatLog { return &chatLog{subs: map[int]chan ChatEvent{}} }

func (l *chatLog) publish(event terminalproto.ACPEvent) {
	data, err := json.Marshal(event)
	if err != nil || len(data) > maxChatBytes {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	l.events = append(l.events, ChatEvent{Seq: l.seq, ACPEvent: event})
	l.bytes += len(data)
	for len(l.events) > maxChatEvents || l.bytes > maxChatBytes {
		l.bytes -= len(mustJSON(l.events[0].ACPEvent))
		l.events = l.events[1:]
		l.truncated = true
	}
	for id, ch := range l.subs {
		select {
		case ch <- l.events[len(l.events)-1]:
		default:
			// A subscriber that cannot keep up reconnects and gets a reset,
			// rather than silently missing events.
			delete(l.subs, id)
			close(ch)
		}
	}
}

func mustJSON(event terminalproto.ACPEvent) []byte {
	data, _ := json.Marshal(event)
	return data
}

func (l *chatLog) subscribe() (snapshot []ChatEvent, truncated bool, events <-chan ChatEvent, cancel func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	snapshot = append([]ChatEvent(nil), l.events...)
	truncated = l.truncated
	ch := make(chan ChatEvent, 256)
	id := l.nextSub
	l.nextSub++
	if len(l.subs) >= maxChatSubs {
		// Oldest subscriber is dropped; its stream reconnects and resets.
		for old, oldCh := range l.subs {
			delete(l.subs, old)
			close(oldCh)
			break
		}
	}
	l.subs[id] = ch
	cancel = func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if ch, ok := l.subs[id]; ok {
			delete(l.subs, id)
			close(ch)
		}
	}
	return snapshot, truncated, ch, cancel
}

func (l *chatLog) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, ch := range l.subs {
		delete(l.subs, id)
		close(ch)
	}
}

func (b *Broker) chatLog(uid int64, session string) *chatLog {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := chatKey{uid, session}
	l := b.chats[key]
	if l == nil {
		l = newChatLog()
		b.chats[key] = l
	}
	return l
}

func (b *Broker) publishChat(uid int64, session string, event terminalproto.ACPEvent) {
	b.mu.Lock()
	l := b.chats[chatKey{uid, session}]
	b.mu.Unlock()
	if l != nil {
		l.publish(event)
	}
	if event.Kind == "permission_resolved" && event.Allow != nil && *event.Allow {
		if s, err := b.st.TerminalSessionFor(uid, session); err == nil && s.Yolo {
			_ = b.st.AddEventFor(uid, "machine", "yolo-allow", "", map[string]string{
				"machine": s.AgentID, "tool": s.Tool, "session": session, "permission": event.PermissionID,
			})
		}
	}
}

// SubscribeChat returns the current ring and a subscription. The caller must
// cancel it. A reconnecting client always starts from a reset and the full
// current ring; Seq lets it ignore a duplicate it has already shown.
func (b *Broker) SubscribeChat(uid int64, session string) ([]ChatEvent, bool, <-chan ChatEvent, func()) {
	return b.chatLog(uid, session).subscribe()
}

// CloseChat forgets a chat's in-memory history and closes its subscribers.
// The session record remains; an archived chat has no transcript to reopen.
func (b *Broker) CloseChat(uid int64, session string) { b.closeChat(uid, session) }

func (b *Broker) closeChat(uid int64, session string) {
	b.mu.Lock()
	key := chatKey{uid, session}
	l := b.chats[key]
	delete(b.chats, key)
	b.mu.Unlock()
	if l != nil {
		l.close()
	}
}

func (b *Broker) chatExists(uid int64, session string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.chats[chatKey{uid, session}]
	return ok
}

func (b *Broker) structuredSession(uid int64, session string) (store.TerminalSession, error) {
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return s, err
	}
	if !s.Structured {
		return s, ErrACPUnsupported
	}
	if s.State != terminalproto.Running {
		return s, ErrACPClosed
	}
	return s, nil
}

func (b *Broker) PromptChat(ctx context.Context, uid int64, session, text string) error {
	if !terminalproto.ValidChatPrompt(text) {
		return terminalproto.ErrInvalid
	}
	s, err := b.structuredSession(uid, session)
	if err != nil {
		return err
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return err
	}
	b.chatLog(uid, session).publish(terminalproto.ACPEvent{Kind: "user", Text: text})
	reply, err := p.callWithin(ctx, terminalproto.Message{RequestID: ID(), SessionID: session, Body: &terminalproto.ACPPrompt{Text: text}}, chatCallTimeout)
	if ack := chatAck(reply, err); ack != nil {
		text := "That message was not sent to OpenCode."
		var lost errReplyLost
		if errors.As(err, &lost) {
			text = "We didn't hear back. OpenCode may have the message; check the chat before sending it again."
		}
		b.chatLog(uid, session).publish(terminalproto.ACPEvent{Kind: "notice", Text: text, Bad: true})
		return ack
	}
	return nil
}

func chatAck(reply terminalproto.Message, err error) error {
	if err != nil {
		return err
	}
	if r, ok := reply.Body.(*terminalproto.Result); ok {
		switch r.Code {
		case terminalproto.OK:
			return nil
		case terminalproto.Conflict:
			return ErrACPBusy
		case terminalproto.NotFound:
			return ErrACPClosed
		}
	}
	return ErrACPClosed
}

func (b *Broker) CancelChat(ctx context.Context, uid int64, session string) error {
	s, err := b.structuredSession(uid, session)
	if err != nil {
		return err
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return err
	}
	reply, err := p.callWithin(ctx, terminalproto.Message{RequestID: ID(), SessionID: session, Body: &terminalproto.ACPCancel{}}, chatCallTimeout)
	return chatAck(reply, err)
}

func (b *Broker) PermissionChat(ctx context.Context, uid int64, session, permissionID string, allow bool) error {
	s, err := b.structuredSession(uid, session)
	if err != nil {
		return err
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return err
	}
	reply, err := p.callWithin(ctx, terminalproto.Message{RequestID: ID(), SessionID: session,
		Body: &terminalproto.ACPPermission{PermissionID: permissionID, Allow: allow}}, chatCallTimeout)
	if err != nil {
		return err
	}
	if r, ok := reply.Body.(*terminalproto.Result); ok {
		switch r.Code {
		case terminalproto.OK:
			return nil
		case terminalproto.Conflict:
			return ErrACPBusy
		}
	}
	return ErrACPClosed
}

// chatCallTimeout bounds a chat command's acknowledgement. The model turn is
// not waited for; its events arrive on the same control connection.
var chatCallTimeout = 10 * time.Second
