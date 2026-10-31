package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Software batch states. A batch is pending until its driver claims the first
// member, running while members are being applied, stopped at the first failed
// member (while later members wait), and done or abandoned when it ends.
const (
	SoftwareBatchPending   = "pending"
	SoftwareBatchRunning   = "running"
	SoftwareBatchStopped   = "stopped"
	SoftwareBatchDone      = "done"
	SoftwareBatchAbandoned = "abandoned"
)

// Software batch member states. Excluded members never run: pre-flight found
// them unable to apply, or another active batch already holds the machine.
const (
	SoftwareMemberPending  = "pending"
	SoftwareMemberRunning  = "running"
	SoftwareMemberApplied  = "applied"
	SoftwareMemberFailed   = "failed"
	SoftwareMemberSkipped  = "skipped"
	SoftwareMemberExcluded = "excluded"
)

var (
	ErrSoftwareBatchNotFound         = errors.New("software batch not found")
	ErrSoftwareBatchInProgress       = errors.New("another software batch is still active")
	ErrSoftwareBatchNoMembers        = errors.New("a batch needs at least one machine that can apply")
	ErrSoftwareBatchMemberNotFound   = errors.New("software batch member not found")
	ErrSoftwareBatchMemberNotPending = errors.New("software batch member is not pending")
	ErrSoftwareBatchMemberNotRunning = errors.New("software batch member is not running")
	ErrSoftwareBatchMemberNotFailed  = errors.New("only a failed software batch member can be retried")
	ErrSoftwareBatchFinished         = errors.New("that software batch has already finished")
	ErrSoftwareBatchRunning          = errors.New("a machine in that batch is still applying")
)

