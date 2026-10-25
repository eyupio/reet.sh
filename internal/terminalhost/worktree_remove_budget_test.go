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
	parent, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var removeErr error
	var left time.Duration
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
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
	if left < gitTimeout-5*time.Second {
		t.Fatal("the remove got only what was left of the request's budget", left)
	}
	if exists(w.Path) {
		t.Fatal("the worktree is still there")
	}
}
