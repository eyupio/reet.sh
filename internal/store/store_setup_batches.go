package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// A setup batch copies one machine's agent setup onto chosen machines, one at
// a time, with the same states as a software batch. The copied text is the
// one piece of setup content the controller ever holds: it lives only in
// setup_batch_content, is deleted when the batch settles (done or abandoned)
// and in any case 24 hours after the batch was made, and is never in a
// controller backup (VacuumInto empties the table in the copy).

// SetupBatchContentTTL is how long a batch may hold the copied text.
const SetupBatchContentTTL = 24 * 60 * 60 * 1000 // milliseconds

var (
	ErrSetupBatchNotFound         = errors.New("setup batch not found")
	ErrSetupBatchInProgress       = errors.New("another setup batch is still active")
	ErrSetupBatchNoMembers        = errors.New("a setup batch needs at least one machine that can apply")
	ErrSetupBatchMemberNotFound   = errors.New("setup batch member not found")
	ErrSetupBatchMemberNotPending = errors.New("setup batch member is not pending")
	ErrSetupBatchMemberNotRunning = errors.New("setup batch member is not running")
	ErrSetupBatchMemberNotFailed  = errors.New("only a failed setup batch member can be retried or skipped")
	ErrSetupBatchFinished         = errors.New("that setup batch has already finished")
	ErrSetupBatchRunning          = errors.New("a machine in that setup batch is still applying")
	ErrSetupBatchNotUndoable      = errors.New("that machine has no applied copy to undo")
	ErrSetupBatchContentGone      = errors.New("the setup batch no longer holds the copied files")
)

// SetupBatchFile is one file of a member's reviewed plan: names and fields,
// never text.
type SetupBatchFile struct {
	Agent  string   `json:"agent"`
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	State  string   `json:"state"`
	Reason string   `json:"reason,omitempty"`
	Fields []string `json:"fields,omitempty"`
	Widens []string `json:"widens,omitempty"`
}

// SetupBatchMember is one target machine. Fingerprint is the plan the person
// reviewed; the machine refuses an apply whose plan no longer matches it.
type SetupBatchMember struct {
	Position    int              `json:"position"`
	AgentID     string           `json:"agentId"`
	Host        string           `json:"host"`
	Fingerprint string           `json:"fingerprint,omitempty"`
	Widens      bool             `json:"widens"`
	Files       []SetupBatchFile `json:"files"`
	State       string           `json:"state"`
	Code        string           `json:"code,omitempty"`
	Reason      string           `json:"reason,omitempty"`
	Journal     string           `json:"journal,omitempty"`
	UndoneAt    int64            `json:"undoneAt,omitempty"`
}

// SetupBatch is one reviewed copy. SourceHost and ReadAt name the machine the
// setup was read from and when.
type SetupBatch struct {
	ID            string `json:"id"`
	SourceAgentID string `json:"sourceAgentId"`
	SourceHost    string `json:"sourceHost"`
	ReadAt        int64  `json:"readAt"`
	State         string `json:"state"`
	Widens        bool   `json:"widens"`
	// ContentHeld says whether the controller still holds the copied text.
	ContentHeld bool               `json:"contentHeld"`
	CreatedAt   int64              `json:"createdAt"`
	UpdatedAt   int64              `json:"updatedAt"`
	Members     []SetupBatchMember `json:"members"`
}

// SetupBatchSource names where a batch's setup came from.
type SetupBatchSource struct {
	AgentID, Host string
	ReadAt        int64
}

// SetupBatchMemberInput is one machine as the caller reviewed it.
type SetupBatchMemberInput struct {
	Position    int
	AgentID     string
	Host        string
	Fingerprint string
	Widens      bool
	Files       []SetupBatchFile
	Excluded    bool
	Code        string
	Reason      string
}

