package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// SyncRun chains automatic preview/apply work across the saved scopes of one
// machine. Additive plans are approved automatically; plans that remove skills
// remain awaiting review while later scopes continue.
type SyncRun struct {
	ID        string   `json:"id"`
	AgentID   string   `json:"agentId"`
	Targets   []string `json:"targets"`
	Cursor    int      `json:"cursor"`
	Status    string   `json:"status"`
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

func (s *Store) SyncRunsFor(uid int64) ([]SyncRun, error) {
	rows, err := s.db.Query(`SELECT id,agent_id,targets,cursor,status,created_at,updated_at FROM sync_runs WHERE user_id=? ORDER BY created_at DESC LIMIT 20`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SyncRun{}
	for rows.Next() {
		var run SyncRun
		var targets string
		if err = rows.Scan(&run.ID, &run.AgentID, &targets, &run.Cursor, &run.Status, &run.CreatedAt, &run.UpdatedAt); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(targets), &run.Targets) != nil {
			return nil, errors.New("invalid sync run")
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// StartSyncRun queues automatic sync for every saved scope on an enrolled
// machine. Callers may narrow it to specific target IDs.
func (s *Store) StartSyncRun(uid int64, agentID string, targetIDs []string) (SyncRun, error) {
	var host string
	if err := s.db.QueryRow(`SELECT host FROM sync_agents WHERE id=? AND user_id=?`, agentID, uid).Scan(&host); err != nil {
		return SyncRun{}, ErrNotFound
	}
	if len(targetIDs) == 0 {
		rows, err := s.db.Query(`SELECT t.target_id FROM sync_targets t JOIN environments e ON e.id=t.target_id WHERE t.user_id=? AND e.host=? ORDER BY t.updated_at`, uid, host)
		if err != nil {
			return SyncRun{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return SyncRun{}, err
			}
			targetIDs = append(targetIDs, id)
		}
		if err = rows.Err(); err != nil {
			return SyncRun{}, err
		}
	}
	if len(targetIDs) == 0 {
		return SyncRun{}, errors.New("save at least one desired skill set for this machine before syncing")
	}
	seen := map[string]bool{}
	ordered := make([]string, 0, len(targetIDs))
	for _, id := range targetIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		var targetHost, agent string
		var complete bool
		var scanned int64
		if err := s.db.QueryRow(`SELECT e.host,e.agent,sc.complete,sc.scanned_at FROM sync_targets t JOIN environments e ON e.id=t.target_id LEFT JOIN environment_scans sc ON sc.env_id=e.id WHERE t.user_id=? AND t.target_id=?`, uid, id).Scan(&targetHost, &agent, &complete, &scanned); err != nil || !strings.EqualFold(targetHost, host) {
			return SyncRun{}, errors.New("sync target does not belong to this machine")
		}
		if !complete || scanned < time.Now().Add(-14*24*time.Hour).UnixMilli() {
			return SyncRun{}, errors.New("one or more saved scopes need a fresh complete scan before syncing")
		}
		if !Syncable(agent) {
			return SyncRun{}, errors.New("one or more saved scopes cannot receive sync jobs")
		}
		ordered = append(ordered, id)
	}
	targets, err := json.Marshal(ordered)
	if err != nil {
		return SyncRun{}, err
	}
	run := SyncRun{ID: syncID(), AgentID: agentID, Targets: ordered, Status: "running", CreatedAt: now(), UpdatedAt: now()}
	err = s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE sync_jobs SET status='cancelled',message='Replaced by a newer sync run.',updated_at=? WHERE status IN ('queued','awaiting-approval') AND id IN (SELECT job_id FROM sync_run_jobs WHERE run_id IN (SELECT id FROM sync_runs WHERE user_id=? AND agent_id=? AND status IN ('running','review')))`, now(), uid, agentID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE sync_runs SET status='cancelled',updated_at=? WHERE user_id=? AND agent_id=? AND status IN ('running','review')`, now(), uid, agentID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO sync_runs(id,user_id,agent_id,targets,cursor,status,created_at,updated_at) VALUES(?,?,?,?,0,?,?,?)`, run.ID, uid, agentID, string(targets), run.Status, run.CreatedAt, run.UpdatedAt); err != nil {
			return err
		}
		job, err := insertSyncJobTx(tx, uid, agentID, ordered[0], "auto")
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO sync_run_jobs(job_id,run_id,position) VALUES(?,?,0)`, job.ID, run.ID)
		return err
	})
	return run, err
}

func insertSyncJobTx(tx *sql.Tx, uid int64, agentID, targetID, kind string) (SyncJob, error) {
	revision := ""
	if targetID != "" {
		var targetHost, agentHost string
		if err := tx.QueryRow(`SELECT e.host,a.host,t.revision FROM sync_targets t JOIN environments e ON e.id=t.target_id JOIN sync_agents a ON a.id=? WHERE t.user_id=? AND t.target_id=?`, agentID, uid, targetID).Scan(&targetHost, &agentHost, &revision); err != nil {
			return SyncJob{}, ErrSync
		}
		if !strings.EqualFold(targetHost, agentHost) {
			return SyncJob{}, errors.New("target belongs to another enrolled machine")
		}
	}
	j := SyncJob{ID: syncID(), AgentID: agentID, TargetID: targetID, Kind: kind, Status: "queued", Revision: revision, Plan: json.RawMessage(`{}`), CreatedAt: now(), UpdatedAt: now()}
	_, err := tx.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, j.ID, uid, agentID, targetID, kind, j.Status, revision, j.CreatedAt, j.UpdatedAt)
	return j, err
}

