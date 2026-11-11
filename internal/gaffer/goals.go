package gaffer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type GoalPins struct {
	ProviderID       string `json:"providerId"`
	ProviderRevision int64  `json:"providerRevision"`
	Model            string `json:"model"`
	ContextRevision  int64  `json:"contextRevision"`
	WorkerRevision   int64  `json:"workerRevision"`
}
type GoalInput struct {
	GoalPins
	Revision   int64  `json:"revision"`
	Text       string `json:"text"`
	MaxReviews int    `json:"maxReviews"`
	Hours      int    `json:"hours"`
}
type GoalView struct {
	Goal                   *store.GafferGoal `json:"goal"`
	Pins                   GoalPins          `json:"pins"`
	Allowed                bool              `json:"allowed"`
	Reason                 string            `json:"reason,omitempty"`
	SetupAllowed           bool              `json:"setupAllowed"`
	ProviderName           string            `json:"providerName"`
	RequiresProviderAccess bool              `json:"requiresProviderAccess"`
	ConversationRevision   int64             `json:"conversationRevision"`
}

func (s *Service) Goal(uid int64, cid string) (GoalView, error) {
	c, err := s.conversation(uid, cid)
	if err != nil {
		return GoalView{}, err
	}
	v := GoalView{ConversationRevision: c.Revision}
	if c.SessionID == "" {
		v.Reason = "Open a structured session to ask Gaffer to look after it."
		return v, nil
	}
	w, err := s.WorkerSettings(uid, "gaffer", cid, c.SessionID)
	if err != nil {
		return v, err
	}
	sharing, err := s.ContextSettings(uid, "gaffer", cid, c.SessionID)
	if err != nil {
		return v, err
	}
	v.Pins = GoalPins{ProviderID: w.ProviderID, ProviderRevision: w.ProviderRevision, Model: w.Model, ContextRevision: sharing.Grant.Revision, WorkerRevision: w.Grant.Revision}
	provider, providerErr := s.store.GafferProviderFor(uid, c.ProviderID)
	row, rowErr := s.store.TerminalSessionFor(uid, c.SessionID)
	principal, principalErr := s.chatPrincipal(uid)
	if providerErr == nil && rowErr == nil && principalErr == nil {
		caps, capsErr := s.store.RemoteCapabilitiesFor(uid, row.AgentID)
		_, available := caps.Capabilities.ChatAgent(row.Tool)
		v.ProviderName = provider.Name
		v.RequiresProviderAccess = !provider.AllowSessions || !provider.AllowCode
		v.SetupAllowed = s.Available() && s.worker != nil && (s.workerEnabled == nil || s.workerEnabled()) && provider.Enabled && (!provider.LocalOnly || principal.CanUseLocal) && row.State == terminalproto.Running && available && capsErr == nil && caps.Capabilities.ACPLiveControls
	}
	v.Allowed = w.Allowed && w.Grant.Enabled && !w.NeedsReview && !sharing.NeedsReview && sharing.Grant.History && sharing.Grant.Text && sharing.Allowed.Contains(sharing.Grant.ContextFlags)
	if !v.Allowed {
		v.Reason = "Review Worker messages and Context sharing for this session. Follow-up needs session event history and message text, under the chosen provider's permissions."
	}
	g, err := s.store.GafferGoalFor(uid, cid)
	if err == nil {
		v.Goal = &g
	} else if !errors.Is(err, store.ErrNotFound) {
		return v, err
	}
	return v, nil
}

type GoalSetupInput struct {
	GoalInput
	ConversationRevision int64  `json:"conversationRevision"`
	Confirmed            bool   `json:"confirmed"`
	Action               string `json:"action"`
}

