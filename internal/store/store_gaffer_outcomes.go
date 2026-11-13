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
	// MaxGafferOutcomePipelines bounds an outcome after pipelines are added to it.
	MaxGafferOutcomePipelines = 10
)

var (
	ErrGafferOutcomeLimit  = errors.New("Gaffer is still starting three outcomes for this account. Wait for one to settle, or stop it.")
	ErrGafferOutcomeBusy   = errors.New("That outcome changed. Reload it before trying again.")
	ErrGafferOutcomeActive = errors.New("That outcome is still moving. Stop following it or let it finish first.")
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
	Kind     string `json:"kind"`
	Tool     string `json:"tool"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	Provider string `json:"provider,omitempty"`
	// What the stage's working agent reported using, from its own figures: kept
	// when the stage is handed over, and read live for the stage in hand.
	// ContextUsed and ContextSize are the agent's context window, live only.
	AgentInputTokens   int64 `json:"agentInputTokens,omitempty"`
	AgentOutputTokens  int64 `json:"agentOutputTokens,omitempty"`
	AgentCachedTokens  int64 `json:"agentCachedTokens,omitempty"`
	AgentUsageReported bool  `json:"agentUsageReported,omitempty"`
	ContextUsed        int64 `json:"contextUsed,omitempty"`
	ContextSize        int64 `json:"contextSize,omitempty"`
	// AgentState and AgentActiveAt are what the stage in hand's session last
	// reported doing and when it was last active. Live only, never saved.
	AgentState    string `json:"agentState,omitempty"`
	AgentActiveAt int64  `json:"agentActiveAt,omitempty"`
	// SentBack is why the person sent the pipeline back to this stage, told
	// to the agent that does it again.
	SentBack       string `json:"sentBack,omitempty"`
	State          string `json:"state"`
	RequestID      string `json:"requestId"`
	SessionID      string `json:"sessionId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	Summary        string `json:"summary,omitempty"`
	// ClosedBy is who ended a done stage: "gaffer" (its review handed the
	// stage over), "person" (they moved it on) or "chat" (they told Gaffer to).
	ClosedBy  string `json:"closedBy,omitempty"`
	StartedAt int64  `json:"startedAt,omitempty"`
	EndedAt   int64  `json:"endedAt,omitempty"`
	// What Gaffer's own model used following this stage, recorded when the
	// stage was handed over. It is Gaffer's usage, not the working agent's.
	Reviews       int  `json:"reviews,omitempty"`
	InputTokens   int  `json:"inputTokens,omitempty"`
	OutputTokens  int  `json:"outputTokens,omitempty"`
	UsageReported bool `json:"usageReported,omitempty"`
}

// GafferOutcomeReport is Gaffer's account of the whole outcome, made when
// the person asks for one. Proposals are pipelines it suggests adding; none
// exists until the person approves it.
type GafferOutcomeReport struct {
	Text          string                  `json:"text"`
	At            int64                   `json:"at"`
	Model         string                  `json:"model"`
	InputTokens   int                     `json:"inputTokens"`
	OutputTokens  int                     `json:"outputTokens"`
	UsageReported bool                    `json:"usageReported"`
	Proposals     []GafferOutcomeProposal `json:"proposals,omitempty"`
	// Auto marks a report Gaffer made by itself, when a pipeline finished or stopped.
	Auto bool `json:"auto,omitempty"`
}

