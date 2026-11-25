package store

import "testing"

// seedUpdateJobs inserts n update jobs of one status for an agent, created
// from createdFrom upwards, as the history a machine that keeps failing builds up.
func seedUpdateJobs(t *testing.T, s *Store, uid int64, agentID, status string, n int, createdFrom int64) {
	t.Helper()
	_, err := s.db.Exec(`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i<?-1)
		INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,revision,stage,stages,created_at,updated_at)
		SELECT ?||i,?,?,'','update',?,'latest','','[]',?+i,?+i FROM n`,
		n, agentID+status, uid, agentID, status, createdFrom, createdFrom)
	if err != nil {
		t.Fatal(err)
	}
}

func countUpdateJobs(t *testing.T, s *Store, agentID, statuses string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE agent_id=? AND kind='update' AND status IN (`+statuses+`)`, agentID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

const finishedStatuses = `'succeeded','failed','rolled_back','cancelled'`

// Queueing an update drops the machine's older finished updates beyond the
// newest twenty, and nothing else: not what is queued or running, and not
// another machine's jobs.
func TestQueueingAnUpdateKeepsTheNewestTwentyFinishedOnes(t *testing.T) {
	s, uid, a, first := updateFixture(t)
	other, _, err := s.EnrollSyncAgent(uid, "desktop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	seedUpdateJobs(t, s, uid, other.ID, "failed", 30, 1000)
	seedUpdateJobs(t, s, uid, other.ID, "running", 1, 2000)

	job := first
	for i := range 25 {
		if i%2 == 0 {
			if _, err = s.ClaimSyncJob(uid, a); err != nil {
				t.Fatal(err)
			}
			err = s.FinishSyncJob(uid, a.ID, job.ID, "failed", "Download failed.")
		} else {
			err = s.ControlSyncJob(uid, job.ID, "cancel")
		}
		if err != nil {
			t.Fatal(err)
		}
		if job, err = s.QueueAgentUpdate(uid, a.ID, "latest"); err != nil {
			t.Fatal(err)
		}
	}

	if n := countUpdateJobs(t, s, a.ID, finishedStatuses); n != 20 {
		t.Fatalf("finished updates kept: %d, want 20", n)
	}
	if n := countUpdateJobs(t, s, a.ID, `'queued'`); n != 1 {
		t.Fatalf("queued updates: %d, want the one just queued", n)
	}
	if n := countUpdateJobs(t, s, a.ID, `'queued','running','awaiting-approval'`); n != 1 {
		t.Fatalf("unfinished updates: %d", n)
	}
	var gone int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE id=?`, first.ID).Scan(&gone); err != nil || gone != 0 {
		t.Fatalf("the oldest finished update was kept: %d %v", gone, err)
	}
	if n := countUpdateJobs(t, s, other.ID, finishedStatuses); n != 30 {
		t.Fatalf("another machine's finished updates: %d, want 30", n)
	}
	if n := countUpdateJobs(t, s, other.ID, `'running'`); n != 1 {
		t.Fatalf("another machine's running update: %d, want 1", n)
	}
	if settled, err := s.AgentUpdateSettledSince(uid, a.ID, 0); err != nil || !settled {
		t.Fatalf("the cooldown lost the last update: %v %v", settled, err)
	}
}

// One machine that keeps failing to update must not push another machine's
// update out of the newest-500 window the sync page reads.
func TestAStuckMachineDoesNotHideAnotherMachinesUpdate(t *testing.T) {
	s, uid, stuck, _ := updateFixture(t)
	other, _, err := s.EnrollSyncAgent(uid, "desktop", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	seedUpdateJobs(t, s, uid, other.ID, "succeeded", 1, 1)
	seedUpdateJobs(t, s, uid, stuck.ID, "failed", 600, 1000)
	if _, err = s.db.Exec(`DELETE FROM sync_jobs WHERE agent_id=? AND status='queued'`, stuck.ID); err != nil {
		t.Fatal(err)
	}

	queued, err := s.QueueAgentUpdate(uid, stuck.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestUpdateJobsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	byAgent := map[string]string{}
	for _, j := range latest {
		byAgent[j.AgentID] = j.Status
	}
	if len(latest) != 2 || byAgent[stuck.ID] != "queued" || byAgent[other.ID] != "succeeded" {
		t.Fatalf("latest updates (queued %s): %+v", queued.ID, latest)
	}
}
