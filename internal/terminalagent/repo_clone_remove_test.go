//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	cloneRemoveFeatures = cloneFeatures + "," + terminalproto.RepoCloneRemoveFeature
	shownFingerprint    = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

// heldRemove replaces the connector's clone status and remove for one test:
// each call records what it was given; a remove says it started and waits
// for release before answering. A remove does not end with its context, as
// a delete that has started does not.
type heldRemove struct {
	mu       sync.Mutex
	homes    []string
	statuses []terminalproto.RepoCloneStatus
	removes  []terminalproto.RepoCloneRemove
	started  chan struct{}
	release  chan struct{}
	state    terminalproto.RepoCloneState
	removed  terminalproto.RepoCloneRemoved
	open     func()
}

func holdRemove(t *testing.T) *heldRemove {
	h := &heldRemove{started: make(chan struct{}, 8), release: make(chan struct{}),
		state:   terminalproto.RepoCloneState{Marked: true, Fingerprint: shownFingerprint},
		removed: terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneChanged}}
	originalStatus, originalRemove := cloneStatus, removeClone
	cloneStatus = func(_ context.Context, home string, req terminalproto.RepoCloneStatus, git terminalhost.GitRunner) terminalproto.RepoCloneState {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.homes = append(h.homes, home)
		h.statuses = append(h.statuses, req)
		if git == nil {
			t.Error("status given no git")
		}
		return h.state
	}
	removeClone = func(_ context.Context, home string, req terminalproto.RepoCloneRemove, git terminalhost.GitRunner) terminalproto.RepoCloneRemoved {
		h.mu.Lock()
		h.homes = append(h.homes, home)
		h.removes = append(h.removes, req)
		h.mu.Unlock()
		if git == nil {
			t.Error("remove given no git")
		}
		h.started <- struct{}{}
		<-h.release
		return h.removed
	}
	var once sync.Once
	h.open = func() { once.Do(func() { close(h.release) }) }
	t.Cleanup(func() { h.open(); cloneStatus, removeClone = originalStatus, originalRemove })
	return h
}

func (h *heldRemove) calls() (statuses, removes int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.statuses), len(h.removes)
}

func waitRemove(t *testing.T, h *heldRemove) {
	t.Helper()
	select {
	case <-h.started:
	case <-time.After(10 * time.Second):
		t.Fatal("remove never started")
	}
}

func cloneStatusRequest(home string) *terminalproto.RepoCloneStatus {
	return &terminalproto.RepoCloneStatus{Dir: filepath.Join(home, "reet", "repos", "o", "r"), Origin: "o/r"}
}

func cloneRemoveRequest(home string) *terminalproto.RepoCloneRemove {
	return &terminalproto.RepoCloneRemove{Dir: filepath.Join(home, "reet", "repos", "o", "r"), Origin: "o/r", Fingerprint: shownFingerprint}
}

// A connection that negotiated cloning but not repo-clone-remove-v1 never
// reaches the status or the remove.
func TestRepoCloneRemoveRequiresTheNegotiatedFeature(t *testing.T) {
	h := holdRemove(t)
	h.open()
	handle, _, home := browseConnector(t, cloneFeatures, nil)
	for id, body := range map[string]terminalproto.Body{"clone-status-unasked1": cloneStatusRequest(home), "clone-remove-unasked1": cloneRemoveRequest(home)} {
		if err := handle(t.Context(), message(id, body)); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(id, err)
		}
	}
	if statuses, removes := h.calls(); statuses != 0 || removes != 0 {
		t.Fatal("ran without the feature", statuses, removes)
	}
}

// With terminal access off both are refused as denied, as a clone is.
func TestRepoCloneRemoveRequiresTerminalAccess(t *testing.T) {
	h := holdRemove(t)
	h.open()
	for _, consent := range []func() error{nil, func() error { return errors.New("off") }} {
		handle, got, home := browseConnector(t, cloneRemoveFeatures, func(o *terminalhost.ManagerOptions) { o.Consent = consent })
		if err := handle(t.Context(), message("clone-status-denied1", cloneStatusRequest(home))); err != nil {
			t.Fatal(err)
		}
		if reply := next(t, got, "clone-status-denied1"); reply.Body != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneDenied}) {
			t.Fatal(reply.Body)
		}
		if err := handle(t.Context(), message("clone-remove-denied1", cloneRemoveRequest(home))); err != nil {
			t.Fatal(err)
		}
		if reply := next(t, got, "clone-remove-denied1"); reply.Body != (terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneDenied}) {
			t.Fatal(reply.Body)
		}
	}
	if statuses, removes := h.calls(); statuses != 0 || removes != 0 {
		t.Fatal("ran without terminal access", statuses, removes)
	}
}

