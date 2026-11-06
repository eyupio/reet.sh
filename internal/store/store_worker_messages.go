package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var ErrWorkerGrantFull = errors.New("this client already has 100 messaging scopes; stop an existing grant first")

func ValidWorkerText(text string) bool {
	return len(text) > 0 && len(text) <= 8000 && strings.TrimSpace(text) != "" && utf8.ValidString(text) && strings.IndexFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) < 0
}

var ErrWorkerMessageFull = errors.New("worker message receipts are full; remove old Gaffer conversations or archive finished sessions")

// WorkerGrant is independent of context/shelf permissions. A Gaffer grant also
// pins the destination provider/model so a settings edit cannot redirect consent.
type WorkerGrant struct {
	SessionID        string `json:"sessionId"`
	Enabled          bool   `json:"enabled"`
	Revision         int64  `json:"revision"`
	ProviderID       string `json:"providerId,omitempty"`
	ProviderRevision int64  `json:"providerRevision,omitempty"`
	Model            string `json:"model,omitempty"`
}
type WorkerMessage struct {
	ID            string `json:"id"`
	SessionID     string `json:"sessionId"`
	QueueID       string `json:"queueId"`
	TurnID        string `json:"turnId,omitempty"`
	Text          string `json:"text,omitempty"`
	State         string `json:"state"`
	Note          string `json:"note,omitempty"`
	Revision      int64  `json:"revision"`
	GrantRevision int64  `json:"grantRevision"`
	CreatedAt     int64  `json:"createdAt"`
}

