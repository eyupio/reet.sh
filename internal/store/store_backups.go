package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
)

// BackupRemote is one off-site destination. OwnerID 0 is the instance's own
// destination; a user's is theirs alone. SecretSealed is the sealed access
// key, never returned to a client.
type BackupRemote struct {
	ID           int64  `json:"id"`
	OwnerID      int64  `json:"-"`
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	Bucket       string `json:"bucket"`
	Prefix       string `json:"prefix"`
	AccessKey    string `json:"accessKey"`
	SecretSealed string `json:"-"`
	PathStyle    int    `json:"pathStyle"`
	Keep         int    `json:"keep"`
	Disabled     bool   `json:"disabled"`
	Uploading    bool   `json:"uploading"`
	LastUploadAt int64  `json:"lastUploadAt,omitempty"`
	LastError    string `json:"lastError,omitempty"`
	LastErrorAt  int64  `json:"lastErrorAt,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

// BackupSettings is one owner's schedule, retention and where copies go.
// NextAt is when the scheduler may take the next copy; 0 means manual only.
// Destination is "local" (this machine only), "remote" (an enabled
// destination only, kept here only when none took it) or "both".
type BackupSettings struct {
	OwnerID      int64  `json:"-"`
	EverySeconds int64  `json:"everySeconds"`
	Keep         int    `json:"keep"`
	Destination  string `json:"destination"`
	NextAt       int64  `json:"nextAt"`
	LastAt       int64  `json:"lastAt"`
	LastState    string `json:"lastState"`
	LastError    string `json:"lastError"`
	UpdatedAt    int64  `json:"updatedAt"`
}

// Backup destinations. "both" is the behaviour every instance had before the
// choice existed, and what an unset or unknown value falls back to, so an
// upgrade keeps copying both ways.
const (
	DestinationBoth   = "both"
	DestinationLocal  = "local"
	DestinationRemote = "remote"
)

// ValidBackupDestination reports whether destination is one of the three.
func ValidBackupDestination(destination string) bool {
	switch destination {
	case DestinationBoth, DestinationLocal, DestinationRemote:
		return true
	}
	return false
}

// BackupRun is one attempt to take a copy, kept for the pages that show what
// happened and when.
type BackupRun struct {
	ID         int64  `json:"id"`
	OwnerID    int64  `json:"-"`
	Scope      string `json:"scope"`
	Trigger    string `json:"trigger"`
	State      string `json:"state"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"`
	Bytes      int64  `json:"bytes"`
	Object     string `json:"object"`
	Offsite    string `json:"offsite"`
	Error      string `json:"error"`
}

// Profile is the part of a person's account the UI reads back.
type Profile struct {
	Theme      string `json:"theme"`
	Timezone   string `json:"timezone"`
	DateFormat string `json:"dateFormat"`
	UpdatedAt  int64  `json:"updatedAt"`
}

