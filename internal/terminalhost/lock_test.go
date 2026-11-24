//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Taking the machine lock cuts every attachment within a second, keeps the
// shell running, and once lifted the session can be attached again.
func TestTheLockClosesAttachmentsKeepsTheShellAndAllowsReattachAfterUnlock(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 1)
	pid := managedPID(t, m, s.ID)
	var locked atomic.Bool
	m.locked = locked.Load
	a, _ := attachManaged(t, m, s.ID, 1, false, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	locked.Store(true)
	select {
	case <-a.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the lock left the client attached")
	}
	if managedPID(t, m, s.ID) != pid {
		t.Fatal("the lock killed work")
	}
	locked.Store(false)
	again, _ := attachManaged(t, m, s.ID, 2, false, time.Minute)
	time.Sleep(1500 * time.Millisecond)
	select {
	case <-again.Done():
		t.Fatal("an attachment after unlock was cut")
	default:
	}
	again.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer did not stop")
	}
}

// While the machine is locked, or terminal access is off, no screen is read
// for a question, the question already listed goes (so the controller is
// told there is none), and none comes back until the lock is lifted.
func TestNoQuestionIsReadOrListedWhileLockedOrDisabled(t *testing.T) {
	for _, why := range []string{"locked", "terminal access off"} {
		m := promptFixture(t)
		var held atomic.Bool
		if why == "locked" {
			m.locked = held.Load
		} else {
			m.consent = func() error {
				if held.Load() {
					return ErrTerminalDisabled
				}
				return nil
			}
		}
		n := 60
		if why != "locked" {
			n = 61
		}
		id, tty := agentSession(t, m, n, "claude")
		draw(t, m, id, tty, claudeCommandPrompt, "Do you want to proceed?")
		if q := questionOf(t, m, id); q == nil {
			t.Fatalf("%s: the question was not read before", why)
		}
		changed := m.Changed()
		held.Store(true)
		if q := questionOf(t, m, id); q != nil {
			t.Fatalf("%s: a question was still listed: %+v", why, q)
		}
		select {
		case <-changed:
		default:
			t.Fatalf("%s: the question going was not signalled", why)
		}
		read := m.prompts[id].at
		draw(t, m, id, tty, claudeWorking, "Running tests")
		draw(t, m, id, tty, claudeCommandPrompt, "Do you want to proceed?")
		if q := questionOf(t, m, id); q != nil || !m.prompts[id].at.Equal(read) {
			t.Fatalf("%s: the screen was read: %+v", why, q)
		}
		held.Store(false)
		if q := questionOf(t, m, id); q == nil {
			t.Fatalf("%s: the question did not come back", why)
		}
	}
}

// An attachment is cut within a second of the lock even while a reconcile
// holds the manager (one can take seconds), and a locked manager refuses
// the next step of anything that would start, attach, type or read.
func TestTheLockCutsAnAttachmentWithoutWaitingForAReconcile(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 3)
	var locked atomic.Bool
	m.locked = locked.Load
	a, _ := attachManaged(t, m, s.ID, 1, false, time.Minute)
	m.mu.Lock() // as a slow reconcile would
	locked.Store(true)
	select {
	case <-a.Done():
	case <-time.After(time.Second):
		m.mu.Unlock()
		t.Fatal("the lock left the client attached behind a busy manager")
	}
	m.mu.Unlock()
	ctx := context.Background()
	if _, err := m.Attach(ctx, s.ID, AttachOptions{ID: testRequestID(301), Generation: 9, Cols: 80, Rows: 24, Takeover: true, Lease: time.Minute}); !errors.Is(err, ErrLocked) {
		t.Fatalf("attach while locked: %v", err)
	}
	if _, err := m.Create(ctx, testRequestID(302), testSessionID(302), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}); !errors.Is(err, ErrLocked) {
		t.Fatalf("create while locked: %v", err)
	}
	if _, err := m.History(ctx, s.ID, 10); !errors.Is(err, ErrLocked) {
		t.Fatalf("history while locked: %v", err)
	}
	if !errors.Is(ErrLocked, ErrTerminalDisabled) {
		t.Fatal("an older controller would not read the lock as a refusal")
	}
}

// An upload already streaming when the lock is taken stops at its next read.
func TestTheLockStopsAnUploadMidStream(t *testing.T) {
	var locked atomic.Bool
	m := &Manager{locked: locked.Load}
	r := m.whileUnlocked(strings.NewReader(strings.Repeat("x", 100)))
	buf := make([]byte, 10)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	locked.Store(true)
	if _, err := r.Read(buf); !errors.Is(err, ErrLocked) {
		t.Fatalf("read after the lock: %v", err)
	}
}

// A session's live folder is withheld while the machine is locked: it says
// where someone is working, not whether the session runs.
func TestALockedMachineListsNoLiveFolder(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 4)
	var locked atomic.Bool
	m.locked = locked.Load
	m.mu.Lock()
	m.liveDir[s.ID] = "/home/someone/secret-project"
	m.mu.Unlock()
	listed := func() string {
		list, err := m.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range list {
			if got.ID == s.ID {
				return got.LiveDir
			}
		}
		t.Fatal("session not listed")
		return ""
	}
	if listed() == "" {
		t.Fatal("the live folder was not listed before the lock")
	}
	locked.Store(true)
	if dir := listed(); dir != "" {
		t.Fatalf("a locked machine listed %q", dir)
	}
}

// Ending a desk shell still works on a locked machine, but its history file
// is kept: a locked machine deletes nothing for its controller.
func TestALockedMachineEndsADeskShellButKeepsItsHistory(t *testing.T) {
	m, home := deskShellFixture(t, "bash", nil)
	s := createDeskShell(t, m, 1, deskPaneA)
	typeInShell(t, m, s.ID, "echo reet-kept")
	file := deskHistoryFile(home, m.shell, deskPaneA)
	waitFileHas(t, file, "echo reet-kept")
	m.locked = func() bool { return true }
	if _, err := m.End(context.Background(), testRequestID(9), s.ID); err != nil {
		t.Fatalf("ending on a locked machine: %v", err)
	}
	if err := m.ForgetDeskSession(context.Background(), s.ID); !errors.Is(err, ErrLocked) {
		t.Fatalf("forget on a locked machine: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("a locked machine deleted the history: %v", err)
	}
}

// reet agent lock --end-sessions ends every session the manager runs, as the
// controller's end does, and counts them; Running counts what is left.
func TestEndAllEndsEveryRunningSession(t *testing.T) {
	m, _ := managerFixture(t)
	createManaged(t, m, 5)
	createManaged(t, m, 6)
	ctx := context.Background()
	if n, err := m.Running(ctx); err != nil || n != 2 {
		t.Fatalf("running: %d %v", n, err)
	}
	m.locked = func() bool { return true }
	if n, err := m.EndAll(ctx); err != nil || n != 2 {
		t.Fatalf("ended: %d %v", n, err)
	}
	if n, err := m.Running(ctx); err != nil || n != 0 {
		t.Fatalf("still running: %d %v", n, err)
	}
}
