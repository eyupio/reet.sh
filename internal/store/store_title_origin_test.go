package store

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// reserveTitled reserves a structured chat row with the given title origin.
func reserveTitled(t *testing.T, s *Store, uid int64, a SyncAgent, c RemoteAgentCapabilities, structured bool, origin terminalproto.TitleOrigin) TerminalSession {
	t.Helper()
	in := terminalproto.Create{Title: "Chat", Cols: 80, Rows: 24, Dir: "/tmp", TitleOrigin: origin}
	if structured {
		in.Tool, in.Structured = "opencode", true
	}
	row, created, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, in)
	if err != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, err)
	}
	return row
}

func retitleState(t *testing.T, s *Store, uid int64, id string) (title string, origin terminalproto.TitleOrigin, tries int) {
	t.Helper()
	got, err := s.TerminalSessionFor(uid, id)
	if err != nil {
		t.Fatal(err)
	}
	origin, err = s.TerminalTitleOriginFor(uid, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT retitle_tries FROM terminal_sessions WHERE id=?`, id).Scan(&tries); err != nil {
		t.Fatal(err)
	}
	return got.Title, origin, tries
}

func wantRetitle(t *testing.T, s *Store, uid int64, id, text string, wantRenamed bool) {
	t.Helper()
	renamed, err := s.RetitleChatFromPrompt(uid, id, text)
	if err != nil || renamed != wantRenamed {
		t.Fatalf("retitle %q: renamed=%v err=%v, want renamed=%v", text, renamed, err, wantRenamed)
	}
}

func TestTitleOriginMigrationKeepsExistingRowsManual(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	row := createTerminal(t, s, uid, a, c)
	for _, col := range []string{"title_origin", "retitle_tries"} {
		if _, err := s.db.Exec(`ALTER TABLE terminal_sessions DROP COLUMN ` + col); err != nil {
			t.Fatal(col, err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := s.migrateTitleOrigin(); err != nil {
			t.Fatal(i, err)
		}
	}
	title, origin, tries := retitleState(t, s, uid, row.ID)
	if title != "Shell" || origin != terminalproto.TitleManual || tries != 0 {
		t.Fatalf("title=%q origin=%q tries=%d", title, origin, tries)
	}
}

func TestReserveStoresOriginFromCreate(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	for _, tc := range []struct {
		in, want terminalproto.TitleOrigin
	}{
		{terminalproto.TitleFallback, terminalproto.TitleFallback},
		{terminalproto.TitlePrompt, terminalproto.TitlePrompt},
		{"", terminalproto.TitleManual},
		{"bogus", terminalproto.TitleManual},
	} {
		row := reserveTitled(t, s, uid, a, c, true, tc.in)
		if _, origin, _ := retitleState(t, s, uid, row.ID); origin != tc.want {
			t.Fatalf("create %q: stored %q, want %q", tc.in, origin, tc.want)
		}
	}
}

func TestRenameTerminalForLocksManual(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	row := reserveTitled(t, s, uid, a, c, true, terminalproto.TitleFallback)
	if err := s.RenameTerminalFor(uid, row.ID, "Mine"); err != nil {
		t.Fatal(err)
	}
	if title, origin, _ := retitleState(t, s, uid, row.ID); title != "Mine" || origin != terminalproto.TitleManual {
		t.Fatalf("title=%q origin=%q", title, origin)
	}
}

func TestRetitleRenamesOnceFromFirstRealTask(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	row := reserveTitled(t, s, uid, a, c, true, terminalproto.TitleFallback)
	wantRetitle(t, s, uid, row.ID, "hi", false)
	if title, origin, tries := retitleState(t, s, uid, row.ID); title != "Chat" || origin != terminalproto.TitleFallback || tries != 1 {
		t.Fatalf("after greeting: title=%q origin=%q tries=%d", title, origin, tries)
	}
	wantRetitle(t, s, uid, row.ID, "Please fix session end navigation", true)
	if title, origin, tries := retitleState(t, s, uid, row.ID); title != "Fix session end navigation" || origin != terminalproto.TitlePrompt || tries != 2 {
		t.Fatalf("after task: title=%q origin=%q tries=%d", title, origin, tries)
	}
	wantRetitle(t, s, uid, row.ID, "Now add a settings page for themes", false)
	if title, _, _ := retitleState(t, s, uid, row.ID); title != "Fix session end navigation" {
		t.Fatalf("renamed twice: %q", title)
	}
}

func TestRetitleStopsAfterThreeTries(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	row := reserveTitled(t, s, uid, a, c, true, terminalproto.TitleFallback)
	for _, g := range []string{"hi", "hello", "hey there!"} {
		wantRetitle(t, s, uid, row.ID, g, false)
	}
	wantRetitle(t, s, uid, row.ID, "Please fix session end navigation", false)
	if title, origin, tries := retitleState(t, s, uid, row.ID); title != "Chat" || origin != terminalproto.TitleFallback || tries != 3 {
		t.Fatalf("title=%q origin=%q tries=%d", title, origin, tries)
	}
}

func TestRetitleSkipsManualPromptAndPlainSessions(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	for name, row := range map[string]TerminalSession{
		"manual":    reserveTitled(t, s, uid, a, c, true, terminalproto.TitleManual),
		"prompt":    reserveTitled(t, s, uid, a, c, true, terminalproto.TitlePrompt),
		"plain tui": reserveTitled(t, s, uid, a, c, false, terminalproto.TitleFallback),
	} {
		wantRetitle(t, s, uid, row.ID, "Please fix session end navigation", false)
		if title, _, tries := retitleState(t, s, uid, row.ID); title != "Chat" || tries != 0 {
			t.Fatalf("%s: title=%q tries=%d", name, title, tries)
		}
	}
}

func TestRetitleImageOnlyCountsATry(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	row := reserveTitled(t, s, uid, a, c, true, terminalproto.TitleFallback)
	wantRetitle(t, s, uid, row.ID, "", false)
	if title, _, tries := retitleState(t, s, uid, row.ID); title != "Chat" || tries != 1 {
		t.Fatalf("title=%q tries=%d", title, tries)
	}
}

func TestRetitleManualRenameWins(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	row := reserveTitled(t, s, uid, a, c, true, terminalproto.TitleFallback)
	if err := s.RenameTerminalFor(uid, row.ID, "Mine"); err != nil {
		t.Fatal(err)
	}
	wantRetitle(t, s, uid, row.ID, "Please fix session end navigation", false)
	if title, origin, _ := retitleState(t, s, uid, row.ID); title != "Mine" || origin != terminalproto.TitleManual {
		t.Fatalf("title=%q origin=%q", title, origin)
	}
}

func TestRetitleWrongOwner(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	row := reserveTitled(t, s, uid, a, c, true, terminalproto.TitleFallback)
	wantRetitle(t, s, uid+1000, row.ID, "Please fix session end navigation", false)
	wantRetitle(t, s, uid, "no-such-session", "Please fix session end navigation", false)
	if title, origin, tries := retitleState(t, s, uid, row.ID); title != "Chat" || origin != terminalproto.TitleFallback || tries != 0 {
		t.Fatalf("title=%q origin=%q tries=%d", title, origin, tries)
	}
}
