package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// AccountsCallTimeout is the controller's deadline for a sign-in read. The
// machine asks its agents' providers side by side, ten seconds at most, and
// answers timed_out at twelve; this leaves room for that answer.
var AccountsCallTimeout = 14 * time.Second

// ErrAccountsUnsupported means the machine did not report the agent-accounts
// capability: an older reet there, or terminal access off; nothing was sent.
var ErrAccountsUnsupported = errors.New("machine does not report its agents' sign-ins")

// Accounts asks a machine which of its agents are signed in and, when
// req.Limits is set, how much of each plan's allowance is left. It is sent
// only to a connection that reported the capability. The answer is returned
// as it came, a refusal as its Reason, for the caller to word and store;
// nothing is stored here, and no credential passes through: the machine uses
// its agents' sign-ins itself.
func (b *Broker) Accounts(ctx context.Context, uid int64, agent string, req terminalproto.Accounts) (terminalproto.AccountsResult, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.AccountsResult{}, err
	}
	if !caps.AgentAccounts {
		return terminalproto.AccountsResult{}, ErrAccountsUnsupported
	}
	body, err := p.ask(ctx, req, AccountsCallTimeout)
	if err != nil {
		return terminalproto.AccountsResult{}, err
	}
	r, ok := body.(*terminalproto.AccountsResult)
	if !ok {
		return terminalproto.AccountsResult{}, ErrMachineAnswer
	}
	return *r, nil
}
