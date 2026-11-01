package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// SessionDraft is an account-owned launch plan. Its ID is also the terminal
// create request ID, so a lost response or concurrent Start cannot launch twice.
type SessionDraft struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Prompt    string `json:"prompt"`
	AgentID   string `json:"agentId"`
	ProjectID string `json:"projectId"`
	Dir       string `json:"dir"`
	Tool      string `json:"tool"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	// Idea marks a placeholder that is only being remembered: it may have no
	// prompt and can never be started until the flag is cleared.
	Idea bool `json:"idea,omitempty"`
	// Launch options carried to Start. Yolo is only an intent: the account
	// setting, machine and agent gates and the audit run when it starts.
	Yolo           bool         `json:"yolo,omitempty"`
	Worktree       bool         `json:"worktree,omitempty"`
	Base           string       `json:"base,omitempty"`
	Chat           bool         `json:"chat,omitempty"`
	Trust          bool         `json:"trust,omitempty"`
	Images         []DraftImage `json:"images,omitempty"`
	ImageCount     int          `json:"imageCount"`
	PreparedPrompt string       `json:"preparedPrompt,omitempty"`
	// PreparedProject is the worktree project slug frozen with the prompt, so a
	// retried start sends the same create parameters even if the project is
	// renamed or stops being a repository. Empty means no worktree.
	PreparedProject string `json:"preparedProject,omitempty"`
	Starting        bool   `json:"starting"`
	SessionID       string `json:"sessionId,omitempty"`
	Version         int64  `json:"version"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
}

type DraftImage struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

var ErrDraftConflict = errors.New("this backlog item changed or has already been started; refresh before trying again")
var ErrDraftLimit = errors.New("the backlog is full; keep at most 100 items and 32 MB of saved content")

func (s *Store) migrateSessionBacklog() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS session_backlog (
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		id TEXT NOT NULL, body TEXT NOT NULL, version INTEGER NOT NULL,
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
		PRIMARY KEY(user_id,id))`)
	return err
}

func scanDraft(r rowScanner) (SessionDraft, error) {
	var d SessionDraft
	var raw string
	var version, created, updated int64
	if err := r.Scan(&raw, &version, &created, &updated); err != nil {
		return d, err
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return d, err
	}
	d.Version, d.CreatedAt, d.UpdatedAt = version, created, updated
	d.ImageCount = len(d.Images)
	return d, nil
}

func (s *Store) SessionDraftFor(uid int64, id string) (SessionDraft, error) {
	return scanDraft(s.db.QueryRow(`SELECT body,version,created_at,updated_at FROM session_backlog WHERE user_id=? AND id=?`, uid, id))
}

func (s *Store) SessionDraftsFor(uid int64) ([]SessionDraft, error) {
	rows, err := s.db.Query(`SELECT b.body,b.version,b.created_at,b.updated_at,
		COALESCE((SELECT o.session_id FROM terminal_operations o WHERE o.user_id=b.user_id AND o.kind='create' AND o.request_id=b.id AND o.agent_id=json_extract(b.body,'$.agentId')),json_extract(b.body,'$.sessionId'),'')
		FROM session_backlog b WHERE b.user_id=? ORDER BY b.created_at,b.id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []SessionDraft{}
	for rows.Next() {
		var raw, sessionID string
		var version, created, updated int64
		if err := rows.Scan(&raw, &version, &created, &updated, &sessionID); err != nil {
			return nil, err
		}
		var d SessionDraft
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			return nil, err
		}
		d.Version, d.CreatedAt, d.UpdatedAt, d.SessionID = version, created, updated, sessionID
		d.ImageCount = len(d.Images)
		d.Images = nil // Large image bodies only leave the store on an explicit detail read.
		d.Starting = d.PreparedPrompt != ""
		d.PreparedPrompt, d.PreparedProject = "", ""
		list = append(list, d)
	}
	return list, rows.Err()
}

