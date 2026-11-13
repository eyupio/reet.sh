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
	// what that worktree started from, and why a refused create was refused.
	for _, c := range []struct{ table, ddl string }{
		{"terminal_sessions", "worktree_path TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "worktree_repo TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "worktree_branch TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "base_relation TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "base_start TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "base_name TEXT NOT NULL DEFAULT ''"},
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
		{"terminal_sessions", "agent_mode TEXT NOT NULL DEFAULT ''"},
		{"terminal_sessions", "prompt TEXT NOT NULL DEFAULT ''"},
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
	if err := s.migrateAttention(); err != nil {
		return err
	}
	return s.migrateTitleOrigin()
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
			`CREATE TABLE IF NOT EXISTS terminal_prompts (
                id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', body TEXT NOT NULL,
                category TEXT NOT NULL DEFAULT '', tool TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
                effort TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL CHECK(revision>0),
                created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
			`CREATE INDEX IF NOT EXISTS terminal_prompts_owner ON terminal_prompts(user_id)`,
			`CREATE TABLE IF NOT EXISTS worktree_cleanups (
				user_id INTEGER NOT NULL, agent_id TEXT NOT NULL, path TEXT NOT NULL, removed_at INTEGER NOT NULL,
				PRIMARY KEY(user_id,agent_id,path))`,
			`CREATE TABLE IF NOT EXISTS terminal_session_archives (
                session_id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, agent_id TEXT NOT NULL,
                archived_at INTEGER NOT NULL,
                FOREIGN KEY(session_id,agent_id,user_id) REFERENCES terminal_sessions(id,agent_id,user_id) ON DELETE CASCADE)`,
			`CREATE TABLE IF NOT EXISTS terminal_chat_events (
				user_id INTEGER NOT NULL, session_id TEXT NOT NULL, seq INTEGER NOT NULL,
				at INTEGER NOT NULL, event TEXT NOT NULL, bytes INTEGER NOT NULL,
				PRIMARY KEY(user_id,session_id,seq))`,
			`CREATE INDEX IF NOT EXISTS terminal_chat_events_transcript ON terminal_chat_events(user_id,session_id,seq)`,
			`CREATE TABLE IF NOT EXISTS terminal_chat_queue (
				seq INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL,
				session_id TEXT NOT NULL REFERENCES terminal_sessions(id) ON DELETE CASCADE,
				id TEXT NOT NULL, text TEXT NOT NULL, images TEXT NOT NULL,
				state TEXT NOT NULL CHECK(state IN ('queued','sending','uncertain','failed','sent','removed')),
				intent TEXT NOT NULL, note TEXT NOT NULL, revision INTEGER NOT NULL, owner TEXT NOT NULL,
				UNIQUE(user_id,session_id,id))`,
			`CREATE INDEX IF NOT EXISTS terminal_chat_queue_session ON terminal_chat_queue(user_id,session_id,state)`,
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
	validRecord := r.FormatVersion == 1 && len(r.Ciphertext) <= 64<<10
	validRaw := err == nil && len(raw) >= 44
	if !validRecord || !validRaw {
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
	validIDs := terminalproto.ValidID(agentID) && terminalproto.ValidID(instanceID)
	if !validIDs || caps.Validate() != nil {
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
	c, err := s.RemoteCapabilitiesRawFor(uid, agentID)
	if err != nil {
		return c, err
	}
	// A gated agent's chat is hidden from an account that has not turned it on.
	// The mask is applied here, when the record is read for use, never to the
	// stored record.
	c.Capabilities = c.Capabilities.WithoutChats(disabledChats(s.db, uid))
	return c, nil
}

// RemoteCapabilitiesRawFor returns the stored record exactly as the connector
// reported it, without the account's chat setting applied. It is for code that
// reads a record in order to write it back; everything else reads
// RemoteCapabilitiesFor.
func (s *Store) RemoteCapabilitiesRawFor(uid int64, agentID string) (RemoteAgentCapabilities, error) {
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
	// BaseRelation and BaseStart are what that worktree started from, as the
	// machine reported it: how the repository's own base stood against the
	// same branch on GitHub (terminalproto.BaseRelation) and the commit the
	// worktree was made at. Both are absent from the JSON for a session whose
	// machine reported none.
	BaseRelation string `json:"baseRelation,omitempty"`
	BaseStart    string `json:"baseStart,omitempty"`
	// BaseName is the branch the machine resolved that base to, which can
	// differ from the one the create named (a create that named none starts
	// from the remote's default). It is what the session's own notice names,
	// so it goes out with the other two; the session view still shows the
	// worktree's branch as the branch the agent works on.
	BaseName string `json:"baseName,omitempty"`
	// Launch metadata for the listing. Effort is the requested launch level;
	// Yolo records the person's confirmation for this session. ProjectID is null
	// for a session started outside a folder project.
	Tool      string  `json:"tool"`
	Model     string  `json:"model"`
	Effort    string  `json:"effort"`
	Yolo      bool    `json:"yolo"`
	ProjectID *string `json:"projectId"`
	// PromptExcerpt is the first 300 characters of the opening prompt and
	// PromptMore says there was more. The listing carries only this much, because
	// it goes to every open tab and rides the event stream; the whole prompt (at
	// most terminalproto.MaxPromptBytes) is read with TerminalPromptFor. The
	// controller holds the prompt only while the session is in the person's
	// history: sessions are never deleted, so archiving is what ends the hold, and
	// both archive paths clear it (clearArchivedPrompt).
	PromptExcerpt string `json:"promptExcerpt,omitempty"`
	PromptMore    bool   `json:"promptMore,omitempty"`
	// Dir is where the agent ran: the worktree it made, else the folder its
	// create named. It is kept on the session so a continuation starts in the
	// same folder even after the project's folder changes.
	Dir string `json:"dir"`
	// Structured is an OpenCode chat over ACP; it has no terminal to attach to.
	Structured bool `json:"structured,omitempty"`
	// ACPSessionID is OpenCode's own session id, the handle for reopening a
	// chat's stored history. It is empty until the machine reports it.
	ACPSessionID string `json:"acpSessionId,omitempty"`
	ActivityAt   int64  `json:"activityAt,omitempty"`
	BellAt       int64  `json:"bellAt,omitempty"`
	CompletedAt  int64  `json:"completedAt,omitempty"`
	ChatStatus   string `json:"chatStatus,omitempty"`
	// AgentMode is the agent mode the chat is confirmed to be in (Codex:
	// "read-only" or "agent"). Empty means the agent offers no modes or the mode
	// is not known, for example after a switch it did not confirm.
	AgentMode         string `json:"agentMode,omitempty"`
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

const terminalSessionCols = `id,agent_id,title,state,created_at,updated_at,exit_code,worktree_path,worktree_repo,worktree_branch,base_relation,base_start,base_name,CASE WHEN worktree_path<>'' THEN worktree_path ELSE dir END,tool,model,effort,yolo,project_id,structured,acp_session_id,activity_at,bell_at,attention_observed,EXISTS(SELECT 1 FROM terminal_operations o WHERE o.session_id=terminal_sessions.id AND o.user_id=terminal_sessions.user_id AND o.agent_id=terminal_sessions.agent_id AND o.kind='end' AND o.parameters='{"removeHistory":true}'),completed_at,chat_status,agent_mode,substr(prompt,1,300),length(prompt)>300`

func readTerminal(row interface{ Scan(...any) error }) (TerminalSession, error) {
	var t TerminalSession
	var exit sql.NullInt64
	var project sql.NullString
	var attentionObserved int
	err := row.Scan(&t.ID, &t.AgentID, &t.Title, &t.State, &t.CreatedAt, &t.UpdatedAt, &exit, &t.WorktreePath, &t.WorktreeRepo, &t.WorktreeBranch, &t.BaseRelation, &t.BaseStart, &t.BaseName, &t.Dir, &t.Tool, &t.Model, &t.Effort, &t.Yolo, &project, &t.Structured, &t.ACPSessionID, &t.ActivityAt, &t.BellAt, &attentionObserved, &t.RemoveHistory, &t.CompletedAt, &t.ChatStatus, &t.AgentMode, &t.PromptExcerpt, &t.PromptMore)
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

// TerminalPromptFor is the whole opening prompt of one of the account's
// sessions: "" for a session that had none and for one that has been archived,
// whose prompt is cleared. An unknown session and another account's answer
// alike, under the conditions TerminalSessionFor applies. This is the only read
// of the full text; callers must not log it or write it into an audit detail.
func (s *Store) TerminalPromptFor(uid int64, sessionID string) (string, error) {
	if !terminalproto.ValidID(sessionID) {
		return "", terminalproto.ErrInvalid
	}
	var prompt string
	err := s.db.QueryRow(`SELECT prompt FROM terminal_sessions WHERE id=? AND user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, sessionID, uid, uid).Scan(&prompt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrRemoteNotFound
	}
	return prompt, err
}

