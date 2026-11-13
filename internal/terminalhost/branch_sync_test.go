//go:build linux || darwin

package terminalhost

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// syncFixture is a repository with a bare origin and a second clone that
// pushes new work to main, as a merged pull request would.
type syncFixture struct {
	home, repo, other, origin string
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
	return syncFixture{home: home, repo: repo, other: other, origin: origin}
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

// fakeGh puts an executable named gh at the front of PATH and returns its
// path, as a machine the person has signed in to gh on has.
func fakeGh(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gh := filepath.Join(dir, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return gh
}

// withoutGh leaves only git on PATH, as a machine gh was never installed on.
func withoutGh(t *testing.T) {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(git, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// An ssh github.com origin is fetched over https with gh's credentials, and
// never over ssh: the hardened environment has no ssh agent to ask and no
// terminal to answer a passphrase or host-key prompt.
func TestSyncBranchSSHOriginFetchesOverHTTPSWithGh(t *testing.T) {
	f := newSyncFixture(t)
	gh := fakeGh(t)
	testGit(t, f.repo, "remote", "set-url", "origin", "git@github.com:o/r.git")
	f.pushMerged(t, "merged-pr")
	var calls [][]string
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		calls = append(calls, slices.Clone(args))
		rewrite := []string{"-c", "url." + f.origin + ".insteadOf=https://github.com/o/r.git"}
		return ExecGit(ctx, dir, append(rewrite, args...)...)
	}
	if r := SyncBranch(context.Background(), f.home, terminalproto.BranchSync{Dir: f.repo}, git); r.Status != terminalproto.BranchSyncUpdated {
		t.Fatalf("result = %+v, want updated", r)
	}
	var fetch []string
	for _, args := range calls {
		if slices.Contains(args, "fetch") {
			fetch = args
		}
		for _, arg := range args {
			if strings.Contains(arg, "ssh://") || strings.Contains(arg, "git@github.com") {
				t.Fatalf("a git call named an ssh URL: %q", args)
			}
		}
	}
	if !slices.Contains(fetch, "https://github.com/o/r.git") {
		t.Fatalf("fetch %q did not name the repository's https URL", fetch)
	}
	if !slices.Contains(fetch, "credential.https://github.com.helper=!'"+gh+"' auth git-credential") {
		t.Fatalf("fetch %q did not carry gh's credentials", fetch)
	}
}

// With no gh to authenticate as, an ssh origin is not fetched at all: the
// folder is left exactly as it was and the answer says GitHub was not reached.
func TestSyncBranchSSHOriginWithoutGhDoesNotFetch(t *testing.T) {
	f := newSyncFixture(t)
	withoutGh(t)
	testGit(t, f.repo, "remote", "set-url", "origin", "git@github.com:o/r.git")
	before := testGit(t, f.repo, "rev-parse", "HEAD")
	var calls [][]string
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		calls = append(calls, slices.Clone(args))
		return ExecGit(ctx, dir, args...)
	}
	r := SyncBranch(context.Background(), f.home, terminalproto.BranchSync{Dir: f.repo}, git)
	if r.Status != terminalproto.BranchSyncFetchFailed {
		t.Fatalf("result = %+v, want fetch_failed", r)
	}
	for _, args := range calls {
		if slices.Contains(args, "fetch") {
			t.Fatalf("an ssh origin was fetched without gh: %q", args)
		}
	}
	if after := testGit(t, f.repo, "rev-parse", "HEAD"); after != before {
		t.Fatalf("HEAD moved from %s to %s", before, after)
	}
}

// A sync of a github.com repository fetches with the machine's own gh
// sign-in, and a repository hosted anywhere else is fetched exactly as before.
func TestSyncBranchFetchCarriesTheGhHelper(t *testing.T) {
	f := newSyncFixture(t)
	gh := fakeGh(t)
	helper := "credential.https://github.com.helper=!'" + gh + "' auth git-credential"
	var fetches [][]string
	record := func(args []string) {
		if slices.Contains(args, "fetch") {
			fetches = append(fetches, slices.Clone(args))
		}
	}
	plain := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		record(args)
		return ExecGit(ctx, dir, args...)
	}
	if r := SyncBranch(context.Background(), f.home, terminalproto.BranchSync{Dir: f.repo}, plain); r.Reason != "" {
		t.Fatalf("sync refused: %+v", r)
	}
	if len(fetches) != 1 || slices.Contains(fetches[0], helper) {
		t.Fatalf("a fetch from %s carried gh's credentials: %q", f.origin, fetches)
	}

	// The same repository, now hosted on github.com; the fixture's origin
	// stands in for it so nothing reaches the network.
	testGit(t, f.repo, "remote", "set-url", "origin", "https://github.com/o/r")
	fetches = nil
	onGitHub := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		record(args)
		rewrite := []string{"-c", "url." + f.origin + ".insteadOf=https://github.com/o/r"}
		return ExecGit(ctx, dir, append(rewrite, args...)...)
	}
	f.pushMerged(t, "merged-pr")
	r := SyncBranch(context.Background(), f.home, terminalproto.BranchSync{Dir: f.repo}, onGitHub)
	if r.Status != terminalproto.BranchSyncUpdated {
		t.Fatalf("result = %+v, want updated", r)
	}
	if len(fetches) != 1 || !slices.Contains(fetches[0], helper) {
		t.Fatalf("a fetch from github.com did not carry gh's credentials: %q", fetches)
	}
	// The person's own helpers are kept: a machine whose gh is signed out
	// still fetches with the helper their ~/.gitconfig names, as it did
	// before the base was ever fetched.
	if slices.Contains(fetches[0], "credential.helper=") {
		t.Fatalf("a fetch from github.com reset the inherited credential helpers: %q", fetches[0])
	}
}
