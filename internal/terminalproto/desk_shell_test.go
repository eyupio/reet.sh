package terminalproto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSupportsDeskShell(t *testing.T) {
	if !SupportsDeskShell("attention-v1, desk-shell-v1") || SupportsDeskShell("attention-v1,desk-shell-v2") || SupportsDeskShell("") {
		t.Fatal("desk-shell-v1 negotiation read wrongly")
	}
}

// An older controller's strict decoder refuses an unknown field, so a
// connection that did not negotiate desk-shell-v1 never carries the folder.
func TestDeskShellNotSentToOldController(t *testing.T) {
	snap := SessionSnapshot{ID: "session_1234567890", Title: "Shell", State: Running, Dir: "/home/me/project"}
	changed := SessionChanged{State: Running, Dir: "/home/me/project"}
	for _, features := range []string{"", "command-history-v1,launch-v1,attention-v1,shell-dir-v1"} {
		for _, body := range []any{snap.ForFeatures(features), changed.ForFeatures(features)} {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), `"dir"`) {
				t.Fatalf("features %q carried the folder: %s", features, raw)
			}
		}
	}
	with := snap.ForFeatures("attention-v1," + DeskShellFeature)
	encoded, err := Encode(Message{Version: Version, RequestID: "req_1234567890123456", Generation: 1, Body: Reconcile{Complete: true, Sessions: []SessionSnapshot{with}}})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Body.(*Reconcile).Sessions[0].Dir; got != "/home/me/project" {
		t.Fatalf("negotiated folder = %q", got)
	}
	sc := changed.ForFeatures(DeskShellFeature)
	encoded, err = Encode(Message{Version: Version, RequestID: "req_1234567890123456", SessionID: "session_1234567890", Generation: 1, Body: sc})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Body.(*SessionChanged).Dir; got != "/home/me/project" {
		t.Fatalf("negotiated change folder = %q", got)
	}
}

