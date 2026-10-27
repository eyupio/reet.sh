package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Limits for Projects. They are the contract the API and the pages repeat.
const (
	MaxProjectsPerAccount = 50
	MaxProjectMembers     = 12
	MaxProjectNameRunes   = 80
	MaxRunsListed         = 20
	maxHostBytes          = 255
	maxSessionIDBytes     = 128
	maxWorktreeBytes      = 4096
	maxBranchBytes        = 255
	maxCodeBytes          = 64
)

// Run member states. A member only moves forward: pending to started or
// failed, and started to removed when its worktree is removed. The one way
// back is an explicit retry of a failed member.
const (
	RunMemberPending = "pending"
	RunMemberStarted = "started"
	RunMemberFailed  = "failed"
	// RunMemberRemoved: the member's worktree was removed through reet. The
	// member had started; it never moves again.
	RunMemberRemoved = "removed"
)

// Derived run states.
const (
	RunRunning  = "running"
	RunComplete = "complete"
	RunPartial  = "partial"
	RunFailed   = "failed"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrProjectLimit    = fmt.Errorf("an account can have at most %d Projects", MaxProjectsPerAccount)
	ErrProjectName     = fmt.Errorf("a Project name is 1 to %d characters of plain text", MaxProjectNameRunes)
	// ErrProjectNameTaken also satisfies errors.Is(err, ErrProjectName).
	ErrProjectNameTaken = fmt.Errorf("%w: another Project on this account already has that name", ErrProjectName)
	ErrMemberLimit      = fmt.Errorf("a Project can have at most %d repositories", MaxProjectMembers)
	ErrDuplicateMember  = errors.New("duplicate repository: a machine and folder can appear in a Project only once")
	// ErrProjectInvalid wraps every other field refusal; the message names the
	// field and the rule and never repeats the value (a prompt is never echoed).
	ErrProjectInvalid = errors.New("invalid Project")
	// ErrProjectStale refuses an update whose expected version is not the
	// stored one: someone else saved the Project in between.
	ErrProjectStale = errors.New("the Project changed since it was read")

	ErrRunNotFound         = errors.New("run not found")
	ErrRunNoMembers        = errors.New("a run needs at least one repository")
	ErrRunMemberNotPending = errors.New("run member is not pending, or is already being started")
	ErrRunMemberTransition = errors.New("run member cannot move to that state")
	ErrRunMemberStale      = errors.New("run member attempt does not match the stored attempt")
	ErrRunMemberNotFailed  = errors.New("only a failed run member can be retried")
	ErrRunRequestChanged   = errors.New("that start request id was used with other repositories")
	ErrRunInProgress       = errors.New("an earlier run of this Project is still starting")
	// ErrSelectionSize refuses a start in several projects that does not name
	// MinSelectionProjects to MaxSelectionProjects different projects.
	ErrSelectionSize = fmt.Errorf("a start in several projects names %d to %d different projects", MinSelectionProjects, MaxSelectionProjects)
)

// Where a run came from: a saved Project (a set of repositories, kept to read
// for one more release) or a selection of folder projects in the rail.
const (
	RunOriginSet       = "set"
	RunOriginSelection = "selection"
)

// How many folder projects one start in several projects names.
const (
	MinSelectionProjects = 2
	MaxSelectionProjects = 12
)

// RunLaunch is what a start in several projects sends to every member: the
// agent, its model and effort, the opening prompt, whether each repository
// gets a fresh worktree, and YOLO. The prompt is user text: it never reaches
// audit, logs, errors or JSON.
type RunLaunch struct {
	Tool, Model, Effort string
	Prompt              string `json:"-"`
	Worktree, Yolo      bool
}

type ProjectMember struct {
	Host     string
	Dir      string
	Tool     string
	Model    string
	Position int
}

// ProjectMemberError is a refusal of one repository's field. Index is the
// member's place in the list (0-based); it unwraps to ErrProjectInvalid.
type ProjectMemberError struct {
	Index int
	Field string
	Err   error
}

func (e *ProjectMemberError) Error() string {
	return fmt.Sprintf("repository %d: %v", e.Index+1, e.Err)
}
func (e *ProjectMemberError) Unwrap() error { return e.Err }

type Project struct {
	ID           string
	Name         string
	DefaultTool  string
	DefaultModel string
	SharedPrompt string
	UseWorktrees bool
	Members      []ProjectMember
	CreatedAt    int64
	UpdatedAt    int64
	// Version starts at 1 and goes up by one on every update. Given to
	// UpdateProjectFor (non-zero), it is the version the caller read, and the
	// update is refused with ErrProjectStale when the stored one differs.
	Version int64
}

// RunMember is one repository of a run, snapshotted when the run was created.
// Tool and Model are the resolved values. Position is the Project member's
// position at that time, so a subset run keeps stable positions for retries.
// WorktreeToken is derived by the orchestrator per member (never per attempt)
// and survives retries; the other worktree fields are what the machine reported.
type RunMember struct {
	Position       int
	Host           string
	Dir            string
	Tool           string
	Model          string
	SessionID      string
	WorktreeToken  string
	WorktreePath   string
	WorktreeRepo   string
	WorktreeBranch string
	State          string
	Code           string
	Attempt        int
	// Sent is true from just before the attempt's create is handed to the
	// broker until that create is known to have started nothing. While it is
	// set, a retry must reuse the attempt (the same request id): an agent may
	// be running, and a new request id with the same worktree token would
	// start a second one in the same worktree.
	Sent bool
	// For a selection run only: the folder project the member was made from,
	// its name and base branch then, and NoWorktree for a folder that is not a
	// repository (it works in its folder although the run makes worktrees).
	ProjectID  string
	Name       string
	Base       string
	NoWorktree bool
}

// InWorktree reports whether the member of r starts in a fresh worktree.
func (m RunMember) InWorktree(r Run) bool { return r.UseWorktrees && !m.NoWorktree }

// Run is a historical record. Prompt is the shared prompt as it was when the
// run started; it is kept for the orchestrator and must never reach audit,
// logs, errors or JSON.
type Run struct {
	ID string
	// Origin is RunOriginSet or RunOriginSelection.
	Origin string
	// ProjectID is the saved Project of a set run (empty once the Project is
	// deleted); a selection run has none, and each member names its own.
	ProjectID string
	// ProjectName is the Project's name, or a selection's names joined.
	ProjectName  string
	State        string
	Prompt       string `json:"-"`
	UseWorktrees bool
	// Effort and Yolo are a selection run's launch options for every member.
	Effort    string
	Yolo      bool
	Members   []RunMember
	CreatedAt int64
}

