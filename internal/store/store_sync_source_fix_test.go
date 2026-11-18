package store

import (
	"sort"
	"strings"
	"testing"
)

func sourceFixFixture(t *testing.T) (*Store, int64, string) {
	t.Helper()
	s := newStore(t)
	uid, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.EnrollSyncAgent(uid, "laptop", "test"); err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "laptop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	target := EnvironmentIDFor(uid, "laptop", "agents", "")
	if err = s.RecordInventoryFor(uid, target, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "laptop", "claude", "", ""); err != nil {
		t.Fatal(err)
	}
	claude := EnvironmentIDFor(uid, "laptop", "claude", "")
	if err = s.RecordInventoryFor(uid, claude, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	installed, err := s.UpsertFor(uid, mustRef(t, "x/y/one"), Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, installed.ID, "laptop", "claude", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = legacyUpsert(t, s, uid, mustRef(t, "a/b/one")); err != nil {
		t.Fatal(err)
	}
	candidates, err := s.SyncCandidatesFor(uid, "owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	for _, candidate := range candidates {
		if candidate.ID == "a/b/one" {
			want = candidate.ID
		}
	}
	if want == "" {
		t.Fatal("desired candidate missing", candidates)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: target, SourceID: "shelf", SkillIDs: []string{want}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	return s, uid, target
}

func TestResolveSyncSourceConflictFollowsInstalledCopy(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	before, err := s.SyncTargetsFor(uid)
	if err != nil || len(before) != 1 || before[0].Desired[0].Source != "a/b" {
		t.Fatalf("fixture desired: %+v %v", before, err)
	}
	preview, err := s.ResolveSyncSourceConflictsFor(uid, []string{target}, true)
	if err != nil || len(preview) != 1 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if preview[0].From != "a/b" || preview[0].To != "x/y" || preview[0].Agent != "claude" {
		t.Fatalf("preview change: %+v", preview[0])
	}
	still, _ := s.SyncTargetsFor(uid)
	if still[0].Desired[0].Source != "a/b" {
		t.Fatal("a dry run changed the saved plan")
	}
	applied, err := s.ResolveSyncSourceConflictsFor(uid, []string{target}, false)
	if err != nil || len(applied) != 1 {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	after, _ := s.SyncTargetsFor(uid)
	if after[0].Desired[0].Source != "x/y" || after[0].Revision == before[0].Revision {
		t.Fatalf("plan not updated: %+v", after[0])
	}
}

func TestResolveSyncSourceConflictRefusesAmbiguousMachine(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	if err := s.TouchEnvironmentFor(uid, "laptop", "cursor", "", ""); err != nil {
		t.Fatal(err)
	}
	cursor := EnvironmentIDFor(uid, "laptop", "cursor", "")
	if err := s.RecordInventoryFor(uid, cursor, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	other, err := legacyUpsert(t, s, uid, mustRef(t, "z/w/one"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, other.ID, "laptop", "cursor", "", "", ""); err != nil {
		t.Fatal(err)
	}
	_, err = s.ResolveSyncSourceConflictsFor(uid, []string{target}, false)
	if err == nil || !strings.Contains(err.Error(), "more than one source") {
		t.Fatalf("ambiguous machine accepted: %v", err)
	}
	after, _ := s.SyncTargetsFor(uid)
	if after[0].Desired[0].Source != "a/b" {
		t.Fatal("a refused fix changed the saved plan")
	}
}

func TestMaintainSyncFollowsUnambiguousInstalledSourceAndReportsIt(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 {
		t.Fatalf("agents: %+v %v", agents, err)
	}
	agent := agents[0]
	if err = s.ControlSyncAgent(uid, agent.ID, "keep-sync-on"); err != nil {
		t.Fatal(err)
	}
	agents, err = s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || !agents[0].KeepSynced {
		t.Fatalf("keep synced: %+v %v", agents, err)
	}
	agent = agents[0]
	job, err := s.QueueSyncJob(uid, agent.ID, target, "auto")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimSyncJob(uid, agent)
	if err != nil || claimed == nil || claimed.ID != job.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	plan := []byte(`{"problems":{"one":"Claude Code already has this skill from x/y, so reet left it alone."}}`)
	if err = s.FinishSyncJob(uid, agent.ID, job.ID, "awaiting-approval", strings.Repeat("a", 64), "", plan); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	after, err := s.SyncTargetsFor(uid)
	if err != nil || len(after) != 1 || after[0].Desired[0].Source != "x/y" {
		t.Fatalf("source not followed: %+v %v", after, err)
	}
	next, err := s.ClaimSyncJob(uid, agent)
	if err != nil || next == nil || next.Kind != "auto" || !strings.Contains(next.Message, "Auto-fixed") {
		t.Fatalf("repair job: %+v %v", next, err)
	}
	again, err := s.AutoResolveSyncSourceConflictsFor(uid, target)
	if err != nil || len(again) != 0 {
		t.Fatalf("the repair was not idempotent: %+v %v", again, err)
	}
}

func TestMaintainSyncLeavesAmbiguousSourceConflictReported(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	if err := s.TouchEnvironmentFor(uid, "laptop", "cursor", "", ""); err != nil {
		t.Fatal(err)
	}
	cursor := EnvironmentIDFor(uid, "laptop", "cursor", "")
	if err := s.RecordInventoryFor(uid, cursor, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	other, err := legacyUpsert(t, s, uid, mustRef(t, "z/w/one"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, other.ID, "laptop", "cursor", "", "", ""); err != nil {
		t.Fatal(err)
	}
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 {
		t.Fatalf("agents: %+v %v", agents, err)
	}
	agent := agents[0]
	if err = s.ControlSyncAgent(uid, agent.ID, "keep-sync-on"); err != nil {
		t.Fatal(err)
	}
	agents, err = s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || !agents[0].KeepSynced {
		t.Fatalf("keep synced: %+v %v", agents, err)
	}
	agent = agents[0]
	job, err := s.QueueSyncJob(uid, agent.ID, target, "auto")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimSyncJob(uid, agent)
	if err != nil || claimed == nil || claimed.ID != job.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	plan := []byte(`{"problems":{"one":"Claude Code already has this skill from x/y, so reet left it alone."}}`)
	if err = s.FinishSyncJob(uid, agent.ID, job.ID, "awaiting-approval", strings.Repeat("b", 64), "", plan); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	after, err := s.SyncTargetsFor(uid)
	if err != nil || len(after) != 1 || after[0].Desired[0].Source != "a/b" {
		t.Fatalf("ambiguous conflict changed the plan: %+v %v", after, err)
	}
	if !strings.Contains(after[0].Problem, "more than one source") || !strings.Contains(after[0].Problem, "Auto fix could not settle") {
		t.Fatalf("ambiguous conflict was not reported: %+v", after[0])
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 1 {
		t.Fatalf("an ambiguous conflict queued another job: %+v", jobs)
	}
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	jobs, _ = s.SyncJobsFor(uid)
	if len(jobs) != 1 {
		t.Fatalf("an ambiguous conflict queued repeated jobs: %+v", jobs)
	}
}

func TestMaintainSyncRepairsSourceConflictWhilePreviewAwaitsApproval(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 {
		t.Fatalf("agents: %+v %v", agents, err)
	}
	agent := agents[0]
	if err = s.ControlSyncAgent(uid, agent.ID, "keep-sync-on"); err != nil {
		t.Fatal(err)
	}
	agents, err = s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || !agents[0].KeepSynced {
		t.Fatalf("keep synced: %+v %v", agents, err)
	}
	agent = agents[0]
	job, err := s.QueueSyncJob(uid, agent.ID, target, "preview")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimSyncJob(uid, agent)
	if err != nil || claimed == nil || claimed.ID != job.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	hash := strings.Repeat("c", 64)
	plan := []byte(`{"problems":{"one":"Claude Code already has this skill from x/y, so reet left it alone."}}`)
	if err = s.FinishSyncJob(uid, agent.ID, job.ID, "awaiting-approval", hash, "", plan); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	after, err := s.SyncTargetsFor(uid)
	if err != nil || len(after) != 1 || after[0].Desired[0].Source != "x/y" {
		t.Fatalf("source not followed while a preview awaited: %+v %v", after, err)
	}
	jobs, _ := s.SyncJobsFor(uid)
	var cancelled bool
	for _, item := range jobs {
		if item.ID == job.ID {
			cancelled = item.Status == "cancelled" && strings.Contains(item.Message, "Source changed")
		}
	}
	if !cancelled {
		t.Fatalf("stale approval was not cancelled: %+v", jobs)
	}
	next, err := s.ClaimSyncJob(uid, agent)
	if err != nil || next == nil || next.Kind != "auto" || !strings.Contains(next.Message, "Auto-fixed") {
		t.Fatalf("repair job: %+v %v", next, err)
	}
}

// Following the installed copy means the plan now asks for that copy. The fix
// used to change the source and keep the old skill's identity, so once the
// skill was installed the controller still listed the old one as missing and
// the installed one as an extra, for ever.
func TestResolveSyncSourceConflictAdoptsTheInstalledSkill(t *testing.T) {
	s, uid, target := sourceFixFixture(t)
	if _, err := s.ResolveSyncSourceConflictsFor(uid, []string{target}, false); err != nil {
		t.Fatal(err)
	}
	after, _ := s.SyncTargetsFor(uid)
	if len(after[0].Desired) != 1 || after[0].Desired[0].ID != "x/y/one" || after[0].Desired[0].Source != "x/y" {
		t.Fatalf("the plan does not ask for the installed skill: %+v", after[0].Desired)
	}
	if err := s.RecordEnvironmentFor(uid, "x/y/one", "laptop", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	after, _ = s.SyncTargetsFor(uid)
	if len(after[0].Missing) != 0 || len(after[0].Extra) != 0 {
		t.Fatalf("installed, and still not settled: missing %+v extra %+v", after[0].Missing, after[0].Extra)
	}
}

// The skills CLI installs a skill into a folder made from its name, which is
// not always the catalogue's slug. A copy another app holds under that folder,
// from another source, is the one sync refuses to overwrite, so it is the one
// Auto fix follows.
func TestResolveSyncSourceConflictFindsTheCopyUnderTheInstalledFolder(t *testing.T) {
	s := newStore(t)
	uid, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"agents", "claude"} {
		if err = s.TouchEnvironmentFor(uid, "laptop", agent, "", ""); err != nil {
			t.Fatal(err)
		}
		if err = s.RecordInventoryFor(uid, EnvironmentIDFor(uid, "laptop", agent, ""), true, MachineInfo{}); err != nil {
			t.Fatal(err)
		}
	}
	target := EnvironmentIDFor(uid, "laptop", "agents", "")
	held, err := s.UpsertFor(uid, mustRef(t, "o/r/ui-ux-design-review"), Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, held.ID, "laptop", "claude", "", "", ""); err != nil {
		t.Fatal(err)
	}
	const catalogued = "rknall/claude-skills/uiux-design-review"
	if _, err = s.UpsertFor(uid, mustRef(t, catalogued), Patch{}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetMeta(catalogued, "UI/UX Design Review", "", 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: target, SourceID: "shelf", SkillIDs: []string{catalogued}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	fixes, err := s.ResolveSyncSourceConflictsFor(uid, []string{target}, false)
	if err != nil || len(fixes) != 1 || fixes[0].From != "rknall/claude-skills" || fixes[0].To != "o/r" || fixes[0].Agent != "claude" {
		t.Fatalf("the copy under the installed folder was not followed: %+v %v", fixes, err)
	}
	after, _ := s.SyncTargetsFor(uid)
	if len(after[0].Desired) != 1 || after[0].Desired[0].ID != "o/r/ui-ux-design-review" || after[0].Desired[0].Slug != "ui-ux-design-review" {
		t.Fatalf("the plan does not ask for the installed skill: %+v", after[0].Desired)
	}
}

// A folder that follows a source has its saved plan compared with that source
// on every poll. The comparison knew nothing of Auto fix, so the poll after a
// fix wrote the source's own skill back, the machine refused it again, and the
// two took turns for ever. The followed copy now stands for as long as that
// copy is on the machine.
func TestMaintainSyncKeepsAnAutoFixOnAFolderThatFollowsItsSource(t *testing.T) {
	s := newStore(t)
	uid, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.EnrollSyncAgent(uid, "laptop", "test"); err != nil {
		t.Fatal(err)
	}
	for _, env := range [][2]string{{"laptop", "agents"}, {"laptop", "claude"}, {"desktop", "agents"}} {
		if err = s.TouchEnvironmentFor(uid, env[0], env[1], "", ""); err != nil {
			t.Fatal(err)
		}
		if err = s.RecordInventoryFor(uid, EnvironmentIDFor(uid, env[0], env[1], ""), true, MachineInfo{}); err != nil {
			t.Fatal(err)
		}
	}
	target, source := EnvironmentIDFor(uid, "laptop", "agents", ""), EnvironmentIDFor(uid, "desktop", "agents", "")
	for _, held := range [][3]string{{"x/y/one", "laptop", "claude"}, {"a/b/one", "desktop", "agents"}} {
		sk, e := legacyUpsert(t, s, uid, mustRef(t, held[0]))
		if e != nil {
			t.Fatal(e)
		}
		if e = s.RecordEnvironmentFor(uid, sk.ID, held[1], held[2], "", "", ""); e != nil {
			t.Fatal(e)
		}
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: target, SourceID: source, Mode: "additive", FollowSource: true}); err != nil {
		t.Fatal(err)
	}
	agents, _ := s.SyncAgentsFor(uid)
	if err = s.ControlSyncAgent(uid, agents[0].ID, "keep-sync-on"); err != nil {
		t.Fatal(err)
	}
	agents, _ = s.SyncAgentsFor(uid)
	agent := agents[0]
	desired := func() string {
		t.Helper()
		targets, e := s.SyncTargetsFor(uid)
		if e != nil || len(targets) != 1 {
			t.Fatalf("targets: %+v %v", targets, e)
		}
		ids := []string{}
		for _, sk := range targets[0].Desired {
			ids = append(ids, sk.ID)
		}
		sort.Strings(ids)
		return strings.Join(ids, ",")
	}
	drain := func() {
		t.Helper()
		for {
			job, e := s.ClaimSyncJob(uid, agent)
			if e != nil {
				t.Fatal(e)
			}
			if job == nil {
				return
			}
			if e = s.FinishSyncJob(uid, agent.ID, job.ID, "awaiting-approval", strings.Repeat("b", 64), "", []byte(`{"actions":[]}`)); e != nil {
				t.Fatal(e)
			}
		}
	}
	finish := func(plan string) {
		t.Helper()
		job, e := s.ClaimSyncJob(uid, agent)
		if e != nil || job == nil {
			t.Fatalf("no job to run: %+v %v", job, e)
		}
		if e = s.FinishSyncJob(uid, agent.ID, job.ID, "awaiting-approval", strings.Repeat("b", 64), "", []byte(plan)); e != nil {
			t.Fatal(e)
		}
	}
	if desired() != "a/b/one" {
		t.Fatalf("fixture follows %s", desired())
	}
	if _, err = s.QueueSyncJob(uid, agent.ID, target, "auto"); err != nil {
		t.Fatal(err)
	}
	finish(`{"problems":{"one":"Claude Code already has this skill from x/y, so reet left it alone."}}`)
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	if desired() != "x/y/one" {
		t.Fatalf("the installed copy was not followed: %s", desired())
	}
	// The job the fix queued runs and finds nothing to do.
	finish(`{"actions":[]}`)
	jobsBefore, _ := s.SyncJobsFor(uid)
	for range 3 {
		if err = s.MaintainSync(uid, "owner", agent); err != nil {
			t.Fatal(err)
		}
		if desired() != "x/y/one" {
			t.Fatalf("a later poll wrote the source's skill back: %s", desired())
		}
	}
	// Those polls ask the machine to install the followed copy, and no more than that.
	jobsAfter, _ := s.SyncJobsFor(uid)
	if len(jobsAfter) > len(jobsBefore)+1 {
		t.Fatalf("the polls queued %d jobs", len(jobsAfter)-len(jobsBefore))
	}
	drain()
	// The source gains a skill. The folder takes it and still follows the copy.
	added, err := s.UpsertFor(uid, mustRef(t, "a/b/two"), Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, added.ID, "desktop", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	if desired() != "a/b/two,x/y/one" {
		t.Fatalf("a change in the source dropped the followed copy: %s", desired())
	}
	drain()
	// The copy that was followed is gone, so the folder follows its source again.
	if _, err = s.PruneEnvironmentFor(uid, EnvironmentIDFor(uid, "laptop", "claude", ""), nil); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", agent); err != nil {
		t.Fatal(err)
	}
	if desired() != "a/b/one,a/b/two" {
		t.Fatalf("the folder did not return to its source once the copy was gone: %s", desired())
	}
}