func (s *Store) migrateSetupBatches() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS setup_batches (
			id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			source_agent_id TEXT NOT NULL, source_host TEXT NOT NULL, read_at INTEGER NOT NULL,
			state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','running','stopped','done','abandoned')),
			request_id TEXT NOT NULL DEFAULT '', widens INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
			UNIQUE(id,user_id))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS setup_batches_request ON setup_batches(user_id,request_id) WHERE request_id<>''`,
		`CREATE INDEX IF NOT EXISTS setup_batches_active ON setup_batches(user_id,state)`,
		// The only place the controller keeps setup text. Backups empty it.
		`CREATE TABLE IF NOT EXISTS setup_batch_content (
			batch_id TEXT PRIMARY KEY, user_id INTEGER NOT NULL,
			items TEXT NOT NULL, created_at INTEGER NOT NULL,
			FOREIGN KEY(batch_id,user_id) REFERENCES setup_batches(id,user_id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS setup_batch_members (
			batch_id TEXT NOT NULL, user_id INTEGER NOT NULL,
			position INTEGER NOT NULL CHECK(position>=0),
			agent_id TEXT NOT NULL, host TEXT NOT NULL,
			fingerprint TEXT NOT NULL DEFAULT '', widens INTEGER NOT NULL DEFAULT 0, files TEXT NOT NULL DEFAULT '[]',
			state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','running','applied','failed','skipped','excluded')),
			code TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '',
			journal TEXT NOT NULL DEFAULT '', undone_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(batch_id,position),
			FOREIGN KEY(batch_id,user_id) REFERENCES setup_batches(id,user_id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS setup_batch_members_agent ON setup_batch_members(user_id,agent_id,state)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// CreateSetupBatchFor stores an ordered batch and the copied items. It
// refuses while another setup batch is active for uid; a repeated requestID
// returns the batch it already made, even after its content is gone.
func (s *Store) CreateSetupBatchFor(uid int64, requestID string, source SetupBatchSource, items []byte, members []SetupBatchMemberInput) (SetupBatch, bool, error) {
	if len(members) == 0 {
		return SetupBatch{}, false, ErrSetupBatchNoMembers
	}
	var out SetupBatch
	created := true
	err := s.withTx(func(tx *sql.Tx) error {
		if requestID != "" {
			var existing string
			err := tx.QueryRow(`SELECT id FROM setup_batches WHERE user_id=? AND request_id=?`, uid, requestID).Scan(&existing)
			if err == nil {
				out, err = loadSetupBatch(tx, uid, existing)
				created = false
				return err
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		var active int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM setup_batches WHERE user_id=? AND state IN ('pending','running','stopped')`, uid).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return ErrSetupBatchInProgress
		}
		id, err := projectRandomID()
		if err != nil {
			return err
		}
		t := now()
		widens := false
		included := 0
		positions, agents := map[int]bool{}, map[string]bool{}
		for _, m := range members {
			if m.Position < 0 || positions[m.Position] || agents[m.AgentID] {
				return fmt.Errorf("%w: duplicate position or machine", ErrSetupBatchNoMembers)
			}
			positions[m.Position], agents[m.AgentID] = true, true
			if !m.Excluded {
				included++
				widens = widens || m.Widens
			}
		}
		if included == 0 {
			return ErrSetupBatchNoMembers
		}
		if _, err := tx.Exec(`INSERT INTO setup_batches(id,user_id,source_agent_id,source_host,read_at,state,request_id,widens,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			id, uid, source.AgentID, source.Host, source.ReadAt, SoftwareBatchPending, requestID, boolInt(widens), t, t); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO setup_batch_content(batch_id,user_id,items,created_at) VALUES(?,?,?,?)`, id, uid, string(items), t); err != nil {
			return err
		}
		for _, m := range members {
			state, code, reason := SoftwareMemberPending, m.Code, m.Reason
			if m.Excluded {
				state = SoftwareMemberExcluded
				if !shortPlain(reason, 240) || !shortPlain(code, maxCodeBytes) {
					return fmt.Errorf("%w: an exclusion reason is too long", ErrSetupBatchNoMembers)
				}
			}
			files, err := json.Marshal(nonNilFiles(m.Files))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO setup_batch_members(batch_id,user_id,position,agent_id,host,fingerprint,widens,files,state,code,reason,updated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
				id, uid, m.Position, m.AgentID, m.Host, m.Fingerprint, boolInt(m.Widens), string(files), state, code, reason, t); err != nil {
				return err
			}
		}
		out, err = loadSetupBatch(tx, uid, id)
		return err
	})
	return out, created, err
}

func nonNilFiles(files []SetupBatchFile) []SetupBatchFile {
	if files == nil {
		return []SetupBatchFile{}
	}
	return files
}

// SetupBatchFor reads one batch and its members in position order.
func (s *Store) SetupBatchFor(uid int64, batchID string) (SetupBatch, error) {
	return loadSetupBatch(s.db, uid, batchID)
}

// ActiveSetupBatchFor reads the person's pending, running or stopped setup
// batch, or false when there is none.
func (s *Store) ActiveSetupBatchFor(uid int64) (SetupBatch, bool, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM setup_batches WHERE user_id=? AND state IN ('pending','running','stopped') ORDER BY created_at DESC LIMIT 1`, uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return SetupBatch{}, false, nil
	}
	if err != nil {
		return SetupBatch{}, false, err
	}
	batch, err := loadSetupBatch(s.db, uid, id)
	return batch, err == nil, err
}

// SetupBatchItems returns the copied items a batch holds, or
// ErrSetupBatchContentGone once they were deleted.
func (s *Store) SetupBatchItems(uid int64, batchID string) ([]byte, error) {
	var items string
	err := s.db.QueryRow(`SELECT items FROM setup_batch_content WHERE batch_id=? AND user_id=?`, batchID, uid).Scan(&items)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSetupBatchContentGone
	}
	return []byte(items), err
}

const setupMemberCols = `position,agent_id,host,fingerprint,widens,files,state,code,reason,journal,undone_at`

func scanSetupMember(scan func(...any) error, m *SetupBatchMember) error {
	var widens int
	var files string
	if err := scan(&m.Position, &m.AgentID, &m.Host, &m.Fingerprint, &widens, &files, &m.State, &m.Code, &m.Reason, &m.Journal, &m.UndoneAt); err != nil {
		return err
	}
	m.Widens = widens != 0
	m.Files = []SetupBatchFile{}
	return json.Unmarshal([]byte(files), &m.Files)
}

func loadSetupBatch(q softwareBatchQuerier, uid int64, batchID string) (SetupBatch, error) {
	var b SetupBatch
	var widens, held int
	err := q.QueryRow(`SELECT id,source_agent_id,source_host,read_at,state,widens,created_at,updated_at,
		EXISTS(SELECT 1 FROM setup_batch_content c WHERE c.batch_id=setup_batches.id AND c.user_id=setup_batches.user_id)
		FROM setup_batches WHERE id=? AND user_id=?`, batchID, uid).
		Scan(&b.ID, &b.SourceAgentID, &b.SourceHost, &b.ReadAt, &b.State, &widens, &b.CreatedAt, &b.UpdatedAt, &held)
	if errors.Is(err, sql.ErrNoRows) {
		return SetupBatch{}, ErrSetupBatchNotFound
	}
	if err != nil {
		return SetupBatch{}, err
	}
	b.Widens, b.ContentHeld = widens != 0, held != 0
	rows, err := q.Query(`SELECT `+setupMemberCols+` FROM setup_batch_members WHERE batch_id=? AND user_id=? ORDER BY position`, batchID, uid)
	if err != nil {
		return SetupBatch{}, err
	}
	defer rows.Close()
	b.Members = []SetupBatchMember{}
	for rows.Next() {
		var m SetupBatchMember
		if err := scanSetupMember(rows.Scan, &m); err != nil {
			return SetupBatch{}, err
		}
		b.Members = append(b.Members, m)
	}
	return b, rows.Err()
}

func loadSetupBatchMember(q softwareBatchQuerier, uid int64, batchID string, position int) (SetupBatchMember, error) {
	var m SetupBatchMember
	err := scanSetupMember(q.QueryRow(`SELECT `+setupMemberCols+` FROM setup_batch_members WHERE batch_id=? AND user_id=? AND position=?`, batchID, uid, position).Scan, &m)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrSetupBatchMemberNotFound
	}
	return m, err
}

// ResumeSetupBatches resets members left running by a stopped controller and
// names every batch with pending work. A member left running is tried again;
// the machine finds its own completed journal if the first apply went through.
func (s *Store) ResumeSetupBatches() ([]PendingSoftwareBatch, error) {
	err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE setup_batch_members SET state='pending',updated_at=?
			WHERE state='running' AND batch_id IN (SELECT id FROM setup_batches WHERE state IN ('pending','running'))`, now()); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE setup_batches SET state='pending',updated_at=? WHERE state='running'`, now())
		return err
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT b.user_id,b.id FROM setup_batches b
		WHERE b.state IN ('pending','running')
		AND EXISTS (SELECT 1 FROM setup_batch_members m WHERE m.batch_id=b.id AND m.user_id=b.user_id AND m.state='pending')
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

func setupBatchState(tx *sql.Tx, uid int64, batchID string) (string, error) {
	var state string
	err := tx.QueryRow(`SELECT state FROM setup_batches WHERE id=? AND user_id=?`, batchID, uid).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSetupBatchNotFound
	}
	return state, err
}

// moveSetupMember changes one member from one state to another inside tx and
// fails with notFrom when it was not in from.
func moveSetupMember(tx *sql.Tx, uid int64, batchID string, position int, from, to, code string, notFrom error) error {
	res, err := tx.Exec(`UPDATE setup_batch_members SET state=?,code=?,updated_at=? WHERE batch_id=? AND user_id=? AND position=? AND state=?`,
		to, code, now(), batchID, uid, position, from)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		if err != nil {
			return err
		}
		return notFrom
	}
	return nil
}

// SetupBatchMemberStart claims a pending member and moves the batch to running.
func (s *Store) SetupBatchMemberStart(uid int64, batchID string, position int) (SetupBatchMember, error) {
	var m SetupBatchMember
	err := s.withTx(func(tx *sql.Tx) error {
		state, err := setupBatchState(tx, uid, batchID)
		if err != nil {
			return err
		}
		if state == SoftwareBatchDone || state == SoftwareBatchAbandoned {
			return ErrSetupBatchFinished
		}
		if err := moveSetupMember(tx, uid, batchID, position, SoftwareMemberPending, SoftwareMemberRunning, "", ErrSetupBatchMemberNotPending); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE setup_batches SET state='running',updated_at=? WHERE id=? AND user_id=?`, now(), batchID, uid); err != nil {
			return err
		}
		m, err = loadSetupBatchMember(tx, uid, batchID, position)
		return err
	})
	return m, err
}

// SetSetupBatchMemberState records a running member's outcome: applied with
// the machine's journal id, failed with a code, or skipped with a code for a
// machine the apply never reached. The batch is then settled.
func (s *Store) SetSetupBatchMemberState(uid int64, batchID string, position int, state, code, journal string) error {
	if state != SoftwareMemberApplied && state != SoftwareMemberFailed && state != SoftwareMemberSkipped {
		return fmt.Errorf("%w: an outcome is applied, failed or skipped", ErrSetupBatchMemberNotRunning)
	}
	if state != SoftwareMemberApplied && !validCode(code) {
		code = "apply_failed"
	}
	if !shortPlain(journal, 200) || !shortPlain(code, maxCodeBytes) {
		return fmt.Errorf("%w: an outcome carries plain text within its limits", ErrSetupBatchMemberNotRunning)
	}
	return s.withTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE setup_batch_members SET state=?,code=?,journal=?,updated_at=?
			WHERE batch_id=? AND user_id=? AND position=? AND state='running'`, state, code, journal, now(), batchID, uid, position)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			if err != nil {
				return err
			}
			return ErrSetupBatchMemberNotRunning
		}
		return settleSetupBatchTx(tx, uid, batchID)
	})
}

// RetrySetupBatchMember moves one failed member back to pending.
func (s *Store) RetrySetupBatchMember(uid int64, batchID string, position int) (SetupBatchMember, error) {
	var m SetupBatchMember
	err := s.withTx(func(tx *sql.Tx) error {
		state, err := setupBatchState(tx, uid, batchID)
		if err != nil {
			return err
		}
		if state == SoftwareBatchDone || state == SoftwareBatchAbandoned {
			return ErrSetupBatchFinished
		}
		if err := moveSetupMember(tx, uid, batchID, position, SoftwareMemberFailed, SoftwareMemberPending, "", ErrSetupBatchMemberNotFailed); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE setup_batches SET state='running',updated_at=? WHERE id=? AND user_id=?`, now(), batchID, uid); err != nil {
			return err
		}
		m, err = loadSetupBatchMember(tx, uid, batchID, position)
		return err
	})
	return m, err
}

// SkipSetupBatchMember marks one failed member skipped and continues.
func (s *Store) SkipSetupBatchMember(uid int64, batchID string, position int) error {
	return s.withTx(func(tx *sql.Tx) error {
		state, err := setupBatchState(tx, uid, batchID)
		if err != nil {
			return err
		}
		if state == SoftwareBatchDone || state == SoftwareBatchAbandoned {
			return ErrSetupBatchFinished
		}
		var code string
		if err := tx.QueryRow(`SELECT code FROM setup_batch_members WHERE batch_id=? AND user_id=? AND position=?`, batchID, uid, position).Scan(&code); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := moveSetupMember(tx, uid, batchID, position, SoftwareMemberFailed, SoftwareMemberSkipped, code, ErrSetupBatchMemberNotFailed); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE setup_batches SET state='running',updated_at=? WHERE id=? AND user_id=?`, now(), batchID, uid); err != nil {
			return err
		}
		return settleSetupBatchTx(tx, uid, batchID)
	})
}