// SessionInfo is one signed-in browser.
type SessionInfo struct {
	ID        string `json:"id"`
	UserAgent string `json:"userAgent"`
	CreatedAt int64  `json:"createdAt"`
	LastSeen  int64  `json:"lastSeen"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (s *Store) migrateBackups() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS backup_remotes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			endpoint TEXT NOT NULL DEFAULT '',
			region TEXT NOT NULL DEFAULT '',
			bucket TEXT NOT NULL DEFAULT '',
			prefix TEXT NOT NULL DEFAULT '',
			access_key TEXT NOT NULL DEFAULT '',
			secret_sealed TEXT NOT NULL DEFAULT '',
			path_style INTEGER NOT NULL DEFAULT -1,
			keep INTEGER NOT NULL DEFAULT 0,
			disabled INTEGER NOT NULL DEFAULT 0,
			uploading INTEGER NOT NULL DEFAULT 0,
			last_upload_at INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			last_error_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(owner_id, name))`,
		`CREATE TABLE IF NOT EXISTS backup_settings (
			owner_id INTEGER PRIMARY KEY,
			every_seconds INTEGER NOT NULL DEFAULT 0,
			keep INTEGER NOT NULL DEFAULT 0,
			destination TEXT NOT NULL DEFAULT 'both',
			next_at INTEGER NOT NULL DEFAULT 0,
			last_at INTEGER NOT NULL DEFAULT 0,
			last_state TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS backup_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_id INTEGER NOT NULL DEFAULT 0,
			scope TEXT NOT NULL DEFAULT 'instance',
			trigger TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT '',
			started_at INTEGER NOT NULL,
			finished_at INTEGER NOT NULL DEFAULT 0,
			bytes INTEGER NOT NULL DEFAULT 0,
			object TEXT NOT NULL DEFAULT '',
			offsite TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS idx_backup_runs_owner ON backup_runs(owner_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_backup_settings_due ON backup_settings(next_at)`,
		`CREATE TABLE IF NOT EXISTS user_profiles (
			user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			theme TEXT NOT NULL DEFAULT 'system',
			timezone TEXT NOT NULL DEFAULT '',
			date_format TEXT NOT NULL DEFAULT 'locale',
			updated_at INTEGER NOT NULL DEFAULT 0)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	for _, c := range []struct{ table, name, ddl string }{
		{"backup_remotes", "uploading", "uploading INTEGER NOT NULL DEFAULT 0"},
		{"backup_remotes", "last_upload_at", "last_upload_at INTEGER NOT NULL DEFAULT 0"},
		{"backup_remotes", "last_error", "last_error TEXT NOT NULL DEFAULT ''"},
		{"backup_remotes", "last_error_at", "last_error_at INTEGER NOT NULL DEFAULT 0"},
		{"backup_settings", "destination", "destination TEXT NOT NULL DEFAULT 'both'"},
	} {
		ok, err := hasColumn(s.db, c.table, c.name)
		if err != nil {
			return err
		}
		if !ok {
			if _, err := s.db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

// EnsureBackupSettings gives an owner the defaults once, and never overwrites
// a choice already made.
func (s *Store) EnsureBackupSettings(ownerID, every int64, keep int, destination string) (BackupSettings, error) {
	next := int64(0)
	if every > 0 {
		next = now() + every*1000
	}
	if !ValidBackupDestination(destination) {
		destination = DestinationBoth
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO backup_settings(owner_id,every_seconds,keep,destination,next_at,updated_at)
		VALUES(?,?,?,?,?,?)`, ownerID, every, keep, destination, next, now()); err != nil {
		return BackupSettings{}, err
	}
	return s.BackupSettingsFor(ownerID)
}

// SetBackupRemoteUploading marks a destination as sending, or done sending.
func (s *Store) SetBackupRemoteUploading(ownerID int64, name string, uploading bool) error {
	_, err := s.db.Exec(`UPDATE backup_remotes SET uploading=? WHERE owner_id=? AND name=?`,
		b2i(&uploading), ownerID, name)
	return err
}

// SetBackupRemoteResult records what became of the last copy to a destination.
func (s *Store) SetBackupRemoteResult(ownerID int64, name string, at int64, errText string) error {
	errAt := int64(0)
	if errText != "" {
		errAt = at
	}
	_, err := s.db.Exec(`UPDATE backup_remotes SET uploading=0,last_upload_at=?,last_error=?,last_error_at=? WHERE owner_id=? AND name=?`,
		at, errText, errAt, ownerID, name)
	return err
}