type runOutcome int

const (
	runNext runOutcome = iota
	runReview
	runFailed
	runCancelled
)

// finishSyncRun advances the chain after one scope finishes. It is called inside
// the same transaction that records the job result.
func (s *Store) finishSyncRun(tx *sql.Tx, uid int64, jobID string, outcome runOutcome) error {
	var runID string
	var position int
	if err := tx.QueryRow(`SELECT run_id,position FROM sync_run_jobs WHERE job_id=?`, jobID).Scan(&runID, &position); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	var targetsJSON, status string
	var cursor int
	if err := tx.QueryRow(`SELECT targets,cursor,status FROM sync_runs WHERE id=? AND user_id=?`, runID, uid).Scan(&targetsJSON, &cursor, &status); err != nil {
		return err
	}
	if status == "cancelled" || status == "completed" || status == "failed" {
		return nil
	}
	if outcome == runFailed {
		_, err := tx.Exec(`UPDATE sync_runs SET status='failed',updated_at=? WHERE id=?`, now(), runID)
		return err
	}
	if outcome == runCancelled {
		_, err := tx.Exec(`UPDATE sync_runs SET status='cancelled',updated_at=? WHERE id=?`, now(), runID)
		return err
	}
	// A reviewed plan stays visible while later scopes continue. A result for an
	// older position (for example, approval after the chain moved on) must not
	// advance the cursor a second time.
	if position < cursor {
		return nil
	}
	var targets []string
	if json.Unmarshal([]byte(targetsJSON), &targets) != nil {
		return errors.New("invalid sync run")
	}
	next := cursor + 1
	nextStatus := "running"
	if outcome == runReview || status == "review" {
		nextStatus = "review"
	}
	if next >= len(targets) {
		if nextStatus == "running" {
			nextStatus = "completed"
		}
		_, err := tx.Exec(`UPDATE sync_runs SET cursor=?,status=?,updated_at=? WHERE id=?`, next, nextStatus, now(), runID)
		return err
	}
	job, err := insertSyncJobTx(tx, uid, agentIDForRun(tx, uid, runID), targets[next], "auto")
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO sync_run_jobs(job_id,run_id,position) VALUES(?,?,?)`, job.ID, runID, next); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE sync_runs SET cursor=?,status=?,updated_at=? WHERE id=?`, next, nextStatus, now(), runID)
	return err
}

func agentIDForRun(tx *sql.Tx, uid int64, runID string) string {
	var agentID string
	if err := tx.QueryRow(`SELECT agent_id FROM sync_runs WHERE id=? AND user_id=?`, runID, uid).Scan(&agentID); err != nil {
		return ""
	}
	return agentID
}

// additivePlan reports whether a preview only installs or replaces entries.
func additivePlan(plan json.RawMessage) bool {
	var parsed struct {
		Actions []struct {
			Add bool `json:"add"`
		} `json:"actions"`
	}
	if len(plan) == 0 || json.Unmarshal(plan, &parsed) != nil {
		return false
	}
	for _, action := range parsed.Actions {
		if !action.Add {
			return false
		}
	}
	return true
}

func (s *Store) QueueAgentUpdate(uid int64, agentID, version string) (SyncJob, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "latest"
	}
	if version != "latest" && version != "dev" && !strings.HasPrefix(version, "v") && !strings.HasPrefix(version, "V") {
		return SyncJob{}, errors.New("choose latest, dev or a release tag")
	}
	var owned int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sync_agents WHERE id=? AND user_id=?`, agentID, uid).Scan(&owned); err != nil {
		return SyncJob{}, err
	}
	if owned == 0 {
		return SyncJob{}, ErrNotFound
	}
	var active int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE user_id=? AND agent_id=? AND status IN ('queued','running','awaiting-approval')`, uid, agentID).Scan(&active); err != nil {
		return SyncJob{}, err
	}
	if active > 0 {
		return SyncJob{}, errors.New("finish or cancel this machine's current job first")
	}
	j := SyncJob{ID: syncID(), AgentID: agentID, Kind: "update", Status: "queued", Revision: version, Plan: json.RawMessage(`{}`), Stage: UpdateStageQueued, Stages: NewUpdateStages(), CreatedAt: now(), UpdatedAt: now()}
	_, err := s.db.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,revision,stage,stages,created_at,updated_at) VALUES(?,?,?,'',?,?,?,?,?,?,?)`, j.ID, uid, agentID, j.Kind, j.Status, j.Revision, j.Stage, encodeStages(j.Stages), j.CreatedAt, j.UpdatedAt)
	return j, err
}

func planActions(plan json.RawMessage) []json.RawMessage {
	var parsed struct {
		Actions []json.RawMessage `json:"actions"`
	}
	if json.Unmarshal(plan, &parsed) != nil {
		return nil
	}
	return parsed.Actions
}
