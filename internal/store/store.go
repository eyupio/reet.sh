// Package store persists skills and their like/bookmark/used flags in SQLite.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/eyupio/reet.sh/internal/ref"
)

// ErrNotFound is returned when a skill id does not exist.
var ErrNotFound = errors.New("skill not found")

type Skill struct {
	ID           string  `json:"id"`
	Source       string  `json:"source"`
	Slug         string  `json:"slug"`
	SourceType   string  `json:"sourceType"`
	URL          string  `json:"url"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Notes        string  `json:"notes"`
	Installs     int64   `json:"installs"`
	InstallsWeek int64   `json:"installsWeek"`
	Trend        []int64 `json:"trend"`
	Liked        bool    `json:"liked"`
	Bookmarked   bool    `json:"bookmarked"`
	Used         bool    `json:"used"`
	CreatedAt    int64   `json:"createdAt"`
	UpdatedAt    int64   `json:"updatedAt"`
}

// Patch holds optional changes; nil fields are left alone.
type Patch struct {
	Liked      *bool   `json:"liked,omitempty"`
	Bookmarked *bool   `json:"bookmarked,omitempty"`
	Used       *bool   `json:"used,omitempty"`
	Notes      *string `json:"notes,omitempty"`
}

// Filter selects and orders List results. Tab: all|liked|bookmarked|used. Sort: recent|name|installs.
type Filter struct{ Tab, Query, Sort string }

type Counts struct {
	All        int `json:"all"`
	Liked      int `json:"liked"`
	Bookmarked int `json:"bookmarked"`
	Used       int `json:"used"`
}

type Store struct{ db *sql.DB }

const cols = `id,source,slug,source_type,url,name,description,notes,installs,installs_week,trend,liked,bookmarked,used,created_at,updated_at`

const schema = `CREATE TABLE IF NOT EXISTS skills (
	id TEXT PRIMARY KEY, source TEXT NOT NULL DEFAULT '', slug TEXT NOT NULL DEFAULT '',
	source_type TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '',
	installs INTEGER NOT NULL DEFAULT 0, installs_week INTEGER NOT NULL DEFAULT 0, trend TEXT NOT NULL DEFAULT '',
	liked INTEGER NOT NULL DEFAULT 0, bookmarked INTEGER NOT NULL DEFAULT 0, used INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`

// Open opens (creating if needed) the database in dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "reet.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	for _, c := range []struct{ name, ddl string }{
		{"installs_week", "installs_week INTEGER NOT NULL DEFAULT 0"},
		{"trend", "trend TEXT NOT NULL DEFAULT ''"},
	} {
		if err := ensureColumn(db, c.name, c.ddl); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
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
	return k, err == nil, err
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
	if term := strings.TrimSpace(f.Query); term != "" {
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
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		k, err := scan(rows)
		if err != nil {
			return nil, c, err
		}
		out = append(out, k)
	}
	return out, c, rows.Err()
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
		trend=CASE WHEN ?='' THEN trend ELSE ? END
		WHERE id=?`,
		name, name, description, description, installs, installs, installsWeek, installsWeek, trendJSON, trendJSON, id)
	return err
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
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		k, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
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
	r, err := ref.Parse(k.ID)
	if err != nil || r.ID != k.ID {
		return fmt.Errorf("invalid skill id %q", k.ID)
	}
	k.Source, k.Slug, k.SourceType, k.URL = r.Source, r.Slug, r.SourceType, r.URL
	return nil
}

// Restore inserts or replaces every skill given, in one transaction.
func (s *Store) Restore(all []Skill) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
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
	}
	return tx.Commit()
}
