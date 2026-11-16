package store

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestRecordTerminalLiveDirKeepsTheNewestAndNotesTheDeskPane(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	sess := createTerminal(t, s, uid, a, c)
	d := mustDesk(t, s, uid, Desk{Name: "A", Doc: deskDoc(a.ID, sess.ID)})

	if err := s.RecordTerminalLiveDirFor(uid, a.ID, sess.ID, "/home/me/app", 100); err != nil {
		t.Fatal(err)
	}
	got, err := s.TerminalSessionFor(uid, sess.ID)
	if err != nil || got.LiveDir != "/home/me/app" || got.LiveDirAt != 100 {
		t.Fatalf("%+v %v", got, err)
	}
	desk, _ := s.DeskFor(uid, d.ID)
	if p := desk.Doc.Tabs[0].Panes["p1"]; p.Dir != "/home/me/app" || p.DirAt != 100 || desk.Version != d.Version {
		t.Fatalf("desk pane %+v version %d", p, desk.Version)
	}

	// An older report changes neither.
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, sess.ID, "/old", 50); err != nil {
		t.Fatal(err)
	}
	got, _ = s.TerminalSessionFor(uid, sess.ID)
	desk, _ = s.DeskFor(uid, d.ID)
	if got.LiveDir != "/home/me/app" || desk.Doc.Tabs[0].Panes["p1"].Dir != "/home/me/app" {
		t.Fatalf("older report won: %q %+v", got.LiveDir, desk.Doc.Tabs[0].Panes["p1"])
	}

	// A session in no desk is recorded all the same.
	other := createTerminal(t, s, uid, a, c)
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, other.ID, "/srv", 10); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.TerminalSessionFor(uid, other.ID); got.LiveDir != "/srv" {
		t.Fatalf("%+v", got)
	}
}

func TestRecordTerminalLiveDirRefusesBadInput(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	sess := createTerminal(t, s, uid, a, c)
	for _, dir := range []string{"", "relative", "/a\nb", "/a\x00b"} {
		if err := s.RecordTerminalLiveDirFor(uid, a.ID, sess.ID, dir, 1); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%q: %v", dir, err)
		}
	}
	if err := s.RecordTerminalLiveDirFor(uid, a.ID, "session-unknown-000001", "/srv", 1); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatalf("unknown session: %v", err)
	}
	b, _ := terminalAgent(t, s, uid, "other")
	if err := s.RecordTerminalLiveDirFor(uid, b.ID, sess.ID, "/srv", 1); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatalf("another machine's session: %v", err)
	}
}