func (s *Store) migrateWorkerMessages() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS worker_message_grants(
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,kind TEXT NOT NULL,subject TEXT NOT NULL,session_id TEXT NOT NULL,
 enabled INTEGER NOT NULL,revision INTEGER NOT NULL,provider_id TEXT NOT NULL,provider_revision INTEGER NOT NULL,model TEXT NOT NULL,
 PRIMARY KEY(user_id,kind,subject,session_id));
 CREATE TABLE IF NOT EXISTS worker_message_receipts(
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,kind TEXT NOT NULL,subject TEXT NOT NULL,session_id TEXT NOT NULL,id TEXT NOT NULL,
 queue_id TEXT NOT NULL UNIQUE,turn_id TEXT NOT NULL,text TEXT NOT NULL,text_hash TEXT NOT NULL,grant_revision INTEGER NOT NULL,
 state TEXT NOT NULL,note TEXT NOT NULL,created_at INTEGER NOT NULL,PRIMARY KEY(user_id,kind,subject,session_id,id));
 CREATE TRIGGER IF NOT EXISTS worker_conversation_removed AFTER DELETE ON gaffer_conversations BEGIN
 UPDATE terminal_chat_queue SET state='removed',text='',images='[]',revision=revision+1,note='Gaffer conversation removed.' WHERE state='queued' AND origin='companion' AND id IN(SELECT queue_id FROM worker_message_receipts WHERE user_id=OLD.user_id AND kind='gaffer' AND subject=OLD.id);
 DELETE FROM worker_message_grants WHERE user_id=OLD.user_id AND kind='gaffer' AND subject=OLD.id;
 DELETE FROM worker_message_receipts WHERE user_id=OLD.user_id AND kind='gaffer' AND subject=OLD.id; END;
 CREATE TRIGGER IF NOT EXISTS worker_session_archived AFTER INSERT ON terminal_session_archives BEGIN
 UPDATE worker_message_grants SET enabled=0,revision=revision+1 WHERE user_id=NEW.user_id AND session_id=NEW.session_id; END;`)
	if err != nil {
		return err
	}
	return nil
}
func scanWorkerGrant(row interface{ Scan(...any) error }, session string) (WorkerGrant, error) {
	g := WorkerGrant{SessionID: session}
	err := row.Scan(&g.Enabled, &g.Revision, &g.ProviderID, &g.ProviderRevision, &g.Model)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return g, err
}
func workerGrant(q contextQuerier, uid int64, kind, subject, session string) (WorkerGrant, error) {
	return scanWorkerGrant(q.QueryRow(`SELECT enabled,revision,provider_id,provider_revision,model FROM worker_message_grants WHERE user_id=? AND kind=? AND subject=? AND session_id=?`, uid, kind, subject, session), session)
}
func workerSubject(q contextQuerier, uid int64, kind, subject, session string, active bool) error {
	bound, err := contextSubject(q, uid, kind, subject, active)
	if err != nil {
		return err
	}
	if session == "" || kind == "gaffer" && bound != session {
		return ErrNotFound
	}
	if err = contextSession(q, uid, session); err != nil {
		return err
	}
	if active && kind != "gaffer" {
		var scope string
		if kind == "mcp" {
			err = q.QueryRow(`SELECT scope FROM mcp_connections WHERE user_id=? AND id=?`, uid, subject).Scan(&scope)
		} else {
			err = q.QueryRow(`SELECT browser_scope FROM mcp_access WHERE user_id=?`, uid).Scan(&scope)
		}
		if err != nil {
			return err
		}
		if scope != "write" {
			return ErrNotFound
		}
	}
	return nil
}
func workerAllowed(q contextQuerier, uid int64, kind, subject, session string, revision int64) error {
	if err := workerSubject(q, uid, kind, subject, session, true); err != nil {
		return err
	}
	g, err := workerGrant(q, uid, kind, subject, session)
	if err != nil {
		return err
	}
	if !g.Enabled || g.Revision != revision {
		return ErrNotFound
	}
	if kind == "gaffer" {
		var id, model string
		var rev int64
		var enabled, allowed bool
		err = q.QueryRow(`SELECT p.id,p.revision,p.enabled,p.allow_sessions,CASE WHEN c.model='' THEN p.model ELSE c.model END FROM gaffer_conversations c JOIN gaffer_providers p ON p.id=c.provider_id AND p.user_id=c.user_id WHERE c.user_id=? AND c.id=? AND c.session_id=?`, uid, subject, session).Scan(&id, &rev, &enabled, &allowed, &model)
		if err != nil {
			return err
		}
		if !enabled || !allowed || id != g.ProviderID || rev != g.ProviderRevision || model != g.Model {
			return ErrNotFound
		}
	}
	return nil
}
func (s *Store) CheckWorkerSubject(uid int64, kind, subject, session string, active bool) error {
	return workerSubject(s.db, uid, kind, subject, session, active)
}
func (s *Store) WorkerGrantFor(uid int64, kind, subject, session string) (WorkerGrant, error) {
	if err := workerSubject(s.db, uid, kind, subject, session, false); err != nil {
		return WorkerGrant{}, err
	}
	return workerGrant(s.db, uid, kind, subject, session)
}
func (s *Store) CheckWorkerGrant(uid int64, kind, subject, session string, revision int64) error {
	return workerAllowed(s.db, uid, kind, subject, session, revision)
}
func (s *Store) SetWorkerGrantFor(uid int64, kind, subject string, expected int64, g WorkerGrant) (WorkerGrant, error) {
	err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, uid); err != nil {
			return err
		}
		if err := workerSubject(tx, uid, kind, subject, g.SessionID, false); err != nil {
			return err
		}
		old, err := workerGrant(tx, uid, kind, subject, g.SessionID)
		if err != nil {
			return err
		}
		if expected < 0 || old.Revision != expected {
			return ErrGafferConflict
		}
		if g.Enabled && !old.Enabled {
			var count int
			if err = tx.QueryRow(`SELECT COUNT(*) FROM worker_message_grants WHERE user_id=? AND kind=? AND subject=? AND enabled=1`, uid, kind, subject).Scan(&count); err != nil {
				return err
			}
			if count >= 100 {
				return ErrWorkerGrantFull
			}
		}
		g.Revision = old.Revision + 1
		_, err = tx.Exec(`INSERT INTO worker_message_grants VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id,kind,subject,session_id) DO UPDATE SET enabled=excluded.enabled,revision=excluded.revision,provider_id=excluded.provider_id,provider_revision=excluded.provider_revision,model=excluded.model`, uid, kind, subject, g.SessionID, g.Enabled, g.Revision, g.ProviderID, g.ProviderRevision, g.Model)
		if err != nil {
			return err
		}
		if g.Enabled {
			if err = workerAllowed(tx, uid, kind, subject, g.SessionID, g.Revision); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`UPDATE terminal_chat_queue SET state='removed',text='',images='[]',note='Worker messaging permission changed.',revision=revision+1 WHERE user_id=? AND state='queued' AND origin='companion' AND id IN(SELECT queue_id FROM worker_message_receipts WHERE user_id=? AND kind=? AND subject=? AND session_id=?)`, uid, uid, kind, subject, g.SessionID); err != nil {
			return err
		}
		if kind == "gaffer" {
			_, err = tx.Exec(`UPDATE gaffer_conversations SET revision=revision+1,updated_at=? WHERE user_id=? AND id=?`, now(), uid, subject)
		}
		return err
	})
	return g, err
}
func (s *Store) WorkerScopesFor(uid int64, kind, subject string) ([]WorkerGrant, error) {
	if _, err := contextSubject(s.db, uid, kind, subject, true); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT session_id FROM worker_message_grants WHERE user_id=? AND kind=? AND subject=? AND enabled=1 ORDER BY session_id LIMIT 100`, uid, kind, subject)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []WorkerGrant{}
	for _, id := range ids {
		g, err := workerGrant(s.db, uid, kind, subject, id)
		if err != nil {
			return nil, err
		}
		if workerAllowed(s.db, uid, kind, subject, id, g.Revision) == nil {
			out = append(out, g)
		}
	}
	return out, nil
}

