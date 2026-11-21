package gaffer

import (
	"context"
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
		use, why := balanceChoice(c.accounts, first, backup, 0, now)
		if use != c.use || !strings.Contains(why, c.says) || !use && why != "" {
			t.Errorf("%s: %v %q", name, use, why)
		}
	}
	// A window for one model does not count against a stage on another, so a cheaper model of the same agent can be the backup.
	same := []terminalproto.AgentAccount{planAccount("claude", now, window("week", 40, ""), window("week", 96, "Opus"))}
	if use, why := balanceChoice(same, first, store.GafferStageAgent{Tool: "claude", Model: "sonnet"}, 0, now); !use || !strings.Contains(why, "Claude Code (sonnet), which had 60% left") {
		t.Errorf("same agent, another model: %v %q", use, why)
	}
}

// Copilot labels every window it has (Premium, Chat, Completions), and no
// model's name holds those labels. Such a plan is judged by its tightest
// window, never read as one with no figures.
func TestAPlanWithOnlyLabelledWindowsIsJudgedByItsTightest(t *testing.T) {
	now := time.Now()
	claude, copilot := store.GafferStageAgent{Tool: "claude", Model: "opus"}, store.GafferStageAgent{Tool: "copilot", Model: "made-up-large"}
	copilotPlan := func(premium, chat int) terminalproto.AgentAccount {
		return planAccount("copilot", now, window("month", premium, "Premium"), window("month", chat, "Chat"))
	}
	low := planAccount("claude", now, window("week", 88, ""))
	// A backup that has run out is not chosen.
	if use, why := balanceChoice([]terminalproto.AgentAccount{low, copilotPlan(100, 20)}, claude, copilot, 0, now); use {
		t.Errorf("a Copilot backup that has run out was chosen: %q", why)
	}
	// A backup with room is chosen, and the reason gives its figure.
	if use, why := balanceChoice([]terminalproto.AgentAccount{low, copilotPlan(30, 50)}, claude, copilot, 0, now); !use || why != "Claude Code (opus)’s plan had 12% left, so this stage runs on GitHub Copilot (made-up-large), which had 50% left." {
		t.Errorf("a Copilot backup with room: %v %q", use, why)
	}
	// A first choice running low is balanced away.
	if use, why := balanceChoice([]terminalproto.AgentAccount{copilotPlan(95, 10), planAccount("claude", now, window("week", 30, ""))}, copilot, claude, 0, now); !use || !strings.Contains(why, "GitHub Copilot (made-up-large)’s plan had 5% left") || !strings.Contains(why, "which had 70% left.") {
		t.Errorf("a Copilot first choice running low: %v %q", use, why)
	}
	// A plan-wide window still decides beside one for another model.
	if use, why := balanceChoice([]terminalproto.AgentAccount{low, planAccount("copilot", now, window("month", 40, ""), window("month", 99, "Premium"))}, claude, copilot, 0, now); !use || !strings.HasSuffix(why, "which had 60% left.") {
		t.Errorf("a plan-wide window: %v %q", use, why)
	}
	// Figures too old to go by are not called no figures.
	old := planAccount("copilot", now.Add(-7*time.Hour), window("month", 10, "Premium"))
	if use, why := balanceChoice([]terminalproto.AgentAccount{low, old}, claude, copilot, 0, now); !use || strings.Contains(why, "no plan figures") || !strings.HasSuffix(why, "whose plan figures are too old to go by.") {
		t.Errorf("a backup with old figures: %v %q", use, why)
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

// The share under which a backup takes over is the person's to set: saved
// with the crew, taken by an outcome that names none, and bounded.
func TestTheBalancingThresholdIsThePersonsToSet(t *testing.T) {
	now := time.Now()
	first, backup := store.GafferStageAgent{Tool: "claude"}, store.GafferStageAgent{Tool: "codex"}
	accounts := []terminalproto.AgentAccount{planAccount("claude", now, window("week", 60, "")), planAccount("codex", now, window("week", 10, ""))}
	// 40% left: above the default of 20, under a threshold of 50.
	if use, _ := balanceChoice(accounts, first, backup, 0, now); use {
		t.Fatal("moved at the default threshold with 40% left")
	}
	if use, why := balanceChoice(accounts, first, backup, 50, now); !use || !strings.Contains(why, "had 40% left") {
		t.Fatalf("not moved under a threshold of 50: %q", why)
	}
	r := outcomeFixture(t)
	for _, bad := range []int{-1, 4, 91} {
		if _, err := r.s.SaveCrew(r.p.UserID, store.GafferCrew{BalanceBelow: bad}); err == nil {
			t.Fatalf("a threshold of %d was saved", bad)
		}
		in := r.in
		in.BalanceBelow = bad
		if _, err := r.s.CreateOutcome(r.p.UserID, in); err == nil {
			t.Fatalf("an outcome took a threshold of %d", bad)
		}
	}
	if saved, err := r.s.SaveCrew(r.p.UserID, store.GafferCrew{BalanceBelow: 35}); err != nil || saved.BalanceBelow != 35 {
		t.Fatalf("saved: %+v %v", saved, err)
	}
	r.s.StartGoalSupervisor()
	// An outcome that names none takes the saved crew's; one that names its own keeps it.
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil || created.BalanceBelow != 35 {
		t.Fatalf("from the crew: %+v %v", created.BalanceBelow, err)
	}
	in := r.in
	in.BalanceBelow = 60
	if created, err = r.s.CreateOutcome(r.p.UserID, in); err != nil || created.BalanceBelow != 60 {
		t.Fatalf("its own: %+v %v", created.BalanceBelow, err)
	}
}

// Backups are tried in the order given: the first with more of its plan left
// does the stage. At most three, none the first choice again, none twice.
func TestBackupsAreTriedInOrder(t *testing.T) {
	r := outcomeFixture(t)
	now := time.Now()
	// The first backup's own model is as low as the first choice's; the second has room.
	if err := r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 30, ""), window("week", 92, "cheap"), window("week", 95, "tiny"))}, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint.", Stages: []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "tiny/coder"},
			More: []store.GafferStageAgent{{Tool: "opencode", Model: "other/coder"}, {Tool: "opencode", Model: "tiny/coder"}, {Tool: "opencode", Model: "cheap/coder"}}},
	}}}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	stage := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" }).Stages[0]
	// The repeat and the first choice again were dropped; the second backup did the stage.
	if stage.Backup == nil || stage.Backup.Model != "tiny/coder" || len(stage.More) != 1 || stage.More[0].Model != "other/coder" || stage.Model != "other/coder" || !strings.Contains(stage.Balanced, "which had 70% left") {
		t.Fatalf("stage: %+v", stage)
	}
	four := []store.GafferStageAgent{{Tool: "opencode", Model: "b"}, {Tool: "opencode", Model: "c"}, {Tool: "opencode", Model: "d"}}
	if _, err = r.s.SaveCrew(r.p.UserID, store.GafferCrew{Heavy: store.GafferCrewTier{Tool: "opencode", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "a"}, More: four}}); err == nil {
		t.Fatal("four backups were saved")
	}
}

