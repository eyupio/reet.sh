package store

import (
	"database/sql"
	"errors"
	"strings"
)

// A push subscription is one browser on one device that asked to be woken
// when a session needs its person. Only the push service's address is kept:
// wake-ups carry no payload, so there are no keys to store.

// MaxPushSubscriptions is how many devices one person may register.
const MaxPushSubscriptions = 12

// ErrPushLimit means the person already has MaxPushSubscriptions devices.
var ErrPushLimit = errors.New("too many devices are registered for notifications")

// PushSubscription is one registered browser.
type PushSubscription struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Endpoint  string `json:"-"`
	Label     string `json:"label"`
	Waiting   bool   `json:"waiting"`
	Ended     bool   `json:"ended"`
	CreatedAt int64  `json:"createdAt"`
	LastOKAt  int64  `json:"lastOkAt"`
}

func (s *Store) migratePush() error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS push_subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			endpoint TEXT NOT NULL UNIQUE,
			label TEXT NOT NULL DEFAULT '',
			on_waiting INTEGER NOT NULL DEFAULT 1 CHECK(on_waiting IN (0,1)),
			on_ended INTEGER NOT NULL DEFAULT 1 CHECK(on_ended IN (0,1)),
			created_at INTEGER NOT NULL,
			last_ok_at INTEGER NOT NULL DEFAULT 0)`,
		`CREATE INDEX IF NOT EXISTS push_subscriptions_user ON push_subscriptions(user_id)`,
		// What each person was last told about each session, so one moment is told once, across restarts.
		`CREATE TABLE IF NOT EXISTS push_told (
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			session_id TEXT NOT NULL REFERENCES terminal_sessions(id) ON DELETE CASCADE,
			marker TEXT NOT NULL,
			PRIMARY KEY(user_id, session_id))`,
	} {
		if _, err := s.db.Exec(ddl); err != nil {
			return err
		}
	}
	return nil
}

// SavePushSubscription registers endpoint for uid, or updates its label and
// choices if this browser is already registered. A browser that signs in as
// somebody else moves to them: one device tells one person.
func (s *Store) SavePushSubscription(uid int64, endpoint, label string, waiting, ended bool) (PushSubscription, error) {
	endpoint = strings.TrimSpace(endpoint)
	if uid <= 0 || endpoint == "" {
		return PushSubscription{}, errors.New("a subscription needs a person and an endpoint")
	}
	if len(label) > 80 {
		label = label[:80]
	}
	var out PushSubscription
	err := s.withTx(func(tx *sql.Tx) error {
		var existing int64
		err := tx.QueryRow(`SELECT id FROM push_subscriptions WHERE endpoint=?`, endpoint).Scan(&existing)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if errors.Is(err, sql.ErrNoRows) {
			var count int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM push_subscriptions WHERE user_id=?`, uid).Scan(&count); err != nil {
				return err
			}
			if count >= MaxPushSubscriptions {
				return ErrPushLimit
			}
			res, err := tx.Exec(`INSERT INTO push_subscriptions(user_id,endpoint,label,on_waiting,on_ended,created_at) VALUES(?,?,?,?,?,?)`,
				uid, endpoint, label, waiting, ended, now())
			if err != nil {
				return err
			}
			existing, err = res.LastInsertId()
			if err != nil {
				return err
			}
		} else if _, err := tx.Exec(`UPDATE push_subscriptions SET user_id=?,label=?,on_waiting=?,on_ended=? WHERE id=?`,
			uid, label, waiting, ended, existing); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT id,user_id,endpoint,label,on_waiting,on_ended,created_at,last_ok_at FROM push_subscriptions WHERE id=?`, existing).
			Scan(&out.ID, &out.UserID, &out.Endpoint, &out.Label, &out.Waiting, &out.Ended, &out.CreatedAt, &out.LastOKAt)
	})
	return out, err
}

// PushSubscriptionsFor lists uid's registered browsers, oldest first.
func (s *Store) PushSubscriptionsFor(uid int64) ([]PushSubscription, error) {
	rows, err := s.db.Query(`SELECT id,user_id,endpoint,label,on_waiting,on_ended,created_at,last_ok_at FROM push_subscriptions WHERE user_id=? ORDER BY id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushSubscription{}
	for rows.Next() {
		var p PushSubscription
		if err := rows.Scan(&p.ID, &p.UserID, &p.Endpoint, &p.Label, &p.Waiting, &p.Ended, &p.CreatedAt, &p.LastOKAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PushUsers lists the people who have at least one registered browser and
// can still use sessions.
func (s *Store) PushUsers() ([]int64, error) {
	rows, err := s.db.Query(`SELECT DISTINCT p.user_id FROM push_subscriptions p JOIN users u ON u.id=p.user_id WHERE u.disabled_at=0 AND u.role<>'viewer' ORDER BY p.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeletePushSubscription forgets one of uid's browsers, by its endpoint or
// its id. It reports whether there was one.
func (s *Store) DeletePushSubscription(uid int64, endpoint string, id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM push_subscriptions WHERE user_id=? AND (endpoint=? OR id=?)`, uid, endpoint, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DropPushEndpoint forgets a subscription the push service no longer knows.
func (s *Store) DropPushEndpoint(endpoint string) error {
	_, err := s.db.Exec(`DELETE FROM push_subscriptions WHERE endpoint=?`, endpoint)
	return err
}

// MarkPushDelivered records that the push service accepted a wake-up.
func (s *Store) MarkPushDelivered(endpoint string) error {
	_, err := s.db.Exec(`UPDATE push_subscriptions SET last_ok_at=? WHERE endpoint=?`, now(), endpoint)
	return err
}

// PushTold returns what uid was last told about each session.
func (s *Store) PushTold(uid int64) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT session_id,marker FROM push_told WHERE user_id=?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, marker string
		if err := rows.Scan(&id, &marker); err != nil {
			return nil, err
		}
		out[id] = marker
	}
	return out, rows.Err()
}

// SetPushTold records the moment uid has now been told about for sessionID.
func (s *Store) SetPushTold(uid int64, sessionID, marker string) error {
	_, err := s.db.Exec(`INSERT INTO push_told(user_id,session_id,marker) VALUES(?,?,?)
		ON CONFLICT(user_id,session_id) DO UPDATE SET marker=excluded.marker`, uid, sessionID, marker)
	return err
}
