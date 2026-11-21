//go:build linux || darwin

package terminalagent

import (
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// handleWorktree serves a worktree status or remove. They are served only on a
// generation that negotiated worktree-v1, and are always answered with a
// worktree_state or worktree_result.
func (r *agentReq) handleWorktree() error {
	msg, callCtx := r.msg, r.callCtx
	r.reply.Body = terminalproto.WorktreeResult{Code: terminalproto.Denied}
	if !r.features.Has(terminalproto.WorktreeFeature) {
		return r.send(callCtx, r.reply)
	}
	if r.managerErr != nil {
		r.reply.Body = terminalproto.WorktreeResult{Code: terminalproto.Unavailable}
		return r.send(callCtx, r.reply)
	}
	select {
	case r.worktreeSlot <- struct{}{}:
	default:
		r.reply.Body = terminalproto.WorktreeResult{Code: terminalproto.Conflict}
		return r.send(callCtx, r.reply)
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		body := answerWorktree(callCtx, r.m, msg.Body)
		// Free the slot before answering, so a request the
		// controller sends once it has this answer is not busy.
		<-r.worktreeSlot
		// A request whose connection has ended answers nothing: the
		// controller has already given up on it and reads status again.
		if callCtx.Err() != nil {
			return
		}
		r.reply.Body = body
		_ = r.send(callCtx, r.reply)
	}()
	return nil
}
