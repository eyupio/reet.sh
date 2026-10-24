// Package store persists skills and their like/bookmark/used flags in SQLite.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/eyupio/reet.sh/internal/ref"
)

// ErrNotFound is returned when a skill id does not exist.
var ErrNotFound = errors.New("skill not found")

// ErrUserExists is returned when an email is already registered.
var ErrUserExists = errors.New("user already exists")

type Skill struct {
	ID           string        `json:"id"`
	Source       string        `json:"source"`
	Slug         string        `json:"slug"`
	SourceType   string        `json:"sourceType"`
	URL          string        `json:"url"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Notes        string        `json:"notes"`
	Installs     int64         `json:"installs"`
	InstallsWeek int64         `json:"installsWeek"`
	Trend        []int64       `json:"trend"`
	Liked        bool          `json:"liked"`
	Bookmarked   bool          `json:"bookmarked"`
	Used         bool          `json:"used"`
	CreatedAt    int64         `json:"createdAt"`
	UpdatedAt    int64         `json:"updatedAt"`
	Tags         []Tag         `json:"tags,omitempty"`
	HiddenTags   []Tag         `json:"hiddenTags,omitempty"`
	Environments []Environment `json:"environments,omitempty"`
	Lists        []ListRef     `json:"lists,omitempty"`
}

// ListRef is a shared list containing a skill, as shown on a card.
type ListRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Tag is a label on a skill. Source is "topic" or "user".
type Tag struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
	Count  int    `json:"count,omitempty"`
}

// Environment is one (host, agent[, project]) place a skill is installed.
type Environment struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Agent      string `json:"agent"`
	Host       string `json:"host"`
	Path       string `json:"path,omitempty"`
	Project    string `json:"project,omitempty"`
	FirstSeen  int64  `json:"firstSeen,omitempty"`
	LastSeen   int64  `json:"lastSeen"`
	SourcePath string `json:"sourcePath,omitempty"`
	SkillCount int    `json:"skillCount,omitempty"`
}

// Event is one local usage or audit row.
type Event struct {
	ID        int64  `json:"id"`
	At        int64  `json:"at"`
	Actor     string `json:"actor,omitempty"`
	Source    string `json:"source"`
	Action    string `json:"action"`
	SkillID   string `json:"skillId,omitempty"`
	SkillName string `json:"skillName,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// Ignored is a skill that scans and imports should skip.
type Ignored struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
	At     int64  `json:"at"`
}

// Snapshot is the export format. Version 1 backups have skills only.
type Snapshot struct {
	Version    int       `json:"version"`
	ExportedAt int64     `json:"exportedAt"`
	Skills     []Skill   `json:"skills"`
	Ignored    []Ignored `json:"ignored,omitempty"`
	Events     []Event   `json:"events,omitempty"`
}

// User is an account. PasswordHash is never written to JSON.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
	CreatedAt    int64  `json:"createdAt"`
	DisabledAt   int64  `json:"disabledAt,omitempty"`
}

// Patch holds optional changes; nil fields are left alone.
type Patch struct {
	Liked      *bool   `json:"liked,omitempty"`
	Bookmarked *bool   `json:"bookmarked,omitempty"`
	Used       *bool   `json:"used,omitempty"`
	Notes      *string `json:"notes,omitempty"`
}

// Filter selects and orders List results. Tab: all|liked|bookmarked|used. Sort: recent|name|installs.
type Filter struct {
	Tab, Query, Sort string
	Tags             []int64
	Env              string
	Machine          string
	Only             string
	Stage            string
	SourceType       string
	Untagged         bool
}

type Counts struct {
	All        int `json:"all"`
	Liked      int `json:"liked"`
	Bookmarked int `json:"bookmarked"`
	Used       int `json:"used"`
}

type Store struct {
	db  *sql.DB
	dir string
}

const cols = `id,source,slug,source_type,url,name,description,notes,installs,installs_week,trend,liked,bookmarked,used,created_at,updated_at`

const schema = `CREATE TABLE IF NOT EXISTS skills (
	id TEXT PRIMARY KEY, source TEXT NOT NULL DEFAULT '', slug TEXT NOT NULL DEFAULT '',
	source_type TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '',
	installs INTEGER NOT NULL DEFAULT 0, installs_week INTEGER NOT NULL DEFAULT 0, trend TEXT NOT NULL DEFAULT '',
	liked INTEGER NOT NULL DEFAULT 0, bookmarked INTEGER NOT NULL DEFAULT 0, used INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, fetched_at INTEGER NOT NULL DEFAULT 0)`

