//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// removeAsShown runs a status and then a remove with exactly the counts it
// showed, as the person confirming them would.
func removeAsShown(t *testing.T, home string) (terminalproto.WorktreeState, error) {
	t.Helper()
	s := status(t, home, "site", testRun)
	return s, WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{Changed: s.Changed, Unpushed: s.Unpushed}, ExecGit)
}

func ignoreInWorktree(t *testing.T, wt, pattern string) {
	t.Helper()
	writeFile(t, filepath.Join(wt, ".gitignore"), pattern+"\n")
	testGit(t, wt, "add", ".gitignore")
	testGit(t, wt, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "ignore")
}

func TestWorktreeRemoveRefusesDetachedCommits(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	testGit(t, w.Path, "checkout", "-q", "--detach")
	lone := commitIn(t, w.Path, "detached-work")
	s, err := removeAsShown(t, home)
	if s.Unreferenced != 1 {
		t.Fatalf("the detached commit must be reported: %+v", s)
	}
	refusedWith(t, err, terminalproto.WorktreeUnreferencedCommits)
	if !exists(w.Path) {
		t.Fatal("removed")
	}
	if _, err := testGitErr(repo, "cat-file", "-e", lone+"^{commit}"); err != nil {
		t.Fatal("the commit is gone")
	}
	// Once the commit is on a branch, it is referenced and the remove goes ahead.
	testGit(t, w.Path, "branch", "keep-it")
	if s, err := removeAsShown(t, home); err != nil || s.Unreferenced != 0 {
		t.Fatal(s, err)
	}
	if got := testGit(t, repo, "rev-parse", "keep-it"); got != lone {
		t.Fatal("branch moved")
	}
}

func TestWorktreeRemoveRefusesIgnoredNestedWorktree(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	ignoreInWorktree(t, w.Path, ".worktrees/")
	nested := filepath.Join(w.Path, ".worktrees", "x")
	testGit(t, w.Path, "worktree", "add", "-q", "-b", "nested-x", nested, "HEAD")
	writeFile(t, filepath.Join(nested, "work-in-progress"), "precious\n")
	s, err := removeAsShown(t, home)
	if !s.Nested || s.Changed != 0 {
		t.Fatalf("the nested worktree must be reported: %+v", s)
	}
	refusedWith(t, err, terminalproto.WorktreeNestedRepository)
	if !exists(filepath.Join(nested, "work-in-progress")) {
		t.Fatal("the nested worktree's work was deleted")
	}
}

func TestWorktreeRemoveRefusesIgnoredNestedClone(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	ignoreInWorktree(t, w.Path, "vendor/")
	clone := filepath.Join(w.Path, "vendor", "x")
	testGit(t, home, "clone", "-q", repo, clone)
	tip := commitIn(t, clone, "vendored-change")
	s, err := removeAsShown(t, home)
	if !s.Nested || s.Changed != 0 {
		t.Fatalf("the nested clone must be reported: %+v", s)
	}
	refusedWith(t, err, terminalproto.WorktreeNestedRepository)
	if got := testGit(t, clone, "rev-parse", "HEAD"); got != tip {
		t.Fatal("the nested clone's commit is gone")
	}
}

// A symlink inside the worktree is never followed: a link to a repository is
// not a nested repository, and removing the worktree removes only the link.
func TestWorktreeScanDoesNotFollowSymlinks(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	other := newRepo(t, home, "code/other")
	w := madeWorktree(t, home, repo, testRun)
	if err := os.Symlink(other, filepath.Join(w.Path, "link")); err != nil {
		t.Fatal(err)
	}
	testGit(t, w.Path, "add", "link")
	testGit(t, w.Path, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "link")
	s, err := removeAsShown(t, home)
	if s.Nested || err != nil {
		t.Fatal(s, err)
	}
	if !exists(filepath.Join(other, ".git")) || !exists(filepath.Join(other, "README")) {
		t.Fatal("the link's target was touched")
	}
}

