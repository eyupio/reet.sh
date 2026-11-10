package gaffer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// outcomeStarterFake starts real session rows on one enrolled machine, the
// way the launch path would, and keeps what it was asked for.
type outcomeStarterFake struct {
	t        *testing.T
	st       *store.Store
	agent    string
	gen      int64
	mu       sync.Mutex
	launches []OutcomeLaunch
	refuse   map[string]string
	hold     chan struct{}
	entered  chan struct{}
	once     sync.Once
}

func (f *outcomeStarterFake) StartOutcomeSession(ctx context.Context, uid int64, in OutcomeLaunch) (string, string, error) {
	f.mu.Lock()
	f.launches = append(f.launches, in)
	hold, refusal := f.hold, f.refuse[in.Title]
	f.mu.Unlock()
	if f.entered != nil {
		f.once.Do(func() { close(f.entered) })
	}
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}
	if refusal != "" {
		return "", "", errors.New(refusal)
	}
	row, _, err := f.st.ReserveTerminalCreateFor(uid, in.AgentID, in.RequestID, f.gen, terminalproto.Create{Title: in.Title, Tool: in.Tool, Structured: true, Dir: in.Dir, Prompt: in.Prompt, Cols: 100, Rows: 30})
	if err != nil {
		return "", "", err
	}
	if row.State != terminalproto.Running {
		if err = f.st.ObserveTerminalStateFor(uid, in.AgentID, row.ID, 1, terminalproto.Running, nil); err != nil {
			return "", "", err
		}
	}
	// The first launch of a project gets a worktree; later stages are sent there.
	work := in.Dir
	if in.ProjectID != "" {
		work = in.Dir + "/.reet/worktrees/" + in.RequestID[:8]
	}
	return row.ID, work, nil
}

func (f *outcomeStarterFake) asked() []OutcomeLaunch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]OutcomeLaunch(nil), f.launches...)
}

type outcomeRig struct {
	s       *Service
	st      *store.Store
	p       Principal
	starter *outcomeStarterFake
	project store.FolderProject
	in      OutcomeInput
	reply   func(http.ResponseWriter, *http.Request)
}