var schemaV12 = []string{
	`CREATE TABLE IF NOT EXISTS scan_errors (env_id TEXT PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE, error TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS sync_options (target_id TEXT PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE, mode TEXT NOT NULL DEFAULT 'mirror', baseline TEXT NOT NULL DEFAULT '[]', applied_revision TEXT NOT NULL DEFAULT '')`,
	`CREATE TABLE IF NOT EXISTS sync_agents (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, host TEXT NOT NULL, secret_hash TEXT NOT NULL, version TEXT NOT NULL, source_commit TEXT NOT NULL DEFAULT '', last_seen INTEGER NOT NULL, paused INTEGER NOT NULL DEFAULT 0, auto_update INTEGER NOT NULL DEFAULT 1, UNIQUE(user_id,host))`,
	`CREATE TABLE IF NOT EXISTS sync_jobs (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, agent_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE, target_id TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL, status TEXT NOT NULL, revision TEXT NOT NULL DEFAULT '', plan TEXT NOT NULL DEFAULT '{}', hash TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '', stage TEXT NOT NULL DEFAULT '', stages TEXT NOT NULL DEFAULT '[]', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS sync_receipts (job_id TEXT NOT NULL REFERENCES sync_jobs(id) ON DELETE CASCADE, status TEXT NOT NULL, hash TEXT NOT NULL, PRIMARY KEY(job_id,status,hash))`,
	`CREATE UNIQUE INDEX IF NOT EXISTS sync_agents_host ON sync_agents(user_id,host COLLATE NOCASE)`,
	`CREATE INDEX IF NOT EXISTS sync_jobs_agent ON sync_jobs(agent_id,status)`,
	`CREATE TABLE IF NOT EXISTS sync_runs (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, agent_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE, targets TEXT NOT NULL, cursor INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS sync_run_jobs (job_id TEXT PRIMARY KEY REFERENCES sync_jobs(id) ON DELETE CASCADE, run_id TEXT NOT NULL REFERENCES sync_runs(id) ON DELETE CASCADE, position INTEGER NOT NULL DEFAULT 0)`,
	`CREATE INDEX IF NOT EXISTS sync_runs_agent ON sync_runs(agent_id,status)`,
	`CREATE TABLE IF NOT EXISTS machine_aliases (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, alias TEXT NOT NULL, host TEXT NOT NULL, PRIMARY KEY(user_id,alias))`,
	`CREATE TABLE IF NOT EXISTS environments (
		id TEXT PRIMARY KEY, user_id INTEGER NOT NULL DEFAULT 0, name TEXT NOT NULL,
		kind TEXT NOT NULL DEFAULT 'machine', agent TEXT NOT NULL DEFAULT '', host TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '', project TEXT NOT NULL DEFAULT '',
		first_seen INTEGER NOT NULL, last_seen INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS skill_envs (
		skill_id TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
		env_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
		installed_at INTEGER NOT NULL, last_seen INTEGER NOT NULL, source_path TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (skill_id, env_id))`,
	`CREATE TABLE IF NOT EXISTS tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL COLLATE NOCASE UNIQUE)`,
	`CREATE TABLE IF NOT EXISTS skill_tags (
		user_id INTEGER NOT NULL DEFAULT 0,
		skill_id TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
		tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
		source TEXT NOT NULL DEFAULT 'user', hidden INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (user_id, skill_id, tag_id))`,
	`CREATE TABLE IF NOT EXISTS topic_skills (
		tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
		skill_id TEXT NOT NULL, PRIMARY KEY (tag_id, skill_id))`,
	`CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL, user_id INTEGER NOT NULL DEFAULT 0,
		actor TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'ui', action TEXT NOT NULL,
		skill_id TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '')`,
	`CREATE TABLE IF NOT EXISTS ignored (
		user_id INTEGER NOT NULL DEFAULT 0, id TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT '', at INTEGER NOT NULL, PRIMARY KEY (user_id, id))`,
	`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL COLLATE NOCASE UNIQUE,
		name TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'member',
		password_hash TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		disabled_at INTEGER NOT NULL DEFAULT 0)`,
	`CREATE TABLE IF NOT EXISTS publishers (
        username TEXT PRIMARY KEY, user_id INTEGER UNIQUE REFERENCES users(id) ON DELETE SET NULL)`,
	`CREATE TABLE IF NOT EXISTS setup_profiles (
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL, revision INTEGER NOT NULL,
		snapshot_id TEXT NOT NULL, snapshot_digest TEXT NOT NULL,
		updated_at INTEGER NOT NULL, payload TEXT NOT NULL,
		PRIMARY KEY(user_id,name))`,
	`CREATE INDEX IF NOT EXISTS idx_setup_profiles_user ON setup_profiles(user_id)`,
	`CREATE TABLE IF NOT EXISTS hosted_skills (
        user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        slug TEXT NOT NULL, description TEXT NOT NULL, content TEXT NOT NULL, files TEXT NOT NULL DEFAULT '{}',
        published INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
        PRIMARY KEY(user_id,slug))`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at INTEGER NOT NULL,
		ua TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		last_seen INTEGER NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
	`CREATE TABLE IF NOT EXISTS api_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL, hash TEXT NOT NULL UNIQUE, created_at INTEGER NOT NULL,
		last_used_at INTEGER NOT NULL DEFAULT 0, revoked_at INTEGER NOT NULL DEFAULT 0)`,
	`CREATE TABLE IF NOT EXISTS invites (
		token_hash TEXT PRIMARY KEY, email TEXT NOT NULL, role TEXT NOT NULL,
		created_by INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_invites_expires ON invites(expires_at)`,
	`CREATE TABLE IF NOT EXISTS shelf (
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		skill_id TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
		liked INTEGER NOT NULL DEFAULT 0, bookmarked INTEGER NOT NULL DEFAULT 0, used INTEGER NOT NULL DEFAULT 0,
		notes TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
		PRIMARY KEY (user_id, skill_id))`,
	`CREATE INDEX IF NOT EXISTS idx_shelf_skill ON shelf(skill_id)`,
	`CREATE TABLE IF NOT EXISTS lists (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
		visibility TEXT NOT NULL DEFAULT 'members', slug TEXT NOT NULL UNIQUE,
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS list_members (
		list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role TEXT NOT NULL DEFAULT 'viewer', PRIMARY KEY (list_id, user_id))`,
	`CREATE TABLE IF NOT EXISTS audience_groups(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,kind TEXT NOT NULL,parent_id INTEGER REFERENCES audience_groups(id) ON DELETE CASCADE,owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE)`,
	`CREATE TABLE IF NOT EXISTS audience_group_members(group_id INTEGER NOT NULL REFERENCES audience_groups(id) ON DELETE CASCADE,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,role TEXT NOT NULL DEFAULT 'member',PRIMARY KEY(group_id,user_id))`,
	`CREATE TABLE IF NOT EXISTS list_group_access(list_id INTEGER PRIMARY KEY REFERENCES lists(id) ON DELETE CASCADE,group_id INTEGER NOT NULL REFERENCES audience_groups(id) ON DELETE CASCADE,role TEXT NOT NULL DEFAULT 'viewer')`,
	`CREATE TABLE IF NOT EXISTS list_exclusions(list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,PRIMARY KEY(list_id,user_id))`,
	`CREATE TABLE IF NOT EXISTS list_skills (
		list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
		skill_id TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
		note TEXT NOT NULL DEFAULT '', added_by INTEGER NOT NULL DEFAULT 0,
		position INTEGER NOT NULL DEFAULT 0, added_at INTEGER NOT NULL,
		PRIMARY KEY (list_id, skill_id))`,
	`CREATE INDEX IF NOT EXISTS idx_list_members_user ON list_members(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_list_skills_skill ON list_skills(skill_id)`,
	`CREATE INDEX IF NOT EXISTS idx_skill_envs_env ON skill_envs(env_id)`,
	`CREATE INDEX IF NOT EXISTS idx_skill_tags_tag ON skill_tags(tag_id)`,
	`CREATE INDEX IF NOT EXISTS idx_topic_skills_skill ON topic_skills(skill_id)`,
	`CREATE INDEX IF NOT EXISTS idx_events_at ON events(at)`,
	`CREATE INDEX IF NOT EXISTS idx_events_skill ON events(skill_id)`,
	`CREATE TABLE IF NOT EXISTS environment_scans (
 env_id TEXT PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE,
 scanned_at INTEGER NOT NULL, complete INTEGER NOT NULL DEFAULT 0,
 os TEXT NOT NULL DEFAULT '', arch TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '')`,
	`CREATE TABLE IF NOT EXISTS sync_targets (
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 target_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
 source_id TEXT NOT NULL, source_name TEXT NOT NULL, desired TEXT NOT NULL,
 revision TEXT NOT NULL, updated_at INTEGER NOT NULL, PRIMARY KEY(user_id,target_id))`,
	`CREATE TRIGGER IF NOT EXISTS public_lists_require_sources BEFORE UPDATE OF visibility ON lists WHEN NEW.visibility='public' AND EXISTS (SELECT 1 FROM list_skills ls JOIN skills s ON s.id=ls.skill_id WHERE ls.list_id=NEW.id AND s.source_type='local') BEGIN SELECT RAISE(ABORT,'link local skills to an install source before publishing'); END`,
	`CREATE TRIGGER IF NOT EXISTS public_list_insert_requires_source BEFORE INSERT ON list_skills WHEN (SELECT visibility FROM lists WHERE id=NEW.list_id)='public' AND (SELECT source_type FROM skills WHERE id=NEW.skill_id)='local' BEGIN SELECT RAISE(ABORT,'link local skills to an install source before publishing'); END`,
	`CREATE TRIGGER IF NOT EXISTS public_list_update_requires_source BEFORE UPDATE OF list_id,skill_id ON list_skills WHEN (SELECT visibility FROM lists WHERE id=NEW.list_id)='public' AND (SELECT source_type FROM skills WHERE id=NEW.skill_id)='local' BEGIN SELECT RAISE(ABORT,'link local skills to an install source before publishing'); END`,
}

