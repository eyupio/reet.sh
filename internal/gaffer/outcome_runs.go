package gaffer

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A controller-owned outcome. The person approves a plan in their browser;
// from then on the controller starts each planned session through the
// ordinary launch and puts it in Gaffer's care, whether or not that browser
// stays open. The person's approval of the plan is the confirmation for each
// session's access and goal, and is the only thing that can start a session:
// the model's plan is editable text until then.

// OutcomeLaunch is one approved session for the launch path to start.
type OutcomeLaunch struct {
	AgentID, RequestID, Title, Tool, Model, Dir, ProjectID, Prompt string
	// Trust is the person's own confirmation, made when they approved the
	// plan, that this folder's plugins may run in the agent.
	Trust bool
}

// SessionStarter starts an approved session as its owner through the same
// checks as a launch from the browser, without YOLO. A repeated request ID
// answers the session that request already started. The error text is shown
// to the person.
type SessionStarter interface {
	StartOutcomeSession(ctx context.Context, uid int64, in OutcomeLaunch) (sessionID, workDir string, err error)
}

func (s *Service) SetSessionStarter(starter SessionStarter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starter = starter
}

type OutcomeInput struct {
	Objective  string           `json:"objective"`
	ProjectID  string           `json:"projectId"`
	Tool       string           `json:"tool"`
	Model      string           `json:"model"`
	Trust      bool             `json:"trust"`
	Sessions   []OutcomeSession `json:"sessions"`
	MaxReviews int              `json:"maxReviews"`
	Hours      int              `json:"hours"`
	Confirmed  bool             `json:"confirmed"`
	// AutoMerge lets every pipeline of this outcome merge by itself once it is
	// green, unless a pipeline says otherwise. It is the person's own choice.
	AutoMerge bool `json:"autoMerge"`
}

// OutcomeStageInput is one stage of a planned pipeline: what it is and which
// agent and model do it. A plan from Gaffer's provider carries the kind only.
type OutcomeStageInput struct {
	Kind  string `json:"kind"`
	Tool  string `json:"tool,omitempty"`
	Model string `json:"model,omitempty"`
}

// StageKinds is the order a pipeline runs in. A pipeline may leave stages
// out, never reorder them.
var StageKinds = []string{"plan", "design", "code", "review", "test", "pr", "babysit", "merge"}

var stageWork = map[string]string{
	"plan":    "Plan this piece of work: read the relevant code, then write a short implementation plan in the session with the files to change, the approach and the risks. Change no code yet.",
	"design":  "Settle the design for this piece: interfaces, data shapes, error cases and how it will be tested. Record the decisions in the session. Do not implement yet.",
	"code":    "Implement this piece of work with tests, following the plan and design already agreed in this branch or handover. Commit as you go.",
	"review":  "Review the changes on this branch as a critical second pair of eyes: correctness, security, missing tests and needless complexity. Fix what is clearly wrong and commit it; list anything that needs a decision.",
	"test":    "Run the project's full checks and tests for this branch. Fix failures this work caused, and report any that look unrelated.",
	"pr":      "Raise a pull request for this branch with an accurate title and description of what changed and how it was checked.",
	"babysit": "Look after the pull request until it is ready: fix failing checks, answer and address review comments, and resolve conflicts with the base branch.",
	"merge":   "Merge the pull request, but only once every check passes, no review thread is open and there are no conflicts. If any of those is not true, fix it first or bring it back to me.",
}

func stageIndex(kind string) int {
	for i, k := range StageKinds {
		if k == kind {
			return i
		}
	}
	return -1
}

// StageGoal is the saved goal for one stage of a pipeline. Every stage but
// merge ends "Do not merge."
func StageGoal(objective, title, kind string) string {
	text := "Outcome: " + clipBytes(objective, outcomeLineBytes) + "\nThis session’s part: " + clipBytes(title, 240) + "\nStage: " + kind + "\n" + stageWork[kind] + " Stay inside this stage and this part of the outcome. Bring scope changes and decisions back to me."
	if kind != "merge" {
		text += " Do not merge."
	}
	return text
}

