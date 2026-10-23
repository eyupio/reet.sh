package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	ErrRemoteNotFound   = errors.New("remote machine record unavailable")
	ErrTerminalConflict = errors.New("terminal operation conflicts with existing state")
	ErrTerminalCapacity = errors.New("terminal session capacity reached")
	ErrTerminalNotReady = errors.New("terminal is not ready")
)

func (s *Store) migrateTerminal() error {
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
	ID        string                     `json:"id"`
	AgentID   string                     `json:"agentId"`
	Title     string                     `json:"title"`
	State     terminalproto.SessionState `json:"state"`
	CreatedAt int64                      `json:"createdAt"`
	UpdatedAt int64                      `json:"updatedAt"`
	ExitCode  *int                       `json:"exitCode"`
}

const terminalSessionCols = `id,agent_id,title,state,created_at,updated_at,exit_code`

func readTerminal(row interface{ Scan(...any) error }) (TerminalSession, error) {
	var t TerminalSession
	var exit sql.NullInt64
	err := row.Scan(&t.ID, &t.AgentID, &t.Title, &t.State, &t.CreatedAt, &t.UpdatedAt, &exit)
	if exit.Valid {
		n := int(exit.Int64)
		t.ExitCode = &n
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
	return readTerminal(s.db.QueryRow(`SELECT `+terminalSessionCols+` FROM terminal_sessions WHERE id=? AND user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, sessionID, uid, uid))
}
func (s *Store) TerminalSessionsFor(uid int64) ([]TerminalSession, error) {
	rows, err := s.db.Query(`SELECT `+terminalSessionCols+` FROM terminal_sessions WHERE user_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer') ORDER BY state IN ('creating','running','end_requested') DESC,updated_at DESC,id LIMIT 100`, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TerminalSession{}
	for rows.Next() {
		t, err := readTerminal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
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
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
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
		t = TerminalSession{ID: syncID(), AgentID: agentID, Title: in.Title, State: terminalproto.Creating, CreatedAt: now(), UpdatedAt: now()}
		if _, err := tx.Exec(`INSERT INTO terminal_sessions(id,user_id,agent_id,title,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, t.ID, uid, agentID, t.Title, t.State, t.CreatedAt, t.UpdatedAt); err != nil {
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

// RequestTerminalEndFor is deliberately independent of connection freshness or
// local opt-in. An offline request remains pending and retains quota until the
// host acknowledges an ended/exited/lost state. It never touches tmux itself.
func (s *Store) RequestTerminalEndFor(uid int64, agentID, sessionID, requestID string) (TerminalSession, error) {
	var t TerminalSession
	if !terminalproto.ValidID(requestID) || !terminalproto.ValidID(sessionID) || !terminalproto.ValidID(agentID) {
		return t, terminalproto.ErrInvalid
	}
	err := s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, true); err != nil {
			return err
		}
		var existing string
		err := tx.QueryRow(`SELECT session_id FROM terminal_operations WHERE user_id=? AND agent_id=? AND kind='end' AND request_id=?`, uid, agentID, requestID).Scan(&existing)
		if err == nil {
			if existing != sessionID {
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
		_, err = tx.Exec(`INSERT INTO terminal_operations(request_id,user_id,agent_id,session_id,kind,state,result_code,created_at,updated_at) VALUES(?,?,?,?,'end',?,?,?,?)`, requestID, uid, agentID, sessionID, state, code, now(), now())
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
	Create    *terminalproto.Create
}

func readTerminalOperation(row interface{ Scan(...any) error }) (TerminalOperation, error) {
	var op TerminalOperation
	var raw string
	err := row.Scan(&op.RequestID, &op.SessionID, &op.Kind, &op.State, &op.Code, &raw)
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
	return readTerminalOperation(s.db.QueryRow(`SELECT request_id,session_id,kind,state,result_code,parameters FROM terminal_operations WHERE user_id=? AND agent_id=? AND kind=? AND request_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`, uid, agentID, kind, requestID, uid))
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
		rows, err := tx.Query(`SELECT request_id,session_id,kind,state,result_code,parameters FROM terminal_operations o WHERE user_id=? AND agent_id=? AND state='pending' AND (kind='end' OR EXISTS(SELECT 1 FROM terminal_sessions t WHERE t.id=o.session_id AND t.state='creating')) ORDER BY kind='end' DESC,created_at,request_id LIMIT 100`, uid, agentID)
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
		}
		return err
	})
}

// FailTerminalCreateFor records a definitive authenticated rejection by the
// host. A transport timeout is uncertain and must NOT call this method: keep
// the reservation pending and reconcile the host registry after reconnecting.
func (s *Store) FailTerminalCreateFor(uid int64, agentID, requestID string, generation int64, code terminalproto.ResultCode) error {
	if !terminalproto.ValidID(requestID) || !code.Valid() || code == terminalproto.OK {
		return terminalproto.ErrInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := lockRemoteAgent(tx, uid, agentID, false); err != nil {
			return err
		}
		if _, err := currentGeneration(tx, uid, agentID, generation); err != nil {
			return err
		}
		var sessionID, state, storedCode string
		err := tx.QueryRow(`SELECT session_id,state,result_code FROM terminal_operations WHERE user_id=? AND agent_id=? AND request_id=? AND kind='create'`, uid, agentID, requestID).Scan(&sessionID, &state, &storedCode)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
		if err != nil {
			return err
		}
		if state == "failed" && storedCode == string(code) {
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
		if _, err := tx.Exec(`UPDATE terminal_operations SET state='failed',result_code=?,updated_at=? WHERE user_id=? AND agent_id=? AND session_id=? AND kind='create'`, code, now(), uid, agentID, sessionID); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE terminal_operations SET state='succeeded',result_code='ok',updated_at=? WHERE user_id=? AND agent_id=? AND session_id=? AND kind='end' AND state='pending'`, now(), uid, agentID, sessionID)
		return err
	})
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