// Open opens (creating if needed) the database in dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "reet.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	for _, stmt := range schemaV12 {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := migrateMultiUser(db); err != nil {
		db.Close()
		return nil, err
	}
	st := &Store{db: db, dir: dir}
	if err := st.migrateBackups(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateMCP(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateMail(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateTickets(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateNotify(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migratePrivateSkills(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateTerminal(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateSyncAgents(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateSyncJobs(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateContentObservations(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateRemoteImports(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateSkillContentTargets(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateSkillContentPreviews(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.migrateSkillContentReceipts(); err != nil {
		db.Close()
		return nil, err
	}
	var ownerID int64
	if err := db.QueryRow(`SELECT id FROM users WHERE role='owner' ORDER BY id LIMIT 1`).Scan(&ownerID); err == nil {
		if err := st.MigrateToOwner(ownerID); err != nil {
			db.Close()
			return nil, err
		}
	}
	for _, c := range []struct{ name, ddl string }{
		{"installs_week", "installs_week INTEGER NOT NULL DEFAULT 0"},
		{"trend", "trend TEXT NOT NULL DEFAULT ''"},
		{"fetched_at", "fetched_at INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := ensureColumn(db, c.name, c.ddl); err != nil {
			db.Close()
			return nil, err
		}
	}
	return st, nil
}

func ensureColumn(db *sql.DB, name, ddl string) error {
	rows, err := db.Query(`PRAGMA table_info(skills)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var col, typ string
		var def sql.NullString
		if err := rows.Scan(&cid, &col, &typ, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if col == name {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if found {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE skills ADD COLUMN ` + ddl)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

// Path is the data directory that holds the database.
func (s *Store) Path() string { return s.dir }

// Size is the database file size in bytes.
func (s *Store) Size() (int64, error) {
	st, err := os.Stat(filepath.Join(s.dir, "reet.db"))
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// Health proves the database can take a write lock, then counts the shelf.
// A read-only data directory fails the lock rather than reporting a false all-clear.
func (s *Store) Health(ctx context.Context) (int, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return 0, err
	}
	if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
		return 0, err
	}
	var n int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM skills").Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func b2i(b *bool) int {
	if b != nil && *b {
		return 1
	}
	return 0
}

func nullBool(b *bool) sql.NullInt64 {
	if b == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(b2i(b)), Valid: true}
}

func now() int64 { return time.Now().UnixMilli() }

// Upsert inserts the skill or, if present, switches on any flags given. It never clears flags.
func (s *Store) Upsert(r ref.Ref, f Patch) (Skill, error) {
	return s.upsert(r.ID, r.Source, r.Slug, r.SourceType, r.URL, r.Slug, f)
}

// UpsertLocal records an installed skill whose skills.sh source is unknown.
func (s *Store) UpsertLocal(name string, f Patch) (Skill, error) {
	return s.upsert("local/"+name, "local", name, "local", "", name, f)
}

func (s *Store) upsert(id, source, slug, typ, url, name string, f Patch) (Skill, error) {
	t := now()
	notes := ""
	if f.Notes != nil {
		notes = *f.Notes
	}
	_, err := s.db.Exec(`INSERT INTO skills(`+cols+`) VALUES(?,?,?,?,?,?,'',?,0,0,'',?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			updated_at=CASE WHEN (liked=0 AND excluded.liked=1) OR (bookmarked=0 AND excluded.bookmarked=1) OR (used=0 AND excluded.used=1) THEN excluded.updated_at ELSE updated_at END,
			liked=liked OR excluded.liked, bookmarked=bookmarked OR excluded.bookmarked, used=used OR excluded.used`,
		id, source, slug, typ, url, name, notes, b2i(f.Liked), b2i(f.Bookmarked), b2i(f.Used), t, t)
	if err != nil {
		return Skill{}, err
	}
	if f.Notes != nil {
		if _, err := s.db.Exec(`UPDATE skills SET notes=? WHERE id=?`, *f.Notes, id); err != nil {
			return Skill{}, err
		}
	}
	_ = s.applyTopics(id)
	sk, _, err := s.Get(id)
	return sk, err
}

func scan(r interface{ Scan(...any) error }) (Skill, error) {
	var k Skill
	var l, b, u int
	var trend string
	err := r.Scan(&k.ID, &k.Source, &k.Slug, &k.SourceType, &k.URL, &k.Name, &k.Description, &k.Notes,
		&k.Installs, &k.InstallsWeek, &trend, &l, &b, &u, &k.CreatedAt, &k.UpdatedAt)
	if trend != "" {
		_ = json.Unmarshal([]byte(trend), &k.Trend)
	}
	k.Liked, k.Bookmarked, k.Used = l != 0, b != 0, u != 0
	return k, err
}

func (s *Store) Get(id string) (Skill, bool, error) {
	k, err := scan(s.db.QueryRow(`SELECT `+cols+` FROM skills WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Skill{}, false, nil
	}
	if err != nil {
		return Skill{}, false, err
	}
	one := []Skill{k}
	if err := s.attachMeta(one); err != nil {
		return Skill{}, false, err
	}
	return one[0], true, nil
}

func (s *Store) List(f Filter) ([]Skill, Counts, error) {
	var c Counts
	if err := s.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(liked),0),COALESCE(SUM(bookmarked),0),COALESCE(SUM(used),0) FROM skills`).
		Scan(&c.All, &c.Liked, &c.Bookmarked, &c.Used); err != nil {
		return nil, c, err
	}
	q := `SELECT ` + cols + ` FROM skills WHERE 1=1`
	var args []any
	switch f.Tab {
	case "liked", "bookmarked", "used":
		q += ` AND ` + f.Tab + `=1`
	}
	switch f.Stage {
	case "use":
		q += ` AND used=1`
	case "try":
		q += ` AND used=0 AND bookmarked=1`
	case "shelf":
		q += ` AND used=0 AND bookmarked=0`
	}
	for _, id := range f.Tags {
		q += ` AND EXISTS (SELECT 1 FROM skill_tags st WHERE st.skill_id=skills.id AND st.tag_id=? AND st.hidden=0)`
		args = append(args, id)
	}
	term, tagNames := splitQuery(f.Query)
	for _, name := range tagNames {
		q += ` AND EXISTS (SELECT 1 FROM skill_tags st JOIN tags t ON t.id=st.tag_id WHERE st.skill_id=skills.id AND st.hidden=0 AND lower(t.name)=?)`
		args = append(args, name)
	}
	if f.Env != "" {
		q += ` AND EXISTS (SELECT 1 FROM skill_envs se WHERE se.skill_id=skills.id AND se.env_id=?)`
		args = append(args, f.Env)
	}
	if f.Machine != "" {
		q += ` AND EXISTS (SELECT 1 FROM skill_envs se JOIN environments e ON e.id=se.env_id WHERE se.skill_id=skills.id AND e.host=?)`
		args = append(args, f.Machine)
	}
	if f.Only == "one-machine" {
		q += ` AND (SELECT COUNT(DISTINCT e.host) FROM skill_envs se JOIN environments e ON e.id=se.env_id WHERE se.skill_id=skills.id)=1`
	}
	if term != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(term)) + "%"
		q += ` AND (lower(name) LIKE ? ESCAPE '\' OR lower(id) LIKE ? ESCAPE '\' OR lower(description) LIKE ? ESCAPE '\' OR lower(notes) LIKE ? ESCAPE '\')`
		args = append(args, like, like, like, like)
	}
	switch f.Sort {
	case "name":
		q += ` ORDER BY lower(name) ASC`
	case "installs":
		q += ` ORDER BY installs DESC, lower(name) ASC`
	default:
		q += ` ORDER BY updated_at DESC, created_at DESC`
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, c, err
	}
	out := []Skill{}
	for rows.Next() {
		k, err := scan(rows)
		if err != nil {
			rows.Close()
			return nil, c, err
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, c, err
	}
	rows.Close()
	if err := s.attachMeta(out); err != nil {
		return nil, c, err
	}
	return out, c, nil
}

func (s *Store) attachHiddenTags(skills []Skill) error {
	if len(skills) == 0 {
		return nil
	}
	index := make(map[string]int, len(skills))
	for i := range skills {
		index[skills[i].ID] = i
	}
	rows, err := s.db.Query(`SELECT st.skill_id,t.id,t.name,st.source
		FROM skill_tags st JOIN tags t ON t.id=st.tag_id
		WHERE st.hidden=1 ORDER BY lower(t.name)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var skillID string
		var tag Tag
		if err := rows.Scan(&skillID, &tag.ID, &tag.Name, &tag.Source); err != nil {
			return err
		}
		tag.Hidden = true
		if i, ok := index[skillID]; ok {
			skills[i].HiddenTags = append(skills[i].HiddenTags, tag)
		}
	}
	return rows.Err()
}

func splitQuery(q string) (string, []string) {
	var words, tags []string
	for i := 0; i < len(q); {
		for i < len(q) && q[i] == ' ' {
			i++
		}
		if i >= len(q) {
			break
		}
		if strings.HasPrefix(q[i:], `tag:"`) {
			rest := q[i+5:]
			if end := strings.IndexByte(rest, '"'); end >= 0 {
				if name := strings.TrimSpace(rest[:end]); name != "" {
					tags = append(tags, strings.ToLower(name))
				}
				i += 5 + end + 1
				continue
			}
		}
		j := i
		for j < len(q) && q[j] != ' ' {
			j++
		}
		word := q[i:j]
		if name, ok := strings.CutPrefix(word, "tag:"); ok && name != "" {
			tags = append(tags, strings.ToLower(name))
		} else {
			words = append(words, word)
		}
		i = j
	}
	return strings.Join(words, " "), tags
}

func (s *Store) attachMeta(skills []Skill) error {
	if len(skills) == 0 {
		return nil
	}
	index := make(map[string]int, len(skills))
	for i := range skills {
		index[skills[i].ID] = i
	}
	tagRows, err := s.db.Query(`SELECT st.skill_id,t.id,t.name,st.source,st.hidden
		FROM skill_tags st JOIN tags t ON t.id=st.tag_id
		WHERE st.hidden=0 ORDER BY lower(t.name)`)
	if err != nil {
		return err
	}
	for tagRows.Next() {
		var skillID string
		var tag Tag
		var hidden int
		if err := tagRows.Scan(&skillID, &tag.ID, &tag.Name, &tag.Source, &hidden); err != nil {
			tagRows.Close()
			return err
		}
		tag.Hidden = hidden != 0
		if i, ok := index[skillID]; ok {
			skills[i].Tags = append(skills[i].Tags, tag)
		}
	}
	if err := tagRows.Err(); err != nil {
		tagRows.Close()
		return err
	}
	tagRows.Close()

	envRows, err := s.db.Query(`SELECT se.skill_id,e.id,e.name,e.kind,e.agent,e.host,e.path,e.first_seen,se.last_seen,se.source_path
		FROM skill_envs se JOIN environments e ON e.id=se.env_id ORDER BY lower(e.name)`)
	if err != nil {
		return err
	}
	defer envRows.Close()
	for envRows.Next() {
		var skillID string
		var env Environment
		if err := envRows.Scan(&skillID, &env.ID, &env.Name, &env.Kind, &env.Agent, &env.Host, &env.Path,
			&env.FirstSeen, &env.LastSeen, &env.SourcePath); err != nil {
			return err
		}
		if i, ok := index[skillID]; ok {
			skills[i].Environments = append(skills[i].Environments, env)
		}
	}
	return envRows.Err()
}

// Patch applies explicit changes (which may clear flags) to an existing skill.
func (s *Store) Patch(id string, p Patch) (Skill, error) {
	var notes sql.NullString
	if p.Notes != nil {
		notes = sql.NullString{String: *p.Notes, Valid: true}
	}
	res, err := s.db.Exec(`UPDATE skills SET liked=COALESCE(?,liked), bookmarked=COALESCE(?,bookmarked), used=COALESCE(?,used), notes=COALESCE(?,notes), updated_at=? WHERE id=?`,
		nullBool(p.Liked), nullBool(p.Bookmarked), nullBool(p.Used), notes, now(), id)
	if err != nil {
		return Skill{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Skill{}, ErrNotFound
	}
	k, _, err := s.Get(id)
	return k, err
}

// PatchMany applies the same flag changes to many skills in one statement.
func (s *Store) PatchMany(ids []string, p Patch) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if p.Notes != nil {
		return 0, errors.New("bulk notes are not supported")
	}
	q := `UPDATE skills SET liked=COALESCE(?,liked), bookmarked=COALESCE(?,bookmarked), used=COALESCE(?,used), updated_at=?
		WHERE id IN (` + placeholders(len(ids)) + `)`
	args := []any{nullBool(p.Liked), nullBool(p.Bookmarked), nullBool(p.Used), now()}
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// TagMany adds and removes tags across many skills in one transaction.
func (s *Store) TagMany(ids []string, add []string, remove []int64) (int, error) {
	return s.TagManyFor(0, ids, add, remove)
}

// RemoveMany deletes many skills, optionally recording each as ignored.
func (s *Store) RemoveMany(ids []string, ignore bool) (int, error) {
	n := 0
	err := s.withTx(func(tx *sql.Tx) error {
		for _, id := range ids {
			name := id
			if err := tx.QueryRow(`SELECT name FROM skills WHERE id=?`, id).Scan(&name); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if ignore {
				if _, err := tx.Exec(`INSERT INTO ignored(id,name,reason,at) VALUES(?,?,?,?)
					ON CONFLICT(id) DO UPDATE SET name=excluded.name, reason=excluded.reason, at=excluded.at`, id, name, "removed", now()); err != nil {
					return err
				}
			}
			res, err := tx.Exec(`DELETE FROM skills WHERE id=?`, id)
			if err != nil {
				return err
			}
			if rows, _ := res.RowsAffected(); rows > 0 {
				n += int(rows)
			}
		}
		return nil
	})
	return n, err
}

// SetMeta stores fetched metadata; empty values leave the stored value alone.
func (s *Store) SetMeta(id, name, description string, installs, installsWeek int64, trend []int64) error {
	trendJSON := ""
	if len(trend) > 0 {
		if b, err := json.Marshal(trend); err == nil {
			trendJSON = string(b)
		}
	}
	_, err := s.db.Exec(`UPDATE skills SET
		name=CASE WHEN ?='' THEN name ELSE ? END,
		description=CASE WHEN ?='' THEN description ELSE ? END,
		installs=CASE WHEN ?>0 THEN ? ELSE installs END,
		installs_week=CASE WHEN ?>0 THEN ? ELSE installs_week END,
		trend=CASE WHEN ?='' THEN trend ELSE ? END,
		fetched_at=?
		WHERE id=?`,
		name, name, description, description, installs, installs, installsWeek, installsWeek, trendJSON, trendJSON, now(), id)
	return err
}

// TouchFetched records that a refresh was attempted even when it returned nothing.
func (s *Store) TouchFetched(id string) error {
	_, err := s.db.Exec(`UPDATE skills SET fetched_at=? WHERE id=?`, now(), id)
	return err
}

// RefreshQueue returns the sourced skills with the oldest metadata.
func (s *Store) RefreshQueue(limit int) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM skills
		WHERE source_type IN ('github','well-known')
		ORDER BY COALESCE(fetched_at,0) ASC, id ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM skills WHERE id=?`, id)
	return err
}

// Resolve moves a skill's flags and notes onto the real skills.sh id and removes the old row.
func (s *Store) Resolve(oldID string, r ref.Ref) (Skill, error) {
	old, ok, err := s.Get(oldID)
	if err != nil {
		return Skill{}, err
	}
	if !ok {
		return Skill{}, ErrNotFound
	}
	nk, err := s.Upsert(r, Patch{Liked: &old.Liked, Bookmarked: &old.Bookmarked, Used: &old.Used})
	if err != nil {
		return Skill{}, err
	}
	if nk.Notes == "" && old.Notes != "" {
		if nk, err = s.Patch(nk.ID, Patch{Notes: &old.Notes}); err != nil {
			return Skill{}, err
		}
	}
	if oldID != nk.ID {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO skill_tags(skill_id,tag_id,source,hidden)
			SELECT ?,tag_id,source,hidden FROM skill_tags WHERE skill_id=?`, nk.ID, oldID); err != nil {
			return Skill{}, err
		}
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path)
			SELECT ?,env_id,installed_at,last_seen,source_path FROM skill_envs WHERE skill_id=?`, nk.ID, oldID); err != nil {
			return Skill{}, err
		}
		if err := s.Delete(oldID); err != nil {
			return Skill{}, err
		}
	}
	return nk, nil
}

func (s *Store) Export() ([]Skill, error) {
	rows, err := s.db.Query(`SELECT ` + cols + ` FROM skills ORDER BY id`)
	if err != nil {
		return nil, err
	}
	out := []Skill{}
	for rows.Next() {
		k, err := scan(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := s.attachMeta(out); err != nil {
		return nil, err
	}
	if err := s.attachHiddenTags(out); err != nil {
		return nil, err
	}
	return out, nil
}

// ExportSnapshot returns skills, ignored ids and optionally the local event history.
func (s *Store) ExportSnapshot(includeEvents bool) (Snapshot, error) {
	skills, err := s.Export()
	if err != nil {
		return Snapshot{}, err
	}
	ignored, err := s.ListIgnored()
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Version: 2, ExportedAt: now(), Skills: skills, Ignored: ignored}
	if includeEvents {
		events, err := s.ListEvents(0, "")
		if err != nil {
			return Snapshot{}, err
		}
		snap.Events = events
	}
	return snap, nil
}

func (s *Store) RecordEnvironment(skillID, host, agent, project, path, sourcePath string) error {
	if strings.TrimSpace(host) == "" || strings.TrimSpace(agent) == "" {
		return nil
	}
	t := now()
	id := EnvironmentID(host, agent, project)
	if err := s.TouchEnvironment(host, agent, project, path); err != nil {
		return err
	}
	if sourcePath == "" {
		sourcePath = path
	}
	_, err := s.db.Exec(`INSERT INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path)
		VALUES(?,?,?,?,?)
		ON CONFLICT(skill_id,env_id) DO UPDATE SET last_seen=excluded.last_seen, source_path=excluded.source_path`,
		skillID, id, t, t, sourcePath)
	return err
}

func (s *Store) TouchEnvironment(host, agent, project, path string) error {
	if strings.TrimSpace(host) == "" || strings.TrimSpace(agent) == "" {
		return nil
	}
	t := now()
	id := EnvironmentID(host, agent, project)
	name := EnvironmentName(host, agent, project)
	kind := "machine"
	if strings.TrimSpace(project) != "" {
		kind = "project"
	}
	_, err := s.db.Exec(`INSERT INTO environments(id,name,kind,agent,host,path,first_seen,last_seen)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, last_seen=excluded.last_seen,
			path=CASE WHEN environments.path='' THEN excluded.path ELSE environments.path END`,
		id, name, kind, agent, host, path, t, t)
	return err
}

// EnvironmentID is stable for a host, agent and optional project.
func EnvironmentID(host, agent, project string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	agent = strings.ToLower(strings.TrimSpace(agent))
	id := host + "/" + agent
	if project = strings.TrimSpace(project); project != "" {
		id += "/" + strings.ToLower(strings.ReplaceAll(project, "/", "-"))
	}
	return id
}

// EnvironmentName is what a person sees on a card.
func EnvironmentName(host, agent, project string) string {
	if strings.TrimSpace(host) == "" {
		host = "unknown"
	}
	name := strings.TrimSpace(host) + " · " + AgentLabel(agent)
	if project = strings.TrimSpace(project); project != "" {
		name += " · " + project
	}
	return name
}

func AgentLabel(agent string) string {
	switch strings.ToLower(agent) {
	case "claude":
		return "Claude Code"
	case "cursor":
		return "Cursor"
	case "codex":
		return "Codex"
	case "opencode":
		return "OpenCode"
	case "agents":
		return "Shared agents / Codex"
	case "other":
		return "Other"
	}
	if agent == "" {
		return "Other"
	}
	return agent
}

func (s *Store) ListEnvironments() ([]Environment, error) {
	rows, err := s.db.Query(`SELECT e.id,e.name,e.kind,e.agent,e.host,e.path,e.first_seen,e.last_seen,COUNT(se.skill_id)
		FROM environments e LEFT JOIN skill_envs se ON se.env_id=e.id
		GROUP BY e.id ORDER BY lower(e.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Environment{}
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.Name, &e.Kind, &e.Agent, &e.Host, &e.Path, &e.FirstSeen, &e.LastSeen, &e.SkillCount); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) PruneEnvironment(envID string, keep []string) (int64, error) {
	if len(keep) == 0 {
		res, err := s.db.Exec(`DELETE FROM skill_envs WHERE env_id=?`, envID)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	args := []any{envID}
	for _, id := range keep {
		args = append(args, id)
	}
	res, err := s.db.Exec(`DELETE FROM skill_envs WHERE env_id=? AND skill_id NOT IN (`+placeholders(len(keep))+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) ListTags() ([]Tag, error) {
	rows, err := s.db.Query(`SELECT t.id,t.name,COUNT(st.skill_id)
		FROM tags t LEFT JOIN skill_tags st ON st.tag_id=t.id AND st.hidden=0
		GROUP BY t.id ORDER BY lower(t.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) AddSkillTags(skillID string, names []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tagID, err := upsertTagTx(tx, name)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO skill_tags(user_id,skill_id,tag_id,source,hidden) VALUES(0,?,?,'user',0)
			ON CONFLICT(user_id,skill_id,tag_id) DO UPDATE SET source='user', hidden=0`, skillID, tagID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RemoveSkillTag(skillID string, tagID int64) error {
	return s.RemoveSkillTagFor(0, skillID, tagID)
}

func (s *Store) RenameTag(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("tag name is required")
	}
	res, err := s.db.Exec(`UPDATE tags SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MergeTags(fromID, intoID int64) error {
	if fromID == intoID {
		return nil
	}
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO skill_tags(user_id,skill_id,tag_id,source,hidden)
			SELECT user_id,skill_id,?,source,hidden FROM skill_tags WHERE tag_id=?`, intoID, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM skill_tags WHERE tag_id=?`, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO topic_skills(tag_id,skill_id)
			SELECT ?,skill_id FROM topic_skills WHERE tag_id=?`, intoID, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM topic_skills WHERE tag_id=?`, fromID); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM tags WHERE id=?`, fromID)
		return err
	})
}

// Topic is one skills.sh curated topic and its current skill ids.
type Topic struct {
	Slug   string
	Name   string
	Skills []string
}

func (s *Store) ReplaceTopics(topics []Topic) error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, topic := range topics {
			tagID, err := upsertTagTx(tx, topic.Name)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM topic_skills WHERE tag_id=?`, tagID); err != nil {
				return err
			}
			for _, id := range topic.Skills {
				if _, err := tx.Exec(`INSERT OR IGNORE INTO topic_skills(tag_id,skill_id) VALUES(?,?)`, tagID, id); err != nil {
					return err
				}
			}
			if len(topic.Skills) == 0 {
				if _, err := tx.Exec(`DELETE FROM skill_tags WHERE tag_id=? AND source='topic' AND hidden=0`, tagID); err != nil {
					return err
				}
				continue
			}
			args := []any{tagID}
			for _, id := range topic.Skills {
				args = append(args, id)
			}
			if _, err := tx.Exec(`DELETE FROM skill_tags WHERE tag_id=? AND source='topic' AND hidden=0 AND skill_id NOT IN (`+placeholders(len(topic.Skills))+`)`, args...); err != nil {
				return err
			}
			args = []any{tagID}
			for _, id := range topic.Skills {
				args = append(args, id)
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO skill_tags(skill_id,tag_id,source,hidden)
				SELECT id,?,'topic',0 FROM skills WHERE id IN (`+placeholders(len(topic.Skills))+`)`, args...); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`INSERT INTO settings(key,value) VALUES('topics_last_synced',?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.FormatInt(now(), 10))
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO settings(key,value) VALUES('topics_last_error','')
			ON CONFLICT(key) DO UPDATE SET value=''`)
		return err
	})
}

func (s *Store) applyTopics(id string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO skill_tags(skill_id,tag_id,source,hidden)
		SELECT ?,tag_id,'topic',0 FROM topic_skills WHERE skill_id=?`, id, id)
	return err
}

func (s *Store) ListIgnored() ([]Ignored, error) {
	rows, err := s.db.Query(`SELECT id,name,reason,at FROM ignored ORDER BY at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ignored{}
	for rows.Next() {
		var i Ignored
		if err := rows.Scan(&i.ID, &i.Name, &i.Reason, &i.At); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) IsIgnored(id string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM ignored WHERE id=?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) Ignore(id, name, reason string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO ignored(user_id,id,name,reason,at) VALUES(0,?,?,?,?)
			ON CONFLICT(user_id,id) DO UPDATE SET name=excluded.name, reason=excluded.reason, at=excluded.at`, id, name, reason, now()); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM skills WHERE id=?`, id)
		return err
	})
}

func (s *Store) Unignore(id string) error {
	_, err := s.db.Exec(`DELETE FROM ignored WHERE id=?`, id)
	return err
}

func (s *Store) AddEvent(source, action, skillID string, detail any) error {
	detailJSON := ""
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		}
	}
	_, err := s.db.Exec(`INSERT INTO events(at,actor,source,action,skill_id,detail) VALUES(?,?,?,?,?,?)`,
		now(), "", source, action, skillID, detailJSON)
	return err
}

