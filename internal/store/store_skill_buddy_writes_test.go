package store

import (
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbuddy"
)

// buddyWrites counts the rows of skill_buddy_choices written from now on: a
// row updated to what it already holds still counts, as it still costs a write.
func buddyWrites(t *testing.T, s *Store) func() int {
	t.Helper()
	for _, q := range []string{
		`CREATE TABLE buddy_writes(n INTEGER)`,
		`CREATE TRIGGER buddy_inserted AFTER INSERT ON skill_buddy_choices BEGIN INSERT INTO buddy_writes VALUES(1); END`,
		`CREATE TRIGGER buddy_updated AFTER UPDATE ON skill_buddy_choices BEGIN INSERT INTO buddy_writes VALUES(1); END`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return func() int {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM buddy_writes`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
}

// The chat composer asks for suggestions after every pause in typing, so a
// suggestion the receipts already hold writes nothing. A person's choice does.
func TestSuggestingASkillAgainWritesNothing(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	writes := buddyWrites(t, s)
	const scope = "session:abc"
	suggest := skillbuddy.Skill{ID: "builtin:review", Name: "Review", Source: "builtin", Revision: "r1", Reason: "wording matches"}
	if err := s.RecordSkillBuddySuggestionsFor(uid, scope, "abc", "review", []skillbuddy.Skill{suggest}); err != nil {
		t.Fatal(err)
	}
	if n := writes(); n != 1 {
		t.Fatalf("a first suggestion wrote %d rows, want 1", n)
	}
	for range 3 {
		if err := s.RecordSkillBuddySuggestionsFor(uid, scope, "abc", "review", []skillbuddy.Skill{suggest}); err != nil {
			t.Fatal(err)
		}
	}
	if n := writes(); n != 1 {
		t.Fatalf("suggesting the same skill again wrote %d rows in all, want 1", n)
	}
	chosen := suggest
	chosen.State, chosen.Instructions = "selected", "Check tenant isolation."
	if err := s.RecordSkillBuddyFor(uid, scope, "abc", "review", chosen); err != nil {
		t.Fatal(err)
	}
	if n := writes(); n != 2 {
		t.Fatalf("a choice wrote %d rows in all, want 2", n)
	}
	if err := s.RecordSkillBuddySuggestionsFor(uid, scope, "abc", "review", []skillbuddy.Skill{suggest}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.SkillBuddyRecordsFor(uid, scope)
	if err != nil || len(rows) != 1 || rows[0].Skill.State != "selected" || rows[0].Skill.Instructions != "Check tenant isolation." || rows[0].Skill.Revision != "r1" {
		t.Fatalf("a suggestion must not replace a choice: %+v %v", rows, err)
	}
	if n := writes(); n != 2 {
		t.Fatalf("a suggestion over a choice wrote %d rows in all, want 2", n)
	}
	// A skill not yet suggested for the scope is written.
	other := skillbuddy.Skill{ID: "builtin:test", Name: "Test", Source: "builtin", Revision: "r1", Reason: "wording matches"}
	if err = s.RecordSkillBuddySuggestionsFor(uid, scope, "abc", "review", []skillbuddy.Skill{suggest, other}); err != nil {
		t.Fatal(err)
	}
	if n := writes(); n != 3 {
		t.Fatalf("a new suggestion wrote %d rows in all, want 3", n)
	}
}

// Receipts are kept 90 days. The purge runs once for all the suggestions of a
// call, and still takes what has aged out.
func TestSuggestionsPurgeOldReceiptsOncePerCall(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	old := time.Now().Add(-91 * 24 * time.Hour).UnixMilli()
	if _, err := s.db.Exec(`INSERT INTO skill_buddy_choices(user_id,scope,skill_id,name,source,revision,reason,stage,at) VALUES(?,?,?,?,?,?,?,?,?)`,
		uid, "session:old", "builtin:review", "Review", "builtin", "r1", "", "review", old); err != nil {
		t.Fatal(err)
	}
	counter := countStatements(t, s)
	const purge = "DELETE FROM skill_buddy_choices"
	var suggestions []skillbuddy.Skill
	for _, id := range []string{"builtin:review", "builtin:test", "builtin:plan", "builtin:code"} {
		suggestions = append(suggestions, skillbuddy.Skill{ID: id, Name: id, Source: "builtin", Revision: "r1"})
	}
	if err := s.RecordSkillBuddySuggestionsFor(uid, "session:new", "new", "review", suggestions); err != nil {
		t.Fatal(err)
	}
	if got := counter.count(purge); got != 1 {
		t.Fatalf("%d purges for one call, want 1", got)
	}
	rows, err := s.SkillBuddyRecordsFor(uid, "")
	if err != nil || len(rows) != 4 {
		t.Fatalf("receipts after the purge: %d, %v (the old one should be gone, the four new ones kept)", len(rows), err)
	}
	for _, r := range rows {
		if r.Scope == "session:old" {
			t.Fatalf("the receipt past 90 days is still there: %+v", r)
		}
	}
	// A single choice purges too.
	counter.reset()
	if err = s.RecordSkillBuddyFor(uid, "session:new", "new", "review", skillbuddy.Skill{ID: "builtin:plan", Name: "Plan", Source: "builtin", State: "dismissed"}); err != nil {
		t.Fatal(err)
	}
	if got := counter.count(purge); got != 1 {
		t.Fatalf("%d purges for one choice, want 1", got)
	}
}
