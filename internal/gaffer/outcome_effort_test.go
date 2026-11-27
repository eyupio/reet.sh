package gaffer

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// The machine's own refusal of an effort its agent did not confirm, as the
// launch path hands it to the controller.
var effortRefusal = EffortNotOffered("The agent did not confirm the chosen effort for this model. Choose an offered effort or Agent default. No opening prompt was sent.")

const highNotOffered = "You chose the effort high; OpenCode does not offer it for this model here, so the session was started on the agent's default effort."

// highAsMedium is the note for a stage put on the nearest lower level offered.
const highAsMedium = "You chose the effort high; OpenCode does not offer it for this model here, so the session was started on medium, the nearest level it offers."

// highDropped is the activity log's reason, written before the start is answered.
const highDropped = "OpenCode does not offer it for this model here."

func effortInput(r *outcomeRig) OutcomeInput {
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint with tests.", Stages: []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder", Effort: "high"},
		{Kind: "review", Tool: "opencode", Model: "strong/reviewer"},
	}}}
	return in
}

// reconnect has the machine report caps, as it does when it connects again.
func (r *outcomeRig) reconnect(t *testing.T, caps terminalproto.Capabilities) {
	t.Helper()
	c, err := r.st.BeginRemoteConnectionFor(r.p.UserID, r.project.Host, strings.Repeat("2", 32), caps)
	if err != nil {
		t.Fatal(err)
	}
	r.starter.mu.Lock()
	r.starter.gen = c.Generation
	r.starter.mu.Unlock()
}

// effortCaps is a machine whose OpenCode descriptor lists options; with none,
// the machine does not describe its agents' options at all.
func effortCaps(startOptions bool, options ...terminalproto.ACPAgentOption) terminalproto.Capabilities {
	agents := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", Options: options}}
	return terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready, Launch: true, Tools: []string{"opencode"},
		ACP: true, ACPLiveControls: true, ACPStartOptions: startOptions, ACPAgentOptions: len(options) > 0, ACPAgents: &agents}
}

func effortLogged(log *auditLog, how string) bool {
	for _, row := range log.all() {
		if row.action == "gaffer-stage-effort-not-offered" && row.detail["stage"] == "code" && row.detail["effort"] == "high" && row.detail["agent"] == "opencode" && row.detail["how"] == how && row.detail["reason"] == highDropped {
			return true
		}
	}
	return false
}

