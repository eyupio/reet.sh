package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// BaseCheckCallTimeout is the controller's deadline for a base check. The
// machine answers by 36 s even when its fetch hangs; this leaves room for
// that answer.
var BaseCheckCallTimeout = 40 * time.Second

// ErrBaseCheckUnsupported means the machine did not negotiate base-fresh-v1
// (an older reet there); nothing was sent.
var ErrBaseCheckUnsupported = errors.New("machine does not support base checks")

// BaseCheckError is a check the machine refused: not_repo_root, outside_home,
// git_failed, denied, or base_missing when neither the repository nor GitHub
// has the branch. A busy machine is ErrBranchPruneBusy instead.
type BaseCheckError struct{ Reason terminalproto.BranchesReason }

func (e *BaseCheckError) Error() string { return "base check refused: " + string(e.Reason) }

// CheckBaseCheck applies the protocol's own rules to a base check, so a
// caller can refuse a malformed one before anything else.
func CheckBaseCheck(req terminalproto.BaseCheck) error { return validWorktreeRequest(req) }

// CheckBase asks the machine where a worktree for req.Base would start in the
// repository at req.Dir. It makes nothing: the machine reads the two sides and
// answers. It is sent only to a connection that reported the base-fresh
// capability, and nothing of the answer is stored.
func (b *Broker) CheckBase(ctx context.Context, uid int64, agent string, req terminalproto.BaseCheck) (terminalproto.BaseState, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.BaseState{}, err
	}
	if !caps.BaseFresh {
		return terminalproto.BaseState{}, ErrBaseCheckUnsupported
	}
	body, err := p.ask(ctx, req, BaseCheckCallTimeout)
	if err != nil {
		return terminalproto.BaseState{}, err
	}
	r, ok := body.(*terminalproto.BaseCheckResult)
	if !ok {
		return terminalproto.BaseState{}, ErrMachineAnswer
	}
	switch r.Reason {
	case "":
		return r.State, nil
	case terminalproto.BranchesConflict:
		// The machine is already running git over a repository: it shares one
		// slot with a prune and a sync, so it is busy, not broken.
		return terminalproto.BaseState{}, ErrBranchPruneBusy
	}
	return terminalproto.BaseState{}, &BaseCheckError{Reason: r.Reason}
}
