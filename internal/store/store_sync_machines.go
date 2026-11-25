package store

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"
)

// SyncFailure is a skill a machine reported it could not install or take off.
type SyncFailure struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// SyncMachine is one machine's row on the Sync page. State is, first that
// applies: paused (left alone by the person), needs-newer-controller (its reet
// is newer than this controller), needs-update (its reet is older than the
// skills protocol), blocked (allow-apply is off, or the machine is locked), failed, offline, updating
// (installing and removing), installing, removing, or ok. A machine off the
// skills protocol, or paused, gets no skill work, so it lists none. Failed and
// Waiting are by name; Waiting holds the skills it has still to install or
// take off that have not failed.
type SyncMachine struct {
	AgentID  string        `json:"agentId"`
	Host     string        `json:"host"`
	Paused   bool          `json:"paused"`
	Online   bool          `json:"online"`
	Protocol int           `json:"protocol"`
	State    string        `json:"state"`
	Failed   []SyncFailure `json:"failed"`
	Waiting  []string      `json:"waiting"`
	LastSeen int64         `json:"lastSeen"`
}

// skillRetrying is the state a failed row takes when someone asks for it to be
// tried again: it counts as work still to do until the machine reports again.
const skillRetrying = "retrying"

// skillRetryAnswerWithin is how long a retried skill reads as work still to
// do. A machine polls every few seconds and reports what it tried that cycle;
// past this, the poll that carried the retry or the report after it was lost,
// and the skill reads as failed again, for its earlier reason.
const skillRetryAnswerWithin = 2 * time.Minute

// reportedState is machine_skills' state as health reads it at a cutoff (a
// time in Unix ms, its one argument): a retry asked before then and not yet
// answered reads as failed again. m is machine_skills.
const reportedState = `CASE WHEN m.state='` + skillRetrying + `' AND m.at<? THEN 'failed' ELSE m.state END`

// retryCutoff is the cutoff reportedState takes at now.
func retryCutoff(now time.Time) int64 { return now.Add(-skillRetryAnswerWithin).UnixMilli() }

// skillRetryMaxNames bounds one Retry: every skill reet can hold.
const skillRetryMaxNames = 1024

// ErrNoSuchMachine is returned for a machine the account does not have.
var ErrNoSuchMachine = errors.New("No such machine on this account.")

// SkillRetryRefused says why a machine cannot be asked to try again.
type SkillRetryRefused string

func (e SkillRetryRefused) Error() string { return string(e) }

