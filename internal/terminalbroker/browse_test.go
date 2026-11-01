package terminalbroker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// browsePeer is a connected peer that answers browse and branches requests
// with answer; a nil answer holds the request in held for a later reply.
type browsePeer struct {
	b      *Broker
	st     *store.Store
	uid    int64
	agent  string
	peer   *Peer
	mu     sync.Mutex
	asked  []terminalproto.Body
	answer func(terminalproto.Body) terminalproto.Body
	held   []terminalproto.Message
}

func browseCapabilities() terminalproto.Capabilities {
	caps := worktreeCapabilities("claude")
	caps.Browse, caps.Branches = true, true
	return caps
}

func browseFixture(t *testing.T, caps terminalproto.Capabilities) *browsePeer {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "studio", "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	f := &browsePeer{b: New(st), st: st, uid: uid, agent: a.ID}
	t.Cleanup(f.b.Close)
	f.peer, err = f.b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Browse, terminalproto.Branches, terminalproto.BranchPrunePlan, terminalproto.BranchPrune:
			f.mu.Lock()
			f.asked = append(f.asked, op)
			answer := f.answer
			f.mu.Unlock()
			if answer != nil {
				reply.Body = answer(op)
			} else {
				reply.Body = nil
			}
			if reply.Body == nil {
				f.mu.Lock()
				f.held = append(f.held, reply)
				f.mu.Unlock()
				return nil
			}
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		return f.deliver(reply)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.peer.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f
}

// deliver passes a reply through the wire encoding to the peer, as the
// controller's reader does.
func (f *browsePeer) deliver(m terminalproto.Message) error {
	data, err := terminalproto.Encode(m)
	if err != nil {
		return err
	}
	decoded, err := terminalproto.Decode(data)
	if err != nil {
		return err
	}
	return f.peer.Receive(decoded)
}

func (f *browsePeer) answering(fn func(terminalproto.Body) terminalproto.Body) {
	f.mu.Lock()
	f.answer = fn
	f.mu.Unlock()
}

func (f *browsePeer) sentRequests() []terminalproto.Body {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Body(nil), f.asked...)
}

func (f *browsePeer) heldReplies() []terminalproto.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Message(nil), f.held...)
}

func homeListing() terminalproto.BrowseResult {
	return terminalproto.BrowseResult{Path: "/home/test", Home: "/home/test", Repo: terminalproto.BrowseRepoNone,
		Entries: []terminalproto.BrowseEntry{{Name: "site", Dir: true, RepoRoot: true}, {Name: "work", Dir: true}}, Scanned: 3, Omitted: 1}
}

func TestBrowseIsNeverSentToAPeerWithoutTheCapability(t *testing.T) {
	for name, tc := range map[string]struct {
		caps                   terminalproto.Capabilities
		browseErr, branchesErr error
	}{
		"old peer":    {worktreeCapabilities("claude"), ErrBrowseUnsupported, ErrBranchesUnsupported},
		"legacy peer": {legacyCapabilities(), ErrBrowseUnsupported, ErrBranchesUnsupported},
		"browse switched off": {func() terminalproto.Capabilities {
			c := browseCapabilities()
			c.Browse, c.BrowseOff = false, true
			return c
		}(), ErrBrowseOff, nil},
		// A current machine with browse off and no git is not an older reet.
		"browse off and no git": {func() terminalproto.Capabilities {
			c := browseCapabilities()
			c.Browse, c.BrowseOff, c.Branches = false, true, false
			return c
		}(), ErrBrowseOff, ErrBranchesUnavailable},
		"branches not offered": {func() terminalproto.Capabilities { c := browseCapabilities(); c.Branches = false; return c }(), nil, ErrBranchesUnavailable},
	} {
		f := browseFixture(t, tc.caps)
		f.answering(func(body terminalproto.Body) terminalproto.Body {
			if _, ok := body.(terminalproto.Browse); ok {
				return homeListing()
			}
			return terminalproto.BranchesResult{Branches: []string{"main"}}
		})
		_, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{})
		if tc.browseErr != nil && !errors.Is(err, tc.browseErr) || tc.browseErr == nil && err != nil {
			t.Fatal(name, "browse", err)
		}
		_, err = f.b.Branches(t.Context(), f.uid, f.agent, terminalproto.Branches{Dir: "/home/test/site"})
		if tc.branchesErr != nil && !errors.Is(err, tc.branchesErr) || tc.branchesErr == nil && err != nil {
			t.Fatal(name, "branches", err)
		}
		for _, body := range f.sentRequests() {
			_, browse := body.(terminalproto.Browse)
			if browse && tc.browseErr != nil || !browse && tc.branchesErr != nil {
				t.Fatal(name, "a request reached a peer without its capability", body)
			}
		}
	}
}

