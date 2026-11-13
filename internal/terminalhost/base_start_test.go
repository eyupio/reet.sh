//go:build linux || darwin

package terminalhost

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// baseFixture is a repository whose origin is https://github.com/o/r, with a
// second local repository standing in for the one on GitHub. The fixture's
// GitRunner rewrites that URL to the second repository's path with
// url.<path>.insteadOf, so a real fetch runs without reaching the network.
// `git config --get remote.origin.url` is not rewritten, so ResolveBase still
// sees a github.com origin.
type baseFixture struct {
	home, repo, remote string

	mu    sync.Mutex
	calls [][]string
}

func newBaseFixture(t *testing.T) *baseFixture {
	t.Helper()
	home := worktreeHome(t)
	remote := newRepo(t, home, "github")
	repo := filepath.Join(home, "local")
	testGit(t, home, "clone", "-q", remote, repo)
	testGit(t, repo, "remote", "set-url", "origin", "https://github.com/o/r")
	return &baseFixture{home: home, repo: repo, remote: remote}
}

// git is the GitRunner ResolveBase is given: it records the arguments and
// teaches git where "github.com/o/r" really is, in both the form an https
// origin is fetched by and the one an ssh origin is rewritten to.
func (f *baseFixture) git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, slices.Clone(args))
	f.mu.Unlock()
	rewrite := []string{
		"-c", "url." + f.remote + ".insteadOf=https://github.com/o/r.git",
		"-c", "url." + f.remote + ".insteadOf=https://github.com/o/r",
	}
	return ExecGit(ctx, dir, append(rewrite, args...)...)
}

// noSSH fails when any git call named an ssh URL: these fetches never invoke
// ssh, whatever the origin is spelled as.
func (f *baseFixture) noSSH(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, args := range f.calls {
		for _, arg := range args {
			if strings.Contains(arg, "ssh://") || strings.Contains(arg, "git@github.com") {
				t.Fatalf("a git call named an ssh URL: %q", args)
			}
		}
	}
}

// fetched reports whether any call was a fetch.
func (f *baseFixture) fetched() bool { return f.count("fetch") > 0 }

// forget drops the calls recorded so far, so a later one can be read alone.
func (f *baseFixture) forget() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// fetchArgs is the arguments of the first recorded fetch, or nil.
func (f *baseFixture) fetchArgs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, args := range f.calls {
		if slices.Contains(args, "fetch") {
			return args
		}
	}
	return nil
}

// count is how many calls had sub as one of their arguments.
func (f *baseFixture) count(sub string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, args := range f.calls {
		if slices.Contains(args, sub) {
			n++
		}
	}
	return n
}

func (f *baseFixture) resolve(t *testing.T, base string, fetch bool) terminalproto.BaseState {
	t.Helper()
	state, err := ResolveBase(context.Background(), f.repo, base, fetch, f.git)
	if err != nil {
		t.Fatalf("ResolveBase(%q, fetch=%v): %v", base, fetch, err)
	}
	if !state.Valid() {
		t.Fatalf("ResolveBase(%q, fetch=%v) returned an invalid state: %+v", base, fetch, state)
	}
	return state
}

// deleteLocalBase leaves HEAD where it is and removes the local main branch,
// as in a repository that has never checked the base out.
func (f *baseFixture) deleteLocalBase(t *testing.T) {
	t.Helper()
	testGit(t, f.repo, "checkout", "--detach", "HEAD")
	testGit(t, f.repo, "branch", "-D", "main")
}

// commitOf is the commit a ref names, which must exist.
func commitOf(t *testing.T, dir, ref string) string {
	t.Helper()
	return testGit(t, dir, "rev-parse", "--verify", ref+"^{commit}")
}

func wantBase(t *testing.T, state terminalproto.BaseState, relation terminalproto.BaseRelation, start string) {
	t.Helper()
	if state.Relation != relation || state.Start != start {
		t.Fatalf("state = %+v, want relation %s starting from %s", state, relation, start)
	}
}

func TestBaseSame(t *testing.T) {
	f := newBaseFixture(t)
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseSame, commitOf(t, f.repo, "refs/heads/main"))
}

