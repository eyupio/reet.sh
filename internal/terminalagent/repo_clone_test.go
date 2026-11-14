//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const cloneFeatures = allFeatures + "," + terminalproto.BaseFreshFeature + "," + terminalproto.RepoCloneFeature

// heldClone replaces the connector's clone for one test: each call records
// what it was given, says it started, and waits for release (or its
// context) before answering result.
type heldClone struct {
	mu      sync.Mutex
	homes   []string
	reqs    []terminalproto.RepoClone
	started chan struct{}
	release chan struct{}
	result  terminalproto.RepoCloneResult
	open    func()
}

func holdClone(t *testing.T, result terminalproto.RepoCloneResult) *heldClone {
	h := &heldClone{started: make(chan struct{}, 8), release: make(chan struct{}), result: result}
	original := cloneRepo
	cloneRepo = func(ctx context.Context, home string, req terminalproto.RepoClone, git terminalhost.GitRunner, gh terminalhost.GHRunner) terminalproto.RepoCloneResult {
		h.mu.Lock()
		h.homes = append(h.homes, home)
		h.reqs = append(h.reqs, req)
		h.mu.Unlock()
		if git == nil || gh == nil {
			t.Error("clone given no git or no gh")
		}
		h.started <- struct{}{}
		select {
		case <-h.release:
		case <-ctx.Done():
		}
		return h.result
	}
	var once sync.Once
	t.Cleanup(func() { cloneRepo = original; once.Do(func() { close(h.release) }) })
	h.open = func() { once.Do(func() { close(h.release) }) }
	return h
}

func (h *heldClone) calls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.reqs)
}

func waitClone(t *testing.T, h *heldClone) {
	t.Helper()
	select {
	case <-h.started:
	case <-time.After(10 * time.Second):
		t.Fatal("clone never started")
	}
}

// A connection that did not negotiate repo-clone-v1 never reaches the clone.
func TestRepoCloneRequiresTheNegotiatedFeature(t *testing.T) {
	h := holdClone(t, terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed})
	handle, _, _ := browseConnector(t, allFeatures+","+terminalproto.BaseFreshFeature, nil)
	err := handle(t.Context(), message("clone-unasked-00001", &terminalproto.RepoClone{Owner: "o", Repo: "r"}))
	if !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal(err)
	}
	if h.calls() != 0 {
		t.Fatal("cloned")
	}
}

// With terminal access off the clone is refused as denied, as a base check is.
func TestRepoCloneRequiresTerminalAccess(t *testing.T) {
	h := holdClone(t, terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed})
	for _, consent := range []func() error{nil, func() error { return errors.New("off") }} {
		handle, got, _ := browseConnector(t, cloneFeatures, func(o *terminalhost.ManagerOptions) { o.Consent = consent })
		if err := handle(t.Context(), message("clone-denied-00001", &terminalproto.RepoClone{Owner: "o", Repo: "r"})); err != nil {
			t.Fatal(err)
		}
		reply := next(t, got, "clone-denied-00001")
		if reply.Body != (terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneDenied}) {
			t.Fatal(reply.Body)
		}
	}
	if h.calls() != 0 {
		t.Fatal("cloned")
	}
}

// A clone is answered with what the machine did, for the connector's home.
func TestRepoCloneAnswersTheResult(t *testing.T) {
	handle, got, home := browseConnector(t, cloneFeatures, nil)
	want := terminalproto.RepoCloneResult{Dir: filepath.Join(home, "reet", "repos", "o", "r"), DefaultBranch: "main", Marked: true}
	h := holdClone(t, want)
	h.open()
	if err := handle(t.Context(), message("clone-request-00001", &terminalproto.RepoClone{Owner: "o", Repo: "r"})); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-request-00001"); reply.Body != want {
		t.Fatal(reply.Body)
	}
	if h.homes[0] != home || h.reqs[0] != (terminalproto.RepoClone{Owner: "o", Repo: "r"}) {
		t.Fatal(h.homes, h.reqs)
	}
}

// One clone runs at a time: a second is answered busy at once. The clone
// has a slot of its own, so a base check (which shares its slot with prune
// and sync) still runs beside it, and a clone runs beside a held base check.
func TestRepoCloneHasASlotOfItsOwn(t *testing.T) {
	h := holdClone(t, terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed})
	hold := make(chan struct{})
	var holdOnce sync.Once
	t.Cleanup(func() { holdOnce.Do(func() { close(hold) }) })
	gitStarted := make(chan struct{}, 64)
	blocking := false
	var mu sync.Mutex
	held := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		mu.Lock()
		block := blocking
		mu.Unlock()
		if block {
			gitStarted <- struct{}{}
			select {
			case <-hold:
			case <-ctx.Done():
			}
		}
		return cannedBranches(nil)(ctx, dir, args...)
	}
	handle, got, home := browseConnector(t, cloneFeatures, func(o *terminalhost.ManagerOptions) { o.Git = held })
	check := &terminalproto.BaseCheck{Dir: filepath.Join(home, "site"), Base: "main"}

	if err := handle(t.Context(), message("clone-request-00001", &terminalproto.RepoClone{Owner: "o", Repo: "r"})); err != nil {
		t.Fatal(err)
	}
	waitClone(t, h)
	if err := handle(t.Context(), message("clone-request-00002", &terminalproto.RepoClone{Owner: "o", Repo: "s"})); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-request-00002"); reply.Body != (terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneBusy}) {
		t.Fatal(reply.Body)
	}
	if err := handle(t.Context(), message("base-check-request-1", check)); err != nil {
		t.Fatal(err)
	}
	reply := next(t, got, "base-check-request-1")
	if r, ok := reply.Body.(terminalproto.BaseCheckResult); !ok || r.Reason == terminalproto.BranchesConflict {
		t.Fatal("base check busy beside a clone", reply.Body)
	}
	h.open()
	if reply := next(t, got, "clone-request-00001"); reply.Body != (terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed}) {
		t.Fatal(reply.Body)
	}
	if h.calls() != 1 {
		t.Fatal("the busy clone ran", h.calls())
	}

	// Now hold a base check in the shared slot and clone beside it.
	mu.Lock()
	blocking = true
	mu.Unlock()
	if err := handle(t.Context(), message("base-check-request-2", check)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gitStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("base check never ran git")
	}
	if err := handle(t.Context(), message("clone-request-00003", &terminalproto.RepoClone{Owner: "o", Repo: "r"})); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-request-00003"); reply.Body != (terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed}) {
		t.Fatal("clone beside a base check", reply.Body)
	}
	holdOnce.Do(func() { close(hold) })
	next(t, got, "base-check-request-2")
}

// The connector answers a clone that has not returned after the clone's own
// 10-minute bound and before the controller's call gives up, so a stuck
// clone's answer never races the controller.
func TestRepoCloneAnswerBoundSitsBetweenTheClonesAndTheControllers(t *testing.T) {
	if repoCloneAnswerAfter != 10*time.Minute+15*time.Second {
		t.Fatalf("repoCloneAnswerAfter = %v, want 10m15s", repoCloneAnswerAfter)
	}
	if repoCloneAnswerAfter >= terminalbroker.RepoCloneCallTimeout {
		t.Fatalf("answer %v is not under the controller's %v", repoCloneAnswerAfter, terminalbroker.RepoCloneCallTimeout)
	}
}