// (a) A stage whose effort the machine's last report shows its agent does not
// offer starts straight away on the nearest lower level the agent lists, or on
// its default when it lists none below, and says so. A machine that offers it,
// or does not say, is asked for it as chosen.
func TestAStageStartsWithoutAnEffortItsMachineSaysIsNotOffered(t *testing.T) {
	for name, c := range map[string]struct {
		caps    terminalproto.Capabilities
		dropped bool
		instead string
	}{
		// A machine whose reet takes no start options is too old: its own
		// refusal, which says to update it, stands.
		"no start options":     {effortCaps(false, terminalproto.ACPAgentOption{ID: "variant", Category: terminalproto.AgentOptionModelConfig, Values: []string{"fast"}}), false, ""},
		"no effort option":     {effortCaps(true, terminalproto.ACPAgentOption{ID: "variant", Category: terminalproto.AgentOptionModelConfig, Values: []string{"fast"}}), true, ""},
		"effort not listed":    {effortCaps(true, terminalproto.ACPAgentOption{ID: "effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"low", "medium"}}), true, "medium"},
		"only heavier listed":  {effortCaps(true, terminalproto.ACPAgentOption{ID: "effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"default", "max"}}), true, ""},
		"effort listed":        {effortCaps(true, terminalproto.ACPAgentOption{ID: "effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"low", "high"}}), false, ""},
		"options not reported": {effortCaps(true), false, ""},
	} {
		t.Run(name, func(t *testing.T) {
			r := outcomeFixture(t)
			log := recordAudit(r.s)
			r.reconnect(t, c.caps)
			r.s.StartGoalSupervisor()
			created, err := r.s.CreateOutcome(r.p.UserID, effortInput(r))
			if err != nil {
				t.Fatal(err)
			}
			code := r.checked(t, created.ID, 0)
			asked := r.starter.asked()
			if len(asked) != 1 {
				t.Fatalf("launches: %+v", asked)
			}
			stage := code
			if !c.dropped {
				if asked[0].Effort != "high" || stage.Effort != "high" || stage.EffortNotOffered != "" || effortLogged(log, "reported") {
					t.Fatalf("an offered effort was dropped: %+v %+v", asked[0], stage)
				}
				return
			}
			if asked[0].Effort != c.instead || asked[0].Model != "cheap/coder" || stage.Effort != c.instead || stage.EffortNotOffered != "high" {
				t.Fatalf("launch %+v stage %+v", asked[0], stage)
			}
			note := highNotOffered
			if c.instead != "" {
				note = highAsMedium
			}
			if stage.ChoiceCheck != "applied" || stage.ChoiceNote != note {
				t.Fatalf("choice: %q %q", stage.ChoiceCheck, stage.ChoiceNote)
			}
			if !effortLogged(log, "reported") {
				t.Fatalf("not logged: %+v", log.all())
			}
		})
	}
}

// (b) A start refused because the agent did not confirm the chosen effort is
// asked for once more without it, on a request of its own, and that session is
// the stage's. (e) The stage keeps what was asked, reads as confirmed with the
// reason, and stays so when the page reads it again later.
func TestAStageRefusedForItsEffortStartsOnTheAgentsDefault(t *testing.T) {
	r := outcomeFixture(t)
	log := recordAudit(r.s)
	r.starter.refusal = func(_ int, in OutcomeLaunch) error {
		if in.Effort != "" {
			return effortRefusal
		}
		return nil
	}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, effortInput(r))
	if err != nil {
		t.Fatal(err)
	}
	stage := r.checked(t, created.ID, 0)
	asked := r.starter.asked()
	if len(asked) != 2 || asked[0].Effort != "high" || asked[1].Effort != "" || asked[1].Model != "cheap/coder" || asked[1].Tool != "opencode" {
		t.Fatalf("launches: %+v", asked)
	}
	if asked[0].RequestID == asked[1].RequestID || stage.RequestID != asked[1].RequestID || r.starter.sessions() != 1 {
		t.Fatalf("the second start must be a request of its own: %q %q stage %q, %d sessions", asked[0].RequestID, asked[1].RequestID, stage.RequestID, r.starter.sessions())
	}
	piece := mustOutcome(t, r, created.ID).Sessions[0]
	if piece.State != "following" || piece.RequestID != asked[1].RequestID || stage.SessionID == "" {
		t.Fatalf("pipeline: %+v", piece)
	}
	if stage.Effort != "" || stage.EffortNotOffered != "high" || stage.ChoiceCheck != "applied" || stage.ChoiceNote != highNotOffered {
		t.Fatalf("stage: %+v", stage)
	}
	if !effortLogged(log, "refused") {
		t.Fatalf("not logged: %+v", log.all())
	}
	for _, row := range log.all() {
		if row.action == "gaffer-stage-choice-not-applied" {
			t.Fatalf("a dropped effort was logged as a mismatch: %+v", row)
		}
	}
	// The session's own figures are compared again on every read: the agent's default effort is no mismatch.
	if err = r.st.RecordTerminalControlsFor(r.p.UserID, stage.SessionID, "cheap/coder", "medium"); err != nil {
		t.Fatal(err)
	}
	o, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	live := o.Sessions[0].Stages[0]
	if live.ChoiceCheck != "applied" || live.ChoiceNote != highNotOffered || live.AppliedEffort != "medium" || live.EffortNotOffered != "high" {
		t.Fatalf("after a read: %+v", live)
	}
	// A model changed in the session is still said to differ, with the effort's reason beside it.
	if err = r.st.RecordTerminalControlsFor(r.p.UserID, stage.SessionID, "other/model", "medium"); err != nil {
		t.Fatal(err)
	}
	if o, err = r.s.Outcome(r.p.UserID, created.ID); err != nil {
		t.Fatal(err)
	}
	if live = o.Sessions[0].Stages[0]; live.ChoiceCheck != "mismatch" || !strings.Contains(live.ChoiceNote, "other/model") || !strings.Contains(live.ChoiceNote, highNotOffered) {
		t.Fatalf("a changed model: %+v", live)
	}
	// The next stage is asked for as chosen: the record is the code stage's own.
	r.s.stageHandedOver(r.p.UserID, piece.ConversationID, "Done.", "gaffer")
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.Stage == 1 && p.State == "following" })
	if got := review.Stages[1]; got.EffortNotOffered != "" || len(r.starter.asked()) != 3 {
		t.Fatalf("review stage: %+v", got)
	}
	// Sent back, the code stage asks for its effort again, so a machine that now offers it gets it; this one still
	// does not, so it is dropped again with exactly one more start.
	if _, err = r.s.SendPipelineBack(r.p.UserID, created.ID, "a", "code", "The export misses a header row."); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool {
		return p.Stage == 0 && p.State == "following" && p.Stages[0].SessionID != "" && p.Stages[0].SessionID != stage.SessionID
	})
	if asked := r.starter.asked(); len(asked) != 5 || asked[3].Effort != "high" || asked[4].Effort != "" || asked[4].RequestID == asked[3].RequestID || again.Stages[0].EffortNotOffered != "high" || again.Stages[0].Effort != "" {
		t.Fatalf("sent back: %+v %+v", asked[3:], again.Stages[0])
	}
}

// (c) The start without the effort is the only second try: refused too, the
// stage stops as before, with that refusal's reason. A pipeline tried again
// afterwards is not asked for the effort again.
func TestAStageRefusedAgainWithoutItsEffortStops(t *testing.T) {
	model := errors.New("The agent did not use the chosen model, so the chat was not started.")
	for name, second := range map[string]error{"wrong model": model, "effort again": effortRefusal} {
		t.Run(name, func(t *testing.T) {
			r := outcomeFixture(t)
			refusing := true
			r.starter.refusal = func(n int, in OutcomeLaunch) error {
				r.starter.mu.Lock()
				defer r.starter.mu.Unlock()
				switch {
				case !refusing:
					return nil
				case n == 0:
					return effortRefusal
				}
				return second
			}
			r.s.StartGoalSupervisor()
			created, err := r.s.CreateOutcome(r.p.UserID, effortInput(r))
			if err != nil {
				t.Fatal(err)
			}
			piece := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
			time.Sleep(50 * time.Millisecond)
			if asked := r.starter.asked(); len(asked) != 2 || asked[0].Effort != "high" || asked[1].Effort != "" || r.starter.sessions() != 0 {
				t.Fatalf("launches: %+v", asked)
			}
			want := "Not started: " + second.Error()
			if piece.Note != want || piece.SessionID != "" {
				t.Fatalf("note %q, want %q", piece.Note, want)
			}
			if stage := piece.Stages[0]; stage.Effort != "" || stage.EffortNotOffered != "high" || stage.RequestID != r.starter.asked()[1].RequestID {
				t.Fatalf("stage: %+v", stage)
			}
			// Tried again, the stage asks for its effort again: the machine may
			// have been updated. "wrong model": it now takes the effort, and the
			// record goes. "effort again": still refused, so it is dropped again
			// with exactly one more start.
			updated := name == "wrong model"
			r.starter.mu.Lock()
			refusing = false
			r.starter.refusal = func(_ int, in OutcomeLaunch) error {
				if in.Effort != "" && !updated {
					return effortRefusal
				}
				return nil
			}
			r.starter.mu.Unlock()
			if _, err = r.s.RetryPipeline(r.p.UserID, created.ID, "a", mustOutcome(t, r, created.ID).Revision); err != nil {
				t.Fatal(err)
			}
			stage := r.checked(t, created.ID, 0)
			asked := r.starter.asked()
			if updated {
				if len(asked) != 3 || asked[2].Effort != "high" || stage.Effort != "high" || stage.EffortNotOffered != "" || stage.ChoiceCheck != "applied" || stage.ChoiceNote != "" {
					t.Fatalf("tried again on an updated machine: %+v %+v", asked[2:], stage)
				}
				return
			}
			if len(asked) != 4 || asked[2].Effort != "high" || asked[3].Effort != "" || asked[3].RequestID == asked[2].RequestID || stage.Effort != "" || stage.EffortNotOffered != "high" || stage.ChoiceNote != highNotOffered {
				t.Fatalf("tried again: %+v %+v", asked[2:], stage)
			}
		})
	}
}

// (d) A launch that asked for no effort is never asked for again: whatever
// the machine said, the stage stops on its first refusal.
func TestAStageWithNoEffortIsNotAskedForTwice(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.refusal = func(int, OutcomeLaunch) error { return effortRefusal }
	in := effortInput(r)
	in.Sessions[0].Stages[0].Effort = ""
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	piece := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	time.Sleep(50 * time.Millisecond)
	if asked := r.starter.asked(); len(asked) != 1 || piece.Stages[0].EffortNotOffered != "" || !strings.Contains(piece.Note, "did not confirm the chosen effort") {
		t.Fatalf("launches %+v pipeline %+v", asked, piece)
	}
}

// A refusal that is not the typed effort refusal is not taken for an effort
// the agent does not offer, even when its words are the same: the stage stops
// as before, after exactly one start, and keeps its effort.
func TestAStageRefusedForAnotherReasonIsNotAskedForAgain(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.refusal = func(int, OutcomeLaunch) error {
		return errors.New("The agent did not confirm the chosen effort for this model.")
	}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, effortInput(r))
	if err != nil {
		t.Fatal(err)
	}
	piece := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	time.Sleep(50 * time.Millisecond)
	if asked := r.starter.asked(); len(asked) != 1 || piece.Stages[0].Effort != "high" || piece.Stages[0].EffortNotOffered != "" {
		t.Fatalf("launches %+v pipeline %+v", asked, piece)
	}
}

// (f) A stage that starts on its backup is handled the same way: the
// backup's effort, refused, is dropped and recorded, and the stage keeps its
// first choice and the reason it moved.
func TestABackupsEffortThatIsNotOfferedIsDroppedAndRecorded(t *testing.T) {
	for name, byCaps := range map[string]bool{"refused": false, "reported": true} {
		t.Run(name, func(t *testing.T) {
			r := outcomeFixture(t)
			log := recordAudit(r.s)
			now := time.Now()
			if err := r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 20, ""), window("week", 91, "cheap"))}, now.UnixMilli()); err != nil {
				t.Fatal(err)
			}
			if byCaps {
				r.reconnect(t, effortCaps(true, terminalproto.ACPAgentOption{ID: "effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"low", "medium"}}))
			} else {
				r.starter.refusal = func(_ int, in OutcomeLaunch) error {
					if in.Effort != "" {
						return effortRefusal
					}
					return nil
				}
			}
			in := r.in
			in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint.", Stages: []OutcomeStageInput{
				{Kind: "code", Tool: "opencode", Model: "cheap/coder", Effort: "low", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "other/coder", Effort: "high"}},
			}}}
			r.s.StartGoalSupervisor()
			created, err := r.s.CreateOutcome(r.p.UserID, in)
			if err != nil {
				t.Fatal(err)
			}
			stage := r.checked(t, created.ID, 0)
			asked := r.starter.asked()
			last := asked[len(asked)-1]
			// Reported, it starts on medium, the nearest level listed; refused, on the default.
			instead, note := "", highNotOffered
			if byCaps {
				instead, note = "medium", highAsMedium
			}
			if last.Model != "other/coder" || last.Effort != instead || byCaps && len(asked) != 1 || !byCaps && (len(asked) != 2 || asked[0].Effort != "high") {
				t.Fatalf("launches: %+v", asked)
			}
			if stage.Model != "other/coder" || stage.Effort != instead || stage.EffortNotOffered != "high" || stage.First == nil || stage.First.Effort != "low" || stage.Balanced == "" {
				t.Fatalf("stage: %+v", stage)
			}
			if stage.ChoiceCheck != "applied" || stage.ChoiceNote != note {
				t.Fatalf("choice: %q %q", stage.ChoiceCheck, stage.ChoiceNote)
			}
			how := "refused"
			if byCaps {
				how = "reported"
			}
			if !effortLogged(log, how) {
				t.Fatalf("not logged: %+v", log.all())
			}
		})
	}
}

