package store

import "testing"

func TestShelfDefaultsMigrateAndSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	user, err := st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the profile schema shipped before shelf preferences existed.
	if _, err := st.db.Exec(`DROP TABLE user_profiles; CREATE TABLE user_profiles (user_id INTEGER PRIMARY KEY, theme TEXT NOT NULL DEFAULT 'system', timezone TEXT NOT NULL DEFAULT '', date_format TEXT NOT NULL DEFAULT 'locale', updated_at INTEGER NOT NULL DEFAULT 0); INSERT INTO user_profiles(user_id,theme,timezone,date_format) VALUES (?, 'dark', 'Europe/London', 'dmy')`, user.ID); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.ProfileFor(user.ID)
	if err != nil || !p.GafferAnimations || p.ShelfView != "list" || p.CardsExpanded || p.TerminalRemoveOnClose || !p.GafferCompact || p.HostedPrefix != DefaultHostedPrefix || p.Theme != "dark" || p.Timezone != "Europe/London" || p.DateFormat != "dmy" {
		t.Fatalf("migration lost preferences: %+v %v", p, err)
	}
	p.GafferAnimations, p.GafferCompact = false, false
	p.ShelfView, p.CardsExpanded, p.TerminalRemoveOnClose, p.HostedPrefix = "board", true, true, "acme-"
	if err := st.SaveProfileFor(user.ID, p); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err = st.ProfileFor(user.ID)
	if err != nil || p.GafferAnimations || p.GafferCompact || p.ShelfView != "board" || !p.CardsExpanded || !p.TerminalRemoveOnClose || p.HostedPrefix != "acme-" {
		t.Fatalf("defaults did not survive reopen: %+v %v", p, err)
	}
}

// A profile saved before the default Cline source existed reads as "Cline's own
// choice", and a saved source survives a reopen.
func TestDefaultClineSourceMigratesAndSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	user, err := st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`DROP TABLE user_profiles; CREATE TABLE user_profiles (user_id INTEGER PRIMARY KEY, theme TEXT NOT NULL DEFAULT 'system', timezone TEXT NOT NULL DEFAULT '', date_format TEXT NOT NULL DEFAULT 'locale', default_tool TEXT NOT NULL DEFAULT '', default_model TEXT NOT NULL DEFAULT '', default_effort TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL DEFAULT 0); INSERT INTO user_profiles(user_id,theme,default_tool,default_model) VALUES (?, 'dark', 'cline', 'stub/model')`, user.ID); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.ProfileFor(user.ID)
	if err != nil || p.DefaultClineSource != "" || p.DefaultTool != "cline" || p.DefaultModel != "stub/model" || p.Theme != "dark" {
		t.Fatalf("migration lost the profile: %+v %v", p, err)
	}
	p.DefaultClineSource = "cline-pass"
	if err := st.SaveProfileFor(user.ID, p); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if p, err = st.ProfileFor(user.ID); err != nil || p.DefaultClineSource != "cline-pass" || p.DefaultTool != "cline" {
		t.Fatalf("the default source did not survive reopen: %+v %v", p, err)
	}
}

func TestFactoryRemoveModeDefaultsToArchiveAndOnlyKeepsKnownValues(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	user, err := st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.ProfileFor(user.ID)
	if err != nil || p.FactoryRemoveMode != "archive" {
		t.Fatalf("default: %+v %v", p, err)
	}
	p.FactoryRemoveMode = "delete"
	if err := st.SaveProfileFor(user.ID, p); err != nil {
		t.Fatal(err)
	}
	if p, err = st.ProfileFor(user.ID); err != nil || p.FactoryRemoveMode != "delete" {
		t.Fatalf("saved: %+v %v", p, err)
	}
	p.FactoryRemoveMode = "anything else"
	if err := st.SaveProfileFor(user.ID, p); err != nil {
		t.Fatal(err)
	}
	if p, err = st.ProfileFor(user.ID); err != nil || p.FactoryRemoveMode != "archive" {
		t.Fatalf("unknown value: %+v %v", p, err)
	}
}
