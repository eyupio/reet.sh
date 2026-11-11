package gaffer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/store"
)

const MaxContextBytes = 16 << 10

var ErrContextForbidden = errors.New("Context access is not allowed for this connection or data policy. Review explicit sharing permissions in a signed-in browser.")
var ErrContextBusy = errors.New("The context read limit is 60 per account per minute. Wait before reading again.")

type ContextState struct {
	ProviderID       string             `json:"providerId,omitempty"`
	ProviderName     string             `json:"providerName,omitempty"`
	Grant            store.ContextGrant `json:"grant"`
	Allowed          store.ContextFlags `json:"allowed"`
	NeedsReview      bool               `json:"needsReview"`
	Reason           string             `json:"reason,omitempty"`
	ProviderRevision int64              `json:"providerRevision,omitempty"`
	Model            string             `json:"model,omitempty"`
}

type ContextInput struct {
	ProviderID string `json:"providerId"`
	store.ContextFlags
	Revision         int64  `json:"revision"`
	ProviderRevision int64  `json:"providerRevision"`
	Model            string `json:"model"`
}

func (s *Service) ContextSettings(uid int64, kind, subject, session string) (ContextState, error) {
	if err := s.store.CheckContextSubject(uid, kind, subject, session, false); err != nil {
		return ContextState{}, err
	}
	g, err := s.store.ContextGrantFor(uid, kind, subject, session)
	if err != nil {
		return ContextState{}, err
	}
	v := ContextState{Grant: g, Allowed: store.ContextFlags{Status: true, History: session != "", Text: session != ""}}
	if kind == "gaffer" {
		c, err := s.store.GafferConversationFor(uid, subject)
		if err != nil {
			return v, err
		}
		p, err := s.store.GafferProviderFor(uid, c.ProviderID)
		v.ProviderID = c.ProviderID
		v.ProviderName = p.Name
		v.Allowed = store.ContextFlags{}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return v, err
		}
		v.ProviderRevision = p.Revision
		v.Model = c.Model
		if v.Model == "" {
			v.Model = p.Model
		}
		if err == nil && p.Enabled {
			v.Allowed = store.ContextFlags{Status: p.AllowStatus, History: session != "" && p.AllowSessions, Text: session != "" && p.AllowSessions && p.AllowCode}
		}
		v.NeedsReview = g.Any() && (g.ProviderID != c.ProviderID || g.ProviderRevision != p.Revision || g.Model != v.Model || !v.Allowed.Contains(g.ContextFlags))
		if v.NeedsReview {
			v.Reason = "The provider, model or data policy changed. Review sharing before sending another message."
		}
		if !p.Enabled {
			v.Reason = "Enable the selected provider before sharing context. You can still stop sharing."
		}
	}
	return v, nil
}

func (s *Service) ConfigureContext(uid int64, kind, subject, session string, in ContextInput) (ContextState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ContextState{}, ErrUnavailable
	}
	v, err := s.ContextSettings(uid, kind, subject, session)
	if err != nil {
		return v, err
	}
	if in.Text && !in.History || session == "" && (in.History || in.Text) || !v.Allowed.Contains(in.ContextFlags) {
		return v, invalid("context", "Choose context allowed by this provider and session. Message text needs the code/logs permission as well as session history.")
	}
	g := store.ContextGrant{SessionID: session, ContextFlags: in.ContextFlags}
	if kind == "gaffer" && in.Any() {
		if in.ProviderID != v.ProviderID || in.ProviderRevision != v.ProviderRevision || in.Model != v.Model {
			return v, store.ErrGafferConflict
		}
		c, err := s.store.GafferConversationFor(uid, subject)
		if err != nil {
			return v, err
		}
		g.ProviderID = c.ProviderID
		g.ProviderRevision = v.ProviderRevision
		g.Model = v.Model
	}
	if _, err = s.store.SetContextGrantFor(uid, kind, subject, in.Revision, g); err != nil {
		return v, err
	}
	if kind == "gaffer" {
		if job := s.chatJobs[subject]; job != nil && job.uid == uid {
			job.cancel(errGafferPolicy)
		}
	}
	return s.ContextSettings(uid, kind, subject, session)
}

