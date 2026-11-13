package gaffer

import (
	"context"
	"errors"
	"strconv"
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
		if seq > 0 && s.pipelineStage(uid, cid) {
			// A pipeline stage is resumed to carry the pipeline on: Gaffer
			// reviews the work already there, which may be the finished stage,
			// and does not wait for progress an idle agent will never make.
			bind.LastSeq = seq - 1
		}
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
	s.startOutcomeSupervisor(ctx)
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

// goalNoStepNote is what a goal says when Gaffer's review ended without
// instructing the working model, asking the person anything or handing over.
const goalNoStepNote = "Gaffer reviewed this and chose no next step, so it has stopped here. Nothing is being asked of you in particular: tell the agent what to do next, resume the goal to have Gaffer look again, or move the work on if it is done."

const (
	workerWaitingNote = "The working model needs your input or approval. Gaffer cannot answer for you, and carries on once you have."
	heldGoalRecheck   = 15 * time.Second
)

// How long a goal may look active while nothing can happen without the
// person. Variables so tests can shorten them.
var (
	// instructionDeliveryWait is how long Gaffer's instruction may sit
	// undelivered while the working model is not busy.
	instructionDeliveryWait = 10 * time.Minute
	// idleWorkerWait is how long the working model may sit idle after Gaffer's
	// last review, with nothing queued for it, before the goal needs the person.
	idleWorkerWait = 30 * time.Minute
)

// workerControls asks the working model's chat what it is doing. ok is false
// when it could not be asked.
func (s *Service) workerControls(ctx context.Context, g store.GafferGoal) (terminalproto.ACPControls, bool) {
	if s.worker == nil || s.workerEnabled != nil && !s.workerEnabled() {
		return terminalproto.ACPControls{}, false
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	status, err := s.worker.ChatControl(probe, g.OwnerID, g.SessionID, terminalproto.ACPControl{Action: "get"})
	return status.Controls, err == nil
}

// minutes is a wait in words for a goal's note.
func minutes(d time.Duration) string {
	n := int(d.Round(time.Minute) / time.Minute)
	if n <= 1 {
		return "a minute"
	}
	return strconv.Itoa(n) + " minutes"
}

// reviewHeldGoal looks again at a goal that needs the person only because the
// working model was waiting for them. No model request is made. Once the wait
// is over the goal is active again and reviewed like any other; anything else
// that has gone wrong meanwhile leaves it needing the person.
func (s *Service) reviewHeldGoal(ctx context.Context, g store.GafferGoal) {
	hold := func(note string) {
		if ctx.Err() == nil {
			_ = s.store.HoldGafferGoalForWorker(g, note, time.Now().Add(heldGoalRecheck).UnixMilli())
		}
	}
	// The hold ends without the goal carrying on: it now simply needs the person.
	end := func(note string) {
		if s.store.EndGafferGoalHold(g, note) == nil {
			s.auditGoal(g, "gaffer-goal-needs-you", note)
		}
	}
	if g.ExpiresAt <= time.Now().UnixMilli() {
		end("The follow-up time limit was reached while the working model waited for you. Review or start a new budget.")
		return
	}
	// Access is checked as for any goal on the first review after the release.
	row, err := s.store.TerminalSessionFor(g.OwnerID, g.SessionID)
	if err != nil || row.State != terminalproto.Running {
		end("The working session has ended or is unavailable. Review its outcome before choosing further work.")
		return
	}
	if s.worker == nil || s.workerEnabled != nil && !s.workerEnabled() {
		hold(g.Note)
		return
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	status, err := s.worker.ChatControl(probe, g.OwnerID, g.SessionID, terminalproto.ACPControl{Action: "get"})
	cancel()
	if ctx.Err() != nil {
		return
	}
	if err != nil || status.Controls.Waiting {
		hold(g.Note)
		return
	}
	if err = s.store.ReleaseGafferGoalHold(g, "The working model is no longer waiting for you. Gaffer is following again."); err != nil {
		// No room among the active goals yet, or the goal changed: look again later.
		hold(g.Note)
		return
	}
	s.auditGoal(g, "gaffer-goal-released", "The working model is no longer waiting for the person. Gaffer is following again.")
	s.wakeGoals()
}

func (s *Service) reviewGoal(ctx context.Context, g store.GafferGoal) {
	if g.ActiveTurnID != "" {
		return
	}
	observe := func(state, note string, delay time.Duration) {
		if ctx.Err() != nil {
			return
		}
		if s.store.ObserveGafferGoal(g, state, note, time.Now().Add(delay).UnixMilli()) == nil {
			// Only a goal that stops being followed is recorded, not each look at an active one.
			if action := goalStateAction(state); action != "" {
				s.auditGoal(g, action, note)
			}
		}
	}
	// Only the person can answer a waiting working model. The goal needs them
	// until they have, and then carries on by itself.
	hold := func() {
		if s.store.HoldGafferGoalForWorker(g, workerWaitingNote, time.Now().Add(heldGoalRecheck).UnixMilli()) == nil {
			s.auditGoal(g, "gaffer-goal-held", workerWaitingNote)
		}
	}
	if g.State == "needs_user" {
		s.reviewHeldGoal(ctx, g)
		return
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
			// A queued instruction waits for the working model's turn to end.
			// One that is still undelivered long after, while the model is not
			// busy, will not be delivered by waiting: the person has to look.
			if waited := time.Since(time.UnixMilli(m.CreatedAt)); waited >= instructionDeliveryWait && s.worker != nil && (s.workerEnabled == nil || s.workerEnabled()) {
				controls, ok := s.workerControls(ctx, g)
				if ctx.Err() != nil {
					return
				}
				switch {
				case ok && controls.Waiting:
					hold()
					return
				case ok && (controls.Working || controls.Configuring || controls.Compacting):
				case ok:
					observe("needs_user", "Gaffer’s instruction has waited "+minutes(waited)+" without reaching the working model, which is not busy. Check its session; the instruction is still queued. Resume the goal to try again.", time.Minute)
					return
				default:
					observe("needs_user", "Gaffer’s instruction has waited "+minutes(waited)+" undelivered and the working session could not be reached. Check the machine and the session, then resume the goal.", time.Minute)
					return
				}
			}
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
		// Gaffer has reviewed everything there is. If the working model has
		// then sat idle with nothing queued for it, no progress is coming:
		// most often it asked the person something. Say so rather than look
		// active until the goal expires.
		since := max(g.LastReviewedAt, g.ResumedAt)
		if g.LastReviewedAt > 0 && time.Since(time.UnixMilli(since)) >= idleWorkerWait {
			controls, ok := s.workerControls(ctx, g)
			if ctx.Err() != nil {
				return
			}
			if ok && controls.Waiting {
				hold()
				return
			}
			if ok && !controls.Working && !controls.Configuring && !controls.Compacting {
				observe("needs_user", "The working model has been idle for "+minutes(time.Since(time.UnixMilli(since)))+" since Gaffer last looked, and nothing is queued for it. It may be waiting for your answer: read its last message, then resume the goal or move the work on.", time.Minute)
				return
			}
		}
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
		hold()
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
