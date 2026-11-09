package store

import (
	"strings"
	"testing"
)

func localRestoreFixture(t *testing.T) removalJobEnv {
	t.Helper()
	e := removalJobFixture(t, false)
	if err := e.s.refreshCanonicalSelections(e.uid, e.agent); err != nil {
		t.Fatal(err)
	}
	if err := e.s.ControlSyncAgent(e.uid, e.agent.ID, "keep-sync-on"); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestLibraryRestoreSyncIsQueuedOnlyForCurrentIntentAndOnce(t *testing.T) {
	e := localRestoreFixture(t)
	queued, err := e.s.QueueLibraryRestoreSyncFor(e.uid, e.agent.ID, e.in)
	if err != nil || !queued {
		t.Fatal("a stable deletion of a wanted item did not queue a sync", queued, err)
	}
	// A second report while that job is pending queues nothing more.
	if queued, err = e.s.QueueLibraryRestoreSyncFor(e.uid, e.agent.ID, e.in); err != nil || queued {
		t.Fatal("a repeated report queued another job", queued, err)
	}
	jobs, _ := e.s.SyncJobsFor(e.uid)
	if len(jobs) != 1 || jobs[0].Kind != "auto" || jobs[0].Status != "queued" {
		t.Fatalf("%+v", jobs)
	}

	// An item that is no longer wanted is never restored.
	off := localRestoreFixture(t)
	off.deactivate(t)
	if queued, err = off.s.QueueLibraryRestoreSyncFor(off.uid, off.agent.ID, off.in); err != nil || queued {
		t.Fatal("an inactive item queued a restore", queued, err)
	}
	// A machine that is not keeping the library synced does not accumulate work.
	manual := localRestoreFixture(t)
	if err = manual.s.ControlSyncAgent(manual.uid, manual.agent.ID, "keep-sync-off"); err != nil {
		t.Fatal(err)
	}
	if queued, err = manual.s.QueueLibraryRestoreSyncFor(manual.uid, manual.agent.ID, manual.in); err != nil || queued {
		t.Fatal("a machine with sync off queued a restore", queued, err)
	}
	// A paused machine does not accumulate work.
	paused := localRestoreFixture(t)
	if err = paused.s.ControlSyncAgent(paused.uid, paused.agent.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	// The shared agent check already refuses a paused machine, as it does for intent reads.
	if queued, _ = paused.s.QueueLibraryRestoreSyncFor(paused.uid, paused.agent.ID, paused.in); queued {
		t.Fatal("a paused machine queued a restore")
	}
}

func TestLibraryRestoreReceiptCreatesOneFactualNotification(t *testing.T) {
	e := localRestoreFixture(t)
	receipt := LibraryRestoreReceipt{LibraryDeploymentCheck: e.in, Episode: strings.Repeat("e", 32), Files: 2}
	first, created, err := e.s.RecordLibraryRestoreFor(e.uid, e.agent.ID, receipt)
	if err != nil || !created || first.ID == 0 {
		t.Fatal(first, created, err)
	}
	if !strings.HasPrefix(first.Title, "Restored managed on ") || !strings.Contains(first.Title, "after it was deleted locally") || first.Tone != "info" || first.Kind != "library_restore" {
		t.Fatalf("notification is not factual: %+v", first)
	}
	// Retry, restart and replay of the same episode tell nobody twice.
	again, created, err := e.s.RecordLibraryRestoreFor(e.uid, e.agent.ID, receipt)
	if err != nil || created || again.ID != first.ID {
		t.Fatal("a replayed receipt created another notification", again, created, err)
	}
	// A different deletion episode is a different event.
	receipt.Episode = strings.Repeat("f", 32)
	other, created, err := e.s.RecordLibraryRestoreFor(e.uid, e.agent.ID, receipt)
	if err != nil || !created || other.ID == first.ID {
		t.Fatal(other, created, err)
	}
	list, _ := e.s.NotificationsFor(e.uid, false, 10)
	if len(list) != 2 {
		t.Fatalf("%d notifications", len(list))
	}
}

func TestLibraryRestoreReceiptIsNotAnnouncedForUnwantedOrForeignItems(t *testing.T) {
	// The item was deactivated while the machine restored it: no notice, but the receipt is accepted so it is not retried forever.
	e := localRestoreFixture(t)
	e.deactivate(t)
	got, created, err := e.s.RecordLibraryRestoreFor(e.uid, e.agent.ID, LibraryRestoreReceipt{LibraryDeploymentCheck: e.in, Episode: strings.Repeat("e", 32)})
	if err != nil || created || got.ID != 0 {
		t.Fatal("a notice was created for an item that is no longer wanted", got, created, err)
	}
	// Another machine, another account and a malformed episode are refused.
	f := localRestoreFixture(t)
	other, _, err := f.s.EnrollSyncAgent(f.uid, "other-host", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.RecordLibraryRestoreFor(f.uid, other.ID, LibraryRestoreReceipt{LibraryDeploymentCheck: f.in, Episode: strings.Repeat("e", 32)}); err == nil {
		t.Fatal("another machine reported a restore")
	}
	if _, _, err = f.s.RecordLibraryRestoreFor(f.uid+1, f.agent.ID, LibraryRestoreReceipt{LibraryDeploymentCheck: f.in, Episode: strings.Repeat("e", 32)}); err == nil {
		t.Fatal("another account reported a restore")
	}
	if _, _, err = f.s.RecordLibraryRestoreFor(f.uid, f.agent.ID, LibraryRestoreReceipt{LibraryDeploymentCheck: f.in, Episode: "not-an-episode"}); err == nil {
		t.Fatal("a malformed episode was accepted")
	}
}
