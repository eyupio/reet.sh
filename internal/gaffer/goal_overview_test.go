package gaffer

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestGoalOverviewRetainsEvidenceAndReconcilesWithoutProviderOrWorkerCalls(t *testing.T) {
	s, st, p := settingsFixture(t)
	s.SetWorker(&goalWorker{fakeWorker: fakeWorker{st: st}})
	var calls atomic.Int32
	provider := advancingGoalProvider(&calls)
	defer provider.Close()
	_, g := goalFixture(t, s, st, p, provider.URL, 12)
	workerProgress(t, st, g, 1)
	s.reviewDueGoals(context.Background())
	g = finishReview(t, s, g)
	q, err := st.TerminalQueueFor(p.UserID, g.SessionID)
	if err != nil || len(q) != 1 {
		t.Fatal(q, err)
	}
	if ok, err := st.TransitionTerminalQueueFor(p.UserID, g.SessionID, q[0], "sending", "wait", "", "previous-controller"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	s.Close()
	reopened, err := store.Open(st.Dir())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	next := New(reopened, s.sealer)
	defer next.Close()
	worker := &fakeWorker{st: reopened}
	next.SetWorker(worker)
	beforeCalls := calls.Load()
	v, err := next.Goals(p.UserID)
	if err != nil || len(v.Goals) != 1 {
		t.Fatal(v, err)
	}
	r := v.Goals[0]
	if !r.Available || !r.Attention || r.Delivery == nil || r.Delivery.State != "uncertain" || r.Goal.Summary != g.Summary || r.Goal.LastSeq != 1 || r.Goal.LastObservedAt != g.LastObservedAt || r.Goal.LastReviewedAt != g.LastReviewedAt || v.ReadAt < g.LastReviewedAt {
		t.Fatal("overview lost evidence or falsely confirmed delivery", r)
	}
	if calls.Load() != beforeCalls || worker.probes.Load() != 0 || worker.enqueues.Load() != 0 {
		t.Fatal("overview invoked external work")
	}
	// An account's own retained evidence stays reviewable after permissions change.
	if _, err := next.ConfigureWorker(p.UserID, "gaffer", g.ConversationID, g.SessionID, store.WorkerGrant{Enabled: false, Revision: g.WorkerRevision}); err != nil {
		t.Fatal(err)
	}
	v, err = next.Goals(p.UserID)
	if err != nil || len(v.Goals) != 1 || !v.Goals[0].Attention || v.Goals[0].Reason == "" || v.Goals[0].Delivery.State != "uncertain" {
		t.Fatal(v, err)
	}
	other, err := reopened.CreateUser("other-overview@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherView, err := next.Goals(other.ID)
	if err != nil || len(otherView.Goals) != 0 {
		t.Fatal("another account read the goal", otherView, err)
	}
	if err := reopened.UpdateUserRole(other.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := next.Goals(other.ID); err == nil {
		t.Fatal("freshly demoted account read overview")
	}
	// Archived scopes keep only retained owner metadata and offer no worker route.
	session, _ := reopened.TerminalSessionFor(p.UserID, g.SessionID)
	if err := reopened.ObserveTerminalStateFor(p.UserID, session.AgentID, session.ID, 1, terminalproto.Exited, nil); err != nil {
		t.Fatal(err)
	}
	if err := reopened.ArchiveTerminalFor(p.UserID, session.ID); err != nil {
		t.Fatal(err)
	}
	v, err = next.Goals(p.UserID)
	if err != nil || len(v.Goals) != 1 || v.Goals[0].Available || v.Goals[0].Delivery != nil || v.Goals[0].Goal.Summary != g.Summary || !strings.Contains(v.Goals[0].Reason, "archived") {
		t.Fatal(v, err)
	}
	c, _ := reopened.GafferConversationFor(p.UserID, g.ConversationID)
	if err := next.DeleteConversation(p.UserID, c.ID, c.Revision); err != nil {
		t.Fatal(err)
	}
	v, err = next.Goals(p.UserID)
	if err != nil || len(v.Goals) != 0 || calls.Load() != beforeCalls || worker.probes.Load() != 0 || worker.enqueues.Load() != 0 {
		t.Fatal(v, err)
	}
}

func TestGoalOverviewPutsDecisionsBeforeActiveWorkWithoutChangingGoals(t *testing.T) {
	s, st, p := settingsFixture(t)
	s.SetWorker(&goalWorker{fakeWorker: fakeWorker{st: st}})
	_, a := goalFixture(t, s, st, p, "http://127.0.0.1:1", 12)
	first, _ := st.TerminalSessionFor(p.UserID, a.SessionID)
	row, _, err := st.ReserveTerminalCreateFor(p.UserID, first.AgentID, strings.Repeat("3", 32), 1, terminalproto.Create{Title: "Decision session", Tool: "opencode", Structured: true, Dir: "/work", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ObserveTerminalStateFor(p.UserID, row.AgentID, row.ID, 1, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(p.UserID, ConversationInput{SessionID: row.ID, ProviderID: a.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	c = grantWorker(t, s, p.UserID, c, true)
	c = shareContext(t, s, p.UserID, c, store.ContextFlags{History: true, Text: true})
	pins, _ := s.Goal(p.UserID, c.Conversation.ID)
	b, err := s.SaveGoal(p.UserID, c.Conversation.ID, GoalInput{GoalPins: pins.Pins, Text: "Ask me about scope", MaxReviews: 12, Hours: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ObserveGafferGoal(*b.Goal, "needs_user", "Scope decision needed", 0); err != nil {
		t.Fatal(err)
	}
	v, err := s.Goals(p.UserID)
	if err != nil || len(v.Goals) != 2 || v.Goals[0].Goal.ID != b.Goal.ID || !v.Goals[0].Attention || v.Goals[1].Goal.ID != a.ID {
		t.Fatal(v, err)
	}
	after, _ := st.GafferGoalFor(p.UserID, a.ConversationID)
	if after.Revision != a.Revision || after.UsedReviews != 0 || after.LastSeq != a.LastSeq {
		t.Fatal("overview mutated goal authority or budget", after)
	}
}
