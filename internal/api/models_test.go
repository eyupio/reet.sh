package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// modelsAPI is a server with machines connected in memory that answer models
// requests with answer (nil holds the request, as a lost answer).
type modelsAPI struct {
	srv    *Server
	st     *store.Store
	uid    int64
	agent  string
	cookie string
	peers  map[string]*terminalbroker.Peer
	mu     sync.Mutex
	asked  []terminalproto.Body
	answer func(terminalproto.Body) terminalproto.Body
}

func modelsCaps() terminalproto.Capabilities {
	caps := launchCaps("opencode")
	caps.Models = true
	return caps
}

func modelsAPIFixture(t *testing.T, caps terminalproto.Capabilities) *modelsAPI {
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
	f := &modelsAPI{srv: srv, st: st, uid: uid, cookie: terminalLogin(t, st, uid), peers: map[string]*terminalbroker.Peer{}}
	f.agent = f.addMachine(t, "studio", caps)
	f.answering(func(terminalproto.Body) terminalproto.Body { return modelsListed() })
	return f
}

func (f *modelsAPI) addMachine(t *testing.T, host string, caps terminalproto.Capabilities) string {
	t.Helper()
	agent, _, err := f.st.EnrollSyncAgent(f.uid, host, "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err := f.st.BeginRemoteConnectionFor(f.uid, agent.ID, terminalbroker.ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	var p *terminalbroker.Peer
	p, err = f.srv.s.terminals.Connect(f.uid, agent.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Models:
			f.mu.Lock()
			f.asked = append(f.asked, op)
			answer := f.answer
			f.mu.Unlock()
			if answer == nil {
				return nil
			}
			reply.Body = answer(op)
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
		return p.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.peers[agent.ID] = p
	return agent.ID
}

func (f *modelsAPI) answering(fn func(terminalproto.Body) terminalproto.Body) {
	f.mu.Lock()
	f.answer = fn
	f.mu.Unlock()
}

func (f *modelsAPI) sentRequests() []terminalproto.Body {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Body(nil), f.asked...)
}

func modelsListed() terminalproto.ModelsResult {
	return terminalproto.ModelsResult{Tool: "opencode", Models: []terminalproto.ModelEntry{
		{ID: "anthropic/claude-sonnet-4-5", Provider: "anthropic"},
		{ID: "openai/gpt-5", Provider: "openai"},
	}, Version: "1.19.5"}
}

func modelsPath(agent, tool string) string {
	q := url.Values{}
	if tool != "" {
		q.Set("tool", tool)
	}
	return "/api/v2/machines/" + agent + "/models?" + q.Encode()
}

func modelsRefreshPath(agent string) string {
	return "/api/v2/machines/" + agent + "/models/refresh"
}

func (f *modelsAPI) get(t *testing.T, cookie, path string, headers ...string) (int, map[string]any, string) {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, w.Body.String()
}

func (f *modelsAPI) refresh(t *testing.T, cookie, agent, body string) (int, map[string]any, string) {
	t.Helper()
	r := httptest.NewRequest("POST", modelsRefreshPath(agent), strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://reet.example")
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, w.Body.String()
}

func modelsField(t *testing.T, out map[string]any, name string) []any {
	t.Helper()
	v, ok := out[name].([]any)
	if !ok {
		t.Fatalf("%s is %T, want an array: %v", name, out[name], out)
	}
	return v
}

func TestMachineModelsCacheHitNeedsNoMachineRead(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	if err := f.st.UpsertMachineModelsFor(f.uid, store.MachineModels{
		AgentID: f.agent, Tool: "opencode", State: store.MachineModelsListed,
		Models: []terminalproto.ModelEntry{{ID: "cached/model", Provider: "cached"}},
		More:   true, Source: "opencode models", AgentVersion: "1.0.0", FetchedAt: 1000, CheckedAt: 1100,
	}); err != nil {
		t.Fatal(err)
	}
	code, out, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
	if code != 200 {
		t.Fatal(code, raw)
	}
	if out["tool"] != "opencode" || out["state"] != store.MachineModelsListed {
		t.Fatal(raw)
	}
	models := modelsField(t, out, "models")
	if len(models) != 1 || models[0].(map[string]any)["id"] != "cached/model" {
		t.Fatal(raw)
	}
	if out["fetchedAt"] != float64(1000) || out["checkedAt"] != float64(1100) || out["version"] != "1.0.0" || out["more"] != true {
		t.Fatal(raw)
	}
	if out["sentence"] != "Models read from studio." {
		t.Fatal(raw)
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatal("a cache hit reached the machine", sent)
	}
}

func TestMachineModelsFirstReadIsSynchronousAndStored(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	code, out, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
	if code != 200 || out["state"] != store.MachineModelsListed {
		t.Fatal(code, raw)
	}
	if models := modelsField(t, out, "models"); len(models) != 2 {
		t.Fatal(raw)
	}
	if out["reason"] != "" || out["sentence"] != "Models read from studio." {
		t.Fatal(raw)
	}
	if out["fetchedAt"] == nil || out["checkedAt"] == nil {
		t.Fatal("a listed read has timestamps", raw)
	}
	sent := f.sentRequests()
	if len(sent) != 1 || sent[0] != (terminalproto.Models{Tool: "opencode"}) {
		t.Fatal("request", sent)
	}
	row, ok, err := f.st.MachineModelsFor(f.uid, f.agent, "opencode")
	if err != nil || !ok || row.State != store.MachineModelsListed || len(row.Models) != 2 || row.FetchedAt == 0 {
		t.Fatal("not stored", row, ok, err)
	}
	// The next read is from the cache.
	if code, _, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode")); code != 200 {
		t.Fatal(code, raw)
	}
	if sent := f.sentRequests(); len(sent) != 1 {
		t.Fatal("a second read reached the machine", sent)
	}
}

func TestMachineModelsStatesWithoutACache(t *testing.T) {
	t.Run("offline", func(t *testing.T) {
		f := modelsAPIFixture(t, modelsCaps())
		f.peers[f.agent].Close()
		code, out, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
		if code != 200 || out["state"] != "offline" || out["sentence"] != sentenceMachineOffline || out["fetchedAt"] != nil {
			t.Fatal(code, raw)
		}
		if len(f.sentRequests()) != 0 {
			t.Fatal("an offline machine was sent a read")
		}
	})
	t.Run("update", func(t *testing.T) {
		caps := modelsCaps()
		caps.Models = false
		f := modelsAPIFixture(t, caps)
		code, out, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
		if code != 200 || out["state"] != "update" || out["sentence"] != "Update reet on that machine to read its models." {
			t.Fatal(code, raw)
		}
		if len(f.sentRequests()) != 0 {
			t.Fatal("an old connector was sent a read")
		}
	})
	t.Run("off", func(t *testing.T) {
		caps := modelsCaps()
		caps.Models, caps.ModelsOff = false, true
		f := modelsAPIFixture(t, caps)
		code, out, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
		if code != 200 || out["state"] != "off" || out["sentence"] != "Models are switched off on that machine." {
			t.Fatal(code, raw)
		}
	})
	t.Run("unsupported", func(t *testing.T) {
		f := modelsAPIFixture(t, modelsCaps())
		f.answering(func(terminalproto.Body) terminalproto.Body {
			return terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsUnsupported}
		})
		code, out, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
		if code != 200 || out["state"] != store.MachineModelsUnsupported || out["reason"] != "unsupported" ||
			out["sentence"] != "reet can't read this agent's models; it uses its own list." {
			t.Fatal(code, raw)
		}
		row, ok, err := f.st.MachineModelsFor(f.uid, f.agent, "opencode")
		if err != nil || !ok || row.State != store.MachineModelsUnsupported || row.FetchedAt != 0 {
			t.Fatal("unsupported not cached", row, ok, err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		f := modelsAPIFixture(t, modelsCaps())
		f.answering(func(terminalproto.Body) terminalproto.Body {
			return terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsTimedOut}
		})
		code, out, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
		if code != 200 || out["state"] != store.MachineModelsFailed || out["reason"] != "timed_out" ||
			out["sentence"] != "That machine didn't answer in time. Try again." {
			t.Fatal(code, raw)
		}
	})
	t.Run("broker timeout", func(t *testing.T) {
		old := terminalbroker.ModelsCallTimeout
		terminalbroker.ModelsCallTimeout = 50 * time.Millisecond
		t.Cleanup(func() { terminalbroker.ModelsCallTimeout = old })
		f := modelsAPIFixture(t, modelsCaps())
		f.answering(nil)
		code, out, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
		if code != 200 || out["state"] != store.MachineModelsFailed || out["reason"] != "timed_out" ||
			out["sentence"] != "That machine didn't answer in time. Try again." || out["fetchedAt"] != nil {
			t.Fatal(code, raw)
		}
		row, ok, err := f.st.MachineModelsFor(f.uid, f.agent, "opencode")
		if err != nil || !ok || row.State != store.MachineModelsFailed || row.Reason != terminalproto.ModelsTimedOut || row.FetchedAt != 0 {
			t.Fatal("timeout not cached", row, ok, err)
		}
	})
	t.Run("failed", func(t *testing.T) {
		f := modelsAPIFixture(t, modelsCaps())
		f.answering(func(terminalproto.Body) terminalproto.Body {
			return terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsExited}
		})
		code, out, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
		if code != 200 || out["state"] != store.MachineModelsFailed || out["reason"] != "exited" ||
			out["sentence"] != "That machine couldn't read its models." {
			t.Fatal(code, raw)
		}
		if models := modelsField(t, out, "models"); len(models) != 0 {
			t.Fatal(raw)
		}
	})
}

