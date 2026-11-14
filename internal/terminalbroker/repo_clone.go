package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// RepoCloneCallTimeout is the controller's deadline for a clone. The machine
// stops a clone at 10 minutes and answers timed_out; this leaves room for
// that answer.
var RepoCloneCallTimeout = 10*time.Minute + 30*time.Second

// ErrRepoCloneUnsupported means the machine did not negotiate repo-clone-v1
// (an older reet there, or no git or GitHub CLI); nothing was sent.
var ErrRepoCloneUnsupported = errors.New("machine does not support cloning from GitHub")

// CloneRepo asks the machine to clone github.com/req.Owner/req.Repo into its
// projects folder. It is sent only to a connection that reported the
// repo-clone capability. The machine's answer is returned as it came, a
// refusal (busy and denied included) as its Reason, for the caller to word;
// nothing of it is stored here and no GitHub credential passes through.
func (b *Broker) CloneRepo(ctx context.Context, uid int64, agent string, req terminalproto.RepoClone) (terminalproto.RepoCloneResult, error) {
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.RepoCloneResult{}, err
	}
	if !caps.RepoClone {
		return terminalproto.RepoCloneResult{}, ErrRepoCloneUnsupported
	}
	body, err := p.ask(ctx, req, RepoCloneCallTimeout)
	if err != nil {
		return terminalproto.RepoCloneResult{}, err
	}
	r, ok := body.(*terminalproto.RepoCloneResult)
	if !ok {
		return terminalproto.RepoCloneResult{}, ErrMachineAnswer
	}
	return *r, nil
}