// SoftwareBatchMember is one machine in a batch. The expected fields are the
// evidence a person reviewed when the batch was made; every attempt applies
// that plan, and the connector re-checks the live evidence before it acts.
type SoftwareBatchMember struct {
	Position         int    `json:"position"`
	AgentID          string `json:"agentId"`
	Host             string `json:"host"`
	Action           string `json:"action,omitempty"`
	Target           string `json:"target,omitempty"`
	ExpectedState    string `json:"expectedState,omitempty"`
	ExpectedVersion  string `json:"expectedVersion,omitempty"`
	ExpectedPrevious string `json:"expectedPrevious,omitempty"`
	ExpectedManaged  bool   `json:"expectedManaged,omitempty"`
	State            string `json:"state"`
	Code             string `json:"code,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Version          string `json:"version,omitempty"`
}

// SoftwareBatch is one reviewed maintenance batch for an account.
type SoftwareBatch struct {
	ID        string                `json:"id"`
	Tool      string                `json:"tool"`
	State     string                `json:"state"`
	CreatedAt int64                 `json:"createdAt"`
	UpdatedAt int64                 `json:"updatedAt"`
	Members   []SoftwareBatchMember `json:"members"`
}

// SoftwareBatchMemberInput is one machine as the caller selected it. Excluded
// members carry a reason and never run; the store may exclude more of them
// when a machine is already held by another active batch.
type SoftwareBatchMemberInput struct {
	Position         int
	AgentID          string
	Host             string
	Action           string
	Target           string
	ExpectedState    string
	ExpectedVersion  string
	ExpectedPrevious string
	ExpectedManaged  bool
	Excluded         bool
	Reason           string
}

// PendingSoftwareBatch names a batch a restarted controller should resume.
type PendingSoftwareBatch struct {
	UID     int64
	BatchID string
}

func (s *Store) migrateSoftwareBatches() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS software_batches (
			id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			tool TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','running','stopped','done','abandoned')),
			request_id TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
			UNIQUE(id,user_id))`,
		// A create request id names one batch per account.
		`CREATE UNIQUE INDEX IF NOT EXISTS software_batches_request ON software_batches(user_id,request_id) WHERE request_id<>''`,
		`CREATE INDEX IF NOT EXISTS software_batches_active ON software_batches(user_id,state)`,
		`CREATE TABLE IF NOT EXISTS software_batch_members (
			batch_id TEXT NOT NULL, user_id INTEGER NOT NULL,
			position INTEGER NOT NULL CHECK(position>=0),
			agent_id TEXT NOT NULL, host TEXT NOT NULL,
			action TEXT NOT NULL DEFAULT '', target TEXT NOT NULL DEFAULT '',
			expected_state TEXT NOT NULL DEFAULT '', expected_version TEXT NOT NULL DEFAULT '',
			expected_previous TEXT NOT NULL DEFAULT '', expected_managed INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','running','applied','failed','skipped','excluded')),
			code TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(batch_id,position),
			FOREIGN KEY(batch_id,user_id) REFERENCES software_batches(id,user_id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS software_batch_members_pending ON software_batch_members(state,user_id,batch_id)`,
		`CREATE INDEX IF NOT EXISTS software_batch_members_agent ON software_batch_members(user_id,agent_id,state)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// CreateSoftwareBatchFor stores an ordered batch. It refuses with
// ErrSoftwareBatchInProgress while another batch is active for uid, and stores
// any machine already held by another active batch as excluded rather than
// running it twice. A repeated requestID returns the batch it already made.
func (s *Store) CreateSoftwareBatchFor(uid int64, requestID, tool string, members []SoftwareBatchMemberInput) (SoftwareBatch, bool, error) {
	if len(members) == 0 {
		return SoftwareBatch{}, false, ErrSoftwareBatchNoMembers
	}
	var out SoftwareBatch
	created := true
	err := s.withTx(func(tx *sql.Tx) error {
		if requestID != "" {
			var existing string
			err := tx.QueryRow(`SELECT id FROM software_batches WHERE user_id=? AND request_id=?`, uid, requestID).Scan(&existing)
			if err == nil {
				out, err = loadSoftwareBatch(tx, uid, existing)
				created = false
				return err
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		var active int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM software_batches WHERE user_id=? AND state IN ('pending','running','stopped')`, uid).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return ErrSoftwareBatchInProgress
		}
		id, err := projectRandomID()
		if err != nil {
			return err
		}
		t := now()
		if _, err := tx.Exec(`INSERT INTO software_batches(id,user_id,tool,state,request_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
			id, uid, tool, SoftwareBatchPending, requestID, t, t); err != nil {
			return err
		}
		positions := map[int]bool{}
		agents := map[string]bool{}
		included := 0
		for _, m := range members {
			if m.Position < 0 || positions[m.Position] || agents[m.AgentID] {
				return fmt.Errorf("%w: duplicate position or machine", ErrSoftwareBatchNoMembers)
			}
			positions[m.Position] = true
			agents[m.AgentID] = true
			state, code, reason := SoftwareMemberPending, "", ""
			if m.Excluded {
				state, reason = SoftwareMemberExcluded, m.Reason
				if !shortPlain(reason, 240) {
					return fmt.Errorf("%w: an exclusion reason is too long", ErrSoftwareBatchNoMembers)
				}
			} else if host, ok, err := enrolledHost(tx, uid, m.AgentID); err != nil {
				return err
			} else if !ok {
				state, code, reason = SoftwareMemberExcluded, "unknown_machine", "That machine isn't enrolled."
			} else {
				m.Host = host
				busy, err := agentInActiveBatch(tx, uid, m.AgentID)
				if err != nil {
					return err
				}
				if busy {
					state, code, reason = SoftwareMemberExcluded, "in_another_batch", "That machine is already in another batch."
				} else {
					included++
				}
			}
			if _, err := tx.Exec(`INSERT INTO software_batch_members(batch_id,user_id,position,agent_id,host,action,target,expected_state,expected_version,expected_previous,expected_managed,state,code,reason,version,updated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				id, uid, m.Position, m.AgentID, m.Host, m.Action, m.Target, m.ExpectedState, m.ExpectedVersion, m.ExpectedPrevious, boolInt(m.ExpectedManaged), state, code, reason, "", t); err != nil {
				return err
			}
		}
		if included == 0 {
			return ErrSoftwareBatchNoMembers
		}
		out, err = loadSoftwareBatch(tx, uid, id)
		return err
	})
	return out, created, err
}

