package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// installAgentOnline is how recently a machine's connector must have been seen
// to count as connected. It is the window the sync UI uses for "online"
// (AGENT_ONLINE_WINDOW in web/src/lib/machines.ts, 90 seconds), the same one
// the agent health hub uses for a fresh report.
const installAgentOnline = 90 * time.Second

// InstallError is a refusal with a stable code and a sentence written for the
// person. Every refusal leaves the database untouched.
type InstallError struct{ Code, Message string }

func (e *InstallError) Error() string { return e.Message }

func installRefused(code, message string) error { return &InstallError{Code: code, Message: message} }

// InstallRequest names one published hosted skill to put on one machine.
// AgentID is the session machine's sync agent; Host is its resolved name.
type InstallRequest struct{ Tool, Host, AgentID, Slug, Origin string }

type InstallResult struct {
	JobID     string
	Reused    bool
	Directory string
	Host      string
}

// MergeDesired returns existing plus add, keeping order and adding the slug
// once. It never writes into the argument or its spare capacity.
func MergeDesired(existing []SyncSkill, add SyncSkill) []SyncSkill {
	out := make([]SyncSkill, 0, len(existing)+1)
	out = append(out, existing...)
	for _, sk := range existing {
		if sk.Slug == add.Slug {
			return out
		}
	}
	return append(out, add)
}

// desiredStatus says how add stands against a saved set: present when an entry
// is the same skill (slug, ID, source and source type), conflict when an entry
// has the slug but is a different skill.
func desiredStatus(existing []SyncSkill, add SyncSkill) (present, conflict bool) {
	for _, sk := range existing {
		if sk.Slug != add.Slug {
			continue
		}
		if sk.ID == add.ID && sk.Source == add.Source && sk.SourceType == add.SourceType {
			present = true
		} else {
			conflict = true
		}
	}
	return present, conflict
}

// RemoveDesired returns existing without the slug, keeping order. The result
// is never nil.
func RemoveDesired(existing []SyncSkill, slug string) []SyncSkill {
	out := make([]SyncSkill, 0, len(existing))
	for _, sk := range existing {
		if sk.Slug != slug {
			out = append(out, sk)
		}
	}
	return out
}

