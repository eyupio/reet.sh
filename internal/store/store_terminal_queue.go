package store

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var ErrChatQueueConflict = errors.New("that queued message changed; refresh the queue")
var ErrChatQueueFull = errors.New("the queue is full; send or remove a message first")

// TerminalQueuedMessage is shared by all browsers. Completed rows retain only
// their idempotency key until the session is deleted, so a lost HTTP reply can
// never turn a repeated enqueue into a second delivery.
type TerminalQueuedMessage struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Images   []string `json:"images"`
	State    string   `json:"state"`
	Intent   string   `json:"intent"`
	Note     string   `json:"note"`
	Revision int64    `json:"revision"`
}

func (s *Store) TerminalQueueFor(uid int64, session string) ([]TerminalQueuedMessage, error) {
	if _, err := s.TerminalSessionFor(uid, session); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,text,images,state,intent,note,revision FROM terminal_chat_queue WHERE user_id=? AND session_id=? AND state NOT IN ('sent','removed') ORDER BY (intent IN ('send','stop','stopping')) DESC, seq`, uid, session)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TerminalQueuedMessage{}
	for rows.Next() {
		var q TerminalQueuedMessage
		var images string
		if err := rows.Scan(&q.ID, &q.Text, &images, &q.State, &q.Intent, &q.Note, &q.Revision); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(images), &q.Images); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (s *Store) EnqueueTerminalFor(uid int64, session, id, text string, images []string) error {
	if !terminalproto.ValidID(id) || !terminalproto.ValidACPPrompt(text, images) {
		return terminalproto.ErrInvalid
	}
	if images == nil {
		images = []string{}
	}
	encoded, _ := json.Marshal(images)
	return s.withTx(func(tx *sql.Tx) error {
		var count int
		// Acquire the write lock before counting, including across Store handles.
		if _, err := tx.Exec(`UPDATE terminal_sessions SET id=id WHERE id=? AND user_id=?`, session, uid); err != nil {
			return err
		}
		// Ownership is checked inside the mutation transaction too.
		if err := tx.QueryRow(`SELECT COUNT(*) FROM terminal_sessions WHERE user_id=? AND id=? AND structured=1 AND state='running' AND NOT EXISTS(SELECT 1 FROM terminal_session_archives WHERE session_id=terminal_sessions.id)`, uid, session).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return ErrRemoteNotFound
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM terminal_chat_queue WHERE user_id=? AND session_id=? AND id=?`, uid, session, id).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM terminal_chat_queue WHERE user_id=? AND session_id=? AND state NOT IN ('sent','removed')`, uid, session).Scan(&count); err != nil {
			return err
		}
		if count >= 10 {
			return ErrChatQueueFull
		}
		_, err := tx.Exec(`INSERT INTO terminal_chat_queue(user_id,session_id,id,text,images,state,intent,note,revision,owner) VALUES(?,?,?,?,?,'queued','wait','',1,'')`, uid, session, id, text, string(encoded))
		return err
	})
}

// ChangeTerminalQueueFor uses optimistic revisions so two tabs cannot edit or
// remove a message that a worker has already claimed. Retrying uncertainty
// requires an explicit acknowledgement that it can duplicate a delivery.
func (s *Store) ChangeTerminalQueueFor(uid int64, session, id string, revision int64, action, text string, acknowledge bool) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE terminal_chat_queue SET revision=revision WHERE user_id=? AND session_id=? AND id=?`, uid, session, id); err != nil {
			return err
		}
		var state, intent string
		var current int64
		err := tx.QueryRow(`SELECT state,intent,revision FROM terminal_chat_queue WHERE user_id=? AND session_id=? AND id=?`, uid, session, id).Scan(&state, &intent, &current)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
		if err != nil {
			return err
		}
		if current != revision || state == "sending" || state == "sent" || state == "removed" {
			return ErrChatQueueConflict
		}
		note := ""
		switch action {
		case "edit":
			if state != "queued" && state != "failed" {
				return ErrChatQueueConflict
			}
			var images string
			if err := tx.QueryRow(`SELECT images FROM terminal_chat_queue WHERE user_id=? AND session_id=? AND id=?`, uid, session, id).Scan(&images); err != nil {
				return err
			}
			var paths []string
			if json.Unmarshal([]byte(images), &paths) != nil || !terminalproto.ValidACPPrompt(text, paths) {
				return terminalproto.ErrInvalid
			}
			_, err = tx.Exec(`UPDATE terminal_chat_queue SET text=?,state='queued',intent='retry',note='',revision=revision+1 WHERE user_id=? AND session_id=? AND id=?`, text, uid, session, id)
			return err
		case "remove":
			state = "removed"
		case "retry":
			if state == "uncertain" && !acknowledge {
				return ErrChatQueueConflict
			}
			state, intent = "queued", "retry"
		case "send", "stop":
			if state != "queued" {
				return ErrChatQueueConflict
			}
			intent = action
		default:
			return terminalproto.ErrInvalid
		}
		if state == "removed" {
			_, err = tx.Exec(`UPDATE terminal_chat_queue SET text='',images='[]',state='removed',note='',revision=revision+1 WHERE user_id=? AND session_id=? AND id=?`, uid, session, id)
			return err
		}
		_, err = tx.Exec(`UPDATE terminal_chat_queue SET state=?,intent=?,note=?,revision=revision+1 WHERE user_id=? AND session_id=? AND id=?`, state, intent, note, uid, session, id)
		return err
	})
}

func (s *Store) RecoverTerminalQueueFor(uid int64, session, owner string) error {
	_, err := s.db.Exec(`UPDATE terminal_chat_queue SET state='uncertain',note='The controller restarted during delivery. Check the conversation before retrying.',revision=revision+1 WHERE user_id=? AND session_id=? AND state='sending' AND owner!=?`, uid, session, owner)
	return err
}

// TransitionTerminalQueueFor atomically reserves or completes one immutable
// revision. A crash after reservation is deliberately treated as uncertain.
func (s *Store) TransitionTerminalQueueFor(uid int64, session string, q TerminalQueuedMessage, state, intent, note, owner string) (bool, error) {
	text, images := q.Text, q.Images
	if state == "sent" {
		text = ""
		images = []string{}
	}
	encoded, _ := json.Marshal(images)
	r, err := s.db.Exec(`UPDATE terminal_chat_queue SET state=?,intent=?,note=?,owner=?,text=?,images=?,revision=revision+1 WHERE user_id=? AND session_id=? AND id=? AND revision=? AND state=?`, state, intent, note, owner, text, string(encoded), uid, session, q.ID, q.Revision, q.State)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}