// Each is answered with what the machine said, for the connector's home.
func TestRepoCloneRemoveAnswersTheResult(t *testing.T) {
	h := holdRemove(t)
	h.open()
	handle, got, home := browseConnector(t, cloneRemoveFeatures, nil)
	h.removed = terminalproto.RepoCloneRemoved{Dir: filepath.Join(home, "reet", "repos", "o", "r")}
	if err := handle(t.Context(), message("clone-status-request", cloneStatusRequest(home))); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-status-request"); reply.Body != h.state {
		t.Fatal(reply.Body)
	}
	if err := handle(t.Context(), message("clone-remove-request", cloneRemoveRequest(home))); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-remove-request"); reply.Body != h.removed {
		t.Fatal(reply.Body)
	}
	if len(h.homes) != 2 || h.homes[0] != home || h.homes[1] != home || h.statuses[0] != *cloneStatusRequest(home) || h.removes[0] != *cloneRemoveRequest(home) {
		t.Fatal(h.homes, h.statuses, h.removes)
	}
}

// A clone and a remove never run together: each is answered busy while the
// other holds the clone slot. A status only reads, so it runs beside both.
func TestRepoCloneRemoveSharesTheCloneSlot(t *testing.T) {
	clone := holdClone(t, terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed})
	remove := holdRemove(t)
	handle, got, home := browseConnector(t, cloneRemoveFeatures, nil)

	if err := handle(t.Context(), message("clone-request-00001", &terminalproto.RepoClone{Owner: "o", Repo: "r"})); err != nil {
		t.Fatal(err)
	}
	waitClone(t, clone)
	if err := handle(t.Context(), message("clone-remove-00001", cloneRemoveRequest(home))); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-remove-00001"); reply.Body != (terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneBusy}) {
		t.Fatal("remove beside a clone", reply.Body)
	}
	if err := handle(t.Context(), message("clone-status-00001", cloneStatusRequest(home))); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-status-00001"); reply.Body != remove.state {
		t.Fatal("status beside a clone", reply.Body)
	}
	clone.open()
	next(t, got, "clone-request-00001")
	if _, removes := remove.calls(); removes != 0 {
		t.Fatal("the busy remove ran")
	}

	if err := handle(t.Context(), message("clone-remove-00002", cloneRemoveRequest(home))); err != nil {
		t.Fatal(err)
	}
	waitRemove(t, remove)
	if err := handle(t.Context(), message("clone-request-00002", &terminalproto.RepoClone{Owner: "o", Repo: "r"})); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-request-00002"); reply.Body != (terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneBusy}) {
		t.Fatal("clone beside a remove", reply.Body)
	}
	if err := handle(t.Context(), message("clone-remove-00003", cloneRemoveRequest(home))); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-remove-00003"); reply.Body != (terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneBusy}) {
		t.Fatal("a second remove", reply.Body)
	}
	if err := handle(t.Context(), message("clone-status-00002", cloneStatusRequest(home))); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-status-00002"); reply.Body != remove.state {
		t.Fatal("status beside a remove", reply.Body)
	}
	remove.open()
	if reply := next(t, got, "clone-remove-00002"); reply.Body != remove.removed {
		t.Fatal(reply.Body)
	}
	if calls := clone.calls(); calls != 1 {
		t.Fatal("the busy clone ran", calls)
	}
}

