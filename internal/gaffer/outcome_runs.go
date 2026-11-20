package gaffer

import (
	"context"
	"errors"
	"strconv"
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
	AgentID, RequestID, Title, Tool, Model, Effort, Provider, Dir, ProjectID, Prompt string
	// LinkProjectID ties a later stage's session to the outcome's project in
	// the session list. It is not a project launch: no worktree is made for it.
	LinkProjectID string
	// Trust is the person's own confirmation, made when they approved the
	// plan, that this folder's plugins may run in the agent.
	Trust bool
	// Yolo is the person's own choice, confirmed when they approved the plan,
	// that the agent need not wait for approvals. The launch path checks the
	// account setting, the agent and the machine again at every start.
	Yolo bool
	// PullLatest is the person's answer for this outcome's fresh worktrees:
	// "yes" for GitHub's latest commit for the base, "no" for the machine's
	// copy, "" for the project's setting, then the account's.
	PullLatest string
	// Images are every uploaded path the person attached to the outcome, in
	// order. Every agent the outcome starts is given all of them, not only the
	// first. Older outcomes have none, which starts an agent as before.
	Images []string
}

// SessionStarter starts an approved session as its owner through the same
// checks as a launch from the browser. A repeated request ID
// answers the session that request already started. The error text is shown
// to the person.
type SessionStarter interface {
	StartOutcomeSession(ctx context.Context, uid int64, in OutcomeLaunch) (sessionID, workDir string, err error)
}

// SessionChoice is the agent, model and effort a running session is on, as
// its machine reports them.
type SessionChoice struct {
	Tool   string
	Model  string
	Effort string
}

// SessionChoiceReader asks a session's machine what the session is running.
// A launch carries the person's choice as a request; this is the read-back
// that says whether it took.
type SessionChoiceReader interface {
	OutcomeSessionChoice(ctx context.Context, uid int64, sessionID string) (SessionChoice, error)
}

// ErrLaunchUnknown marks a launch error that says nothing about whether a
// session was started: the machine could not be reached or did not answer.
// A refusal by the machine is not one. The controller keeps an unanswered
// launch's request until it gets an answer that is.
var ErrLaunchUnknown = errors.New("the launch could not be confirmed either way")

type launchUnknown struct{ message string }

func (e launchUnknown) Error() string        { return e.message }
func (e launchUnknown) Is(target error) bool { return target == ErrLaunchUnknown }

// LaunchUnknown is an error a SessionStarter returns when it could not reach
// the machine. Its text is shown to the person as it is.
func LaunchUnknown(message string) error { return launchUnknown{message} }

// ErrEffortNotOffered marks a launch the machine refused because the agent
// did not confirm the chosen effort for the model. Nothing was started and no
// prompt was sent, so the launch may be asked for again without the effort.
var ErrEffortNotOffered = errors.New("the agent did not confirm the chosen effort")

type effortNotOffered struct{ message string }

func (e effortNotOffered) Error() string        { return e.message }
func (e effortNotOffered) Is(target error) bool { return target == ErrEffortNotOffered }

// EffortNotOffered is the error a SessionStarter returns when the machine
// refused the start because the agent did not confirm the chosen effort. Its
// text is shown to the person as it is.
func EffortNotOffered(message string) error { return effortNotOffered{message} }

// SessionEnder ends a session the controller started for a pipeline, once
// the pipeline has finished with it. The session's history is kept.
type SessionEnder interface {
	EndOutcomeSession(ctx context.Context, uid int64, sessionID string) error
}

// leftBehind marks a session the controller ended for a pipeline as seen. The
// pipeline carries on elsewhere and its card says where it stands, so the
// session is not shown to the person as newly finished with something for
// them to do. The time is a little ahead because the machine's own report
// that the session ended can arrive after the request returns.
func (s *Service) leftBehind(uid int64, sessionID string) {
	_ = s.store.MarkSessionSeenFor(uid, sessionID, time.Now().Add(10*time.Minute).UnixMilli())
}

func (s *Service) SetSessionStarter(starter SessionStarter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starter = starter
}

type OutcomeInput struct {
	Objective string `json:"objective"`
	ProjectID string `json:"projectId"`
	Tool      string `json:"tool"`
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	Trust     bool   `json:"trust"`
	// Yolo starts every session of the outcome without approvals. It is the
	// person's choice only: no plan and no model can ask for it.
	Yolo bool `json:"yolo"`
	// PullLatest is the person's answer, for this outcome's first worktrees,
	// whether each starts from GitHub's latest commit for its base ("yes"),
	// this machine's copy ("no"), or whichever the project, then the account,
	// says (""). It changes nothing for a project that is not a repository.
	PullLatest string `json:"pullLatest"`
	// Images are the uploaded machine paths the person attached to the
	// outcome. They travel with every agent the outcome starts.
	Images     []string         `json:"images,omitempty"`
	Sessions   []OutcomeSession `json:"sessions"`
	MaxReviews int              `json:"maxReviews"`
	Hours      int              `json:"hours"`
	Confirmed  bool             `json:"confirmed"`
	// AutoMerge lets every pipeline of this outcome merge by itself once it is
	// green, unless a pipeline says otherwise. It is the person's own choice.
	AutoMerge bool `json:"autoMerge"`
	// AutoReport has Gaffer report across the outcome by itself when a
	// pipeline finishes or stops. Each report is one model request, and it
	// only ever proposes.
	AutoReport bool `json:"autoReport"`
	// Automation gives Gaffer this outcome's decisions: what to do when a
	// stage stops. It is the person's choice only, and never covers a merge
	// or an agent's approval.
	Automation bool `json:"automation"`
	// BalanceBelow is the share of a plan, as a whole percentage, under which
	// a stage starts on its backup. Zero takes the saved crew's, else reet's
	// default.
	BalanceBelow int `json:"balanceBelow"`
	// RequestID, when given, names this request: asking again under the same
	// one answers the outcome it already started instead of starting another.
	// It is for a client that may not hear back; the browser sends none.
	RequestID string `json:"-"`
}