// Balancing takes a dropped effort for the one the person asked for: a stage
// that stays on its first choice is left as it is, record and all, and one
// moved to its backup keeps the asked effort with its first choice and
// leaves the record behind, since it was made for the other agent.
func TestBalancingTreatsADroppedEffortAsTheOneAskedFor(t *testing.T) {
	r := outcomeFixture(t)
	stage := store.GafferOutcomeStage{Kind: "code", Tool: "opencode", Model: "cheap/coder", EffortNotOffered: "high", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "other/coder"}}
	o := store.GafferOutcome{OwnerID: r.p.UserID, AgentID: r.project.Host}
	if _, changed := r.s.balanceStage(o, stage); changed {
		t.Fatal("a stage on its first choice with a dropped effort was changed by balancing")
	}
	now := time.Now()
	if err := r.st.SaveMachineAccountsFor(r.p.UserID, r.project.Host, []terminalproto.AgentAccount{planAccount("opencode", now, window("week", 20, ""), window("week", 91, "cheap"))}, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	moved, changed := r.s.balanceStage(o, stage)
	if !changed || moved.Model != "other/coder" || moved.First == nil || moved.First.Effort != "high" || moved.EffortNotOffered != "" {
		t.Fatalf("moved to the backup: %v %+v", changed, moved)
	}
	// Already on its backup, whose effort was dropped, with the plan figures
	// moved on since: the sentence about them is no reason to ask for the
	// backup's effort, and be refused it, again.
	onBackup := store.GafferOutcomeStage{Kind: "code", Tool: "opencode", Model: "other/coder", EffortNotOffered: "low", Backup: &store.GafferStageAgent{Tool: "opencode", Model: "other/coder", Effort: "low"},
		First: &store.GafferStageAgent{Tool: "opencode", Model: "cheap/coder", Effort: "high"}, Balanced: "OpenCode (cheap/coder)’s plan had 15% left, so this stage runs on OpenCode (other/coder), which had 85% left."}
	if again, changed := r.s.balanceStage(o, onBackup); changed {
		t.Fatalf("a stage on its backup was changed for its plan figures alone: %+v", again)
	}
}

