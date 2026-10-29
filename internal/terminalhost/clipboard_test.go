//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

func clipboardOption(t *testing.T, m *Manager) string {
	t.Helper()
	out, err := m.command(context.Background(), 0, "show-options", "-gv", "set-clipboard")
	if err != nil {
		t.Fatal("read set-clipboard", err)
	}
	return strings.TrimSpace(string(out))
}

// The manager makes the tmux server's clipboard option follow the local
// switch: on by default, off when the consent refuses, and flipped on the
// next reconcile when the switch changes while sessions run.
func TestManagerAppliesClipboardSwitch(t *testing.T) {
	m, opts := managerFixture(t)
	before := clipboardOption(t, m)
	t.Cleanup(func() {
		if _, err := m.command(context.Background(), 0, "set-option", "-g", "set-clipboard", before); err != nil {
			t.Error("restore set-clipboard", err)
		}
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := clipboardOption(t, m); got != "on" {
		t.Fatalf("default clipboard option %q", got)
	}

	var allow atomic.Bool
	offOpts := opts
	offOpts.ClipboardConsent = func() error {
		if allow.Load() {
			return nil
		}
		return errors.New("clipboard disabled locally")
	}
	off, err := NewManager(context.Background(), offOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer off.Close()
	if err := off.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := clipboardOption(t, off); got != "off" {
		t.Fatalf("refused clipboard option %q", got)
	}

	allow.Store(true)
	if err := off.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := clipboardOption(t, off); got != "on" {
		t.Fatalf("reconciled clipboard option %q", got)
	}
}
