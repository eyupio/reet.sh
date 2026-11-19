package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func updateFixture(t *testing.T) (*Store, int64, SyncAgent, SyncJob) {
	t.Helper()
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	return s, uid, a, job
}

func TestAnUpdateJobCarriesItsTimeline(t *testing.T) {
	s, uid, a, job := updateFixture(t)
	if job.Stage != UpdateStageQueued || len(job.Stages) != len(UpdateStageKeys) {
		t.Fatalf("queued timeline: %+v", job)
	}
	if job.Stages[0].Status != "running" || job.Stages[1].Status != "pending" {
		t.Fatalf("queued stage states: %+v", job.Stages)
	}
	claimed, err := s.ClaimSyncJob(uid, a)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	if claimed.Stage != UpdateStageDownload || claimed.Stages[0].Status != "done" || claimed.Stages[1].Status != "running" {
		t.Fatalf("claimed timeline: %+v", claimed)
	}
	if err = s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageVerify, "Checking the downloaded build"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageRestart, "Restarting into v1.3.0"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageVerify, "too late"); err == nil {
		t.Fatal("a stage moved backwards")
	}
	if err = s.SetAgentUpdateStage(uid, a.ID, job.ID, "invented", ""); err == nil {
		t.Fatal("an unknown stage was accepted")
	}
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", "", "Updated from v1.2.0 to v1.3.0.", nil); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 1 || jobs[0].Status != "succeeded" || jobs[0].Stage != UpdateStageCheck {
		t.Fatalf("finished update: %+v", jobs)
	}
	for _, stage := range jobs[0].Stages {
		if stage.Status != "done" {
			t.Fatalf("stage left unfinished: %+v", jobs[0].Stages)
		}
	}
	if jobs[0].Stages[stageIndex(jobs[0].Stages, UpdateStageRestart)].Detail != "Restarting into v1.3.0" {
		t.Fatalf("detail lost: %+v", jobs[0].Stages)
	}
}

func TestAFailedUpdateStopsAtTheStageAndARollbackSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		last   string
	}{
		{"failed", "failed", UpdateStageCheck},
		{"rolled back", "rolled_back", UpdateStageRollback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, uid, a, job := updateFixture(t)
			if _, err := s.ClaimSyncJob(uid, a); err != nil {
				t.Fatal(err)
			}
			if err := s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageCheck, "Waiting for the new agent"); err != nil {
				t.Fatal(err)
			}
			if err := s.FinishSyncJob(uid, a.ID, job.ID, tc.status, "", "The new build did not report in.", nil); err != nil {
				t.Fatal(err)
			}
			jobs, _ := s.SyncJobsFor(uid)
			got := jobs[0]
			if got.Status != tc.status || got.Stage != tc.last {
				t.Fatalf("outcome: %+v", got)
			}
			if got.Stages[stageIndex(got.Stages, UpdateStageCheck)].Status != "failed" {
				t.Fatalf("check not failed: %+v", got.Stages)
			}
			if tc.status == "rolled_back" {
				if stageIndex(got.Stages, UpdateStageRollback) < 0 || got.Stages[len(got.Stages)-1].Status != "done" {
					t.Fatalf("rollback missing: %+v", got.Stages)
				}
			}
		})
	}
}

func TestAutoUpdateWaitsAfterAnAttemptSettlesButNotWhileOneIsQueued(t *testing.T) {
	s, uid, a, job := updateFixture(t)
	recent := time.Now().Add(-time.Hour).UnixMilli()
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, recent); err != nil || settled {
		t.Fatalf("a queued attempt counted as settled: %t %v", settled, err)
	}
	if _, err := s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, recent); err != nil || settled {
		t.Fatalf("a running attempt counted as settled: %t %v", settled, err)
	}
	if err := s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", "", "Updated.", nil); err != nil {
		t.Fatal(err)
	}
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, recent); err != nil || !settled {
		t.Fatalf("a finished attempt did not count as settled: %t %v", settled, err)
	}
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, time.Now().Add(time.Minute).UnixMilli()); err != nil || settled {
		t.Fatalf("a future window reported an attempt: %t %v", settled, err)
	}
	job, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishSyncJob(uid, a.ID, job.ID, "rolled_back", "", "Restored the previous binary.", nil); err != nil {
		t.Fatal(err)
	}
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, time.Now().Add(-time.Minute).UnixMilli()); err != nil || !settled {
		t.Fatalf("a rollback did not count as settled: %t %v", settled, err)
	}
}

