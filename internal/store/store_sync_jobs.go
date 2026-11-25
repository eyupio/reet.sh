package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// sync_jobs holds the one job a machine still takes: a binary update, which a
// person or the auto-update queues. Skills reach machines through the desired
// state, and a machine scans itself every minute. The older engine's finished
// jobs stay as history.

func (s *Store) QueueAgentUpdate(uid int64, agentID, version string) (SyncJob, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "latest"
	}
	validTag := strings.HasPrefix(version, "v") || strings.HasPrefix(version, "V")
	validVersion := version == "latest" || version == "dev" || validTag
	if !validVersion {
		return SyncJob{}, errors.New("choose latest, dev or a release tag")
	}
	j := SyncJob{ID: syncID(), AgentID: agentID, Kind: "update", Status: "queued", Revision: version, Plan: json.RawMessage(`{}`), Stage: UpdateStageQueued, Stages: NewUpdateStages(), CreatedAt: now(), UpdatedAt: now()}
	err := s.withTx(func(tx *sql.Tx) error {
		var owned int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_agents WHERE id=? AND user_id=?`, agentID, uid).Scan(&owned); err != nil {
			return err
		}
		if owned == 0 {
			return ErrNotFound
		}
		// Expired work is never replayed automatically, so it must not keep
		// blocking a person who asks for an update.
		if err := reapStaleSyncJobsTx(tx, `j.user_id=? AND j.agent_id=?`, uid, agentID); err != nil {
			return err
		}
		var running, pendingUpdate int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE user_id=? AND agent_id=? AND status='running'`, uid, agentID).Scan(&running); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE user_id=? AND agent_id=? AND kind='update' AND status IN ('queued','awaiting-approval')`, uid, agentID).Scan(&pendingUpdate); err != nil {
			return err
		}
		// A queued scan is safe to wait behind: the agent runs one job at a
		// time, so the update keeps its turn after it.
		if running > 0 {
			return errors.New("a job is still running on this machine; wait for it to finish")
		}
		if pendingUpdate > 0 {
			return errors.New("an update is already queued for this machine; wait for it or cancel it")
		}
		if _, err := tx.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,revision,stage,stages,created_at,updated_at) VALUES(?,?,?,'',?,?,?,?,?,?,?)`, j.ID, uid, agentID, j.Kind, j.Status, j.Revision, j.Stage, encodeStages(j.Stages), j.CreatedAt, j.UpdatedAt); err != nil {
			return err
		}
		// A machine that cannot update is queued again every hour; keep its
		// newest twenty finished updates so they do not crowd out the others'.
		_, err := tx.Exec(`DELETE FROM sync_jobs WHERE rowid IN (SELECT rowid FROM sync_jobs WHERE user_id=? AND agent_id=? AND kind='update' AND status IN ('succeeded','failed','rolled_back','cancelled') ORDER BY created_at DESC,rowid DESC LIMIT -1 OFFSET 20)`, uid, agentID)
		return err
	})
	return j, err
}

// staleJobMessage is the reaper's message: an update may still be restarting,
// any other job lost its agent.
const staleJobMessage = `CASE WHEN kind='update' THEN 'The machine did not report back. It may still be restarting; check its build and try again.' ELSE 'Agent disconnected or timed out; it reports again on its next scan.' END`

// endSyncJobsTx ends every job matching where (columns qualified as j.) with
// status, 'failed' or 'cancelled', and message. message is a literal, or
// staleJobMessage for the reaper.
func endSyncJobsTx(tx *sql.Tx, status, message, where string, args ...any) error {
	set, values := `status=?,message=?,updated_at=?`, []any{status, message, now()}
	if message == staleJobMessage {
		set, values = `status=?,message=`+staleJobMessage+`,updated_at=?`, []any{status, now()}
	}
	_, err := tx.Exec(`UPDATE sync_jobs AS j SET `+set+` WHERE `+where, append(values, args...)...)
	return err
}

// reapStaleSyncJobsTx fails running jobs whose agent has not reported for ten
// minutes, for the jobs matching where. Expired work is never replayed.
func reapStaleSyncJobsTx(tx *sql.Tx, where string, args ...any) error {
	return endSyncJobsTx(tx, "failed", staleJobMessage, `j.status='running' AND j.updated_at<? AND `+where, append([]any{time.Now().Add(-10 * time.Minute).UnixMilli()}, args...)...)
}

func scanJob(row interface{ Scan(...any) error }) (SyncJob, error) {
	var j SyncJob
	var plan, stages string
	err := row.Scan(&j.ID, &j.AgentID, &j.TargetID, &j.Kind, &j.Status, &j.Revision, &plan, &j.Hash, &j.Message, &j.Stage, &stages, &j.CreatedAt, &j.UpdatedAt)
	j.Plan = json.RawMessage(plan)
	j.Stages = decodeStages(stages)
	return j, err
}

const jobCols = `id,agent_id,target_id,kind,status,revision,plan,hash,message,stage,stages,created_at,updated_at`

