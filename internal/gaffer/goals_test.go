package gaffer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type goalWorker struct {
	fakeWorker
	working atomic.Bool
	accept  bool
}

func (f *goalWorker) ChatControl(context.Context, int64, string, terminalproto.ACPControl) (terminalproto.ACPControlResult, error) {
	f.probes.Add(1)
	return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Working: f.working.Load(), Waiting: f.waiting.Load()}}, nil
}
func (f *goalWorker) EnqueueChat(uid int64, session, id, text string, images []string) error {
	if err := f.fakeWorker.EnqueueChat(uid, session, id, text, images); err != nil {
		return err
	}
	if !f.accept {
		return nil
	}
	list, err := f.st.TerminalQueueFor(uid, session)
	if err != nil {
		return err
	}
	for _, q := range list {
		if q.ID == id {
			ok, err := f.st.TransitionTerminalQueueFor(uid, session, q, "sending", "wait", "", "fake-worker-owner")
			if err != nil || !ok {
				return err
			}
			q.State = "sending"
			q.Revision++
			_, err = f.st.TransitionTerminalQueueFor(uid, session, q, "sent", "wait", "", "fake-worker-owner")
			return err
		}
	}
	return nil
}
func goalFixture(t *testing.T, s *Service, st *store.Store, p Principal, url string, max int) (ConversationView, store.GafferGoal) {
	t.Helper()
	c := workerConversation(t, s, st, p, url)
	provider, err := st.GafferProviderFor(p.UserID, c.Conversation.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	in := inputFrom(provider)
	in.AllowCode = true
	if _, err = s.Save(p, in); err != nil {
		t.Fatal(err)
	}
	c = grantWorker(t, s, p.UserID, c, true)
	c = shareContext(t, s, p.UserID, c, store.ContextFlags{History: true, Text: true})
	v, err := s.Goal(p.UserID, c.Conversation.ID)
	if err != nil || !v.Allowed {
		t.Fatal(v, err)
	}
	v, err = s.SaveGoal(p.UserID, c.Conversation.ID, GoalInput{GoalPins: v.Pins, Text: "Raise a PR, fix checks and conflicts. Ask me before scope changes. Do not merge.", MaxReviews: max, Hours: 24})
	if err != nil {
		t.Fatal(err)
	}
	return c, *v.Goal
}
func workerProgress(t *testing.T, st *store.Store, g store.GafferGoal, seq int64) {
	t.Helper()
	if err := st.AppendTerminalChatEventFor(g.OwnerID, g.SessionID, seq, time.Now().UnixMilli(), []byte(fmt.Sprintf(`{"kind":"agent","text":"Worker progress %d; PR checks need attention."}`, seq))); err != nil {
		t.Fatal(err)
	}
}
func dueGoal(t *testing.T, st *store.Store, g store.GafferGoal) store.GafferGoal {
	t.Helper()
	g, err := st.GafferGoalFor(g.OwnerID, g.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.ObserveGafferGoal(g, "active", "", 0); err != nil {
		t.Fatal(err)
	}
	return g
}
func finishReview(t *testing.T, s *Service, g store.GafferGoal) store.GafferGoal {
	t.Helper()
	previousReviews := g.UsedReviews
	g, err := s.store.GafferGoalFor(g.OwnerID, g.ConversationID)
	if err != nil {
		t.Fatal(g, err)
	}
	if g.ActiveTurnID == "" {
		if g.UsedReviews <= previousReviews || g.LastReviewedAt == 0 {
			t.Fatal("review was not admitted", g)
		}
		return g
	}
	waitTurn(t, s, g.OwnerID, g.ConversationID, g.ActiveTurnID)
	// Wait for the atomic goal finalisation that follows the saved turn.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		g, err = s.store.GafferGoalFor(g.OwnerID, g.ConversationID)
		if err != nil {
			t.Fatal(err)
		}
		if g.ActiveTurnID == "" {
			return g
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("goal review did not finish")
	return g
}
func fakeGoalProvider(calls *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); workerToolResponse(w, "") }))
}