func TestBaseBehindStartsFromGitHub(t *testing.T) {
	f := newBaseFixture(t)
	ahead := commitIn(t, f.remote, "on-github")
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseBehind, ahead)
}

func TestBaseNoLocalStartsFromGitHub(t *testing.T) {
	f := newBaseFixture(t)
	ahead := commitIn(t, f.remote, "on-github")
	f.deleteLocalBase(t)
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseNoLocal, ahead)
}

func TestBaseAheadStartsFromLocal(t *testing.T) {
	f := newBaseFixture(t)
	ahead := commitIn(t, f.repo, "local-only")
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseAhead, ahead)
}

func TestBaseDivergedStartsFromLocal(t *testing.T) {
	f := newBaseFixture(t)
	commitIn(t, f.remote, "on-github")
	local := commitIn(t, f.repo, "local-only")
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseDiverged, local)
}

func TestBaseFetchFailedUsesLastKnown(t *testing.T) {
	f := newBaseFixture(t)
	local := commitOf(t, f.repo, "refs/heads/main")
	if err := os.RemoveAll(f.remote); err != nil {
		t.Fatal(err)
	}
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseFetchFailed, local)
}

func TestBaseFetchFailedWithoutLocalUsesRemoteTracking(t *testing.T) {
	f := newBaseFixture(t)
	commitIn(t, f.remote, "on-github")
	f.resolve(t, "main", true) // the last successful fetch
	tracking := commitOf(t, f.repo, "refs/remotes/origin/main")
	f.deleteLocalBase(t)
	if err := os.RemoveAll(f.remote); err != nil {
		t.Fatal(err)
	}
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseFetchFailed, tracking)
}

// A branch that is only in the person's own repository was never on GitHub,
// which the fetch cannot tell from a GitHub it could not reach: one
// ls-remote settles it, and the run starts from the local branch as ahead.
func TestBaseLocalOnlyBranchIsAheadNotFetchFailed(t *testing.T) {
	f := newBaseFixture(t)
	testGit(t, f.repo, "branch", "feature")
	local := commitOf(t, f.repo, "refs/heads/feature")
	state := f.resolve(t, "feature", true)
	wantBase(t, state, terminalproto.BaseAhead, local)
	if state.Remote != "" {
		t.Fatalf("state = %+v, want no commit on GitHub's side", state)
	}
	if n := f.count("ls-remote"); n != 1 {
		t.Fatalf("ls-remote ran %d times, want once", n)
	}
}

// A GitHub that cannot be reached at all still answers fetch_failed: the
// ls-remote fails too, rather than exiting 2 for a missing ref.
func TestBaseFetchFailedStillReportedWhenRemoteIsUnreachable(t *testing.T) {
	f := newBaseFixture(t)
	local := commitOf(t, f.repo, "refs/heads/main")
	if err := os.RemoveAll(f.remote); err != nil {
		t.Fatal(err)
	}
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseFetchFailed, local)
	if n := f.count("ls-remote"); n != 1 {
		t.Fatalf("ls-remote ran %d times, want once", n)
	}
}

// A branch deleted on GitHub that the repository never had is missing, even
// though an old fetch left a remote-tracking branch behind.
func TestBaseMissingOnBothSidesAfterLsRemote(t *testing.T) {
	f := newBaseFixture(t)
	testGit(t, f.remote, "branch", "feature")
	f.resolve(t, "feature", true) // leaves refs/remotes/origin/feature behind
	testGit(t, f.remote, "branch", "-D", "feature")
	_, err := ResolveBase(context.Background(), f.repo, "feature", true, f.git)
	refusedWith(t, err, terminalproto.WorktreeBaseMissing)
}

// An ssh github.com origin is fetched over https with gh's credentials: the
// gh helper only serves https, and the hardened environment has no ssh agent
// to ask, so ssh is never invoked.
func TestBaseSSHOriginIsFetchedOverHTTPSWithGh(t *testing.T) {
	for name, origin := range map[string]string{"scp": "git@github.com:o/r.git", "ssh": "ssh://git@github.com/o/r"} {
		t.Run(name, func(t *testing.T) {
			f := newBaseFixture(t)
			gh := fakeGh(t)
			testGit(t, f.repo, "remote", "set-url", "origin", origin)
			ahead := commitIn(t, f.remote, "on-github")
			wantBase(t, f.resolve(t, "main", true), terminalproto.BaseBehind, ahead)
			args := f.fetchArgs()
			if !slices.Contains(args, "https://github.com/o/r.git") {
				t.Fatalf("fetch %q did not name the repository's https URL", args)
			}
			if !slices.Contains(args, "credential.https://github.com.helper=!'"+gh+"' auth git-credential") {
				t.Fatalf("fetch %q did not carry gh's credentials", args)
			}
			f.noSSH(t)
		})
	}
}

