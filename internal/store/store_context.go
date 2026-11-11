package store

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A grant belongs to one conversation or MCP client and one immutable scope.
// Shelf read/write scopes never imply context access.
type ContextFlags struct {
	Status  bool `json:"status"`
	History bool `json:"history"`
	Text    bool `json:"text"`
}

func (f ContextFlags) Any() bool { return f.Status || f.History || f.Text }
func (f ContextFlags) Contains(other ContextFlags) bool {
	return (!other.Status || f.Status) && (!other.History || f.History) && (!other.Text || f.Text)
}

type ContextGrant struct {
	SessionID string `json:"sessionId"`
	ContextFlags
	Revision         int64  `json:"revision"`
	ProviderID       string `json:"providerId,omitempty"`
	ProviderRevision int64  `json:"providerRevision,omitempty"`
	Model            string `json:"model,omitempty"`
	UpdatedAt        int64  `json:"updatedAt"`
}

type ContextUse struct {
	Version int `json:"version"`
	ContextFlags
	GrantRevision int64  `json:"grantRevision"`
	ReadAt        int64  `json:"readAt"`
	FirstSeq      int64  `json:"firstSeq,omitempty"`
	LastSeq       int64  `json:"lastSeq,omitempty"`
	LastEventAt   int64  `json:"lastEventAt,omitempty"`
	Truncated     bool   `json:"truncated"`
	Digest        string `json:"digest"`
}

