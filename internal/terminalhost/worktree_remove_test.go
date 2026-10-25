//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// madeWorktree makes the run's worktree for repo with the real git, as a
// Project run would.
func madeWorktree(t *testing.T, home, repo, run string) Worktree {
	t.Helper()
	w, err := PrepareWorktree(context.Background(), home, repo, "p", run, ExecGit)
	if err != nil || !w.Created {
		t.Fatal(w, err)
	}
	return w
}

func commitIn(t *testing.T, dir, name string) string {
	t.Helper()
	writeFile(t, filepath.Join(dir, name), name+"\n")
	testGit(t, dir, "add", name)
	testGit(t, dir, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", name)
	return testGit(t, dir, "rev-parse", "HEAD")
}

func status(t *testing.T, home, repoName, run string) terminalproto.WorktreeState {
	t.Helper()
	s, err := WorktreeStatus(context.Background(), home, "p", repoName, run, ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: testRequestID(1), Generation: 1, Body: s}); err != nil {
		t.Fatalf("status is not a valid message: %+v", s)
	}
	return s
}

func registered(t *testing.T, repo, path string) bool {
	t.Helper()
	return slices.Contains(strings.Split(testGit(t, repo, "worktree", "list", "--porcelain"), "\n"), "worktree "+path)
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func TestWorktreeStatusFreshWorktreeIsZero(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	for i := 0; i < 40; i++ {
		commitIn(t, repo, fmt.Sprintf("f%02d", i))
	}
	w := madeWorktree(t, home, repo, testRun)
	s := status(t, home, "site", testRun)
	if s.Path != w.Path || s.Branch != w.Branch || s.Changed != 0 || s.Unpushed != 0 || len(s.Paths) != 0 {
		t.Fatalf("a fresh worktree must report nothing: %+v", s)
	}
}

func TestWorktreeStatusCountsEveryKindOfChange(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	for _, name := range []string{"a", "b", "c", "d"} {
		commitIn(t, repo, name)
	}
	w := madeWorktree(t, home, repo, testRun)
	writeFile(t, filepath.Join(w.Path, "a"), "modified\n")        // modified, unstaged
	writeFile(t, filepath.Join(w.Path, "staged"), "new\n")        // staged new file
	testGit(t, w.Path, "add", "staged")                           //
	if err := os.Remove(filepath.Join(w.Path, "b")); err != nil { // deleted
		t.Fatal(err)
	}
	testGit(t, w.Path, "mv", "c", "c-renamed") // renamed (staged)
	if err := os.MkdirAll(filepath.Join(w.Path, "new/deeper"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(w.Path, "new/deeper/untracked"), "u\n") // untracked, in a new folder
	writeFile(t, filepath.Join(w.Path, "loose"), "u\n")                // untracked
	writeFile(t, filepath.Join(w.Path, ".gitignore"), "ignored\n")     // untracked
	writeFile(t, filepath.Join(w.Path, "ignored"), "never counted\n")  // ignored
	s := status(t, home, "site", testRun)
	want := []string{".gitignore", "a", "b", "c-renamed", "loose", "new/deeper/untracked", "staged"}
	got := slices.Clone(s.Paths)
	slices.Sort(got)
	if s.Changed != len(want) || !slices.Equal(got, want) || s.Unpushed != 0 {
		t.Fatalf("status: %+v, want %d changes %v", s, len(want), want)
	}
}

// A repository may hide untracked files from status; a remove would still
// delete them, so they must be counted regardless.
func TestWorktreeStatusCountsUntrackedFilesTheRepositoryHides(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	testGit(t, repo, "config", "status.showUntrackedFiles", "no")
	w := madeWorktree(t, home, repo, testRun)
	writeFile(t, filepath.Join(w.Path, "notes"), "precious\n")
	s := status(t, home, "site", testRun)
	if s.Changed != 1 || !slices.Equal(s.Paths, []string{"notes"}) {
		t.Fatalf("hidden untracked file not counted: %+v", s)
	}
	err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{Changed: 1}, ExecGit)
	refusedWith(t, err, terminalproto.WorktreeHasChanges)
	if !exists(filepath.Join(w.Path, "notes")) || !registered(t, repo, w.Path) {
		t.Fatal("the untracked file or the worktree was removed")
	}
}

func TestWorktreeStatusPathsAreBoundedAndPlain(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	long := strings.Repeat("l", 200)
	if err := os.MkdirAll(filepath.Join(w.Path, "0dir", long), 0o700); err != nil {
		t.Fatal(err)
	}
	special := []string{"0a\nnewline", "0b‮evil.txt", "0c\x1b[2Jesc", filepath.Join("0dir", long, strings.Repeat("m", 200))}
	for _, name := range special {
		writeFile(t, filepath.Join(w.Path, name), "x\n")
	}
	for i := len(special); i < 25; i++ {
		writeFile(t, filepath.Join(w.Path, fmt.Sprintf("file%02d", i)), "x\n")
	}
	s := status(t, home, "site", testRun)
	if s.Changed != 25 || len(s.Paths) != terminalproto.MaxWorktreePaths {
		t.Fatalf("want 25 changes and 20 paths, got %d and %d", s.Changed, len(s.Paths))
	}
	for _, p := range s.Paths {
		if p == "" || len(p) > terminalproto.MaxWorktreePathBytes || !utf8.ValidString(p) || hasControl(p) {
			t.Fatalf("path not plain or too long: %q", p)
		}
	}
	joined := strings.Join(s.Paths, "|")
	if !strings.Contains(joined, "0a�newline") || !strings.Contains(joined, "0b�evil.txt") || !strings.Contains(joined, "0c�[2Jesc") {
		t.Fatalf("control and bidi characters not replaced: %q", s.Paths)
	}
	if !slices.ContainsFunc(s.Paths, func(p string) bool {
		return strings.HasPrefix(p, "0dir/") && len(p) == terminalproto.MaxWorktreePathBytes
	}) {
		t.Fatalf("long path not cut to %d bytes: %q", terminalproto.MaxWorktreePathBytes, s.Paths)
	}
}

func TestPlainPath(t *testing.T) {
	for in, want := range map[string]string{
		"ok.txt":                       "ok.txt",
		"a\tb":                         "a�b",
		"a b c":                        "a�b�c",
		"bad\xffutf8":                  "bad�utf8",
		"zero​width":                   "zero�width",
		strings.Repeat("é", 200):       strings.Repeat("é", 127),
		"x" + strings.Repeat("é", 200): "x" + strings.Repeat("é", 127),
	} {
		if got := plainPath(in); got != want {
			t.Errorf("plainPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseStatusCountsRenamesOnce(t *testing.T) {
	out := []byte("R  new\x00old\x00 M a\x00?? b\x00C  copy\x00orig\x00D  gone\x00")
	n, paths := parseStatus(out)
	if n != 5 || !slices.Equal(paths, []string{"new", "a", "b", "copy", "gone"}) {
		t.Fatal(n, paths)
	}
	if n, paths := parseStatus(nil); n != 0 || paths != nil {
		t.Fatal(n, paths)
	}
}

func TestWorktreeUnpushedWithoutARemote(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	for i := 0; i < 5; i++ {
		commitIn(t, repo, fmt.Sprintf("f%d", i))
	}
	w := madeWorktree(t, home, repo, testRun)
	commitIn(t, w.Path, "agent1")
	commitIn(t, w.Path, "agent2")
	if s := status(t, home, "site", testRun); s.Unpushed != 2 || s.Changed != 0 {
		t.Fatalf("two commits made in the worktree: %+v", s)
	}
	commitIn(t, repo, "main-moves-on")
	if s := status(t, home, "site", testRun); s.Unpushed != 2 {
		t.Fatalf("the source branch moving on changes nothing: %+v", s)
	}
	// Detached HEAD: the run's commits and the one made there all count.
	testGit(t, w.Path, "checkout", "-q", "--detach")
	commitIn(t, w.Path, "detached")
	if s := status(t, home, "site", testRun); s.Unpushed != 3 {
		t.Fatalf("a detached commit is unpushed too: %+v", s)
	}
	// On another branch the agent made: the same.
	testGit(t, w.Path, "checkout", "-q", "-b", "agent-branch")
	commitIn(t, w.Path, "on-agent-branch")
	if s := status(t, home, "site", testRun); s.Unpushed != 4 {
		t.Fatalf("commits on the checked-out branch count: %+v", s)
	}
}

func TestWorktreeUnpushedWithARemote(t *testing.T) {
	home := worktreeHome(t)
	upstream := newRepo(t, home, "upstream/site")
	commitIn(t, upstream, "u1")
	testGit(t, home, "clone", "-q", upstream, filepath.Join(home, "code/site"))
	clone := filepath.Join(home, "code/site")
	// The clone's main is one commit ahead of origin: not the run's commit.
	commitIn(t, clone, "local-only")
	w := madeWorktree(t, home, clone, testRun)
	if s := status(t, home, "site", testRun); s.Unpushed != 0 {
		t.Fatalf("a fresh worktree from a clone reports nothing: %+v", s)
	}
	commitIn(t, w.Path, "a1")
	commitIn(t, w.Path, "a2")
	commitIn(t, w.Path, "a3")
	if s := status(t, home, "site", testRun); s.Unpushed != 3 {
		t.Fatalf("three commits made in the worktree: %+v", s)
	}
	testGit(t, w.Path, "push", "-q", "origin", "HEAD:refs/heads/"+w.Branch)
	if s := status(t, home, "site", testRun); s.Unpushed != 0 {
		t.Fatalf("pushed commits are not unpushed: %+v", s)
	}
	commitIn(t, w.Path, "a4")
	if s := status(t, home, "site", testRun); s.Unpushed != 1 {
		t.Fatalf("one commit after the push: %+v", s)
	}
}

func TestWorktreeRemoveWhenCountsMatchKeepsTheBranch(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	commitIn(t, w.Path, "agent1")
	tip := commitIn(t, w.Path, "agent2")
	s := status(t, home, "site", testRun)
	if s.Changed != 0 || s.Unpushed != 2 {
		t.Fatal(s)
	}
	if err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{Changed: s.Changed, Unpushed: s.Unpushed}, ExecGit); err != nil {
		t.Fatal(err)
	}
	if exists(w.Path) || registered(t, repo, w.Path) {
		t.Fatal("the worktree is still there")
	}
	if got := testGit(t, repo, "rev-parse", "refs/heads/"+w.Branch); got != tip {
		t.Fatalf("the branch must be kept at %s, got %s", tip, got)
	}
	if got := testGit(t, repo, "rev-list", "--count", w.Branch, "--not", "main"); got != "2" {
		t.Fatal("the run's commits must stay on the branch", got)
	}
	// Already gone: not found, for both, and still nothing else touched.
	err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, ExecGit)
	if !errors.Is(err, ErrWorktreeNotFound) {
		t.Fatal("a second remove must be not found", err)
	}
	if _, err := WorktreeStatus(context.Background(), home, "p", "site", testRun, ExecGit); !errors.Is(err, ErrWorktreeNotFound) {
		t.Fatal(err)
	}
	if got := testGit(t, repo, "rev-parse", "refs/heads/"+w.Branch); got != tip {
		t.Fatal("branch moved")
	}
}

func TestWorktreeRemoveFreshWorktree(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	if err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, ExecGit); err != nil {
		t.Fatal(err)
	}
	if exists(w.Path) || !slices.Equal(reetBranches(t, repo), []string{"refs/heads/" + w.Branch}) {
		t.Fatal("fresh remove")
	}
}

func TestWorktreeRemoveRefusesWhenCountsChanged(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, path string)
	}{
		{"file modified after status", func(t *testing.T, path string) { writeFile(t, filepath.Join(path, "README"), "changed\n") }},
		{"untracked file after status", func(t *testing.T, path string) { writeFile(t, filepath.Join(path, "new"), "new\n") }},
		{"commit after status", func(t *testing.T, path string) { commitIn(t, path, "late") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := worktreeHome(t)
			repo := newRepo(t, home, "code/site")
			w := madeWorktree(t, home, repo, testRun)
			s := status(t, home, "site", testRun)
			tc.change(t, w.Path)
			before := testGit(t, w.Path, "status", "--porcelain=v1", "--untracked-files=all")
			err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{Changed: s.Changed, Unpushed: s.Unpushed}, ExecGit)
			if !errors.Is(err, ErrWorktreeChanged) {
				t.Fatal("want worktree changed", err)
			}
			if !exists(w.Path) || !registered(t, repo, w.Path) || testGit(t, w.Path, "status", "--porcelain=v1", "--untracked-files=all") != before {
				t.Fatal("the worktree was touched")
			}
		})
	}
	// Counts that are stale the other way (shown more than there is) are refused too.
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	if err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{Unpushed: 1}, ExecGit); !errors.Is(err, ErrWorktreeChanged) || !exists(w.Path) {
		t.Fatal(err)
	}
}

