//go:build linux || darwin

package terminalhost

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const testRun = "0123456789abcdef0123456789abcdef"
const otherRun = "fedcba9876543210fedcba9876543210"

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH; the worktree tests need real git")
	}
}

// worktreeHome is a resolved temporary home. HOME points at it so the
// developer's own git configuration never reaches ExecGit in these tests.
func worktreeHome(t *testing.T) string {
	t.Helper()
	requireGit(t)
	home, err := filepath.EvalSymlinks(shortTempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	return home
}

// testGit runs real git for test setup, never through ExecGit, with a
// clean environment of its own.
func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := testGitErr(dir, args...)
	if err != nil {
		t.Fatalf("git %q in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(out)
}

func testGitErr(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	env := []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// newRepo makes a repository with one commit at home/<rel>.
func newRepo(t *testing.T, home, rel string) string {
	t.Helper()
	dir := filepath.Join(home, rel)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	testGit(t, dir, "init", "-q", "-b", "main")
	testGit(t, dir, "config", "user.name", "Reet Test")
	testGit(t, dir, "config", "user.email", "test@example.invalid")
	testGit(t, dir, "config", "commit.gpgsign", "false")
	writeFile(t, filepath.Join(dir, "README"), "hello\n")
	testGit(t, dir, "add", "README")
	testGit(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func refusedWith(t *testing.T, err error, reason terminalproto.WorktreeReason) {
	t.Helper()
	var w *WorktreeError
	if !errors.Is(err, ErrWorktreeRefused) || !errors.As(err, &w) || w.Reason != reason {
		t.Fatalf("want refusal %s, got %v", reason, err)
	}
	r, ok := WorktreeResult(err)
	if !ok || r.Code != terminalproto.WorktreeRefused || r.Reason != reason {
		t.Fatalf("result for %v: %+v", err, r)
	}
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: testRequestID(1), Generation: 1, Body: r}); err != nil {
		t.Fatalf("refusal result is not a valid message: %+v", r)
	}
}

// reetBranches lists the reet/* branches of repo.
func reetBranches(t *testing.T, repo string) []string {
	t.Helper()
	out := testGit(t, repo, "for-each-ref", "--format=%(refname)", "refs/heads/reet/")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func worktreeCount(t *testing.T, repo string) int {
	t.Helper()
	return strings.Count(testGit(t, repo, "worktree", "list", "--porcelain"), "worktree ")
}

// nothingCreated: no reet branch in repo and nothing under ~/.reet/worktrees.
func nothingCreated(t *testing.T, home, repo string) {
	t.Helper()
	if b := reetBranches(t, repo); len(b) != 0 {
		t.Fatal("a branch was created", b)
	}
	if n := worktreeCount(t, repo); n != 1 {
		t.Fatal("a worktree was registered", n)
	}
	filepath.WalkDir(filepath.Join(home, ".reet", "worktrees"), func(path string, d os.DirEntry, err error) error {
		if err == nil && d.Name() == testRun {
			t.Fatal("a destination was created", path)
		}
		return nil
	})
}

func wantPath(home, project, repo, run string) string {
	return filepath.Join(home, ".reet", "worktrees", project, repo, run)
}

func TestPrepareWorktreeCreatesBranchAtHead(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/Site")
	head := testGit(t, repo, "rev-parse", "HEAD")
	w, err := PrepareWorktree(context.Background(), home, repo, "My Project", testRun, "", ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	if !w.Created || w.Path != wantPath(home, "my-project", "site", testRun) || w.Branch != "reet/my-project-"+testRun {
		t.Fatalf("%+v", w)
	}
	if got := testGit(t, w.Path, "rev-parse", "HEAD"); got != head {
		t.Fatal("worktree is not at HEAD", got, head)
	}
	if got := testGit(t, w.Path, "symbolic-ref", "HEAD"); got != "refs/heads/"+w.Branch {
		t.Fatal("worktree is not on its branch", got)
	}
	if got := testGit(t, w.Path, "rev-parse", "--show-toplevel"); got != w.Path {
		t.Fatal("worktree top", got)
	}
	if !strings.Contains(testGit(t, repo, "worktree", "list", "--porcelain"), "worktree "+w.Path+"\n") {
		t.Fatal("worktree not registered")
	}
	if got := testGit(t, repo, "symbolic-ref", "HEAD"); got != "refs/heads/main" {
		t.Fatal("the repository's own branch changed", got)
	}
	for _, dir := range []string{".reet", ".reet/worktrees", ".reet/worktrees/my-project", ".reet/worktrees/my-project/site"} {
		info, err := os.Lstat(filepath.Join(home, dir))
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatal("folder not private", dir, info, err)
		}
	}
}

func TestPrepareWorktreeFollowsSymlinkedRepoInsideHome(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "real")
	link := filepath.Join(home, "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	w, err := PrepareWorktree(context.Background(), home, link, "p", testRun, "", ExecGit)
	if err != nil || w.Path != wantPath(home, "p", "real", testRun) {
		t.Fatal(w, err)
	}
}

func TestPrepareWorktreeRefusesWhatIsNotARepositoryRoot(t *testing.T) {
	home := worktreeHome(t)
	main := newRepo(t, home, "main")
	cases := map[string]func() string{
		"linked worktree (.git file)": func() string {
			dir := filepath.Join(home, "linked")
			testGit(t, main, "worktree", "add", "-q", "-b", "side", dir)
			return dir
		},
		"submodule-style .git file": func() string {
			dir := filepath.Join(home, "gitfile")
			os.MkdirAll(dir, 0o700)
			writeFile(t, filepath.Join(dir, ".git"), "gitdir: "+filepath.Join(main, ".git")+"\n")
			return dir
		},
		"bare repository": func() string {
			dir := filepath.Join(home, "bare.git")
			os.MkdirAll(dir, 0o700)
			testGit(t, dir, "init", "-q", "--bare")
			return dir
		},
		"bare repository in .git": func() string {
			dir := filepath.Join(home, "barein")
			os.MkdirAll(dir, 0o700)
			testGit(t, dir, "init", "-q", "--bare", ".git")
			return dir
		},
		"subdirectory of a repository": func() string {
			dir := filepath.Join(main, "sub")
			os.MkdirAll(dir, 0o700)
			return dir
		},
		"empty .git folder in a subdirectory": func() string {
			dir := filepath.Join(main, "fake")
			os.MkdirAll(filepath.Join(dir, ".git"), 0o700)
			return dir
		},
		".git symlink": func() string {
			dir := filepath.Join(home, "symgit")
			os.MkdirAll(dir, 0o700)
			if err := os.Symlink(filepath.Join(main, ".git"), filepath.Join(dir, ".git")); err != nil {
				t.Fatal(err)
			}
			return dir
		},
		"core.worktree elsewhere": func() string {
			dir := newRepo(t, home, "moved")
			other := filepath.Join(home, "other-tree")
			os.MkdirAll(other, 0o700)
			testGit(t, dir, "config", "core.worktree", other)
			return dir
		},
		"plain folder": func() string {
			dir := filepath.Join(home, "plain")
			os.MkdirAll(dir, 0o700)
			return dir
		},
		"home itself": func() string {
			testGit(t, home, "init", "-q", "-b", "main")
			testGit(t, home, "config", "user.name", "Reet Test")
			testGit(t, home, "config", "user.email", "test@example.invalid")
			testGit(t, home, "commit", "-q", "--allow-empty", "-m", "dotfiles")
			t.Cleanup(func() { os.RemoveAll(filepath.Join(home, ".git")) })
			return home
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := setup()
			_, err := PrepareWorktree(context.Background(), home, dir, "p", testRun, "", ExecGit)
			refusedWith(t, err, terminalproto.WorktreeNotRepoRoot)
			if _, err := os.Lstat(wantPath(home, "p", filepath.Base(dir), testRun)); err == nil {
				t.Fatal("destination created")
			}
			if b := reetBranches(t, main); len(b) != 0 {
				t.Fatal("branch created in the main repository", b)
			}
		})
	}
}

func TestPrepareWorktreeRefusesUnbornHead(t *testing.T) {
	home := worktreeHome(t)
	dir := filepath.Join(home, "empty")
	os.MkdirAll(dir, 0o700)
	testGit(t, dir, "init", "-q", "-b", "main")
	_, err := PrepareWorktree(context.Background(), home, dir, "p", testRun, "", ExecGit)
	refusedWith(t, err, terminalproto.WorktreeNotRepoRoot)
	if _, err := os.Lstat(filepath.Join(home, ".reet")); err == nil {
		t.Fatal("~/.reet made for a refused repository")
	}
	if out := testGit(t, dir, "for-each-ref"); out != "" {
		t.Fatal("refs created", out)
	}
}

func TestPrepareWorktreeRefusesBusyRepository(t *testing.T) {
	home := worktreeHome(t)
	for _, marker := range busyMarkers {
		t.Run("marker "+marker, func(t *testing.T) {
			repo := newRepo(t, home, "m-"+strings.ToLower(marker))
			path := filepath.Join(repo, ".git", marker)
			if strings.HasPrefix(marker, "rebase-") {
				os.Mkdir(path, 0o700)
			} else {
				writeFile(t, path, testGit(t, repo, "rev-parse", "HEAD")+"\n")
			}
			_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
			refusedWith(t, err, terminalproto.WorktreeRepoBusy)
			nothingCreated(t, home, repo)
		})
	}
	// Real half-finished operations, made by git itself.
	conflicted := func(name string) string {
		repo := newRepo(t, home, name)
		writeFile(t, filepath.Join(repo, "README"), "ours\n")
		testGit(t, repo, "commit", "-q", "-am", "ours")
		testGit(t, repo, "checkout", "-q", "-b", "theirs", "HEAD~1")
		writeFile(t, filepath.Join(repo, "README"), "theirs\n")
		testGit(t, repo, "commit", "-q", "-am", "theirs")
		return repo
	}
	for name, start := range map[string][]string{
		"merge":       {"merge", "main"},
		"rebase":      {"rebase", "main"},
		"cherry-pick": {"cherry-pick", "main"},
		"revert":      {"revert", "--no-edit", "HEAD"},
	} {
		t.Run("real "+name, func(t *testing.T) {
			repo := conflicted("real-" + name)
			if name == "revert" {
				// Reverting "theirs" on top of a commit that changes the same
				// line conflicts.
				writeFile(t, filepath.Join(repo, "README"), "later\n")
				testGit(t, repo, "commit", "-q", "-am", "later")
				start = []string{"revert", "--no-edit", "HEAD~1"}
			}
			if out, err := testGitErr(repo, start...); err == nil {
				t.Fatalf("expected a conflict from %v: %s", start, out)
			}
			_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
			refusedWith(t, err, terminalproto.WorktreeRepoBusy)
			nothingCreated(t, home, repo)
		})
	}
	t.Run("real bisect", func(t *testing.T) {
		repo := newRepo(t, home, "bisect")
		testGit(t, repo, "bisect", "start")
		_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
		refusedWith(t, err, terminalproto.WorktreeRepoBusy)
		nothingCreated(t, home, repo)
	})
}

func TestPrepareWorktreeAllowsDirtyTree(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "dirty")
	writeFile(t, filepath.Join(repo, "README"), "changed\n")
	writeFile(t, filepath.Join(repo, "untracked"), "new\n")
	testGit(t, repo, "add", "README")
	writeFile(t, filepath.Join(repo, "README"), "changed again\n")
	w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
	if err != nil || !w.Created {
		t.Fatal(w, err)
	}
	data, _ := os.ReadFile(filepath.Join(w.Path, "README"))
	if string(data) != "hello\n" {
		t.Fatal("worktree does not hold HEAD", string(data))
	}
	if _, err := os.Lstat(filepath.Join(w.Path, "untracked")); err == nil {
		t.Fatal("untracked file copied")
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "README")); string(data) != "changed again\n" {
		t.Fatal("the repository's changes were touched")
	}
}

func TestPrepareWorktreeRefusesExistingDestinationOrBranch(t *testing.T) {
	home := worktreeHome(t)
	t.Run("destination folder", func(t *testing.T) {
		repo := newRepo(t, home, "a/dest")
		dest := wantPath(home, "p", "dest", testRun)
		os.MkdirAll(dest, 0o700)
		writeFile(t, filepath.Join(dest, "keep"), "mine")
		_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
		refusedWith(t, err, terminalproto.WorktreeExists)
		if data, _ := os.ReadFile(filepath.Join(dest, "keep")); string(data) != "mine" {
			t.Fatal("existing destination touched")
		}
		if b := reetBranches(t, repo); len(b) != 0 {
			t.Fatal(b)
		}
	})
	t.Run("destination symlink", func(t *testing.T) {
		repo := newRepo(t, home, "b/link")
		dest := wantPath(home, "p", "link", testRun)
		os.MkdirAll(filepath.Dir(dest), 0o700)
		target := shortTempDir(t)
		os.Symlink(target, dest)
		_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
		refusedWith(t, err, terminalproto.WorktreeExists)
		if entries, _ := os.ReadDir(target); len(entries) != 0 {
			t.Fatal("wrote through the symlink")
		}
	})
	t.Run("branch", func(t *testing.T) {
		repo := newRepo(t, home, "c/branch")
		testGit(t, repo, "branch", "reet/p-"+testRun)
		before := testGit(t, repo, "rev-parse", "reet/p-"+testRun)
		writeFile(t, filepath.Join(repo, "README"), "moved\n")
		testGit(t, repo, "commit", "-q", "-am", "move on")
		_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
		refusedWith(t, err, terminalproto.WorktreeExists)
		if after := testGit(t, repo, "rev-parse", "reet/p-"+testRun); after != before {
			t.Fatal("existing branch moved")
		}
		if _, err := os.Lstat(wantPath(home, "p", "branch", testRun)); err == nil {
			t.Fatal("destination created")
		}
	})
	t.Run("registered worktree on another branch", func(t *testing.T) {
		repo := newRepo(t, home, "d/switched")
		w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
		if err != nil {
			t.Fatal(err)
		}
		testGit(t, w.Path, "switch", "-q", "-c", "elsewhere")
		_, err = PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
		refusedWith(t, err, terminalproto.WorktreeExists)
	})
	t.Run("branch left after the folder was deleted", func(t *testing.T) {
		repo := newRepo(t, home, "e/stale")
		w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
		if err != nil {
			t.Fatal(err)
		}
		os.RemoveAll(w.Path)
		_, err = PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
		refusedWith(t, err, terminalproto.WorktreeExists)
	})
}

func TestPrepareWorktreeRetryAdopts(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "retry")
	first, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
	if err != nil || !first.Created {
		t.Fatal(first, err)
	}
	writeFile(t, filepath.Join(first.Path, "work"), "agent output")
	calls, git := recordingGit(ExecGit)
	again, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", git)
	if err != nil || again.Created || again.Path != first.Path || again.Branch != first.Branch {
		t.Fatal(again, err)
	}
	for _, c := range calls() {
		if mutating(c.args) {
			t.Fatal("a retry ran a mutating git command", c.args)
		}
	}
	if n := worktreeCount(t, repo); n != 2 {
		t.Fatal("worktrees", n)
	}
	if b := reetBranches(t, repo); len(b) != 1 {
		t.Fatal("branches", b)
	}
	// Undo of an adopted worktree does nothing at all.
	undoCalls, undoGit := recordingGit(ExecGit)
	if err := again.Undo(context.Background(), undoGit); err != nil || len(undoCalls()) != 0 {
		t.Fatal("undo of an adopted worktree ran git", undoCalls(), err)
	}
	if data, _ := os.ReadFile(filepath.Join(first.Path, "work")); string(data) != "agent output" {
		t.Fatal("adopted worktree touched")
	}
}

