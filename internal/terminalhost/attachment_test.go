//go:build linux || darwin

package terminalhost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
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

func TestAttachmentScrollsRetainedHistoryAfterReattach(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 1)
	ctx := context.Background()
	// Simulate a server started before mouse support was enabled.
	if _, err := m.command(ctx, 0, "set-option", "-g", "mouse", "off"); err != nil {
		t.Fatal(err)
	}
	first, _ := attachManaged(t, m, s.ID, 1, false, time.Minute)
	if _, err := first.Write([]byte("i=0; while [ $i -lt 100 ]; do printf 'retained-line-%s\\n' $i; i=$((i+1)); done\r")); err != nil {
		t.Fatal(err)
	}
	wait := func(ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if ready() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("terminal scroll state did not arrive")
	}
	wait(func() bool {
		data, _ := m.History(ctx, s.ID, 200)
		return bytes.Contains(data, []byte("retained-line-99"))
	})
	first.Close()
	second, _ := attachManaged(t, m, s.ID, 2, false, time.Minute)
	// An SGR wheel report from xterm enters tmux history, rather than recalling
	// a shell command. It uses the history kept while the browser was detached.
	if _, err := second.Write([]byte("\x1b[<64;10;5M")); err != nil {
		t.Fatal(err)
	}
	wait(func() bool {
		data, _ := m.command(ctx, 0, "display-message", "-p", "-t", s.PaneID, "#{pane_in_mode}")
		return bytes.Equal(bytes.TrimSpace(data), []byte("1"))
	})
	data, err := m.History(ctx, s.ID, 200)
	if err != nil || !bytes.Contains(data, []byte("retained-line-0\n")) {
		t.Fatalf("earlier history was lost after reattach: %v", err)
	}
	// Scrolling back to the bottom returns to live output on touch devices,
	// which may have no hardware Escape key.
	if _, err := second.Write([]byte("\x1b[<65;10;5M")); err != nil {
		t.Fatal(err)
	}
	wait(func() bool {
		data, _ := m.command(ctx, 0, "display-message", "-p", "-t", s.PaneID, "#{pane_in_mode}")
		return bytes.Equal(bytes.TrimSpace(data), []byte("0"))
	})
}

func TestAttachmentForwardsWheelToMouseAwareTUI(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 1)
	a, _ := attachManaged(t, m, s.ID, 1, false, time.Minute)
	// A minimal full-screen app reads a wheel report without echoing it as
	// prompt input. Tmux must forward it rather than entering its own history.
	app := "stty -echo -icanon; printf '\\033[?1049h\\033[?1000h\\033[?1006h'; dd bs=1 count=11 2>/dev/null | od -An -tx1; stty echo icanon\r"
	if _, err := a.Write([]byte(app)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		data, _ := m.command(context.Background(), 0, "display-message", "-p", "-t", s.PaneID, "#{mouse_standard_flag}:#{mouse_sgr_flag}")
		if bytes.Equal(bytes.TrimSpace(data), []byte("1:1")) {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("TUI did not enable mouse reporting")
	}
	if _, err := a.Write([]byte("\x1b[<64;10;5M")); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	var output []byte
	for time.Now().Before(deadline) {
		output, _ = m.History(context.Background(), s.ID, 100)
		// BSD and GNU od use different padding between byte columns.
		if strings.Contains(strings.Join(strings.Fields(string(output)), " "), "1b 5b 3c 36 34 3b 31 30 3b 35 4d") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("wheel report did not reach the TUI; synthetic screen: %q", output)
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