// stagePrompt opens a session that takes over at a later stage.
func stagePrompt(o store.GafferOutcome, item store.GafferOutcomeSession, n int) string {
	stage := item.Stages[n]
	text := item.Prompt
	if n > 0 {
		text = "You are taking over one stage of work already under way in this folder and branch.\n\nThe work: " + item.Prompt
		for i := n - 1; i >= 0; i-- {
			if item.Stages[i].Summary != "" {
				text += "\n\nHandover from the " + item.Stages[i].Kind + " stage: " + item.Stages[i].Summary
				break
			}
		}
	}
	text += "\n\nYour stage: " + stage.Kind + ". " + stageWork[stage.Kind]
	if stage.Kind != "merge" {
		text += " Do not merge."
	}
	return clipPrompt(text)
}

func clipPrompt(text string) string {
	if len(text) <= MaxOutcomePromptBytes {
		return text
	}
	cut := MaxOutcomePromptBytes - len("…")
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

const (
	outcomeLineBytes  = 600
	outcomeStartWait  = 90 * time.Second
	outcomeFollowWait = 60 * time.Second
)

var outcomeFollowEvery = 2 * time.Second

func clipBytes(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= limit {
		return text
	}
	cut := limit - len("…")
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return strings.TrimSpace(text[:cut]) + "…"
}

// OutcomeGoal is the saved goal of one session of an outcome. Its first line
// names the outcome and it always ends "Do not merge."
func OutcomeGoal(objective, title string) string {
	return "Outcome: " + clipBytes(objective, outcomeLineBytes) + "\nThis session’s part: " + clipBytes(title, 240) + "\nKeep this session moving on its part of the outcome and nothing beyond it. Make sure a pull request is raised and its description is accurate, fix failing checks and resolve straightforward conflicts. Bring scope changes, decisions and anything that belongs to another session back to me. Do not merge."
}

func (s *Service) outcomesReady() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.goalRecoveryErr != nil || s.goalCancel == nil || s.starter == nil || s.worker == nil || (s.workerEnabled != nil && !s.workerEnabled()) {
		return ErrUnavailable
	}
	return nil
}