func TestBrowseRequestsAreValidatedBeforeAnyPeerState(t *testing.T) {
	f := browseFixture(t, browseCapabilities())
	f.answering(func(terminalproto.Body) terminalproto.Body { return homeListing() })
	for name, req := range map[string]terminalproto.Browse{
		"relative path":       {Path: "work"},
		"too long":            {Path: "/" + strings.Repeat("a", terminalproto.MaxDirBytes)},
		"control character":   {Path: "/home/test/a\nb"},
		"long after":          {After: strings.Repeat("a", 256)},
		"long filter":         {Filter: strings.Repeat("f", 65)},
		"filter with newline": {Filter: "a\nb"},
	} {
		if err := CheckBrowse(req); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(name, "check", err)
		}
		if _, err := f.b.Browse(t.Context(), f.uid, f.agent, req); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(name, err)
		}
	}
	for name, req := range map[string]terminalproto.Branches{
		"no folder":   {},
		"relative":    {Dir: "site"},
		"long filter": {Dir: "/home/test/site", Filter: strings.Repeat("f", 65)},
		"control":     {Dir: "/home/test/\x07site"},
	} {
		if err := CheckBranches(req); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(name, "check", err)
		}
		if _, err := f.b.Branches(t.Context(), f.uid, f.agent, req); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(name, err)
		}
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatal("an invalid request reached the connector", sent)
	}
	if err := CheckBrowse(terminalproto.Browse{}); err != nil {
		t.Fatal("home browse refused", err)
	}
}

func TestBrowseAndBranchesAreForwardedExactly(t *testing.T) {
	f := browseFixture(t, browseCapabilities())
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.Browse); ok {
			return homeListing()
		}
		return terminalproto.BranchesResult{Current: "main", Default: "main", Branches: []string{"main", "origin/main"}, More: true}
	})
	req := terminalproto.Browse{Path: "/home/test", After: "a", Filter: "si", Hidden: true}
	got, err := f.b.Browse(t.Context(), f.uid, f.agent, req)
	if err != nil || got.Path != "/home/test" || len(got.Entries) != 2 || got.Entries[0].Name != "site" || !got.Entries[0].RepoRoot || got.Scanned != 3 || got.Omitted != 1 {
		t.Fatal(got, err)
	}
	branches, err := f.b.Branches(t.Context(), f.uid, f.agent, terminalproto.Branches{Dir: "/home/test/site", Filter: "ma"})
	if err != nil || branches.Current != "main" || branches.Default != "main" || len(branches.Branches) != 2 || !branches.More {
		t.Fatal(branches, err)
	}
	sent := f.sentRequests()
	if len(sent) != 2 || sent[0] != (terminalproto.Browse{Path: "/home/test", After: "a", Filter: "si", Hidden: true}) || sent[1] != (terminalproto.Branches{Dir: "/home/test/site", Filter: "ma"}) {
		t.Fatal("requests not forwarded exactly", sent)
	}
}

func TestBrowseReasonsMapToErrors(t *testing.T) {
	f := browseFixture(t, browseCapabilities())
	for _, reason := range []terminalproto.BrowseReason{terminalproto.BrowseNotFound, terminalproto.BrowseNotDir, terminalproto.BrowseOutsideHome, terminalproto.BrowseDenied, terminalproto.BrowseUnreadable} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.BrowseResult{Reason: reason} })
		_, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{Path: "/home/test/x"})
		var refusal *BrowseError
		if !errors.As(err, &refusal) || refusal.Reason != reason {
			t.Fatal(reason, err)
		}
	}
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BrowseResult{Reason: terminalproto.BrowseConflict}
	})
	if _, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{}); !errors.Is(err, ErrBrowseBusy) {
		t.Fatal("conflict", err)
	}
	for _, reason := range []terminalproto.BranchesReason{terminalproto.BranchesNotRepoRoot, terminalproto.BranchesOutsideHome, terminalproto.BranchesGitFailed, terminalproto.BranchesDenied} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.BranchesResult{Reason: reason} })
		_, err := f.b.Branches(t.Context(), f.uid, f.agent, terminalproto.Branches{Dir: "/home/test/site"})
		var refusal *BranchesError
		if !errors.As(err, &refusal) || refusal.Reason != reason {
			t.Fatal(reason, err)
		}
	}
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BranchesResult{Reason: terminalproto.BranchesConflict}
	})
	if _, err := f.b.Branches(t.Context(), f.uid, f.agent, terminalproto.Branches{Dir: "/home/test/site"}); !errors.Is(err, ErrBranchesBusy) {
		t.Fatal("branches conflict", err)
	}
	// off says the machine has browse switched off; denied is always about
	// the folder, even from a machine whose switch went off meanwhile.
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BrowseResult{Reason: terminalproto.BrowseOff}
	})
	if _, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{}); !errors.Is(err, ErrBrowseOff) {
		t.Fatal("off", err)
	}
	off := browseCapabilities()
	off.Browse, off.BrowseOff = false, true
	record, _ := f.st.RemoteCapabilitiesFor(f.uid, f.agent)
	f.answering(func(terminalproto.Body) terminalproto.Body {
		if err := f.st.UpdateRemoteCapabilitiesFor(f.uid, f.agent, record.InstanceID, record.Generation, off); err != nil {
			t.Error(err)
		}
		return terminalproto.BrowseResult{Reason: terminalproto.BrowseDenied}
	})
	var refusal *BrowseError
	if _, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{}); !errors.As(err, &refusal) || refusal.Reason != terminalproto.BrowseDenied {
		t.Fatal("denied is about the folder", err)
	}
}