func (s *Store) ListEvents(limit int, skillID string) ([]Event, error) {
	q := `SELECT ev.id,ev.at,ev.actor,ev.source,ev.action,ev.skill_id,COALESCE(s.name,''),ev.detail
		FROM events ev LEFT JOIN skills s ON s.id=ev.skill_id`
	var args []any
	if skillID != "" {
		q += ` WHERE ev.skill_id=?`
		args = append(args, skillID)
	}
	q += ` ORDER BY ev.at DESC, ev.id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.ID, &ev.At, &ev.Actor, &ev.Source, &ev.Action, &ev.SkillID, &ev.SkillName, &ev.Detail); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Store) PruneEvents(cutoff int64) error {
	_, err := s.db.Exec(`DELETE FROM events WHERE at<?`, cutoff)
	return err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) withTx(fn func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertTagTx(tx *sql.Tx, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("tag name is required")
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO tags(name) VALUES(?)`, name); err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow(`SELECT id FROM tags WHERE name=? COLLATE NOCASE`, name).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// UserCount is the number of accounts, used to decide whether setup is needed.
func (s *Store) UserCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(email, name, role, passwordHash string) (User, error) {
	email = normalizeEmail(email)
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	if role == "" {
		role = "member"
	}
	res, err := s.db.Exec(`INSERT INTO users(email,name,role,password_hash,created_at) VALUES(?,?,?,?,?)`,
		email, name, role, passwordHash, now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, ErrUserExists
		}
		return User{}, err
	}
	id, _ := res.LastInsertId()
	u, _, err := s.UserByID(id)
	return u, err
}