// BackupSettingsFor reads one owner's schedule, creating the default row the
// first time so a page always has figures to draw.
func (s *Store) BackupSettingsFor(ownerID int64) (BackupSettings, error) {
	var b BackupSettings
	err := s.db.QueryRow(`SELECT owner_id,every_seconds,keep,destination,next_at,last_at,last_state,last_error,updated_at
		FROM backup_settings WHERE owner_id=?`, ownerID).
		Scan(&b.OwnerID, &b.EverySeconds, &b.Keep, &b.Destination, &b.NextAt, &b.LastAt, &b.LastState, &b.LastError, &b.UpdatedAt)
	if err == nil {
		if !ValidBackupDestination(b.Destination) {
			b.Destination = DestinationBoth
		}
		return b, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return b, err
	}
	b = BackupSettings{OwnerID: ownerID, Destination: DestinationBoth}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO backup_settings(owner_id,destination,updated_at) VALUES(?,?,?)`, ownerID, b.Destination, now()); err != nil {
		return b, err
	}
	return b, nil
}

// SaveBackupSettingsFor stores a schedule, retention and where copies go,
// leaving the run cursor to the scheduler unless it was set explicitly.
func (s *Store) SaveBackupSettingsFor(b BackupSettings) error {
	if !ValidBackupDestination(b.Destination) {
		b.Destination = DestinationBoth
	}
	_, err := s.db.Exec(`INSERT INTO backup_settings(owner_id,every_seconds,keep,destination,next_at,last_at,last_state,last_error,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(owner_id) DO UPDATE SET
			every_seconds=excluded.every_seconds, keep=excluded.keep, destination=excluded.destination,
			next_at=excluded.next_at,
			last_at=excluded.last_at, last_state=excluded.last_state, last_error=excluded.last_error,
			updated_at=excluded.updated_at`,
		b.OwnerID, b.EverySeconds, b.Keep, b.Destination, b.NextAt, b.LastAt, b.LastState, b.LastError, now())
	return err
}

// FinishBackup records the outcome and moves the next scheduled copy on.
func (s *Store) FinishBackup(ownerID, at, nextAt int64, state, errText string) error {
	_, err := s.db.Exec(`INSERT INTO backup_settings(owner_id,next_at,last_at,last_state,last_error,updated_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(owner_id) DO UPDATE SET next_at=excluded.next_at, last_at=excluded.last_at,
			last_state=excluded.last_state, last_error=excluded.last_error, updated_at=excluded.updated_at`,
		ownerID, nextAt, at, state, errText, now())
	return err
}

// BackupDueUsers lists user owners whose next copy is due, never the instance
// (owner 0), which the service schedules itself.
func (s *Store) BackupDueUsers(at int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT owner_id FROM backup_settings
		WHERE owner_id<>0 AND every_seconds>0 AND next_at>0 AND next_at<=?`, at)
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

func (s *Store) AddBackupRun(r BackupRun) (BackupRun, error) {
	if r.StartedAt == 0 {
		r.StartedAt = now()
	}
	res, err := s.db.Exec(`INSERT INTO backup_runs(owner_id,scope,trigger,state,started_at,finished_at,bytes,object,offsite,error)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		r.OwnerID, r.Scope, r.Trigger, r.State, r.StartedAt, r.FinishedAt, r.Bytes, r.Object, r.Offsite, r.Error)
	if err != nil {
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	return r, nil
}

func (s *Store) ListBackupRuns(ownerID int64, limit int) ([]BackupRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,owner_id,scope,trigger,state,started_at,finished_at,bytes,object,offsite,error
		FROM backup_runs WHERE owner_id=? ORDER BY started_at DESC, id DESC LIMIT ?`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupRun{}
	for rows.Next() {
		var r BackupRun
		if err := rows.Scan(&r.ID, &r.OwnerID, &r.Scope, &r.Trigger, &r.State, &r.StartedAt, &r.FinishedAt,
			&r.Bytes, &r.Object, &r.Offsite, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) PruneBackupRuns(ownerID int64, keep int) error {
	if keep <= 0 {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM backup_runs WHERE owner_id=? AND id NOT IN (
		SELECT id FROM backup_runs WHERE owner_id=? ORDER BY started_at DESC, id DESC LIMIT ?)`,
		ownerID, ownerID, keep)
	return err
}

func scanBackupRemote(row interface{ Scan(...any) error }) (BackupRemote, error) {
	var r BackupRemote
	var uploading int
	err := row.Scan(&r.ID, &r.OwnerID, &r.Name, &r.Endpoint, &r.Region, &r.Bucket, &r.Prefix,
		&r.AccessKey, &r.SecretSealed, &r.PathStyle, &r.Keep, &r.Disabled, &uploading,
		&r.LastUploadAt, &r.LastError, &r.LastErrorAt, &r.CreatedAt, &r.UpdatedAt)
	r.Uploading = uploading != 0
	return r, err
}

func (s *Store) BackupRemotes(ownerID int64) ([]BackupRemote, error) {
	rows, err := s.db.Query(`SELECT id,owner_id,name,endpoint,region,bucket,prefix,access_key,secret_sealed,
		path_style,keep,disabled,uploading,last_upload_at,last_error,last_error_at,created_at,updated_at
		FROM backup_remotes WHERE owner_id=? ORDER BY lower(name)`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupRemote{}
	for rows.Next() {
		r, err := scanBackupRemote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) BackupRemoteByName(ownerID int64, name string) (BackupRemote, bool, error) {
	r, err := scanBackupRemote(s.db.QueryRow(`SELECT id,owner_id,name,endpoint,region,bucket,prefix,access_key,secret_sealed,
		path_style,keep,disabled,uploading,last_upload_at,last_error,last_error_at,created_at,updated_at
		FROM backup_remotes WHERE owner_id=? AND name=?`,
		ownerID, strings.ToLower(strings.TrimSpace(name))))
	if errors.Is(err, sql.ErrNoRows) {
		return BackupRemote{}, false, nil
	}
	return r, err == nil, err
}

func (s *Store) CreateBackupRemote(r BackupRemote) (BackupRemote, error) {
	t := now()
	res, err := s.db.Exec(`INSERT INTO backup_remotes(owner_id,name,endpoint,region,bucket,prefix,access_key,secret_sealed,
		path_style,keep,disabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.OwnerID, r.Name, r.Endpoint, r.Region, r.Bucket, r.Prefix, r.AccessKey, r.SecretSealed,
		r.PathStyle, r.Keep, b2i(&r.Disabled), t, t)
	if err != nil {
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	r.CreatedAt, r.UpdatedAt = t, t
	return r, nil
}

func (s *Store) UpdateBackupRemote(r BackupRemote) error {
	res, err := s.db.Exec(`UPDATE backup_remotes SET endpoint=?,region=?,bucket=?,prefix=?,access_key=?,secret_sealed=?,
		path_style=?,keep=?,disabled=?,updated_at=? WHERE owner_id=? AND name=?`,
		r.Endpoint, r.Region, r.Bucket, r.Prefix, r.AccessKey, r.SecretSealed, r.PathStyle, r.Keep,
		b2i(&r.Disabled), now(), r.OwnerID, r.Name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteBackupRemote(ownerID int64, name string) error {
	res, err := s.db.Exec(`DELETE FROM backup_remotes WHERE owner_id=? AND name=?`, ownerID, strings.ToLower(strings.TrimSpace(name)))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ProfileFor reads a person's display preferences, giving defaults the first
// time so the page has something to show.
func (s *Store) ProfileFor(userID int64) (Profile, error) {
	var p Profile
	err := s.db.QueryRow(`SELECT theme,timezone,date_format,updated_at FROM user_profiles WHERE user_id=?`, userID).
		Scan(&p.Theme, &p.Timezone, &p.DateFormat, &p.UpdatedAt)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return p, err
	}
	p = Profile{Theme: "system", DateFormat: "locale"}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO user_profiles(user_id,theme,timezone,date_format,updated_at) VALUES(?,?,?,?,?)`,
		userID, p.Theme, p.Timezone, p.DateFormat, now()); err != nil {
		return p, err
	}
	return p, nil
}

func (s *Store) SaveProfileFor(userID int64, p Profile) error {
	_, err := s.db.Exec(`INSERT INTO user_profiles(user_id,theme,timezone,date_format,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET theme=excluded.theme, timezone=excluded.timezone,
			date_format=excluded.date_format, updated_at=excluded.updated_at`,
		userID, p.Theme, p.Timezone, p.DateFormat, now())
	return err
}

func (s *Store) UpdateUserName(id int64, name string) error {
	res, err := s.db.Exec(`UPDATE users SET name=? WHERE id=?`, strings.TrimSpace(name), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SessionsFor lists the browsers signed in to one account, newest first.
func (s *Store) SessionsFor(userID int64) ([]SessionInfo, error) {
	rows, err := s.db.Query(`SELECT id,ua,created_at,last_seen,expires_at FROM sessions
		WHERE user_id=? ORDER BY last_seen DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		var x SessionInfo
		if err := rows.Scan(&x.ID, &x.UserAgent, &x.CreatedAt, &x.LastSeen, &x.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// DeleteSessionFor ends one of a person's sessions, never somebody else's.
func (s *Store) DeleteSessionFor(userID int64, id string) error {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteOtherSessions ends everything but the browser making the change.
func (s *Store) DeleteOtherSessions(userID int64, keep string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE user_id=? AND id<>?`, userID, keep)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetUserPasswordKeep changes the hash without ending sessions; the caller
// ends the others once it has a fresh session for this browser.
func (s *Store) SetUserPasswordKeep(id int64, hash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// VacuumInto writes a consistent copy of the live database to dest. It uses
// SQLite's own VACUUM INTO, which includes committed WAL pages and never
// copies a half-written file.
func (s *Store) VacuumInto(ctx context.Context, dest string) error {
	u := url.URL{Scheme: "file", Path: filepath.Join(s.dir, "reet.db")}
	q := url.Values{"mode": {"ro"}, "_pragma": {"busy_timeout(3000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		return err
	}
	return nil
}

// CheckDatabaseFile runs SQLite's integrity check against a file.
func CheckDatabaseFile(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("SQLite integrity check failed")
	}
	return nil
}

// FinishBackupRun writes the outcome of one attempt.
func (s *Store) FinishBackupRun(id int64, state string, finishedAt, bytes int64, object, offsite, errText string) error {
	_, err := s.db.Exec(`UPDATE backup_runs SET state=?,finished_at=?,bytes=?,object=?,offsite=?,error=? WHERE id=?`,
		state, finishedAt, bytes, object, offsite, errText, id)
	return err
}
