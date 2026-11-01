package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	ErrRemoteNotFound         = errors.New("remote machine record unavailable")
	ErrTerminalConflict       = errors.New("terminal operation conflicts with existing state")
	ErrTerminalCapacity       = errors.New("terminal session capacity reached")
	ErrTerminalNotReady       = errors.New("terminal is not ready")
	ErrProjectMachineMismatch = errors.New("project is on another machine")
	// ErrProjectFolderMismatch: the session would not start in the project's
	// folder, so it is not put in that project.
	ErrProjectFolderMismatch = errors.New("session is not in the project's folder")
)

func (s *Store) migrateTerminal() error {
	if err := s.migrateTerminalTables(); err != nil {
		return err
	}
	// Launch metadata the session listing shows. tool and model are copied
	// from the stored create (never its prompt) when the column first appears,
	// so sessions started before then list them too. effort and yolo have no
	// source yet and stay '' and 0. project_id is NULL for a session started
	// outside a project.
	hadTool, err := hasColumn(s.db, "terminal_sessions", "tool")
	if err != nil {
		return err
	}
	hadDir, err := hasColumn(s.db, "terminal_sessions", "dir")
	if err != nil {
		return err
	}
	// Columns added for worktrees: where a worktree create made its worktree,
	// and why a refused create was refused.
	for _, c := range []struct{ table, ddl string }{
		{"terminal_sessions", "worktree_path TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "worktree_repo TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "worktree_branch TEXT NOT NULL DEFAULT ''"},
		{"terminal_operations", "reason TEXT NOT NULL DEFAULT ''"},
		{"terminal_operations", "detail TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "tool TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "model TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "effort TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "yolo INTEGER NOT NULL DEFAULT 0 CHECK(yolo IN (0,1))"},
		{"terminal_sessions", "project_id TEXT"},
		{"terminal_sessions", "dir TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "structured INTEGER NOT NULL DEFAULT 0 CHECK(structured IN (0,1))"},
		{"terminal_sessions", "acp_session_id TEXT NOT NULL DEFAULT ''"},
	} {
		ok, err := hasColumn(s.db, c.table, strings.Fields(c.ddl)[0])
		if err != nil {
			return err
		}
		if !ok {
			if _, err := s.db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.ddl); err != nil {
				return err
			}
		}
	}
	if !hadTool {
		if _, err := s.db.Exec(`UPDATE terminal_sessions SET
			tool=COALESCE((SELECT json_extract(o.parameters,'$.tool') FROM terminal_operations o WHERE o.session_id=terminal_sessions.id AND o.user_id=terminal_sessions.user_id AND o.agent_id=terminal_sessions.agent_id AND o.kind='create' AND json_valid(o.parameters)),''),
			model=COALESCE((SELECT json_extract(o.parameters,'$.model') FROM terminal_operations o WHERE o.session_id=terminal_sessions.id AND o.user_id=terminal_sessions.user_id AND o.agent_id=terminal_sessions.agent_id AND o.kind='create' AND json_valid(o.parameters)),'')`); err != nil {
			return err
		}
	}
	if !hadDir {
		if _, err := s.db.Exec(`UPDATE terminal_sessions SET
			dir=COALESCE((SELECT json_extract(o.parameters,'$.dir') FROM terminal_operations o WHERE o.session_id=terminal_sessions.id AND o.user_id=terminal_sessions.user_id AND o.agent_id=terminal_sessions.agent_id AND o.kind='create' AND json_valid(o.parameters)),'')`); err != nil {
			return err
		}
	}
	return s.migrateAttention()
}