func TestWorktreeRemoveRefusesUncommittedChangesEvenWhenShown(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	writeFile(t, filepath.Join(w.Path, "README"), "agent's edit\n")
	writeFile(t, filepath.Join(w.Path, "draft"), "draft\n")
	s := status(t, home, "site", testRun)
	err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{Changed: s.Changed, Unpushed: s.Unpushed}, ExecGit)
	refusedWith(t, err, terminalproto.WorktreeHasChanges)
	if !exists(filepath.Join(w.Path, "draft")) || !registered(t, repo, w.Path) {
		t.Fatal("changes were thrown away")
	}
}

func TestWorktreeRemoveRefusesALockedWorktree(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	testGit(t, repo, "worktree", "lock", w.Path)
	err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, ExecGit)
	refusedWith(t, err, terminalproto.WorktreeGitFailed)
	if !exists(w.Path) || !registered(t, repo, w.Path) {
		t.Fatal("a locked worktree was removed")
	}
}

// Something at the path that git does not list as this repository's worktree
// is refused (not_registered), never reported as gone and never removed.
func TestWorktreeRefusesWhatOnlyLooksLikeAWorktree(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func(t *testing.T, home, dest string)
	}{
		{"plain folder", func(t *testing.T, home, dest string) {
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dest, "keep"), "x\n")
		}},
		{"file at the path", func(t *testing.T, home, dest string) {
			if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, dest, "x\n")
		}},
		{"a whole repository", func(t *testing.T, home, dest string) {
			if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				t.Fatal(err)
			}
			newRepo(t, filepath.Dir(dest), filepath.Base(dest))
		}},
		{"a plain folder in a home that is itself a repository", func(t *testing.T, home, dest string) {
			newRepo(t, home, ".")
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dest, "keep"), "x\n")
		}},
		{"a copied .git file of another worktree", func(t *testing.T, home, dest string) {
			repo := newRepo(t, home, "code/site")
			other := madeWorktree(t, home, repo, otherRun)
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(other.Path, ".git"))
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dest, ".git"), string(data))
			writeFile(t, filepath.Join(dest, "keep"), "x\n")
		}},
		{"a worktree of a repository with another name", func(t *testing.T, home, dest string) {
			other := newRepo(t, home, "code/other")
			if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				t.Fatal(err)
			}
			testGit(t, other, "worktree", "add", "-q", "-b", "x", dest, "HEAD")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := worktreeHome(t)
			dest := filepath.Join(home, ".reet", "worktrees", "p", "site", testRun)
			tc.make(t, home, dest)
			_, err := WorktreeStatus(context.Background(), home, "p", "site", testRun, ExecGit)
			refusedWith(t, err, terminalproto.WorktreeNotRegistered)
			var calls [][]string
			var mu sync.Mutex
			rec := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
				mu.Lock()
				calls = append(calls, args)
				mu.Unlock()
				return ExecGit(ctx, dir, args...)
			}
			err = WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, rec)
			refusedWith(t, err, terminalproto.WorktreeNotRegistered)
			for _, c := range calls {
				if slices.Contains(c, "remove") {
					t.Fatal("remove ran", calls)
				}
			}
			if !exists(dest) {
				t.Fatal("the folder was removed")
			}
		})
	}
}