// OutcomeStageInput is one stage of a planned pipeline: what it is and which
// agent and model do it. A plan from Gaffer's provider carries the kind only.
type OutcomeStageInput struct {
	Kind  string `json:"kind"`
	Tool  string `json:"tool,omitempty"`
	Model string `json:"model,omitempty"`
	// Provider is the billing source a Cline stage starts on; empty is Cline's own choice.
	Provider string `json:"provider,omitempty"`
	// Effort is how hard the stage's agent thinks: empty for its own default.
	Effort string `json:"effort,omitempty"`
	// Tier is how demanding the stage is: light, standard or heavy. Empty
	// takes the default for its kind. TierWhy is Gaffer's reason for a tier it
	// assessed. A stage that names no agent is done by its tier's.
	Tier    string `json:"tier,omitempty"`
	TierWhy string `json:"tierWhy,omitempty"`
	// Backup is the person's second choice for the stage, used when the
	// first choice's plan is running low and the backup's has more left.
	Backup *store.GafferStageAgent `json:"backup,omitempty"`
	// More are further backups, tried in order after Backup.
	More []store.GafferStageAgent `json:"moreBackups,omitempty"`
	// Part and Brief make this entry one part of a split stage: its short
	// name and what it is to do. Entries of one kind that carry a part sit
	// together and run in order, each on its own tier's agent.
	Part  string `json:"part,omitempty"`
	Brief string `json:"brief,omitempty"`
}

// MaxStageParts is how many parts one stage can be split into, and
// MaxPipelineStages how many stage entries a pipeline can have in all.
const (
	MaxStageParts     = 4
	MaxPipelineStages = 16
	maxPartBriefBytes = 1200
)

// partTitle names the session of one part of a split stage: the pipeline,
// the stage and the part, within the protocol's limit for a session title
// whatever the lengths of the three. The pipeline's name gives way first.
func partTitle(pipeline, kind, part string) string {
	clip := func(text string, max int) string {
		runes := []rune(text)
		if len(runes) <= max {
			return text
		}
		if max < 1 {
			return ""
		}
		return strings.TrimSpace(string(runes[:max-1])) + "…"
	}
	tail := " · " + kind + ": " + clip(part, 30)
	return clip(pipeline, terminalproto.MaxTitleRunes-len([]rune(tail))) + tail
}

// stageTakesParts reports whether a kind of stage can be split. The pull
// request, its babysitting and the merge are each one thing.
func stageTakesParts(kind string) bool {
	switch kind {
	case "plan", "design", "code", "review", "test":
		return true
	}
	return false
}

// StageTiers are how demanding a stage can be judged to be, lightest first.
var StageTiers = []string{"light", "standard", "heavy"}

func validStageTier(tier string) bool {
	return tier == "light" || tier == "standard" || tier == "heavy"
}

// DefaultStageTier is the tier a stage has until someone says otherwise:
// thinking stages are heavy, building ones standard and shipping ones light.
func DefaultStageTier(kind string) string {
	switch kind {
	case "plan", "design", "review":
		return "heavy"
	case "code", "test":
		return "standard"
	}
	return "light"
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
		// Merging is the merge stage's work, decided by the pipeline's own
		// merge setting when it gets there. Said this way so a review does not
		// read the setting as a conflict with this stage.
		text += " Nothing is merged in this stage."
	}
	return text
}

// stageDeliverable is what a stage must leave behind before it is done, said
// to the working agent so it knows when to stop.
var stageDeliverable = map[string]string{
	"plan":    "the written plan, as text in this session",
	"design":  "the design decisions, as text in this session",
	"code":    "the implementation and its tests, committed on this branch",
	"review":  "the review's findings, with what you fixed committed",
	"test":    "the checks and tests run, with their results reported",
	"pr":      "the pull request raised, with its link",
	"babysit": "a pull request with passing checks, no open review thread and no conflict",
	"merge":   "the pull request merged",
}

// stagePrompt is what the working agent is told at the start of a stage. The
// stage and its boundary come first and last: an agent given the whole brief
// first and its stage as a footnote did the whole job in the plan stage.
func stagePrompt(o store.GafferOutcome, item store.GafferOutcomeSession, n int) string {
	stage := item.Stages[n]
	var later []string
	for _, next := range item.Stages[n+1:] {
		later = append(later, next.Kind)
	}
	text := "You are doing ONE stage of a staged piece of work: the " + stage.Kind + " stage. " + stageWork[stage.Kind]
	if stage.Part != "" {
		// A split stage: this session does one named part of it and no other.
		var others []string
		for i, other := range item.Stages {
			if i != n && other.Kind == stage.Kind && other.Part != "" {
				others = append(others, other.Part)
			}
		}
		text = "You are doing ONE PART of one stage of a staged piece of work: the “" + stage.Part + "” part of the " + stage.Kind + " stage. " + stageWork[stage.Kind] + "\n\nThis part: " + stage.Brief
		if len(others) > 0 {
			text += "\nThe other parts of this stage (" + strings.Join(others, ", ") + ") are done separately, in sessions of their own: do not do their work."
		}
	}
	if n > 0 {
		text += "\n\nThe work is already under way in this folder and branch."
		for i := n - 1; i >= 0; i-- {
			if item.Stages[i].Summary != "" {
				from := "the " + item.Stages[i].Kind + " stage"
				if item.Stages[i].Part != "" {
					from = "the “" + item.Stages[i].Part + "” part of " + from
				}
				text += " Handover from " + from + ": " + item.Stages[i].Summary
				break
			}
		}
	}
	if stage.SentBack != "" {
		text += "\n\nThis stage is being done again: the work was sent back to it. Why: " + stage.SentBack + " Work in the same folder and branch, keep what is sound, and put right what was sent back."
	}
	lead := "\n\nThe whole piece of work, for context only. Do not carry out the parts that belong to other stages:\n"
	// What follows the brief is the stage's boundary, which the agent must
	// read last; it is built apart so that a long prompt loses brief, not it.
	text, brief := text+lead, item.Prompt
	end := ""
	if stage.Part != "" {
		end += "\n\nThis part is finished when you have left, for this part only, " + stageDeliverable[stage.Kind] + ". Then stop and say so in one short message."
	} else {
		end += "\n\nThis stage is finished when you have left " + stageDeliverable[stage.Kind] + ". Then stop and say so in one short message."
	}
	if len(later) > 0 {
		// Named once each: the later parts of this stage are the same kind.
		var names []string
		for _, kind := range later {
			if kind != stage.Kind && (len(names) == 0 || names[len(names)-1] != kind) {
				names = append(names, kind)
			}
		}
		if len(names) > 0 {
			end += " The later stages (" + strings.Join(names, ", ") + ") are done separately, after this one is checked: do not start them."
		}
	}
	if stage.Kind != "merge" {
		end += " Do not merge."
	}
	if room := MaxOutcomePromptBytes - len(text) - len(end); len(brief) > room {
		if room < len("…") {
			// Even the start is too long: it is cut, and the boundary kept.
			return clipTo(text, MaxOutcomePromptBytes-len(end)) + end
		}
		brief = clipTo(brief, room)
	}
	return text + brief + end
}

// clipTo cuts text to at most limit bytes on a character boundary, marking
// the cut with "…".
func clipTo(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit - len("…")
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:max(cut, 0)] + "…"
}

const (
	outcomeLineBytes  = 600
	outcomeFollowWait = 60 * time.Second
)

