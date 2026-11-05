package store

import (
	"errors"
	"testing"
)

func softwareBatchFixture(t *testing.T) (*Store, int64, SyncAgent, SyncAgent) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	uid, _ := s.DefaultOwner()
	a, _, err := s.EnrollSyncAgent(uid, "alpha", "test")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := s.EnrollSyncAgent(uid, "beta", "test")
	if err != nil {
		t.Fatal(err)
	}
	return s, uid, a, b
}

func pendingMember(position int, a SyncAgent) SoftwareBatchMemberInput {
	return SoftwareBatchMemberInput{
		Position: position, AgentID: a.ID, Host: a.Host, Action: "update", Target: "1.19.5",
		ExpectedState: "installed", ExpectedVersion: "1.18.0", ExpectedManaged: true,
	}
}

func TestSoftwareBatchCreateReplayAndActiveGuard(t *testing.T) {
	s, uid, a, b := softwareBatchFixture(t)
	members := []SoftwareBatchMemberInput{pendingMember(0, a), pendingMember(1, b)}

	batch, created, err := s.CreateSoftwareBatchFor(uid, "batch_request_0001", "opencode", members)
	if err != nil {
		t.Fatal(err)
	}
	if !created || batch.State != SoftwareBatchPending || len(batch.Members) != 2 {
		t.Fatalf("unexpected batch: %+v created=%v", batch, created)
	}
	if batch.Members[0].Host != a.Host || batch.Members[1].Host != b.Host {
		t.Fatalf("member hosts: %+v", batch.Members)
	}
	if batch.Members[0].Action != "update" || batch.Members[0].Target != "1.19.5" || !batch.Members[0].ExpectedManaged {
		t.Fatalf("frozen evidence: %+v", batch.Members[0])
	}

	replay, createdAgain, err := s.CreateSoftwareBatchFor(uid, "batch_request_0001", "opencode", members)
	if err != nil {
		t.Fatal(err)
	}
	if createdAgain || replay.ID != batch.ID {
		t.Fatalf("replay should return the same batch: %+v created=%v", replay, createdAgain)
	}

	if _, _, err := s.CreateSoftwareBatchFor(uid, "batch_request_0002", "opencode", members); !errors.Is(err, ErrSoftwareBatchInProgress) {
		t.Fatalf("second active batch: %v", err)
	}

	if _, err := s.AbandonSoftwareBatch(uid, batch.ID); err != nil {
		t.Fatal(err)
	}
	ended, err := s.SoftwareBatchFor(uid, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ended.State != SoftwareBatchAbandoned {
		t.Fatalf("abandoned state: %s", ended.State)
	}
	for _, m := range ended.Members {
		if m.State != SoftwareMemberSkipped || m.Code != "abandoned" {
			t.Fatalf("pending member should be skipped: %+v", m)
		}
	}
	// With the first batch abandoned, a new one may start.
	if _, created, err := s.CreateSoftwareBatchFor(uid, "batch_request_0003", "opencode", members); err != nil || !created {
		t.Fatalf("new batch after abandon: %v created=%v", err, created)
	}
}

func TestSoftwareBatchExcludesUnknownMachineAndNeedsOne(t *testing.T) {
	s, uid, a, _ := softwareBatchFixture(t)
	members := []SoftwareBatchMemberInput{
		pendingMember(0, a),
		{Position: 1, AgentID: "missing_machine", Excluded: true, Reason: "That machine isn't enrolled."},
	}
	batch, _, err := s.CreateSoftwareBatchFor(uid, "", "opencode", members)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Members[1].State != SoftwareMemberExcluded || batch.Members[1].Reason == "" {
		t.Fatalf("excluded member: %+v", batch.Members[1])
	}
	if _, err := s.AbandonSoftwareBatch(uid, batch.ID); err != nil {
		t.Fatal(err)
	}

	onlyExcluded := []SoftwareBatchMemberInput{{Position: 0, AgentID: a.ID, Excluded: true, Reason: "Not now."}}
	if _, _, err := s.CreateSoftwareBatchFor(uid, "", "opencode", onlyExcluded); !errors.Is(err, ErrSoftwareBatchNoMembers) {
		t.Fatalf("a batch of only excluded machines: %v", err)
	}
}

func TestSoftwareBatchOutcomesRetrySkipAndAbandon(t *testing.T) {
	s, uid, a, b := softwareBatchFixture(t)
	batch, _, err := s.CreateSoftwareBatchFor(uid, "", "opencode", []SoftwareBatchMemberInput{pendingMember(0, a), pendingMember(1, b)})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SoftwareBatchMemberStart(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SoftwareBatchMemberStart(uid, batch.ID, 0); !errors.Is(err, ErrSoftwareBatchMemberNotPending) {
		t.Fatalf("a running member cannot be claimed again: %v", err)
	}
	if err := s.SetSoftwareBatchMemberState(uid, batch.ID, 0, SoftwareMemberFailed, "offline", ""); err != nil {
		t.Fatal(err)
	}
	stopped, _ := s.SoftwareBatchFor(uid, batch.ID)
	if stopped.State != SoftwareBatchStopped || stopped.Members[1].State != SoftwareMemberPending {
		t.Fatalf("stop should leave later members waiting: %+v", stopped)
	}

	if _, err := s.RetrySoftwareBatchMember(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	retried, _ := s.SoftwareBatchFor(uid, batch.ID)
	if retried.State != SoftwareBatchRunning || retried.Members[0].State != SoftwareMemberPending || retried.Members[0].Code != "" {
		t.Fatalf("retry: %+v", retried)
	}

	if _, err := s.SoftwareBatchMemberStart(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSoftwareBatchMemberState(uid, batch.ID, 0, SoftwareMemberApplied, "", "1.19.5"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SoftwareBatchMemberStart(uid, batch.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSoftwareBatchMemberState(uid, batch.ID, 1, SoftwareMemberApplied, "", "1.19.5"); err != nil {
		t.Fatal(err)
	}
	done, _ := s.SoftwareBatchFor(uid, batch.ID)
	if done.State != SoftwareBatchDone || done.Members[0].State != SoftwareMemberApplied || done.Members[1].State != SoftwareMemberApplied {
		t.Fatalf("done: %+v", done)
	}

	// Skip turns a stopped batch's failure into a finish when nothing waits.
	second, _, err := s.CreateSoftwareBatchFor(uid, "", "opencode", []SoftwareBatchMemberInput{pendingMember(0, a)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SoftwareBatchMemberStart(uid, second.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSoftwareBatchMemberState(uid, second.ID, 0, SoftwareMemberFailed, "checksum_failed", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SkipSoftwareBatchMember(uid, second.ID, 0); err != nil {
		t.Fatal(err)
	}
	skipped, _ := s.SoftwareBatchFor(uid, second.ID)
	if skipped.State != SoftwareBatchDone || skipped.Members[0].State != SoftwareMemberSkipped {
		t.Fatalf("skip: %+v", skipped)
	}

	// Abandon is refused mid-apply and allowed between machines.
	third, _, err := s.CreateSoftwareBatchFor(uid, "", "opencode", []SoftwareBatchMemberInput{pendingMember(0, a)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SoftwareBatchMemberStart(uid, third.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbandonSoftwareBatch(uid, third.ID); !errors.Is(err, ErrSoftwareBatchRunning) {
		t.Fatalf("abandon while running: %v", err)
	}
	if err := s.SetSoftwareBatchMemberState(uid, third.ID, 0, SoftwareMemberApplied, "", "1.19.5"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbandonSoftwareBatch(uid, third.ID); !errors.Is(err, ErrSoftwareBatchFinished) {
		t.Fatalf("abandon a finished batch: %v", err)
	}
}

func TestResumeSoftwareBatchesResetsRunning(t *testing.T) {
	s, uid, a, b := softwareBatchFixture(t)
	batch, _, err := s.CreateSoftwareBatchFor(uid, "", "opencode", []SoftwareBatchMemberInput{pendingMember(0, a), pendingMember(1, b)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SoftwareBatchMemberStart(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	pending, err := s.ResumeSoftwareBatches()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].UID != uid || pending[0].BatchID != batch.ID {
		t.Fatalf("pending batches: %+v", pending)
	}
	resumed, _ := s.SoftwareBatchFor(uid, batch.ID)
	if resumed.State != SoftwareBatchPending || resumed.Members[0].State != SoftwareMemberPending {
		t.Fatalf("resume: %+v", resumed)
	}
}

func TestActiveSoftwareBatchForRestoresTheStoppedBatch(t *testing.T) {
	s, uid, a, _ := softwareBatchFixture(t)
	if _, ok, err := s.ActiveSoftwareBatchFor(uid); err != nil || ok {
		t.Fatalf("no batch yet: ok=%v err=%v", ok, err)
	}
	created, _, err := s.CreateSoftwareBatchFor(uid, "batch_request_active", "opencode", []SoftwareBatchMemberInput{pendingMember(0, a)})
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.ActiveSoftwareBatchFor(uid)
	if err != nil || !ok || got.ID != created.ID || len(got.Members) != 1 {
		t.Fatalf("active batch: %+v ok=%v err=%v", got, ok, err)
	}
	if _, ok, err := s.ActiveSoftwareBatchFor(uid + 1); err != nil || ok {
		t.Fatalf("another person must not see it: ok=%v err=%v", ok, err)
	}
}

// A machine the apply never reached is recorded as skipped. Unlike a failure it
// does not stop the batch, so the machines behind it still get their turn and the
// batch ends done. Only a running member can be given an outcome.
func TestSoftwareBatchSkippedOutcomeDoesNotStopTheBatch(t *testing.T) {
	s, uid, a, b := softwareBatchFixture(t)
	batch, _, err := s.CreateSoftwareBatchFor(uid, "", "opencode", []SoftwareBatchMemberInput{pendingMember(0, a), pendingMember(1, b)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSoftwareBatchMemberState(uid, batch.ID, 0, SoftwareMemberSkipped, "offline", ""); !errors.Is(err, ErrSoftwareBatchMemberNotRunning) {
		t.Fatalf("a member that never started was given an outcome: %v", err)
	}
	if _, err := s.SoftwareBatchMemberStart(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSoftwareBatchMemberState(uid, batch.ID, 0, SoftwareMemberSkipped, "offline", ""); err != nil {
		t.Fatalf("skipped is not accepted as an outcome: %v", err)
	}
	running, _ := s.SoftwareBatchFor(uid, batch.ID)
	if running.State != SoftwareBatchRunning || running.Members[0].State != SoftwareMemberSkipped || running.Members[0].Code != "offline" || running.Members[1].State != SoftwareMemberPending {
		t.Fatalf("a skipped machine should leave the batch running for the next: %+v", running)
	}
	if _, err := s.SoftwareBatchMemberStart(uid, batch.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSoftwareBatchMemberState(uid, batch.ID, 1, SoftwareMemberApplied, "", "1.19.5"); err != nil {
		t.Fatal(err)
	}
	done, _ := s.SoftwareBatchFor(uid, batch.ID)
	if done.State != SoftwareBatchDone || done.Members[0].State != SoftwareMemberSkipped || done.Members[1].State != SoftwareMemberApplied {
		t.Fatalf("a batch with a skipped machine should finish: %+v", done)
	}
	// A skip with no usable code still says it was skipped.
	other, _, err := s.CreateSoftwareBatchFor(uid, "", "opencode", []SoftwareBatchMemberInput{pendingMember(0, a)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SoftwareBatchMemberStart(uid, other.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSoftwareBatchMemberState(uid, other.ID, 0, SoftwareMemberSkipped, "Not A Code", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.SoftwareBatchFor(uid, other.ID); got.Members[0].Code != "skipped" {
		t.Fatalf("an unusable code was stored as given: %+v", got.Members[0])
	}
}
