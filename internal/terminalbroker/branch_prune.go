package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// BranchPrunePlanCallTimeout is the controller's deadline for a plan. The
	// machine answers by 22 s even when git hangs; this leaves room for that
	// answer to arrive.
	BranchPrunePlanCallTimeout = 25 * time.Second
	// BranchPruneCallTimeout does the same for a prune. The machine plans and
	// deletes inside one 20 s budget and answers by 22 s.
	BranchPruneCallTimeout = 25 * time.Second
)

var (
	// ErrBranchPruneUnsupported means the machine did not negotiate
	// branch-prune-v1 (an older reet there); nothing was sent.
	ErrBranchPruneUnsupported = errors.New("machine does not support branch pruning")
	// ErrBranchPruneBusy means the machine is already planning or pruning on
	// this connection and answered conflict at once.
	ErrBranchPruneBusy = errors.New("machine is already pruning branches")
	// ErrBranchPruneDenied means a current machine does not offer branch
	// pruning now (terminal access is off there).
	ErrBranchPruneDenied = errors.New("machine does not offer branch pruning now")
)

// BranchPruneError is a plan or prune the machine refused for the folder or
// the base it was given: not_repo_root, outside_home, git_failed or denied.
type BranchPruneError struct{ Reason terminalproto.BranchesReason }

func (e *BranchPruneError) Error() string { return "branch prune refused: " + string(e.Reason) }

// CheckBranchPrunePlan applies the protocol's own rules to a plan request.
func CheckBranchPrunePlan(req terminalproto.BranchPrunePlan) error {
	return validWorktreeRequest(req)
}

// CheckBranchPrune applies the protocol's own rules to a prune request.
func CheckBranchPrune(req terminalproto.BranchPrune) error { return validWorktreeRequest(req) }

// BranchPrunePlan asks the machine which local branches of the repository at
// req.Dir are safe to delete. It is sent only to a connection that reported
// the branch-prune capability, and nothing of it is stored.
func (b *Broker) BranchPrunePlan(ctx context.Context, uid int64, agent string, req terminalproto.BranchPrunePlan) (terminalproto.BranchPrunePlanResult, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.BranchPrunePlanResult{}, err
	}
	if !caps.BranchPrune {
		return terminalproto.BranchPrunePlanResult{}, ErrBranchPruneUnsupported
	}
	body, err := p.ask(ctx, req, BranchPrunePlanCallTimeout)
	if err != nil {
		return terminalproto.BranchPrunePlanResult{}, err
	}
	r, ok := body.(*terminalproto.BranchPrunePlanResult)
	if !ok {
		return terminalproto.BranchPrunePlanResult{}, ErrMachineAnswer
	}
	switch r.Reason {
	case "":
		return *r, nil
	case terminalproto.BranchesConflict:
		return terminalproto.BranchPrunePlanResult{}, ErrBranchPruneBusy
	case terminalproto.BranchesDenied:
		return terminalproto.BranchPrunePlanResult{}, ErrBranchPruneDenied
	}
	return terminalproto.BranchPrunePlanResult{}, &BranchPruneError{Reason: r.Reason}
}

// PruneBranches asks the machine to delete the branches named in req that are
// still safe and still at the tip shown. It is sent only to a connection that
// reported the branch-prune capability.
func (b *Broker) PruneBranches(ctx context.Context, uid int64, agent string, req terminalproto.BranchPrune) (terminalproto.BranchPruneResult, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.BranchPruneResult{}, err
	}
	if !caps.BranchPrune {
		return terminalproto.BranchPruneResult{}, ErrBranchPruneUnsupported
	}
	body, err := p.ask(ctx, req, BranchPruneCallTimeout)
	if err != nil {
		return terminalproto.BranchPruneResult{}, err
	}
	r, ok := body.(*terminalproto.BranchPruneResult)
	if !ok {
		return terminalproto.BranchPruneResult{}, ErrMachineAnswer
	}
	switch r.Reason {
	case "":
		return *r, nil
	case terminalproto.BranchesConflict:
		return terminalproto.BranchPruneResult{}, ErrBranchPruneBusy
	case terminalproto.BranchesDenied:
		return terminalproto.BranchPruneResult{}, ErrBranchPruneDenied
	}
	return terminalproto.BranchPruneResult{}, &BranchPruneError{Reason: r.Reason}
}
