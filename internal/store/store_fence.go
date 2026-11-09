package store

import (
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Fence is the persisted, owner-set "this controller has moved" state. It
// changes no data: it is two settings rows, so lifting it returns the
// controller to exactly what it was. The state is read from the database each
// time it is asked for, never cached, so a change made by the command line
// while the server runs is seen by the server at once.
type Fence struct {
	Fenced bool
	// Since is when the fence was raised, in milliseconds since the Unix epoch.
	Since int64
	// MovedTo is the address the controller moved to, when the owner said.
	MovedTo string
}

// Fence reads the current state.
func (s *Store) Fence() (Fence, error) {
	since, err := s.FenceSince()
	if err != nil || since == 0 {
		return Fence{}, err
	}
	to, err := s.GetSetting(SettingFenceMovedTo)
	if err != nil {
		return Fence{}, err
	}
	return Fence{Fenced: true, Since: since, MovedTo: to}, nil
}

// Fenced reports whether this controller is fenced. When the state cannot be
// read it says yes: a controller that cannot tell whether it moved must not
// act as though it had not.
func (s *Store) Fenced() bool {
	f, err := s.Fence()
	return err != nil || f.Fenced
}

// SetFence raises the fence. When one is already raised it changes nothing and
// returns the existing state, so a second call cannot move the date or the
// address.
func (s *Store) SetFence(movedTo string) (Fence, error) {
	err := s.withTx(func(tx *sql.Tx) error {
		var have string
		err := tx.QueryRow(`SELECT value FROM settings WHERE key=?`, SettingFenceSince).Scan(&have)
		if err == nil && have != "" && have != "0" {
			return nil
		}
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			SettingFenceSince, strconv.FormatInt(time.Now().UnixMilli(), 10)); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			SettingFenceMovedTo, movedTo)
		return err
	})
	if err != nil {
		return Fence{}, err
	}
	return s.Fence()
}

// LiftFence removes the fence. Lifting one that is not raised changes nothing.
func (s *Store) LiftFence() (Fence, error) {
	_, err := s.db.Exec(`DELETE FROM settings WHERE key IN(?,?)`, SettingFenceSince, SettingFenceMovedTo)
	return Fence{}, err
}

// ValidMovedTo accepts an http or https address without credentials, or
// nothing at all, and returns it trimmed.
func ValidMovedTo(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errors.New("movedTo must be an http or https address without credentials")
	}
	return raw, nil
}
