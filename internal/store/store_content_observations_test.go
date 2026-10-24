package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func observationFixture(t *testing.T) (*Store, int64, SyncAgent, string) {
	t.Helper()
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "laptop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	env := EnvironmentIDFor(uid, "laptop", "agents", "")
	if err = s.RecordInventoryFor(uid, env, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	return s, uid, a, env
}
func observationReport(g contentobs.Grant) contentobs.Report {
	return contentobs.Report{SchemaVersion: 1, ScanID: g.ScanID, ObservedAt: now(), Scopes: []contentobs.Scope{{Agent: "agents", Complete: true, Skills: []contentobs.Skill{{Name: "demo", SchemaVersion: skillbundle.SchemaVersion, Algorithm: skillbundle.Algorithm, ModeSemantics: skillbundle.ModeSemantics, Complete: true, Digest: strings.Repeat("a", 64), Files: 1, Bytes: 12}}}}}
}
func TestContentObservationLifecycleAndInventoryInvalidation(t *testing.T) {
	s, uid, a, env := observationFixture(t)
	state := func(want string) {
		t.Helper()
		v, err := s.ContentObservationFor(uid, env)
		if err != nil || v.State != want {
			t.Fatalf("state %+v %v, want %s", v, err, want)
		}
	}
	state("not_reported")
	g, err := s.PrepareContentObservationFor(uid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := observationReport(g)
	if err = s.RecordContentObservationFor(uid, a.ID, r); err != nil {
		t.Fatal(err)
	}
	state("observed")
	// Lost response retries preserve server freshness, even after expiry.
	old := now() - contentobs.FreshMillis - 1000
	if _, err = s.db.Exec(`UPDATE content_observation_scopes SET received_at=?`, old); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE content_observation_grants SET expires_at=1`); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordContentObservationFor(uid, a.ID, r); err != nil {
		t.Fatal(err)
	}
	state("stale")
	r.ObservedAt++
	if err = s.RecordContentObservationFor(uid, a.ID, r); !errors.Is(err, ErrContentObservationStale) {
		t.Fatal("changed retry", err)
	}
	if err = s.ControlSyncAgent(uid, a.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	state("paused")
	if _, err = s.PrepareContentObservationFor(uid, a.ID); err == nil {
		t.Fatal("paused grant")
	}
	if err = s.ControlSyncAgent(uid, a.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordInventoryFor(uid, env, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	state("not_reported")
	g, err = s.PrepareContentObservationFor(uid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	r = observationReport(g)
	// A newer legacy scan cannot acquire an earlier observation, including after
	// a scope is forgotten and recreated with the same deterministic environment ID.
	if err = s.ForgetEnvironmentFor(uid, env); err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "laptop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordInventoryFor(uid, env, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordContentObservationFor(uid, a.ID, r); !errors.Is(err, ErrContentObservationStale) {
		t.Fatal("stale inventory", err)
	}
	state("not_reported")
	g, err = s.PrepareContentObservationFor(uid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	r = observationReport(g)
	r.Scopes[0].Complete = false
	r.Scopes[0].Skills[0].Complete = false
	r.Scopes[0].Skills[0].Digest = ""
	r.Scopes[0].Skills[0].Issues = []string{"symlink"}
	if err = s.RecordContentObservationFor(uid, a.ID, r); err != nil {
		t.Fatal(err)
	}
	state("incomplete")
	if err = s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	state("not_reported")
}
func TestContentObservationOwnershipSupersessionAndRollback(t *testing.T) {
	s, uid, a, env := observationFixture(t)
	other, err := s.CreateUser("obs@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PrepareContentObservationFor(other.ID, a.ID); err == nil {
		t.Fatal("foreign grant")
	}
	g, err := s.PrepareContentObservationFor(uid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := observationReport(g)
	if err = s.RecordContentObservationFor(other.ID, a.ID, r); err == nil {
		t.Fatal("foreign write")
	}
	next, err := s.PrepareContentObservationFor(uid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordContentObservationFor(uid, a.ID, r); !errors.Is(err, ErrContentObservationStale) {
		t.Fatal("superseded", err)
	}
	r = observationReport(next)
	if err = s.RecordContentObservationFor(uid, a.ID, r); err != nil {
		t.Fatal(err)
	}
	foreign, err := s.ContentObservationFor(other.ID, env)
	if err != nil || foreign.State != "not_reported" || len(foreign.Skills) != 0 {
		t.Fatal(foreign, err)
	}
	next, err = s.PrepareContentObservationFor(uid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	r = observationReport(next)
	r.Scopes[0].Skills[0].Digest = strings.Repeat("b", 64)
	if _, err = s.db.Exec(`CREATE TRIGGER fail_observation BEFORE INSERT ON content_observation_scopes BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordContentObservationFor(uid, a.ID, r); err == nil {
		t.Fatal("injected write succeeded")
	}
	got, err := s.ContentObservationFor(uid, env)
	if err != nil || got.Skills[0].Digest != strings.Repeat("a", 64) {
		t.Fatal("lost old observation", got, err)
	}
	if _, err = s.db.Exec(`DROP TRIGGER fail_observation`); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordContentObservationFor(uid, a.ID, r); err != nil {
		t.Fatal(err)
	}
	// Omitted scopes become unknown, never an inferred deletion or empty bundle.
	next, err = s.PrepareContentObservationFor(uid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	r = observationReport(next)
	r.Scopes = []contentobs.Scope{}
	if err = s.RecordContentObservationFor(uid, a.ID, r); err != nil {
		t.Fatal(err)
	}
	got, err = s.ContentObservationFor(uid, env)
	if err != nil || got.State != "not_reported" {
		t.Fatal(got, err)
	}
}

func TestContentObservationTwoMachinesAndRoleChanges(t *testing.T) {
	s, uid, a, env := observationFixture(t)
	b, _, err := s.EnrollSyncAgent(uid, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "desktop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	second := EnvironmentIDFor(uid, "desktop", "agents", "")
	if err = s.RecordInventoryFor(uid, second, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	for _, machine := range []SyncAgent{a, b} {
		g, err := s.PrepareContentObservationFor(uid, machine.ID)
		if err != nil {
			t.Fatal(err)
		}
		r := observationReport(g)
		if machine.ID == b.ID {
			r.Scopes[0].Skills[0].Digest = strings.Repeat("b", 64)
		}
		if err = s.RecordContentObservationFor(uid, machine.ID, r); err != nil {
			t.Fatal(err)
		}
	}
	firstObs, err := s.ContentObservationFor(uid, env)
	if err != nil {
		t.Fatal(err)
	}
	secondObs, err := s.ContentObservationFor(uid, second)
	if err != nil {
		t.Fatal(err)
	}
	if firstObs.Skills[0].Digest == secondObs.Skills[0].Digest {
		t.Fatal("same-name different content was collapsed")
	}
	var bundles, blobs int
	if err = s.db.QueryRow(`SELECT count(*) FROM private_skill_bundles`).Scan(&bundles); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM private_skill_blobs`).Scan(&blobs); err != nil {
		t.Fatal(err)
	}
	if bundles != 0 || blobs != 0 {
		t.Fatal("periodic observation imported content")
	}
	g, err := s.PrepareContentObservationFor(uid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordContentObservationFor(uid, a.ID, observationReport(g)); err == nil {
		t.Fatal("role downgrade bypassed")
	}
	if _, err = s.PrepareContentObservationFor(uid, a.ID); err == nil {
		t.Fatal("viewer prepares")
	}
	if _, err = s.db.Exec(`UPDATE users SET role='owner',disabled_at=1 WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordContentObservationFor(uid, a.ID, observationReport(g)); err == nil {
		t.Fatal("disabled account bypassed")
	}
}