// With no gh to authenticate as, an ssh origin is not fetched at all: the
// base is the local branch, as it was before any base was fetched, and
// nothing is said about GitHub.
func TestBaseSSHOriginWithoutGhIsNotFetched(t *testing.T) {
	f := newBaseFixture(t)
	withoutGh(t)
	testGit(t, f.repo, "remote", "set-url", "origin", "git@github.com:o/r.git")
	commitIn(t, f.remote, "on-github")
	local := commitOf(t, f.repo, "refs/heads/main")
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseNoRemote, local)
	if f.fetched() {
		t.Fatalf("an ssh origin was fetched without gh: %q", f.calls)
	}
	f.noSSH(t)
}

// Several resolves of the same repository at once all answer from GitHub's
// branch. Concurrent fetches of one refspec make git fail every one but the
// winner ("unable to update local ref"), so they are taken one at a time and
// a fetch that failed all the same is settled against what GitHub printed.
func TestBaseConcurrentResolvesAllSeeGitHubsCommit(t *testing.T) {
	f := newBaseFixture(t)
	ahead := commitIn(t, f.remote, "on-github")
	const n = 12
	states := make([]terminalproto.BaseState, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			states[i], errs[i] = ResolveBase(context.Background(), f.repo, "main", true, f.git)
		}()
	}
	wg.Wait()
	for i, state := range states {
		if errs[i] != nil {
			t.Fatalf("resolve %d: %v", i, errs[i])
		}
		if state.Relation != terminalproto.BaseBehind || state.Start != ahead {
			t.Fatalf("resolve %d = %+v, want behind starting from GitHub's %s", i, state, ahead)
		}
	}
}

// A fetch that failed although it did update the tracking ref (the loser of a
// race that ran far enough) is not a GitHub that could not be reached: the
// id ls-remote printed is already there, so the base is fetched.
func TestBaseFetchFailureWithTheTrackingRefAlreadyThereIsFetched(t *testing.T) {
	f := newBaseFixture(t)
	ahead := commitIn(t, f.remote, "on-github")
	fetches := 0
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if slices.Contains(args, "fetch") {
			fetches++
			out, _ := f.git(ctx, dir, args...)
			// The fetch did its work and then failed, as the loser of a race
			// for the same refspec does.
			return out, exitStatus(t, 1)
		}
		return f.git(ctx, dir, args...)
	}
	state, err := ResolveBase(context.Background(), f.repo, "main", true, git)
	if err != nil {
		t.Fatal(err)
	}
	if state.Relation != terminalproto.BaseBehind || state.Start != ahead {
		t.Fatalf("state = %+v, want behind starting from %s", state, ahead)
	}
	if fetches != 1 {
		t.Fatalf("the fetch ran %d times, want once", fetches)
	}
}

// A fetch that failed and left the tracking ref behind what GitHub printed is
// tried once more before GitHub is called unreachable.
func TestBaseFetchFailureIsRetriedOnceWhenGitHubAnswered(t *testing.T) {
	f := newBaseFixture(t)
	ahead := commitIn(t, f.remote, "on-github")
	fetches := 0
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if slices.Contains(args, "fetch") {
			if fetches++; fetches == 1 {
				return []byte("fatal: unable to update local ref\n"), exitStatus(t, 1)
			}
		}
		return f.git(ctx, dir, args...)
	}
	state, err := ResolveBase(context.Background(), f.repo, "main", true, git)
	if err != nil {
		t.Fatal(err)
	}
	if state.Relation != terminalproto.BaseBehind || state.Start != ahead {
		t.Fatalf("state = %+v, want behind starting from %s", state, ahead)
	}
	if fetches != 2 {
		t.Fatalf("the fetch ran %d times, want twice", fetches)
	}
}

