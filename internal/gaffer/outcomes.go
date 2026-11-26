package gaffer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/elicore"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// An outcome is an objective the person hands Gaffer. Planning it is one model
// request that proposes the sessions to start. The only reet data it reads is the
// name, description and stage names of each of the person's enabled pathways, so
// the plan can suggest one; never a stage's prompt, agent, model or skills. It
// starts no session, saves nothing and grants nothing: the person reviews and
// edits the plan, and their browser starts each session through the ordinary
// launch path.
const (
	MaxOutcomeSessions = 5
	// MaxOutcomeImages matches the images one ACP prompt can carry: an
	// outcome's images are handed to its sessions, so more could not travel.
	MaxOutcomeImages         = terminalproto.MaxACPImages
	MaxOutcomeObjectiveBytes = 4000
	MaxOutcomePromptBytes    = 8000
	MaxOutcomeReplyBytes     = 2000
	maxOutcomeSummaryBytes   = 2000
	// maxHandoverBytes is what a finished stage may hand the next one. A plan
	// or design is handed over as its content, which does not fit in 2,000
	// bytes; half a stage prompt leaves room for the brief and the boundary.
	maxHandoverBytes = 4000
)

type OutcomePlanInput struct {
	Revision  int64  `json:"revision"`
	Objective string `json:"objective"`
	Project   string `json:"project"`
	Sessions  int    `json:"sessions"`
	// Reply is the person's answer to the previous proposal. It is sent only
	// with Previous, and never saved or run.
	Reply    string               `json:"reply,omitempty"`
	Previous *OutcomePlanPrevious `json:"previous,omitempty"`
}

// OutcomePlanPrevious is a compact copy of the proposal a reply answers: the
// summary and session titles are enough for Gaffer to know what it proposed,
// without replaying every prompt through the provider again.
type OutcomePlanPrevious struct {
	Summary  string                       `json:"summary"`
	Sessions []OutcomePlanPreviousSession `json:"sessions"`
}

type OutcomePlanPreviousSession struct {
	Title  string   `json:"title"`
	Stages []string `json:"stages,omitempty"`
}

type OutcomeSession struct {
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	// Stages makes the session a pipeline. AutoMerge overrides the outcome's
	// own setting for this pipeline.
	Stages    []OutcomeStageInput `json:"stages,omitempty"`
	AutoMerge *bool               `json:"autoMerge,omitempty"`
	// Criteria are the acceptance criteria the person agreed for this
	// pipeline. They are the person's own: a plan from Gaffer's provider has
	// no field for them. A pipeline with criteria stops for the person's
	// delivery review before it merges or finishes.
	Criteria []string `json:"criteria,omitempty"`
	// CriteriaIDs are a template's ids for Criteria, so a criterion keeps its
	// identity from the template through the run and back into a new version.
	CriteriaIDs []string `json:"-"`
	// Pathway, set only by a pathway launch, is the pathway the stages came
	// from: the pipeline keeps their order and may have custom stages.
	Pathway *store.GafferPathwayRef `json:"-"`
}

type OutcomePlan struct {
	Summary       string           `json:"summary"`
	Sessions      []OutcomeSession `json:"sessions"`
	Model         string           `json:"model"`
	UsageReported bool             `json:"usageReported"`
	InputTokens   int              `json:"inputTokens"`
	OutputTokens  int              `json:"outputTokens"`
	Pathway       *PlanPathway     `json:"pathway,omitempty"`
}

// PlanPathway is one of the person's enabled pathways that Gaffer suggests for
// the plan. Why is the model's words: one line, shown and never run.
type PlanPathway struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Why  string `json:"why,omitempty"` // one line, at most 200 bytes
}

