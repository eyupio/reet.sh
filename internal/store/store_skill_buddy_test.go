package store

import (
	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"testing"
)

func TestSkillBuddyPreparedReceiptTracksRestartedStage(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u, err := s.CreateUser("buddy@example.test", "Buddy", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	const scope = "factory:outcome:a:1"
	sk := skillbuddy.Skill{ID: "builtin:review", Name: "Review", Source: "builtin", Revision: "reviewed", Instructions: "Check tenant isolation.", State: "selected"}
	if err = s.RecordSkillBuddyFor(u.ID, scope, "", "review", sk); err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{"first-run", "restarted-run"} {
		if err = s.SkillBuddyPreparedFor(u.ID, scope, session); err != nil {
			t.Fatal(err)
		}
		rows, e := s.SkillBuddyRecordsFor(u.ID, scope)
		if e != nil || len(rows) != 1 {
			t.Fatalf("receipts: %+v %v", rows, e)
		}
		if rows[0].SessionID != session || rows[0].Skill.State != "prepared" || rows[0].Skill.Instructions != sk.Instructions || rows[0].Skill.Revision != sk.Revision {
			t.Fatalf("restart receipt stale: %+v", rows[0])
		}
	}
}