func (s *Store) migrateSkillRetries() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS skill_retries(user_id INTEGER NOT NULL,agent_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE,name TEXT NOT NULL COLLATE NOCASE,at INTEGER NOT NULL,PRIMARY KEY(user_id,agent_id,name))`)
	return err
}

// SyncMachinesFor lists every machine of the account by host, with what it is
// doing about the desired state at now.
func (s *Store) SyncMachinesFor(uid int64, now time.Time) ([]SyncMachine, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	desired, err := desiredSkills(tx, uid, now)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT a.id,a.host,a.paused,a.skills_protocol,a.last_seen,COALESCE(r.blocked,'') FROM sync_agents a LEFT JOIN machine_skill_reports r ON r.agent_id=a.id WHERE a.user_id=? ORDER BY a.host,a.id`, uid)
	if err != nil {
		return nil, err
	}
	out := []SyncMachine{}
	blocked := map[string]bool{}
	for rows.Next() {
		m := SyncMachine{Failed: []SyncFailure{}, Waiting: []string{}}
		var b string
		if err = rows.Scan(&m.AgentID, &m.Host, &m.Paused, &m.Protocol, &m.LastSeen, &b); err != nil {
			break
		}
		m.Online = now.UnixMilli()-m.LastSeen <= skillOfflineAfter.Milliseconds()
		blocked[m.AgentID] = b == "allow-apply" || b == "locked"
		out = append(out, m)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	reported := map[string]map[string][]reportedRow{}
	rows, err = tx.Query(`SELECT m.agent_id,m.name,`+reportedState+`,m.digest,m.detail FROM machine_skills m WHERE m.user_id=? ORDER BY m.folder`, retryCutoff(now), uid)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, name string
		var r reportedRow
		if err = rows.Scan(&id, &name, &r.state, &r.digest, &r.detail); err != nil {
			break
		}
		name = strings.ToLower(name)
		if reported[id] == nil {
			reported[id] = map[string][]reportedRow{}
		}
		reported[id][name] = append(reported[id][name], r)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		m := &out[i]
		switch {
		case m.Paused:
			m.State = "paused"
			continue
		case m.Protocol > CurrentSkillsProtocol:
			m.State = "needs-newer-controller"
			continue
		case m.Protocol < CurrentSkillsProtocol:
			m.State = "needs-update"
			continue
		}
		installing, removing := false, false
		look := func(name string, rows []reportedRow, want DesiredSkill, install bool) {
			switch machineSkillState(rows, want, install, !install) {
			case "failed":
				for _, r := range rows {
					if r.state == "failed" {
						m.Failed = append(m.Failed, SyncFailure{Name: name, Detail: r.detail})
						break
					}
				}
			case "pending":
				m.Waiting = append(m.Waiting, name)
				installing = installing || install
				removing = removing || !install
			}
		}
		for _, d := range desired.Active {
			look(d.Name, reported[m.AgentID][strings.ToLower(d.Name)], d, true)
		}
		for _, r := range desired.Remove {
			look(r.Name, reported[m.AgentID][strings.ToLower(r.Name)], DesiredSkill{}, false)
		}
		slices.SortFunc(m.Failed, func(a, b SyncFailure) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
		slices.SortFunc(m.Waiting, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
		switch {
		case blocked[m.AgentID]:
			m.State = "blocked"
		case len(m.Failed) > 0:
			m.State = "failed"
		case !m.Online:
			m.State = "offline"
		case installing && removing:
			m.State = "updating"
		case installing:
			m.State = "installing"
		case removing:
			m.State = "removing"
		default:
			m.State = "ok"
		}
	}
	return out, nil
}

// RequestSkillRetryFor asks the machine agentID to try names again on its
// next poll, and clears their failures there meanwhile: each reads as work
// still to do until the machine reports again. It returns the names recorded,
// trimmed and once each. A paused machine, or one off the skills protocol,
// does no skill work, so it cannot be asked.
func (s *Store) RequestSkillRetryFor(uid int64, agentID string, names []string, now time.Time) ([]string, error) {
	var kept []string
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || len(n) > 256 || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		kept = append(kept, n)
	}
	if len(kept) == 0 {
		return nil, SkillRetryRefused("Name the skills to try again.")
	}
	if len(kept) > skillRetryMaxNames {
		return nil, SkillRetryRefused("Too many skills to try again at once.")
	}
	at := now.UnixMilli()
	err := s.withTx(func(tx *sql.Tx) error {
		var paused bool
		var protocol int
		var host string
		if err := tx.QueryRow(`SELECT host,paused,skills_protocol FROM sync_agents WHERE id=? AND user_id=?`, agentID, uid).Scan(&host, &paused, &protocol); errors.Is(err, sql.ErrNoRows) {
			return ErrNoSuchMachine
		} else if err != nil {
			return err
		}
		switch {
		case paused:
			return SkillRetryRefused("Sync is paused on " + host + ". Turn it back on to try again.")
		case protocol < CurrentSkillsProtocol:
			return SkillRetryRefused("Update reet on " + host + " to keep its skills in sync.")
		case protocol > CurrentSkillsProtocol:
			return SkillRetryRefused(host + " runs a newer reet than this controller; update the controller to keep its skills in sync.")
		}
		for _, n := range kept {
			if _, err := tx.Exec(`INSERT INTO skill_retries(user_id,agent_id,name,at) VALUES(?,?,?,?) ON CONFLICT(user_id,agent_id,name) DO UPDATE SET at=excluded.at`, uid, agentID, n, at); err != nil {
				return err
			}
			res, err := tx.Exec(`UPDATE machine_skills SET state=?,at=? WHERE user_id=? AND agent_id=? AND name=? AND state='failed'`, skillRetrying, at, uid, agentID, n)
			if err != nil {
				return err
			}
			if cleared, _ := res.RowsAffected(); cleared == 0 {
				continue
			}
			if _, err = tx.Exec(`INSERT INTO skill_activity(user_id,at,agent_id,name,action,detail) VALUES(?,?,?,?,?,?)`, uid, at, agentID, n, "retrying", "asked to try again"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return kept, nil
}

// TakeSkillRetriesFor hands over, once, the names the machine has been asked
// to try again, oldest request first.
func (s *Store) TakeSkillRetriesFor(uid int64, agentID string) ([]string, error) {
	var names []string
	err := s.withTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT name FROM skill_retries WHERE user_id=? AND agent_id=? ORDER BY at,rowid`, uid, agentID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var n string
			if err = rows.Scan(&n); err != nil {
				break
			}
			names = append(names, n)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil || len(names) == 0 {
			return err
		}
		_, err = tx.Exec(`DELETE FROM skill_retries WHERE user_id=? AND agent_id=?`, uid, agentID)
		return err
	})
	return names, err
}
