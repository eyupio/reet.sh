package terminalbroker

import (
	"errors"
	"reflect"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestSyncBranchIsForwardedExactlyAndNeedsTheCapability(t *testing.T) {
	caps := browseCapabilities()
	caps.BranchSync = true
	f := browseFixture(t, caps)
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		return terminalproto.BranchSyncResult{Status: terminalproto.BranchSyncUpdated, Base: "main"}
	})
	req := terminalproto.BranchSync{Dir: "/home/test/site"}
	got, err := f.b.SyncBranch(t.Context(), f.uid, f.agent, req)
	if err != nil || got.Status != terminalproto.BranchSyncUpdated || got.Base != "main" {
		t.Fatal(got, err)
	}
	if sent := f.sentRequests(); len(sent) != 1 || !reflect.DeepEqual(sent[0], req) {
		t.Fatalf("request not forwarded exactly: %+v", sent)
	}

	old := browseFixture(t, browseCapabilities()) // no branch-sync-v1
	if _, err := old.b.SyncBranch(t.Context(), old.uid, old.agent, req); !errors.Is(err, ErrBranchSyncUnsupported) {
		t.Fatalf("err = %v, want unsupported", err)
	}
	if len(old.sentRequests()) != 0 {
		t.Fatal("sync sent to a machine that did not negotiate it")
	}
}

func TestSyncBranchRefusalIsAnError(t *testing.T) {
	caps := browseCapabilities()
	caps.BranchSync = true
	f := browseFixture(t, caps)
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		return terminalproto.BranchSyncResult{Reason: terminalproto.BranchesNotRepoRoot}
	})
	_, err := f.b.SyncBranch(t.Context(), f.uid, f.agent, terminalproto.BranchSync{Dir: "/home/test/site"})
	var refusal *BranchSyncError
	if !errors.As(err, &refusal) || refusal.Reason != terminalproto.BranchesNotRepoRoot {
		t.Fatalf("err = %v, want a refusal", err)
	}
}
