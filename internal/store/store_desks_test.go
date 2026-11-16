package store

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/eyupio/reet.sh/internal/desk"
)

func deskDoc(host string, sessions ...string) desk.Doc {
	tab := desk.Tab{ID: "t1", Name: "main", Panes: map[string]desk.Pane{}}
	for i, s := range sessions {
		id := fmt.Sprintf("p%d", i+1)
		tab.Panes[id] = desk.Pane{ID: id, Kind: "shell", Host: host, Session: s}
		n := &desk.Node{Pane: id}
		if tab.Root == nil {
			tab.Root, tab.Active = n, id
		} else {
			tab.Root = &desk.Node{Split: "right", Ratio: 0.5, A: tab.Root, B: n}
		}
	}
	if len(sessions) == 0 {
		return desk.Doc{}
	}
	return desk.Doc{Tabs: []desk.Tab{tab}, Active: "t1"}
}

func mustDesk(t *testing.T, s *Store, uid int64, d Desk) Desk {
	t.Helper()
	got, err := s.CreateDeskFor(uid, d)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestDeskCreateListOrder(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	a := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc("m1", "")})
	b := mustDesk(t, s, uid, Desk{Name: " B ", Colour: "blue"})
	if a.Version != 1 || a.Position != 0 || b.Position != 1 || b.Name != "B" || b.Schema != desk.Schema || a.ID == "" {
		t.Fatalf("%+v %+v", a, b)
	}
	list, err := s.ListDesksFor(uid)
	if err != nil || len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Fatalf("%v %+v", err, list)
	}
	if list[1].Doc.Tabs == nil {
		t.Fatal("empty doc read back with nil tabs")
	}
	if err := s.ReorderDesksFor(uid, []string{b.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListDesksFor(uid)
	if list[0].ID != b.ID || list[1].Version != 1 {
		t.Fatalf("%+v", list)
	}
	if err := s.ReorderDesksFor(uid, []string{b.ID}); !errors.Is(err, ErrDeskInvalid) {
		t.Fatal(err)
	}
	if _, err := s.CreateDeskFor(uid, Desk{Name: "a"}); !errors.Is(err, ErrDeskNameTaken) {
		t.Fatal(err)
	}
	if _, err := s.CreateDeskFor(uid, Desk{Name: ""}); !errors.Is(err, ErrDeskInvalid) {
		t.Fatal(err)
	}
	if _, err := s.CreateDeskFor(uid, Desk{Name: "C", Colour: "pink"}); !errors.Is(err, ErrDeskInvalid) {
		t.Fatal(err)
	}
	if _, err := s.DeskFor(uid, "nope"); !errors.Is(err, ErrDeskMissing) {
		t.Fatal(err)
	}
	if err := s.DeleteDeskFor(uid, a.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeskFor(uid, a.ID); !errors.Is(err, ErrDeskMissing) {
		t.Fatal(err)
	}
}

func TestDeskLimit(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	for i := 0; i < MaxDesks; i++ {
		mustDesk(t, s, uid, Desk{Name: fmt.Sprintf("d%d", i)})
	}
	if _, err := s.CreateDeskFor(uid, Desk{Name: "one more"}); !errors.Is(err, ErrDeskFull) {
		t.Fatal(err)
	}
}

func TestDeskStaleWrite(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	d := mustDesk(t, s, uid, Desk{Name: "A"})
	d.Name = "A2"
	u, err := s.UpdateDeskFor(uid, d, 1)
	if err != nil || u.Version != 2 || u.Name != "A2" {
		t.Fatalf("%v %+v", err, u)
	}
	if _, err := s.UpdateDeskFor(uid, d, 1); !errors.Is(err, ErrDeskStale) {
		t.Fatal(err)
	}
	if err := s.DeleteDeskFor(uid, d.ID, 1); !errors.Is(err, ErrDeskStale) {
		t.Fatal(err)
	}
	if _, err := s.UpdateDeskFor(uid, d, 2); err != nil {
		t.Fatal(err)
	}
}

func TestDeskSessionInTwoDesks(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	a := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc("m1", "s1")})
	if _, err := s.CreateDeskFor(uid, Desk{Name: "B", Doc: deskDoc("m1", "s1")}); !errors.Is(err, ErrDeskSessionTaken) {
		t.Fatal(err)
	}
	b := mustDesk(t, s, uid, Desk{Name: "B", Doc: deskDoc("m1", "")})
	if _, err := setPaneSession(s, uid, b.ID, "p1", "s1"); !errors.Is(err, ErrDeskSessionTaken) {
		t.Fatal(err)
	}
	id, pane, ok, err := s.DeskPaneForSession(uid, "s1")
	if err != nil || !ok || id != a.ID || pane != "p1" {
		t.Fatalf("%v %v %v %v", id, pane, ok, err)
	}
	// Clearing frees the session for another desk.
	if _, err := setPaneSession(s, uid, a.ID, "p1", ""); err != nil {
		t.Fatal(err)
	}
	got, err := setPaneSession(s, uid, b.ID, "p1", "s1")
	if err != nil || got.Version != 2 || got.Doc.Tabs[0].Panes["p1"].Session != "s1" {
		t.Fatalf("%v %+v", err, got)
	}
	if _, _, ok, _ := s.DeskPaneForSession(uid, "nope"); ok {
		t.Fatal("unknown session found")
	}
}

