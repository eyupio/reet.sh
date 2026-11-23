package store

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/eyupio/reet.sh/internal/skillbuddy"
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
// to start its next stage), "delivery_review" (the work is done and waits for
// the person's decision against the criteria they agreed), "ready_to_merge"
// (waiting for the person) and "done" (its last stage was handed over).
//
// A stage is one step of a pipeline, done by the agent and model the person
// approved for it. Stage states: "pending", "running", "done".
type GafferOutcomeStage struct {
	SkillGuidance []skillbuddy.Skill `json:"skillGuidance,omitempty"`
	Kind          string             `json:"kind"`
	Tool          string             `json:"tool"`
	Model         string             `json:"model,omitempty"`
	Effort        string             `json:"effort,omitempty"`
	Provider      string             `json:"provider,omitempty"`
	// Tier is how demanding the stage was judged to be (light, standard or
	// heavy), which is what chose its agent and model from the person's crew.
	// TierWhy is Gaffer's reason where the tier is its own assessment.
	Tier    string `json:"tier,omitempty"`
	TierWhy string `json:"tierWhy,omitempty"`
	// Tool, Model and Effort above are what the person chose. The Applied
	// fields are what the stage's session was read back to be running, and
	// when. ChoiceCheck is "applied" (nothing chosen differs), "mismatch"
	// (ChoiceNote names what differs) or "unchecked" (the machine could not
	// be asked; ChoiceNote says why). Empty means no check has been made. For
	// the stage in hand they are read again from the session on every read.
	AppliedTool   string `json:"appliedTool,omitempty"`
	AppliedModel  string `json:"appliedModel,omitempty"`
	AppliedEffort string `json:"appliedEffort,omitempty"`
	AppliedAt     int64  `json:"appliedAt,omitempty"`
	ChoiceCheck   string `json:"choiceCheck,omitempty"`
	ChoiceNote    string `json:"choiceNote,omitempty"`
	// EffortNotOffered is the effort that was chosen for the stage when its
	// agent turned out not to offer it for the model on that machine: the
	// machine's report said so, or it refused the start for it. The stage then
	// started on the agent's default effort and Effort is empty, so a later
	// start does not ask for it again. Empty when nothing was dropped.
	EffortNotOffered string `json:"effortNotOffered,omitempty"`
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
	// stage over), "person" (they moved it on), "chat" (they told Gaffer to)
	// or "auto" (Gaffer decided to, on an outcome whose decisions it was given).
	ClosedBy string `json:"closedBy,omitempty"`
	// Part and Brief mark one part of a stage that was split: the part's
	// short name and what it is to do. Parts of a stage sit next to each
	// other under the same Kind and run in order, each in a session of its
	// own on the agent its own tier means. Empty for a stage done as one.
	Part  string `json:"part,omitempty"`
	Brief string `json:"brief,omitempty"`
	// Backup is the person's second choice for this stage: used instead of
	// Tool, Model and Effort when their plan is running low and the backup's
	// has more left. When that happened, First is the first choice the stage
	// would otherwise have run on and Balanced says why in a sentence; Tool,
	// Model, Effort and Provider are then the backup's, which is what ran.
	Backup   *GafferStageAgent `json:"backup,omitempty"`
	First    *GafferStageAgent `json:"first,omitempty"`
	Balanced string            `json:"balanced,omitempty"`
	// More are further backups, tried in order after Backup. Rebalanced marks
	// a stage that was run again on a backup because its agent stopped with
	// its plan low: that is done once for a stage, never in a loop.
	More          []GafferStageAgent `json:"moreBackups,omitempty"`
	Rebalanced    bool               `json:"rebalanced,omitempty"`
	UsageFailover bool               `json:"usageFailover,omitempty"`
	// NextUsageBackup is computed for the view from the configured choices.
	NextUsageBackup string `json:"nextUsageBackup,omitempty"`
	// Decided counts the decisions Gaffer took on this stage for the person,
	// BudgetRenewed is whether one of them gave the stage a new budget, and
	// DecidedFor names the stop last put to Gaffer, so one stop is one request.
	Decided       int    `json:"decided,omitempty"`
	BudgetRenewed bool   `json:"budgetRenewed,omitempty"`
	DecidedFor    string `json:"decidedFor,omitempty"`
	// DecidingSince is when the stop named by DecidedFor was put to Gaffer, so
	// a controller that stopped before it recorded the decision asks again
	// instead of leaving the pipeline waiting for ever.
	DecidingSince int64 `json:"decidingSince,omitempty"`
	StartedAt     int64 `json:"startedAt,omitempty"`
	EndedAt       int64 `json:"endedAt,omitempty"`
	// What Gaffer's own model used following this stage, recorded when the
	// stage was handed over. It is Gaffer's usage, not the working agent's.
	Reviews       int  `json:"reviews,omitempty"`
	InputTokens   int  `json:"inputTokens,omitempty"`
	OutputTokens  int  `json:"outputTokens,omitempty"`
	UsageReported bool `json:"usageReported,omitempty"`
}

