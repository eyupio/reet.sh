//go:build linux || darwin

package terminalagent

import (
	"context"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (r *agentReq) handleInspect(op *terminalproto.Inspect) error {
	callCtx, opts := r.callCtx, r.opts
	browseView := op.View == "files" || op.View == "file"
	denied := !allowed(opts.Consent)
	browseDenied := browseView && !allowed(opts.BrowseConsent)
	if denied || browseDenied {
		r.reply.Body = terminalproto.InspectResult{Reason: "denied"}
		return r.send(callCtx, r.reply)
	}
	req, inspect := *op, inspectWorkspace
	// A slow GitHub lookup must not hold local status or file reads.
	slot := r.inspectSlot
	if req.View == "github" {
		slot = r.inspectGitHubSlot
	}
	return r.aside(callCtx, r.reply, slot, &inspectsRunning, 14*time.Second,
		terminalproto.InspectResult{Reason: "busy"}, terminalproto.InspectResult{Reason: "timed_out"},
		func(ctx context.Context) terminalproto.Body { return inspect(ctx, opts.Home, req) })
}

func (r *agentReq) handleBrowse(op *terminalproto.Browse) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) || !allowed(opts.BrowseConsent) {
		r.reply.Body = terminalproto.BrowseResult{Reason: terminalproto.BrowseOff}
		return r.send(callCtx, r.reply)
	}
	req, home, fn := *op, opts.Home, browse
	return r.aside(callCtx, r.reply, r.browseSlot, &browsesRunning, browseAnswerAfter,
		terminalproto.BrowseResult{Reason: terminalproto.BrowseConflict}, terminalproto.BrowseResult{Reason: terminalproto.BrowseUnreadable},
		func(ctx context.Context) terminalproto.Body {
			return fn(ctx, home, req, terminalhost.OSFS, time.Now)
		})
}