func TestPrepareWorktreeSameTokenOtherRepositoryRefused(t *testing.T) {
	home := worktreeHome(t)
	a := newRepo(t, home, "one/site")
	b := newRepo(t, home, "two/site")
	w, err := PrepareWorktree(context.Background(), home, a, "p", testRun, "", ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	_, err = PrepareWorktree(context.Background(), home, b, "p", testRun, "", ExecGit)
	refusedWith(t, err, terminalproto.WorktreeExists)
	if br := reetBranches(t, b); len(br) != 0 || worktreeCount(t, b) != 1 {
		t.Fatal("the second repository changed", br)
	}
	if got := testGit(t, w.Path, "rev-parse", "--git-common-dir"); got != filepath.Join(a, ".git") {
		t.Fatal("first worktree changed", got)
	}
	// Another run token, or another Project, is another worktree.
	if w2, err := PrepareWorktree(context.Background(), home, b, "p", otherRun, "", ExecGit); err != nil || !w2.Created {
		t.Fatal(w2, err)
	}
	if w3, err := PrepareWorktree(context.Background(), home, b, "q", testRun, "", ExecGit); err != nil || !w3.Created || w3.Branch != "reet/q-"+testRun {
		t.Fatal(w3, err)
	}
}

func TestPrepareWorktreeRefusesSymlinkedReetFolders(t *testing.T) {
	for _, rel := range []string{".reet", ".reet/worktrees", ".reet/worktrees/p", ".reet/worktrees/p/site"} {
		t.Run(rel, func(t *testing.T) {
			home := worktreeHome(t)
			repo := newRepo(t, home, "site")
			outside := shortTempDir(t)
			link := filepath.Join(home, rel)
			os.MkdirAll(filepath.Dir(link), 0o700)
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
			refusedWith(t, err, terminalproto.WorktreeOutsideHome)
			if entries, _ := os.ReadDir(outside); len(entries) != 0 {
				t.Fatal("wrote through the symlink", entries)
			}
			if b := reetBranches(t, repo); len(b) != 0 {
				t.Fatal(b)
			}
		})
	}
	t.Run("file", func(t *testing.T) {
		home := worktreeHome(t)
		repo := newRepo(t, home, "site")
		writeFile(t, filepath.Join(home, ".reet"), "")
		_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
		refusedWith(t, err, terminalproto.WorktreeOutsideHome)
	})
	t.Run("existing open ~/.reet is used", func(t *testing.T) {
		home := worktreeHome(t)
		repo := newRepo(t, home, "site")
		os.Mkdir(filepath.Join(home, ".reet"), 0o755)
		if _, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPrepareWorktreeRefusesRepositoryOutsideHome(t *testing.T) {
	home := worktreeHome(t)
	outside, _ := filepath.EvalSymlinks(shortTempDir(t))
	repo := newRepo(t, outside, "site")
	os.Symlink(repo, filepath.Join(home, "escape"))
	for _, dir := range []string{repo, filepath.Join(home, "escape"), "relative/site", filepath.Join(home, "missing")} {
		if _, err := PrepareWorktree(context.Background(), home, dir, "p", testRun, "", ExecGit); !errors.Is(err, ErrLaunchUnavailable) {
			t.Fatal("folder outside home not refused as a launch", dir, err)
		}
	}
	if b := reetBranches(t, repo); len(b) != 0 {
		t.Fatal(b)
	}
}

func TestPrepareWorktreeSlugs(t *testing.T) {
	home := worktreeHome(t)
	for i, tc := range []struct{ project, repo, wantProject, wantRepo string }{
		{"Reet.SH", "My.Repo", "reet-sh", "my-repo"},
		{"", "plain", "project", "plain"},
		{"p", "a #(x) b;", "p", "a-x-b"},
		{"p", "trailing space ", "p", "trailing-space"},
		{"../../etc", "x", "etc", "x"},
		{"Café", "日本", "caf", ""},
		{strings.Repeat("Long", 30), strings.Repeat("r", 120), "", ""},
	} {
		repo := newRepo(t, home, filepath.Join("slugs", string(rune('a'+i)), tc.repo))
		w, err := PrepareWorktree(context.Background(), home, repo, tc.project, testRun, "", ExecGit)
		if err != nil {
			t.Fatal(tc, err)
		}
		rel, _ := filepath.Rel(filepath.Join(home, ".reet", "worktrees"), w.Path)
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) != 3 || parts[2] != testRun || !terminalproto.ValidSlug(parts[0]) || !terminalproto.ValidSlug(parts[1]) {
			t.Fatal("path", w.Path)
		}
		if tc.wantProject != "" && parts[0] != tc.wantProject || tc.wantRepo != "" && parts[1] != tc.wantRepo {
			t.Fatal("slugs", parts, tc)
		}
		if w.Branch != "reet/"+parts[0]+"-"+testRun {
			t.Fatal("branch", w.Branch)
		}
		if got := testGit(t, w.Path, "symbolic-ref", "HEAD"); got != "refs/heads/"+w.Branch {
			t.Fatal(got)
		}
	}
	if _, err := PrepareWorktree(context.Background(), home, newRepo(t, home, "bad"), "p", "NOTATOKEN", "", ExecGit); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal("bad run token", err)
	}
}

func TestPrepareWorktreeNeverRunsHooks(t *testing.T) {
	home := worktreeHome(t)
	hook := func(dir, marker string) {
		os.MkdirAll(dir, 0o700)
		script := "#!/bin/sh\necho ran > '" + marker + "'\n"
		for _, name := range []string{"post-checkout", "reference-transaction", "post-index-change"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Run(".git/hooks", func(t *testing.T) {
		repo := newRepo(t, home, "hooked")
		marker := filepath.Join(home, "hook-ran")
		hook(filepath.Join(repo, ".git", "hooks"), marker)
		// The hook works when git is allowed to run it.
		testGit(t, repo, "checkout", "-q", "-b", "probe")
		if _, err := os.Stat(marker); err != nil {
			t.Fatal("test hook does not run at all", err)
		}
		os.Remove(marker)
		testGit(t, repo, "-c", "core.hooksPath=/dev/null", "checkout", "-q", "main")
		if _, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatal("a repository hook ran during creation")
		}
	})
	t.Run("core.hooksPath", func(t *testing.T) {
		repo := newRepo(t, home, "hookspath")
		marker := filepath.Join(home, "hookspath-ran")
		hook(filepath.Join(repo, ".husky"), marker)
		testGit(t, repo, "config", "core.hooksPath", ".husky")
		testGit(t, repo, "checkout", "-q", "-b", "probe")
		if _, err := os.Stat(marker); err != nil {
			t.Fatal("test hook does not run at all", err)
		}
		os.Remove(marker)
		testGit(t, repo, "-c", "core.hooksPath=/dev/null", "checkout", "-q", "main")
		if _, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatal("core.hooksPath hook ran during creation")
		}
	})
}

func TestExecGitIgnoresGitEnvironment(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "real")
	decoy := newRepo(t, home, "decoy")
	writeFile(t, filepath.Join(decoy, "README"), "decoy\n")
	testGit(t, decoy, "commit", "-q", "-am", "decoy")
	badConfig := filepath.Join(home, "bad-config")
	writeFile(t, badConfig, "[[[ not a config\n")
	marker := filepath.Join(home, "env-hook-ran")
	hooks := filepath.Join(home, "env-hooks")
	os.MkdirAll(hooks, 0o700)
	os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte("#!/bin/sh\necho ran > '"+marker+"'\n"), 0o700)
	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	t.Setenv("GIT_CONFIG_GLOBAL", badConfig)
	t.Setenv("GIT_CONFIG_SYSTEM", badConfig)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(home, "decoy-index"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := testGit(t, w.Path, "rev-parse", "HEAD"), testGit(t, repo, "rev-parse", "HEAD"); got != want {
		t.Fatal("worktree made from the decoy", got, want)
	}
	if b := reetBranches(t, decoy); len(b) != 0 {
		t.Fatal("decoy changed", b)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("hook from the environment ran")
	}
}

func TestGitEnvironmentIsExactlyHomePathLang(t *testing.T) {
	got := gitEnvironment([]string{"GIT_DIR=/x", "HOME=/h", "PATH=/bin", "LANG=C.UTF-8", "LC_ALL=C", "GIT_CONFIG_GLOBAL=/y", "SSH_AUTH_SOCK=/s", "HOME=/other", "PATHX=1", "XDG_CONFIG_HOME=/x", "EDITOR=vim"})
	if want := []string{"HOME=/h", "PATH=/bin", "LANG=C.UTF-8"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := gitEnvironment(nil); len(got) != 0 {
		t.Fatal(got)
	}
}

// fakeGit writes an executable script standing in for git.
func fakeGit(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(shortTempDir(t), "git")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunGitArgumentVectorAndEnvironment(t *testing.T) {
	dir := shortTempDir(t)
	record := filepath.Join(dir, "record")
	git := fakeGit(t, "printf '%s\\0' \"$@\" > '"+record+"'\nenv | grep -c '^GIT_' >> '"+record+"' || true\n")
	t.Setenv("GIT_DIR", "/nowhere")
	arg := "a b;$(touch " + filepath.Join(dir, "pwned") + ")`x`"
	if _, err := runGit(context.Background(), git, time.Minute, 4096, dir, "show", arg, "--", "*"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(record)
	fields := strings.Split(string(data), "\x00")
	want := []string{"-C", dir, "show", arg, "--", "*", "0\n"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("got %q want %q", fields, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Fatal("an argument was run by a shell")
	}
	if _, err := runGit(context.Background(), git, time.Minute, 4096, "relative", "status"); err == nil {
		t.Fatal("relative folder accepted")
	}
}

func TestRunGitTimeout(t *testing.T) {
	git := fakeGit(t, "exec sleep 30\n")
	start := time.Now()
	_, err := runGit(context.Background(), git, 200*time.Millisecond, 4096, shortTempDir(t), "status")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 10*time.Second {
		t.Fatal("timeout not enforced", err, time.Since(start))
	}
	if gitTimeout != 60*time.Second || gitOutputLimit != 4096 {
		t.Fatal("limits changed", gitTimeout, gitOutputLimit)
	}
	refusal := gitFailed(nil, err)
	refusedWith(t, refusal, terminalproto.WorktreeGitFailed)
}

func TestExecGitOutputCap(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "big")
	writeFile(t, filepath.Join(repo, "big"), strings.Repeat("0123456789abcdef", 1024))
	testGit(t, repo, "add", "big")
	testGit(t, repo, "commit", "-q", "-m", "big")
	out, err := ExecGit(context.Background(), repo, "cat-file", "-p", "HEAD:big")
	if err != nil || len(out) != 4096 || !bytes.HasPrefix(out, []byte("0123456789abcdef")) {
		t.Fatal("output not capped", len(out), err)
	}
	git := fakeGit(t, "head -c 100000 /dev/zero | tr '\\0' x >&2\nexit 3\n")
	out, err = runGit(context.Background(), git, time.Minute, 4096, repo, "x")
	var exit *exec.ExitError
	if len(out) != 4096 || !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatal("stderr not capped or exit status lost", len(out), err)
	}
}

func TestExecGitRefusesWhenGitIsMissing(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "site")
	t.Setenv("PATH", shortTempDir(t))
	if _, err := ExecGit(context.Background(), repo, "status"); !errors.Is(err, errGitMissing) {
		t.Fatal(err)
	}
	_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
	refusedWith(t, err, terminalproto.WorktreeGitFailed)
	var w *WorktreeError
	errors.As(err, &w)
	if w.Detail != "git is not installed" {
		t.Fatal(w.Detail)
	}
}

type gitCall struct {
	dir  string
	args []string
}

func recordingGit(next GitRunner) (func() []gitCall, GitRunner) {
	var mu sync.Mutex
	calls := []gitCall{}
	return func() []gitCall {
			mu.Lock()
			defer mu.Unlock()
			return append([]gitCall{}, calls...)
		}, func(ctx context.Context, dir string, args ...string) ([]byte, error) {
			mu.Lock()
			calls = append(calls, gitCall{dir, append([]string{}, args...)})
			mu.Unlock()
			return next(ctx, dir, args...)
		}
}

// readOnly lists the git reads PrepareWorktree may make.
func readOnly(args []string) bool {
	switch {
	case reflect.DeepEqual(args, []string{"rev-parse", "--show-toplevel"}),
		reflect.DeepEqual(args, []string{"rev-parse", "--verify", "--quiet", "HEAD^{commit}"}),
		reflect.DeepEqual(args, []string{"worktree", "list", "--porcelain"}),
		len(args) == 3 && args[0] == "for-each-ref" && args[1] == "--format=%(refname)" && strings.HasPrefix(args[2], "refs/heads/reet/"):
		return true
	}
	return false
}

func mutating(args []string) bool { return !readOnly(args) }

func TestPrepareWorktreeArgumentVectorIsExact(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "exact")
	calls, git := recordingGit(ExecGit)
	w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", git)
	if err != nil {
		t.Fatal(err)
	}
	changes := []gitCall{}
	for _, c := range calls() {
		if c.dir != repo {
			t.Fatal("git ran outside the repository", c)
		}
		if mutating(c.args) {
			changes = append(changes, c)
		}
	}
	want := []string{"-c", "core.hooksPath=/dev/null", "worktree", "add", "-b", "reet/p-" + testRun, wantPath(home, "p", "exact", testRun), "HEAD"}
	if len(changes) != 1 || !reflect.DeepEqual(changes[0].args, want) {
		t.Fatalf("mutating git commands:\n got %q\nwant %q", changes, want)
	}
	if w.Path != want[6] || w.Branch != want[5] {
		t.Fatal(w)
	}
	// Undo is exactly the forced removal of that path and the deletion of that
	// branch, and leaves nothing behind.
	undoCalls, undoGit := recordingGit(ExecGit)
	if err := w.Undo(context.Background(), undoGit); err != nil {
		t.Fatal(err)
	}
	wantUndo := []gitCall{{repo, []string{"worktree", "remove", "--force", w.Path}}, {repo, []string{"branch", "-D", w.Branch}}}
	if got := undoCalls(); !reflect.DeepEqual(got, wantUndo) {
		t.Fatalf("undo:\n got %q\nwant %q", got, wantUndo)
	}
	nothingCreated(t, home, repo)
}

func TestWorktreeUndoRemovesWorktreeAndBranch(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "undo")
	testGit(t, repo, "branch", "keep-me")
	w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(w.Path, "scratch"), "x")
	if err := w.Undo(context.Background(), ExecGit); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(w.Path); err == nil {
		t.Fatal("worktree folder left")
	}
	nothingCreated(t, home, repo)
	if testGit(t, repo, "rev-parse", "--verify", "keep-me") == "" {
		t.Fatal("other branch deleted")
	}
	if err := (Worktree{}).Undo(context.Background(), func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("zero worktree ran git")
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareWorktreeCleansUpAFailedAdd(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "flaky")
	// git makes the worktree but the call reports failure (a timeout after
	// the work was done looks exactly like this).
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		out, err := ExecGit(ctx, dir, args...)
		if len(args) > 3 && args[2] == "worktree" && args[3] == "add" {
			return []byte("fatal: \x1b[31msomething\x07 broke\nmore\n"), errors.New("exit status 128")
		}
		return out, err
	}
	_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "", git)
	refusedWith(t, err, terminalproto.WorktreeGitFailed)
	var w *WorktreeError
	errors.As(err, &w)
	if w.Detail != "fatal: [31msomething broke" {
		t.Fatalf("detail %q", w.Detail)
	}
	nothingCreated(t, home, repo)
	if _, err := os.Lstat(wantPath(home, "p", "flaky", testRun)); err == nil {
		t.Fatal("destination left behind")
	}
	// A real failure: a branch named reet/p blocks reet/p-<run>? No: a
	// folder-shaped ref conflict does. A branch "reet" makes "reet/..." fail.
	repo2 := newRepo(t, home, "conflict")
	testGit(t, repo2, "branch", "reet")
	_, err = PrepareWorktree(context.Background(), home, repo2, "p", testRun, "", ExecGit)
	refusedWith(t, err, terminalproto.WorktreeGitFailed)
	errors.As(err, &w)
	if w.Detail == "" {
		t.Fatal("no detail from git")
	}
	if testGit(t, repo2, "rev-parse", "--verify", "reet") == "" || worktreeCount(t, repo2) != 1 {
		t.Fatal("pre-existing branch or worktrees touched")
	}
	if _, err := os.Lstat(wantPath(home, "p", "conflict", testRun)); err == nil {
		t.Fatal("destination left behind")
	}
}

