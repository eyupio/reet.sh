//go:build linux || darwin

package terminalagent

import (
	"context"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (r *agentReq) handleModels(op *terminalproto.Models) error {
	callCtx, opts, m := r.callCtx, r.opts, r.m
	if !terminalproto.ValidToolV2(op.Tool) {
		return terminalproto.ErrInvalid
	}
	if !allowed(opts.Consent) || !allowed(opts.ModelsConsent) {
		r.reply.Body = terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsOff}
		return r.send(callCtx, r.reply)
	}
	if !terminalhost.ListsModels(op.Tool) {
		r.reply.Body = terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsUnsupported}
		return r.send(callCtx, r.reply)
	}
	if r.managerErr != nil || m == nil {
		// The host manager could not start (for example tmux is
		// missing), so no listing can run; that is a failure, not
		// the local switch being off.
		r.reply.Body = terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsExited}
		return r.send(callCtx, r.reply)
	}
	r.modelsMu.Lock()
	cached, seen := r.modelsLast[op.Tool]
	r.modelsMu.Unlock()
	// The floor answers the previous result instead of running the
	// listing again, whether that result was a list or a failure. A
	// refresh is a person asking now, so it always runs.
	if !op.Refresh && seen && time.Since(cached.at) < modelListFloor {
		r.reply.Body = cached.result
		return r.send(callCtx, r.reply)
	}
	tool, fn := op.Tool, listModels
	return r.aside(callCtx, r.reply, r.modelsSlot, &modelsRunning, modelsAnswerAfter,
		terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsConflict},
		terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsTimedOut},
		func(ctx context.Context) terminalproto.Body {
			res := fn(m, ctx, tool)
			r.modelsMu.Lock()
			r.modelsLast[tool] = modelListCacheEntry{result: res, at: time.Now()}
			r.modelsMu.Unlock()
			return res
		})
}