// GafferAppliedChoice is what a session was read back to be running, and
// whether that is what was chosen for it: applied, mismatch or unchecked.
type GafferAppliedChoice struct {
	Tool   string `json:"appliedTool,omitempty"`
	Model  string `json:"appliedModel,omitempty"`
	Effort string `json:"appliedEffort,omitempty"`
	At     int64  `json:"appliedAt,omitempty"`
	Check  string `json:"choiceCheck"`
	Note   string `json:"choiceNote,omitempty"`
	// ChosenTool and ChosenModel are what it was compared with.
	ChosenTool  string `json:"tool"`
	ChosenModel string `json:"model,omitempty"`
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
	Stages  []GafferOutcomeStage `json:"stages,omitempty"`
	Stage   int                  `json:"stage"`
	WorkDir string               `json:"workDir,omitempty"`
	// UnsureRequest is the request of a launch that was not answered in time.
	// Its session may have started all the same, so the next attempt asks
	// under this request first and takes that session up if it is there.
	UnsureRequest string `json:"unsureRequest,omitempty"`
	// Applied is what a pipeline with no stages was read back to be running,
	// against the outcome's own agent and model. A pipeline with stages keeps
	// this on each stage instead.
	Applied   *GafferAppliedChoice `json:"applied,omitempty"`
	AutoMerge bool                 `json:"autoMerge,omitempty"`
	// YoloMergeOK is the person's separate confirmation that a pipeline whose
	// agent runs on YOLO may also merge by itself. Without it such a pipeline
	// stops at Ready to merge whatever AutoMerge says.
	YoloMergeOK   bool `json:"yoloMergeOk,omitempty"`
	MergeApproved bool `json:"mergeApproved,omitempty"`
	// Retired are the Gaffer conversations of sessions this pipeline has left
	// behind: a stage that was sent back, or a session that ended and was
	// replaced. Their closed goals belong to the pipeline, not to the list of
	// the person's other sessions.
	Retired []string `json:"retired,omitempty"`
	// Criteria are the acceptance criteria the person agreed for this pipeline
	// before it started, or edited since. CriteriaRevision counts those edits
	// from 1; ReviewRevision counts every change to the delivery review
	// (criteria, evidence, a decision, the pipeline arriving at review), so a
	// write can name the review it was made on. Evidence is what was recorded
	// against a criterion and a commit, Deliveries the person's decisions,
	// newest last, and DeliveryAccepted whether the delivery in hand was
	// accepted. A pipeline saved before delivery review has none of these and
	// never stops for one.
	Criteria         []GafferCriterion        `json:"criteria,omitempty"`
	CriteriaRevision int                      `json:"criteriaRevision,omitempty"`
	ReviewRevision   int                      `json:"reviewRevision,omitempty"`
	Evidence         []GafferEvidence         `json:"evidence,omitempty"`
	Deliveries       []GafferDeliveryDecision `json:"deliveries,omitempty"`
	DeliveryAccepted bool                     `json:"deliveryAccepted,omitempty"`
	// SessionState is whether the session in hand is still there: "running",
	// "ended" (closed, history kept) or "gone" (removed). Live only, never saved.
	SessionState string `json:"sessionState,omitempty"`
}

// Limits of a pipeline's delivery review.
const (
	MaxGafferCriteria      = 10
	MaxGafferCriterionText = 240
	MaxGafferEvidence      = 40
	MaxGafferDeliveries    = 20
)

// GafferCriterion is one acceptance criterion. ID is stable for as long as
// the criterion exists; Revision is the criteria revision in which its text
// was last set, so evidence recorded against an earlier wording is not
// counted for the present one.
type GafferCriterion struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Revision int    `json:"revision"`
}

