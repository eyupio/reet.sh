package gaffer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type fakeWorker struct {
	st               *store.Store
	probes, enqueues atomic.Int32
	waiting          atomic.Bool
	started          chan struct{}
	hold             bool
}

func (f *fakeWorker) ChatControl(ctx context.Context, _ int64, _ string, _ terminalproto.ACPControl) (terminalproto.ACPControlResult, error) {
	f.probes.Add(1)
	if f.hold {
		select {
		case f.started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return terminalproto.ACPControlResult{}, ctx.Err()
	}
	return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Working: true, Waiting: f.waiting.Load()}}, nil
}
func (f *fakeWorker) EnqueueChat(uid int64, session, id, text string, images []string) error {
	f.enqueues.Add(1)
	return f.st.EnqueueTerminalFor(uid, session, id, text, images)
}
func (f *fakeWorker) ReconcileWorkerQueue(uid int64, session string) error {
	return f.st.RecoverTerminalQueueFor(uid, session, "fake-worker-owner")
}
func runningWorkerSession(t *testing.T, st *store.Store, p Principal) store.TerminalSession {
	t.Helper()
	row := contextSessionFixture(t, st, p)
	if err := st.ObserveTerminalStateFor(p.UserID, row.AgentID, row.ID, 1, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	return row
}
func workerClient(t *testing.T, s *Service, st *store.Store, p Principal, session string, enabled bool) string {
	t.Helper()
	c, err := st.CreateMCPConnection(p.UserID, "Worker client", "write", "worker-hash-"+session+strconv.Itoa(int(time.Now().UnixNano())), time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetMCPEnabled(p.UserID, true); err != nil {
		t.Fatal(err)
	}
	subject := strconv.FormatInt(c.ID, 10)
	if enabled {
		if _, err = s.ConfigureWorker(p.UserID, "mcp", subject, session, store.WorkerGrant{Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	return subject
}
func grantWorker(t *testing.T, s *Service, uid int64, c ConversationView, enabled bool) ConversationView {
	t.Helper()
	w, err := s.WorkerSettings(uid, "gaffer", c.Conversation.ID, c.Conversation.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureWorker(uid, "gaffer", c.Conversation.ID, c.Conversation.SessionID, store.WorkerGrant{Enabled: enabled, Revision: w.Grant.Revision, ProviderID: w.ProviderID, ProviderRevision: w.ProviderRevision, Model: w.Model})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.Conversation(uid, c.Conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestWorkerMessagesAreScopedIdempotentAndRevocable(t *testing.T) {
	s, st, p := settingsFixture(t)
	row := runningWorkerSession(t, st, p)
	worker := &fakeWorker{st: st}
	s.SetWorker(worker)
	subject := workerClient(t, s, st, p, row.ID, false)
	id := strings.Repeat("a", 32)
	if _, err := s.WorkerMessage(context.Background(), p.UserID, "mcp", subject, row.ID, id, "", "Raise a PR when ready."); !errors.Is(err, ErrContextForbidden) {
		t.Fatal("implicit permission", err)
	}
	if worker.probes.Load() != 0 {
		t.Fatal("ungranted request touched worker")
	}
	if _, err := s.ConfigureWorker(p.UserID, "mcp", subject, row.ID, store.WorkerGrant{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	msg, err := s.WorkerMessage(context.Background(), p.UserID, "mcp", subject, row.ID, id, "", "Raise a PR when ready.")
	if err != nil || msg.State != "queued" {
		t.Fatal(msg, err)
	}
	again, err := s.WorkerMessage(context.Background(), p.UserID, "mcp", subject, row.ID, id, "", msg.Text)
	if err != nil || again.QueueID != msg.QueueID || worker.enqueues.Load() != 1 || worker.probes.Load() != 1 {
		t.Fatal("duplicate was dispatched", again, err, worker.enqueues.Load())
	}
	if _, err = s.WorkerMessage(context.Background(), p.UserID, "mcp", subject, row.ID, id, "", "Changed instruction"); !errors.Is(err, store.ErrGafferConflict) {
		t.Fatal("changed duplicate", err)
	}
	other := workerClient(t, s, st, p, row.ID, false)
	if _, err = s.WorkerMessageReceipt(p.UserID, "mcp", other, row.ID, id); err == nil {
		t.Fatal("another client read receipt")
	}
	if _, err = s.WorkerMessage(context.Background(), p.UserID, "mcp", subject, strings.Repeat("f", 32), strings.Repeat("b", 32), "", "Other session"); err == nil {
		t.Fatal("wrong session allowed")
	}
	queue, err := st.TerminalQueueFor(p.UserID, row.ID)
	if err != nil || len(queue) != 1 || queue[0].Origin != "companion" {
		t.Fatal(queue, err)
	}
	if err = st.ChangeTerminalQueueFor(p.UserID, row.ID, msg.QueueID, queue[0].Revision, "edit", "Tampered", false); !errors.Is(err, store.ErrChatQueueConflict) {
		t.Fatal("managed instruction was edited", err)
	}
	if _, err = s.ConfigureWorker(p.UserID, "mcp", subject, row.ID, store.WorkerGrant{Revision: 1, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	stale := queue[0]
	stale.Origin = "" // A caller's projection cannot bypass the stored fence.
	changed, err := st.TransitionTerminalQueueFor(p.UserID, row.ID, stale, "sending", "wait", "", "owner")
	if err == nil || changed {
		t.Fatal("revoked instruction was claimed", changed, err)
	}
	receipt, err := st.WorkerMessageFor(p.UserID, "mcp", subject, row.ID, id)
	if err != nil || receipt.State != "removed" {
		t.Fatal(receipt, err)
	}
}
func TestWorkerMessageUncertaintySurvivesReopenWithoutDispatch(t *testing.T) {
	s, st, p := settingsFixture(t)
	row := runningWorkerSession(t, st, p)
	f := &fakeWorker{st: st}
	s.SetWorker(f)
	subject := workerClient(t, s, st, p, row.ID, true)
	id := strings.Repeat("b", 32)
	m, err := s.WorkerMessage(context.Background(), p.UserID, "mcp", subject, row.ID, id, "", "Check the PR.")
	if err != nil {
		t.Fatal(err)
	}
	q, _ := st.TerminalQueueFor(p.UserID, row.ID)
	if ok, err := st.TransitionTerminalQueueFor(p.UserID, row.ID, q[0], "sending", "wait", "", "old-controller"); err != nil || !ok {
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
	fresh := &fakeWorker{st: reopened}
	next.SetWorker(fresh)
	receipt, err := next.WorkerMessageReceipt(p.UserID, "mcp", subject, row.ID, id)
	if err != nil || receipt.State != "uncertain" {
		t.Fatal(receipt, err)
	}
	again, err := next.WorkerMessage(context.Background(), p.UserID, "mcp", subject, row.ID, id, "", m.Text)
	if err != nil || again.State != "uncertain" || fresh.enqueues.Load() != 0 {
		t.Fatal("uncertainty replayed", again, err)
	}
}
func workerToolResponse(w http.ResponseWriter, kind string) {
	calls := []any{map[string]any{"index": 0, "id": "call_one", "type": "function", "function": map[string]any{"name": "message_worker", "arguments": `{"text":"Raise a PR and fix its checks; do not merge."}`}}}
	if kind == "unknown" {
		calls = []any{map[string]any{"index": 0, "id": "call_one", "type": "function", "function": map[string]any{"name": "run_shell", "arguments": `{"text":"bad"}`}}}
	}
	if kind == "extra" {
		calls = []any{map[string]any{"index": 0, "id": "call_one", "type": "function", "function": map[string]any{"name": "message_worker", "arguments": `{"text":"bad","interrupt":true,"session_id":"another"}`}}}
	}
	if kind == "multiple" {
		calls = append(calls, map[string]any{"index": 1, "id": "call_two", "type": "function", "function": map[string]any{"name": "message_worker", "arguments": `{"text":"second"}`}})
	}
	raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": calls}}}})
	fmt.Fprintf(w, "data: %s\n\n", raw)
	if kind == "incomplete" {
		return
	}
	finish := "tool_calls"
	if kind == "cut" {
		finish = "length"
	}
	fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":%q}]}\n\ndata: [DONE]\n\n", finish)
}
func workerConversation(t *testing.T, s *Service, st *store.Store, p Principal, url string) ConversationView {
	t.Helper()
	row := runningWorkerSession(t, st, p)
	provider, err := s.Save(p, Input{Name: "Worker tools", Kind: "openai_compatible", BaseURL: url, LocalOnly: true, Enabled: true, Model: "fake-model", AllowSessions: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(p.UserID, ConversationInput{SessionID: row.ID, ProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestGafferWorkerToolRequiresOptInAndKeepsReceiptAfterReplyFailure(t *testing.T) {
	s, st, p := settingsFixture(t)
	f := &fakeWorker{st: st}
	s.SetWorker(f)
	var providerCalls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		var body struct {
			Tools    []any            `json:"tools"`
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, message := range body.Messages {
			if message["role"] == "tool" {
				http.Error(w, "continuation failed", 502)
				return
			}
		}
		workerToolResponse(w, "")
	}))
	defer fake.Close()
	c := workerConversation(t, s, st, p, fake.URL)
	if _, err := s.StartTurn(p.UserID, c.Conversation.ID, TurnInput{ID: strings.Repeat("a", 32), Revision: c.Conversation.Revision, Text: "Ask the worker", Relay: true}); err == nil {
		t.Fatal("ungranted relay admitted")
	}
	c = grantWorker(t, s, p.UserID, c, true)
	normal := TurnInput{ID: strings.Repeat("b", 32), Revision: c.Conversation.Revision, Text: "A status-only question"}
	if _, err := s.StartTurn(p.UserID, c.Conversation.ID, normal); err != nil {
		t.Fatal(err)
	}
	turn := waitTurn(t, s, p.UserID, c.Conversation.ID, normal.ID)
	if turn.State != "failed" || f.enqueues.Load() != 0 {
		t.Fatal("ordinary chat executed a tool", turn)
	}
	c, _ = s.Conversation(p.UserID, c.Conversation.ID)
	relay := TurnInput{ID: strings.Repeat("c", 32), Revision: c.Conversation.Revision, Text: "Ask the worker to raise the PR and fix checks without merging.", Relay: true}
	if _, err := s.StartTurn(p.UserID, c.Conversation.ID, relay); err != nil {
		t.Fatal(err)
	}
	turn = waitTurn(t, s, p.UserID, c.Conversation.ID, relay.ID)
	view, err := s.Conversation(p.UserID, c.Conversation.ID)
	if err != nil || turn.State != "failed" || len(view.Messages) != 1 || view.Messages[0].State != "queued" || f.enqueues.Load() != 1 {
		t.Fatal("lost or duplicated committed action", view.Messages, turn, err)
	}
	if providerCalls.Load() != 3 {
		t.Fatal("automatic retry", providerCalls.Load())
	}
	if _, err = s.StartTurn(p.UserID, c.Conversation.ID, relay); err != nil {
		t.Fatal(err)
	}
	relay.Relay = false
	if _, err = s.StartTurn(p.UserID, c.Conversation.ID, relay); !errors.Is(err, store.ErrGafferConflict) {
		t.Fatal("receipt intent changed", err)
	}
	if f.enqueues.Load() != 1 || providerCalls.Load() != 3 {
		t.Fatal("receipt lookup replayed work")
	}
}
func TestGafferRejectsInvalidOrIncompleteWorkerToolsBeforeQueue(t *testing.T) {
	for _, kind := range []string{"unknown", "extra", "multiple", "cut", "incomplete"} {
		t.Run(kind, func(t *testing.T) {
			s, st, p := settingsFixture(t)
			f := &fakeWorker{st: st}
			s.SetWorker(f)
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { workerToolResponse(w, kind) }))
			defer fake.Close()
			c := grantWorker(t, s, p.UserID, workerConversation(t, s, st, p, fake.URL), true)
			id := strings.Repeat("d", 32)
			if _, err := s.StartTurn(p.UserID, c.Conversation.ID, TurnInput{ID: id, Revision: c.Conversation.Revision, Text: "Ask the worker to check.", Relay: true}); err != nil {
				t.Fatal(err)
			}
			turn := waitTurn(t, s, p.UserID, c.Conversation.ID, id)
			if turn.State != "failed" || f.enqueues.Load() != 0 || f.probes.Load() != 0 {
				t.Fatal("invalid tool touched worker", turn, f.probes.Load())
			}
		})
	}
}
func TestWorkerGrantRevocationCancelsBeforeAdmission(t *testing.T) {
	s, st, p := settingsFixture(t)
	f := &fakeWorker{st: st, hold: true, started: make(chan struct{}, 1)}
	s.SetWorker(f)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { workerToolResponse(w, "") }))
	defer fake.Close()
	c := grantWorker(t, s, p.UserID, workerConversation(t, s, st, p, fake.URL), true)
	id := strings.Repeat("e", 32)
	if _, err := s.StartTurn(p.UserID, c.Conversation.ID, TurnInput{ID: id, Revision: c.Conversation.Revision, Text: "Ask the worker.", Relay: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker preflight did not start")
	}
	grantWorker(t, s, p.UserID, c, false)
	turn := waitTurn(t, s, p.UserID, c.Conversation.ID, id)
	view, err := s.Conversation(p.UserID, c.Conversation.ID)
	if err != nil || turn.State != "failed" || len(view.Messages) != 0 || f.enqueues.Load() != 0 {
		t.Fatal("revoked preflight queued work", turn, view.Messages, err)
	}
}

func TestQueuedGafferInstructionCannotOutliveItsProviderModelOrAccountPermission(t *testing.T) {
	for _, change := range []string{"provider_revision", "conversation_model", "provider_policy", "account_role", "session_archive"} {
		t.Run(change, func(t *testing.T) {
			s, st, p := settingsFixture(t)
			worker := &fakeWorker{st: st}
			s.SetWorker(worker)
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []map[string]any `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				for _, m := range body.Messages {
					if m["role"] == "tool" {
						fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Queued for the working model.\"}}]}\n\ndata: [DONE]\n\n")
						return
					}
				}
				workerToolResponse(w, "")
			}))
			defer fake.Close()
			c := grantWorker(t, s, p.UserID, workerConversation(t, s, st, p, fake.URL), true)
			id := strings.Repeat("f", 32)
			if _, err := s.StartTurn(p.UserID, c.Conversation.ID, TurnInput{ID: id, Revision: c.Conversation.Revision, Text: "Ask the worker to check the PR.", Relay: true}); err != nil {
				t.Fatal(err)
			}
			if turn := waitTurn(t, s, p.UserID, c.Conversation.ID, id); turn.State != "completed" {
				t.Fatal(turn)
			}
			queue, err := st.TerminalQueueFor(p.UserID, c.Conversation.SessionID)
			if err != nil || len(queue) != 1 || queue[0].State != "queued" {
				t.Fatal(queue, err)
			}
			switch change {
			case "provider_revision", "provider_policy":
				provider, err := st.GafferProviderFor(p.UserID, c.Conversation.ProviderID)
				if err != nil {
					t.Fatal(err)
				}
				in := inputFrom(provider)
				if change == "provider_revision" {
					in.Name = "Changed provider"
				} else {
					in.AllowSessions = false
				}
				if _, err = s.Save(p, in); err != nil {
					t.Fatal(err)
				}
			case "conversation_model":
				c, err = s.Conversation(p.UserID, c.Conversation.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.ConfigureConversation(p.UserID, c.Conversation.ID, c.Conversation.Revision, ConversationInput{SessionID: c.Conversation.SessionID, ProviderID: c.Conversation.ProviderID, Model: "another-model"}); err != nil {
					t.Fatal(err)
				}
			case "account_role":
				if err = st.UpdateUserRole(p.UserID, "viewer"); err != nil {
					t.Fatal(err)
				}
			case "session_archive":
				row, err := st.TerminalSessionFor(p.UserID, c.Conversation.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				if err = st.ObserveTerminalStateFor(p.UserID, row.AgentID, row.ID, 1, terminalproto.Exited, new(int)); err != nil {
					t.Fatal(err)
				}
				if err = st.ArchiveTerminalFor(p.UserID, row.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err = st.CheckWorkerDelivery(p.UserID, c.Conversation.SessionID, queue[0].ID); err == nil {
				t.Fatal("changed permission still authorised delivery")
			}
			claimed, err := st.TransitionTerminalQueueFor(p.UserID, c.Conversation.SessionID, queue[0], "sending", "wait", "", "worker-owner")
			if err == nil || claimed {
				t.Fatal("changed permission still allowed queue claim", claimed, err)
			}
			if worker.enqueues.Load() != 1 {
				t.Fatal("configuration replayed instruction", worker.enqueues.Load())
			}
		})
	}
}

func TestWorkerApprovalWaitProducesBlockedReceiptWithoutQueueOrReplay(t *testing.T) {
	s, st, p := settingsFixture(t)
	row := runningWorkerSession(t, st, p)
	worker := &fakeWorker{st: st}
	worker.waiting.Store(true)
	s.SetWorker(worker)
	subject := workerClient(t, s, st, p, row.ID, true)
	id := strings.Repeat("f", 32)
	m, err := s.WorkerMessage(context.Background(), p.UserID, "mcp", subject, row.ID, id, "", "Continue after the checks.")
	if err != nil || m.State != "blocked" || worker.enqueues.Load() != 0 {
		t.Fatal(m, err)
	}
	worker.waiting.Store(false)
	again, err := s.WorkerMessage(context.Background(), p.UserID, "mcp", subject, row.ID, id, "", m.Text)
	if err != nil || again.State != "blocked" || worker.probes.Load() != 1 || worker.enqueues.Load() != 0 {
		t.Fatal("blocked retry was dispatched", again, err)
	}
	q, err := st.TerminalQueueFor(p.UserID, row.ID)
	if err != nil || len(q) != 0 {
		t.Fatal(q, err)
	}
}
