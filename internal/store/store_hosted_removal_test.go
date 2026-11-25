package store

import (
	"reflect"
	"testing"
	"time"
)

// desiredRemovals is what machines are told to take off at at.
func desiredRemovals(t *testing.T, s *Store, uid int64, at time.Time) []DesiredRemoval {
	t.Helper()
	d, err := s.DesiredSkillsFor(uid, at)
	if err != nil {
		t.Fatal(err)
	}
	return d.Remove
}

// Deleting a written skill from Write a skill leaves its folder on no machine:
// the name goes on the removal list, as a delete from the Skills page puts it.
func TestHostedDeleteTellsMachinesToRemoveTheSkill(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	a := reportAgent(t, s, uid, "elitebook")
	base := time.Now()
	reportOf(t, s, uid, a, base, installedAt("demo", itemDigest(t, s, uid, item.id)))
	if err := s.DeleteHostedSkill(uid, "demo", ""); err != nil {
		t.Fatal(err)
	}
	if r := skillRemovals(t, s, uid); r["demo"].Reason != "deleted" {
		t.Fatalf("the deleted written skill is not on the removal list: %+v", r)
	}
	want := []DesiredRemoval{{Name: "demo", Reason: "deleted"}}
	if got := desiredRemovals(t, s, uid, base.Add(time.Hour)); !reflect.DeepEqual(got, want) {
		t.Fatalf("machines are told to remove %+v, want %+v", got, want)
	}
	// The machine still has the folder, so it reports it unknown. That is the
	// skill it was told to remove, not a new one.
	reportOf(t, s, uid, a, base.Add(time.Minute), SkillReportEntry{Folder: "~/.agents/skills", Name: "demo", State: "unknown", Digest: "d"})
	if got, err := s.NewOnMachinesFor(uid); err != nil || len(got) != 0 {
		t.Fatalf("a deleted written skill is listed as new: %+v %v", got, err)
	}
}

// A written skill already deleted from the Skills page is on the removal list
// from then; deleting it from Write a skill keeps that row as it is.
func TestHostedDeleteAfterASkillsPageDeleteKeepsTheFirstRemoval(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	first := time.Now().Add(-time.Minute)
	if err := s.DeleteSkillsFor(uid, []string{item.id}, false, first); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHostedSkill(uid, "demo", ""); err != nil {
		t.Fatal(err)
	}
	want := SkillRemoval{Name: "demo", Reason: "deleted", CreatedAt: first.UnixMilli()}
	if r := skillRemovals(t, s, uid); len(r) != 1 || r["demo"] != want {
		t.Fatalf("the first removal row changed: %+v", r)
	}
}

// A written skill with no library item has no name to put on the list, and is
// deleted all the same.
func TestHostedDeleteWithoutALibraryItemStillDeletes(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	savedWrittenSkill(t, s, uid, "demo", false)
	if _, err := s.db.Exec(`DELETE FROM library_aliases WHERE user_id=? AND kind='hosted' AND reference='demo'`, uid); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHostedSkill(uid, "demo", ""); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.HostedSkillExists(uid, "demo"); err != nil || ok {
		t.Fatal("the written skill was not deleted", err)
	}
	if r := skillRemovals(t, s, uid); len(r) != 0 {
		t.Fatalf("a skill with no item was put on the removal list: %+v", r)
	}
}

// Renaming a written skill moves it to a new folder name, so the folder of the
// old name goes off every machine and the new name is not told to be removed.
func TestHostedRenameTellsMachinesToRemoveTheOldName(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	a := reportAgent(t, s, uid, "elitebook")
	base := time.Now()
	reportOf(t, s, uid, a, base, installedAt("demo", itemDigest(t, s, uid, item.id)))
	if _, err := s.RenameHostedSkill(uid, "demo", "demo-two", mustRef(t, "hosted/writer/demo"), mustRef(t, "hosted/writer/demo-two")); err != nil {
		t.Fatal(err)
	}
	r := skillRemovals(t, s, uid)
	if r["demo"].Reason != "deleted" {
		t.Fatalf("the old name is not on the removal list: %+v", r)
	}
	if _, listed := r["demo-two"]; listed {
		t.Fatalf("the new name is on the removal list: %+v", r)
	}
	want := []DesiredRemoval{{Name: "demo", Reason: "deleted"}}
	if got := desiredRemovals(t, s, uid, base.Add(time.Hour)); !reflect.DeepEqual(got, want) {
		t.Fatalf("machines are told to remove %+v, want %+v", got, want)
	}
}
