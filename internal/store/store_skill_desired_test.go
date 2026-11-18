package store

import (
	"reflect"
	"testing"
	"time"
)

var desiredT0 = time.UnixMilli(1_800_000_000_000)

func desiredAt(t *testing.T, s *Store, uid int64, at time.Time) DesiredState {
	t.Helper()
	d, err := s.DesiredSkillsFor(uid, at)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func desiredNames(d DesiredState) (active, remove []string) {
	for _, a := range d.Active {
		active = append(active, a.Name)
	}
	for _, r := range d.Remove {
		remove = append(remove, r.Name)
	}
	return
}

func TestDesiredListsActiveWithContent(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	var items []skillStateRow
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		items = append(items, savedWrittenSkill(t, s, uid, name, false))
	}
	d := desiredAt(t, s, uid, desiredT0)
	if len(d.Active) != 3 || len(d.Remove) != 0 || d.Version == "" {
		t.Fatalf("desired: %+v", d)
	}
	byName := map[string]skillStateRow{}
	for _, it := range items {
		byName[it.name] = it
	}
	for i, name := range []string{"alpha", "bravo", "charlie"} {
		got, want := d.Active[i], byName[name]
		var digest string
		if err := s.db.QueryRow(`SELECT digest FROM library_items WHERE user_id=? AND id=?`, uid, want.id).Scan(&digest); err != nil {
			t.Fatal(err)
		}
		if got != (DesiredSkill{Name: name, Item: want.id, Revision: want.revision, Bundle: want.bundle, Digest: digest}) || got.Bundle == "" || got.Digest == "" {
			t.Fatalf("active[%d] = %+v, item %+v", i, got, want)
		}
	}
}

func TestDesiredHoldsChangeForGrace(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, desiredT0); err != nil {
		t.Fatal(err)
	}
	if active, remove := desiredNames(desiredAt(t, s, uid, desiredT0.Add(9*time.Second))); !reflect.DeepEqual(active, []string{"seo-audit"}) || len(remove) != 0 {
		t.Fatalf("at t0+9s: active %v remove %v", active, remove)
	}
	d := desiredAt(t, s, uid, desiredT0.Add(10*time.Second))
	if len(d.Active) != 0 || !reflect.DeepEqual(d.Remove, []DesiredRemoval{{Name: "seo-audit", Reason: "archived"}}) {
		t.Fatalf("at t0+10s: %+v", d)
	}
}

func TestDesiredHoldsChangeAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	if err = s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, desiredT0); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	uid, _ = s.DefaultOwner()
	if active, remove := desiredNames(desiredAt(t, s, uid, desiredT0.Add(5*time.Second))); !reflect.DeepEqual(active, []string{"seo-audit"}) || len(remove) != 0 {
		t.Fatalf("after reopen: active %v remove %v", active, remove)
	}
}

func TestDesiredLeavesOutItemsWithoutContent(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.db.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, uid, "no-copy", LibrarySkill, "no-copy", SkillActive, 1, 1); err != nil {
		t.Fatal(err)
	}
	d := desiredAt(t, s, uid, desiredT0)
	if len(d.Active) != 0 || len(d.Remove) != 0 {
		t.Fatalf("an Active item without content was listed: %+v", d)
	}
}

func TestDesiredVersionChangesOnlyWithContent(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	savedWrittenSkill(t, s, uid, "alpha", false)
	first := desiredAt(t, s, uid, desiredT0)
	if again := desiredAt(t, s, uid, desiredT0.Add(time.Hour)); again.Version != first.Version {
		t.Fatalf("same inputs, versions %s and %s", first.Version, again.Version)
	}
	savedWrittenSkill(t, s, uid, "bravo", false)
	if next := desiredAt(t, s, uid, desiredT0); next.Version == first.Version {
		t.Fatal("a new skill left the version unchanged")
	}
	if _, err := s.db.Exec(`UPDATE library_items SET revision_id='rev-2' WHERE user_id=? AND lower(name)='alpha'`, uid); err != nil {
		t.Fatal(err)
	}
	before := desiredAt(t, s, uid, desiredT0)
	if _, err := s.db.Exec(`UPDATE library_items SET revision_id='rev-3' WHERE user_id=? AND lower(name)='alpha'`, uid); err != nil {
		t.Fatal(err)
	}
	if after := desiredAt(t, s, uid, desiredT0); after.Version == before.Version {
		t.Fatal("a new revision left the version unchanged")
	}
}

