package store

import (
	"errors"
	"testing"
	"time"
)

// historyFixture is a written skill edited twice: three versions, the last
// one current.
func historyFixture(t *testing.T) (*Store, int64, skillStateRow, []string) {
	t.Helper()
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "notes", false)
	revs := []string{item.revision}
	for _, body := range []string{"Second.", "Third."} {
		res, err := s.SetSkillContentFromFor(uid, item.id, contentFiles("notes", body), PrivateSkillOrigin{Kind: "edit"}, revs[len(revs)-1])
		if err != nil {
			t.Fatal(err)
		}
		revs = append(revs, res.Revision.ID)
	}
	return s, uid, skillStateNamed(t, s, uid, "notes"), revs
}

func TestSkillHistoryListsVersionsNewestFirst(t *testing.T) {
	s, uid, item, revs := historyFixture(t)
	got, err := s.SkillHistoryFor(uid, item.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Revision != revs[2] || got[1].Revision != revs[1] || got[2].Revision != revs[0] {
		t.Fatalf("history order: %+v (want %v reversed)", got, revs)
	}
	if !got[0].Current || got[1].Current || got[2].Current {
		t.Fatalf("only the head is current: %+v", got)
	}
	for _, e := range got[:2] {
		if e.Kind != "edit" || e.Files != 1 || e.Digest == "" || e.SavedAt == 0 {
			t.Fatalf("edit entry: %+v", e)
		}
	}
	if got[2].Kind == "" || got[2].Files < 1 {
		t.Fatalf("first entry: %+v", got[2])
	}
}

// A capture says which machine it came from.
func TestSkillHistoryNamesTheMachineOfACapture(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := localItem(t, s, uid, "elitebook", "demo")
	if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("demo", "Kept."), PrivateSkillOrigin{Kind: "capture", Reference: "machine:elitebook:~/.agents/skills"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.SkillHistoryFor(uid, item.id)
	if err != nil || len(got) != 1 || got[0].Kind != "capture" || got[0].Machine != "elitebook" {
		t.Fatalf("capture entry: %+v %v", got, err)
	}
}

func TestSkillHistoryIsTheOwnersOnly(t *testing.T) {
	s, _, item, _ := historyFixture(t)
	other, err := s.CreateUser("history-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SkillHistoryFor(other.ID, item.id); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatalf("another account read the history: %v", err)
	}
	if _, err = s.RestoreSkillRevisionFor(other.ID, item.id, item.revision, item.revision); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatalf("another account put a version back: %v", err)
	}
}

// Put back makes the old version's files a new head of kind restore, so the
// history only grows, and machines are given it like any edit.
func TestPutBackMakesANewRevisionThatMachinesGet(t *testing.T) {
	s, uid, item, revs := historyFixture(t)
	res, err := s.RestoreSkillRevisionFor(uid, item.id, revs[0], revs[2])
	if err != nil {
		t.Fatal(err)
	}
	now := skillStateNamed(t, s, uid, "notes")
	if now.revision != res.Revision.ID || now.revision == revs[0] || now.bundle != item.bundle {
		t.Fatalf("head after put back: %+v (result %s)", now, res.Revision.ID)
	}
	old, err := s.PrivateSkillRevisionFor(uid, item.bundle, revs[0])
	if err != nil {
		t.Fatal(err)
	}
	if res.Revision.ManifestDigest != old.ManifestDigest {
		t.Fatalf("put back saved other files: %s, want %s", res.Revision.ManifestDigest, old.ManifestDigest)
	}
	got, err := s.SkillHistoryFor(uid, item.id)
	if err != nil || len(got) != 4 || got[0].Kind != "restore" || got[0].DerivedFrom != revs[0] || !got[0].Current {
		t.Fatalf("history after put back: %+v %v", got, err)
	}
	found := false
	for _, a := range desiredAt(t, s, uid, time.Now().Add(time.Hour)).Active {
		if a.Item == item.id {
			found = a.Revision == res.Revision.ID && a.Digest == old.ManifestDigest
		}
	}
	if !found {
		t.Fatalf("machines are not given the version put back: %+v", desiredAt(t, s, uid, time.Now().Add(time.Hour)).Active)
	}
}

func TestPutBackRefusesAMovedHeadTheCurrentVersionAndAnotherSkills(t *testing.T) {
	s, uid, item, revs := historyFixture(t)
	if _, err := s.RestoreSkillRevisionFor(uid, item.id, revs[0], revs[1]); !errors.Is(err, ErrPrivateSkillConflict) {
		t.Fatalf("a stale head: %v", err)
	}
	if _, err := s.RestoreSkillRevisionFor(uid, item.id, revs[2], revs[2]); !errors.Is(err, ErrSkillRevisionCurrent) {
		t.Fatalf("the current version: %v", err)
	}
	other := savedWrittenSkill(t, s, uid, "other", false)
	if _, err := s.RestoreSkillRevisionFor(uid, item.id, other.revision, revs[2]); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatalf("another skill's version: %v", err)
	}
	if after := skillStateNamed(t, s, uid, "notes"); after.revision != revs[2] {
		t.Fatalf("a refused put back moved the head: %+v", after)
	}
}
