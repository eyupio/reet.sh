//go:build linux || darwin

package terminalagent

import (
	"context"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// handleSessionMode swaps a session between the structured chat and the tool's
// own terminal.
func (r *agentReq) handleSessionMode(op *terminalproto.SessionMode) error {
	msg, callCtx, chats := r.msg, r.callCtx, r.chats
	// A swap stops one process and starts another; answer from its
	// own goroutine like a structured create. It runs to the end
	// (or its rollback) even if this connection is lost meanwhile.
	if !chats.sup.addWork() {
		return r.respond(terminalhost.ErrManagerClosed)
	}
	r.swaps.Add(1)
	go func(in terminalproto.SessionMode) {
		defer chats.sup.wg.Done()
		defer r.swaps.Done()
		swapCtx, cancel := context.WithTimeout(context.WithoutCancel(callCtx), sessionModeTimeout)
		defer cancel()
		changed, err := r.switcher.swap(swapCtx, msg.RequestID, msg.SessionID, in)
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

// handleReconcile answers the complete snapshot of this machine's terminal and
// chat sessions.
func (r *agentReq) handleReconcile(op *terminalproto.Reconcile) error {
	callCtx := r.callCtx
	if op.Complete || len(op.Sessions) != 0 {
		return terminalproto.ErrInvalid
	}
	if r.m != nil {
		if err := r.m.Reconcile(callCtx); err != nil {
			return r.respond(err)
		}
	}
	snapshot, err := r.switcher.reconcile(callCtx, r.featureHeader)
	if err != nil {
		return r.respond(err)
	}
	r.reply.Body = snapshot
	return r.respond(nil)
}
