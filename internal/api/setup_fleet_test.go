package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type setupFleetAPI struct {
	srv         *Server
	st          *store.Store
	uid         int64
	cookie      string
	agents      []store.SyncAgent
	generations []int64
	mu          sync.Mutex
	calls       []string // host/op in the order machines were asked
}

func setupTestCapabilities(apply bool) terminalproto.Capabilities {
	return terminalproto.Capabilities{
		ProtocolVersion: terminalproto.Version, OS: "linux", OSUser: "tester",
		TerminalEnabled: true, TerminalControl: true, HealthEnabled: true, Readiness: terminalproto.Ready,
		SetupRead: true, SetupApply: apply, SetupRollback: apply,
	}
}

// setupFleetFixture enrols machines; machine i supports applying a copy
// unless it is listed in old.
func setupFleetFixture(t *testing.T, machines int, old ...int) *setupFleetAPI {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	if _, err := st.CreateAPIToken(uid, "owner", auth.HashToken("fixture-owner")); err != nil {
		t.Fatal(err)
	}
	srv := New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts), WithRemoteTerminals(true), WithPublicURL("https://reet.example"))
	t.Cleanup(func() { srv.Close() })
	f := &setupFleetAPI{srv: srv, st: st, uid: uid, cookie: terminalLogin(t, st, uid)}
	for i := 0; i < machines; i++ {
		a, _, err := st.EnrollSyncAgent(uid, fmt.Sprintf("machine%d", i), "test")
		if err != nil {
			t.Fatal(err)
		}
		apply := true
		for _, o := range old {
			apply = apply && o != i
		}
		record, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), setupTestCapabilities(apply))
		if err != nil {
			t.Fatal(err)
		}
		f.agents = append(f.agents, a)
		f.generations = append(f.generations, record.Generation)
	}
	return f
}

func setupTestItem(agent, kind, name, format, content string) terminalproto.SetupItem {
	item := terminalproto.SetupItem{Agent: agent, Scope: "global", Kind: kind, Name: name, Format: format, Content: content, Label: "/home/src/" + name}
	item.SHA256 = terminalproto.SetupItemSHA256(content)
	return item
}

func sourceItems() []terminalproto.SetupItem {
	return []terminalproto.SetupItem{
		setupTestItem("claude", "instructions", "CLAUDE.md", "text", "Use British spelling.\n"),
		setupTestItem("claude", "settings", "settings.json", "json", `{"permissions":{"allow":["Read"]}}`),
	}
}

func planFor(widens bool) terminalproto.SetupFleetResult {
	file := terminalproto.SetupFilePlan{Agent: "claude", Kind: "settings", Name: "settings.json", Destination: "/home/t/.claude/settings.json", State: "merge", Fields: []string{"permissions"}}
	if widens {
		file.Widens = []string{"permissions"}
	}
	return terminalproto.SetupFleetResult{Op: terminalproto.SetupOpPlan, State: terminalproto.SetupOK, Fingerprint: strings.Repeat("ab", 32), Files: []terminalproto.SetupFilePlan{file}}
}

// connect answers each setup operation with answer, or a plain default.
func (f *setupFleetAPI) connect(t *testing.T, index int, answer func(terminalproto.SetupFleet) terminalproto.SetupFleetResult) {
	t.Helper()
	agent := f.agents[index]
	var peer *terminalbroker.Peer
	peer, err := f.srv.s.terminals.Connect(f.uid, agent.ID, f.generations[index], true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch body := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.SetupFleet:
			f.mu.Lock()
			f.calls = append(f.calls, agent.Host+"/"+body.Op)
			f.mu.Unlock()
			reply.Body = answer(body)
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		data, err := terminalproto.Encode(reply)
		if err != nil {
			return err
		}
		decoded, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		return peer.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := peer.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(peer.Close)
}

// machine answers like a target whose plan is plan.
func machine(plan terminalproto.SetupFleetResult) func(terminalproto.SetupFleet) terminalproto.SetupFleetResult {
	return func(in terminalproto.SetupFleet) terminalproto.SetupFleetResult {
		switch in.Op {
		case terminalproto.SetupOpRead:
			return terminalproto.SetupFleetResult{Op: in.Op, State: terminalproto.SetupOK, Items: sourceItems()}
		case terminalproto.SetupOpPlan:
			return plan
		case terminalproto.SetupOpApply:
			if in.Fingerprint != plan.Fingerprint {
				return terminalproto.SetupFleetResult{Op: in.Op, State: terminalproto.SetupRefused, Reason: terminalproto.SetupReasonChanged}
			}
			return terminalproto.SetupFleetResult{Op: in.Op, State: terminalproto.SetupOK, Journal: "restore-" + in.Label + "-1", Fingerprint: in.Fingerprint}
		}
		return terminalproto.SetupFleetResult{Op: in.Op, State: terminalproto.SetupOK}
	}
}

func (f *setupFleetAPI) request(t *testing.T, cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		r.Header.Set("Origin", "https://reet.example")
	}
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	return w
}