// CreateOutcome saves the plan the person approved and wakes the controller
// to start it. It starts nothing itself.
func (s *Service) CreateOutcome(uid int64, in OutcomeInput) (store.GafferOutcome, error) {
	var o store.GafferOutcome
	if !s.Available() {
		return o, ErrUnavailable
	}
	if _, err := s.chatPrincipal(uid); err != nil {
		return o, err
	}
	if err := s.outcomesReady(); err != nil {
		return o, err
	}
	if !in.Confirmed {
		return o, invalid("outcome", "Review the plan and approve it before any session starts.")
	}
	objective := strings.TrimSpace(in.Objective)
	if objective == "" || len(objective) > MaxOutcomeObjectiveBytes || !store.ValidWorkerText(objective) {
		return o, invalid("objective", "Write the objective in up to 4,000 UTF-8 bytes of plain text.")
	}
	if !terminalproto.ReviewedChatTool(in.Tool) {
		return o, invalid("tool", "Choose an agent that can run a Gaffer chat.")
	}
	model := strings.TrimSpace(in.Model)
	if model != "" && !terminalproto.ValidModel(model) {
		return o, invalid("model", "Choose a model name this agent accepts, or leave it to the agent's default.")
	}
	if in.Trust && in.Tool != "opencode" && in.Tool != "antigravity" && !outcomeWantsTrust(in) {
		return o, invalid("trust", "Only OpenCode and Google Antigravity ask you to trust a folder's plugins.")
	}
	if in.MaxReviews < 1 || in.MaxReviews > 48 || in.Hours < 1 || in.Hours > 168 {
		return o, invalid("goal", "Choose 1 to 48 reviews over 1 to 168 hours for each session.")
	}
	if len(in.Sessions) < 1 || len(in.Sessions) > store.MaxGafferOutcomeSessions {
		return o, invalid("sessions", "An outcome starts between 1 and 5 sessions.")
	}
	project, err := s.store.FolderProjectFor(uid, in.ProjectID)
	if err != nil {
		return o, invalid("projectId", "Choose a project on this account.")
	}
	var provider store.GafferProvider
	rows, err := s.store.GafferProvidersFor(uid)
	if err != nil {
		return o, err
	}
	for _, row := range rows {
		if row.Default && row.Enabled {
			provider = row
		}
	}
	if provider.ID == "" {
		return o, invalid("provider", "Choose an enabled default Gaffer provider before giving Gaffer an outcome.")
	}
	id, err := newChatID()
	if err != nil {
		return o, err
	}
	seen := map[string]bool{}
	staged, trustless := 0, 0
	sessions := make([]store.GafferOutcomeSession, 0, len(in.Sessions))
	for i, item := range in.Sessions {
		title, prompt := strings.TrimSpace(item.Title), strings.TrimSpace(item.Prompt)
		if !terminalproto.ValidTitle(title) || seen[title] {
			return o, invalid("sessions", "Give each session its own title of 1 to 80 plain characters.")
		}
		if prompt == "" || len(prompt) > MaxOutcomePromptBytes || !terminalproto.ValidPrompt(prompt) {
			return o, invalid("sessions", "Write a prompt of at most 8,000 bytes without control characters for “"+title+"”.")
		}
		seen[title] = true
		request, err := newChatID()
		if err != nil {
			return o, err
		}
		piece := store.GafferOutcomeSession{Key: string(rune('a' + i)), Title: title, Prompt: prompt, RequestID: request, Note: "Waiting its turn.", AutoMerge: in.AutoMerge}
		if item.AutoMerge != nil {
			piece.AutoMerge = *item.AutoMerge
		}
		last := -1
		for _, stage := range item.Stages {
			at := stageIndex(stage.Kind)
			if at <= last {
				return o, invalid("sessions", "Keep the stages of “"+title+"” in pipeline order, each at most once.")
			}
			last = at
			tool, stageModel := stage.Tool, strings.TrimSpace(stage.Model)
			if tool == "" {
				tool, stageModel = in.Tool, model
			}
			if !terminalproto.ReviewedChatTool(tool) || stageModel != "" && !terminalproto.ValidModel(stageModel) {
				return o, invalid("sessions", "Choose an agent that can run a Gaffer chat, and a model name it accepts, for every stage of “"+title+"”.")
			}
			if in.Trust && tool != "opencode" && tool != "antigravity" {
				trustless++
			}
			id, err := newChatID()
			if err != nil {
				return o, err
			}
			piece.Stages = append(piece.Stages, store.GafferOutcomeStage{Kind: stage.Kind, Tool: tool, Model: stageModel, RequestID: id})
		}
		if len(piece.Stages) > 0 {
			piece.Stages[0].RequestID = request
		}
		staged += len(piece.Stages)
		sessions = append(sessions, piece)
	}
	o = store.GafferOutcome{ID: id, Objective: objective, AgentID: project.Host, ProjectID: project.ID, ProjectName: project.Name, Dir: project.Dir, Tool: in.Tool, Model: model, Trust: in.Trust, ProviderID: provider.ID, MaxReviews: in.MaxReviews, Hours: in.Hours, Sessions: sessions}
	o, err = s.store.CreateGafferOutcomeFor(uid, o)
	if err != nil {
		return o, err
	}
	s.wakeOutcomes()
	return o, nil
}

func outcomeWantsTrust(in OutcomeInput) bool {
	for _, piece := range in.Sessions {
		for _, stage := range piece.Stages {
			if stage.Tool == "opencode" || stage.Tool == "antigravity" {
				return true
			}
		}
	}
	return false
}

// outcomeView reports what the person can rely on: a session the controller
// will no longer attempt reads as not started.
func outcomeView(o store.GafferOutcome) store.GafferOutcome {
	if o.State == "starting" {
		return o
	}
	for i := range o.Sessions {
		item := &o.Sessions[i]
		switch {
		case item.State == "advancing":
			item.State, item.Note = "unfollowed", "Stopped before its next stage. Its session keeps running; look after it from the floor."
		case (item.State == "waiting" || item.State == "starting") && item.SessionID == "":
			item.State, item.Note = "cancelled", "Not started: this outcome was stopped first."
		case item.State == "starting":
			item.State, item.Note = "unfollowed", "Started, but not followed: this outcome was stopped first. Look after it from the floor."
		}
	}
	return o
}