type ContextSession struct {
	ID            string `json:"id"`
	State         string `json:"state,omitempty"`
	Worker        string `json:"worker,omitempty"`
	UpdatedAt     int64  `json:"updatedAt,omitempty"`
	Title         string `json:"title,omitempty"`
	GoalExcerpt   string `json:"goalExcerpt,omitempty"`
	GoalTruncated bool   `json:"goalTruncated,omitempty"`
}
type ContextEvent struct {
	Seq       int64  `json:"seq"`
	At        int64  `json:"at"`
	Kind      string `json:"kind"`
	State     string `json:"state,omitempty"`
	ToolKind  string `json:"toolKind,omitempty"`
	Status    string `json:"status,omitempty"`
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}
type ContextHistory struct {
	Events           []ContextEvent `json:"events"`
	FirstRetainedSeq int64          `json:"firstRetainedSeq"`
	LastRetainedSeq  int64          `json:"lastRetainedSeq"`
	RetainedCount    int            `json:"retainedCount"`
	Unavailable      bool           `json:"unavailable"`
	Truncated        bool           `json:"truncated"`
}

// ContextPipeline tells Gaffer where a session sits in a Factory pipeline: the
// controller's own record, read from the outcome the person approved. Stage
// names and states need status sharing; the outcome's words and the handover
// notes are text and need text sharing.
type ContextPipeline struct {
	Outcome      string                 `json:"outcome,omitempty"`
	Title        string                 `json:"title,omitempty"`
	State        string                 `json:"state"`
	Note         string                 `json:"note,omitempty"`
	StageInHand  string                 `json:"stageInHand,omitempty"`
	Stages       []ContextPipelineStage `json:"stages"`
	Merge        string                 `json:"merge,omitempty"`
	GoalState    string                 `json:"goalState,omitempty"`
	GoalNote     string                 `json:"goalNote,omitempty"`
	HowItMovesOn string                 `json:"howItMovesOn"`
}
type ContextPipelineStage struct {
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Worker      string `json:"worker,omitempty"`
	ThisSession bool   `json:"thisSession,omitempty"`
	Handover    string `json:"handover,omitempty"`
}

const pipelineMovesOn = "A pipeline goes to its next stage when Gaffer's own goal review finds the stage's deliverable done, or when the person moves it on themselves: with Move to next stage on the Factory page, or by telling Gaffer in this session's chat to continue. If the stage's goal needs the person, they can answer the working model, or use Review goal then Resume goal on the Factory page to have the work already there reviewed. The merge setting applies only at the merge stage and can be changed per pipeline on the Factory page."

// contextPipeline finds the pipeline a session belongs to, if any.
func (s *Service) contextPipeline(uid int64, session string, flags store.ContextFlags) *ContextPipeline {
	rows, err := s.store.GafferOutcomesFor(uid)
	if err != nil {
		return nil
	}
	for _, o := range rows {
		for _, piece := range o.Sessions {
			mine := piece.SessionID == session
			for _, stage := range piece.Stages {
				mine = mine || stage.SessionID == session
			}
			if !mine || len(piece.Stages) == 0 {
				continue
			}
			out := &ContextPipeline{State: piece.State, Stages: []ContextPipelineStage{}, HowItMovesOn: pipelineMovesOn}
			if piece.Stage < len(piece.Stages) {
				out.StageInHand = piece.Stages[piece.Stage].Kind
			}
			for _, stage := range piece.Stages {
				if stage.Kind != "merge" {
					continue
				}
				out.Merge = "waits for the person's approval"
				if piece.AutoMerge {
					out.Merge = "by itself once checks pass"
				} else if piece.MergeApproved {
					out.Merge = "approved by the person"
				}
			}
			if flags.Text {
				out.Outcome, _ = contextText(o.Objective, 1024)
				out.Title, _ = contextText(piece.Title, 320)
				out.Note, _ = contextText(piece.Note, 320)
			}
			for _, stage := range piece.Stages {
				row := ContextPipelineStage{Kind: stage.Kind, State: stage.State, Worker: stage.Tool, ThisSession: stage.SessionID == session}
				if flags.Text {
					row.Handover, _ = contextText(stage.Summary, 600)
				}
				out.Stages = append(out.Stages, row)
			}
			if piece.ConversationID != "" {
				if g, err := s.store.GafferGoalFor(uid, piece.ConversationID); err == nil {
					out.GoalState = g.State
					if flags.Text {
						out.GoalNote, _ = contextText(g.Note, 320)
					}
				}
			}
			return out
		}
	}
	return nil
}

type ContextSnapshot struct {
	Version   int                 `json:"version"`
	SessionID string              `json:"sessionId"`
	ReadAt    int64               `json:"readAt"`
	Source    string              `json:"source"`
	Shared    store.ContextFlags  `json:"shared"`
	Fleet     *store.ContextFleet `json:"fleet,omitempty"`
	Session   *ContextSession     `json:"session,omitempty"`
	History   *ContextHistory     `json:"history,omitempty"`
	// Pipeline is set when the session is one stage of a Factory pipeline.
	Pipeline *ContextPipeline `json:"pipeline,omitempty"`
	NotRead  []string         `json:"notRead"`
}

