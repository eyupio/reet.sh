package terminalhost

import (
	"context"
	"testing"
	"time"
)

// A remove reports the folder it deleted and the branch it kept, and its one
// changing git call gets a budget of its own: neither the time the checks used
// nor the end of the request's connection cuts it short once it has started.
func TestWorktreeRemoveReportsWhatWentAndHasItsOwnBudget(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "code/site")
	w := madeWorktree(t, home, repo, testRun)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	var removeErr error
	var left, checkLeft time.Duration
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "status" {
			if d, ok := ctx.Deadline(); ok {
				checkLeft = time.Until(d)
			}
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "remove" {
			cancel() // the connection closes just as the remove starts
			removeErr = ctx.Err()
			if d, ok := ctx.Deadline(); ok {
				left = time.Until(d)
			}
		}
		return ExecGit(ctx, dir, args...)
	}
	removed, err := RemoveWorktree(parent, home, "p", "site", testRun, WorktreeCounts{}, git)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Path != w.Path || removed.Branch != w.Branch {
		t.Fatal("remove reported", removed, "want", w.Path, w.Branch)
	}
	if removeErr != nil {
		t.Fatal("the remove ran under a context the connection could end", removeErr)
	}
	// The whole remove is bounded by 60 seconds: 30 for the checks, then 30
	// of its own for the one changing call.
	if checkLeft <= 0 || checkLeft > removeCheckTimeout || removeCheckTimeout != 30*time.Second {
		t.Fatal("the checks' budget", checkLeft, removeCheckTimeout)
	}
	if left < removeStepTimeout-5*time.Second || left > removeStepTimeout || removeStepTimeout != 30*time.Second {
		t.Fatal("the remove's own budget", left, removeStepTimeout)
	}
	if exists(w.Path) {
		t.Fatal("the worktree is still there")
	}
}