func advancingGoalProvider(calls *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return
		}
		if len(body.Tools) > 0 {
			workerToolResponse(w, "")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"The instruction is accepted; work remains.\"}}]}\n\ndata: [DONE]\n\n")
		}
	}))
}
func TestGoalRequiresSeparateReadAndWriteGrants(t *testing.T) {
	s, st, p := settingsFixture(t)
	s.SetWorker(&goalWorker{fakeWorker: fakeWorker{st: st}})
	c := workerConversation(t, s, st, p, "http://127.0.0.1:1")
	v, err := s.Goal(p.UserID, c.Conversation.ID)
	if err != nil || v.Allowed {
		t.Fatal(v, err)
	}
	if _, err = s.SaveGoal(p.UserID, c.Conversation.ID, GoalInput{GoalPins: v.Pins, Text: "Do the work", MaxReviews: 12, Hours: 24}); err == nil {
		t.Fatal("implicit goal grants")
	}
	if rows, err := st.DueGafferGoals(time.Now().UnixMilli()); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
func TestGoalReviewsOnlyFreshIdleProgressAndHonoursBudget(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := advancingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 2)
	s.reviewDueGoals(context.Background())
	if calls.Load() != 0 {
		t.Fatal("unchanged empty evidence used provider")
	}
	workerProgress(t, st, g, 1)
	g = dueGoal(t, st, g)
	worker.working.Store(true)
	s.reviewDueGoals(context.Background())
	if calls.Load() != 0 {
		t.Fatal("busy worker used provider")
	}
	worker.working.Store(false)
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	if g.UsedReviews != 1 || g.LastSeq != 1 || g.State != "active" || calls.Load() != 2 {
		t.Fatal(g, calls.Load())
	}
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	if calls.Load() != 2 {
		t.Fatal("same progress consumed budget")
	}
	workerProgress(t, st, g, 2)
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	workerProgress(t, st, g, 3)
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	g, _ = st.GafferGoalFor(g.OwnerID, g.ConversationID)
	if g.State != "limit" || g.UsedReviews != 2 || calls.Load() != 4 {
		t.Fatal("budget exceeded", g, calls.Load())
	}
}
func TestGoalPauseFencesItsQueueAndResumeWaitsForProgress(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			workerToolResponse(w, "")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Instruction queued, not completed.\"}}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	queue, err := st.TerminalQueueFor(p.UserID, g.SessionID)
	if err != nil || len(queue) != 1 || worker.enqueues.Load() != 1 {
		t.Fatal(queue, err)
	}
	v, err := s.ControlGoal(p.UserID, g.ConversationID, g.Revision, "pause", GoalPins{}, false)
	if err != nil || v.Goal.State != "paused" {
		t.Fatal(v, err)
	}
	receipt, err := st.WorkerMessageFor(p.UserID, "gaffer", g.ConversationID, g.SessionID, g.PendingID)
	if err != nil || receipt.State != "removed" {
		t.Fatal(receipt, err)
	}
	if ok, err := st.TransitionTerminalQueueFor(p.UserID, g.SessionID, queue[0], "sending", "wait", "", "stale-owner"); err == nil || ok {
		t.Fatal("paused goal delivered", ok, err)
	}
	workerProgress(t, st, g, 2)
	v, err = s.ControlGoal(p.UserID, g.ConversationID, v.Goal.Revision, "resume", v.Pins, true)
	if err != nil {
		t.Fatal(err)
	}
	s.reviewDueGoals(context.Background())
	if calls.Load() != 2 || v.Goal.UsedReviews != 1 {
		t.Fatal("resume repeated old progress", calls.Load(), v.Goal)
	}
}
func TestGoalApprovalWaitAndProviderRevocationDoNotCallModel(t *testing.T) {
	for _, mode := range []string{"approval", "revoked"} {
		t.Run(mode, func(t *testing.T) {
			s, st, p := settingsFixture(t)
			worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
			s.SetWorker(worker)
			var calls atomic.Int32
			fake := fakeGoalProvider(&calls)
			defer fake.Close()
			_, g := goalFixture(t, s, st, p, fake.URL, 12)
			workerProgress(t, st, g, 1)
			if mode == "approval" {
				worker.waiting.Store(true)
			} else {
				provider, _ := st.GafferProviderFor(p.UserID, g.ProviderID)
				in := inputFrom(provider)
				in.AllowCode = false
				if _, err := s.Save(p, in); err != nil {
					t.Fatal(err)
				}
			}
			s.reviewDueGoals(context.Background())
			g, _ = st.GafferGoalFor(p.UserID, g.ConversationID)
			if g.State != "needs_user" || g.UsedReviews != 0 || calls.Load() != 0 || worker.enqueues.Load() != 0 {
				t.Fatal(g, calls.Load())
			}
		})
	}
}

