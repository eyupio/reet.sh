package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type workerAPIFake struct {
	st       *store.Store
	enqueues atomic.Int32
}

func (f *workerAPIFake) ChatControl(context.Context, int64, string, terminalproto.ACPControl) (terminalproto.ACPControlResult, error) {
	return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Working: true}}, nil
}
func (f *workerAPIFake) EnqueueChat(uid int64, session, id, text string, images []string) error {
	f.enqueues.Add(1)
	return f.st.EnqueueTerminalFor(uid, session, id, text, images)
}
func (f *workerAPIFake) ReconcileWorkerQueue(uid int64, session string) error {
	return f.st.RecoverTerminalQueueFor(uid, session, "api-worker")
}
func TestWorkerMessagingHTTPAndMCPRequireSeparateGrants(t *testing.T) {
	t.Parallel()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(nil)
	h := New(st, meta.Fetcher{}, "", WithAuthMode(AuthAccounts), WithPublicURL("http://"+ts.Listener.Addr().String()), WithRemoteTerminals(true))
	ts.Config.Handler = h
	ts.Start()
	t.Cleanup(func() { ts.Close(); h.Close(); st.Close() })
	fake := &workerAPIFake{st: st}
	h.s.gafferProviders.SetWorker(fake, h.s.terminalEnabled)
	browser := &http.Client{Jar: mustJar(t)}
	contextAPIJSON(t, browser, ts, "POST", "/api/v2/auth/setup", map[string]any{"name": "Owner", "email": "worker-owner@example.test", "password": "long worker password"}, 200)
	user, _, err := st.OwnerUser()
	if err != nil {
		t.Fatal(err)
	}
	session := contextAPISession(t, st, user.ID)
	if err = st.ObserveTerminalStateFor(user.ID, session.AgentID, session.ID, 1, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	token, clientID := mcpManual(t, browser, ts, "write")
	client := mcpSession(t, ts, token)
	id := strings.Repeat("a", 32)
	args := map[string]any{"session_id": session.ID, "request_id": id, "text": "Raise a PR; do not merge."}
	denied := func(name string, in map[string]any) {
		t.Helper()
		out, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: in})
		if err == nil && !out.IsError {
			t.Fatal("ungranted worker operation", name, out)
		}
	}
	denied("reet_send_worker_message", args)
	access := mcpData(t, client, "reet_list_worker_access", map[string]any{})
	if len(access["scopes"].([]any)) != 0 {
		t.Fatal(access)
	}
	path := fmt.Sprintf("/api/v2/mcp/connections/%d/worker?sessionId=%s", clientID, session.ID)
	code, _ := doClient(t, browser, ts, "PUT", path, `{"revision":0,"enabled":true}`, "Content-Type", "application/json", "Origin", "https://other.example")
	if code != 403 {
		t.Fatal("cross-site grant", code)
	}
	code, _ = do(t, ts, "PUT", path, `{"revision":0,"enabled":true}`, "Content-Type", "application/json", "Authorization", "Bearer "+token, "Origin", ts.URL)
	if code != 403 && code != 401 {
		t.Fatal("credential self-granted", code)
	}
	contextAPIJSON(t, browser, ts, "PUT", path, map[string]any{"revision": 0, "enabled": true}, 200)
	first := mcpData(t, client, "reet_send_worker_message", args)["message"].(map[string]any)
	if first["state"] != "queued" || (first["text"] != nil && first["text"] != "") {
		t.Fatal("receipt projection", first)
	}
	second := mcpData(t, client, "reet_send_worker_message", args)["message"].(map[string]any)
	if first["queueId"] != second["queueId"] || fake.enqueues.Load() != 1 {
		t.Fatal("duplicate dispatch", first, second, fake.enqueues.Load())
	}
	denied("reet_send_worker_message", map[string]any{"session_id": session.ID, "request_id": id, "text": "Changed body"})
	otherToken, _ := mcpManual(t, browser, ts, "write")
	other := mcpSession(t, ts, otherToken)
	out, err := other.CallTool(context.Background(), &mcp.CallToolParams{Name: "reet_get_worker_message", Arguments: map[string]any{"session_id": session.ID, "request_id": id}})
	if err == nil && !out.IsError {
		t.Fatal("client inherited receipt")
	}
	mcpJSON(t, browser, ts, "PATCH", "/api/v2/mcp", map[string]any{"browserEnabled": true, "browserScope": "write"}, 200)
	call := map[string]any{"name": "reet_send_worker_message", "arguments": map[string]any{"session_id": session.ID, "request_id": strings.Repeat("b", 32), "text": "A separate browser request."}}
	contextAPIJSON(t, browser, ts, "POST", "/api/v2/mcp/browser/call", call, 400)
	contextAPIJSON(t, browser, ts, "PUT", "/api/v2/mcp/browser/worker?sessionId="+session.ID, map[string]any{"revision": 0, "enabled": true}, 200)
	contextAPIJSON(t, browser, ts, "POST", "/api/v2/mcp/browser/call", call, 200)
	if fake.enqueues.Load() != 2 {
		t.Fatal("browser grant did not stay separate", fake.enqueues.Load())
	}
	queue, err := st.TerminalQueueFor(user.ID, session.ID)
	if err != nil || len(queue) != 2 {
		t.Fatal(queue, err)
	}
	if claimed, err := st.TransitionTerminalQueueFor(user.ID, session.ID, queue[0], "sending", "wait", "", "previous-controller"); err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	view := contextAPIJSON(t, browser, ts, "GET", path, nil, 200)
	messages := view["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["state"] != "uncertain" || fake.enqueues.Load() != 2 {
		t.Fatal("browser read did not reconcile without dispatch", view)
	}
	contextAPIJSON(t, browser, ts, "PUT", path, map[string]any{"revision": 1, "enabled": false}, 200)
	denied("reet_get_worker_message", map[string]any{"session_id": session.ID, "request_id": id})
	receipt, err := st.WorkerMessageFor(user.ID, "mcp", fmt.Sprint(clientID), session.ID, id)
	if err != nil || receipt.State != "uncertain" {
		t.Fatal("revocation left a queued message", receipt, err)
	}
	if err = st.UpdateUserRole(user.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	contextAPIJSON(t, browser, ts, "GET", "/api/v2/mcp/browser/worker?sessionId="+session.ID, nil, 403)
	// Fresh role checks also apply to a real previously granted browser receipt.
	remaining, err := st.WorkerMessagesFor(user.ID, "browser", "browser", session.ID)
	if err != nil || len(remaining) != 1 {
		t.Fatal(remaining, err)
	}
	if err = st.CheckWorkerDelivery(user.ID, session.ID, remaining[0].QueueID); err == nil {
		t.Fatal("viewer delivery was allowed")
	}
}