// HasPrompt reports whether the run carries a shared prompt, without exposing it.
func (r Run) HasPrompt() bool { return r.Prompt != "" }

// PendingRun names a run that still has pending members.
type PendingRun struct {
	UID   int64
	RunID string
}

type claimKey struct {
	uid      int64
	run      string
	position int
}

// RunClaimTTL is how long a claim holds a member that is still pending. A
// caller that vanished without deciding or releasing cannot block it for ever.
const RunClaimTTL = 10 * time.Minute

type runClaims struct {
	mu  sync.Mutex
	m   map[claimKey]int64 // claim time, Unix milliseconds
	now func() int64       // injectable clock for tests; nil means the wall clock
}

func (c *runClaims) take(k claimKey) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := now()
	if c.now != nil {
		t = c.now()
	}
	if at, held := c.m[k]; held && t-at < RunClaimTTL.Milliseconds() {
		return false
	}
	if c.m == nil {
		c.m = map[claimKey]int64{}
	}
	for old, at := range c.m { // sweep expired claims
		if t-at >= RunClaimTTL.Milliseconds() {
			delete(c.m, old)
		}
	}
	c.m[k] = t
	return true
}

func (c *runClaims) release(k claimKey) {
	c.mu.Lock()
	delete(c.m, k)
	c.mu.Unlock()
}

func (s *Store) migrateProjects() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS projects (
				id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				name TEXT NOT NULL, name_key TEXT NOT NULL,
				default_tool TEXT NOT NULL, default_model TEXT NOT NULL DEFAULT '',
				shared_prompt TEXT NOT NULL DEFAULT '', use_worktrees INTEGER NOT NULL DEFAULT 1,
				created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
				version INTEGER NOT NULL DEFAULT 1,
				UNIQUE(user_id,name_key), UNIQUE(id,user_id))`,
			`CREATE TABLE IF NOT EXISTS project_members (
				project_id TEXT NOT NULL, user_id INTEGER NOT NULL, position INTEGER NOT NULL CHECK(position>=0),
				host TEXT NOT NULL, dir TEXT NOT NULL, tool TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
				PRIMARY KEY(project_id,position),
				FOREIGN KEY(project_id,user_id) REFERENCES projects(id,user_id) ON DELETE CASCADE)`,
			// project_id is cleared, not cascaded, so deleting a Project keeps its runs.
			`CREATE INDEX IF NOT EXISTS project_members_user ON project_members(user_id,project_id)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS project_members_machine ON project_members(project_id,lower(host),dir)`,
			`CREATE TABLE IF NOT EXISTS project_runs (
				id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
				project_name TEXT NOT NULL, prompt TEXT NOT NULL DEFAULT '', use_worktrees INTEGER NOT NULL,
				created_at INTEGER NOT NULL, request_id TEXT NOT NULL DEFAULT '', UNIQUE(id,user_id))`,
			`CREATE INDEX IF NOT EXISTS project_runs_project ON project_runs(user_id,project_id,created_at)`,
			// A start request id names one run per account and Project.
			`CREATE UNIQUE INDEX IF NOT EXISTS project_runs_request ON project_runs(user_id,project_id,request_id) WHERE request_id<>''`,
			`CREATE TABLE IF NOT EXISTS project_run_members (
				run_id TEXT NOT NULL, user_id INTEGER NOT NULL, position INTEGER NOT NULL CHECK(position>=0),
				host TEXT NOT NULL, dir TEXT NOT NULL, tool TEXT NOT NULL, model TEXT NOT NULL DEFAULT '',
				state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','started','failed','removed')),
				code TEXT NOT NULL DEFAULT '', session_id TEXT NOT NULL DEFAULT '',
				worktree_token TEXT NOT NULL DEFAULT '', worktree_path TEXT NOT NULL DEFAULT '',
				worktree_repo TEXT NOT NULL DEFAULT '', worktree_branch TEXT NOT NULL DEFAULT '',
				attempt INTEGER NOT NULL DEFAULT 1 CHECK(attempt>=1),
				sent INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY(run_id,position),
				FOREIGN KEY(run_id,user_id) REFERENCES project_runs(id,user_id) ON DELETE CASCADE)`,
			`CREATE INDEX IF NOT EXISTS project_run_members_pending ON project_run_members(state,user_id,run_id)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		// Runs made before selection runs gain their columns.
		for _, c := range [][3]string{
			{"project_runs", "origin", `TEXT NOT NULL DEFAULT 'set'`},
			{"project_runs", "effort", `TEXT NOT NULL DEFAULT ''`},
			{"project_runs", "yolo", `INTEGER NOT NULL DEFAULT 0`},
			{"project_run_members", "folder_project_id", `TEXT NOT NULL DEFAULT ''`},
			{"project_run_members", "name", `TEXT NOT NULL DEFAULT ''`},
			{"project_run_members", "base", `TEXT NOT NULL DEFAULT ''`},
			{"project_run_members", "no_worktree", `INTEGER NOT NULL DEFAULT 0`},
		} {
			var n int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, c[0], c[1]).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				if _, err := tx.Exec(`ALTER TABLE ` + c[0] + ` ADD COLUMN ` + c[1] + ` ` + c[2]); err != nil {
					return err
				}
			}
		}
		// A start request id names one selection run per account.
		if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS project_runs_selection_request ON project_runs(user_id,request_id) WHERE origin='selection' AND request_id<>''`); err != nil {
			return err
		}
		// Tables made before versions existed gain the column.
		var have int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name='version'`).Scan(&have); err != nil {
			return err
		}
		if have == 0 {
			if _, err := tx.Exec(`ALTER TABLE projects ADD COLUMN version INTEGER NOT NULL DEFAULT 1`); err != nil {
				return err
			}
		}
		return nil
	})
}

type dbQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func projectRandomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrProjectInvalid, fmt.Sprintf(format, args...))
}

func plainText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false
		}
	}
	return true
}

// hostKey is how the server compares machine hosts (see EnvironmentID).
func hostKey(host string) string { return strings.ToLower(strings.TrimSpace(host)) }

func nameKey(name string) string { return strings.ToLower(name) }

