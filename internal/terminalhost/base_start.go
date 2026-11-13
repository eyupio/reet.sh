package terminalhost

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// baseFetchTimeout bounds the one call that reaches github.com, on top of the
// budget the caller gives: a repository that cannot be reached must not hold
// up starting a run for long.
const baseFetchTimeout = 30 * time.Second

// baseCheckTimeout bounds one base check: the fetch and every other git call
// it makes. It leaves the fetch its whole budget and a little over.
const baseCheckTimeout = 33 * time.Second

// CheckBase answers where a worktree made now for req.Base would start in the
// repository whose working tree top is req.Dir, making none. The folder is
// checked exactly as a branch listing's is, and the only write is the fetch
// that brings refs/remotes/origin/<base> up to date: no worktree, branch or
// checkout is created, removed or moved.
//
// A branch neither the repository nor GitHub has is base_missing, so the
// browser can say so rather than showing a failure; anything else git could
// not answer is git_failed.
func CheckBase(ctx context.Context, home string, req terminalproto.BaseCheck, git GitRunner) terminalproto.BaseCheckResult {
	ctx, cancel := context.WithTimeout(ctx, baseCheckTimeout)
	defer cancel()
	dir, reason := branchesRepo(home, req.Dir)
	if reason != "" {
		return terminalproto.BaseCheckResult{Reason: reason}
	}
	if git == nil {
		return terminalproto.BaseCheckResult{Reason: terminalproto.BranchesGitFailed}
	}
	state, err := ResolveBase(pinGitDir(ctx, dir), dir, req.Base, true, git)
	if err != nil {
		var refusal *WorktreeError
		if errors.As(err, &refusal) && refusal.Reason == terminalproto.WorktreeBaseMissing {
			return terminalproto.BaseCheckResult{Reason: terminalproto.BranchesBaseMissing}
		}
		return terminalproto.BaseCheckResult{Reason: terminalproto.BranchesGitFailed}
	}
	if !state.Valid() {
		return terminalproto.BaseCheckResult{Reason: terminalproto.BranchesGitFailed}
	}
	return terminalproto.BaseCheckResult{State: state}
}

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
//   - no_remote: the local branch. With nothing to fetch from — no github.com
//     origin at all, or one spelled as an ssh URL on a machine with no gh to
//     authenticate as (githubFetch) — nothing is fetched and the lookup is the
//     one a worktree create has always done, refs/heads/<base> and then
//     refs/remotes/<base>; with a github.com origin that has no such branch,
//     only refs/heads/<base>.
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
	look := lastFetched
	if fetch {
		look = fetchBase
	}
	return resolveBase(ctx, repo, base, look, git)
}

// ResolveBaseFrom resolves the base from what plan's one fetch found, making
// no call that reaches the network, so it is safe under a lock.
func ResolveBaseFrom(ctx context.Context, repo, base string, plan BasePlan, git GitRunner) (terminalproto.BaseState, error) {
	return resolveBase(ctx, repo, base, plan.lookup, git)
}

// baseLookup is how one resolve learns what GitHub has for a branch:
// fetchBase reaches github.com, a BasePlan's lookup answers from the fetch it
// already made, and lastFetched answers from the remote-tracking ref alone.
type baseLookup func(ctx context.Context, git GitRunner, repo, from, branch string) baseFetch

// lastFetched reaches nothing: whatever the last fetch left in
// refs/remotes/origin/<branch> is all that is known.
func lastFetched(context.Context, GitRunner, string, string, string) baseFetch { return baseFetched }