func (s *Service) Outcomes(uid int64) ([]store.GafferOutcome, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return nil, err
	}
	rows, err := s.store.GafferOutcomesFor(uid)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i] = outcomeView(rows[i])
	}
	return rows, nil
}

func (s *Service) Outcome(uid int64, id string) (store.GafferOutcome, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferOutcome{}, err
	}
	o, err := s.store.GafferOutcomeFor(uid, id)
	return outcomeView(o), err
}

// StopOutcome ends the starting of sessions not yet attempted. Sessions
// already started keep running and keep their goals.
func (s *Service) StopOutcome(uid int64, id string, revision int64) (store.GafferOutcome, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferOutcome{}, err
	}
	o, err := s.store.UpdateGafferOutcomeFor(uid, id, revision, func(o *store.GafferOutcome) bool {
		if o.State != "starting" {
			return false
		}
		o.State, o.Note = "stopped", "You stopped this outcome. Sessions already started keep running."
		return true
	})
	return outcomeView(o), err
}

func (s *Service) wakeOutcomes() {
	if s.outcomeWake != nil {
		select {
		case s.outcomeWake <- struct{}{}:
		default:
		}
	}
}

// startOutcomeSupervisor runs beside the goal supervisor, under its context,
// and is joined by Close. The caller holds s.mu.
func (s *Service) startOutcomeSupervisor(ctx context.Context) {
	s.outcomeWake = make(chan struct{}, 1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			s.advanceOutcomes(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-s.outcomeWake:
			}
		}
	}()
}

func (s *Service) advanceOutcomes(ctx context.Context) {
	// A fenced controller starts nothing; the outcomes wait for the lift.
	if s.store.Fenced() {
		return
	}
	rows, err := s.store.StartingGafferOutcomes()
	// The controller's settings are read only when there is an outcome to
	// start, as the goal supervisor reads them only for a due goal.
	if err != nil || len(rows) == 0 || s.outcomesReady() != nil {
		return
	}
	for _, o := range rows {
		if ctx.Err() != nil {
			return
		}
		s.runOutcome(ctx, o.OwnerID, o.ID)
	}
}

// outcomeStep applies change to the current record, retrying when the person
// changed it in between. It reports the record after the change.
func (s *Service) outcomeStep(uid int64, id string, change func(*store.GafferOutcome) bool) (store.GafferOutcome, error) {
	for attempt := 0; ; attempt++ {
		o, err := s.store.GafferOutcomeFor(uid, id)
		if err != nil {
			return o, err
		}
		o, err = s.store.UpdateGafferOutcomeFor(uid, id, o.Revision, change)
		if !errors.Is(err, store.ErrGafferOutcomeBusy) || attempt >= 5 {
			return o, err
		}
	}
}

func setOutcomeSession(key, state, note string, more func(*store.GafferOutcomeSession)) func(*store.GafferOutcome) bool {
	return func(o *store.GafferOutcome) bool {
		for i := range o.Sessions {
			if o.Sessions[i].Key == key {
				o.Sessions[i].State, o.Sessions[i].Note, o.Sessions[i].UpdatedAt = state, note, time.Now().UnixMilli()
				if more != nil {
					more(&o.Sessions[i])
				}
				return true
			}
		}
		return false
	}
}

// runOutcome does the controller's part for an outcome, one step at a
// time: start a session, put it in Gaffer's care, or move a pipeline to its
// next stage. It returns early, leaving the record resumable, when the
// controller is shutting down or the person stopped the outcome.
func (s *Service) runOutcome(ctx context.Context, uid int64, id string) {
	for {
		o, err := s.store.GafferOutcomeFor(uid, id)
		if err != nil || o.State != "starting" || ctx.Err() != nil {
			return
		}
		var item *store.GafferOutcomeSession
		for i := range o.Sessions {
			if st := o.Sessions[i].State; st == "waiting" || st == "starting" || st == "advancing" {
				item = &o.Sessions[i]
				break
			}
		}
		if item == nil {
			_, _ = s.outcomeStep(uid, id, func(o *store.GafferOutcome) bool {
				if o.State != "starting" {
					return false
				}
				for _, piece := range o.Sessions {
					if piece.State == "waiting" || piece.State == "starting" || piece.State == "advancing" {
						return false
					}
				}
				o.State, o.Note = "settled", "The controller has nothing to start. Gaffer follows the sessions in hand."
				return true
			})
			return
		}
		proceed := true
		if item.State == "advancing" {
			proceed = s.advancePipeline(ctx, o, *item)
		} else {
			proceed = s.runOutcomeSession(ctx, o, *item)
		}
		if !proceed {
			return
		}
	}
}

