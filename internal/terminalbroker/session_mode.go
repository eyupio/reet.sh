package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// ErrSessionModeBusy means a turn is running or an approval or question
	// is waiting, here or on the machine; nothing was switched.
	ErrSessionModeBusy = errors.New("Wait for the agent to finish or answer what it is asking, then switch.")
	// ErrSessionModeUnsupported means the machine did not negotiate
	// session-mode-v1, so it was not asked.
	ErrSessionModeUnsupported = errors.New("Update reet on this machine to switch between the reet TUI and the OpenCode TUI.")
	// ErrSessionModeNoSession means the row has no OpenCode session id to
	// load, because the session began in the OpenCode TUI.
	ErrSessionModeNoSession = errors.New("This session started in the OpenCode TUI, so the reet TUI cannot pick up its conversation.")
	// ErrSessionModeTrust is the structured create's trust refusal: the folder
	// has OpenCode plugins and the switch did not carry the person's trust.
	ErrSessionModeTrust = errors.New("This folder has OpenCode plugins. Start only in a folder you trust.")
	// ErrSessionModeFailed means the machine could not start the other mode.
	// It restored the previous one, or ended the session and says so itself.
	ErrSessionModeFailed = errors.New("The session could not be switched. Try again, or end it and start a new session.")
)

// sessionModeTrustError is ErrSessionModeTrust carrying the machine's
// ACPTrustError, so a caller can show the plugin files it names.
type sessionModeTrustError struct{ trust *ACPTrustError }

func (e sessionModeTrustError) Error() string   { return ErrSessionModeTrust.Error() }
func (e sessionModeTrustError) Unwrap() []error { return []error{ErrSessionModeTrust, e.trust} }

// The terminal size a switch to the OpenCode TUI starts with, the same a
// create uses by default (the web's createTerminal and project runs); the
// first attachment resizes it.
const (
	switchCols = 100
	switchRows = 30
)

// sessionModeCallTimeout bounds a switch's acknowledgement. The machine stops
// one process, starts the other and may restore the first before it answers,
// all within terminalproto.SessionModeTimeout; the controller waits that long
// and a margin, so its own deadline never loses an answer.
var sessionModeCallTimeout = terminalproto.SessionModeTimeout + 15*time.Second

// claimSwitch marks a session as switching, so a second switch of it is
// refused before it touches the first one's ring. It reports false when one
// is already running; release with releaseSwitch.
func (b *Broker) claimSwitch(uid int64, session string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	k := chatKey{uid, session}
	if _, held := b.switching[k]; held {
		return false
	}
	if b.switching == nil {
		b.switching = map[chatKey]struct{}{}
	}
	b.switching[k] = struct{}{}
	return true
}

func (b *Broker) releaseSwitch(uid int64, session string) {
	b.mu.Lock()
	delete(b.switching, chatKey{uid, session})
	b.mu.Unlock()
}

// chatBusy reports whether the controller's ring for a chat shows a running
// turn, a pending approval or a waiting question.
func (b *Broker) chatBusy(uid int64, session string) bool {
	b.mu.Lock()
	l := b.chats[chatKey{uid, session}]
	b.mu.Unlock()
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running || len(l.pending) > 0 || len(l.questions) > 0
}

