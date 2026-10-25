package terminalbroker

import (
	"context"
	"errors"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// ErrWorktreeUnsupported means the machine did not negotiate worktree-v1:
	// no worktree field or request was sent and nothing was reserved.
	ErrWorktreeUnsupported = errors.New("machine does not support worktrees")
	// ErrWorktreeRefused means the connector refused to make or remove a
	// worktree (worktree_refused). A *WorktreeError carries the reason when
	// this call learned it.
	ErrWorktreeRefused = errors.New("machine refused the worktree operation")
	// ErrWorktreeNotFound means nothing is at the run's worktree path.
	ErrWorktreeNotFound = errors.New("worktree not found on that machine")
	// ErrWorktreeChanged means the counts the person was shown no longer match.
	ErrWorktreeChanged = errors.New("worktree changed since it was shown")
	// ErrWorktreeBusy means the machine is already running a worktree status
	// or remove; it answers a second one at once instead of queueing it.
	ErrWorktreeBusy = errors.New("machine is already working on a worktree")
	// ErrWorktreeUnavailable means the machine answered but could not do it
	// just now (unavailable, failed or capacity).
	ErrWorktreeUnavailable = errors.New("machine could not do that worktree request just now")
)

// WorktreeError is a worktree refusal with the connector's reason and, for
// git_failed, the first plain line of git's message.
type WorktreeError struct {
	Reason terminalproto.WorktreeReason
	Detail string
}

func (e *WorktreeError) Error() string {
	return ErrWorktreeRefused.Error() + ": " + string(e.Reason)
}
func (e *WorktreeError) Is(target error) bool { return target == ErrWorktreeRefused }

// WorktreeCounts are the counts the person was shown before a remove.
type WorktreeCounts struct {
	Changed  int
	Unpushed int
}

// validWorktreeRequest applies the protocol's own rules to a request body.
func validWorktreeRequest(body terminalproto.Body) error {
	probe := terminalproto.Message{Version: terminalproto.Version, RequestID: ID(), Generation: 1, Body: body}
	if probe.Validate() != nil {
		return terminalproto.ErrInvalid
	}
	return nil
}

// CheckWorktreeRemove validates a remove request without sending it, so a
// caller can refuse a malformed one before it asks for status.
func CheckWorktreeRemove(project, repo, run string, shown WorktreeCounts) error {
	return validWorktreeRequest(terminalproto.WorktreeRemove{Run: run, Project: project, Repo: repo, Changed: shown.Changed, Unpushed: shown.Unpushed})
}

// worktreePeer returns a ready peer whose current connection advertised the
// worktree capability; the request is validated before any of that.
func (b *Broker) worktreePeer(uid int64, agent string, body terminalproto.Body) (*Peer, error) {
	if err := validWorktreeRequest(body); err != nil {
		return nil, err
	}
	p, err := b.peer(uid, agent, true)
	if err != nil {
		return nil, err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, agent)
	if err != nil || caps.Generation != p.Generation {
		return nil, ErrOffline
	}
	if !caps.Capabilities.Worktree {
		return nil, ErrWorktreeUnsupported
	}
	return p, nil
}

// worktreeError maps a connector's worktree_result to the broker's errors.
func worktreeError(r *terminalproto.WorktreeResult) error {
	switch r.Code {
	case terminalproto.OK:
		return nil
	case terminalproto.NotFound:
		return ErrWorktreeNotFound
	case terminalproto.WorktreeChanged:
		return ErrWorktreeChanged
	case terminalproto.WorktreeRefused:
		return &WorktreeError{Reason: r.Reason, Detail: r.Detail}
	case terminalproto.Denied:
		return ErrDenied
	case terminalproto.Conflict:
		return ErrWorktreeBusy
	case terminalproto.Unavailable, terminalproto.Failed, terminalproto.Capacity:
		return ErrWorktreeUnavailable
	}
	return ErrOffline
}

// WorktreeStatus asks the machine for a run's worktree counts. It reads only;
// a lost reply is safe to ask again.
func (b *Broker) WorktreeStatus(ctx context.Context, uid int64, agent, project, repo, run string) (terminalproto.WorktreeState, error) {
	body := terminalproto.WorktreeStatus{Run: run, Project: project, Repo: repo}
	p, err := b.worktreePeer(uid, agent, body)
	if err != nil {
		return terminalproto.WorktreeState{}, err
	}
	reply, err := p.callWithin(ctx, terminalproto.Message{Body: body}, worktreeCallTimeout)
	if err != nil {
		return terminalproto.WorktreeState{}, err
	}
	switch r := reply.Body.(type) {
	case *terminalproto.WorktreeState:
		return *r, nil
	case *terminalproto.WorktreeResult:
		if err := worktreeError(r); err != nil {
			return terminalproto.WorktreeState{}, err
		}
	}
	return terminalproto.WorktreeState{}, ErrOffline
}

// WorktreeRemove asks the machine to remove a run's worktree, only if its
// counts still equal the ones the person was shown. The branch is kept. A
// lost reply is reported as a reconnect; asking for status again shows
// whether the worktree is gone.
func (b *Broker) WorktreeRemove(ctx context.Context, uid int64, agent, project, repo, run string, shown WorktreeCounts) error {
	body := terminalproto.WorktreeRemove{Run: run, Project: project, Repo: repo, Changed: shown.Changed, Unpushed: shown.Unpushed}
	p, err := b.worktreePeer(uid, agent, body)
	if err != nil {
		return err
	}
	reply, err := p.callWithin(ctx, terminalproto.Message{Body: body}, worktreeCallTimeout)
	if err != nil {
		return err
	}
	if r, ok := reply.Body.(*terminalproto.WorktreeResult); ok {
		return worktreeError(r)
	}
	return ErrOffline
}