// (M10) A machine that says nothing either way keeps the effort: nothing is
// stored for it, it has no chats, it does not list the agent as a chat, it
// lists no agents, or its reet takes no start options at all.
func TestAnEffortIsKeptWhenTheMachineDoesNotSay(t *testing.T) {
	thought := terminalproto.ACPAgentOption{ID: "effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"low"}}
	r := outcomeFixture(t)
	if n, _ := r.s.effortNotReported(r.p.UserID, strings.Repeat("9", 48), "opencode", "high"); n {
		t.Error("a machine with nothing stored")
	}
	if n, _ := r.s.effortNotReported(r.p.UserID, r.project.Host, "opencode", ""); n {
		t.Error("no effort asked for")
	}
	noChats := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready}
	noAgents := effortCaps(true)
	noAgents.ACPAgents = nil
	for name, c := range map[string]struct {
		caps terminalproto.Capabilities
		tool string
	}{
		"no chats":          {noChats, "opencode"},
		"agent not a chat":  {effortCaps(true, thought), "claude"},
		"no agent list":     {noAgents, "opencode"},
		"no start options":  {effortCaps(false, thought), "opencode"},
		"options not given": {effortCaps(true), "opencode"},
	} {
		r.reconnect(t, c.caps)
		if n, _ := r.s.effortNotReported(r.p.UserID, r.project.Host, c.tool, "high"); n {
			t.Errorf("%s: the effort was dropped", name)
		}
	}
	r.reconnect(t, effortCaps(true, thought))
	if n, instead := r.s.effortNotReported(r.p.UserID, r.project.Host, "opencode", "high"); !n || instead != "low" {
		t.Error("a listed thought level without this one did not drop it for the nearest lower one", n, instead)
	}
}