func (s *Store) TerminalSessionsFor(uid int64) ([]TerminalSession, error) {
	rows, err := s.readStmts.query(`SELECT `+terminalSessionCols+` FROM terminal_sessions WHERE user_id=? AND NOT EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.session_id=terminal_sessions.id) AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer') ORDER BY state IN ('creating','running','end_requested') DESC,updated_at DESC,id LIMIT 100`, uid, uid)
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
	seen, err := s.terminalSeenFor(uid, out)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].SeenAt = seen[out[i].ID]
		applyAttention(&out[i], nowTime)
	}
	return out, nil
}

// TerminalCleanupCandidatesFor lists one machine's sessions whose end asked to
// remove history, that are closed and still hold a not-yet-archived worktree.
// Cleanup removes the worktree, then archives the session. It is how a
// controller restart or a reconnect resumes a cleanup that was interrupted.
func (s *Store) TerminalCleanupCandidatesFor(uid int64, agentID string, limit int) ([]TerminalSession, error) {
	rows, err := s.readStmts.query(`SELECT `+terminalSessionCols+` FROM terminal_sessions
		WHERE user_id=? AND agent_id=? AND worktree_path<>'' AND state IN ('ended','exited','lost')
		AND NOT EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.session_id=terminal_sessions.id)
		AND EXISTS(SELECT 1 FROM terminal_operations o WHERE o.session_id=terminal_sessions.id AND o.user_id=terminal_sessions.user_id AND o.agent_id=terminal_sessions.agent_id AND o.kind='end' AND o.parameters='{"removeHistory":true}')
		ORDER BY updated_at,id LIMIT ?`, uid, agentID, limit)
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
	return out, nil
}

