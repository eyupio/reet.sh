package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// MaxFolderProjects is the most folder projects one account can have.
const MaxFolderProjects = 100

const maxFolderProjectPromptBytes = 2000

// maxFolderProjectNoteBytes bounds the stored note, a JSON list of saved
// Project names.
const maxFolderProjectNoteBytes = 4000

// noteNames reads a stored note: a JSON list of whole Project names.
// Anything else (never written today) reads as one name.
func noteNames(raw string) []string {
	if raw == "" {
		return nil
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return []string{raw}
	}
	return names
}

// encodeNote stores the names as a JSON list; none is "".
func encodeNote(names []string) string {
	if len(names) == 0 {
		return ""
	}
	b, _ := json.Marshal(names)
	return string(b)
}

// noteText is how a note reads: the names joined by ", ".
func noteText(raw string) string { return strings.Join(noteNames(raw), ", ") }

// maxFolderProjectHostBytes bounds Host, the machine's enrollment id
// (sync_agents.id), as remote agent authentication bounds ids.
const maxFolderProjectHostBytes = 128

var (
	ErrFolderProjectLimit    = fmt.Errorf("an account can have at most %d projects", MaxFolderProjects)
	ErrFolderProjectExists   = errors.New("that machine and folder is already a project")
	ErrFolderProjectChanged  = errors.New("the project changed since it was read")
	ErrFolderProjectName     = fmt.Errorf("a project name is 1 to %d characters of plain text, and no other project may have it", MaxProjectNameRunes)
	ErrFolderProjectNotFound = errors.New("project not found")
	// ErrFolderProjectInvalid wraps every other field refusal. Messages name
	// the field and the rule and never repeat a value.
	ErrFolderProjectInvalid = errors.New("invalid project")
)

// FolderProject is one folder on one machine. Host is the machine's
// enrollment id (sync_agents.id), never its name, so a rename keeps it.
// WorktreeDefault is "inherit"
// (use the person's setting), "yes" or "no". LegacyProjectID names the old
// multi-repository Project member it was made from ("" for a new project), so
// the migration can run again without making a second copy.
type FolderProject struct {
	ID, Host, Dir, Name                                                   string
	IsRepo                                                                bool
	BaseBranch, WorktreeDefault, DefaultTool, DefaultModel, StarterPrompt string
	Position                                                              int
	LegacyProjectID                                                       string
	// Note is a one-time notice from the migration of saved Projects, as it
	// reads: the names of the other Projects with this folder whose shared
	// prompts were not kept ("A, B"). It is stored as a list of whole names,
	// is written only by the migration, and an update can only clear it ("").
	Note                          string
	CreatedAt, UpdatedAt, Version int64
}