func (s *Service) SetupGoal(uid int64, cid string, in GoalSetupInput) (GoalView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.goalRecoveryErr != nil {
		return GoalView{}, ErrUnavailable
	}
	v, err := s.Goal(uid, cid)
	if err != nil {
		return v, err
	}
	if !in.Confirmed || !v.SetupAllowed {
		return v, invalid("goal", "Review the session, model, shared information and follow-up limits before continuing.")
	}
	if in.GoalPins != v.Pins || in.ConversationRevision != v.ConversationRevision {
		return v, store.ErrGafferConflict
	}
	if in.Action != "start" && in.Action != "resume" {
		return v, invalid("goal", "Choose start or resume for this session.")
	}
	if in.Action == "start" && (in.Hours < 1 || in.Hours > 168) {
		return v, invalid("goal", "Choose between 1 and 168 hours of follow-up.")
	}
	id, err := newChatID()
	if err != nil {
		return v, err
	}
	g := store.GafferGoal{ID: id, ConversationID: cid, Text: in.Text, ProviderID: v.Pins.ProviderID, ProviderRevision: v.Pins.ProviderRevision, Model: v.Pins.Model, ContextRevision: v.Pins.ContextRevision, WorkerRevision: v.Pins.WorkerRevision, MaxReviews: in.MaxReviews, ExpiresAt: time.Now().Add(time.Duration(in.Hours) * time.Hour).UnixMilli(), LastSeq: -1}
	if _, err = s.store.SetupGafferGoal(uid, in.ConversationRevision, in.Revision, g, in.Action == "resume"); err != nil {
		return v, err
	}
	for _, job := range s.chatJobs {
		if job.uid == uid && (job.conversation == cid || v.RequiresProviderAccess && job.provider == g.ProviderID) {
			job.cancel(errGafferPolicy)
		}
	}
	s.wakeGoals()
	return s.Goal(uid, cid)
}
func (s *Service) SaveGoal(uid int64, cid string, in GoalInput) (GoalView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.goalRecoveryErr != nil {
		return GoalView{}, ErrUnavailable
	}
	v, err := s.Goal(uid, cid)
	if err != nil {
		return v, err
	}
	if !v.Allowed || in.GoalPins != v.Pins {
		return v, invalid("goal", "Review the current model, sharing and worker permission before starting follow-up.")
	}
	if in.Hours < 1 || in.Hours > 168 {
		return v, invalid("goal", "Choose between 1 and 168 hours of follow-up.")
	}
	c, err := s.conversation(uid, cid)
	if err != nil {
		return v, err
	}
	id, err := newChatID()
	if err != nil {
		return v, err
	}
	g := store.GafferGoal{ID: id, ConversationID: cid, SessionID: c.SessionID, Text: in.Text, ProviderID: v.Pins.ProviderID, ProviderRevision: v.Pins.ProviderRevision, Model: v.Pins.Model, ContextRevision: v.Pins.ContextRevision, WorkerRevision: v.Pins.WorkerRevision, MaxReviews: in.MaxReviews, ExpiresAt: time.Now().Add(time.Duration(in.Hours) * time.Hour).UnixMilli(), LastSeq: -1}
	if _, err = s.store.SaveGafferGoal(uid, in.Revision, g); err != nil {
		return v, err
	}
	if job := s.chatJobs[cid]; job != nil && job.goalID != "" {
		job.cancel(errGafferPolicy)
	}
	s.wakeGoals()
	return s.Goal(uid, cid)
}
func (s *Service) ControlGoal(uid int64, cid string, revision int64, action string, pins GoalPins, confirmed bool) (GoalView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return GoalView{}, ErrUnavailable
	}
	v, err := s.Goal(uid, cid)
	if err != nil {
		return v, err
	}
	c, err := s.conversation(uid, cid)
	if err != nil {
		return v, err
	}
	bind := store.GafferGoal{SessionID: c.SessionID}
	if action == "resume" {
		if !confirmed || !v.Allowed || pins != v.Pins {
			return v, invalid("goal", "Review the working conversation and current permissions before resuming.")
		}
		seq, err := s.store.GafferWorkerProgress(uid, c.SessionID)
		if err != nil {
			return v, err
		}
		bind.ProviderID = pins.ProviderID
		bind.ProviderRevision = pins.ProviderRevision
		bind.Model = pins.Model
		bind.WorkerRevision = pins.WorkerRevision
		bind.ContextRevision = pins.ContextRevision
		bind.LastSeq = seq
	}
	if _, err = s.store.ControlGafferGoal(uid, cid, revision, action, bind); err != nil {
		return v, err
	}
	if job := s.chatJobs[cid]; job != nil && job.goalID != "" {
		job.cancel(errGafferPolicy)
	}
	s.wakeGoals()
	return s.Goal(uid, cid)
}
func (s *Service) wakeGoals() {
	if s.goalWake != nil {
		select {
		case s.goalWake <- struct{}{}:
		default:
		}
	}
}

