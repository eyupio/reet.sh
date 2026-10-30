//go:build linux || darwin

package terminalhost

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func paneTTY(t *testing.T, m *Manager, sessionID string) string {
	t.Helper()
	out, err := m.command(context.Background(), 1024, "list-panes", "-t", "="+m.name(sessionID), "-F", "#{pane_tty}")
	if err != nil {
		t.Fatal(err)
	}
	tty := strings.TrimSpace(string(out))
	if tty == "" {
		t.Fatal("empty pane_tty")
	}
	return tty
}

func TestReadAttentionActivityAndBell(t *testing.T) {
	m, opts := managerFixture(t)
	ctx := context.Background()
	sid := testSessionID(41)
	if _, err := m.Create(ctx, testRequestID(41), sid, terminalproto.Create{Title: "Attention", Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	tty := paneTTY(t, m, sid)

	act1, bell1, err := ReadAttention(ctx, opts.Layout.Socket, m.name(sid))
	if err != nil {
		t.Fatal(err)
	}
	if act1 <= 0 || bell1 != 0 {
		t.Fatalf("initial attention = (%d, %d), want activity > 0 and bell == 0", act1, bell1)
	}

	// tmux window_activity has 1-second resolution; cross a second boundary
	// before writing output so activityAt advances deterministically.
	time.Sleep(1100 * time.Millisecond)
	if err := os.WriteFile(tty, []byte("working output\r\n"), 0); err != nil {
		t.Fatal(err)
	}
	var act2 int64
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		act2, _, err = ReadAttention(ctx, opts.Layout.Socket, m.name(sid))
		if err != nil {
			t.Fatal(err)
		}
		if act2 > act1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if act2 <= act1 {
		t.Fatalf("output did not advance activityAt: before=%d after=%d", act1, act2)
	}

	// Ring the terminal bell (`printf '\a'`) in the pane.
	if err := os.WriteFile(tty, []byte("\a"), 0); err != nil {
		t.Fatal(err)
	}
	var bellAt int64
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, bellAt, err = ReadAttention(ctx, opts.Layout.Socket, m.name(sid))
		if err != nil {
			t.Fatal(err)
		}
		if bellAt > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if bellAt <= 0 {
		t.Fatal("bell did not set bellAt")
	}

	// The flag was cleared when first seen, so an immediate read without a new
	// bell reports bellAt == 0.
	_, clearedBell, err := ReadAttention(ctx, opts.Layout.Socket, m.name(sid))
	if err != nil {
		t.Fatal(err)
	}
	if clearedBell != 0 {
		t.Fatalf("expected cleared bell flag, got %d", clearedBell)
	}

	// A second bell sets the flag again and Manager.Reconcile records it on the
	// managed session.
	if err := os.WriteFile(tty, []byte("\a"), 0); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	var recorded ManagedSession
	for time.Now().Before(deadline) {
		if err := m.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		list, err := m.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if s.ID == sid {
				recorded = s
			}
		}
		if recorded.BellAt > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if recorded.BellAt <= 0 || recorded.ActivityAt < act2 {
		t.Fatalf("manager did not persist attention: %+v", recorded)
	}
}

func TestAttentionCarriesNoOutput(t *testing.T) {
	m, _ := managerFixture(t)
	ctx := context.Background()
	sid := testSessionID(42)
	if _, err := m.Create(ctx, testRequestID(42), sid, terminalproto.Create{Title: "Quiet", Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	tty := paneTTY(t, m, sid)
	secret := "SECRET_PANE_TEXT_DO_NOT_LEAK_98765"
	if err := os.WriteFile(tty, []byte(secret+"\a\r\n"), 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := m.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := m.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatal(err, list)
	}
	s := list[0]
	snap := terminalproto.SessionSnapshot{
		ID:         s.ID,
		Title:      s.Title,
		State:      s.State,
		ExitCode:   s.ExitCode,
		ActivityAt: s.ActivityAt,
		BellAt:     s.BellAt,
		ExitStatus: s.ExitCode,
	}.ForFeatures(terminalproto.AttentionFeature)
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("snapshot leaked pane output: %s", raw)
	}
}
