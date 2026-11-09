package store

import (
	"errors"
	"strings"
	"testing"
)

func deploymentIntentFixture(t *testing.T) (*Store, int64, SyncAgent, LibraryDeploymentCheck) {
	t.Helper()
	s, uid, target := controlFixture(t)
	if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHostedSkill(uid, canonicalHostedFixture("managed", "recoverable bytes")); err != nil {
		t.Fatal(err)
	}
	var item LibraryItem
	for _, candidate := range canonicalItems(t, s, uid) {
		if candidate.Name == "managed" {
			item = candidate
		}
	}
	if item.ID == "" {
		t.Fatal("missing canonical item")
	}
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveLibrarySelectionFor(uid, LibrarySelection{TargetID: target, ItemIDs: []string{item.ID}}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetLibraryStatusFor(uid, item.ID, LibraryInUse, item.Version); err != nil {
		t.Fatal(err)
	}
	base := LibraryBase{BindingID: strings.Repeat("a", 64), ItemID: item.ID, RevisionID: item.RevisionID}
	if err = s.RecordLibraryBaseFor(uid, agent.ID, base); err != nil {
		t.Fatal(err)
	}
	return s, uid, agent, LibraryDeploymentCheck{TargetID: target, LibraryBase: base}
}

func TestLibraryDeploymentIntentTracksControlAndRejectsABA(t *testing.T) {
	s, uid, agent, in := deploymentIntentFixture(t)
	first, err := s.LibraryDeploymentIntentFor(uid, agent.ID, in)
	if err != nil || first.Action != "maintain" || first.Generation == "" {
		t.Fatal(first, err)
	}
	item, _ := s.LibraryItemFor(uid, in.ItemID)
	if err = s.SetLibraryStatusFor(uid, item.ID, LibraryDisabled, item.Version); err != nil {
		t.Fatal(err)
	}
	removed, err := s.LibraryDeploymentIntentFor(uid, agent.ID, in)
	if err != nil || removed.Action != "remove" || removed.Reason != "inactive" {
		t.Fatal(removed, err)
	}
	stale := in
	stale.Generation = first.Generation
	if _, err = s.LibraryDeploymentIntentFor(uid, agent.ID, stale); !errors.Is(err, ErrLibraryConflict) {
		t.Fatalf("stale install allowed: %v", err)
	}
	item, _ = s.LibraryItemFor(uid, in.ItemID)
	if err = s.SetLibraryStatusFor(uid, item.ID, LibraryInUse, item.Version); err != nil {
		t.Fatal(err)
	}
	current, err := s.LibraryDeploymentIntentFor(uid, agent.ID, in)
	if err != nil || current.Action != "maintain" || current.Generation == first.Generation {
		t.Fatal("status ABA revived old work", current, err)
	}
	selection, _ := s.LibrarySelectionFor(uid, in.TargetID)
	selection.ItemIDs = []string{}
	if _, err = s.SaveLibrarySelectionFor(uid, selection); err != nil {
		t.Fatal(err)
	}
	removed, err = s.LibraryDeploymentIntentFor(uid, agent.ID, in)
	if err != nil || removed.Action != "remove" || removed.Reason != "deselected" {
		t.Fatal(removed, err)
	}
	selection, _ = s.LibrarySelectionFor(uid, in.TargetID)
	selection.ItemIDs = []string{in.ItemID}
	if _, err = s.SaveLibrarySelectionFor(uid, selection); err != nil {
		t.Fatal(err)
	}
	stale.Generation = current.Generation
	if _, err = s.LibraryDeploymentIntentFor(uid, agent.ID, stale); !errors.Is(err, ErrLibraryConflict) {
		t.Fatalf("selection ABA revived old work: %v", err)
	}
	// Offline/restarted readers derive the same durable current generation.
	a, _ := s.LibraryDeploymentIntentFor(uid, agent.ID, in)
	b, _ := s.LibraryDeploymentIntentFor(uid, agent.ID, in)
	if a.Generation != b.Generation {
		t.Fatal("generation depends on request time")
	}
	if _, err = s.db.Exec(`UPDATE library_items SET deleted_at=1 WHERE user_id=? AND id=?`, uid, in.ItemID); err != nil {
		t.Fatal(err)
	}
	removed, err = s.LibraryDeploymentIntentFor(uid, agent.ID, in)
	if err != nil || removed.Action != "remove" || removed.Reason != "library_deleted" {
		t.Fatal(removed, err)
	}
}

func TestLibraryDeploymentIntentRequiresExactEnrollmentTargetAndPin(t *testing.T) {
	s, uid, agent, in := deploymentIntentFixture(t)
	other, _, err := s.EnrollSyncAgent(uid, "other-host", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.LibraryDeploymentIntentFor(uid, other.ID, in); err == nil {
		t.Fatal("another machine read deployment intent")
	}
	if _, err = s.LibraryDeploymentIntentFor(uid+1, agent.ID, in); err == nil {
		t.Fatal("another account read deployment intent")
	}
	missing := in
	missing.BindingID = strings.Repeat("b", 64)
	if _, err = s.LibraryDeploymentIntentFor(uid, agent.ID, missing); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("unproven binding", err)
	}
	stale := in
	stale.RevisionID = strings.Repeat("c", 32)
	if _, err = s.LibraryDeploymentIntentFor(uid, agent.ID, stale); !errors.Is(err, ErrLibraryConflict) {
		t.Fatal("wrong applied base", err)
	}
	// Content trouble is not deletion intent. Keep the local bytes for recovery.
	if _, err = s.db.Exec(`UPDATE library_items SET problem='capture unavailable' WHERE user_id=? AND id=?`, uid, in.ItemID); err != nil {
		t.Fatal(err)
	}
	intent, err := s.LibraryDeploymentIntentFor(uid, agent.ID, in)
	if err != nil || intent.Action != "blocked" || intent.Reason != "content_unavailable" {
		t.Fatal(intent, err)
	}
}
