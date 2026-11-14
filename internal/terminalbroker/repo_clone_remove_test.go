package terminalbroker

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const cloneFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func repoCloneRemoveCapabilities() terminalproto.Capabilities {
	caps := browseCapabilities()
	caps.RepoCloneRemove = true
	return caps
}

func TestCloneStatusIsForwardedExactlyAndNeedsTheCapability(t *testing.T) {
	f := browseFixture(t, repoCloneRemoveCapabilities())
	want := terminalproto.RepoCloneState{Marked: true, Changed: 2, Fingerprint: cloneFingerprint}
	f.answering(func(terminalproto.Body) terminalproto.Body { return want })
	req := terminalproto.RepoCloneStatus{Dir: "/home/test/reet/repos/acme/site", Origin: "acme/site"}
	got, err := f.b.CloneStatus(t.Context(), f.uid, f.agent, req)
	if err != nil || got != want {
		t.Fatal(got, err)
	}
	if sent := f.sentRequests(); len(sent) != 1 || !reflect.DeepEqual(sent[0], req) {
		t.Fatalf("request not forwarded exactly: %+v", sent)
	}

	// A machine that can clone but shipped before the remove is never asked.
	old := browseFixture(t, repoCloneCapabilities())
	if _, err := old.b.CloneStatus(t.Context(), old.uid, old.agent, req); !errors.Is(err, ErrRepoCloneRemoveUnsupported) {
		t.Fatalf("err = %v, want unsupported", err)
	}
	if len(old.sentRequests()) != 0 {
		t.Fatal("a status was sent to a machine that did not negotiate it")
	}
}

func TestRemoveCloneIsForwardedExactlyAndNeedsTheCapability(t *testing.T) {
	f := browseFixture(t, repoCloneRemoveCapabilities())
	want := terminalproto.RepoCloneRemoved{Dir: "/home/test/reet/repos/acme/site"}
	f.answering(func(terminalproto.Body) terminalproto.Body { return want })
	req := terminalproto.RepoCloneRemove{Dir: "/home/test/reet/repos/acme/site", Origin: "acme/site", Fingerprint: cloneFingerprint}
	got, err := f.b.RemoveClone(t.Context(), f.uid, f.agent, req)
	if err != nil || got != want {
		t.Fatal(got, err)
	}
	if sent := f.sentRequests(); len(sent) != 1 || !reflect.DeepEqual(sent[0], req) {
		t.Fatalf("request not forwarded exactly: %+v", sent)
	}

	old := browseFixture(t, repoCloneCapabilities())
	if _, err := old.b.RemoveClone(t.Context(), old.uid, old.agent, req); !errors.Is(err, ErrRepoCloneRemoveUnsupported) {
		t.Fatalf("err = %v, want unsupported", err)
	}
	if len(old.sentRequests()) != 0 {
		t.Fatal("a remove was sent to a machine that did not negotiate it")
	}
}

// Every reason the machine gives comes back as its answer for the caller to
// word; none is an error.
func TestCloneStatusAndRemoveReturnEachReason(t *testing.T) {
	f := browseFixture(t, repoCloneRemoveCapabilities())
	status := terminalproto.RepoCloneStatus{Dir: "/home/test/reet/repos/acme/site", Origin: "acme/site"}
	remove := terminalproto.RepoCloneRemove{Dir: status.Dir, Origin: status.Origin, Fingerprint: cloneFingerprint}
	for _, reason := range []terminalproto.RepoCloneReason{terminalproto.RepoCloneDenied, terminalproto.RepoCloneBusy, terminalproto.RepoCloneTimedOut,
		terminalproto.RepoCloneFailed, terminalproto.RepoCloneExists, terminalproto.RepoCloneNotRemovable} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.RepoCloneState{Reason: reason} })
		got, err := f.b.CloneStatus(t.Context(), f.uid, f.agent, status)
		if err != nil || got.Reason != reason {
			t.Fatalf("status %s: %+v %v", reason, got, err)
		}
	}
	for _, reason := range []terminalproto.RepoCloneReason{terminalproto.RepoCloneChanged, terminalproto.RepoCloneNotRemovable, terminalproto.RepoCloneDenied,
		terminalproto.RepoCloneBusy, terminalproto.RepoCloneTimedOut, terminalproto.RepoCloneFailed, terminalproto.RepoCloneExists} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.RepoCloneRemoved{Reason: reason} })
		got, err := f.b.RemoveClone(t.Context(), f.uid, f.agent, remove)
		if err != nil || got.Reason != reason {
			t.Fatalf("remove %s: %+v %v", reason, got, err)
		}
	}
	// An answer of the other kind is not an answer.
	f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.RepoCloneRemoved{Dir: status.Dir} })
	if _, err := f.b.CloneStatus(t.Context(), f.uid, f.agent, status); !errors.Is(err, ErrMachineAnswer) {
		t.Fatalf("status answered by a removed: %v", err)
	}
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.RepoCloneState{Marked: true, Fingerprint: cloneFingerprint}
	})
	if _, err := f.b.RemoveClone(t.Context(), f.uid, f.agent, remove); !errors.Is(err, ErrMachineAnswer) {
		t.Fatalf("remove answered by a state: %v", err)
	}
}