func TestAnUpdateStageNeedsItsOwnRunningUpdate(t *testing.T) {
	s, uid, a, job := updateFixture(t)
	if err := s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageDownload, ""); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("stage written before the machine collected the job: %v", err)
	}
	if _, err := s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateUser("other@updates.invalid", "Other", "member", "")
	foreign, _, _ := s.EnrollSyncAgent(other.ID, "desktop", "v1.2.0")
	if err := s.SetAgentUpdateStage(other.ID, foreign.ID, job.ID, UpdateStageVerify, ""); err == nil {
		t.Fatal("a foreign agent wrote a stage")
	}
}

func TestOpenAddsTheUpdateTimelineToExistingJobs(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE sync_agents (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, host TEXT NOT NULL, secret_hash TEXT NOT NULL, version TEXT NOT NULL, source_commit TEXT NOT NULL DEFAULT '', last_seen INTEGER NOT NULL, paused INTEGER NOT NULL DEFAULT 0, auto_update INTEGER NOT NULL DEFAULT 1, UNIQUE(user_id,host))`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE sync_jobs (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, agent_id TEXT NOT NULL, target_id TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL, status TEXT NOT NULL, revision TEXT NOT NULL DEFAULT '', plan TEXT NOT NULL DEFAULT '{}', hash TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid, _ := s.DefaultOwner()
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil || job.Stage != UpdateStageQueued || len(job.Stages) == 0 {
		t.Fatalf("update on a migrated database: %+v %v", job, err)
	}
	jobs, err := s.SyncJobsFor(uid)
	if err != nil || len(jobs) != 1 || jobs[0].Stages[0].Key != UpdateStageQueued {
		t.Fatalf("read back: %+v %v", jobs, err)
	}
}

func TestALateUpdateReportReplacesTheTimeoutItOutlived(t *testing.T) {
	s, uid, a, job := updateFixture(t)
	if _, err := s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAgentUpdateStage(uid, a.ID, job.ID, UpdateStageRestart, "Restarting into v1.3.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE sync_jobs SET status='failed',message='The machine did not report back. It may still be restarting; check its build and try again.',updated_at=? WHERE id=?`, time.Now().Add(-11*time.Minute).UnixMilli(), job.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSyncJob(uid, a.ID, job.ID, "succeeded", "", "Updated from v1.2.0 to v1.3.0. The agent is reporting again.", nil); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.SyncJobsFor(uid)
	if len(jobs) != 1 || jobs[0].Status != "succeeded" || jobs[0].Stage != UpdateStageCheck {
		t.Fatalf("late report: %+v", jobs)
	}
	for _, stage := range jobs[0].Stages {
		if stage.Status != "done" {
			t.Fatalf("late report left a stage: %+v", jobs[0].Stages)
		}
	}
}

