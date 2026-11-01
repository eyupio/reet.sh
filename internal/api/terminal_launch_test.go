package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const launchTestPrompt = "fix the flaky login test and explain why"

type launchAPI struct {
	srv     *Server
	st      *store.Store
	uid     int64
	agent   store.SyncAgent
	cookie  string
	mu      sync.Mutex
	creates []terminalproto.Create
	reply   func(terminalproto.Create) terminalproto.Body
	// worktree answers worktree_status and worktree_remove; worktrees records them.
	worktree  func(terminalproto.Body) terminalproto.Body
	worktrees []terminalproto.Body
	// late, when set, receives a worktree request that got no answer.
	late func(terminalproto.Message)
	// prompts records every ACP prompt, including its image paths.
	prompts []terminalproto.ACPPrompt
	// modes records every session_mode request; mode answers it (default: the
	// session is running).
	modes []terminalproto.SessionMode
	mode  func(terminalproto.SessionMode) terminalproto.Body
	peer  *terminalbroker.Peer
}

func (f *launchAPI) sent() []terminalproto.Create {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Create(nil), f.creates...)
}

// launchAPIFixture connects an in-memory peer with the given capabilities to
// the server's broker; it records every create it receives.
func launchAPIFixture(t *testing.T, caps terminalproto.Capabilities) *launchAPI {
	t.Helper()
	srv, st, agent, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	f := &launchAPI{srv: srv, st: st, uid: uid, agent: agent, cookie: terminalLogin(t, st, uid)}
	record, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	var p *terminalbroker.Peer
	p, err = srv.s.terminals.Connect(uid, agent.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Create:
			f.mu.Lock()
			f.creates = append(f.creates, op)
			respond := f.reply
			f.mu.Unlock()
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
			if respond != nil {
				reply.Body = respond(op)
			}
		case terminalproto.End:
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Ended}
		case *terminalproto.ACPPrompt:
			f.mu.Lock()
			f.prompts = append(f.prompts, *op)
			f.mu.Unlock()
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		case terminalproto.ACPPrompt:
			f.mu.Lock()
			f.prompts = append(f.prompts, op)
			f.mu.Unlock()
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		case *terminalproto.SessionMode:
			f.mu.Lock()
			f.modes = append(f.modes, *op)
			respond := f.mode
			f.mu.Unlock()
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
			if respond != nil {
				reply.Body = respond(*op)
			}
		case terminalproto.WorktreeStatus, terminalproto.WorktreeRemove:
			f.mu.Lock()
			f.worktrees = append(f.worktrees, op)
			respond := f.worktree
			f.mu.Unlock()
			reply.Body = terminalproto.WorktreeResult{Code: terminalproto.Unavailable}
			if respond != nil {
				reply.Body = respond(op)
			}
			if reply.Body == nil {
				f.mu.Lock()
				late := f.late
				f.mu.Unlock()
				if late != nil {
					late(reply)
				}
				return nil // no answer now: the reply is lost or comes late
			}
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
	f.peer = p
	return f
}

func (f *launchAPI) do(t *testing.T, cookie, method, path string, body any) (int, map[string]any, string) {
	t.Helper()
	return f.doCtx(t, context.Background(), cookie, method, path, body)
}

func (f *launchAPI) doCtx(t *testing.T, ctx context.Context, cookie, method, path string, body any) (int, map[string]any, string) {
	t.Helper()
	var reader *strings.Reader
	switch b := body.(type) {
	case string:
		reader = strings.NewReader(b)
	default:
		data, _ := json.Marshal(b)
		reader = strings.NewReader(string(data))
	}
	r := httptest.NewRequestWithContext(ctx, method, path, reader)
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

func (f *launchAPI) create(t *testing.T, body map[string]any) (int, map[string]any, string) {
	t.Helper()
	return f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions", body)
}

func launchBody(agent, request string, extra map[string]any) map[string]any {
	body := map[string]any{"agentId": agent, "requestId": request, "title": "Agent", "cols": 80, "rows": 24}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func launchCaps(tools ...string) terminalproto.Capabilities {
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalControl: true, TerminalEnabled: true, HealthEnabled: true, Readiness: terminalproto.Ready}
	if tools != nil {
		caps.Launch = true
		caps.Tools = tools
	}
	return caps
}

func launchEvents(t *testing.T, st *store.Store, uid int64) []store.Event {
	t.Helper()
	events, err := st.ListEventsFor(uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Event
	for _, e := range events {
		if e.Action == "terminal-launch" {
			out = append(out, e)
		}
	}
	return out
}

func TestTerminalLaunchCreateIsStrictAndValidated(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	for name, body := range map[string]string{
		"unknown field":       `{"agentId":"` + f.agent.ID + `","requestId":"` + terminalbroker.ID() + `","title":"A","cols":80,"rows":24,"tool":"claude","command":"rm"}`,
		"model shell string":  `{"agentId":"` + f.agent.ID + `","requestId":"` + terminalbroker.ID() + `","title":"A","cols":80,"rows":24,"tool":"claude","model":"x; rm -rf ~"}`,
		"relative dir":        `{"agentId":"` + f.agent.ID + `","requestId":"` + terminalbroker.ID() + `","title":"A","cols":80,"rows":24,"tool":"claude","dir":"work"}`,
		"flag prompt":         `{"agentId":"` + f.agent.ID + `","requestId":"` + terminalbroker.ID() + `","title":"A","cols":80,"rows":24,"tool":"claude","prompt":"--dangerously-skip-permissions now"}`,
		"prompt without tool": `{"agentId":"` + f.agent.ID + `","requestId":"` + terminalbroker.ID() + `","title":"A","cols":80,"rows":24,"prompt":"hello there"}`,
	} {
		if code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions", body); code != 400 {
			t.Fatal(name, code, raw)
		}
	}
	// A tool outside the catalogue is refused as one reet does not start.
	binary := `{"agentId":"` + f.agent.ID + `","requestId":"` + terminalbroker.ID() + `","title":"A","cols":80,"rows":24,"tool":"/bin/sh"}`
	if code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions", binary); code != 422 || out["code"] != "tool_unavailable" {
		t.Fatal("binary named", code, raw)
	}
	if len(f.sent()) != 0 {
		t.Fatal("invalid launch reached the connector", f.sent())
	}
}

