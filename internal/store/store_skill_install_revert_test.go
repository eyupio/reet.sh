package store

import (
	"database/sql"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// claimInstall hands the queued install job to the fixture's agent.
func (e *installEnv) claim(t *testing.T) *SyncJob {
	t.Helper()
	j, err := e.s.ClaimSyncJob(e.uid, e.agent)
	if err != nil || j == nil {
		t.Fatal(j, err)
	}
	return j
}

// fail claims the queued install job and reports it failed.
func (e *installEnv) fail(t *testing.T) {
	t.Helper()
	j := e.claim(t)
	if err := e.s.FinishSyncJob(e.uid, e.agent.ID, j.ID, "failed", "", "npx exited 1", nil); err != nil {
		t.Fatal(err)
	}
}

// awaitApproval claims the queued install job and reports an additive plan.
func (e *installEnv) awaitApproval(t *testing.T, slug string) string {
	t.Helper()
	j := e.claim(t)
	hash := strings.Repeat("a", 64)
	if err := e.s.FinishSyncJob(e.uid, e.agent.ID, j.ID, "awaiting-approval", hash, "", []byte(`{"actions":[{"add":true,"skill":{"slug":"`+slug+`"}}]}`)); err != nil {
		t.Fatal(err)
	}
	return hash
}

// existingSet saves a mirror set holding a/b/one, which is already on disk, so
// Keep synced has nothing to do for it.
func (e *installEnv) existingSet(t *testing.T) SyncTarget {
	t.Helper()
	e.s.UpsertFor(e.uid, mustRef(t, "a/b/one"), Patch{})
	if err := e.s.RecordEnvironmentFor(e.uid, "a/b/one", "laptop", "claude", "", "/home/u/.claude/skills", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.s.RecordInventoryFor(e.uid, e.env, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.SaveDesiredFor(e.uid, "owner", DesiredSelection{TargetID: e.env, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	return e.target(t)
}

func (e *installEnv) jobStatus(t *testing.T, id string) string {
	t.Helper()
	var status string
	if err := e.s.db.QueryRow(`SELECT status FROM sync_jobs WHERE id=?`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (e *installEnv) noSetLeft(t *testing.T) {
	t.Helper()
	for _, table := range []string{"sync_targets", "sync_policies", "sync_options"} {
		var n int
		e.s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE target_id=?`, e.env).Scan(&n)
		if n != 0 {
			t.Fatal(table, "row left behind")
		}
	}
}

func (e *installEnv) sameSet(t *testing.T, want SyncTarget) {
	t.Helper()
	got := e.target(t)
	if got.Revision != want.Revision || !reflect.DeepEqual(slugs(got.Desired), slugs(want.Desired)) || got.Mode != want.Mode || got.SourceID != want.SourceID {
		t.Fatalf("saved set %+v, want %+v", got, want)
	}
}

func TestFailedInstallRevertsTheSavedSet(t *testing.T) {
	t.Run("created set", func(t *testing.T) {
		e := installFixture(t)
		res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
		if err != nil {
			t.Fatal(err)
		}
		e.fail(t)
		if e.jobStatus(t, res.JobID) != "failed" {
			t.Fatal("job not failed")
		}
		e.noSetLeft(t)
	})
	t.Run("existing set", func(t *testing.T) {
		e := installFixture(t)
		before := e.existingSet(t)
		if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
			t.Fatal(err)
		}
		e.fail(t)
		e.sameSet(t, before)
	})
	t.Run("failed apply", func(t *testing.T) {
		e := installFixture(t)
		before := e.existingSet(t)
		res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
		if err != nil {
			t.Fatal(err)
		}
		hash := e.awaitApproval(t, "pdf")
		if err := e.s.ControlSyncJob(e.uid, res.JobID, "approve", hash); err != nil {
			t.Fatal(err)
		}
		e.fail(t)
		e.sameSet(t, before)
	})
}

func TestSucceededInstallKeepsTheSkill(t *testing.T) {
	e := installFixture(t)
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	hash := e.awaitApproval(t, "pdf")
	if err := e.s.ControlSyncJob(e.uid, res.JobID, "approve", hash); err != nil {
		t.Fatal(err)
	}
	j := e.claim(t)
	if err := e.s.FinishSyncJob(e.uid, e.agent.ID, j.ID, "succeeded", hash, "", nil); err != nil {
		t.Fatal(err)
	}
	if got := slugs(e.target(t).Desired); !reflect.DeepEqual(got, []string{"pdf"}) {
		t.Fatalf("%v", got)
	}
}

func TestCancelFromTheSyncPageRevertsAnInstall(t *testing.T) {
	for _, awaiting := range []bool{false, true} {
		name := "queued"
		if awaiting {
			name = "awaiting approval"
		}
		t.Run(name, func(t *testing.T) {
			e := installFixture(t)
			before := e.existingSet(t)
			res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
			if err != nil {
				t.Fatal(err)
			}
			if awaiting {
				e.awaitApproval(t, "pdf")
			}
			if err := e.s.ControlSyncJob(e.uid, res.JobID, "cancel", ""); err != nil {
				t.Fatal(err)
			}
			if e.jobStatus(t, res.JobID) != "cancelled" {
				t.Fatal("not cancelled")
			}
			e.sameSet(t, before)
		})
	}
	t.Run("created set", func(t *testing.T) {
		e := installFixture(t)
		res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
		if err != nil {
			t.Fatal(err)
		}
		if err := e.s.ControlSyncJob(e.uid, res.JobID, "cancel", ""); err != nil {
			t.Fatal(err)
		}
		e.noSetLeft(t)
	})
}

func TestAgentTimeoutOfAnInstallRevertsIt(t *testing.T) {
	reapers := map[string]func(e *installEnv) error{
		"jobs list": func(e *installEnv) error { _, err := e.s.SyncJobsFor(e.uid); return err },
		"claim":     func(e *installEnv) error { _, err := e.s.ClaimSyncJob(e.uid, e.agent); return err },
		"update":    func(e *installEnv) error { _, err := e.s.QueueAgentUpdate(e.uid, e.agent.ID, "latest"); return err },
	}
	for name, reap := range reapers {
		t.Run(name, func(t *testing.T) {
			e := installFixture(t)
			before := e.existingSet(t)
			res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
			if err != nil {
				t.Fatal(err)
			}
			e.claim(t)
			e.s.db.Exec(`UPDATE sync_jobs SET updated_at=? WHERE id=?`, time.Now().Add(-11*time.Minute).UnixMilli(), res.JobID)
			if err := reap(e); err != nil {
				t.Fatal(err)
			}
			if e.jobStatus(t, res.JobID) != "failed" {
				t.Fatal("not reaped")
			}
			e.sameSet(t, before)
		})
	}
}

func TestOtherCancellationsRevertAnInstall(t *testing.T) {
	t.Run("desired set removed", func(t *testing.T) {
		e := installFixture(t)
		res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
		if err != nil {
			t.Fatal(err)
		}
		if err := e.s.DeleteSyncTargetFor(e.uid, e.env); err != nil {
			t.Fatal(err)
		}
		if e.jobStatus(t, res.JobID) != "cancelled" {
			t.Fatal("not cancelled")
		}
		e.noSetLeft(t)
		// A set saved afterwards is never touched by the old install row.
		e.saveHostedSet(t, "additive", "pdf")
		later := e.target(t)
		if _, err := e.s.CancelInstallFor(e.uid, res.JobID); err == nil {
			t.Fatal("cancelled a finished install")
		}
		e.sameSet(t, later)
	})
	t.Run("machine renamed", func(t *testing.T) {
		e := installFixture(t)
		if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
			t.Fatal(err)
		}
		if err := e.s.MergeMachinesFor(e.uid, "laptop", "work-laptop"); err != nil {
			t.Fatal(err)
		}
		var n int
		e.s.db.QueryRow(`SELECT COUNT(*) FROM sync_targets WHERE user_id=?`, e.uid).Scan(&n)
		if n != 0 {
			t.Fatal("the install's set survived the rename")
		}
	})
	t.Run("machine renamed, existing set", func(t *testing.T) {
		e := installFixture(t)
		before := e.existingSet(t)
		if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
			t.Fatal(err)
		}
		if err := e.s.MergeMachinesFor(e.uid, "laptop", "work-laptop"); err != nil {
			t.Fatal(err)
		}
		e.env = EnvironmentIDFor(e.uid, "work-laptop", "claude", "")
		got := e.target(t)
		if got.Revision != before.Revision || !reflect.DeepEqual(slugs(got.Desired), slugs(before.Desired)) {
			t.Fatalf("saved set %+v, want %+v", got, before)
		}
	})
	t.Run("machine revoked", func(t *testing.T) {
		e := installFixture(t)
		before := e.existingSet(t)
		if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
			t.Fatal(err)
		}
		if err := e.s.ControlSyncAgent(e.uid, e.agent.ID, "revoke"); err != nil {
			t.Fatal(err)
		}
		e.sameSet(t, before)
	})
}

func TestAbandonedInstallCannotBeAppliedByKeepSynced(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "no saved set"
		if existing {
			name = "existing set"
		}
		t.Run(name, func(t *testing.T) {
			e := installFixture(t)
			var before SyncTarget
			if existing {
				before = e.existingSet(t)
			}
			if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
				t.Fatal(err)
			}
			e.fail(t)
			res, err := e.s.InstallHostedSkillFor(e.uid, e.req("docx"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.s.CancelInstallFor(e.uid, res.JobID); err != nil {
				t.Fatal(err)
			}
			agent := e.agent
			agent.KeepSynced = true
			if err := e.s.MaintainSync(e.uid, "owner", agent); err != nil {
				t.Fatal(err)
			}
			var auto int
			e.s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE kind IN ('auto','apply') OR status IN ('queued','running','awaiting-approval')`).Scan(&auto)
			if auto != 0 {
				jobs, _ := e.s.SyncJobsFor(e.uid)
				t.Fatalf("Keep synced queued work: %+v", jobs)
			}
			if existing {
				e.sameSet(t, before)
			} else {
				e.noSetLeft(t)
			}
		})
	}
}

func TestRevertRestoresThePreviousRevision(t *testing.T) {
	e := installFixture(t)
	before := e.existingSet(t)
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	var prev string
	if err := e.s.db.QueryRow(`SELECT prev_revision FROM skill_installs WHERE job_id=?`, res.JobID).Scan(&prev); err != nil || prev != before.Revision {
		t.Fatal(prev, err)
	}
	reverted, err := e.s.CancelInstallFor(e.uid, res.JobID)
	if err != nil || !reverted {
		t.Fatal(reverted, err)
	}
	e.sameSet(t, before)
}

func TestKeepSyncedHoldsOffWhileAnInstallIsPending(t *testing.T) {
	e := installFixture(t)
	e.existingSet(t)
	if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
		t.Fatal(err)
	}
	e.awaitApproval(t, "pdf")
	agent := e.agent
	agent.KeepSynced = true
	if err := e.s.MaintainSync(e.uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	var auto int
	e.s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE kind!='preview'`).Scan(&auto)
	if auto != 0 {
		t.Fatal("Keep synced queued a job beside the install")
	}
}

// projectSet saves a second set on the same machine, for a project folder.
func (e *installEnv) projectSet(t *testing.T) string {
	t.Helper()
	if err := e.s.TouchEnvironmentFor(e.uid, "laptop", "claude", "/work/app", "/work/app/.claude/skills"); err != nil {
		t.Fatal(err)
	}
	id := EnvironmentIDFor(e.uid, "laptop", "claude", "/work/app")
	if err := e.s.RecordInventoryFor(e.uid, id, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	e.s.UpsertFor(e.uid, mustRef(t, "a/b/two"), Patch{})
	if err := e.s.SaveDesiredFor(e.uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/two"}}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRunSkipsATargetWithAPendingInstall(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		e := installFixture(t)
		project := e.projectSet(t)
		res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
		if err != nil {
			t.Fatal(err)
		}
		e.awaitApproval(t, "pdf")
		var targets []string
		if explicit {
			targets = []string{e.env, project}
		}
		run, err := e.s.StartSyncRun(e.uid, e.agent.ID, targets)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(run.Targets, []string{project}) {
			t.Fatalf("run targets %v", run.Targets)
		}
		var onInstallTarget int
		e.s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE target_id=? AND id!=?`, e.env, res.JobID).Scan(&onInstallTarget)
		if onInstallTarget != 0 {
			t.Fatal("a job was queued beside the install")
		}
		if e.jobStatus(t, res.JobID) != "awaiting-approval" {
			t.Fatal("install job changed")
		}
	}
}

func TestRunRefusesWhenOnlyPendingInstallsRemain(t *testing.T) {
	for _, targets := range [][]string{nil, {"env"}} {
		e := installFixture(t)
		if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
			t.Fatal(err)
		}
		if targets != nil {
			targets = []string{e.env}
		}
		before := e.dump(t)
		_, err := e.s.StartSyncRun(e.uid, e.agent.ID, targets)
		if err == nil || err.Error() != "an install is waiting on this folder; finish or cancel it first" {
			t.Fatal(err)
		}
		if e.dump(t) != before {
			t.Fatal("refusal wrote")
		}
	}
}

func TestSaveDesiredAcceptsAnInstalledHostedSkillForShelfListAndMachineSources(t *testing.T) {
	sources := map[string]func(t *testing.T, e *installEnv) DesiredSelection{
		"shelf": func(t *testing.T, e *installEnv) DesiredSelection {
			e.s.UpsertFor(e.uid, mustRef(t, "a/b/one"), Patch{})
			return DesiredSelection{TargetID: e.env, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}}
		},
		"list": func(t *testing.T, e *installEnv) DesiredSelection {
			e.s.UpsertFor(e.uid, mustRef(t, "a/b/one"), Patch{})
			list, err := e.s.CreateList(e.uid, "Kit", "", "private")
			if err != nil {
				t.Fatal(err)
			}
			if err := e.s.AddListSkill(list.ID, "a/b/one", "", e.uid); err != nil {
				t.Fatal(err)
			}
			return DesiredSelection{TargetID: e.env, ListID: list.ID, Mode: "additive", SkillIDs: []string{"a/b/one"}}
		},
		"machine": func(t *testing.T, e *installEnv) DesiredSelection {
			e.s.UpsertFor(e.uid, mustRef(t, "a/b/one"), Patch{})
			if err := e.s.RecordEnvironmentFor(e.uid, "a/b/one", "desk", "claude", "", "/home/d/.claude/skills", ""); err != nil {
				t.Fatal(err)
			}
			desk := EnvironmentIDFor(e.uid, "desk", "claude", "")
			if err := e.s.RecordInventoryFor(e.uid, desk, true, MachineInfo{}); err != nil {
				t.Fatal(err)
			}
			return DesiredSelection{TargetID: e.env, SourceID: desk, Mode: "additive", SkillIDs: []string{"a/b/one"}}
		},
	}
	for name, prepare := range sources {
		t.Run(name, func(t *testing.T) {
			e := installFixture(t)
			in := prepare(t, e)
			if err := e.s.SaveDesiredFor(e.uid, "owner", in); err != nil {
				t.Fatal(err)
			}
			res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
			if err != nil {
				t.Fatal(err)
			}
			hash := e.awaitApproval(t, "pdf")
			if err := e.s.ControlSyncJob(e.uid, res.JobID, "approve", hash); err != nil {
				t.Fatal(err)
			}
			// The install applies first: a re-save is refused while it is pending.
			j := e.claim(t)
			if err := e.s.FinishSyncJob(e.uid, e.agent.ID, j.ID, "succeeded", hash, "", nil); err != nil {
				t.Fatal(err)
			}
			tg := e.target(t)
			in.Revision = tg.Revision
			in.SkillIDs = nil
			for _, sk := range tg.Desired {
				in.SkillIDs = append(in.SkillIDs, sk.ID)
			}
			// Without an origin the hosted skill is not a candidate, as before.
			if err := e.s.SaveDesiredFor(e.uid, "owner", in); err == nil || !strings.Contains(err.Error(), "selected skills are unavailable") {
				t.Fatal("saved without an origin:", err)
			}
			in.Origin = installOrigin
			if err := e.s.SaveDesiredFor(e.uid, "owner", in); err != nil {
				t.Fatal(err)
			}
			if got := slugs(e.target(t).Desired); !reflect.DeepEqual(got, []string{"one", "pdf"}) {
				t.Fatalf("%v", got)
			}
		})
	}
}

