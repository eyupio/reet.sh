//go:build linux || darwin

package terminalhost

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A branch that origin does not have is "no such remote branch", not a failed
// fetch: nothing is wrong with the network or the credentials.
func TestSyncBranchOfABranchOriginDoesNotHaveIsNoRemote(t *testing.T) {
	f := newSyncFixture(t)
	testGit(t, f.repo, "branch", "never-pushed")
	r := SyncBranch(context.Background(), f.home, terminalproto.BranchSync{Dir: f.repo, Base: "never-pushed"}, ExecGit)
	if r.Status != terminalproto.BranchSyncNoRemote {
		t.Fatalf("result = %+v, want no_remote", r)
	}
}

// An origin that cannot be reached at all is still a failed fetch.
func TestSyncBranchOfAnUnreachableOriginIsFetchFailed(t *testing.T) {
	f := newSyncFixture(t)
	testGit(t, f.repo, "remote", "set-url", "origin", filepath.Join(f.home, "no-such-origin.git"))
	r := f.sync(t)
	if r.Status != terminalproto.BranchSyncFetchFailed {
		t.Fatalf("result = %+v, want fetch_failed", r)
	}
}
