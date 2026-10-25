//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// processGone waits until pid no longer exists (a zombie still counts as
// present, so this also proves it was reaped).
func processGone(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// TestTimedOutAddKillsGitsChildrenAndCleansUp reproduces a repository-local
// smudge filter that hangs the checkout of `worktree add`: the add times
// out, the filter (git's child) is killed with git, what the add left is
// removed, and a retry makes a fresh worktree instead of adopting a broken one.
func TestTimedOutAddKillsGitsChildrenAndCleansUp(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "slow")
	pidfile := filepath.Join(home, "filter.pid")
	writeFile(t, filepath.Join(repo, ".gitattributes"), "README filter=slow\n")
	testGit(t, repo, "add", ".gitattributes")
	testGit(t, repo, "commit", "-q", "-m", "attributes")
	testGit(t, repo, "config", "filter.slow.smudge", "echo $$ > '"+pidfile+"'; exec sleep 30")
	start := time.Now()
	_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, newExecGit(2*time.Second))
	refusedWith(t, err, terminalproto.WorktreeGitFailed)
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatal("the timed-out add was waited for", elapsed)
	}
	data, rerr := os.ReadFile(pidfile)
	pid, perr := strconv.Atoi(strings.TrimSpace(string(data)))
	if rerr != nil || perr != nil || pid <= 1 {
		t.Fatal("the filter never ran", rerr, perr, string(data))
	}
	if !processGone(pid, 5*time.Second) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("git's child outlived the timeout", pid)
	}
	dest := wantPath(home, "p", "slow", testRun)
	if _, err := os.Lstat(dest); err == nil {
		t.Fatal("destination left after a timed-out add")
	}
	if b := reetBranches(t, repo); len(b) != 0 {
		t.Fatal("branch left after a timed-out add", b)
	}
	if n := worktreeCount(t, repo); n != 1 {
		t.Fatal("worktree still registered", testGit(t, repo, "worktree", "list", "--porcelain"))
	}
	testGit(t, repo, "config", "filter.slow.smudge", "cat")
	w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, ExecGit)
	if err != nil || !w.Created || w.Path != dest {
		t.Fatal("retry did not make a fresh worktree", w, err)
	}
}

func TestLockedOrPrunableWorktreeIsNotAdopted(t *testing.T) {
	home := worktreeHome(t)
	t.Run("locked", func(t *testing.T) {
		repo := newRepo(t, home, "locked")
		w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, ExecGit)
		if err != nil {
			t.Fatal(err)
		}
		testGit(t, repo, "worktree", "lock", "--reason", "initializing", w.Path)
		_, err = PrepareWorktree(context.Background(), home, repo, "p", testRun, ExecGit)
		refusedWith(t, err, terminalproto.WorktreeExists)
	})
	t.Run("prunable", func(t *testing.T) {
		repo := newRepo(t, home, "prunable")
		w, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, ExecGit)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(w.Path, ".git")); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(testGit(t, repo, "worktree", "list", "--porcelain"), "prunable") {
			t.Skip("this git does not report prunable worktrees")
		}
		_, err = PrepareWorktree(context.Background(), home, repo, "p", testRun, ExecGit)
		refusedWith(t, err, terminalproto.WorktreeExists)
	})
}

// A worktree whose path contains a newline prints extra lines in the
// porcelain list; they must never make another path look adoptable.
func TestWorktreeListParseResistsNewlinePaths(t *testing.T) {
	dest := "/h/.reet/worktrees/p/r/" + testRun
	branch := "reet/p-" + testRun
	sha := strings.Repeat("a", 40)
	fake := func(out string) GitRunner {
		return func(context.Context, string, ...string) ([]byte, error) { return []byte(out), nil }
	}
	good := "worktree /h/r\nHEAD " + sha + "\nbranch refs/heads/main\n\nworktree " + dest + "\nHEAD " + sha + "\nbranch refs/heads/" + branch + "\n\n"
	if listed, adoptable, err := gitWorktreeListed(context.Background(), fake(good), "/h/r", dest, branch); err != nil || !listed || !adoptable {
		t.Fatal("a well-formed entry is not adoptable", listed, adoptable, err)
	}
	for name, out := range map[string]string{
		"injected record": "worktree /h/r\nHEAD " + sha + "\nbranch refs/heads/main\n\nworktree /x\n\nworktree " + dest + "\nHEAD " + sha + "\nbranch refs/heads/" + branch + "\n\nHEAD " + sha + "\nbranch refs/heads/other\n\n",
		"injected lines":  "worktree /h/r\nHEAD " + sha + "\nbranch refs/heads/main\n\nworktree " + dest + "\nbranch refs/heads/" + branch + "\nHEAD " + sha + "\nbranch refs/heads/other\n\n",
		"listed twice":    good + "worktree " + dest + "\nHEAD " + sha + "\nbranch refs/heads/" + branch + "\n\n",
		"locked":          "worktree " + dest + "\nHEAD " + sha + "\nbranch refs/heads/" + branch + "\nlocked initializing\n\n",
		"prunable":        "worktree " + dest + "\nHEAD " + sha + "\nbranch refs/heads/" + branch + "\nprunable gitdir file points to non-existent location\n\n",
		"detached":        "worktree " + dest + "\nHEAD " + sha + "\ndetached\n\n",
		"other branch":    "worktree " + dest + "\nHEAD " + sha + "\nbranch refs/heads/" + branch + "x\n\n",
	} {
		if _, adoptable, _ := gitWorktreeListed(context.Background(), fake(out), "/h/r", dest, branch); adoptable {
			t.Error("adoptable:", name)
		}
	}
	// A destination containing a newline is never adoptable either.
	if _, adoptable, _ := gitWorktreeListed(context.Background(), fake(good), "/h/r", dest+"\nHEAD "+sha, branch); adoptable {
		t.Error("newline destination adoptable")
	}
}

