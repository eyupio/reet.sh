package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// A Factory template is a plan the person saved to start again: its stage
// layout, criteria and approved skill revisions, and nothing that ties it to a
// machine, a sign-in or a run. Each version is a row of its own and is never
// rewritten, so a later change cannot alter what an earlier run was started on.
const (
	MaxGafferTemplates        = 20
	MaxGafferTemplateVersions = 20
	MaxGafferTemplateBytes    = 64 << 10
)

var (
	ErrGafferTemplateChanged  = errors.New("That template changed. Reload it before trying again.")
	ErrGafferTemplateLimit    = errors.New("This account already has 20 templates. Delete one before saving another.")
	ErrGafferTemplateTooLarge = errors.New("That template is too large to save.")
	// ErrGafferTemplateRepeat comes with the version an earlier request under
	// the same request id made.
	ErrGafferTemplateRepeat = errors.New("That request was already answered.")
)

type GafferTemplateSkill struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	Revision string `json:"revision"`
}

type GafferTemplateCriterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type GafferTemplateStage struct {
	Kind    string                `json:"kind"`
	Part    string                `json:"part,omitempty"`
	Brief   string                `json:"brief,omitempty"`
	Tier    string                `json:"tier"`
	TierWhy string                `json:"tierWhy,omitempty"`
	Skills  []GafferTemplateSkill `json:"skills,omitempty"`
}

type GafferTemplatePipeline struct {
	Title     string                    `json:"title"`
	Prompt    string                    `json:"prompt"`
	AutoMerge bool                      `json:"autoMerge"`
	Stages    []GafferTemplateStage     `json:"stages"`
	Criteria  []GafferTemplateCriterion `json:"criteria,omitempty"`
}

type GafferTemplateContent struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Objective   string                   `json:"objective"`
	MaxReviews  int                      `json:"maxReviews"`
	Hours       int                      `json:"hours"`
	Pipelines   []GafferTemplatePipeline `json:"pipelines"`
}

type GafferTemplate struct {
	OwnerID   int64                 `json:"-"`
	ID        string                `json:"id"`
	Version   int                   `json:"version"`
	RequestID string                `json:"requestId"`
	OutcomeID string                `json:"outcomeId,omitempty"`
	CreatedAt int64                 `json:"createdAt"`
	Content   GafferTemplateContent `json:"content"`
	// FirstCreatedAt is when the oldest kept version was saved; set by the list only.
	FirstCreatedAt int64 `json:"-"`
}

// GafferTemplateRef is the template version an outcome was started from.
type GafferTemplateRef struct {
	ID      string
	Version int
}

