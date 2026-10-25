//go:build linux || darwin

package terminalhost

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestWorktreeRemoveRefusesNestedBareRepositories(t *testing.T) {
	for _, name := range []string{"bare/keep.git", "bare/store"} {
		t.Run(name, func(t *testing.T) {
			home := worktreeHome(t)
			repo := newRepo(t, home, "code/site")
			other := newRepo(t, home, "code/other")
			w := madeWorktree(t, home, repo, testRun)
			ignoreInWorktree(t, w.Path, "bare/")
			bare := filepath.Join(w.Path, name)
			testGit(t, home, "clone", "-q", "--bare", other, bare)
			s, err := removeAsShown(t, home)
			if !s.Nested || s.Changed != 0 {
				t.Fatalf("the bare repository must be reported: %+v", s)
			}
			refusedWith(t, err, terminalproto.WorktreeNestedRepository)
			if !exists(filepath.Join(bare, "HEAD")) {
				t.Fatal("the bare repository was deleted")
			}
		})
	}
}

func TestLooksLikeGitDirectory(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel string) {
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	mk("bare/objects")
	mk("bare/refs")
	writeFile(t, filepath.Join(dir, "bare", "HEAD"), "ref: refs/heads/main\n")
	mk("half/objects")
	writeFile(t, filepath.Join(dir, "half", "HEAD"), "x\n")
	mk("plain")
	for name, want := range map[string]bool{".git": true, ".GIT": true, ".Git": true, "x.git": true, "X.GIT": true, "git": false, "gitx": false, "x.gitignore": false} {
		if got := gitLikeName(name); got != want {
			t.Errorf("gitLikeName(%q) = %v", name, got)
		}
	}
	for rel, want := range map[string]bool{"bare": true, "half": false, "plain": false} {
		if got := hasGitDirLayout(filepath.Join(dir, rel)); got != want {
			t.Errorf("hasGitDirLayout(%q) = %v", rel, got)
		}
	}
}

// A .Git folder (as a case-insensitive file system would find .git) inside
// the worktree is a nested repository too.
func TestWorktreeRemoveRefusesNestedGitInAnyCase(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	ignoreInWorktree(t, w.Path, "inner/")
	if err := os.MkdirAll(filepath.Join(w.Path, "inner", ".Git"), 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := removeAsShown(t, home)
	if !s.Nested {
		t.Fatalf("%+v", s)
	}
	refusedWith(t, err, terminalproto.WorktreeNestedRepository)
}

// A scan that stops early blocks the remove: past the entry cap and past the
// depth cap the result is check_incomplete and everything is kept.
func TestWorktreeRemoveRefusesAPartialScan(t *testing.T) {
	for _, tc := range []struct {
		name           string
		entries, depth int
	}{
		{"entry cap", 5, 256},
		{"depth cap", 200_000, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := worktreeHome(t)
			repo := newRepo(t, home, "code/site")
			w := madeWorktree(t, home, repo, testRun)
			ignoreInWorktree(t, w.Path, "build/")
			deep := filepath.Join(w.Path, "build", "a", "b", "c")
			if err := os.MkdirAll(deep, 0o700); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 10; i++ {
				writeFile(t, filepath.Join(deep, fmt.Sprintf("out%d", i)), "x\n")
			}
			oldEntries, oldDepth := scanEntryLimit, scanDepthLimit
			scanEntryLimit, scanDepthLimit = tc.entries, tc.depth
			t.Cleanup(func() { scanEntryLimit, scanDepthLimit = oldEntries, oldDepth })
			s, err := removeAsShown(t, home)
			if !s.Incomplete || s.Nested {
				t.Fatalf("a partial scan must be reported: %+v", s)
			}
			refusedWith(t, err, terminalproto.WorktreeCheckIncomplete)
			if !exists(filepath.Join(deep, "out9")) || !exists(w.Path) {
				t.Fatal("removed")
			}
			// Within the default caps the same worktree is removed.
			scanEntryLimit, scanDepthLimit = oldEntries, oldDepth
			if _, err := removeAsShown(t, home); err != nil || exists(w.Path) {
				t.Fatal(err)
			}
		})
	}
}

func TestScanWorktreeStopsWhenTheBudgetEnds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "f"), "x")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, incomplete := scanWorktree(ctx, dir, 100, 10); !incomplete {
		t.Fatal("a scan whose budget has ended is incomplete")
	}
}

