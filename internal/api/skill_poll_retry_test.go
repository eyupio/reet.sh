package api

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

type retryPollReply struct {
	Retry []string
	Job   *store.SyncJob
}

// retryFixture is a machine that failed to install seo-cluster and was asked
// to try again, with an update queued for it: the next poll has a retry to
// hand over and a job to claim.
func retryFixture(t *testing.T) (e reconcileEnv, update store.SyncJob, db *sql.DB, poll func() (int, retryPollReply, string)) {
	t.Helper()
	e = newReconcileEnv(t)
	e.writtenSkill(t, "seo-cluster")
	failed := store.SkillReportEntry{Folder: "~/.claude/skills", Name: "seo-cluster", State: "failed", Detail: "no write access"}
	if code, raw := e.call(t, "/api/v2/sync/skills/report", skillReportBody(failed), true); code != 200 {
		t.Fatalf("report: %d %s", code, raw)
	}
	if names, err := e.st.RequestSkillRetryFor(e.uid, e.agent.ID, []string{"seo-cluster"}, time.Now()); err != nil || len(names) != 1 {
		t.Fatalf("retry asked for: %v %v", names, err)
	}
	update, err := e.st.QueueAgentUpdate(e.uid, e.agent.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"skillsVersion": ""})
	poll = func() (int, retryPollReply, string) {
		code, raw := e.call(t, "/api/v2/sync/poll", string(body), true)
		var reply retryPollReply
		if code == 200 && json.Unmarshal([]byte(raw), &reply) != nil {
			t.Fatalf("poll reply: %s", raw)
		}
		return code, reply, raw
	}
	return e, update, e.db(t), poll
}

// A poll hands a machine the skills it was asked to try again, once. When the
// poll fails (here, claiming the machine's update job) the machine was told
// nothing, so the retries stay for its next poll; a poll that succeeds hands
// them over and they are gone.
func TestAPollThatFailsLeavesTheRetriesForTheNext(t *testing.T) {
	_, update, db, poll := retryFixture(t)
	if _, err := db.Exec(`CREATE TRIGGER refuse_claim BEFORE UPDATE ON sync_jobs BEGIN SELECT RAISE(ABORT, 'claim refused'); END`); err != nil {
		t.Fatal(err)
	}
	if code, _, raw := poll(); code != 500 {
		t.Fatalf("a poll whose claim fails: %d %s", code, raw)
	}
	if _, err := db.Exec(`DROP TRIGGER refuse_claim`); err != nil {
		t.Fatal(err)
	}
	code, reply, raw := poll()
	if code != 200 || len(reply.Retry) != 1 || reply.Retry[0] != "seo-cluster" || reply.Job == nil || reply.Job.ID != update.ID {
		t.Fatalf("the next poll should carry the retry and the update: %d %s", code, raw)
	}
	if code, again, raw := poll(); code != 200 || len(again.Retry) != 0 {
		t.Fatalf("the retry was handed over twice: %d %s", code, raw)
	}
}

// If the retries cannot be taken once the job is claimed, the machine is still
// told of the job (it will not be offered again), and the retries wait.
func TestARetryThatCannotBeTakenDoesNotLoseTheClaimedJob(t *testing.T) {
	_, update, db, poll := retryFixture(t)
	if _, err := db.Exec(`CREATE TRIGGER refuse_take BEFORE DELETE ON skill_retries BEGIN SELECT RAISE(ABORT, 'take refused'); END`); err != nil {
		t.Fatal(err)
	}
	code, reply, raw := poll()
	if code != 200 || reply.Job == nil || reply.Job.ID != update.ID || len(reply.Retry) != 0 {
		t.Fatalf("the claimed job must reach the machine, without the retry: %d %s", code, raw)
	}
	if _, err := db.Exec(`DROP TRIGGER refuse_take`); err != nil {
		t.Fatal(err)
	}
	if code, reply, raw = poll(); code != 200 || len(reply.Retry) != 1 || reply.Retry[0] != "seo-cluster" {
		t.Fatalf("the retry should have waited: %d %s", code, raw)
	}
}
