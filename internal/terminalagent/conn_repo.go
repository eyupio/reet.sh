//go:build linux || darwin

package terminalagent

import (
	"context"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (r *agentReq) handleRepoClone(op *terminalproto.RepoClone) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneDenied}
		return r.send(callCtx, r.reply)
	}
	req, home, git := *op, opts.Home, opts.Git
	if git == nil {
		git = terminalhost.ExecCloneGit
	}
	return r.aside(callCtx, r.reply, cloneSlot, &clonesRunning, repoCloneAnswerAfter,
		terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneBusy}, terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneTimedOut},
		func(ctx context.Context) terminalproto.Body {
			return cloneRepo(ctx, home, req, git, terminalhost.ExecGH)
		})
}

func (r *agentReq) handleRepoCloneStatus(op *terminalproto.RepoCloneStatus) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneDenied}
		return r.send(callCtx, r.reply)
	}
	req, home, git := *op, opts.Home, opts.Git
	if git == nil {
		git = terminalhost.ExecGit
	}
	answerAfter := repoCloneStatusAnswerAfter
	if req.Fetch {
		answerAfter = repoCloneFetchAnswerAfter
	}
	return r.aside(callCtx, r.reply, r.cloneStatusSlot, &cloneStatusesRunning, answerAfter,
		terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneBusy}, terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneTimedOut},
		func(ctx context.Context) terminalproto.Body {
			return cloneStatus(ctx, home, req, git)
		})
}

func (r *agentReq) handleRepoCloneRemove(op *terminalproto.RepoCloneRemove) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneDenied}
		return r.send(callCtx, r.reply)
	}
	req, home, git := *op, opts.Home, opts.Git
	if git == nil {
		git = terminalhost.ExecGit
	}
	// The clone slot, held until the delete has ended: a clone and
	// a remove never run together.
	return r.asideHolding(callCtx, r.reply, cloneSlot, &clonesRunning, repoCloneRemoveAnswerAfter, true,
		terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneBusy}, terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneTimedOut},
		func(ctx context.Context) terminalproto.Body {
			return removeClone(ctx, home, req, git)
		})
}

func (r *agentReq) handleRepoList() error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.RepoListResult{Reason: terminalproto.RepoListDenied}
		return r.send(callCtx, r.reply)
	}
	return r.aside(callCtx, r.reply, r.repoListSlot, &repoListsRunning, repoListAnswerAfter,
		terminalproto.RepoListResult{Reason: terminalproto.RepoListBusy}, terminalproto.RepoListResult{Reason: terminalproto.RepoListTimedOut},
		func(ctx context.Context) terminalproto.Body {
			return listRepos(ctx, terminalhost.ExecGH, terminalhost.ExecGHList)
		})
}
