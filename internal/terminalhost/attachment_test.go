//go:build linux || darwin

package terminalhost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func attachManaged(t *testing.T, m *Manager, id string, generation int64, takeover bool, lease time.Duration) (*Attachment, <-chan struct{}) {
	t.Helper()
	a, err := m.Attach(context.Background(), id, AttachOptions{ID: testRequestID(int(generation) + 100), Generation: generation, Cols: 80, Rows: 24, Takeover: takeover, Lease: lease})
	if err != nil {
		t.Fatal(err)
	}
	drained := make(chan struct{})
	ready := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		_, err := a.Read(buf)
		ready <- err
		if err == nil {
			_, _ = io.Copy(io.Discard, a)
		}
		close(drained)
	}()
	t.Cleanup(func() { a.Close() })
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal("attachment did not redraw", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("attachment did not become ready")
	}
	return a, drained
}

func TestAttachmentDisconnectTransferAndLeaseKeepShell(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 1)
	pid := managedPID(t, m, s.ID)
	first, readDone := attachManaged(t, m, s.ID, 1, false, time.Minute)
	if err := first.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Attach(context.Background(), s.ID, AttachOptions{ID: testRequestID(200), Generation: 2, Cols: 80, Rows: 24, Lease: time.Minute}); !errors.Is(err, ErrAttachmentBusy) {
		t.Fatal("implicit takeover allowed", err)
	}
	second, _ := attachManaged(t, m, s.ID, 2, true, time.Minute)
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("old client survived transfer")
	}
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("old reader leaked")
	}
	if _, err := first.Write([]byte("stale input")); err == nil {
		t.Fatal("old writer accepted input")
	}
	if err := m.RenewAttachment(s.ID, first.ID, 1, time.Minute); !errors.Is(err, ErrAttachmentStale) {
		t.Fatal("old lease revived", err)
	}
	if managedPID(t, m, s.ID) != pid {
		t.Fatal("takeover replaced shell")
	}
	second.Close()
	third, _ := attachManaged(t, m, s.ID, 3, false, time.Minute)
	if err := m.RenewAttachment(s.ID, third.ID, 3, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select {
	case <-third.Done():
	case <-time.After(time.Second):
		t.Fatal("expired lease left a client attached")
	}
	if err := m.RenewAttachment(s.ID, third.ID, 3, time.Minute); !errors.Is(err, ErrAttachmentStale) {
		t.Fatal("expired lease revived", err)
	}
	if managedPID(t, m, s.ID) != pid {
		t.Fatal("lease expiry killed shell")
	}
	m.Close()
}

func TestTerminalContentAndAmbientCredentialsNeverEnterRegistry(t *testing.T) {
	t.Setenv("REET_TOKEN", "secret-must-not-be-inherited")
	m, _ := managerFixture(t)
	s := createManaged(t, m, 1)
	a, _ := attachManaged(t, m, s.ID, 1, false, time.Minute)
	if _, err := a.Write([]byte("printf 'terminal-only-marker\\n'; printf 'credential<%s>\\n' \"$REET_TOKEN\"\r")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var snapshot []byte
	for time.Now().Before(deadline) {
		var err error
		snapshot, err = m.History(context.Background(), s.ID, 100)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(snapshot, []byte("credential<>")) || bytes.Contains(snapshot, []byte("secret-must-not-be-inherited")) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !bytes.Contains(snapshot, []byte("terminal-only-marker")) {
		t.Fatal("terminal input did not reach original shell")
	}
	if bytes.Contains(snapshot, []byte("secret-must-not-be-inherited")) {
		t.Fatal("agent credential inherited into shell")
	}
	if !bytes.Contains(snapshot, []byte("credential<>")) {
		t.Fatal("shell did not execute the credential check")
	}
	registry, err := os.ReadFile(m.registryPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range [][]byte{[]byte("terminal-only-marker"), []byte("secret-must-not-be-inherited"), []byte("printf")} {
		if bytes.Contains(registry, secret) {
			t.Fatal("terminal content stored in metadata")
		}
	}
	small, err := m.History(context.Background(), s.ID, 1)
	if err != nil || bytes.Count(small, []byte{'\n'}) > 1 {
		t.Fatal("history line bound violated", err)
	}
}

func TestAttachmentStreamsActualOutput(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 1)
	a, err := m.Attach(context.Background(), s.ID, AttachOptions{ID: testRequestID(101), Generation: 1, Cols: 80, Rows: 24, Lease: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	result := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		var output []byte
		signaled := false
		buf := make([]byte, 4096)
		for len(output) < 256<<10 {
			n, err := a.Read(buf)
			if n > 0 && !signaled {
				close(ready)
				signaled = true
			}
			output = append(output, buf[:n]...)
			if bytes.Contains(output, []byte("stage5-stream-ready")) {
				result <- nil
				return
			}
			if err != nil {
				result <- err
				return
			}
		}
		result <- ErrOutputLimit
	}()
	select {
	case <-ready:
	case err := <-result:
		t.Fatal("no initial redraw", err)
	case <-time.After(2 * time.Second):
		t.Fatal("no initial redraw")
	}
	if _, err := a.Write([]byte("printf 'stage5-%s\\n' 'stream-ready'\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal("PTY output did not stream", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no program output received")
	}
}

func TestLocalDisableClosesAttachmentButPreservesShell(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 1)
	pid := managedPID(t, m, s.ID)
	var disabled atomic.Bool
	m.consent = func() error {
		if disabled.Load() {
			return ErrTerminalDisabled
		}
		return nil
	}
	a, _ := attachManaged(t, m, s.ID, 1, false, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	disabled.Store(true)
	select {
	case <-a.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("local disable left the client attached")
	}
	if managedPID(t, m, s.ID) != pid {
		t.Fatal("local disable killed work")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer did not stop")
	}
}

func TestAttachmentLockPreventsASecondManagerTakingControl(t *testing.T) {
	m, opts := managerFixture(t)
	s := createManaged(t, m, 1)
	a, _ := attachManaged(t, m, s.ID, 1, false, time.Minute)
	second, err := NewManager(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.Attach(context.Background(), s.ID, AttachOptions{ID: testRequestID(202), Generation: 2, Cols: 80, Rows: 24, Takeover: true, Lease: time.Minute}); !errors.Is(err, ErrAttachmentBusy) {
		t.Fatal("a second manager controlled the same shell", err)
	}
	a.Close()
	other, _ := attachManaged(t, second, s.ID, 2, false, time.Minute)
	other.Close()
}