func TestWorktreeDetail(t *testing.T) {
	for in, want := range map[string]string{
		"": "",
		"Preparing worktree (new branch 'x')\nfatal: bad\n": "fatal: bad",
		"\n\n  first line  \nsecond":                        "first line",
		"error: a\u202eb\u2028c\x00d\xff":                   "error: abcd",
		"fatal: " + strings.Repeat("é", 300):                "fatal: " + strings.Repeat("é", (200-7)/2),
	} {
		got := worktreeDetail([]byte(in))
		if got != want || len(got) > terminalproto.MaxWorktreeDetailBytes {
			t.Errorf("worktreeDetail(%q) = %q, want %q", in, got, want)
		}
	}
}

// worktreeManager is a manager with a fake agent and a repository in its home.
func worktreeManager(t *testing.T) (m *Manager, opts ManagerOptions, agent, record, repo, realHome string) {
	t.Helper()
	requireGit(t)
	m, opts = managerFixture(t)
	t.Setenv("HOME", opts.Home)
	agent, record = fakeAgent(t)
	m.lookPath = func(name string) (string, error) {
		if name == "claude" {
			return agent, nil
		}
		return "", errors.New("not found")
	}
	realHome, _ = filepath.EvalSymlinks(opts.Home)
	repo = newRepo(t, opts.Home, "code/site")
	return
}

