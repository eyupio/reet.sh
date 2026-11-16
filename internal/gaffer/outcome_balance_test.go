package gaffer

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func planAccount(tool string, at time.Time, limits ...terminalproto.AccountLimit) terminalproto.AgentAccount {
	a := terminalproto.AgentAccount{Tool: tool, State: terminalproto.AccountSignedIn, Method: "subscription", LimitsState: terminalproto.AccountLimitsRead, Limits: limits, LimitsAt: at.UnixMilli()}
	if len(limits) == 0 {
		a.Method, a.LimitsState, a.LimitsAt = "api_key", terminalproto.AccountLimitsUnsupported, 0
	}
	return a
}

func window(kind terminalproto.AccountLimitKind, used int, label string) terminalproto.AccountLimit {
	return terminalproto.AccountLimit{Kind: kind, UsedPercent: used, Label: label}
}

// A stage stays on its first choice until that plan's tightest window is
// under a fifth, and moves only to a backup with more left.
func TestAStageUsesItsBackupOnlyWhenItsFirstChoiceRunsLowAndTheBackupHasMore(t *testing.T) {
	now := time.Now()
	first, backup := store.GafferStageAgent{Tool: "claude", Model: "opus"}, store.GafferStageAgent{Tool: "codex"}
	for name, c := range map[string]struct {
		accounts []terminalproto.AgentAccount
		use      bool
		says     string
	}{
		"plenty left":            {[]terminalproto.AgentAccount{planAccount("claude", now, window("session", 10, ""), window("week", 60, "")), planAccount("codex", now, window("week", 5, ""))}, false, ""},
		"low, backup has more":   {[]terminalproto.AgentAccount{planAccount("claude", now, window("session", 10, ""), window("week", 88, "")), planAccount("codex", now, window("week", 36, ""))}, true, "Claude Code (opus)’s plan had 12% left, so this stage runs on Codex, which had 64% left."},
		"low, backup lower":      {[]terminalproto.AgentAccount{planAccount("claude", now, window("week", 88, "")), planAccount("codex", now, window("week", 95, ""))}, false, ""},
		"low, backup on a key":   {[]terminalproto.AgentAccount{planAccount("claude", now, window("week", 88, "")), planAccount("codex", now)}, true, "which has no plan figures to run low."},
		"low, backup signed out": {[]terminalproto.AgentAccount{planAccount("claude", now, window("week", 88, "")), {Tool: "codex", State: terminalproto.AccountNone, LimitsState: terminalproto.AccountLimitsNotAsked}}, false, ""},
		"no figures":             {[]terminalproto.AgentAccount{planAccount("claude", now), planAccount("codex", now, window("week", 5, ""))}, false, ""},
		"figures too old":        {[]terminalproto.AgentAccount{planAccount("claude", now.Add(-7*time.Hour), window("week", 99, "")), planAccount("codex", now, window("week", 5, ""))}, false, ""},
		"only its model is low":  {[]terminalproto.AgentAccount{planAccount("claude", now, window("week", 30, ""), window("week", 96, "Opus")), planAccount("codex", now, window("week", 5, ""))}, true, "had 4% left"},
	} {
		use, why := balanceChoice(c.accounts, first, backup, now)
		if use != c.use || !strings.Contains(why, c.says) || !use && why != "" {
			t.Errorf("%s: %v %q", name, use, why)
		}
	}
	// A window for one model does not count against a stage on another, so a cheaper model of the same agent can be the backup.
	same := []terminalproto.AgentAccount{planAccount("claude", now, window("week", 40, ""), window("week", 96, "Opus"))}
	if use, why := balanceChoice(same, first, store.GafferStageAgent{Tool: "claude", Model: "sonnet"}, now); !use || !strings.Contains(why, "Claude Code (sonnet), which had 60% left") {
		t.Errorf("same agent, another model: %v %q", use, why)
	}
}

// The controller applies the rule when a stage starts: the session is asked
// for on the backup, the stage says so and keeps its first choice, and the
// move is in the activity log.
func TestAStageStartsOnItsBackupWhenItsPlanIsLowAndSaysSo(t *testing.T) {
	r := outcomeFixture(t)
	log := recordAudit(r.s)
	now := time.Now()
	if err := r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 20, ""), window("week", 91, "cheap"))}, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint.", Stages: []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "other/coder", Effort: "low"}},
		// No backup: this stage is never moved.
		{Kind: "review", Tool: "opencode", Model: "strong/reviewer"},
	}}}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	stage := code.Stages[0]
	if stage.Tool != "opencode" || stage.Model != "other/coder" || stage.Effort != "low" || stage.First == nil || stage.First.Tool != "opencode" || stage.First.Model != "cheap/coder" || !strings.Contains(stage.Balanced, "OpenCode (cheap/coder)’s plan had 9% left") || !strings.Contains(stage.Balanced, "which had 80% left") {
		t.Fatalf("balanced stage: %+v", stage)
	}
	asked := r.starter.asked()
	if len(asked) != 1 || asked[0].Tool != "opencode" || asked[0].Model != "other/coder" || asked[0].Effort != "low" {
		t.Fatalf("launches: %+v", asked)
	}
	found := false
	for _, row := range log.all() {
		found = found || row.action == "gaffer-stage-balanced" && row.detail["stage"] == "code" && row.detail["tool"] == "opencode"
	}
	if !found {
		t.Fatalf("the move was not recorded: %+v", log.all())
	}
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Done.", "gaffer")
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Stage == 1 && p.State == "following" })
	if got := review.Stages[1]; got.Model != "strong/reviewer" || got.First != nil || got.Balanced != "" {
		t.Fatalf("a stage with no backup was moved: %+v", got)
	}
}

// A crew tier's backup is saved with the crew and comes with a stage that
// takes its agent from the tier. A backup that is the first choice again, or
// names no agent, is no backup.
func TestATiersBackupIsSavedAndComesWithItsStages(t *testing.T) {
	r := outcomeFixture(t)
	saved, err := r.s.SaveCrew(r.p.UserID, store.GafferCrew{
		Heavy:    store.GafferCrewTier{Tool: "opencode", Model: "strong/reviewer", Backup: &store.GafferStageAgent{Tool: "codex", Model: "gpt-strong"}},
		Standard: store.GafferCrewTier{Tool: "opencode", Model: "cheap/coder", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "cheap/coder"}},
		Light:    store.GafferCrewTier{Tool: "opencode", Backup: &store.GafferStageAgent{}},
	})
	if err != nil || saved.Heavy.Backup == nil || saved.Heavy.Backup.Tool != "codex" || saved.Standard.Backup != nil || saved.Light.Backup != nil {
		t.Fatalf("saved crew: %+v %v", saved, err)
	}
	if _, err = r.s.SaveCrew(r.p.UserID, store.GafferCrew{Heavy: store.GafferCrewTier{Tool: "opencode", Backup: &store.GafferStageAgent{Tool: "not-an-agent"}}}); err == nil {
		t.Fatal("a backup that is not a chat agent was saved")
	}
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint.", Stages: []OutcomeStageInput{{Kind: "review"}, {Kind: "pr"}}}}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	stages := mustOutcome(t, r, created.ID).Sessions[0].Stages
	if stages[0].Tool != "opencode" || stages[0].Backup == nil || stages[0].Backup.Tool != "codex" || stages[0].Backup.Model != "gpt-strong" || stages[1].Backup != nil {
		t.Fatalf("stages: %+v", stages)
	}
}