func TestDeskHigherSchemaHidden(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	d := mustDesk(t, s, uid, Desk{Name: "A"})
	if _, err := s.db.Exec(`UPDATE desks SET schema=2 WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListDesksFor(uid)
	if err != nil || len(list) != 0 {
		t.Fatalf("%v %+v", err, list)
	}
	if _, err := s.UpdateDeskFor(uid, d, 1); !errors.Is(err, ErrDeskMissing) {
		t.Fatal(err)
	}
	if _, err := s.DeskFor(uid, d.ID); !errors.Is(err, ErrDeskMissing) {
		t.Fatal(err)
	}
	if err := s.DeleteDeskFor(uid, d.ID, 1); !errors.Is(err, ErrDeskMissing) {
		t.Fatal(err)
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM desks WHERE id=?`, d.ID).Scan(&n)
	if n != 1 {
		t.Fatal("hidden desk was removed")
	}
}

func TestTemplateHoldsNoSessions(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	if _, err := s.CreateDeskFor(uid, Desk{Name: "T", Template: true, Doc: deskDoc("m1", "s1")}); !errors.Is(err, ErrDeskInvalid) {
		t.Fatal(err)
	}
	d := mustDesk(t, s, uid, Desk{Name: "T", Template: true, Doc: deskDoc("m1", "")})
	if _, err := setPaneSession(s, uid, d.ID, "p1", "s1"); !errors.Is(err, ErrDeskInvalid) {
		t.Fatal(err)
	}
	d.Doc = deskDoc("m1", "s2")
	if _, err := s.UpdateDeskFor(uid, d, 1); !errors.Is(err, ErrDeskInvalid) {
		t.Fatal(err)
	}
}