// SyncJobsFor is the account's latest jobs, the older engine's finished ones
// included, which stay as history.
func (s *Store) SyncJobsFor(uid int64) ([]SyncJob, error) {
	if err := s.withTx(func(tx *sql.Tx) error { return reapStaleSyncJobsTx(tx, `j.user_id=?`, uid) }); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT `+jobCols+` FROM sync_jobs WHERE user_id=? ORDER BY created_at DESC LIMIT 100`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SyncJob{}
	for rows.Next() {
		j, e := scanJob(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// QueueSyncJob is refused for every machine: it follows the library and
// checks its skills on every poll, so there is no job to queue. An update is
// queued with QueueAgentUpdate.
func (s *Store) QueueSyncJob(uid int64, agentID, kind string) (SyncJob, error) {
	var host string
	if err := s.db.QueryRow(`SELECT host FROM sync_agents WHERE id=? AND user_id=?`, agentID, uid).Scan(&host); err != nil {
		return SyncJob{}, ErrNotFound
	}
	host, err := s.ResolveMachineHostFor(uid, host)
	if err != nil {
		return SyncJob{}, err
	}
	return SyncJob{}, fmt.Errorf("%s follows your library; its skills are checked on every poll, so there's nothing to scan.", host)
}

// ControlSyncJob cancels a job that has not started.
func (s *Store) ControlSyncJob(uid int64, id, action string) error {
	if action != "cancel" {
		return errors.New("a job can only be cancelled")
	}
	return s.withTx(func(tx *sql.Tx) error {
		j, err := scanJob(tx.QueryRow(`SELECT `+jobCols+` FROM sync_jobs WHERE id=? AND user_id=?`, id, uid))
		if err != nil {
			return ErrNotFound
		}
		if j.Status == "running" {
			return errors.New("operation already running; pause the machine to prevent further jobs, then inspect its result")
		}
		if j.Status != "queued" && j.Status != "awaiting-approval" {
			return errors.New("job already finished")
		}
		stage, stages := j.Stage, j.Stages
		if len(stages) > 0 {
			stage, stages = finishStages(stages, "cancelled", "")
		}
		_, err = tx.Exec(`UPDATE sync_jobs SET status='cancelled',stage=?,stages=?,updated_at=? WHERE id=?`, stage, encodeStages(stages), now(), id)
		return err
	})
}

// ClaimSyncJob hands a machine its next queued update, one at a time. A paused
// machine is sent none, and a job of the older engine left queued stays where
// it is.
func (s *Store) ClaimSyncJob(uid int64, a SyncAgent) (*SyncJob, error) {
	if a.Paused {
		return nil, nil
	}
	var job *SyncJob
	err := s.withTx(func(tx *sql.Tx) error {
		// Expired work is never replayed automatically.
		err := reapStaleSyncJobsTx(tx, `j.agent_id=?`, a.ID)
		if err != nil {
			return err
		}
		var running int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE agent_id=? AND status='running'`, a.ID).Scan(&running); err != nil || running > 0 {
			return err
		}
		j, err := scanJob(tx.QueryRow(`SELECT `+jobCols+` FROM sync_jobs WHERE user_id=? AND agent_id=? AND status='queued' AND kind='update' ORDER BY created_at LIMIT 1`, uid, a.ID))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE sync_jobs SET status='running',updated_at=? WHERE id=?`, now(), j.ID); err != nil {
			return err
		}
		j.Status = "running"
		if j.Kind == "update" {
			// The agent is collecting the job now; the download is the first
			// step it can be seen working on.
			stages := j.Stages
			if len(stages) == 0 {
				stages = NewUpdateStages()
			}
			advanceStages(stages, UpdateStageDownload, "The machine is downloading the release.")
			j.Stage, j.Stages = UpdateStageDownload, stages
			if _, err = tx.Exec(`UPDATE sync_jobs SET stage=?,stages=? WHERE id=?`, j.Stage, encodeStages(stages), j.ID); err != nil {
				return err
			}
		}
		job = &j
		return nil
	})
	return job, err
}

// SyncJobResultRefused marks a result the controller will never accept for
// this job. The agent may discard its durable copy; any other FinishSyncJob
// error is transient and should be retried with the same result.
type SyncJobResultRefused struct{ Reason string }

func (e SyncJobResultRefused) Error() string { return e.Reason }

func refuseSyncJobResult(reason string) error { return SyncJobResultRefused{Reason: reason} }

// FinishSyncJob takes a machine's result for its running update or scan: it
// succeeded or failed, or an update rolled back. The same result sent again,
// after a lost acknowledgement, is taken without changing anything.
func (s *Store) FinishSyncJob(uid int64, agentID, id, status, message string) error {
	if len(message) > 8000 {
		message = message[:8000]
	}
	return s.withTx(func(tx *sql.Tx) error {
		j, err := scanJob(tx.QueryRow(`SELECT `+jobCols+` FROM sync_jobs WHERE user_id=? AND agent_id=? AND id=?`, uid, agentID, id))
		if errors.Is(err, sql.ErrNoRows) {
			return refuseSyncJobResult("job not found")
		}
		if err != nil {
			return err
		}
		known := status == "succeeded" || status == "failed" || (j.Kind == "update" && status == "rolled_back")
		if !known || (j.Kind != "update" && j.Kind != "scan") {
			return refuseSyncJobResult("invalid job result")
		}
		if j.Status == status && j.Message == message {
			return nil
		}
		if j.Status != "running" {
			// An update the controller gave up on can still report the truth:
			// the machine did restart, or it rolled itself back. Taking that
			// late account is better than showing a failure that is not one.
			lateSuccess := j.Status == "failed" && j.Kind == "update" &&
				strings.HasPrefix(j.Message, "The machine did not report back")
			if !lateSuccess || (status != "succeeded" && status != "rolled_back") {
				return refuseSyncJobResult("job is no longer running")
			}
		}
		stage, stages := j.Stage, j.Stages
		if len(stages) > 0 {
			stage, stages = finishStages(stages, status, message)
		}
		_, err = tx.Exec(`UPDATE sync_jobs SET status=?,message=?,stage=?,stages=?,updated_at=? WHERE id=?`, status, message, stage, encodeStages(stages), now(), id)
		return err
	})
}
