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
	if err := s.DeleteDeskFor(uid, a.ID, 0); err != nil {
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
	if _, err := s.UpdateDeskFor(uid, d, 0); err != nil {
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
	if _, err := s.SetDeskPaneSessionFor(uid, b.ID, "p1", "s1"); !errors.Is(err, ErrDeskSessionTaken) {
		t.Fatal(err)
	}
	id, pane, ok, err := s.DeskPaneForSession(uid, "s1")
	if err != nil || !ok || id != a.ID || pane != "p1" {
		t.Fatalf("%v %v %v %v", id, pane, ok, err)
	}
	// Clearing frees the session for another desk.
	if _, err := s.SetDeskPaneSessionFor(uid, a.ID, "p1", ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.SetDeskPaneSessionFor(uid, b.ID, "p1", "s1")
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
	if _, err := s.UpdateDeskFor(uid, d, 0); !errors.Is(err, ErrDeskMissing) {
		t.Fatal(err)
	}
	if _, err := s.DeskFor(uid, d.ID); !errors.Is(err, ErrDeskMissing) {
		t.Fatal(err)
	}
	if err := s.DeleteDeskFor(uid, d.ID, 0); !errors.Is(err, ErrDeskMissing) {
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
	if _, err := s.SetDeskPaneSessionFor(uid, d.ID, "p1", "s1"); !errors.Is(err, ErrDeskInvalid) {
		t.Fatal(err)
	}
	d.Doc = deskDoc("m1", "s2")
	if _, err := s.UpdateDeskFor(uid, d, 0); !errors.Is(err, ErrDeskInvalid) {
		t.Fatal(err)
	}
}

func deskAgent(t *testing.T, s *Store, uid int64, id, host string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO sync_agents(id,user_id,host,secret_hash,version,last_seen) VALUES(?,?,?,'h','v',1)`, id, uid, host); err != nil {
		t.Fatal(err)
	}
}

func TestForgetMachineClearsDeskPanes(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	deskAgent(t, s, uid, "agent-1", "laptop")
	deskAgent(t, s, uid, "agent-2", "server")
	a := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc("agent-1", "s1")})
	b := mustDesk(t, s, uid, Desk{Name: "B", Doc: deskDoc("agent-2", "s2")})
	if err := s.ForgetMachineFor(uid, "laptop"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeskFor(uid, a.ID)
	if got.Doc.Tabs[0].Panes["p1"].Session != "" || got.Version != 2 || got.Doc.Tabs[0].Panes["p1"].Host != "agent-1" {
		t.Fatalf("%+v", got)
	}
	if _, _, ok, _ := s.DeskPaneForSession(uid, "s1"); ok {
		t.Fatal("session index kept")
	}
	other, _ := s.DeskFor(uid, b.ID)
	if other.Doc.Tabs[0].Panes["p1"].Session != "s2" || other.Version != 1 {
		t.Fatalf("%+v", other)
	}
}

func TestMergeMachinesRepointsDeskPanes(t *testing.T) {
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
	// Panes follow when one machine's enrollment id is replaced by another.
	if err := s.withTx(func(tx *sql.Tx) error { return repointDeskHostsTx(tx, uid, "agent-1", "agent-9") }); err != nil {
		t.Fatal(err)
	}
	got, _ = s.DeskFor(uid, d.ID)
	if got.Doc.Tabs[0].Panes["p1"].Host != "agent-9" || got.Version != 2 {
		t.Fatalf("%+v", got)
	}
	if id, _, ok, _ := s.DeskPaneForSession(uid, "s1"); !ok || id != d.ID {
		t.Fatal("session index lost")
	}
}

func TestNoteDirDoesNotBumpVersion(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc("m1", "s1")})
	if err := s.NoteDeskPaneDirFor(uid, "s1", "/srv/app", 100); err != nil {
		t.Fatal(err)
	}
	got, _ := s.DeskFor(uid, d.ID)
	p := got.Doc.Tabs[0].Panes["p1"]
	if got.Version != 1 || p.Dir != "/srv/app" || p.DirAt != 100 {
		t.Fatalf("%+v", got)
	}
	// An older report is ignored; an unknown session is not an error.
	if err := s.NoteDeskPaneDirFor(uid, "s1", "/old", 50); err != nil {
		t.Fatal(err)
	}
	if err := s.NoteDeskPaneDirFor(uid, "zzz", "/x", 5); err != nil {
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
	if err := s.NoteDeskPaneDirFor(uid, "s1", "/new", 200); err != nil {
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
