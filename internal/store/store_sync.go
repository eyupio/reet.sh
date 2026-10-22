package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

var ErrSync = errors.New("sync requires two different, recently scanned environments owned by this account")

type MachineInfo struct {
	Host    string `json:"host"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Version string `json:"version"`
	Server  bool   `json:"server"`
}

type SyncEnvironment struct {
	Error string `json:"error,omitempty"`
	Environment
	ScannedAt int64       `json:"scannedAt"`
	Complete  bool        `json:"complete"`
	OS        string      `json:"os"`
	Arch      string      `json:"arch"`
	Version   string      `json:"version"`
	Skills    []SyncSkill `json:"skills"`
}

type SyncSkill struct {
	URL        string `json:"url,omitempty"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Source     string `json:"source"`
	SourceType string `json:"sourceType"`
}

type SyncTarget struct {
	Mode            string      `json:"mode"`
	Baseline        []SyncSkill `json:"baseline"`
	AppliedRevision string      `json:"appliedRevision"`
	HostAliases     []string    `json:"hostAliases,omitempty"`
	TargetAliases   []string    `json:"targetAliases,omitempty"`
	Target          Environment `json:"target"`
	SourceID        string      `json:"sourceId"`
	SourceName      string      `json:"sourceName"`
	Desired         []SyncSkill `json:"desired"`
	Missing         []SyncSkill `json:"missing"`
	Extra           []SyncSkill `json:"extra"`
	Revision        string      `json:"revision"`
	UpdatedAt       int64       `json:"updatedAt"`
}

func (s *Store) RecordInventoryFor(userID int64, envID string, complete bool, machine MachineInfo) error {
	_, err := s.db.Exec(`INSERT INTO environment_scans(env_id,scanned_at,complete,os,arch,version)
 SELECT id,?,?,?,?,? FROM environments WHERE id=? AND user_id=?
 ON CONFLICT(env_id) DO UPDATE SET scanned_at=excluded.scanned_at,complete=excluded.complete,os=excluded.os,arch=excluded.arch,version=excluded.version`, now(), complete, machine.OS, machine.Arch, machine.Version, envID, userID)
	return err
}

func (s *Store) RegisterMachineFor(userID int64, info MachineInfo) error {
	canonical, err := s.ResolveMachineHostFor(userID, info.Host)
	if err != nil {
		return err
	}
	s.autoPruneServerEnvironments(userID)
	info.Host = canonical
	agent := "reet"
	if info.Server {
		agent = "reet-server"
	}
	if err := s.TouchEnvironmentFor(userID, info.Host, agent, "", ""); err != nil {
		return err
	}
	return s.RecordInventoryFor(userID, EnvironmentIDFor(userID, info.Host, agent, ""), false, info)
}