func enrolledHost(tx *sql.Tx, uid int64, agentID string) (string, bool, error) {
	var host string
	err := tx.QueryRow(`SELECT host FROM sync_agents WHERE id=? AND user_id=?`, agentID, uid).Scan(&host)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return host, err == nil, err
}

func agentInActiveBatch(tx *sql.Tx, uid int64, agentID string) (bool, error) {
	var n int
	err := tx.QueryRow(`SELECT COUNT(*) FROM software_batch_members m
		JOIN software_batches b ON b.id=m.batch_id AND b.user_id=m.user_id
		WHERE m.user_id=? AND m.agent_id=? AND m.state IN ('pending','running')
		AND b.state IN ('pending','running','stopped')`, uid, agentID).Scan(&n)
	return n > 0, err
}

// SoftwareBatchFor reads one batch and its members in position order.
func (s *Store) SoftwareBatchFor(uid int64, batchID string) (SoftwareBatch, error) {
	return loadSoftwareBatch(s.db, uid, batchID)
}

// ActiveSoftwareBatchFor reads the person's pending, running or stopped batch,
// or false when there is none. At most one can exist.
func (s *Store) ActiveSoftwareBatchFor(uid int64) (SoftwareBatch, bool, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM software_batches WHERE user_id=? AND state IN ('pending','running','stopped') ORDER BY created_at DESC LIMIT 1`, uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return SoftwareBatch{}, false, nil
	}
	if err != nil {
		return SoftwareBatch{}, false, err
	}
	batch, err := loadSoftwareBatch(s.db, uid, id)
	return batch, err == nil, err
}

type softwareBatchQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

const softwareBatchCols = `id,tool,state,created_at,updated_at`

func loadSoftwareBatch(q softwareBatchQuerier, uid int64, batchID string) (SoftwareBatch, error) {
	var b SoftwareBatch
	err := q.QueryRow(`SELECT `+softwareBatchCols+` FROM software_batches WHERE id=? AND user_id=?`, batchID, uid).
		Scan(&b.ID, &b.Tool, &b.State, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SoftwareBatch{}, ErrSoftwareBatchNotFound
	}
	if err != nil {
		return SoftwareBatch{}, err
	}
	rows, err := q.Query(`SELECT position,agent_id,host,action,target,expected_state,expected_version,expected_previous,expected_managed,state,code,reason,version
		FROM software_batch_members WHERE batch_id=? AND user_id=? ORDER BY position`, batchID, uid)
	if err != nil {
		return SoftwareBatch{}, err
	}
	defer rows.Close()
	b.Members = []SoftwareBatchMember{}
	for rows.Next() {
		var m SoftwareBatchMember
		var managed int
		if err := rows.Scan(&m.Position, &m.AgentID, &m.Host, &m.Action, &m.Target, &m.ExpectedState,
			&m.ExpectedVersion, &m.ExpectedPrevious, &managed, &m.State, &m.Code, &m.Reason, &m.Version); err != nil {
			return SoftwareBatch{}, err
		}
		m.ExpectedManaged = managed != 0
		b.Members = append(b.Members, m)
	}
	return b, rows.Err()
}

// ResumeSoftwareBatches resets members left running by a stopped controller
// and names every batch that still has pending work. A member left running was
// in flight; its outcome is unknown, so it is tried again and reconciled
// against the machine's fresh evidence. Stopped batches are not resumed: a
// person decides whether to retry, skip or abandon them.
func (s *Store) ResumeSoftwareBatches() ([]PendingSoftwareBatch, error) {
	err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE software_batch_members SET state='pending',updated_at=?
			WHERE state='running' AND batch_id IN (SELECT id FROM software_batches WHERE state IN ('pending','running'))`, now()); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE software_batches SET state='pending',updated_at=? WHERE state='running'`, now())
		return err
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT b.user_id,b.id FROM software_batches b
		WHERE b.state IN ('pending','running')
		AND EXISTS (SELECT 1 FROM software_batch_members m WHERE m.batch_id=b.id AND m.user_id=b.user_id AND m.state='pending')
		ORDER BY b.created_at,b.rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PendingSoftwareBatch{}
	for rows.Next() {
		var p PendingSoftwareBatch
		if err := rows.Scan(&p.UID, &p.BatchID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SoftwareBatchMemberStart claims a pending member for the driver and moves
// the batch to running. Of two callers only one gets the member.
func (s *Store) SoftwareBatchMemberStart(uid int64, batchID string, position int) (SoftwareBatchMember, error) {
	var m SoftwareBatchMember
	err := s.withTx(func(tx *sql.Tx) error {
		var state string
		err := tx.QueryRow(`SELECT state FROM software_batches WHERE id=? AND user_id=?`, batchID, uid).Scan(&state)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSoftwareBatchNotFound
		}
		if err != nil {
			return err
		}
		if state == SoftwareBatchDone || state == SoftwareBatchAbandoned {
			return ErrSoftwareBatchFinished
		}
		res, err := tx.Exec(`UPDATE software_batch_members SET state='running',updated_at=?
			WHERE batch_id=? AND user_id=? AND position=? AND state='pending'`, now(), batchID, uid, position)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrSoftwareBatchMemberNotPending
		}
		if _, err := tx.Exec(`UPDATE software_batches SET state='running',updated_at=? WHERE id=? AND user_id=?`, now(), batchID, uid); err != nil {
			return err
		}
		return loadSoftwareBatchMember(tx, uid, batchID, position, &m)
	})
	return m, err
}

func loadSoftwareBatchMember(q softwareBatchQuerier, uid int64, batchID string, position int, m *SoftwareBatchMember) error {
	var managed int
	err := q.QueryRow(`SELECT position,agent_id,host,action,target,expected_state,expected_version,expected_previous,expected_managed,state,code,reason,version
		FROM software_batch_members WHERE batch_id=? AND user_id=? AND position=?`, batchID, uid, position).
		Scan(&m.Position, &m.AgentID, &m.Host, &m.Action, &m.Target, &m.ExpectedState,
			&m.ExpectedVersion, &m.ExpectedPrevious, &managed, &m.State, &m.Code, &m.Reason, &m.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSoftwareBatchMemberNotFound
	}
	m.ExpectedManaged = managed != 0
	return err
}

// SetSoftwareBatchMemberState records a running member's outcome: applied
// (with the version now installed) or failed (with a code). The batch is then
// settled: still running while work remains, stopped at a failure, or done.
func (s *Store) SetSoftwareBatchMemberState(uid int64, batchID string, position int, state, code, version string) error {
	if state != SoftwareMemberApplied && state != SoftwareMemberFailed {
		return fmt.Errorf("%w: an outcome is applied or failed", ErrSoftwareBatchMemberNotRunning)
	}
	if state == SoftwareMemberFailed && !validCode(code) {
		code = "apply_failed"
	}
	if !shortPlain(version, 64) || !shortPlain(code, maxCodeBytes) {
		return fmt.Errorf("%w: an outcome carries plain text within its limits", ErrSoftwareBatchMemberNotRunning)
	}
	return s.withTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE software_batch_members SET state=?,code=?,version=?,updated_at=?
			WHERE batch_id=? AND user_id=? AND position=? AND state='running'`, state, code, version, now(), batchID, uid, position)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrSoftwareBatchMemberNotRunning
		}
		return settleSoftwareBatchTx(tx, uid, batchID)
	})
}