func TestTerminalLaunchIsBlockedForViewersAndLegacyAuth(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	viewer, err := f.st.CreateUser("launch-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	body := launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "prompt": launchTestPrompt})
	if code, _, raw := f.do(t, terminalLogin(t, f.st, viewer.ID), "POST", "/api/v2/terminal/sessions", body); code != 403 {
		t.Fatal("viewer started an agent", code, raw)
	}
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		f.srv.s.authMode = mode
		if code, _, raw := f.create(t, body); code != 403 {
			t.Fatal("legacy auth started an agent", mode, code, raw)
		}
	}
	if len(f.sent()) != 0 || len(launchEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("blocked launch was dispatched or audited")
	}
}

func TestTerminalLaunchOnAPeerWithoutLaunchAsksForAnUpdate(t *testing.T) {
	f := launchAPIFixture(t, launchCaps())
	code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude"}))
	if code != 409 || out["error"] != "Update reet on that machine to start agents." || out["code"] != "launch_unsupported" {
		t.Fatal(code, raw)
	}
	if len(f.sent()) != 0 {
		t.Fatal("launch fields were sent to an old peer", f.sent())
	}
	code, out, raw = f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), nil))
	if code != 200 || out["state"] != "running" {
		t.Fatal("plain shell on an old peer", code, raw)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0] != (terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24}) {
		t.Fatal("plain create changed", sent)
	}
	if len(launchEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("plain shell audited as a launch")
	}
}

func TestTerminalResumeForwardsTheFieldAndNeedsTheMachineCapability(t *testing.T) {
	caps := launchCaps("opencode")
	caps.Resume = true
	f := launchAPIFixture(t, caps)
	body := launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "dir": "/home/tester/site", "resume": true})
	if code, out, raw := f.create(t, body); code != 200 || out["state"] != "running" {
		t.Fatal("resume create", code, raw)
	}
	want := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "opencode", Dir: "/home/tester/site", Resume: true}
	if sent := f.sent(); len(sent) != 1 || sent[0] != want {
		t.Fatal("resume was not forwarded exactly", sent)
	}

	old := launchAPIFixture(t, launchCaps("opencode"))
	oldBody := launchBody(old.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "dir": "/home/tester/site", "resume": true})
	code, out, raw := old.create(t, oldBody)
	if code != 409 || out["code"] != "resume_unsupported" || out["error"] != "Update reet on that machine to continue sessions." {
		t.Fatal("resume on a machine without resume-v1", code, raw)
	}
	if len(old.sent()) != 0 {
		t.Fatal("resume reached a machine without resume-v1", old.sent())
	}
}

func TestTerminalLaunchRefusesSingleWordPromptBeforeSending(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude", "codex"))
	for _, tool := range []string{"claude", "codex"} {
		code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": tool, "prompt": "purge"}))
		if code != 422 || out["code"] != "prompt_unsupported" || out["error"] != "Write the opening prompt as at least two words." {
			t.Fatal(tool, code, raw)
		}
	}
	if len(f.sent()) != 0 {
		t.Fatal("single-word prompt reached the connector")
	}
}