func (s *Store) migrateTerminalTables() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS sync_agents_identity_owner ON sync_agents(id,user_id)`,
			`CREATE TABLE IF NOT EXISTS private_transport_identity (
				id INTEGER PRIMARY KEY CHECK(id=1), format_version INTEGER NOT NULL CHECK(format_version=1),
				ciphertext TEXT NOT NULL, created_at INTEGER NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS remote_agent_capabilities (
				agent_id TEXT PRIMARY KEY, user_id INTEGER NOT NULL,
				generation INTEGER NOT NULL CHECK(generation>0), instance_id TEXT NOT NULL,
				capabilities TEXT NOT NULL, observed_at INTEGER NOT NULL,
				FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
			`CREATE TABLE IF NOT EXISTS terminal_sessions (
				id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, agent_id TEXT NOT NULL,
				title TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN ('creating','running','end_requested','ended','exited','lost')),
				created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, exit_code INTEGER CHECK(exit_code IS NULL OR (exit_code BETWEEN 0 AND 255 AND state='exited')),
				UNIQUE(id,agent_id,user_id),
				FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
			`CREATE INDEX IF NOT EXISTS terminal_sessions_quota ON terminal_sessions(user_id,agent_id,state)`,
			`CREATE TABLE IF NOT EXISTS terminal_commands (
                id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                name TEXT NOT NULL, command TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
                revision INTEGER NOT NULL CHECK(revision>0), created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
			`CREATE INDEX IF NOT EXISTS terminal_commands_owner ON terminal_commands(user_id)`,
			`CREATE TABLE IF NOT EXISTS terminal_session_archives (
                session_id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, agent_id TEXT NOT NULL,
                archived_at INTEGER NOT NULL,
                FOREIGN KEY(session_id,agent_id,user_id) REFERENCES terminal_sessions(id,agent_id,user_id) ON DELETE CASCADE)`,
			`CREATE TABLE IF NOT EXISTS terminal_operations (
				request_id TEXT NOT NULL, user_id INTEGER NOT NULL, agent_id TEXT NOT NULL, session_id TEXT NOT NULL,
				kind TEXT NOT NULL CHECK(kind IN ('create','end')),
				state TEXT NOT NULL CHECK(state IN ('pending','succeeded','failed')),
				result_code TEXT NOT NULL DEFAULT '', parameters TEXT NOT NULL DEFAULT '{}',
				created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
				PRIMARY KEY(user_id,agent_id,kind,request_id),
				FOREIGN KEY(session_id,agent_id,user_id) REFERENCES terminal_sessions(id,agent_id,user_id) ON DELETE CASCADE)`,
			`CREATE INDEX IF NOT EXISTS terminal_operations_pending ON terminal_operations(agent_id,state,created_at)`,
			`CREATE TABLE IF NOT EXISTS remote_health_summaries (
				agent_id TEXT NOT NULL, user_id INTEGER NOT NULL, minute INTEGER NOT NULL,
				generation INTEGER NOT NULL, revision INTEGER NOT NULL, summary TEXT NOT NULL,
				PRIMARY KEY(agent_id,minute),
				FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
			`CREATE INDEX IF NOT EXISTS remote_health_retention ON remote_health_summaries(minute)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}

// PrivateIdentityRecord contains only the ciphertext produced by Identity.Seal.
// Opening/validating its purpose and keys belongs to privateconn.OpenIdentity.
// This store must never accept or return the plaintext capability to a browser.
type PrivateIdentityRecord struct {
	FormatVersion int    `json:"-"`
	Ciphertext    string `json:"-"`
}

func (PrivateIdentityRecord) String() string   { return "[sealed private connection identity]" }
func (PrivateIdentityRecord) GoString() string { return "[sealed private connection identity]" }
func (s *Store) PrivateIdentity() (PrivateIdentityRecord, bool, error) {
	var r PrivateIdentityRecord
	err := s.db.QueryRow(`SELECT format_version,ciphertext FROM private_transport_identity WHERE id=1`).Scan(&r.FormatVersion, &r.Ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

// SavePrivateIdentityIfAbsent cannot rotate/overwrite an existing identity.
// Callers must restore an existing record and fail closed on any opening error.
func (s *Store) SavePrivateIdentityIfAbsent(r PrivateIdentityRecord) error {
	if r.FormatVersion != 1 || len(r.Ciphertext) > 64<<10 {
		return terminalproto.ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(r.Ciphertext)
	if r.FormatVersion != 1 || len(r.Ciphertext) > 64<<10 || err != nil || len(raw) < 44 {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO private_transport_identity(id,format_version,ciphertext,created_at) VALUES(1,?,?,?) ON CONFLICT(id) DO NOTHING`, r.FormatVersion, r.Ciphertext, now()); err != nil {
			return err
		}
		var stored PrivateIdentityRecord
		if err := tx.QueryRow(`SELECT format_version,ciphertext FROM private_transport_identity WHERE id=1`).Scan(&stored.FormatVersion, &stored.Ciphertext); err != nil {
			return err
		}
		if stored != r {
			return ErrTerminalConflict
		}
		return nil
	})
}

type RemoteAgentCapabilities struct {
	AgentID      string                     `json:"agentId"`
	Generation   int64                      `json:"generation"`
	InstanceID   string                     `json:"instanceId"`
	Capabilities terminalproto.Capabilities `json:"capabilities"`
	ObservedAt   int64                      `json:"observedAt"`
}

// lockRemoteAgent is intentionally the first SQL in mutation transactions. A
// write before quota/idempotency reads serializes even separate Store handles
// (SQLite WAL deferred read-then-write transactions cannot safely upgrade).
func lockRemoteAgent(tx *sql.Tx, uid int64, agentID string, terminal bool) error {
	role := ""
	if terminal {
		role = " AND u.role<>'viewer'"
	}
	r, err := tx.Exec(`UPDATE sync_agents SET id=id WHERE id=? AND user_id=? AND EXISTS(SELECT 1 FROM users u WHERE u.id=? AND u.disabled_at=0`+role+`)`, agentID, uid, uid)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrRemoteNotFound
	}
	return nil
}
func readCapabilities(row interface{ Scan(...any) error }) (RemoteAgentCapabilities, error) {
	var c RemoteAgentCapabilities
	var raw string
	err := row.Scan(&c.AgentID, &c.Generation, &c.InstanceID, &raw, &c.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrRemoteNotFound
	}
	if err != nil {
		return c, err
	}
	if json.Unmarshal([]byte(raw), &c.Capabilities) != nil || c.Capabilities.Validate() != nil {
		return c, terminalproto.ErrInvalid
	}
	return c, nil
}

const capabilityCols = `agent_id,generation,instance_id,capabilities,observed_at`

// BeginRemoteConnectionFor persists a new fencing generation after enrollment
// authentication. It is not itself authentication or proof of a live socket.
func (s *Store) BeginRemoteConnectionFor(uid int64, agentID, instanceID string, caps terminalproto.Capabilities) (RemoteAgentCapabilities, error) {
	var c RemoteAgentCapabilities
	if !terminalproto.ValidID(agentID) || !terminalproto.ValidID(instanceID) || caps.Validate() != nil {
		return c, terminalproto.ErrInvalid
	}
	err := s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		var generation int64
		err := tx.QueryRow(`SELECT generation FROM remote_agent_capabilities WHERE agent_id=? AND user_id=?`, agentID, uid).Scan(&generation)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if generation >= terminalproto.MaxCounter {
			return ErrTerminalConflict
		}
		c = RemoteAgentCapabilities{agentID, generation + 1, instanceID, caps, now()}
		raw, _ := json.Marshal(caps)
		_, err = tx.Exec(`INSERT INTO remote_agent_capabilities(agent_id,user_id,generation,instance_id,capabilities,observed_at) VALUES(?,?,?,?,?,?) ON CONFLICT(agent_id) DO UPDATE SET generation=excluded.generation,instance_id=excluded.instance_id,capabilities=excluded.capabilities,observed_at=excluded.observed_at`, agentID, uid, c.Generation, instanceID, string(raw), c.ObservedAt)
		return err
	})
	return c, err
}
func (s *Store) RemoteCapabilitiesFor(uid int64, agentID string) (RemoteAgentCapabilities, error) {
	return readCapabilities(s.db.QueryRow(`SELECT `+capabilityCols+` FROM remote_agent_capabilities WHERE agent_id=? AND user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0)`, agentID, uid, uid))
}

// UpdateRemoteCapabilitiesFor refreshes consent/readiness without changing a
// live connection's generation. An old process or connection cannot re-enable
// access. The caller must also close active bridges on a local-disable event.
func (s *Store) UpdateRemoteCapabilitiesFor(uid int64, agentID, instanceID string, generation int64, caps terminalproto.Capabilities) error {
	if caps.Validate() != nil || !terminalproto.ValidID(instanceID) {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		current, err := currentGeneration(tx, uid, agentID, generation)
		if err != nil {
			return err
		}
		if current.InstanceID != instanceID {
			return ErrTerminalConflict
		}
		raw, _ := json.Marshal(caps)
		_, err = tx.Exec(`UPDATE remote_agent_capabilities SET capabilities=?,observed_at=? WHERE agent_id=? AND user_id=?`, string(raw), now(), agentID, uid)
		return err
	})
}

func currentGeneration(tx *sql.Tx, uid int64, agentID string, generation int64) (RemoteAgentCapabilities, error) {
	c, err := readCapabilities(tx.QueryRow(`SELECT `+capabilityCols+` FROM remote_agent_capabilities WHERE agent_id=? AND user_id=?`, agentID, uid))
	if err != nil {
		return c, err
	}
	if !terminalproto.ValidCounter(generation) || c.Generation != generation {
		return c, ErrTerminalConflict
	}
	return c, nil
}

type TerminalSession struct {
	RemoveHistory bool                       `json:"removeHistory"`
	ID            string                     `json:"id"`
	AgentID       string                     `json:"agentId"`
	Title         string                     `json:"title"`
	State         terminalproto.SessionState `json:"state"`
	CreatedAt     int64                      `json:"createdAt"`
	UpdatedAt     int64                      `json:"updatedAt"`
	ExitCode      *int                       `json:"exitCode"`
	// Set only for a session started in its own worktree: the folder, the
	// repository name the machine used (what worktree status and remove take)
	// and the branch. All three are absent from the JSON without a worktree.
	WorktreePath   string `json:"worktreePath,omitempty"`
	WorktreeRepo   string `json:"worktreeRepo,omitempty"`
	WorktreeBranch string `json:"worktreeBranch,omitempty"`
	// Launch metadata for the listing. The opening prompt is deliberately
	// absent: it is kept only with the create request. Effort is the requested
	// launch level; Yolo records the person's confirmation for this session.
	// ProjectID is null for a session started outside a folder project.
	Tool      string  `json:"tool"`
	Model     string  `json:"model"`
	Effort    string  `json:"effort"`
	Yolo      bool    `json:"yolo"`
	ProjectID *string `json:"projectId"`
	// Dir is where the agent ran: the worktree it made, else the folder its
	// create named. It is kept on the session so a continuation starts in the
	// same folder even after the project's folder changes.
	Dir string `json:"dir"`
	// Structured is an OpenCode chat over ACP; it has no terminal to attach to.
	Structured bool `json:"structured,omitempty"`
	// ACPSessionID is OpenCode's own session id, the handle for reopening a
	// chat's stored history. It is empty until the machine reports it.
	ACPSessionID      string `json:"acpSessionId,omitempty"`
	ActivityAt        int64  `json:"activityAt,omitempty"`
	BellAt            int64  `json:"bellAt,omitempty"`
	CompletedAt       int64  `json:"completedAt,omitempty"`
	ChatStatus        string `json:"chatStatus,omitempty"`
	SeenAt            int64  `json:"-"`
	Attention         string `json:"attention,omitempty"`
	AttentionSource   string `json:"attentionSource,omitempty"`
	ChangedSinceSeen  bool   `json:"changedSinceSeen,omitempty"`
	AttentionObserved bool   `json:"-"`
	NoAttention       bool   `json:"-"`
	// PendingPermission is the earliest unanswered tool approval of a structured
	// chat that is waiting, so Home can answer it without opening the chat.
	PendingPermission *PendingPermission `json:"pendingPermission,omitempty"`
	// PendingQuestion is the earliest unanswered typed question of a structured chat that is waiting. Home shows it
	// and sends the person to the chat to answer: a question needs its whole card, not a button.
	PendingQuestion *PendingQuestion `json:"pendingQuestion,omitempty"`
}

// PendingQuestion is the card-sized view of one unanswered question: a bounded preview of what was asked. Queued counts
// the questions behind it.
type PendingQuestion struct {
	QuestionID string `json:"questionId"`
	Message    string `json:"message"`
	Truncated  bool   `json:"truncated,omitempty"`
	Queued     int    `json:"queued,omitempty"`
}

// PendingPermission is the card-sized view of one unanswered approval. Queued
// counts the approvals behind it.
type PendingPermission struct {
	PermissionID string `json:"permissionId"`
	Title        string `json:"title,omitempty"`
	ToolKind     string `json:"toolKind,omitempty"`
	Command      string `json:"command,omitempty"`
	Path         string `json:"path,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	Queued       int    `json:"queued,omitempty"`
}

const terminalSessionCols = `id,agent_id,title,state,created_at,updated_at,exit_code,worktree_path,worktree_repo,worktree_branch,CASE WHEN worktree_path<>'' THEN worktree_path ELSE dir END,tool,model,effort,yolo,project_id,structured,acp_session_id,activity_at,bell_at,attention_observed,EXISTS(SELECT 1 FROM terminal_operations o WHERE o.session_id=terminal_sessions.id AND o.user_id=terminal_sessions.user_id AND o.agent_id=terminal_sessions.agent_id AND o.kind='end' AND o.parameters='{"removeHistory":true}'),completed_at,chat_status`

func readTerminal(row interface{ Scan(...any) error }) (TerminalSession, error) {
	var t TerminalSession
	var exit sql.NullInt64
	var project sql.NullString
	var attentionObserved int
	err := row.Scan(&t.ID, &t.AgentID, &t.Title, &t.State, &t.CreatedAt, &t.UpdatedAt, &exit, &t.WorktreePath, &t.WorktreeRepo, &t.WorktreeBranch, &t.Dir, &t.Tool, &t.Model, &t.Effort, &t.Yolo, &project, &t.Structured, &t.ACPSessionID, &t.ActivityAt, &t.BellAt, &attentionObserved, &t.RemoveHistory, &t.CompletedAt, &t.ChatStatus)
	t.AttentionObserved = attentionObserved != 0
	if exit.Valid {
		n := int(exit.Int64)
		t.ExitCode = &n
	}
	if project.Valid {
		t.ProjectID = &project.String
	}
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrRemoteNotFound
	}
	return t, err
}
func terminalForTx(tx *sql.Tx, uid int64, agentID, sessionID string) (TerminalSession, error) {
	return readTerminal(tx.QueryRow(`SELECT `+terminalSessionCols+` FROM terminal_sessions WHERE id=? AND agent_id=? AND user_id=?`, sessionID, agentID, uid))
}
func (s *Store) TerminalSessionFor(uid int64, sessionID string) (TerminalSession, error) {
	t, err := readTerminal(s.db.QueryRow(`SELECT `+terminalSessionCols+` FROM terminal_sessions WHERE id=? AND user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, sessionID, uid, uid))
	if err != nil {
		return t, err
	}
	s.populateAttention(uid, &t, time.Now())
	return t, nil
}
func (s *Store) TerminalSessionsFor(uid int64) ([]TerminalSession, error) {
	rows, err := s.db.Query(`SELECT `+terminalSessionCols+` FROM terminal_sessions WHERE user_id=? AND NOT EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.session_id=terminal_sessions.id) AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer') ORDER BY state IN ('creating','running','end_requested') DESC,updated_at DESC,id LIMIT 100`, uid, uid)
	if err != nil {
		return nil, err
	}
	out := []TerminalSession{}
	for rows.Next() {
		t, err := readTerminal(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	nowTime := time.Now()
	for i := range out {
		s.populateAttention(uid, &out[i], nowTime)
	}
	return out, nil
}

// ArchiveTerminalFor hides only confirmed closed sessions from browser history.
// Session/operation rows remain available to retry and host reconciliation paths.
// The initial write serializes state checks against concurrent end/observation
// transactions across Store handles without depending on host connectivity.
func (s *Store) ArchiveTerminalFor(uid int64, sessionID string) error {
	if !terminalproto.ValidID(sessionID) {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		result, err := tx.Exec(`UPDATE terminal_sessions SET id=id WHERE id=? AND user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, sessionID, uid, uid)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrRemoteNotFound
		}
		session, err := readTerminal(tx.QueryRow(`SELECT `+terminalSessionCols+` FROM terminal_sessions WHERE id=? AND user_id=?`, sessionID, uid))
		if err != nil {
			return err
		}
		switch session.State {
		case terminalproto.Ended, terminalproto.Exited, terminalproto.Lost:
		default:
			return ErrTerminalConflict
		}
		_, err = tx.Exec(`INSERT INTO terminal_session_archives(session_id,user_id,agent_id,archived_at) VALUES(?,?,?,?) ON CONFLICT(session_id) DO NOTHING`, session.ID, uid, session.AgentID, now())
		return err
	})
}

// ReserveTerminalCreateFor only reserves metadata/quota. The future broker must
// verify a live authenticated connection BEFORE calling it, then dispatch using
// the returned session ID; it must not queue new shells for unreachable hosts.
// Retries return the original record even after connection generations change.
func (s *Store) ReserveTerminalCreateFor(uid int64, agentID, requestID string, generation int64, in terminalproto.Create) (TerminalSession, bool, error) {
	var t TerminalSession
	created := false
	if !terminalproto.ValidID(agentID) || !terminalproto.ValidID(requestID) || in.Validate() != nil {
		return t, false, terminalproto.ErrInvalid
	}
	parameters, _ := json.Marshal(in)
	err := s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, true); err != nil {
			return err
		}
		var sessionID, stored string
		err := tx.QueryRow(`SELECT session_id,parameters FROM terminal_operations WHERE user_id=? AND agent_id=? AND kind='create' AND request_id=?`, uid, agentID, requestID).Scan(&sessionID, &stored)
		if err == nil {
			if stored != string(parameters) {
				return ErrTerminalConflict
			}
			t, err = terminalForTx(tx, uid, agentID, sessionID)
			if err != nil {
				return err
			}
			// The project is not in the stored parameters; a retry must
			// name the same one (or none) all the same.
			if (t.ProjectID == nil && in.ProjectID != "") || (t.ProjectID != nil && *t.ProjectID != in.ProjectID) {
				return ErrTerminalConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var project sql.NullString
		if in.ProjectID != "" {
			if err := projectOnMachine(tx, uid, agentID, in.ProjectID, in.Dir); err != nil {
				return err
			}
			project = sql.NullString{String: in.ProjectID, Valid: true}
		}
		c, err := currentGeneration(tx, uid, agentID, generation)
		if err != nil {
			return err
		}
		if !c.Capabilities.TerminalEnabled || c.Capabilities.Readiness != terminalproto.Ready {
			return ErrTerminalNotReady
		}
		var accountCount, agentCount int
		if err := tx.QueryRow(`SELECT COUNT(*),COALESCE(SUM(agent_id=?),0) FROM terminal_sessions WHERE user_id=? AND state IN ('creating','running','end_requested')`, agentID, uid).Scan(&accountCount, &agentCount); err != nil {
			return err
		}
		if accountCount >= terminalproto.MaxSessionsPerAccount || agentCount >= terminalproto.MaxSessionsPerAgent {
			return ErrTerminalCapacity
		}
		t = TerminalSession{ID: syncID(), AgentID: agentID, Title: in.Title, State: terminalproto.Creating, CreatedAt: now(), UpdatedAt: now(), Tool: in.Tool, Model: in.Model, Effort: in.Effort, Yolo: in.Yolo, Dir: in.Dir, Structured: in.Structured, ACPSessionID: in.ResumeACP}
		if project.Valid {
			t.ProjectID = &project.String
		}
		if _, err := tx.Exec(`INSERT INTO terminal_sessions(id,user_id,agent_id,title,state,created_at,updated_at,tool,model,effort,yolo,project_id,dir,structured,acp_session_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, t.ID, uid, agentID, t.Title, t.State, t.CreatedAt, t.UpdatedAt, t.Tool, t.Model, t.Effort, t.Yolo, project, t.Dir, t.Structured, t.ACPSessionID); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO terminal_operations(request_id,user_id,agent_id,session_id,kind,state,parameters,created_at,updated_at) VALUES(?,?,?,?,'create','pending',?,?,?)`, requestID, uid, agentID, t.ID, string(parameters), t.CreatedAt, t.UpdatedAt)
		created = err == nil
		return err
	})
	if err != nil {
		return TerminalSession{}, false, err
	}
	return t, created, nil
}

// projectOnMachine checks that a folder project is the account's own, on the
// session's machine and in the session's folder (dir, cleaned; a session with
// no folder starts in the home folder, which is not the project's). An unknown
// id and another account's id give the same ErrFolderProjectNotFound, so the
// reply never says whether an id exists.
func projectOnMachine(q interface {
	QueryRow(string, ...any) *sql.Row
}, uid int64, agentID, projectID, dir string) error {
	var same bool
	var folder string
	err := q.QueryRow(`SELECT EXISTS(SELECT 1 FROM sync_agents a WHERE a.id=? AND a.user_id=p.user_id AND a.id=p.host),p.dir FROM folder_projects p WHERE p.user_id=? AND p.id=?`, agentID, uid, projectID).Scan(&same, &folder)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrFolderProjectNotFound
	}
	if err != nil {
		return err
	}
	if !same {
		return ErrProjectMachineMismatch
	}
	if dir == "" || path.Clean(dir) != folder {
		return ErrProjectFolderMismatch
	}
	return nil
}

// TerminalProjectCheckFor answers, before any machine is asked, whether a
// session on agentID in dir may belong to projectID (see projectOnMachine).
func (s *Store) TerminalProjectCheckFor(uid int64, agentID, projectID, dir string) error {
	return projectOnMachine(s.db, uid, agentID, projectID, dir)
}

// RequestTerminalEndFor is deliberately independent of connection freshness or
// local opt-in. An offline request remains pending and retains quota until the
// host acknowledges an ended/exited/lost state. It never touches tmux itself.
func (s *Store) RequestTerminalEndFor(uid int64, agentID, sessionID, requestID string, removeHistory ...bool) (TerminalSession, error) {
	remove := len(removeHistory) > 0 && removeHistory[0]
	parameters := "{}"
	if remove {
		parameters = `{"removeHistory":true}`
	}
	var t TerminalSession
	if !terminalproto.ValidID(requestID) || !terminalproto.ValidID(sessionID) || !terminalproto.ValidID(agentID) {
		return t, terminalproto.ErrInvalid
	}
	err := s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, true); err != nil {
			return err
		}
		var existing, stored string
		err := tx.QueryRow(`SELECT session_id,parameters FROM terminal_operations WHERE user_id=? AND agent_id=? AND kind='end' AND request_id=?`, uid, agentID, requestID).Scan(&existing, &stored)
		if err == nil {
			if existing != sessionID || stored != parameters {
				return ErrTerminalConflict
			}
			t, err = terminalForTx(tx, uid, agentID, sessionID)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		t, err = terminalForTx(tx, uid, agentID, sessionID)
		if err != nil {
			return err
		}
		state, code := "succeeded", string(terminalproto.OK)
		if t.State.ReservesQuota() {
			t.State = terminalproto.EndRequested
			state, code = "pending", ""
			t.UpdatedAt = now()
			if _, err = tx.Exec(`UPDATE terminal_sessions SET state=?,updated_at=? WHERE id=?`, t.State, t.UpdatedAt, t.ID); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`INSERT INTO terminal_operations(request_id,user_id,agent_id,session_id,kind,state,result_code,parameters,created_at,updated_at) VALUES(?,?,?,?,'end',?,?,?,?,?)`, requestID, uid, agentID, sessionID, state, code, parameters, now(), now())
		t.RemoveHistory = t.RemoveHistory || remove
		if err == nil && !t.State.ReservesQuota() {
			err = archiveTerminalOnClose(tx, uid, agentID, sessionID)
		}
		return err
	})
	return t, err
}

type TerminalOperation struct {
	RequestID string
	SessionID string
	Kind      string
	State     string
	Code      terminalproto.ResultCode
	// Reason and Detail explain a worktree_refused create.
	Reason terminalproto.WorktreeReason
	Detail string
	Create *terminalproto.Create
}

func readTerminalOperation(row interface{ Scan(...any) error }) (TerminalOperation, error) {
	var op TerminalOperation
	var raw string
	err := row.Scan(&op.RequestID, &op.SessionID, &op.Kind, &op.State, &op.Code, &op.Reason, &op.Detail, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return op, ErrRemoteNotFound
	}
	if err != nil {
		return op, err
	}
	if op.Kind == "create" {
		var in terminalproto.Create
		if json.Unmarshal([]byte(raw), &in) != nil || in.Validate() != nil {
			return op, terminalproto.ErrInvalid
		}
		op.Create = &in
	}
	return op, nil
}

// TerminalOperationFor also returns durable failure codes to the owning
// browser/broker, so a retry can display the original outcome without dispatch.
func (s *Store) TerminalOperationFor(uid int64, agentID, kind, requestID string) (TerminalOperation, error) {
	return readTerminalOperation(s.db.QueryRow(`SELECT request_id,session_id,kind,state,result_code,reason,detail,parameters FROM terminal_operations WHERE user_id=? AND agent_id=? AND kind=? AND request_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, uid, agentID, kind, requestID, uid))
}

// PendingTerminalOperationsFor is for a freshly authenticated agent control
// connection, not a public API. Ends precede creates during reconciliation.
func (s *Store) PendingTerminalOperationsFor(uid int64, agentID string, generation int64) ([]TerminalOperation, error) {
	var out []TerminalOperation
	err := s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		if _, err := currentGeneration(tx, uid, agentID, generation); err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT request_id,session_id,kind,state,result_code,reason,detail,parameters FROM terminal_operations o WHERE user_id=? AND agent_id=? AND state='pending' AND (kind='end' OR EXISTS(SELECT 1 FROM terminal_sessions t WHERE t.id=o.session_id AND t.state='creating')) ORDER BY kind='end' DESC,created_at,request_id LIMIT 100`, uid, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			op, err := readTerminalOperation(rows)
			if err != nil {
				return err
			}
			out = append(out, op)
		}
		return rows.Err()
	})
	return out, err
}

// archiveTerminalOnClose only runs inside a transaction that has confirmed closure.
func archiveTerminalOnClose(tx *sql.Tx, uid int64, agentID, sessionID string) error {
	_, err := tx.Exec(`INSERT INTO terminal_session_archives(session_id,user_id,agent_id,archived_at) SELECT id,user_id,agent_id,? FROM terminal_sessions WHERE id=? AND user_id=? AND agent_id=? AND state IN ('ended','exited','lost') AND EXISTS(SELECT 1 FROM terminal_operations WHERE session_id=? AND user_id=? AND agent_id=? AND kind='end' AND parameters='{"removeHistory":true}') ON CONFLICT(session_id) DO NOTHING`, now(), sessionID, uid, agentID, sessionID, uid, agentID)
	return err
}

// ObserveTerminalStateFor applies authenticated, generation-fenced reconciliation.
// A late create acknowledgment cannot cancel an already durable end request.
func (s *Store) ObserveTerminalStateFor(uid int64, agentID, sessionID string, generation int64, next terminalproto.SessionState, exitCode *int) error {
	if !next.Valid() || exitCode != nil && (next != terminalproto.Exited || *exitCode < 0 || *exitCode > 255) {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		if _, err := currentGeneration(tx, uid, agentID, generation); err != nil {
			return err
		}
		t, err := terminalForTx(tx, uid, agentID, sessionID)
		if err != nil {
			return err
		}
		observedRunning := next == terminalproto.Running
		if t.State == terminalproto.EndRequested && observedRunning {
			next = terminalproto.EndRequested
		}
		if !terminalproto.CanTransition(t.State, next) {
			return ErrTerminalConflict
		}
		if t.ExitCode != nil && (exitCode == nil || *t.ExitCode != *exitCode) {
			return ErrTerminalConflict
		}
		if _, err := tx.Exec(`UPDATE terminal_sessions SET state=?,exit_code=?,updated_at=? WHERE id=?`, next, exitCode, now(), sessionID); err != nil {
			return err
		}
		// Successful creation is evidence of a live shell, including one awaiting
		// end. Final absence closes uncertain create work without recreating it.
		if observedRunning || !next.ReservesQuota() {
			state, code := "succeeded", terminalproto.OK
			if next == terminalproto.Lost || next == terminalproto.Ended {
				state, code = "failed", terminalproto.NotFound
			}
			if _, err := tx.Exec(`UPDATE terminal_operations SET state=?,result_code=?,updated_at=? WHERE user_id=? AND agent_id=? AND session_id=? AND kind='create' AND state='pending'`, state, code, now(), uid, agentID, sessionID); err != nil {
				return err
			}
		}
		if !next.ReservesQuota() {
			_, err = tx.Exec(`UPDATE terminal_operations SET state='succeeded',result_code='ok',updated_at=? WHERE user_id=? AND agent_id=? AND session_id=? AND kind='end' AND state='pending'`, now(), uid, agentID, sessionID)
			if err == nil {
				err = archiveTerminalOnClose(tx, uid, agentID, sessionID)
			}
		}
		return err
	})
}

// FailTerminalCreateFor records a definitive authenticated rejection by the
// host. A transport timeout is uncertain and must NOT call this method: keep
// the reservation pending and reconcile the host registry after reconnecting.
func (s *Store) FailTerminalCreateFor(uid int64, agentID, requestID string, generation int64, code terminalproto.ResultCode) error {
	return s.FailTerminalCreateWithResultFor(uid, agentID, requestID, generation, terminalproto.Result{Code: code})
}

// validRefusal accepts a failed create's result: a reason only on
// worktree_refused, a detail only with a reason, as plain text.
func validRefusal(r terminalproto.Result) bool {
	if !r.Code.Valid() || r.Code == terminalproto.OK {
		return false
	}
	if r.Reason != "" && (r.Code != terminalproto.WorktreeRefused || !r.Reason.Valid()) {
		return false
	}
	if r.Detail == "" {
		return true
	}
	switch r.Code {
	case terminalproto.ACPUnavailable, terminalproto.ACPVersion, terminalproto.ACPTrustRequired:
		return len(r.Detail) <= 500 && plainLine(r.Detail)
	}
	return r.Reason != "" && len(r.Detail) <= terminalproto.MaxWorktreeDetailBytes && plainLine(r.Detail)
}

func plainLine(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || c == '\u2028' || c == '\u2029' {
			return false
		}
	}
	return true
}

// FailTerminalCreateWithResultFor records a definite refusal of a create,
// keeping a worktree refusal's reason and detail so a replay reports it.
func (s *Store) FailTerminalCreateWithResultFor(uid int64, agentID, requestID string, generation int64, result terminalproto.Result) error {
	code := result.Code
	if !terminalproto.ValidID(requestID) || !validRefusal(result) {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		if _, err := currentGeneration(tx, uid, agentID, generation); err != nil {
			return err
		}
		var sessionID, state, storedCode, storedReason, storedDetail string
		err := tx.QueryRow(`SELECT session_id,state,result_code,reason,detail FROM terminal_operations WHERE user_id=? AND agent_id=? AND request_id=? AND kind='create'`, uid, agentID, requestID).Scan(&sessionID, &state, &storedCode, &storedReason, &storedDetail)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
		if err != nil {
			return err
		}
		if state == "failed" && storedCode == string(code) && storedReason == string(result.Reason) && storedDetail == result.Detail {
			return nil
		}
		if state != "pending" {
			return ErrTerminalConflict
		}
		t, err := terminalForTx(tx, uid, agentID, sessionID)
		if err != nil {
			return err
		}
		if t.State != terminalproto.Creating && t.State != terminalproto.EndRequested {
			return ErrTerminalConflict
		}
		if _, err := tx.Exec(`UPDATE terminal_sessions SET state='lost',updated_at=? WHERE id=?`, now(), sessionID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE terminal_operations SET state='failed',result_code=?,reason=?,detail=?,updated_at=? WHERE user_id=? AND agent_id=? AND session_id=? AND kind='create'`, code, result.Reason, result.Detail, now(), uid, agentID, sessionID); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE terminal_operations SET state='succeeded',result_code='ok',updated_at=? WHERE user_id=? AND agent_id=? AND session_id=? AND kind='end' AND state='pending'`, now(), uid, agentID, sessionID)
		if err == nil {
			err = archiveTerminalOnClose(tx, uid, agentID, sessionID)
		}
		return err
	})
}

// CanResumeACPSessionFor reports whether acpSessionID is the stored handle of
// one of this account's closed structured sessions on agentID. It stops a
// crafted resumeAcp from loading another project's OpenCode history.
func (s *Store) CanResumeACPSessionFor(uid int64, agentID, acpSessionID, dir string) (bool, error) {
	return s.CanResumeACPToolSessionFor(uid, agentID, "opencode", acpSessionID, dir)
}

// CanResumeACPToolSessionFor binds a native history handle to the tool as well
// as the account, machine, folder and closed structured session that owns it.
func (s *Store) CanResumeACPToolSessionFor(uid int64, agentID, tool, acpSessionID, dir string) (bool, error) {
	if !terminalproto.ValidID(agentID) || !terminalproto.ValidACPSessionIDFor(tool, acpSessionID) || dir != "" && !terminalproto.ValidDir(dir) {
		return false, terminalproto.ErrInvalid
	}
	var ok bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM terminal_sessions WHERE user_id=? AND agent_id=? AND tool=? AND structured=1 AND state IN ('ended','exited','lost') AND acp_session_id=? AND dir=?)`, uid, agentID, tool, acpSessionID, dir).Scan(&ok)
	return ok, err
}

// RecordTerminalACPSessionFor stores OpenCode's session id on a structured
// session. It is idempotent for the same id and refuses a different one.
func (s *Store) RecordTerminalACPSessionFor(uid int64, agentID, sessionID, acpSessionID string) error {
	if !terminalproto.ValidID(sessionID) || !terminalproto.ValidACPSessionID(acpSessionID) {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		t, err := terminalForTx(tx, uid, agentID, sessionID)
		if err != nil {
			return err
		}
		if !t.Structured || !terminalproto.ValidACPSessionIDFor(t.Tool, acpSessionID) {
			return ErrTerminalConflict
		}
		if t.ACPSessionID == acpSessionID {
			return nil
		}
		if t.ACPSessionID != "" {
			return ErrTerminalConflict
		}
		_, err = tx.Exec(`UPDATE terminal_sessions SET acp_session_id=?,updated_at=? WHERE id=? AND user_id=? AND agent_id=? AND structured=1 AND acp_session_id=''`, acpSessionID, now(), sessionID, uid, agentID)
		return err
	})
}

// SetTerminalYoloFor records a live structured session's current approval
// mode. The broker asks the machine before writing it; a refusal leaves the
// record as it was.
func (s *Store) SetTerminalYoloFor(uid int64, sessionID string, yolo bool) error {
	if !terminalproto.ValidID(sessionID) {
		return terminalproto.ErrInvalid
	}
	r, err := s.db.Exec(`UPDATE terminal_sessions SET yolo=?,updated_at=? WHERE id=? AND user_id=? AND structured=1 AND state IN ('creating','running','end_requested') AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, yolo, now(), sessionID, uid, uid)
	if err != nil {
		return err
	}
	if n, err := r.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrRemoteNotFound
	}
	return nil
}

// SetTerminalStructuredFor switches a running OpenCode session between the
// structured chat and the raw terminal view. Enabling needs the ACP session
// id the chat resumes from. Only the structured flag changes. Already being
// at the requested value is a no-op.
func (s *Store) SetTerminalStructuredFor(uid int64, sessionID string, structured bool) error {
	if !terminalproto.ValidID(sessionID) {
		return terminalproto.ErrInvalid
	}
	r, err := s.db.Exec(`UPDATE terminal_sessions SET structured=?,updated_at=? WHERE id=? AND user_id=? AND state='running' AND structured<>? AND (?=0 OR acp_session_id<>'') AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, structured, now(), sessionID, uid, structured, structured, uid)
	if err != nil {
		return err
	}
	if n, err := r.RowsAffected(); err != nil {
		return err
	} else if n == 1 {
		return nil
	}
	t, err := s.TerminalSessionFor(uid, sessionID)
	if err != nil {
		return err
	}
	if t.State == terminalproto.Running && t.Structured == structured {
		return nil
	}
	return ErrTerminalConflict
}

func (s *Store) RenameTerminalFor(uid int64, sessionID, title string) error {
	if !terminalproto.ValidTitle(title) {
		return terminalproto.ErrInvalid
	}
	r, err := s.db.Exec(`UPDATE terminal_sessions SET title=?,updated_at=? WHERE id=? AND user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, title, now(), sessionID, uid, uid)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrRemoteNotFound
	}
	return nil
}

const healthRetention = 24 * time.Hour

// RecordTerminalWorktreeFor stores where the machine made a session's
// worktree, as its create reply reported it. Only a session whose create
// asked for a worktree can have one; the branch follows from that create.
// A second report must match the first.
func (s *Store) RecordTerminalWorktreeFor(uid int64, agentID, sessionID, path, repo string) error {
	if !filepath.IsAbs(path) || len(path) > 4096 || !plainLine(path) || !terminalproto.ValidSlug(repo) {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		var raw, storedPath, storedRepo string
		err := tx.QueryRow(`SELECT o.parameters,t.worktree_path,t.worktree_repo FROM terminal_operations o JOIN terminal_sessions t ON t.id=o.session_id AND t.user_id=o.user_id AND t.agent_id=o.agent_id WHERE o.user_id=? AND o.agent_id=? AND o.session_id=? AND o.kind='create'`, uid, agentID, sessionID).Scan(&raw, &storedPath, &storedRepo)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
		if err != nil {
			return err
		}
		var in terminalproto.Create
		if json.Unmarshal([]byte(raw), &in) != nil || in.Worktree == "" {
			return ErrTerminalConflict
		}
		// The folder must be this run's folder under ~/.reet/worktrees.
		if !strings.HasSuffix(path, "/.reet/worktrees/"+in.Project+"/"+repo+"/"+in.Worktree) {
			return terminalproto.ErrInvalid
		}
		if storedPath != "" {
			if storedPath == path && storedRepo == repo {
				return nil
			}
			return ErrTerminalConflict
		}
		_, err = tx.Exec(`UPDATE terminal_sessions SET worktree_path=?,worktree_repo=?,worktree_branch=? WHERE id=? AND user_id=? AND agent_id=?`, path, repo, "reet/"+in.Project+"-"+in.Worktree, sessionID, uid, agentID)
		return err
	})
}

// RecordTerminalModelFor records a model only after the running agent accepted it.
func (s *Store) RecordTerminalModelFor(uid int64, sessionID, model string) error {
	if model == "" || !terminalproto.ValidModel(model) {
		return terminalproto.ErrInvalid
	}
	_, err := s.db.Exec(`UPDATE terminal_sessions SET model=?,updated_at=? WHERE id=? AND user_id=? AND structured=1`, model, now(), sessionID, uid)
	return err
}