// RetrySoftwareBatchMember moves one failed member back to pending and returns
// the batch to running, so the driver tries it again from the frozen plan.
func (s *Store) RetrySoftwareBatchMember(uid int64, batchID string, position int) (SoftwareBatchMember, error) {
	var m SoftwareBatchMember
	err := s.withTx(func(tx *sql.Tx) error {
		state, err := softwareBatchState(tx, uid, batchID)
		if err != nil {
			return err
		}
		if state == SoftwareBatchDone || state == SoftwareBatchAbandoned {
			return ErrSoftwareBatchFinished
		}
		res, err := tx.Exec(`UPDATE software_batch_members SET state='pending',code='',version='',updated_at=?
			WHERE batch_id=? AND user_id=? AND position=? AND state='failed'`, now(), batchID, uid, position)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrSoftwareBatchMemberNotFailed
		}
		if _, err := tx.Exec(`UPDATE software_batches SET state='running',updated_at=? WHERE id=? AND user_id=?`, now(), batchID, uid); err != nil {
			return err
		}
		return loadSoftwareBatchMember(tx, uid, batchID, position, &m)
	})
	return m, err
}

// SkipSoftwareBatchMember marks one failed member skipped and continues the
// batch. With no pending work left the batch is done.
func (s *Store) SkipSoftwareBatchMember(uid int64, batchID string, position int) error {
	return s.withTx(func(tx *sql.Tx) error {
		state, err := softwareBatchState(tx, uid, batchID)
		if err != nil {
			return err
		}
		if state == SoftwareBatchDone || state == SoftwareBatchAbandoned {
			return ErrSoftwareBatchFinished
		}
		res, err := tx.Exec(`UPDATE software_batch_members SET state='skipped',updated_at=?
			WHERE batch_id=? AND user_id=? AND position=? AND state='failed'`, now(), batchID, uid, position)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrSoftwareBatchMemberNotFailed
		}
		if _, err := tx.Exec(`UPDATE software_batches SET state='running',updated_at=? WHERE id=? AND user_id=?`, now(), batchID, uid); err != nil {
			return err
		}
		return settleSoftwareBatchTx(tx, uid, batchID)
	})
}