func TestWorktreeRemoveRefusesHiddenEdits(t *testing.T) {
	for _, flag := range []string{"--skip-worktree", "--assume-unchanged"} {
		t.Run(flag, func(t *testing.T) {
			home := worktreeHome(t)
			repo := newRepo(t, home, "code/site")
			w := madeWorktree(t, home, repo, testRun)
			testGit(t, w.Path, "update-index", flag, "README")
			writeFile(t, filepath.Join(w.Path, "README"), "an edit git does not look at\n")
			s, err := removeAsShown(t, home)
			if s.Hidden != 1 || s.Changed != 0 {
				t.Fatalf("the hidden edit must be reported: %+v", s)
			}
			refusedWith(t, err, terminalproto.WorktreeHiddenChanges)
			data, _ := os.ReadFile(filepath.Join(w.Path, "README"))
			if string(data) != "an edit git does not look at\n" {
				t.Fatal("the edit was lost")
			}
		})
	}
}

func TestWorktreeRemoveRefusesASparseCheckout(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	if err := os.MkdirAll(filepath.Join(repo, "kept"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "elsewhere"), 0o700); err != nil {
		t.Fatal(err)
	}
	commitIn(t, repo, "kept/a")
	commitIn(t, repo, "elsewhere/b") // cone mode always keeps top-level files
	w := madeWorktree(t, home, repo, testRun)
	testGit(t, w.Path, "sparse-checkout", "set", "kept")
	if exists(filepath.Join(w.Path, "elsewhere", "b")) {
		t.Skip("this git did not apply the sparse checkout")
	}
	s, err := removeAsShown(t, home)
	if s.Hidden == 0 {
		t.Fatalf("files outside the sparse checkout must be reported: %+v", s)
	}
	refusedWith(t, err, terminalproto.WorktreeHiddenChanges)
	if !exists(filepath.Join(w.Path, "kept", "a")) {
		t.Fatal("removed")
	}
}

