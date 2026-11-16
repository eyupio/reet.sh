package terminalagent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// The live folder is sent only when it differs from the last one sent for that
// session on this connection, and never on a connection without desk-shell-v1.
func TestLiveDirOnlySentOnChange(t *testing.T) {
	features := terminalproto.AttentionFeature + "," + terminalproto.DeskShellFeature
	observed := map[string]terminalproto.SessionChanged{}
	sent := map[string]string{}
	s := terminalhost.ManagedSession{ID: "session-0123456789", State: terminalproto.Running, LiveDir: "/home/me"}

	first, send := observeShell(observed, sent, s, features)
	if !send || first.Dir != "/home/me" {
		t.Fatalf("first observation: %+v %v", first, send)
	}
	again, send := observeShell(observed, sent, s, features)
	if send || again.Dir != "" {
		t.Fatalf("an unchanged folder was sent again: %+v %v", again, send)
	}
	s.LiveDir = "/home/me/project"
	moved, send := observeShell(observed, sent, s, features)
	if !send || moved.Dir != "/home/me/project" {
		t.Fatalf("a folder change was not sent: %+v %v", moved, send)
	}
	// A state change with the same folder goes out without the folder.
	s.State = terminalproto.Ended
	ended, send := observeShell(observed, sent, s, features)
	if !send || ended.Dir != "" || ended.State != terminalproto.Ended {
		t.Fatalf("state change: %+v %v", ended, send)
	}

	// Without the feature a folder change is no news and never encoded.
	oldObserved := map[string]terminalproto.SessionChanged{}
	oldSent := map[string]string{}
	old := terminalhost.ManagedSession{ID: "session-0123456789", State: terminalproto.Running, LiveDir: "/home/me"}
	observeShell(oldObserved, oldSent, old, terminalproto.AttentionFeature)
	old.LiveDir = "/home/me/project"
	body, send := observeShell(oldObserved, oldSent, old, terminalproto.AttentionFeature)
	if send {
		t.Fatalf("a folder change was sent to an older controller: %+v", body)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), `"dir"`) {
		t.Fatalf("folder encoded for an older controller: %s", raw)
	}
}

func TestShellSnapshotsCarryLiveDirOnlyWhenNegotiated(t *testing.T) {
	sessions := []terminalhost.ManagedSession{{ID: "session-0123456789", Title: "Shell", State: terminalproto.Running, LiveDir: "/home/me"}}
	if got := shellSnapshots(sessions, terminalproto.DeskShellFeature); got[0].Dir != "/home/me" {
		t.Fatalf("negotiated snapshot: %+v", got)
	}
	if got := shellSnapshots(sessions, terminalproto.AttentionFeature); got[0].Dir != "" {
		t.Fatalf("snapshot for an older controller: %+v", got)
	}
}
