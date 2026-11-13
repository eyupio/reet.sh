package terminalhost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// branchSyncTimeout bounds one sync: the fetch and every other git call it
// makes. A fetch talks to the network, so it gets more than a prune does.
const branchSyncTimeout = 45 * time.Second

// SyncBranch brings the repository whose working tree top is req.Dir up to
// date with its remote after a worktree made from it has gone: it fetches the
// base branch from origin and fast-forwards the folder to it.
//
// It never discards anything. The folder is changed only by
// `git merge --ff-only`, and only when it is on the base branch, has no
// uncommitted changes to tracked files, has no merge, rebase or similar in
// progress, and the base is behind the remote. In every other case the folder
// is left exactly as it was and the answer says why. Repository hooks are
// disabled for the merge, as they are for a worktree add. git runs as an
// argument vector with the hardened environment, so a remote that needs an
// ssh agent or credential helper outside that environment answers
// fetch_failed.
func SyncBranch(ctx context.Context, home string, req terminalproto.BranchSync, git GitRunner) terminalproto.BranchSyncResult {
	ctx, cancel := context.WithTimeout(ctx, branchSyncTimeout)
	defer cancel()
	dir, reason := branchesRepo(home, req.Dir)
	if reason != "" {
		return terminalproto.BranchSyncResult{Reason: reason}
	}
	if git == nil {
		return terminalproto.BranchSyncResult{Reason: terminalproto.BranchesGitFailed}
	}
	ctx = pinGitDir(ctx, dir)

	base := req.Base
	if base == "" {
		base = syncDefaultBase(ctx, dir, git)
	}
	if base == "" || !terminalproto.ValidRefName(base) {
		return terminalproto.BranchSyncResult{Status: terminalproto.BranchSyncNoRemote}
	}
	answer := func(status terminalproto.BranchSyncStatus) terminalproto.BranchSyncResult {
		return terminalproto.BranchSyncResult{Status: status, Base: base}
	}

	// A repository in the middle of something is left alone entirely.
	for _, name := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG", "sequencer"} {
		if _, err := os.Lstat(filepath.Join(dir, ".git", name)); err == nil {
			return answer(terminalproto.BranchSyncBusy)
		}
	}
	if _, err := git(ctx, dir, "remote", "get-url", "origin"); err != nil {
		return answer(terminalproto.BranchSyncNoRemote)
	}
	remoteRef := "refs/remotes/origin/" + base
	// A github.com origin is fetched with the machine's own gh sign-in, for
	// github.com only, and with no terminal prompt to block on; an ssh one is
	// fetched from the repository's https URL instead, so ssh is never
	// invoked (githubFetch). Any other host is fetched exactly as before,
	// with hooks off and nothing else.
	fetchCtx, fetchArgs, from := ctx, []string{"-c", "core.hooksPath=/dev/null"}, "origin"
	switch origin := githubFetch(ctx, git, dir); {
	case origin.from != "":
		fetchCtx, fetchArgs, from = withGitEnv(ctx, networkGitEnv()...), networkGitArgs(), origin.from
	case origin.gitHub:
		// A github.com origin spelled as an ssh URL on a machine with no gh:
		// there is nothing to authenticate as and ssh is never invoked, so
		// GitHub was not reached and the folder is left exactly as it is.
		return answer(terminalproto.BranchSyncFetchFailed)
	}
	fetchArgs = append(fetchArgs, "fetch", "--no-tags", "--no-write-fetch-head", from, "+refs/heads/"+base+":"+remoteRef)
	// One fetch per repository at a time, as a base resolve does: concurrent
	// fetches of the same refspec make all but one fail.
	release := fetchLocks.lock(dir)
	_, fetchErr := git(fetchCtx, dir, fetchArgs...)
	release()
	if fetchErr != nil || ctx.Err() != nil {
		return answer(terminalproto.BranchSyncFetchFailed)
	}
	remote, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", remoteRef+"^{commit}")
	if err != nil {
		return answer(terminalproto.BranchSyncNoRemote)
	}
	remoteOID := strings.TrimSpace(string(remote))

	current, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSuffix(string(current), "\n") != base {
		return answer(terminalproto.BranchSyncOffBase)
	}
	status, err := git(ctx, dir, "status", "--porcelain=v1", "--untracked-files=no", "--ignore-submodules=none")
	if err != nil || strings.TrimSpace(string(status)) != "" {
		return answer(terminalproto.BranchSyncDirty)
	}
	head, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return answer(terminalproto.BranchSyncDiverged)
	}
	if strings.TrimSpace(string(head)) == remoteOID {
		return answer(terminalproto.BranchSyncCurrent)
	}
	if _, err := git(ctx, dir, "merge-base", "--is-ancestor", "HEAD", remoteRef); err != nil {
		return answer(terminalproto.BranchSyncDiverged)
	}
	if _, err := git(ctx, dir, "-c", "core.hooksPath=/dev/null", "merge", "--ff-only", "--quiet", remoteRef); err != nil || ctx.Err() != nil {
		// git refuses a fast-forward that would overwrite a file it cannot
		// see in the listing above (an untracked one, for example).
		return answer(terminalproto.BranchSyncDirty)
	}
	return answer(terminalproto.BranchSyncUpdated)
}

// syncDefaultBase is the repository's default branch: origin/HEAD, then a
// local main, then a local master. Empty when none exists.
func syncDefaultBase(ctx context.Context, dir string, git GitRunner) string {
	if out, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if name, ok := strings.CutPrefix(strings.TrimSuffix(string(out), "\n"), "origin/"); ok && terminalproto.ValidRefName(name) {
			return name
		}
	}
	for _, name := range []string{"main", "master"} {
		if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+name); err == nil {
			return name
		}
	}
	return ""
}
