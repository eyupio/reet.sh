//go:build linux || darwin

package terminalagent

import (
	"context"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (r *agentReq) handleBranches(op *terminalproto.Branches) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.BranchesResult{Reason: terminalproto.BranchesDenied}
		return r.send(callCtx, r.reply)
	}
	req, home, git := *op, opts.Home, opts.Git
	if git == nil {
		git = terminalhost.ExecGit
	}
	return r.aside(callCtx, r.reply, r.branchesSlot, &branchesRunning, branchesAnswerAfter,
		terminalproto.BranchesResult{Reason: terminalproto.BranchesConflict}, terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed},
		func(ctx context.Context) terminalproto.Body {
			return terminalhost.ListBranches(ctx, home, req, git)
		})
}

func (r *agentReq) handleBranchPrunePlan(op *terminalproto.BranchPrunePlan) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.BranchPrunePlanResult{Reason: terminalproto.BranchesDenied}
		return r.send(callCtx, r.reply)
	}
	req, home, git := *op, opts.Home, opts.Git
	if git == nil {
		git = terminalhost.ExecGit
	}
	return r.aside(callCtx, r.reply, r.pruneSlot, &prunesRunning, branchPruneAnswerAfter,
		terminalproto.BranchPrunePlanResult{Reason: terminalproto.BranchesConflict}, terminalproto.BranchPrunePlanResult{Reason: terminalproto.BranchesGitFailed},
		func(ctx context.Context) terminalproto.Body {
			return terminalhost.PlanBranchPrune(ctx, home, req, git)
		})
}

func (r *agentReq) handleBranchPrune(op *terminalproto.BranchPrune) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.BranchPruneResult{Reason: terminalproto.BranchesDenied}
		return r.send(callCtx, r.reply)
	}
	req, home, git := *op, opts.Home, opts.Git
	if git == nil {
		git = terminalhost.ExecGit
	}
	return r.aside(callCtx, r.reply, r.pruneSlot, &prunesRunning, branchPruneAnswerAfter,
		terminalproto.BranchPruneResult{Reason: terminalproto.BranchesConflict}, terminalproto.BranchPruneResult{Reason: terminalproto.BranchesGitFailed},
		func(ctx context.Context) terminalproto.Body {
			return terminalhost.PruneBranches(ctx, home, req, git)
		})
}

func (r *agentReq) handleBranchSync(op *terminalproto.BranchSync) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.BranchSyncResult{Reason: terminalproto.BranchesDenied}
		return r.send(callCtx, r.reply)
	}
	req, home, git := *op, opts.Home, opts.Git
	if git == nil {
		git = terminalhost.ExecGit
	}
	return r.aside(callCtx, r.reply, r.pruneSlot, &prunesRunning, branchSyncAnswerAfter,
		terminalproto.BranchSyncResult{Reason: terminalproto.BranchesConflict}, terminalproto.BranchSyncResult{Reason: terminalproto.BranchesGitFailed},
		func(ctx context.Context) terminalproto.Body {
			return terminalhost.SyncBranch(ctx, home, req, git)
		})
}

func (r *agentReq) handleBaseCheck(op *terminalproto.BaseCheck) error {
	callCtx, opts := r.callCtx, r.opts
	if !allowed(opts.Consent) {
		r.reply.Body = terminalproto.BaseCheckResult{Reason: terminalproto.BranchesDenied}
		return r.send(callCtx, r.reply)
	}
	req, home, git := *op, opts.Home, opts.Git
	if git == nil {
		git = terminalhost.ExecGit
	}
	return r.aside(callCtx, r.reply, r.pruneSlot, &prunesRunning, baseCheckAnswerAfter,
		terminalproto.BaseCheckResult{Reason: terminalproto.BranchesConflict}, terminalproto.BaseCheckResult{Reason: terminalproto.BranchesGitFailed},
		func(ctx context.Context) terminalproto.Body {
			return terminalhost.CheckBase(ctx, home, req, git)
		})
}