func TestMachineModelsRefreshOfflineKeepsTheCachedList(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	if err := f.st.UpsertMachineModelsFor(f.uid, store.MachineModels{
		AgentID: f.agent, Tool: "opencode", State: store.MachineModelsListed,
		Models: []terminalproto.ModelEntry{{ID: "cached/model", Provider: "cached"}},
		Source: "opencode models", FetchedAt: 1000, CheckedAt: 1100,
	}); err != nil {
		t.Fatal(err)
	}
	f.peers[f.agent].Close()
	code, out, raw := f.refresh(t, f.cookie, f.agent, `{"tool":"opencode"}`)
	if code != 200 || out["state"] != "offline" || out["sentence"] != sentenceMachineOffline {
		t.Fatal(code, raw)
	}
	if models := modelsField(t, out, "models"); len(models) != 1 || models[0].(map[string]any)["id"] != "cached/model" {
		t.Fatal("the cached list was dropped", raw)
	}
	if out["fetchedAt"] != float64(1000) {
		t.Fatal("the last good fetchedAt was lost", raw)
	}
}

func TestMachineModelsRefreshKeepsLastGoodOnFailure(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	code, out, raw := f.refresh(t, f.cookie, f.agent, `{"tool":"opencode"}`)
	if code != 200 || out["state"] != store.MachineModelsListed {
		t.Fatal(code, raw)
	}
	firstFetched := out["fetchedAt"]
	if models := modelsField(t, out, "models"); len(models) != 2 {
		t.Fatal(raw)
	}
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsExited}
	})
	code, out, raw = f.refresh(t, f.cookie, f.agent, `{"tool":"opencode"}`)
	if code != 200 || out["state"] != store.MachineModelsFailed || out["reason"] != "exited" || out["sentence"] != "That machine couldn't read its models." {
		t.Fatal(code, raw)
	}
	if models := modelsField(t, out, "models"); len(models) != 2 || models[0].(map[string]any)["id"] != "anthropic/claude-sonnet-4-5" {
		t.Fatal("failed refresh lost the last good list", raw)
	}
	if out["fetchedAt"] != firstFetched || out["checkedAt"] == nil {
		t.Fatal("failed refresh changed fetchedAt", raw)
	}
	// The cache also keeps the last good list.
	code, out, raw = f.get(t, f.cookie, modelsPath(f.agent, "opencode"))
	if code != 200 || out["state"] != store.MachineModelsFailed || len(modelsField(t, out, "models")) != 2 {
		t.Fatal(code, raw)
	}
}