func TestGoalDoesNotGrantActionsToOrdinaryChat(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := fakeGoalProvider(&calls)
	defer fake.Close()
	c, g := goalFixture(t, s, st, p, fake.URL, 12)
	c, _ = s.Conversation(p.UserID, c.Conversation.ID)
	id := strings.Repeat("a", 32)
	if _, err := s.StartTurn(p.UserID, c.Conversation.ID, TurnInput{ID: id, Revision: c.Conversation.Revision, Text: "How is the goal going?"}); err != nil {
		t.Fatal(err)
	}
	turn := waitTurn(t, s, p.UserID, c.Conversation.ID, id)
	if turn.State != "failed" || worker.enqueues.Load() != 0 {
		t.Fatal(turn)
	}
	g, _ = st.GafferGoalFor(p.UserID, g.ConversationID)
	if g.UsedReviews != 0 {
		t.Fatal(g)
	}
}

func TestActiveGoalsResumeAfterDatabaseReopenWithoutBrowser(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
	worker.working.Store(true)
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := advancingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	s.Close()
	g = dueGoal(t, st, g)
	reopened, err := store.Open(st.Dir())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	next := New(reopened, s.sealer)
	defer next.Close()
	fresh := &goalWorker{fakeWorker: fakeWorker{st: reopened}, accept: true}
	next.SetWorker(fresh)
	next.StartGoalSupervisor()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		g, err = reopened.GafferGoalFor(p.UserID, g.ConversationID)
		if err != nil {
			t.Fatal(err)
		}
		if g.UsedReviews == 1 && g.ActiveTurnID == "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if g.State != "active" || g.UsedReviews != 1 || calls.Load() != 2 || g.Summary == "" {
		t.Fatal("goal did not continue without browser", g, calls.Load())
	}
}

func TestInterruptedGoalReviewIsNotReplayedAtRestart(t *testing.T) {
	s, st, p := settingsFixture(t)
	s.SetWorker(&goalWorker{fakeWorker: fakeWorker{st: st}})
	_, g := goalFixture(t, s, st, p, "http://127.0.0.1:1", 12)
	workerProgress(t, st, g, 1)
	c, _ := st.GafferConversationFor(p.UserID, g.ConversationID)
	id := strings.Repeat("d", 32)
	if _, _, err := st.StartGafferTurnFor(p.UserID, store.GafferTurn{ID: id, ConversationID: c.ID, RequestRevision: c.Revision, Prompt: "An admitted review", ProviderID: g.ProviderID, Model: g.Model, WorkerGrantRevision: g.WorkerRevision, GoalID: g.ID, GoalRevision: g.Revision, Context: &store.ContextUse{GrantRevision: g.ContextRevision, LastSeq: 1}}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	reopened, err := store.Open(st.Dir())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	next := New(reopened, s.sealer)
	defer next.Close()
	fresh := &goalWorker{fakeWorker: fakeWorker{st: reopened}}
	next.SetWorker(fresh)
	next.StartGoalSupervisor()
	g, err = reopened.GafferGoalFor(p.UserID, g.ConversationID)
	if err != nil || g.State != "needs_user" || g.UsedReviews != 1 || g.ActiveTurnID != "" || fresh.probes.Load() != 0 || fresh.enqueues.Load() != 0 {
		t.Fatal("interrupted review replayed", g, err)
	}
	turn, err := reopened.GafferTurnFor(p.UserID, c.ID, id)
	if err != nil || turn.State != "interrupted" {
		t.Fatal(turn, err)
	}
}

type pausedGoalWorker struct {
	goalWorker
	started chan struct{}
}

func (f *pausedGoalWorker) ChatControl(ctx context.Context, uid int64, session string, in terminalproto.ACPControl) (terminalproto.ACPControlResult, error) {
	if f.probes.Add(1) == 2 {
		close(f.started)
		<-ctx.Done()
		return terminalproto.ACPControlResult{}, ctx.Err()
	}
	return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{}}, nil
}
func TestPausingGoalCancelsInFlightProposalBeforeQueueAdmission(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &pausedGoalWorker{goalWorker: goalWorker{fakeWorker: fakeWorker{st: st}}, started: make(chan struct{})}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := fakeGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	select {
	case <-worker.started:
	case <-time.After(3 * time.Second):
		t.Fatal("proposal did not reach worker preflight")
	}
	before, _ := st.GafferGoalFor(p.UserID, g.ConversationID)
	if _, err := s.ControlGoal(p.UserID, g.ConversationID, g.Revision, "pause", GoalPins{}, false); err != nil {
		t.Fatal(err)
	}
	waitTurn(t, s, p.UserID, g.ConversationID, before.ActiveTurnID)
	messages, err := st.WorkerMessagesFor(p.UserID, "gaffer", g.ConversationID, g.SessionID)
	if err != nil || len(messages) != 0 || worker.enqueues.Load() != 0 || calls.Load() != 1 {
		t.Fatal("paused proposal escaped", messages, err)
	}
}