func (s *Store) migrateFolderProjects() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS folder_projects (
				id TEXT NOT NULL, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				host TEXT NOT NULL, dir TEXT NOT NULL,
				name TEXT NOT NULL, name_key TEXT NOT NULL,
				is_repo INTEGER NOT NULL DEFAULT 0,
				base_branch TEXT NOT NULL DEFAULT '',
				worktree_default TEXT NOT NULL DEFAULT 'inherit' CHECK(worktree_default IN ('inherit','yes','no')),
				default_tool TEXT NOT NULL DEFAULT '', default_model TEXT NOT NULL DEFAULT '',
				starter_prompt TEXT NOT NULL DEFAULT '',
				position INTEGER NOT NULL CHECK(position>=0),
				legacy_project_id TEXT,
				note TEXT NOT NULL DEFAULT '',
				created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
				version INTEGER NOT NULL DEFAULT 1,
				PRIMARY KEY(user_id,id))`,
			`CREATE UNIQUE INDEX IF NOT EXISTS folder_projects_name ON folder_projects(user_id,name_key)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS folder_projects_legacy ON folder_projects(user_id,legacy_project_id) WHERE legacy_project_id IS NOT NULL`,
			// Which folder project each member of a saved Project became
			// (legacy_key "<projectID>:<position>"), also when two Projects
			// share one folder or the folder was already registered; a
			// legacy_key of just "<projectID>" with an empty folder_project_id
			// marks a Project whose migration was audited. No foreign key: a
			// folder project the person removes stays removed.
			`CREATE TABLE IF NOT EXISTS folder_project_legacy (
				user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				legacy_key TEXT NOT NULL, folder_project_id TEXT NOT NULL,
				PRIMARY KEY(user_id,legacy_key))`,
			// Each account's saved Projects as last migrated (counts and
			// newest change): a start passes over an account that matches.
			`CREATE TABLE IF NOT EXISTS folder_project_migration (
				user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
				fingerprint TEXT NOT NULL)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		// Tables made before the migration notice gain the column.
		var hasNote int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('folder_projects') WHERE name='note'`).Scan(&hasNote); err != nil {
			return err
		}
		if hasNote == 0 {
			if _, err := tx.Exec(`ALTER TABLE folder_projects ADD COLUMN note TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
		// Hosts were once machine names (index folder_projects_machine, on
		// lower(host)). Where exactly one of the account's machines has the
		// name, and no row already holds that machine and folder, the name
		// becomes the machine's id; any other row is left as it was. Runs
		// once: the old index is dropped right after.
		var old int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='folder_projects_machine'`).Scan(&old); err != nil {
			return err
		}
		if old > 0 {
			if _, err := tx.Exec(`UPDATE folder_projects SET host=(SELECT a.id FROM sync_agents a WHERE a.user_id=folder_projects.user_id AND lower(a.host)=lower(folder_projects.host))
				WHERE NOT EXISTS(SELECT 1 FROM sync_agents a WHERE a.user_id=folder_projects.user_id AND a.id=folder_projects.host)
				AND (SELECT COUNT(*) FROM sync_agents a WHERE a.user_id=folder_projects.user_id AND lower(a.host)=lower(folder_projects.host))=1
				AND NOT EXISTS(SELECT 1 FROM folder_projects q JOIN sync_agents a ON a.user_id=q.user_id AND a.id=q.host
					WHERE q.user_id=folder_projects.user_id AND q.dir=folder_projects.dir AND lower(a.host)=lower(folder_projects.host))`); err != nil {
				return err
			}
			if _, err := tx.Exec(`DROP INDEX folder_projects_machine`); err != nil {
				return err
			}
		}
		// The machine is its enrollment id (sync_agents.id), compared exactly.
		_, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS folder_projects_agent_dir ON folder_projects(user_id,host,dir)`)
		return err
	})
}

func folderInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrFolderProjectInvalid, fmt.Sprintf(format, args...))
}

// normalizeFolderProject validates every field except the name, which needs
// the account's other names (see resolveFolderName). It returns the project as
// it will be stored. ID, Position, times and Version are the caller's to set.
func normalizeFolderProject(p FolderProject) (FolderProject, error) {
	return normalizeFolder(p, true)
}

// normalizeFolder is normalizeFolderProject; checkPrompt false leaves the
// starter prompt unchecked (an update that keeps the saved one).
func normalizeFolder(p FolderProject, checkPrompt bool) (FolderProject, error) {
	if p.Host == "" || len(p.Host) > maxFolderProjectHostBytes || strings.TrimSpace(p.Host) != p.Host || !plainText(p.Host) {
		return p, folderInvalid("machine id is required, at most %d bytes, plain text", maxFolderProjectHostBytes)
	}
	dir, err := CleanProjectDir(p.Dir)
	if err != nil {
		return p, fmt.Errorf("%w: %v", ErrFolderProjectInvalid, err)
	}
	p.Dir = dir
	p.Name = strings.TrimSpace(p.Name)
	if p.Name != "" && !validFolderProjectName(p.Name) {
		return p, ErrFolderProjectName
	}
	if p.BaseBranch != "" && !terminalproto.ValidRefName(p.BaseBranch) {
		return p, folderInvalid("base branch must be a valid git branch name of at most 255 bytes")
	}
	switch p.WorktreeDefault {
	case "":
		p.WorktreeDefault = "inherit"
	case "inherit", "yes", "no":
	default:
		return p, folderInvalid("worktree default must be inherit, yes or no")
	}
	if p.DefaultTool != "" && !terminalproto.ValidTool(p.DefaultTool) {
		return p, folderInvalid("default agent must be one of claude, codex, opencode or gemini")
	}
	if p.DefaultModel != "" && !terminalproto.ValidModel(p.DefaultModel) {
		return p, folderInvalid("default model has characters or a length the connector would refuse")
	}
	if checkPrompt && p.StarterPrompt != "" && (len(p.StarterPrompt) > maxFolderProjectPromptBytes || strings.HasPrefix(p.StarterPrompt, "-") || !terminalproto.ValidPrompt(p.StarterPrompt)) {
		return p, folderInvalid("starter prompt must be valid text of at most %d bytes with no control characters except newline, and must not start with -", maxFolderProjectPromptBytes)
	}
	return p, nil
}

