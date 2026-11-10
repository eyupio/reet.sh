package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// An outcome is an objective the person handed Gaffer with a plan they
// approved. The controller starts each planned session and puts it in
// Gaffer's care; the record is what lets that finish without a browser and
// what ties the sessions together afterwards.
const (
	MaxGafferOutcomes        = 30
	MaxGafferOutcomeSessions = 5
)

var (
	ErrGafferOutcomeLimit = errors.New("Gaffer is still starting three outcomes for this account. Wait for one to settle, or stop it.")
	ErrGafferOutcomeBusy  = errors.New("That outcome changed. Reload it before trying again.")
)

// Outcome states: "starting" while the controller still has sessions to
// start, "settled" once every session has an answer, "stopped" when the
// person or a restore ended it early.
// Session states: "waiting", "starting", "following", "unfollowed" (started,
// no goal), "failed" (not started) and "cancelled" (never attempted). A
// session with stages is a pipeline and adds "advancing" (the controller is
// to start its next stage), "ready_to_merge" (waiting for the person) and
// "done" (its last stage was handed over).
//
// A stage is one step of a pipeline, done by the agent and model the person
// approved for it. Stage states: "pending", "running", "done".
type GafferOutcomeStage struct {
	Kind           string `json:"kind"`
	Tool           string `json:"tool"`
	Model          string `json:"model,omitempty"`
	State          string `json:"state"`
	RequestID      string `json:"requestId"`
	SessionID      string `json:"sessionId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	Summary        string `json:"summary,omitempty"`
	StartedAt      int64  `json:"startedAt,omitempty"`
	EndedAt        int64  `json:"endedAt,omitempty"`
}

type GafferOutcomeSession struct {
	Key            string `json:"key"`
	Title          string `json:"title"`
	Prompt         string `json:"prompt"`
	RequestID      string `json:"requestId"`
	State          string `json:"state"`
	Note           string `json:"note"`
	SessionID      string `json:"sessionId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	UpdatedAt      int64  `json:"updatedAt"`
	// Stages is empty for a plain followed session. Stage is the index of the
	// stage in hand. WorkDir is where the first stage's session works, which
	// later stages share. AutoMerge and MergeApproved are the person's own
	// choices: without one of them the merge stage never starts.
	Stages        []GafferOutcomeStage `json:"stages,omitempty"`
	Stage         int                  `json:"stage"`
	WorkDir       string               `json:"workDir,omitempty"`
	AutoMerge     bool                 `json:"autoMerge,omitempty"`
	MergeApproved bool                 `json:"mergeApproved,omitempty"`
}

type GafferOutcome struct {
	OwnerID     int64                  `json:"-"`
	ID          string                 `json:"id"`
	Revision    int64                  `json:"revision"`
	Objective   string                 `json:"objective"`
	State       string                 `json:"state"`
	Note        string                 `json:"note"`
	AgentID     string                 `json:"agentId"`
	ProjectID   string                 `json:"projectId"`
	ProjectName string                 `json:"projectName"`
	Dir         string                 `json:"dir"`
	Tool        string                 `json:"tool"`
	Model       string                 `json:"model,omitempty"`
	Trust       bool                   `json:"trust,omitempty"`
	ProviderID  string                 `json:"providerId"`
	MaxReviews  int                    `json:"maxReviews"`
	Hours       int                    `json:"hours"`
	Sessions    []GafferOutcomeSession `json:"sessions"`
	CreatedAt   int64                  `json:"createdAt"`
	UpdatedAt   int64                  `json:"updatedAt"`
}