func worktreeCreate(repo, run string) terminalproto.Create {
	return terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Dir: repo, Worktree: run, Project: "p"}
}

func readRecord(t *testing.T, record string) []string {
	t.Helper()
	var data []byte
	var err error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err = os.ReadFile(record); err == nil {
			return strings.Split(strings.TrimSuffix(string(data), "\n"), "\x00")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fake agent did not start", err)
	return nil
}

func TestManagerWorktreeIsMadeBeforeIntentAndAgentStartsInIt(t *testing.T) {
	m, opts, agent, record, repo, realHome := worktreeManager(t)
	calls := recordingTmux(t, m)
	var violations []string
	gitCalls, recorded := recordingGit(ExecGit)
	m.git = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if mutating(args) {
			r, err := m.loadRegistry()
			if err != nil || len(r.Sessions) != 0 || len(r.Operations) != 0 {
				violations = append(violations, "registry intent saved before the worktree was made")
			}
			if len(newSessionCalls(calls())) != 0 {
				violations = append(violations, "tmux started before the worktree was made")
			}
		}
		return recorded(ctx, dir, args...)
	}
	in := worktreeCreate(repo, testRun)
	s, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), in)
	if err != nil || s.State != terminalproto.Running {
		t.Fatal(s, err)
	}
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	dest := wantPath(realHome, "p", "site", testRun)
	want := []string{"-S", opts.Layout.Socket, "new-session", "-d", "-s", m.name(testSessionID(1)), "-x", "80", "-y", "24", "-c", dest, "-e", "HOME=" + opts.Home, agent}
	if created := newSessionCalls(calls()); len(created) != 1 || !reflect.DeepEqual(created[0], want) {
		t.Fatalf("new-session:\n got %q\nwant %q", created, want)
	}
	if got := readRecord(t, record); !reflect.DeepEqual(got, []string{"", dest}) {
		t.Fatalf("agent argv and cwd %q, want cwd %q", got, dest)
	}
	if got := testGit(t, dest, "symbolic-ref", "HEAD"); got != "refs/heads/reet/p-"+testRun {
		t.Fatal(got)
	}
	// A retry of the same request is a lookup: no git, no second agent.
	before := len(gitCalls())
	again, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), in)
	if err != nil || again.ID != s.ID || len(gitCalls()) != before || len(newSessionCalls(calls())) != 1 {
		t.Fatal("retry repeated work", again, err)
	}
	// The session records where the worktree is, and a replay returns it.
	if s.WorktreePath != dest || s.WorktreeRepo != "site" || again.WorktreePath != dest || again.WorktreeRepo != "site" {
		t.Fatal("worktree not recorded on the session", s, again)
	}
	if r, err := m.loadRegistry(); err != nil || r.Sessions[s.ID].WorktreePath != dest {
		t.Fatal("worktree not persisted in the registry", err)
	}
	// A plain create records no worktree.
	plain, err := m.Create(context.Background(), testRequestID(2), testSessionID(2), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil || plain.WorktreePath != "" || plain.WorktreeRepo != "" {
		t.Fatal("plain create", plain, err)
	}
	changed := in
	changed.Project = "q"
	if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), changed); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("a changed Project must conflict", err)
	}
}