// The repo name is slugged exactly as creation slugs the folder name, so any
// spelling that slugs the same finds the worktree.
func TestWorktreeRepositoryNameIsSluggedLikeCreation(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/Site")
	w := madeWorktree(t, home, repo, testRun)
	for _, name := range []string{"Site", "site", "SITE"} {
		if s, err := WorktreeStatus(context.Background(), home, "p", name, testRun, ExecGit); err != nil || s.Path != w.Path {
			t.Fatal(name, s, err)
		}
	}
}

func TestWorktreeRefusesSymlinks(t *testing.T) {
	for _, tc := range []struct {
		name string
		link func(t *testing.T, home string) string // returns the symlink path
	}{
		{"run folder links to another run's worktree", func(t *testing.T, home string) string {
			repo := newRepo(t, home, "code/site")
			other := madeWorktree(t, home, repo, otherRun)
			link := filepath.Join(filepath.Dir(other.Path), testRun)
			if err := os.Symlink(other.Path, link); err != nil {
				t.Fatal(err)
			}
			return other.Path
		}},
		{"repo folder links outside ~/.reet", func(t *testing.T, home string) string {
			repo := newRepo(t, home, "code/site")
			outside := filepath.Join(home, "outside", "site")
			if err := os.MkdirAll(filepath.Dir(outside), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(outside, 0o700); err != nil {
				t.Fatal(err)
			}
			real := filepath.Join(outside, testRun)
			testGit(t, repo, "worktree", "add", "-q", "-b", "outside", real, "HEAD")
			if err := os.MkdirAll(filepath.Join(home, ".reet", "worktrees", "p"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(home, ".reet", "worktrees", "p", "site")); err != nil {
				t.Fatal(err)
			}
			return real
		}},
		{"~/.reet links elsewhere", func(t *testing.T, home string) string {
			repo := newRepo(t, home, "code/site")
			elsewhere := filepath.Join(home, "elsewhere")
			real := filepath.Join(elsewhere, "worktrees", "p", "site", testRun)
			if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
				t.Fatal(err)
			}
			testGit(t, repo, "worktree", "add", "-q", "-b", "elsewhere", real, "HEAD")
			if err := os.Symlink(elsewhere, filepath.Join(home, ".reet")); err != nil {
				t.Fatal(err)
			}
			return real
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := worktreeHome(t)
			target := tc.link(t, home)
			_, err := WorktreeStatus(context.Background(), home, "p", "site", testRun, ExecGit)
			refusedWith(t, err, terminalproto.WorktreeOutsideHome)
			err = WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, ExecGit)
			refusedWith(t, err, terminalproto.WorktreeOutsideHome)
			if !exists(filepath.Join(target, "README")) {
				t.Fatal("the symlink's target was removed")
			}
		})
	}
}

