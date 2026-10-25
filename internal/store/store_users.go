package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/ref"
)

const shelfCols = `s.id,s.source,s.slug,s.source_type,s.url,s.name,s.description,sh.notes,s.installs,s.installs_week,s.trend,sh.liked,sh.bookmarked,sh.used,sh.created_at,sh.updated_at`

func hasColumn(db *sql.DB, table, name string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var col, typ string
		var def sql.NullString
		if err := rows.Scan(&cid, &col, &typ, &notnull, &def, &pk); err != nil {
			return false, err
		}
		if col == name {
			return true, nil
		}
	}
	return false, rows.Err()
}

func migrateMultiUser(db *sql.DB) error {
	add := func(table, ddl string) error {
		ok, err := hasColumn(db, table, strings.Fields(ddl)[0])
		if err != nil || ok {
			return err
		}
		_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + ddl)
		return err
	}
	if err := add("environments", "user_id INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := add("environments", "project TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := add("events", "user_id INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if ok, err := hasColumn(db, "skill_tags", "user_id"); err != nil {
		return err
	} else if !ok {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, stmt := range []string{
			`CREATE TABLE skill_tags_v2 (
				user_id INTEGER NOT NULL DEFAULT 0,
				skill_id TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
				tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
				source TEXT NOT NULL DEFAULT 'user', hidden INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY (user_id, skill_id, tag_id))`,
			`INSERT INTO skill_tags_v2(user_id,skill_id,tag_id,source,hidden) SELECT 0,skill_id,tag_id,source,hidden FROM skill_tags`,
			`DROP TABLE skill_tags`,
			`ALTER TABLE skill_tags_v2 RENAME TO skill_tags`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	if ok, err := hasColumn(db, "ignored", "user_id"); err != nil {
		return err
	} else if !ok {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, stmt := range []string{
			`CREATE TABLE ignored_v2 (
				user_id INTEGER NOT NULL DEFAULT 0, id TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
				reason TEXT NOT NULL DEFAULT '', at INTEGER NOT NULL, PRIMARY KEY (user_id, id))`,
			`INSERT INTO ignored_v2(user_id,id,name,reason,at) SELECT 0,id,name,reason,at FROM ignored`,
			`DROP TABLE ignored`,
			`ALTER TABLE ignored_v2 RENAME TO ignored`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_skill_tags_tag ON skill_tags(tag_id)`,
		`CREATE INDEX IF NOT EXISTS idx_skill_tags_user ON skill_tags(user_id, skill_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_user ON events(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_environments_user ON environments(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ignored_user ON ignored(user_id)`,
	} {
		if _, err := db.Exec(idx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) OwnerUser() (User, bool, error) {
	return s.scanUser(s.db.QueryRow(`SELECT id,email,name,role,password_hash,created_at,disabled_at FROM users WHERE role='owner' ORDER BY id LIMIT 1`))
}

// DefaultOwner returns the owner account, creating a local one when the instance has no accounts.
func (s *Store) DefaultOwner() (int64, error) {
	if u, ok, err := s.OwnerUser(); err != nil {
		return 0, err
	} else if ok {
		if err := s.MigrateToOwner(u.ID); err != nil {
			return 0, err
		}
		return u.ID, nil
	}
	u, err := s.CreateUser("local@reet.invalid", "Local owner", "owner", "")
	if errors.Is(err, ErrUserExists) {
		u, _, err = s.UserByEmail("local@reet.invalid")
	}
	if err != nil {
		return 0, err
	}
	if err := s.MigrateToOwner(u.ID); err != nil {
		return 0, err
	}
	return u.ID, nil
}

func (s *Store) EnsureLocalOwner() (User, error) {
	id, err := s.DefaultOwner()
	if err != nil {
		return User{}, err
	}
	u, _, err := s.UserByID(id)
	return u, err
}

// MigrateToOwner copies legacy single-user state onto the first owner.
func (s *Store) MigrateToOwner(ownerID int64) error {
	if v, _ := s.GetSetting("shelf_migrated"); v == "1" {
		return nil
	}
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`PRAGMA defer_foreign_keys=ON`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO shelf(user_id,skill_id,liked,bookmarked,used,notes,created_at,updated_at)
			SELECT ?,id,liked,bookmarked,used,notes,created_at,updated_at FROM skills`, ownerID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO skill_tags(user_id,skill_id,tag_id,source,hidden)
			SELECT ?,skill_id,tag_id,source,hidden FROM skill_tags WHERE user_id=0`, ownerID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM skill_tags WHERE user_id=0`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO ignored(user_id,id,name,reason,at)
			SELECT ?,id,name,reason,at FROM ignored WHERE user_id=0`, ownerID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM ignored WHERE user_id=0`); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE events SET user_id=? WHERE user_id=0`, ownerID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE environments SET user_id=? WHERE user_id=0`, ownerID); err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT id FROM environments WHERE user_id=?`, ownerID)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			if !strings.HasPrefix(id, strconv.FormatInt(ownerID, 10)+"/") {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, oldID := range ids {
			newID := strconv.FormatInt(ownerID, 10) + "/" + oldID
			if _, err := tx.Exec(`UPDATE skill_envs SET env_id=? WHERE env_id=?`, newID, oldID); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE environments SET id=? WHERE id=?`, newID, oldID); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`INSERT INTO settings(key,value) VALUES('shelf_migrated','1')
			ON CONFLICT(key) DO UPDATE SET value='1'`)
		return err
	})
}

func (s *Store) attachMetaFor(userID int64, skills []Skill) error {
	if len(skills) == 0 {
		return nil
	}
	index := make(map[string]int, len(skills))
	for i := range skills {
		index[skills[i].ID] = i
	}
	tagRows, err := s.db.Query(`SELECT st.skill_id,t.id,t.name,st.source,st.hidden
		FROM skill_tags st JOIN tags t ON t.id=st.tag_id
		WHERE st.user_id=? AND st.hidden=0 ORDER BY lower(t.name)`, userID)
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
	envRows, err := s.db.Query(`SELECT se.skill_id,e.id,e.name,e.kind,e.agent,e.host,e.path,e.project,
		e.first_seen,se.last_seen,se.source_path
		FROM skill_envs se JOIN environments e ON e.id=se.env_id
		WHERE e.user_id=? ORDER BY lower(e.name)`, userID)
	if err != nil {
		return err
	}
	defer envRows.Close()
	for envRows.Next() {
		var skillID string
		var env Environment
		if err := envRows.Scan(&skillID, &env.ID, &env.Name, &env.Kind, &env.Agent, &env.Host, &env.Path,
			&env.Project, &env.FirstSeen, &env.LastSeen, &env.SourcePath); err != nil {
			return err
		}
		if i, ok := index[skillID]; ok {
			skills[i].Environments = append(skills[i].Environments, env)
		}
	}
	if err := envRows.Err(); err != nil {
		return err
	}
	return s.attachListsFor(userID, skills)
}

func (s *Store) attachHiddenTagsFor(userID int64, skills []Skill) error {
	index := make(map[string]int, len(skills))
	for i := range skills {
		index[skills[i].ID] = i
	}
	rows, err := s.db.Query(`SELECT st.skill_id,t.id,t.name,st.source
		FROM skill_tags st JOIN tags t ON t.id=st.tag_id
		WHERE st.user_id=? AND st.hidden=1 ORDER BY lower(t.name)`, userID)
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

func (s *Store) GetFor(userID int64, id string) (Skill, bool, error) {
	k, err := scan(s.db.QueryRow(`SELECT `+shelfCols+` FROM skills s JOIN shelf sh ON sh.skill_id=s.id
		WHERE sh.user_id=? AND s.id=?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Skill{}, false, nil
	}
	if err != nil {
		return Skill{}, false, err
	}
	one := []Skill{k}
	if err := s.attachMetaFor(userID, one); err != nil {
		return Skill{}, false, err
	}
	return one[0], true, nil
}

func (s *Store) ListFor(userID int64, f Filter) ([]Skill, Counts, error) {
	if f.Machine != "" {
		host, err := s.ResolveMachineHostFor(userID, f.Machine)
		if err != nil {
			return nil, Counts{}, err
		}
		f.Machine = host
	}
	var c Counts
	if err := s.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(liked),0),COALESCE(SUM(bookmarked),0),COALESCE(SUM(used),0)
		FROM shelf WHERE user_id=?`, userID).Scan(&c.All, &c.Liked, &c.Bookmarked, &c.Used); err != nil {
		return nil, c, err
	}
	q := `SELECT ` + shelfCols + ` FROM skills s JOIN shelf sh ON sh.skill_id=s.id WHERE sh.user_id=?`
	args := []any{userID}
	tabWhere := map[string]string{
		"liked":      ` AND sh.liked=1`,
		"bookmarked": ` AND sh.bookmarked=1`,
		"used":       ` AND sh.used=1`,
	}
	if clause, ok := tabWhere[f.Tab]; ok {
		q += clause
	}
	switch f.Stage {
	case "use":
		q += ` AND sh.used=1`
	case "try":
		q += ` AND sh.used=0 AND sh.bookmarked=1`
	case "shelf":
		q += ` AND sh.used=0 AND sh.bookmarked=0`
	}
	for _, id := range f.Tags {
		q += ` AND EXISTS (SELECT 1 FROM skill_tags st WHERE st.user_id=? AND st.skill_id=sh.skill_id AND st.tag_id=? AND st.hidden=0)`
		args = append(args, userID, id)
	}
	term, tagNames := splitQuery(f.Query)
	for _, name := range tagNames {
		q += ` AND EXISTS (SELECT 1 FROM skill_tags st JOIN tags t ON t.id=st.tag_id
			WHERE st.user_id=? AND st.skill_id=sh.skill_id AND st.hidden=0 AND lower(t.name)=?)`
		args = append(args, userID, name)
	}
	if f.Env != "" {
		q += ` AND EXISTS (SELECT 1 FROM skill_envs se JOIN environments e ON e.id=se.env_id
			WHERE se.skill_id=sh.skill_id AND e.user_id=? AND e.id=?)`
		args = append(args, userID, f.Env)
	}
	if f.Machine != "" {
		q += ` AND EXISTS (SELECT 1 FROM skill_envs se JOIN environments e ON e.id=se.env_id WHERE se.skill_id=sh.skill_id AND e.user_id=? AND e.host=?)`
		args = append(args, userID, f.Machine)
	}
	if f.Only == "one-machine" {
		q += ` AND (SELECT COUNT(DISTINCT e.host) FROM skill_envs se JOIN environments e ON e.id=se.env_id
			WHERE se.skill_id=sh.skill_id AND e.user_id=?)=1`
		args = append(args, userID)
	}
	if f.SourceType != "" {
		q += ` AND s.source_type=?`
		args = append(args, f.SourceType)
	}
	if f.Untagged {
		q += ` AND NOT EXISTS (SELECT 1 FROM skill_tags st WHERE st.user_id=? AND st.skill_id=sh.skill_id AND st.hidden=0)`
		args = append(args, userID)
	}
	if term != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(term)) + "%"
		q += ` AND (lower(s.name) LIKE ? ESCAPE '\' OR lower(s.id) LIKE ? ESCAPE '\' OR lower(s.description) LIKE ? ESCAPE '\' OR lower(sh.notes) LIKE ? ESCAPE '\')`
		args = append(args, like, like, like, like)
	}
	switch f.Sort {
	case "name":
		q += ` ORDER BY lower(s.name) ASC`
	case "installs":
		q += ` ORDER BY s.installs DESC, lower(s.name) ASC`
	default:
		q += ` ORDER BY sh.updated_at DESC, sh.created_at DESC`
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
	if err := s.attachMetaFor(userID, out); err != nil {
		return nil, c, err
	}
	return out, c, nil
}

func (s *Store) UpsertFor(userID int64, r ref.Ref, f Patch) (Skill, error) {
	return s.upsertFor(userID, r.ID, r.Source, r.Slug, r.SourceType, r.URL, r.Slug, f)
}

func (s *Store) UpsertLocalFor(userID int64, name string, f Patch) (Skill, error) {
	return s.upsertFor(userID, "local/"+name, "local", name, "local", "", name, f)
}

func (s *Store) upsertFor(userID int64, id, source, slug, typ, url, name string, f Patch) (Skill, error) {
	t := now()
	notes := ""
	if f.Notes != nil {
		notes = *f.Notes
	}
	if _, err := s.db.Exec(`INSERT INTO skills(`+cols+`) VALUES(?,?,?,?,?,?,'','',0,0,'',0,0,0,?,?)
		ON CONFLICT(id) DO NOTHING`, id, source, slug, typ, url, name, t, t); err != nil {
		return Skill{}, err
	}
	if _, err := s.db.Exec(`INSERT INTO shelf(user_id,skill_id,liked,bookmarked,used,notes,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id,skill_id) DO UPDATE SET
			updated_at=CASE WHEN (shelf.liked=0 AND excluded.liked=1) OR (shelf.bookmarked=0 AND excluded.bookmarked=1) OR (shelf.used=0 AND excluded.used=1) THEN excluded.updated_at ELSE shelf.updated_at END,
			liked=shelf.liked OR excluded.liked, bookmarked=shelf.bookmarked OR excluded.bookmarked, used=shelf.used OR excluded.used`,
		userID, id, b2i(f.Liked), b2i(f.Bookmarked), b2i(f.Used), notes, t, t); err != nil {
		return Skill{}, err
	}
	if f.Notes != nil {
		if _, err := s.db.Exec(`UPDATE shelf SET notes=? WHERE user_id=? AND skill_id=?`, *f.Notes, userID, id); err != nil {
			return Skill{}, err
		}
	}
	_ = s.applyTopicsFor(userID, id)
	sk, _, err := s.GetFor(userID, id)
	return sk, err
}

func (s *Store) PatchFor(userID int64, id string, p Patch) (Skill, error) {
	var notes sql.NullString
	if p.Notes != nil {
		notes = sql.NullString{String: *p.Notes, Valid: true}
	}
	res, err := s.db.Exec(`UPDATE shelf SET liked=COALESCE(?,liked), bookmarked=COALESCE(?,bookmarked), used=COALESCE(?,used), notes=COALESCE(?,notes), updated_at=?
		WHERE user_id=? AND skill_id=?`,
		nullBool(p.Liked), nullBool(p.Bookmarked), nullBool(p.Used), notes, now(), userID, id)
	if err != nil {
		return Skill{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Skill{}, ErrNotFound
	}
	k, _, err := s.GetFor(userID, id)
	return k, err
}

func (s *Store) removeShelfTx(tx *sql.Tx, userID int64, id string, keepCatalogue bool) error {
	if _, err := tx.Exec(`DELETE FROM shelf WHERE user_id=? AND skill_id=?`, userID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM skill_tags WHERE user_id=? AND skill_id=?`, userID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM skill_envs WHERE skill_id=? AND env_id IN (SELECT id FROM environments WHERE user_id=?)`, id, userID); err != nil {
		return err
	}
	if !keepCatalogue {
		_, err := tx.Exec(`DELETE FROM skills WHERE id=? AND NOT EXISTS (SELECT 1 FROM shelf WHERE skill_id=?)
			AND NOT EXISTS (SELECT 1 FROM list_skills WHERE skill_id=?)`, id, id, id)
		return err
	}
	return nil
}

func (s *Store) RemoveFor(userID int64, id string) error {
	return s.withTx(func(tx *sql.Tx) error { return s.removeShelfTx(tx, userID, id, false) })
}

func (s *Store) IgnoreFor(userID int64, id, name, reason string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO ignored(user_id,id,name,reason,at) VALUES(?,?,?,?,?)
			ON CONFLICT(user_id,id) DO UPDATE SET name=excluded.name, reason=excluded.reason, at=excluded.at`,
			userID, id, name, reason, now()); err != nil {
			return err
		}
		return s.removeShelfTx(tx, userID, id, false)
	})
}

func (s *Store) IsIgnoredFor(userID int64, id string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM ignored WHERE user_id=? AND id=?`, userID, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) UnignoreFor(userID int64, id string) error {
	_, err := s.db.Exec(`DELETE FROM ignored WHERE user_id=? AND id=?`, userID, id)
	return err
}

func (s *Store) ListIgnoredFor(userID int64) ([]Ignored, error) {
	rows, err := s.db.Query(`SELECT id,name,reason,at FROM ignored WHERE user_id=? ORDER BY at DESC`, userID)
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

func (s *Store) ResolveFor(userID int64, oldID string, r ref.Ref) (Skill, error) {
	old, ok, err := s.GetFor(userID, oldID)
	if err != nil {
		return Skill{}, err
	}
	if !ok {
		return Skill{}, ErrNotFound
	}
	nk, err := s.UpsertFor(userID, r, Patch{Liked: &old.Liked, Bookmarked: &old.Bookmarked, Used: &old.Used})
	if err != nil {
		return Skill{}, err
	}
	if nk.Notes == "" && old.Notes != "" {
		if nk, err = s.PatchFor(userID, nk.ID, Patch{Notes: &old.Notes}); err != nil {
			return Skill{}, err
		}
	}
	if oldID != nk.ID {
		if err := s.withTx(func(tx *sql.Tx) error {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO skill_tags(user_id,skill_id,tag_id,source,hidden)
				SELECT user_id,?,tag_id,source,hidden FROM skill_tags WHERE user_id=? AND skill_id=?`, nk.ID, userID, oldID); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path)
				SELECT ?,env_id,installed_at,last_seen,source_path FROM skill_envs
				WHERE skill_id=? AND env_id IN (SELECT id FROM environments WHERE user_id=?)`, nk.ID, oldID, userID); err != nil {
				return err
			}
			return s.removeShelfTx(tx, userID, oldID, false)
		}); err != nil {
			return Skill{}, err
		}
	}
	return nk, nil
}

func (s *Store) PatchManyFor(userID int64, ids []string, p Patch) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if p.Notes != nil {
		return 0, errors.New("bulk notes are not supported")
	}
	q := `UPDATE shelf SET liked=COALESCE(?,liked), bookmarked=COALESCE(?,bookmarked), used=COALESCE(?,used), updated_at=?
		WHERE user_id=? AND skill_id IN (` + placeholders(len(ids)) + `)`
	args := []any{nullBool(p.Liked), nullBool(p.Bookmarked), nullBool(p.Used), now(), userID}
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

func (s *Store) TagManyFor(userID int64, ids []string, add []string, remove []int64) (int, error) {
	err := s.withTx(func(tx *sql.Tx) error {
		var addIDs []int64
		for _, name := range add {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			tagID, err := upsertTagTx(tx, name)
			if err != nil {
				return err
			}
			addIDs = append(addIDs, tagID)
		}
		for _, skillID := range ids {
			for _, tagID := range addIDs {
				if _, err := tx.Exec(`INSERT INTO skill_tags(user_id,skill_id,tag_id,source,hidden) VALUES(?,?,?,'user',0)
					ON CONFLICT(user_id,skill_id,tag_id) DO UPDATE SET source='user', hidden=0`, userID, skillID, tagID); err != nil {
					return err
				}
			}
			for _, tagID := range remove {
				// Keep a tombstone even for user tags: a later topic sync must
				// not undo an explicit removal. Adding the tag clears hidden.
				if _, err := tx.Exec(`UPDATE skill_tags SET hidden=1 WHERE user_id=? AND skill_id=? AND tag_id=?`, userID, skillID, tagID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return len(ids), err
}

func (s *Store) RemoveManyFor(userID int64, ids []string, ignore bool) (int, error) {
	n := 0
	for _, id := range ids {
		sk, ok, err := s.GetFor(userID, id)
		if err != nil {
			return n, err
		}
		if !ok {
			continue
		}
		if ignore {
			if err := s.IgnoreFor(userID, id, sk.Name, "removed"); err != nil {
				return n, err
			}
		} else if err := s.RemoveFor(userID, id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Store) AddSkillTagsFor(userID int64, skillID string, names []string) error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			tagID, err := upsertTagTx(tx, name)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO skill_tags(user_id,skill_id,tag_id,source,hidden) VALUES(?,?,?,'user',0)
				ON CONFLICT(user_id,skill_id,tag_id) DO UPDATE SET source='user', hidden=0`, userID, skillID, tagID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) RemoveSkillTagFor(userID int64, skillID string, tagID int64) error {
	_, err := s.TagManyFor(userID, []string{skillID}, nil, []int64{tagID})
	return err
}

func (s *Store) ListTagsFor(userID int64) ([]Tag, error) {
	rows, err := s.db.Query(`SELECT t.id,t.name,COUNT(st.skill_id)
		FROM tags t LEFT JOIN skill_tags st ON st.tag_id=t.id AND st.user_id=? AND st.hidden=0
		GROUP BY t.id ORDER BY lower(t.name)`, userID)
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

func (s *Store) applyTopicsFor(userID int64, id string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO skill_tags(user_id,skill_id,tag_id,source,hidden)
		SELECT ?,?,tag_id,'topic',0 FROM topic_skills WHERE skill_id=?`, userID, id, id)
	return err
}

func (s *Store) ReplaceTopicsForAll(topics []Topic) error {
	users, err := s.ListUsers()
	if err != nil {
		return err
	}
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
			for _, user := range users {
				if len(topic.Skills) == 0 {
					if _, err := tx.Exec(`DELETE FROM skill_tags WHERE user_id=? AND tag_id=? AND source='topic' AND hidden=0`, user.ID, tagID); err != nil {
						return err
					}
					continue
				}
				args := []any{user.ID, tagID, user.ID}
				for _, id := range topic.Skills {
					args = append(args, id)
				}
				if _, err := tx.Exec(`DELETE FROM skill_tags WHERE user_id=? AND tag_id=? AND source='topic' AND hidden=0 AND (
					skill_id NOT IN (`+placeholders(len(topic.Skills))+`) OR NOT EXISTS (
						SELECT 1 FROM shelf sh WHERE sh.user_id=? AND sh.skill_id=skill_tags.skill_id))`, args...); err != nil {
					return err
				}
				args = []any{user.ID, tagID, user.ID}
				for _, id := range topic.Skills {
					args = append(args, id)
				}
				if _, err := tx.Exec(`INSERT OR IGNORE INTO skill_tags(user_id,skill_id,tag_id,source,hidden)
					SELECT ?,sh.skill_id,?,'topic',0 FROM shelf sh WHERE sh.user_id=? AND sh.skill_id IN (`+placeholders(len(topic.Skills))+`)`, args...); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *Store) ListEnvironmentsFor(userID int64) ([]Environment, error) {
	rows, err := s.db.Query(`SELECT e.id,e.name,e.kind,e.agent,e.host,e.path,e.project,e.first_seen,e.last_seen,COUNT(se.skill_id)
		FROM environments e LEFT JOIN skill_envs se ON se.env_id=e.id
		WHERE e.user_id=? GROUP BY e.id ORDER BY lower(e.name)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Environment{}
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.Name, &e.Kind, &e.Agent, &e.Host, &e.Path, &e.Project, &e.FirstSeen, &e.LastSeen, &e.SkillCount); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func EnvironmentIDFor(userID int64, host, agent, project string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	agent = strings.ToLower(strings.TrimSpace(agent))
	id := strconv.FormatInt(userID, 10) + "/" + host + "/" + agent
	if project = strings.TrimSpace(project); project != "" {
		if strings.HasPrefix(project, "/") {
			sum := sha256.Sum256([]byte(project))
			id += "/project-" + hex.EncodeToString(sum[:])
		} else {
			id += "/" + strings.ToLower(strings.ReplaceAll(project, "/", "-"))
		}
	}
	return id
}

func (s *Store) RecordEnvironmentFor(userID int64, skillID, host, agent, project, path, sourcePath string) error {
	host, err := s.ResolveMachineHostFor(userID, host)
	if err != nil {
		return err
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(agent) == "" {
		return nil
	}
	t := now()
	id := EnvironmentIDFor(userID, host, agent, project)
	if err := s.TouchEnvironmentFor(userID, host, agent, project, path); err != nil {
		return err
	}
	if sourcePath == "" {
		sourcePath = path
	}
	_, err = s.db.Exec(`INSERT INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path)
		VALUES(?,?,?,?,?)
		ON CONFLICT(skill_id,env_id) DO UPDATE SET last_seen=excluded.last_seen, source_path=excluded.source_path`,
		skillID, id, t, t, sourcePath)
	return err
}

func (s *Store) TouchEnvironmentFor(userID int64, host, agent, project, path string) error {
	host, err := s.ResolveMachineHostFor(userID, host)
	if err != nil {
		return err
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(agent) == "" {
		return nil
	}
	t := now()
	id := EnvironmentIDFor(userID, host, agent, project)
	name := EnvironmentName(host, agent, project)
	kind := "machine"
	if strings.TrimSpace(project) != "" {
		kind = "project"
	}
	_, err = s.db.Exec(`INSERT INTO environments(id,user_id,name,kind,agent,host,path,project,first_seen,last_seen)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET last_seen=excluded.last_seen, name=excluded.name,
			path=CASE WHEN environments.path='' THEN excluded.path ELSE environments.path END`,
		id, userID, name, kind, agent, host, path, project, t, t)
	return err
}

func (s *Store) PruneEnvironmentFor(userID int64, envID string, keep []string) (int64, error) {
	var owner int64
	if err := s.db.QueryRow(`SELECT user_id FROM environments WHERE id=?`, envID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	if owner != userID {
		return 0, nil
	}
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

func (s *Store) AddEventFor(userID int64, source, action, skillID string, detail any) error {
	detailJSON := ""
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		}
	}
	_, err := s.db.Exec(`INSERT INTO events(at,user_id,actor,source,action,skill_id,detail) VALUES(?,?,?,?,?,?,?)`,
		now(), userID, "", source, action, skillID, detailJSON)
	return err
}

func (s *Store) ListEventsFor(userID int64, limit int, skillID string) ([]Event, error) {
	return s.listEvents(`WHERE ev.user_id=?`, []any{userID}, limit, skillID)
}

func (s *Store) ListAllEvents(limit int, skillID string) ([]Event, error) {
	return s.listEvents(``, nil, limit, skillID)
}

func (s *Store) listEvents(where string, args []any, limit int, skillID string) ([]Event, error) {
	q := `SELECT ev.id,ev.at,ev.actor,ev.source,ev.action,ev.skill_id,COALESCE(s.name,''),ev.detail
		FROM events ev LEFT JOIN skills s ON s.id=ev.skill_id`
	var clauses []string
	if where != "" {
		clauses = append(clauses, strings.TrimPrefix(where, "WHERE "))
	}
	if skillID != "" {
		clauses = append(clauses, "ev.skill_id=?")
		args = append(args, skillID)
	}
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
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

func (s *Store) ExportFor(userID int64, includeEvents bool) (Snapshot, error) {
	skills, _, err := s.ListFor(userID, Filter{})
	if err != nil {
		return Snapshot{}, err
	}
	if err := s.attachHiddenTagsFor(userID, skills); err != nil {
		return Snapshot{}, err
	}
	ignored, err := s.ListIgnoredFor(userID)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Version: 2, ExportedAt: now(), Skills: skills, Ignored: ignored}
	if includeEvents {
		events, err := s.ListEventsFor(userID, 0, "")
		if err != nil {
			return Snapshot{}, err
		}
		snap.Events = events
	}
	return snap, nil
}

func (s *Store) RestoreSnapshotFor(userID int64, snapshot Snapshot) error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, k := range snapshot.Skills {
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
			trend := ""
			if len(k.Trend) > 0 {
				if b, err := json.Marshal(k.Trend); err == nil {
					trend = string(b)
				}
			}
			if _, err := tx.Exec(`INSERT INTO skills(`+cols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,0,0,0,?,?)
				ON CONFLICT(id) DO UPDATE SET name=CASE WHEN excluded.name='' THEN skills.name ELSE excluded.name END,
					description=CASE WHEN excluded.description='' THEN skills.description ELSE excluded.description END,
					installs=CASE WHEN excluded.installs>0 THEN excluded.installs ELSE skills.installs END,
					installs_week=CASE WHEN excluded.installs_week>0 THEN excluded.installs_week ELSE skills.installs_week END,
					trend=CASE WHEN excluded.trend='' THEN skills.trend ELSE excluded.trend END`,
				k.ID, k.Source, k.Slug, k.SourceType, k.URL, k.Name, k.Description, k.Notes, k.Installs, k.InstallsWeek, trend,
				k.CreatedAt, k.UpdatedAt); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT OR REPLACE INTO shelf(user_id,skill_id,liked,bookmarked,used,notes,created_at,updated_at)
				VALUES(?,?,?,?,?,?,?,?)`, userID, k.ID, b2i(&k.Liked), b2i(&k.Bookmarked), b2i(&k.Used), k.Notes, k.CreatedAt, k.UpdatedAt); err != nil {
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
				if _, err := tx.Exec(`INSERT OR REPLACE INTO skill_tags(user_id,skill_id,tag_id,source,hidden) VALUES(?,?,?,?,?)`,
					userID, k.ID, tagID, source, b2i(&tag.Hidden)); err != nil {
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
				if _, err := tx.Exec(`INSERT OR REPLACE INTO skill_tags(user_id,skill_id,tag_id,source,hidden) VALUES(?,?,?,?,1)`,
					userID, k.ID, tagID, source); err != nil {
					return err
				}
			}
			for _, env := range k.Environments {
				if env.Host == "" || env.Agent == "" {
					continue
				}
				canonical, err := resolveMachineHost(tx, userID, env.Host)
				if err != nil {
					return err
				}
				if canonical != env.Host {
					env.Host = canonical
					env.Name = EnvironmentName(canonical, env.Agent, env.Project)
				}
				id := EnvironmentIDFor(userID, env.Host, env.Agent, env.Project)
				name := env.Name
				if name == "" {
					name = EnvironmentName(env.Host, env.Agent, env.Project)
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
				if _, err := tx.Exec(`INSERT INTO environments(id,user_id,name,kind,agent,host,path,project,first_seen,last_seen)
					VALUES(?,?,?,?,?,?,?,?,?,?)
					ON CONFLICT(id) DO UPDATE SET name=excluded.name, path=CASE WHEN environments.path='' THEN excluded.path ELSE environments.path END,
						last_seen=MAX(environments.last_seen,excluded.last_seen)`,
					id, userID, name, kind, env.Agent, env.Host, env.Path, env.Project, first, last); err != nil {
					return err
				}
				if _, err := tx.Exec(`INSERT INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path)
					VALUES(?,?,?,?,?)
					ON CONFLICT(skill_id,env_id) DO UPDATE SET last_seen=excluded.last_seen, source_path=excluded.source_path`,
					k.ID, id, first, last, env.SourcePath); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`DELETE FROM ignored WHERE user_id=? AND id=?`, userID, k.ID); err != nil {
				return err
			}
		}
		for _, i := range snapshot.Ignored {
			if i.ID == "" {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO ignored(user_id,id,name,reason,at) VALUES(?,?,?,?,?)
				ON CONFLICT(user_id,id) DO UPDATE SET name=excluded.name, reason=excluded.reason, at=excluded.at`,
				userID, i.ID, i.Name, i.Reason, i.At); err != nil {
				return err
			}
		}
		return nil
	})
}

// APIToken is a named personal credential; the secret itself is never stored.
type APIToken struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"createdAt"`
	LastUsedAt int64  `json:"lastUsedAt,omitempty"`
}

func (s *Store) CreateAPIToken(userID int64, name, hash string) (APIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "token"
	}
	t := now()
	res, err := s.db.Exec(`INSERT INTO api_tokens(user_id,name,hash,created_at) VALUES(?,?,?,?)`, userID, name, hash, t)
	if err != nil {
		return APIToken{}, err
	}
	id, _ := res.LastInsertId()
	return APIToken{ID: id, Name: name, CreatedAt: t}, nil
}

func (s *Store) ListAPITokens(userID int64) ([]APIToken, error) {
	rows, err := s.db.Query(`SELECT id,name,created_at,last_used_at FROM api_tokens
		WHERE user_id=? AND revoked_at=0 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAPIToken(userID, id int64) error {
	res, err := s.db.Exec(`UPDATE api_tokens SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at=0`, now(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RevokeUserTokens(userID int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, table := range []string{"api_tokens", "mcp_connections"} {
			if _, err := tx.Exec(`UPDATE `+table+` SET revoked_at=? WHERE user_id=? AND revoked_at=0`, now(), userID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) HasActiveAPITokens(userID int64) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM api_tokens WHERE revoked_at=0 AND (?=0 OR user_id=?)`, userID, userID).Scan(&n)
	return n > 0, err
}

func (s *Store) APITokenUser(hash string, at int64) (User, bool, error) {
	var u User
	err := s.db.QueryRow(`SELECT u.id,u.email,u.name,u.role,u.password_hash,u.created_at,u.disabled_at
		FROM api_tokens t JOIN users u ON u.id=t.user_id
		WHERE t.hash=? AND t.revoked_at=0 AND u.disabled_at=0`, hash).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.CreatedAt, &u.DisabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	_, _ = s.db.Exec(`UPDATE api_tokens SET last_used_at=? WHERE hash=?`, at, hash)
	return u, true, nil
}

// Invite is a one-time account invitation, stored hashed.
type Invite struct {
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (s *Store) CreateInvite(hash, email, role string, createdBy, expiresAt int64) error {
	_, err := s.db.Exec(`INSERT INTO invites(token_hash,email,role,created_by,created_at,expires_at)
		VALUES(?,?,?,?,?,?)`, hash, normalizeEmail(email), role, createdBy, now(), expiresAt)
	return err
}

func (s *Store) InviteByHash(hash string, at int64) (Invite, bool, error) {
	var i Invite
	err := s.db.QueryRow(`SELECT email,role,expires_at FROM invites WHERE token_hash=? AND expires_at>?`, hash, at).
		Scan(&i.Email, &i.Role, &i.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Invite{}, false, nil
	}
	return i, err == nil, err
}

func (s *Store) DeleteInvite(hash string) error {
	_, err := s.db.Exec(`DELETE FROM invites WHERE token_hash=?`, hash)
	return err
}

func (s *Store) PruneInvites(at int64) error {
	_, err := s.db.Exec(`DELETE FROM invites WHERE expires_at<=?`, at)
	return err
}

// UserMeta is a user plus the admin list's last-active and token counts.
type UserMeta struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	CreatedAt  int64  `json:"createdAt"`
	DisabledAt int64  `json:"disabledAt,omitempty"`
	LastActive int64  `json:"lastActive,omitempty"`
	Tokens     int    `json:"tokens"`
}

func (s *Store) ListUsersMeta() ([]UserMeta, error) {
	rows, err := s.db.Query(`SELECT u.id,u.email,u.name,u.role,u.created_at,u.disabled_at,
		COALESCE((SELECT MAX(s.last_seen) FROM sessions s WHERE s.user_id=u.id),0),
		(SELECT COUNT(*) FROM api_tokens t WHERE t.user_id=u.id AND t.revoked_at=0)
		FROM users u ORDER BY lower(u.name), lower(u.email)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserMeta{}
	for rows.Next() {
		var u UserMeta
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt, &u.DisabledAt, &u.LastActive, &u.Tokens); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUserRole(id int64, role string) error {
	switch role {
	case "owner", "admin", "member", "viewer":
	default:
		return errors.New("invalid role")
	}
	res, err := s.db.Exec(`UPDATE users SET role=? WHERE id=?`, role, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetUserPassword(id int64, hash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.DeleteUserSessions(id)
}

func (s *Store) TransferOwner(currentOwnerID, targetID int64) error {
	if currentOwnerID == targetID {
		return errors.New("already the owner")
	}
	return s.withTx(func(tx *sql.Tx) error {
		var role, hash string
		var disabled int64
		if err := tx.QueryRow(`SELECT role,password_hash,disabled_at FROM users WHERE id=?`, targetID).Scan(&role, &hash, &disabled); err != nil {
			return err
		}
		if disabled != 0 {
			return errors.New("the target account is disabled")
		}
		// The owner signs in with a password and never through a provider, and
		// a passwordless account cannot add one, so the transfer would leave
		// the new owner with no way in.
		if hash == "" {
			return errors.New("the target account has no password; the owner signs in with one")
		}
		if _, err := tx.Exec(`UPDATE users SET role='admin' WHERE id=?`, currentOwnerID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE users SET role='owner' WHERE id=?`, targetID)
		return err
	})
}

func (s *Store) DeleteUser(id int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		var role string
		if err := tx.QueryRow(`SELECT role FROM users WHERE id=?`, id).Scan(&role); err != nil {
			return err
		}
		if role == "owner" {
			return errors.New("transfer ownership before deleting the owner")
		}
		for _, stmt := range []string{
			`DELETE FROM shelf WHERE user_id=?`,
			`DELETE FROM skill_tags WHERE user_id=?`,
			`DELETE FROM skill_envs WHERE env_id IN (SELECT id FROM environments WHERE user_id=?)`,
			`DELETE FROM environments WHERE user_id=?`,
			`DELETE FROM ignored WHERE user_id=?`,
			`DELETE FROM events WHERE user_id=?`,
			`DELETE FROM api_tokens WHERE user_id=?`,
			`DELETE FROM mcp_connections WHERE user_id=?`,
			`DELETE FROM mcp_access WHERE user_id=?`,
			`DELETE FROM sessions WHERE user_id=?`,
			`DELETE FROM invites WHERE created_by=?`,
			`DELETE FROM backup_remotes WHERE owner_id=?`,
			`DELETE FROM backup_settings WHERE owner_id=?`,
			`DELETE FROM backup_runs WHERE owner_id=?`,
			`DELETE FROM user_profiles WHERE user_id=?`,
			`DELETE FROM users WHERE id=?`,
		} {
			if _, err := tx.Exec(stmt, id); err != nil {
				return err
			}
		}
		return nil
	})
}