// A fetch that keeps failing although GitHub answers is fetch_failed, after
// exactly one retry.
func TestBaseFetchFailureRetriedOnceThenUnreachable(t *testing.T) {
	f := newBaseFixture(t)
	commitIn(t, f.remote, "on-github")
	local := commitOf(t, f.repo, "refs/heads/main")
	fetches := 0
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if slices.Contains(args, "fetch") {
			fetches++
			return []byte("fatal: unable to update local ref\n"), exitStatus(t, 1)
		}
		return f.git(ctx, dir, args...)
	}
	state, err := ResolveBase(context.Background(), f.repo, "main", true, git)
	if err != nil {
		t.Fatal(err)
	}
	if state.Relation != terminalproto.BaseFetchFailed || state.Start != local {
		t.Fatalf("state = %+v, want fetch_failed starting from %s", state, local)
	}
	if fetches != 2 {
		t.Fatalf("the fetch ran %d times, want twice", fetches)
	}
}

func TestBaseNonGitHubOriginIsNotFetched(t *testing.T) {
	f := newBaseFixture(t)
	testGit(t, f.repo, "remote", "set-url", "origin", "https://gitlab.com/a/b")
	local := commitOf(t, f.repo, "refs/heads/main")
	wantBase(t, f.resolve(t, "main", true), terminalproto.BaseNoRemote, local)
	if f.fetched() {
		t.Fatalf("a non-GitHub origin was fetched: %q", f.calls)
	}
}

func TestBaseMissingEverywhere(t *testing.T) {
	f := newBaseFixture(t)
	_, err := ResolveBase(context.Background(), f.repo, "nope", true, f.git)
	refusedWith(t, err, terminalproto.WorktreeBaseMissing)
}

func TestBaseFetchIsBoundedAt30Seconds(t *testing.T) {
	f := newBaseFixture(t)
	// GitHub has moved on, so the tracking ref a failed fetch leaves behind
	// is not the commit ls-remote prints and the fetch really did fail.
	commitIn(t, f.remote, "on-github")
	local := commitOf(t, f.repo, "refs/heads/main")
	var deadline time.Time
	var bounded bool
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if !slices.Contains(args, "fetch") {
			return f.git(ctx, dir, args...)
		}
		// A fetch that hangs is cut off by the bound of its own context. The
		// test asserts that bound instead of waiting 30 seconds for it.
		deadline, bounded = ctx.Deadline()
		return nil, context.DeadlineExceeded
	}
	state, err := ResolveBase(context.Background(), f.repo, "main", true, git)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if !bounded {
		t.Fatal("the fetch ran without a deadline of its own")
	}
	if left := time.Until(deadline); left > 30*time.Second || left < 25*time.Second {
		t.Fatalf("the fetch had %v left, want at most 30 seconds", left)
	}
	wantBase(t, state, terminalproto.BaseFetchFailed, local)
}

