package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// BranchSyncCallTimeout is the controller's deadline for a sync. The machine
// answers by 48 s even when git hangs; this leaves room for that answer.
var BranchSyncCallTimeout = 52 * time.Second

// ErrBranchSyncUnsupported means the machine did not negotiate branch-sync-v1
// (an older reet there); nothing was sent.
var ErrBranchSyncUnsupported = errors.New("machine does not support branch sync")

// BranchSyncError is a sync the machine refused for the folder it was given:
// not_repo_root, outside_home, git_failed, conflict (busy) or denied.
type BranchSyncError struct{ Reason terminalproto.BranchesReason }

func (e *BranchSyncError) Error() string { return "branch sync refused: " + string(e.Reason) }

// SyncBranch asks the machine to fast-forward the base branch of the
// repository at req.Dir. It is sent only to a connection that reported the
// branch-sync capability, and nothing of it is stored.
func (b *Broker) SyncBranch(ctx context.Context, uid int64, agent string, req terminalproto.BranchSync) (terminalproto.BranchSyncResult, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.BranchSyncResult{}, err
	}
	if !caps.BranchSync {
		return terminalproto.BranchSyncResult{}, ErrBranchSyncUnsupported
	}
	body, err := p.ask(ctx, req, BranchSyncCallTimeout)
	if err != nil {
		return terminalproto.BranchSyncResult{}, err
	}
	r, ok := body.(*terminalproto.BranchSyncResult)
	if !ok {
		return terminalproto.BranchSyncResult{}, ErrMachineAnswer
	}
	if r.Reason != "" {
		return terminalproto.BranchSyncResult{}, &BranchSyncError{Reason: r.Reason}
	}
	return *r, nil
}