var contextKey = regexp.MustCompile(`(?i)\b(?:sk-(?:proj-|ant-)?|github_pat_|gh[pousr]_|xox[baprs]-)[a-z0-9_-]{8,}|\bAKIA[A-Z0-9]{16}\b`)
var contextAssignment = regexp.MustCompile(`(?im)\b(authorization|api[_ -]?key|access[_ -]?token|password|secret|token)\b["' :=\t]+[^\s,\r\n]+`)
var contextPrivateKey = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`)
var contextURLCredential = regexp.MustCompile(`https?://[^\s/@]+:[^\s/@]+@`)

// Best effort only: text sharing is an explicit code/logs disclosure, not a
// promise to detect every secret or distinguish prose from embedded code.
func contextText(text string, limit int) (string, bool) {
	text = strings.ToValidUTF8(text, "")
	text = contextPrivateKey.ReplaceAllString(text, "[private key omitted]")
	text = contextKey.ReplaceAllString(text, "[credential omitted]")
	text = contextAssignment.ReplaceAllString(text, "$1: [credential omitted]")
	text = contextURLCredential.ReplaceAllString(text, "https://[credential omitted]@")
	if len(text) <= limit {
		return text, false
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text, true
}

// Read only controller-retained facts. No filesystem, live worker request,
// GitHub request, transcript ingestion or raw tool payload is involved.
func (s *Service) contextSnapshot(uid int64, session string, flags store.ContextFlags) (ContextSnapshot, error) {
	x := ContextSnapshot{Version: 1, SessionID: session, ReadAt: time.Now().UnixMilli(), Source: "controller_retained_not_live", Shared: flags, NotRead: []string{"live_machine_health", "live_git_or_pull_request_checks", "raw_tool_arguments_and_output", "files_and_diffs", "reasoning", "approval_prompts"}}
	if !flags.Any() {
		return x, nil
	}
	if session == "" {
		if flags.Status {
			f, err := s.store.ContextFleetFor(uid)
			if err != nil {
				return x, err
			}
			x.Fleet = &f
		}
		return x, nil
	}
	if err := s.store.CheckContextSubject(uid, "browser", "browser", session, false); err != nil {
		return x, err
	}
	row, err := s.store.TerminalSessionFor(uid, session)
	if err != nil {
		return x, err
	}
	if flags.Status {
		worker := row.Tool
		switch worker {
		case "claude", "codex", "opencode", "cline", "antigravity":
		default:
			worker = "unknown"
		}
		x.Session = &ContextSession{ID: row.ID, State: string(row.State), Worker: worker, UpdatedAt: row.UpdatedAt}
	}
	x.Pipeline = s.contextPipeline(uid, session, flags)
	if flags.Text {
		if x.Session == nil {
			x.Session = &ContextSession{ID: session}
		}
		x.Session.Title, _ = contextText(row.Title, 320)
		x.Session.GoalExcerpt, x.Session.GoalTruncated = contextText(row.PromptExcerpt, 1024)
		x.Session.GoalTruncated = x.Session.GoalTruncated || row.PromptMore
	}
	if flags.History {
		raw, first, last, count, err := s.store.ContextEventsFor(uid, session, flags.Text)
		if err != nil {
			return x, err
		}
		h := &ContextHistory{Events: []ContextEvent{}, FirstRetainedSeq: first, LastRetainedSeq: last, RetainedCount: count, Unavailable: count == 0, Truncated: first > 1 || count > 40}
		for i := len(raw) - 1; i >= 0; i-- {
			var e ContextEvent
			if json.Unmarshal(raw[i].Event, &e) != nil {
				h.Truncated = true
				continue
			}
			e.Seq = raw[i].Seq
			e.At = raw[i].At
			switch e.Kind {
			case "user", "agent":
				e.State = ""
				e.Status = ""
				e.ToolKind = ""
				if flags.Text {
					original := e.Text
					e.Text, e.Truncated = contextText(original, 2048)
					e.Truncated = e.Truncated || utf8.RuneCountInString(original) >= 4096
				} else {
					e.Text = ""
				}
			case "status":
				e.Text = ""
				e.Status = ""
				e.ToolKind = ""
				switch e.State {
				case "starting", "working", "waiting", "idle", "cancelled", "error", "ended":
				default:
					e.State = "unreported"
				}
			case "tool":
				e.Text = ""
				e.State = ""
				switch e.ToolKind {
				case "read", "edit", "delete", "move", "search", "execute", "think", "fetch", "switch_mode", "other":
				default:
					e.ToolKind = "other"
				}
				switch e.Status {
				case "pending", "in_progress", "completed", "failed", "cancelled":
				default:
					e.Status = "unreported"
				}
			default:
				h.Truncated = true
				continue
			}
			h.Truncated = h.Truncated || e.Truncated
			h.Events = append(h.Events, e)
		}
		if len(h.Events) > 20 {
			h.Events = h.Events[len(h.Events)-20:]
			h.Truncated = true
		}
		x.History = h
		for {
			b, err := json.Marshal(x)
			if err != nil {
				return x, err
			}
			if len(b) <= MaxContextBytes {
				break
			}
			if len(h.Events) == 0 {
				return x, errors.New("The context metadata exceeds its limit.")
			}
			h.Events = h.Events[1:]
			h.Truncated = true
		}
	}
	return x, nil
}

func (s *Service) PreviewContext(uid int64, kind, subject, session string, flags store.ContextFlags) (ContextSnapshot, error) {
	v, err := s.ContextSettings(uid, kind, subject, session)
	if err != nil {
		return ContextSnapshot{}, err
	}
	if !v.Allowed.Contains(flags) || flags.Text && !flags.History || session == "" && (flags.History || flags.Text) {
		return ContextSnapshot{}, ErrContextForbidden
	}
	x, err := s.contextSnapshot(uid, session, flags)
	if err != nil {
		return x, err
	}
	if err = s.store.CheckContextSubject(uid, kind, subject, session, false); err != nil {
		return x, err
	}
	return x, nil
}

func (s *Service) MCPContextAccess(uid int64, kind, subject string) ([]store.ContextGrant, error) {
	if kind != "mcp" && kind != "browser" {
		return nil, ErrContextForbidden
	}
	if err := s.store.CheckContextSubject(uid, kind, subject, "", true); err != nil {
		return nil, err
	}
	out, err := s.store.ContextGrantsFor(uid, kind, subject)
	if err != nil {
		return nil, err
	}
	if err = s.store.CheckContextSubject(uid, kind, subject, "", true); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) ReadMCPContext(uid int64, kind, subject, session string) (ContextSnapshot, error) {
	if kind != "mcp" && kind != "browser" {
		return ContextSnapshot{}, ErrContextForbidden
	}
	s.mu.Lock()
	key := strconv.FormatInt(uid, 10)
	allowed := !s.closed && s.contextLimiter.Allow(key)
	if allowed {
		s.contextLimiter.Hit(key)
	}
	s.mu.Unlock()
	if !allowed {
		return ContextSnapshot{}, ErrContextBusy
	}
	if err := s.store.CheckContextSubject(uid, kind, subject, "", true); err != nil {
		return ContextSnapshot{}, err
	}
	g, err := s.store.ContextGrantFor(uid, kind, subject, session)
	if err != nil {
		return ContextSnapshot{}, err
	}
	if !g.Any() {
		return ContextSnapshot{}, ErrContextForbidden
	}
	if err := s.store.CheckContextSubject(uid, kind, subject, session, true); err != nil {
		return ContextSnapshot{}, err
	}
	x, err := s.contextSnapshot(uid, session, g.ContextFlags)
	if err != nil {
		return x, err
	}
	if err = s.store.CheckContextSubject(uid, kind, subject, session, true); err != nil {
		return x, err
	}
	current, err := s.store.ContextGrantFor(uid, kind, subject, session)
	if err != nil {
		return x, err
	}
	if current.Revision != g.Revision {
		return x, store.ErrGafferConflict
	}
	return x, nil
}

func contextUse(x ContextSnapshot, revision int64) *store.ContextUse {
	raw, _ := json.Marshal(x)
	digest := sha256.Sum256(raw)
	u := &store.ContextUse{Version: 1, ContextFlags: x.Shared, GrantRevision: revision, ReadAt: x.ReadAt, Digest: hex.EncodeToString(digest[:])}
	if x.History != nil {
		u.FirstSeq = x.History.FirstRetainedSeq
		u.LastSeq = x.History.LastRetainedSeq
		u.Truncated = x.History.Truncated
		for _, e := range x.History.Events {
			u.LastEventAt = max(u.LastEventAt, e.At)
		}
	}
	return u
}