func TestTheLatestUpdateIsKeptForEachMachineWhenJobHistoryIsBusy(t *testing.T) {
	s, uid, a, first := updateFixture(t)
	if _, err := s.ClaimSyncJob(uid, a); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSyncJob(uid, a.ID, first.ID, "failed", "", "Download failed.", nil); err != nil {
		t.Fatal(err)
	}
	second, err := s.QueueAgentUpdate(uid, a.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.EnrollSyncAgent(uid, "desktop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	third, err := s.QueueAgentUpdate(uid, other.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	// Millisecond timestamps can collide during a quick retry. The later
	// inserted update must still supersede the earlier failed one.
	if _, err = s.db.Exec(`UPDATE sync_jobs SET created_at=1 WHERE user_id=?`, uid); err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestUpdateJobsFor(uid)
	if err != nil || len(latest) != 2 {
		t.Fatalf("latest updates: %+v %v", latest, err)
	}
	byAgent := map[string]string{}
	for _, job := range latest {
		byAgent[job.AgentID] = job.ID
	}
	if byAgent[a.ID] != second.ID || byAgent[other.ID] != third.ID {
		t.Fatalf("wrong jobs: %+v", byAgent)
	}
}

// An agent on skills protocol 2 follows the desired state, not jobs: it takes
// only binary updates, and other jobs queued for it stay where they are.
func TestClaimUpdateJobTakesOnlyUpdates(t *testing.T) {
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.QueueSyncJob(uid, a.ID, "", "scan")
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := s.ClaimUpdateJob(uid, a); err != nil || claimed != nil {
		t.Fatalf("an old-engine job was claimed: %+v %v", claimed, err)
	}
	update, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	paused := a
	paused.Paused = true
	if claimed, err := s.ClaimUpdateJob(uid, paused); err != nil || claimed != nil {
		t.Fatalf("a paused machine claimed %+v %v", claimed, err)
	}
	claimed, err := s.ClaimUpdateJob(uid, a)
	if err != nil || claimed == nil || claimed.ID != update.ID || claimed.Status != "running" || claimed.Stage != UpdateStageDownload {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	jobs, err := s.SyncJobsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ID == scan.ID && j.Status != "queued" {
			t.Fatalf("the scan job moved: %+v", j)
		}
	}
}

// An agent carries the skills protocol it last announced; below 2 it gets no
// skill work and reads needs-update.
func TestAgentsCarryTheirSkillsProtocol(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a, secret, err := s.EnrollSyncAgent(uid, "laptop", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	agents, err := s.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || agents[0].SkillsProtocol != 0 || agents[0].SkillsState != SkillsNeedUpdate {
		t.Fatalf("a new enrollment: %+v %v", agents, err)
	}
	if err = s.SetAgentSkillsProtocolFor(uid, a.ID, CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	agents, err = s.SyncAgentsFor(uid)
	if err != nil || agents[0].SkillsProtocol != CurrentSkillsProtocol || agents[0].SkillsState != "" {
		t.Fatalf("after protocol 2: %+v %v", agents, err)
	}
	authed, err := s.AuthenticateSyncAgentBuild(uid, a.ID, secret, "v1.2.0", "")
	if err != nil || authed.SkillsProtocol != CurrentSkillsProtocol || authed.SkillsState != "" {
		t.Fatalf("authenticated: %+v %v", authed, err)
	}
	authed.SetSkillsProtocol(1)
	if authed.SkillsProtocol != 1 || authed.SkillsState != SkillsNeedUpdate {
		t.Fatalf("set below 2: %+v", authed)
	}
	authed.SetSkillsProtocol(CurrentSkillsProtocol + 1)
	if authed.SkillsState != SkillsNeedNewerController {
		t.Fatalf("set above the controller's: %+v", authed)
	}
}

// A machine is on the skills protocol whether it is named by its own name or
// by an alias a merge left, and at a newer protocol too: it takes no job-based
// sync either way.
func TestHostOnSkillsProtocolResolvesAliases(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a, _, err := s.EnrollSyncAgent(uid, "studio", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO machine_aliases(user_id,alias,host) VALUES(?,?,?)`, uid, "old-studio", "studio"); err != nil {
		t.Fatal(err)
	}
	check := func(host string, want bool) {
		t.Helper()
		on, err := s.HostOnSkillsProtocolFor(uid, host)
		if err != nil || on != want {
			t.Fatalf("%s: %t %v, want %t", host, on, err, want)
		}
	}
	check("studio", false)
	if err = s.SetAgentSkillsProtocolFor(uid, a.ID, CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	check("STUDIO", true)
	check("Old-Studio", true)
	check("elsewhere", false)
	if err = s.SetAgentSkillsProtocolFor(uid, a.ID, CurrentSkillsProtocol+1); err != nil {
		t.Fatal(err)
	}
	check("old-studio", true)
}

const followsLibraryMessage = "This machine now follows your library; this job is no longer needed."

// When a machine first announces the skills protocol, its queued job-based
// sync work is closed, once, saying why; its update job stays.
func TestSkillsProtocolClosesOldJobsOnce(t *testing.T) {
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.QueueSyncJob(uid, a.ID, "", "scan")
	if err != nil {
		t.Fatal(err)
	}
	waiting := "waiting-" + scan.ID
	if _, err = s.db.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,revision,created_at,updated_at) VALUES(?,?,?,'','preview','awaiting-approval','',1,1)`, waiting, uid, a.ID); err != nil {
		t.Fatal(err)
	}
	update, err := s.QueueAgentUpdate(uid, a.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentSkillsProtocolFor(uid, a.ID, CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	status := func() map[string]SyncJob {
		t.Helper()
		jobs, err := s.SyncJobsFor(uid)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]SyncJob{}
		for _, j := range jobs {
			out[j.ID] = j
		}
		return out
	}
	jobs := status()
	for _, id := range []string{scan.ID, waiting} {
		if j := jobs[id]; j.Status != "cancelled" || j.Message != followsLibraryMessage {
			t.Fatalf("%s: %+v", id, j)
		}
	}
	if j := jobs[update.ID]; j.Status != "queued" {
		t.Fatalf("the update job: %+v", j)
	}
	// Only the change closes jobs: the same protocol again leaves a job alone.
	late := "late-" + scan.ID
	if _, err = s.db.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,revision,created_at,updated_at) VALUES(?,?,?,'','scan','queued','',2,2)`, late, uid, a.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentSkillsProtocolFor(uid, a.ID, CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	if j := status()[late]; j.Status != "queued" {
		t.Fatalf("the same protocol again closed a job: %+v", j)
	}
}

// A machine on the skills protocol is given no job-based sync: queueing one
// is refused, while an update is still queued.
func TestJobBasedSyncRefusesAMachineThatFollowsTheLibrary(t *testing.T) {
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentSkillsProtocolFor(uid, a.ID, CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueSyncJob(uid, a.ID, "", "scan"); !errors.Is(err, ErrAgentFollowsLibrary) || err.Error() != "laptop follows your library; its skills are checked on every poll, so there's nothing to scan." {
		t.Fatalf("queue a scan: %v", err)
	}
	if err = s.withTx(func(tx *sql.Tx) error {
		_, err := insertSyncJobTx(tx, uid, a.ID, "", "auto")
		return err
	}); !errors.Is(err, ErrAgentFollowsLibrary) || err.Error() != "laptop follows your library. Make the skill Active to put it on every machine." {
		t.Fatalf("queue an automatic sync: %v", err)
	}
	if _, err = s.QueueAgentUpdate(uid, a.ID, "latest"); err != nil {
		t.Fatalf("queue an update: %v", err)
	}
	// Syncing that one machine is refused, saying what to do instead; syncing
	// all machines leaves it out.
	if err = s.SyncLibraryMachineFor(uid, "owner", "https://reet.example", a.ID); !errors.Is(err, ErrAgentFollowsLibrary) || err.Error() != "laptop follows your library. Make the skill Active to put it on every machine." {
		t.Fatalf("sync one machine that follows the library: %v", err)
	}
	if err = s.SyncLibraryMachinesFor(uid, "owner", "https://reet.example"); err != nil {
		t.Fatalf("sync all machines: %v", err)
	}
	jobs, err := s.SyncJobsFor(uid)
	if err != nil || len(jobs) != 1 || jobs[0].Kind != "update" {
		t.Fatalf("jobs: %+v %v", jobs, err)
	}
}

// The refusal names the machine as the person knows it: its merged name, not
// the host the agent reports, which is now only an alias.
func TestFollowsLibraryRefusalNamesTheResolvedMachine(t *testing.T) {
	s, uid, _ := controlFixture(t)
	a, _, err := s.EnrollSyncAgent(uid, "laptop", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentSkillsProtocolFor(uid, a.ID, CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	// The agent still reports "laptop", which is an alias of "work-laptop".
	if _, err = s.db.Exec(`INSERT INTO machine_aliases(user_id,alias,host) VALUES(?,?,?)`, uid, "laptop", "work-laptop"); err != nil {
		t.Fatal(err)
	}
	_, err = s.QueueSyncJob(uid, a.ID, "", "scan")
	if !errors.Is(err, ErrAgentFollowsLibrary) || err.Error() != "work-laptop follows your library; its skills are checked on every poll, so there's nothing to scan." {
		t.Fatalf("queue a scan: %v", err)
	}
}