// CleanProjectDir cleans a member folder with path.Clean semantics. It refuses
// anything terminalproto would refuse as a Dir, and every relative path.
func CleanProjectDir(dir string) (string, error) {
	if !terminalproto.ValidDir(dir) || !plainText(dir) {
		return "", invalid("folder must be an absolute path (starting with /) of at most %d bytes with no control characters", terminalproto.MaxDirBytes)
	}
	// Cleaning an absolute path keeps it absolute and never lengthens it.
	return path.Clean(dir), nil
}

// normalizeProject validates every field and returns the Project as it will be
// stored: trimmed name, cleaned folders, positions 0..n-1 in the given order.
func normalizeProject(p Project) (Project, error) {
	p.Name = strings.TrimSpace(p.Name)
	if n := utf8.RuneCountInString(p.Name); n < 1 || n > MaxProjectNameRunes || !plainText(p.Name) {
		return p, ErrProjectName
	}
	if !terminalproto.ValidTool(p.DefaultTool) {
		return p, invalid("default agent must be one of claude, codex, opencode or gemini")
	}
	if p.DefaultModel != "" && !terminalproto.ValidModel(p.DefaultModel) {
		return p, invalid("default model has characters or a length the connector would refuse")
	}
	if p.SharedPrompt != "" && !terminalproto.ValidPrompt(p.SharedPrompt) {
		return p, invalid("shared prompt must be valid text of at most %d bytes with no control characters except newline, and must not start with -", terminalproto.MaxPromptBytes)
	}
	if len(p.Members) > MaxProjectMembers {
		return p, ErrMemberLimit
	}
	members := make([]ProjectMember, len(p.Members))
	seen := map[[2]string]bool{}
	for i, m := range p.Members {
		m.Host = strings.TrimSpace(m.Host)
		if m.Host == "" || len(m.Host) > maxHostBytes || !plainText(m.Host) {
			return p, invalid("repository %d: machine is required, at most %d bytes, plain text", i+1, maxHostBytes)
		}
		dir, err := CleanProjectDir(m.Dir)
		if err != nil {
			return p, &ProjectMemberError{Index: i, Field: "dir", Err: err}
		}
		if m.Tool != "" && !terminalproto.ValidTool(m.Tool) {
			return p, invalid("repository %d: agent must be one of claude, codex, opencode or gemini", i+1)
		}
		if m.Model != "" && !terminalproto.ValidModel(m.Model) {
			return p, invalid("repository %d: model has characters or a length the connector would refuse", i+1)
		}
		// The folder comparison is case-sensitive: right on Linux, but two
		// spellings of one folder on a case-insensitive macOS file system are
		// not caught (the controller cannot see the machine's file system).
		key := [2]string{hostKey(m.Host), dir}
		if seen[key] {
			return p, fmt.Errorf("%w (machine %q, folder %q)", ErrDuplicateMember, m.Host, dir)
		}
		seen[key] = true
		members[i] = ProjectMember{Host: m.Host, Dir: dir, Tool: m.Tool, Model: m.Model, Position: i}
		// A single word could be read as a subcommand by the tool, which the
		// connector refuses; refuse it at save time instead of at every run.
		tool := m.Tool
		if tool == "" {
			tool = p.DefaultTool
		}
		if terminalproto.PromptNeedsWords(tool, p.SharedPrompt) {
			return p, invalid("shared prompt must be more than one word for %s", tool)
		}
	}
	if len(members) == 0 && terminalproto.PromptNeedsWords(p.DefaultTool, p.SharedPrompt) {
		return p, invalid("shared prompt must be more than one word for %s", p.DefaultTool)
	}
	p.Members = members
	return p, nil
}

func (s *Store) userExists(q dbQuerier, uid int64) error {
	var one int
	err := q.QueryRow(`SELECT 1 FROM users WHERE id=?`, uid).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProjectNotFound
	}
	return err
}

func isNameConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "name_key")
}

func insertMembers(tx *sql.Tx, uid int64, id string, members []ProjectMember) error {
	for _, m := range members {
		if _, err := tx.Exec(`INSERT INTO project_members(project_id,user_id,position,host,dir,tool,model) VALUES(?,?,?,?,?,?,?)`,
			id, uid, m.Position, m.Host, m.Dir, m.Tool, m.Model); err != nil {
			return err
		}
	}
	return nil
}

// CreateProjectFor stores a new Project for uid. The Project's ID, times and
// member positions are assigned here; any given ID is ignored.
func (s *Store) CreateProjectFor(uid int64, p Project) (Project, error) {
	p, err := normalizeProject(p)
	if err != nil {
		return Project{}, err
	}
	id, err := projectRandomID()
	if err != nil {
		return Project{}, err
	}
	t := now()
	p.ID, p.CreatedAt, p.UpdatedAt, p.Version = id, t, t, 1
	err = s.withTx(func(tx *sql.Tx) error {
		if err := s.userExists(tx, uid); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM projects WHERE user_id=?`, uid).Scan(&n); err != nil {
			return err
		}
		if n >= MaxProjectsPerAccount {
			return ErrProjectLimit
		}
		if _, err := tx.Exec(`INSERT INTO projects(id,user_id,name,name_key,default_tool,default_model,shared_prompt,use_worktrees,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			id, uid, p.Name, nameKey(p.Name), p.DefaultTool, p.DefaultModel, p.SharedPrompt, p.UseWorktrees, t, t); err != nil {
			if isNameConflict(err) {
				return ErrProjectNameTaken
			}
			return err
		}
		return insertMembers(tx, uid, id, p.Members)
	})
	if err != nil {
		return Project{}, err
	}
	return p, nil
}

// UpdateProjectFor replaces the Project p.ID of uid, members included, in one
// transaction: a refusal at any step leaves the old Project untouched. A
// non-zero p.Version is a precondition (compare-and-set); zero means the last
// write wins. Either way the stored version goes up by one.
func (s *Store) UpdateProjectFor(uid int64, p Project) (Project, error) {
	id, expect := p.ID, p.Version
	p, err := normalizeProject(p)
	if err != nil {
		return Project{}, err
	}
	p.ID = id
	t := now()
	err = s.withTx(func(tx *sql.Tx) error {
		var created, version int64
		if err := tx.QueryRow(`SELECT created_at,version FROM projects WHERE id=? AND user_id=?`, id, uid).Scan(&created, &version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrProjectNotFound
			}
			return err
		}
		if expect != 0 && expect != version {
			return ErrProjectStale
		}
		p.CreatedAt, p.Version = created, version+1
		if _, err := tx.Exec(`DELETE FROM project_members WHERE project_id=? AND user_id=?`, id, uid); err != nil {
			return err
		}
		if err := insertMembers(tx, uid, id, p.Members); err != nil {
			return err
		}
		if t < created {
			t = created
		}
		p.UpdatedAt = t
		res, err := tx.Exec(`UPDATE projects SET name=?,name_key=?,default_tool=?,default_model=?,shared_prompt=?,use_worktrees=?,updated_at=?,version=? WHERE id=? AND user_id=? AND version=?`,
			p.Name, nameKey(p.Name), p.DefaultTool, p.DefaultModel, p.SharedPrompt, p.UseWorktrees, t, version+1, id, uid, version)
		if err != nil {
			if isNameConflict(err) {
				return ErrProjectNameTaken
			}
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrProjectStale
		}
		return nil
	})
	if err != nil {
		return Project{}, err
	}
	return p, nil
}