// terminalSeenFor reads the seen stamps for a page of sessions in one query.
func (s *Store) terminalSeenFor(uid int64, sessions []TerminalSession) (map[string]int64, error) {
	seen := make(map[string]int64, len(sessions))
	if len(sessions) == 0 {
		return seen, nil
	}
	args := make([]any, 0, len(sessions)+1)
	args = append(args, uid)
	for _, session := range sessions {
		args = append(args, session.ID)
	}
	rows, err := s.readStmts.query(`SELECT session_id,seen_at FROM terminal_session_seen WHERE user_id=? AND session_id IN (`+placeholders(len(sessions))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var at int64
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		seen[id] = at
	}
	return seen, rows.Err()
}

// ArchiveTerminalFor hides closed or end-requested sessions from browser history.
// Hiding a pending end does not confirm termination or release its quota.
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
		case terminalproto.EndRequested, terminalproto.Ended, terminalproto.Exited, terminalproto.Lost:
		default:
			return ErrTerminalConflict
		}
		_, err = tx.Exec(`INSERT INTO terminal_session_archives(session_id,user_id,agent_id,archived_at) VALUES(?,?,?,?) ON CONFLICT(session_id) DO NOTHING`, session.ID, uid, session.AgentID, now())
		if err != nil {
			return err
		}
		// What its agent reported using goes with the session. A finished
		// pipeline stage has already kept its own figures.
		if _, err = tx.Exec(`DELETE FROM terminal_session_usage WHERE user_id=? AND session_id=?`, uid, session.ID); err != nil {
			return err
		}
		if err = clearArchivedPrompt(tx, uid, session.ID); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE terminal_chat_queue SET text='',images='[]',state='removed',note='',revision=revision+1 WHERE session_id=? AND user_id=?`, session.ID, uid)
		return err
	})
}

// clearArchivedPrompt drops the opening prompt of a session that has an archive
// row. Sessions are never deleted, only hidden, so without this the controller
// would hold what the person typed for as long as the database lives. It runs
// straight after each archive INSERT, in the same transaction, and does nothing
// for a session that is not archived.
func clearArchivedPrompt(tx *sql.Tx, uid int64, sessionID string) error {
	if _, err := tx.Exec(`UPDATE terminal_sessions SET prompt='' WHERE id=? AND user_id=? AND EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.session_id=terminal_sessions.id)`, sessionID, uid); err != nil {
		return err
	}
	// The stored create request carries the prompt too (the retry comparison needs
	// the whole request). Dispatch reads a create only while the session is
	// creating, and archiving needs a closed or end-requested one, so nothing
	// sends it again. A retry of an archived create that had a prompt no longer
	// matches and answers ErrTerminalConflict instead of replaying.
	_, err := tx.Exec(`UPDATE terminal_operations SET parameters=json_remove(parameters,'$.prompt') WHERE session_id=? AND user_id=? AND kind='create' AND json_valid(parameters) AND EXISTS(SELECT 1 FROM terminal_session_archives a WHERE a.session_id=terminal_operations.session_id)`, sessionID, uid)
	return err
}

// ReserveTerminalCreateFor only reserves metadata/quota. The future broker must
// verify a live authenticated connection BEFORE calling it, then dispatch using
// the returned session ID; it must not queue new shells for unreachable hosts.
// Retries return the original record even after connection generations change.
func (s *Store) ReserveTerminalCreateFor(uid int64, agentID, requestID string, generation int64, in terminalproto.Create) (TerminalSession, bool, error) {
	var t TerminalSession
	created := false
	validIDs := terminalproto.ValidID(agentID) && terminalproto.ValidID(requestID)
	if !validIDs || in.Validate() != nil {
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
			projectMatches := (t.ProjectID == nil && in.ProjectID == "") ||
				(t.ProjectID != nil && *t.ProjectID == in.ProjectID)
			if !projectMatches {
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
		t = TerminalSession{ID: syncID(), AgentID: agentID, Title: in.Title, State: terminalproto.Creating, CreatedAt: now(), UpdatedAt: now(), Tool: in.Tool, Model: in.Model, Effort: in.Effort, Yolo: in.Yolo, Dir: in.Dir, Structured: in.Structured, ACPSessionID: in.ResumeACP}
		if project.Valid {
			t.ProjectID = &project.String
		}
		origin := in.TitleOrigin
		if !terminalproto.ValidTitleOrigin(string(origin)) {
			origin = terminalproto.TitleManual
		}
		// The prompt is written here and nowhere else, so a retry (which returns
		// the recorded session above) can never change it.
		if _, err := tx.Exec(`INSERT INTO terminal_sessions(id,user_id,agent_id,title,state,created_at,updated_at,tool,model,effort,yolo,project_id,dir,structured,acp_session_id,prompt,title_origin) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, t.ID, uid, agentID, t.Title, t.State, t.CreatedAt, t.UpdatedAt, t.Tool, t.Model, t.Effort, t.Yolo, project, t.Dir, t.Structured, t.ACPSessionID, in.Prompt, origin); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO terminal_operations(request_id,user_id,agent_id,session_id,kind,state,parameters,created_at,updated_at) VALUES(?,?,?,?,'create','pending',?,?,?)`, requestID, uid, agentID, t.ID, string(parameters), t.CreatedAt, t.UpdatedAt); err != nil {
			return err
		}
		// Read the row back so a first create and a retry return the same
		// excerpt, cut by the one rule in terminalSessionCols.
		t, err = terminalForTx(tx, uid, agentID, t.ID)
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
	validIDs := terminalproto.ValidID(requestID) &&
		terminalproto.ValidID(sessionID) && terminalproto.ValidID(agentID)
	if !validIDs {
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
// A session that worked in a worktree stays in history until that worktree is
// gone: the controller removes it once the machine is no longer using it, so
// the archive never leaves a worktree behind.
func archiveTerminalOnClose(tx *sql.Tx, uid int64, agentID, sessionID string) error {
	_, err := tx.Exec(`INSERT INTO terminal_session_archives(session_id,user_id,agent_id,archived_at) SELECT id,user_id,agent_id,? FROM terminal_sessions WHERE id=? AND user_id=? AND agent_id=? AND worktree_path='' AND state IN ('ended','exited','lost') AND EXISTS(SELECT 1 FROM terminal_operations WHERE session_id=? AND user_id=? AND agent_id=? AND kind='end' AND parameters='{"removeHistory":true}') ON CONFLICT(session_id) DO NOTHING`, now(), sessionID, uid, agentID, sessionID, uid, agentID)
	if err != nil {
		return err
	}
	return clearArchivedPrompt(tx, uid, sessionID)
}

// ObserveTerminalStateFor applies authenticated, generation-fenced reconciliation.
// A late create acknowledgment cannot cancel an already durable end request.
func (s *Store) ObserveTerminalStateFor(uid int64, agentID, sessionID string, generation int64, next terminalproto.SessionState, exitCode *int) error {
	validState := next.Valid()
	validExit := exitCode == nil ||
		(next == terminalproto.Exited && *exitCode >= 0 && *exitCode <= 255)
	if !validState || !validExit {
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
		closedChat := t.Structured && next == terminalproto.Exited
		endRequested := t.State == terminalproto.EndRequested || t.State == terminalproto.Ended
		if closedChat && endRequested {
			// A chat's machine answers a person's End with exited (code 0);
			// that close was asked for, so it is recorded as ended and never
			// reconnected in place. A repeat of that receipt stays ended.
			next, exitCode = terminalproto.Ended, nil
		}
		closed := t.State == terminalproto.Lost || t.State == terminalproto.Exited
		if closed && next == terminalproto.Running {
			// A structured chat can be reopened (from lost or exited) from the
			// agent's own store after the machine's reet process restarted; the machine then
			// reports it running under the same reet session id. A plain
			// shell is never resurrected.
			if !t.Structured {
				return ErrTerminalConflict
			}
		} else if !terminalproto.CanTransition(t.State, next) {
			return ErrTerminalConflict
		}
		// A structured reopen to running clears the recorded exit code; every
		// other observation must repeat the recorded code exactly.
		reopened := (t.State == terminalproto.Lost || t.State == terminalproto.Exited) && next == terminalproto.Running
		if !reopened && t.ExitCode != nil {
			if exitCode == nil || *t.ExitCode != *exitCode {
				return ErrTerminalConflict
			}
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
	invalidReason := r.Code != terminalproto.WorktreeRefused || !r.Reason.Valid()
	if r.Reason != "" && invalidReason {
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
		isControl := unicode.IsControl(c) || unicode.Is(unicode.Cf, c)
		isSeparator := c == '\u2028' || c == '\u2029'
		if isControl || isSeparator {
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
		sameFailure := state == "failed" && storedCode == string(code)
		sameDetail := storedReason == string(result.Reason) && storedDetail == result.Detail
		if sameFailure && sameDetail {
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
		// A definite chat refusal never confirmed the requested model. Keep
		// the operation fingerprint for replay, but do not display that request
		// as a confirmed choice on the failed chat. Terminal metadata is unchanged.
		if _, err := tx.Exec(`UPDATE terminal_sessions SET state='lost',model=CASE WHEN structured=1 THEN '' ELSE model END,effort=CASE WHEN structured=1 THEN '' ELSE effort END,updated_at=? WHERE id=?`, now(), sessionID); err != nil {
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
	validAgent := terminalproto.ValidID(agentID)
	validACP := terminalproto.ValidACPSessionIDFor(tool, acpSessionID)
	validDir := dir == "" || terminalproto.ValidDir(dir)
	validTarget := validAgent && validACP
	if !validTarget || !validDir {
		return false, terminalproto.ErrInvalid
	}
	// A row whose create operation failed is a continuation that never
	// started; resuming from it would pile up sessions, so it is excluded.
	var ok bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM terminal_sessions WHERE user_id=? AND agent_id=? AND tool=? AND structured=1 AND state IN ('ended','exited','lost') AND acp_session_id=? AND dir=? AND NOT EXISTS(SELECT 1 FROM terminal_operations o WHERE o.session_id=terminal_sessions.id AND o.user_id=terminal_sessions.user_id AND o.kind='create' AND o.state='failed'))`, uid, agentID, tool, acpSessionID, dir).Scan(&ok)
	return ok, err
}