const workerMessageSelect = `r.id,r.session_id,r.queue_id,r.turn_id,r.text,
 CASE WHEN r.state='blocked' THEN 'blocked' WHEN q.state IS NULL THEN 'uncertain' WHEN q.state='failed' THEN 'blocked' ELSE q.state END,
 CASE WHEN r.state='blocked' THEN r.note ELSE COALESCE(q.note,'Queue receipt unavailable; delivery is uncertain.') END,COALESCE(q.revision,0),r.grant_revision,r.created_at`

func scanWorkerMessage(row interface{ Scan(...any) error }) (WorkerMessage, error) {
	var r WorkerMessage
	err := row.Scan(&r.ID, &r.SessionID, &r.QueueID, &r.TurnID, &r.Text, &r.State, &r.Note, &r.Revision, &r.GrantRevision, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return r, err
}
func (s *Store) WorkerMessageFor(uid int64, kind, subject, session, id string) (WorkerMessage, error) {
	return scanWorkerMessage(s.db.QueryRow(`SELECT `+workerMessageSelect+` FROM worker_message_receipts r LEFT JOIN terminal_chat_queue q ON q.user_id=r.user_id AND q.session_id=r.session_id AND q.id=r.queue_id WHERE r.user_id=? AND r.kind=? AND r.subject=? AND r.session_id=? AND r.id=?`, uid, kind, subject, session, id))
}
func (s *Store) WorkerMessagesFor(uid int64, kind, subject, session string) ([]WorkerMessage, error) {
	rows, err := s.db.Query(`SELECT `+workerMessageSelect+` FROM worker_message_receipts r LEFT JOIN terminal_chat_queue q ON q.user_id=r.user_id AND q.session_id=r.session_id AND q.id=r.queue_id WHERE r.user_id=? AND r.kind=? AND r.subject=? AND r.session_id=? ORDER BY r.created_at DESC,r.id LIMIT 100`, uid, kind, subject, session)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkerMessage{}
	for rows.Next() {
		r, err := scanWorkerMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Receipt and queue entry commit together. Duplicate requests never invoke a
// second enqueue, even after a crash between admission and the broker wakeup.
func (s *Store) AdmitWorkerMessageFor(uid int64, kind, subject string, m WorkerMessage, blocked string) (WorkerMessage, bool, error) {
	if !validGafferID(m.ID) || !ValidWorkerText(m.Text) {
		return m, false, terminalproto.ErrInvalid
	}

	sum := sha256.Sum256([]byte(m.Text))
	digest := hex.EncodeToString(sum[:])
	queue := sha256.Sum256([]byte(strconv.FormatInt(uid, 10) + "\x00" + kind + "\x00" + subject + "\x00" + m.SessionID + "\x00" + m.ID))
	m.QueueID = hex.EncodeToString(queue[:16])
	created := false
	err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, uid); err != nil {
			return err
		}
		if err := workerAllowed(tx, uid, kind, subject, m.SessionID, m.GrantRevision); err != nil {
			return err
		}
		var oldHash, oldTurn string
		err := tx.QueryRow(`SELECT text_hash,turn_id FROM worker_message_receipts WHERE user_id=? AND kind=? AND subject=? AND session_id=? AND id=?`, uid, kind, subject, m.SessionID, m.ID).Scan(&oldHash, &oldTurn)
		if err == nil {
			if oldHash != digest || oldTurn != m.TurnID {
				return ErrGafferConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if kind == "gaffer" {
			if m.ID != m.TurnID {
				return ErrGafferConflict
			}
			var permitted bool
			if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM gaffer_turns WHERE conversation_id=? AND id=? AND state='running' AND worker_grant_revision=? AND worker_grant_revision>0)`, subject, m.TurnID, m.GrantRevision).Scan(&permitted); err != nil {
				return err
			}
			if !permitted {
				return ErrNotFound
			}
			if err = goalTurnAllowed(tx, uid, subject, m.TurnID, true); err != nil {
				return err
			}
			var goalID string
			if err = tx.QueryRow(`SELECT goal_id FROM gaffer_turns WHERE conversation_id=? AND id=?`, subject, m.TurnID).Scan(&goalID); err != nil {
				return err
			}
			if goalID != "" {
				var repeats int
				if err = tx.QueryRow(`SELECT COUNT(*) FROM worker_message_receipts r JOIN gaffer_turns t ON t.conversation_id=r.subject AND t.id=r.turn_id WHERE r.user_id=? AND r.kind='gaffer' AND r.subject=? AND t.goal_id=? AND r.text_hash=?`, uid, subject, goalID, digest).Scan(&repeats); err != nil {
					return err
				}
				if repeats >= 2 {
					return ErrGafferGoalRepeat
				}
			}

		} else if m.TurnID != "" {
			return ErrNotFound
		}
		// Inaccessible scopes cannot be replayed, so their obsolete receipt bodies
		// may be removed to free the bounded account quota. Queue origin stays fenced.
		if _, err = tx.Exec(`DELETE FROM worker_message_receipts WHERE user_id=? AND ((kind='gaffer' AND NOT EXISTS(SELECT 1 FROM gaffer_conversations c WHERE c.user_id=worker_message_receipts.user_id AND c.id=worker_message_receipts.subject)) OR (kind='mcp' AND NOT EXISTS(SELECT 1 FROM mcp_connections c WHERE c.user_id=worker_message_receipts.user_id AND CAST(c.id AS TEXT)=worker_message_receipts.subject AND c.revoked_at=0)) OR EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.user_id=worker_message_receipts.user_id AND a.session_id=worker_message_receipts.session_id))`, uid); err != nil {
			return err
		}
		var count int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM worker_message_receipts WHERE user_id=?`, uid).Scan(&count); err != nil {
			return err
		}
		if count >= 500 {
			return ErrWorkerMessageFull
		}
		m.CreatedAt = now()
		state := "queued"
		if blocked != "" {
			state = "blocked"
		} else {
			if err = enqueueTerminal(tx, uid, m.SessionID, m.QueueID, m.Text, nil, "companion"); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`INSERT INTO worker_message_receipts VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, uid, kind, subject, m.SessionID, m.ID, m.QueueID, m.TurnID, m.Text, digest, m.GrantRevision, state, blocked, m.CreatedAt)
		created = err == nil
		return err
	})
	if err != nil {
		return m, false, err
	}
	m, err = s.WorkerMessageFor(uid, kind, subject, m.SessionID, m.ID)
	return m, created, err
}

// Check both at claim and just before handoff. Orphan companion items fail closed.
func workerDelivery(q contextQuerier, uid int64, session, id string) error {
	var kind, subject string
	var revision int64
	err := q.QueryRow(`SELECT kind,subject,grant_revision FROM worker_message_receipts WHERE user_id=? AND session_id=? AND queue_id=?`, uid, session, id).Scan(&kind, &subject, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err = workerAllowed(q, uid, kind, subject, session, revision); err != nil {
		return err
	}
	if kind == "gaffer" {
		var turn string
		if err = q.QueryRow(`SELECT turn_id FROM worker_message_receipts WHERE user_id=? AND queue_id=?`, uid, id).Scan(&turn); err != nil {
			return err
		}
		return goalTurnAllowed(q, uid, subject, turn, false)
	}
	return nil
}
func (s *Store) CheckWorkerDelivery(uid int64, session, id string) error {
	return workerDelivery(s.db, uid, session, id)
}
func (s *Store) InterruptWorkerMessageFor(uid int64, kind, subject, session, id string, revision int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, uid); err != nil {
			return err
		}
		var uncertain int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM terminal_chat_queue WHERE user_id=? AND session_id=? AND state IN('uncertain','sending')`, uid, session).Scan(&uncertain); err != nil {
			return err
		}
		if uncertain > 0 {
			return ErrChatQueueConflict
		}

		m, err := scanWorkerMessage(tx.QueryRow(`SELECT `+workerMessageSelect+` FROM worker_message_receipts r LEFT JOIN terminal_chat_queue q ON q.user_id=r.user_id AND q.session_id=r.session_id AND q.id=r.queue_id WHERE r.user_id=? AND r.kind=? AND r.subject=? AND r.session_id=? AND r.id=?`, uid, kind, subject, session, id))
		if err != nil {
			return err
		}
		if err = workerAllowed(tx, uid, kind, subject, session, m.GrantRevision); err != nil {
			return err
		}
		result, err := tx.Exec(`UPDATE terminal_chat_queue SET intent='stop',note='Explicit stop and send requested.',revision=revision+1 WHERE user_id=? AND session_id=? AND id=? AND revision=? AND state='queued' AND origin='companion'`, uid, session, m.QueueID, revision)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrChatQueueConflict
		}
		return nil
	})
}

func (s *Store) CheckContextManagementSubject(uid int64, kind, subject string) (string, error) {
	return contextSubject(s.db, uid, kind, subject, false)
}
func (s *Store) WorkerGrantScopesFor(uid int64, kind, subject string) ([]WorkerGrant, error) {
	if _, err := contextSubject(s.db, uid, kind, subject, false); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT session_id,enabled,revision,provider_id,provider_revision,model FROM worker_message_grants WHERE user_id=? AND kind=? AND subject=? AND enabled=1 ORDER BY session_id LIMIT 100`, uid, kind, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkerGrant{}
	for rows.Next() {
		var g WorkerGrant
		if err = rows.Scan(&g.SessionID, &g.Enabled, &g.Revision, &g.ProviderID, &g.ProviderRevision, &g.Model); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
