package gaffer

import (
	"context"
	"errors"
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

func (f *outcomeStarterFake) StartOutcomeSession(ctx context.Context, uid int64, in OutcomeLaunch) (string, error) {
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
			return "", ctx.Err()
		}
	}
	if refusal != "" {
		return "", errors.New(refusal)
	}
	row, _, err := f.st.ReserveTerminalCreateFor(uid, in.AgentID, in.RequestID, f.gen, terminalproto.Create{Title: in.Title, Tool: in.Tool, Structured: true, Dir: in.Dir, Prompt: in.Prompt, Cols: 100, Rows: 30})
	if err != nil {
		return "", err
	}
	if row.State != terminalproto.Running {
		if err = f.st.ObserveTerminalStateFor(uid, in.AgentID, row.ID, 1, terminalproto.Running, nil); err != nil {
			return "", err
		}
	}
	return row.ID, nil
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
}

func outcomeFixture(t *testing.T) *outcomeRig {
	t.Helper()
	outcomeFollowEvery = 10 * time.Millisecond
	s, st, p := settingsFixture(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("an outcome must not call the provider to start sessions")
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
	return &outcomeRig{s: s, st: st, p: p, starter: starter, project: project, in: in}
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
		if launch.Title != want.Title || launch.Prompt != want.Prompt || launch.AgentID != r.project.Host || launch.Dir != "/work/billing" || launch.ProjectID != r.project.ID || launch.Tool != "opencode" || !terminalproto.ValidID(launch.RequestID) {
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