// DeleteProjectFor removes the Project and its members. Its runs stay, with
// their name snapshot, and no longer name a Project.
func (s *Store) DeleteProjectFor(uid int64, id string) error { return s.DeleteProjectAtFor(uid, id, 0) }

// DeleteProjectAtFor deletes the Project only while its stored version is
// still expect, as an update does: a Project someone changed since it was
// read is refused with ErrProjectStale. An expect of 0 has no precondition.
func (s *Store) DeleteProjectAtFor(uid int64, id string, expect int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		var version int64
		if err := tx.QueryRow(`SELECT version FROM projects WHERE id=? AND user_id=?`, id, uid).Scan(&version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrProjectNotFound
			}
			return err
		}
		if expect != 0 && expect != version {
			return ErrProjectStale
		}
		res, err := tx.Exec(`DELETE FROM projects WHERE id=? AND user_id=? AND version=?`, id, uid, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrProjectStale
		}
		return nil
	})
}

// ProjectMayHaveWorktrees reports whether any run of uid's Project, however
// old, recorded a worktree for a member. It cannot say whether the worktree is
// still on the machine.
func (s *Store) ProjectMayHaveWorktrees(uid int64, id string) (bool, error) {
	var has bool
	err := s.withTx(func(tx *sql.Tx) error {
		var one int
		if err := tx.QueryRow(`SELECT 1 FROM projects WHERE id=? AND user_id=?`, id, uid).Scan(&one); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrProjectNotFound
			}
			return err
		}
		return tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM project_runs r JOIN project_run_members m ON m.run_id=r.id AND m.user_id=r.user_id
			WHERE r.user_id=? AND r.project_id=? AND m.worktree_path<>'')`, uid, id).Scan(&has)
	})
	return has, err
}

const projectCols = `id,name,default_tool,default_model,shared_prompt,use_worktrees,created_at,updated_at,version`

func scanProject(row interface{ Scan(...any) error }) (p Project, err error) {
	err = row.Scan(&p.ID, &p.Name, &p.DefaultTool, &p.DefaultModel, &p.SharedPrompt, &p.UseWorktrees, &p.CreatedAt, &p.UpdatedAt, &p.Version)
	return
}

func loadProjectMembers(q dbQuerier, uid int64, id string) ([]ProjectMember, error) {
	rows, err := q.Query(`SELECT position,host,dir,tool,model FROM project_members WHERE project_id=? AND user_id=? ORDER BY position`, id, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectMember{}
	for rows.Next() {
		var m ProjectMember
		if err := rows.Scan(&m.Position, &m.Host, &m.Dir, &m.Tool, &m.Model); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func loadProject(q dbQuerier, uid int64, id string) (Project, error) {
	p, err := scanProject(q.QueryRow(`SELECT `+projectCols+` FROM projects WHERE id=? AND user_id=?`, id, uid))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrProjectNotFound
	}
	if err != nil {
		return Project{}, err
	}
	p.Members, err = loadProjectMembers(q, uid, id)
	return p, err
}

func (s *Store) ProjectFor(uid int64, id string) (Project, error) {
	return loadProject(s.db, uid, id)
}

// ListProjectsFor returns every Project of uid, by name, with members.
func (s *Store) ListProjectsFor(uid int64) ([]Project, error) {
	var out []Project
	err := s.withTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT `+projectCols+` FROM projects WHERE user_id=? ORDER BY name_key, id`, uid)
		if err != nil {
			return err
		}
		index := map[string]int{}
		for rows.Next() {
			p, err := scanProject(rows)
			if err != nil {
				rows.Close()
				return err
			}
			p.Members = []ProjectMember{}
			index[p.ID] = len(out)
			out = append(out, p)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		mrows, err := tx.Query(`SELECT project_id,position,host,dir,tool,model FROM project_members WHERE user_id=? ORDER BY project_id,position`, uid)
		if err != nil {
			return err
		}
		defer mrows.Close()
		for mrows.Next() {
			var id string
			var m ProjectMember
			if err := mrows.Scan(&id, &m.Position, &m.Host, &m.Dir, &m.Tool, &m.Model); err != nil {
				return err
			}
			if i, ok := index[id]; ok {
				out[i].Members = append(out[i].Members, m)
			}
		}
		return mrows.Err()
	})
	if out == nil {
		out = []Project{}
	}
	return out, err
}

// resolvedLaunch applies a member's overrides to the Project defaults. A
// default model belongs to the default agent, so a member that switches agent
// without naming a model gets the agent's own default (empty).
func resolvedLaunch(p Project, m ProjectMember) (tool, model string) {
	tool = m.Tool
	if tool == "" {
		tool = p.DefaultTool
	}
	switch {
	case m.Model != "":
		model = m.Model
	case tool == p.DefaultTool:
		model = p.DefaultModel
	}
	return tool, model
}

// CreateRunFor snapshots a Project into a new run with every member pending.
// With positions it snapshots only those members, keeping their positions.
func (s *Store) CreateRunFor(uid int64, projectID string, positions ...int) (Run, error) {
	run, _, err := s.startRun(uid, projectID, "", 0, positions)
	return run, err
}

// StartRunFor is how a person starts a run. requestID (a terminal id: 16 to
// 128 letters, digits, - and _) makes it idempotent: the same id with the
// same members answers with the run it made (created false), the same id with
// other members is ErrRunRequestChanged. A new run is refused with
// ErrRunInProgress while any member of an earlier run of the Project is still
// pending, so two runs never race for the same folders.
func (s *Store) StartRunFor(uid int64, projectID, requestID string, positions []int) (Run, bool, error) {
	return s.StartRunAtFor(uid, projectID, requestID, 0, positions)
}