// CreateFirstOwner creates the only account if the users table is still empty.
func (s *Store) CreateFirstOwner(email, name, role, passwordHash string) (User, bool, error) {
	var user User
	created := false
	err := s.withTx(func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		email = normalizeEmail(email)
		name = strings.TrimSpace(name)
		if name == "" {
			name = strings.Split(email, "@")[0]
		}
		res, err := tx.Exec(`INSERT INTO users(email,name,role,password_hash,created_at) VALUES(?,?,?,?,?)`,
			email, name, role, passwordHash, now())
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if err := tx.QueryRow(`SELECT id,email,name,role,password_hash,created_at,disabled_at FROM users WHERE id=?`, id).
			Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.PasswordHash, &user.CreatedAt, &user.DisabledAt); err != nil {
			return err
		}
		created = true
		return nil
	})
	return user, created, err
}

func (s *Store) UserByEmail(email string) (User, bool, error) {
	return s.scanUser(s.db.QueryRow(`SELECT id,email,name,role,password_hash,created_at,disabled_at FROM users WHERE email=?`, normalizeEmail(email)))
}

func (s *Store) UserByID(id int64) (User, bool, error) {
	return s.scanUser(s.db.QueryRow(`SELECT id,email,name,role,password_hash,created_at,disabled_at FROM users WHERE id=?`, id))
}

