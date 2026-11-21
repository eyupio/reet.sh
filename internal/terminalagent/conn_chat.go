//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// handleChatCreate starts a structured chat. The caller holds the session id
// in r.held; the goroutine takes the hold over.
func (r *agentReq) handleChatCreate(op *terminalproto.Create) error {
	msg, callCtx, chats := r.msg, r.callCtx, r.chats
	// Starting ACP probes a version and opens a session; keep the
	// control reader answering health probes while it runs. A
	// slow start must never look like a lost machine.
	if !chats.sup.addWork() {
		return r.respond(terminalhost.ErrManagerClosed)
	}
	r.held = false
	go func(in terminalproto.Create) {
		defer chats.sup.wg.Done()
		defer chats.sup.unhold(msg.SessionID)
		changed, err := chats.open(msg.RequestID, msg.SessionID, in, r.features.Has(terminalproto.BaseFreshFeature))
		if callCtx.Err() != nil {
			return
		}
		if err != nil {
			r.reply.Body = chatResult(err)
		} else {
			r.reply.Body = changed.ForFeatures(r.featureHeader)
		}
		_ = r.send(callCtx, r.reply)
	}(*op)
	return nil
}

func (r *agentReq) handleChatReattach(op *terminalproto.ChatReattach) error {
	msg, callCtx, chats := r.msg, r.callCtx, r.chats
	// A reattach starts OpenCode with session/load; answer from its
	// own goroutine like a structured create. The start is bounded
	// by openChat's 60 s timeout on the supervisor's context, so a
	// connection lost meanwhile does not kill a chat the controller
	// can adopt by sending the reattach again.
	if !chats.sup.addWork() {
		return r.respond(terminalhost.ErrManagerClosed)
	}
	r.held = false
	go func(in terminalproto.ChatReattach) {
		defer chats.sup.wg.Done()
		defer chats.sup.unhold(msg.SessionID)
		changed, err := chats.reattach(msg.RequestID, msg.SessionID, in)
		if callCtx.Err() != nil {
			return
		}
		if err != nil {
			r.reply.Body = chatResult(err)
			if errors.Is(err, errReattachStarting) {
				r.reply.Body = resultFor(err, r.featureHeader)
			}
		} else {
			r.reply.Body = changed.ForFeatures(r.featureHeader)
		}
		_ = r.send(callCtx, r.reply)
	}(*op)
	return nil
}

// handleChatEnd closes a chat.
func (r *agentReq) handleChatEnd() error {
	msg, callCtx, chats := r.msg, r.callCtx, r.chats
	// Closing a chat waits for its process group; answer from
	// its own goroutine so the connection stays responsive.
	if !chats.sup.addWork() {
		return r.respond(terminalhost.ErrManagerClosed)
	}
	r.held = false
	go func() {
		defer chats.sup.wg.Done()
		defer chats.sup.unhold(msg.SessionID)
		changed, err := chats.end(msg.SessionID)
		if callCtx.Err() != nil {
			return
		}
		if err != nil {
			r.reply.Body = result(err)
		} else {
			r.reply.Body = changed
		}
		_ = r.send(callCtx, r.reply)
	}()
	return nil
}

func (r *agentReq) handleNativeView() error {
	_, available := r.chats.nativeTerminal(r.msg.SessionID)
	available = available && r.m != nil
	// The controller words the refusal; the machine only reports
	// whether it has a native view to offer.
	r.reply.Body = terminalproto.NativeViewResult{Available: available}
	return r.send(r.callCtx, r.reply)
}

func (r *agentReq) handleACPPrompt(op *terminalproto.ACPPrompt) error {
	if err := r.chats.prompt(r.msg.SessionID, op.Text, op.Images); err != nil {
		return r.respond(err)
	}
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.send(r.callCtx, r.reply)
}

func (r *agentReq) handleACPPreview(op *terminalproto.ACPPreview) error {
	callCtx := r.callCtx
	if !allowed(r.opts.Consent) {
		r.reply.Body = terminalproto.Result{Code: terminalproto.ACPUnavailable}
		return r.send(callCtx, r.reply)
	}
	answer, err := r.chats.sup.previews.read(r.msg.SessionID, *op)
	if err != nil {
		r.reply.Body = terminalproto.Result{Code: terminalproto.ACPUnavailable}
	} else {
		r.reply.Body = answer
	}
	return r.send(callCtx, r.reply)
}

func (r *agentReq) handleACPControl(op *terminalproto.ACPControl) error {
	msg, callCtx, chats := r.msg, r.callCtx, r.chats
	if controlsRunning.Add(1) > maxAsideRunning {
		controlsRunning.Add(-1)
		return r.respond(terminalhost.ErrOperationConflict)
	}
	if !chats.sup.addWork() {
		controlsRunning.Add(-1)
		return r.respond(terminalhost.ErrManagerClosed)
	}
	operation := *op
	go func() {
		defer controlsRunning.Add(-1)
		defer chats.sup.wg.Done()
		controlCtx, cancel := context.WithTimeout(callCtx, 8*time.Second)
		defer cancel()
		answer, err := chats.control(controlCtx, msg.SessionID, operation)
		if err != nil {
			r.reply.Body = result(err)
		} else {
			r.reply.Body = answer
		}
		_ = r.send(callCtx, r.reply)
	}()
	return nil
}

func (r *agentReq) handleACPModel(op *terminalproto.ACPModel) error {
	if err := r.chats.setModel(r.callCtx, r.msg.SessionID, op.Model); err != nil {
		return r.respond(err)
	}
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.send(r.callCtx, r.reply)
}

func (r *agentReq) handleACPCancel() error {
	if err := r.chats.cancel(r.msg.SessionID); err != nil {
		return r.respond(err)
	}
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.send(r.callCtx, r.reply)
}

func (r *agentReq) handleACPPermission(op *terminalproto.ACPPermission) error {
	if err := r.chats.permission(r.msg.SessionID, op.PermissionID, op.Allow); err != nil {
		return r.respond(err)
	}
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.send(r.callCtx, r.reply)
}

func (r *agentReq) handleACPQuestionAnswer(op *terminalproto.ACPQuestionAnswer) error {
	answer, err := r.chats.question(r.msg.SessionID, op)
	if err != nil {
		return r.respond(err)
	}
	r.reply.Body = answer
	return r.send(r.callCtx, r.reply)
}

func (r *agentReq) handleACPAgentMode(op *terminalproto.ACPAgentMode) error {
	if err := r.chats.setAgentMode(r.callCtx, r.msg.SessionID, op.Mode); err != nil {
		return r.respond(err)
	}
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.send(r.callCtx, r.reply)
}

func (r *agentReq) handleACPMode(op *terminalproto.ACPMode) error {
	if err := r.chats.mode(r.msg.SessionID, op.Yolo); err != nil {
		return r.respond(err)
	}
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.send(r.callCtx, r.reply)
}
