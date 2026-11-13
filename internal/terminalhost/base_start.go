package terminalhost

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// baseFetchTimeout bounds the one call that reaches github.com, on top of the
// budget the caller gives: a repository that cannot be reached must not hold
// up starting a run for long.
const baseFetchTimeout = 30 * time.Second

// ResolveBase decides which commit a worktree for base should start from,
// changing nothing in repo but refs/remotes/origin/<base>.
//
// An empty base is the repository's default branch (syncDefaultBase); when
// there is none the state is the repository's HEAD with no remote. Otherwise,
// when origin is a github.com repository, base is fetched from it (with fetch
// false the last fetched refs/remotes/origin/<base> is used instead and
// nothing reaches the network) and the two sides are compared:
//
//   - same, behind, no_local: GitHub's commit, so a run starts from what the
//     pull request will be merged into;
//   - ahead, diverged: the local commit, so work the person has not pushed is
//     never silently left out. A branch GitHub answered that it does not have
//     is ahead too, with no commit on GitHub's side;
//   - fetch_failed (GitHub itself could not be reached): the local branch, or
//     the last fetched remote-tracking branch when there is no local one;
//   - no_remote: the local branch. With no github.com origin at all, nothing
//     is fetched and the lookup is the one PrepareWorktree has always done,
//     refs/heads/<base> and then refs/remotes/<base>; with one that has no
//     such branch, only refs/heads/<base>.
//
// Two bases naming a remote-tracking branch are resolved on their own terms,
// and only when the repository has no local branch of that whole name (one
// literally named origin/x is what the base means, as it always has been):
//
//   - origin/<name> on a github.com repository is GitHub's own branch, chosen
//     by the person: <name> is fetched and its refs/remotes/origin/<name> is
//     the start whatever the local <name> holds, reported as same with no
//     Local. A GitHub that could not be reached leaves the last fetched one
//     (fetch_failed); one that answered that it has no such branch is
//     base_missing, a remote-tracking branch left by an older fetch being
//     stale;
//   - any other remote's branch (upstream/main) is refs/remotes/<base>, as it
//     was before any of this, reported as no_remote: nothing the fetch found
//     applies to it. This is the last thing the base can be.
//
// Neither side having the branch is refuseWorktree(WorktreeBaseMissing, "").
func ResolveBase(ctx context.Context, repo, base string, fetch bool, git GitRunner) (terminalproto.BaseState, error) {
	if git == nil || (base != "" && !terminalproto.ValidRefName(base)) {
		return terminalproto.BaseState{}, terminalproto.ErrInvalid
	}
	if base == "" {
		base = syncDefaultBase(ctx, repo, git)
	}
	if base == "" {
		head, err := gitCommit(ctx, git, repo, "HEAD")
		if err != nil {
			return terminalproto.BaseState{}, err
		}
		if head == "" {
			return terminalproto.BaseState{}, refuseWorktree(terminalproto.WorktreeBaseMissing, "")
		}
		return terminalproto.BaseState{Relation: terminalproto.BaseNoRemote, Start: head}, nil
	}

	state := terminalproto.BaseState{Base: base}
	local, err := gitCommit(ctx, git, repo, "refs/heads/"+base)
	if err != nil {
		return terminalproto.BaseState{}, err
	}
	state.Local = local

	if !githubOrigin(ctx, git, repo) {
		state.Relation = terminalproto.BaseNoRemote
		state.Start = local
		if local == "" {
			if state.Start, err = gitCommit(ctx, git, repo, "refs/remotes/"+base); err != nil {
				return terminalproto.BaseState{}, err
			}
		}
		return resolvedBase(state)
	}

	// origin/<name>, with no local branch of that name, is the person naming
	// GitHub's own branch: it is fetched and started from on its own terms.
	if name, ok := strings.CutPrefix(base, "origin/"); ok && local == "" && terminalproto.ValidRefName(name) {
		return originBase(ctx, git, repo, base, name, fetch)
	}

	found := baseFetched
	if fetch {
		found = fetchBase(ctx, git, repo, base)
	}
	remote, err := gitCommit(ctx, git, repo, "refs/remotes/origin/"+base)
	if err != nil {
		return terminalproto.BaseState{}, err
	}
	state.Remote = remote

	switch {
	case found == baseAbsent:
		// GitHub was reached and has no such branch, so every local commit is
		// one it lacks and a remote-tracking branch left by an older fetch is
		// stale. With no local branch either, nothing is left to start from.
		state.Relation = terminalproto.BaseAhead
		state.Remote = ""
		state.Start = local
	case found == baseUnreachable:
		// The remote-tracking branch may be out of date, so the local branch
		// wins and the two sides are not compared.
		state.Relation = terminalproto.BaseFetchFailed
		state.Start = local
		if local == "" {
			state.Start = remote
		}
	case remote == "":
		// GitHub has no such branch, or nothing has been fetched yet.
		state.Relation = terminalproto.BaseNoRemote
		state.Start = local
	case local == "":
		state.Relation = terminalproto.BaseNoLocal
		state.Start = remote
	case local == remote:
		state.Relation = terminalproto.BaseSame
		state.Start = remote
	case isAncestor(ctx, git, repo, local, remote):
		state.Relation = terminalproto.BaseBehind
		state.Start = remote
	case isAncestor(ctx, git, repo, remote, local):
		state.Relation = terminalproto.BaseAhead
		state.Start = local
	default:
		state.Relation = terminalproto.BaseDiverged
		state.Start = local
	}
	if state.Start == "" {
		// Neither a local branch nor one of GitHub's: another remote's
		// tracking branch (upstream/main) is the last thing the base can be,
		// exactly as it was before anything was fetched. Nothing the fetch
		// found applies to it.
		other, err := gitCommit(ctx, git, repo, "refs/remotes/"+base)
		if err != nil {
			return terminalproto.BaseState{}, err
		}
		state.Relation, state.Start = terminalproto.BaseNoRemote, other
	}
	return resolvedBase(state)
}