// failingNewSession makes tmux refuse new-session without starting anything.
func failingNewSession(t *testing.T, m *Manager) {
	t.Helper()
	wrapper := filepath.Join(shortTempDir(t), "tmux")
	script := "#!/bin/sh\nfor a in \"$@\"; do [ \"$a\" = new-session ] && exit 1; done\nexec '" + m.tmux.Path + "' \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	m.tmux.Path = wrapper
}

func TestManagerWorktreeFailedStartUndoesOnlyWhatItMade(t *testing.T) {
	m, _, _, _, repo, realHome := worktreeManager(t)
	realRepo, _ := filepath.EvalSymlinks(repo)
	// A worktree from an earlier attempt, adopted by this one.
	adopted, err := PrepareWorktree(context.Background(), realHome, realRepo, "p", otherRun, "", ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	failingNewSession(t, m)
	if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), worktreeCreate(repo, testRun)); err == nil {
		t.Fatal("start did not fail")
	}
	if _, err := os.Lstat(wantPath(realHome, "p", "site", testRun)); err == nil {
		t.Fatal("worktree left after a failed start")
	}
	if b := reetBranches(t, repo); !reflect.DeepEqual(b, []string{"refs/heads/" + adopted.Branch}) {
		t.Fatal("branches after a failed start", b)
	}
	if _, err := m.Create(context.Background(), testRequestID(2), testSessionID(2), worktreeCreate(repo, otherRun)); err == nil {
		t.Fatal("start did not fail")
	}
	if info, err := os.Lstat(adopted.Path); err != nil || !info.IsDir() || worktreeCount(t, repo) != 2 {
		t.Fatal("an adopted worktree was removed", err)
	}
	if b := reetBranches(t, repo); len(b) != 1 {
		t.Fatal("adopted branch removed", b)
	}
}