// GafferEvidence is one thing recorded against a criterion at a commit. Kind
// is "note" (a reviewer's own note), "artefact" (a link inside the pipeline's
// GitHub repository) or "check" (a GitHub check by Name, whose State and URL
// are what the machine reported when it was added). Verdict is "passed" or
// "needs_work". Commit is empty where the folder is not a Git repository.
type GafferEvidence struct {
	ID               string `json:"id"`
	CriterionID      string `json:"criterionId"`
	Kind             string `json:"kind"`
	Verdict          string `json:"verdict,omitempty"`
	Text             string `json:"text,omitempty"`
	URL              string `json:"url,omitempty"`
	Name             string `json:"name,omitempty"`
	State            string `json:"state,omitempty"`
	Commit           string `json:"commit,omitempty"`
	CriteriaRevision int    `json:"criteriaRevision"`
	By               int64  `json:"by"`
	ByName           string `json:"byName,omitempty"`
	At               int64  `json:"at"`
}

// GafferDeliveryDecision is the person's recorded decision on a delivery:
// "accept" or "send_back", for one criteria revision and one commit, with who
// decided and when. Statuses is each criterion's status as it stood, Failed
// the criteria that needed work, and Stage where a sent-back pipeline went.
type GafferDeliveryDecision struct {
	RequestID        string            `json:"requestId"`
	Decision         string            `json:"decision"`
	Commit           string            `json:"commit,omitempty"`
	CriteriaRevision int               `json:"criteriaRevision"`
	By               int64             `json:"by"`
	ByName           string            `json:"byName,omitempty"`
	At               int64             `json:"at"`
	Notes            string            `json:"notes,omitempty"`
	Stage            string            `json:"stage,omitempty"`
	Statuses         map[string]string `json:"statuses,omitempty"`
	Failed           []string          `json:"failed,omitempty"`
}

