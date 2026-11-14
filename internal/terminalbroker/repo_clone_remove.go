package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// RepoCloneStatusCallTimeout is the controller's deadline for a clone
	// status. The machine bounds its look at 10 seconds and its connector
	// answers by 12.
	RepoCloneStatusCallTimeout = 15 * time.Second
	// RepoCloneRemoveCallTimeout is the controller's deadline for a clone
	// remove. The machine's connector answers by 55 seconds; a delete that
	// takes longer goes on there and is answered timed_out.
	RepoCloneRemoveCallTimeout = 60 * time.Second
)

// ErrRepoCloneRemoveUnsupported means the machine did not negotiate
// repo-clone-remove-v1 (an older reet there); nothing was sent.
var ErrRepoCloneRemoveUnsupported = errors.New("machine does not support removing a clone")

// CloneStatus asks the machine what the clone at req.Dir holds that deleting
// it would lose. It is sent only to a connection that reported the
// repo-clone-remove capability. The machine's answer is returned as it came,
// a refusal as its Reason, for the caller to word.
func (b *Broker) CloneStatus(ctx context.Context, uid int64, agent string, req terminalproto.RepoCloneStatus) (terminalproto.RepoCloneState, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.RepoCloneState{}, err
	}
	if !caps.RepoCloneRemove {
		return terminalproto.RepoCloneState{}, ErrRepoCloneRemoveUnsupported
	}
	body, err := p.ask(ctx, req, RepoCloneStatusCallTimeout)
	if err != nil {
		return terminalproto.RepoCloneState{}, err
	}
	s, ok := body.(*terminalproto.RepoCloneState)
	if !ok {
		return terminalproto.RepoCloneState{}, ErrMachineAnswer
	}
	return *s, nil
}

// RemoveClone asks the machine to delete the clone at req.Dir, which it does
// only when the clone is still what req.Fingerprint names and nothing in it
// would be lost. It is sent only to a connection that reported the
// repo-clone-remove capability. The machine's answer is returned as it came,
// a refusal as its Reason. An error (a timeout, a dropped connection) says
// nothing about the folder: the machine may have deleted it, so the caller
// asks for status again before it says anything.
func (b *Broker) RemoveClone(ctx context.Context, uid int64, agent string, req terminalproto.RepoCloneRemove) (terminalproto.RepoCloneRemoved, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.RepoCloneRemoved{}, err
	}
	if !caps.RepoCloneRemove {
		return terminalproto.RepoCloneRemoved{}, ErrRepoCloneRemoveUnsupported
	}
	body, err := p.ask(ctx, req, RepoCloneRemoveCallTimeout)
	if err != nil {
		return terminalproto.RepoCloneRemoved{}, err
	}
	r, ok := body.(*terminalproto.RepoCloneRemoved)
	if !ok {
		return terminalproto.RepoCloneRemoved{}, ErrMachineAnswer
	}
	return *r, nil
}
