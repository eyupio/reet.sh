//go:build linux || darwin

package terminalagent

import (
	"context"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (r *agentReq) handleAccounts(op *terminalproto.Accounts) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.AccountsResult{Reason: terminalproto.AccountsOff}
		return r.send(callCtx, r.reply)
	}
	req, reader := *op, accountReader(opts.Home)
	return r.aside(callCtx, r.reply, accountsSlot, &accountsRunning, accountsAnswerAfter,
		terminalproto.AccountsResult{Reason: terminalproto.AccountsConflict}, terminalproto.AccountsResult{Reason: terminalproto.AccountsTimedOut},
		func(ctx context.Context) terminalproto.Body {
			return terminalproto.AccountsResult{Accounts: reader.Read(ctx, req.Limits, req.Refresh)}
		})
}
