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
	if err != nil || p.ShelfView != "list" || p.CardsExpanded || p.TerminalRemoveOnClose || p.HostedPrefix != DefaultHostedPrefix || p.Theme != "dark" || p.Timezone != "Europe/London" || p.DateFormat != "dmy" {
		t.Fatalf("migration lost preferences: %+v %v", p, err)
	}
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
	if err != nil || p.ShelfView != "board" || !p.CardsExpanded || !p.TerminalRemoveOnClose || p.HostedPrefix != "acme-" {
		t.Fatalf("defaults did not survive reopen: %+v %v", p, err)
	}
}
