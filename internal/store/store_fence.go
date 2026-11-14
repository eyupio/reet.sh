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
// address; SetFenceAddress changes the address on purpose.
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
		// What machines said of an earlier move is not about this one.
		if _, err := tx.Exec(`DELETE FROM fence_machines`); err != nil {
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
	err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM settings WHERE key IN(?,?)`, SettingFenceSince, SettingFenceMovedTo); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM fence_machines`)
		return err
	})
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

// fenceMachinesTable is what a fenced controller hears from each machine while
// the move is under way: when the fence last turned the machine away, and when
// the machine said it had gone. It describes this controller's move and no
// other, so it is emptied when a fence is raised or lifted and when a copy is
// restored.
const fenceMachinesTable = `CREATE TABLE IF NOT EXISTS fence_machines (
	agent_id TEXT PRIMARY KEY REFERENCES sync_agents(id) ON DELETE CASCADE,
	told_at INTEGER NOT NULL DEFAULT 0, left_at INTEGER NOT NULL DEFAULT 0,
	left_to TEXT NOT NULL DEFAULT '', held TEXT NOT NULL DEFAULT '')`

// ErrNotFenced is returned by what only makes sense while the fence is up.
var ErrNotFenced = errors.New("this controller is not fenced")

// ErrUnknownMachine is a machine id and secret that match no enrollment.
var ErrUnknownMachine = errors.New("machine enrollment is missing or revoked")

// SetFenceAddress changes where a raised fence says the controller moved to,
// keeping the date. An owner who fenced before the new address was known, or
// typed it wrong, says so here without lifting the fence, which would set the
// controller's background work going again.
func (s *Store) SetFenceAddress(movedTo string) (Fence, error) {
	err := s.withTx(func(tx *sql.Tx) error {
		var have string
		err := tx.QueryRow(`SELECT value FROM settings WHERE key=?`, SettingFenceSince).Scan(&have)
		if err == sql.ErrNoRows || (err == nil && (have == "" || have == "0")) {
			return ErrNotFenced
		}
		if err != nil {
			return err
		}
		var was string
		if err := tx.QueryRow(`SELECT value FROM settings WHERE key=?`, SettingFenceMovedTo).Scan(&was); err != nil && err != sql.ErrNoRows {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			SettingFenceMovedTo, movedTo); err != nil {
			return err
		}
		if was == movedTo {
			return nil
		}
		// A machine that refused the old address has not been asked about
		// this one; one with following turned off still will not follow.
		_, err = tx.Exec(`UPDATE fence_machines SET held='' WHERE held='address-refused'`)
		return err
	})
	if err != nil {
		return Fence{}, err
	}
	return s.Fence()
}

// NoteFenceTold records that the fence turned a machine away, and so told it
// where the controller went. Only a machine that names its own secret counts.
func (s *Store) NoteFenceTold(id, secret string) error {
	if id == "" || secret == "" || len(id) > 128 || len(secret) > 256 {
		return ErrUnknownMachine
	}
	res, err := s.db.Exec(`INSERT INTO fence_machines(agent_id,told_at) SELECT id,? FROM sync_agents WHERE id=? AND secret_hash=?
		ON CONFLICT(agent_id) DO UPDATE SET told_at=excluded.told_at`, now(), id, secretHash(secret))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnknownMachine
	}
	return nil
}

// NoteFenceLeft records what a machine said of the move: that it now points at
// the controller at `to` (held empty), or that it will not follow on its own
// (held names why). Only a machine that names its own secret counts.
func (s *Store) NoteFenceLeft(id, secret, to, held string) error {
	if id == "" || secret == "" || len(id) > 128 || len(secret) > 256 {
		return ErrUnknownMachine
	}
	at := now()
	leftAt := int64(0)
	if held == "" {
		leftAt = at
	}
	res, err := s.db.Exec(`INSERT INTO fence_machines(agent_id,told_at,left_at,left_to,held) SELECT id,?,?,?,? FROM sync_agents WHERE id=? AND secret_hash=?
		ON CONFLICT(agent_id) DO UPDATE SET told_at=excluded.told_at,left_at=excluded.left_at,left_to=excluded.left_to,held=excluded.held`,
		at, leftAt, to, held, id, secretHash(secret))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnknownMachine
	}
	return nil
}

// FenceMachine is one enrolled machine as the move sees it.
type FenceMachine struct {
	ID       string `json:"id"`
	Host     string `json:"host"`
	Owner    string `json:"owner"`
	LastSeen int64  `json:"lastSeen"`
	// ToldAt is when the fence last turned this machine away; 0 when it has
	// not asked since the fence went up.
	ToldAt int64 `json:"toldAt"`
	// LeftAt is when the machine said it now points at LeftTo.
	LeftAt int64  `json:"leftAt"`
	LeftTo string `json:"leftTo"`
	// Held says why the machine will not follow on its own, when it said so.
	Held string `json:"held"`
}

// FenceMachines lists every enrolled machine on this controller with what it
// has said since the fence went up, those still to move first.
func (s *Store) FenceMachines() ([]FenceMachine, error) {
	rows, err := s.db.Query(`SELECT a.id,a.host,COALESCE(NULLIF(u.email,''),u.name,''),a.last_seen,
		COALESCE(f.told_at,0),COALESCE(f.left_at,0),COALESCE(f.left_to,''),COALESCE(f.held,'')
		FROM sync_agents a JOIN users u ON u.id=a.user_id LEFT JOIN fence_machines f ON f.agent_id=a.id
		ORDER BY COALESCE(f.left_at,0)>0, a.host COLLATE NOCASE, a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FenceMachine{}
	for rows.Next() {
		var m FenceMachine
		if err := rows.Scan(&m.ID, &m.Host, &m.Owner, &m.LastSeen, &m.ToldAt, &m.LeftAt, &m.LeftTo, &m.Held); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