// outcomeStartWait is how long one launch is waited for.
var (
	outcomeStartWait   = 90 * time.Second
	outcomeFollowEvery = 2 * time.Second
)

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
	providerSource := strings.TrimSpace(in.Provider)
	if providerSource != "" && (!terminalproto.ToolTakesProvider(in.Tool) || !terminalproto.ValidProvider(providerSource)) {
		return o, invalid("provider", "Choose a Cline billing source, or leave it to Cline’s own.")
	}
	if in.MaxReviews < 1 || in.MaxReviews > 48 || in.Hours < 1 || in.Hours > 168 {
		return o, invalid("goal", "Choose 1 to 48 reviews over 1 to 168 hours for each session.")
	}
	if len(in.Sessions) < 1 || len(in.Sessions) > store.MaxGafferOutcomeSessions {
		return o, invalid("sessions", "An outcome starts between 1 and 5 sessions.")
	}
	if !terminalproto.ValidACPPrompt("outcome", in.Images) {
		return o, invalid("images", "Attach up to "+strconv.Itoa(MaxOutcomeImages)+" uploaded images to an outcome.")
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
	if in.RequestID != "" {
		if !store.ValidGafferOutcomeID(in.RequestID) {
			return o, invalid("requestId", "A request ID is 32 lowercase hexadecimal characters.")
		}
		// Asked for before under this request: that outcome is the answer.
		if existing, err := s.store.GafferOutcomeFor(uid, in.RequestID); err == nil {
			return outcomeView(existing), nil
		}
		id = in.RequestID
	}
	crew, err := s.store.GafferCrewFor(uid)
	if err != nil {
		return o, err
	}
	sessions, err := outcomePieces(in.Sessions, in.Tool, model, providerSource, crew, in.AutoMerge, map[string]bool{}, 0)
	if err != nil {
		return o, err
	}
	if !validBalanceBelow(in.BalanceBelow) {
		return o, invalid("balance", "Choose between 5 and 90 percent for when a backup takes over.")
	}
	balanceBelow := in.BalanceBelow
	if balanceBelow == 0 {
		balanceBelow = crew.BalanceBelow
	}
	// Trust and YOLO are judged on the agents that will run: a stage that
	// names none takes its tier's from the saved crew, and a stage may start
	// on its backup.
	if in.Trust && in.Tool != "opencode" && in.Tool != "antigravity" && !outcomeRunsOn(sessions, "opencode", "antigravity") {
		return o, invalid("trust", "Only OpenCode and Google Antigravity ask you to trust a folder's plugins.")
	}
	if !in.Trust && (in.Tool == "antigravity" || outcomeRunsOn(sessions, "antigravity")) {
		return o, invalid("trust", "Google Antigravity starts only in a folder whose hooks and customizations you have trusted. Approve this outcome on the Factory page with folder trust ticked, or choose another agent.")
	}
	if in.Yolo && !outcomeTakesYolo(in.Tool, sessions) {
		return o, invalid("yolo", "One of the chosen agents has no YOLO option reet can use.")
	}
	if in.PullLatest != "" && in.PullLatest != "yes" && in.PullLatest != "no" {
		return o, invalid("pullLatest", "Choose GitHub's latest code, this machine's copy, or the project's default.")
	}
	o = store.GafferOutcome{ID: id, Objective: objective, AgentID: project.Host, ProjectID: project.ID, ProjectName: project.Name, Dir: project.Dir, Tool: in.Tool, Model: model, Provider: providerSource, Trust: in.Trust, Yolo: in.Yolo, PullLatest: in.PullLatest, Images: append([]string(nil), in.Images...), Automation: in.Automation, BalanceBelow: balanceBelow, AutoReport: in.AutoReport, ProviderID: provider.ID, MaxReviews: in.MaxReviews, Hours: in.Hours, Sessions: sessions}
	o, err = s.store.CreateGafferOutcomeFor(uid, o)
	if err != nil {
		return o, err
	}
	s.wakeOutcomes()
	return o, nil
}