// StartRunAtFor starts a run only while the Project's stored version is still
// expect (the version the confirmation sheet was made from), in the same
// transaction that creates the run; otherwise ErrProjectStale and nothing is
// created. A replay of an existing request id answers with its run whatever
// the version is now. An expect of 0 has no precondition.
func (s *Store) StartRunAtFor(uid int64, projectID, requestID string, expect int64, positions []int) (Run, bool, error) {
	if !terminalproto.ValidID(requestID) {
		return Run{}, false, invalid("a start needs a request id of 16 to 128 letters, digits, - or _")
	}
	return s.startRun(uid, projectID, requestID, expect, positions)
}

func samePositions(members []RunMember, positions []int) bool {
	if len(members) != len(positions) {
		return false
	}
	want := map[int]bool{}
	for _, p := range positions {
		want[p] = true
	}
	for _, m := range members {
		if !want[m.Position] {
			return false
		}
	}
	return len(want) == len(members)
}

func (s *Store) startRun(uid int64, projectID, requestID string, expect int64, positions []int) (Run, bool, error) {
	id, err := projectRandomID()
	if err != nil {
		return Run{}, false, err
	}
	var run Run
	created := false
	err = s.withTx(func(tx *sql.Tx) error {
		if requestID != "" {
			var existing string
			err := tx.QueryRow(`SELECT id FROM project_runs WHERE user_id=? AND project_id=? AND request_id=?`, uid, projectID, requestID).Scan(&existing)
			if err == nil {
				if run, err = loadRun(tx, uid, existing); err != nil {
					return err
				}
				if !samePositions(run.Members, positions) {
					return ErrRunRequestChanged
				}
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if expect != 0 {
				var version int64
				if err := tx.QueryRow(`SELECT version FROM projects WHERE id=? AND user_id=?`, projectID, uid).Scan(&version); err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return ErrProjectNotFound
					}
					return err
				}
				if version != expect {
					return ErrProjectStale
				}
			}
			var busy int
			err = tx.QueryRow(`SELECT 1 FROM project_run_members m JOIN project_runs r ON r.id=m.run_id AND r.user_id=m.user_id
				WHERE r.user_id=? AND r.project_id=? AND m.state='pending' LIMIT 1`, uid, projectID).Scan(&busy)
			if err == nil {
				return ErrRunInProgress
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		created = true
		p, err := loadProject(tx, uid, projectID)
		if err != nil {
			return err
		}
		chosen := p.Members
		if len(positions) > 0 {
			byPos := map[int]ProjectMember{}
			for _, m := range p.Members {
				byPos[m.Position] = m
			}
			chosen = nil
			picked := map[int]bool{}
			for _, pos := range positions {
				m, ok := byPos[pos]
				if !ok || picked[pos] {
					return invalid("run repositories must be distinct members of the Project")
				}
				picked[pos] = true
				chosen = append(chosen, m)
			}
			sortMembers(chosen)
		}
		if len(chosen) == 0 {
			return ErrRunNoMembers
		}
		seen := map[[2]string]bool{}
		t := now()
		if _, err := tx.Exec(`INSERT INTO project_runs(id,user_id,project_id,project_name,prompt,use_worktrees,created_at,request_id) VALUES(?,?,?,?,?,?,?,?)`,
			id, uid, p.ID, p.Name, p.SharedPrompt, p.UseWorktrees, t, requestID); err != nil {
			return err
		}
		run = Run{ID: id, Origin: RunOriginSet, ProjectID: p.ID, ProjectName: p.Name, Prompt: p.SharedPrompt, UseWorktrees: p.UseWorktrees, CreatedAt: t}
		for _, m := range chosen {
			key := [2]string{hostKey(m.Host), m.Dir}
			if seen[key] {
				return fmt.Errorf("%w (machine %q, folder %q)", ErrDuplicateMember, m.Host, m.Dir)
			}
			seen[key] = true
			tool, model := resolvedLaunch(p, m)
			if _, err := tx.Exec(`INSERT INTO project_run_members(run_id,user_id,position,host,dir,tool,model) VALUES(?,?,?,?,?,?,?)`,
				id, uid, m.Position, m.Host, m.Dir, tool, model); err != nil {
				return err
			}
			run.Members = append(run.Members, RunMember{Position: m.Position, Host: m.Host, Dir: m.Dir, Tool: tool, Model: model, State: RunMemberPending, Attempt: 1})
		}
		run.State = DeriveRunState(run.Members)
		return nil
	})
	if err != nil {
		return Run{}, false, err
	}
	return run, created, nil
}

// validate checks a selection's launch with the connector's own rules, so a
// run never holds a create every member would refuse. The messages never
// repeat the prompt.
func (l RunLaunch) validate() error {
	if !terminalproto.ValidToolV2(l.Tool) {
		return invalid("agent must be one that reet starts")
	}
	if l.Model != "" && (!terminalproto.ValidModel(l.Model) || !terminalproto.ToolTakesModel(l.Tool)) {
		return invalid("model has characters or a length the connector would refuse, or the agent takes none")
	}
	switch l.Effort {
	case "":
	case "low", "medium", "high":
		if !terminalproto.ToolTakesEffort(l.Tool) {
			return invalid("that agent takes no thinking effort")
		}
	default:
		return invalid("thinking effort must be low, medium or high")
	}
	if l.Prompt != "" && !terminalproto.ValidPrompt(l.Prompt) {
		return invalid("opening prompt must be valid text of at most %d bytes with no control characters except newline, and must not start with -", terminalproto.MaxPromptBytes)
	}
	if terminalproto.PromptNeedsWords(l.Tool, l.Prompt) {
		return invalid("opening prompt must be more than one word for %s", l.Tool)
	}
	return nil
}

// sameSelection reports whether an existing selection run was made from
// exactly these projects, in this order, with this launch.
func sameSelection(r Run, projectIDs []string, l RunLaunch) bool {
	if r.Origin != RunOriginSelection || len(r.Members) != len(projectIDs) || r.Prompt != l.Prompt || r.UseWorktrees != l.Worktree || r.Effort != l.Effort || r.Yolo != l.Yolo {
		return false
	}
	for i, m := range r.Members {
		if m.Position != i || m.ProjectID != projectIDs[i] || m.Tool != l.Tool || m.Model != l.Model {
			return false
		}
	}
	return true
}

// SelectionRunForRequest reports whether uid already has a selection run made
// by requestID, so a resend of a lost reply can be told from a new start.
func (s *Store) SelectionRunForRequest(uid int64, requestID string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM project_runs WHERE user_id=? AND origin='selection' AND request_id=? AND request_id<>''`, uid, requestID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// StartSelectionRunFor is how a person starts agents in several folder
// projects at once (2 to 12, each once, in the order given). The run snapshots
// one member per project: its machine, folder and name and, with a worktree,
// its base branch (the project's own, else the account's default, else none).
// A folder that is not a repository works in its folder. requestID makes it
// idempotent as for StartRunFor: the same id with the same projects and launch
// answers with its run (created false), with anything else
// ErrRunRequestChanged. A new run is refused with ErrRunInProgress while a
// member of any run in one of these folders is still pending. The
// orchestrator drives it with the same rules as a set run.
func (s *Store) StartSelectionRunFor(uid int64, requestID string, projectIDs []string, launch RunLaunch) (Run, bool, error) {
	if !terminalproto.ValidID(requestID) {
		return Run{}, false, invalid("a start needs a request id of 16 to 128 letters, digits, - or _")
	}
	if len(projectIDs) < MinSelectionProjects || len(projectIDs) > MaxSelectionProjects {
		return Run{}, false, ErrSelectionSize
	}
	named := map[string]bool{}
	for _, id := range projectIDs {
		if named[id] {
			return Run{}, false, ErrSelectionSize
		}
		named[id] = true
	}
	if err := launch.validate(); err != nil {
		return Run{}, false, err
	}
	id, err := projectRandomID()
	if err != nil {
		return Run{}, false, err
	}
	var run Run
	created := false
	err = s.withTx(func(tx *sql.Tx) error {
		var existing string
		err := tx.QueryRow(`SELECT id FROM project_runs WHERE user_id=? AND origin='selection' AND request_id=?`, uid, requestID).Scan(&existing)
		if err == nil {
			if run, err = loadRun(tx, uid, existing); err != nil {
				return err
			}
			if !sameSelection(run, projectIDs, launch) {
				return ErrRunRequestChanged
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		projects := make([]FolderProject, 0, len(projectIDs))
		for _, pid := range projectIDs {
			p, err := scanFolderProject(tx.QueryRow(`SELECT `+folderColumns+` FROM folder_projects WHERE id=? AND user_id=?`, pid, uid))
			if errors.Is(err, sql.ErrNoRows) {
				return ErrFolderProjectNotFound
			}
			if err != nil {
				return err
			}
			var busy int
			err = tx.QueryRow(`SELECT 1 FROM project_run_members WHERE user_id=? AND state='pending' AND host=? AND dir=? LIMIT 1`, uid, p.Host, p.Dir).Scan(&busy)
			if err == nil {
				return ErrRunInProgress
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			projects = append(projects, p)
		}
		var profileBase string
		if err := tx.QueryRow(`SELECT default_base_branch FROM user_profiles WHERE user_id=?`, uid).Scan(&profileBase); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name
		}
		created = true
		t := now()
		run = Run{ID: id, Origin: RunOriginSelection, ProjectName: strings.Join(names, ", "), Prompt: launch.Prompt, UseWorktrees: launch.Worktree,
			Effort: launch.Effort, Yolo: launch.Yolo, CreatedAt: t}
		if _, err := tx.Exec(`INSERT INTO project_runs(id,user_id,project_id,project_name,prompt,use_worktrees,created_at,request_id,origin,effort,yolo) VALUES(?,?,NULL,?,?,?,?,?,'selection',?,?)`,
			id, uid, run.ProjectName, run.Prompt, run.UseWorktrees, t, requestID, run.Effort, run.Yolo); err != nil {
			return err
		}
		for i, p := range projects {
			m := RunMember{Position: i, Host: p.Host, Dir: p.Dir, Tool: launch.Tool, Model: launch.Model, State: RunMemberPending, Attempt: 1, ProjectID: p.ID, Name: p.Name}
			switch {
			case launch.Worktree && !p.IsRepo:
				m.NoWorktree = true
			case launch.Worktree:
				m.Base = p.BaseBranch
				if m.Base == "" {
					m.Base = profileBase
				}
			}
			if _, err := tx.Exec(`INSERT INTO project_run_members(run_id,user_id,position,host,dir,tool,model,folder_project_id,name,base,no_worktree) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
				id, uid, m.Position, m.Host, m.Dir, m.Tool, m.Model, m.ProjectID, m.Name, m.Base, m.NoWorktree); err != nil {
				return err
			}
			run.Members = append(run.Members, m)
		}
		run.State = DeriveRunState(run.Members)
		return nil
	})
	if err != nil {
		return Run{}, false, err
	}
	return run, created, nil
}