func deskAgent(t *testing.T, s *Store, uid int64, id, host string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO sync_agents(id,user_id,host,secret_hash,version,last_seen) VALUES(?,?,?,'h','v',1)`, id, uid, host); err != nil {
		t.Fatal(err)
	}
}

// setPaneSession sets (or, when empty, clears) one pane's session with a
// version bump, as a reopen does without its expect.
func setPaneSession(s *Store, uid int64, deskID, pane, session string) (Desk, error) {
	return s.setDeskPaneSession(uid, deskID, pane, session, 0, false)
}

// noteDir records a folder a machine reported for the pane holding session.
func noteDir(s *Store, uid int64, session, dir string, at int64) error {
	return s.withTx(func(tx *sql.Tx) error { return noteDeskPaneDirTx(tx, uid, session, dir, at) })
}

// Forgetting a machine removes what it listed, not its enrolment: its shells
// keep running, so the desk panes holding them keep them, with the session
// index, the version and what the machine reported for them.
func TestForgetMachineKeepsDeskPanes(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	sess := createTerminal(t, s, uid, a, c)
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc(a.ID, sess.ID)})
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, sess.ID, "/srv/app", 100); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetMachineFor(uid, "laptop"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeskFor(uid, d.ID)
	if got.Doc.Tabs[0].Panes["p1"].Session != sess.ID || got.Version != d.Version {
		t.Fatalf("a forget changed the desk: %+v", got)
	}
	if _, _, ok, _ := s.DeskPaneForSession(uid, sess.ID); !ok {
		t.Fatal("session index lost")
	}
	if _, dir, ok, _ := s.DeskPaneReportedDirFor(uid, d.ID, "p1"); !ok || dir != "/srv/app" {
		t.Fatalf("the pane's reported folder lost: %q %v", dir, ok)
	}
}

// Revoking a machine's enrolment deletes its sessions by cascade, so every
// desk pane that held one is emptied in the same step, with a version bump,
// and no index row or reported folder of that machine is left. Another
// machine's panes are untouched.
func TestRevokeMachineClearsDeskPanes(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	other, oc := terminalAgent(t, s, uid, "server")
	sess := createTerminal(t, s, uid, a, c)
	kept := createTerminal(t, s, uid, other, oc)
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc(a.ID, sess.ID)})
	e := mustDesk(t, s, uid, Desk{Name: "B", Doc: deskDoc(other.ID, kept.ID)})
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, sess.ID, "/srv/app", 100); err != nil {
		t.Fatal(err)
	}
	if err := s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeskFor(uid, d.ID)
	if p := got.Doc.Tabs[0].Panes["p1"]; p.Session != "" || p.Host != a.ID || got.Version != d.Version+1 {
		t.Fatalf("%+v", got)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM desk_sessions WHERE user_id=? AND session_id=?`, uid, sess.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("index rows left: %d %v", n, err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM desk_pane_dirs WHERE user_id=? AND agent_id=?`, uid, a.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("reported folders left: %d %v", n, err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM terminal_sessions WHERE id=?`, sess.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("session row left: %d %v", n, err)
	}
	still, _ := s.DeskFor(uid, e.ID)
	if still.Doc.Tabs[0].Panes["p1"].Session != kept.ID || still.Version != e.Version {
		t.Fatalf("another machine's desk changed: %+v", still)
	}
}

// A pane on one machine that holds a session of the revoked machine is
// emptied as well.
func TestRevokeMachineClearsPaneHoldingItsSession(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	other, _ := terminalAgent(t, s, uid, "server")
	sess := createTerminal(t, s, uid, a, c)
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc(other.ID, sess.ID)})
	if err := s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeskFor(uid, d.ID)
	if got.Doc.Tabs[0].Panes["p1"].Session != "" {
		t.Fatalf("%+v", got)
	}
	if _, _, ok, _ := s.DeskPaneForSession(uid, sess.ID); ok {
		t.Fatal("index row left")
	}
}

func TestMergeMachinesKeepsDeskPanes(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	deskAgent(t, s, uid, "agent-1", "laptop")
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc("agent-1", "s1")})
	// A rename keeps the enrollment id, so the pane stays valid.
	if err := s.MergeMachinesFor(uid, "laptop", "work-laptop"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeskFor(uid, d.ID)
	if got.Doc.Tabs[0].Panes["p1"].Host != "agent-1" || got.Doc.Tabs[0].Panes["p1"].Session != "s1" {
		t.Fatalf("%+v", got)
	}
	if err := got.Doc.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := s.DeskPaneForSession(uid, "s1"); !ok {
		t.Fatal("session index lost")
	}
}

func TestDeskWriteNeedsExpect(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	d := mustDesk(t, s, uid, Desk{Name: "A"})
	for _, e := range []int64{0, -1} {
		if _, err := s.UpdateDeskFor(uid, d, e); !errors.Is(err, ErrDeskStale) {
			t.Fatal(e, err)
		}
		if err := s.DeleteDeskFor(uid, d.ID, e); !errors.Is(err, ErrDeskStale) {
			t.Fatal(e, err)
		}
	}
	if _, err := s.UpdateDeskFor(uid, d, 1); err != nil {
		t.Fatal(err)
	}
}