func outcomeFixture(t *testing.T) *outcomeRig {
	t.Helper()
	outcomeFollowEvery = 10 * time.Millisecond
	s, st, p := settingsFixture(t)
	rig := &outcomeRig{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rig.reply == nil {
			t.Error("an outcome must not call the provider to start sessions")
			return
		}
		rig.reply(w, r)
	}))
	t.Cleanup(provider.Close)
	if _, err := s.Save(p, Input{Name: "Outcome brains", Kind: "openai_compatible", BaseURL: provider.URL, LocalOnly: true, Enabled: true, MakeDefault: true, Model: "fake-model"}); err != nil {
		t.Fatal(err)
	}
	agent, _, err := st.EnrollSyncAgent(p.UserID, "outcome-machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps, err := st.BeginRemoteConnectionFor(p.UserID, agent.ID, strings.Repeat("1", 32), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready, ACP: true, ACPLiveControls: true})
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateFolderProjectFor(p.UserID, store.FolderProject{Host: agent.ID, Dir: "/work/billing", Name: "billing", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	starter := &outcomeStarterFake{t: t, st: st, agent: agent.ID, gen: caps.Generation, refuse: map[string]string{}}
	s.SetWorker(&goalWorker{fakeWorker: fakeWorker{st: st}})
	s.SetSessionStarter(starter)
	in := OutcomeInput{Objective: "Ship CSV export for billing.", ProjectID: project.ID, Tool: "opencode", MaxReviews: 8, Hours: 24, Confirmed: true, Sessions: []OutcomeSession{
		{Title: "Export API", Prompt: "Add the CSV export endpoint with tests. Raise a pull request. Do not merge."},
		{Title: "Export button", Prompt: "Add the export button to the billing page. Raise a pull request. Do not merge."},
	}}
	rig.s, rig.st, rig.p, rig.starter, rig.project, rig.in = s, st, p, starter, project, in
	return rig
}

func (r *outcomeRig) settle(t *testing.T, id string) store.GafferOutcome {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		o, err := r.s.Outcome(r.p.UserID, id)
		if err != nil {
			t.Fatal(err)
		}
		if o.State != "starting" {
			return o
		}
		if time.Now().After(deadline) {
			t.Fatalf("outcome never settled: %+v", o)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOutcomeStartsAndFollowsItsApprovedSessionsWithoutABrowser(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	r.in.Model, r.in.Trust = "provider/model-a", true
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	if created.State != "starting" || len(created.Sessions) != 2 || created.AgentID != r.project.Host || created.Dir != "/work/billing" {
		t.Fatalf("created: %+v", created)
	}
	o := r.settle(t, created.ID)
	if o.State != "settled" {
		t.Fatalf("outcome: %+v", o)
	}
	asked := r.starter.asked()
	if len(asked) != 2 {
		t.Fatalf("launches: %+v", asked)
	}
	for i, launch := range asked {
		want := r.in.Sessions[i]
		// The machine, folder, project and agent are the approved ones; the model chose none of them.
		if launch.Title != want.Title || launch.Prompt != want.Prompt || launch.AgentID != r.project.Host || launch.Dir != "/work/billing" || launch.ProjectID != r.project.ID || launch.Tool != "opencode" || launch.Model != "provider/model-a" || !launch.Trust || !terminalproto.ValidID(launch.RequestID) {
			t.Fatalf("launch %d: %+v", i, launch)
		}
		item := o.Sessions[i]
		if item.State != "following" || item.SessionID == "" || item.ConversationID == "" {
			t.Fatalf("session %d: %+v", i, item)
		}
		goal, err := r.st.GafferGoalFor(r.p.UserID, item.ConversationID)
		if err != nil || goal.State != "active" || goal.SessionID != item.SessionID || goal.MaxReviews != 8 {
			t.Fatalf("goal %d: %+v %v", i, goal, err)
		}
		if !strings.HasPrefix(goal.Text, "Outcome: Ship CSV export for billing.\nThis session’s part: "+want.Title+"\n") || !strings.HasSuffix(goal.Text, "Do not merge.") || len(goal.Text) > 2000 {
			t.Fatalf("goal text %d: %q", i, goal.Text)
		}
	}
	if asked[0].RequestID == asked[1].RequestID {
		t.Fatal("sessions shared a request ID")
	}
	list, err := r.s.Outcomes(r.p.UserID)
	if err != nil || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list: %+v %v", list, err)
	}
	// Another account sees and stops nothing.
	other, err := r.st.CreateUser("other-outcome@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := r.s.Outcomes(other.ID); err != nil || len(rows) != 0 {
		t.Fatalf("other account's list: %+v %v", rows, err)
	}
	if _, err := r.s.Outcome(other.ID, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("other account's read: %v", err)
	}
}

func TestOutcomeReportsEachSessionAndCarriesOnPastARefusal(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.refuse["Export API"] = "That agent isn't installed on that machine."
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	o := r.settle(t, created.ID)
	first, second := o.Sessions[0], o.Sessions[1]
	if first.State != "failed" || first.SessionID != "" || first.Note != "Not started: That agent isn't installed on that machine." {
		t.Fatalf("refused session: %+v", first)
	}
	if second.State != "following" || second.SessionID == "" {
		t.Fatalf("second session: %+v", second)
	}
	if len(r.starter.asked()) != 2 {
		t.Fatal("a refused launch was retried or the next was skipped")
	}
}

func TestOutcomeIsRefusedWholeBeforeAnythingStarts(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	change := func(f func(*OutcomeInput)) OutcomeInput {
		in := r.in
		in.Sessions = append([]OutcomeSession(nil), r.in.Sessions...)
		f(&in)
		return in
	}
	six := make([]OutcomeSession, 6)
	for i := range six {
		six[i] = OutcomeSession{Title: "Part " + string(rune('A'+i)), Prompt: "Do the work. Do not merge."}
	}
	cases := map[string]OutcomeInput{
		"not approved":        change(func(in *OutcomeInput) { in.Confirmed = false }),
		"empty objective":     change(func(in *OutcomeInput) { in.Objective = "  " }),
		"unknown project":     change(func(in *OutcomeInput) { in.ProjectID = "missing" }),
		"terminal tool":       change(func(in *OutcomeInput) { in.Tool = "bash" }),
		"model as a flag":     change(func(in *OutcomeInput) { in.Model = "--yolo" }),
		"trust for no reason": change(func(in *OutcomeInput) { in.Tool, in.Trust = "claude", true }),
		"fractional reviews":  change(func(in *OutcomeInput) { in.MaxReviews = 0 }),
		"too many hours":      change(func(in *OutcomeInput) { in.Hours = 169 }),
		"no sessions":         change(func(in *OutcomeInput) { in.Sessions = nil }),
		"six sessions":        change(func(in *OutcomeInput) { in.Sessions = six }),
		"repeated title":      change(func(in *OutcomeInput) { in.Sessions[1].Title = " Export API " }),
		"empty prompt":        change(func(in *OutcomeInput) { in.Sessions[1].Prompt = " " }),
		"prompt as a flag":    change(func(in *OutcomeInput) { in.Sessions[1].Prompt = "--dangerously-skip-permissions" }),
		"control in a prompt": change(func(in *OutcomeInput) { in.Sessions[1].Prompt = "Do it\x00now" }),
	}
	for name, in := range cases {
		var bad *Invalid
		if _, err := r.s.CreateOutcome(r.p.UserID, in); !errors.As(err, &bad) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	other, err := r.st.CreateUser("viewer-outcome@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.CreateOutcome(other.ID, r.in); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("viewer: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if rows, _ := r.s.Outcomes(r.p.UserID); len(rows) != 0 || len(r.starter.asked()) != 0 {
		t.Fatalf("a refused outcome left %d records and %d launches", len(rows), len(r.starter.asked()))
	}
	// Without the launch path or the worker channel there is nothing to hand an outcome to.
	bare, _, p := settingsFixture(t)
	bare.StartGoalSupervisor()
	if _, err := bare.CreateOutcome(p.UserID, r.in); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no starter: %v", err)
	}
}

func TestStoppingAnOutcomeLeavesUnstartedSessionsUnstarted(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.hold, r.starter.entered = make(chan struct{}), make(chan struct{})
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	<-r.starter.entered
	if _, err := r.s.StopOutcome(r.p.UserID, created.ID, created.Revision); !errors.Is(err, store.ErrGafferOutcomeBusy) {
		t.Fatalf("stale stop: %v", err)
	}
	current, err := r.s.Outcome(r.p.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := r.s.StopOutcome(r.p.UserID, created.ID, current.Revision)
	if err != nil || stopped.State != "stopped" {
		t.Fatalf("stop: %+v %v", stopped, err)
	}
	close(r.starter.hold)
	// The session already being started finishes and is followed; the next is never asked for.
	deadline := time.Now().Add(10 * time.Second)
	for {
		o, err := r.s.Outcome(r.p.UserID, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if o.Sessions[0].State == "following" {
			if o.State != "stopped" || o.Sessions[1].State != "cancelled" || o.Sessions[1].SessionID != "" {
				t.Fatalf("stopped outcome: %+v", o)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("first session never settled: %+v", o)
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(r.starter.asked()); n != 1 {
		t.Fatalf("launches after stop: %d", n)
	}
}

func TestOutcomeResumesAfterAControllerRestartWithTheSameRequest(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.hold, r.starter.entered = make(chan struct{}), make(chan struct{})
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	<-r.starter.entered
	// The controller stops while the first launch is in flight: nothing is marked failed.
	r.s.Close()
	mid, err := r.st.GafferOutcomeFor(r.p.UserID, created.ID)
	if err != nil || mid.State != "starting" || mid.Sessions[0].State != "starting" || mid.Sessions[1].State != "waiting" {
		t.Fatalf("after shutdown: %+v %v", mid, err)
	}
	seal, err := sealed.NewSealer(strings.Repeat("test-key-", 8), "test")
	if err != nil {
		t.Fatal(err)
	}
	next := New(r.st, seal)
	t.Cleanup(next.Close)
	starter := &outcomeStarterFake{t: t, st: r.st, agent: r.starter.agent, gen: r.starter.gen, refuse: map[string]string{}}
	next.SetWorker(&goalWorker{fakeWorker: fakeWorker{st: r.st}})
	next.SetSessionStarter(starter)
	next.StartGoalSupervisor()
	r.s = next
	o := r.settle(t, created.ID)
	if o.State != "settled" || o.Sessions[0].State != "following" || o.Sessions[1].State != "following" {
		t.Fatalf("after restart: %+v", o)
	}
	before, after := r.starter.asked(), starter.asked()
	if len(before) != 1 || len(after) != 2 || after[0].RequestID != before[0].RequestID {
		t.Fatalf("the in-flight launch was not retried under its own request: %+v then %+v", before, after)
	}
}

func TestARestoredCopyStartsNoMoreSessions(t *testing.T) {
	r := outcomeFixture(t)
	// No supervisor: the record waits, as it would in a copy taken mid-start.
	if _, err := r.s.CreateOutcome(r.p.UserID, r.in); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("create without a running supervisor: %v", err)
	}
	o, err := r.st.CreateGafferOutcomeFor(r.p.UserID, store.GafferOutcome{ID: strings.Repeat("9", 32), Objective: "Ship it.", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it. Do not merge.", RequestID: strings.Repeat("8", 32)}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.st.UpdateGafferOutcomeFor(r.p.UserID, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Note = "stopped", "This controller was restored from a copy."
		return true
	}); err != nil {
		t.Fatal(err)
	}
	r.s.StartGoalSupervisor()
	time.Sleep(100 * time.Millisecond)
	view, err := r.s.Outcome(r.p.UserID, o.ID)
	if err != nil || view.State != "stopped" || view.Sessions[0].State != "cancelled" || len(r.starter.asked()) != 0 {
		t.Fatalf("restored outcome: %+v %v launches=%d", view, err, len(r.starter.asked()))
	}
}

func TestOutcomeGoalFitsTheGoalLimitAndKeepsItsLastSentence(t *testing.T) {
	goal := OutcomeGoal(strings.Repeat("é", 4000), strings.Repeat("T", 500))
	if len(goal) > 2000 || !strings.HasSuffix(goal, "Do not merge.") || !strings.HasPrefix(goal, "Outcome: é") || !store.ValidWorkerText(goal) {
		t.Fatalf("goal: %d bytes %q", len(goal), goal[len(goal)-40:])
	}
	if got := OutcomeGoal("Ship  it\n now", "API"); !strings.HasPrefix(got, "Outcome: Ship it now\nThis session’s part: API\n") {
		t.Fatalf("goal: %q", got)
	}
}

func (r *outcomeRig) piece(t *testing.T, id string, want func(store.GafferOutcomeSession) bool) store.GafferOutcomeSession {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		o, err := r.st.GafferOutcomeFor(r.p.UserID, id)
		if err != nil {
			t.Fatal(err)
		}
		if want(o.Sessions[0]) {
			return o.Sessions[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("pipeline never got there: %+v", o.Sessions[0])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func pipelineInput(r *outcomeRig, auto bool) OutcomeInput {
	in := r.in
	in.AutoMerge = auto
	in.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Add the CSV export endpoint with tests.", Stages: []OutcomeStageInput{
		{Kind: "code", Tool: "opencode", Model: "cheap/coder"},
		{Kind: "review", Tool: "opencode", Model: "strong/reviewer"},
		{Kind: "test", Tool: "opencode", Model: "strong/reviewer"},
		{Kind: "merge", Tool: "opencode", Model: "strong/reviewer"},
	}}}
	return in
}

// A pipeline moves through its stages on hand-overs: a stage done by another
// agent or model gets its own session in the same folder, one done by the same
// keeps the session, and the merge stage waits for the person.
func TestPipelineHandsOverStageByStageAndWaitsForTheMerge(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if code.Stage != 0 || code.Stages[0].State != "running" || code.WorkDir == "" || !strings.Contains(code.WorkDir, "/.reet/worktrees/") {
		t.Fatalf("code stage: %+v", code)
	}
	goal, err := r.st.GafferGoalFor(r.p.UserID, code.ConversationID)
	if err != nil || !strings.Contains(goal.Text, "\nStage: code\n") || !strings.HasSuffix(goal.Text, "Do not merge.") || len(goal.Text) > 2000 {
		t.Fatalf("code goal: %q %v", goal.Text, err)
	}
	if !r.s.pipelineStage(r.p.UserID, code.ConversationID) {
		t.Fatal("the stage goal was not recognised as a pipeline stage")
	}
	// Code is handed over to a different model: a new session in the first one's folder, with the handover.
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Endpoint and tests are committed on the branch.")
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	asked := r.starter.asked()
	if len(asked) != 2 || review.SessionID == code.SessionID || review.Stages[0].State != "done" || review.Stages[0].Summary == "" {
		t.Fatalf("review stage: %+v launches=%d", review, len(asked))
	}
	second := asked[1]
	if second.Dir != code.WorkDir || second.ProjectID != "" || second.Model != "strong/reviewer" || second.RequestID != review.Stages[1].RequestID || !strings.Contains(second.Prompt, "Handover from the code stage: Endpoint and tests are committed") || !strings.Contains(second.Prompt, "Your stage: review.") || !strings.HasSuffix(second.Prompt, "Do not merge.") {
		t.Fatalf("review launch: %+v", second)
	}
	if old, err := r.st.GafferGoalFor(r.p.UserID, code.ConversationID); err != nil || old.State != "stopped" {
		t.Fatalf("the finished stage's goal: %+v %v", old, err)
	}
	// Review to test is the same agent and model: the session carries on under a fresh goal.
	r.s.stageHandedOver(r.p.UserID, review.ConversationID, "Two fixes committed; nothing needs a decision.")
	test := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	if test.SessionID != review.SessionID || test.ConversationID != review.ConversationID || len(r.starter.asked()) != 2 {
		t.Fatalf("test stage should keep the session: %+v", test)
	}
	if g, err := r.st.GafferGoalFor(r.p.UserID, test.ConversationID); err != nil || g.State != "active" || !strings.Contains(g.Text, "\nStage: test\n") || g.UsedReviews != 0 {
		t.Fatalf("test goal: %+v %v", g, err)
	}
	// Test is done. Merge was not allowed, so nothing more starts until the person says so.
	r.s.stageHandedOver(r.p.UserID, test.ConversationID, "All checks pass locally.")
	ready := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	time.Sleep(60 * time.Millisecond)
	if ready.Stage != 3 || ready.Stages[3].State != "pending" || len(r.starter.asked()) != 2 {
		t.Fatalf("ready to merge: %+v", ready)
	}
	o, _ := r.s.Outcome(r.p.UserID, created.ID)
	if o.State != "settled" {
		t.Fatalf("outcome while waiting for the merge decision: %s", o.State)
	}
	other, err := r.st.CreateUser("merge-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.ApproveMerge(other.ID, created.ID, ready.Key, o.Revision); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another account's approval: %v", err)
	}
	if _, err := r.s.ApproveMerge(r.p.UserID, created.ID, ready.Key, o.Revision-1); !errors.Is(err, store.ErrGafferOutcomeBusy) {
		t.Fatalf("stale approval: %v", err)
	}
	if _, err := r.s.ApproveMerge(r.p.UserID, created.ID, ready.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	merge := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 3 })
	g, err := r.st.GafferGoalFor(r.p.UserID, merge.ConversationID)
	if err != nil || !strings.Contains(g.Text, "\nStage: merge\n") || strings.Contains(g.Text, "Do not merge") || !strings.Contains(g.Text, "only once every check passes") {
		t.Fatalf("merge goal: %q %v", g.Text, err)
	}
	if merge.SessionID != test.SessionID {
		t.Fatalf("merge should keep the session: %+v", merge)
	}
	r.s.stageHandedOver(r.p.UserID, merge.ConversationID, "Merged.")
	done := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "done" })
	if done.Stages[3].State != "done" {
		t.Fatalf("done: %+v", done)
	}
	if _, err := r.s.ApproveMerge(r.p.UserID, created.ID, done.Key, mustOutcome(t, r, created.ID).Revision); err == nil {
		t.Fatal("a finished pipeline accepted a merge approval")
	}
}

func mustOutcome(t *testing.T, r *outcomeRig, id string) store.GafferOutcome {
	t.Helper()
	o, err := r.st.GafferOutcomeFor(r.p.UserID, id)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestAPipelineAllowedToMergeGoesStraightToItsMergeStage(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := pipelineInput(r, true)
	in.Sessions[0].Stages = []OutcomeStageInput{{Kind: "code", Tool: "opencode"}, {Kind: "merge", Tool: "opencode"}}
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Done.")
	merge := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	if !merge.AutoMerge || merge.MergeApproved {
		t.Fatalf("merge: %+v", merge)
	}
	// One pipeline can still refuse what the outcome allows.
	no := false
	in.Sessions[0].AutoMerge = &no
	in.Sessions[0].Title = "Second"
	second, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	c2 := r.piece(t, second.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.s.stageHandedOver(r.p.UserID, c2.ConversationID, "Done.")
	r.piece(t, second.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
}

func TestPipelineStagesMustBeInOrderWithAnAgentThatCanChat(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	bad := map[string][]OutcomeStageInput{
		"out of order":  {{Kind: "review", Tool: "opencode"}, {Kind: "code", Tool: "opencode"}},
		"twice":         {{Kind: "code", Tool: "opencode"}, {Kind: "code", Tool: "opencode"}},
		"unknown stage": {{Kind: "deploy", Tool: "opencode"}},
		"terminal tool": {{Kind: "code", Tool: "bash"}},
		"model flag":    {{Kind: "code", Tool: "opencode", Model: "--yolo"}},
	}
	for name, stages := range bad {
		in := pipelineInput(r, false)
		in.Sessions[0].Stages = stages
		var invalid *Invalid
		if _, err := r.s.CreateOutcome(r.p.UserID, in); !errors.As(err, &invalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if len(r.starter.asked()) != 0 {
		t.Fatal("a refused pipeline started a session")
	}
}

// Gaffer's own review hands a stage over: its model says the stage is done,
// and the controller, not the model, starts what the person approved next.
func TestAGoalReviewHandsAPipelineStageOver(t *testing.T) {
	r := outcomeFixture(t)
	var mu sync.Mutex
	var sent []string
	seen := func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), sent...) }
	r.reply = func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		mu.Lock()
		sent = append(sent, string(raw))
		mu.Unlock()
		call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "h1", "type": "function", "function": map[string]any{"name": "hand_over", "arguments": `{"summary":"Endpoint and tests are in."}`}}}}, "finish_reason": "tool_calls"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", call)
	}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	if err := r.st.AppendTerminalChatEventFor(r.p.UserID, code.SessionID, 1, time.Now().UnixMilli(), []byte(`{"kind":"agent","text":"Implemented the endpoint; tests pass."}`)); err != nil {
		t.Fatal(err)
	}
	// The supervisor may already have looked and found nothing new; make the goal due again.
	deadline := time.Now().Add(10 * time.Second)
	for len(seen()) == 0 && time.Now().Before(deadline) {
		if g, err := r.st.GafferGoalFor(r.p.UserID, code.ConversationID); err == nil && g.State == "active" && g.ActiveTurnID == "" {
			_ = r.st.ObserveGafferGoal(g, "active", "", 0)
		}
		r.s.wakeGoals()
		time.Sleep(50 * time.Millisecond)
	}
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	tools := seen()
	// What Gaffer's model used on the stage is kept with the stage.
	if review.Stages[0].Reviews != 1 {
		t.Fatalf("stage usage: %+v", review.Stages[0])
	}
	if review.Stages[0].Summary != "Endpoint and tests are in." || len(tools) == 0 || !strings.Contains(tools[0], `"hand_over"`) {
		t.Fatalf("handed over: %+v", review.Stages[0])
	}
	// An ordinary goal is never offered the tool.
	if strings.Contains(tools[0], `"propose_sessions"`) {
		t.Fatal("a review was offered the planning tool")
	}
}

func TestAStoppedPipelineIsTriedAgainOnlyWhenAsked(t *testing.T) {
	r := outcomeFixture(t)
	r.starter.refuse["Export API"] = "That agent isn't installed on that machine."
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	o := r.settle(t, created.ID)
	failed := o.Sessions[0]
	if failed.State != "failed" || len(r.starter.asked()) != 2 {
		t.Fatalf("before retry: %+v", failed)
	}
	if _, err := r.s.RetryPipeline(r.p.UserID, created.ID, o.Sessions[1].Key, o.Revision); err == nil {
		t.Fatal("a followed pipeline was tried again")
	}
	if _, err := r.s.RetryPipeline(r.p.UserID, created.ID, failed.Key, o.Revision-1); !errors.Is(err, store.ErrGafferOutcomeBusy) {
		t.Fatalf("stale retry: %v", err)
	}
	r.starter.mu.Lock()
	delete(r.starter.refuse, "Export API")
	r.starter.mu.Unlock()
	if _, err := r.s.RetryPipeline(r.p.UserID, created.ID, failed.Key, o.Revision); err != nil {
		t.Fatal(err)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	asked := r.starter.asked()
	// The retry is a new request: the refused one is not replayed.
	if len(asked) != 3 || asked[2].Title != "Export API" || asked[2].RequestID == asked[0].RequestID || again.SessionID == "" {
		t.Fatalf("after retry: %+v launches=%+v", again, asked)
	}
}

func TestPipelinesAreAddedToAnOutcomeOnlyWithApproval(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil {
		t.Fatal(err)
	}
	o := r.settle(t, created.ID)
	more := AddPipelinesInput{Revision: o.Revision, Confirmed: true, Sessions: []OutcomeSession{{Title: "Export docs", Prompt: "Document the export. Do not merge.", Stages: []OutcomeStageInput{{Kind: "code"}, {Kind: "pr"}}}}}
	unapproved := more
	unapproved.Confirmed = false
	var bad *Invalid
	if _, err := r.s.AddPipelines(r.p.UserID, created.ID, unapproved); !errors.As(err, &bad) {
		t.Fatalf("unapproved: %v", err)
	}
	repeat := more
	repeat.Sessions = []OutcomeSession{{Title: "Export API", Prompt: "Again. Do not merge."}}
	if _, err := r.s.AddPipelines(r.p.UserID, created.ID, repeat); !errors.As(err, &bad) {
		t.Fatalf("repeated title: %v", err)
	}
	if len(r.starter.asked()) != 2 {
		t.Fatal("a refused addition started a session")
	}
	added, err := r.s.AddPipelines(r.p.UserID, created.ID, more)
	if err != nil || len(added.Sessions) != 3 || added.State != "starting" || added.Sessions[2].Key == added.Sessions[1].Key {
		t.Fatalf("added: %+v %v", added, err)
	}
	final := r.settle(t, created.ID)
	docs := final.Sessions[2]
	// A stage with no agent of its own takes the outcome's.
	if docs.State != "following" || len(docs.Stages) != 2 || docs.Stages[0].Tool != "opencode" || docs.Stages[0].State != "running" {
		t.Fatalf("added pipeline: %+v", docs)
	}
	many := AddPipelinesInput{Revision: final.Revision, Confirmed: true}
	for i := 0; i < 5; i++ {
		many.Sessions = append(many.Sessions, OutcomeSession{Title: "More " + string(rune('A'+i)), Prompt: "Do it. Do not merge."})
	}
	if _, err := r.s.AddPipelines(r.p.UserID, created.ID, many); err != nil {
		t.Fatal(err)
	}
	over := AddPipelinesInput{Revision: mustOutcome(t, r, created.ID).Revision, Confirmed: true}
	for i := 0; i < 3; i++ {
		over.Sessions = append(over.Sessions, OutcomeSession{Title: "Over " + string(rune('A'+i)), Prompt: "Do it. Do not merge."})
	}
	if _, err := r.s.AddPipelines(r.p.UserID, created.ID, over); !errors.As(err, &bad) {
		t.Fatalf("more than an outcome holds: %v", err)
	}
}

// One report across an outcome goes to the outcome's own provider, carries
// only what Gaffer already recorded, and proposes pipelines without starting any.
func TestOutcomeReviewReportsAcrossPipelinesAndOnlyProposes(t *testing.T) {
	r := outcomeFixture(t)
	var mu sync.Mutex
	var sent []string
	r.reply = func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		mu.Lock()
		sent = append(sent, string(raw))
		mu.Unlock()
		args := `{"summary":"The endpoint is in review; nothing covers the docs.","proposals":[{"title":"Export docs","prompt":"Document the export in the README. Do not merge.","why":"No pipeline covers documentation.","stages":["code","pr"]},{"title":"Export API","prompt":"Repeat. Do not merge."}]}`
		call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "r1", "type": "function", "function": map[string]any{"name": "report_outcome", "arguments": args}}}}, "finish_reason": "tool_calls"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":300,\"completion_tokens\":80}}\n\ndata: [DONE]\n\n", call)
	}
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Endpoint and tests are committed on the branch.")
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	before := mustOutcome(t, r, created.ID)
	launches := len(r.starter.asked())
	if _, err := r.s.ReviewOutcome(context.Background(), r.p, created.ID, before.Revision-1); !errors.Is(err, store.ErrGafferOutcomeBusy) {
		t.Fatalf("stale review: %v", err)
	}
	got, err := r.s.ReviewOutcome(context.Background(), r.p, created.ID, before.Revision)
	if err != nil {
		t.Fatal(err)
	}
	rep := got.Report
	if rep == nil || rep.Text != "The endpoint is in review; nothing covers the docs." || rep.InputTokens != 300 || rep.OutputTokens != 80 || !rep.UsageReported || rep.At == 0 {
		t.Fatalf("report: %+v", rep)
	}
	// The proposal that repeats an existing pipeline is dropped; the other waits for the person.
	if len(rep.Proposals) != 1 || rep.Proposals[0].Title != "Export docs" || strings.Join(rep.Proposals[0].Stages, ",") != "code,pr" || rep.Proposals[0].Why == "" {
		t.Fatalf("proposals: %+v", rep.Proposals)
	}
	if len(got.Sessions) != 1 || len(r.starter.asked()) != launches {
		t.Fatal("a report added or started a pipeline")
	}
	mu.Lock()
	body := sent[len(sent)-1]
	mu.Unlock()
	for _, want := range []string{"Ship CSV export for billing.", "Pipeline 1: Export API", "Handover: Endpoint and tests are committed", "report_outcome", "review: running (in hand)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("evidence lacks %q: %s", want, body)
		}
	}
	// Approving the proposal is a separate, explicit step that clears it.
	added, err := r.s.AddPipelines(r.p.UserID, created.ID, AddPipelinesInput{Revision: got.Revision, Confirmed: true, Sessions: []OutcomeSession{{Title: rep.Proposals[0].Title, Prompt: rep.Proposals[0].Prompt, Stages: []OutcomeStageInput{{Kind: "code"}, {Kind: "pr"}}}}})
	if err != nil || len(added.Sessions) != 2 || added.Report == nil || len(added.Report.Proposals) != 0 || added.Report.Text == "" {
		t.Fatalf("after approval: %+v %v", added, err)
	}
	// A provider no longer allowed session text is not sent the evidence.
	row, err := r.st.GafferProviderFor(r.p.UserID, created.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	in := inputFrom(row)
	in.AllowCode = false
	if _, err = r.s.Save(r.p, in); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	calls := len(sent)
	mu.Unlock()
	// The pipeline just approved is being started in the background, which
	// moves the outcome on. A revision read a moment ago can be stale by the
	// time it is used, so reload and ask again, as the page does, until the
	// answer is about the provider and not about the revision.
	var bad *Invalid
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err = r.s.ReviewOutcome(context.Background(), r.p, created.ID, mustOutcome(t, r, created.ID).Revision)
		if !errors.Is(err, store.ErrGafferOutcomeBusy) || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !errors.As(err, &bad) {
		t.Fatalf("review with session text revoked: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != calls {
		t.Fatal("the evidence was sent to a provider that may no longer read session text")
	}
}

func TestALaterStageSessionIsListedUnderItsProject(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, pipelineInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	code := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" })
	r.s.stageHandedOver(r.p.UserID, code.ConversationID, "Done.")
	review := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 })
	if asked := r.starter.asked(); asked[1].LinkProjectID != r.project.ID || asked[1].ProjectID != "" {
		t.Fatalf("later stage launch: %+v", asked[1])
	}
	// The link holds only for a session working in the project's folder or one of its worktrees.
	if err := r.st.LinkTerminalSessionProjectFor(r.p.UserID, review.SessionID, r.project.ID); err != nil {
		t.Fatal(err)
	}
	row, err := r.st.TerminalSessionFor(r.p.UserID, review.SessionID)
	if err != nil || row.ProjectID == nil || *row.ProjectID != r.project.ID {
		t.Fatalf("linked session: %+v %v", row.ProjectID, err)
	}
	elsewhere, _, err := r.st.ReserveTerminalCreateFor(r.p.UserID, r.project.Host, strings.Repeat("7", 32), r.starter.gen, terminalproto.Create{Title: "Elsewhere", Tool: "opencode", Structured: true, Dir: "/work/other", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.st.LinkTerminalSessionProjectFor(r.p.UserID, elsewhere.ID, r.project.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a session outside the project was linked: %v", err)
	}
}