func TestManagerWorktreeUndoneWhenRefusedAfterIntent(t *testing.T) {
	m, _, _, _, repo, realHome := worktreeManager(t)
	calls := recordingTmux(t, m)
	n := 0
	m.consent = func() error {
		n++
		if n > 1 {
			return errors.New("disabled meanwhile")
		}
		return nil
	}
	if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), worktreeCreate(repo, testRun)); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatal(err)
	}
	if len(newSessionCalls(calls())) != 0 {
		t.Fatal("tmux started")
	}
	if _, err := os.Lstat(wantPath(realHome, "p", "site", testRun)); err == nil {
		t.Fatal("worktree left")
	}
	if b := reetBranches(t, repo); len(b) != 0 {
		t.Fatal(b)
	}
}

func TestManagerRefusedWorktreeLeavesNothingBehind(t *testing.T) {
	m, _, _, _, repo, _ := worktreeManager(t)
	calls := recordingTmux(t, m)
	writeFile(t, filepath.Join(repo, ".git", "MERGE_HEAD"), testGit(t, repo, "rev-parse", "HEAD")+"\n")
	_, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), worktreeCreate(repo, testRun))
	refusedWith(t, err, terminalproto.WorktreeRepoBusy)
	if len(newSessionCalls(calls())) != 0 {
		t.Fatal("tmux started")
	}
	r, err := m.loadRegistry()
	if err != nil || len(r.Sessions) != 0 || len(r.Operations) != 0 {
		t.Fatal("a refused worktree left registry state", r, err)
	}
	if b := reetBranches(t, repo); len(b) != 0 {
		t.Fatal(b)
	}
}