type setupReview struct {
	ReadID string `json:"readId"`
	Source struct {
		AgentID string `json:"agentId"`
		Host    string `json:"host"`
		ReadAt  int64  `json:"readAt"`
	} `json:"source"`
	Items    []setupItemView `json:"items"`
	Machines []setupMachine  `json:"machines"`
}

func (f *setupFleetAPI) review(t *testing.T, source int, targets ...int) setupReview {
	t.Helper()
	ids := make([]string, 0, len(targets))
	for _, i := range targets {
		ids = append(ids, f.agents[i].ID)
	}
	w := f.request(t, f.cookie, http.MethodPost, "/api/v2/setup/fleet/source", fmt.Sprintf(`{"agentId":%q,"targets":%s}`, f.agents[source].ID, quoteList(ids)))
	if w.Code != http.StatusOK {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	var out setupReview
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f *setupFleetAPI) start(t *testing.T, requestID, readID, confirm string, agents ...int) (int, store.SetupBatch, string) {
	t.Helper()
	ids := make([]string, 0, len(agents))
	for _, i := range agents {
		ids = append(ids, f.agents[i].ID)
	}
	w := f.request(t, f.cookie, http.MethodPost, "/api/v2/setup/fleet/batches", fmt.Sprintf(`{"requestId":%q,"readId":%q,"agents":%s,"confirm":%q}`, requestID, readID, quoteList(ids), confirm))
	var batch store.SetupBatch
	_ = json.Unmarshal(w.Body.Bytes(), &batch)
	return w.Code, batch, w.Body.String()
}

func (f *setupFleetAPI) wait(t *testing.T, id, want string) store.SetupBatch {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := f.st.SetupBatchFor(f.uid, id)
		if err != nil {
			t.Fatal(err)
		}
		if b.State == want {
			return b
		}
		time.Sleep(5 * time.Millisecond)
	}
	b, _ := f.st.SetupBatchFor(f.uid, id)
	t.Fatalf("batch %s, want %s: %+v", b.State, want, b)
	return b
}

func TestSetupFleetReviewsEachMachineThenAppliesInOrder(t *testing.T) {
	// 0 is the source; 1 and 2 accept; 3 has allow-apply off; 4 is offline;
	// 5 runs an older reet.
	f := setupFleetFixture(t, 6, 5)
	f.connect(t, 0, machine(planFor(false)))
	f.connect(t, 1, machine(planFor(false)))
	f.connect(t, 2, machine(planFor(false)))
	f.connect(t, 3, func(in terminalproto.SetupFleet) terminalproto.SetupFleetResult {
		return terminalproto.SetupFleetResult{Op: in.Op, State: terminalproto.SetupRefused, Reason: terminalproto.SetupReasonNotAllowed}
	})
	f.connect(t, 5, machine(planFor(false)))
	review := f.review(t, 0, 2, 1, 3, 4, 5, 0)
	if review.Source.Host != "machine0" || review.Source.ReadAt == 0 || len(review.Items) != 2 || review.Items[0].Bytes == 0 {
		t.Fatalf("review: %+v", review)
	}
	if strings.Contains(fmt.Sprint(review.Items), "British") {
		t.Fatal("the review returned setup text")
	}
	states := map[string]string{}
	for _, m := range review.Machines {
		states[m.Host] = m.State + "/" + m.Code
	}
	want := map[string]string{"machine1": "ready/", "machine2": "ready/", "machine3": "excluded/not_allowed", "machine4": "excluded/offline", "machine5": "excluded/unsupported", "machine0": "excluded/same_machine"}
	for host, state := range want {
		if states[host] != state {
			t.Fatalf("%s: %s, want %s (%v)", host, states[host], state, states)
		}
	}

	code, batch, body := f.start(t, "setup_request_00001", review.ReadID, "", 2, 1, 3)
	if code != http.StatusCreated {
		t.Fatalf("start: %d %s", code, body)
	}
	if batch.SourceHost != "machine0" || batch.ReadAt != review.Source.ReadAt || batch.Widens || !batch.ContentHeld {
		t.Fatalf("batch: %+v", batch)
	}
	if batch.Members[2].State != store.SoftwareMemberExcluded || !strings.Contains(batch.Members[2].Reason, "allow-apply") {
		t.Fatalf("excluded member: %+v", batch.Members[2])
	}
	done := f.wait(t, batch.ID, store.SoftwareBatchDone)
	if done.ContentHeld || done.Members[0].Journal == "" || done.Members[1].State != store.SoftwareMemberApplied {
		t.Fatalf("done: %+v", done)
	}
	f.mu.Lock()
	var applies []string
	for _, call := range f.calls {
		if strings.HasSuffix(call, "/apply") {
			applies = append(applies, call)
		}
	}
	f.mu.Unlock()
	if len(applies) != 2 || applies[0] != "machine2/apply" || applies[1] != "machine1/apply" {
		t.Fatalf("apply order: %v", applies)
	}
	// The review was used up: the text now lived only in the batch, and is gone.
	if code, _, _ := f.start(t, "setup_request_00002", review.ReadID, "", 1); code != http.StatusGone {
		t.Fatalf("reused review: %d", code)
	}

	// Undo one machine, once.
	w := f.request(t, f.cookie, http.MethodPost, "/api/v2/setup/fleet/batches/"+batch.ID+"/members/0/undo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"undoneAt"`) {
		t.Fatalf("undo: %d %s", w.Code, w.Body.String())
	}
	w = f.request(t, f.cookie, http.MethodPost, "/api/v2/setup/fleet/batches/"+batch.ID+"/members/0/undo", "")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "setup_undo_done") {
		t.Fatalf("second undo: %d %s", w.Code, w.Body.String())
	}
	w = f.request(t, f.cookie, http.MethodPost, "/api/v2/setup/fleet/batches/"+batch.ID+"/members/2/undo", "")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "setup_undo_unavailable") {
		t.Fatalf("undo an excluded machine: %d %s", w.Code, w.Body.String())
	}
}