// A working agent is never interrupted. When a stage's session has ended and
// its plan is now below the threshold, the stage is run again on its backup,
// once, and the activity log says so.
func TestAStageThatStoppedWithItsPlanLowIsRunAgainOnItsBackup(t *testing.T) {
	r := outcomeFixture(t)
	log := recordAudit(r.s)
	now := time.Now()
	plenty := []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 30, ""), window("week", 40, "cheap"))}
	if err := r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, plenty, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint.", Stages: []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "other/coder"}},
	}}}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	first := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if first.Stages[0].Model != "cheap/coder" {
		t.Fatalf("started on its backup with plenty left: %+v", first.Stages[0])
	}
	// The plan runs low while the agent is working: nothing is touched.
	low := []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 30, ""), window("week", 97, "cheap"))}
	if err = r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, low, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	r.s.rebalanceStopped(context.Background())
	if got := mustOutcome(t, r, created.ID).Sessions[0]; got.SessionID != first.SessionID || got.Stages[0].Rebalanced || len(r.starter.closed()) != 0 {
		t.Fatalf("a working agent was interrupted: %+v", got)
	}
	// Its session ends. Now the stage is run again, on the backup.
	if err = r.starter.EndOutcomeSession(context.Background(), r.p.UserID, first.SessionID); err != nil {
		t.Fatal(err)
	}
	r.s.rebalanceStopped(context.Background())
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.SessionID != first.SessionID
	})
	if stage := again.Stages[0]; stage.Model != "other/coder" || !stage.Rebalanced || stage.First == nil || stage.First.Model != "cheap/coder" || !strings.Contains(stage.Balanced, "had 3% left") {
		t.Fatalf("run again: %+v", stage)
	}
	found := false
	for _, row := range log.all() {
		found = found || row.action == "gaffer-stage-rebalanced" && row.detail["stage"] == "code"
	}
	if !found {
		t.Fatalf("not recorded: %+v", log.all())
	}
	// Once for a stage: a second stop does not start a third session.
	if err = r.starter.EndOutcomeSession(context.Background(), r.p.UserID, again.SessionID); err != nil {
		t.Fatal(err)
	}
	r.s.rebalanceStopped(context.Background())
	time.Sleep(80 * time.Millisecond)
	if asked := r.starter.asked(); len(asked) != 2 {
		t.Fatalf("launches: %d", len(asked))
	}
}

