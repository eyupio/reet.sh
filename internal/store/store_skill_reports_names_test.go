package store

import (
	"errors"
	"testing"
	"time"
)

// Machines fold a skill's name with Unicode lower-casing, and so must the store:
// SQLite's lower() leaves a letter such as É alone, so a name folded there and a
// name folded in Go never meet. The ASCII name is the control.
var foldedSkillNames = []string{"École", "demo"}

func TestReportHealthReadsASkillWithAnyLetterInstalled(t *testing.T) {
	for _, name := range foldedSkillNames {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			uid, _ := s.DefaultOwner()
			insertDesiredItem(t, s, uid, "item-1", LibrarySkill, name, SkillActive, "rev-1")
			a := reportAgent(t, s, uid, "elitebook")
			reportOf(t, s, uid, a, reportT0, installedAt(name, ""))
			if h := healthOf(t, s, uid, "item-1", reportT0); h.State != "ok" || h.Pending != 0 || h.Hosts == nil {
				t.Fatalf("an installed %s is not healthy: %+v", name, h)
			}
			work, err := s.SkillWorkFor(uid, reportT0)
			if err != nil || work != (SkillWork{}) {
				t.Fatalf("an installed %s leaves work: %+v %v", name, work, err)
			}
		})
	}
}

func TestSyncMachinesReadsASkillWithAnyLetterInstalled(t *testing.T) {
	for _, name := range foldedSkillNames {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			uid, _ := s.DefaultOwner()
			insertDesiredItem(t, s, uid, "item-1", LibrarySkill, name, SkillActive, "rev-1")
			a := reportAgent(t, s, uid, "elitebook")
			reportOf(t, s, uid, a, reportT0, installedAt(name, ""))
			ms, err := s.SyncMachinesFor(uid, reportT0)
			if err != nil || len(ms) != 1 {
				t.Fatal(ms, err)
			}
			if ms[0].State != "ok" || len(ms[0].Waiting) != 0 {
				t.Fatalf("an installed %s is still waiting: %+v", name, ms[0])
			}
		})
	}
}

// A deleted name leaves the removal list once the machine has stopped listing
// it, and not before.
func TestReportDeletedSkillStaysOnTheListUntilTheMachineDropsIt(t *testing.T) {
	for _, name := range foldedSkillNames {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			uid, _ := s.DefaultOwner()
			insertDesiredItem(t, s, uid, "item-1", LibrarySkill, name, SkillActive, "rev-1")
			a := reportAgent(t, s, uid, "elitebook")
			base := time.Now()
			reportOf(t, s, uid, a, base, installedAt(name, ""))
			if err := s.DeleteSkillsFor(uid, []string{"item-1"}, false, base); err != nil {
				t.Fatal(err)
			}
			reportOf(t, s, uid, a, base.Add(time.Minute), installedAt(name, ""))
			if r := skillRemovals(t, s, uid); r[name].Reason != "deleted" {
				t.Fatalf("%s was forgotten while the machine still has it: %+v", name, r)
			}
			reportOf(t, s, uid, a, base.Add(2*time.Minute))
			if r := skillRemovals(t, s, uid); len(r) != 0 {
				t.Fatalf("%s stayed once the machine dropped it: %+v", name, r)
			}
		})
	}
}

func TestReportRepeatedReportWritesOneActivityRow(t *testing.T) {
	for _, name := range foldedSkillNames {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			uid, _ := s.DefaultOwner()
			a := reportAgent(t, s, uid, "elitebook")
			for i := 0; i < 3; i++ {
				reportOf(t, s, uid, a, reportT0.Add(time.Duration(i)*time.Minute), installedAt(name, "d1"))
			}
			if got := activityOf(t, s, uid); len(got) != 1 || got[0].Action != "installed" {
				t.Fatalf("want one installed row for %s, got %+v", name, got)
			}
		})
	}
}

// Two names that differ only in the case of a letter beyond ASCII are one name
// to every machine, so the library holds one of them.
func TestSkillNameTakenFoldsAnyLetter(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	insertDesiredItem(t, s, uid, "item-1", LibrarySkill, "École", SkillActive, "rev-1")
	if err := s.DeleteSkillsFor(uid, []string{"item-1"}, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	insertDesiredItem(t, s, uid, "item-2", LibrarySkill, "école", SkillActive, "rev-2")
	if err := s.UndeleteSkillsFor(uid, []string{"item-1"}); !errors.Is(err, ErrSkillNameTaken) {
		t.Fatal("undo made a second skill of the same name", err)
	}
}