func TestSetupFleetWideningNeedsTheTypedWordAndRequestIDsAreIdempotent(t *testing.T) {
	f := setupFleetFixture(t, 3)
	f.connect(t, 0, machine(planFor(false)))
	f.connect(t, 1, machine(planFor(true)))
	f.connect(t, 2, machine(planFor(false)))
	review := f.review(t, 0, 1, 2)
	for _, confirm := range []string{"", "yes", "Copy"} {
		code, _, body := f.start(t, "setup_request_00010", review.ReadID, confirm, 1, 2)
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, "setup_confirm_required") {
			t.Fatalf("confirm %q: %d %s", confirm, code, body)
		}
	}
	code, batch, body := f.start(t, "setup_request_00010", review.ReadID, "copy", 1, 2)
	if code != http.StatusCreated || !batch.Widens || !batch.Members[0].Widens || batch.Members[0].Files[0].Widens[0] != "permissions" {
		t.Fatalf("start: %d %s", code, body)
	}
	// The same request id answers the batch it made, even with the review gone.
	code, again, _ := f.start(t, "setup_request_00010", "gone", "", 2)
	if code != http.StatusOK || again.ID != batch.ID {
		t.Fatalf("replay: %d %+v", code, again)
	}
	f.wait(t, batch.ID, store.SoftwareBatchDone)
	// A machine that would not widen needs no word at all.
	review = f.review(t, 0, 2)
	if code, _, body := f.start(t, "setup_request_00011", review.ReadID, "", 2); code != http.StatusCreated {
		t.Fatalf("plain batch: %d %s", code, body)
	}
}

