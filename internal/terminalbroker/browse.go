package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// BrowseCallTimeout is the controller's deadline for a browse. The machine
	// answers by 4 s even when a mount hangs (see docs/folder-browse.md).
	BrowseCallTimeout = 6 * time.Second
	// BranchesCallTimeout is the controller's deadline for a branch listing.
	// The machine answers by 8 s (git's 5 s, its 2 s to exit, and a margin),
	// so the answer it gives at its own deadline still arrives in time.
	BranchesCallTimeout = 10 * time.Second
)

var (
	// ErrBrowseUnsupported means the machine did not negotiate browse-v1
	// (an older reet there); nothing was sent.
	ErrBrowseUnsupported = errors.New("machine does not support folder browse")
	// ErrBranchesUnsupported means the machine did not negotiate branches-v1.
	ErrBranchesUnsupported = errors.New("machine does not support branch listing")
	// ErrBrowseOff means a current machine does not offer browse: its
	// folder-browse switch is off (it reports browseOff, or answered off).
	ErrBrowseOff = errors.New("machine has folder browse switched off")
	// ErrBranchesUnavailable means a current machine does not offer branch
	// listing now (terminal access off there, no git, or no worktrees).
	ErrBranchesUnavailable = errors.New("machine does not offer branch listing now")
	// ErrBrowseBusy means the machine is already running a browse on this
	// connection and answered conflict at once.
	ErrBrowseBusy = errors.New("machine is already listing a folder")
	// ErrBranchesBusy is ErrBrowseBusy for a branch listing.
	ErrBranchesBusy = errors.New("machine is already listing branches")
	// ErrMachineTimeout means the request was sent on a live connection but
	// no answer came by the deadline. An answer that comes later is dropped.
	ErrMachineTimeout = errors.New("machine did not answer in time")
	// ErrMachineAnswer means the machine answered with a reply of another
	// kind than the request's; nothing of it is used.
	ErrMachineAnswer = errors.New("machine sent an unexpected answer")
)

// BrowseError is a browse the machine refused for the folder asked about:
// not_found, not_dir, outside_home, denied (the folder cannot be read) or
// unreadable.
type BrowseError struct{ Reason terminalproto.BrowseReason }

func (e *BrowseError) Error() string { return "browse refused: " + string(e.Reason) }

// BranchesError is a branch listing the machine refused: not_repo_root,
// outside_home, git_failed or denied.
type BranchesError struct{ Reason terminalproto.BranchesReason }

func (e *BranchesError) Error() string { return "branch listing refused: " + string(e.Reason) }

// CheckBrowse applies the protocol's own rules to a browse request, so a
// caller can refuse a malformed one before anything else.
func CheckBrowse(req terminalproto.Browse) error { return validWorktreeRequest(req) }

// CheckBranches applies the protocol's own rules to a branches request.
func CheckBranches(req terminalproto.Branches) error { return validWorktreeRequest(req) }

// LateReplies counts browse and branches answers that came after their
// request gave up (or for no request at all) and were dropped.
func (b *Broker) LateReplies() int64 { return b.lateReplies.Load() }

// machinePeer returns a ready peer and the capabilities its current
// connection reported; the request is validated before any of that.
func (b *Broker) machinePeer(uid int64, agent string, body terminalproto.Body) (*Peer, terminalproto.Capabilities, error) {
	if err := validWorktreeRequest(body); err != nil {
		return nil, terminalproto.Capabilities{}, err
	}
	p, err := b.peer(uid, agent, true)
	if err != nil {
		return nil, terminalproto.Capabilities{}, err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, agent)
	if err != nil || caps.Generation != p.Generation {
		return nil, terminalproto.Capabilities{}, ErrOffline
	}
	return p, caps.Capabilities, nil
}

// ask sends one read-only request and waits for its answer. A request whose
// connection ends is ErrOffline; one that is not answered in time on a live
// connection is ErrMachineTimeout. Answers are matched to requests by their
// request id only, so a late answer never reaches another request.
func (p *Peer) ask(ctx context.Context, body terminalproto.Body, limit time.Duration) (terminalproto.Body, error) {
	reply, err := p.callWithin(ctx, terminalproto.Message{Body: body}, limit)
	if errors.Is(err, errReplyLost{}) {
		select {
		case <-p.done:
			return nil, ErrOffline
		default:
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrMachineTimeout
	}
	if err != nil {
		return nil, err
	}
	return reply.Body, nil
}

// Browse lists one level of folders on the machine. It is sent only to a
// connection that reported the browse capability (browse-v1 negotiated and
// browsing on there), and nothing of it is stored.
func (b *Broker) Browse(ctx context.Context, uid int64, agent string, req terminalproto.Browse) (terminalproto.BrowseResult, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.BrowseResult{}, err
	}
	if !caps.Browse {
		// A connector that negotiated browse-v1 but has it switched off says
		// so; without either, it is an older reet.
		if caps.BrowseOff {
			return terminalproto.BrowseResult{}, ErrBrowseOff
		}
		return terminalproto.BrowseResult{}, ErrBrowseUnsupported
	}
	body, err := p.ask(ctx, req, BrowseCallTimeout)
	if err != nil {
		return terminalproto.BrowseResult{}, err
	}
	r, ok := body.(*terminalproto.BrowseResult)
	if !ok {
		return terminalproto.BrowseResult{}, ErrMachineAnswer
	}
	switch r.Reason {
	case "":
		return *r, nil
	case terminalproto.BrowseConflict:
		return terminalproto.BrowseResult{}, ErrBrowseBusy
	case terminalproto.BrowseOff:
		return terminalproto.BrowseResult{}, ErrBrowseOff
	}
	return terminalproto.BrowseResult{}, &BrowseError{Reason: r.Reason}
}

// Branches lists the branches of the repository at req.Dir on the machine.
// It is sent only to a connection that reported the branches capability, and
// nothing of it is stored.
func (b *Broker) Branches(ctx context.Context, uid int64, agent string, req terminalproto.Branches) (terminalproto.BranchesResult, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.BranchesResult{}, err
	}
	if !caps.Branches {
		// branches-v1 shipped with browse-v1: a connector that reports
		// either browse capability negotiated it and does not offer it now.
		if caps.Browse || caps.BrowseOff {
			return terminalproto.BranchesResult{}, ErrBranchesUnavailable
		}
		return terminalproto.BranchesResult{}, ErrBranchesUnsupported
	}
	body, err := p.ask(ctx, req, BranchesCallTimeout)
	if err != nil {
		return terminalproto.BranchesResult{}, err
	}
	r, ok := body.(*terminalproto.BranchesResult)
	if !ok {
		return terminalproto.BranchesResult{}, ErrMachineAnswer
	}
	switch r.Reason {
	case "":
		return *r, nil
	case terminalproto.BranchesConflict:
		return terminalproto.BranchesResult{}, ErrBranchesBusy
	}
	return terminalproto.BranchesResult{}, &BranchesError{Reason: r.Reason}
}