// pipelineStage reports whether this conversation's goal is the stage in hand
// of a pipeline. It reads the store only.
func (s *Service) pipelineStage(uid int64, conversation string) bool {
	rows, err := s.store.GafferOutcomesFor(uid)
	if err != nil {
		return false
	}
	for _, o := range rows {
		for _, piece := range o.Sessions {
			if piece.State == "following" && len(piece.Stages) > 0 && piece.ConversationID == conversation {
				return true
			}
		}
	}
	return false
}

// stageHandedOver records that Gaffer's review found the stage in hand
// finished, closes that stage's goal and asks the controller for the next
// stage. Gaffer's model chose none of what follows.
func (s *Service) stageHandedOver(uid int64, conversation, summary string) {
	rows, err := s.store.GafferOutcomesFor(uid)
	if err != nil {
		return
	}
	for _, o := range rows {
		for _, piece := range o.Sessions {
			if piece.State != "following" || len(piece.Stages) == 0 || piece.ConversationID != conversation {
				continue
			}
			key := piece.Key
			_, err = s.outcomeStep(uid, o.ID, func(o *store.GafferOutcome) bool {
				for i := range o.Sessions {
					row := &o.Sessions[i]
					if row.Key != key || row.State != "following" || row.ConversationID != conversation || row.Stage >= len(row.Stages) {
						continue
					}
					row.Stages[row.Stage].State, row.Stages[row.Stage].Summary, row.Stages[row.Stage].EndedAt = "done", clipBytes(summary, 1800), time.Now().UnixMilli()
					row.State, row.Note, row.UpdatedAt = "advancing", "The "+row.Stages[row.Stage].Kind+" stage is done. Starting the next.", time.Now().UnixMilli()
					if o.State != "stopped" {
						o.State = "starting"
					}
					return true
				}
				return false
			})
			if err != nil {
				return
			}
			// The finished stage's goal is closed without calling it the
			// person's completion: only they mark a goal complete.
			if g, err := s.store.GafferGoalFor(uid, conversation); err == nil && g.State == "active" && g.ActiveTurnID == "" {
				_ = s.store.ObserveGafferGoal(g, "stopped", "This pipeline stage is done and was handed over.", 0)
			}
			s.wakeOutcomes()
			return
		}
	}
}

// ApproveMerge is the person saying a pipeline that is ready may merge.
func (s *Service) ApproveMerge(uid int64, id, key string, revision int64) (store.GafferOutcome, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferOutcome{}, err
	}
	found := false
	o, err := s.store.UpdateGafferOutcomeFor(uid, id, revision, func(o *store.GafferOutcome) bool {
		for i := range o.Sessions {
			row := &o.Sessions[i]
			if row.Key == key && row.State == "ready_to_merge" {
				row.MergeApproved, row.State, row.Note, row.UpdatedAt = true, "advancing", "You approved the merge. Starting the merge stage.", time.Now().UnixMilli()
				o.State = "starting"
				found = true
				return true
			}
		}
		return false
	})
	if err != nil {
		return outcomeView(o), err
	}
	if !found {
		return outcomeView(o), invalid("merge", "That pipeline is not waiting for a merge decision.")
	}
	s.wakeOutcomes()
	return outcomeView(o), nil
}