// originBase resolves a base spelled origin/<name> on a github.com
// repository, where the repository has no local branch of that whole name:
// the person named GitHub's branch, so it is fetched and the fetched
// refs/remotes/origin/<name> is the start whatever the local <name> holds.
// Local stays empty, because there is nothing to tell them about: they chose
// GitHub's side themselves. A GitHub that could not be reached leaves the
// last fetched remote-tracking branch (fetch_failed); a branch GitHub
// answered that it does not have is base_missing, since a remote-tracking
// branch an older fetch left behind is stale.
func originBase(ctx context.Context, git GitRunner, repo, base, name string, fetch bool) (terminalproto.BaseState, error) {
	found := baseFetched
	if fetch {
		found = fetchBase(ctx, git, repo, name)
	}
	if found == baseAbsent {
		return terminalproto.BaseState{}, refuseWorktree(terminalproto.WorktreeBaseMissing, "")
	}
	remote, err := gitCommit(ctx, git, repo, "refs/remotes/origin/"+name)
	if err != nil {
		return terminalproto.BaseState{}, err
	}
	state := terminalproto.BaseState{Base: base, Relation: terminalproto.BaseSame, Remote: remote, Start: remote}
	if found == baseUnreachable {
		state.Relation = terminalproto.BaseFetchFailed
	}
	return resolvedBase(state)
}

// resolvedBase is state as it stands, or the base_missing refusal when it
// has no starting commit: neither side has the branch.
func resolvedBase(state terminalproto.BaseState) (terminalproto.BaseState, error) {
	if state.Start == "" {
		return terminalproto.BaseState{}, refuseWorktree(terminalproto.WorktreeBaseMissing, "")
	}
	return state, nil
}

// githubOrigin reports whether origin is a github.com repository, from the
// URL as it is configured: `git config --get remote.origin.url`, never
// `git remote get-url`, which applies the repository's own
// url.<base>.insteadOf rewrites. Only the configured URL says where the
// person believes their code lives.
func githubOrigin(ctx context.Context, git GitRunner, repo string) bool {
	out, err := git(ctx, repo, "config", "--get", "remote.origin.url")
	if err != nil {
		return false
	}
	_, _, ok := GitHubRemote(strings.TrimSpace(string(out)))
	return ok
}

// baseFetch is what the one attempt to reach GitHub found.
type baseFetch int

const (
	// baseFetched: refs/remotes/origin/<base> is now GitHub's branch.
	baseFetched baseFetch = iota
	// baseUnreachable: GitHub did not answer, so nothing is known about it.
	baseUnreachable
	// baseAbsent: GitHub answered and has no such branch.
	baseAbsent
)

// fetchBase fetches base from origin into refs/remotes/origin/<base>. Every
// call runs with hooks off, gh's credentials for github.com only
// (ghCredentialArgs) and GIT_TERMINAL_PROMPT=0, so a missing sign-in fails
// instead of waiting for a prompt (networkGitEnv); the whole attempt shares
// baseFetchTimeout. The fetch writes no FETCH_HEAD and fetches no tags, so
// nothing but that one remote-tracking branch moves.
//
// A fetch fails the same way for a GitHub that cannot be reached and for a
// branch that is not on it, so a failure within the bound asks once more
// with `ls-remote --exit-code`, whose documented exit status 2 is "no
// matching refs" and nothing else. The exit status is read from the error,
// never from git's output, which is localised.
func fetchBase(ctx context.Context, git GitRunner, repo, base string) baseFetch {
	ctx, cancel := context.WithTimeout(ctx, baseFetchTimeout)
	defer cancel()
	ctx = withGitEnv(ctx, networkGitEnv()...)
	args := append(networkGitArgs(), "fetch", "--no-tags", "--no-write-fetch-head", "origin",
		"+refs/heads/"+base+":refs/remotes/origin/"+base)
	if _, err := git(ctx, repo, args...); err == nil {
		if ctx.Err() == nil {
			return baseFetched
		}
		return baseUnreachable
	}
	if ctx.Err() != nil {
		return baseUnreachable
	}
	args = append(networkGitArgs(), "ls-remote", "--exit-code", "origin", "refs/heads/"+base)
	_, err := git(ctx, repo, args...)
	var exit *exec.ExitError
	if ctx.Err() == nil && errors.As(err, &exit) && exit.ExitCode() == 2 {
		return baseAbsent
	}
	return baseUnreachable
}

// networkGitArgs are the arguments every call that may reach github.com
// carries: hooks off, and gh's credentials for github.com only.
func networkGitArgs() []string {
	return append([]string{"-c", "core.hooksPath=/dev/null"}, ghCredentialArgs()...)
}

// isAncestor reports whether commit is an ancestor of other (or the same
// commit); a git that cannot answer is not an ancestor.
func isAncestor(ctx context.Context, git GitRunner, repo, commit, other string) bool {
	_, err := git(ctx, repo, "merge-base", "--is-ancestor", commit, other)
	return err == nil
}

// gitCommit is the commit ref names, or "" when the repository has no such
// ref. A git that failed for any other reason (it is missing, or the budget
// ended) is an error, as is a commit id that is not one.
func gitCommit(ctx context.Context, git GitRunner, repo, ref string) (string, error) {
	out, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		if errors.Is(err, errGitMissing) || ctx.Err() != nil {
			return "", gitFailed(out, err)
		}
		return "", nil
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "", nil
	}
	if !validHeadLine("HEAD " + commit) {
		return "", gitFailed(nil, errors.New("git returned an invalid commit"))
	}
	return commit, nil
}
