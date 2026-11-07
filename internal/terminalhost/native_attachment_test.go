//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Native clients use the common lease/generation/PTY lifecycle but do not
// require a tmux server. Exercise this path even on hosts without tmux.
func TestNativeAttachmentLeaseTakeoverAndDetach(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := exec.CommandContext(ctx, "/bin/sleep", "30")
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = worker.Process.Kill(); _ = worker.Wait() }()
	var disabled atomic.Bool
	layout := privateLayout(t)
	manager := func() *Manager {
		return &Manager{enrollment: testEnrollment, layout: layout, home: t.TempDir(), attachments: map[string]*Attachment{}, consent: func() error {
			if disabled.Load() {
				return errors.New("disabled")
			}
			return nil
		}}
	}
	m := manager()
	native := NativeCommand{Binary: "/bin/sh", Dir: t.TempDir(), Args: []string{"-c", "printf 'native-client-ready\\n'; while IFS= read -r line; do printf '%s\\n' \"$line\"; done"}, Env: []string{"PATH=/usr/bin:/bin"}}
	id := testSessionID(1)
	attach := func(generation int64, takeover bool, lease time.Duration) (*Attachment, error) {
		return m.AttachNative(ctx, id, AttachOptions{ID: testRequestID(int(generation)), Generation: generation, Cols: 80, Rows: 24, Takeover: takeover, Lease: lease}, native)
	}
	read := func(a *Attachment) {
		t.Helper()
		ready := make(chan error, 1)
		go func() {
			buf := make([]byte, 1024)
			_, err := a.Read(buf)
			ready <- err
			if err == nil {
				_, _ = io.Copy(io.Discard, a)
			}
		}()
		select {
		case err := <-ready:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("native client did not redraw")
		}
	}
	first, err := attach(1, false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	read(first)
	if _, err := attach(2, false, time.Second); !errors.Is(err, ErrAttachmentBusy) {
		t.Fatalf("second controller: %v", err)
	}
	if _, err := attach(1, true, time.Second); !errors.Is(err, ErrAttachmentStale) {
		t.Fatalf("stale generation: %v", err)
	}
	other := manager()
	if _, err := other.AttachNative(ctx, id, AttachOptions{ID: testRequestID(2), Generation: 2, Cols: 80, Rows: 24, Takeover: true, Lease: time.Second}, native); !errors.Is(err, ErrAttachmentBusy) {
		t.Fatalf("other manager bypassed attachment lock: %v", err)
	}
	if err := m.RenewAttachment(id, testRequestID(1), 2, time.Second); !errors.Is(err, ErrAttachmentStale) {
		t.Fatalf("stale renewal: %v", err)
	}
	if err := m.RenewAttachment(id, testRequestID(1), 1, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if !first.live() {
		t.Fatal("renewed lease expired at the old deadline")
	}
	second, err := attach(2, true, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	read(second)
	select {
	case <-first.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("takeover left old native client alive")
	}
	if _, err := first.Write([]byte("old controller")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("old input accepted: %v", err)
	}
	if err := m.RenewAttachment(id, testRequestID(1), 1, time.Second); !errors.Is(err, ErrAttachmentStale) {
		t.Fatalf("taken-over lease revived: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := worker.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("detaching client killed worker", err)
	}
	third, err := attach(3, false, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	read(third)
	select {
	case <-third.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("expired lease retained native client")
	}
	if err := m.RenewAttachment(id, testRequestID(3), 3, time.Second); !errors.Is(err, ErrAttachmentStale) {
		t.Fatalf("expired lease revived: %v", err)
	}
	if err := worker.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("lease expiry killed worker", err)
	}
	disabled.Store(true)
	if _, err := attach(4, false, time.Second); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatalf("disabled host attached native client: %v", err)
	}
}

func TestNativeAttachmentClosesDisposableClientChildren(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "client-child-outlived-detach")
	m := &Manager{enrollment: testEnrollment, layout: privateLayout(t), home: dir, attachments: map[string]*Attachment{}, consent: func() error { return nil }}
	native := NativeCommand{Binary: "/bin/sh", Dir: dir, Args: []string{"-c", `trap '' HUP; (sleep 0.5; printf leaked > "$1") & printf ready; wait`, "native-test", marker}, Env: []string{"PATH=/usr/bin:/bin"}}
	a, err := m.AttachNative(ctx, testSessionID(1), AttachOptions{ID: testRequestID(1), Generation: 1, Cols: 80, Rows: 24, Lease: time.Second}, native)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ready := make(chan error, 1)
	go func() { buf := make([]byte, 100); _, err := a.Read(buf); ready <- err }()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client did not start")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(800 * time.Millisecond)
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disposable client child survived detach: %v", err)
	}
}