// AbandonSetupBatch ends a batch between machines and deletes its content.
// A machine still applying blocks it.
func (s *Store) AbandonSetupBatch(uid int64, batchID string) (SetupBatch, error) {
	var out SetupBatch
	err := s.withTx(func(tx *sql.Tx) error {
		state, err := setupBatchState(tx, uid, batchID)
		if err != nil {
			return err
		}
		if state == SoftwareBatchDone || state == SoftwareBatchAbandoned {
			return ErrSetupBatchFinished
		}
		var running int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM setup_batch_members WHERE batch_id=? AND user_id=? AND state='running'`, batchID, uid).Scan(&running); err != nil {
			return err
		}
		if running > 0 {
			return ErrSetupBatchRunning
		}
		if err := endSetupBatchTx(tx, uid, batchID, "abandoned"); err != nil {
			return err
		}
		out, err = loadSetupBatch(tx, uid, batchID)
		return err
	})
	return out, err
}

// endSetupBatchTx abandons a batch: waiting and failed members are skipped
// with code, and the copied text is deleted.
func endSetupBatchTx(tx *sql.Tx, uid int64, batchID, code string) error {
	if _, err := tx.Exec(`UPDATE setup_batch_members SET state='skipped',code=?,updated_at=?
		WHERE batch_id=? AND user_id=? AND state IN ('pending','failed')`, code, now(), batchID, uid); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE setup_batches SET state='abandoned',updated_at=? WHERE id=? AND user_id=?`, now(), batchID, uid); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM setup_batch_content WHERE batch_id=? AND user_id=?`, batchID, uid)
	return err
}

// settleSetupBatchTx derives the batch state from its members, as a software
// batch does, and deletes the copied text once the batch is done.
func settleSetupBatchTx(tx *sql.Tx, uid int64, batchID string) error {
	state, err := setupBatchState(tx, uid, batchID)
	if err != nil || state == SoftwareBatchAbandoned {
		return err
	}
	var pending, running, failed int
	if err := tx.QueryRow(`SELECT
		COALESCE(SUM(state='pending'),0), COALESCE(SUM(state='running'),0), COALESCE(SUM(state='failed'),0)
		FROM setup_batch_members WHERE batch_id=? AND user_id=?`, batchID, uid).Scan(&pending, &running, &failed); err != nil {
		return err
	}
	next := SoftwareBatchDone
	switch {
	case failed > 0:
		next = SoftwareBatchStopped
	case pending > 0 || running > 0:
		next = SoftwareBatchRunning
	}
	if _, err := tx.Exec(`UPDATE setup_batches SET state=?,updated_at=? WHERE id=? AND user_id=?`, next, now(), batchID, uid); err != nil {
		return err
	}
	if next == SoftwareBatchDone {
		_, err = tx.Exec(`DELETE FROM setup_batch_content WHERE batch_id=? AND user_id=?`, batchID, uid)
	}
	return err
}

// SetSetupBatchMemberUndone records that an applied member's copy was undone.
// It is allowed once, whatever state the batch is in.
func (s *Store) SetSetupBatchMemberUndone(uid int64, batchID string, position int) error {
	res, err := s.db.Exec(`UPDATE setup_batch_members SET undone_at=?,updated_at=?
		WHERE batch_id=? AND user_id=? AND position=? AND state='applied' AND journal<>'' AND undone_at=0`, now(), now(), batchID, uid, position)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrSetupBatchNotUndoable
	}
	return nil
}

// PurgeSetupBatchContent deletes copied text that must not be kept any
// longer: that of settled batches, and that of any batch made more than
// SetupBatchContentTTL ago. Such a batch can no longer apply, so it is
// abandoned, unless a machine is applying right now; that member records its
// outcome and the batch settles without the text. It returns how many
// batches lost their content.
func (s *Store) PurgeSetupBatchContent(at int64) (int, error) {
	var purged int
	err := s.withTx(func(tx *sql.Tx) error {
		cutoff := at - SetupBatchContentTTL
		rows, err := tx.Query(`SELECT b.user_id,b.id,b.state,EXISTS(SELECT 1 FROM setup_batch_members m WHERE m.batch_id=b.id AND m.user_id=b.user_id AND m.state='running')
			FROM setup_batches b JOIN setup_batch_content c ON c.batch_id=b.id AND c.user_id=b.user_id
			WHERE b.state IN ('done','abandoned') OR c.created_at<=?`, cutoff)
		if err != nil {
			return err
		}
		type expired struct {
			uid     int64
			id      string
			state   string
			running bool
		}
		var list []expired
		for rows.Next() {
			var e expired
			if err := rows.Scan(&e.uid, &e.id, &e.state, &e.running); err != nil {
				rows.Close()
				return err
			}
			list = append(list, e)
		}
		rows.Close()
		for _, e := range list {
			active := e.state != SoftwareBatchDone && e.state != SoftwareBatchAbandoned
			if active && !e.running {
				if err := endSetupBatchTx(tx, e.uid, e.id, "expired"); err != nil {
					return err
				}
			} else {
				if active {
					if _, err := tx.Exec(`UPDATE setup_batch_members SET state='skipped',code='expired',updated_at=? WHERE batch_id=? AND user_id=? AND state IN ('pending','failed')`, now(), e.id, e.uid); err != nil {
						return err
					}
				}
				if _, err := tx.Exec(`DELETE FROM setup_batch_content WHERE batch_id=? AND user_id=?`, e.id, e.uid); err != nil {
					return err
				}
			}
			purged++
		}
		return nil
	})
	return purged, err
}

// scrubSetupContent empties the setup copy table in a database file, so a
// backup never carries setup text. secure_delete overwrites the freed pages.
func scrubSetupContent(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(3000)")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='setup_batch_content'`).Scan(&n); err != nil || n == 0 {
		return err
	}
	for _, stmt := range []string{`PRAGMA secure_delete=ON`, `DELETE FROM setup_batch_content`, `VACUUM`} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// SetupBatchForRequest finds the batch a create request id already made.
func (s *Store) SetupBatchForRequest(uid int64, requestID string) (SetupBatch, bool, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM setup_batches WHERE user_id=? AND request_id=? AND request_id<>''`, uid, requestID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return SetupBatch{}, false, nil
	}
	if err != nil {
		return SetupBatch{}, false, err
	}
	batch, err := loadSetupBatch(s.db, uid, id)
	return batch, err == nil, err
}
