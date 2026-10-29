package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var ErrMachineName = errors.New("machine names must be non-empty, at most 200 characters and contain no control characters")
var ErrMachineConflict = errors.New("these reports have different saved sync references for the same agent group; stop tracking one reference before merging")

type machineQuerier interface{ QueryRow(string, ...any) *sql.Row }

func resolveMachineHost(q machineQuerier, userID int64, host string) (string, error) {
	host = strings.TrimSpace(host)
	var canonical string
	err := q.QueryRow(`SELECT host FROM machine_aliases WHERE user_id=? AND alias=?`, userID, strings.ToLower(host)).Scan(&canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return host, nil
	}
	return canonical, err
}
func (s *Store) ResolveMachineHostFor(userID int64, host string) (string, error) {
	return resolveMachineHost(s.db, userID, host)
}
func (s *Store) MachineAliasesFor(userID int64) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT alias,host FROM machine_aliases WHERE user_id=? ORDER BY alias`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var alias, host string
		if err := rows.Scan(&alias, &host); err != nil {
			return nil, err
		}
		out[alias] = host
	}
	return out, rows.Err()
}
func validMachineName(host string) bool {
	return host != "" && len(host) <= 200 && !strings.ContainsFunc(host, unicode.IsControl)
}

// MergeMachinesFor consolidates an account's reports under a preferred name.
// The latest report wins per agent/project; inventories are never unioned into a fictitious scan.
// Aliases redirect later imports, and reference snapshots/revisions survive unchanged.
func (s *Store) MergeMachinesFor(userID int64, from, to string) error {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if !validMachineName(from) || !validMachineName(to) {
		return ErrMachineName
	}
	return s.withTx(func(tx *sql.Tx) error {
		source, err := resolveMachineHost(tx, userID, from)
		if err != nil {
			return err
		}
		destination, err := resolveMachineHost(tx, userID, to)
		if err != nil {
			return err
		}
		// A name already attached to this machine can become its preferred name again.
		if strings.EqualFold(destination, source) {
			destination = to
		}
		// Enrolled identities may be renamed, but two installations cannot be merged.
		var enrollments, running int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_agents WHERE user_id=? AND (host=? COLLATE NOCASE OR host=? COLLATE NOCASE)`, userID, source, destination).Scan(&enrollments); err != nil {
			return err
		}
		if enrollments > 1 {
			return errors.New("revoke one enrollment before merging two installed agents")
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_jobs j JOIN sync_agents a ON a.id=j.agent_id WHERE a.user_id=? AND (a.host=? COLLATE NOCASE OR a.host=? COLLATE NOCASE) AND j.status='running'`, userID, source, destination).Scan(&running); err != nil {
			return err
		}
		if running > 0 {
			return errors.New("wait for the machine's running job before renaming its reports")
		}
		// An alias belonging to another report selects that report’s preferred name.
		to = destination
		rows, err := tx.Query(`SELECT e.id,e.agent,e.project,e.path,e.first_seen,e.last_seen,COALESCE(sc.scanned_at,e.last_seen) FROM environments e LEFT JOIN environment_scans sc ON sc.env_id=e.id WHERE e.user_id=? AND e.host=? COLLATE NOCASE`, userID, source)
		if err != nil {
			return err
		}
		type entry struct {
			id, agent, project, path string
			first, last, scanned     int64
		}
		var entries []entry
		for rows.Next() {
			var e entry
			if err := rows.Scan(&e.id, &e.agent, &e.project, &e.path, &e.first, &e.last, &e.scanned); err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		mapping := map[string]string{}
		for _, e := range entries {
			mapping[e.id] = EnvironmentIDFor(userID, to, e.agent, e.project)
		}
		canonicalID := func(id string) string {
			if next, ok := mapping[id]; ok {
				return next
			}
			return id
		}
		for _, e := range entries {
			id := mapping[e.id]
			name := EnvironmentName(to, e.agent, e.project)
			if id == e.id {
				if _, err := tx.Exec(`UPDATE environments SET host=?,name=? WHERE id=? AND user_id=?`, to, name, id, userID); err != nil {
					return err
				}
				continue
			}
			var oldDesired, oldSource, newDesired, newSource string
			a := tx.QueryRow(`SELECT desired,source_id FROM sync_targets WHERE user_id=? AND target_id=?`, userID, e.id).Scan(&oldDesired, &oldSource)
			b := tx.QueryRow(`SELECT desired,source_id FROM sync_targets WHERE user_id=? AND target_id=?`, userID, id).Scan(&newDesired, &newSource)
			if a != nil && !errors.Is(a, sql.ErrNoRows) {
				return a
			}
			if b != nil && !errors.Is(b, sql.ErrNoRows) {
				return b
			}
			if a == nil && b == nil && (oldDesired != newDesired || canonicalID(oldSource) != canonicalID(newSource)) {
				return ErrMachineConflict
			}
			var scanned int64
			err = tx.QueryRow(`SELECT COALESCE(sc.scanned_at,e.last_seen) FROM environments e LEFT JOIN environment_scans sc ON sc.env_id=e.id WHERE e.id=? AND e.user_id=?`, id, userID).Scan(&scanned)
			exists := err == nil
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO environments(id,user_id,name,kind,agent,host,path,project,first_seen,last_seen) SELECT ?,user_id,?,kind,agent,?,path,project,first_seen,last_seen FROM environments WHERE id=? AND user_id=? ON CONFLICT(id) DO UPDATE SET host=excluded.host,name=excluded.name,first_seen=MIN(environments.first_seen,excluded.first_seen),last_seen=MAX(environments.last_seen,excluded.last_seen)`, id, name, to, e.id, userID); err != nil {
				return err
			}
			if !exists || e.scanned > scanned {
				if _, err := tx.Exec(`DELETE FROM skill_envs WHERE env_id=?`, id); err != nil {
					return err
				}
				if _, err := tx.Exec(`INSERT INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path) SELECT skill_id,?,installed_at,last_seen,source_path FROM skill_envs WHERE env_id=?`, id, e.id); err != nil {
					return err
				}
				if _, err := tx.Exec(`DELETE FROM environment_scans WHERE env_id=?`, id); err != nil {
					return err
				}
				if _, err := tx.Exec(`INSERT INTO environment_scans(env_id,scanned_at,complete,os,arch,version) SELECT ?,scanned_at,complete,os,arch,version FROM environment_scans WHERE env_id=?`, id, e.id); err != nil {
					return err
				}
				if _, err := tx.Exec(`UPDATE environments SET path=? WHERE id=?`, e.path, id); err != nil {
					return err
				}
			}
			if a == nil {
				if b == nil {
					_, err = tx.Exec(`DELETE FROM sync_targets WHERE user_id=? AND target_id=?`, userID, e.id)
				} else {
					_, err = tx.Exec(`UPDATE sync_targets SET target_id=? WHERE user_id=? AND target_id=?`, id, userID, e.id)
				}
				if err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`UPDATE sync_targets SET source_id=?,source_name=? WHERE user_id=? AND source_id=?`, id, name, userID, e.id); err != nil {
				return err
			}
			var oldMode, newMode, oldBase, newBase string
			oldErr := tx.QueryRow(`SELECT mode,baseline FROM sync_options WHERE target_id=?`, e.id).Scan(&oldMode, &oldBase)
			newErr := tx.QueryRow(`SELECT mode,baseline FROM sync_options WHERE target_id=?`, id).Scan(&newMode, &newBase)
			if oldErr == nil && newErr == nil && (oldMode != newMode || oldMode == "bidirectional" && oldBase != newBase) {
				return ErrMachineConflict
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO sync_options(target_id,mode,baseline,applied_revision) SELECT ?,mode,baseline,applied_revision FROM sync_options WHERE target_id=?`, id, e.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO sync_policies(target_id,follow_source,problem) SELECT ?,follow_source,problem FROM sync_policies WHERE target_id=?`, id, e.id); err != nil {
				return err
			}
			if !exists || e.scanned > scanned {
				if _, err := tx.Exec(`INSERT INTO scan_errors(env_id,error) SELECT ?,error FROM scan_errors WHERE env_id=? ON CONFLICT(env_id) DO UPDATE SET error=excluded.error`, id, e.id); err != nil {
					return err
				}
			}
			if err := moveSkillContentTargets(tx, userID, e.id, id); err != nil {
				return err
			}
			if err := moveSkillContentReceipts(tx, userID, e.id, id); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM environments WHERE id=? AND user_id=?`, e.id, userID); err != nil {
				return err
			}
		}
		if !strings.EqualFold(source, to) {
			if _, err := tx.Exec(`UPDATE sync_jobs SET status='cancelled',message='Machine renamed; preview again.',updated_at=? WHERE agent_id IN (SELECT id FROM sync_agents WHERE user_id=? AND host=? COLLATE NOCASE) AND status IN ('queued','awaiting-approval')`, now(), userID, source); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE sync_agents SET host=? WHERE user_id=? AND host=? COLLATE NOCASE`, to, userID, source); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE machine_aliases SET host=? WHERE user_id=? AND host=? COLLATE NOCASE`, to, userID, source); err != nil {
			return err
		}
		for _, alias := range []string{from, source, to} {
			if _, err := tx.Exec(`INSERT INTO machine_aliases(user_id,alias,host) VALUES(?,?,?) ON CONFLICT(user_id,alias) DO UPDATE SET host=excluded.host`, userID, strings.ToLower(alias), to); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ForgetMachineFor(userID int64, host string) error {
	host, err := s.ResolveMachineHostFor(userID, host)
	if err != nil {
		return err
	}
	if !validMachineName(host) {
		return ErrMachineName
	}
	// Retain aliases so a future scan cannot resurrect duplicate names.
	_, err = s.db.Exec(`DELETE FROM environments WHERE user_id=? AND host=? COLLATE NOCASE`, userID, host)
	if err != nil {
		return fmt.Errorf("forget machine: %w", err)
	}
	return nil
}

func (s *Store) GroupMachinesFor(userID int64, envs []SyncEnvironment) ([]Machine, error) {
	aliases, err := s.MachineAliasesFor(userID)
	if err != nil {
		return nil, err
	}
	machines := GroupMachines(envs)
	for i := range machines {
		for alias, host := range aliases {
			if strings.EqualFold(host, machines[i].Host) && !strings.EqualFold(alias, host) {
				machines[i].Aliases = append(machines[i].Aliases, alias)
			}
		}
		sort.Strings(machines[i].Aliases)
	}
	return machines, nil
}