// AbandonSoftwareBatch ends a batch between machines. Pending members are
// skipped; a machine still applying blocks the abandon, so nothing is cut off
// mid-install.
func (s *Store) AbandonSoftwareBatch(uid int64, batchID string) (SoftwareBatch, error) {
	var out SoftwareBatch
	err := s.withTx(func(tx *sql.Tx) error {
		state, err := softwareBatchState(tx, uid, batchID)
		if err != nil {
			return err
		}
		if state == SoftwareBatchDone || state == SoftwareBatchAbandoned {
			return ErrSoftwareBatchFinished
		}
		var running int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM software_batch_members WHERE batch_id=? AND user_id=? AND state='running'`, batchID, uid).Scan(&running); err != nil {
			return err
		}
		if running > 0 {
			return ErrSoftwareBatchRunning
		}
		if _, err := tx.Exec(`UPDATE software_batch_members SET state='skipped',code='abandoned',updated_at=?
			WHERE batch_id=? AND user_id=? AND state='pending'`, now(), batchID, uid); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE software_batches SET state='abandoned',updated_at=? WHERE id=? AND user_id=?`, now(), batchID, uid); err != nil {
			return err
		}
		out, err = loadSoftwareBatch(tx, uid, batchID)
		return err
	})
	return out, err
}

func softwareBatchState(tx *sql.Tx, uid int64, batchID string) (string, error) {
	var state string
	err := tx.QueryRow(`SELECT state FROM software_batches WHERE id=? AND user_id=?`, batchID, uid).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSoftwareBatchNotFound
	}
	return state, err
}

// settleSoftwareBatchTx derives the batch state from its members. An abandoned
// batch stays abandoned.
func settleSoftwareBatchTx(tx *sql.Tx, uid int64, batchID string) error {
	state, err := softwareBatchState(tx, uid, batchID)
	if err != nil || state == SoftwareBatchAbandoned {
		return err
	}
	var pending, running, failed int
	if err := tx.QueryRow(`SELECT
		COALESCE(SUM(state='pending'),0), COALESCE(SUM(state='running'),0), COALESCE(SUM(state='failed'),0)
		FROM software_batch_members WHERE batch_id=? AND user_id=?`, batchID, uid).Scan(&pending, &running, &failed); err != nil {
		return err
	}
	next := SoftwareBatchDone
	switch {
	case failed > 0:
		// The driver stops at the first failure, even while later machines wait.
		next = SoftwareBatchStopped
	case pending > 0 || running > 0:
		next = SoftwareBatchRunning
	}
	_, err = tx.Exec(`UPDATE software_batches SET state=?,updated_at=? WHERE id=? AND user_id=?`, next, now(), batchID, uid)
	return err
}