func (s *Store) migrateContext() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS context_grants (
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK(kind IN ('gaffer','mcp','browser')), subject TEXT NOT NULL,
 session_id TEXT NOT NULL, status INTEGER NOT NULL, history INTEGER NOT NULL, text INTEGER NOT NULL,
 revision INTEGER NOT NULL, provider_id TEXT NOT NULL DEFAULT '',provider_revision INTEGER NOT NULL DEFAULT 0,
 model TEXT NOT NULL DEFAULT '',updated_at INTEGER NOT NULL,
 PRIMARY KEY(user_id,kind,subject,session_id));
 CREATE TRIGGER IF NOT EXISTS context_conversation_removed AFTER DELETE ON gaffer_conversations BEGIN
 DELETE FROM context_grants WHERE user_id=OLD.user_id AND kind='gaffer' AND subject=OLD.id; END;
 CREATE TRIGGER IF NOT EXISTS context_session_archived AFTER INSERT ON terminal_session_archives BEGIN
 UPDATE context_grants SET status=0,history=0,text=0,revision=revision+1,updated_at=NEW.archived_at WHERE user_id=NEW.user_id AND session_id=NEW.session_id;
 END;`)
	if err != nil {
		return err
	}
	exists, err := hasColumn(s.db, "gaffer_turns", "context_json")
	if err != nil {
		return err
	}
	if !exists {
		_, err = s.db.Exec(`ALTER TABLE gaffer_turns ADD COLUMN context_json TEXT NOT NULL DEFAULT ''`)
	}
	return err
}

type contextQuerier interface{ QueryRow(string, ...any) *sql.Row }

func contextSubject(q contextQuerier, uid int64, kind, subject string, active bool) (string, error) {
	var allowed bool
	if err := q.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, uid).Scan(&allowed); err != nil {
		return "", err
	}
	if !allowed {
		return "", ErrNotFound
	}
	session := ""
	switch kind {
	case "gaffer":
		if !validGafferID(subject) {
			return "", ErrNotFound
		}
		if err := q.QueryRow(`SELECT session_id FROM gaffer_conversations WHERE id=? AND user_id=?`, subject, uid).Scan(&session); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", ErrNotFound
			}
			return "", err
		}
	case "mcp":
		id, err := strconv.ParseInt(subject, 10, 64)
		if err != nil || id < 1 || strconv.FormatInt(id, 10) != subject {
			return "", ErrNotFound
		}
		extra := ""
		if active {
			extra = ` AND c.enabled=1 AND c.expires_at>` + strconv.FormatInt(now(), 10) + ` AND EXISTS(SELECT 1 FROM mcp_access a WHERE a.user_id=c.user_id AND a.enabled=1)`
		}
		if err = q.QueryRow(`SELECT EXISTS(SELECT 1 FROM mcp_connections c WHERE c.user_id=? AND c.id=? AND c.revoked_at=0`+extra+`)`, uid, id).Scan(&allowed); err != nil {
			return "", err
		}
		if !allowed {
			return "", ErrNotFound
		}
	case "browser":
		if subject != "browser" {
			return "", ErrNotFound
		}
		if active {
			if err := q.QueryRow(`SELECT EXISTS(SELECT 1 FROM mcp_access WHERE user_id=? AND enabled=1 AND browser_enabled=1)`, uid).Scan(&allowed); err != nil {
				return "", err
			}
			if !allowed {
				return "", ErrNotFound
			}
		}
	default:
		return "", ErrNotFound
	}
	return session, nil
}

func contextSession(q contextQuerier, uid int64, session string) error {
	if session == "" {
		return nil
	}
	if !terminalproto.ValidID(session) {
		return ErrNotFound
	}
	var allowed bool
	err := q.QueryRow(`SELECT EXISTS(SELECT 1 FROM terminal_sessions t WHERE user_id=? AND id=? AND structured=1
 AND NOT EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.user_id=t.user_id AND a.session_id=t.id))`, uid, session).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CheckContextSubject(uid int64, kind, subject, session string, active bool) error {
	bound, err := contextSubject(s.db, uid, kind, subject, active)
	if err != nil {
		return err
	}
	if kind == "gaffer" && bound != session {
		return ErrNotFound
	}
	return contextSession(s.db, uid, session)
}

const contextColumns = `session_id,status,history,text,revision,provider_id,provider_revision,model,updated_at`

func scanContextGrant(row interface{ Scan(...any) error }, session string) (ContextGrant, error) {
	g := ContextGrant{SessionID: session}
	err := row.Scan(&g.SessionID, &g.Status, &g.History, &g.Text, &g.Revision, &g.ProviderID, &g.ProviderRevision, &g.Model, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return g, err
}

func (s *Store) ContextGrantFor(uid int64, kind, subject, session string) (ContextGrant, error) {
	return scanContextGrant(s.db.QueryRow(`SELECT `+contextColumns+` FROM context_grants WHERE user_id=? AND kind=? AND subject=? AND session_id=?`, uid, kind, subject, session), session)
}

func (s *Store) SetContextGrantFor(uid int64, kind, subject string, expected int64, grant ContextGrant) (ContextGrant, error) {
	if expected < 0 || (grant.Text && !grant.History) || (grant.SessionID == "" && (grant.History || grant.Text)) {
		return grant, ErrGafferConflict
	}
	err := s.withTx(func(tx *sql.Tx) error {
		bound, err := contextSubject(tx, uid, kind, subject, false)
		if err != nil {
			return err
		}
		if kind == "gaffer" && bound != grant.SessionID {
			return ErrNotFound
		}
		if err = contextSession(tx, uid, grant.SessionID); err != nil {
			return err
		}
		old, err := scanContextGrant(tx.QueryRow(`SELECT `+contextColumns+` FROM context_grants WHERE user_id=? AND kind=? AND subject=? AND session_id=?`, uid, kind, subject, grant.SessionID), grant.SessionID)
		if err != nil {
			return err
		}
		if old.Revision != expected {
			return ErrGafferConflict
		}
		if !old.Any() && grant.Any() {
			var count int
			if err = tx.QueryRow(`SELECT COUNT(*) FROM context_grants WHERE user_id=? AND kind=? AND subject=? AND (status=1 OR history=1 OR text=1)`, uid, kind, subject).Scan(&count); err != nil {
				return err
			}
			if count >= 100 {
				return ErrGafferChatFull
			}
		}
		grant.Revision = old.Revision + 1
		grant.UpdatedAt = now()
		_, err = tx.Exec(`INSERT INTO context_grants(user_id,kind,subject,session_id,status,history,text,revision,provider_id,provider_revision,model,updated_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id,kind,subject,session_id) DO UPDATE SET status=excluded.status,history=excluded.history,text=excluded.text,
 revision=excluded.revision,provider_id=excluded.provider_id,provider_revision=excluded.provider_revision,model=excluded.model,updated_at=excluded.updated_at`, uid, kind, subject, grant.SessionID, grant.Status, grant.History, grant.Text, grant.Revision, grant.ProviderID, grant.ProviderRevision, grant.Model, grant.UpdatedAt)
		if err != nil {
			return err
		}
		if kind == "gaffer" {
			_, err = tx.Exec(`UPDATE gaffer_conversations SET revision=revision+1,updated_at=? WHERE user_id=? AND id=?`, grant.UpdatedAt, uid, subject)
		}
		return err
	})
	return grant, err
}

func (s *Store) ContextGrantsFor(uid int64, kind, subject string) ([]ContextGrant, error) {
	rows, err := s.db.Query(`SELECT `+contextColumns+` FROM context_grants g WHERE user_id=? AND kind=? AND subject=?
 AND (status=1 OR history=1 OR text=1) AND (session_id='' OR EXISTS(SELECT 1 FROM terminal_sessions t WHERE t.id=g.session_id AND t.user_id=g.user_id AND t.structured=1
 AND NOT EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.session_id=t.id AND a.user_id=t.user_id))) ORDER BY session_id LIMIT 100`, uid, kind, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContextGrant{}
	for rows.Next() {
		g, err := scanContextGrant(rows, "")
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

type ContextFleet struct {
	Machines       int            `json:"machines"`
	Sessions       map[string]int `json:"sessions"`
	LastReportedAt int64          `json:"lastReportedAt"`
}

type ContextSessionChoice struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (s *Store) ContextSessionChoicesFor(uid int64) ([]ContextSessionChoice, error) {
	rows, err := s.db.Query(`SELECT id,substr(title,1,160) FROM terminal_sessions t WHERE user_id=? AND structured=1
 AND NOT EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.session_id=t.id AND a.user_id=t.user_id) ORDER BY updated_at DESC,id LIMIT 100`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContextSessionChoice{}
	for rows.Next() {
		var c ContextSessionChoice
		if err := rows.Scan(&c.ID, &c.Title); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ContextFleetFor(uid int64) (ContextFleet, error) {
	out := ContextFleet{Sessions: map[string]int{}}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sync_agents WHERE user_id=?`, uid).Scan(&out.Machines); err != nil {
		return out, err
	}
	rows, err := s.db.Query(`SELECT state,COUNT(*),MAX(updated_at) FROM terminal_sessions t WHERE user_id=?
 AND NOT EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.session_id=t.id AND a.user_id=t.user_id) GROUP BY state`, uid)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var count int
		var at int64
		if err := rows.Scan(&state, &count, &at); err != nil {
			return out, err
		}
		out.Sessions[state] = count
		out.LastReportedAt = max(out.LastReportedAt, at)
	}
	return out, rows.Err()
}

