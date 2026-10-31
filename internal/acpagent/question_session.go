//go:build linux || darwin

package acpagent

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrNoQuestion means the question is not waiting: it was answered, cancelled,
// timed out, or never existed. A question id is single use.
var ErrNoQuestion = errors.New("that question is no longer waiting")

// QuestionAction is the person's decision. Accept, decline and cancel are
// distinct: declining skips the question and lets the agent continue; cancelling
// aborts the agent's tool call.
type QuestionAction string

const (
	QuestionAccept  QuestionAction = "accept"
	QuestionDecline QuestionAction = "decline"
	QuestionCancel  QuestionAction = "cancel"
)

// QuestionResolution is one answer to a pending question.
type QuestionResolution struct {
	Action  QuestionAction
	Answers map[string]json.RawMessage
}

type pendingQuestion struct {
	rpcID json.RawMessage
	q     Question
	timer *time.Timer
}

func cancelledQuestion() map[string]any { return map[string]any{"action": "cancel"} }

// handleElicitation receives an elicitation/create request. Questions are off
// unless Options.Questions is set, and the form capability is not advertised in
// initialize by this package, so an adapter should never send one; if one
// arrives anyway it is cancelled, never guessed at.
func (s *Session) handleElicitation(rpcID json.RawMessage, raw json.RawMessage) {
	if len(rpcID) == 0 {
		return
	}
	if !s.opts.Questions {
		s.respond(rpcID, cancelledQuestion(), nil)
		return
	}
	q, sessionID, err := parseQuestion(raw)
	s.mu.Lock()
	acpID, closed := s.acpID, s.closed
	s.mu.Unlock()
	switch {
	case closed:
		s.respond(rpcID, cancelledQuestion(), nil)
		return
	case err != nil:
		s.refuseQuestion(rpcID)
		return
	case acpID == "" || sessionID != acpID:
		// A question before the session exists, or for another session, has no
		// admitted owner in this slice; it is refused rather than attached.
		s.refuseQuestion(rpcID)
		return
	}
	id, err := permissionID()
	if err != nil {
		s.respond(rpcID, cancelledQuestion(), nil)
		return
	}
	pending := &pendingQuestion{rpcID: rpcID, q: q}
	s.mu.Lock()
	if s.closed || len(s.questions) >= maxPendingQuestions {
		s.mu.Unlock()
		s.refuseQuestion(rpcID)
		return
	}
	if s.questions == nil {
		s.questions = map[string]*pendingQuestion{}
	}
	s.questions[id] = pending
	s.mu.Unlock()
	pending.timer = time.AfterFunc(s.opts.permissionTimeout(), func() {
		if s.resolveQuestion(id, QuestionResolution{Action: QuestionCancel}, "timeout") == nil {
			s.emit(Event{Kind: "notice", Text: "The question timed out, so " + s.opts.Agent.displayName() + " was stopped.", Bad: true})
			_ = s.sendCancel()
		}
	})
	s.emit(Event{Kind: "status", State: "waiting", Detail: "Waiting for your answer."})
	s.emit(Event{Kind: "question", QuestionID: id, ToolCallID: q.ToolCallID, Question: &q})
}

func (s *Session) refuseQuestion(rpcID json.RawMessage) {
	s.respond(rpcID, cancelledQuestion(), nil)
	s.emit(Event{Kind: "notice", Text: s.opts.Agent.displayName() + " asked a question this version cannot show, so it was cancelled."})
}

// AnswerQuestion resolves a pending question exactly once. An answer that fails
// validation returns an *AnswerError and leaves the question waiting.
func (s *Session) AnswerQuestion(id string, resolution QuestionResolution) error {
	return s.resolveQuestion(id, resolution, "")
}

func (s *Session) resolveQuestion(id string, resolution QuestionResolution, reason string) error {
	s.mu.Lock()
	pending := s.questions[id]
	s.mu.Unlock()
	if pending == nil {
		return ErrNoQuestion
	}
	var result map[string]any
	switch resolution.Action {
	case QuestionAccept:
		content, err := pending.q.Validate(resolution.Answers)
		if err != nil {
			return err // not consumed: the person can correct and resubmit
		}
		result = map[string]any{"action": "accept", "content": content}
	case QuestionDecline:
		result = map[string]any{"action": "decline"}
	case QuestionCancel:
		result = cancelledQuestion()
	default:
		return rejected("", "unknown action")
	}
	// Single use: whoever removes it from the map owns the reply.
	s.mu.Lock()
	if s.questions[id] != pending {
		s.mu.Unlock()
		return ErrNoQuestion
	}
	delete(s.questions, id)
	s.mu.Unlock()
	if pending.timer != nil {
		pending.timer.Stop()
	}
	if err := s.write(map[string]any{"jsonrpc": "2.0", "id": pending.rpcID, "result": result}); err != nil {
		s.emit(Event{Kind: "question_cancelled", QuestionID: id, Reason: "not_sent"})
		return err
	}
	if resolution.Action == QuestionCancel {
		if reason == "" {
			reason = "cancelled"
		}
		s.emit(Event{Kind: "question_cancelled", QuestionID: id, Reason: reason})
	} else {
		s.emit(Event{Kind: "question_resolved", QuestionID: id, Reason: string(resolution.Action)})
	}
	return nil
}

// cancelQuestions cancels every waiting question, replying to the agent when it
// can still hear. It runs when a turn is cancelled or the session closes, so a
// question never outlives the turn that asked it.
func (s *Session) cancelQuestions(reason string) {
	s.mu.Lock()
	ids := make([]string, 0, len(s.questions))
	for id := range s.questions {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.resolveQuestion(id, QuestionResolution{Action: QuestionCancel}, reason)
	}
}

// dropQuestionsAfterExit forgets questions of an agent that has already exited:
// there is nobody to reply to, but the person's card must still close.
func (s *Session) dropQuestionsAfterExit() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.questions))
	for id, pending := range s.questions {
		if pending.timer != nil {
			pending.timer.Stop()
		}
		ids = append(ids, id)
	}
	s.questions = nil
	return ids
}