// A live session can report exhaustion even when its provider has no plan
// figures. Keep the pipeline at Code and restart in its existing worktree.
func TestFactoryWorkerUsageLimit(t *testing.T) {
	for _, automation := range []bool{false, true} {
		name := "manual"
		if automation {
			name = "automatic"
		}
		t.Run(name, func(t *testing.T) {
			r := outcomeFixture(t)
			if _, err := r.s.SaveCrew(r.p.UserID, store.GafferCrew{
				Standard: store.GafferCrewTier{Tool: "opencode", Model: "opencode/primary", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "paid/backup"}, More: []store.GafferStageAgent{{Tool: "codex", Model: "gpt-backup"}}},
			}); err != nil {
				t.Fatal(err)
			}
			in := pipelineInput(r, false)
			in.Automation = automation
			in.Sessions[0].Stages[0] = OutcomeStageInput{Kind: "code"}
			r.s.StartGoalSupervisor()
			created, err := r.s.CreateOutcome(r.p.UserID, in)
			if err != nil {
				t.Fatal(err)
			}
			first := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
			r.settle(t, created.ID)
			if got := r.starter.asked()[0].Model; got != "opencode/primary" {
				t.Fatalf("Code did not launch with the saved Standard model: %q", got)
			}
			goal, err := r.st.GafferGoalFor(r.p.UserID, first.ConversationID)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.st.AppendTerminalChatEventFor(r.p.UserID, first.SessionID, 1, time.Now().UnixMilli(), []byte(`{"kind":"status","state":"error","detail":"Free usage limit reached. Free models work in OpenCode."}`)); err != nil {
				t.Fatal(err)
			}
			r.s.reviewGoal(t.Context(), goal)
			if !automation {
				if views, err := r.s.Goals(r.p.UserID); err != nil || len(views.Goals) != 1 || !views.Goals[0].WorkerUsageLimited {
					t.Fatalf("usage failure not visible: %+v, %v", views, err)
				}
				r.s.rebalanceStopped(t.Context())
				o, err := r.s.Outcome(r.p.UserID, created.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got := o.Sessions[0].Stages[0].NextUsageBackup; !strings.Contains(got, "paid/backup") || o.Sessions[0].SessionID != first.SessionID {
					t.Fatalf("the stopped worker should offer its backup: %+v", o.Sessions[0])
				}
				if _, err = r.s.SwitchPipelineToBackup(r.p.UserID, o.ID, first.Key, first.SessionID, o.Revision); err != nil {
					t.Fatal(err)
				}
			}
			again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool {
				return p.State == "following" && p.SessionID != first.SessionID
			})
			launch := r.starter.asked()[1]
			if again.Stage != first.Stage || launch.Model != "paid/backup" || launch.Dir != first.WorkDir || launch.ProjectID != "" || !strings.Contains(launch.Prompt, "previous worker reached its usage limit") {
				t.Fatalf("backup did not continue the same stage: %+v", launch)
			}
			if old, err := r.st.GafferGoalFor(r.p.UserID, first.ConversationID); err != nil || old.State != "stopped" {
				t.Fatalf("old goal was not retired: %+v, %v", old, err)
			}
			if len(r.starter.asked()) != 2 {
				t.Fatal("the backup was launched more than once")
			}
		})
	}
}