// (M4) A stage whose effort was already dropped can be answered with the
// refusal recorded for its old request. It is asked for once more on a request
// of its own, and starts.
func TestAStageWithADroppedEffortRefusedOnItsOldRequestStartsOnANewOne(t *testing.T) {
	r := outcomeFixture(t)
	r.reconnect(t, effortCaps(true, terminalproto.ACPAgentOption{ID: "effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"low"}}))
	r.starter.refusal = func(n int, _ OutcomeLaunch) error {
		if n == 0 {
			return effortRefusal
		}
		return nil
	}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, effortInput(r))
	if err != nil {
		t.Fatal(err)
	}
	stage := r.checked(t, created.ID, 0)
	asked := r.starter.asked()
	// The report puts it on low, the nearest level listed; refused for that too, it starts on the default.
	if len(asked) != 2 || asked[0].Effort != "low" || asked[1].Effort != "" || asked[0].RequestID == asked[1].RequestID || stage.RequestID != asked[1].RequestID || stage.EffortNotOffered != "high" || stage.Effort != "" {
		t.Fatalf("launches %+v stage %+v", asked, stage)
	}
}

// (M10) The start without the effort can go unanswered in time. Its request
// is then the one kept as unsure, and trying again takes up the session it
// started instead of starting another.
func TestALateAnswerToTheStartWithoutTheEffortIsTakenUp(t *testing.T) {
	wait := outcomeStartWait
	outcomeStartWait = 150 * time.Millisecond
	t.Cleanup(func() { outcomeStartWait = wait })
	r := outcomeFixture(t)
	r.starter.late = "Export API · code"
	r.starter.refusal = func(_ int, in OutcomeLaunch) error {
		if in.Effort != "" {
			return effortRefusal
		}
		return nil
	}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, effortInput(r))
	if err != nil {
		t.Fatal(err)
	}
	failed := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "failed" })
	asked := r.starter.asked()
	if len(asked) != 2 || failed.UnsureRequest != asked[1].RequestID || r.starter.sessions() != 1 {
		t.Fatalf("after the late answer: %+v %+v", asked, failed)
	}
	o := r.settle(t, created.ID)
	outcomeStartWait = wait
	if _, err = r.s.RetryPipeline(r.p.UserID, created.ID, "a", o.Revision); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	asked = r.starter.asked()
	if r.starter.sessions() != 1 || len(asked) != 3 || asked[2].RequestID != asked[1].RequestID || again.UnsureRequest != "" || again.Stages[0].EffortNotOffered != "high" {
		t.Fatalf("tried again: %d sessions, %+v %+v", r.starter.sessions(), asked, again)
	}
}

