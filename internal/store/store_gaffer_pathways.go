package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// A Factory pathway is a person's saved, ordered line of stages. Rules about
// what a valid line is belong to the caller; this file only keeps them.
const MaxGafferPathways = 50

var (
	ErrGafferPathwayChanged = errors.New("That pathway changed. Reload it before trying again.")
	ErrGafferPathwayLimit   = errors.New("This account already has 50 pathways. Delete one before saving another.")
	ErrGafferPathwayName    = errors.New("Another pathway already has that name.")
)

type GafferPathwayStage struct {
	Kind   string            `json:"kind"`           // a built-in kind or "custom"
	Name   string            `json:"name,omitempty"` // custom only
	Prompt string            `json:"prompt,omitempty"`
	Tier   string            `json:"tier,omitempty"`   // set, or Agent is
	Agent  *GafferStageAgent `json:"agent,omitempty"`  // existing type: tool, model, effort, provider
	Skills []string          `json:"skills,omitempty"` // skill ids
}

type GafferPathway struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Enabled     bool                 `json:"enabled"`
	Stages      []GafferPathwayStage `json:"stages"`
	CreatedAt   int64                `json:"createdAt"`
	UpdatedAt   int64                `json:"updatedAt"`
}

// GafferPathwayRef is the pathway an outcome's pipeline ran, and its name then.
type GafferPathwayRef struct{ ID, Name string }

type gafferPathwayData struct {
	Description string               `json:"description"`
	Stages      []GafferPathwayStage `json:"stages"`
}

func (s *Store) migrateGafferPathways() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS gaffer_pathways(
 id TEXT PRIMARY KEY,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 name TEXT NOT NULL COLLATE NOCASE,enabled INTEGER NOT NULL,
 created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,
 data TEXT NOT NULL CHECK(json_valid(data)),
 UNIQUE(user_id,name));`)
	return err
}

const pathwayColumns = `id,name,enabled,created_at,updated_at,data`

func scanPathway(row interface{ Scan(...any) error }) (GafferPathway, error) {
	var p GafferPathway
	var raw string
	if err := row.Scan(&p.ID, &p.Name, &p.Enabled, &p.CreatedAt, &p.UpdatedAt, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return p, ErrNotFound
		}
		return p, err
	}
	var d gafferPathwayData
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return p, err
	}
	p.Description, p.Stages = d.Description, d.Stages
	return p, nil
}

// GafferPathwaysFor lists the account's pathways by name, without case.
func (s *Store) GafferPathwaysFor(uid int64) ([]GafferPathway, error) {
	rows, err := s.db.Query(`SELECT `+pathwayColumns+` FROM gaffer_pathways WHERE user_id=? ORDER BY name,id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GafferPathway{}
	for rows.Next() {
		p, err := scanPathway(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GafferPathwayFor(uid int64, id string) (GafferPathway, error) {
	return scanPathway(s.db.QueryRow(`SELECT `+pathwayColumns+` FROM gaffer_pathways WHERE user_id=? AND id=?`, uid, id))
}

// SaveGafferPathwayFor saves p. A readAt of 0 makes a new pathway; otherwise it
// replaces the one whose UpdatedAt is readAt, and is refused if it has moved on.
func (s *Store) SaveGafferPathwayFor(uid int64, p GafferPathway, readAt int64) (GafferPathway, error) {
	raw, err := json.Marshal(gafferPathwayData{Description: p.Description, Stages: p.Stages})
	if err != nil {
		return p, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	t := now()
	if readAt == 0 {
		var count int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM gaffer_pathways WHERE user_id=?`, uid).Scan(&count); err != nil {
			return p, err
		}
		if count >= MaxGafferPathways {
			return p, ErrGafferPathwayLimit
		}
		p.CreatedAt, p.UpdatedAt = t, t
		_, err = tx.Exec(`INSERT INTO gaffer_pathways(id,user_id,name,enabled,created_at,updated_at,data) VALUES(?,?,?,?,?,?,?)`,
			p.ID, uid, p.Name, p.Enabled, t, t, string(raw))
	} else {
		var created, updated int64
		if err = tx.QueryRow(`SELECT created_at,updated_at FROM gaffer_pathways WHERE user_id=? AND id=?`, uid, p.ID).Scan(&created, &updated); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return p, ErrNotFound
			}
			return p, err
		}
		if updated != readAt {
			return p, ErrGafferPathwayChanged
		}
		p.CreatedAt, p.UpdatedAt = created, max(t, updated+1)
		_, err = tx.Exec(`UPDATE gaffer_pathways SET name=?,enabled=?,updated_at=?,data=? WHERE user_id=? AND id=?`,
			p.Name, p.Enabled, p.UpdatedAt, string(raw), uid, p.ID)
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: gaffer_pathways.user_id, gaffer_pathways.name") {
			return p, ErrGafferPathwayName
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return p, ErrGafferPathwayChanged // the id is taken
		}
		return p, err
	}
	return p, tx.Commit()
}

func (s *Store) DeleteGafferPathwayFor(uid int64, id string) error {
	res, err := s.db.Exec(`DELETE FROM gaffer_pathways WHERE user_id=? AND id=?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
