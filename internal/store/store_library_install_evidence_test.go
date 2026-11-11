package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// finishedApply runs an automatic job to success with the given plan, the way a
// machine reports an apply, and returns the job id.
func finishedApply(t *testing.T, e removalJobEnv, plan json.RawMessage, status string) string {
	t.Helper()
	job := e.submit(t, plan)
	if job.Status != "queued" || job.Kind != "apply" {
		t.Fatalf("additive plan was not approved: %+v", job)
	}
	claimed, err := e.s.ClaimSyncJob(e.uid, e.agent)
	if err != nil || claimed == nil {
		t.Fatal(claimed, err)
	}
	if err = e.s.FinishSyncJob(e.uid, e.agent.ID, claimed.ID, status, strings.Repeat("b", 64), "", plan); err != nil {
		t.Fatal(err)
	}
	return claimed.ID
}

func addPlan(t *testing.T, check LibraryDeploymentCheck, source string, replace bool) json.RawMessage {
	t.Helper()
	plan, err := json.Marshal(map[string]any{"target": check.TargetID, "actions": []any{map[string]any{"add": true, "replace": replace, "skill": map[string]any{"itemId": check.ItemID, "slug": "managed", "sourceType": "private", "source": source}}}})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestLibraryInstallEvidenceProvesOnlyWhatAnApplyInstalled(t *testing.T) {
	e := localRestoreFixture(t)
	source := "private:" + strings.Repeat("c", 32) + ":" + e.in.RevisionID + ":" + strings.Repeat("9", 64)
	finishedApply(t, e, addPlan(t, e.in, source, false), "succeeded")

	got, err := e.s.LibraryInstallEvidenceFor(e.uid, e.agent.ID, e.in)
	if err != nil || !got.Installed || got.JobID == "" {
		t.Fatal("an applied install was not recognised", got, err)
	}

	// A revision that is not the machine's pinned base proves nothing.
	other := e.in
	other.RevisionID = strings.Repeat("7", 32)
	if _, err = e.s.LibraryInstallEvidenceFor(e.uid, e.agent.ID, other); err == nil {
		t.Fatal("a revision that is not the pinned base was accepted")
	}
}

func TestLibraryInstallEvidenceNeedsTheSameRevisionAndTarget(t *testing.T) {
	// The apply installed a different revision than the one now pinned.
	differs := localRestoreFixture(t)
	finishedApply(t, differs, addPlan(t, differs.in, "private:"+strings.Repeat("c", 32)+":"+strings.Repeat("7", 32)+":"+strings.Repeat("9", 64), false), "succeeded")
	if got, err := differs.s.LibraryInstallEvidenceFor(differs.uid, differs.agent.ID, differs.in); err != nil || got.Installed {
		t.Fatal("an apply of another revision counted", got, err)
	}
	// The same install, asked about another target, is not evidence there.
	same := localRestoreFixture(t)
	finishedApply(t, same, addPlan(t, same.in, "private:"+strings.Repeat("c", 32)+":"+same.in.RevisionID+":"+strings.Repeat("9", 64), true), "succeeded")
	if got, err := same.s.LibraryInstallEvidenceFor(same.uid, same.agent.ID, same.in); err != nil || !got.Installed {
		t.Fatal("a replacing install was not recognised", got, err)
	}
	elsewhere := same.in
	elsewhere.TargetID = "someone-elses-target"
	if got, err := same.s.LibraryInstallEvidenceFor(same.uid, same.agent.ID, elsewhere); err != nil || got.Installed {
		t.Fatal("another target's install counted", got, err)
	}
}

func TestLibraryInstallEvidenceIgnoresRemovalsFailuresAndOtherTargets(t *testing.T) {
	// Only a succeeded apply that added this revision counts: a failed apply does not.
	failed := localRestoreFixture(t)
	source := "private:" + strings.Repeat("c", 32) + ":" + failed.in.RevisionID + ":" + strings.Repeat("9", 64)
	failedJob := failed.submit(t, addPlan(t, failed.in, source, false))
	claimed, _ := failed.s.ClaimSyncJob(failed.uid, failed.agent)
	if err := failed.s.FinishSyncJob(failed.uid, failed.agent.ID, claimed.ID, "failed", "", "installer failed", nil); err != nil {
		t.Fatal(err)
	}
	_ = failedJob
	if got, err := failed.s.LibraryInstallEvidenceFor(failed.uid, failed.agent.ID, failed.in); err != nil || got.Installed {
		t.Fatal("a failed apply counted as an install", got, err)
	}

	// A plan with no add of this item is not evidence, even if it succeeded.
	none := localRestoreFixture(t)
	finishedApply(t, none, json.RawMessage(`{"actions":[{"add":true,"skill":{"itemId":"`+strings.Repeat("0", 32)+`","slug":"other","source":"private:x:y:z"}}]}`), "succeeded")
	if got, err := none.s.LibraryInstallEvidenceFor(none.uid, none.agent.ID, none.in); err != nil || got.Installed {
		t.Fatal("an apply of another item counted", got, err)
	}
}

func TestLibraryInstallEvidenceRequiresTheExactEnrollmentAndAccount(t *testing.T) {
	e := localRestoreFixture(t)
	source := "private:" + strings.Repeat("c", 32) + ":" + e.in.RevisionID + ":" + strings.Repeat("9", 64)
	finishedApply(t, e, addPlan(t, e.in, source, false), "succeeded")
	other, _, err := e.s.EnrollSyncAgent(e.uid, "other-host", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.s.LibraryInstallEvidenceFor(e.uid, other.ID, e.in); err == nil {
		t.Fatal("another machine read install evidence")
	}
	if _, err = e.s.LibraryInstallEvidenceFor(e.uid+1, e.agent.ID, e.in); err == nil {
		t.Fatal("another account read install evidence")
	}
	bad := e.in
	bad.BindingID = "short"
	if _, err = e.s.LibraryInstallEvidenceFor(e.uid, e.agent.ID, bad); err == nil {
		t.Fatal("a malformed binding was accepted")
	}
}