func (s *Store) migrateGafferTemplates() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS gaffer_templates(
 id TEXT NOT NULL,version INTEGER NOT NULL,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 request_id TEXT NOT NULL,outcome_id TEXT NOT NULL,created_at INTEGER NOT NULL,
 data TEXT NOT NULL CHECK(json_valid(data)),
 PRIMARY KEY(id,version),UNIQUE(user_id,request_id));
 CREATE INDEX IF NOT EXISTS gaffer_templates_owner ON gaffer_templates(user_id,id);`)
	return err
}

const templateColumns = `id,version,request_id,outcome_id,created_at,data`

func scanTemplate(uid int64, row interface{ Scan(...any) error }) (GafferTemplate, error) {
	t := GafferTemplate{OwnerID: uid}
	var raw string
	if err := row.Scan(&t.ID, &t.Version, &t.RequestID, &t.OutcomeID, &t.CreatedAt, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, ErrNotFound
		}
		return t, err
	}
	return t, json.Unmarshal([]byte(raw), &t.Content)
}

// SaveGafferTemplateFor saves t as the version after base, base 0 making a new
// template. It is refused unless base is the template's latest version. A
// request id already used by the account answers the version it made, with
// ErrGafferTemplateRepeat, and saves nothing.
func (s *Store) SaveGafferTemplateFor(uid int64, t GafferTemplate, base int) (GafferTemplate, error) {
	raw, err := json.Marshal(t.Content)
	if err != nil {
		return t, err
	}
	if len(raw) > MaxGafferTemplateBytes {
		return t, ErrGafferTemplateTooLarge
	}
	tx, err := s.db.Begin()
	if err != nil {
		return t, err
	}
	defer tx.Rollback()
	if seen, err := scanTemplate(uid, tx.QueryRow(`SELECT `+templateColumns+` FROM gaffer_templates WHERE user_id=? AND request_id=?`, uid, t.RequestID)); err == nil {
		return seen, ErrGafferTemplateRepeat
	} else if !errors.Is(err, ErrNotFound) {
		return t, err
	}
	var owner sql.NullInt64
	var latest int
	if err = tx.QueryRow(`SELECT MAX(user_id),COALESCE(MAX(version),0) FROM gaffer_templates WHERE id=?`, t.ID).Scan(&owner, &latest); err != nil {
		return t, err
	}
	if owner.Valid && owner.Int64 != uid || !owner.Valid && base > 0 {
		return t, ErrNotFound
	}
	if latest != base {
		return t, ErrGafferTemplateChanged
	}
	if base == 0 {
		var count int
		if err = tx.QueryRow(`SELECT COUNT(DISTINCT id) FROM gaffer_templates WHERE user_id=?`, uid).Scan(&count); err != nil {
			return t, err
		}
		if count >= MaxGafferTemplates {
			return t, ErrGafferTemplateLimit
		}
	}
	t.OwnerID, t.Version, t.CreatedAt = uid, base+1, now()
	if _, err = tx.Exec(`INSERT INTO gaffer_templates(id,version,user_id,request_id,outcome_id,created_at,data) VALUES(?,?,?,?,?,?,?)`, t.ID, t.Version, uid, t.RequestID, t.OutcomeID, t.CreatedAt, string(raw)); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			// Another save took this version first.
			return t, ErrGafferTemplateChanged
		}
		return t, err
	}
	if _, err = tx.Exec(`DELETE FROM gaffer_templates WHERE user_id=? AND id=? AND version<=?`, uid, t.ID, t.Version-MaxGafferTemplateVersions); err != nil {
		return t, err
	}
	return t, tx.Commit()
}

// GafferTemplateFor reads one version of the account's template, version 0
// being the latest.
func (s *Store) GafferTemplateFor(uid int64, id string, version int) (GafferTemplate, error) {
	if version == 0 {
		return scanTemplate(uid, s.db.QueryRow(`SELECT `+templateColumns+` FROM gaffer_templates WHERE user_id=? AND id=? ORDER BY version DESC LIMIT 1`, uid, id))
	}
	return scanTemplate(uid, s.db.QueryRow(`SELECT `+templateColumns+` FROM gaffer_templates WHERE user_id=? AND id=? AND version=?`, uid, id, version))
}

// GafferTemplateVersionsFor lists the kept versions of a template, oldest first.
func (s *Store) GafferTemplateVersionsFor(uid int64, id string) ([]int, error) {
	rows, err := s.db.Query(`SELECT version FROM gaffer_templates WHERE user_id=? AND id=? ORDER BY version`, uid, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err = rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err = rows.Err(); err == nil && len(out) == 0 {
		err = ErrNotFound
	}
	return out, err
}

// GafferTemplatesFor lists the latest version of each of the account's
// templates, most recently changed first.
func (s *Store) GafferTemplatesFor(uid int64) ([]GafferTemplate, error) {
	rows, err := s.db.Query(`SELECT t.id,t.version,t.request_id,t.outcome_id,t.created_at,t.data,
 (SELECT MIN(created_at) FROM gaffer_templates f WHERE f.user_id=t.user_id AND f.id=t.id)
 FROM gaffer_templates t WHERE t.user_id=? AND t.version=(SELECT MAX(version) FROM gaffer_templates l WHERE l.user_id=t.user_id AND l.id=t.id)
 ORDER BY t.created_at DESC,t.id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GafferTemplate{}
	for rows.Next() {
		t := GafferTemplate{OwnerID: uid}
		var raw string
		if err = rows.Scan(&t.ID, &t.Version, &t.RequestID, &t.OutcomeID, &t.CreatedAt, &raw, &t.FirstCreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &t.Content); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteGafferTemplateFor removes a template and every version of it when
// version is still its latest. Outcomes started from it keep what they copied.
func (s *Store) DeleteGafferTemplateFor(uid int64, id string, version int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var latest int
	if err = tx.QueryRow(`SELECT COALESCE(MAX(version),0) FROM gaffer_templates WHERE user_id=? AND id=?`, uid, id).Scan(&latest); err != nil {
		return err
	}
	if latest == 0 {
		return ErrNotFound
	}
	if latest != version {
		return ErrGafferTemplateChanged
	}
	if _, err = tx.Exec(`DELETE FROM gaffer_templates WHERE user_id=? AND id=?`, uid, id); err != nil {
		return err
	}
	return tx.Commit()
}