// An older machine sends neither the field nor anything new; its reconcile
// and session changes decode as before, with no folder.
func TestOldMachineSnapshotDecodes(t *testing.T) {
	reconcile := `{"version":1,"requestId":"req_1234567890123456","generation":1,"type":"reconcile","body":{"complete":true,"sessions":[{"id":"session_1234567890","title":"Shell","state":"running","exitCode":null}]}}`
	changed := `{"version":1,"requestId":"req_1234567890123456","sessionId":"session_1234567890","generation":1,"type":"session_changed","body":{"state":"running","exitCode":null}}`
	for _, raw := range []string{reconcile, changed} {
		m, err := Decode([]byte(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		switch b := m.Body.(type) {
		case *Reconcile:
			if b.Sessions[0].Dir != "" {
				t.Fatal("folder invented")
			}
		case *SessionChanged:
			if b.Dir != "" {
				t.Fatal("folder invented")
			}
		}
	}
}

func TestDeskShellFolderMustBeAbsolutePlainText(t *testing.T) {
	for _, dir := range []string{"relative/path", "/bad\nline", "/bad\x00nul", "/" + strings.Repeat("a", MaxDirBytes)} {
		s := SessionSnapshot{ID: "session_1234567890", Title: "Shell", State: Running, Dir: dir}
		if (Reconcile{Complete: true, Sessions: []SessionSnapshot{s}}).valid() {
			t.Fatalf("reconcile accepted folder %q", dir)
		}
		if (SessionChanged{State: Running, Dir: dir}).valid() {
			t.Fatalf("session change accepted folder %q", dir)
		}
	}
	ok := SessionSnapshot{ID: "session_1234567890", Title: "Shell", State: Running, Dir: `/home/me/it's "here"`}
	if !(Reconcile{Complete: true, Sessions: []SessionSnapshot{ok}}).valid() {
		t.Fatal("reconcile refused a folder with quotes")
	}
}

func TestDeskShellCapabilityNeedsTerminalControl(t *testing.T) {
	c := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "me", TerminalEnabled: true, Readiness: Ready, DeskShell: true}
	if c.Validate() == nil {
		t.Fatal("desk shell accepted without terminal control")
	}
	c.TerminalControl = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(Capabilities{ProtocolVersion: 1})
	if strings.Contains(string(raw), "deskShell") {
		t.Fatalf("unset capability encoded: %s", raw)
	}
}

const deskPane = "0123456789abcdef"

// A desk reference is a plain shell's only: never with an agent, never with a
// folder in both places, and only with a pane id fit for a path.
func TestCreateDeskRefRefusedForAgent(t *testing.T) {
	base := Create{Title: "Shell", Cols: 80, Rows: 24}
	ok := []Create{
		{Title: "Shell", Cols: 80, Rows: 24, Desk: DeskRef{Pane: deskPane}},
		{Title: "Shell", Cols: 80, Rows: 24, Desk: DeskRef{Pane: deskPane, Dir: "/srv/app"}},
		{Title: "Shell", Cols: 80, Rows: 24, Dir: "/home/me/app", Desk: DeskRef{Pane: deskPane}},
	}
	for _, c := range ok {
		if err := c.Validate(); err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
	}
	bad := map[string]Create{
		"agent":          {Title: "A", Cols: 80, Rows: 24, Tool: "claude", Desk: DeskRef{Pane: deskPane}},
		"chat":           {Title: "A", Cols: 80, Rows: 24, Tool: "opencode", Structured: true, Desk: DeskRef{Pane: deskPane}},
		"two folders":    {Title: "A", Cols: 80, Rows: 24, Dir: "/home/me", Desk: DeskRef{Pane: deskPane, Dir: "/srv"}},
		"folder no pane": {Title: "A", Cols: 80, Rows: 24, Desk: DeskRef{Dir: "/srv"}},
		"short pane":     {Title: "A", Cols: 80, Rows: 24, Desk: DeskRef{Pane: "p1"}},
		"path pane":      {Title: "A", Cols: 80, Rows: 24, Desk: DeskRef{Pane: "../../etc/passwd0"}},
		"relative":       {Title: "A", Cols: 80, Rows: 24, Desk: DeskRef{Pane: deskPane, Dir: "srv"}},
		"newline":        {Title: "A", Cols: 80, Rows: 24, Desk: DeskRef{Pane: deskPane, Dir: "/srv\n"}},
	}
	for name, c := range bad {
		if c.Validate() == nil {
			t.Fatalf("%s accepted: %+v", name, c)
		}
	}
	// Create stays comparable, and a plain create encodes exactly as before.
	if base != (Create{Title: "Shell", Cols: 80, Rows: 24}) {
		t.Fatal("not comparable")
	}
	raw, _ := json.Marshal(base)
	if strings.Contains(string(raw), "desk") {
		t.Fatalf("plain create carried a desk: %s", raw)
	}
	with := Create{Title: "Shell", Cols: 80, Rows: 24, Desk: DeskRef{Pane: deskPane, Dir: "/srv/app"}}
	encoded, err := Encode(Message{Version: Version, RequestID: "req_1234567890123456", SessionID: "session_1234567890", Generation: 1, Body: with})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"desk":{"pane":"`+deskPane+`","dir":"/srv/app"}`) {
		t.Fatalf("wire form: %s", encoded)
	}
	decoded, err := Decode(encoded)
	if err != nil || *decoded.Body.(*Create) != with {
		t.Fatalf("round trip: %+v %v", decoded.Body, err)
	}
}

// A machine that cannot open a desk pane's folder says so in a way only a
// desk-shell-v1 controller is sent: launch_failed with the folder-gone detail.
func TestFolderGoneResult(t *testing.T) {
	if !(Result{Code: LaunchFailed, Detail: FolderGone}).valid() {
		t.Fatal("folder gone refused")
	}
	for _, r := range []Result{{Code: LaunchFailed, Detail: "other"}, {Code: Failed, Detail: FolderGone}} {
		if r.valid() {
			t.Fatalf("%+v accepted", r)
		}
	}
}

// A machine reports deskShell only on a link that negotiated desk-shell-v1.
func TestDeskShellCapabilityNeedsFeature(t *testing.T) {
	c := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "me", TerminalEnabled: true, TerminalControl: true, Readiness: Ready, DeskShell: true}
	if c.ValidateFor("shell-dir-v1") == nil {
		t.Fatal("desk shell accepted without the feature")
	}
	if err := c.ValidateFor("shell-dir-v1," + DeskShellFeature); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
