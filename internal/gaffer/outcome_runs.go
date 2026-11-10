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
	StartOutcomeSession(ctx context.Context, uid int64, in OutcomeLaunch) (sessionID string, err error)
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
	if in.Trust && in.Tool != "opencode" && in.Tool != "antigravity" {
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
		sessions = append(sessions, store.GafferOutcomeSession{Key: string(rune('a' + i)), Title: title, Prompt: prompt, RequestID: request, Note: "Waiting its turn."})
	}
	o = store.GafferOutcome{ID: id, Objective: objective, AgentID: project.Host, ProjectID: project.ID, ProjectName: project.Name, Dir: project.Dir, Tool: in.Tool, Model: model, Trust: in.Trust, ProviderID: provider.ID, MaxReviews: in.MaxReviews, Hours: in.Hours, Sessions: sessions}
	o, err = s.store.CreateGafferOutcomeFor(uid, o)
	if err != nil {
		return o, err
	}
	s.wakeOutcomes()
	return o, nil
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
	if s.store.Fenced() || s.outcomesReady() != nil {
		return
	}
	rows, err := s.store.StartingGafferOutcomes()
	if err != nil {
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

// runOutcome starts and follows the outcome's remaining sessions one at a
// time. It returns early, leaving the record resumable, when the controller
// is shutting down or the person stopped the outcome.
func (s *Service) runOutcome(ctx context.Context, uid int64, id string) {
	for {
		o, err := s.store.GafferOutcomeFor(uid, id)
		if err != nil || o.State != "starting" || ctx.Err() != nil {
			return
		}
		var item *store.GafferOutcomeSession
		for i := range o.Sessions {
			if o.Sessions[i].State == "waiting" || o.Sessions[i].State == "starting" {
				item = &o.Sessions[i]
				break
			}
		}
		if item == nil {
			_, _ = s.outcomeStep(uid, id, func(o *store.GafferOutcome) bool {
				if o.State != "starting" {
					return false
				}
				o.State, o.Note = "settled", "Every session has its answer."
				return true
			})
			return
		}
		if !s.runOutcomeSession(ctx, o, *item) {
			return
		}
	}
}

// runOutcomeSession reports false when the controller is stopping and the
// session must be picked up again later.
func (s *Service) runOutcomeSession(ctx context.Context, o store.GafferOutcome, item store.GafferOutcomeSession) bool {
	uid := o.OwnerID
	fail := func(state, note string) bool {
		_, err := s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, state, note, nil))
		return err == nil
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
		started, err := starter.StartOutcomeSession(startCtx, uid, OutcomeLaunch{AgentID: o.AgentID, RequestID: item.RequestID, Title: item.Title, Tool: o.Tool, Model: o.Model, Trust: o.Trust, Dir: o.Dir, ProjectID: o.ProjectID, Prompt: item.Prompt})
		cancel()
		if ctx.Err() != nil {
			// The same request ID is sent again after a restart, so a launch
			// whose answer was lost is adopted, not repeated.
			return false
		}
		if err != nil {
			return fail("failed", "Not started: "+clipBytes(err.Error(), 400))
		}
		sessionID = started
		if _, err = s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, "starting", "Started. Putting it in Gaffer’s care…", func(row *store.GafferOutcomeSession) { row.SessionID = sessionID })); err != nil {
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
			_, err = s.SetupGoal(uid, cid, GoalSetupInput{GoalInput: GoalInput{GoalPins: v.Pins, Text: OutcomeGoal(o.Objective, item.Title), MaxReviews: o.MaxReviews, Hours: o.Hours}, ConversationRevision: v.ConversationRevision, Confirmed: true, Action: "start"})
			if err == nil {
				_, err = s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, "following", "Started and followed. Gaffer waits for its first progress.", func(row *store.GafferOutcomeSession) { row.ConversationID = cid }))
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
