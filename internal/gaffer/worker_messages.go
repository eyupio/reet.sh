package gaffer

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// WorkerMessenger deliberately has no shell, GitHub, approval or model-setting
// operation. It is supplied once before the server exposes its handlers.
type WorkerMessenger interface {
	ChatControl(context.Context, int64, string, terminalproto.ACPControl) (terminalproto.ACPControlResult, error)
	EnqueueChat(int64, string, string, string, []string) error
	ReconcileWorkerQueue(int64, string) error
}

func (s *Service) SetWorker(worker WorkerMessenger, enabled ...func() bool) {
	s.worker = worker
	if len(enabled) > 0 {
		s.workerEnabled = enabled[0]
	}
}

type WorkerSettings struct {
	Grant            store.WorkerGrant `json:"grant"`
	Allowed          bool              `json:"allowed"`
	NeedsReview      bool              `json:"needsReview"`
	Reason           string            `json:"reason,omitempty"`
	ProviderID       string            `json:"providerId,omitempty"`
	ProviderRevision int64             `json:"providerRevision,omitempty"`
	Model            string            `json:"model,omitempty"`
}

func (s *Service) WorkerSettings(uid int64, kind, subject, session string) (WorkerSettings, error) {
	g, err := s.store.WorkerGrantFor(uid, kind, subject, session)
	if err != nil {
		return WorkerSettings{}, err
	}
	v := WorkerSettings{Grant: g, Allowed: s.worker != nil && (s.workerEnabled == nil || s.workerEnabled()) && s.store.CheckWorkerSubject(uid, kind, subject, session, true) == nil}
	if kind == "gaffer" {
		c, err := s.store.GafferConversationFor(uid, subject)
		if err != nil {
			return v, err
		}
		p, err := s.store.GafferProviderFor(uid, c.ProviderID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return v, err
		}
		v.ProviderID = c.ProviderID
		v.ProviderRevision = p.Revision
		v.Model = c.Model
		if v.Model == "" {
			v.Model = p.Model
		}
		v.Allowed = v.Allowed && p.Enabled && p.AllowSessions && s.Available()
		v.NeedsReview = g.Enabled && (g.ProviderID != v.ProviderID || g.ProviderRevision != v.ProviderRevision || g.Model != v.Model || !v.Allowed)
	}
	if !v.Allowed {
		v.Reason = "Enable the controller worker channel and a provider allowing session data. Worker availability is checked at dispatch."
		if kind != "gaffer" {
			v.Reason = "Enable this client with management access and the controller worker channel. Worker availability is checked at dispatch."
		}
	} else if v.NeedsReview {
		v.Reason = "The provider, model or policy changed. Review worker messaging permission."
	}
	return v, nil
}
func (s *Service) ConfigureWorker(uid int64, kind, subject, session string, in store.WorkerGrant) (WorkerSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return WorkerSettings{}, ErrUnavailable
	}
	v, err := s.WorkerSettings(uid, kind, subject, session)
	if err != nil {
		return v, err
	}
	if in.Enabled && (!v.Allowed || kind == "gaffer" && (in.ProviderID != v.ProviderID || in.ProviderRevision != v.ProviderRevision || in.Model != v.Model)) {
		return v, invalid("worker", "Review the session, client and provider permissions before enabling worker messages.")
	}
	in.SessionID = session
	if kind != "gaffer" || !in.Enabled {
		in.ProviderID = ""
		in.ProviderRevision = 0
		in.Model = ""
	}
	if _, err = s.store.SetWorkerGrantFor(uid, kind, subject, in.Revision, in); err != nil {
		return v, err
	}
	if kind == "gaffer" {
		if job := s.chatJobs[subject]; job != nil && job.uid == uid {
			job.cancel(errGafferPolicy)
		}
	}
	return s.WorkerSettings(uid, kind, subject, session)
}
func (s *Service) WorkerMessage(ctx context.Context, uid int64, kind, subject, session, id, turn, text string) (store.WorkerMessage, error) {
	if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" || !store.ValidWorkerText(text) {
		return store.WorkerMessage{}, invalid("worker", "Use a stable 32-character message ID and an instruction of up to 8,000 UTF-8 bytes without control characters.")
	}

	v, err := s.WorkerSettings(uid, kind, subject, session)
	if err != nil {
		return store.WorkerMessage{}, err
	}
	if !v.Grant.Enabled || !v.Allowed || v.NeedsReview {
		return store.WorkerMessage{}, ErrContextForbidden
	}
	// The store compares the complete original body before any repeat can enqueue.
	if old, err := s.store.WorkerMessageFor(uid, kind, subject, session, id); err == nil {
		if old.Text != text || old.TurnID != turn {
			return old, store.ErrGafferConflict
		}
		return s.WorkerMessageReceipt(uid, kind, subject, session, id)
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.WorkerMessage{}, err
	}
	s.mu.Lock()
	allowed := !s.closed && s.workerLimiter.Allow(strconv.FormatInt(uid, 10))
	if allowed {
		s.workerLimiter.Hit(strconv.FormatInt(uid, 10))
	}
	s.mu.Unlock()
	if !allowed {
		return store.WorkerMessage{}, ErrBusy
	}
	if err = ctx.Err(); err != nil {
		return store.WorkerMessage{}, err
	}
	probe, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	status, err := s.worker.ChatControl(probe, uid, session, terminalproto.ACPControl{Action: "get"})
	blocked := ""
	if err != nil {
		blocked = "The worker channel is unavailable or unsupported. No instruction was queued."
	} else if status.Controls.Waiting {
		blocked = "The worker needs human input or approval. No instruction was queued."
	} else if status.Controls.Configuring {
		blocked = "The worker is changing settings. No instruction was queued."
	}
	if err = ctx.Err(); err != nil {
		return store.WorkerMessage{}, err
	}
	s.mu.Lock()
	if s.closed || ctx.Err() != nil {
		s.mu.Unlock()
		if ctx.Err() != nil {
			return store.WorkerMessage{}, ctx.Err()
		}
		return store.WorkerMessage{}, ErrUnavailable
	}
	message, created, err := s.store.AdmitWorkerMessageFor(uid, kind, subject, store.WorkerMessage{ID: id, SessionID: session, TurnID: turn, Text: text, GrantRevision: v.Grant.Revision}, blocked)
	s.mu.Unlock()
	if err != nil {
		return message, err
	}
	if created && message.State == "queued" {
		// Admission is already durable. A changed connection cannot justify submitting
		// a new message ID. The existing queue will recheck its delivery fence.
		_ = s.worker.EnqueueChat(uid, session, message.QueueID, message.Text, nil)
	}
	return s.WorkerMessageReceipt(uid, kind, subject, session, id)
}
func (s *Service) WorkerMessageReceipt(uid int64, kind, subject, session, id string) (store.WorkerMessage, error) {
	v, err := s.WorkerSettings(uid, kind, subject, session)
	if err != nil {
		return store.WorkerMessage{}, err
	}
	if !v.Grant.Enabled || !v.Allowed || v.NeedsReview {
		return store.WorkerMessage{}, ErrContextForbidden
	}
	if s.worker != nil {
		if err = s.worker.ReconcileWorkerQueue(uid, session); err != nil {
			return store.WorkerMessage{}, err
		}
	}
	m, err := s.store.WorkerMessageFor(uid, kind, subject, session, id)
	if err != nil {
		return m, err
	}
	if err = s.store.CheckWorkerGrant(uid, kind, subject, session, v.Grant.Revision); err != nil {
		return store.WorkerMessage{}, err
	}
	return m, nil
}
func (s *Service) InterruptWorker(uid int64, kind, subject, session, id string, revision int64) (store.WorkerMessage, error) {
	if s.worker == nil {
		return store.WorkerMessage{}, ErrUnavailable
	}
	if err := s.store.InterruptWorkerMessageFor(uid, kind, subject, session, id, revision); err != nil {
		return store.WorkerMessage{}, err
	}
	m, err := s.store.WorkerMessageFor(uid, kind, subject, session, id)
	if err != nil {
		return m, err
	}
	_ = s.worker.EnqueueChat(uid, session, m.QueueID, m.Text, nil)
	return s.store.WorkerMessageFor(uid, kind, subject, session, id)
}
func (s *Service) workerTurnAccess(job *chatJob, c store.GafferConversation) error {
	if job.workerRevision == 0 {
		return nil
	}
	return s.store.CheckWorkerGrant(job.uid, "gaffer", c.ID, c.SessionID, job.workerRevision)
}
func workerToolResult(m store.WorkerMessage) string {
	// This operation's receipt is shared; it does not grant transcript/tool-result
	// reads. Do not copy worker output or raw transport errors into model context.
	raw, _ := json.Marshal(map[string]any{"id": m.ID, "queueId": m.QueueID, "state": m.State, "accepted_is_not_completed": true})
	return string(raw)
}