// advancePipeline moves a pipeline to the stage after the one just handed
// over. The next stage keeps the session when the same agent and model do it,
// and otherwise gets a session of its own in the same working folder.
func (s *Service) advancePipeline(ctx context.Context, o store.GafferOutcome, item store.GafferOutcomeSession) bool {
	uid := o.OwnerID
	set := func(state, note string, more func(*store.GafferOutcomeSession)) bool {
		_, err := s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, state, note, more))
		return err == nil
	}
	next := item.Stage + 1
	if item.Stage < len(item.Stages) && item.Stages[item.Stage].State != "done" {
		// The merge stage was approved while its predecessor's handover was already recorded.
		next = item.Stage
	}
	if next >= len(item.Stages) {
		return set("done", "Every stage is done. Check the result; only you mark the work complete.", nil)
	}
	stage := item.Stages[next]
	if stage.Kind == "merge" && !item.AutoMerge && !item.MergeApproved {
		return set("ready_to_merge", "Ready to merge. Nothing merges until you approve it.", func(row *store.GafferOutcomeSession) { row.Stage = next })
	}
	if _, err := s.chatPrincipal(uid); err != nil {
		return set("unfollowed", "Stopped before the "+stage.Kind+" stage: this account can no longer start sessions.", nil)
	}
	goal := StageGoal(o.Objective, item.Title, stage.Kind)
	prev := stage
	if next > 0 {
		prev = item.Stages[next-1]
	}
	if next > 0 && prev.State == "done" && prev.Tool == stage.Tool && prev.Model == stage.Model && item.ConversationID != "" {
		// Same agent and model: the session carries on, under a fresh goal and budget for the new stage.
		v, err := s.Goal(uid, item.ConversationID)
		if err == nil && v.SetupAllowed {
			in := GoalSetupInput{GoalInput: GoalInput{GoalPins: v.Pins, Text: goal, MaxReviews: o.MaxReviews, Hours: o.Hours}, ConversationRevision: v.ConversationRevision, Confirmed: true, Action: "start"}
			if v.Goal != nil {
				in.Revision = v.Goal.Revision
			}
			if _, err = s.SetupGoal(uid, item.ConversationID, in); err == nil {
				return set("following", "The "+stage.Kind+" stage is under way in the same session.", func(row *store.GafferOutcomeSession) {
					row.Stage = next
					row.Stages[next].State, row.Stages[next].SessionID, row.Stages[next].ConversationID, row.Stages[next].StartedAt = "running", row.SessionID, row.ConversationID, time.Now().UnixMilli()
				})
			}
			if errors.Is(err, ErrUnavailable) && ctx.Err() != nil {
				return false
			}
		}
		if ctx.Err() != nil {
			return false
		}
		// The session could not take the next stage (it ended, say): give the stage its own.
	}
	if !set("starting", "Starting the "+stage.Kind+" stage…", func(row *store.GafferOutcomeSession) {
		row.Stage = next
		row.SessionID, row.ConversationID = row.Stages[next].SessionID, ""
	}) {
		return false
	}
	fresh, err := s.store.GafferOutcomeFor(uid, o.ID)
	if err != nil {
		return false
	}
	for _, row := range fresh.Sessions {
		if row.Key == item.Key {
			return s.runOutcomeSession(ctx, fresh, row)
		}
	}
	return false
}

