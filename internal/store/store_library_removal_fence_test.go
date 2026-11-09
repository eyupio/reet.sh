package store

import (
	"encoding/json"
	"strings"
	"testing"
)

type removalJobEnv struct {
	s     *Store
	uid   int64
	agent SyncAgent
	in    LibraryDeploymentCheck
}

func removalJobFixture(t *testing.T, enabled bool) removalJobEnv {
	t.Helper()
	s, uid, agent, in := deploymentIntentFixture(t)
	s.SetManagedRemovals(enabled)
	if err := s.NegotiateLibraryFor(uid, agent.ID); err != nil {
		t.Fatal(err)
	}
	agent.LibraryProtocol = 1
	return removalJobEnv{s, uid, agent, in}
}

// removalPlan builds the plan a machine would report for one managed removal.
func (e removalJobEnv) removalPlan(t *testing.T, check LibraryDeploymentCheck) json.RawMessage {
	t.Helper()
	plan, err := json.Marshal(map[string]any{"target": check.TargetID, "actions": []any{map[string]any{"add": false, "skill": map[string]any{"itemId": check.ItemID, "slug": "managed"}, "removalIntent": check}}})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func (e removalJobEnv) intentCheck(t *testing.T) LibraryDeploymentCheck {
	t.Helper()
	intent, err := e.s.LibraryDeploymentIntentFor(e.uid, e.agent.ID, e.in)
	if err != nil {
		t.Fatal(err)
	}
	check := e.in
	check.Generation = intent.Generation
	return check
}

func (e removalJobEnv) deactivate(t *testing.T) {
	t.Helper()
	item, _ := e.s.LibraryItemFor(e.uid, e.in.ItemID)
	if err := e.s.SetLibraryStatusFor(e.uid, item.ID, LibraryDisabled, item.Version); err != nil {
		t.Fatal(err)
	}
}

func (e removalJobEnv) reactivate(t *testing.T) {
	t.Helper()
	item, _ := e.s.LibraryItemFor(e.uid, e.in.ItemID)
	if err := e.s.SetLibraryStatusFor(e.uid, item.ID, LibraryInUse, item.Version); err != nil {
		t.Fatal(err)
	}
}

// submit queues an automatic job, has the machine report plan, and returns the job.
func (e removalJobEnv) submit(t *testing.T, plan json.RawMessage) SyncJob {
	t.Helper()
	if err := e.s.refreshCanonicalSelections(e.uid, e.agent); err != nil {
		t.Fatal(err)
	}
	queued, err := e.s.QueueSyncJob(e.uid, e.agent.ID, e.in.TargetID, "auto")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := e.s.ClaimSyncJob(e.uid, e.agent)
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatal(claimed, err)
	}
	if err = e.s.FinishSyncJob(e.uid, e.agent.ID, claimed.ID, "awaiting-approval", strings.Repeat("a", 64), "", plan); err != nil {
		t.Fatal(err)
	}
	return e.job(t, claimed.ID)
}

func (e removalJobEnv) job(t *testing.T, id string) SyncJob {
	t.Helper()
	jobs, err := e.s.SyncJobsFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job.ID == id {
			return job
		}
	}
	t.Fatal("job not found", id)
	return SyncJob{}
}

func TestManagedRemovalPlanIsApprovedOnlyForCurrentIntentAndWhenEnabled(t *testing.T) {
	// Off by default: the same authorised plan still needs review.
	off := removalJobFixture(t, false)
	off.deactivate(t)
	job := off.submit(t, off.removalPlan(t, off.intentCheck(t)))
	if job.Status != "awaiting-approval" || job.Kind != "auto" || !strings.Contains(job.Message, "Removals need review") {
		t.Fatalf("disabled capability approved a removal: %+v", job)
	}

	on := removalJobFixture(t, true)
	on.deactivate(t)
	job = on.submit(t, on.removalPlan(t, on.intentCheck(t)))
	if job.Status != "queued" || job.Kind != "apply" {
		t.Fatalf("current authorised removal was not approved: %+v", job)
	}
}

func TestManagedRemovalPlanRefusesStaleOrUnprovenIntent(t *testing.T) {
	e := removalJobFixture(t, true)
	// Still In use and selected: intent is maintain, so a removal is stale.
	stale := e.removalPlan(t, e.intentCheck(t))
	job := e.submit(t, stale)
	if job.Status != "failed" || !strings.Contains(job.Message, "plan changed before it ran") {
		t.Fatalf("removal of a still-desired item was not refused: %+v", job)
	}

	// A generation captured before a status ABA is stale even though the visible state matches.
	e2 := removalJobFixture(t, true)
	e2.deactivate(t)
	old := e2.intentCheck(t)
	e2.reactivate(t)
	e2.deactivate(t)
	job = e2.submit(t, e2.removalPlan(t, old))
	if job.Status != "failed" || !strings.Contains(job.Message, "plan changed before it ran") {
		t.Fatalf("ABA generation approved: %+v", job)
	}

	// Another target's proof, a missing proof and an unknown shape stay with the person.
	e3 := removalJobFixture(t, true)
	e3.deactivate(t)
	foreign := e3.intentCheck(t)
	foreign.TargetID = "other-target"
	job = e3.submit(t, e3.removalPlan(t, foreign))
	if job.Status != "awaiting-approval" {
		t.Fatalf("foreign target proof approved: %+v", job)
	}
	e4 := removalJobFixture(t, true)
	e4.deactivate(t)
	job = e4.submit(t, json.RawMessage(`{"actions":[{"add":false,"skill":{"itemId":"`+e4.in.ItemID+`","slug":"managed"}}]}`))
	if job.Status != "awaiting-approval" {
		t.Fatalf("unproven removal approved: %+v", job)
	}
}

func TestManagedRemovalAcknowledgementIsFencedByCurrentIntent(t *testing.T) {
	e := removalJobFixture(t, true)
	e.deactivate(t)
	approved := e.submit(t, e.removalPlan(t, e.intentCheck(t)))
	if approved.Status != "queued" {
		t.Fatal(approved)
	}
	claimed, err := e.s.ClaimSyncJob(e.uid, e.agent)
	if err != nil || claimed == nil {
		t.Fatal(claimed, err)
	}
	// The item is reactivated while the machine is removing it: the late
	// success must not be recorded as a completed removal.
	e.reactivate(t)
	if err = e.s.FinishSyncJob(e.uid, e.agent.ID, claimed.ID, "succeeded", strings.Repeat("b", 64), "", approved.Plan); err != nil {
		t.Fatal(err)
	}
	job := e.job(t, claimed.ID)
	if job.Status != "failed" || !strings.Contains(job.Message, "plan changed before it ran") {
		t.Fatalf("superseded removal acknowledged: %+v", job)
	}

	// Unchanged intent acknowledges normally.
	ok := removalJobFixture(t, true)
	ok.deactivate(t)
	approved = ok.submit(t, ok.removalPlan(t, ok.intentCheck(t)))
	claimed, _ = ok.s.ClaimSyncJob(ok.uid, ok.agent)
	if err = ok.s.FinishSyncJob(ok.uid, ok.agent.ID, claimed.ID, "succeeded", strings.Repeat("b", 64), "", approved.Plan); err != nil {
		t.Fatal(err)
	}
	if job = ok.job(t, claimed.ID); job.Status != "succeeded" {
		t.Fatalf("current removal not acknowledged: %+v", job)
	}
}