func TestTerminalLaunchFailedMapsToToolOrFolder(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.LaunchFailed}
	}
	for _, tc := range []struct {
		tool, code, sentence string
	}{
		{"claude", "folder_unavailable", "That folder isn't available on that machine."},
		{"gemini", "tool_not_installed", "That agent isn't installed on that machine."},
	} {
		request := terminalbroker.ID()
		body := launchBody(f.agent.ID, request, map[string]any{"tool": tc.tool, "dir": "/somewhere/else", "prompt": launchTestPrompt})
		for attempt := 0; attempt < 2; attempt++ {
			code, out, raw := f.create(t, body)
			if code != 422 || out["code"] != tc.code || out["error"] != tc.sentence {
				t.Fatal(tc.tool, attempt, code, raw)
			}
		}
	}
	// Only the folder refusal reaches the machine: gemini is not advertised, so
	// it is refused before any session is reserved.
	if len(f.sent()) != 1 {
		t.Fatal("a refused launch was dispatched again on retry", len(f.sent()))
	}
	if len(launchEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("refused launch was audited")
	}
}

func TestTerminalLaunchAuditsOnceWithoutThePrompt(t *testing.T) {
	var logs bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(original) })
	f := launchAPIFixture(t, launchCaps("claude", "codex"))
	f.srv.s.debug = true
	request := terminalbroker.ID()
	body := launchBody(f.agent.ID, request, map[string]any{"tool": "claude", "model": "sonnet", "dir": "/home/tester/site", "prompt": launchTestPrompt})
	code, first, raw := f.create(t, body)
	if code != 200 || first["state"] != "running" {
		t.Fatal(code, raw)
	}
	want := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Model: "sonnet", Dir: "/home/tester/site", Prompt: launchTestPrompt}
	if sent := f.sent(); len(sent) != 1 || sent[0] != want {
		t.Fatal("launch fields were not forwarded exactly", sent)
	}
	code, again, raw := f.create(t, body)
	if code != 200 || again["id"] != first["id"] || len(f.sent()) != 1 {
		t.Fatal("idempotent retry", code, raw, len(f.sent()))
	}
	changed := launchBody(f.agent.ID, request, map[string]any{"tool": "claude", "model": "opus", "dir": "/home/tester/site", "prompt": launchTestPrompt})
	if code, _, raw := f.create(t, changed); code != 409 {
		t.Fatal("changed launch field", code, raw)
	}
	events := launchEvents(t, f.st, f.uid)
	if len(events) != 1 {
		t.Fatal("launch audited", len(events), "times")
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["machine"] != f.agent.ID || detail["host"] != f.agent.Host || detail["tool"] != "claude" || detail["model"] != "sonnet" || detail["dir"] != "/home/tester/site" || events[0].Source != "ui" {
		t.Fatal("audit detail", events[0].Detail, events[0].Source)
	}
	all, err := f.st.ListEventsFor(f.uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	everything, _ := json.Marshal(all)
	for _, fragment := range []string{launchTestPrompt, "flaky login"} {
		if strings.Contains(string(everything), fragment) || strings.Contains(logs.String(), fragment) {
			t.Fatal("the opening prompt was written to audit or logs")
		}
	}
	// Activity reads the same rows through the events API.
	code, _, raw = f.do(t, f.cookie, "GET", "/api/events?limit=100", nil)
	if code != 200 || !strings.Contains(raw, "terminal-launch") || strings.Contains(raw, "flaky login") {
		t.Fatal("activity feed", code, raw)
	}
}

func TestTerminalLaunchCapabilitiesReachTheWeb(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude", "gemini"))
	code, _, raw := f.do(t, f.cookie, "GET", "/api/v2/terminal/machines", nil)
	if code != 200 || !strings.Contains(raw, `"launch":true`) || !strings.Contains(raw, `"tools":["claude","gemini"]`) {
		t.Fatal("terminal machines", code, raw)
	}
	r := httptest.NewRequest("GET", "/api/v2/machines/health", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"launch":true`) || !strings.Contains(w.Body.String(), `"tools":["claude","gemini"]`) {
		t.Fatal("machine health", w.Code, w.Body.String())
	}
	legacy := launchAPIFixture(t, launchCaps())
	_, _, raw = legacy.do(t, legacy.cookie, "GET", "/api/v2/terminal/machines", nil)
	if strings.Contains(raw, `"launch"`) || strings.Contains(raw, `"tools"`) {
		t.Fatal("old machine reported launch", raw)
	}
}

func TestPrivateControlAdvertisesLaunchBesideCommandHistory(t *testing.T) {
	srv, _, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	defer private.Close()
	c, response, err := websocket.Dial(t.Context(), private.URL+"/private/v1/control", &websocket.DialOptions{HTTPHeader: http.Header{"X-Reet-Agent": {agent.ID}, "X-Reet-Agent-Key": {secret}, "X-Reet-Version": {"test"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	features := response.Header.Get(terminalproto.FeatureHeader)
	if !terminalproto.SupportsCommandHistory(features) || !terminalproto.SupportsLaunch(features) {
		t.Fatal("features", features)
	}
}