func TestManagerPlainAndLaunchCreatesNeverRunGit(t *testing.T) {
	m, opts, agent, _, _, _ := worktreeManager(t)
	m.git = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("git ran for a create without a worktree")
		return nil, nil
	}
	calls := recordingTmux(t, m)
	if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), testRequestID(2), testSessionID(2), terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude"}); err != nil {
		t.Fatal(err)
	}
	created := newSessionCalls(calls())
	want := [][]string{
		{"-S", opts.Layout.Socket, "new-session", "-d", "-s", m.name(testSessionID(1)), "-x", "80", "-y", "24", "-c", opts.Home, "-e", "HOME=" + opts.Home},
		{"-S", opts.Layout.Socket, "new-session", "-d", "-s", m.name(testSessionID(2)), "-x", "80", "-y", "24", "-c", opts.Home, "-e", "HOME=" + opts.Home, agent},
	}
	if !reflect.DeepEqual(created, want) {
		t.Fatalf("got %q\nwant %q", created, want)
	}
	if _, err := os.Lstat(filepath.Join(opts.Home, ".reet")); err == nil {
		t.Fatal("~/.reet made without a worktree")
	}
}

func TestWorktreeFromLocalBase(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "local")
	base := testGit(t, repo, "rev-parse", "HEAD")
	testGit(t, repo, "branch", "chosen")
	writeFile(t, filepath.Join(repo, "README"), "new head\n")
	testGit(t, repo, "commit", "-qam", "second")
	// A local branch takes precedence over a same-named remote reference.
	testGit(t, repo, "update-ref", "refs/remotes/chosen", "HEAD")
	w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "chosen", ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	if got := testGit(t, w.Path, "rev-parse", "HEAD"); got != base {
		t.Fatalf("got %s, want base %s", got, base)
	}
}

