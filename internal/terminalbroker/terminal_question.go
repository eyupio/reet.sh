package terminalbroker

import (
	"context"
	"errors"
	"strconv"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// ErrTerminalQuestionUnsupported means the machine did not negotiate
	// terminal-question-v1.
	ErrTerminalQuestionUnsupported = errors.New("Update reet on this machine to answer from here.")
	// ErrQuestionChanged means the screen no longer shows the question the
	// answer named.
	ErrQuestionChanged = errors.New("The question on the terminal changed. Look at it again before answering.")
	// ErrAlreadyAnswered means an option was already chosen for that question.
	ErrAlreadyAnswered = errors.New("That question was already answered.")
	// ErrAnswerBusy means another answer for the session is on its way.
	ErrAnswerBusy = errors.New("An answer to this question is already on its way.")
)

// noteQuestion keeps the question a machine reported for a running terminal
// session, or forgets it when the report has none. It lives on the
// connection, in memory only: it is never written to the store, and it goes
// with the connection. The machine sends it again when it reconnects.
func (p *Peer) noteQuestion(session string, state terminalproto.SessionState, q *terminalproto.TerminalQuestion) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if q == nil || state != terminalproto.Running {
		delete(p.questions, session)
		delete(p.answered, session)
		return
	}
	if p.questions == nil {
		p.questions = map[string]terminalproto.TerminalQuestion{}
	}
	p.questions[session] = *q
	if p.answered[session] != q.Fingerprint {
		delete(p.answered, session)
	}
}

// ApplyTerminalQuestion overlays the question a running terminal session's
// agent is waiting on, as its machine last reported it. While it waits the
// session is waiting, as a chat with a pending question is: having looked at
// the session does not answer it. Without one the stored state stands.
func (b *Broker) ApplyTerminalQuestion(uid int64, s *store.TerminalSession) {
	if s == nil || s.Structured || s.State != terminalproto.Running {
		return
	}
	b.mu.Lock()
	p := b.peers[key{uid, s.AgentID}]
	b.mu.Unlock()
	if p == nil {
		return
	}
	select {
	case <-p.done:
		return
	default:
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if q, ok := p.questions[s.ID]; ok {
		q.Options = append([]terminalproto.TerminalOption(nil), q.Options...)
		s.TerminalQuestion = &q
		s.Attention, s.AttentionSource = "waiting", "terminal"
	}
}

// AnswerTerminalQuestion asks the machine to choose option on the question
// with this fingerprint in the caller's own terminal session. The machine
// reads its screen again and works out the keys; nothing typed here reaches
// the terminal. One answer is on its way per session at a time, and a repeat
// of an answer that succeeded is ErrAlreadyAnswered without asking the
// machine again. Only the option number is audited, never the question.
func (b *Broker) AnswerTerminalQuestion(ctx context.Context, uid int64, session, fingerprint string, option int) error {
	if !terminalproto.ValidID(session) || !terminalproto.ValidFingerprint(fingerprint) || option < 1 || option > terminalproto.MaxTerminalOptions {
		return terminalproto.ErrInvalid
	}
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return err
	}
	if s.Structured {
		return store.ErrRemoteNotFound
	}
	if s.State != terminalproto.Running {
		return ErrQuestionChanged
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
	if err != nil || caps.Generation != p.Generation {
		return ErrOffline
	}
	if !caps.Capabilities.TerminalQuestion {
		return ErrTerminalQuestionUnsupported
	}
	p.mu.Lock()
	switch {
	case p.answered[session] == fingerprint:
		p.mu.Unlock()
		return ErrAlreadyAnswered
	case p.answering[session]:
		p.mu.Unlock()
		return ErrAnswerBusy
	}
	if p.answering == nil {
		p.answering = map[string]bool{}
	}
	p.answering[session] = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.answering, session)
		p.mu.Unlock()
	}()
	reply, err := p.callWithin(ctx, terminalproto.Message{RequestID: ID(), SessionID: session,
		Body: &terminalproto.TerminalAnswer{Fingerprint: fingerprint, Option: option}}, chatCallTimeout)
	if err != nil {
		return err
	}
	result, ok := reply.Body.(*terminalproto.Result)
	if !ok {
		return ErrOffline
	}
	switch result.Code {
	case terminalproto.OK, terminalproto.AlreadyAnswered:
		p.mu.Lock()
		if p.answered == nil {
			p.answered = map[string]string{}
		}
		p.answered[session] = fingerprint
		if p.questions[session].Fingerprint == fingerprint {
			delete(p.questions, session)
		}
		p.mu.Unlock()
		if result.Code == terminalproto.AlreadyAnswered {
			return ErrAlreadyAnswered
		}
		_ = b.st.AddEventFor(uid, "machine", "terminal-answer", "", map[string]string{
			"machine": s.AgentID, "tool": s.Tool, "session": session, "option": strconv.Itoa(option),
		})
		return nil
	case terminalproto.QuestionChanged, terminalproto.NotFound:
		return ErrQuestionChanged
	case terminalproto.Denied:
		return ErrDenied
	}
	return ErrOffline
}