func TestScanWorktreeBounds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".git"), "gitdir: x\n") // the top-level .git file is the worktree's own
	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("f%02d", i)), "x")
	}
	if nested, incomplete := scanWorktree(dir, 100, 10); nested || incomplete {
		t.Fatal("plain worktree", nested, incomplete)
	}
	if _, incomplete := scanWorktree(dir, 10, 10); !incomplete {
		t.Fatal("past the entry cap the scan must say it is incomplete")
	}
	deep := dir
	for i := 0; i < 6; i++ {
		deep = filepath.Join(deep, "d")
	}
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, incomplete := scanWorktree(dir, 100, 3); !incomplete {
		t.Fatal("past the depth cap the scan must say it is incomplete")
	}
	if nested, incomplete := scanWorktree(dir, 100, 10); nested || incomplete {
		t.Fatal("within the caps", nested, incomplete)
	}
	if err := os.MkdirAll(filepath.Join(dir, "a", "b", ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if nested, _ := scanWorktree(dir, 100, 10); !nested {
		t.Fatal("a nested .git folder")
	}
	if err := os.RemoveAll(filepath.Join(dir, "a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nowhere", filepath.Join(dir, "d", ".git")); err != nil {
		t.Fatal(err)
	}
	if nested, _ := scanWorktree(dir, 100, 10); !nested {
		t.Fatal("a nested .git symlink")
	}
	if err := os.Remove(filepath.Join(dir, "d", ".git")); err != nil {
		t.Fatal(err)
	}
	if _, incomplete := scanWorktree(filepath.Join(dir, "missing"), 100, 10); !incomplete {
		t.Fatal("a folder that cannot be read makes the scan incomplete")
	}
	if os.Getuid() != 0 {
		locked := filepath.Join(dir, "locked")
		if err := os.Mkdir(locked, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(locked, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
		if _, incomplete := scanWorktree(dir, 100, 10); !incomplete {
			t.Fatal("an unreadable folder makes the scan incomplete")
		}
	}
}

// git worktree list --porcelain prints about 150 bytes per worktree; with 40
// in one repository the listing must still be read whole. The other worktrees'
// paths and administrative names sort before the run's, so it is listed last
// whichever order git uses.
func TestWorktreeManyWorktreesInOneRepository(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	for i := 0; i < 40; i++ {
		testGit(t, repo, "worktree", "add", "-q", "-b", fmt.Sprintf("other-%02d", i), filepath.Join(home, "+o", fmt.Sprintf("000-worktree-with-a-longish-name-%02d", i)), "HEAD")
	}
	if out := testGit(t, repo, "worktree", "list", "--porcelain"); len(out) <= gitOutputLimit {
		t.Fatalf("the listing (%d bytes) must exceed the old cap for this test", len(out))
	}
	w := madeWorktree(t, home, repo, testRun)
	again, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, ExecGit)
	if err != nil || again.Created || again.Path != w.Path {
		t.Fatal("retry must adopt", again, err)
	}
	if s := status(t, home, "site", testRun); s.Path != w.Path {
		t.Fatal(s)
	}
	if _, err := removeAsShown(t, home); err != nil || exists(w.Path) {
		t.Fatal(err)
	}
}

// A listing as long as the read limit may have been cut short; it is never
// trusted to show (or to not show) a worktree.
func TestTruncatedListingsAreNotTrusted(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	// padded makes a listing exactly gitListLimit long that still parses:
	// for worktree list, one long well-formed record is inserted after the
	// main worktree's, so only the length says it may have been cut short.
	padded := func(arg string) GitRunner {
		return func(ctx context.Context, dir string, args ...string) ([]byte, error) {
			out, err := ExecGit(ctx, dir, args...)
			if len(args) == 0 || args[0] != arg {
				return out, err
			}
			if arg == "worktree" {
				first, rest, _ := strings.Cut(string(out), "\n\n")
				filler := "worktree /" + strings.Repeat("f", gitListLimit-len(out)-len("worktree /\n\n"))
				return []byte(first + "\n\n" + filler + "\n\n" + rest), err
			}
			return append(out, make([]byte, gitListLimit-len(out))...), err
		}
	}
	_, err := WorktreeStatus(context.Background(), home, "p", "site", testRun, padded("worktree"))
	refusedWith(t, err, terminalproto.WorktreeNotRegistered)
	if _, err := prepareWorktree(context.Background(), home, repo, "p", testRun, padded("worktree"), gitTimeout, cleanupTimeout); !errors.Is(err, ErrWorktreeRefused) {
		t.Fatal("a truncated listing must not adopt", err)
	}
	s, err := WorktreeStatus(context.Background(), home, "p", "site", testRun, padded("ls-files"))
	if err != nil || !s.Incomplete {
		t.Fatal("a truncated index listing is incomplete", s, err)
	}
	err = WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, padded("ls-files"))
	refusedWith(t, err, terminalproto.WorktreeCheckIncomplete)
	if !exists(w.Path) {
		t.Fatal("removed")
	}
}

// --ignore-submodules=none: an edit inside a submodule shows as a change even
// when the repository's configuration ignores submodules.
func TestWorktreeStatusSeesSubmoduleEdits(t *testing.T) {
	home := worktreeHome(t)
	lib := newRepo(t, home, "code/lib")
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	testGit(t, w.Path, "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "lib")
	testGit(t, w.Path, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "submodule")
	testGit(t, repo, "config", "diff.ignoreSubmodules", "all")
	writeFile(t, filepath.Join(w.Path, "lib", "README"), "edited inside the submodule\n")
	if out := testGit(t, w.Path, "status", "--porcelain=v1"); out != "" {
		t.Skip("this git does not hide submodule edits with diff.ignoreSubmodules=all")
	}
	s, err := removeAsShown(t, home)
	if s.Changed != 1 || len(s.Paths) != 1 || s.Paths[0] != "lib" {
		t.Fatalf("the submodule edit must be counted: %+v", s)
	}
	if !errors.Is(err, ErrWorktreeRefused) || !exists(filepath.Join(w.Path, "lib", "README")) {
		t.Fatal(err)
	}
}

func TestWorktreeErrorWordingFitsEveryOperation(t *testing.T) {
	for _, reason := range []terminalproto.WorktreeReason{terminalproto.WorktreeHasChanges, terminalproto.WorktreeNotRegistered, terminalproto.WorktreeExists} {
		msg := refuseWorktree(reason, "detail").Error()
		if strings.Contains(msg, "made") || !strings.Contains(msg, string(reason)) || !strings.Contains(msg, "refused") {
			t.Fatal(msg)
		}
	}
}

func TestOnlyReadOnlyListingsGetTheLargerCap(t *testing.T) {
	for _, args := range [][]string{{"worktree", "list", "--porcelain"}, {"status", "--porcelain=v1", "-z"}, {"ls-files", "-v", "-z"}} {
		if outputLimit(args) != gitListLimit {
			t.Fatal(args)
		}
	}
	for _, args := range [][]string{{"worktree", "remove", "x"}, {"worktree", "add"}, {"rev-parse", "--show-toplevel"}, {"cat-file", "-p", "x"}, {"branch", "-D", "x"}, {}} {
		if outputLimit(args) != gitOutputLimit {
			t.Fatal(args)
		}
	}
}
