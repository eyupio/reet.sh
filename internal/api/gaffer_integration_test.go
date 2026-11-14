package api

// This fixture uses the production API, scheduler, store, provider transport,
// private WebSocket and broker. Only the external LLM and session worker are fake.
// Its control routes exist solely in this Go test binary, never in reet serve.

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const gm6Password = "synthetic offline flow password"

type gm6Worker struct {
	Session store.TerminalSession `json:"session"`
	Working bool                  `json:"working"`
	Waiting bool                  `json:"waiting"`
	Mode    string                `json:"mode"`
	Prompts []string              `json:"prompts"`
	Calls   int                   `json:"calls"`
	Reviews int                   `json:"reviews"`
}
type gm6Harness struct {
	t                         *testing.T
	dir, publicURL, secret    string
	uid                       int64
	agent                     store.SyncAgent
	current                   atomic.Pointer[Server]
	st                        *store.Store
	public, private, provider *httptest.Server
	serving                   sync.RWMutex
	mu                        sync.Mutex
	workers                   map[string]*gm6Worker
	forbidden                 int
	conn                      *websocket.Conn
	write                     sync.Mutex
	workerDone                chan struct{}
	linkMu                    sync.Mutex // guards conn, generation, workerDone, deliberate, stopping and linkErr
	generation                int64      // the open link's generation, which every worker message must carry
	deliberate                bool       // the fixture closed the link on purpose (an "uncertain" prompt), so it stays closed
	stopping                  bool       // the fixture is closing the link for a restart or the end of the test
	linkErr                   error      // why the worker reader last stopped
	finished                  chan bool
	errors                    chan error
}