func sortMembers(ms []ProjectMember) {
	for i := 1; i < len(ms); i++ {
		for j := i; j > 0 && ms[j].Position < ms[j-1].Position; j-- {
			ms[j], ms[j-1] = ms[j-1], ms[j]
		}
	}
}

// DeriveRunState is running while any member is pending, then complete when
// every member started, failed when none did, otherwise partial. A removed
// member counts as started: it did start.
func DeriveRunState(members []RunMember) string {
	started, failed := 0, 0
	for _, m := range members {
		switch m.State {
		case RunMemberPending:
			return RunRunning
		case RunMemberStarted, RunMemberRemoved:
			started++
		case RunMemberFailed:
			failed++
		}
	}
	switch {
	case started > 0 && failed == 0:
		return RunComplete
	case started == 0:
		return RunFailed
	}
	return RunPartial
}

const runMemberCols = `position,host,dir,tool,model,state,code,session_id,worktree_token,worktree_path,worktree_repo,worktree_branch,attempt,sent,folder_project_id,name,base,no_worktree`

func scanRunMember(row interface{ Scan(...any) error }) (m RunMember, err error) {
	err = row.Scan(&m.Position, &m.Host, &m.Dir, &m.Tool, &m.Model, &m.State, &m.Code, &m.SessionID, &m.WorktreeToken, &m.WorktreePath, &m.WorktreeRepo, &m.WorktreeBranch, &m.Attempt, &m.Sent,
		&m.ProjectID, &m.Name, &m.Base, &m.NoWorktree)
	return
}