type GafferOutcome struct {
	OwnerID     int64  `json:"-"`
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	Objective   string `json:"objective"`
	State       string `json:"state"`
	Note        string `json:"note"`
	AgentID     string `json:"agentId"`
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	Dir         string `json:"dir"`
	Tool        string `json:"tool"`
	Model       string `json:"model,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Trust       bool   `json:"trust,omitempty"`
	Yolo        bool   `json:"yolo,omitempty"`
	// PullLatest is the person's answer for this outcome's fresh worktrees:
	// "yes" for GitHub's latest commit for the base, "no" for the machine's
	// copy, "" for the project's setting, then the account's.
	PullLatest string `json:"pullLatest,omitempty"`
	ProviderID string `json:"providerId"`
	// Images are the uploaded machine paths the person attached to the
	// outcome. An outcome saved before images could be attached has none.
	Images     []string               `json:"images,omitempty"`
	MaxReviews int                    `json:"maxReviews"`
	Hours      int                    `json:"hours"`
	Sessions   []GafferOutcomeSession `json:"sessions"`
	Report     *GafferOutcomeReport   `json:"report,omitempty"`
	// AutoReport is the person's choice, made when they approved the plan,
	// that Gaffer reports across the outcome by itself when a pipeline
	// finishes or stops. AutoReportedFor is what it last reported on, and
	// AutoReports how many it has made, so one event is one report.
	AutoReport      bool   `json:"autoReport,omitempty"`
	AutoReportedFor string `json:"autoReportedFor,omitempty"`
	AutoReports     int    `json:"autoReports,omitempty"`
	// BalanceBelow is the share of a plan, as a whole percentage, under which
	// a stage of this outcome starts on its backup. Zero is reet's default.
	BalanceBelow int `json:"balanceBelow,omitempty"`
	// Automation is the person's choice that Gaffer takes this outcome's
	// decisions for them: what to do when a stage stops. It never covers a
	// merge or an agent's approval. Decisions is what Gaffer then decided and
	// why, newest last, the most recent MaxGafferOutcomeDecisions kept.
	Automation bool                    `json:"automation,omitempty"`
	Decisions  []GafferOutcomeDecision `json:"decisions,omitempty"`
	// ArchivedAt is when the person put this finished outcome away; zero means
	// it is on the Factory floor. The record and its report are kept.
	ArchivedAt int64 `json:"archivedAt,omitempty"`
	CreatedAt  int64 `json:"createdAt"`
	UpdatedAt  int64 `json:"updatedAt"`
}

// MaxGafferOutcomeDecisions is how many of Gaffer's decisions an outcome keeps.
const MaxGafferOutcomeDecisions = 60

// GafferOutcomeDecision is one decision Gaffer took for the person on an
// outcome whose decisions it was given: which pipeline and stage, what had
// stopped it (About), what Gaffer chose and why. Failed marks a stop Gaffer
// could not decide, with the reason in Why; that stop stays with the person.
type GafferOutcomeDecision struct {
	At            int64  `json:"at"`
	Key           string `json:"key"`
	Title         string `json:"title"`
	Stage         string `json:"stage"`
	About         string `json:"about"`
	Choice        string `json:"choice,omitempty"`
	Why           string `json:"why"`
	Text          string `json:"text,omitempty"`
	Failed        bool   `json:"failed,omitempty"`
	Model         string `json:"model,omitempty"`
	InputTokens   int    `json:"inputTokens,omitempty"`
	OutputTokens  int    `json:"outputTokens,omitempty"`
	UsageReported bool   `json:"usageReported,omitempty"`
}

// Decide records one of Gaffer's decisions, keeping the most recent.
func (o *GafferOutcome) Decide(d GafferOutcomeDecision) {
	o.Decisions = append(o.Decisions, d)
	if extra := len(o.Decisions) - MaxGafferOutcomeDecisions; extra > 0 {
		o.Decisions = append([]GafferOutcomeDecision(nil), o.Decisions[extra:]...)
	}
}

// Retire records conversations the pipeline has left behind, newest last,
// keeping the most recent twenty.
func (p *GafferOutcomeSession) Retire(conversations ...string) {
	for _, id := range conversations {
		known := id == ""
		for _, have := range p.Retired {
			known = known || have == id
		}
		if !known {
			p.Retired = append(p.Retired, id)
		}
	}
	if len(p.Retired) > 20 {
		p.Retired = p.Retired[len(p.Retired)-20:]
	}
}

// Finished reports whether nothing in the outcome is still moving: it is not
// starting sessions and no pipeline is following, waiting, advancing, in
// delivery review or ready to merge. Only a finished outcome may be archived or deleted.
func (o GafferOutcome) Finished() bool {
	if o.State == "starting" {
		return false
	}
	for _, item := range o.Sessions {
		switch item.State {
		case "following", "starting", "advancing", "waiting", "delivery_review", "ready_to_merge":
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
 CREATE INDEX IF NOT EXISTS gaffer_outcomes_state ON gaffer_outcomes(json_extract(data,'$.state'));
 CREATE TABLE IF NOT EXISTS gaffer_crew(
 user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 updated_at INTEGER NOT NULL,data TEXT NOT NULL CHECK(json_valid(data)));`)
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
	// Only an outcome with nothing still moving is dropped: one whose pipelines
	// are followed or waiting for a merge decision is kept however old it is.
	if _, err = tx.Exec(`DELETE FROM gaffer_outcomes WHERE user_id=? AND json_extract(data,'$.state')!='starting'
 AND NOT EXISTS(SELECT 1 FROM json_each(gaffer_outcomes.data,'$.sessions') WHERE json_extract(value,'$.state') IN('following','starting','advancing','waiting','delivery_review','ready_to_merge'))
 AND id NOT IN(SELECT id FROM gaffer_outcomes WHERE user_id=? ORDER BY created_at DESC,id DESC LIMIT ?)`, uid, uid, MaxGafferOutcomes); err != nil {
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
	rows, err := s.db.Query(`SELECT data FROM gaffer_outcomes WHERE user_id=? ORDER BY created_at DESC,id DESC LIMIT ?`, uid, 2*MaxGafferOutcomes)
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

// GafferCrewTier is the agent, model and effort that does stages of one tier.
// An empty Tool means the tier is not set: such a stage is done by the
// outcome's own agent.
type GafferCrewTier struct {
	Tool     string `json:"tool,omitempty"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	Provider string `json:"provider,omitempty"`
	// Backup is who does the tier's stages when the first choice's plan is
	// running low and the backup has more left. Nil when none was chosen.
	Backup *GafferStageAgent `json:"backup,omitempty"`
	// More are further backups, tried in order after Backup.
	More []GafferStageAgent `json:"moreBackups,omitempty"`
}

// MaxGafferBackups is how many backups a tier or a stage can have.
const MaxGafferBackups = 3

// Spares lists a first backup and any further ones, in the order they are tried.
func Spares(first *GafferStageAgent, more []GafferStageAgent) []GafferStageAgent {
	if first == nil {
		return nil
	}
	return append([]GafferStageAgent{*first}, more...)
}

// GafferStageAgent is an agent, model and effort a stage can run on.
type GafferStageAgent struct {
	Tool     string `json:"tool"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// GafferCrew is a person's crew: who does light, standard and heavy stages.
// It is the account's default; an outcome may be approved with another.
type GafferCrew struct {
	Light    GafferCrewTier `json:"light"`
	Standard GafferCrewTier `json:"standard"`
	Heavy    GafferCrewTier `json:"heavy"`
	// BalanceBelow is the share of a plan, as a whole percentage, under which
	// a tier's stages start on its backup. Zero is reet's own default.
	BalanceBelow int `json:"balanceBelow,omitempty"`
	// GafferDecides is what the outcome dialog starts on for who takes an
	// outcome's decisions. It only sets where the dialog starts: each outcome
	// is still given to Gaffer, or not, by the person when they approve it,
	// and an outcome started any other way is never given to Gaffer by it.
	GafferDecides bool  `json:"gafferDecides,omitempty"`
	UpdatedAt     int64 `json:"updatedAt,omitempty"`
}

// Saved reports whether the person has ever saved a crew.
func (c GafferCrew) Saved() bool { return c.UpdatedAt > 0 }

// Tier is the crew for a tier by name; the zero tier for any other name.
func (c GafferCrew) Tier(name string) GafferCrewTier {
	switch name {
	case "light":
		return c.Light
	case "standard":
		return c.Standard
	case "heavy":
		return c.Heavy
	}
	return GafferCrewTier{}
}

// GafferCrewFor returns the account's saved crew, or an empty one.
func (s *Store) GafferCrewFor(uid int64) (GafferCrew, error) {
	var raw string
	var at int64
	err := s.db.QueryRow(`SELECT data,updated_at FROM gaffer_crew WHERE user_id=?`, uid).Scan(&raw, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return GafferCrew{}, nil
	}
	if err != nil {
		return GafferCrew{}, err
	}
	var crew GafferCrew
	if err = json.Unmarshal([]byte(raw), &crew); err != nil {
		return GafferCrew{}, err
	}
	crew.UpdatedAt = at
	return crew, nil
}

// SetGafferCrewFor replaces the account's crew.
func (s *Store) SetGafferCrewFor(uid int64, crew GafferCrew) (GafferCrew, error) {
	crew.UpdatedAt = now()
	raw, err := json.Marshal(crew)
	if err != nil {
		return crew, err
	}
	_, err = s.db.Exec(`INSERT INTO gaffer_crew(user_id,updated_at,data) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET updated_at=excluded.updated_at,data=excluded.data`, uid, crew.UpdatedAt, string(raw))
	return crew, err
}

// ValidGafferOutcomeID reports whether id has the form of an outcome's id.
func ValidGafferOutcomeID(id string) bool { return validGafferID(id) }

// GafferWorkDirInUseFor reports whether any pipeline that is not finished, in
// any of the account's outcomes that is not archived, works in dir. It reads
// every outcome, not only the ones the list shows.
func (s *Store) GafferWorkDirInUseFor(uid int64, dir string) (bool, error) {
	var used bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM gaffer_outcomes o, json_each(o.data,'$.sessions') piece
 WHERE o.user_id=? AND COALESCE(json_extract(o.data,'$.archivedAt'),0)=0
 AND json_extract(piece.value,'$.workDir')=? AND json_extract(piece.value,'$.state')!='done')`, uid, dir).Scan(&used)
	return used, err
}

// GafferWorkDirsInUseFor lists the folders that pipelines which are not
// finished, in outcomes that are not archived, work in.
func (s *Store) GafferWorkDirsInUseFor(uid int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT json_extract(piece.value,'$.workDir') FROM gaffer_outcomes o, json_each(o.data,'$.sessions') piece
 WHERE o.user_id=? AND COALESCE(json_extract(o.data,'$.archivedAt'),0)=0
 AND COALESCE(json_extract(piece.value,'$.workDir'),'')!='' AND json_extract(piece.value,'$.state')!='done'`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var dir string
		if err = rows.Scan(&dir); err != nil {
			return nil, err
		}
		out = append(out, dir)
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

// RetiredGafferOutcomes lists every account's outcomes, not archived, with a
// pipeline that has left sessions behind and is followed, waits for the
// person or is done: a session it left may still run.
func (s *Store) RetiredGafferOutcomes() ([]GafferOutcome, error) {
	rows, err := s.db.Query(`SELECT user_id,data FROM gaffer_outcomes WHERE COALESCE(json_extract(data,'$.archivedAt'),0)=0 AND EXISTS(SELECT 1 FROM json_each(gaffer_outcomes.data,'$.sessions') WHERE json_extract(value,'$.state') IN ('following','delivery_review','ready_to_merge','done') AND json_array_length(value,'$.retired')>0) ORDER BY created_at,id`)
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

// AutomatedGafferOutcomes lists the outcomes whose owner gave Gaffer the
// decisions, and that are neither stopped nor archived.
func (s *Store) AutomatedGafferOutcomes() ([]GafferOutcome, error) {
	rows, err := s.db.Query(`SELECT user_id,data FROM gaffer_outcomes WHERE json_extract(data,'$.automation') AND json_extract(data,'$.state')!='stopped' AND COALESCE(json_extract(data,'$.archivedAt'),0)=0 ORDER BY created_at,id`)
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

// BackedGafferOutcomes lists the outcomes that are neither stopped nor
// archived and have a stage with a backup.
func (s *Store) BackedGafferOutcomes() ([]GafferOutcome, error) {
	rows, err := s.db.Query(`SELECT user_id,data FROM gaffer_outcomes WHERE data LIKE '%"backup":{%' AND json_extract(data,'$.state')!='stopped' AND COALESCE(json_extract(data,'$.archivedAt'),0)=0 ORDER BY created_at,id`)
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
	return s.updateGafferOutcomeFor(uid, id, revision, func(_ *sql.Tx, o *GafferOutcome) (bool, error) {
		return change(o), nil
	})
}

// ReplaceGafferOutcomeWorkerFor retires the stopped goal and queues its replacement in one transaction.
func (s *Store) ReplaceGafferOutcomeWorkerFor(uid int64, id string, revision int64, session, conversation, note string, change func(*GafferOutcome) bool) (GafferOutcome, error) {
	return s.updateGafferOutcomeFor(uid, id, revision, func(tx *sql.Tx, o *GafferOutcome) (bool, error) {
		if !change(o) {
			return false, ErrGafferOutcomeBusy
		}
		g, err := goalRow(tx, uid, conversation)
		if err != nil {
			return false, err
		}
		status, err := gafferWorkerStatus(tx, uid, session)
		if err != nil {
			return false, err
		}
		if g.SessionID != session || g.State != "needs_user" || g.WaitingOnWorker || g.ActiveTurnID != "" || !status.UsageLimited() {
			return false, ErrGafferOutcomeBusy
		}
		if err = removeGoalQueue(tx, g); err != nil {
			return false, err
		}
		g.State, g.Note, g.NextAt, g.Advice = "stopped", note, 0, nil
		return true, saveGoal(tx, g)
	})
}

func (s *Store) updateGafferOutcomeFor(uid int64, id string, revision int64, change func(*sql.Tx, *GafferOutcome) (bool, error)) (GafferOutcome, error) {
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
	if changed, err := change(tx, &o); err != nil || !changed {
		return o, err
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
// folder, or the worktree of a session already listed under that project, on
// the project's machine. A session already listed there is left as it is.
func (s *Store) LinkTerminalSessionProjectFor(uid int64, sessionID, projectID string) error {
	res, err := s.db.Exec(`UPDATE terminal_sessions SET project_id=? WHERE id=? AND user_id=? AND (project_id IS NULL OR project_id=?)
 AND EXISTS(SELECT 1 FROM folder_projects p WHERE p.id=? AND p.user_id=? AND p.host=terminal_sessions.agent_id
  AND (terminal_sessions.dir=p.dir OR EXISTS(SELECT 1 FROM terminal_sessions f WHERE f.user_id=p.user_id AND f.agent_id=p.host
   AND f.project_id=p.id AND f.worktree_path<>'' AND f.worktree_path=terminal_sessions.dir)))`, projectID, sessionID, uid, projectID, projectID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