func TestNearestEffortStepsDownOnly(t *testing.T) {
	for _, c := range []struct {
		asked   string
		offered []string
		want    string
	}{
		{"max", []string{"low", "medium", "high"}, "high"},
		{"high", []string{"low", "xhigh"}, "low"},
		{"high", []string{"xhigh", "max"}, ""},
		{"low", []string{"none", "medium"}, ""},
		{"ultra", []string{"high"}, ""},
	} {
		if got := nearestEffort(c.asked, c.offered); got != c.want {
			t.Errorf("%s from %v: %q, want %q", c.asked, c.offered, got, c.want)
		}
	}
}

func TestStageChoiceCheckSaysWhenTheEffortWasNotOffered(t *testing.T) {
	stage := store.GafferOutcomeStage{Tool: "opencode", Model: "cheap/coder", EffortNotOffered: "high"}
	if check, note := stageChoiceCheck(stage, SessionChoice{Tool: "opencode", Model: "cheap/coder", Effort: "medium"}); check != "applied" || note != highNotOffered {
		t.Fatalf("%s %q", check, note)
	}
	if check, note := stageChoiceCheck(stage, SessionChoice{Tool: "opencode", Model: "cheap/coder"}); check != "applied" || note != highNotOffered {
		t.Fatalf("no effort reported: %s %q", check, note)
	}
	if check, note := stageChoiceCheck(stage, SessionChoice{Tool: "claude", Model: "cheap/coder"}); check != "mismatch" || !strings.HasSuffix(note, highNotOffered) || !strings.Contains(note, "agent opencode") {
		t.Fatalf("another agent: %s %q", check, note)
	}
	if got := effortNotOfferedNote("claude", "max", ""); got != "You chose the effort max; Claude Code does not offer it for this model here, so the session was started on the agent's default effort." {
		t.Fatal(got)
	}
	// Put on the nearest lower level, the session on that level reads as applied.
	stage.Effort = "medium"
	if check, note := stageChoiceCheck(stage, SessionChoice{Tool: "opencode", Model: "cheap/coder", Effort: "medium"}); check != "applied" || note != highAsMedium {
		t.Fatalf("nearest level: %s %q", check, note)
	}
}
