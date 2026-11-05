package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/contentobs"
)

func TestScopedLibraryRetryPreservesOtherMachineCaptures(t *testing.T) {
	s, uid, source, destination, destID, original, _ := machineSyncFixture(t)
	if err := s.CancelRemoteImportFor(uid, original.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertLocalFor(uid, "other", Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(uid, "local/other", source.Host, "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	selected, err := s.RequestMachineSkillSync(uid, "local/demo", destination.Host, original.EnvironmentID, "library_"+secretHash("local/demo"), destID, true)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.RequestMachineSkillSync(uid, "local/other", source.Host, original.EnvironmentID, "library_"+secretHash("local/other"), original.EnvironmentID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []RemoteImport{selected, other} {
		if err = s.CancelRemoteImportFor(uid, request.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.SyncLibraryMachineFor(uid, "owner", "https://reet.example", destination.ID); err != nil {
		t.Fatal(err)
	}
	retried, err := s.RemoteImportFor(uid, selected.ID)
	if err != nil || retried.RequestKey == selected.RequestKey {
		t.Fatal("selected machine's failed capture cannot be retried", retried, err)
	}
	unchanged, err := s.RemoteImportFor(uid, other.ID)
	if err != nil || unchanged.RequestKey != other.RequestKey || unchanged.State != "cancelled" {
		t.Fatal("scoped retry changed another machine's capture", unchanged, err)
	}
	if err = s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	retried, err = s.RemoteImportFor(uid, other.ID)
	if err != nil || retried.RequestKey == other.RequestKey {
		t.Fatal("explicit fleet retry did not release the other capture", retried, err)
	}
}

func TestPrivateSyncAcceptsCompleteSkillsInFreshIncompleteObservations(t *testing.T) {
	for _, state := range []string{"fresh", "stale", "paused", "invalidated"} {
		t.Run(state, func(t *testing.T) {
			s, uid, agent, env := observationFixture(t)
			if err := s.ClaimPublishingUsername(uid, "writer"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SaveHostedSkill(uid, HostedSkill{Slug: "demo", Content: "---\nname: demo\ndescription: Private skill\n---\nInstructions.\n"}); err != nil {
				t.Fatal(err)
			}
			candidates, err := s.SyncHostedFor(uid, "https://reet.example")
			if err != nil || len(candidates) != 1 {
				t.Fatal(candidates, err)
			}
			want := candidates[0]
			second := EnvironmentIDFor(uid, "laptop", "claude", "")
			if err = s.TouchEnvironmentFor(uid, "laptop", "claude", "", ""); err != nil {
				t.Fatal(err)
			}
			if err = s.RecordInventoryFor(uid, second, true, MachineInfo{}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{env, second} {
				if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "hosted", Origin: "https://reet.example", SkillIDs: []string{want.ID}, Mode: "additive"}); err != nil {
					t.Fatal(err)
				}
			}
			grant, err := s.PrepareContentObservationFor(uid, agent.ID)
			if err != nil {
				t.Fatal(err)
			}
			report := observationReport(grant)
			report.Scopes[0].Complete = false
			report.Scopes[0].Skills[0].Digest = want.Source[strings.LastIndex(want.Source, ":")+1:]
			unreadable := report.Scopes[0].Skills[0]
			unreadable.Name, unreadable.Complete, unreadable.Digest, unreadable.Issues = "unrelated", false, "", []string{"unreadable"}
			report.Scopes[0].Skills = append(report.Scopes[0].Skills, unreadable)
			if err = s.RecordContentObservationFor(uid, agent.ID, report); err != nil {
				t.Fatal(err)
			}
			switch state {
			case "stale":
				_, err = s.db.Exec(`UPDATE content_observation_scopes SET received_at=? WHERE agent_id=?`, now()-contentobs.FreshMillis-1, agent.ID)
			case "paused":
				err = s.ControlSyncAgent(uid, agent.ID, "pause")
			case "invalidated":
				err = s.RecordInventoryFor(uid, env, true, MachineInfo{})
			}
			if err != nil {
				t.Fatal(err)
			}
			targets, err := s.SyncTargetsFor(uid)
			if err != nil {
				t.Fatal(err)
			}
			for _, target := range targets {
				if target.Target.ID == env {
					missing := 1
					if state == "fresh" {
						missing = 0
					}
					if len(target.Missing) != missing {
						t.Fatalf("%s: matching individual digest evaluated incorrectly: %+v", state, target.Missing)
					}
				}
			}
			if state == "fresh" {
				agent.KeepSynced = true
				if err = s.MaintainSync(uid, "owner", agent); err != nil {
					t.Fatal(err)
				}
				jobs, err := s.SyncJobsFor(uid)
				if err != nil || len(jobs) != 1 || jobs[0].TargetID != second {
					t.Fatalf("already-matching folder starved the later folder: %+v %v", jobs, err)
				}
			}
		})
	}
}

func TestNoChangePreviewReplansWhenLibraryRevisionChanged(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartSyncRun(uid, a.ID, []string{id}); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimSyncJob(uid, a)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if err = s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}, Revision: job.Revision}); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "awaiting-approval", strings.Repeat("a", 64), "", json.RawMessage(`{"actions":[]}`)); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 2 {
		t.Fatalf("fresh plan not queued: %+v", jobs)
	}
	for _, next := range jobs {
		if next.ID != job.ID && (next.Status != "queued" || next.Revision == job.Revision || next.Kind != "auto") {
			t.Fatalf("wrong recovery: %+v", next)
		}
	}
}

