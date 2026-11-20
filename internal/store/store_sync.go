package store

import (
	"database/sql"
	"errors"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/ref"
)

type MachineInfo struct {
	Host    string `json:"host"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Version string `json:"version"`
	Server  bool   `json:"server"`
}

type SyncEnvironment struct {
	Content *contentobs.Inventory `json:"content,omitempty"`
	Error   string                `json:"error,omitempty"`
	Environment
	ScannedAt int64       `json:"scannedAt"`
	Complete  bool        `json:"complete"`
	OS        string      `json:"os"`
	Arch      string      `json:"arch"`
	Version   string      `json:"version"`
	Skills    []SyncSkill `json:"skills"`
}

type SyncSkill struct {
	ItemID     string `json:"itemId,omitempty"`
	URL        string `json:"url,omitempty"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Source     string `json:"source"`
	SourceType string `json:"sourceType"`
	// Problem says why an owned draft cannot be synced right now (for example
	// private storage is full). Such a candidate is listed but not installable.
	Problem string `json:"problem,omitempty"`
}

// Each inventory receives a fresh opaque generation, including after a scope
// is forgotten/recreated. Timestamps or per-row counters can revive old hashes.
func (s *Store) RecordInventoryFor(userID int64, envID string, complete bool, machine MachineInfo) error {
	_, err := s.db.Exec(`INSERT INTO environment_scans(env_id,scanned_at,complete,os,arch,version,inventory_revision)
 SELECT id,?,?,?,?,?,(random() & 9223372036854775807) FROM environments WHERE id=? AND user_id=?
 ON CONFLICT(env_id) DO UPDATE SET scanned_at=excluded.scanned_at,complete=excluded.complete,os=excluded.os,arch=excluded.arch,version=excluded.version,inventory_revision=excluded.inventory_revision`, now(), complete, machine.OS, machine.Arch, machine.Version, envID, userID)
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
		item.Content, err = s.ContentObservationFor(userID, env.ID)
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

// SkillInstalledAsFor finds the skill on a person's shelf, from this source,
// that the skills CLI installs into folder when the catalogue knows it by
// another name. The CLI names the folder after the skill's own name
// ("UI/UX Design Review" becomes ui-ux-design-review) while the catalogue has
// its own slug (uiux-design-review), so a scan reports a folder the shelf does
// not know. It reports false when no skill, or more than one, fits.
func (s *Store) SkillInstalledAsFor(userID int64, source, folder string) (ref.Ref, bool, error) {
	rows, err := s.db.Query(`SELECT k.id,k.source,k.slug,k.source_type,k.url,k.name FROM skills k JOIN shelf sh ON sh.skill_id=k.id WHERE sh.user_id=? AND k.source=? AND k.slug<>? AND k.name<>'' AND k.name<>k.slug`, userID, source, folder)
	if err != nil {
		return ref.Ref{}, false, err
	}
	defer rows.Close()
	var found []ref.Ref
	for rows.Next() {
		var r ref.Ref
		var name string
		if err := rows.Scan(&r.ID, &r.Source, &r.Slug, &r.SourceType, &r.URL, &name); err != nil {
			return ref.Ref{}, false, err
		}
		if ref.InstalledFolder(name) == folder {
			found = append(found, r)
		}
	}
	if err := rows.Err(); err != nil || len(found) != 1 {
		return ref.Ref{}, false, err
	}
	return found[0], true, nil
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