func TestTwoExplicitSessionGoalsKeepSeparateProgressAndBudgets(t *testing.T) {
	s, st, p := settingsFixture(t)
	s.SetWorker(&goalWorker{fakeWorker: fakeWorker{st: st}})
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"This session still has work.\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer fake.Close()
	_, a := goalFixture(t, s, st, p, fake.URL, 12)
	first, err := st.TerminalSessionFor(p.UserID, a.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	row, _, err := st.ReserveTerminalCreateFor(p.UserID, first.AgentID, strings.Repeat("3", 32), 1, terminalproto.Create{Title: "Other session", Tool: "opencode", Structured: true, Dir: "/work", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.ObserveTerminalStateFor(p.UserID, row.AgentID, row.ID, 1, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(p.UserID, ConversationInput{SessionID: row.ID, ProviderID: a.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	c = grantWorker(t, s, p.UserID, c, true)
	c = shareContext(t, s, p.UserID, c, store.ContextFlags{History: true, Text: true})
	v, err := s.Goal(p.UserID, c.Conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.SaveGoal(p.UserID, c.Conversation.ID, GoalInput{GoalPins: v.Pins, Text: "A separate session goal", MaxReviews: 4, Hours: 8})
	if err != nil {
		t.Fatal(err)
	}
	b := *v.Goal
	workerProgress(t, st, a, 1)
	workerProgress(t, st, b, 1)
	s.reviewDueGoals(context.Background())
	a = finishReview(t, s, a)
	b = finishReview(t, s, b)
	if calls.Load() != 2 || a.UsedReviews != 1 || b.UsedReviews != 1 || a.SessionID == b.SessionID || a.ID == b.ID || a.Text == b.Text || a.State != "needs_user" || b.State != "needs_user" {
		t.Fatal("session goals were mixed", a, b, calls.Load())
	}
}

func TestGoalBudgetAdmissionIsAtomicAcrossStoreHandles(t *testing.T) {
	s, st, p := settingsFixture(t)
	s.SetWorker(&goalWorker{fakeWorker: fakeWorker{st: st}})
	_, g := goalFixture(t, s, st, p, "http://127.0.0.1:1", 1)
	c, _ := st.GafferConversationFor(p.UserID, g.ConversationID)
	other, err := store.Open(st.Dir())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	start := make(chan struct{})
	results := make(chan bool, 2)
	for i, db := range []*store.Store{st, other} {
		go func(i int, db *store.Store) {
			<-start
			_, made, err := db.StartGafferTurnFor(p.UserID, store.GafferTurn{ID: strings.Repeat(fmt.Sprintf("%x", i+1), 32), ConversationID: c.ID, RequestRevision: c.Revision, Prompt: "Reserved goal review", ProviderID: g.ProviderID, Model: g.Model, WorkerGrantRevision: g.WorkerRevision, GoalID: g.ID, GoalRevision: g.Revision, Context: &store.ContextUse{GrantRevision: g.ContextRevision, LastSeq: 1}})
			results <- made && err == nil
		}(i, db)
	}
	close(start)
	made := 0
	for i := 0; i < 2; i++ {
		if <-results {
			made++
		}
	}
	g, err = st.GafferGoalFor(p.UserID, g.ConversationID)
	if err != nil || made != 1 || g.UsedReviews != 1 || g.ActiveTurnID == "" {
		t.Fatal("goal budget or claim duplicated", made, g, err)
	}
}

func TestGoalCanAskUserButCannotDeclareItselfComplete(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"question_one\",\"type\":\"function\",\"function\":{\"name\":\"ask_user\",\"arguments\":\"{\\\"summary\\\":\\\"The worker proposes a scope change. Please review it.\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	if g.State != "needs_user" || !strings.Contains(g.Summary, "scope change") || worker.enqueues.Load() != 0 || calls.Load() != 1 {
		t.Fatal(g, calls.Load())
	}
	v, err := s.ControlGoal(p.UserID, g.ConversationID, g.Revision, "complete", GoalPins{}, true)
	if err != nil || v.Goal.State != "completed" || !strings.Contains(v.Goal.Note, "You marked") {
		t.Fatal(v, err)
	}
}

func TestExpiredGoalDoesNotProbeWorkerOrUseProvider(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
	s.SetWorker(worker)
	_, g := goalFixture(t, s, st, p, "http://127.0.0.1:1", 12)
	g.ID = strings.Repeat("f", 32)
	g.ExpiresAt = time.Now().Add(3 * time.Second).UnixMilli()
	g, err := st.SaveGafferGoal(p.UserID, g.Revision, g)
	if err != nil {
		t.Fatal(err)
	}
	workerProgress(t, st, g, 1)
	time.Sleep(time.Until(time.UnixMilli(g.ExpiresAt).Add(10 * time.Millisecond)))
	s.reviewDueGoals(context.Background())
	g, err = st.GafferGoalFor(p.UserID, g.ConversationID)
	if err != nil || g.State != "limit" || g.UsedReviews != 0 || worker.probes.Load() != 0 || worker.enqueues.Load() != 0 {
		t.Fatal(g, err)
	}
}

func TestGoalBringsAStoppedWorkerBackToUser(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
	s.SetWorker(worker)
	_, g := goalFixture(t, s, st, p, "http://127.0.0.1:1", 12)
	if err := st.AppendTerminalChatEventFor(p.UserID, g.SessionID, 1, time.Now().UnixMilli(), []byte(`{"kind":"status","state":"cancelled"}`)); err != nil {
		t.Fatal(err)
	}
	s.reviewDueGoals(context.Background())
	g, err := st.GafferGoalFor(p.UserID, g.ConversationID)
	if err != nil || g.State != "needs_user" || g.UsedReviews != 0 || worker.probes.Load() != 0 || worker.enqueues.Load() != 0 {
		t.Fatal("stopped worker was restarted", g, err)
	}
}

func TestGoalStopsAfterTwoIdenticalWorkerInstructions(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}, accept: true}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if len(body.Tools) > 0 {
			workerToolResponse(w, "")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"The worker accepted the instruction; it is not complete.\"}}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	for seq := int64(1); seq <= 3; seq++ {
		workerProgress(t, st, g, seq)
		g = dueGoal(t, st, g)
		s.reviewDueGoals(context.Background())
		g = finishReview(t, s, g)
	}
	messages, err := st.WorkerMessagesFor(p.UserID, "gaffer", g.ConversationID, g.SessionID)
	if err != nil || len(messages) != 2 || g.State != "needs_user" || g.UsedReviews != 3 || worker.enqueues.Load() != 2 || calls.Load() != 5 {
		t.Fatal("repeated instruction loop escaped", g, messages, err, calls.Load())
	}
}

func TestGoalWaitsForQueuedDeliveryThroughExplicitWorkerStop(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &goalWorker{fakeWorker: fakeWorker{st: st}}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := advancingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	if err := st.AppendTerminalChatEventFor(p.UserID, g.SessionID, 2, time.Now().UnixMilli(), []byte(`{"kind":"status","state":"cancelled"}`)); err != nil {
		t.Fatal(err)
	}
	g = dueGoal(t, st, g)
	s.reviewDueGoals(context.Background())
	g, err := st.GafferGoalFor(p.UserID, g.ConversationID)
	m, e := st.WorkerMessageFor(p.UserID, "gaffer", g.ConversationID, g.SessionID, g.PendingID)
	if err != nil || e != nil || g.State != "active" || m.State != "queued" || calls.Load() != 2 {
		t.Fatal("pending stop-and-send was mistaken for another review", g, m, err, e)
	}
}

type shutdownGoalWorker struct {
	goalWorker
	stop context.CancelFunc
}

func (f *shutdownGoalWorker) ReconcileWorkerQueue(uid int64, session string) error {
	if f.stop != nil {
		f.stop()
		return context.Canceled
	}
	return f.goalWorker.ReconcileWorkerQueue(uid, session)
}
func TestControllerShutdownDuringReceiptReadLeavesIdleGoalResumable(t *testing.T) {
	s, st, p := settingsFixture(t)
	worker := &shutdownGoalWorker{goalWorker: goalWorker{fakeWorker: fakeWorker{st: st}}}
	s.SetWorker(worker)
	var calls atomic.Int32
	fake := advancingGoalProvider(&calls)
	defer fake.Close()
	_, g := goalFixture(t, s, st, p, fake.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.stop = cancel
	s.reviewGoal(ctx, g)
	saved, err := st.GafferGoalFor(p.UserID, g.ConversationID)
	if err != nil || saved.State != "active" || saved.PendingID != g.PendingID || calls.Load() != 2 {
		t.Fatal("shutdown paused an idle goal", saved, err)
	}
}