func TestBaseEmptyUsesTheRemoteDefault(t *testing.T) {
	f := newBaseFixture(t)
	testGit(t, f.remote, "branch", "trunk")
	testGit(t, f.repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	state := f.resolve(t, "", true)
	if state.Base != "trunk" {
		t.Fatalf("state = %+v, want the remote default trunk", state)
	}
	wantBase(t, state, terminalproto.BaseNoLocal, commitOf(t, f.remote, "refs/heads/trunk"))
}

func TestBaseWithoutFetchComparesTheLastFetchedRef(t *testing.T) {
	f := newBaseFixture(t)
	commitIn(t, f.remote, "on-github")
	local := commitOf(t, f.repo, "refs/heads/main")
	wantBase(t, f.resolve(t, "main", false), terminalproto.BaseSame, local)
	if f.fetched() {
		t.Fatalf("a read-only resolve fetched: %q", f.calls)
	}
}

// TestWithGitEnvAddsOneCallsVariables covers the plumbing the fetch needs:
// GIT_TERMINAL_PROMPT=0 reaches git although gitEnvironment strips every
// GIT_* variable, and only for the call that asked for it.
func TestWithGitEnvAddsOneCallsVariables(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "env")
	ctx := withGitEnv(context.Background(), "GIT_AUTHOR_NAME=Env Test", "GIT_AUTHOR_EMAIL=env@example.invalid")
	out, err := ExecGit(ctx, repo, "var", "GIT_AUTHOR_IDENT")
	if err != nil {
		t.Fatalf("git var: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "Env Test <env@example.invalid>") {
		t.Fatalf("git did not see the call's own variables: %q", out)
	}
	out, err = ExecGit(context.Background(), repo, "var", "GIT_AUTHOR_IDENT")
	if err != nil || strings.Contains(string(out), "Env Test") {
		t.Fatalf("the variables outlived the call: %v: %q", err, out)
	}
}

func TestBaseWithoutFetchAndWithoutATrackingRefUsesLocal(t *testing.T) {
	f := newBaseFixture(t)
	testGit(t, f.repo, "update-ref", "-d", "refs/remotes/origin/main")
	local := commitOf(t, f.repo, "refs/heads/main")
	wantBase(t, f.resolve(t, "main", false), terminalproto.BaseNoRemote, local)
}

// The one call that reaches github.com runs with the repository's hooks off
// and moves nothing but refs/remotes/origin/<base>: no tags, no FETCH_HEAD.
func TestBaseFetchArgvIsHardened(t *testing.T) {
	f := newBaseFixture(t)
	f.resolve(t, "main", true)
	args := f.fetchArgs()
	hooks := slices.Index(args, "core.hooksPath=/dev/null")
	if hooks < 1 || args[hooks-1] != "-c" {
		t.Fatalf("fetch %q does not disable the repository's hooks", args)
	}
	for _, want := range []string{"--no-tags", "--no-write-fetch-head"} {
		if !slices.Contains(args, want) {
			t.Fatalf("fetch %q is missing %s", args, want)
		}
	}
}

// A base spelled origin/<name> names GitHub's own branch: it is fetched and
// started from, whatever the repository's own <name> holds.
func TestBaseOriginPrefixedStartsFromGitHub(t *testing.T) {
	f := newBaseFixture(t)
	onGitHub := commitOf(t, f.remote, "refs/heads/main")
	ahead := commitIn(t, f.repo, "local-only")
	state := f.resolve(t, "origin/main", true)
	wantBase(t, state, terminalproto.BaseSame, onGitHub)
	if state.Start == ahead {
		t.Fatalf("state = %+v, want GitHub's branch, not the local one", state)
	}
	if state.Local != "" || state.Remote != onGitHub {
		t.Fatalf("state = %+v, want GitHub's commit alone", state)
	}
	if args := f.fetchArgs(); !slices.Contains(args, "+refs/heads/main:refs/remotes/origin/main") {
		t.Fatalf("fetch %q did not fetch main from origin", args)
	}
}

// A GitHub that cannot be reached leaves the last fetched origin/<name>.
func TestBaseOriginPrefixedFetchFailedUsesLastFetched(t *testing.T) {
	f := newBaseFixture(t)
	tracking := commitOf(t, f.repo, "refs/remotes/origin/main")
	if err := os.RemoveAll(f.remote); err != nil {
		t.Fatal(err)
	}
	state := f.resolve(t, "origin/main", true)
	wantBase(t, state, terminalproto.BaseFetchFailed, tracking)
}

// A branch GitHub does not have is missing, whether or not an older fetch
// left a remote-tracking branch behind: the person asked for GitHub's branch.
func TestBaseOriginPrefixedMissingIsRefused(t *testing.T) {
	f := newBaseFixture(t)
	_, err := ResolveBase(context.Background(), f.repo, "origin/nope", true, f.git)
	refusedWith(t, err, terminalproto.WorktreeBaseMissing)

	testGit(t, f.remote, "branch", "feature")
	f.resolve(t, "origin/feature", true) // leaves refs/remotes/origin/feature behind
	testGit(t, f.remote, "branch", "-D", "feature")
	_, err = ResolveBase(context.Background(), f.repo, "origin/feature", true, f.git)
	refusedWith(t, err, terminalproto.WorktreeBaseMissing)
}

// A local branch literally named origin/<name> is what the base means, as it
// always has, and is resolved by the ordinary rule.
func TestBaseLocalBranchNamedLikeARemoteWins(t *testing.T) {
	f := newBaseFixture(t)
	testGit(t, f.repo, "branch", "origin/x")
	local := commitOf(t, f.repo, "refs/heads/origin/x")
	state := f.resolve(t, "origin/x", true)
	wantBase(t, state, terminalproto.BaseAhead, local)
	if state.Local != local {
		t.Fatalf("state = %+v, want the local branch %s", state, local)
	}
}

// Another remote's tracking branch still resolves on a github.com repository:
// GitHub has no such branch, and the fetch spent on finding that out must not
// turn the base into a refusal.
func TestBaseOtherRemoteStillResolves(t *testing.T) {
	f := newBaseFixture(t)
	upstream := newRepo(t, f.home, "upstream")
	testGit(t, f.repo, "remote", "add", "upstream", upstream)
	testGit(t, f.repo, "fetch", "-q", "upstream")
	want := commitOf(t, upstream, "refs/heads/main")
	state := f.resolve(t, "upstream/main", true)
	wantBase(t, state, terminalproto.BaseNoRemote, want)
}

// A base check says where a run would start and changes nothing but the
// remote-tracking branch its fetch updates: no worktree is made, and the
// repository's own checkout and branches are left exactly as they were.
func TestCheckBaseReportsWithoutMakingAWorktree(t *testing.T) {
	f := newBaseFixture(t)
	ahead := commitIn(t, f.remote, "on-github")
	local := commitOf(t, f.repo, "refs/heads/main")
	worktrees := testGit(t, f.repo, "worktree", "list")
	branches := testGit(t, f.repo, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads")

	result := CheckBase(context.Background(), f.home, terminalproto.BaseCheck{Dir: f.repo, Base: "main"}, f.git)
	if result.Reason != "" {
		t.Fatalf("check refused: %s", result.Reason)
	}
	if !result.State.Valid() {
		t.Fatalf("invalid state: %+v", result.State)
	}
	wantBase(t, result.State, terminalproto.BaseBehind, ahead)
	if result.State.Base != "main" || result.State.Local != local || result.State.Remote != ahead {
		t.Fatalf("state = %+v, want main local %s remote %s", result.State, local, ahead)
	}
	if !f.fetched() {
		t.Fatalf("the base was not fetched: %q", f.calls)
	}
	if got := testGit(t, f.repo, "worktree", "list"); got != worktrees {
		t.Fatalf("worktrees changed:\nbefore %s\nafter  %s", worktrees, got)
	}
	if got := testGit(t, f.repo, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads"); got != branches {
		t.Fatalf("branches changed:\nbefore %s\nafter  %s", branches, got)
	}
	if got := commitOf(t, f.repo, "HEAD"); got != local {
		t.Fatalf("the checkout moved to %s, want %s", got, local)
	}
	// The one write the check is allowed: the fetched remote-tracking branch.
	if got := commitOf(t, f.repo, "refs/remotes/origin/main"); got != ahead {
		t.Fatalf("refs/remotes/origin/main = %s, want %s", got, ahead)
	}
}

// A branch neither side has is a reason the browser can show, not an error.
func TestCheckBaseAnswersAMissingBranchWithAReason(t *testing.T) {
	f := newBaseFixture(t)
	result := CheckBase(context.Background(), f.home, terminalproto.BaseCheck{Dir: f.repo, Base: "nope"}, f.git)
	if result.Reason != terminalproto.BranchesBaseMissing || result.State != (terminalproto.BaseState{}) {
		t.Fatalf("result = %+v, want reason %s", result, terminalproto.BranchesBaseMissing)
	}
}

// A folder that is not a repository top inside home is refused before git is
// asked, as a branch listing is.
func TestCheckBaseRefusesAFolderThatIsNotARepository(t *testing.T) {
	f := newBaseFixture(t)
	result := CheckBase(context.Background(), f.home, terminalproto.BaseCheck{Dir: f.home, Base: "main"}, f.git)
	if result.Reason != terminalproto.BranchesNotRepoRoot {
		t.Fatalf("result = %+v, want reason %s", result, terminalproto.BranchesNotRepoRoot)
	}
	if f.fetched() {
		t.Fatalf("a folder that is not a repository was fetched: %q", f.calls)
	}
}
