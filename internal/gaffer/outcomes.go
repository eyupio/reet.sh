package gaffer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/elicore"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// An outcome is an objective the person hands Gaffer. Planning it is one model
// request that proposes the sessions to start. It reads no reet data, starts no
// session, saves nothing and grants nothing: the person reviews and edits the
// plan, and their browser starts each session through the ordinary launch path.
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
}

type OutcomePlan struct {
	Summary       string           `json:"summary"`
	Sessions      []OutcomeSession `json:"sessions"`
	Model         string           `json:"model"`
	UsageReported bool             `json:"usageReported"`
	InputTokens   int              `json:"inputTokens"`
	OutputTokens  int              `json:"outputTokens"`
}

var planSessionsTool = elicore.Tool{Name: "propose_sessions", Description: "Propose the coding-agent sessions to start for the user's objective. This starts nothing: the user reviews and edits the proposal before any session exists.", Parameters: json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string","minLength":1,"maxLength":1000},"sessions":{"type":"array","minItems":1,"maxItems":5,"items":{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":80},"prompt":{"type":"string","minLength":1,"maxLength":6000},"stages":{"type":"array","maxItems":8,"items":{"type":"string","enum":["plan","design","code","review","test","pr","babysit","merge"]}},"ratings":{"type":"array","maxItems":8,"items":{"type":"object","properties":{"stage":{"type":"string","enum":["plan","design","code","review","test","pr","babysit","merge"]},"tier":{"type":"string","enum":["light","standard","heavy"]},"why":{"type":"string","maxLength":240}},"required":["stage","tier"],"additionalProperties":false}},"parts":{"type":"array","maxItems":12,"items":{"type":"object","properties":{"stage":{"type":"string","enum":["plan","design","code","review","test"]},"title":{"type":"string","minLength":1,"maxLength":60},"brief":{"type":"string","minLength":1,"maxLength":1000},"tier":{"type":"string","enum":["light","standard","heavy"]},"why":{"type":"string","maxLength":240}},"required":["stage","title","brief"],"additionalProperties":false}}},"required":["title","prompt"],"additionalProperties":false}}},"required":["summary","sessions"],"additionalProperties":false}`)}

func outcomeSystem(max int) string {
	return "You are Gaffer, the user's warm, practical critical buddy in reet.sh, with occasional dry Yorkshire humour. The user has given you an objective to own. Plan the coding-agent sessions that would meet it, then call propose_sessions exactly once. You start nothing: the user reviews, edits and approves the plan before any session exists.\n" +
		"Each session is a separate coding agent working in its own isolated copy of the same project. Sessions cannot see or message each other, so every prompt must stand alone: say what to build or change, what is out of scope, how to check it, and that it should raise a pull request describing the work. Split only where the work is genuinely independent, so two sessions never edit the same code. Prefer one session for a small objective. Propose at most " + itoa(max) + " sessions.\n" +
		"Stay inside the objective: add no scope the user did not ask for, and when something is ambiguous say so in the summary instead of guessing. Each session is a pipeline that runs through stages in this order: plan, design, code, review, test, pr, babysit, merge. For each session list in \"stages\" the ones it needs: leave out plan and design for small, obvious work, and include review and test for anything that changes behaviour. You advise on how demanding each stage is; the user decides which agent and model that means and whether merge happens without them, and you name neither. In \"ratings\" rate a stage light, standard or heavy with one short reason, judged from this work and not from the stage's name: light is mechanical or routine (a pull request for finished work, a small rename), standard is ordinary building and checking, heavy needs careful judgement (unfamiliar or intricate code, data that must not be lost, security, a design with real trade-offs). The user's defaults are heavy for plan, design and review, standard for code and test, and light for pr, babysit and merge: rate only the stages where this work calls for something else, and leave the rest out. A stage whose work falls into clearly different pieces that suit different tiers can be split: in \"parts\" give two to four parts for that stage, each with a short title, a brief saying exactly what that part does and, where it differs from the stage's, its own tier and reason. Parts of a stage run one after another in the same folder, each done by a separate agent that sees only its own brief and the part before's handover, so each brief must stand alone and the order must make sense. Split only when it lets cheaper work go to a lighter tier or hard work to a heavier one; most stages are not split, and the pull request, babysitting and merge never are. Every prompt must end with the sentence \"Do not merge.\" Titles are short plain names of at most 80 characters. The summary says in two or three sentences how the sessions divide the work and what the user should check.\n" +
		"When the person replies to a previous proposal, their reply is the instruction to act on. Keep what is still right, change what they asked, and address their reply directly in the summary rather than repeating a proposal they turned down."
}

func itoa(n int) string { return string(rune('0' + n)) }

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

// PlanOutcome asks the chosen provider for a session plan. The provider
// receives the typed objective and project name; a reply to an earlier
// proposal also carries that reply and a compact copy of what it answered.
func (s *Service) PlanOutcome(parent context.Context, p Principal, id string, in OutcomePlanInput) (OutcomePlan, error) {
	var plan OutcomePlan
	objective, project := strings.TrimSpace(in.Objective), strings.TrimSpace(in.Project)
	if objective == "" || len(objective) > MaxOutcomeObjectiveBytes || !store.ValidWorkerText(objective) {
		return plan, invalid("objective", "Write the objective in up to 4,000 UTF-8 bytes of plain text.")
	}
	if len(project) > 200 || !utf8.ValidString(project) || strings.ContainsAny(project, "\x00\r\n") {
		return plan, invalid("project", "Use a project name of up to 200 bytes on one line.")
	}
	if in.Sessions < 1 || in.Sessions > MaxOutcomeSessions {
		return plan, invalid("sessions", "Choose between 1 and 5 sessions.")
	}
	reply := strings.TrimSpace(in.Reply)
	if reply != "" && (len(reply) > MaxOutcomeReplyBytes || !store.ValidWorkerText(reply)) {
		return plan, invalid("reply", "Write the reply in up to 2,000 UTF-8 bytes of plain text.")
	}
	var previous *OutcomePlanPrevious
	if reply != "" {
		var err error
		previous, err = validPlanPrevious(in.Previous)
		if err != nil {
			return plan, err
		}
	}
	old, err := s.owned(p, Input{ID: id, Revision: in.Revision})
	if err != nil {
		return plan, err
	}
	if old.ID == "" {
		return plan, store.ErrNotFound
	}
	if !old.Enabled {
		return plan, invalid("provider", "Enable this Gaffer provider before asking it for a plan.")
	}
	cfg, err := normalize(p, inputFrom(old), true)
	if err != nil {
		return plan, err
	}
	ctx, done, err := s.begin(parent, p, id, 90*time.Second)
	if err != nil {
		return plan, err
	}
	defer done()
	if err = s.current(p, old); err != nil {
		return plan, err
	}
	backend, client, err := s.open(p, cfg, old, "")
	if err != nil {
		return plan, err
	}
	defer client.CloseIdleConnections()
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
				ask += "\nSession " + itoa(i+1) + ": " + session.Title
				if len(session.Stages) > 0 {
					ask += " (stages: " + strings.Join(session.Stages, ", ") + ")"
				}
			}
		} else {
			ask += "\n(The previous proposal's text was not sent.)"
		}
		ask += "\n\nThe person replied:\n" + reply + "\n\nRevise the proposal as a direct response to that reply. Keep what is still right, change what the person asked, and address their reply in the summary. Call propose_sessions exactly once."
	}
	stream, err := backend.Chat(ctx, elicore.Request{Model: cfg.Model, System: outcomeSystem(in.Sessions), Messages: []elicore.Message{{Role: elicore.RoleUser, Content: ask}}, Tools: []elicore.Tool{planSessionsTool}, MaxTokens: 4096})
	if err != nil {
		return plan, planFailure(parent, err)
	}
	defer stream.Close()
	var calls []elicore.ToolCall
	cut := false
	for {
		event, ok := stream.Next(ctx)
		if !ok {
			break
		}
		if event.Err != nil {
			return plan, planFailure(parent, event.Err)
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
	if parent.Err() != nil {
		return plan, parent.Err()
	}
	if err = s.current(p, old); err != nil {
		return plan, err
	}
	if cut {
		return plan, &ProbeError{"The provider stopped before the plan was complete. Try fewer sessions or a shorter objective, or write the plan yourself."}
	}
	if len(calls) != 1 || calls[0].Name != planSessionsTool.Name {
		return plan, &ProbeError{"This model did not return a plan. Choose a model that supports tools, or write the plan yourself."}
	}
	summary, sessions, err := outcomeSessions(calls[0].Arguments, in.Sessions)
	if err != nil {
		return plan, err
	}
	plan.Summary, plan.Sessions, plan.Model = summary, sessions, cfg.Model
	return plan, nil
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
// and a session exists only after the person approves it.
func outcomeSessions(raw json.RawMessage, max int) (string, []OutcomeSession, error) {
	malformed := &ProbeError{"The provider returned a plan reet could not read. Try again, or write the plan yourself."}
	if len(raw) > 64<<10 {
		return "", nil, malformed
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
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		return "", nil, malformed
	}
	var extra any
	if d.Decode(&extra) != io.EOF || len(in.Sessions) == 0 {
		return "", nil, malformed
	}
	if len(in.Sessions) > max {
		return "", nil, &ProbeError{"The provider proposed more sessions than you asked for. Try again, or write the plan yourself."}
	}
	summary := strings.TrimSpace(in.Summary)
	if len(summary) > maxOutcomeSummaryBytes || !utf8.ValidString(summary) || strings.ContainsRune(summary, 0) {
		return "", nil, malformed
	}
	seen := map[string]bool{}
	out := make([]OutcomeSession, 0, len(in.Sessions))
	for _, item := range in.Sessions {
		// Models write tabs and Windows line ends; neither changes the meaning, and the launch prompt rules refuse both.
		prompt := strings.TrimSpace(strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ").Replace(item.Prompt))
		if prompt == "" || len(prompt) > MaxOutcomePromptBytes || !terminalproto.ValidPrompt(prompt) {
			return "", nil, malformed
		}
		title := strings.TrimSpace(item.Title)
		if !terminalproto.ValidTitle(title) {
			title = terminalproto.PromptTitle(prompt)
		}
		if !terminalproto.ValidTitle(title) || seen[title] {
			return "", nil, malformed
		}
		seen[title] = true
		var stages []OutcomeStageInput
		last := -1
		for _, kind := range item.Stages {
			at := stageIndex(kind)
			if at <= last {
				return "", nil, malformed
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
				return "", nil, malformed
			}
			rated[rating.Stage] = true
			found := false
			for i := range stages {
				if stages[i].Kind == rating.Stage {
					stages[i].Tier, stages[i].TierWhy, found = rating.Tier, why, true
				}
			}
			if !found {
				return "", nil, malformed
			}
		}
		// A split replaces a stage with its parts, in the order given. A part is
		// a name, a brief and a tier: like a rating, it names no agent or model.
		split := map[string][]OutcomeStageInput{}
		var order []string
		for _, part := range item.Parts {
			name := strings.TrimSpace(part.Title)
			brief := strings.TrimSpace(strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ").Replace(part.Brief))
			why := strings.TrimSpace(part.Why)
			if !stageTakesParts(part.Stage) || !terminalproto.ValidTitle(name) || brief == "" || len(brief) > maxPartBriefBytes || !store.ValidWorkerText(brief) ||
				part.Tier != "" && !validStageTier(part.Tier) || len(why) > 300 || why != "" && !store.ValidWorkerText(why) {
				return "", nil, malformed
			}
			for _, have := range split[part.Stage] {
				if have.Part == name {
					return "", nil, malformed
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
				return "", nil, malformed
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
	return summary, out, nil
}