var planSessionsTool = elicore.Tool{Name: "propose_sessions", Description: "Propose the coding-agent sessions to start for the user's objective. This starts nothing: the user reviews and edits the proposal before any session exists.", Parameters: json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string","minLength":1,"maxLength":1000},"sessions":{"type":"array","minItems":1,"maxItems":5,"items":{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":80},"prompt":{"type":"string","minLength":1,"maxLength":6000},"stages":{"type":"array","maxItems":8,"items":{"type":"string","enum":["plan","design","code","review","test","pr","babysit","merge"]}},"ratings":{"type":"array","maxItems":8,"items":{"type":"object","properties":{"stage":{"type":"string","enum":["plan","design","code","review","test","pr","babysit","merge"]},"tier":{"type":"string","enum":["light","standard","heavy"]},"why":{"type":"string","maxLength":240}},"required":["stage","tier"],"additionalProperties":false}},"parts":{"type":"array","maxItems":12,"items":{"type":"object","properties":{"stage":{"type":"string","enum":["plan","design","code","review","test"]},"title":{"type":"string","minLength":1,"maxLength":60},"brief":{"type":"string","minLength":1,"maxLength":1000},"tier":{"type":"string","enum":["light","standard","heavy"]},"why":{"type":"string","maxLength":240}},"required":["stage","title","brief"],"additionalProperties":false}}},"required":["title","prompt"],"additionalProperties":false}}},"required":["summary","sessions"],"additionalProperties":false}`)}

// planSessionsWithPathwaysTool is planSessionsTool with a top-level choice of
// one of the person's pathways. It is offered only to an account that has an
// enabled pathway, so without one the request is exactly as it always was.
var planSessionsWithPathwaysTool = elicore.Tool{Name: planSessionsTool.Name, Description: planSessionsTool.Description, Parameters: json.RawMessage(strings.Replace(string(planSessionsTool.Parameters),
	`},"required":["summary","sessions"]`, `,"pathway":{"type":"string","maxLength":60},"pathway_why":{"type":"string","maxLength":200}},"required":["summary","sessions"]`, 1))}

// maxPlanPathways is how many enabled pathways, by name, planning offers.
const maxPlanPathways = 20

// maxPlanPathwayWhyBytes is the most of the model's reason for a pathway kept.
const maxPlanPathwayWhyBytes = 200

func outcomeSystem(max int, pathways bool) string {
	suggest := ""
	if pathways {
		suggest = "\nIf one of the person's saved pathways clearly fits the objective, name it in `pathway` with a short reason in `pathway_why`; otherwise leave both out. Still propose stages for every session."
	}
	return "You are Gaffer, the user's warm, practical critical buddy in reet.sh, with occasional dry Yorkshire humour. The user has given you an objective to own. Plan the coding-agent sessions that would meet it, then call propose_sessions exactly once. You start nothing: the user reviews, edits and approves the plan before any session exists.\n" +
		"Each session is a separate coding agent working in its own isolated copy of the same project. Sessions cannot see or message each other, so every prompt must stand alone: say what to build or change, what is out of scope, how to check it, and that it should raise a pull request describing the work. Split only where the work is genuinely independent, so two sessions never edit the same code. Prefer one session for a small objective. Propose at most " + strconv.Itoa(max) + " sessions.\n" +
		"Stay inside the objective: add no scope the user did not ask for, and when something is ambiguous say so in the summary instead of guessing. Each session is a pipeline that runs through stages in this order: plan, design, code, review, test, pr, babysit, merge. For each session list in \"stages\" the ones it needs: leave out plan and design for small, obvious work, and include review and test for anything that changes behaviour. You advise on how demanding each stage is; the user decides which agent and model that means and whether merge happens without them, and you name neither. In \"ratings\" rate a stage light, standard or heavy with one short reason, judged from this work and not from the stage's name: light is mechanical or routine (a pull request for finished work, a small rename), standard is ordinary building and checking, heavy needs careful judgement (unfamiliar or intricate code, data that must not be lost, security, a design with real trade-offs). The user's defaults are heavy for plan, design and review, standard for code and test, and light for pr, babysit and merge: rate only the stages where this work calls for something else, and leave the rest out. A stage whose work falls into clearly different pieces that suit different tiers can be split: in \"parts\" give two to four parts for that stage, each with a short title, a brief saying exactly what that part does and, where it differs from the stage's, its own tier and reason. Parts of a stage run one after another in the same folder, each done by a separate agent that sees only its own brief and the part before's handover, so each brief must stand alone and the order must make sense. Split only when it lets cheaper work go to a lighter tier or hard work to a heavier one; most stages are not split, and the pull request, babysitting and merge never are. Every prompt must end with the sentence \"Do not merge.\" Titles are short plain names of at most 80 characters. The summary says in two or three sentences how the sessions divide the work and what the user should check.\n" +
		"When the person replies to a previous proposal, their reply is the instruction to act on. Keep what is still right, change what they asked, and address their reply directly in the summary rather than repeating a proposal they turned down." + suggest
}

// strictJSON decodes raw into v, refusing a field v does not name and anything after the one value.
func strictJSON(raw []byte, v any) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var extra any
	return d.Decode(v) == nil && d.Decode(&extra) == io.EOF
}

// pipelineText is what a model wrote, with the tabs and Windows line ends it
// writes made plain: neither changes the meaning, and the launch prompt rules refuse both.
var pipelineText = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ")

// validPlanPrevious accepts the compact proposal a reply answers. It is
// model text the browser kept, so it is checked again before it is sent back
// to a provider: only the summary, titles and stage kinds are carried.
func validPlanPrevious(in *OutcomePlanPrevious) (*OutcomePlanPrevious, error) {
	if in == nil {
		return nil, nil
	}
	summary := strings.TrimSpace(in.Summary)
	if len(summary) > maxOutcomeSummaryBytes || (summary != "" && !store.ValidWorkerText(summary)) {
		return nil, invalid("previous", "The previous proposal could not be read. Ask Gaffer for a fresh plan.")
	}
	if len(in.Sessions) > MaxOutcomeSessions {
		return nil, invalid("previous", "The previous proposal could not be read. Ask Gaffer for a fresh plan.")
	}
	out := &OutcomePlanPrevious{Summary: summary, Sessions: make([]OutcomePlanPreviousSession, 0, len(in.Sessions))}
	for _, session := range in.Sessions {
		title := strings.TrimSpace(session.Title)
		if !terminalproto.ValidTitle(title) || len(session.Stages) > len(StageKinds) {
			return nil, invalid("previous", "The previous proposal could not be read. Ask Gaffer for a fresh plan.")
		}
		for _, kind := range session.Stages {
			if stageIndex(kind) < 0 {
				return nil, invalid("previous", "The previous proposal could not be read. Ask Gaffer for a fresh plan.")
			}
		}
		out.Sessions = append(out.Sessions, OutcomePlanPreviousSession{Title: title, Stages: append([]string(nil), session.Stages...)})
	}
	return out, nil
}

// planRequest is one validated plan ask: the provider row it was checked
// against, its normalized settings and the text sent to the model.
type planRequest struct {
	old      store.GafferProvider
	cfg      Input
	ask      string
	sessions int
	// pathways are the enabled pathways offered to the model, by name.
	pathways []store.GafferPathway
}

// PlanOutcome asks the chosen provider for a session plan. The provider
// receives the typed objective and project name; a reply to an earlier
// proposal also carries that reply and a compact copy of what it answered.
func (s *Service) PlanOutcome(parent context.Context, p Principal, id string, in OutcomePlanInput) (OutcomePlan, error) {
	request, err := s.preparePlan(p, id, in)
	if err != nil {
		return OutcomePlan{}, err
	}
	ctx, done, err := s.begin(parent, p, id, 90*time.Second)
	if err != nil {
		return OutcomePlan{}, err
	}
	defer done()
	return s.executePlan(ctx, parent, p, request, func(string) {})
}

func (s *Service) preparePlan(p Principal, id string, in OutcomePlanInput) (planRequest, error) {
	objective, project := strings.TrimSpace(in.Objective), strings.TrimSpace(in.Project)
	if objective == "" || len(objective) > MaxOutcomeObjectiveBytes || !store.ValidWorkerText(objective) {
		return planRequest{}, invalid("objective", "Write the objective in up to 4,000 UTF-8 bytes of plain text.")
	}
	if len(project) > 200 || !utf8.ValidString(project) || strings.ContainsAny(project, "\x00\r\n") {
		return planRequest{}, invalid("project", "Use a project name of up to 200 bytes on one line.")
	}
	if in.Sessions < 1 || in.Sessions > MaxOutcomeSessions {
		return planRequest{}, invalid("sessions", "Choose between 1 and 5 sessions.")
	}
	reply := strings.TrimSpace(in.Reply)
	if reply != "" && (len(reply) > MaxOutcomeReplyBytes || !store.ValidWorkerText(reply)) {
		return planRequest{}, invalid("reply", "Write the reply in up to 2,000 UTF-8 bytes of plain text.")
	}
	var previous *OutcomePlanPrevious
	if reply != "" {
		var err error
		previous, err = validPlanPrevious(in.Previous)
		if err != nil {
			return planRequest{}, err
		}
	}
	old, err := s.owned(p, Input{ID: id, Revision: in.Revision})
	if err != nil {
		return planRequest{}, err
	}
	if old.ID == "" {
		return planRequest{}, store.ErrNotFound
	}
	if !old.Enabled {
		return planRequest{}, invalid("provider", "Enable this Gaffer provider before asking it for a plan.")
	}
	cfg, err := normalize(p, inputFrom(old), true)
	if err != nil {
		return planRequest{}, err
	}
	ask := "Objective:\n" + objective
	if project != "" {
		ask += "\n\nProject: " + project
	}
	if reply != "" {
		ask += "\n\nYour previous proposal:"
		if previous != nil && (previous.Summary != "" || len(previous.Sessions) > 0) {
			if previous.Summary != "" {
				ask += "\nSummary: " + previous.Summary
			}
			for i, session := range previous.Sessions {
				ask += "\nSession " + strconv.Itoa(i+1) + ": " + session.Title
				if len(session.Stages) > 0 {
					ask += " (stages: " + strings.Join(session.Stages, ", ") + ")"
				}
			}
		} else {
			ask += "\n(The previous proposal's text was not sent.)"
		}
		ask += "\n\nThe person replied:\n" + reply + "\n\nRevise the proposal as a direct response to that reply. Keep what is still right, change what the person asked, and address their reply in the summary. Call propose_sessions exactly once."
	}
	all, err := s.store.GafferPathwaysFor(p.UserID)
	if err != nil {
		return planRequest{}, err
	}
	var pathways []store.GafferPathway
	for _, pathway := range all {
		if pathway.Enabled && len(pathways) < maxPlanPathways {
			pathways = append(pathways, pathway)
		}
	}
	if len(pathways) > 0 {
		// The name and description are the person's own text, checked when
		// saved; each pathway is one line of data.
		ask += "\n\nPathways the person has saved (optional):"
		for _, pathway := range pathways {
			labels := make([]string, len(pathway.Stages))
			for i, stage := range pathway.Stages {
				labels[i] = stage.Kind
				if stage.Kind == "custom" {
					labels[i] = stage.Name
				}
			}
			ask += "\n- " + pathway.Name + ":"
			// A saved description may hold line breaks; folding its whitespace
			// keeps it on its pathway's line, so it cannot end the list.
			if description := strings.Join(strings.Fields(pathway.Description), " "); description != "" {
				ask += " " + description
			}
			ask += " [" + strings.Join(labels, " › ") + "]"
		}
	}
	return planRequest{old: old, cfg: cfg, ask: ask, sessions: in.Sessions, pathways: pathways}, nil
}

// step is told where the plan has got to: PlanSending until the provider accepts the request, PlanWriting while it
// answers, PlanChecking once the answer is in.
func (s *Service) executePlan(ctx context.Context, outer context.Context, p Principal, request planRequest, step func(string)) (OutcomePlan, error) {
	var plan OutcomePlan
	if err := s.current(p, request.old); err != nil {
		return plan, err
	}
	backend, client, err := s.open(p, request.cfg, request.old, "")
	if err != nil {
		return plan, err
	}
	defer client.CloseIdleConnections()
	tool := planSessionsTool
	if len(request.pathways) > 0 {
		tool = planSessionsWithPathwaysTool
	}
	stream, err := backend.Chat(ctx, elicore.Request{Model: request.cfg.Model, System: outcomeSystem(request.sessions, len(request.pathways) > 0), Messages: []elicore.Message{{Role: elicore.RoleUser, Content: request.ask}}, Tools: []elicore.Tool{tool}, MaxTokens: 4096})
	if err != nil {
		return plan, planFailure(outer, err)
	}
	defer stream.Close()
	step(PlanWriting)
	var calls []elicore.ToolCall
	cut := false
	for {
		event, ok := stream.Next(ctx)
		if !ok {
			break
		}
		if event.Err != nil {
			return plan, planFailure(outer, event.Err)
		}
		if event.ToolCall != nil {
			calls = append(calls, *event.ToolCall)
		}
		if event.Usage != nil {
			plan.UsageReported, plan.InputTokens, plan.OutputTokens = event.Usage.Reported, event.Usage.InputTokens, event.Usage.OutputTokens
		}
		cut = cut || event.Cut
		if event.Done {
			break
		}
	}
	step(PlanChecking)
	if outer.Err() != nil {
		return plan, outer.Err()
	}
	if err = s.current(p, request.old); err != nil {
		return plan, err
	}
	if cut {
		return plan, &ProbeError{"The provider stopped before the plan was complete. Try fewer sessions or a shorter objective, or write the plan yourself."}
	}
	if len(calls) != 1 || calls[0].Name != planSessionsTool.Name {
		return plan, &ProbeError{"This model did not return a plan. Choose a model that supports tools, or write the plan yourself."}
	}
	summary, sessions, pathway, err := outcomeSessions(calls[0].Arguments, request.sessions, request.pathways)
	if err != nil {
		return plan, err
	}
	plan.Summary, plan.Sessions, plan.Pathway, plan.Model = summary, sessions, pathway, request.cfg.Model
	return plan, nil
}

// planJobTTL is how long a finished plan stays available for the browser to
// read. Planning has no side effects, so an expired job is simply asked for
// again.
const planJobTTL = 5 * time.Minute

// The steps of a plan job, as the browser is told them.
const (
	PlanSending  = "sending"
	PlanWriting  = "writing"
	PlanChecking = "checking"
)

type planJob struct {
	uid      int64
	provider string
	phase    string
	cancel   context.CancelFunc
	ready    bool
	plan     OutcomePlan
	err      error
	finished time.Time
}

// StartPlan begins a plan in the background and returns its job ID. The
// provider call is not tied to the browser request: a proxy that gives up
// waiting (Cloudflare ends an origin response after 100 seconds) no longer
// cancels the plan, and the browser reads it with PlanResult.
func (s *Service) StartPlan(p Principal, id string, in OutcomePlanInput) (string, error) {
	request, err := s.preparePlan(p, id, in)
	if err != nil {
		return "", err
	}
	ctx, done, err := s.begin(context.Background(), p, id, 90*time.Second)
	if err != nil {
		return "", err
	}
	jobID, err := newChatID()
	if err != nil {
		done()
		return "", err
	}
	ctx, cancel := context.WithCancel(ctx)
	job := &planJob{uid: p.UserID, provider: id, phase: PlanSending, cancel: cancel}
	s.mu.Lock()
	s.prunePlansLocked(time.Now())
	s.plans[jobID] = job
	s.mu.Unlock()
	go s.runPlan(ctx, done, job, p, request)
	return jobID, nil
}

func (s *Service) runPlan(ctx context.Context, done func(), job *planJob, p Principal, request planRequest) {
	defer done()
	plan, err := s.executePlan(ctx, context.Background(), p, request, func(phase string) {
		s.mu.Lock()
		job.phase = phase
		s.mu.Unlock()
	})
	s.mu.Lock()
	job.plan, job.err, job.ready, job.finished = plan, err, true, time.Now()
	s.mu.Unlock()
}

// PlanResult returns a finished plan, or ready=false while it is still being
// asked for. A failed plan returns its error, which the API maps as usual.
func (s *Service) PlanResult(p Principal, providerID, jobID string) (OutcomePlan, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prunePlansLocked(time.Now())
	job := s.plans[jobID]
	if job == nil || job.uid != p.UserID || job.provider != providerID {
		return OutcomePlan{}, false, store.ErrNotFound
	}
	if !job.ready {
		return OutcomePlan{}, false, nil
	}
	if job.err != nil {
		return OutcomePlan{}, false, job.err
	}
	return job.plan, true, nil
}

// PlanPhase is the step a plan job is on, or the step it failed on. It is empty for a job this person cannot read.
func (s *Service) PlanPhase(p Principal, providerID, jobID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.plans[jobID]; job != nil && job.uid == p.UserID && job.provider == providerID {
		return job.phase
	}
	return ""
}

// StopPlan gives up a plan the person no longer wants: the provider request is cancelled, which frees its place, and
// the job is forgotten.
func (s *Service) StopPlan(p Principal, providerID, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.plans[jobID]
	if job == nil || job.uid != p.UserID || job.provider != providerID {
		return store.ErrNotFound
	}
	job.cancel()
	delete(s.plans, jobID)
	return nil
}

func (s *Service) prunePlansLocked(now time.Time) {
	for id, job := range s.plans {
		if job.ready && now.Sub(job.finished) > planJobTTL {
			delete(s.plans, id)
		}
	}
}

func planFailure(parent context.Context, err error) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	message := "The provider could not plan this outcome: " + err.Error()
	if len(message) > 600 {
		message = strings.ToValidUTF8(message[:600], "") + "…"
	}
	return &ProbeError{message}
}

// outcomeSessions accepts only a complete plan within the asked size. Model
// text becomes editable titles and prompts and nothing else: it is never run,
// and a session exists only after the person approves it. A suggested pathway
// is kept only when it names one of pathways, the ones the model was offered;
// otherwise it is dropped and the plan kept.
func outcomeSessions(raw json.RawMessage, max int, pathways []store.GafferPathway) (string, []OutcomeSession, *PlanPathway, error) {
	malformed := &ProbeError{"The provider returned a plan reet could not read. Try again, or write the plan yourself."}
	if len(raw) > 64<<10 {
		return "", nil, nil, malformed
	}
	// The provider's plan is a title, a prompt and stage kinds. It has no
	// field for an agent, a model or auto-merge, so it cannot choose them.
	var in struct {
		Summary  string `json:"summary"`
		Sessions []struct {
			Title   string   `json:"title"`
			Prompt  string   `json:"prompt"`
			Stages  []string `json:"stages"`
			Ratings []struct {
				Stage string `json:"stage"`
				Tier  string `json:"tier"`
				Why   string `json:"why"`
			} `json:"ratings"`
			Parts []struct {
				Stage string `json:"stage"`
				Title string `json:"title"`
				Brief string `json:"brief"`
				Tier  string `json:"tier"`
				Why   string `json:"why"`
			} `json:"parts"`
		} `json:"sessions"`
		// Raw, so that a field's presence counts even when its value is null.
		Pathway    json.RawMessage `json:"pathway"`
		PathwayWhy json.RawMessage `json:"pathway_why"`
	}
	// Without pathways the tool has no such fields, and a plan naming them is
	// refused as any unknown field is.
	var name, why string
	if !strictJSON(raw, &in) || len(in.Sessions) == 0 || len(pathways) == 0 && (in.Pathway != nil || in.PathwayWhy != nil) ||
		in.Pathway != nil && json.Unmarshal(in.Pathway, &name) != nil || in.PathwayWhy != nil && json.Unmarshal(in.PathwayWhy, &why) != nil {
		return "", nil, nil, malformed
	}
	if len(in.Sessions) > max {
		return "", nil, nil, &ProbeError{"The provider proposed more sessions than you asked for. Try again, or write the plan yourself."}
	}
	summary := strings.TrimSpace(in.Summary)
	if len(summary) > maxOutcomeSummaryBytes || !utf8.ValidString(summary) || strings.ContainsRune(summary, 0) {
		return "", nil, nil, malformed
	}
	seen := map[string]bool{}
	out := make([]OutcomeSession, 0, len(in.Sessions))
	for _, item := range in.Sessions {
		prompt := strings.TrimSpace(pipelineText.Replace(item.Prompt))
		if prompt == "" || len(prompt) > MaxOutcomePromptBytes || !terminalproto.ValidPrompt(prompt) {
			return "", nil, nil, malformed
		}
		title := strings.TrimSpace(item.Title)
		if !terminalproto.ValidTitle(title) {
			title = terminalproto.PromptTitle(prompt)
		}
		if !terminalproto.ValidTitle(title) || seen[title] {
			return "", nil, nil, malformed
		}
		seen[title] = true
		var stages []OutcomeStageInput
		last := -1
		for _, kind := range item.Stages {
			at := stageIndex(kind)
			if at <= last {
				return "", nil, nil, malformed
			}
			last = at
			stages = append(stages, OutcomeStageInput{Kind: kind})
		}
		// A rating is a tier and a reason for one of the pipeline's own stages:
		// it has no field for an agent or a model. The reason is the model's
		// words, shown to the person and never run.
		rated := map[string]bool{}
		for _, rating := range item.Ratings {
			why := strings.TrimSpace(rating.Why)
			if !validStageTier(rating.Tier) || rated[rating.Stage] || len(why) > 300 || why != "" && !store.ValidWorkerText(why) {
				return "", nil, nil, malformed
			}
			rated[rating.Stage] = true
			found := false
			for i := range stages {
				if stages[i].Kind == rating.Stage {
					stages[i].Tier, stages[i].TierWhy, found = rating.Tier, why, true
				}
			}
			if !found {
				return "", nil, nil, malformed
			}
		}
		// A split replaces a stage with its parts, in the order given. A part is
		// a name, a brief and a tier: like a rating, it names no agent or model.
		split := map[string][]OutcomeStageInput{}
		var order []string
		for _, part := range item.Parts {
			name := strings.TrimSpace(part.Title)
			brief := strings.TrimSpace(pipelineText.Replace(part.Brief))
			why := strings.TrimSpace(part.Why)
			if !stageTakesParts(part.Stage) || !terminalproto.ValidTitle(name) || brief == "" || len(brief) > maxPartBriefBytes || !store.ValidWorkerText(brief) ||
				part.Tier != "" && !validStageTier(part.Tier) || len(why) > 300 || why != "" && !store.ValidWorkerText(why) {
				return "", nil, nil, malformed
			}
			for _, have := range split[part.Stage] {
				if have.Part == name {
					return "", nil, nil, malformed
				}
			}
			if len(split[part.Stage]) == 0 {
				order = append(order, part.Stage)
			}
			split[part.Stage] = append(split[part.Stage], OutcomeStageInput{Kind: part.Stage, Part: name, Brief: brief, Tier: part.Tier, TierWhy: why})
		}
		for _, kind := range order {
			parts := split[kind]
			at := -1
			for i := range stages {
				if stages[i].Kind == kind {
					at = i
				}
			}
			// One part is not a split, and a split is of a stage the pipeline has.
			if at < 0 || len(parts) < 2 || len(parts) > MaxStageParts {
				return "", nil, nil, malformed
			}
			for i := range parts {
				if parts[i].Tier == "" {
					// A part that says nothing takes the stage's own rating.
					parts[i].Tier, parts[i].TierWhy = stages[at].Tier, stages[at].TierWhy
				}
			}
			stages = append(stages[:at], append(parts, stages[at+1:]...)...)
		}
		out = append(out, OutcomeSession{Title: title, Prompt: prompt, Stages: stages})
	}
	return summary, out, planPathway(name, why, pathways), nil
}

// planPathway is the pathway the model named, matched without case against
// those it was offered, or nil. Its reason is clipped and kept only as one line
// of plain text; a bad reason drops the reason, not the suggestion.
func planPathway(name, why string, pathways []store.GafferPathway) *PlanPathway {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for _, pathway := range pathways {
		if !strings.EqualFold(name, pathway.Name) {
			continue
		}
		out := &PlanPathway{ID: pathway.ID, Name: pathway.Name}
		text := strings.TrimSpace(why)
		if len(text) > maxPlanPathwayWhyBytes {
			text = strings.TrimSpace(strings.ToValidUTF8(text[:maxPlanPathwayWhyBytes], ""))
		}
		if store.ValidWorkerText(text) && !strings.ContainsAny(text, "\r\n") {
			out.Why = text
		}
		return out
	}
	return nil
}