func TestMachineModelsRefreshRateLimit(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	for i := 0; i < 6; i++ {
		if code, _, raw := f.refresh(t, f.cookie, f.agent, `{"tool":"opencode"}`); code != 200 {
			t.Fatal(i, code, raw)
		}
	}
	code, out, raw := f.refresh(t, f.cookie, f.agent, `{"tool":"opencode"}`)
	if code != http.StatusTooManyRequests || out["code"] != "models_rate_limited" {
		t.Fatal("7th refresh", code, raw)
	}
	if sent := f.sentRequests(); len(sent) != 6 {
		t.Fatal("the rate-limited refresh reached the machine", sent)
	}
	second := f.addMachine(t, "laptop", modelsCaps())
	if code, _, raw := f.refresh(t, f.cookie, second, `{"tool":"opencode"}`); code != 200 {
		t.Fatal("another machine", code, raw)
	}
}

func TestMachineModelsRefreshBusy(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsConflict}
	})
	code, out, raw := f.refresh(t, f.cookie, f.agent, `{"tool":"opencode"}`)
	if code != http.StatusConflict || out["code"] != "models_busy" ||
		out["error"] != "That machine is already reading its models. Try again in a moment." {
		t.Fatal(code, raw)
	}
}

func TestMachineModelsRefreshIsStrict(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	for name, body := range map[string]string{
		"unknown field": `{"tool":"opencode","extra":true}`,
		"invalid tool":  `{"tool":"pi"}`,
		"trailing":      `{"tool":"opencode"} {}`,
	} {
		if code, _, raw := f.refresh(t, f.cookie, f.agent, body); code != http.StatusBadRequest {
			t.Fatal(name, code, raw)
		}
	}
	if len(f.sentRequests()) != 0 {
		t.Fatal("an invalid refresh reached the machine")
	}
}

