package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// RepoListCallTimeout is the controller's deadline for a repository listing.
// The machine bounds its gh calls at 30 seconds and its connector answers by
// 35.
var RepoListCallTimeout = 40 * time.Second

// ErrRepoListUnsupported means the machine did not report the repo-list
// capability (an older reet there, gh missing, or terminal access off);
// nothing was sent.
var ErrRepoListUnsupported = errors.New("machine does not support listing GitHub repositories")

// ListRepos asks the machine which GitHub repositories its gh sign-in can
// reach. It is sent only to a connection that reported the repo-list
// capability. The machine's answer is returned as it came, a refusal as its
// Reason, for the caller to word.
func (b *Broker) ListRepos(ctx context.Context, uid int64, agent string) (terminalproto.RepoListResult, error) {
	req := terminalproto.RepoList{}
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.RepoListResult{}, err
	}
	if !caps.RepoList {
		return terminalproto.RepoListResult{}, ErrRepoListUnsupported
	}
	body, err := p.ask(ctx, req, RepoListCallTimeout)
	if err != nil {
		return terminalproto.RepoListResult{}, err
	}
	r, ok := body.(*terminalproto.RepoListResult)
	if !ok {
		return terminalproto.RepoListResult{}, ErrMachineAnswer
	}
	return *r, nil
}