// Commits held only by refs that live inside the worktree (refs/worktree/*,
// refs/bisect/*) are deleted with it, so they count as unreferenced.
func TestWorktreeRemoveRefusesCommitsHeldByWorktreeRefs(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	testGit(t, w.Path, "checkout", "-q", "--detach")
	lone := commitIn(t, w.Path, "kept-by-a-worktree-ref")
	testGit(t, w.Path, "update-ref", "refs/worktree/keep", lone)
	testGit(t, w.Path, "checkout", "-q", w.Branch)
	s, err := removeAsShown(t, home)
	if s.Unreferenced != 1 {
		t.Fatalf("the commit only refs/worktree holds must be reported: %+v", s)
	}
	refusedWith(t, err, terminalproto.WorktreeUnreferencedCommits)
	if _, err := testGitErr(repo, "cat-file", "-e", lone+"^{commit}"); err != nil {
		t.Fatal("the commit is gone")
	}
}

// refs/rewritten/* is the third per-worktree ref namespace git documents
// (with refs/worktree and refs/bisect): a commit held only there is deleted
// with the worktree, so it counts as unreferenced.
func TestWorktreeRemoveRefusesCommitsHeldByRewrittenRefs(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	testGit(t, w.Path, "checkout", "-q", "--detach")
	lone := commitIn(t, w.Path, "kept-by-a-rewritten-ref")
	testGit(t, w.Path, "update-ref", "refs/rewritten/keep", lone)
	testGit(t, w.Path, "checkout", "-q", w.Branch)
	s, err := removeAsShown(t, home)
	if s.Unreferenced != 1 {
		t.Fatalf("the commit only refs/rewritten holds must be reported: %+v", s)
	}
	refusedWith(t, err, terminalproto.WorktreeUnreferencedCommits)
	if _, err := testGitErr(repo, "cat-file", "-e", lone+"^{commit}"); err != nil {
		t.Fatal("the commit is gone")
	}
}

// The stash lives in the shared repository and survives the remove, so a
// commit it holds is protected and does not block.
func TestWorktreeStashProtectsWithoutBlocking(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	testGit(t, w.Path, "checkout", "-q", "--detach")
	lone := commitIn(t, w.Path, "detached")
	writeFile(t, filepath.Join(w.Path, "README"), "stashed edit\n")
	testGit(t, w.Path, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "stash", "-q")
	// A worktree ref also holds it, so only the stash keeps it out of the count.
	testGit(t, w.Path, "update-ref", "refs/worktree/keep", lone)
	testGit(t, w.Path, "checkout", "-q", w.Branch)
	s, err := removeAsShown(t, home)
	if err != nil || s.Unreferenced != 0 || exists(w.Path) {
		t.Fatal(s, err)
	}
	if _, err := testGitErr(repo, "cat-file", "-e", lone+"^{commit}"); err != nil {
		t.Fatal("the stashed commit is gone")
	}
	if testGit(t, repo, "stash", "list") == "" {
		t.Fatal("the stash is gone")
	}
}

// A per-worktree ref listing that may be cut short makes the check
// incomplete, never a count that might miss a commit.
func TestWorktreeRefListingCutShortIsIncomplete(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	long := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		out, err := ExecGit(ctx, dir, args...)
		if len(args) > 0 && args[0] == "for-each-ref" && dir == w.Path {
			// One ref whose name runs to the limit: few tips, but cut short.
			head := strings.Repeat("a", 40) + " commit refs/worktree/"
			out = []byte(head + strings.Repeat("x", gitListLimit-len(head)))
		}
		return out, err
	}
	s, err := WorktreeStatus(context.Background(), home, "p", "site", testRun, long)
	if err != nil || !s.Incomplete {
		t.Fatal(s, err)
	}
	err = WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, long)
	refusedWith(t, err, terminalproto.WorktreeCheckIncomplete)
	if !exists(w.Path) {
		t.Fatal("removed")
	}
}

func TestTooManyWorktreeRefsIsIncomplete(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	head := testGit(t, w.Path, "rev-parse", "HEAD")
	var refs strings.Builder
	for i := 0; i <= maxWorktreeRefTips; i++ {
		fmt.Fprintf(&refs, "create refs/worktree/r%04d %s\n", i, head)
	}
	input := filepath.Join(home, "refs.txt")
	writeFile(t, input, refs.String())
	f, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cmd := exec.Command("git", "-C", w.Path, "update-ref", "--stdin")
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	if s := status(t, home, "site", testRun); !s.Incomplete || s.Unreferenced != 0 {
		t.Fatalf("more than %d worktree refs: %+v", maxWorktreeRefTips, s)
	}
}