func TestSaveDesiredWithoutAnOriginStillSavesShelfSkills(t *testing.T) {
	e := installFixture(t)
	e.s.UpsertFor(e.uid, mustRef(t, "a/b/one"), Patch{})
	if err := e.s.SaveDesiredFor(e.uid, "owner", DesiredSelection{TargetID: e.env, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	// A broken origin is ignored rather than failing the save.
	tg := e.target(t)
	if err := e.s.SaveDesiredFor(e.uid, "owner", DesiredSelection{TargetID: e.env, SourceID: "shelf", Mode: "additive", Origin: "not a url", Revision: tg.Revision, SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
}

func TestLeftoverFollowPolicyDoesNotBlockAnInstall(t *testing.T) {
	e := installFixture(t)
	e.s.db.Exec(`INSERT INTO sync_policies(target_id,follow_source) VALUES(?,1)`, e.env)
	if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
		t.Fatal(err)
	}
	if tg := e.target(t); tg.FollowSource {
		t.Fatal("the new set follows a source")
	}
}

func TestDeletingASavedSetRemovesItsPolicy(t *testing.T) {
	e := installFixture(t)
	e.s.RecordEnvironmentFor(e.uid, "a/b/one", "other", "claude", "", "", "")
	if err := e.s.SaveDesiredFor(e.uid, "owner", DesiredSelection{TargetID: e.env, SourceID: "shelf", Mode: "mirror", FollowSource: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.DeleteSyncTargetFor(e.uid, e.env); err != nil {
		t.Fatal(err)
	}
	e.noSetLeft(t)
}

func TestCancelOfAFinishedInstallSaysFinished(t *testing.T) {
	for _, status := range []string{"succeeded", "failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			e := installFixture(t)
			res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
			if err != nil {
				t.Fatal(err)
			}
			e.s.db.Exec(`UPDATE sync_jobs SET status=? WHERE id=?`, status, res.JobID)
			before := e.dump(t)
			_, err = e.s.CancelInstallFor(e.uid, res.JobID)
			wantInstallError(t, err, "cannot-cancel", "This install has already finished.")
			if e.dump(t) != before {
				t.Fatal("refusal wrote")
			}
		})
	}
}

// mergeTarget records a complete scan of the project folder /work/app on laptop.
func (e *installEnv) mergeTarget(t *testing.T) string {
	t.Helper()
	if err := e.s.TouchEnvironmentFor(e.uid, "laptop", "claude", "/work/app", "/work/app/.claude/skills"); err != nil {
		t.Fatal(err)
	}
	id := EnvironmentIDFor(e.uid, "laptop", "claude", "/work/app")
	if err := e.s.RecordInventoryFor(e.uid, id, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestMergeEnvironmentsRevertsAPendingInstallBeforeTheCopy(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "no saved set"
		if existing {
			name = "existing set"
		}
		t.Run(name, func(t *testing.T) {
			e := installFixture(t)
			var before SyncTarget
			if existing {
				before = e.existingSet(t)
			}
			dest := e.mergeTarget(t)
			res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.s.MergeEnvironmentsFor(e.uid, e.env, dest); err != nil {
				t.Fatal(err)
			}
			if e.jobStatus(t, res.JobID) != "cancelled" {
				t.Fatal("the install was not cancelled by the merge")
			}
			if _, err := e.s.CancelInstallFor(e.uid, res.JobID); err == nil {
				t.Fatal("cancelled an install the merge had ended")
			}
			agent := e.agent
			agent.KeepSynced = true
			if err := e.s.MaintainSync(e.uid, "owner", agent); err != nil {
				t.Fatal(err)
			}
			var queued int
			e.s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE kind!='preview' OR status IN ('queued','running','awaiting-approval')`).Scan(&queued)
			if queued != 0 {
				jobs, _ := e.s.SyncJobsFor(e.uid)
				t.Fatalf("Keep synced queued work: %+v", jobs)
			}
			e.env = dest
			if existing {
				e.sameSet(t, before)
			} else {
				e.noSetLeft(t)
			}
		})
	}
	t.Run("running job refuses", func(t *testing.T) {
		e := installFixture(t)
		dest := e.mergeTarget(t)
		if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
			t.Fatal(err)
		}
		e.claim(t)
		before := e.dump(t)
		if _, err := e.s.MergeEnvironmentsFor(e.uid, e.env, dest); !errors.Is(err, ErrEnvironmentJobRunning) {
			t.Fatal(err)
		}
		if e.dump(t) != before {
			t.Fatal("refusal wrote")
		}
	})
}

const installPendingSentence = "An install is waiting on this folder. Finish or cancel it first."

func TestSaveDesiredRefusedWhileAnInstallIsPending(t *testing.T) {
	resave := func(t *testing.T, e *installEnv) error {
		t.Helper()
		e.s.UpsertFor(e.uid, mustRef(t, "a/b/two"), Patch{})
		tg := e.target(t)
		ids := []string{"a/b/two"}
		for _, sk := range tg.Desired {
			ids = append(ids, sk.ID)
		}
		return e.s.SaveDesiredFor(e.uid, "owner", DesiredSelection{TargetID: e.env, SourceID: "shelf", Mode: "additive", Origin: installOrigin, Revision: tg.Revision, SkillIDs: ids})
	}
	for _, status := range []string{"queued", "running", "awaiting-approval"} {
		t.Run(status, func(t *testing.T) {
			e := installFixture(t)
			e.existingSet(t)
			res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
			if err != nil {
				t.Fatal(err)
			}
			switch status {
			case "running":
				e.claim(t)
			case "awaiting-approval":
				e.awaitApproval(t, "pdf")
			}
			// Another folder on the same machine is unaffected.
			e.projectSet(t)
			before := e.dump(t)
			if err := resave(t, e); err == nil || err.Error() != installPendingSentence {
				t.Fatal(err)
			}
			if e.dump(t) != before {
				t.Fatal("refusal wrote")
			}
			// Reusing the install for the same skill still works.
			if again, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil || again.JobID != res.JobID {
				t.Fatal(again, err)
			}
			// Once the install has ended, a re-save works again.
			if status == "running" {
				if err := e.s.FinishSyncJob(e.uid, e.agent.ID, res.JobID, "failed", "", "npx exited 1", nil); err != nil {
					t.Fatal(err)
				}
			} else if _, err := e.s.CancelInstallFor(e.uid, res.JobID); err != nil {
				t.Fatal(err)
			}
			if err := resave(t, e); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("applied", func(t *testing.T) {
		e := installFixture(t)
		e.existingSet(t)
		res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
		if err != nil {
			t.Fatal(err)
		}
		hash := e.awaitApproval(t, "pdf")
		if err := e.s.ControlSyncJob(e.uid, res.JobID, "approve", hash); err != nil {
			t.Fatal(err)
		}
		if err := resave(t, e); err == nil || err.Error() != installPendingSentence {
			t.Fatal("approved but unapplied install allowed a re-save:", err)
		}
		j := e.claim(t)
		if err := e.s.FinishSyncJob(e.uid, e.agent.ID, j.ID, "succeeded", hash, "", nil); err != nil {
			t.Fatal(err)
		}
		if err := resave(t, e); err != nil {
			t.Fatal(err)
		}
		got := slugs(e.target(t).Desired)
		sort.Strings(got)
		if !reflect.DeepEqual(got, []string{"one", "pdf", "two"}) {
			t.Fatalf("%v", got)
		}
	})
}

func TestRunRefusalNamesEveryFolderWithAPendingInstall(t *testing.T) {
	e := installFixture(t)
	if err := e.s.TouchEnvironmentFor(e.uid, "laptop", "opencode", "", "/home/u/.config/opencode/skills"); err != nil {
		t.Fatal(err)
	}
	if err := e.s.RecordInventoryFor(e.uid, EnvironmentIDFor(e.uid, "laptop", "opencode", ""), true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf")); err != nil {
		t.Fatal(err)
	}
	in := e.req("docx")
	in.Tool = "opencode"
	if _, err := e.s.InstallHostedSkillFor(e.uid, in); err != nil {
		t.Fatal(err)
	}
	_, err := e.s.StartSyncRun(e.uid, e.agent.ID, nil)
	if err == nil || err.Error() != "an install is waiting on each of these folders; finish or cancel them first" {
		t.Fatal(err)
	}
}

func TestAnAppliedInstallIsNeverReverted(t *testing.T) {
	e := installFixture(t)
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	hash := e.awaitApproval(t, "pdf")
	if err := e.s.ControlSyncJob(e.uid, res.JobID, "approve", hash); err != nil {
		t.Fatal(err)
	}
	j := e.claim(t)
	if err := e.s.FinishSyncJob(e.uid, e.agent.ID, j.ID, "succeeded", hash, "", nil); err != nil {
		t.Fatal(err)
	}
	before := e.target(t)
	// Copy the applied set elsewhere, as a merge would, then try every revert.
	dest := e.mergeTarget(t)
	if _, err := e.s.MergeEnvironmentsFor(e.uid, e.env, dest); err != nil {
		t.Fatal(err)
	}
	var reverted bool
	if err := e.s.withTx(func(tx *sql.Tx) error { var e error; reverted, e = revertInstallTx(tx, res.JobID); return e }); err != nil || reverted {
		t.Fatal(reverted, err)
	}
	e.env = dest
	e.sameSet(t, before)
}

func TestSaveSyncTargetRefusedWhileAnInstallIsPending(t *testing.T) {
	e := installFixture(t)
	e.s.UpsertFor(e.uid, mustRef(t, "a/b/one"), Patch{})
	if err := e.s.RecordEnvironmentFor(e.uid, "a/b/one", "desk", "claude", "", "/home/d/.claude/skills", ""); err != nil {
		t.Fatal(err)
	}
	desk := EnvironmentIDFor(e.uid, "desk", "claude", "")
	if err := e.s.RecordInventoryFor(e.uid, desk, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	res, err := e.s.InstallHostedSkillFor(e.uid, e.req("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	before := e.dump(t)
	if err := e.s.SaveSyncTargetFor(e.uid, desk, e.env); err != ErrInstallPending {
		t.Fatal(err)
	}
	if e.dump(t) != before {
		t.Fatal("refusal wrote")
	}
	if _, err := e.s.CancelInstallFor(e.uid, res.JobID); err != nil {
		t.Fatal(err)
	}
	if err := e.s.SaveSyncTargetFor(e.uid, desk, e.env); err != nil {
		t.Fatal(err)
	}
}