func (s *Store) migrateGafferOutcomes() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS gaffer_outcomes(
 id TEXT PRIMARY KEY,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 created_at INTEGER NOT NULL,data TEXT NOT NULL CHECK(json_valid(data)));
 CREATE INDEX IF NOT EXISTS gaffer_outcomes_owner ON gaffer_outcomes(user_id,created_at);
 CREATE INDEX IF NOT EXISTS gaffer_outcomes_state ON gaffer_outcomes(json_extract(data,'$.state'));`)
	return err
}

func scanOutcome(uid int64, raw string) (GafferOutcome, error) {
	var o GafferOutcome
	err := json.Unmarshal([]byte(raw), &o)
	o.OwnerID = uid
	if o.Sessions == nil {
		o.Sessions = []GafferOutcomeSession{}
	}
	return o, err
}

// CreateGafferOutcomeFor saves an approved plan. At most three outcomes per
// account may be starting at once, and the oldest finished ones are dropped
// past the retention limit; a starting outcome is never dropped.
func (s *Store) CreateGafferOutcomeFor(uid int64, o GafferOutcome) (GafferOutcome, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return o, err
	}
	defer tx.Rollback()
	var starting int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM gaffer_outcomes WHERE user_id=? AND json_extract(data,'$.state')='starting'`, uid).Scan(&starting); err != nil {
		return o, err
	}
	if starting >= 3 {
		return o, ErrGafferOutcomeLimit
	}
	o.OwnerID, o.Revision, o.State, o.CreatedAt, o.UpdatedAt = uid, 1, "starting", now(), now()
	for i := range o.Sessions {
		o.Sessions[i].State, o.Sessions[i].UpdatedAt = "waiting", o.CreatedAt
		for j := range o.Sessions[i].Stages {
			o.Sessions[i].Stages[j].State = "pending"
		}
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return o, err
	}
	if _, err = tx.Exec(`INSERT INTO gaffer_outcomes(id,user_id,created_at,data) VALUES(?,?,?,?)`, o.ID, uid, o.CreatedAt, string(raw)); err != nil {
		return o, err
	}
	if _, err = tx.Exec(`DELETE FROM gaffer_outcomes WHERE user_id=? AND json_extract(data,'$.state')!='starting' AND id NOT IN(SELECT id FROM gaffer_outcomes WHERE user_id=? ORDER BY created_at DESC,id DESC LIMIT ?)`, uid, uid, MaxGafferOutcomes); err != nil {
		return o, err
	}
	return o, tx.Commit()
}

func (s *Store) GafferOutcomeFor(uid int64, id string) (GafferOutcome, error) {
	var raw string
	err := s.db.QueryRow(`SELECT data FROM gaffer_outcomes WHERE user_id=? AND id=?`, uid, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return GafferOutcome{}, ErrNotFound
	}
	if err != nil {
		return GafferOutcome{}, err
	}
	return scanOutcome(uid, raw)
}

func (s *Store) GafferOutcomesFor(uid int64) ([]GafferOutcome, error) {
	rows, err := s.db.Query(`SELECT data FROM gaffer_outcomes WHERE user_id=? ORDER BY created_at DESC,id DESC LIMIT ?`, uid, MaxGafferOutcomes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GafferOutcome{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		o, err := scanOutcome(uid, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// StartingGafferOutcomes lists every account's outcomes the controller still
// has work for, oldest first.
func (s *Store) StartingGafferOutcomes() ([]GafferOutcome, error) {
	rows, err := s.db.Query(`SELECT user_id,data FROM gaffer_outcomes WHERE json_extract(data,'$.state')='starting' ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GafferOutcome
	for rows.Next() {
		var uid int64
		var raw string
		if err = rows.Scan(&uid, &raw); err != nil {
			return nil, err
		}
		o, err := scanOutcome(uid, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// UpdateGafferOutcomeFor applies change to the saved outcome when it is still
// at revision. change reports false to leave it as it is. The person's stop
// and the controller's progress both go through here, so neither overwrites
// the other.
func (s *Store) UpdateGafferOutcomeFor(uid int64, id string, revision int64, change func(*GafferOutcome) bool) (GafferOutcome, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return GafferOutcome{}, err
	}
	defer tx.Rollback()
	var raw string
	err = tx.QueryRow(`SELECT data FROM gaffer_outcomes WHERE user_id=? AND id=?`, uid, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return GafferOutcome{}, ErrNotFound
	}
	if err != nil {
		return GafferOutcome{}, err
	}
	o, err := scanOutcome(uid, raw)
	if err != nil {
		return o, err
	}
	if o.Revision != revision {
		return o, ErrGafferOutcomeBusy
	}
	if !change(&o) {
		return o, nil
	}
	o.Revision++
	o.UpdatedAt = now()
	next, err := json.Marshal(o)
	if err != nil {
		return o, err
	}
	if _, err = tx.Exec(`UPDATE gaffer_outcomes SET data=? WHERE user_id=? AND id=?`, string(next), uid, id); err != nil {
		return o, err
	}
	return o, tx.Commit()
}