// InstallHostedSkillFor merges one published hosted skill into the saved set of
// the machine's user-level skills folder and queues a preview job for it. Only
// preview jobs are queued, so the plan is always shown and approved by hash.
// The merge only adds: an existing set keeps its skills and mode, and an
// additive set is created only when there was none. Every check runs before
// the first write, in the one transaction.
func (s *Store) InstallHostedSkillFor(uid int64, in InstallRequest) (InstallResult, error) {
	var res InstallResult
	if !Syncable(in.Tool) {
		return res, installRefused("unsupported-agent", "Skills can't be installed for "+in.Tool+" from reet yet.")
	}
	// Read before the transaction: this helper uses the connection pool.
	published, hostedErr := s.SyncHostedFor(uid, in.Origin)
	err := s.withTx(func(tx *sql.Tx) error {
		host, err := resolveMachineHost(tx, uid, in.Host)
		if err != nil {
			return err
		}
		var envID, dir string
		var complete bool
		var scanned int64
		err = tx.QueryRow(`SELECT e.id,e.path,sc.complete,sc.scanned_at FROM environments e JOIN environment_scans sc ON sc.env_id=e.id WHERE e.user_id=? AND e.host=? COLLATE NOCASE AND e.agent=? AND e.project=''`, uid, host, in.Tool).Scan(&envID, &dir, &complete, &scanned)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err != nil || !complete || scanned < time.Now().Add(-14*24*time.Hour).UnixMilli() {
			return installRefused("no-scan", "reet hasn't read "+host+"'s "+in.Tool+" skills folder recently. Run a scan on "+host+", then try again.")
		}
		offline := installRefused("connector-offline", host+" isn't connected to reet right now, so it can't install this skill.")
		var agentHost string
		var seen int64
		var paused bool
		err = tx.QueryRow(`SELECT host,last_seen,paused FROM sync_agents WHERE id=? AND user_id=?`, in.AgentID, uid).Scan(&agentHost, &seen, &paused)
		if errors.Is(err, sql.ErrNoRows) {
			return offline
		}
		if err != nil {
			return err
		}
		if agentHost, err = resolveMachineHost(tx, uid, agentHost); err != nil {
			return err
		}
		if paused || !strings.EqualFold(agentHost, host) || seen < time.Now().Add(-installAgentOnline).UnixMilli() {
			return offline
		}
		var skill SyncSkill
		found := false
		if hostedErr != nil {
			return hostedErr
		}
		for _, sk := range published {
			if sk.Slug == in.Slug {
				skill, found = sk, true
				break
			}
		}
		if !found {
			return installRefused("unpublished", "Publish this skill to install it.")
		}
		actual, err := environmentSkills(tx, uid, envID)
		if err != nil {
			return err
		}
		for _, sk := range actual {
			if sk.Slug == in.Slug {
				return installRefused("already-installed", "This skill is already on "+host+" for "+in.Tool+".")
			}
		}
		// A policy row can outlive its saved set; only a saved set that
		// follows a source refuses the install.
		var follow bool
		if err = tx.QueryRow(`SELECT p.follow_source FROM sync_policies p JOIN sync_targets t ON t.target_id=p.target_id AND t.user_id=? WHERE p.target_id=?`, uid, envID).Scan(&follow); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if follow {
			return installRefused("follows-source", host+"'s skills follow another machine, so this skill would be removed the next time it syncs.")
		}
		var data, oldRevision string
		created := false
		err = tx.QueryRow(`SELECT desired,revision FROM sync_targets WHERE user_id=? AND target_id=?`, uid, envID).Scan(&data, &oldRevision)
		if errors.Is(err, sql.ErrNoRows) {
			created, data = true, "[]"
		} else if err != nil {
			return err
		}
		var existing []SyncSkill
		if err = json.Unmarshal([]byte(data), &existing); err != nil {
			return err
		}
		present, conflict := desiredStatus(existing, skill)
		if conflict {
			return installRefused("slug-conflict", "A different skill named "+in.Slug+" is already in "+host+"'s saved list. Remove it on the Sync page first.")
		}
		rows, err := tx.Query(`SELECT j.id,COALESCE(i.slug,'') FROM sync_jobs j LEFT JOIN skill_installs i ON i.job_id=j.id WHERE j.user_id=? AND j.target_id=? AND j.status IN ('queued','running','awaiting-approval') ORDER BY j.created_at,j.id`, uid, envID)
		if err != nil {
			return err
		}
		pending, same := 0, ""
		for rows.Next() {
			var id, slug string
			if err = rows.Scan(&id, &slug); err != nil {
				rows.Close()
				return err
			}
			pending++
			if slug == in.Slug && same == "" {
				same = id
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if same != "" {
			res = InstallResult{JobID: same, Reused: true, Directory: dir, Host: host}
			return nil
		}
		if pending > 0 {
			return installRefused("install-in-progress", "Another install is waiting on "+host+". Finish or cancel it first.")
		}
		// QueueSyncJob's machine rule, in this transaction: a scan or apply
		// anywhere on the machine blocks it, as does another preview here.
		var busy int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE agent_id=? AND status IN ('queued','running','awaiting-approval') AND (?='scan' OR kind!='preview' OR target_id=?)`, in.AgentID, "preview", envID).Scan(&busy); err != nil {
			return err
		}
		if busy > 0 {
			return installRefused("install-in-progress", host+" is busy with another sync job. Finish or cancel it, then try again.")
		}
		revision, at := oldRevision, now()
		if !present {
			merged, _ := json.Marshal(MergeDesired(existing, skill))
			revision = syncID()
			if created {
				baseline, _ := json.Marshal(actual)
				if _, err = tx.Exec(`INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,?,?,?,?,?)`, uid, envID, "hosted", "My published reet.sh skills", string(merged), revision, at); err != nil {
					return err
				}
				if _, err = tx.Exec(`INSERT INTO sync_policies(target_id,follow_source) VALUES(?,0) ON CONFLICT(target_id) DO UPDATE SET follow_source=0,problem=''`, envID); err != nil {
					return err
				}
				if _, err = tx.Exec(`INSERT INTO sync_options(target_id,mode,baseline) VALUES(?,'additive',?) ON CONFLICT(target_id) DO UPDATE SET mode='additive',baseline=excluded.baseline`, envID, string(baseline)); err != nil {
					return err
				}
			} else if _, err = tx.Exec(`UPDATE sync_targets SET desired=?,revision=?,updated_at=? WHERE user_id=? AND target_id=?`, string(merged), revision, at, uid, envID); err != nil {
				return err
			}
		}
		jobID := syncID()
		if _, err = tx.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, jobID, uid, in.AgentID, envID, "preview", "queued", revision, at, at); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO skill_installs(job_id,user_id,target_id,slug,created_target,added,revision,prev_revision) VALUES(?,?,?,?,?,?,?,?)`, jobID, uid, envID, in.Slug, created, !present, revision, oldRevision); err != nil {
			return err
		}
		res = InstallResult{JobID: jobID, Directory: dir, Host: host}
		return nil
	})
	if err != nil {
		return InstallResult{}, err
	}
	return res, nil
}