func validFolderProjectName(n string) bool {
	return utf8.RuneCountInString(n) >= 1 && utf8.RuneCountInString(n) <= MaxProjectNameRunes && plainText(n)
}

// resolveFolderName settles the name inside the transaction. An explicit name
// that another project of the account has is ErrFolderProjectName. An empty
// name becomes the folder's name, with " (2)", " (3)"... on a clash. self is
// the project being updated (its own name is not a clash), or "".
func resolveFolderName(tx *sql.Tx, uid int64, self string, p FolderProject) (string, error) {
	taken := func(name string) (bool, error) {
		var n int
		err := tx.QueryRow(`SELECT COUNT(*) FROM folder_projects WHERE user_id=? AND name_key=? AND id<>?`, uid, nameKey(name), self).Scan(&n)
		return n > 0, err
	}
	if p.Name != "" {
		t, err := taken(p.Name)
		if err != nil {
			return "", err
		}
		if t {
			return "", ErrFolderProjectName
		}
		return p.Name, nil
	}
	base := path.Base(p.Dir)
	if r := []rune(base); len(r) > MaxProjectNameRunes {
		base = string(r[:MaxProjectNameRunes])
	}
	if base == "/" || base == "." || !validFolderProjectName(base) {
		base = "project"
	}
	for i := 1; i < MaxFolderProjects+2; i++ {
		name := base
		if i > 1 {
			suffix := fmt.Sprintf(" (%d)", i)
			r := []rune(base)
			if len(r)+len([]rune(suffix)) > MaxProjectNameRunes {
				r = r[:MaxProjectNameRunes-len([]rune(suffix))]
			}
			name = string(r) + suffix
		}
		t, err := taken(name)
		if err != nil {
			return "", err
		}
		if !t {
			return name, nil
		}
	}
	return "", ErrFolderProjectName
}

// folderConflict maps a unique-index refusal to the matching error.
func folderConflict(err error) error {
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return err
	}
	if strings.Contains(err.Error(), "name_key") {
		return ErrFolderProjectName
	}
	return ErrFolderProjectExists
}

const folderColumns = `id,host,dir,name,is_repo,base_branch,worktree_default,default_tool,default_model,starter_prompt,position,COALESCE(legacy_project_id,''),note,created_at,updated_at,version`

type rowScanner interface{ Scan(dest ...any) error }

func scanFolderProject(r rowScanner) (FolderProject, error) {
	var p FolderProject
	err := r.Scan(&p.ID, &p.Host, &p.Dir, &p.Name, &p.IsRepo, &p.BaseBranch, &p.WorktreeDefault, &p.DefaultTool, &p.DefaultModel,
		&p.StarterPrompt, &p.Position, &p.LegacyProjectID, &p.Note, &p.CreatedAt, &p.UpdatedAt, &p.Version)
	p.Note = noteText(p.Note)
	return p, err
}

// CreateFolderProjectFor stores a new project at the end of uid's list. The
// ID, Position, times and Version are assigned here; given ones are ignored.
func (s *Store) CreateFolderProjectFor(uid int64, p FolderProject) (FolderProject, error) {
	p, err := normalizeFolderProject(p)
	if err != nil {
		return FolderProject{}, err
	}
	var saved FolderProject
	err = s.withTx(func(tx *sql.Tx) error {
		if err := s.userExists(tx, uid); err != nil {
			return ErrFolderProjectNotFound
		}
		var note []string
		if p.Note != "" {
			note = []string{p.Note}
		}
		saved, err = insertFolderProject(tx, uid, p, encodeNote(note))
		return err
	})
	if err != nil {
		return FolderProject{}, err
	}
	return saved, nil
}

