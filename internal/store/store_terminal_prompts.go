package store

import (
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	MaxTerminalPrompts      = 200
	MaxTerminalPromptBytes  = 8000
	MaxTerminalPromptNotes  = 500
	MaxTerminalPromptCat    = 40
	MaxTerminalPromptAgent  = 64
	MaxTerminalPromptModel  = 200
	MaxTerminalPromptEffort = 64
)

var ErrTerminalPromptLimit = errors.New("saved prompt limit reached")

// TerminalPrompt is one user-authored, reusable opening prompt. It is account-scoped
// and never expands a token itself: the browser resolves variables and skill
// references when the person inserts it.
type TerminalPrompt struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Body        string `json:"body"`
	Category    string `json:"category"`
	Tool        string `json:"tool"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	Revision    int64  `json:"revision"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// TerminalPromptWrite is the exact text a save carries. Revision fences a
// concurrent editor; it is zero for a new prompt.
type TerminalPromptWrite struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Body        string `json:"body"`
	Category    string `json:"category"`
	Tool        string `json:"tool"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	Revision    int64  `json:"revision"`
}

const terminalPromptCols = `id,title,description,body,category,tool,model,effort,revision,created_at,updated_at`

func readTerminalPrompt(row interface{ Scan(...any) error }) (TerminalPrompt, error) {
	var item TerminalPrompt
	err := row.Scan(&item.ID, &item.Title, &item.Description, &item.Body, &item.Category, &item.Tool, &item.Model, &item.Effort, &item.Revision, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrRemoteNotFound
	}
	return item, err
}

// validPromptPlain allows an empty value and rejects control characters. These
// fields are one line each, so a line break is not plain text here.
func validPromptPlain(value string, max int) bool {
	return validPromptText(value, max, false)
}

// validPromptNotes is the description: line breaks are part of written notes.
func validPromptNotes(value string, max int) bool {
	return validPromptText(value, max, true)
}

func validPromptText(value string, max int, allowNewlines bool) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > max {
		return false
	}
	for _, r := range value {
		allowed := allowNewlines && (r == '\n' || r == '\r' || r == '\t')
		if unicode.IsControl(r) && !allowed {
			return false
		}
	}
	return true
}

// validTerminalPrompt accepts the prompt body as editable prose: line breaks and
// tabs are fine, terminal control codes are not.
func validTerminalPrompt(in TerminalPromptWrite) bool {
	title := strings.TrimSpace(in.Title)
	if !terminalproto.ValidTitle(title) || !utf8.ValidString(in.Body) || strings.TrimSpace(in.Body) == "" || len(in.Body) > MaxTerminalPromptBytes {
		return false
	}
	if !validPromptNotes(in.Description, MaxTerminalPromptNotes) || !validPromptPlain(in.Category, MaxTerminalPromptCat) ||
		!validPromptPlain(in.Tool, MaxTerminalPromptAgent) || !validPromptPlain(in.Model, MaxTerminalPromptModel) || !validPromptPlain(in.Effort, MaxTerminalPromptEffort) {
		return false
	}
	for _, r := range in.Body {
		allowedWhitespace := r == '\n' || r == '\r' || r == '\t'
		if unicode.IsControl(r) && !allowedWhitespace {
			return false
		}
	}
	return true
}

func lockTerminalPromptOwner(tx *sql.Tx, uid int64) error {
	result, err := tx.Exec(`UPDATE users SET id=id WHERE id=? AND disabled_at=0 AND role<>'viewer'`, uid)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrRemoteNotFound
	}
	return nil
}

func (s *Store) TerminalPromptsFor(uid int64) ([]TerminalPrompt, error) {
	rows, err := s.db.Query(`SELECT `+terminalPromptCols+` FROM terminal_prompts WHERE user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer') ORDER BY lower(title),id`, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TerminalPrompt{}
	for rows.Next() {
		item, err := readTerminalPrompt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// SaveTerminalPromptFor stores only the person's text and their optional
// suggestions. Nothing is sent to a machine, and no token is resolved here.
func (s *Store) SaveTerminalPromptFor(uid int64, id string, in TerminalPromptWrite) (TerminalPrompt, error) {
	in.Title = strings.TrimSpace(in.Title)
	var item TerminalPrompt
	createOK := id != "" || in.Revision == 0
	updateOK := id == "" || (terminalproto.ValidID(id) && terminalproto.ValidCounter(in.Revision))
	if !validTerminalPrompt(in) || !createOK || !updateOK {
		return item, terminalproto.ErrInvalid
	}
	err := s.withTx(func(tx *sql.Tx) error {
		if err := lockTerminalPromptOwner(tx, uid); err != nil {
			return err
		}
		if id == "" {
			var count int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM terminal_prompts WHERE user_id=?`, uid).Scan(&count); err != nil {
				return err
			}
			if count >= MaxTerminalPrompts {
				return ErrTerminalPromptLimit
			}
			item = TerminalPrompt{
				ID: syncID(), Title: in.Title, Description: in.Description, Body: in.Body, Category: in.Category,
				Tool: in.Tool, Model: in.Model, Effort: in.Effort, Revision: 1, CreatedAt: now(), UpdatedAt: now(),
			}
			_, err := tx.Exec(`INSERT INTO terminal_prompts(id,user_id,title,description,body,category,tool,model,effort,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
				item.ID, uid, item.Title, item.Description, item.Body, item.Category, item.Tool, item.Model, item.Effort, item.Revision, item.CreatedAt, item.UpdatedAt)
			return err
		}
		previous, err := readTerminalPrompt(tx.QueryRow(`SELECT `+terminalPromptCols+` FROM terminal_prompts WHERE id=? AND user_id=?`, id, uid))
		if err != nil {
			return err
		}
		if previous.Revision != in.Revision || in.Revision >= terminalproto.MaxCounter {
			return ErrTerminalConflict
		}
		item = TerminalPrompt{
			ID: id, Title: in.Title, Description: in.Description, Body: in.Body, Category: in.Category,
			Tool: in.Tool, Model: in.Model, Effort: in.Effort, Revision: in.Revision + 1, CreatedAt: previous.CreatedAt, UpdatedAt: now(),
		}
		_, err = tx.Exec(`UPDATE terminal_prompts SET title=?,description=?,body=?,category=?,tool=?,model=?,effort=?,revision=?,updated_at=? WHERE id=? AND user_id=?`,
			item.Title, item.Description, item.Body, item.Category, item.Tool, item.Model, item.Effort, item.Revision, item.UpdatedAt, id, uid)
		return err
	})
	return item, err
}

func (s *Store) DeleteTerminalPromptFor(uid int64, id string, revision int64) error {
	if !terminalproto.ValidID(id) || !terminalproto.ValidCounter(revision) {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockTerminalPromptOwner(tx, uid); err != nil {
			return err
		}
		item, err := readTerminalPrompt(tx.QueryRow(`SELECT `+terminalPromptCols+` FROM terminal_prompts WHERE id=? AND user_id=?`, id, uid))
		if err != nil {
			return err
		}
		if item.Revision != revision {
			return ErrTerminalConflict
		}
		_, err = tx.Exec(`DELETE FROM terminal_prompts WHERE id=? AND user_id=?`, id, uid)
		return err
	})
}
