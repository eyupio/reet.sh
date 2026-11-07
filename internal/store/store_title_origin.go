package store

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// retitleMaxTries is how many first messages may be tried before a fallback
// title is left alone for good.
const retitleMaxTries = 3

// migrateTitleOrigin adds who-owns-the-title and the retitle attempt counter.
// Rows that exist already keep their title, so they become manual.
func (s *Store) migrateTitleOrigin() error {
	for _, ddl := range []string{
		"title_origin TEXT NOT NULL DEFAULT 'manual' CHECK(title_origin IN ('manual','prompt','fallback'))",
		"retitle_tries INTEGER NOT NULL DEFAULT 0",
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
	return nil
}

// RetitleChatFromPrompt renames a structured chat whose title is still the
// generic fallback from the text of its first real message. Each call on such
// a chat spends one of retitleMaxTries, whether or not the text yields a
// title; the try and the rename commit together. It returns false, with
// nothing changed, for a chat that is not eligible or not the caller's.
func (s *Store) RetitleChatFromPrompt(uid int64, sessionID, text string) (renamed bool, err error) {
	err = s.withTx(func(tx *sql.Tx) error {
		renamed = false
		var structured bool
		var origin string
		var tries int
		err := tx.QueryRow(`SELECT structured,title_origin,retitle_tries FROM terminal_sessions WHERE id=? AND user_id=?`, sessionID, uid).Scan(&structured, &origin, &tries)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if !structured || terminalproto.TitleOrigin(origin) != terminalproto.TitleFallback || tries >= retitleMaxTries {
			return nil
		}
		if _, err := tx.Exec(`UPDATE terminal_sessions SET retitle_tries=retitle_tries+1 WHERE id=? AND user_id=?`, sessionID, uid); err != nil {
			return err
		}
		title := terminalproto.PromptTitle(text)
		if title == "" || !terminalproto.ValidTitle(title) {
			return nil
		}
		r, err := tx.Exec(`UPDATE terminal_sessions SET title=?,title_origin='prompt',updated_at=? WHERE id=? AND user_id=? AND title_origin='fallback'`, title, now(), sessionID, uid)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		renamed = n == 1
		return err
	})
	if err != nil {
		return false, err
	}
	return renamed, nil
}

// TerminalTitleOriginFor reads who owns a session's title. It is not part of
// the session listing.
func (s *Store) TerminalTitleOriginFor(uid int64, sessionID string) (terminalproto.TitleOrigin, error) {
	var origin string
	err := s.db.QueryRow(`SELECT title_origin FROM terminal_sessions WHERE id=? AND user_id=?`, sessionID, uid).Scan(&origin)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrRemoteNotFound
	}
	if err != nil {
		return "", err
	}
	return terminalproto.TitleOrigin(origin), nil
}