func resolveBase(ctx context.Context, repo, base string, look baseLookup, git GitRunner) (terminalproto.BaseState, error) {
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

	from := githubFetch(ctx, git, repo).from
	if from == "" {
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
		return originBase(ctx, git, repo, from, base, name, look)
	}

	found := look(ctx, git, repo, from, base)
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
func originBase(ctx context.Context, git GitRunner, repo, from, base, name string, look baseLookup) (terminalproto.BaseState, error) {
	found := look(ctx, git, repo, from, name)
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

// BasePlan is what one worktree create's fetch found. PlanBase makes it
// before its caller takes any lock, and PrepareWorktreeFrom reads it back
// under one, so no call that reaches github.com ever runs while a lock is
// held: a repository that cannot be reached would otherwise freeze every
// session listing for the whole fetch budget.
//
// The zero BasePlan is a connection that did not negotiate base-fresh-v1:
// nothing was fetched and the worktree starts where a create always started
// it.
type BasePlan struct {
	// fresh is the negotiated base-fresh-v1.
	fresh bool
	// branch is the branch the one fetch was for and found what it found.
	// With no branch no fetch was made, and the last fetched
	// refs/remotes/origin/<branch> is all this create knows.
	branch string
	found  baseFetch
}

// lookup answers a resolve from the plan's own fetch, reaching nothing. A
// branch the plan did not fetch is left to the last fetched
// remote-tracking ref.
func (p BasePlan) lookup(_ context.Context, _ GitRunner, _, _, branch string) baseFetch {
	if branch != "" && branch == p.branch {
		return p.found
	}
	return baseFetched
}

// PlanBase makes the one call a worktree create needs to reach github.com:
// it fetches the base branch of a github.com origin, outside whatever lock
// its caller is about to take, and answers what it found. fresh is the
// connection's negotiated base-fresh-v1; without it nothing is fetched at
// all and the create starts where it always did.
//
// A create whose worktree is already there is a retry that adopts it and
// resolves no base, so nothing is fetched for it either. Everything else this
// call cannot settle — a folder that is not a repository, an origin that is
// not github.com, an ssh origin with no gh to authenticate as — is left to
// PrepareWorktreeFrom, which refuses or resolves it exactly as it would.
func PlanBase(ctx context.Context, home, repo, project, run, base string, fresh bool, git GitRunner) BasePlan {
	plan := BasePlan{fresh: fresh}
	validBase := base == "" || terminalproto.ValidRefName(base)
	if !fresh || git == nil || !validBase || !terminalproto.ValidRunToken(run) {
		return plan
	}
	repoPath, err := ResolveDir(home, repo)
	if err != nil {
		return plan
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return plan
	}
	if repoPath, err = filepath.EvalSymlinks(repoPath); err != nil {
		return plan
	}
	if _, dest, _ := worktreePlace(realHome, repoPath, project, run); !missing(dest) {
		return plan
	}
	from, branch, ok := baseFetchBranch(ctx, git, repoPath, base)
	if !ok {
		return plan
	}
	plan.branch, plan.found = branch, fetchBase(ctx, git, repoPath, from, branch)
	return plan
}

// missing reports whether nothing at all is at path.
func missing(path string) bool {
	_, err := os.Lstat(path)
	return os.IsNotExist(err)
}

// baseFetchBranch is the branch a create fetches for base and the remote it
// fetches from, by the same rules the resolve itself follows: the
// repository's default branch when no base was named, the <name> of an
// origin/<name> base the repository has no local branch for, and otherwise
// the base itself. ok is false when there is nothing to fetch.
func baseFetchBranch(ctx context.Context, git GitRunner, repo, base string) (from, branch string, ok bool) {
	if base == "" {
		base = syncDefaultBase(ctx, repo, git)
	}
	if base == "" {
		return "", "", false
	}
	origin := githubFetch(ctx, git, repo)
	if origin.from == "" {
		return "", "", false
	}
	if name, cut := strings.CutPrefix(base, "origin/"); cut && terminalproto.ValidRefName(name) {
		if local, err := gitCommit(ctx, git, repo, "refs/heads/"+base); err == nil && local == "" {
			return origin.from, name, true
		}
	}
	return origin.from, base, true
}

// baseFreshKey carries, in a context, that a create arrived on a connection
// that negotiated base-fresh-v1 (see WithBaseFresh).
type baseFreshKey struct{}

// WithBaseFresh returns ctx marked as a create from a connection that
// negotiated base-fresh-v1, so the worktree it makes starts from the base as
// GitHub has it and reports where it started. A create without it starts the
// worktree where a create always did and fetches nothing. The connector marks
// each create it serves: every one goes through Manager.Create, which knows
// nothing of connections and their features.
func WithBaseFresh(ctx context.Context) context.Context {
	return context.WithValue(ctx, baseFreshKey{}, true)
}

// baseFresh reports whether ctx is such a create.
func baseFresh(ctx context.Context) bool {
	fresh, _ := ctx.Value(baseFreshKey{}).(bool)
	return fresh
}

// resolvedBase is state as it stands, or the base_missing refusal when it
// has no starting commit: neither side has the branch.
func resolvedBase(state terminalproto.BaseState) (terminalproto.BaseState, error) {
	if state.Start == "" {
		return terminalproto.BaseState{}, refuseWorktree(terminalproto.WorktreeBaseMissing, "")
	}
	return state, nil
}

// githubFetch says what a base branch is fetched from when origin is a
// github.com repository: "origin" itself for an https URL, and the
// repository's own https URL for an ssh one (git@github.com:o/r,
// ssh://git@github.com/o/r), which carries gh's credentials instead. From is
// empty for any other host, and for an ssh origin on a machine with no gh.
//
// ssh is never invoked for these fetches: the gh helper only serves https,
// the hardened environment drops SSH_AUTH_SOCK, and GIT_TERMINAL_PROMPT=0
// does not stop ssh asking for a passphrase or a host key, so a connector
// could sit on a prompt nobody can answer for the whole budget. With no gh to
// authenticate as, an ssh origin is simply not fetched: the base is resolved
// from the repository alone, as it was before any base was fetched.
//
// The URL is read as it is configured: `git config --get remote.origin.url`,
// never `git remote get-url`, which applies the repository's own
// url.<base>.insteadOf rewrites. Only the configured URL says where the
// person believes their code lives.
func githubFetch(ctx context.Context, git GitRunner, repo string) originFetch {
	out, err := git(ctx, repo, "config", "--get", "remote.origin.url")
	if err != nil {
		return originFetch{}
	}
	url := strings.TrimSpace(string(out))
	owner, name, ok := GitHubRemote(url)
	if !ok {
		return originFetch{}
	}
	if strings.HasPrefix(url, "https://") {
		return originFetch{gitHub: true, from: "origin"}
	}
	if ghCredentialArgs() == nil {
		return originFetch{gitHub: true}
	}
	return originFetch{gitHub: true, from: "https://github.com/" + owner + "/" + name + ".git"}
}

// originFetch is what githubFetch found: whether origin is a github.com
// repository at all, and what a fetch of one of its branches names as the
// remote. From empty means nothing may be fetched.
type originFetch struct {
	gitHub bool
	from   string
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

// fetchBase fetches base from the remote githubFetch chose (origin, or the
// repository's https URL for an ssh origin) into refs/remotes/origin/<base>.
// Every call runs with hooks off, gh's credentials for github.com only
// (ghCredentialArgs) and GIT_TERMINAL_PROMPT=0, so a missing sign-in fails
// instead of waiting for a prompt (networkGitEnv); the whole attempt shares
// baseFetchTimeout. The fetch writes no FETCH_HEAD and fetches no tags, so
// nothing but that one remote-tracking branch moves.
//
// Everything this call reaches the network for is serialised per repository
// (fetchLocks): git fails a fetch with "unable to update local ref" when
// another fetch of the same refspec updated it first, and a losing fetch
// would otherwise be reported as a GitHub that could not be reached.
//
// A fetch fails the same way for a GitHub that cannot be reached and for a
// branch that is not on it, so a failure within the bound asks once more
// with `ls-remote --exit-code`, whose documented exit status 2 is "no
// matching refs" and nothing else. The exit status is read from the error,
// never from git's output, which is localised. An ls-remote that answers
// settles the fetch rather than failing it: GitHub was reached, so if the id
// it printed is already in refs/remotes/origin/<base> the branch is fetched,
// and if it is not the fetch is tried once more.
func fetchBase(ctx context.Context, git GitRunner, repo, from, base string) baseFetch {
	ctx, cancel := context.WithTimeout(ctx, baseFetchTimeout)
	defer cancel()
	ctx = withGitEnv(ctx, networkGitEnv()...)
	defer fetchLocks.lock(repo)()
	fetch := func() error {
		args := append(networkGitArgs(), "fetch", "--no-tags", "--no-write-fetch-head", from,
			"+refs/heads/"+base+":refs/remotes/origin/"+base)
		_, err := git(ctx, repo, args...)
		return err
	}
	if err := fetch(); err == nil {
		if ctx.Err() == nil {
			return baseFetched
		}
		return baseUnreachable
	}
	if ctx.Err() != nil {
		return baseUnreachable
	}
	args := append(networkGitArgs(), "ls-remote", "--exit-code", from, "refs/heads/"+base)
	out, err := git(ctx, repo, args...)
	var exit *exec.ExitError
	if ctx.Err() == nil && errors.As(err, &exit) && exit.ExitCode() == 2 {
		return baseAbsent
	}
	if err != nil || ctx.Err() != nil {
		return baseUnreachable
	}
	// GitHub answered: the fetch failed for some other reason, which another
	// fetch of the same refspec winning is the common one.
	if id := lsRemoteID(out); id != "" {
		if tracking, err := gitCommit(ctx, git, repo, "refs/remotes/origin/"+base); err == nil && tracking == id {
			return baseFetched
		}
	}
	if err := fetch(); err == nil && ctx.Err() == nil {
		return baseFetched
	}
	return baseUnreachable
}

// lsRemoteID is the commit id of the first line `ls-remote` printed, or ""
// when the output is not one: "<id>\t<ref>".
func lsRemoteID(out []byte) string {
	line, _, _ := strings.Cut(string(out), "\n")
	id, _, ok := strings.Cut(strings.TrimSpace(line), "\t")
	if !ok || !validHeadLine("HEAD "+id) {
		return ""
	}
	return id
}

// fetchLocks serialises the network fetches of one repository inside this
// process, by resolved repository path.
var fetchLocks repoLocks

// repoLocks holds one mutex per repository, kept only while somebody holds or
// waits for it.
type repoLocks struct {
	mu   sync.Mutex
	held map[string]*repoLock
}

type repoLock struct {
	mu   sync.Mutex
	refs int
}

// lock takes repo's mutex and returns the function that releases it.
func (l *repoLocks) lock(repo string) func() {
	l.mu.Lock()
	if l.held == nil {
		l.held = map[string]*repoLock{}
	}
	entry := l.held[repo]
	if entry == nil {
		entry = &repoLock{}
		l.held[repo] = entry
	}
	entry.refs++
	l.mu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		l.mu.Lock()
		if entry.refs--; entry.refs == 0 {
			delete(l.held, repo)
		}
		l.mu.Unlock()
	}
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