func TestWorktreeRefusesWritableReetFolders(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	dir := filepath.Join(home, ".reet", "worktrees", "p")
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, ExecGit)
	refusedWith(t, err, terminalproto.WorktreeOutsideHome)
	if !exists(w.Path) {
		t.Fatal("removed")
	}
}

func TestWorktreeAlreadyGoneIsNotFound(t *testing.T) {
	home := worktreeHome(t)
	// Nothing ever made: not even ~/.reet.
	if _, err := WorktreeStatus(context.Background(), home, "p", "site", testRun, ExecGit); !errors.Is(err, ErrWorktreeNotFound) {
		t.Fatal(err)
	}
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	// The folder deleted by hand: git still lists it (prunable), but nothing is there.
	if err := os.RemoveAll(w.Path); err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	rec := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return ExecGit(ctx, dir, args...)
	}
	if err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, rec); !errors.Is(err, ErrWorktreeNotFound) {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatal("git ran for a worktree that is not there", calls)
	}
	if got := testGit(t, repo, "rev-parse", "--verify", "-q", "refs/heads/"+w.Branch); got == "" {
		t.Fatal("branch lost")
	}
}

func TestWorktreeRefusesBadArguments(t *testing.T) {
	home := worktreeHome(t)
	for _, tc := range [][3]string{
		{"p", "site", "short"},
		{"P", "site", testRun},
		{"../p", "site", testRun},
		{"p", "", testRun},
	} {
		if _, err := WorktreeStatus(context.Background(), home, tc[0], tc[1], tc[2], ExecGit); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(tc, err)
		}
		if err := WorktreeRemove(context.Background(), home, tc[0], tc[1], tc[2], WorktreeCounts{}, ExecGit); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(tc, err)
		}
	}
	// Repo names are slugged exactly as creation does: "../x" can only name
	// the folder "x" under the project, never a path outside it.
	if _, err := WorktreeStatus(context.Background(), home, "p", "../../../etc", testRun, ExecGit); !errors.Is(err, ErrWorktreeNotFound) {
		t.Fatal(err)
	}
}

