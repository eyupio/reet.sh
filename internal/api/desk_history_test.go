package api

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (f *launchAPI) sentEnds() []terminalproto.End {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.End(nil), f.ends...)
}

// New tab and Split on a desk start a plain shell with the pane's id: a
// desk-shell-v1 machine gets it as Create.Desk.Pane, so the shell keeps that
// pane's own history; an older machine gets the plain create it always had.
func TestCreateDeskPaneReachesOnlyDeskShellMachine(t *testing.T) {
	for _, deskShell := range []bool{true, false} {
		f := launchAPIFixture(t, reopenCaps(deskShell, true))
		code, _, raw := f.create(t, map[string]any{"agentId": f.agent.ID, "requestId": terminalbroker.ID(), "title": "Shell", "cols": 80, "rows": 24, "deskPane": reopenPane})
		if code != 200 {
			t.Fatalf("desk shell %v: %d %s", deskShell, code, raw)
		}
		sent := f.sent()
		want := terminalproto.DeskRef{}
		if deskShell {
			want.Pane = reopenPane
		}
		if len(sent) != 1 || sent[0].Desk != want {
			t.Fatalf("desk shell %v: sent %+v", deskShell, sent)
		}
	}
}

// A pane id is checked before it reaches a machine, where it names a file.
func TestCreateDeskPaneRefusesBadID(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(true, true))
	for _, pane := range []string{"../x", "../../../../etc/passwd", "short", "a/0123456789abcdef"} {
		code, _, raw := f.create(t, map[string]any{"agentId": f.agent.ID, "requestId": terminalbroker.ID(), "title": "Shell", "cols": 80, "rows": 24, "deskPane": pane})
		if code != 400 {
			t.Fatalf("%q: %d %s", pane, code, raw)
		}
	}
	if sent := f.sent(); len(sent) != 0 {
		t.Fatalf("a refused create reached the machine: %+v", sent)
	}
}

// End the shell on a desk pane removes the pane's history on a desk-shell-v1
// machine; an older machine is never sent the field, and a plain end (Take
// off this desk, or Sessions) never asks.
func TestEndRouteForgetsDeskPaneHistory(t *testing.T) {
	for _, tc := range []struct {
		deskShell, forget, want bool
	}{{true, true, true}, {true, false, false}, {false, true, false}} {
		f := launchAPIFixture(t, reopenCaps(tc.deskShell, true))
		session := startShell(t, f)
		code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/end", map[string]any{"requestId": terminalbroker.ID(), "removeHistory": false, "factoryOutcome": "", "forgetDesk": tc.forget})
		if code != 200 {
			t.Fatalf("%+v: %d %s", tc, code, raw)
		}
		ends := f.sentEnds()
		if len(ends) != 1 || ends[0].ForgetDesk != tc.want {
			t.Fatalf("%+v: ends %+v", tc, ends)
		}
	}
}

// Deleting a desk with its shells removes each pane's history with its shell.
func TestDeskDeleteWithShellsForgetsPaneHistory(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(true, true))
	session := startShell(t, f)
	id, version := reopenDesk(t, f, "shell", session, "")
	code, _, raw := f.do(t, f.cookie, "DELETE", "/api/v2/desks/"+id, `{"expect":`+jsonNumber(int64(version))+`,"endSessions":true}`)
	if code != 200 {
		t.Fatal(code, raw)
	}
	ends := f.sentEnds()
	if len(ends) != 1 || !ends[0].ForgetDesk {
		t.Fatalf("ends %+v", ends)
	}
}

// endedShell starts a plain shell and ends it with a plain end, then forgets
// the ends the machine was sent so far.
func endedShell(t *testing.T, f *launchAPI) string {
	t.Helper()
	session := startShell(t, f)
	code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/end", map[string]any{"requestId": terminalbroker.ID(), "removeHistory": false, "factoryOutcome": ""})
	if code != 200 {
		t.Fatal(code, raw)
	}
	if s, err := f.st.TerminalSessionFor(f.uid, session); err != nil || s.State.ReservesQuota() {
		t.Fatalf("shell did not end: %+v %v", s, err)
	}
	f.mu.Lock()
	f.ends = nil
	f.mu.Unlock()
	return session
}

// Closing a pane whose shell already ended or was lost sends the end route
// forgetDesk with removeHistory false: a desk-shell-v1 machine is asked to
// remove the pane's history, an older one gets a plain end it answers as a
// no-op, and the session stays in Sessions either way.
func TestEndRouteForgetsEndedDeskPane(t *testing.T) {
	for _, deskShell := range []bool{true, false} {
		f := launchAPIFixture(t, reopenCaps(deskShell, true))
		session := endedShell(t, f)
		code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/end", map[string]any{"requestId": terminalbroker.ID(), "removeHistory": false, "factoryOutcome": "", "forgetDesk": true})
		if code != 200 {
			t.Fatalf("desk shell %v: %d %s", deskShell, code, raw)
		}
		if ends := f.sentEnds(); len(ends) != 1 || ends[0].ForgetDesk != deskShell {
			t.Fatalf("desk shell %v: ends %+v", deskShell, ends)
		}
		if archived, err := f.st.TerminalArchivedFor(f.uid, session); err != nil || archived {
			t.Fatalf("desk shell %v: removed from history: %v %v", deskShell, archived, err)
		}
		if s, err := f.st.TerminalSessionFor(f.uid, session); err != nil || s.RemoveHistory {
			t.Fatalf("desk shell %v: %+v %v", deskShell, s, err)
		}
	}
}

// End the shell on a live pane takes removeHistory from the request (the
// person's terminalRemoveOnClose setting); nothing on the server forces it.
func TestEndRouteForgetDeskKeepsRemoveHistoryChoice(t *testing.T) {
	for _, remove := range []bool{false, true} {
		f := launchAPIFixture(t, reopenCaps(true, true))
		session := startShell(t, f)
		code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/end", map[string]any{"requestId": terminalbroker.ID(), "removeHistory": remove, "factoryOutcome": "", "forgetDesk": true})
		if code != 200 {
			t.Fatal(code, raw)
		}
		if archived, err := f.st.TerminalArchivedFor(f.uid, session); err != nil || archived != remove {
			t.Fatalf("removeHistory %v: archived %v %v", remove, archived, err)
		}
	}
}

// Deleting a desk with its shells also asks a desk-shell-v1 machine to remove
// the history of a pane whose shell had already ended, without removing that
// session from Sessions; an older machine is sent nothing for it.
func TestDeskDeleteWithShellsForgetsEndedPane(t *testing.T) {
	for _, deskShell := range []bool{true, false} {
		f := launchAPIFixture(t, reopenCaps(deskShell, true))
		session := endedShell(t, f)
		id, version := reopenDesk(t, f, "shell", session, "")
		code, _, raw := f.do(t, f.cookie, "DELETE", "/api/v2/desks/"+id, `{"expect":`+jsonNumber(int64(version))+`,"endSessions":true}`)
		if code != 200 {
			t.Fatal(code, raw)
		}
		ends := f.sentEnds()
		if deskShell && (len(ends) != 1 || !ends[0].ForgetDesk) || !deskShell && len(ends) != 0 {
			t.Fatalf("desk shell %v: ends %+v", deskShell, ends)
		}
		if archived, err := f.st.TerminalArchivedFor(f.uid, session); err != nil || archived {
			t.Fatalf("desk shell %v: removed from history: %v %v", deskShell, archived, err)
		}
	}
}
