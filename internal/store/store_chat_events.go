package store

import (
	"database/sql"
	"encoding/json"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Controller-side chat transcript bounds. They match the live ring so a
// reconnect after a controller restart sees the same bounded conversation the
// running controller holds.
const (
	maxStoredChatEvents = 1000
	maxStoredChatBytes  = 256 << 10
)

// StoredChatEvent is one bounded event of a structured chat's transcript.
type StoredChatEvent struct {
	Seq   int64
	At    int64
	Event []byte
}

// AppendTerminalChatEventFor stores one chat event. It is a single-event
// wrapper around AppendTerminalChatEventsFor so a caller that has one event
// keeps the same idempotent primary-key behaviour.
func (s *Store) AppendTerminalChatEventFor(uid int64, sessionID string, seq, at int64, event []byte) error {
	return s.AppendTerminalChatEventsFor(uid, sessionID, []StoredChatEvent{{Seq: seq, At: at, Event: event}})
}

// AppendTerminalChatEventsFor stores a batch of chat events in one
// transaction. The primary key is the chat's own sequence number, so retrying
// the same batch is idempotent. Batching matters because each commit is an
// fsync in WAL mode: one transaction per streamed chunk is otherwise the
// dominant cost of a live chat. The caller trims the transcript in batches
// with TrimTerminalChatEventsFor.
func (s *Store) AppendTerminalChatEventsFor(uid int64, sessionID string, events []StoredChatEvent) error {
	if len(events) == 0 {
		return nil
	}
	if !terminalproto.ValidID(sessionID) {
		return terminalproto.ErrInvalid
	}
	for _, event := range events {
		if !terminalproto.ValidCounter(event.Seq) || event.At <= 0 ||
			len(event.Event) == 0 || len(event.Event) > maxStoredChatBytes || !json.Valid(event.Event) {
			return terminalproto.ErrInvalid
		}
	}
	return s.withTx(func(tx *sql.Tx) error {
		for _, event := range events {
			if _, err := tx.Exec(`INSERT INTO terminal_chat_events(user_id,session_id,seq,at,event,bytes)
				VALUES(?,?,?,?,?,?) ON CONFLICT(user_id,session_id,seq) DO NOTHING`,
				uid, sessionID, event.Seq, event.At, string(event.Event), len(event.Event)); err != nil {
				return err
			}
		}
		return nil
	})
}

// TrimTerminalChatEventsFor drops stored events older than beforeSeq, keeping
// the persisted transcript aligned with the live ring. It is a single indexed
// range delete, so callers can batch it instead of paying per event.
func (s *Store) TrimTerminalChatEventsFor(uid int64, sessionID string, beforeSeq int64) error {
	if !terminalproto.ValidID(sessionID) || beforeSeq <= 1 {
		return terminalproto.ErrInvalid
	}
	_, err := s.db.Exec(`DELETE FROM terminal_chat_events WHERE user_id=? AND session_id=? AND seq<?`, uid, sessionID, beforeSeq)
	return err
}

// TerminalChatEventsFor reads a chat transcript in order. It returns nil when
// none is stored, which is not an error.
func (s *Store) TerminalChatEventsFor(uid int64, sessionID string) ([]StoredChatEvent, error) {
	if !terminalproto.ValidID(sessionID) {
		return nil, terminalproto.ErrInvalid
	}
	rows, err := s.db.Query(`SELECT seq,at,event FROM terminal_chat_events WHERE user_id=? AND session_id=? ORDER BY seq`,
		uid, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredChatEvent{}
	for rows.Next() {
		var e StoredChatEvent
		var event string
		if err := rows.Scan(&e.Seq, &e.At, &event); err != nil {
			return nil, err
		}
		e.Event = []byte(event)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteTerminalChatEventsFor drops a chat's stored transcript, for example
// when it is archived or its live history is closed.
func (s *Store) DeleteTerminalChatEventsFor(uid int64, sessionID string) error {
	if !terminalproto.ValidID(sessionID) {
		return terminalproto.ErrInvalid
	}
	_, err := s.db.Exec(`DELETE FROM terminal_chat_events WHERE user_id=? AND session_id=?`, uid, sessionID)
	return err
}
