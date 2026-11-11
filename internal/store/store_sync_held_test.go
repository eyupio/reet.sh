package store

import "testing"

func TestSyncTargetsReportSelectedItemsHeldBack(t *testing.T) {
	s, uid, target := controlFixture(t)
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, canonicalHostedFixture("managed", "bytes")); err != nil {
		t.Fatal(err)
	}
	var item LibraryItem
	for _, candidate := range canonicalItems(t, s, uid) {
		if candidate.Name == "managed" {
			item = candidate
		}
	}
	if item.ID == "" || item.Status == LibraryInUse {
		t.Fatalf("fixture item must exist and not be in use: %+v", item)
	}
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveLibrarySelectionFor(uid, LibrarySelection{TargetID: target, ItemIDs: []string{item.ID}}); err != nil {
		t.Fatal(err)
	}
	held := func() []SyncHeld {
		t.Helper()
		// The machine's next check-in is what turns a selection into a sync target.
		if err := s.refreshCanonicalSelections(uid, agent); err != nil {
			t.Fatal(err)
		}
		targets, err := s.SyncTargetsFor(uid)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range targets {
			if entry.Target.ID == target {
				return entry.Held
			}
		}
		t.Fatal("a folder with a saved selection must be listed")
		return nil
	}
	if got := held(); len(got) != 1 || got[0].ItemID != item.ID || got[0].Reason != "status" || got[0].Name != "managed" {
		t.Fatalf("selected item that is not in use was not reported as held: %+v", got)
	}
	if err := s.SetLibraryStatusFor(uid, item.ID, LibraryInUse, item.Version); err != nil {
		t.Fatal(err)
	}
	if got := held(); len(got) != 0 {
		t.Fatalf("an installable selected item is still reported as held: %+v", got)
	}
}