func loadRunMembers(q dbQuerier, uid int64, runID string) ([]RunMember, error) {
	rows, err := q.Query(`SELECT `+runMemberCols+` FROM project_run_members WHERE run_id=? AND user_id=? ORDER BY position`, runID, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunMember{}
	for rows.Next() {
		m, err := scanRunMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanRun(row interface{ Scan(...any) error }) (r Run, err error) {
	var pid sql.NullString
	err = row.Scan(&r.ID, &pid, &r.ProjectName, &r.Prompt, &r.UseWorktrees, &r.CreatedAt, &r.Origin, &r.Effort, &r.Yolo)
	r.ProjectID = pid.String
	return
}

const runCols = `id,project_id,project_name,prompt,use_worktrees,created_at,origin,effort,yolo`

func loadRun(q dbQuerier, uid int64, id string) (Run, error) {
	r, err := scanRun(q.QueryRow(`SELECT `+runCols+` FROM project_runs WHERE id=? AND user_id=?`, id, uid))
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	if err != nil {
		return Run{}, err
	}
	if r.Members, err = loadRunMembers(q, uid, id); err != nil {
		return Run{}, err
	}
	r.State = DeriveRunState(r.Members)
	return r, nil
}

func (s *Store) RunFor(uid int64, runID string) (Run, error) { return loadRun(s.db, uid, runID) }

// RunsFor lists the newest runs of one Project, newest first. A limit outside
// 1..MaxRunsListed means MaxRunsListed.
func (s *Store) RunsFor(uid int64, projectID string, limit int) ([]Run, error) {
	if limit < 1 || limit > MaxRunsListed {
		limit = MaxRunsListed
	}
	var out []Run
	err := s.withTx(func(tx *sql.Tx) error {
		var one int
		if err := tx.QueryRow(`SELECT 1 FROM projects WHERE id=? AND user_id=?`, projectID, uid).Scan(&one); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrProjectNotFound
			}
			return err
		}
		rows, err := tx.Query(`SELECT `+runCols+` FROM project_runs WHERE user_id=? AND project_id=? ORDER BY created_at DESC, rowid DESC LIMIT ?`, uid, projectID, limit)
		if err != nil {
			return err
		}
		for rows.Next() {
			r, err := scanRun(rows)
			if err != nil {
				rows.Close()
				return err
			}
			out = append(out, r)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for i := range out {
			if out[i].Members, err = loadRunMembers(tx, uid, out[i].ID); err != nil {
				return err
			}
			out[i].State = DeriveRunState(out[i].Members)
			out[i].Prompt = "" // lists never carry the prompt; RunFor does, for the orchestrator
		}
		return nil
	})
	if out == nil {
		out = []Run{}
	}
	return out, err
}

// PendingRuns is for the orchestrator only (the API layer must not call it).
// It lists, across accounts, the runs that still have pending
// members, oldest first, for resuming after a controller restart.
func (s *Store) PendingRuns() ([]PendingRun, error) {
	rows, err := s.db.Query(`SELECT r.user_id,r.id FROM project_runs r WHERE EXISTS
		(SELECT 1 FROM project_run_members m WHERE m.run_id=r.id AND m.user_id=r.user_id AND m.state='pending')
		ORDER BY r.created_at, r.rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PendingRun{}
	for rows.Next() {
		var p PendingRun
		if err := rows.Scan(&p.UID, &p.RunID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Orchestrator only: the API layer must not call RunMemberStart,
// ReleaseRunMember or PendingRuns. The orchestrator should `defer
// ReleaseRunMember` for every claim it takes (harmless once the member is
// decided).
//
// RunMemberStart claims a pending member for this process: of two callers only
// one gets the member, the other ErrRunMemberNotPending. The claim is held
// until the member leaves pending through SetRunMemberState, or
// ReleaseRunMember. It lives in memory on purpose; after a restart nothing is
// in flight and every pending member can be claimed again. The returned member
// carries the attempt number the caller derives its request id from.
func (s *Store) RunMemberStart(uid int64, runID string, position int) (RunMember, error) {
	k := claimKey{uid, runID, position}
	if !s.runClaims.take(k) {
		return RunMember{}, ErrRunMemberNotPending
	}
	m, err := scanRunMember(s.db.QueryRow(`SELECT `+runMemberCols+` FROM project_run_members WHERE run_id=? AND user_id=? AND position=?`, runID, uid, position))
	if errors.Is(err, sql.ErrNoRows) {
		s.runClaims.release(k)
		return RunMember{}, ErrRunNotFound
	}
	if err == nil && m.State != RunMemberPending {
		err = ErrRunMemberNotPending
	}
	if err != nil {
		s.runClaims.release(k)
		return RunMember{}, err
	}
	return m, nil
}

// ReleaseRunMember (orchestrator only) drops a claim without changing the member, for a caller that
// gave up before knowing the outcome. It does nothing for an unclaimed member.
func (s *Store) ReleaseRunMember(uid int64, runID string, position int) {
	s.runClaims.release(claimKey{uid, runID, position})
}

func validCode(code string) bool {
	if code == "" || len(code) > maxCodeBytes {
		return false
	}
	for i := 0; i < len(code); i++ {
		b := code[i]
		if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_') {
			return false
		}
	}
	return true
}

func shortPlain(v string, max int) bool { return len(v) <= max && plainText(v) }

// SetRunMemberState moves one member of uid's run. The allowed moves are
// pending to started (needs sessionID, optional worktree facts the machine
// reported), pending to failed (needs a code such as "session_limit"), and
// failed to pending as an explicit retry, which keeps the attempt (outcome
// unknown, the same request id is reused) or raises it by one (definite
// refusal, a new request id). For the other moves attempt must equal the stored
// attempt. A started member never moves again. A retry clears the failure and
// keeps the member's worktree token.
func (s *Store) SetRunMemberState(uid int64, runID string, position int, state, code, sessionID, worktreePath, worktreeRepo, worktreeBranch string, attempt int) error {
	err := s.withTx(func(tx *sql.Tx) error {
		cur, err := scanRunMember(tx.QueryRow(`SELECT `+runMemberCols+` FROM project_run_members WHERE run_id=? AND user_id=? AND position=?`, runID, uid, position))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRunNotFound
		}
		if err != nil {
			return err
		}
		refuse := fmt.Errorf("%w: %s to %s", ErrRunMemberTransition, cur.State, state)
		var res sql.Result
		switch {
		case cur.State == RunMemberPending && state == RunMemberStarted:
			if attempt != cur.Attempt {
				return ErrRunMemberStale
			}
			if sessionID == "" || !shortPlain(sessionID, maxSessionIDBytes) || code != "" ||
				!shortPlain(worktreePath, maxWorktreeBytes) || !shortPlain(worktreeRepo, maxBranchBytes) || !shortPlain(worktreeBranch, maxBranchBytes) {
				return invalid("a started member needs a session id and plain-text worktree facts within their limits, and no code")
			}
			res, err = tx.Exec(`UPDATE project_run_members SET state='started',code='',session_id=?,worktree_path=?,worktree_repo=?,worktree_branch=? WHERE run_id=? AND user_id=? AND position=? AND state='pending' AND attempt=?`,
				sessionID, worktreePath, worktreeRepo, worktreeBranch, runID, uid, position, attempt)
		case cur.State == RunMemberPending && state == RunMemberFailed:
			if attempt != cur.Attempt {
				return ErrRunMemberStale
			}
			if !validCode(code) || sessionID != "" || worktreePath != "" || worktreeRepo != "" || worktreeBranch != "" {
				return invalid("a failed member needs a code of lower-case letters, digits and underscores, and nothing else")
			}
			res, err = tx.Exec(`UPDATE project_run_members SET state='failed',code=? WHERE run_id=? AND user_id=? AND position=? AND state='pending' AND attempt=?`,
				code, runID, uid, position, attempt)
		case cur.State == RunMemberFailed && state == RunMemberPending:
			// A create that may have been sent keeps its request id.
			if attempt != cur.Attempt && (attempt != cur.Attempt+1 || cur.Sent) {
				return ErrRunMemberStale
			}
			if code != "" || sessionID != "" || worktreePath != "" || worktreeRepo != "" || worktreeBranch != "" {
				return invalid("a retry carries no code or session facts")
			}
			res, err = tx.Exec(`UPDATE project_run_members SET state='pending',code='',session_id='',worktree_path='',worktree_repo='',worktree_branch='',attempt=?,sent=CASE WHEN ?=attempt THEN sent ELSE 0 END WHERE run_id=? AND user_id=? AND position=? AND state='failed' AND attempt=?`,
				attempt, attempt, runID, uid, position, cur.Attempt)
		default:
			return refuse
		}
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return refuse
		}
		return nil
	})
	if err == nil && state != RunMemberPending {
		s.runClaims.release(claimKey{uid, runID, position})
	}
	return err
}

// Orchestrator only. MarkRunMemberSent records, committed, that the pending
// member's create for this attempt is about to be handed to the broker. It
// must be called before the call, so a crash or a lost answer can never leave
// a possibly-sent create looking unsent.
func (s *Store) MarkRunMemberSent(uid int64, runID string, position, attempt int) error {
	return s.setRunMemberSent(uid, runID, position, attempt, true)
}

// Orchestrator only. ClearRunMemberSent records that this attempt's create is
// known to have started nothing: refused before anything reached the machine
// under its request id, or refused by the machine. The member must still be
// pending at that attempt.
func (s *Store) ClearRunMemberSent(uid int64, runID string, position, attempt int) error {
	return s.setRunMemberSent(uid, runID, position, attempt, false)
}

func (s *Store) setRunMemberSent(uid int64, runID string, position, attempt int, sent bool) error {
	res, err := s.db.Exec(`UPDATE project_run_members SET sent=? WHERE run_id=? AND user_id=? AND position=? AND state='pending' AND attempt=?`,
		sent, runID, uid, position, attempt)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrRunMemberStale
	}
	return nil
}