func TestBrowseOfflineMachineFailsAtOnce(t *testing.T) {
	f := browseFixture(t, browseCapabilities())
	f.peer.Close()
	start := time.Now()
	if _, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{}); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if _, err := f.b.Branches(t.Context(), f.uid, f.agent, terminalproto.Branches{Dir: "/home/test/site"}); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("offline answer took", time.Since(start))
	}
	if len(f.sentRequests()) != 0 {
		t.Fatal("sent to an offline machine")
	}
}

func TestBrowseDisconnectFailsTheRequestInFlight(t *testing.T) {
	f := browseFixture(t, browseCapabilities())
	go func() {
		for len(f.heldReplies()) == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		f.peer.Close()
	}()
	start := time.Now()
	_, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{})
	if !errors.Is(err, ErrOffline) || errors.Is(err, ErrMachineTimeout) {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("the request waited past the disconnect", time.Since(start))
	}
}

// A browse or branches answer that arrives after its request gave up is
// dropped and counted: it is not a protocol error, so the connection stays,
// and it never answers a later request.
func TestBrowseLateReplyIsDroppedAndCounted(t *testing.T) {
	ordinary, branches := BrowseCallTimeout, BranchesCallTimeout
	BrowseCallTimeout, BranchesCallTimeout = 100*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { BrowseCallTimeout, BranchesCallTimeout = ordinary, branches })
	f := browseFixture(t, browseCapabilities())
	if _, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{}); !errors.Is(err, ErrMachineTimeout) {
		t.Fatal("timeout", err)
	}
	if _, err := f.b.Branches(t.Context(), f.uid, f.agent, terminalproto.Branches{Dir: "/home/test/site"}); !errors.Is(err, ErrMachineTimeout) {
		t.Fatal("branches timeout", err)
	}
	held := f.heldReplies()
	if len(held) != 2 {
		t.Fatal("held", held)
	}
	// The next browse is held too; the late answers arrive while it waits.
	BrowseCallTimeout = 2 * time.Second
	type outcome struct {
		r   terminalproto.BrowseResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{Path: "/home/test/work"})
		done <- outcome{r, err}
	}()
	for len(f.heldReplies()) < 3 {
		time.Sleep(5 * time.Millisecond)
	}
	late := held[0]
	late.Body = homeListing()
	if err := f.deliver(late); err != nil {
		t.Fatal("a late browse answer is a protocol error", err)
	}
	lateBranches := held[1]
	lateBranches.Body = terminalproto.BranchesResult{Branches: []string{"secret"}}
	if err := f.deliver(lateBranches); err != nil {
		t.Fatal("a late branches answer is a protocol error", err)
	}
	if f.b.LateReplies() != 2 {
		t.Fatal("late replies counted", f.b.LateReplies())
	}
	select {
	case <-f.peer.Done():
		t.Fatal("a late answer ended the connection")
	default:
	}
	current := f.heldReplies()[2]
	current.Body = terminalproto.BrowseResult{Path: "/home/test/work", Home: "/home/test", Parent: "/home/test", Repo: terminalproto.BrowseRepoNone}
	if err := f.deliver(current); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err != nil || got.r.Path != "/home/test/work" || len(got.r.Entries) != 0 {
		t.Fatal("the waiting request got another request's answer", got.r, got.err)
	}
}

// A machine that answers a browse with another kind gets no answer through.
func TestBrowseAnswerOfTheWrongKindIsRefused(t *testing.T) {
	f := browseFixture(t, browseCapabilities())
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.Browse); ok {
			return terminalproto.BranchesResult{Branches: []string{"main"}}
		}
		return homeListing()
	})
	if _, err := f.b.Browse(t.Context(), f.uid, f.agent, terminalproto.Browse{}); !errors.Is(err, ErrMachineAnswer) || errors.Is(err, ErrOffline) {
		t.Fatal("a branches answer to a browse", err)
	}
	if _, err := f.b.Branches(t.Context(), f.uid, f.agent, terminalproto.Branches{Dir: "/home/test/site"}); !errors.Is(err, ErrMachineAnswer) || errors.Is(err, ErrOffline) {
		t.Fatal("a browse answer to a branches request", err)
	}
	select {
	case <-f.peer.Done():
		t.Fatal("a wrong-kind answer ended the connection")
	default:
	}
}
