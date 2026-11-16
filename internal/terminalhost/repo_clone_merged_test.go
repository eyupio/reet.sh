package terminalhost

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// squashMerge lands the branch of the clone at dir on the stand-in for GitHub
// as one new commit on main, the way a squash-merged pull request does: the
// branch's content arrives under an id the clone has never seen.
func (f *cloneFixture) squashMerge(t *testing.T, dir, branch string) {
	t.Helper()
	scratch := filepath.Join(f.home, "scratch-"+branch)
	testGit(t, f.home, "clone", "-q", f.remote, scratch)
	testGit(t, scratch, "fetch", "-q", dir, branch)
	testGit(t, scratch, "merge", "-q", "--squash", "FETCH_HEAD")
	testGit(t, scratch, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "Squash "+branch)
	testGit(t, scratch, "push", "-q", "origin", "HEAD:main")
}

// landOnGitHub commits name on main of the stand-in, as another pull request
// would.
func (f *cloneFixture) landOnGitHub(t *testing.T, name, content string) {
	t.Helper()
	scratch := filepath.Join(f.home, "scratch-"+name)
	testGit(t, f.home, "clone", "-q", f.remote, scratch)
	writeFile(t, filepath.Join(scratch, name), content)
	testGit(t, scratch, "add", name)
	testGit(t, scratch, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", name)
	testGit(t, scratch, "push", "-q", "origin", "HEAD:main")
}

// fetchesFromTheStandIn is f.git, with a fetch of github.com sent to the
// stand-in as the clone is.
func (f *cloneFixture) fetchesFromTheStandIn(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if slices.Contains(args, "fetch") {
		args = append([]string{"-c", "url." + f.remote + ".insteadOf=https://github.com/o/" + f.name + ".git"}, args...)
	}
	return f.git(ctx, dir, args...)
}

func (f *cloneFixture) fetchedStatus(t *testing.T, dir string, git GitRunner) terminalproto.RepoCloneState {
	t.Helper()
	s := CloneStatus(t.Context(), f.home, terminalproto.RepoCloneStatus{Dir: dir, Origin: "o/r", Fetch: true}, git)
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: testRequestID(1), Generation: 1, Body: s}); err != nil {
		t.Fatalf("state is not a valid message: %+v", s)
	}
	return s
}

// A branch whose pull request was squash-merged has commits on no branch of
// origin, but everything it changed is there: nothing is lost with the clone.
func TestCloneStatusCountsASquashMergedBranchAsSafe(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	testGit(t, dir, "checkout", "-q", "-b", "task")
	commitIn(t, dir, "a")
	commitIn(t, dir, "b")
	testGit(t, dir, "checkout", "-q", "main")
	f.squashMerge(t, dir, "task")

	// Until the clone has fetched, it cannot know.
	wantCounts(t, f.status(t, dir, "o/r"), 0, 0, 0, 1)

	testGit(t, dir, "-c", "url."+f.remote+".insteadOf=https://github.com/o/r.git", "fetch", "-q", "origin")
	s := f.status(t, dir, "o/r")
	wantCounts(t, s, 0, 0, 0, 0)

	// And the clone goes, the delete naming exactly what was looked at.
	if r := f.remove(t, dir, "o/r", s.Fingerprint); r.Reason != "" || r.Dir != dir {
		t.Fatalf("remove = %+v", r)
	}
}

// Asking for a fetch is what lets a clone that has not looked since the merge
// see it. Without the ask the machine touches no network.
func TestCloneStatusFetchSeesAMergeTheCloneHasNotFetched(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	testGit(t, dir, "checkout", "-q", "-b", "task")
	commitIn(t, dir, "a")
	testGit(t, dir, "checkout", "-q", "main")
	f.squashMerge(t, dir, "task")

	wantCounts(t, f.statusWith(t, dir, "o/r", f.fetchesFromTheStandIn), 0, 0, 0, 1)
	if calls := f.fetchCalls(); calls != 0 {
		t.Fatalf("a status that did not ask fetched %d times", calls)
	}

	s := f.fetchedStatus(t, dir, f.fetchesFromTheStandIn)
	wantCounts(t, s, 0, 0, 0, 0)
	if calls := f.fetchCalls(); calls != 1 {
		t.Fatalf("fetches = %d, want 1", calls)
	}
	// Only the default branch moved: no tag and no FETCH_HEAD arrived.
	if _, err := testGitErr(dir, "rev-parse", "--verify", "--quiet", "FETCH_HEAD"); err == nil {
		t.Fatal("a fetch wrote FETCH_HEAD")
	}
	// What was shown can be deleted.
	if r := f.removeWith(t, dir, "o/r", s.Fingerprint, f.fetchesFromTheStandIn); r.Reason != "" || r.Dir != dir {
		t.Fatalf("remove = %+v", r)
	}
}

// GitHub not answering must never make a clone look safer.
func TestCloneStatusFetchThatFailsKeepsTheClone(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	testGit(t, dir, "checkout", "-q", "-b", "task")
	commitIn(t, dir, "a")
	testGit(t, dir, "checkout", "-q", "main")
	f.squashMerge(t, dir, "task")

	down := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if slices.Contains(args, "fetch") || slices.Contains(args, "ls-remote") {
			return nil, errors.New("github.com is unreachable")
		}
		return f.git(ctx, dir, args...)
	}
	wantCounts(t, f.fetchedStatus(t, dir, down), 0, 0, 0, 1)
}

// Content that differs from what is on GitHub is work that is not there.
func TestCloneStatusKeepsABranchWhoseContentIsNotOnGitHub(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	testGit(t, dir, "checkout", "-q", "-b", "task")
	commitIn(t, dir, "a")
	testGit(t, dir, "checkout", "-q", "main")
	f.squashMerge(t, dir, "task")

	// A commit after the merge, on a file the merge brought.
	testGit(t, dir, "checkout", "-q", "task")
	writeFile(t, filepath.Join(dir, "a"), "changed after the merge\n")
	testGit(t, dir, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-am", "more")
	testGit(t, dir, "checkout", "-q", "main")
	wantCounts(t, f.fetchedStatus(t, dir, f.fetchesFromTheStandIn), 0, 0, 0, 1)
}

// A file the branch changed that GitHub has since changed again reads
// differently there: it is kept, never guessed to be merged.
func TestCloneStatusKeepsABranchWhoseFilesChangedAgainOnGitHub(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	testGit(t, dir, "checkout", "-q", "-b", "task")
	commitIn(t, dir, "a")
	testGit(t, dir, "checkout", "-q", "main")
	f.squashMerge(t, dir, "task")
	f.landOnGitHub(t, "a", "rewritten by someone else\n")

	wantCounts(t, f.fetchedStatus(t, dir, f.fetchesFromTheStandIn), 0, 0, 0, 1)
}

// A branch that was never merged is not made safe by a fetch.
func TestCloneStatusKeepsAnUnmergedBranchAfterAFetch(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	testGit(t, dir, "checkout", "-q", "-b", "task")
	commitIn(t, dir, "a")
	testGit(t, dir, "checkout", "-q", "main")
	f.landOnGitHub(t, "unrelated", "other work\n")

	wantCounts(t, f.fetchedStatus(t, dir, f.fetchesFromTheStandIn), 0, 0, 0, 1)
}

// fetchCalls is how many times a git call fetched.
func (f *cloneFixture) fetchCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, args := range f.calls {
		if slices.Contains(args, "fetch") {
			n++
		}
	}
	return n
}