func TestNoteDirDoesNotBumpVersion(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc("m1", "s1")})
	if err := noteDir(s, uid, "s1", "/srv/app", 100); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeskFor(uid, d.ID)
	p := got.Doc.Tabs[0].Panes["p1"]
	if got.Version != 1 || p.Dir != "/srv/app" || p.DirAt != 100 {
		t.Fatalf("%+v", got)
	}
	// An older report is ignored; an unknown session is not an error.
	if err := noteDir(s, uid, "s1", "/old", 50); err != nil {
		t.Fatal(err)
	}
	if err := noteDir(s, uid, "zzz", "/x", 5); err != nil {
		t.Fatal(err)
	}
	got, _ = s.DeskFor(uid, d.ID)
	if got.Doc.Tabs[0].Panes["p1"].Dir != "/srv/app" {
		t.Fatalf("%+v", got)
	}
	// A browser holding version 1 can still write.
	if _, err := s.UpdateDeskFor(uid, got, 1); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateKeepsNewerDir(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc("m1", "s1", "s2")})
	if err := noteDir(s, uid, "s1", "/new", 200); err != nil {
		t.Fatal(err)
	}
	in := d // the browser's copy has no folder for p1 and an older one for p2
	in.Doc = deskDoc("m1", "s1", "s2")
	p2 := in.Doc.Tabs[0].Panes["p2"]
	p2.Dir, p2.DirAt = "/browser", 10
	in.Doc.Tabs[0].Panes["p2"] = p2
	got, err := s.UpdateDeskFor(uid, in, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p := got.Doc.Tabs[0].Panes["p1"]; p.Dir != "/new" || p.DirAt != 200 {
		t.Fatalf("%+v", p)
	}
	if p := got.Doc.Tabs[0].Panes["p2"]; p.Dir != "/browser" || p.DirAt != 10 {
		t.Fatalf("%+v", p)
	}
	read, _ := s.DeskFor(uid, d.ID)
	if p := read.Doc.Tabs[0].Panes["p1"]; p.Dir != "/new" || p.DirAt != 200 {
		t.Fatalf("%+v", p)
	}
}

// One desk whose stored document cannot be read is left out of the list and
// left alone by a machine's revoke; the other desks are listed and rewritten.
func TestUnreadableDeskSkipped(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	sess := createTerminal(t, s, uid, a, c)
	bad := mustDesk(t, s, uid, Desk{Name: "Bad"})
	good := mustDesk(t, s, uid, Desk{Name: "Good", Doc: deskDoc(a.ID, sess.ID)})
	if _, err := s.db.Exec(`UPDATE desks SET doc='{not json' WHERE id=?`, bad.ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListDesksFor(uid)
	if err != nil || len(list) != 1 || list[0].ID != good.ID {
		t.Fatalf("%v %+v", err, list)
	}
	if err := s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.DeskFor(uid, good.ID); got.Doc.Tabs[0].Panes["p1"].Session != "" {
		t.Fatalf("%+v", got)
	}
}

// A whole-document write that moves a pane to another tab keeps the newer
// folder the machine reported for it.
func TestKeepNewerDirsFollowsMovedPane(t *testing.T) {
	stored := desk.Doc{Tabs: []desk.Tab{{ID: "t1", Panes: map[string]desk.Pane{"p1": {ID: "p1", Dir: "/new", DirAt: 200}}}}}
	doc := desk.Doc{Tabs: []desk.Tab{{ID: "t1", Panes: map[string]desk.Pane{}}, {ID: "t2", Panes: map[string]desk.Pane{"p1": {ID: "p1", Dir: "/old", DirAt: 100}}}}}
	keepNewerDirs(&doc, stored)
	if p := doc.Tabs[1].Panes["p1"]; p.Dir != "/new" || p.DirAt != 200 {
		t.Fatalf("%+v", p)
	}
}

// A desk's name follows the document's rule for names: a format character
// (here a zero-width space) is refused as a control character is.
func TestDeskNameRefusesFormatCharacters(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	for _, name := range []string{"a​b", "a‮b", "a\x1bb", "a b"} {
		if _, err := s.CreateDeskFor(uid, Desk{Name: name}); !errors.Is(err, ErrDeskInvalid) {
			t.Fatalf("%q: %v", name, err)
		}
	}
	mustDesk(t, s, uid, Desk{Name: "café ☕"})
}