// outcomePieces turns approved sessions into pipeline records. taken holds
// the titles already used in the outcome and from is how many pieces it has,
// so added pipelines get titles and keys of their own.
func outcomePieces(items []OutcomeSession, tool, model, provider string, crew store.GafferCrew, autoMerge bool, taken map[string]bool, from int) ([]store.GafferOutcomeSession, error) {
	out := make([]store.GafferOutcomeSession, 0, len(items))
	for i, item := range items {
		title, prompt := strings.TrimSpace(item.Title), strings.TrimSpace(item.Prompt)
		if !terminalproto.ValidTitle(title) || taken[title] {
			return nil, invalid("sessions", "Give each session its own title of 1 to 80 plain characters.")
		}
		if prompt == "" || len(prompt) > MaxOutcomePromptBytes || !terminalproto.ValidPrompt(prompt) {
			return nil, invalid("sessions", "Write a prompt of at most 8,000 bytes without control characters for “"+title+"”.")
		}
		taken[title] = true
		request, err := newChatID()
		if err != nil {
			return nil, err
		}
		piece := store.GafferOutcomeSession{Key: string(rune('a' + from + i)), Title: title, Prompt: prompt, RequestID: request, Note: "Waiting its turn.", AutoMerge: autoMerge}
		if item.AutoMerge != nil {
			piece.AutoMerge = *item.AutoMerge
		}
		last, parts := -1, map[string]bool{}
		if len(item.Stages) > MaxPipelineStages {
			return nil, invalid("sessions", "“"+title+"” has too many stages and parts. Keep it to "+strconv.Itoa(MaxPipelineStages)+".")
		}
		for n, stage := range item.Stages {
			at := stageIndex(stage.Kind)
			// A kind appears once, or as parts of one split stage sitting together.
			part, brief := strings.TrimSpace(stage.Part), strings.TrimSpace(stage.Brief)
			again := at == last && at >= 0
			if at < 0 || at < last || again && (part == "" || strings.TrimSpace(item.Stages[n-1].Part) == "") {
				return nil, invalid("sessions", "Keep the stages of “"+title+"” in pipeline order, each at most once unless it is split into named parts.")
			}
			if !again {
				parts = map[string]bool{}
			}
			if part != "" {
				if !stageTakesParts(stage.Kind) || !terminalproto.ValidTitle(part) || parts[part] || len(parts) >= MaxStageParts || brief == "" || len(brief) > maxPartBriefBytes || !store.ValidWorkerText(brief) {
					return nil, invalid("sessions", "A split stage of “"+title+"” has up to "+strconv.Itoa(MaxStageParts)+" parts, each with its own short name and a brief saying what it does. The pull request, babysitting and merge are not split.")
				}
				parts[part] = true
			} else if brief != "" {
				return nil, invalid("sessions", "A brief belongs to a named part of a stage in “"+title+"”.")
			}
			last = at
			stageTool, stageModel, stageProvider, stageEffort := stage.Tool, strings.TrimSpace(stage.Model), strings.TrimSpace(stage.Provider), stage.Effort
			second, others := stage.Backup, stage.More
			// How demanding the stage is: what Gaffer or the person said, else
			// its kind's default. It is kept with the stage either way.
			tier, why := stage.Tier, strings.TrimSpace(stage.TierWhy)
			if tier != "" && !validStageTier(tier) || len(why) > 300 || why != "" && !store.ValidWorkerText(why) {
				return nil, invalid("sessions", "Choose light, standard or heavy for each stage of “"+title+"”.")
			}
			if tier == "" {
				tier = DefaultStageTier(stage.Kind)
			}
			if stageTool == "" {
				// The stage names no agent: its tier's does it, from the
				// person's saved crew. A tier they have not set falls back to
				// the outcome's own agent and model.
				if member := crew.Tier(tier); member.Tool != "" {
					stageTool, stageModel, stageEffort, stageProvider = member.Tool, member.Model, member.Effort, member.Provider
					// The tier's backup comes with it.
					second, others = member.Backup, member.More
				} else {
					stageTool, stageModel = tool, model
					if stageProvider == "" {
						stageProvider = provider
					}
				}
			}
			if stageProvider != "" && (!terminalproto.ToolTakesProvider(stageTool) || !terminalproto.ValidProvider(stageProvider)) {
				return nil, invalid("provider", "Choose a billing source for Cline, or leave it to Cline's own, for every stage of “"+title+"”.")
			}
			// Every stage is a chat, whose effort is one of the agent's own
			// thinking levels: a name, not one of reet's three. The agent
			// confirms it for the chosen model before any prompt is sent.
			if stageEffort != "" && !terminalproto.ValidModel(stageEffort) {
				return nil, invalid("sessions", "Choose an effort the agent offers, or leave it to the agent's default, in “"+title+"”.")
			}
			if !terminalproto.ReviewedChatTool(stageTool) || stageModel != "" && !terminalproto.ValidModel(stageModel) {
				return nil, invalid("sessions", "Choose an agent that can run a Gaffer chat, and a model name it accepts, for every stage of “"+title+"”.")
			}
			backup, more, err := stageBackups(second, others, stageTool, stageModel, stageEffort, stageProvider)
			if err != nil {
				return nil, err
			}
			id, err := newChatID()
			if err != nil {
				return nil, err
			}
			piece.Stages = append(piece.Stages, store.GafferOutcomeStage{Kind: stage.Kind, Tool: stageTool, Model: stageModel, Effort: stageEffort, Provider: stageProvider, Backup: backup, More: more, Tier: tier, TierWhy: why, Part: part, Brief: brief, RequestID: id})
		}
		if len(piece.Stages) > 0 {
			piece.Stages[0].RequestID = request
		}
		out = append(out, piece)
	}
	return out, nil
}

// stageAgents lists every agent a resolved stage can start on: its own and
// its backup's.
func stageAgents(stage store.GafferOutcomeStage) []string {
	tools := []string{stage.Tool}
	if stage.First != nil {
		tools = append(tools, stage.First.Tool)
	}
	for _, spare := range store.Spares(stage.Backup, stage.More) {
		tools = append(tools, spare.Tool)
	}
	return tools
}

// outcomeTakesYolo reports whether every agent the outcome can run on, the
// main one and each stage's, can be started without approvals.
func outcomeTakesYolo(tool string, sessions []store.GafferOutcomeSession) bool {
	if !terminalproto.ToolTakesYolo(tool) {
		return false
	}
	for _, piece := range sessions {
		for _, stage := range piece.Stages {
			for _, name := range stageAgents(stage) {
				if !terminalproto.ToolTakesYolo(name) {
					return false
				}
			}
		}
	}
	return true
}

// outcomeRunsOn reports whether any stage of the resolved plan can start on
// one of the named agents.
func outcomeRunsOn(sessions []store.GafferOutcomeSession, tools ...string) bool {
	for _, piece := range sessions {
		for _, stage := range piece.Stages {
			for _, name := range stageAgents(stage) {
				for _, tool := range tools {
					if name == tool {
						return true
					}
				}
			}
		}
	}
	return false
}

// outcomeView reports what the person can rely on: a session the controller
// will no longer attempt reads as not started.
func outcomeView(o store.GafferOutcome) store.GafferOutcome {
	settleStoppedOutcome(&o)
	return o
}

// settleStoppedOutcome gives every pipeline of an outcome the controller is
// no longer starting the state the person is shown for it. It is applied to
// the saved record before an outcome is set going again (a retry, an added
// pipeline, an approved merge), so that only what the person asked for
// starts: a pipeline that was waiting when they stopped the outcome stays
// not started until they try that one again.
func settleStoppedOutcome(o *store.GafferOutcome) {
	if o.State == "starting" {
		return
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
		rows[i] = s.withAgentUsage(uid, outcomeView(rows[i]))
	}
	return rows, nil
}

func (s *Service) Outcome(uid int64, id string) (store.GafferOutcome, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferOutcome{}, err
	}
	o, err := s.store.GafferOutcomeFor(uid, id)
	return s.withAgentUsage(uid, outcomeView(o)), err
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

// ArchiveOutcome puts a finished outcome away, or brings it back. The record
// and its report are kept either way.
func (s *Service) ArchiveOutcome(uid int64, id string, revision int64, archive bool) (store.GafferOutcome, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferOutcome{}, err
	}
	var active bool
	o, err := s.store.UpdateGafferOutcomeFor(uid, id, revision, func(o *store.GafferOutcome) bool {
		if archive && !o.Finished() {
			active = true
			return false
		}
		if archive == (o.ArchivedAt > 0) {
			return false
		}
		if archive {
			o.ArchivedAt = time.Now().UnixMilli()
		} else {
			o.ArchivedAt = 0
		}
		return true
	})
	if err == nil && active {
		err = store.ErrGafferOutcomeActive
	}
	return outcomeView(o), err
}