func (s *Store) scanUser(row interface{ Scan(...any) error }) (User, bool, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.CreatedAt, &u.DisabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	return u, err == nil, err
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT id,email,name,role,password_hash,created_at,disabled_at FROM users ORDER BY lower(name), lower(email)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, _, err := s.scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) SetUserDisabled(id int64, disabled bool) error {
	at := int64(0)
	if disabled {
		at = now()
	}
	res, err := s.db.Exec(`UPDATE users SET disabled_at=? WHERE id=?`, at, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateSession(id string, userID, expiresAt int64, ua string) error {
	_, err := s.db.Exec(`INSERT INTO sessions(id,user_id,expires_at,ua,created_at,last_seen) VALUES(?,?,?,?,?,?)`,
		id, userID, expiresAt, ua, now(), now())
	return err
}

func (s *Store) SessionUser(id string, at int64) (User, bool, error) {
	var u User
	err := s.db.QueryRow(`SELECT u.id,u.email,u.name,u.role,u.password_hash,u.created_at,u.disabled_at
		FROM sessions s JOIN users u ON u.id=s.user_id
		WHERE s.id=? AND s.expires_at>? AND u.disabled_at=0`, id, at).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.CreatedAt, &u.DisabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	_, _ = s.db.Exec(`UPDATE sessions SET last_seen=? WHERE id=?`, at, id)
	return u, true, nil
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
	return err
}

func (s *Store) DeleteUserSessions(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

func (s *Store) PruneSessions(at int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at<=?`, at)
	return err
}

// rederive rebuilds the identity fields of a restored skill from its id, so a crafted
// backup cannot plant a foreign link or a mismatched source.
func rederive(k *Skill) error {
	if name, ok := strings.CutPrefix(k.ID, "local/"); ok {
		if name == "" || strings.Contains(name, "/") {
			return fmt.Errorf("invalid skill id %q", k.ID)
		}
		k.Source, k.Slug, k.SourceType, k.URL = "local", name, "local", ""
		return nil
	}
	if strings.HasPrefix(k.ID, "hosted/") {
		u, err := url.Parse(k.URL)
		if err != nil {
			return fmt.Errorf("invalid hosted skill URL")
		}
		r, err := ref.ParseAt(k.URL, u.Host)
		if err != nil || r.SourceType != "hosted" || r.ID != k.ID {
			return fmt.Errorf("invalid hosted skill id %q", k.ID)
		}
		k.Source, k.Slug, k.SourceType, k.URL = r.Source, r.Slug, r.SourceType, r.URL
		return nil
	}
	r, err := ref.Parse(k.ID)
	if err != nil || r.ID != k.ID {
		return fmt.Errorf("invalid skill id %q", k.ID)
	}
	k.Source, k.Slug, k.SourceType, k.URL = r.Source, r.Slug, r.SourceType, r.URL
	return nil
}

// Restore inserts or replaces every skill given, in one transaction.
func (s *Store) Restore(all []Skill) error {
	return s.withTx(func(tx *sql.Tx) error { return restoreSkills(tx, all) })
}

// RestoreSnapshot restores skills and ignored ids; local history is not rewritten.
func (s *Store) RestoreSnapshot(snapshot Snapshot) error {
	return s.withTx(func(tx *sql.Tx) error {
		if err := restoreSkills(tx, snapshot.Skills); err != nil {
			return err
		}
		for _, i := range snapshot.Ignored {
			if i.ID == "" {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO ignored(user_id,id,name,reason,at) VALUES(0,?,?,?,?)
				ON CONFLICT(user_id,id) DO UPDATE SET name=excluded.name, reason=excluded.reason, at=excluded.at`,
				i.ID, i.Name, i.Reason, i.At); err != nil {
				return err
			}
		}
		return nil
	})
}