// Startup happens after the broker is installed, before requests are accepted.
// No provider call is ever replayed; incomplete reviews require reconciliation.
func (s *Service) StartGoalSupervisor() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.goalCancel != nil {
		return
	}
	s.goalRecoveryErr = s.store.RecoverGafferGoals()
	if s.goalRecoveryErr != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.goalCancel = cancel
	s.goalWake = make(chan struct{}, 1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.reviewDueGoals(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-s.goalWake:
			}
		}
	}()
}
func (s *Service) reviewDueGoals(ctx context.Context) {
	// A fenced controller reviews no goal and sends nothing for one. The goals
	// stay due, and are reviewed once the fence is lifted.
	if s.store.Fenced() {
		return
	}
	goals, err := s.store.DueGafferGoals(time.Now().UnixMilli())
	if err != nil {
		return
	}
	candidates := make(chan store.GafferGoal, len(goals))
	for _, g := range goals {
		candidates <- g
	}
	close(candidates)
	var done sync.WaitGroup
	for i := 0; i < 4; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			for g := range candidates {
				if ctx.Err() != nil {
					return
				}
				s.reviewGoal(ctx, g)
			}
		}()
	}
	done.Wait()
}
func (s *Service) reviewGoal(ctx context.Context, g store.GafferGoal) {
	if g.ActiveTurnID != "" {
		return
	}
	observe := func(state, note string, delay time.Duration) {
		if ctx.Err() != nil {
			return
		}
		_ = s.store.ObserveGafferGoal(g, state, note, time.Now().Add(delay).UnixMilli())
	}
	if g.ExpiresAt <= time.Now().UnixMilli() {
		observe("limit", "The follow-up time limit was reached. Review or start a new budget.", time.Minute)
		return
	}
	if err := s.store.CheckGafferGoal(g.OwnerID, g.ConversationID, g.ID, g.Revision, ""); err != nil {
		observe("needs_user", "Session access, provider or sharing permission changed. Review before resuming.", time.Minute)
		return
	}
	row, err := s.store.TerminalSessionFor(g.OwnerID, g.SessionID)
	if err != nil || row.State != terminalproto.Running {
		observe("needs_user", "The working session has ended or is unavailable. Review its outcome before choosing further work.", time.Minute)
		return
	}
	if g.PendingID != "" {
		m, err := s.WorkerMessageReceipt(g.OwnerID, "gaffer", g.ConversationID, g.SessionID, g.PendingID)
		if err != nil {
			observe("needs_user", "The previous instruction's delivery cannot be confirmed. Check the working conversation.", time.Minute)
			return
		}
		switch m.State {
		case "queued", "sending":
			observe("active", "Waiting for the existing instruction's delivery.", 10*time.Second)
			return
		case "uncertain", "blocked", "removed":
			observe("needs_user", "The previous instruction was blocked, removed or has uncertain delivery. Check the working conversation before resuming.", time.Minute)
			return
		}
	}
	halted, err := s.store.GafferWorkerHalted(g.OwnerID, g.SessionID)
	if err != nil || halted {
		observe("needs_user", "The working model stopped or failed. Check its conversation before resuming follow-up.", time.Minute)
		return
	}
	if g.UsedReviews >= g.MaxReviews {
		observe("limit", "The review budget is used. Review or start a new budget.", time.Minute)
		return
	}
	seq, err := s.store.GafferWorkerProgress(g.OwnerID, g.SessionID)
	if err != nil {
		observe("needs_user", "The session history is unavailable.", time.Minute)
		return
	}
	if seq == 0 || seq <= g.LastSeq {
		observe("active", "Waiting for new worker progress. No model request was made.", 15*time.Second)
		return
	}
	if s.worker == nil || s.workerEnabled != nil && !s.workerEnabled() {
		observe("active", "Waiting for the controller's worker channel.", time.Minute)
		return
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	status, err := s.worker.ChatControl(probe, g.OwnerID, g.SessionID, terminalproto.ACPControl{Action: "get"})
	cancel()
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		observe("active", "The worker is unavailable. Gaffer will resume when its verified channel is available.", time.Minute)
		return
	}
	if status.Controls.Waiting {
		observe("needs_user", "The working model needs your input or approval. Gaffer cannot answer for you.", time.Minute)
		return
	}
	if status.Controls.Working || status.Controls.Configuring || status.Controls.Compacting {
		observe("active", "The working model is busy. Gaffer is waiting for its next turn.", 15*time.Second)
		return
	}
	c, err := s.conversation(g.OwnerID, g.ConversationID)
	if err != nil {
		observe("needs_user", "The selected conversation is unavailable.", time.Minute)
		return
	}
	id, err := newChatID()
	if err != nil {
		return
	}
	_, err = s.startTurn(g.OwnerID, g.ConversationID, TurnInput{ID: id, Revision: c.Revision, Text: "Review new worker progress against my saved goal. Summarise what happened and what remains. If useful, ask the working model for one next step within that goal. Bring approvals and scope changes back to me.\n\nSaved goal:\n" + g.Text, Relay: true}, &g)
	if err != nil {
		if errors.Is(err, ErrBusy) || errors.Is(err, store.ErrGafferChatBusy) || errors.Is(err, store.ErrGafferConflict) {
			observe("active", "Waiting for an available Gaffer review slot.", 15*time.Second)
		} else {
			observe("needs_user", "Gaffer could not start this review. Check the provider, permissions and conversation limits.", time.Minute)
		}
	}
}
