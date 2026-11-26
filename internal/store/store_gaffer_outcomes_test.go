package store

import (
	"strings"
	"testing"
)

// The controller's regular look reads only the outcomes it may still have
// work for. A finished outcome drops out of it, one set going again comes
// back, and a start puts right a column that an older reet left behind the
// record.
func TestTheControllersLookReadsOnlyOutcomesItMayStillHaveWorkFor(t *testing.T) {
	s, uid, _, _ := terminalFixture(t)
	o, err := s.CreateGafferOutcomeFor(uid, GafferOutcome{ID: strings.Repeat("1", 32), Objective: "Ship it.", Sessions: []GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it.", RequestID: strings.Repeat("8", 32), SessionID: "s1"}}})
	if err != nil {
		t.Fatal(err)
	}
	// looked reports the live column and how many outcomes each look reads.
	looked := func(outcome, piece string) (live, starting, followed int) {
		t.Helper()
		if outcome != "" {
			if o, err = s.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *GafferOutcome) bool {
				o.State, o.Sessions[0].State = outcome, piece
				return true
			}); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.db.QueryRow(`SELECT live FROM gaffer_outcomes WHERE id=?`, o.ID).Scan(&live); err != nil {
			t.Fatal(err)
		}
		start, err := s.StartingGafferOutcomes()
		if err != nil {
			t.Fatal(err)
		}
		follow, err := s.FollowedGafferOutcomes()
		if err != nil {
			t.Fatal(err)
		}
		return live, len(start), len(follow)
	}
	if live, starting, followed := looked("settled", "following"); live != 1 || starting != 0 || followed != 1 {
		t.Fatalf("a followed outcome: live=%d starting=%d followed=%d", live, starting, followed)
	}
	if live, starting, followed := looked("settled", "done"); live != 0 || starting != 0 || followed != 0 {
		t.Fatalf("a finished outcome: live=%d starting=%d followed=%d", live, starting, followed)
	}
	if live, starting, _ := looked("starting", "advancing"); live != 1 || starting != 1 {
		t.Fatalf("an outcome set going again: live=%d starting=%d", live, starting)
	}
	if _, err = s.db.Exec(`UPDATE gaffer_outcomes SET live=0`); err != nil {
		t.Fatal(err)
	}
	if err = s.migrateGafferOutcomes(); err != nil {
		t.Fatal(err)
	}
	if live, starting, _ := looked("", ""); live != 1 || starting != 1 {
		t.Fatalf("after a start: live=%d starting=%d", live, starting)
	}
	var plan string
	if err = s.db.QueryRow(`EXPLAIN QUERY PLAN SELECT user_id,data FROM gaffer_outcomes WHERE live=1 AND (json_extract(data,'$.autoReport'))`).Scan(new(int), new(int), new(int), &plan); err != nil || !strings.Contains(plan, "gaffer_outcomes_live") {
		t.Fatalf("the look does not use the live index: %q %v", plan, err)
	}
}