func restoreSkills(tx *sql.Tx, all []Skill) error {
	for _, k := range all {
		if err := rederive(&k); err != nil {
			return err
		}
		t := now()
		if k.CreatedAt == 0 {
			k.CreatedAt = t
		}
		if k.UpdatedAt == 0 {
			k.UpdatedAt = t
		}
		l, b, u := k.Liked, k.Bookmarked, k.Used
		trend := ""
		if len(k.Trend) > 0 {
			if tb, err := json.Marshal(k.Trend); err == nil {
				trend = string(tb)
			}
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO skills(`+cols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			k.ID, k.Source, k.Slug, k.SourceType, k.URL, k.Name, k.Description, k.Notes, k.Installs, k.InstallsWeek, trend,
			b2i(&l), b2i(&b), b2i(&u), k.CreatedAt, k.UpdatedAt); err != nil {
			return err
		}
		for _, tag := range k.Tags {
			tagID, err := upsertTagTx(tx, tag.Name)
			if err != nil {
				return err
			}
			source := tag.Source
			if source == "" {
				source = "user"
			}
			if _, err := tx.Exec(`INSERT OR REPLACE INTO skill_tags(skill_id,tag_id,source,hidden) VALUES(?,?,?,?)`,
				k.ID, tagID, source, b2i(&tag.Hidden)); err != nil {
				return err
			}
			if source == "topic" {
				if _, err := tx.Exec(`INSERT OR IGNORE INTO topic_skills(tag_id,skill_id) VALUES(?,?)`, tagID, k.ID); err != nil {
					return err
				}
			}
		}
		for _, tag := range k.HiddenTags {
			tagID, err := upsertTagTx(tx, tag.Name)
			if err != nil {
				return err
			}
			source := tag.Source
			if source == "" {
				source = "topic"
			}
			if _, err := tx.Exec(`INSERT OR REPLACE INTO skill_tags(skill_id,tag_id,source,hidden) VALUES(?,?,?,1)`,
				k.ID, tagID, source); err != nil {
				return err
			}
			if source == "topic" {
				if _, err := tx.Exec(`INSERT OR IGNORE INTO topic_skills(tag_id,skill_id) VALUES(?,?)`, tagID, k.ID); err != nil {
					return err
				}
			}
		}
		for _, env := range k.Environments {
			if env.Host == "" || env.Agent == "" {
				continue
			}
			id := env.ID
			if id == "" {
				id = EnvironmentID(env.Host, env.Agent, "")
			}
			name := env.Name
			if name == "" {
				name = EnvironmentName(env.Host, env.Agent, "")
			}
			kind := env.Kind
			if kind == "" {
				kind = "machine"
			}
			first, last := env.FirstSeen, env.LastSeen
			if first == 0 {
				first = t
			}
			if last == 0 {
				last = t
			}
			if _, err := tx.Exec(`INSERT INTO environments(id,name,kind,agent,host,path,first_seen,last_seen)
				VALUES(?,?,?,?,?,?,?,?)
				ON CONFLICT(id) DO UPDATE SET name=excluded.name, agent=excluded.agent, host=excluded.host,
					path=CASE WHEN environments.path='' THEN excluded.path ELSE environments.path END,
					last_seen=MAX(environments.last_seen, excluded.last_seen)`,
				id, name, kind, env.Agent, env.Host, env.Path, first, last); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path)
				VALUES(?,?,?,?,?)
				ON CONFLICT(skill_id,env_id) DO UPDATE SET last_seen=excluded.last_seen, source_path=excluded.source_path`,
				k.ID, id, first, last, env.SourcePath); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`DELETE FROM ignored WHERE id=?`, k.ID); err != nil {
			return err
		}
	}
	return nil
}