// DeleteOutcome removes a finished outcome's record for good.
func (s *Service) DeleteOutcome(uid int64, id string, revision int64) error {
	if _, err := s.chatPrincipal(uid); err != nil {
		return err
	}
	return s.store.DeleteGafferOutcomeFor(uid, id, revision)
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
	s.releaseGoneSessions()
	s.rebalanceStopped(ctx)
	s.autoReports(ctx)
	s.autoDecisions(ctx)
	s.expirePendingTools()
	rows, err := s.store.StartingGafferOutcomes()
	// The controller's settings are read only when there is an outcome to
	// start, as the goal supervisor reads them only for a due goal.
	if err != nil || len(rows) == 0 || s.outcomesReady() != nil {
		return
	}
	// Each outcome is carried on by itself: a launch that takes its full
	// minute and a half, or a chat that is slow to come up, holds up that
	// outcome's own pipelines and nobody else's. One outcome is never worked
	// on twice at once.
	for _, o := range rows {
		if ctx.Err() != nil {
			return
		}
		if _, running := s.outcomeRuns.LoadOrStore(o.ID, struct{}{}); running {
			continue
		}
		s.wg.Add(1)
		go func(uid int64, id string) {
			defer s.wg.Done()
			s.runOutcome(ctx, uid, id)
			// What was asked for while this one was being worked on is picked up without waiting for the next tick.
			more := false
			if ctx.Err() == nil {
				if now, err := s.store.GafferOutcomeFor(uid, id); err == nil && now.State == "starting" {
					for _, piece := range now.Sessions {
						more = more || controllerHolds(piece.State)
					}
				}
			}
			// The outcome is let go of exactly once, and before the wake: a
			// second release after the next run had begun would let a third
			// start beside it.
			s.outcomeRuns.Delete(id)
			if more {
				s.wakeOutcomes()
			}
		}(o.OwnerID, o.ID)
	}
}

const (
	maxAutoReports    = 6
	autoReportSpacing = 10 * time.Minute
)

// autoReportEvent names what an outcome's pipelines have finished or stopped
// at: the pipelines that are done, ready to merge, not started or no longer
// followed. It is empty while every pipeline is still at work.
func autoReportEvent(o store.GafferOutcome) string {
	var parts []string
	for _, piece := range o.Sessions {
		switch piece.State {
		case "done", "ready_to_merge", "failed", "unfollowed":
			parts = append(parts, piece.Key+":"+piece.State)
		}
	}
	return strings.Join(parts, ",")
}

