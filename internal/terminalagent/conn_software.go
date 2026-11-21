//go:build linux || darwin

package terminalagent

import (
	"context"
	"os/exec"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (r *agentReq) handleSoftwareApply(op *terminalproto.SoftwareApply) error {
	callCtx, opts, m, chats := r.callCtx, r.opts, r.m, r.chats
	if op.Tool != "opencode" || !terminalproto.ValidToolV2(op.Tool) {
		return terminalproto.ErrInvalid
	}
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.SoftwareApplyResult{Tool: op.Tool, Action: op.Action, State: terminalproto.SoftwareApplyRefused, Reason: terminalproto.SoftwareApplyOff}
		return r.send(callCtx, r.reply)
	}
	req := *op
	return r.aside(callCtx, r.reply, softwareSlot, &softwareRunning, softwareAnswerAfter,
		terminalproto.SoftwareApplyResult{Tool: req.Tool, Action: req.Action, State: terminalproto.SoftwareApplyRefused, Reason: terminalproto.SoftwareApplyBusy},
		terminalproto.SoftwareApplyResult{Tool: req.Tool, Action: req.Action, State: terminalproto.SoftwareApplyFailed, Reason: terminalproto.SoftwareApplyTimedOut},
		func(ctx context.Context) terminalproto.Body {
			// The apply outlives the control connection it arrived on:
			// a lost connection must not leave a half-applied update,
			// and a later reconcile observes the version it left.
			workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), softwareApplyWorkTimeout)
			defer cancel()
			release, err := lockSoftwareWrite(workCtx)
			if err != nil {
				return terminalproto.SoftwareApplyResult{Tool: req.Tool, Action: req.Action, State: terminalproto.SoftwareApplyFailed, Reason: terminalproto.SoftwareApplyTimedOut}
			}
			defer release()
			// The controller checked before sending, but a session could
			// have started since. Nothing else can now: every OpenCode
			// launch waits on this gate.
			live := chats.sup.liveTool(agentsoftware.OpenCodeTool)
			if m != nil {
				if sessions, err := m.List(workCtx); err == nil {
					for _, session := range sessions {
						if session.Tool == agentsoftware.OpenCodeTool && session.State.ReservesQuota() {
							live++
						}
					}
				}
			}
			if live > 0 {
				return terminalproto.SoftwareApplyResult{Tool: req.Tool, Action: req.Action, State: terminalproto.SoftwareApplyRefused, Reason: terminalproto.SoftwareApplySessions}
			}
			return applySoftware(workCtx, opts.Home, req, exec.LookPath, terminalhost.SoftwareSource{
				// The controller may need minutes to fill a cold cache
				// before it sends headers; the shared client gives up
				// after 10 seconds.
				Client: privateconn.WithHeaderTimeout(r.client, mirrorHeaderTimeout),
				Header: mirrorHeaders(r.headers),
				Path:   "/private/v1/software/artifact/" + agentsoftware.OpenCodeLinuxX64.ArchiveSHA256,
			})
		})
}
