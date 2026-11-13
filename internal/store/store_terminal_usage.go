package store

import (
	"database/sql"
	"errors"
)

// TerminalSessionUsage is what a chat's agent has reported using. The tokens
// are summed over its finished turns; the context figures are its last report.
// Reported is false for a session whose agent has said nothing.
type TerminalSessionUsage struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	CachedTokens int64 `json:"cachedTokens"`
	ContextUsed  int64 `json:"contextUsed"`
	ContextSize  int64 `json:"contextSize"`
	UpdatedAt    int64 `json:"updatedAt"`
	Reported     bool  `json:"reported"`
}

func (s *Store) migrateTerminalUsage() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS terminal_session_usage(
		user_id INTEGER NOT NULL, session_id TEXT NOT NULL,
		input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0, cached_tokens INTEGER NOT NULL DEFAULT 0,
		context_used INTEGER NOT NULL DEFAULT 0, context_size INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL,
		PRIMARY KEY(user_id, session_id))`)
	if err != nil {
		return err
	}
	// The table has no foreign key, so what an account's sessions reported goes with the account.
	_, err = s.db.Exec(`CREATE TRIGGER IF NOT EXISTS terminal_session_usage_owner AFTER DELETE ON users BEGIN
		DELETE FROM terminal_session_usage WHERE user_id=old.id; END`)
	return err
}

// AddTerminalSessionUsage records one report: token counts are added to the
// session's totals, and a context report (size above zero) replaces the last.
// It is kept only for a session this account owns.
func (s *Store) AddTerminalSessionUsage(uid int64, session string, input, output, cached, contextUsed, contextSize int64) error {
	if input < 0 || output < 0 || cached < 0 || contextUsed < 0 || contextSize < 0 {
		return errors.New("usage cannot be negative")
	}
	if _, err := s.TerminalSessionFor(uid, session); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO terminal_session_usage(user_id,session_id,input_tokens,output_tokens,cached_tokens,context_used,context_size,updated_at) VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id,session_id) DO UPDATE SET
			input_tokens=input_tokens+excluded.input_tokens, output_tokens=output_tokens+excluded.output_tokens, cached_tokens=cached_tokens+excluded.cached_tokens,
			context_used=CASE WHEN excluded.context_size>0 THEN excluded.context_used ELSE context_used END,
			context_size=CASE WHEN excluded.context_size>0 THEN excluded.context_size ELSE context_size END,
			updated_at=excluded.updated_at`, uid, session, input, output, cached, contextUsed, contextSize, now())
	return err
}

// TerminalSessionUsageFor reads a session's reported usage. A session with no
// report answers the zero value with Reported false.
func (s *Store) TerminalSessionUsageFor(uid int64, session string) (TerminalSessionUsage, error) {
	var u TerminalSessionUsage
	err := s.db.QueryRow(`SELECT input_tokens,output_tokens,cached_tokens,context_used,context_size,updated_at FROM terminal_session_usage WHERE user_id=? AND session_id=?`, uid, session).
		Scan(&u.InputTokens, &u.OutputTokens, &u.CachedTokens, &u.ContextUsed, &u.ContextSize, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, nil
	}
	u.Reported = err == nil
	return u, err
}