// The only command that changes anything is exactly
// `git -C <repository root> worktree remove <path>`: never --force, never a
// branch command, never prune.
func TestWorktreeRemoveArgumentVectorIsExact(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	commitIn(t, w.Path, "agent")
	type call struct {
		dir  string
		args []string
	}
	var calls []call
	rec := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		calls = append(calls, call{dir, slices.Clone(args)})
		return ExecGit(ctx, dir, args...)
	}
	if _, err := WorktreeStatus(context.Background(), home, "p", "site", testRun, rec); err != nil {
		t.Fatal(err)
	}
	statusCalls := len(calls)
	if err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{Unpushed: 1}, rec); err != nil {
		t.Fatal(err)
	}
	readOnly := [][]string{
		{"rev-parse", "--show-toplevel"},
		{"rev-parse", "--git-common-dir"},
		{"worktree", "list", "--porcelain"},
		{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none"},
		{"symbolic-ref", "-q", "HEAD"},
		{"rev-list", "--count", "HEAD", "--not", "--remotes", "--exclude=" + w.Branch, "--branches"},
		{"rev-list", "--count", "HEAD", "--not", "--branches", "--remotes", "--tags"},
		{"ls-files", "-v", "-z"},
	}
	mutating := 0
	for i, c := range calls {
		for _, a := range c.args {
			if a == "--force" || a == "-f" || a == "-D" || a == "-d" || a == "branch" || a == "prune" || strings.HasPrefix(a, "-c") {
				t.Fatalf("forbidden argument in %q", c.args)
			}
		}
		if slices.ContainsFunc(readOnly, func(r []string) bool { return slices.Equal(r, c.args) }) {
			if c.dir != w.Path {
				t.Fatalf("read %q ran in %s", c.args, c.dir)
			}
			continue
		}
		mutating++
		if i < statusCalls || !slices.Equal(c.args, []string{"worktree", "remove", w.Path}) || c.dir != repo {
			t.Fatalf("unexpected git call %q in %s", c.args, c.dir)
		}
	}
	if mutating != 1 || statusCalls != len(readOnly) {
		t.Fatalf("want exactly one remove and %d reads for a status, got %d and %d", len(readOnly), mutating, statusCalls)
	}
	if !slices.Equal(reetBranches(t, repo), []string{"refs/heads/" + w.Branch}) {
		t.Fatal("branch not kept")
	}
}

