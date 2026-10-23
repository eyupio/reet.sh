package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Notifications are what a person is told in the centre, and the choices that
// decide whether and how each kind reaches them. The catalogue and the
// delivery rules are internal/notify's; this is the rows behind them.

const (
	NotifyImmediate = "immediate"
	NotifyNone      = "none"
)

// Notification is one row in a person's notification centre.
type Notification struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Kind      string `json:"kind"`
	TeamID    int64  `json:"teamId,omitempty"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Link      string `json:"link,omitempty"`
	Tone      string `json:"tone,omitempty"`
	Key       string `json:"-"`
	ReadAt    int64  `json:"readAt,omitempty"`
	EmailedAt int64  `json:"emailedAt,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// NotifyPref is one person's choice about one kind.
type NotifyPref struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
}

// NotifyDefault is the owner's default for one kind, and for a staff kind
// which staff roles receive it.
type NotifyDefault struct {
	Enabled   bool   `json:"enabled"`
	Mode      string `json:"mode"`
	Owner     bool   `json:"owner"`
	Admin     bool   `json:"admin"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
	UpdatedBy string `json:"updatedBy,omitempty"`
}

func (s *Store) migrateNotify() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			kind TEXT NOT NULL,
			team_id INTEGER REFERENCES audience_groups(id) ON DELETE SET NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			link TEXT NOT NULL DEFAULT '',
			tone TEXT NOT NULL DEFAULT '',
			idem_key TEXT NOT NULL,
			read_at INTEGER NOT NULL DEFAULT 0,
			emailed_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(user_id, idem_key))`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_unread ON notifications(user_id, read_at)`,
		`CREATE TABLE IF NOT EXISTS notify_prefs (
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			kind TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			mode TEXT NOT NULL DEFAULT 'immediate',
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(user_id, kind))`,
		`CREATE TABLE IF NOT EXISTS notify_platform_defaults (
			kind TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 1,
			mode TEXT NOT NULL DEFAULT 'immediate',
			owner INTEGER NOT NULL DEFAULT 1,
			admin INTEGER NOT NULL DEFAULT 1,
			updated_at INTEGER NOT NULL,
			updated_by TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// RecordNotification records one notification for one person unless its key
// is already there, in which case the existing row is returned and created is
// false. The key is what makes an event reported twice tell nobody twice.
func (s *Store) RecordNotification(n Notification) (Notification, bool, error) {
	if n.UserID == 0 || n.Kind == "" || n.Key == "" || strings.TrimSpace(n.Title) == "" {
		return Notification{}, false, fmt.Errorf("a notification needs a person, a kind, a key and a title")
	}
	now := now()
	res, err := s.db.Exec(`INSERT INTO notifications(user_id,kind,team_id,title,body,link,tone,idem_key,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id, idem_key) DO NOTHING`,
		n.UserID, n.Kind, nullableID(n.TeamID), strings.TrimSpace(n.Title), n.Body, n.Link, n.Tone, n.Key, now, now)
	if err != nil {
		return Notification{}, false, err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		got, err := s.NotificationByKey(n.UserID, n.Key)
		return got, false, err
	}
	id, _ := res.LastInsertId()
	got, err := s.NotificationByID(id)
	return got, err == nil, err
}

// NotificationByID reads one notification.
func (s *Store) NotificationByID(id int64) (Notification, error) {
	n, err := scanNotification(s.db.QueryRow(`SELECT id,user_id,kind,COALESCE(team_id,0),title,body,link,tone,idem_key,read_at,emailed_at,created_at,updated_at
		FROM notifications WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Notification{}, ErrNotFound
	}
	return n, err
}

// NotificationByKey reads one person's notification by its event key.
func (s *Store) NotificationByKey(userID int64, key string) (Notification, error) {
	n, err := scanNotification(s.db.QueryRow(`SELECT id,user_id,kind,COALESCE(team_id,0),title,body,link,tone,idem_key,read_at,emailed_at,created_at,updated_at
		FROM notifications WHERE user_id=? AND idem_key=?`, userID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return Notification{}, ErrNotFound
	}
	return n, err
}

func scanNotification(row interface{ Scan(...any) error }) (Notification, error) {
	var n Notification
	if err := row.Scan(&n.ID, &n.UserID, &n.Kind, &n.TeamID, &n.Title, &n.Body, &n.Link, &n.Tone, &n.Key, &n.ReadAt, &n.EmailedAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return Notification{}, err
	}
	return n, nil
}

// NotificationsFor lists a person's notifications, most recently changed
// first. Unread keeps only the ones they have not read.
func (s *Store) NotificationsFor(userID int64, unread bool, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id,user_id,kind,COALESCE(team_id,0),title,body,link,tone,idem_key,read_at,emailed_at,created_at,updated_at
		FROM notifications WHERE user_id=?`
	if unread {
		q += ` AND read_at=0`
	}
	rows, err := s.db.Query(q+` ORDER BY updated_at DESC,id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnreadNotifications counts the rows a person has not read.
func (s *Store) UnreadNotifications(userID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id=? AND read_at=0`, userID).Scan(&n)
	return n, err
}

// MarkNotificationsRead marks some of a person's notifications read, or all
// of them when ids is empty. Another person's ids are silently not theirs: the
// WHERE names the person, so nobody can mark somebody else's.
func (s *Store) MarkNotificationsRead(userID int64, ids []int64, read bool) (int, error) {
	var at int64
	if read {
		at = now()
	}
	q := `UPDATE notifications SET read_at=? WHERE user_id=?`
	args := []any{at, userID}
	if len(ids) > 0 {
		q += ` AND id IN (` + placeholders(len(ids)) + `)`
		for _, id := range ids {
			args = append(args, id)
		}
	} else if read {
		q += ` AND read_at=0`
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SetNotificationEmailed records that an email has said this notification.
func (s *Store) SetNotificationEmailed(id int64) error {
	_, err := s.db.Exec(`UPDATE notifications SET emailed_at=? WHERE id=?`, now(), id)
	return err
}

// PruneNotifications drops read rows older than keep, so the centre stays
// small.
func (s *Store) PruneNotifications(keep time.Duration) (int, error) {
	cut := now() - int64(keep/time.Millisecond)
	res, err := s.db.Exec(`DELETE FROM notifications WHERE read_at<>0 AND updated_at<?`, cut)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// NotificationPrefs is a person's own choices, by kind.
func (s *Store) NotificationPrefs(userID int64) (map[string]NotifyPref, error) {
	rows, err := s.db.Query(`SELECT kind,enabled,mode FROM notify_prefs WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]NotifyPref{}
	for rows.Next() {
		var kind, mode string
		var enabled int
		if err := rows.Scan(&kind, &enabled, &mode); err != nil {
			return nil, err
		}
		out[kind] = NotifyPref{Enabled: enabled != 0, Mode: mode}
	}
	return out, rows.Err()
}

// SetNotificationPref records one person's choice for one kind.
func (s *Store) SetNotificationPref(userID int64, kind string, p NotifyPref) error {
	if userID == 0 || kind == "" || !validNotifyMode(p.Mode) {
		return errors.New("a notification preference needs a person, a kind and a mode of immediate or none")
	}
	enabled := 0
	if p.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`INSERT INTO notify_prefs(user_id,kind,enabled,mode,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(user_id,kind) DO UPDATE SET enabled=excluded.enabled,mode=excluded.mode,updated_at=excluded.updated_at`,
		userID, kind, enabled, p.Mode, now())
	return err
}

func validNotifyMode(mode string) bool { return mode == NotifyImmediate || mode == NotifyNone }

// PlatformNotifyDefaults is the owner's defaults, by kind.
func (s *Store) PlatformNotifyDefaults() (map[string]NotifyDefault, error) {
	rows, err := s.db.Query(`SELECT kind,enabled,mode,owner,admin,updated_at,updated_by FROM notify_platform_defaults`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]NotifyDefault{}
	for rows.Next() {
		var kind, mode string
		var enabled, owner, admin int
		var d NotifyDefault
		if err := rows.Scan(&kind, &enabled, &mode, &owner, &admin, &d.UpdatedAt, &d.UpdatedBy); err != nil {
			return nil, err
		}
		d.Enabled, d.Mode, d.Owner, d.Admin = enabled != 0, mode, owner != 0, admin != 0
		out[kind] = d
	}
	return out, rows.Err()
}

// SetPlatformNotifyDefault records the owner's default for one kind.
func (s *Store) SetPlatformNotifyDefault(kind string, d NotifyDefault, by string) error {
	if kind == "" || !validNotifyMode(d.Mode) {
		return errors.New("a platform default needs a kind and a mode of immediate or none")
	}
	enabled, owner, admin := 0, 0, 0
	if d.Enabled {
		enabled = 1
	}
	if d.Owner {
		owner = 1
	}
	if d.Admin {
		admin = 1
	}
	_, err := s.db.Exec(`INSERT INTO notify_platform_defaults(kind,enabled,mode,owner,admin,updated_at,updated_by) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(kind) DO UPDATE SET enabled=excluded.enabled,mode=excluded.mode,owner=excluded.owner,admin=excluded.admin,
			updated_at=excluded.updated_at,updated_by=excluded.updated_by`,
		kind, enabled, d.Mode, owner, admin, now(), by)
	return err
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