func TestWorktreeFromRemoteBase(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "remote")
	base := testGit(t, repo, "rev-parse", "HEAD")
	testGit(t, repo, "update-ref", "refs/remotes/origin/chosen", base)
	writeFile(t, filepath.Join(repo, "README"), "new head\n")
	testGit(t, repo, "commit", "-qam", "second")
	w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "origin/chosen", ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	if got := testGit(t, w.Path, "rev-parse", "HEAD"); got != base {
		t.Fatalf("got %s, want base %s", got, base)
	}
}

func TestWorktreeBaseMissing(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "missing")
	_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "absent", ExecGit)
	refusedWith(t, err, terminalproto.WorktreeReason("base_missing"))
	nothingCreated(t, home, repo)
}

func TestWorktreeBaseArgvExact(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "exact-base")
	commit := testGit(t, repo, "rev-parse", "HEAD")
	testGit(t, repo, "update-ref", "refs/remotes/origin/chosen", commit)
	var resolutions [][]string
	var add []string
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) > 2 && args[0] == "rev-parse" && args[1] == "--verify" && args[2] == "--end-of-options" {
			resolutions = append(resolutions, append([]string(nil), args...))
		}
		if len(args) > 3 && args[0] == "-c" && args[2] == "worktree" && args[3] == "add" {
			add = append([]string(nil), args...)
		}
		return ExecGit(ctx, dir, args...)
	}
	_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "origin/chosen", git)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"rev-parse", "--verify", "--end-of-options", "refs/heads/origin/chosen^{commit}"}, {"rev-parse", "--verify", "--end-of-options", "refs/remotes/origin/chosen^{commit}"}}
	if !reflect.DeepEqual(resolutions, want) {
		t.Fatalf("resolutions: %q, want %q", resolutions, want)
	}
	if len(add) == 0 || add[len(add)-1] != commit {
		t.Fatalf("add must use resolved commit: %q", add)
	}
	calls := 0
	invalidGit := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		calls++
		return nil, errors.New("must not run")
	}
	if _, err := PrepareWorktree(context.Background(), home, repo, "p", otherRun, "-x", invalidGit); !errors.Is(err, terminalproto.ErrInvalid) || calls != 0 {
		t.Fatalf("invalid base: %v, calls=%d", err, calls)
	}
}

func TestWorktreeBaseRemovedRetryAdoptsExisting(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "retry-base")
	testGit(t, repo, "branch", "chosen")
	first, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "chosen", ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "branch", "-D", "chosen")
	again, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, "chosen", ExecGit)
	if err != nil || again.Created || again.Path != first.Path {
		t.Fatalf("retry must adopt existing worktree after source branch disappears: %+v, %v", again, err)
	}
}