type GafferOutcomeProposal struct {
	Title  string   `json:"title"`
	Prompt string   `json:"prompt"`
	Stages []string `json:"stages,omitempty"`
	Why    string   `json:"why,omitempty"`
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
	Stages    []GafferOutcomeStage `json:"stages,omitempty"`
	Stage     int                  `json:"stage"`
	WorkDir   string               `json:"workDir,omitempty"`
	AutoMerge bool                 `json:"autoMerge,omitempty"`
	// YoloMergeOK is the person's separate confirmation that a pipeline whose
	// agent runs on YOLO may also merge by itself. Without it such a pipeline
	// stops at Ready to merge whatever AutoMerge says.
	YoloMergeOK   bool `json:"yoloMergeOk,omitempty"`
	MergeApproved bool `json:"mergeApproved,omitempty"`
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
	Provider    string                 `json:"provider,omitempty"`
	Trust       bool                   `json:"trust,omitempty"`
	Yolo        bool                   `json:"yolo,omitempty"`
	ProviderID  string                 `json:"providerId"`
	MaxReviews  int                    `json:"maxReviews"`
	Hours       int                    `json:"hours"`
	Sessions    []GafferOutcomeSession `json:"sessions"`
	Report      *GafferOutcomeReport   `json:"report,omitempty"`
	// AutoReport is the person's choice, made when they approved the plan,
	// that Gaffer reports across the outcome by itself when a pipeline
	// finishes or stops. AutoReportedFor is what it last reported on, and
	// AutoReports how many it has made, so one event is one report.
	AutoReport      bool   `json:"autoReport,omitempty"`
	AutoReportedFor string `json:"autoReportedFor,omitempty"`
	AutoReports     int    `json:"autoReports,omitempty"`
	// ArchivedAt is when the person put this finished outcome away; zero means
	// it is on the Factory floor. The record and its report are kept.
	ArchivedAt int64 `json:"archivedAt,omitempty"`
	CreatedAt  int64 `json:"createdAt"`
	UpdatedAt  int64 `json:"updatedAt"`
}

// Finished reports whether nothing in the outcome is still moving: it is not
// starting sessions and no pipeline is following, waiting, advancing or
// ready to merge. Only a finished outcome may be archived or deleted.
func (o GafferOutcome) Finished() bool {
	if o.State == "starting" {
		return false
	}
	for _, item := range o.Sessions {
		switch item.State {
		case "following", "starting", "advancing", "waiting", "ready_to_merge":
			return false
		}
	}
	return true
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

// DeleteGafferOutcomeFor removes one outcome record when it is still at
// revision and finished. Its sessions and pull requests are not touched.
func (s *Store) DeleteGafferOutcomeFor(uid int64, id string, revision int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	err = tx.QueryRow(`SELECT data FROM gaffer_outcomes WHERE user_id=? AND id=?`, uid, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	o, err := scanOutcome(uid, raw)
	if err != nil {
		return err
	}
	if o.Revision != revision {
		return ErrGafferOutcomeBusy
	}
	if !o.Finished() {
		return ErrGafferOutcomeActive
	}
	if _, err = tx.Exec(`DELETE FROM gaffer_outcomes WHERE user_id=? AND id=?`, uid, id); err != nil {
		return err
	}
	return tx.Commit()
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

// FollowedGafferOutcomes lists every account's outcomes that have a pipeline
// following a session, oldest first. The controller checks each such session
// still exists.
func (s *Store) FollowedGafferOutcomes() ([]GafferOutcome, error) {
	rows, err := s.db.Query(`SELECT user_id,data FROM gaffer_outcomes WHERE EXISTS(SELECT 1 FROM json_each(gaffer_outcomes.data,'$.sessions') WHERE json_extract(value,'$.state')='following' AND json_extract(value,'$.sessionId')<>'') ORDER BY created_at,id`)
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

// AutoReportGafferOutcomes lists the outcomes whose owner asked for reports
// without being asked, and that were not stopped.
func (s *Store) AutoReportGafferOutcomes() ([]GafferOutcome, error) {
	rows, err := s.db.Query(`SELECT user_id,data FROM gaffer_outcomes WHERE json_extract(data,'$.autoReport') AND json_extract(data,'$.state')!='stopped' ORDER BY created_at,id`)
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

// LinkTerminalSessionProjectFor records that a session belongs to a folder
// project. It is for a later pipeline stage, which starts inside the worktree
// its first stage was given: the session's folder must be the project's own
// folder or one of its reet worktrees, on the project's machine.
func (s *Store) LinkTerminalSessionProjectFor(uid int64, sessionID, projectID string) error {
	res, err := s.db.Exec(`UPDATE terminal_sessions SET project_id=? WHERE id=? AND user_id=? AND project_id IS NULL
 AND EXISTS(SELECT 1 FROM folder_projects p WHERE p.id=? AND p.user_id=? AND p.host=terminal_sessions.agent_id
  AND (terminal_sessions.dir=p.dir OR substr(terminal_sessions.dir,1,length(p.dir)+17)=p.dir||'/.reet/worktrees/'))`, projectID, sessionID, uid, projectID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
