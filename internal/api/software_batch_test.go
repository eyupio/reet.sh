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

type softwareBatchAPI struct {
	srv         *Server
	st          *store.Store
	uid         int64
	cookie      string
	agents      []store.SyncAgent
	generations []int64
}

func softwareBatchAPIFixture(t *testing.T, machines int) *softwareBatchAPI {
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
	f := &softwareBatchAPI{srv: srv, st: st, uid: uid, cookie: terminalLogin(t, st, uid)}
	caps := softwareTestCapabilities(softwareTestEntry(terminalproto.SoftwareInstalled, "1.18.0"))
	caps.SoftwareApply = true
	for i := 0; i < machines; i++ {
		a, _, err := st.EnrollSyncAgent(uid, fmt.Sprintf("machine%d", i), "test")
		if err != nil {
			t.Fatal(err)
		}
		record, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), caps)
		if err != nil {
			t.Fatal(err)
		}
		f.agents = append(f.agents, a)
		f.generations = append(f.generations, record.Generation)
	}
	return f
}

func (f *softwareBatchAPI) connect(t *testing.T, index int, answer func(terminalproto.SoftwareApply) terminalproto.SoftwareApplyResult) {
	t.Helper()
	agent := f.agents[index]
	var peer *terminalbroker.Peer
	peer, err := f.srv.s.terminals.Connect(f.uid, agent.ID, f.generations[index], true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch body := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.SoftwareApply:
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

func (f *softwareBatchAPI) request(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		r.Header.Set("Origin", "https://reet.example")
	}
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	return w
}

func (f *softwareBatchAPI) start(t *testing.T, requestID string, agents ...string) (int, store.SoftwareBatch, string) {
	t.Helper()
	body := fmt.Sprintf(`{"requestId":%q,"tool":"opencode","agents":%s}`, requestID, quoteList(agents))
	w := f.request(t, http.MethodPost, "/api/v2/software/batches", body)
	var batch store.SoftwareBatch
	_ = json.Unmarshal(w.Body.Bytes(), &batch)
	return w.Code, batch, w.Body.String()
}

func quoteList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

func (f *softwareBatchAPI) wait(t *testing.T, batchID string, want ...string) store.SoftwareBatch {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last store.SoftwareBatch
	for time.Now().Before(deadline) {
		b, err := f.st.SoftwareBatchFor(f.uid, batchID)
		if err != nil {
			t.Fatal(err)
		}
		last = b
		for _, state := range want {
			if b.State == state {
				return b
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("batch stayed %s, wanted %v: %+v", last.State, want, last)
	return last
}

func appliedSoftware(in terminalproto.SoftwareApply) terminalproto.SoftwareApplyResult {
	entry := softwareTestEntry(terminalproto.SoftwareInstalled, "1.19.5")
	return terminalproto.SoftwareApplyResult{
		Tool: in.Tool, Action: in.Action, State: terminalproto.SoftwareApplyApplied,
		Version: "1.19.5", Previous: "1.18.0", Entry: &entry,
	}
}

func TestSoftwareBatchAppliesInOrder(t *testing.T) {
	f := softwareBatchAPIFixture(t, 3)
	var mu sync.Mutex
	var calls []string
	for i := 0; i < 3; i++ {
		i := i
		f.connect(t, i, func(in terminalproto.SoftwareApply) terminalproto.SoftwareApplyResult {
			mu.Lock()
			calls = append(calls, f.agents[i].ID)
			mu.Unlock()
			return appliedSoftware(in)
		})
	}
	code, batch, body := f.start(t, "batch_request_0001", f.agents[0].ID, f.agents[1].ID, f.agents[2].ID)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	if len(batch.Members) != 3 || batch.Members[0].Action != "update" || batch.Members[0].Target != "1.19.5" {
		t.Fatalf("frozen batch: %+v", batch)
	}
	done := f.wait(t, batch.ID, store.SoftwareBatchDone)
	for i, m := range done.Members {
		if m.State != store.SoftwareMemberApplied || m.Version != "1.19.5" {
			t.Fatalf("member %d: %+v", i, m)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 || calls[0] != f.agents[0].ID || calls[1] != f.agents[1].ID || calls[2] != f.agents[2].ID {
		t.Fatalf("applied order: %v", calls)
	}

	// The controller stores the machine's fresh evidence as the batch applies.
	caps, err := f.st.RemoteCapabilitiesFor(f.uid, f.agents[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	version := ""
	for _, entry := range caps.Capabilities.Software {
		if entry.Tool == "opencode" {
			version = entry.Version
		}
	}
	if version != "1.19.5" {
		t.Fatalf("stored evidence after the batch: %q", version)
	}
}

func TestSoftwareBatchStopsThenSkipContinues(t *testing.T) {
	f := softwareBatchAPIFixture(t, 2)
	f.connect(t, 0, func(in terminalproto.SoftwareApply) terminalproto.SoftwareApplyResult {
		return terminalproto.SoftwareApplyResult{Tool: in.Tool, Action: in.Action, State: terminalproto.SoftwareApplyFailed, Reason: terminalproto.SoftwareApplyInstall}
	})
	f.connect(t, 1, appliedSoftware)

	code, batch, body := f.start(t, "batch_request_0002", f.agents[0].ID, f.agents[1].ID)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	stopped := f.wait(t, batch.ID, store.SoftwareBatchStopped)
	if stopped.Members[0].Code != "install_failed" || stopped.Members[1].State != store.SoftwareMemberPending {
		t.Fatalf("stopped: %+v", stopped)
	}

	// Another batch cannot start while this one waits on a decision.
	if code, _, _ := f.start(t, "batch_request_0003", f.agents[0].ID, f.agents[1].ID); code != http.StatusConflict {
		t.Fatalf("second batch: %d", code)
	}

	w := f.request(t, http.MethodPost, "/api/v2/software/batches/"+batch.ID+"/members/0/skip", "")
	if w.Code != http.StatusOK {
		t.Fatalf("skip: %d %s", w.Code, w.Body.String())
	}
	done := f.wait(t, batch.ID, store.SoftwareBatchDone)
	if done.Members[0].State != store.SoftwareMemberSkipped || done.Members[1].State != store.SoftwareMemberApplied {
		t.Fatalf("done: %+v", done)
	}
	if w := f.request(t, http.MethodPost, "/api/v2/software/batches/"+batch.ID+"/abandon", ""); w.Code != http.StatusConflict {
		t.Fatalf("abandon a finished batch: %d %s", w.Code, w.Body.String())
	}
}

func TestSoftwareBatchRetryAndAbandon(t *testing.T) {
	f := softwareBatchAPIFixture(t, 1)
	var mu sync.Mutex
	failedOnce := false
	f.connect(t, 0, func(in terminalproto.SoftwareApply) terminalproto.SoftwareApplyResult {
		mu.Lock()
		defer mu.Unlock()
		if !failedOnce {
			failedOnce = true
			return terminalproto.SoftwareApplyResult{Tool: in.Tool, Action: in.Action, State: terminalproto.SoftwareApplyFailed, Reason: terminalproto.SoftwareApplyDownload}
		}
		return appliedSoftware(in)
	})
	code, batch, body := f.start(t, "batch_request_0004", f.agents[0].ID)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	f.wait(t, batch.ID, store.SoftwareBatchStopped)

	w := f.request(t, http.MethodPost, "/api/v2/software/batches/"+batch.ID+"/members/0/retry", "")
	if w.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", w.Code, w.Body.String())
	}
	done := f.wait(t, batch.ID, store.SoftwareBatchDone)
	if done.Members[0].State != store.SoftwareMemberApplied {
		t.Fatalf("retried member: %+v", done.Members[0])
	}

	// A fresh batch with no connected machine stops offline and can be abandoned.
	second := softwareBatchAPIFixture(t, 1)
	code, batch, body = second.start(t, "batch_request_0005", second.agents[0].ID)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	second.wait(t, batch.ID, store.SoftwareBatchStopped)
	if w := second.request(t, http.MethodPost, "/api/v2/software/batches/"+batch.ID+"/abandon", ""); w.Code != http.StatusOK {
		t.Fatalf("abandon: %d %s", w.Code, w.Body.String())
	}
	abandoned := second.wait(t, batch.ID, store.SoftwareBatchAbandoned)
	if abandoned.Members[0].State != store.SoftwareMemberFailed || abandoned.Members[0].Code != "offline" {
		t.Fatalf("an abandoned batch keeps the failure it stopped on: %+v", abandoned.Members[0])
	}
}

func TestSoftwareBatchExcludesUnknownMachineAndValidatesRequest(t *testing.T) {
	f := softwareBatchAPIFixture(t, 1)
	f.connect(t, 0, appliedSoftware)
	code, batch, body := f.start(t, "batch_request_0006", f.agents[0].ID, "missing_machine")
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	done := f.wait(t, batch.ID, store.SoftwareBatchDone)
	if done.Members[1].State != store.SoftwareMemberExcluded || done.Members[1].Reason == "" {
		t.Fatalf("excluded member: %+v", done.Members[1])
	}

	if code, _, _ := f.start(t, "", f.agents[0].ID); code != http.StatusUnprocessableEntity {
		t.Fatalf("missing request id: %d", code)
	}
	if w := f.request(t, http.MethodGet, "/api/v2/software/batches/does_not_exist", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown batch: %d %s", w.Code, w.Body.String())
	}
}

func TestSoftwareBatchAuditsStartAndFinish(t *testing.T) {
	f := softwareBatchAPIFixture(t, 1)
	f.connect(t, 0, appliedSoftware)
	code, batch, body := f.start(t, "batch_request_0007", f.agents[0].ID)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	f.wait(t, batch.ID, store.SoftwareBatchDone)
	// The finish event is written by the driver just after the state commits.
	phases := map[string]bool{}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !(phases["start"] && phases["finish"]) {
		events, err := f.st.ListEventsFor(f.uid, 50, "")
		if err != nil {
			t.Fatal(err)
		}
		phases = map[string]bool{}
		for _, e := range events {
			if e.Action != "software-batch" {
				continue
			}
			var detail struct {
				Phase   string `json:"phase"`
				BatchID string `json:"batchId"`
			}
			if json.Unmarshal([]byte(e.Detail), &detail) == nil && detail.BatchID == batch.ID {
				phases[detail.Phase] = true
			}
		}
		if !(phases["start"] && phases["finish"]) {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !phases["start"] || !phases["finish"] {
		t.Fatalf("batch audit phases: %v", phases)
	}
}
