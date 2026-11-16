package store

import (
	"errors"
	"testing"
)

// A session placed on a desk after its last folder change still gives the
// pane its folder, and that folder counts as the machine's own report.
func TestSetDeskPaneSessionCopiesLiveDir(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	sess := createTerminal(t, s, uid, a, c)
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, sess.ID, "/srv/app", 100); err != nil {
		t.Fatal(err)
	}
	doc := deskDoc(a.ID, "placeholder-session")
	p := doc.Tabs[0].Panes["p1"]
	p.Session, p.Dir, p.DirAt = "", "/old", 50
	doc.Tabs[0].Panes["p1"] = p
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: doc})
	got, err := s.SetDeskPaneSessionFor(uid, d.ID, "p1", sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p := got.Doc.Tabs[0].Panes["p1"]; p.Dir != "/srv/app" || p.DirAt != 100 || p.Session != sess.ID {
		t.Fatalf("%+v", p)
	}
	agent, dir, ok, err := s.DeskPaneReportedDirFor(uid, d.ID, "p1")
	if err != nil || !ok || agent != a.ID || dir != "/srv/app" {
		t.Fatalf("reported: %q %q %v %v", agent, dir, ok, err)
	}
	// A pane whose folder is newer than the session's keeps its own.
	other := createTerminal(t, s, uid, a, c)
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, other.ID, "/older", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetDeskPaneSessionFor(uid, d.ID, "p1", ""); err != nil {
		t.Fatal(err)
	}
	got, err = s.SetDeskPaneSessionFor(uid, d.ID, "p1", other.ID)
	if err != nil || got.Doc.Tabs[0].Panes["p1"].Dir != "/srv/app" {
		t.Fatalf("%+v %v", got.Doc.Tabs[0].Panes["p1"], err)
	}
	// A whole-document write that binds a session copies it the same way.
	third := createTerminal(t, s, uid, a, c)
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, third.ID, "/srv/third", 300); err != nil {
		t.Fatal(err)
	}
	in := got
	p = in.Doc.Tabs[0].Panes["p1"]
	p.Session = third.ID
	in.Doc.Tabs[0].Panes["p1"] = p
	saved, err := s.UpdateDeskFor(uid, in, got.Version)
	if err != nil || saved.Doc.Tabs[0].Panes["p1"].Dir != "/srv/third" {
		t.Fatalf("%+v %v", saved.Doc.Tabs[0].Panes["p1"], err)
	}
	if _, dir, _, _ := s.DeskPaneReportedDirFor(uid, d.ID, "p1"); dir != "/srv/third" {
		t.Fatalf("reported after update: %q", dir)
	}
}

// The folder a machine reported is noted on the desk pane in the same
// transaction as the session's record: when the note fails, neither is kept.
func TestRecordLiveDirNotesThePaneInOneTransaction(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	sess := createTerminal(t, s, uid, a, c)
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc(a.ID, sess.ID)})
	if _, err := s.db.Exec(`UPDATE desks SET doc='{"tabs":' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, sess.ID, "/srv/app", 100); err == nil {
		t.Fatal("a broken desk went unnoticed")
	}
	if got, _ := s.TerminalSessionFor(uid, sess.ID); got.LiveDir != "" || got.LiveDirAt != 0 {
		t.Fatalf("the session kept a folder its pane did not: %+v", got)
	}
}

// A folder only a browser wrote is never taken for the machine's report, and
// a report is forgotten with its pane.
func TestDeskPaneReportedDirOnlyFromMachine(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	sess := createTerminal(t, s, uid, a, c)
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc(a.ID, sess.ID, "s2")})
	in := d
	p := in.Doc.Tabs[0].Panes["p2"]
	p.Dir, p.DirAt = "/etc", 1<<50
	in.Doc.Tabs[0].Panes["p2"] = p
	d, err := s.UpdateDeskFor(uid, in, d.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.DeskPaneReportedDirFor(uid, d.ID, "p2"); ok || err != nil {
		t.Fatalf("a browser's folder counted as reported: %v %v", ok, err)
	}
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, sess.ID, "/srv/app", 100); err != nil {
		t.Fatal(err)
	}
	if agent, dir, ok, _ := s.DeskPaneReportedDirFor(uid, d.ID, "p1"); !ok || agent != a.ID || dir != "/srv/app" {
		t.Fatalf("machine report: %q %q %v", agent, dir, ok)
	}
	d, _ = s.DeskFor(uid, d.ID)
	in = d
	in.Doc = deskDoc(a.ID) // every pane closed
	if _, err := s.UpdateDeskFor(uid, in, d.Version); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := s.DeskPaneReportedDirFor(uid, d.ID, "p1"); ok {
		t.Fatal("a removed pane kept its report")
	}
}

// Binding a reopened shell checks the version read, turns the pane into a
// shell and bumps the version; unbinding clears only that same session.
func TestReopenDeskPaneBindsAndUnbinds(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	doc := deskDoc(a.ID, "old-session")
	p := doc.Tabs[0].Panes["p1"]
	p.Kind = "agent"
	doc.Tabs[0].Panes["p1"] = p
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: doc})
	sess := createTerminal(t, s, uid, a, c)
	if _, err := s.ReopenDeskPaneFor(uid, d.ID, "p1", sess.ID, d.Version+1); !errors.Is(err, ErrDeskStale) {
		t.Fatalf("stale: %v", err)
	}
	got, err := s.ReopenDeskPaneFor(uid, d.ID, "p1", sess.ID, d.Version)
	if err != nil || got.Version != d.Version+1 {
		t.Fatalf("%+v %v", got, err)
	}
	if p := got.Doc.Tabs[0].Panes["p1"]; p.Session != sess.ID || p.Kind != "shell" {
		t.Fatalf("%+v", p)
	}
	if _, err := s.UnbindDeskPaneSessionFor(uid, d.ID, "p1", "someone-else"); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.DeskFor(uid, d.ID); again.Doc.Tabs[0].Panes["p1"].Session != sess.ID || again.Version != got.Version {
		t.Fatalf("unbound another session: %+v", again)
	}
	cleared, err := s.UnbindDeskPaneSessionFor(uid, d.ID, "p1", sess.ID)
	if err != nil || cleared.Doc.Tabs[0].Panes["p1"].Session != "" || cleared.Version != got.Version+1 {
		t.Fatalf("%+v %v", cleared, err)
	}
}
