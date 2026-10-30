package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const workingWindow = 30 * time.Second

// Session aliases TerminalSession for attention derivation callers.
type Session = TerminalSession

func (s *Store) migrateAttention() error {
	for _, ddl := range []string{
		"activity_at INTEGER NOT NULL DEFAULT 0",
		"bell_at INTEGER NOT NULL DEFAULT 0",
		"attention_observed INTEGER NOT NULL DEFAULT 0 CHECK(attention_observed IN (0,1))",
	} {
		col := strings.Fields(ddl)[0]
		ok, err := hasColumn(s.db, "terminal_sessions", col)
		if err != nil {
			return err
		}
		if !ok {
			if _, err := s.db.Exec(`ALTER TABLE terminal_sessions ADD COLUMN ` + ddl); err != nil {
				return err
			}
		}
	}
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS terminal_session_seen (
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		session_id TEXT NOT NULL REFERENCES terminal_sessions(id) ON DELETE CASCADE,
		seen_at INTEGER NOT NULL,
		PRIMARY KEY(user_id, session_id)
	)`)
	return err
}

func attentionTime(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	if v < 100_000_000_000 {
		return time.Unix(v, 0)
	}
	return time.UnixMilli(v)
}

func attentionMillis(v int64) int64 {
	if v <= 0 {
		return 0
	}
	if v < 100_000_000_000 {
		return v * 1000
	}
	return v
}

// AttentionState derives the session's attention word and its source
// ("terminal" or "chat") at now. Sessions from peers that have not reported
// attention return "", "".
func AttentionState(s Session, now time.Time) (state, source string) {
	if s.NoAttention {
		return "", ""
	}
	src := "terminal"
	if s.Structured {
		src = "chat"
	}
	switch s.State {
	case terminalproto.Creating:
		if !s.AttentionObserved && s.ActivityAt == 0 && s.BellAt == 0 && !s.Structured {
			return "", ""
		}
		return "starting", src
	case terminalproto.Running:
		bell := attentionMillis(s.BellAt)
		seen := attentionMillis(s.SeenAt)
		if bell > 0 && bell > seen {
			return "waiting", src
		}
		if s.ActivityAt > 0 {
			if now.Sub(attentionTime(s.ActivityAt)) <= workingWindow {
				return "working", src
			}
			return "quiet", src
		}
		if s.AttentionObserved || s.BellAt > 0 || s.Structured {
			return "quiet", src
		}
		return "", ""
	case terminalproto.Exited:
		if s.ExitCode != nil {
			if *s.ExitCode == 0 {
				return "finished", src
			}
			return "failed", src
		}
		if s.AttentionObserved || s.ActivityAt > 0 || s.BellAt > 0 {
			return "finished", src
		}
		return "", ""
	case terminalproto.Lost:
		if s.AttentionObserved || s.ActivityAt > 0 || s.BellAt > 0 || s.Structured {
			return "failed", src
		}
		return "", ""
	default:
		return "", ""
	}
}

func changedSinceSeen(s Session) bool {
	if s.NoAttention {
		return false
	}
	if !s.AttentionObserved && s.ActivityAt == 0 && s.BellAt == 0 && !s.Structured {
		return false
	}
	seen := attentionMillis(s.SeenAt)
	latest := attentionMillis(s.UpdatedAt)
	if act := attentionMillis(s.ActivityAt); act > latest {
		latest = act
	}
	if bell := attentionMillis(s.BellAt); bell > latest {
		latest = bell
	}
	return latest > seen
}

// ObserveTerminalAttentionFor records pane activity and bell timestamps from an
// attention-v1 peer for the active generation.
func (s *Store) ObserveTerminalAttentionFor(uid int64, agentID, sessionID string, generation, activityAt, bellAt int64) error {
	if !terminalproto.ValidID(sessionID) || activityAt < 0 || bellAt < 0 {
		return terminalproto.ErrInvalid
	}
	actMs := attentionMillis(activityAt)
	bellMs := attentionMillis(bellAt)
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		if _, err := currentGeneration(tx, uid, agentID, generation); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE terminal_sessions SET
			activity_at = CASE WHEN ? > activity_at THEN ? ELSE activity_at END,
			bell_at = CASE WHEN ? > bell_at THEN ? ELSE bell_at END,
			attention_observed = 1
			WHERE user_id = ? AND agent_id = ? AND id = ?`,
			actMs, actMs, bellMs, bellMs, uid, agentID, sessionID)
		return err
	})
}

// SetTerminalChatAttentionFor records the current activity and waiting timestamps
// of a structured chat session without advancing updated_at.
func (s *Store) SetTerminalChatAttentionFor(uid int64, sessionID string, activityAt, bellAt int64) error {
	if !terminalproto.ValidID(sessionID) || activityAt < 0 || bellAt < 0 {
		return terminalproto.ErrInvalid
	}
	actMs := attentionMillis(activityAt)
	bellMs := attentionMillis(bellAt)
	_, err := s.db.Exec(`UPDATE terminal_sessions SET
		activity_at = CASE WHEN ? > activity_at THEN ? ELSE activity_at END,
		bell_at = ?,
		attention_observed = 1
		WHERE user_id = ? AND id = ? AND structured = 1`,
		actMs, actMs, bellMs, uid, sessionID)
	return err
}

// MarkSessionSeenFor records when uid last saw sessionID. Each person has their
// own seen timestamp per session.
func (s *Store) MarkSessionSeenFor(uid int64, sessionID string, at int64) error {
	if uid <= 0 || !terminalproto.ValidID(sessionID) {
		return terminalproto.ErrInvalid
	}
	seenMs := attentionMillis(at)
	if seenMs <= 0 {
		seenMs = now()
	}
	return s.withTx(func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM terminal_sessions WHERE id=?`, sessionID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRemoteNotFound
			}
			return err
		}
		_, err := tx.Exec(`INSERT INTO terminal_session_seen(user_id, session_id, seen_at) VALUES(?, ?, ?)
			ON CONFLICT(user_id, session_id) DO UPDATE SET seen_at = CASE WHEN excluded.seen_at > terminal_session_seen.seen_at THEN excluded.seen_at ELSE terminal_session_seen.seen_at END`,
			uid, sessionID, seenMs)
		return err
	})
}

// SessionSeenFor returns the seen_at timestamp recorded for uid on sessionID,
// or 0 if uid has not marked the session seen.
func (s *Store) SessionSeenFor(uid int64, sessionID string) (int64, error) {
	var seenAt int64
	err := s.db.QueryRow(`SELECT seen_at FROM terminal_session_seen WHERE user_id=? AND session_id=?`, uid, sessionID).Scan(&seenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return seenAt, err
}

func (s *Store) populateAttention(uid int64, item *TerminalSession, nowTime time.Time) {
	if item == nil {
		return
	}
	if seen, err := s.SessionSeenFor(uid, item.ID); err == nil {
		item.SeenAt = seen
	}
	if !item.AttentionObserved && item.ActivityAt == 0 && item.BellAt == 0 && !item.Structured {
		item.NoAttention = true
		return
	}
	item.Attention, item.AttentionSource = AttentionState(*item, nowTime)
	item.ChangedSinceSeen = changedSinceSeen(*item)
}