// insertFolderProject stores p, already normalized, at the end of uid's list
// inside tx, with note as stored (see encodeNote): the limit, position and
// name are settled there.
func insertFolderProject(tx *sql.Tx, uid int64, p FolderProject, note string) (FolderProject, error) {
	var err error
	if p.ID, err = projectRandomID(); err != nil {
		return FolderProject{}, err
	}
	t := now()
	p.CreatedAt, p.UpdatedAt, p.Version = t, t, 1
	var n, next int
	if err := tx.QueryRow(`SELECT COUNT(*),COALESCE(MAX(position)+1,0) FROM folder_projects WHERE user_id=?`, uid).Scan(&n, &next); err != nil {
		return FolderProject{}, err
	}
	if n >= MaxFolderProjects {
		return FolderProject{}, ErrFolderProjectLimit
	}
	p.Position = next
	if p.Name, err = resolveFolderName(tx, uid, "", p); err != nil {
		return FolderProject{}, err
	}
	var legacy any
	if p.LegacyProjectID != "" {
		legacy = p.LegacyProjectID
	}
	_, err = tx.Exec(`INSERT INTO folder_projects(id,user_id,host,dir,name,name_key,is_repo,base_branch,worktree_default,default_tool,default_model,starter_prompt,position,legacy_project_id,note,created_at,updated_at,version)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1)`,
		p.ID, uid, p.Host, p.Dir, p.Name, nameKey(p.Name), p.IsRepo, p.BaseBranch, p.WorktreeDefault, p.DefaultTool, p.DefaultModel, p.StarterPrompt, p.Position, legacy, note, t, t)
	if err != nil {
		return FolderProject{}, folderConflict(err)
	}
	p.Note = noteText(note)
	return p, nil
}

// UpdateFolderProjectFor replaces the editable fields of project p.ID of uid.
// Position, LegacyProjectID and CreatedAt are kept. A non-zero expect is the
// version the caller read: a different stored one is ErrFolderProjectChanged.
// Zero means the last write wins. Either way the version goes up by one.
// The starter prompt is checked only when it changes, so a saved one that
// today's rules refuse does not block editing the other fields. The note can
// only be cleared: an empty Note clears it, any other value keeps it.
func (s *Store) UpdateFolderProjectFor(uid int64, p FolderProject, expect int64) (FolderProject, error) {
	id := p.ID
	if _, err := normalizeFolder(p, false); err != nil {
		return FolderProject{}, err
	}
	err := s.withTx(func(tx *sql.Tx) error {
		var cur FolderProject
		cur, err := scanFolderProject(tx.QueryRow(`SELECT `+folderColumns+` FROM folder_projects WHERE id=? AND user_id=?`, id, uid))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFolderProjectNotFound
		} else if err != nil {
			return err
		}
		if expect != 0 && expect != cur.Version {
			return ErrFolderProjectChanged
		}
		if p, err = normalizeFolder(p, p.StarterPrompt != cur.StarterPrompt); err != nil {
			return err
		}
		if p.Note != "" {
			p.Note = cur.Note
		}
		if p.Name, err = resolveFolderName(tx, uid, id, p); err != nil {
			return err
		}
		t := now()
		if t < cur.CreatedAt {
			t = cur.CreatedAt
		}
		p.ID, p.Position, p.LegacyProjectID, p.CreatedAt, p.UpdatedAt, p.Version = id, cur.Position, cur.LegacyProjectID, cur.CreatedAt, t, cur.Version+1
		res, err := tx.Exec(`UPDATE folder_projects SET host=?,dir=?,name=?,name_key=?,is_repo=?,base_branch=?,worktree_default=?,default_tool=?,default_model=?,starter_prompt=?,note=CASE WHEN ?='' THEN '' ELSE note END,updated_at=?,version=? WHERE id=? AND user_id=? AND version=?`,
			p.Host, p.Dir, p.Name, nameKey(p.Name), p.IsRepo, p.BaseBranch, p.WorktreeDefault, p.DefaultTool, p.DefaultModel, p.StarterPrompt, p.Note, t, p.Version, id, uid, cur.Version)
		if err != nil {
			return folderConflict(err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrFolderProjectChanged
		}
		return nil
	})
	if err != nil {
		return FolderProject{}, err
	}
	return p, nil
}