func TestSetupFleetRefusesTheSourceAloneAndCredentials(t *testing.T) {
	f := setupFleetFixture(t, 2)
	token := "ghp_" + strings.Repeat("Z9y8", 9)
	leaky := true
	var mu sync.Mutex
	f.connect(t, 0, func(in terminalproto.SetupFleet) terminalproto.SetupFleetResult {
		mu.Lock()
		defer mu.Unlock()
		if in.Op == terminalproto.SetupOpRead && leaky {
			// A machine that skipped its own check: the controller checks again.
			items := append(sourceItems(), setupTestItem("codex", "instructions", "AGENTS.md", "text", "key "+token))
			return terminalproto.SetupFleetResult{Op: in.Op, State: terminalproto.SetupOK, Items: items}
		}
		return machine(planFor(false))(in)
	})
	f.connect(t, 1, machine(planFor(false)))
	w := f.request(t, f.cookie, http.MethodPost, "/api/v2/setup/fleet/source", fmt.Sprintf(`{"agentId":%q}`, f.agents[0].ID))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "setup_credential") || !strings.Contains(w.Body.String(), "AGENTS.md") || strings.Contains(w.Body.String(), token) {
		t.Fatalf("credential: %d %s", w.Code, w.Body.String())
	}
	mu.Lock()
	leaky = false
	mu.Unlock()
	review := f.review(t, 0, 1)
	if code, _, body := f.start(t, "setup_request_00020", review.ReadID, "", 0); code != http.StatusConflict || !strings.Contains(body, "setup_batch_source_only") {
		t.Fatalf("source only: %d %s", code, body)
	}
	if code, _, body := f.start(t, "setup_request_00021", review.ReadID, "", 1, 1); code != http.StatusBadRequest || !strings.Contains(body, "setup_batch_duplicate") {
		t.Fatalf("duplicate: %d %s", code, body)
	}
	w = f.request(t, f.cookie, http.MethodPost, "/api/v2/setup/fleet/batches", `{"requestId":"setup_request_00022","readId":"x","agents":[],"extra":1}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", w.Code)
	}
}

func TestSetupFleetIsForWritersOnly(t *testing.T) {
	f := setupFleetFixture(t, 2)
	viewer, err := f.st.CreateUser("setup-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	cookie := terminalLogin(t, f.st, viewer.ID)
	for _, path := range []string{"/api/v2/setup/fleet/source", "/api/v2/setup/fleet/batches"} {
		if w := f.request(t, cookie, http.MethodPost, path, `{}`); w.Code != http.StatusForbidden {
			t.Fatalf("viewer %s: %d", path, w.Code)
		}
	}
	if w := f.request(t, cookie, http.MethodGet, "/api/v2/setup/fleet/batches/active", ""); w.Code != http.StatusForbidden {
		t.Fatalf("viewer active: %d", w.Code)
	}
}

func TestSetupFleetStopsOnAChangedMachineAndUndoRefusesAChangedFile(t *testing.T) {
	f := setupFleetFixture(t, 3)
	f.connect(t, 0, machine(planFor(false)))
	changed := planFor(false)
	f.connect(t, 1, func(in terminalproto.SetupFleet) terminalproto.SetupFleetResult {
		if in.Op == terminalproto.SetupOpApply {
			return terminalproto.SetupFleetResult{Op: in.Op, State: terminalproto.SetupRefused, Reason: terminalproto.SetupReasonChanged}
		}
		return machine(changed)(in)
	})
	f.connect(t, 2, func(in terminalproto.SetupFleet) terminalproto.SetupFleetResult {
		if in.Op == terminalproto.SetupOpRollback {
			return terminalproto.SetupFleetResult{Op: in.Op, State: terminalproto.SetupRefused, Reason: terminalproto.SetupReasonChanged}
		}
		return machine(planFor(false))(in)
	})
	review := f.review(t, 0, 1, 2)
	code, batch, body := f.start(t, "setup_request_00030", review.ReadID, "", 1, 2)
	if code != http.StatusCreated {
		t.Fatalf("start: %d %s", code, body)
	}
	stopped := f.wait(t, batch.ID, store.SoftwareBatchStopped)
	if stopped.Members[0].Code != "changed" || stopped.Members[1].State != store.SoftwareMemberPending {
		t.Fatalf("stopped: %+v", stopped)
	}
	w := f.request(t, f.cookie, http.MethodGet, "/api/v2/setup/fleet/batches/active", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), batch.ID) {
		t.Fatalf("active: %d %s", w.Code, w.Body.String())
	}
	w = f.request(t, f.cookie, http.MethodPost, "/api/v2/setup/fleet/batches/"+batch.ID+"/members/0/skip", "")
	if w.Code != http.StatusOK {
		t.Fatalf("skip: %d %s", w.Code, w.Body.String())
	}
	done := f.wait(t, batch.ID, store.SoftwareBatchDone)
	if done.Members[1].State != store.SoftwareMemberApplied {
		t.Fatalf("after skip: %+v", done.Members[1])
	}
	w = f.request(t, f.cookie, http.MethodPost, "/api/v2/setup/fleet/batches/"+batch.ID+"/members/1/undo", "")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "setup_changed") {
		t.Fatalf("undo of a changed file: %d %s", w.Code, w.Body.String())
	}
	if b, _ := f.st.SetupBatchFor(f.uid, batch.ID); b.Members[1].UndoneAt != 0 {
		t.Fatal("a refused undo was recorded")
	}
	events, err := f.st.ListEventsFor(f.uid, 200, "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Action] = true
	}
	for _, action := range []string{"setup-read", "setup-batch", "setup-batch-skip", "setup-batch-undo"} {
		if !seen[action] {
			t.Fatalf("no %s audit: %v", action, seen)
		}
	}
}
