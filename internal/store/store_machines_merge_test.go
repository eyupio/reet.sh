package store

import (
	"errors"
	"testing"
)

// Two installed agents cannot be merged: the refusal is one callers can
// recognise, and nothing moves.
func TestMergingTwoEnrolledMachinesIsRefused(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	for _, host := range []string{"laptop", "desktop"} {
		if _, _, err := s.EnrollSyncAgent(uid, host, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MergeMachinesFor(uid, "laptop", "desktop"); !errors.Is(err, ErrMergeTwoAgents) {
		t.Fatalf("merge of two enrolments: %v", err)
	}
	if agents, _ := s.SyncAgentsFor(uid); len(agents) != 2 || agents[0].Host != "desktop" || agents[1].Host != "laptop" {
		t.Fatalf("a refused merge renamed a machine: %+v", agents)
	}
}

// A rename changes the machine's host and nothing a job holds (jobs hold the
// agent id), so it neither waits for a running update nor cancels a queued one.
func TestRenamingAMachineLeavesItsUpdateJobsAlone(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.EnrollSyncAgent(uid, "desktop", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	running, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := s.ClaimSyncJob(uid, a); err != nil || claimed == nil || claimed.ID != running.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	queued, err := s.QueueAgentUpdate(uid, other.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MergeMachinesFor(uid, "laptop", "work-laptop"); err != nil {
		t.Fatalf("rename while an update runs: %v", err)
	}
	if err = s.MergeMachinesFor(uid, "desktop", "work-desktop"); err != nil {
		t.Fatalf("rename while an update is queued: %v", err)
	}
	status := map[string]string{}
	jobs, _ := s.SyncJobsFor(uid)
	for _, j := range jobs {
		status[j.ID] = j.Status
	}
	if status[running.ID] != "running" || status[queued.ID] != "queued" {
		t.Fatalf("a rename changed the jobs: %v", status)
	}
	if err = s.FinishSyncJob(uid, a.ID, running.ID, "succeeded", "Updated."); err != nil {
		t.Fatalf("the renamed machine could not report its update: %v", err)
	}
}