// CancelInstallFor cancels a queued or awaiting-approval install job and takes
// the skill back out of the saved set (see revertInstallTx). reverted is false
// when the install changed nothing or the set has changed since.
func (s *Store) CancelInstallFor(uid int64, jobID string) (reverted bool, err error) {
	err = s.withTx(func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRow(`SELECT j.status FROM sync_jobs j JOIN skill_installs i ON i.job_id=j.id WHERE j.id=? AND j.user_id=? AND i.user_id=?`, jobID, uid, uid).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status == "running" {
			return installRefused("cannot-cancel", "This install is already running and can't be cancelled.")
		}
		if status != "queued" && status != "awaiting-approval" {
			return installRefused("cannot-cancel", "This install has already finished.")
		}
		if _, err := tx.Exec(`UPDATE sync_jobs SET status='cancelled',updated_at=? WHERE id=?`, now(), jobID); err != nil {
			return err
		}
		reverted, err = revertInstallTx(tx, jobID)
		return err
	})
	if err != nil {
		return false, err
	}
	return reverted, nil
}

// revertInstallTx undoes an install's merge once its job has ended without
// applying (failed or cancelled, by any path). A skill an install put in a
// saved set stays there only while its job is pending or once it applied, so
// automatic sync can never install what the person did not approve.
//
// It does nothing for a job that is not an install, for an install that did
// not add the slug, or when the set's revision is no longer the one the install
// produced (someone changed the set since, or it was deleted and saved again).
// A set the install created is deleted; otherwise the slug is removed and the
// set gets back the revision it had before, so no jobless revision appears and
// the earlier jobs still count for Keep synced.
func revertInstallTx(tx *sql.Tx, jobID string) (bool, error) {
	var uid int64
	var target, slug, revision, prev string
	var created, added bool
	err := tx.QueryRow(`SELECT user_id,target_id,slug,created_target,added,revision,prev_revision FROM skill_installs WHERE job_id=?`, jobID).Scan(&uid, &target, &slug, &created, &added, &revision, &prev)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil || !added {
		return false, err
	}
	var data, current string
	err = tx.QueryRow(`SELECT desired,revision FROM sync_targets WHERE user_id=? AND target_id=?`, uid, target).Scan(&data, &current)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && current != revision) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if created {
		if _, err := tx.Exec(`DELETE FROM sync_targets WHERE user_id=? AND target_id=?`, uid, target); err != nil {
			return false, err
		}
		for _, table := range []string{"sync_policies", "sync_options"} {
			if _, err := tx.Exec(`DELETE FROM `+table+` WHERE target_id IN (SELECT id FROM environments WHERE id=? AND user_id=?)`, target, uid); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	var existing []SyncSkill
	if err := json.Unmarshal([]byte(data), &existing); err != nil {
		return false, err
	}
	if prev == "" {
		// An install recorded before prev_revision existed.
		prev = syncID()
	}
	out, _ := json.Marshal(RemoveDesired(existing, slug))
	if _, err := tx.Exec(`UPDATE sync_targets SET desired=?,revision=?,updated_at=? WHERE user_id=? AND target_id=?`, string(out), prev, now(), uid, target); err != nil {
		return false, err
	}
	return true, nil
}

// staleJobMessage is the reaper's message: an update may still be restarting,
// any other job lost its agent.
const staleJobMessage = `CASE WHEN kind='update' THEN 'The machine did not report back. It may still be restarting; check its build and try again.' ELSE 'Agent disconnected or timed out; inspect local files and preview again.' END`

// endSyncJobsTx ends every job matching where (columns qualified as j.) with
// status, 'failed' or 'cancelled', and message, then reverts each install
// among them. message is a literal, or staleJobMessage for the reaper. Every
// bulk path that ends jobs without applying goes through it.
func endSyncJobsTx(tx *sql.Tx, status, message, where string, args ...any) error {
	rows, err := tx.Query(`SELECT j.id FROM sync_jobs j JOIN skill_installs i ON i.job_id=j.id WHERE `+where, args...)
	if err != nil {
		return err
	}
	var installs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		installs = append(installs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	set, values := `status=?,message=?,updated_at=?`, []any{status, message, now()}
	if message == staleJobMessage {
		set, values = `status=?,message=`+staleJobMessage+`,updated_at=?`, []any{status, now()}
	}
	if _, err := tx.Exec(`UPDATE sync_jobs AS j SET `+set+` WHERE `+where, append(values, args...)...); err != nil {
		return err
	}
	for _, id := range installs {
		if _, err := revertInstallTx(tx, id); err != nil {
			return err
		}
	}
	return nil
}

// reapStaleSyncJobsTx fails running jobs whose agent has not reported for ten
// minutes, for the jobs matching where. Expired work is never replayed: it may
// have partially applied.
func reapStaleSyncJobsTx(tx *sql.Tx, where string, args ...any) error {
	return endSyncJobsTx(tx, "failed", staleJobMessage, `j.status='running' AND j.updated_at<? AND `+where, append([]any{time.Now().Add(-10 * time.Minute).UnixMilli()}, args...)...)
}

func (s *Store) migrateSkillInstalls() error {
	ok, err := hasColumn(s.db, "skill_installs", "prev_revision")
	if err != nil || ok {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE skill_installs ADD COLUMN prev_revision TEXT NOT NULL DEFAULT ''`)
	return err
}