// autoReports asks for a report on each outcome whose owner chose that, once
// per change in which pipelines have finished or stopped. It is bounded: at
// most maxAutoReports for an outcome, and not within autoReportSpacing of its
// last report. A report starts nothing and only ever proposes.
func (s *Service) autoReports(ctx context.Context) {
	rows, err := s.store.AutoReportGafferOutcomes()
	if err != nil || len(rows) == 0 {
		return
	}
	for _, o := range rows {
		if ctx.Err() != nil {
			return
		}
		event := autoReportEvent(o)
		if event == "" || event == o.AutoReportedFor || o.AutoReports >= maxAutoReports {
			continue
		}
		if o.Report != nil && time.Since(time.UnixMilli(o.Report.At)) < autoReportSpacing {
			continue
		}
		p, err := s.chatPrincipal(o.OwnerID)
		if err != nil {
			continue
		}
		// Recorded before the request, so a failed or slow one is not asked for again and again.
		// Only the call that recorded the event asks: one that finds it already recorded
		// (another look at the same outcome got there first) must not ask a second time.
		recorded := false
		if _, err = s.outcomeStep(o.OwnerID, o.ID, func(o *store.GafferOutcome) bool {
			if o.AutoReportedFor == event {
				return false
			}
			o.AutoReportedFor, o.AutoReports = event, o.AutoReports+1
			recorded = true
			return true
		}); err != nil || !recorded {
			continue
		}
		s.wg.Add(1)
		go func(id string) {
			defer s.wg.Done()
			_, _ = s.reviewOutcome(ctx, p, id, true)
		}(o.ID)
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

// controllerHolds reports whether a pipeline is in a state the controller is
// working on: waiting to be started, being started, or moving to its next
// stage. Every other state is one the person, or Gaffer's hand-over, moves it
// out of.
func controllerHolds(state string) bool {
	return state == "waiting" || state == "starting" || state == "advancing"
}

// setOutcomeSession records the controller's own progress with a pipeline. A
// launch or a goal setup takes a while, and the person may stop the outcome
// meanwhile: the controller therefore writes only while the pipeline is still
// its to move, and wrote says whether it did. What the person decided stands.
func setOutcomeSession(key, state, note string, wrote *bool, more func(*store.GafferOutcomeSession)) func(*store.GafferOutcome) bool {
	return func(o *store.GafferOutcome) bool {
		*wrote = false
		for i := range o.Sessions {
			if o.Sessions[i].Key == key {
				if !controllerHolds(o.Sessions[i].State) {
					return false
				}
				o.Sessions[i].State, o.Sessions[i].Note, o.Sessions[i].UpdatedAt = state, note, time.Now().UnixMilli()
				if more != nil {
					more(&o.Sessions[i])
				}
				*wrote = true
				return true
			}
		}
		return false
	}
}

// mergeWaitNote is what a pipeline held at its merge stage says.
func mergeWaitNote(item store.GafferOutcomeSession) string {
	if item.AutoMerge {
		return "Ready to merge. This pipeline’s agent runs on YOLO, so the merge waits for you unless you confirmed it may merge by itself."
	}
	return "Ready to merge. Nothing merges until you approve it."
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
func (s *Service) stageHandedOver(uid int64, conversation, summary, by string) {
	rows, err := s.store.GafferOutcomesFor(uid)
	if err != nil {
		return
	}
	for _, o := range rows {
		for _, piece := range o.Sessions {
			if piece.State != "following" || len(piece.Stages) == 0 || piece.ConversationID != conversation {
				continue
			}
			key, kind := piece.Key, ""
			if piece.Stage < len(piece.Stages) {
				kind = piece.Stages[piece.Stage].Kind
			}
			used, _ := s.store.GafferGoalFor(uid, conversation)
			// What the stage's agent reported using is kept with the stage, like Gaffer's own figures.
			agent, agentReported := s.stageAgentUsage(uid, piece, piece.Stage)
			handed := false
			_, err = s.outcomeStep(uid, o.ID, func(o *store.GafferOutcome) bool {
				handed = false
				for i := range o.Sessions {
					row := &o.Sessions[i]
					if row.Key != key || row.State != "following" || row.ConversationID != conversation || row.Stage >= len(row.Stages) {
						continue
					}
					row.Stages[row.Stage].State, row.Stages[row.Stage].Summary, row.Stages[row.Stage].EndedAt = "done", clipBytes(summary, maxHandoverBytes), time.Now().UnixMilli()
					row.Stages[row.Stage].ClosedBy = by
					// What Gaffer's model used on this stage is kept with it: the next stage's goal starts its own count.
					row.Stages[row.Stage].Reviews, row.Stages[row.Stage].InputTokens, row.Stages[row.Stage].OutputTokens, row.Stages[row.Stage].UsageReported = used.UsedReviews, used.InputTokens, used.OutputTokens, used.UsageReported
					if agentReported {
						row.Stages[row.Stage].AgentInputTokens, row.Stages[row.Stage].AgentOutputTokens, row.Stages[row.Stage].AgentCachedTokens, row.Stages[row.Stage].AgentUsageReported = agent.InputTokens, agent.OutputTokens, agent.CachedTokens, true
					}
					// A stopped outcome starts nothing on Gaffer's word, only on the person's.
					if o.State != "stopped" || by != "gaffer" && by != "auto" {
						settleStoppedOutcome(o)
						o.State = "starting"
					}
					row.State, row.Note, row.UpdatedAt = "advancing", "The "+row.Stages[row.Stage].Kind+" stage is done. Starting the next.", time.Now().UnixMilli()
					handed = true
					return true
				}
				return false
			})
			// A hand-over that found the pipeline already moved (stopped, or
			// handed over by the person first) recorded nothing, and is not
			// logged as one.
			if err != nil || !handed {
				return
			}
			// Why the stage ended is recorded: Gaffer's review found it done, or
			// the person said so. The handover's words stay on the stage.
			reason := "Gaffer’s review found the " + kind + " stage done and handed it over."
			switch by {
			case "person":
				reason = "The person moved the pipeline on from the " + kind + " stage."
			case "chat":
				reason = "The person told Gaffer in the session’s chat to move the pipeline on from the " + kind + " stage."
			case "auto":
				reason = "Gaffer, given this outcome’s decisions, moved the pipeline on from the " + kind + " stage."
			}
			s.audit(uid, "gaffer-stage-handed-over", map[string]any{"outcome": o.ID, "pipeline": key, "stage": kind, "by": by, "session": piece.SessionID, "reviews": used.UsedReviews, "reason": reason})
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
				settleStoppedOutcome(o)
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
	// set reports whether the record could be read and written; wrote, whether
	// the pipeline was still the controller's to move and so was changed.
	wrote := false
	set := func(state, note string, more func(*store.GafferOutcomeSession)) bool {
		after, err := s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, state, note, &wrote, more))
		if err == nil && wrote {
			s.auditPipeline(after, item.Key)
		}
		return err == nil
	}
	next := item.Stage + 1
	if item.Stage < len(item.Stages) && item.Stages[item.Stage].State != "done" {
		// The merge stage was approved while its predecessor's handover was already recorded.
		next = item.Stage
	}
	if next >= len(item.Stages) {
		if !set("done", "Every stage is done. Check the result; only you mark the work complete.", nil) {
			return false
		}
		if wrote {
			s.endFinishedSession(ctx, o, item.Key, item.SessionID, "The pipeline finished.")
		}
		return true
	}
	stage := item.Stages[next]
	if stage.Kind == "merge" && !item.MergeApproved && !MergesByItself(o, item) {
		return set("ready_to_merge", mergeWaitNote(item), func(row *store.GafferOutcomeSession) { row.Stage = next })
	}
	if _, err := s.chatPrincipal(uid); err != nil {
		return set("unfollowed", "Stopped before the "+stage.Kind+" stage: this account can no longer start sessions.", nil)
	}
	// Every stage is a session of its own, whatever agent and model do it: it
	// starts with that stage's instruction and the handover and nothing else,
	// in the pipeline's folder. The session of the stage before is finished
	// with and is ended once this one is followed.
	if !set("starting", "Starting the "+stage.Kind+" stage…", func(row *store.GafferOutcomeSession) {
		row.Stage = next
		row.SessionID, row.ConversationID = row.Stages[next].SessionID, ""
	}) {
		return false
	}
	if !wrote {
		// Stopped by the person meanwhile: the next stage is not started.
		return true
	}
	fresh, err := s.store.GafferOutcomeFor(uid, o.ID)
	if err != nil {
		return false
	}
	for _, row := range fresh.Sessions {
		if row.Key == item.Key {
			ok := s.runOutcomeSession(ctx, fresh, row)
			// The stage before worked in a session of its own, which the
			// pipeline has now finished with, whether or not the next stage
			// could be started: a stage tried again gets a session of its own.
			// A controller that is stopping leaves it for the next start.
			if after, err := s.store.GafferOutcomeFor(uid, o.ID); ok && err == nil {
				for _, now := range after.Sessions {
					if now.Key == item.Key && now.SessionID != item.SessionID {
						reason := "The " + stage.Kind + " stage started in a session of its own."
						if now.SessionID == "" {
							reason = "Its stage is done, and the " + stage.Kind + " stage starts in a session of its own when it is tried again."
						}
						s.endFinishedSession(ctx, o, item.Key, item.SessionID, reason)
					}
				}
			}
			return ok
		}
	}
	return false
}

// endFinishedSession ends a session a pipeline has finished with, so it does
// not sit open for good. Its history stays readable and its folder is left
// alone. A session that has already ended, or that another pipeline is
// working in, is left as it is.
func (s *Service) endFinishedSession(ctx context.Context, o store.GafferOutcome, key, sessionID, reason string) {
	if sessionID == "" || !s.sessionRunning(o.OwnerID, sessionID) {
		return
	}
	s.mu.Lock()
	ender, _ := s.starter.(SessionEnder)
	s.mu.Unlock()
	if ender == nil {
		return
	}
	if rows, err := s.store.GafferOutcomesFor(o.OwnerID); err == nil {
		for _, other := range rows {
			for _, piece := range other.Sessions {
				// Still some pipeline's session in hand.
				if piece.SessionID == sessionID && piece.State != "done" {
					return
				}
			}
		}
	}
	endCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := ender.EndOutcomeSession(endCtx, o.OwnerID, sessionID); err != nil {
		return
	}
	s.leftBehind(o.OwnerID, sessionID)
	s.audit(o.OwnerID, "gaffer-session-ended", map[string]any{"outcome": o.ID, "pipeline": key, "session": sessionID, "reason": reason})
}

// runOutcomeSession starts the session for the stage in hand (or the whole
// piece, without stages) and puts it in Gaffer's care. It reports false when
// the controller is stopping and the session must be picked up again later.
func (s *Service) runOutcomeSession(ctx context.Context, o store.GafferOutcome, item store.GafferOutcomeSession) bool {
	uid := o.OwnerID
	wrote := false
	fail := func(state, note string) bool {
		after, err := s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, state, note, &wrote, nil))
		if err == nil && wrote {
			s.auditPipeline(after, item.Key)
		}
		return err == nil
	}
	launch := OutcomeLaunch{AgentID: o.AgentID, RequestID: item.RequestID, Title: item.Title, Tool: o.Tool, Model: o.Model, Provider: o.Provider, Trust: o.Trust, Yolo: o.Yolo, PullLatest: o.PullLatest, Dir: o.Dir, ProjectID: o.ProjectID, Prompt: item.Prompt, Images: append([]string(nil), o.Images...)}
	goal := OutcomeGoal(o.Objective, item.Title)
	staged := len(item.Stages) > 0 && item.Stage < len(item.Stages)
	// droppedEffort is whether the stage in hand starts without an effort its
	// agent was found not to offer.
	droppedEffort := false
	if staged {
		stage := item.Stages[item.Stage]
		launch.RequestID, launch.Tool, launch.Model, launch.Provider, launch.Prompt = stage.RequestID, stage.Tool, stage.Model, stage.Provider, stagePrompt(o, item, item.Stage)
		launch.Effort = stage.Effort
		launch.Trust = o.Trust && (stage.Tool == "opencode" || stage.Tool == "antigravity")
		launch.Title = clipBytes(item.Title, 60) + " · " + stage.Kind
		goal = StageGoal(o.Objective, item.Title, stage.Kind)
		if stage.Part != "" {
			// One part of a split stage: Gaffer follows it against the part's own brief.
			launch.Title = partTitle(item.Title, stage.Kind, stage.Part)
			goal += "\nThis session does one part of the stage only, “" + clipBytes(stage.Part, 80) + "”: " + clipBytes(stage.Brief, 600) + " Hand over when this part is done; the stage's other parts are done in sessions of their own."
		}
		// The merge gate holds on every path into the merge stage, not only on
		// a hand-over: a pipeline tried again at its merge stage, or one whose
		// first stage is the merge, waits for the person like any other.
		if stage.Kind == "merge" && !item.MergeApproved && !MergesByItself(o, item) {
			return fail("ready_to_merge", mergeWaitNote(item))
		}
		// A stage about to start runs on its backup when its first choice's
		// plan is running low and the backup's has more left. A launch that
		// was asked for and not answered is left on what it was asked on.
		if item.SessionID == "" && item.UnsureRequest == "" {
			if next, changed := s.balanceStage(o, stage); changed {
				at := item.Stage
				if _, err := s.outcomeStep(uid, o.ID, func(o *store.GafferOutcome) bool {
					for i := range o.Sessions {
						if row := &o.Sessions[i]; row.Key == item.Key && row.Stage == at && at < len(row.Stages) && row.Stages[at].RequestID == stage.RequestID {
							row.Stages[at].Tool, row.Stages[at].Model, row.Stages[at].Effort, row.Stages[at].Provider = next.Tool, next.Model, next.Effort, next.Provider
							row.Stages[at].First, row.Stages[at].Balanced = next.First, next.Balanced
							row.Stages[at].Rebalanced = row.Stages[at].Rebalanced || next.Rebalanced
							row.Stages[at].EffortNotOffered = next.EffortNotOffered
							return true
						}
					}
					return false
				}); err != nil {
					return false
				}
				stage = next
				launch.Tool, launch.Model, launch.Provider, launch.Effort = stage.Tool, stage.Model, stage.Provider, stage.Effort
				launch.Trust = o.Trust && (stage.Tool == "opencode" || stage.Tool == "antigravity")
				if stage.Balanced != "" {
					s.audit(uid, "gaffer-stage-balanced", map[string]any{"outcome": o.ID, "pipeline": item.Key, "stage": stage.Kind, "tool": stage.Tool, "reason": stage.Balanced})
				}
			}
			// An effort the machine's last report says the agent does not offer
			// is not asked for: the stage starts on the agent's default effort.
			if item.SessionID == "" && item.UnsureRequest == "" && s.effortNotReported(uid, o.AgentID, launch.Tool, launch.Effort) && s.dropStageEffort(o, item.Key, item.Stage, stage.Kind, launch, "", "reported") {
				launch.Effort, stage.EffortNotOffered = "", launch.Effort
			}
			droppedEffort = launch.Effort == "" && stage.EffortNotOffered != ""
		}
	}
	if item.WorkDir != "" {
		// A later stage, or a step started again, works where the first one
		// did. It is not a new project launch, so it gets no worktree of its own.
		launch.Dir, launch.ProjectID, launch.LinkProjectID = item.WorkDir, "", o.ProjectID
	}
	// The account is read again before every session: a person who lost
	// write access after approving starts nothing more.
	if _, err := s.chatPrincipal(uid); err != nil {
		return fail("failed", "Not started: this account can no longer start sessions.")
	}
	sessionID := item.SessionID
	if sessionID == "" {
		if _, err := s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, "starting", "Starting the session…", &wrote, nil)); err != nil {
			return false
		}
		if !wrote {
			// Stopped by the person before anything was asked for.
			return true
		}
		s.mu.Lock()
		starter := s.starter
		s.mu.Unlock()
		if starter == nil {
			return false
		}
		// ask makes one launch request. unknown says the answer tells nothing
		// about whether a session started: the machine did not answer in time
		// (late), or could not be reached at all.
		ask := func(request string) (id, dir string, err error, unknown, late bool) {
			in := launch
			in.RequestID = request
			startCtx, cancel := context.WithTimeout(ctx, outcomeStartWait)
			defer cancel()
			id, dir, err = starter.StartOutcomeSession(startCtx, uid, in)
			if err == nil {
				return id, dir, nil, false, false
			}
			unreachable := errors.Is(err, ErrLaunchUnknown)
			late = !unreachable && startCtx.Err() == context.DeadlineExceeded
			return id, dir, err, unreachable || late, late
		}
		var started, workDir string
		var err error
		unsure, unknown, late := item.UnsureRequest, false, false
		if unsure == launch.RequestID {
			unsure = ""
		}
		if unsure != "" {
			// An earlier launch was not answered in time and may have started
			// its session all the same. Its request is asked again first: a
			// session it started is taken up, and only a launch that is known
			// to have failed is asked for afresh. A machine that cannot be
			// reached now is not that knowledge, so the request is kept.
			started, workDir, err, unknown, late = ask(unsure)
			if ctx.Err() != nil {
				return false
			}
			if err != nil && !unknown {
				unsure = ""
			}
		}
		if unsure == "" {
			started, workDir, err, unknown, late = ask(launch.RequestID)
			// A start refused because the agent did not confirm the chosen
			// effort started nothing and sent no prompt. It is asked for once
			// more without the effort, on the agent's default, under a request
			// of its own: the refused one stays refused on the machine, and is
			// replaced on the stage before it is sent, so a restart sends the
			// new request again and takes up what it started. A launch that
			// asked for no effort is never asked for twice, unless its stage's
			// effort was dropped already: the refusal is then the one recorded
			// for its old request, answered again.
			if err != nil && !unknown && staged && (launch.Effort != "" || droppedEffort) && errors.Is(err, ErrEffortNotOffered) && ctx.Err() == nil {
				if fresh, idErr := newChatID(); idErr == nil && s.dropStageEffort(o, item.Key, item.Stage, item.Stages[item.Stage].Kind, launch, fresh, "refused") {
					launch.RequestID, launch.Effort = fresh, ""
					started, workDir, err, unknown, late = ask(launch.RequestID)
				}
			}
			// Only a launch that ran out of time may have started something: a
			// machine that could not be reached was never asked.
			if late {
				unsure = launch.RequestID
			}
		}
		if ctx.Err() != nil {
			// The same request ID is sent again after a restart, so a launch
			// whose answer was lost is adopted, not repeated.
			return false
		}
		if err != nil {
			state, note := "failed", "Not started: "+clipBytes(err.Error(), 400)
			if staged && item.Stage > 0 {
				state, note = "unfollowed", "The "+item.Stages[item.Stage].Kind+" stage was not started: "+clipBytes(err.Error(), 400)
			}
			switch {
			case late:
				note = "The machine did not answer in time, so reet cannot tell whether the session started. Try again: a session that did start is taken up, not started twice."
			case unsure != "":
				note = "Not started: " + clipBytes(err.Error(), 300) + " An earlier launch was not answered, so reet still cannot tell whether its session started. Try again once the machine can be reached: that session is taken up if it is there."
			}
			after, stepErr := s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, state, note, &wrote, func(row *store.GafferOutcomeSession) { row.UnsureRequest = unsure }))
			if stepErr == nil && wrote {
				s.auditPipeline(after, item.Key)
			}
			return stepErr == nil
		}
		sessionID = started
		// The launch took a while. The session is put in Gaffer's care only if
		// the pipeline is still the controller's to start and still asks for
		// this launch. If the person stopped it meanwhile, their stop stands:
		// the session is kept on the record, so it can be found and tried
		// again, and nothing follows it.
		kept, left := false, false
		// The person may have put the outcome on YOLO, or taken it off, while
		// this launch was in flight: the session then started on the old choice.
		yoloNow := launch.Yolo
		after, err := s.outcomeStep(uid, o.ID, func(o *store.GafferOutcome) bool {
			kept, left = false, false
			yoloNow = o.Yolo
			for i := range o.Sessions {
				row := &o.Sessions[i]
				if row.Key != item.Key {
					continue
				}
				request := row.RequestID
				if len(row.Stages) > 0 && row.Stage < len(row.Stages) {
					request = row.Stages[row.Stage].RequestID
				}
				if request != launch.RequestID || row.SessionID != "" {
					return false
				}
				if controllerHolds(row.State) {
					row.State, row.Note, kept = "starting", "Started. Putting it in Gaffer’s care…", true
				} else {
					row.State, row.Note, left = "unfollowed", "Started, but not followed: this outcome was stopped while its session was starting. Look after it from the floor.", true
				}
				row.SessionID, row.UpdatedAt, row.UnsureRequest = sessionID, time.Now().UnixMilli(), ""
				if row.WorkDir == "" {
					row.WorkDir = workDir
				}
				if row.Stage < len(row.Stages) {
					row.Stages[row.Stage].SessionID, row.Stages[row.Stage].State, row.Stages[row.Stage].StartedAt = sessionID, "running", time.Now().UnixMilli()
				}
				return true
			}
			return false
		})
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return false
		}
		if (kept || left) && yoloNow != launch.Yolo {
			s.mu.Lock()
			setter, _ := s.starter.(SessionYolo)
			s.mu.Unlock()
			switched := false
			if setter != nil {
				setCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				switched = setter.SetOutcomeSessionYolo(setCtx, uid, sessionID, yoloNow) == nil
				cancel()
			}
			s.audit(uid, "gaffer-session-yolo", map[string]any{"outcome": o.ID, "pipeline": item.Key, "session": sessionID, "on": yoloNow, "switched": switched, "reason": "The outcome's approvals were changed while this session was starting."})
		}
		if left {
			s.auditPipeline(after, item.Key)
		}
		if !kept {
			if !left {
				// The outcome was deleted, or the pipeline asks for another
				// launch by now: nothing refers to this session, so it is ended
				// instead of being left to run with nobody looking.
				s.endFinishedSession(ctx, o, item.Key, sessionID, "Its pipeline was stopped while the session was starting.")
			}
			return true
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
	followed := func() bool {
		note := "Started and followed. Gaffer waits for its first progress."
		if staged {
			note = "The " + item.Stages[item.Stage].Kind + " stage is under way."
		}
		after, err := s.outcomeStep(uid, o.ID, setOutcomeSession(item.Key, "following", note, &wrote, func(row *store.GafferOutcomeSession) {
			row.ConversationID = cid
			if row.Stage < len(row.Stages) {
				row.Stages[row.Stage].ConversationID = cid
			}
		}))
		if err == nil && wrote {
			s.auditPipeline(after, item.Key)
			// Staged or not, the session is asked what it is running.
			s.checkStageChoice(ctx, o, item.Key, item.Stage, sessionID)
			if staged && item.Stage > 0 {
				// The stage before worked in a session of its own, which the
				// pipeline has finished with. It is ended here as well as where
				// the stage was moved on, so a controller that restarted in
				// between does not leave it running.
				if before := item.Stages[item.Stage-1].SessionID; before != "" && before != sessionID {
					s.endFinishedSession(ctx, o, item.Key, before, "The "+item.Stages[item.Stage].Kind+" stage started in a session of its own.")
				}
			}
		}
		if err == nil && !wrote {
			// The person stopped the pipeline while its goal was being set: nothing follows it.
			s.stopPipelineGoal(uid, cid, stoppedFollowingNote)
		}
		return err == nil
	}
	deadline := time.Now().Add(outcomeFollowWait)
	for {
		v, err := s.Goal(uid, cid)
		if err != nil {
			return unfollowed(outcomeReason(err))
		}
		// A goal that was closed (the person stopped following, or an earlier
		// stage was handed over) is replaced; one still in hand is the person's.
		var replaces int64
		if v.Goal != nil {
			switch {
			case v.Goal.State == "stopped" || v.Goal.State == "completed":
				replaces = v.Goal.Revision
			case v.Goal.Text == goal:
				// This stage's own goal: the controller set it and stopped
				// before it could record that. It is taken up, not refused.
				return followed()
			default:
				return unfollowed("This session already has a goal.")
			}
		}
		if v.SetupAllowed {
			in := GoalSetupInput{GoalInput: GoalInput{GoalPins: v.Pins, Text: goal, MaxReviews: o.MaxReviews, Hours: o.Hours}, ConversationRevision: v.ConversationRevision, Confirmed: true, Action: "start"}
			in.Revision = replaces
			_, err = s.SetupGoal(uid, cid, in)
			if err == nil {
				return followed()
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
