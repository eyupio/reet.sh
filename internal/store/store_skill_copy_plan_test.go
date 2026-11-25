package store

import (
	"reflect"
	"testing"
)

// A run of the copy worker plans once: the same plan says what to fetch or
// ask a machine for and which Active skills nothing can supply. Planning is a
// walk over every skill with no content, so it is not done twice per run.
func TestOnePlanFeedsFetchingAndMarkingUnsourcedSkills(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	fetch := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	lonely := localItem(t, s, uid, "elitebook", "lonely")
	gone := localItem(t, s, uid, "elitebook", "gone")
	if err := s.SetSkillStatusFor(uid, []string{gone.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	counter := countStatements(t, s)
	const plan = "SELECT id,name,status,copy_problem FROM library_items"

	needs, unsourced, err := s.SkillCopyPlanFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if want := []SkillCopyNeed{{Item: fetch.id, Name: "find-skills", Kind: "fetch"}}; !reflect.DeepEqual(needs, want) {
		t.Fatalf("needs = %+v, want %+v", needs, want)
	}
	if want := []string{lonely.id}; !reflect.DeepEqual(unsourced, want) {
		t.Fatalf("unsourced = %v, want %v (an Archived skill is left alone)", unsourced, want)
	}
	if n, err := s.MarkSkillsUnsourcedFor(uid, unsourced); err != nil || n != 1 {
		t.Fatalf("marked %d, %v", n, err)
	}
	if h := healthOf(t, s, uid, lonely.id, reportT0); h.State != "copy-failed" || h.Reason != SkillNoSourceProblem {
		t.Fatalf("health: %+v", h)
	}
	if got := counter.count(plan); got != 1 {
		t.Fatalf("planned %d times for one run, want 1", got)
	}
	// What the two older methods say is unchanged.
	if again, err := s.SkillsNeedingCopyFor(uid); err != nil || !reflect.DeepEqual(again, needs) {
		t.Fatalf("SkillsNeedingCopyFor = %+v, %v", again, err)
	}
	if n, err := s.MarkUnsourcedSkillsFor(uid); err != nil || n != 0 {
		t.Fatalf("a skill marked already is not marked again: %d, %v", n, err)
	}
}