// Project in SQL before loading: payloads, arguments, files, approval bodies and
// reasoning never leave storage through this reader. Text is separately gated.
func (s *Store) ContextEventsFor(uid int64, session string, text bool) ([]StoredChatEvent, int64, int64, int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer tx.Rollback()
	var first, last int64
	var count int
	if err = tx.QueryRow(`SELECT COALESCE(MIN(seq),0),COALESCE(MAX(seq),0),COUNT(*) FROM terminal_chat_events WHERE user_id=? AND session_id=?`, uid, session).Scan(&first, &last, &count); err != nil {
		return nil, 0, 0, 0, err
	}
	rows, err := tx.Query(`SELECT seq,at,CASE WHEN json_valid(event) THEN json_object(
 'kind',substr(json_extract(event,'$.kind'),1,32),'state',substr(json_extract(event,'$.state'),1,32),
 'status',substr(json_extract(event,'$.status'),1,32),'toolKind',substr(json_extract(event,'$.toolKind'),1,32),
 'text',CASE WHEN ? AND json_extract(event,'$.kind') IN ('user','agent') THEN substr(json_extract(event,'$.text'),1,4096) ELSE '' END) ELSE '{}' END
 FROM terminal_chat_events WHERE user_id=? AND session_id=? ORDER BY seq DESC LIMIT 40`, text, uid, session)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	out := []StoredChatEvent{}
	for rows.Next() {
		var e StoredChatEvent
		var raw string
		if err = rows.Scan(&e.Seq, &e.At, &raw); err != nil {
			rows.Close()
			return nil, 0, 0, 0, err
		}
		e.Event = []byte(raw)
		out = append(out, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, 0, 0, err
	}
	return out, first, last, count, nil
}

// ContextLatestReplyFor returns the working model's most recent reply: the
// text of every agent event after the last message it was sent, oldest first,
// with the sequence numbers it spans. Each event's text is capped, and only
// the latest 60 events are read. It reads retained events only.
func (s *Store) ContextLatestReplyFor(uid int64, session string) ([]string, int64, int64, int64, error) {
	rows, err := s.db.Query(`SELECT seq,at,substr(COALESCE(json_extract(event,'$.text'),''),1,16000) FROM terminal_chat_events
 WHERE user_id=? AND session_id=? AND json_valid(event) AND json_extract(event,'$.kind')='agent'
 AND seq>COALESCE((SELECT MAX(seq) FROM terminal_chat_events WHERE user_id=? AND session_id=? AND json_valid(event) AND json_extract(event,'$.kind')='user'),0)
 ORDER BY seq DESC LIMIT 60`, uid, session, uid, session)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()
	var parts []string
	var first, last, at int64
	for rows.Next() {
		var seq, when int64
		var text string
		if err = rows.Scan(&seq, &when, &text); err != nil {
			return nil, 0, 0, 0, err
		}
		if last == 0 {
			last, at = seq, when
		}
		first = seq
		parts = append(parts, text)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return parts, first, last, at, rows.Err()
}