func TestModelsListIsCacheOnlyAndMapsHosts(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	second := f.addMachine(t, "laptop", modelsCaps())
	for _, row := range []struct {
		agent string
		host  string
	}{
		{f.agent, "studio"},
		{second, "laptop"},
	} {
		if err := f.st.UpsertMachineModelsFor(f.uid, store.MachineModels{
			AgentID: row.agent, Tool: "opencode", State: store.MachineModelsListed,
			Models: []terminalproto.ModelEntry{{ID: "provider/model", Provider: "provider"}},
			Source: "opencode models", FetchedAt: 1000, CheckedAt: 1000,
		}); err != nil {
			t.Fatal(err)
		}
	}
	other, err := f.st.CreateUser("models-list-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherAgent, _, err := f.st.EnrollSyncAgent(other.ID, "other-host", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.UpsertMachineModelsFor(other.ID, store.MachineModels{
		AgentID: otherAgent.ID, Tool: "opencode", State: store.MachineModelsListed,
		Models: []terminalproto.ModelEntry{{ID: "other/model", Provider: "other"}}, FetchedAt: 2000, CheckedAt: 2000,
	}); err != nil {
		t.Fatal(err)
	}
	code, out, raw := f.get(t, f.cookie, "/api/v2/models?tool=opencode")
	if code != 200 {
		t.Fatal(code, raw)
	}
	machines := modelsField(t, out, "machines")
	if len(machines) != 2 {
		t.Fatal("cache rows", raw)
	}
	hosts := map[string]bool{}
	for _, item := range machines {
		m := item.(map[string]any)
		hosts[m["host"].(string)] = true
		if m["agentId"] != f.agent && m["agentId"] != second {
			t.Fatal("another account's row leaked", raw)
		}
		if m["state"] != store.MachineModelsListed || m["fetchedAt"] != float64(1000) || m["sentence"] == "" {
			t.Fatal(raw)
		}
	}
	if !hosts["studio"] || !hosts["laptop"] {
		t.Fatal("hosts", raw)
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatal("the list read the machine", sent)
	}
	if code, _, _ := f.get(t, f.cookie, "/api/v2/models?tool=pi"); code != http.StatusBadRequest {
		t.Fatal("invalid tool", code)
	}
}

func TestModelsRoutesAreOwnerScoped(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	other, err := f.st.CreateUser("models-scope-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherCookie := terminalLogin(t, f.st, other.ID)
	code, out, raw := f.get(t, otherCookie, modelsPath(f.agent, "opencode"))
	if code != 200 || out["state"] != "offline" || out["sentence"] != sentenceMachineOffline {
		t.Fatal(code, raw)
	}
	if code, out, raw := f.get(t, otherCookie, "/api/v2/models?tool=opencode"); code != 200 || len(modelsField(t, out, "machines")) != 0 {
		t.Fatal("another account saw a cache row", code, raw)
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatal("another account reached the machine", sent)
	}
}

func TestModelsRoutesRefuseQueryCredentials(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	for _, path := range []string{
		modelsPath(f.agent, "opencode") + "&t=fixture-owner",
		modelsRefreshPath(f.agent) + "?t=fixture-owner",
	} {
		method := "GET"
		if strings.HasSuffix(path, "/refresh?t=fixture-owner") {
			method = "POST"
		}
		r := httptest.NewRequest(method, path, strings.NewReader(`{"tool":"opencode"}`))
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Origin", "https://reet.example")
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
		w := httptest.NewRecorder()
		f.srv.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest && w.Code != http.StatusForbidden {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatal("a URL credential reached the machine", sent)
	}
}

func TestModelsAuditIsRefreshOnlyAndMetadataOnly(t *testing.T) {
	f := modelsAPIFixture(t, modelsCaps())
	if code, _, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode")); code != 200 {
		t.Fatal(code, raw)
	}
	if events := worktreeEvents(t, f.st, f.uid, "machine-models-read"); len(events) != 0 {
		t.Fatal("a cache read was audited", events)
	}
	if code, _, raw := f.refresh(t, f.cookie, f.agent, `{"tool":"opencode"}`); code != 200 {
		t.Fatal(code, raw)
	}
	events := worktreeEvents(t, f.st, f.uid, "machine-models-read")
	if len(events) != 1 {
		t.Fatal("refresh audit", events)
	}
	if strings.Contains(events[0].Detail, "anthropic/claude") || strings.Contains(events[0].Detail, "openai/gpt") {
		t.Fatal("audit detail carried model ids", events[0].Detail)
	}
	detail := map[string]any{}
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail) != 4 || detail["machine"] != f.agent || detail["tool"] != "opencode" || detail["state"] != store.MachineModelsListed || detail["count"] != float64(2) {
		t.Fatal("audit detail", detail)
	}
	if code, _, raw := f.get(t, f.cookie, modelsPath(f.agent, "opencode")); code != 200 {
		t.Fatal(code, raw)
	}
	if events := worktreeEvents(t, f.st, f.uid, "machine-models-read"); len(events) != 1 {
		t.Fatal("a cache read was audited", events)
	}
}