// A delete cannot be stopped, so a remove that outlasts its answer keeps the
// clone slot until it really ends: no clone starts beside a delete.
func TestRepoCloneRemoveKeepsTheSlotUntilTheDeleteEnds(t *testing.T) {
	original := repoCloneRemoveAnswerAfter
	repoCloneRemoveAnswerAfter = 50 * time.Millisecond
	t.Cleanup(func() { repoCloneRemoveAnswerAfter = original })
	clone := holdClone(t, terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed})
	clone.open()
	remove := holdRemove(t)
	handle, got, home := browseConnector(t, cloneRemoveFeatures, nil)

	if err := handle(t.Context(), message("clone-remove-00001", cloneRemoveRequest(home))); err != nil {
		t.Fatal(err)
	}
	waitRemove(t, remove)
	if reply := next(t, got, "clone-remove-00001"); reply.Body != (terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneTimedOut}) {
		t.Fatal("a remove that outlasted its answer", reply.Body)
	}
	if err := handle(t.Context(), message("clone-request-00001", &terminalproto.RepoClone{Owner: "o", Repo: "r"})); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-request-00001"); reply.Body != (terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneBusy}) {
		t.Fatal("a clone started beside a delete", reply.Body)
	}
	remove.open()
	// The slot is freed when the delete returns; the next clone runs.
	deadline := time.Now().Add(10 * time.Second)
	for id := 2; ; id++ {
		request := "clone-request-0000" + string(rune('0'+id))
		if err := handle(t.Context(), message(request, &terminalproto.RepoClone{Owner: "o", Repo: "r"})); err != nil {
			t.Fatal(err)
		}
		if reply := next(t, got, request); reply.Body == (terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed}) {
			break
		}
		if id == 9 || time.Now().After(deadline) {
			t.Fatal("the slot was never freed")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// The connector answers after the machine's own bound and before the
// controller's call gives up.
func TestRepoCloneRemoveAnswerBoundsSitUnderTheControllers(t *testing.T) {
	if repoCloneStatusAnswerAfter != 12*time.Second || repoCloneStatusAnswerAfter >= terminalbroker.RepoCloneStatusCallTimeout {
		t.Fatalf("status answer %v, controller %v", repoCloneStatusAnswerAfter, terminalbroker.RepoCloneStatusCallTimeout)
	}
	if repoCloneRemoveAnswerAfter != 55*time.Second || repoCloneRemoveAnswerAfter >= terminalbroker.RepoCloneRemoveCallTimeout {
		t.Fatalf("remove answer %v, controller %v", repoCloneRemoveAnswerAfter, terminalbroker.RepoCloneRemoveCallTimeout)
	}
}

// The whole path on the machine, with real git: a clone reet marked is
// reported removable, a remove with another fingerprint keeps it, and a
// remove with the shown one deletes it and its empty owner folder.
func TestRepoCloneRemoveEndToEndWithRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	handle, got, home := browseConnector(t, cloneRemoveFeatures, func(o *terminalhost.ManagerOptions) { o.Git = nil })
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "reet", "repos", "o", "r")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "first"},
		{"config", "remote.origin.url", "https://github.com/o/r.git"},
		{"update-ref", "refs/remotes/origin/main", "HEAD"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %q: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "reet-clone"), []byte("o/r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := &terminalproto.RepoCloneStatus{Dir: dir, Origin: "o/r"}
	if err := handle(t.Context(), message("clone-status-request", status)); err != nil {
		t.Fatal(err)
	}
	state, ok := next(t, got, "clone-status-request").Body.(terminalproto.RepoCloneState)
	if !ok || !state.Removable() {
		t.Fatalf("state = %+v", state)
	}
	stale := &terminalproto.RepoCloneRemove{Dir: dir, Origin: "o/r", Fingerprint: shownFingerprint}
	if err := handle(t.Context(), message("clone-remove-stale-01", stale)); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-remove-stale-01"); reply.Body != (terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneChanged}) {
		t.Fatal(reply.Body)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "reet-clone")); err != nil {
		t.Fatal("a refused remove touched the clone:", err)
	}
	remove := &terminalproto.RepoCloneRemove{Dir: dir, Origin: "o/r", Fingerprint: state.Fingerprint}
	if err := handle(t.Context(), message("clone-remove-request", remove)); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "clone-remove-request"); reply.Body != (terminalproto.RepoCloneRemoved{Dir: dir}) {
		t.Fatal(reply.Body)
	}
	if _, err := os.Lstat(filepath.Dir(dir)); !os.IsNotExist(err) {
		t.Fatal("the owner folder is still there:", err)
	}
	if _, err := os.Stat(filepath.Join(home, "reet", "repos")); err != nil {
		t.Fatal("reet/repos is gone:", err)
	}
}