// Git that fails a read is git_failed (never "not there", never removed).
func TestWorktreeGitFailureIsRefused(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	failing := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if args[0] == "status" {
			return []byte("fatal: index file corrupt\n"), errors.New("exit status 128")
		}
		return ExecGit(ctx, dir, args...)
	}
	err := WorktreeRemove(context.Background(), home, "p", "site", testRun, WorktreeCounts{}, failing)
	refusedWith(t, err, terminalproto.WorktreeGitFailed)
	if !exists(w.Path) {
		t.Fatal("removed")
	}
	missing := func(context.Context, string, ...string) ([]byte, error) { return nil, errGitMissing }
	_, err = WorktreeStatus(context.Background(), home, "p", "site", testRun, missing)
	refusedWith(t, err, terminalproto.WorktreeGitFailed)
}

func TestManagerWorktreeOperationsNeedConsent(t *testing.T) {
	requireGit(t)
	m, opts := managerFixture(t)
	home, err := filepath.EvalSymlinks(opts.Home)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	s, err := m.WorktreeStatus(context.Background(), terminalproto.WorktreeStatus{Run: testRun, Project: "p", Repo: "site"})
	if err != nil || s.Path != w.Path {
		t.Fatal(s, err)
	}
	m.consent = func() error { return errors.New("off") }
	if _, err := m.WorktreeStatus(context.Background(), terminalproto.WorktreeStatus{Run: testRun, Project: "p", Repo: "site"}); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatal(err)
	}
	if err := m.WorktreeRemove(context.Background(), terminalproto.WorktreeRemove{Run: testRun, Project: "p", Repo: "site"}); !errors.Is(err, ErrTerminalDisabled) || !exists(w.Path) {
		t.Fatal(err)
	}
	m.consent = func() error { return nil }
	if err := m.WorktreeRemove(context.Background(), terminalproto.WorktreeRemove{Run: testRun, Project: "p", Repo: "site"}); err != nil || exists(w.Path) {
		t.Fatal(err)
	}
}