func newGM6Harness(t *testing.T) *gm6Harness {
	t.Helper()
	h := &gm6Harness{t: t, dir: t.TempDir(), workers: map[string]*gm6Worker{}, finished: make(chan bool, 1), errors: make(chan error, 16)}
	var err error
	h.st, err = store.Open(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	password, err := auth.HashPassword(gm6Password)
	if err != nil {
		t.Fatal(err)
	}
	u, err := h.st.CreateUser("gm6@example.test", "Offline tester", "owner", password)
	if err != nil {
		t.Fatal(err)
	}
	h.uid = u.ID
	h.agent, h.secret, err = h.st.EnrollSyncAgent(h.uid, "Synthetic worker machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err := h.st.BeginRemoteConnectionFor(h.uid, h.agent.ID, terminalbroker.ID(), gm6Caps())
	if err != nil {
		t.Fatal(err)
	}
	for i, marker := range []string{"ALPHA", "BRAVO", "CHARLIE"} {
		s, _, err := h.st.ReserveTerminalCreateFor(h.uid, h.agent.ID, fmt.Sprintf("%032x", i+1), record.Generation, terminalproto.Create{Title: marker + " release", Tool: "opencode", Model: "working-synthetic", Dir: "/synthetic/work", Structured: true, Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		if err = h.st.ObserveTerminalStateFor(h.uid, h.agent.ID, s.ID, record.Generation, terminalproto.Running, nil); err != nil {
			t.Fatal(err)
		}
		if err = h.st.AppendTerminalChatEventFor(h.uid, s.ID, 1, time.Now().UnixMilli(), []byte(fmt.Sprintf(`{"kind":"agent","messageId":"initial","text":%q}`, marker+" initial retained progress; a simulated PR needs checks."))); err != nil {
			t.Fatal(err)
		}
		h.workers[marker] = &gm6Worker{Session: s, Prompts: []string{}}
	}
	if err = os.WriteFile(filepath.Join(h.dir, "fixture-key"), []byte(strings.Repeat("gm6-synthetic-key-", 4)), 0600); err != nil {
		t.Fatal(err)
	}
	h.provider = httptest.NewServer(http.HandlerFunc(h.model))
	h.public = httptest.NewUnstartedServer(http.HandlerFunc(h.serve))
	if addr := os.Getenv("REET_GAFFER_TEST_ADDR"); addr != "" {
		h.public.Listener.Close()
		h.public.Listener, err = net.Listen("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
	}
	h.publicURL = os.Getenv("REET_GAFFER_TEST_PUBLIC_URL")
	if h.publicURL == "" {
		h.publicURL = "http://" + h.public.Listener.Addr().String()
	}
	h.private = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.current.Load().s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	h.start()
	h.public.Start()
	t.Cleanup(func() {
		h.current.Load().Close()
		h.stopWorker()
		h.public.Close()
		h.private.Close()
		h.provider.Close()
		h.st.Close()
	})
	return h
}
func gm6Caps() terminalproto.Capabilities {
	return terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "synthetic-worker", TerminalEnabled: true, TerminalControl: true, HealthEnabled: true, Readiness: terminalproto.Ready, ACP: true, ACPLiveControls: true}
}
func (h *gm6Harness) fail(err error) {
	select {
	case h.errors <- err:
	default:
	}
}
func (h *gm6Harness) start() {
	key, err := os.ReadFile(filepath.Join(h.dir, "fixture-key"))
	if err != nil {
		h.t.Fatal(err)
	}
	seal, err := sealed.NewSealer(string(key), "GM6 synthetic fixture")
	if err != nil {
		h.t.Fatal(err)
	}
	h.current.Store(New(h.st, meta.Fetcher{}, "", WithAuthMode(AuthAccounts), WithPublicURL(h.publicURL), WithRemoteTerminals(true), WithGafferProviders(seal)))
	h.connect()
}
func (h *gm6Harness) restart() {
	h.serving.Lock()
	defer h.serving.Unlock()
	h.current.Load().Close()
	h.stopWorker()
	if err := h.st.Close(); err != nil {
		h.t.Fatal(err)
	}
	var err error
	h.st, err = store.Open(h.dir)
	if err != nil {
		h.t.Fatal(err)
	}
	h.start()
}
func (h *gm6Harness) stopWorker() {
	h.linkMu.Lock()
	c, done := h.conn, h.workerDone
	h.stopping = true
	h.linkMu.Unlock()
	if c != nil {
		c.CloseNow()
		<-done
		h.linkMu.Lock()
		h.conn = nil
		h.linkMu.Unlock()
	}
}
func (h *gm6Harness) connect() {
	h.linkMu.Lock()
	defer h.linkMu.Unlock()
	if err := h.dialLocked(); err != nil {
		h.t.Fatal(err)
	}
}

// dialLocked opens the worker's control connection, as the connector does after every restart or lost link.
// The caller holds linkMu.
func (h *gm6Harness) dialLocked() error {
	headers := http.Header{"X-Reet-Agent": {h.agent.ID}, "X-Reet-Agent-Key": {h.secret}, "X-Reet-Version": {"test"}}
	ctx := h.t.Context()
	c, _, err := websocket.Dial(ctx, h.private.URL+"/private/v1/control", &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		return err
	}
	raw, err := terminalproto.EncodeHandshake(terminalproto.Handshake{ProtocolVersion: 1, RequestID: terminalbroker.ID(), AgentInstanceID: terminalbroker.ID(), Capabilities: gm6Caps()})
	if err != nil {
		return err
	}
	if err = c.Write(ctx, websocket.MessageText, raw); err != nil {
		return err
	}
	_, raw, err = c.Read(ctx)
	if err != nil {
		return err
	}
	ack, err := terminalproto.DecodeHandshakeReply(raw)
	if err != nil {
		return err
	}
	done := make(chan struct{})
	h.conn, h.generation, h.workerDone, h.deliberate, h.stopping, h.linkErr = c, ack.Generation, done, false, false, nil
	go func() {
		defer close(done)
		for {
			_, raw, err := c.Read(ctx)
			if err != nil {
				h.linkMu.Lock()
				h.linkErr = err
				stopping := h.stopping
				h.linkMu.Unlock()
				// Said now, not when the worker next sends: a goal check that finds the machine gone
				// waits a minute, and no send may follow to report it.
				if !stopping && ctx.Err() == nil {
					h.t.Logf("worker link ended without the fixture closing it, at %s: %v", time.Now().Format("15:04:05.000"), err)
				}
				return
			}
			m, err := terminalproto.Decode(raw)
			if err != nil {
				h.fail(err)
				return
			}
			body, drop := h.workerReply(m)
			if drop {
				h.linkMu.Lock()
				h.deliberate = true
				h.linkMu.Unlock()
				c.CloseNow()
				return
			}
			m.Body = body
			if err := h.send(c, m); err != nil {
				return
			}
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !h.current.Load().s.terminals.Ready(h.uid, h.agent.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !h.current.Load().s.terminals.Ready(h.uid, h.agent.ID) {
		return fmt.Errorf("synthetic private worker did not become ready (generation %d)", ack.Generation)
	}
	return nil
}

// toController sends one worker message, carrying the generation of the link it goes on. If the controller ended
// the link (a superseded or timed-out peer closes it without telling the connector), the worker dials again and
// resends, as a real connector does, and the test log says why the link ended. A link the fixture dropped on purpose
// stays closed and the send fails.
func (h *gm6Harness) toController(m terminalproto.Message) error {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		h.linkMu.Lock()
		c, done := h.conn, h.workerDone
		m.Generation = h.generation
		h.linkMu.Unlock()
		if err = h.send(c, m); err == nil {
			return nil
		}
		// The write can fail before the reader has noticed the close; give it a moment.
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			return err
		}
		h.linkMu.Lock()
		if h.deliberate || h.conn != c {
			h.linkMu.Unlock()
			return err
		}
		h.t.Logf("worker link ended without the fixture closing it (%v; write error: %v); dialling again", h.linkErr, err)
		c.CloseNow()
		derr := h.dialLocked()
		h.linkMu.Unlock()
		if derr != nil {
			return derr
		}
		// The resend takes the new link's generation at the top of the loop.
	}
	return err
}
func (h *gm6Harness) send(c *websocket.Conn, m terminalproto.Message) error {
	raw, err := terminalproto.Encode(m)
	if err != nil {
		return err
	}
	h.write.Lock()
	defer h.write.Unlock()
	return c.Write(h.t.Context(), websocket.MessageText, raw)
}
func (h *gm6Harness) workerReply(m terminalproto.Message) (terminalproto.Body, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var worker *gm6Worker
	for _, w := range h.workers {
		if w.Session.ID == m.SessionID {
			worker = w
			break
		}
	}
	switch op := m.Body.(type) {
	case *terminalproto.Reconcile:
		sessions := []terminalproto.SessionSnapshot{}
		for _, w := range h.workers {
			sessions = append(sessions, terminalproto.SessionSnapshot{ID: w.Session.ID, Title: w.Session.Title, State: terminalproto.Running, ACPSessionID: "ses_gm6synthetic0000000000000001"})
		}
		return terminalproto.Reconcile{Complete: true, Sessions: sessions}, false
	case *terminalproto.Probe:
		return terminalproto.ProbeResult(*op), false
	case *terminalproto.ACPControl:
		if worker != nil && op.Action == "get" {
			return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Working: worker.Working, Waiting: worker.Waiting, Model: "working-synthetic", Options: []terminalproto.ACPSetting{}}}, false
		}
	case *terminalproto.ACPPrompt:
		if worker != nil {
			worker.Prompts = append(worker.Prompts, op.Text)
			worker.Working = true
			return terminalproto.Result{Code: terminalproto.OK}, worker.Mode == "uncertain"
		}
	}
	h.forbidden++
	h.fail(fmt.Errorf("unexpected worker operation %T for %s", m.Body, m.SessionID))
	return terminalproto.Result{Code: terminalproto.Failed}, false
}

// progress has the worker report new output and then its status, and returns once the controller has recorded
// both. Sent is not recorded: the controller stores an event a moment after reading it, and a step that follows
// (a resume, which treats everything recorded as already seen) must not race it.
func (h *gm6Harness) progress(marker string, waiting bool) error {
	h.mu.Lock()
	w := h.workers[marker]
	w.Working = waiting
	w.Waiting = waiting
	sid := w.Session.ID
	h.mu.Unlock()
	before, err := h.st.GafferWorkerProgress(h.uid, sid)
	if err != nil {
		return err
	}
	events := []terminalproto.ACPEvent{{Kind: "agent", MessageID: terminalbroker.ID(), Text: marker + " new worker progress: simulated PR checks/conflicts need attention."}, {Kind: "status", State: map[bool]string{true: "working", false: "idle"}[waiting]}}
	for _, event := range events {
		if err := h.toController(terminalproto.Message{Version: 1, RequestID: terminalbroker.ID(), SessionID: sid, Body: event}); err != nil {
			return err
		}
	}
	// Each event takes the session's next sequence number, so both are recorded once progress has risen by two.
	deadline := time.Now().Add(10 * time.Second)
	for {
		seq, err := h.st.GafferWorkerProgress(h.uid, sid)
		if err == nil && seq >= before+int64(len(events)) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the controller did not record %s's progress: it stands at %d, was %d before %d events (%v)", marker, seq, before, len(events), err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func (h *gm6Harness) model(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/models") {
		writeJSON(w, 200, map[string]any{"data": []map[string]string{{"id": "gaffer-synthetic"}}})
		return
	}
	var request struct {
		Model    string            `json:"model"`
		Messages json.RawMessage   `json:"messages"`
		Tools    []json.RawMessage `json:"tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.fail(err)
		http.Error(w, "bad fixture request", 400)
		return
	}
	if request.Model != "gaffer-synthetic" || r.Header.Get("Authorization") != "Bearer synthetic-gm6-key" {
		h.fail(fmt.Errorf("model or sealed fixture key was not preserved"))
		http.Error(w, "invalid fixture model", 400)
		return
	}
	h.mu.Lock()
	marker := ""
	for name := range h.workers {
		if strings.Contains(string(request.Messages), name) {
			if marker != "" {
				h.fail(fmt.Errorf("cross-session model context: %s and %s", marker, name))
			}
			marker = name
		}
	}
	if marker == "" {
		h.mu.Unlock()
		h.fail(fmt.Errorf("model request lacked its saved session goal"))
		http.Error(w, "missing scope", 400)
		return
	}
	worker := h.workers[marker]
	worker.Calls++
	if len(request.Tools) > 0 {
		worker.Reviews++
		if worker.Mode == "queue" {
			worker.Working = true
			worker.Mode = ""
		}
	}
	review := worker.Reviews
	h.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	if len(request.Tools) > 0 {
		args, _ := json.Marshal(map[string]string{"text": fmt.Sprintf("%s: address the simulated PR checks and conflicts, then report progress (review %d). Do not merge.", marker, review)})
		packet := map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "gm6_instruction", "type": "function", "function": map[string]any{"name": "message_worker", "arguments": string(args)}}}}, "finish_reason": "tool_calls"}}}
		b, _ := json.Marshal(packet)
		fmt.Fprintf(w, "data: %s\n\n", b)
	} else {
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": marker + ": the instruction is recorded. The worker still needs to address checks and conflicts; completion needs your review."}, "finish_reason": "stop"}}})
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":8}}\n\ndata: [DONE]\n\n")
}
func (h *gm6Harness) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/_gaffer_test/ready" {
		writeJSON(w, 200, map[string]bool{"ready": true})
		return
	}
	if strings.HasPrefix(r.URL.Path, "/_gaffer_test/") {
		if u, ok := h.current.Load().s.sessionUser(r); !ok || u.ID != h.uid {
			http.Error(w, "forbidden", 403)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/_gaffer_test/state" {
			h.mu.Lock()
			defer h.mu.Unlock()
			writeJSON(w, 200, map[string]any{"workers": h.workers, "forbidden": h.forbidden, "providerURL": h.provider.URL})
			return
		}
		if r.Method != "POST" || r.Header.Get("Origin") != h.publicURL {
			http.Error(w, "forbidden", 403)
			return
		}
		var in struct {
			Action, Marker, Mode string
			Waiting              bool
			OK                   bool
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&in) != nil {
			http.Error(w, "bad command", 400)
			return
		}
		switch in.Action {
		case "restart":
			h.restart()
		case "progress":
			if err := h.progress(in.Marker, in.Waiting); err != nil {
				h.fail(err)
				http.Error(w, err.Error(), 500)
				return
			}
		case "mode":
			h.mu.Lock()
			if worker := h.workers[in.Marker]; worker != nil {
				worker.Mode = in.Mode
				worker.Waiting = in.Waiting
				worker.Working = in.Waiting
			}
			h.mu.Unlock()
		case "finish":
			h.finished <- in.OK
		default:
			http.Error(w, "unknown test action", 400)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	h.serving.RLock()
	defer h.serving.RUnlock()
	if strings.HasPrefix(r.URL.Path, "/api/") {
		h.current.Load().ServeHTTP(w, r)
		return
	}
	assets := os.Getenv("REET_GAFFER_TEST_WEB")
	if assets == "" {
		assets = filepath.Join("..", "..", "web", "dist")
	}
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		http.FileServer(http.Dir(assets)).ServeHTTP(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(assets, "index.html"))
}

func TestGafferOfflinePrivateIntegration(t *testing.T) {
	t.Parallel()
	h := newGM6Harness(t)
	client := &http.Client{Jar: mustJar(t)}
	contextAPIJSON(t, client, h.public, "POST", "/api/v2/auth/sign-in", map[string]string{"email": "gm6@example.test", "password": gm6Password}, 200)
	key := "synthetic-gm6-key"
	providers := contextAPIJSON(t, client, h.public, "POST", "/api/v2/gaffer/providers", gaffer.Input{Name: "Synthetic Gaffer", Kind: "openai_compatible", BaseURL: h.provider.URL, Model: "gaffer-synthetic", Enabled: true, MakeDefault: true, LocalOnly: true, AllowSessions: true, AllowCode: true, APIKey: &key}, 200)
	pid := providers["providers"].([]any)[0].(map[string]any)["id"].(string)
	sid := h.workers["ALPHA"].Session.ID
	conversation := contextAPIJSON(t, client, h.public, "POST", "/api/v2/gaffer/conversations", map[string]string{"sessionId": sid, "providerId": pid}, 200)["conversation"].(map[string]any)
	base := "/api/v2/gaffer/conversations/" + conversation["id"].(string)
	worker := contextAPIJSON(t, client, h.public, "GET", base+"/worker", nil, 200)["worker"].(map[string]any)
	contextAPIJSON(t, client, h.public, "PUT", base+"/worker", map[string]any{"enabled": true, "revision": 0, "providerId": pid, "providerRevision": worker["providerRevision"], "model": "gaffer-synthetic"}, 200)
	sharing := contextAPIJSON(t, client, h.public, "GET", base+"/context", nil, 200)["context"].(map[string]any)
	contextAPIJSON(t, client, h.public, "PUT", base+"/context", map[string]any{"revision": 0, "providerId": pid, "providerRevision": sharing["providerRevision"], "model": "gaffer-synthetic", "history": true, "text": true}, 200)
	goal := contextAPIJSON(t, client, h.public, "GET", base+"/goal", nil, 200)
	payload := goal["pins"].(map[string]any)
	payload["revision"] = 0
	payload["text"] = "ALPHA ensure the PR checks and conflicts are addressed. Do not merge."
	payload["maxReviews"] = 1
	payload["hours"] = 24
	h.mu.Lock()
	h.workers["ALPHA"].Mode = "queue"
	h.mu.Unlock()
	contextAPIJSON(t, client, h.public, "PUT", base+"/goal", payload, 200)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		g, err := h.st.GafferGoalFor(h.uid, conversation["id"].(string))
		if err == nil && g.PendingID != "" && g.ActiveTurnID == "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	g, err := h.st.GafferGoalFor(h.uid, conversation["id"].(string))
	if err != nil || g.PendingID == "" {
		t.Fatal("no queued goal instruction", g, err)
	}
	h.restart() // No browser or queue-read endpoint reopens the queue.
	if err := h.progress("ALPHA", false); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		count := len(h.workers["ALPHA"].Prompts)
		h.mu.Unlock()
		if count == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.mu.Lock()
	prompts, calls, forbidden := len(h.workers["ALPHA"].Prompts), h.workers["ALPHA"].Calls, h.forbidden
	h.mu.Unlock()
	if prompts != 1 || calls != 2 || forbidden != 0 {
		t.Fatal("offline restart lost or replayed a goal instruction", prompts, calls, forbidden)
	}
	select {
	case err := <-h.errors:
		t.Fatal(err)
	default:
	}
}

func TestGafferOfflineBrowserHarness(t *testing.T) {
	if os.Getenv("REET_GAFFER_BROWSER") != "1" {
		t.Skip("run scripts/test-gaffer-offline.sh for the real-browser native/container flow")
	}
	h := newGM6Harness(t)
	t.Log("GM6 browser harness ready at", h.publicURL)
	select {
	case ok := <-h.finished:
		if !ok {
			t.Fatal("browser flow reported failure")
		}
	case err := <-h.errors:
		t.Fatal(err)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
}

// The controller can end the worker's control link without telling the connector (a superseded or timed-out peer,
// a closed socket under load). A connector dials again, so the fixture must too: the next worker message arrives
// and the test log says why the link ended.
func TestGafferWorkerLinkRedialsAfterItIsLost(t *testing.T) {
	t.Parallel()
	h := newGM6Harness(t)
	h.linkMu.Lock()
	lost, done := h.conn, h.workerDone
	h.linkMu.Unlock()
	lost.CloseNow()
	<-done
	if err := h.progress("ALPHA", false); err != nil {
		t.Fatalf("progress after the link was lost: %v", err)
	}
	h.linkMu.Lock()
	redialled := h.conn != lost
	h.linkMu.Unlock()
	if !redialled || !h.current.Load().s.terminals.Ready(h.uid, h.agent.ID) {
		t.Fatal("the worker did not dial again", redialled)
	}
}