// RetryRunMember moves a failed member back to pending and decides its
// attempt in the same transaction, so racing retries agree: the same attempt
// (the same request id) while its create may have been sent, the next one
// only when the last create is known to have started nothing. Of two racing
// retries one wins; the other gets ErrRunMemberNotFailed.
func (s *Store) RetryRunMember(uid int64, runID string, position int) (RunMember, error) {
	var m RunMember
	err := s.withTx(func(tx *sql.Tx) error {
		cur, err := scanRunMember(tx.QueryRow(`SELECT `+runMemberCols+` FROM project_run_members WHERE run_id=? AND user_id=? AND position=?`, runID, uid, position))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRunNotFound
		}
		if err != nil {
			return err
		}
		if cur.State != RunMemberFailed {
			return ErrRunMemberNotFailed
		}
		attempt := cur.Attempt
		if !cur.Sent {
			attempt++
		}
		res, err := tx.Exec(`UPDATE project_run_members SET state='pending',code='',session_id='',worktree_path='',worktree_repo='',worktree_branch='',attempt=? WHERE run_id=? AND user_id=? AND position=? AND state='failed' AND attempt=? AND sent=?`,
			attempt, runID, uid, position, cur.Attempt, cur.Sent)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrRunMemberNotFailed
		}
		m = cur
		m.State, m.Code, m.SessionID, m.WorktreePath, m.WorktreeRepo, m.WorktreeBranch, m.Attempt = RunMemberPending, "", "", "", "", "", attempt
		return nil
	})
	return m, err
}

// SetRunMemberRemovedFor records that the worktree with this run token on
// this machine was removed: the account's member that made it (started or
// failed, with a worktree path recorded) moves to removed. No member matching
// is not an error: the worktree may not belong to a run.
func (s *Store) SetRunMemberRemovedFor(uid int64, agent, token string) error {
	if !terminalproto.ValidRunToken(token) || agent == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE project_run_members SET state='removed' WHERE user_id=? AND host=? AND worktree_token=? AND state IN ('started','failed') AND worktree_path<>''`,
		uid, agent, token)
	return err
}

// SetRunMemberWorktreeToken records the per-member worktree token the
// orchestrator derived. It is written once; the same token again is a no-op and
// a different one is refused. Retries and new attempts keep it.
func (s *Store) SetRunMemberWorktreeToken(uid int64, runID string, position int, token string) error {
	if !terminalproto.ValidRunToken(token) {
		return invalid("worktree token must be 16 to 48 lower-case hex characters")
	}
	return s.withTx(func(tx *sql.Tx) error {
		var have string
		if err := tx.QueryRow(`SELECT worktree_token FROM project_run_members WHERE run_id=? AND user_id=? AND position=?`, runID, uid, position).Scan(&have); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRunNotFound
			}
			return err
		}
		if have == token {
			return nil
		}
		if have != "" {
			return fmt.Errorf("%w: worktree token is already set", ErrRunMemberTransition)
		}
		_, err := tx.Exec(`UPDATE project_run_members SET worktree_token=? WHERE run_id=? AND user_id=? AND position=? AND worktree_token=''`, token, runID, uid, position)
		return err
	})
}