func TestSyncLibraryAllMachinesPersistsPolicyAndKeepsProjectsExplicit(t *testing.T) {
	s, uid, _ := controlFixture(t)
	for _, host := range []string{"laptop", "desktop"} {
		for _, scope := range []struct{ agent, project string }{{"agents", ""}, {"claude", ""}, {"claude", "/work/project"}} {
			if err := s.TouchEnvironmentFor(uid, host, scope.agent, scope.project, ""); err != nil {
				t.Fatal(err)
			}
			if err := s.RecordInventoryFor(uid, EnvironmentIDFor(uid, host, scope.agent, scope.project), true, MachineInfo{}); err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err := s.EnrollSyncAgent(uid, host, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil || len(targets) != 4 {
		t.Fatalf("targets: %+v %v", targets, err)
	}
	for _, target := range targets {
		if target.SourceID != "library" || !target.FollowSource || target.Mode != "additive" || target.Target.Project != "" {
			t.Fatalf("wrong library policy: %+v", target)
		}
	}
	agents, _ := s.SyncAgentsFor(uid)
	jobs, _ := s.SyncJobsFor(uid)
	for _, agent := range agents {
		if !agent.KeepSynced {
			t.Fatal("sync not persisted", agent)
		}
		if err := s.MaintainSync(uid, "owner", agent); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := s.SyncJobsFor(uid)
	if len(after) != len(jobs) {
		t.Fatal("duplicate jobs while pending")
	}
}

func TestLibrarySourceConflictPreservesLastGoodSelection(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "shelf", Mode: "additive", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	s.EnrollSyncAgent(uid, "laptop", "test")
	s.UpsertFor(uid, mustRef(t, "c/d/one"), Patch{})
	if err := s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err == nil {
		t.Fatal("ambiguous source accepted")
	}
	targets, _ := s.SyncTargetsFor(uid)
	if len(targets) != 1 || len(targets[0].Desired) != 1 || targets[0].Desired[0].ID != "a/b/one" {
		t.Fatalf("last good selection changed: %+v", targets)
	}
}

func TestLibrarySkipsSkillWhosePrivateCopyFailed(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if err := s.SetSyncLibraryOrigin(uid, "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	s.UpsertFor(uid, mustRef(t, "a/b/one"), Patch{})
	if _, err := s.UpsertLocalFor(uid, "huge", Patch{}); err != nil {
		t.Fatal(err)
	}
	source, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "laptop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	srcEnv := EnvironmentIDFor(uid, "laptop", "agents", "")
	if err = s.RecordInventoryFor(uid, srcEnv, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, "local/huge", "laptop", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	destination, _, err := s.EnrollSyncAgent(uid, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "desktop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	destEnv := EnvironmentIDFor(uid, "desktop", "agents", "")
	if err = s.RecordInventoryFor(uid, destEnv, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimRemoteImportFor(uid, source.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	request, err := s.RequestMachineSkillSync(uid, "local/huge", destination.Host, "", "library_"+secretHash("local/huge"), destEnv, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FailRemoteImportFor(uid, source.ID, request.ID, request.Instance, "capture_failed"); err != nil {
		t.Fatal(err)
	}

	candidates, err := s.SyncLibraryFor(uid, "owner")
	if err != nil {
		t.Fatal(err)
	}
	var problem string
	for _, candidate := range candidates {
		if candidate.ID == "local/huge" {
			problem = candidate.Problem
		}
	}
	if !strings.Contains(problem, "could not be copied") {
		t.Fatalf("uncopyable skill not explained: %q", problem)
	}

	// The failure belongs to one skill, not the whole machine: the rest of the
	// library still reaches the destination.
	destination.KeepSynced = true
	if err = s.prepareLibraryTargets(uid, "owner", destination, false); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", destination); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.SyncJobsFor(uid)
	if err != nil || len(jobs) == 0 {
		t.Fatalf("one uncopyable skill blocked the library: %+v %v", jobs, err)
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Target.ID == destEnv && target.Problem != "" {
			t.Fatalf("machine kept the skill failure as its own: %+v", target)
		}
	}
}

func TestFailedCaptureDoesNotRewriteDesiredSetOnIdlePoll(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if err := s.SetSyncLibraryOrigin(uid, "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	s.UpsertFor(uid, mustRef(t, "a/b/one"), Patch{})
	if _, err := s.UpsertLocalFor(uid, "huge", Patch{}); err != nil {
		t.Fatal(err)
	}
	source, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "laptop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	srcEnv := EnvironmentIDFor(uid, "laptop", "agents", "")
	if err = s.RecordInventoryFor(uid, srcEnv, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, "local/huge", "laptop", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	destination, _, err := s.EnrollSyncAgent(uid, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "desktop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	destEnv := EnvironmentIDFor(uid, "desktop", "agents", "")
	if err = s.RecordInventoryFor(uid, destEnv, true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEnvironmentFor(uid, "a/b/one", "desktop", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimRemoteImportFor(uid, source.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	request, err := s.RequestMachineSkillSync(uid, "local/huge", destination.Host, "", "library_"+secretHash("local/huge"), destEnv, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FailRemoteImportFor(uid, source.ID, request.ID, request.Instance, "capture_failed"); err != nil {
		t.Fatal(err)
	}
	destination.KeepSynced = true
	if err = s.prepareLibraryTargets(uid, "owner", destination, false); err != nil {
		t.Fatal(err)
	}
	if err = s.MaintainSync(uid, "owner", destination); err != nil {
		t.Fatal(err)
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	var revision string
	for _, target := range targets {
		if target.Target.ID == destEnv {
			revision = target.Revision
		}
	}
	if revision == "" {
		t.Fatal("library target was not prepared")
	}
	for range 2 {
		if err = s.MaintainSync(uid, "owner", destination); err != nil {
			t.Fatal(err)
		}
	}
	targets, err = s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Target.ID == destEnv && target.Revision != revision {
			t.Fatalf("an idle poll rewrote the desired set: %s -> %s", revision, target.Revision)
		}
	}
}

func TestLibrarySourceConflictStillBlocksAutomation(t *testing.T) {
	s, uid, id := controlFixture(t)
	if err := s.SetSyncLibraryOrigin(uid, "https://reet.example"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.EnrollSyncAgent(uid, "laptop", "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDesiredFor(uid, "owner", DesiredSelection{TargetID: id, SourceID: "library", FollowSource: true, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	s.UpsertFor(uid, mustRef(t, "c/d/one"), Patch{})
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 {
		t.Fatal(agents, err)
	}
	agents[0].KeepSynced = true
	if err = s.MaintainSync(uid, "owner", agents[0]); err != nil {
		t.Fatal(err)
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Problem == "" {
		t.Fatalf("ambiguous library was not reported: %+v", targets)
	}
	if len(targets[0].Desired) != 1 || targets[0].Desired[0].ID != "a/b/one" {
		t.Fatalf("last good selection was replaced: %+v", targets[0].Desired)
	}
}