// DeleteFolderProjectFor removes the project. A non-zero expect must match the
// stored version (ErrFolderProjectChanged otherwise); zero has no precondition.
func (s *Store) DeleteFolderProjectFor(uid int64, id string, expect int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		var version int64
		if err := tx.QueryRow(`SELECT version FROM folder_projects WHERE id=? AND user_id=?`, id, uid).Scan(&version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrFolderProjectNotFound
			}
			return err
		}
		if expect != 0 && expect != version {
			return ErrFolderProjectChanged
		}
		res, err := tx.Exec(`DELETE FROM folder_projects WHERE id=? AND user_id=? AND version=?`, id, uid, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrFolderProjectChanged
		}
		return nil
	})
}

// FolderProjectFor returns one of uid's projects.
func (s *Store) FolderProjectFor(uid int64, id string) (FolderProject, error) {
	p, err := scanFolderProject(s.db.QueryRow(`SELECT `+folderColumns+` FROM folder_projects WHERE id=? AND user_id=?`, id, uid))
	if errors.Is(err, sql.ErrNoRows) {
		return FolderProject{}, ErrFolderProjectNotFound
	}
	return p, err
}

// ListFolderProjectsFor returns uid's projects in rail order (never nil).
func (s *Store) ListFolderProjectsFor(uid int64) ([]FolderProject, error) {
	rows, err := s.db.Query(`SELECT `+folderColumns+` FROM folder_projects WHERE user_id=? ORDER BY position,created_at,id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FolderProject{}
	for rows.Next() {
		p, err := scanFolderProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ReorderFolderProjectsFor puts uid's projects in the order of ids, which must
// name every project of the account once. versions[i] is the version the
// caller read for ids[i]; if any differs (or a project was added or deleted
// meanwhile) nothing moves and the result is ErrFolderProjectChanged. A
// project whose place changes gets a new version.
func (s *Store) ReorderFolderProjectsFor(uid int64, ids []string, versions []int64) error {
	if len(ids) != len(versions) {
		return folderInvalid("every project needs the version that was read")
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if seen[id] {
			return folderInvalid("a project appears twice")
		}
		seen[id] = true
		if versions[i] < 1 {
			return folderInvalid("every project needs the version that was read")
		}
	}
	return s.withTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id,position,version FROM folder_projects WHERE user_id=?`, uid)
		if err != nil {
			return err
		}
		type cur struct {
			pos int
			ver int64
		}
		have := map[string]cur{}
		for rows.Next() {
			var id string
			var c cur
			if err := rows.Scan(&id, &c.pos, &c.ver); err != nil {
				rows.Close()
				return err
			}
			have[id] = c
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			if _, ok := have[id]; !ok {
				// An account with projects reads a missing one as a view
				// gone stale (another tab removed it); one with none has
				// nothing to reorder. Another account's id is just unknown.
				if len(have) > 0 {
					return ErrFolderProjectChanged
				}
				return ErrFolderProjectNotFound
			}
		}
		if len(have) != len(ids) {
			return ErrFolderProjectChanged
		}
		for i, id := range ids {
			if have[id].ver != versions[i] {
				return ErrFolderProjectChanged
			}
		}
		// Positions are unique only by convention, so moving one at a time is
		// safe; the whole change still commits or rolls back as one.
		t := now()
		for i, id := range ids {
			if have[id].pos == i {
				continue
			}
			res, err := tx.Exec(`UPDATE folder_projects SET position=?,updated_at=MAX(updated_at,?),version=version+1 WHERE id=? AND user_id=? AND version=?`,
				i, t, id, uid, versions[i])
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n != 1 {
				return ErrFolderProjectChanged
			}
		}
		return nil
	})
}
