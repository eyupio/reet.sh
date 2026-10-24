package store

import (
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const MaxTerminalCommands = 200

var ErrTerminalCommandLimit = errors.New("saved command limit reached")

type TerminalCommand struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Revision    int64  `json:"revision"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

const terminalCommandCols = `id,name,command,description,revision,created_at,updated_at`

func readTerminalCommand(row interface{ Scan(...any) error }) (TerminalCommand, error) {
	var item TerminalCommand
	err := row.Scan(&item.ID, &item.Name, &item.Command, &item.Description, &item.Revision, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrRemoteNotFound
	}
	return item, err
}
func validTerminalCommand(name, command, description string) bool {
	if !terminalproto.ValidTitle(name) || !utf8.ValidString(command) || !utf8.ValidString(description) || strings.TrimSpace(command) == "" || len(command) > 16<<10 || utf8.RuneCountInString(description) > 500 {
		return false
	}
	for _, r := range command {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func lockTerminalCommandOwner(tx *sql.Tx, uid int64) error {
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
func (s *Store) TerminalCommandsFor(uid int64) ([]TerminalCommand, error) {
	rows, err := s.db.Query(`SELECT `+terminalCommandCols+` FROM terminal_commands WHERE user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer') ORDER BY lower(name),id`, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TerminalCommand{}
	for rows.Next() {
		item, err := readTerminalCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// SaveTerminalCommandFor stores user-authored text only. It never expands,
// evaluates or dispatches a command to a host. Revisions fence concurrent editors.
func (s *Store) SaveTerminalCommandFor(uid int64, id, name, command, description string, revision int64) (TerminalCommand, error) {
	name = strings.TrimSpace(name)
	var item TerminalCommand
	if !validTerminalCommand(name, command, description) || id != "" && !terminalproto.ValidID(id) || id == "" && revision != 0 || id != "" && !terminalproto.ValidCounter(revision) {
		return item, terminalproto.ErrInvalid
	}
	err := s.withTx(func(tx *sql.Tx) error {
		if err := lockTerminalCommandOwner(tx, uid); err != nil {
			return err
		}
		if id == "" {
			var count int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM terminal_commands WHERE user_id=?`, uid).Scan(&count); err != nil {
				return err
			}
			if count >= MaxTerminalCommands {
				return ErrTerminalCommandLimit
			}
			item = TerminalCommand{ID: syncID(), Name: name, Command: command, Description: description, Revision: 1, CreatedAt: now(), UpdatedAt: now()}
			_, err := tx.Exec(`INSERT INTO terminal_commands(id,user_id,name,command,description,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, item.ID, uid, name, command, description, item.Revision, item.CreatedAt, item.UpdatedAt)
			return err
		}
		previous, err := readTerminalCommand(tx.QueryRow(`SELECT `+terminalCommandCols+` FROM terminal_commands WHERE id=? AND user_id=?`, id, uid))
		if err != nil {
			return err
		}
		if previous.Revision != revision || revision >= terminalproto.MaxCounter {
			return ErrTerminalConflict
		}
		item = TerminalCommand{ID: id, Name: name, Command: command, Description: description, Revision: revision + 1, CreatedAt: previous.CreatedAt, UpdatedAt: now()}
		_, err = tx.Exec(`UPDATE terminal_commands SET name=?,command=?,description=?,revision=?,updated_at=? WHERE id=? AND user_id=?`, name, command, description, item.Revision, item.UpdatedAt, id, uid)
		return err
	})
	return item, err
}
func (s *Store) DeleteTerminalCommandFor(uid int64, id string, revision int64) error {
	if !terminalproto.ValidID(id) || !terminalproto.ValidCounter(revision) {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockTerminalCommandOwner(tx, uid); err != nil {
			return err
		}
		item, err := readTerminalCommand(tx.QueryRow(`SELECT `+terminalCommandCols+` FROM terminal_commands WHERE id=? AND user_id=?`, id, uid))
		if err != nil {
			return err
		}
		if item.Revision != revision {
			return ErrTerminalConflict
		}
		_, err = tx.Exec(`DELETE FROM terminal_commands WHERE id=? AND user_id=?`, id, uid)
		return err
	})
}