func TestPrepareWorktreeRefusesHomeWithControlCharacters(t *testing.T) {
	for name, part := range map[string]string{"newline": "a\nb", "bidi": "a‮b", "tab": "a\tb"} {
		t.Run(name, func(t *testing.T) {
			base := worktreeHome(t)
			home := filepath.Join(base, part)
			repo := newRepo(t, home, "site")
			_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, ExecGit)
			refusedWith(t, err, terminalproto.WorktreeOutsideHome)
			if b := reetBranches(t, repo); len(b) != 0 {
				t.Fatal(b)
			}
		})
	}
}

func TestPrepareWorktreeRefusesWritableReetFolders(t *testing.T) {
	for _, tc := range []struct {
		rel  string
		mode os.FileMode
	}{{".reet", 0o777}, {".reet", 0o770}, {".reet/worktrees", 0o757}, {".reet/worktrees/p/site", 0o775}} {
		t.Run(tc.rel+" "+tc.mode.String(), func(t *testing.T) {
			home := worktreeHome(t)
			repo := newRepo(t, home, "site")
			dir := filepath.Join(home, tc.rel)
			os.MkdirAll(dir, 0o700)
			if err := os.Chmod(dir, tc.mode); err != nil {
				t.Fatal(err)
			}
			_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, ExecGit)
			refusedWith(t, err, terminalproto.WorktreeOutsideHome)
			if b := reetBranches(t, repo); len(b) != 0 {
				t.Fatal(b)
			}
		})
	}
}

func TestPrepareWorktreeRefusesSequencer(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "seq")
	os.Mkdir(filepath.Join(repo, ".git", "sequencer"), 0o700)
	_, err := PrepareWorktree(context.Background(), home, repo, "p", testRun, ExecGit)
	refusedWith(t, err, terminalproto.WorktreeRepoBusy)
	nothingCreated(t, home, repo)
}

// blockingGit never answers until its context ends, like a git stuck on a
// lock or a filter, ignoring any per-call cap.
func blockingGit(ctx context.Context, _ string, _ ...string) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestPrepareWorktreeHasOneOverallBudget(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "budget")
	outer, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	_, err := prepareWorktree(outer, home, repo, "p", testRun, blockingGit, 300*time.Millisecond, time.Second)
	refusedWith(t, err, terminalproto.WorktreeGitFailed)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatal("prepare exceeded its budget", elapsed)
	}
	if PrepareBudget := gitTimeout; PrepareBudget != 60*time.Second || cleanupTimeout != 30*time.Second {
		t.Fatal("budgets changed", gitTimeout, cleanupTimeout)
	}
}

func TestCleanupRunsAfterThePrepareBudgetExpired(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "late")
	// git makes the worktree, then the call outlives the budget; every other
	// call refuses a context that has already ended.
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if len(args) > 3 && args[2] == "worktree" && args[3] == "add" {
			if out, err := ExecGit(context.WithoutCancel(ctx), dir, args...); err != nil {
				return out, err
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return ExecGit(ctx, dir, args...)
	}
	_, err := prepareWorktree(context.Background(), home, repo, "p", testRun, git, 2*time.Second, 30*time.Second)
	refusedWith(t, err, terminalproto.WorktreeGitFailed)
	nothingCreated(t, home, repo)
	if _, err := os.Lstat(wantPath(home, "p", "late", testRun)); err == nil {
		t.Fatal("destination left")
	}
}

// tmuxThatFailsAfterNewSession runs the real new-session, then reports
// failure, as a tmux that started the agent but lost its reply would.
func tmuxThatFailsAfterNewSession(t *testing.T, m *Manager) {
	t.Helper()
	wrapper := filepath.Join(shortTempDir(t), "tmux")
	script := "#!/bin/sh\nfor a in \"$@\"; do if [ \"$a\" = new-session ]; then '" + m.tmux.Path + "' \"$@\"; exit 1; fi; done\nexec '" + m.tmux.Path + "' \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	m.tmux.Path = wrapper
}

func TestManagerKeepsWorktreeWhenTmuxFailedButTheSessionExists(t *testing.T) {
	m, _, _, record, repo, realHome := worktreeManager(t)
	tmuxThatFailsAfterNewSession(t, m)
	if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), worktreeCreate(repo, testRun)); err == nil {
		t.Fatal("tmux failure not reported")
	}
	dest := wantPath(realHome, "p", "site", testRun)
	if got := readRecord(t, record); got[len(got)-1] != dest {
		t.Fatal("agent not running in the worktree", got)
	}
	if info, err := os.Lstat(dest); err != nil || !info.IsDir() {
		t.Fatal("the worktree of a running agent was removed", err)
	}
	if b := reetBranches(t, repo); len(b) != 1 {
		t.Fatal("the branch of a running agent was removed", b)
	}
}

func TestManagerRemovesWorktreeWhenTmuxFailedWithoutSession(t *testing.T) {
	m, _, _, _, repo, realHome := worktreeManager(t)
	failingNewSession(t, m)
	if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), worktreeCreate(repo, testRun)); err == nil {
		t.Fatal("start did not fail")
	}
	names, err := m.sessionNames(context.Background())
	if err != nil || names[m.name(testSessionID(1))] {
		t.Fatal("a session exists", err)
	}
	if _, err := os.Lstat(wantPath(realHome, "p", "site", testRun)); err == nil {
		t.Fatal("worktree left")
	}
	if b := reetBranches(t, repo); len(b) != 0 {
		t.Fatal("created branch left", b)
	}
}