// runOutcomeSession starts the session for the stage in hand (or the whole
// piece, without stages) and puts it in Gaffer's care. It reports false when
// the controller is stopping and the session must be picked up again later.
func (s *Service) runOutcomeSession(ctx context.Context, o store.GafferOutcome, item store.GafferOutcomeSession) bool {
	uid := o.OwnerID
	fail := func(state, note string) bool {
		_, err := s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, state, note, nil))
		return err == nil
	}
	launch := OutcomeLaunch{AgentID: o.AgentID, RequestID: item.RequestID, Title: item.Title, Tool: o.Tool, Model: o.Model, Trust: o.Trust, Dir: o.Dir, ProjectID: o.ProjectID, Prompt: item.Prompt}
	goal := OutcomeGoal(o.Objective, item.Title)
	staged := len(item.Stages) > 0 && item.Stage < len(item.Stages)
	if staged {
		stage := item.Stages[item.Stage]
		launch.RequestID, launch.Tool, launch.Model, launch.Prompt = stage.RequestID, stage.Tool, stage.Model, stagePrompt(o, item, item.Stage)
		launch.Trust = o.Trust && (stage.Tool == "opencode" || stage.Tool == "antigravity")
		launch.Title = clipBytes(item.Title, 60) + " · " + stage.Kind
		goal = StageGoal(o.Objective, item.Title, stage.Kind)
		if item.Stage > 0 && item.WorkDir != "" {
			// A later stage works where the first one did. It is not a new
			// project launch, so it gets no worktree of its own.
			launch.Dir, launch.ProjectID = item.WorkDir, ""
		}
	}
	// The account is read again before every session: a person who lost
	// write access after approving starts nothing more.
	if _, err := s.chatPrincipal(uid); err != nil {
		return fail("failed", "Not started: this account can no longer start sessions.")
	}
	sessionID := item.SessionID
	if sessionID == "" {
		if _, err := s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, "starting", "Starting the session…", nil)); err != nil {
			return false
		}
		s.mu.Lock()
		starter := s.starter
		s.mu.Unlock()
		if starter == nil {
			return false
		}
		startCtx, cancel := context.WithTimeout(ctx, outcomeStartWait)
		started, workDir, err := starter.StartOutcomeSession(startCtx, uid, launch)
		cancel()
		if ctx.Err() != nil {
			// The same request ID is sent again after a restart, so a launch
			// whose answer was lost is adopted, not repeated.
			return false
		}
		if err != nil {
			if staged && item.Stage > 0 {
				return fail("unfollowed", "The "+item.Stages[item.Stage].Kind+" stage was not started: "+clipBytes(err.Error(), 400))
			}
			return fail("failed", "Not started: "+clipBytes(err.Error(), 400))
		}
		sessionID = started
		if _, err = s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, "starting", "Started. Putting it in Gaffer’s care…", func(row *store.GafferOutcomeSession) {
			row.SessionID = sessionID
			if row.WorkDir == "" {
				row.WorkDir = workDir
			}
			if row.Stage < len(row.Stages) {
				row.Stages[row.Stage].SessionID, row.Stages[row.Stage].State, row.Stages[row.Stage].StartedAt = sessionID, "running", time.Now().UnixMilli()
			}
		})); err != nil {
			return false
		}
	}
	unfollowed := func(reason string) bool {
		return fail("unfollowed", "Started, but not followed: "+clipBytes(reason, 400)+" Look after it from the floor.")
	}
	c, err := s.CreateConversation(uid, ConversationInput{SessionID: sessionID, ProviderID: o.ProviderID})
	if err != nil {
		if errors.Is(err, ErrUnavailable) && ctx.Err() != nil {
			return false
		}
		return unfollowed(outcomeReason(err))
	}
	cid := c.Conversation.ID
	deadline := time.Now().Add(outcomeFollowWait)
	for {
		v, err := s.Goal(uid, cid)
		if err != nil {
			return unfollowed(outcomeReason(err))
		}
		if v.Goal != nil {
			return unfollowed("This session already has a goal.")
		}
		if v.SetupAllowed {
			_, err = s.SetupGoal(uid, cid, GoalSetupInput{GoalInput: GoalInput{GoalPins: v.Pins, Text: goal, MaxReviews: o.MaxReviews, Hours: o.Hours}, ConversationRevision: v.ConversationRevision, Confirmed: true, Action: "start"})
			if err == nil {
				note := "Started and followed. Gaffer waits for its first progress."
				if staged {
					note = "The " + item.Stages[item.Stage].Kind + " stage is under way."
				}
				_, err = s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, "following", note, func(row *store.GafferOutcomeSession) {
					row.ConversationID = cid
					if row.Stage < len(row.Stages) {
						row.Stages[row.Stage].ConversationID = cid
					}
				}))
				return err == nil
			}
			if errors.Is(err, ErrUnavailable) && ctx.Err() != nil {
				return false
			}
			if !errors.Is(err, store.ErrGafferConflict) {
				return unfollowed(outcomeReason(err))
			}
			// A revision moved between the read and the setup; read it again.
		}
		if time.Now().After(deadline) {
			// The goal view's own reason describes missing grants, which this
			// setup would have granted; the truth is that the chat never came up.
			return unfollowed("Its chat was not ready for follow-up in time.")
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(outcomeFollowEvery):
		}
	}
}

func outcomeReason(err error) string {
	var bad *Invalid
	switch {
	case errors.As(err, &bad):
		return bad.Message
	case errors.Is(err, store.ErrNotFound):
		return "Its chat is not available to Gaffer."
	case errors.Is(err, ErrUnavailable):
		return "Gaffer is not available on this controller just now."
	}
	return err.Error()
}
