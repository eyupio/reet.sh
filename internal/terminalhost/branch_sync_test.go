//go:build linux || darwin

package terminalhost

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// syncFixture is a repository with a bare origin and a second clone that
// pushes new work to main, as a merged pull request would.
type syncFixture struct {
	home, repo, other string
}

func newSyncFixture(t *testing.T) syncFixture {
	t.Helper()
	home := worktreeHome(t)
	repo := newRepo(t, home, "sync")
	origin := filepath.Join(home, "origin.git")
	testGit(t, home, "init", "-q", "--bare", "--initial-branch=main", origin)
	testGit(t, repo, "remote", "add", "origin", origin)
	testGit(t, repo, "push", "-q", "origin", "main")
	other := filepath.Join(home, "other")
	testGit(t, home, "clone", "-q", origin, other)
	testGit(t, other, "config", "user.email", "t@example.com")
	testGit(t, other, "config", "user.name", "t")
	return syncFixture{home: home, repo: repo, other: other}
}

func (f syncFixture) pushMerged(t *testing.T, name string) {
	t.Helper()
	writeFile(t, filepath.Join(f.other, name), "x\n")
	testGit(t, f.other, "add", name)
	testGit(t, f.other, "commit", "-q", "-m", name)
	testGit(t, f.other, "push", "-q", "origin", "main")
}

func (f syncFixture) sync(t *testing.T) terminalproto.BranchSyncResult {
	t.Helper()
	r := SyncBranch(context.Background(), f.home, terminalproto.BranchSync{Dir: f.repo}, ExecGit)
	if r.Reason != "" {
		t.Fatalf("sync refused: %+v", r)
	}
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: "branch-sync-test-0001", Generation: 1, Body: r}); err != nil {
		t.Fatalf("result does not encode: %v (%+v)", err, r)
	}
	return r
}

func TestSyncBranchFastForwardsACleanFolderOnMain(t *testing.T) {
	f := newSyncFixture(t)
	f.pushMerged(t, "merged-pr")
	if r := f.sync(t); r.Status != terminalproto.BranchSyncUpdated || r.Base != "main" {
		t.Fatalf("result = %+v, want updated on main", r)
	}
	if _, err := ExecGit(context.Background(), f.repo, "cat-file", "-e", "HEAD:merged-pr"); err != nil {
		t.Fatalf("folder was not fast-forwarded: %v", err)
	}
	if r := f.sync(t); r.Status != terminalproto.BranchSyncCurrent {
		t.Fatalf("second sync = %+v, want current", r)
	}
}

func TestSyncBranchLeavesADirtyFolderAlone(t *testing.T) {
	f := newSyncFixture(t)
	f.pushMerged(t, "merged-pr")
	writeFile(t, filepath.Join(f.repo, "README"), "edited\n")
	before := testGit(t, f.repo, "rev-parse", "HEAD")
	if r := f.sync(t); r.Status != terminalproto.BranchSyncDirty {
		t.Fatalf("result = %+v, want dirty", r)
	}
	if after := testGit(t, f.repo, "rev-parse", "HEAD"); after != before {
		t.Fatalf("HEAD moved from %s to %s", before, after)
	}
}

func TestSyncBranchLeavesAnotherBranchAlone(t *testing.T) {
	f := newSyncFixture(t)
	f.pushMerged(t, "merged-pr")
	testGit(t, f.repo, "checkout", "-q", "-b", "feature")
	if r := f.sync(t); r.Status != terminalproto.BranchSyncOffBase {
		t.Fatalf("result = %+v, want off_base", r)
	}
	if cur := strings.TrimSpace(testGit(t, f.repo, "symbolic-ref", "--short", "HEAD")); cur != "feature" {
		t.Fatalf("branch changed to %q", cur)
	}
}

func TestSyncBranchRefusesToRewriteADivergedMain(t *testing.T) {
	f := newSyncFixture(t)
	f.pushMerged(t, "merged-pr")
	writeFile(t, filepath.Join(f.repo, "local"), "x\n")
	testGit(t, f.repo, "add", "local")
	testGit(t, f.repo, "commit", "-q", "-m", "local")
	before := testGit(t, f.repo, "rev-parse", "HEAD")
	if r := f.sync(t); r.Status != terminalproto.BranchSyncDiverged {
		t.Fatalf("result = %+v, want diverged", r)
	}
	if after := testGit(t, f.repo, "rev-parse", "HEAD"); after != before {
		t.Fatalf("HEAD moved from %s to %s", before, after)
	}
}

func TestSyncBranchWithoutAnOriginChangesNothing(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "sync-none")
	r := SyncBranch(context.Background(), home, terminalproto.BranchSync{Dir: repo}, ExecGit)
	if r.Status != terminalproto.BranchSyncNoRemote {
		t.Fatalf("result = %+v, want no_remote", r)
	}
}

func TestSyncBranchRefusesAFolderOutsideHome(t *testing.T) {
	home := worktreeHome(t)
	r := SyncBranch(context.Background(), home, terminalproto.BranchSync{Dir: "/"}, ExecGit)
	if r.Reason == "" {
		t.Fatalf("result = %+v, want a refusal", r)
	}
}