func (s *Store) SyncEnvironmentsFor(userID int64) ([]SyncEnvironment, error) {
	envs, err := s.ListEnvironmentsFor(userID)
	if err != nil {
		return nil, err
	}
	out := make([]SyncEnvironment, 0, len(envs))
	for _, env := range envs {
		item := SyncEnvironment{Environment: env}
		err := s.db.QueryRow(`SELECT scanned_at,complete,os,arch,version FROM environment_scans WHERE env_id=?`, env.ID).Scan(&item.ScannedAt, &item.Complete, &item.OS, &item.Arch, &item.Version)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		_ = s.db.QueryRow(`SELECT error FROM scan_errors WHERE env_id=?`, env.ID).Scan(&item.Error)
		item.Skills, err = s.EnvironmentSkillsFor(userID, env.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type environmentQuerier interface {
	Query(string, ...any) (*sql.Rows, error)
}

func (s *Store) EnvironmentSkillsFor(userID int64, envID string) ([]SyncSkill, error) {
	return environmentSkills(s.db, userID, envID)
}
func environmentSkills(query environmentQuerier, userID int64, envID string) ([]SyncSkill, error) {
	rows, err := query.Query(`SELECT s.id,s.name,s.slug,s.source,s.source_type,s.url FROM skills s JOIN skill_envs se ON se.skill_id=s.id JOIN environments e ON e.id=se.env_id WHERE e.id=? AND e.user_id=? ORDER BY s.id`, envID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SyncSkill{}
	for rows.Next() {
		var item SyncSkill
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.Source, &item.SourceType, &item.URL); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func Syncable(agent string) bool {
	return agent == "claude" || agent == "agents" || agent == "cursor" || agent == "opencode"
}

func (s *Store) SaveSyncTargetFor(userID int64, sourceID, targetID string) error {
	if sourceID == targetID {
		return ErrSync
	}
	return s.withTx(func(tx *sql.Tx) error {
		type observed struct {
			name, agent, project string
			scanned              int64
			complete             bool
		}
		read := func(id string) (observed, error) {
			var item observed
			err := tx.QueryRow(`SELECT e.name,e.agent,e.project,sc.scanned_at,sc.complete FROM environments e JOIN environment_scans sc ON sc.env_id=e.id WHERE e.id=? AND e.user_id=?`, id, userID).Scan(&item.name, &item.agent, &item.project, &item.scanned, &item.complete)
			return item, err
		}
		source, err := read(sourceID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSync
		}
		if err != nil {
			return err
		}
		target, err := read(targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSync
		}
		if err != nil {
			return err
		}
		cutoff := time.Now().Add(-14 * 24 * time.Hour).UnixMilli()
		if !source.complete || !target.complete || source.scanned < cutoff || target.scanned < cutoff || !Syncable(target.agent) || (target.project != "" && !strings.HasPrefix(target.project, "/")) {
			return ErrSync
		}
		desired, err := environmentSkills(tx, userID, sourceID)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, skill := range desired {
			if skill.SourceType == "local" || skill.Source == "" || seen[skill.Slug] {
				return errors.New("link local skills to a source and resolve duplicate skill names before syncing")
			}
			seen[skill.Slug] = true
		}
		data, err := json.Marshal(desired)
		if err != nil {
			return err
		}
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,?,?,?,?,?)
  ON CONFLICT(user_id,target_id) DO UPDATE SET source_id=excluded.source_id,source_name=excluded.source_name,desired=excluded.desired,revision=excluded.revision,updated_at=excluded.updated_at`, userID, targetID, sourceID, source.name, string(data), hex.EncodeToString(random[:]), now())
		if err != nil {
			return err
		}
		actual, err := environmentSkills(tx, userID, targetID)
		if err != nil {
			return err
		}
		baseline, err := json.Marshal(actual)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO sync_options(target_id,mode,baseline) VALUES(?,'mirror',?) ON CONFLICT(target_id) DO UPDATE SET mode='mirror',baseline=excluded.baseline`, targetID, string(baseline))
		return err
	})
}

func (s *Store) SyncTargetsFor(userID int64) ([]SyncTarget, error) {
	rows, err := s.db.Query(`SELECT target_id,source_id,source_name,desired,revision,updated_at FROM sync_targets WHERE user_id=? ORDER BY target_id`, userID)
	if err != nil {
		return nil, err
	}
	out := []SyncTarget{}
	for rows.Next() {
		var item SyncTarget
		var data string
		if err := rows.Scan(&item.Target.ID, &item.SourceID, &item.SourceName, &data, &item.Revision, &item.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &item.Desired); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	envs, err := s.ListEnvironmentsFor(userID)
	if err != nil {
		return nil, err
	}
	aliases, err := s.MachineAliasesFor(userID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Mode = "mirror"
		var baseline string
		err := s.db.QueryRow(`SELECT mode,baseline,applied_revision FROM sync_options WHERE target_id=?`, out[i].Target.ID).Scan(&out[i].Mode, &baseline, &out[i].AppliedRevision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if baseline != "" {
			if err := json.Unmarshal([]byte(baseline), &out[i].Baseline); err != nil {
				return nil, err
			}
		}
		for _, env := range envs {
			if env.ID == out[i].Target.ID {
				out[i].Target = env
			}
		}
		for alias, host := range aliases {
			if strings.EqualFold(host, out[i].Target.Host) {
				out[i].HostAliases = append(out[i].HostAliases, alias)
				out[i].TargetAliases = append(out[i].TargetAliases, EnvironmentIDFor(userID, alias, out[i].Target.Agent, out[i].Target.Project))
			}
		}
		sort.Strings(out[i].HostAliases)
		sort.Strings(out[i].TargetAliases)
		actual, err := s.EnvironmentSkillsFor(userID, out[i].Target.ID)
		if err != nil {
			return nil, err
		}
		desiredIDs, actualIDs := map[string]bool{}, map[string]bool{}
		for _, sk := range out[i].Desired {
			desiredIDs[sk.ID] = true
		}
		for _, sk := range actual {
			actualIDs[sk.ID] = true
		}
		out[i].Missing = []SyncSkill{}
		out[i].Extra = []SyncSkill{}
		for _, sk := range out[i].Desired {
			if !actualIDs[sk.ID] {
				out[i].Missing = append(out[i].Missing, sk)
			}
		}
		for _, sk := range actual {
			if !desiredIDs[sk.ID] {
				out[i].Extra = append(out[i].Extra, sk)
			}
		}
	}
	return out, nil
}

func (s *Store) DeleteSyncTargetFor(userID int64, targetID string) error {
	return s.withTx(func(tx *sql.Tx) error {
		var running int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE user_id=? AND target_id=? AND status='running'`, userID, targetID).Scan(&running); err != nil {
			return err
		}
		if running > 0 {
			return errors.New("wait for the running sync before removing its desired set")
		}
		if _, err := tx.Exec(`UPDATE sync_jobs SET status='cancelled',message='Desired set removed.',updated_at=? WHERE user_id=? AND target_id=? AND status IN ('queued','awaiting-approval')`, now(), userID, targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM sync_options WHERE target_id IN (SELECT target_id FROM sync_targets WHERE user_id=? AND target_id=?)`, userID, targetID); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM sync_targets WHERE user_id=? AND target_id=?`, userID, targetID)
		return err
	})
}

func (s *Store) ForgetEnvironmentFor(userID int64, id string) error {
	_, err := s.db.Exec(`DELETE FROM environments WHERE user_id=? AND id=?`, userID, id)
	return err
}

// Machine groups agent/project inventories under their user-scoped host alias.
// Existing environment IDs remain valid subgroup IDs for the scanner and sync CLI.
type Machine struct {
	Aliases    []string          `json:"aliases,omitempty"`
	Host       string            `json:"host"`
	Groups     []SyncEnvironment `json:"groups"`
	SkillCount int               `json:"skillCount"`
}

func GroupMachines(envs []SyncEnvironment) []Machine {
	out := []Machine{}
	index := map[string]int{}
	seen := map[string]map[string]bool{}
	for _, env := range envs {
		i, ok := index[env.Host]
		if !ok {
			i = len(out)
			index[env.Host] = i
			out = append(out, Machine{Host: env.Host, Groups: []SyncEnvironment{}})
			seen[env.Host] = map[string]bool{}
		}
		out[i].Groups = append(out[i].Groups, env)
		for _, skill := range env.Skills {
			seen[env.Host][skill.ID] = true
		}
		out[i].SkillCount = len(seen[env.Host])
	}
	return out
}

func (s *Store) RecordScanErrorFor(userID int64, id, message string) error {
	_, err := s.db.Exec(`INSERT INTO scan_errors(env_id,error) SELECT id,? FROM environments WHERE id=? AND user_id=? ON CONFLICT(env_id) DO UPDATE SET error=excluded.error`, message, id, userID)
	return err
}