func TestCloneStatusAndRemoveRefuseAMalformedRequestBeforeSending(t *testing.T) {
	f := browseFixture(t, repoCloneRemoveCapabilities())
	for _, req := range []terminalproto.RepoCloneStatus{{Origin: "acme/site"}, {Dir: "/home/test/x"}, {Dir: "x", Origin: "acme/site"}, {Dir: "/home/test/x", Origin: "acme/site.git"}} {
		if _, err := f.b.CloneStatus(t.Context(), f.uid, f.agent, req); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%+v: %v", req, err)
		}
	}
	for _, req := range []terminalproto.RepoCloneRemove{{Dir: "/home/test/x", Origin: "acme/site"}, {Dir: "/home/test/x", Origin: "acme/site", Fingerprint: "short"}, {Origin: "acme/site", Fingerprint: cloneFingerprint}} {
		if _, err := f.b.RemoveClone(t.Context(), f.uid, f.agent, req); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%+v: %v", req, err)
		}
	}
	if len(f.sentRequests()) != 0 {
		t.Fatal("a malformed request was sent")
	}
}

func TestCloneStatusAndRemoveCallTimeouts(t *testing.T) {
	if RepoCloneStatusCallTimeout != 15*time.Second || RepoCloneRemoveCallTimeout != 60*time.Second {
		t.Fatal(RepoCloneStatusCallTimeout, RepoCloneRemoveCallTimeout)
	}
}

// An answer that arrives after its call gave up is counted and dropped: it
// does not end the machine's connection and answers nothing later.
func TestCloneStatusAndRemoveLateAnswersAreDroppedAndCounted(t *testing.T) {
	status, remove := RepoCloneStatusCallTimeout, RepoCloneRemoveCallTimeout
	RepoCloneStatusCallTimeout, RepoCloneRemoveCallTimeout = 100*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { RepoCloneStatusCallTimeout, RepoCloneRemoveCallTimeout = status, remove })
	f := browseFixture(t, repoCloneRemoveCapabilities())
	dir := "/home/test/reet/repos/acme/site"
	if _, err := f.b.CloneStatus(t.Context(), f.uid, f.agent, terminalproto.RepoCloneStatus{Dir: dir, Origin: "acme/site"}); !errors.Is(err, ErrMachineTimeout) {
		t.Fatal("status timeout", err)
	}
	if _, err := f.b.RemoveClone(t.Context(), f.uid, f.agent, terminalproto.RepoCloneRemove{Dir: dir, Origin: "acme/site", Fingerprint: cloneFingerprint}); !errors.Is(err, ErrMachineTimeout) {
		t.Fatal("remove timeout", err)
	}
	held := f.heldReplies()
	if len(held) != 2 {
		t.Fatal("held", held)
	}
	late := held[0]
	late.Body = terminalproto.RepoCloneState{Marked: true, Fingerprint: cloneFingerprint}
	if err := f.deliver(late); err != nil {
		t.Fatal("a late clone state is a protocol error", err)
	}
	late = held[1]
	late.Body = terminalproto.RepoCloneRemoved{Dir: dir}
	if err := f.deliver(late); err != nil {
		t.Fatal("a late clone removed is a protocol error", err)
	}
	if f.b.LateReplies() != 2 {
		t.Fatal("late replies counted", f.b.LateReplies())
	}
	select {
	case <-f.peer.Done():
		t.Fatal("a late answer ended the connection")
	default:
	}
}