func TestDesiredIncludesDeletedUntilForgotten(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	if err := s.DeleteSkillsFor(uid, []string{item.id}, false, desiredT0); err != nil {
		t.Fatal(err)
	}
	if active, remove := desiredNames(desiredAt(t, s, uid, desiredT0.Add(5*time.Second))); !reflect.DeepEqual(active, []string{"demo"}) || len(remove) != 0 {
		t.Fatalf("inside grace: active %v remove %v", active, remove)
	}
	d := desiredAt(t, s, uid, desiredT0.Add(SkillGrace))
	if len(d.Active) != 0 || !reflect.DeepEqual(d.Remove, []DesiredRemoval{{Name: "demo", Reason: "deleted"}}) {
		t.Fatalf("after grace: %+v", d)
	}
}

func TestDesiredUndoWithinGraceSendsNothing(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	before := desiredAt(t, s, uid, desiredT0.Add(-time.Minute))
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, desiredT0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillActive, desiredT0.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, offset := range []time.Duration{time.Second, 3 * time.Second, 5 * time.Second, 12 * time.Second, time.Minute} {
		d := desiredAt(t, s, uid, desiredT0.Add(offset))
		if d.Version != before.Version || len(d.Active) != 1 || len(d.Remove) != 0 {
			t.Fatalf("at t0+%s: %+v, before %+v", offset, d, before)
		}
	}
}

func TestDesiredNewActiveSkillOfDeletedNameIsNotRemoved(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	old := savedWrittenSkill(t, s, uid, "demo", false)
	if err := s.DeleteSkillsFor(uid, []string{old.id}, false, desiredT0); err != nil {
		t.Fatal(err)
	}
	later := desiredT0.Add(2 * SkillUndoWindow)
	if err := s.PurgeDeletedSkillsFor(uid, later); err != nil {
		t.Fatal(err)
	}
	savedWrittenSkill(t, s, uid, "demo", false)
	if r := skillRemovals(t, s, uid); r["demo"].Reason != "deleted" {
		t.Fatalf("the deleted row should stay: %+v", r)
	}
	active, remove := desiredNames(desiredAt(t, s, uid, later.Add(time.Hour)))
	if !reflect.DeepEqual(active, []string{"demo"}) || len(remove) != 0 {
		t.Fatalf("active %v remove %v", active, remove)
	}
}

func insertDesiredItem(t *testing.T, s *Store, uid int64, id, itemType, name, status, revision string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,revision_id,created_at,updated_at) VALUES(?,?,?,?,?,?,1,1)`, uid, id, itemType, name, status, revision); err != nil {
		t.Fatal(err)
	}
}

func insertDesiredRemoval(t *testing.T, s *Store, uid int64, name string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO skill_removals(user_id,name,reason,created_at) VALUES(?,?,'archived',1)`, uid, name); err != nil {
		t.Fatal(err)
	}
}

func TestDesiredActiveNameBeatsARemovalRowOfAnotherCase(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	insertDesiredItem(t, s, uid, "item-demo", LibrarySkill, "Demo", SkillActive, "rev")
	insertDesiredRemoval(t, s, uid, "demo")
	active, remove := desiredNames(desiredAt(t, s, uid, desiredT0))
	if !reflect.DeepEqual(active, []string{"Demo"}) || len(remove) != 0 {
		t.Fatalf("active %v remove %v", active, remove)
	}
}

func TestDesiredIgnoresItemsOfOtherTypes(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	// An active agent definition does not keep a skill of its name off the
	// remove list, and is never listed as a skill.
	insertDesiredItem(t, s, uid, "item-agent", "agent_definition", "gone", SkillActive, "rev")
	insertDesiredRemoval(t, s, uid, "gone")
	active, remove := desiredNames(desiredAt(t, s, uid, desiredT0))
	if len(active) != 0 || !reflect.DeepEqual(remove, []string{"gone"}) {
		t.Fatalf("active %v remove %v", active, remove)
	}
}

func TestDesiredOrdersNamesEqualIgnoringCaseByItem(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	insertDesiredItem(t, s, uid, "b-item", LibrarySkill, "zed", SkillActive, "rev-b")
	insertDesiredItem(t, s, uid, "a-item", LibrarySkill, "Zed", SkillActive, "rev-a")
	insertDesiredItem(t, s, uid, "c-item", LibrarySkill, "alpha", SkillActive, "rev-c")
	first := desiredAt(t, s, uid, desiredT0)
	var got []string
	for _, a := range first.Active {
		got = append(got, a.Item)
	}
	if !reflect.DeepEqual(got, []string{"c-item", "a-item", "b-item"}) {
		t.Fatalf("order %v", got)
	}
	if again := desiredAt(t, s, uid, desiredT0); again.Version != first.Version {
		t.Fatal("the version of the same state changed")
	}
}