// SaveSessionDraftFor uses mandatory optimistic concurrency. Prepared requests
// are immutable: every retry must carry exactly the same launch parameters.
func (s *Store) SaveSessionDraftFor(uid int64, d SessionDraft, create bool) (SessionDraft, error) {
	err := s.withTx(func(tx *sql.Tx) error {
		// Launched plans are retained briefly for identical retries; terminal
		// history remains the durable record. They do not consume backlog slots.
		if _, err := tx.Exec(`DELETE FROM session_backlog WHERE user_id=? AND updated_at<? AND (COALESCE(json_extract(body,'$.sessionId'),'')<>'' OR EXISTS (SELECT 1 FROM terminal_operations o WHERE o.user_id=session_backlog.user_id AND o.request_id=session_backlog.id AND o.kind='create'))`, uid, now()-30*24*60*60*1000); err != nil {
			return err
		}
		if create {
			var err error
			d.ID, err = projectRandomID()
			if err != nil {
				return err
			}
			d.Version, d.CreatedAt, d.UpdatedAt = 1, now(), now()
		} else {
			old, err := scanDraft(tx.QueryRow(`SELECT body,version,created_at,updated_at FROM session_backlog WHERE user_id=? AND id=?`, uid, d.ID))
			if err != nil {
				return err
			}
			if old.Version != d.Version || old.PreparedPrompt != "" {
				return ErrDraftConflict
			}
			d.Version, d.CreatedAt, d.UpdatedAt = old.Version+1, old.CreatedAt, now()
		}
		d.SessionID, d.PreparedPrompt, d.PreparedProject = "", "", ""
		d.ImageCount = len(d.Images)
		raw, err := json.Marshal(d)
		if err != nil {
			return err
		}
		var count, size int
		if err := tx.QueryRow(`SELECT COUNT(*),COALESCE(SUM(length(CAST(body AS BLOB))),0) FROM session_backlog b WHERE user_id=? AND id<>? AND COALESCE(json_extract(b.body,'$.sessionId'),'')='' AND NOT EXISTS (SELECT 1 FROM terminal_operations o WHERE o.user_id=b.user_id AND o.request_id=b.id AND o.kind='create')`, uid, d.ID).Scan(&count, &size); err != nil {
			return err
		}
		if count >= 100 || size+len(raw) > 32<<20 {
			return ErrDraftLimit
		}
		_, err = tx.Exec(`INSERT INTO session_backlog(user_id,id,body,version,created_at,updated_at) VALUES(?,?,?,?,?,?)
			ON CONFLICT(user_id,id) DO UPDATE SET body=excluded.body,version=excluded.version,updated_at=excluded.updated_at`, uid, d.ID, string(raw), d.Version, d.CreatedAt, d.UpdatedAt)
		return err
	})
	return d, err
}

// worktreeProject is the resolved worktree slug ("" for none); like the prompt,
// the first prepare freezes it and every retry gets the frozen value back.
func (s *Store) PrepareSessionDraftFor(uid int64, id string, version int64, prompt, worktreeProject string) (SessionDraft, error) {
	var d SessionDraft
	err := s.withTx(func(tx *sql.Tx) error {
		var err error
		d, err = scanDraft(tx.QueryRow(`SELECT body,version,created_at,updated_at FROM session_backlog WHERE user_id=? AND id=?`, uid, id))
		if err != nil {
			return err
		}
		if d.PreparedPrompt != "" {
			return nil
		}
		if d.Version != version {
			return ErrDraftConflict
		}
		d.PreparedPrompt, d.PreparedProject, d.Version, d.UpdatedAt = prompt, worktreeProject, d.Version+1, now()
		raw, err := json.Marshal(d)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE session_backlog SET body=?,version=?,updated_at=? WHERE user_id=? AND id=?`, string(raw), d.Version, d.UpdatedAt, uid, id)
		return err
	})
	return d, err
}

func (s *Store) DeleteSessionDraftFor(uid int64, id string, version int64) error {
	result, err := s.db.Exec(`DELETE FROM session_backlog WHERE user_id=? AND id=? AND version=? AND COALESCE(json_extract(body,'$.preparedPrompt'),'')=''`, uid, id, version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrDraftConflict
	}
	return err
}

// ReleaseSessionDraftFor only thaws a definitively refused start with no broker
// operation. The API holds its start lock until this completes.
func (s *Store) ReleaseSessionDraftFor(uid int64, id string, version int64) error {
	_, err := s.db.Exec(`UPDATE session_backlog SET body=json_remove(json_remove(body,'$.preparedPrompt'),'$.preparedProject'),version=version+1,updated_at=?
		WHERE user_id=? AND id=? AND version=? AND NOT EXISTS (
		SELECT 1 FROM terminal_operations o WHERE o.user_id=session_backlog.user_id AND o.request_id=session_backlog.id AND o.kind='create')`, now(), uid, id, version)
	return err
}

func (s *Store) CompleteSessionDraftFor(uid int64, id string) error {
	_, err := s.db.Exec(`UPDATE session_backlog SET body=json_set(json_remove(body,'$.images'),'$.sessionId',
		(SELECT o.session_id FROM terminal_operations o WHERE o.user_id=session_backlog.user_id AND o.request_id=session_backlog.id AND o.kind='create'))
		WHERE user_id=? AND id=? AND EXISTS (SELECT 1 FROM terminal_operations o WHERE o.user_id=session_backlog.user_id AND o.request_id=session_backlog.id AND o.kind='create')`, uid, id)
	return err
}
