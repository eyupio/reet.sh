package store

import "testing"

func twoMembers(t *testing.T, s *Store) (gone, kept User) {
	t.Helper()
	gone, err := s.CreateUser("gone@example.com", "Gone", "member", "x")
	if err != nil {
		t.Fatal(err)
	}
	kept, err = s.CreateUser("kept@example.com", "Kept", "member", "x")
	if err != nil {
		t.Fatal(err)
	}
	return gone, kept
}

func countFor(t *testing.T, s *Store, table string, uid int64) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM `+table+` WHERE user_id=?`, uid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// What machines reported about a person's skills goes with the person.
func TestDeleteUserRemovesTheirSkillActivity(t *testing.T) {
	s := newStore(t)
	gone, kept := twoMembers(t, s)
	for _, u := range []User{gone, kept} {
		a := reportAgent(t, s, u.ID, "laptop-"+u.Name)
		reportOf(t, s, u.ID, a, reportT0, installedAt("demo", "d1"))
		if countFor(t, s, "skill_activity", u.ID) != 1 {
			t.Fatalf("no activity was recorded for %s", u.Name)
		}
	}
	if err := s.DeleteUser(gone.ID); err != nil {
		t.Fatal(err)
	}
	if n := countFor(t, s, "skill_activity", gone.ID); n != 0 {
		t.Fatalf("a deleted person's skill activity stayed: %d rows", n)
	}
	if n := countFor(t, s, "skill_activity", kept.ID); n != 1 {
		t.Fatalf("another person's skill activity changed: %d rows", n)
	}
}

// Tables keyed by user_id with no foreign key to users are not cleared by
// deleting the user row, so DeleteUser clears them.
func TestDeleteUserRemovesTablesWithNoForeignKey(t *testing.T) {
	s := newStore(t)
	gone, kept := twoMembers(t, s)
	for _, u := range []int64{gone.ID, kept.ID} {
		if _, err := s.db.Exec(`INSERT INTO terminal_chat_events(user_id,session_id,seq,at,event,bytes) VALUES(?,'s1',1,1,'{"kind":"user","text":"hi"}',27)`, u); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO worktree_cleanups(user_id,agent_id,path,removed_at) VALUES(?,'a1','/home/x/tree',1)`, u); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteUser(gone.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"terminal_chat_events", "worktree_cleanups"} {
		if n := countFor(t, s, table, gone.ID); n != 0 {
			t.Fatalf("%s kept %d rows of a deleted person", table, n)
		}
		if n := countFor(t, s, table, kept.ID); n != 1 {
			t.Fatalf("%s changed another person's rows: %d", table, n)
		}
	}
}
