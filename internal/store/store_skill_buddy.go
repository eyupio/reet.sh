package store

import (
	"database/sql"
	"errors"

	"github.com/eyupio/reet.sh/internal/skillbuddy"
)

func (s *Store) CompareSetting(key, expected, next string) error {
	return s.withTx(func(tx *sql.Tx) error {
		var current string
		err := tx.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if current != expected {
			return errors.New("setting changed")
		}
		_, err = tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, next)
		return err
	})
}

type SkillBuddyRecord struct {
	Scope     string           `json:"scope"`
	SessionID string           `json:"sessionId"`
	Stage     string           `json:"stage"`
	Skill     skillbuddy.Skill `json:"skill"`
	At        int64            `json:"at"`
}

func (s *Store) migrateSkillBuddy() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS skill_buddy_choices(
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	scope TEXT NOT NULL,skill_id TEXT NOT NULL,name TEXT NOT NULL,source TEXT NOT NULL,
	revision TEXT NOT NULL,reason TEXT NOT NULL,stage TEXT NOT NULL,session_id TEXT NOT NULL DEFAULT '',
	instructions TEXT NOT NULL DEFAULT '',state TEXT NOT NULL DEFAULT 'suggested',at INTEGER NOT NULL,
	PRIMARY KEY(user_id,scope,skill_id));
	CREATE INDEX IF NOT EXISTS skill_buddy_recent ON skill_buddy_choices(user_id,at)`)
	return err
}

// RecordSkillBuddyFor keeps one receipt per skill and work scope, not one per page refresh.
// Suggestions never replace a person's decision or its pinned instructions.
func (s *Store) RecordSkillBuddyFor(uid int64, scope, session, stage string, sk skillbuddy.Skill) error {
	if len(scope) > 200 || scope == "" || len(sk.ID) > 300 || sk.ID == "" || len(sk.Instructions) > 12000 {
		return errors.New("invalid skill receipt")
	}
	switch sk.State {
	case "suggested", "selected", "drafted", "dismissed":
	default:
		return errors.New("invalid skill receipt state")
	}
	if sk.State == "suggested" || sk.State == "dismissed" {
		sk.Instructions = ""
	}
	_, err := s.db.Exec(`INSERT INTO skill_buddy_choices(user_id,scope,skill_id,name,source,revision,reason,stage,session_id,instructions,state,at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(user_id,scope,skill_id) DO UPDATE SET
	name=CASE WHEN excluded.state='suggested' THEN name ELSE excluded.name END,
	revision=CASE WHEN excluded.state='suggested' THEN revision ELSE excluded.revision END,
	instructions=CASE WHEN excluded.state='suggested' THEN instructions ELSE excluded.instructions END,
	state=CASE WHEN excluded.state='suggested' THEN state ELSE excluded.state END,
	at=CASE WHEN excluded.state='suggested' THEN at ELSE excluded.at END`, uid, scope, sk.ID, sk.Name, sk.Source, sk.Revision, sk.Reason, stage, session, sk.Instructions, sk.State, now())
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM skill_buddy_choices WHERE user_id=? AND at<?`, uid, now()-90*24*60*60*1000)
	return err
}

func (s *Store) SkillBuddyRecordsFor(uid int64, scope string) ([]SkillBuddyRecord, error) {
	query := `SELECT scope,session_id,stage,skill_id,name,source,revision,reason,instructions,state,at FROM skill_buddy_choices WHERE user_id=?`
	args := []any{uid}
	if scope != "" {
		query += ` AND scope=?`
		args = append(args, scope)
	}
	query += ` ORDER BY at DESC,skill_id LIMIT 500`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillBuddyRecord{}
	for rows.Next() {
		var r SkillBuddyRecord
		if err = rows.Scan(&r.Scope, &r.SessionID, &r.Stage, &r.Skill.ID, &r.Skill.Name, &r.Skill.Source, &r.Skill.Revision, &r.Skill.Reason, &r.Skill.Instructions, &r.Skill.State, &r.At); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SkillBuddyPreparedFor links selected stage guidance to the session whose opening
// prompt contained it. Prepared does not mean the agent followed the instructions.
func (s *Store) SkillBuddyPreparedFor(uid int64, scope, session string) error {
	_, err := s.db.Exec(`UPDATE skill_buddy_choices SET session_id=?,state='prepared',at=? WHERE user_id=? AND scope=? AND state='selected'`, session, now(), uid, scope)
	return err
}