// SwitchSessionMode moves a running OpenCode session between the reet TUI
// (chat) and the OpenCode TUI (terminal) under the same row. The machine is
// asked only when it negotiated session-mode-v1, the row has the OpenCode
// session id to carry across, and the chat is idle. The row and its rings
// change only after the machine acknowledges the switch.
func (b *Broker) SwitchSessionMode(ctx context.Context, uid int64, session, to string, trust bool) error {
	if to != terminalproto.SessionModeTerminal && to != terminalproto.SessionModeChat {
		return terminalproto.ErrInvalid
	}
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return err
	}
	if s.State != terminalproto.Running {
		return ErrACPClosed
	}
	if !terminalproto.ReviewedChatTool(s.Tool) {
		return ErrACPUnsupported
	}
	if s.Structured == (to == terminalproto.SessionModeChat) {
		// Already there: nothing to ask the machine or record.
		return nil
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
	if err != nil || caps.Generation != p.Generation {
		return ErrOffline
	}
	if chat, ok := caps.Capabilities.ChatAgent(s.Tool); !caps.Capabilities.SessionMode || !ok || !chat.History {
		return ErrSessionModeUnsupported
	}
	if !terminalproto.ValidACPSessionIDFor(s.Tool, s.ACPSessionID) {
		if to == terminalproto.SessionModeChat {
			return ErrSessionModeNoSession
		}
		// A chat whose id was never recorded cannot be reopened in the TUI.
		return ErrSessionModeUnsupported
	}
	if !b.claimSwitch(uid, session) {
		return ErrSessionModeBusy
	}
	defer b.releaseSwitch(uid, session)
	if s.Structured && b.chatBusy(uid, session) {
		return ErrSessionModeBusy
	}
	in := terminalproto.SessionMode{To: to, Tool: s.Tool, Dir: s.Dir, Yolo: s.Yolo, ACPSessionID: s.ACPSessionID,
		Trust: trust && to == terminalproto.SessionModeChat, Cols: switchCols, Rows: switchRows}
	if terminalproto.ValidModel(s.Model) {
		in.Model = s.Model
	}
	if terminalproto.ValidTitle(s.Title) {
		in.Title = s.Title
	}
	if to == terminalproto.SessionModeChat {
		// The session/load replay streams before the acknowledgement; the
		// ring must be new and exist before the request is sent.
		b.closeChat(uid, session)
		b.chatLog(uid, session)
	}
	// A fresh request id every call: the machine refuses a reused one as a
	// conflict even when the earlier switch succeeded.
	reply, err := p.callWithin(ctx, terminalproto.Message{RequestID: ID(), SessionID: session, Body: &in}, sessionModeCallTimeout)
	if err == nil {
		err = sessionModeAck(reply)
	}
	if err != nil {
		if to == terminalproto.SessionModeChat {
			// The row is still a terminal; a lost reply is settled by the
			// next reconcile, which restores the ring if the chat started.
			b.closeChat(uid, session)
		}
		return err
	}
	if err := b.st.SetTerminalStructuredFor(uid, session, to == terminalproto.SessionModeChat); err != nil {
		// The row ended or changed meanwhile; it keeps its mode.
		if to == terminalproto.SessionModeChat {
			b.closeChat(uid, session)
		}
		return ErrSessionModeFailed
	}
	if to == terminalproto.SessionModeChat {
		s.Structured = true
		b.restoreChat(uid, s)
	} else {
		b.closeChat(uid, session)
	}
	_ = b.st.AddEventFor(uid, "machine", "session-mode", "", map[string]string{
		"machine": s.AgentID, "tool": s.Tool, "session": session, "mode": to,
	})
	return nil
}

// sessionModeAck maps the machine's answer to a switch: the running session
// is success, a conflict means it was busy (or already switching), and any
// other refusal a sentence.
func sessionModeAck(reply terminalproto.Message) error {
	switch body := reply.Body.(type) {
	case *terminalproto.SessionChanged:
		if body.State == terminalproto.Running {
			return nil
		}
		return ErrSessionModeFailed
	case *terminalproto.Result:
		switch body.Code {
		case terminalproto.Conflict:
			return ErrSessionModeBusy
		case terminalproto.ACPTrustRequired:
			return sessionModeTrustError{trust: &ACPTrustError{Detail: body.Detail}}
		}
		if refusal := acpRefusal(body.Code, body.Detail); refusal != nil {
			return refusal
		}
	}
	return ErrSessionModeFailed
}

// adoptSessionMode makes a running row's mode follow the machine's reconcile
// snapshot: a reported OpenCode session id is a chat, and on a connection
// that negotiated session-mode-v1 a structured row with a known id reported
// without one is the OpenCode TUI. Closed rows are never touched, and the
// stored acp_session_id is never cleared. It returns the row as adopted.
func (p *Peer) adoptSessionMode(s store.TerminalSession, observed terminalproto.SessionSnapshot, sessionMode bool) store.TerminalSession {
	if observed.State != terminalproto.Running {
		return s
	}
	var structured bool
	switch {
	case observed.ACPSessionID != "" && !s.Structured:
		structured = true
	case observed.ACPSessionID == "" && s.Structured && s.ACPSessionID != "" && sessionMode:
		structured = false
	default:
		return s
	}
	if err := p.b.st.SetTerminalStructuredFor(p.key.uid, s.ID, structured); err != nil {
		return s
	}
	s.Structured = structured
	if !structured {
		p.b.closeChat(p.key.uid, s.ID)
	}
	return s
}

// adoptLateChat settles a row whose switch to chat was answered after the
// call gave up: a running answer with an OpenCode session id, for a running
// terminal row on a connection that negotiated session-mode-v1, means the
// machine runs the chat. Closed rows and the stored id are never touched.
func (p *Peer) adoptLateChat(session string, changed *terminalproto.SessionChanged) {
	if changed.State != terminalproto.Running || changed.ACPSessionID == "" {
		return
	}
	s, err := p.b.st.TerminalSessionFor(p.key.uid, session)
	if err != nil || s.AgentID != p.key.agent || s.State != terminalproto.Running || s.Structured || s.ACPSessionID != changed.ACPSessionID {
		return
	}
	caps, err := p.b.st.RemoteCapabilitiesFor(p.key.uid, p.key.agent)
	if err != nil || caps.Generation != p.Generation || !caps.Capabilities.SessionMode {
		return
	}
	if p.b.st.SetTerminalStructuredFor(p.key.uid, session, true) != nil {
		return
	}
	s.Structured = true
	p.b.restoreChat(p.key.uid, s)
}