// RecordTerminalACPSessionFor stores a reviewed adapter's own session id on a
// structured session. It is idempotent for the same id and refuses a different
// one. The shape check is per adapter: a handle is bound to the session's tool
// as well as to this account, machine and session.
func (s *Store) RecordTerminalACPSessionFor(uid int64, agentID, sessionID, acpSessionID string) error {
	if !terminalproto.ValidID(sessionID) || !terminalproto.ValidACPHandle(acpSessionID) {
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
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrRemoteNotFound
	}
	return nil
}

// SetTerminalStructuredFor switches a running OpenCode session between the
// structured chat and the raw terminal view. Enabling needs the ACP session
// id the chat resumes from. Only the structured flag changes. Already being
// at the requested value is a no-op for a running row; a closed row returns
// ErrTerminalConflict even when it is already at the requested value.
func (s *Store) SetTerminalStructuredFor(uid int64, sessionID string, structured bool) error {
	if !terminalproto.ValidID(sessionID) {
		return terminalproto.ErrInvalid
	}
	r, err := s.db.Exec(`UPDATE terminal_sessions SET structured=?,updated_at=? WHERE id=? AND user_id=? AND state='running' AND structured<>? AND (?=0 OR acp_session_id<>'') AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, structured, now(), sessionID, uid, structured, structured, uid)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
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
	r, err := s.db.Exec(`UPDATE terminal_sessions SET title=?,title_origin='manual',updated_at=? WHERE id=? AND user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, title, now(), sessionID, uid, uid)
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
	validPath := filepath.IsAbs(path) && len(path) <= 4096 && plainLine(path)
	if !validPath || !terminalproto.ValidSlug(repo) {
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

// RecordTerminalBaseFor stores what a session's worktree started from, as its
// create reply reported it: the branch the machine resolved, how the
// repository's own base stood against the same branch on GitHub, and the
// commit the worktree was made at. Only a
// session whose worktree has been recorded can have one. The first report is
// what the session started from: the same report again is accepted, a
// different one is a conflict and nothing changes.
func (s *Store) RecordTerminalBaseFor(uid int64, agentID, sessionID string, base terminalproto.BaseState) error {
	if !base.Valid() {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		var worktree, relation, start, name string
		err := tx.QueryRow(`SELECT worktree_path,base_relation,base_start,base_name FROM terminal_sessions WHERE id=? AND user_id=? AND agent_id=?`, sessionID, uid, agentID).Scan(&worktree, &relation, &start, &name)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
		if err != nil {
			return err
		}
		if worktree == "" {
			return ErrTerminalConflict
		}
		if relation != "" {
			if relation == string(base.Relation) && start == base.Start && name == base.Base {
				return nil
			}
			return ErrTerminalConflict
		}
		_, err = tx.Exec(`UPDATE terminal_sessions SET base_relation=?,base_start=?,base_name=? WHERE id=? AND user_id=? AND agent_id=?`, string(base.Relation), base.Start, base.Base, sessionID, uid, agentID)
		return err
	})
}

// RecordTerminalAgentModeFor records the agent mode a chat is confirmed to be in.
// An empty mode records that the mode is not known.
func (s *Store) RecordTerminalAgentModeFor(uid int64, sessionID, mode string) error {
	if mode != "" && !terminalproto.ValidAgentMode(mode) {
		return terminalproto.ErrInvalid
	}
	_, err := s.db.Exec(`UPDATE terminal_sessions SET agent_mode=?,updated_at=? WHERE id=? AND user_id=? AND structured=1`, mode, now(), sessionID, uid)
	return err
}

// RecordTerminalModelFor records a confirmed model, or clears it when a sent
// switch was not confirmed. Empty makes no claim about the agent's current model.
func (s *Store) RecordTerminalModelFor(uid int64, sessionID, model string) error {
	if model != "" && !terminalproto.ValidModel(model) {
		return terminalproto.ErrInvalid
	}
	_, err := s.db.Exec(`UPDATE terminal_sessions SET model=?,updated_at=? WHERE id=? AND user_id=? AND structured=1`, model, now(), sessionID, uid)
	return err
}

// RecordTerminalControlsFor stores one confirmed runtime snapshot atomically.
func (s *Store) RecordTerminalControlsFor(uid int64, sessionID, model, effort string) error {
	invalidModel := model != "" && !terminalproto.ValidModel(model)
	invalidEffort := effort != "" && !terminalproto.ValidModel(effort)
	if invalidModel || invalidEffort {
		return terminalproto.ErrInvalid
	}
	_, err := s.db.Exec(`UPDATE terminal_sessions SET model=?,effort=?,updated_at=? WHERE id=? AND user_id=? AND structured=1 AND (model!=? OR effort!=?)`, model, effort, now(), sessionID, uid, model, effort)
	return err
}

// TerminalArchivedFor reports whether a person's session was archived (hidden
// from browser history, its transcript gone). An unknown or foreign session is
// not archived.
func (s *Store) TerminalArchivedFor(uid int64, sessionID string) (bool, error) {
	if !terminalproto.ValidID(sessionID) {
		return false, terminalproto.ErrInvalid
	}
	var ok bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM terminal_session_archives WHERE session_id=? AND user_id=?)`, sessionID, uid).Scan(&ok)
	return ok, err
}

// TerminalEndRequestedFor reports whether a person asked to end sessionID
// (an end operation exists for it on agentID in this account).
func (s *Store) TerminalEndRequestedFor(uid int64, agentID, sessionID string) (bool, error) {
	if !terminalproto.ValidID(agentID) || !terminalproto.ValidID(sessionID) {
		return false, terminalproto.ErrInvalid
	}
	var ok bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM terminal_operations WHERE user_id=? AND agent_id=? AND session_id=? AND kind='end')`, uid, agentID, sessionID).Scan(&ok)
	return ok, err
}
