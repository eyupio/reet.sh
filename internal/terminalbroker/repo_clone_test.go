package terminalbroker

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func repoCloneCapabilities() terminalproto.Capabilities {
	caps := browseCapabilities()
	caps.RepoClone = true
	return caps
}

func TestCloneRepoIsForwardedExactlyAndNeedsTheCapability(t *testing.T) {
	f := browseFixture(t, repoCloneCapabilities())
	want := terminalproto.RepoCloneResult{Dir: "/home/test/reet/repos/acme/site", DefaultBranch: "main", Marked: true}
	f.answering(func(terminalproto.Body) terminalproto.Body { return want })
	req := terminalproto.RepoClone{Owner: "acme", Repo: "site"}
	got, err := f.b.CloneRepo(t.Context(), f.uid, f.agent, req)
	if err != nil || got != want {
		t.Fatal(got, err)
	}
	if sent := f.sentRequests(); len(sent) != 1 || !reflect.DeepEqual(sent[0], req) {
		t.Fatalf("request not forwarded exactly: %+v", sent)
	}

	old := browseFixture(t, browseCapabilities()) // no repo-clone-v1
	if _, err := old.b.CloneRepo(t.Context(), old.uid, old.agent, req); !errors.Is(err, ErrRepoCloneUnsupported) {
		t.Fatalf("err = %v, want unsupported", err)
	}
	if len(old.sentRequests()) != 0 {
		t.Fatal("a clone was sent to a machine that did not negotiate it")
	}
}

// Every reason the machine gives, busy and denied included, comes back as
// the machine's answer for the caller to word; none is an error.
func TestCloneRepoReturnsEachReason(t *testing.T) {
	f := browseFixture(t, repoCloneCapabilities())
	for _, reason := range []terminalproto.RepoCloneReason{terminalproto.RepoCloneNotSignedIn, terminalproto.RepoCloneNoAccess, terminalproto.RepoCloneExists,
		terminalproto.RepoCloneTimedOut, terminalproto.RepoCloneFailed, terminalproto.RepoCloneDenied, terminalproto.RepoCloneBusy} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.RepoCloneResult{Reason: reason} })
		got, err := f.b.CloneRepo(t.Context(), f.uid, f.agent, terminalproto.RepoClone{Owner: "acme", Repo: "site"})
		if err != nil || got.Reason != reason {
			t.Fatalf("%s: %+v %v", reason, got, err)
		}
	}
}

func TestCloneRepoRefusesAMalformedRequestBeforeSending(t *testing.T) {
	f := browseFixture(t, repoCloneCapabilities())
	for _, req := range []terminalproto.RepoClone{{Owner: "acme"}, {Owner: "-acme", Repo: "site"}, {Owner: "acme", Repo: "site.git"}} {
		if _, err := f.b.CloneRepo(t.Context(), f.uid, f.agent, req); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%+v: %v", req, err)
		}
	}
	if len(f.sentRequests()) != 0 {
		t.Fatal("a malformed clone was sent")
	}
}

func TestCloneRepoCallTimeoutOutlastsTheMachinesBound(t *testing.T) {
	if RepoCloneCallTimeout != 10*time.Minute+30*time.Second {
		t.Fatal(RepoCloneCallTimeout)
	}
}
