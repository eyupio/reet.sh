//go:build linux || darwin

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalagent"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type terminalTestTransport struct {
	base      *url.URL
	transport http.RoundTripper
}

func (t terminalTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = t.base.Scheme
	u.Host = t.base.Host
	copy.URL = &u
	return t.transport.RoundTrip(copy)
}

func TestTerminalPrivateLifecyclePreservesActualShell(t *testing.T) {
	tmux, err := terminalhost.FindTmux(t.Context())
	if err != nil {
		if os.Getenv("REET_BROWSER_TESTS") == "1" {
			t.Fatal("browser terminal gate requires installed tmux", err)
		}
		t.Skip(err)
	}
	dir, err := os.MkdirTemp("/tmp", "reet-broker-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	srv, st, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	var current atomic.Pointer[Server]
	current.Store(srv)
	t.Cleanup(func() { current.Load().Close() })
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	layout, err := terminalhost.NewEnrollmentLayout(filepath.Join(dir, "agent.json"), agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	startHost := func(layout terminalhost.Layout, home string) func(...string) string {
		if err := os.MkdirAll(home, 0700); err != nil {
			t.Fatal(err)
		}
		history := "df -h\nprintf '__recall_%s__\\n' 'from-history'\nprintf '__recall_stale_%s__\\n' 'must-not-run'\n"
		if err := os.WriteFile(filepath.Join(home, ".bash_history"), []byte(history), 0600); err != nil {
			t.Fatal(err)
		}
		if err := terminalhost.Ensure(layout); err != nil {
			t.Fatal(err)
		}
		tmuxProcess := exec.Command(tmux.Path, "-D", "-S", layout.Socket, "-f", layout.Config)
		tmuxProcess.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "SHELL=/bin/sh", "TERM=xterm-256color"}
		if err := tmuxProcess.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
			tmuxProcess.Process.Kill()
			tmuxProcess.Wait()
		})
		command := func(args ...string) string {
			t.Helper()
			out, err := exec.Command(tmux.Path, append([]string{"-S", layout.Socket}, args...)...).Output()
			if err != nil {
				t.Fatal("tmux", args, err)
			}
			return strings.TrimSpace(string(out))
		}
		deadline := time.Now().Add(3 * time.Second)
		for !terminalhost.SocketActive(layout) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		command("set-option", "-g", "default-command", "PS1='stage6-ready> ' exec /bin/sh -i")
		return command
	}
	command := startHost(layout, dir)
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current.Load().s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	defer private.Close()
	browserMode := os.Getenv("REET_BROWSER_TESTS") == "1"
	browserCommands := make(chan terminalBrowserCommand)
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if browserMode && strings.HasPrefix(r.URL.Path, "/_test/") {
			if user, ok := current.Load().s.sessionUser(r); !ok || user.ID != uid {
				http.Error(w, "forbidden", 403)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			action := strings.TrimPrefix(r.URL.Path, "/_test/")
			if action != "size" && action != "state" && action != "restart-agent" && action != "restart-controller" && action != "disable-terminals" && action != "enable-terminals" && action != "state-builder" && action != "offline-builder" && action != "online-builder" {
				http.NotFound(w, r)
				return
			}
			reply := make(chan map[string]any, 1)
			select {
			case browserCommands <- terminalBrowserCommand{action, reply}:
			case <-r.Context().Done():
				return
			}
			select {
			case result := <-reply:
				writeJSON(w, 200, result)
			case <-r.Context().Done():
			}
			return
		}
		if !browserMode || strings.HasPrefix(r.URL.Path, "/api/") {
			current.Load().ServeHTTP(w, r)
			return
		}
		assets := filepath.Join("..", "..", "web", "dist")
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			http.FileServer(http.Dir(assets)).ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(assets, "index.html"))
	}))
	defer public.Close()
	srv.s.publicURL = public.URL
	base, _ := url.Parse(private.URL)
	privateClient := &http.Client{Transport: terminalTestTransport{base, http.DefaultTransport}}
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalControl: true, TerminalEnabled: true, HealthEnabled: true, Readiness: terminalproto.Ready}
	connectPeer := func(agent store.SyncAgent, secret string, layout terminalhost.Layout, home string) func() {
		factory := terminalagent.Factory(terminalhost.ManagerOptions{EnrollmentID: agent.ID, Layout: layout, Home: home, Consent: func() error { return nil }})
		headers := http.Header{"X-Reet-Agent": {agent.ID}, "X-Reet-Agent-Key": {secret}, "X-Reet-Version": {"test"}}
		t.Helper()
		ctx, cancel := context.WithCancel(t.Context())
		c, response, err := websocket.Dial(ctx, private.URL+"/private/v1/control", &websocket.DialOptions{HTTPHeader: headers})
		if err != nil {
			t.Fatal(err)
		}
		advertised := caps
		advertised.CommandHistory = response != nil && terminalproto.SupportsCommandHistory(response.Header.Get(terminalproto.FeatureHeader))
		if advertised.CommandHistory {
			headers.Set(terminalproto.FeatureHeader, terminalproto.CommandHistoryFeature)
		}
		instance := terminalbroker.ID()
		handshake, _ := terminalproto.EncodeHandshake(terminalproto.Handshake{ProtocolVersion: 1, RequestID: terminalbroker.ID(), AgentInstanceID: instance, Capabilities: advertised})
		if err = c.Write(ctx, websocket.MessageText, handshake); err != nil {
			t.Fatal(err)
		}
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := terminalproto.DecodeHandshakeReply(data)
		if err != nil {
			t.Fatal(err)
		}
		var mu sync.Mutex
		send := func(ctx context.Context, m terminalproto.Message) error {
			data, err := terminalproto.Encode(m)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			return c.Write(ctx, websocket.MessageText, data)
		}
		handle, cleanup := factory(ctx, privateClient, headers, reply.Generation, agenthealth.Sender(send))
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer cleanup()
			for {
				_, data, err := c.Read(ctx)
				if err != nil {
					return
				}
				m, err := terminalproto.Decode(data)
				if err != nil {
					return
				}
				if probe, ok := m.Body.(*terminalproto.Probe); ok {
					m.Body = terminalproto.ProbeResult(*probe)
					if send(ctx, m) != nil {
						return
					}
				} else if handle(ctx, m) != nil {
					return
				}
			}
		}()
		cpu, total, available, disk, diskFree, uptime := 42.0, float64(4<<30), float64(2<<30), float64(100<<30), float64(75<<30), 3600.0
		_ = send(ctx, terminalproto.Message{Version: 1, RequestID: terminalbroker.ID(), Generation: reply.Generation, Body: terminalproto.HealthReport{SchemaVersion: 1, Sequence: 1, AgentInstanceID: instance, SampledAt: time.Now().UnixMilli(), Scope: "host", CPUPercent: terminalproto.Reading{Value: &cpu}, Memory: terminalproto.CapacityReading{Total: terminalproto.Reading{Value: &total}, Available: terminalproto.Reading{Value: &available}}, Disk: terminalproto.CapacityReading{Total: terminalproto.Reading{Value: &disk}, Available: terminalproto.Reading{Value: &diskFree}}, HostUptimeSeconds: terminalproto.Reading{Value: &uptime}}})
		stop := func() {
			cancel()
			c.CloseNow()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("private agent workers did not stop")
			}
		}
		deadline := time.Now().Add(3 * time.Second)
		for current.Load().s.terminalEnabled() && !current.Load().s.terminals.Ready(uid, agent.ID) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if current.Load().s.terminalEnabled() && !current.Load().s.terminals.Ready(uid, agent.ID) {
			stop()
			t.Fatal("private connection never became terminal-ready")
		}
		return stop
	}
	connect := func() func() { return connectPeer(agent, secret, layout, dir) }
	stop := connect()
	defer func() { stop() }()
	if browserMode {
		builder, builderSecret, err := st.EnrollSyncAgent(uid, "builder", "test")
		if err != nil {
			t.Fatal(err)
		}
		builderHome := filepath.Join(dir, "builder")
		builderLayout, err := terminalhost.NewEnrollmentLayout(filepath.Join(builderHome, "agent.json"), builder.ID)
		if err != nil {
			t.Fatal(err)
		}
		builderCommand := startHost(builderLayout, builderHome)
		connectBuilder := func() func() { return connectPeer(builder, builderSecret, builderLayout, builderHome) }
		stopBuilder := connectBuilder()
		defer func() { stopBuilder() }()
		builderOnline := true
		controlDir := filepath.Join(dir, "controller")
		control, err := terminalcontrol.Open(controlDir, true)
		if err != nil {
			t.Fatal(err)
		}
		defer control.Close()
		if err = control.Serve(func(enabled bool) { current.Load().SetRemoteTerminals(enabled) }, func() bool { return current.Load().RemoteTerminalsAvailable() }); err != nil {
			t.Fatal(err)
		}
		var originalPID string
		runTerminalBrowser(t, public.URL, cookie, agent.ID, builder.ID, browserCommands, func(action string) map[string]any {
			if action == "offline-builder" {
				if builderOnline {
					stopBuilder()
					builderOnline = false
				}
				return map[string]any{"online": false}
			}
			if action == "online-builder" {
				if !builderOnline {
					stopBuilder = connectBuilder()
					builderOnline = true
				}
				return map[string]any{"online": true}
			}
			if action == "size" {
				return map[string]any{"grid": command("list-panes", "-a", "-F", "#{pane_height} #{pane_width}")}
			}
			if action == "state-builder" {
				if builderCommand("list-sessions", "-F", "#{session_name}") == "" {
					return map[string]any{"pid": ""}
				}
				return map[string]any{"pid": builderCommand("list-panes", "-a", "-F", "#{pane_pid}")}
			}
			if action == "disable-terminals" || action == "enable-terminals" {
				operation := "stop"
				if action == "enable-terminals" {
					operation = "start"
				}
				if _, err := terminalcontrol.Command(t.Context(), controlDir, operation, true); err != nil {
					t.Fatal(err)
				}
				stop()
				stopBuilder()
				stop = connect()
				stopBuilder = connectBuilder()
			}
			if action == "restart-controller" {
				stop()
				stopBuilder()
				current.Load().Close()
				next := New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts), WithRemoteTerminals(action != "disable-terminals"), WithPublicURL(public.URL))
				current.Store(next)
				stop = connect()
				stopBuilder = connectBuilder()
			}
			if action == "restart-agent" {
				stop()
				stop = connect()
			}
			pid := command("list-panes", "-a", "-F", "#{pane_pid}")
			if originalPID == "" {
				originalPID = pid
			} else if originalPID != pid {
				t.Fatal("browser recovery replaced the original shell", originalPID, pid)
			}
			sessions, err := st.TerminalSessionsFor(uid)
			if err != nil {
				t.Fatal(err)
			}
			return map[string]any{"pid": pid, "sessions": len(sessions)}
		})
		if originalPID == "" {
			t.Fatal("browser did not verify an original shell process")
		}
		for _, root := range []string{st.Path(), layout.Dir, builderLayout.Dir} {
			err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				for _, marker := range []string{"__browser_started__", "__controller_survived__", "__rollback_survived__"} {
					if bytes.Contains(data, []byte(marker)) {
						t.Errorf("terminal content persisted to %s", path)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal("privacy inspection", err)
			}
		}
		deadline := time.Now().Add(2 * time.Second)
		traffic := false
		for time.Now().Before(deadline) {
			minutes, err := st.RemoteHealthMinutesFor(uid, agent.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, minute := range minutes {
				if minute.Summary.InputBytes > 0 && minute.Summary.OutputBytes > 0 {
					traffic = true
				}
			}
			if traffic {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !traffic {
			t.Fatal("actual terminal payload traffic was not counted")
		}
		return
	}
	request := func(method, path string, in any) (int, []byte) {
		t.Helper()
		data, _ := json.Marshal(in)
		r, _ := http.NewRequestWithContext(t.Context(), method, public.URL+path, strings.NewReader(string(data)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", public.URL)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		resp, err := public.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, out
	}
	createID := terminalbroker.ID()
	in := map[string]any{"agentId": agent.ID, "requestId": createID, "title": "Build", "cols": 80, "rows": 24}
	code, data := request("POST", "/api/v2/terminal/sessions", in)
	if code != 200 {
		t.Fatalf("create %d %s", code, data)
	}
	var session store.TerminalSession
	if json.Unmarshal(data, &session) != nil || session.State != terminalproto.Running {
		t.Fatal(string(data))
	}
	pid := command("list-panes", "-a", "-F", "#{pane_pid}")
	code, data = request("POST", "/api/v2/terminal/sessions", in)
	var duplicate store.TerminalSession
	json.Unmarshal(data, &duplicate)
	if code != 200 || duplicate.ID != session.ID || command("list-panes", "-a", "-F", "#{pane_pid}") != pid {
		t.Fatal("creation retry replaced shell")
	}
	ticket := func(takeover bool) string {
		t.Helper()
		code, data := request("POST", "/api/v2/terminal/sessions/"+session.ID+"/attachments", terminalbroker.Options{Cols: 80, Rows: 24, Takeover: takeover})
		if code != 200 {
			t.Fatalf("ticket %d %s", code, data)
		}
		var out struct {
			Ticket string `json:"ticket"`
		}
		json.Unmarshal(data, &out)
		return out.Ticket
	}
	attach := func(id string) *websocket.Conn {
		t.Helper()
		c, _, err := websocket.Dial(t.Context(), public.URL+"/api/v2/terminal/socket", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {public.URL}, "Cookie": {sessionCookie + "=" + cookie}}})
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(map[string]string{"ticket": id})
		if err = c.Write(t.Context(), websocket.MessageText, data); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		kind, data, err := c.Read(ctx)
		if err != nil || kind != websocket.MessageText || !strings.Contains(string(data), "ready") {
			c.CloseNow()
			t.Fatalf("attach ready %q %v", data, err)
		}
		return c
	}
	firstTicket := ticket(false)
	browser := attach(firstTicket)
	defer browser.CloseNow()
	// Wait for the redraw before accepting input, as the browser UI must do.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	var screen []byte
	for !strings.Contains(string(screen), "stage6-ready>") {
		_, data, err = browser.Read(ctx)
		if err != nil {
			cancel()
			t.Fatal("redraw", err)
		}
		screen = append(screen, data...)
	}
	cancel()
	if err = browser.Write(t.Context(), websocket.MessageBinary, []byte("printf '__reet_%s__\\n' 'stage6'\r")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 3*time.Second)
	for !strings.Contains(string(screen), "__reet_stage6__") {
		_, data, err = browser.Read(ctx)
		if err != nil {
			cancel()
			t.Fatal("terminal output", err)
		}
		screen = append(screen, data...)
	}
	cancel()
	code, data = request("GET", "/api/v2/terminal/sessions/"+session.ID+"/history?lines=100", nil)
	if code != 200 || !strings.Contains(string(data), "__reet_stage6__") {
		t.Fatalf("history %d %s", code, data)
	}
	// Explicit transfer fences the previous socket and keeps the exact process.
	next := attach(ticket(true))
	defer next.CloseNow()
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	for {
		_, _, err = browser.Read(ctx)
		if err != nil {
			break
		}
	}
	cancel()
	if err == nil {
		t.Fatal("displaced browser kept control")
	}
	if command("list-panes", "-a", "-F", "#{pane_pid}") != pid {
		t.Fatal("takeover replaced shell")
	}
	next.CloseNow()
	stop()
	stop = func() {}
	if command("list-panes", "-a", "-F", "#{pane_pid}") != pid {
		t.Fatal("agent disconnect killed shell")
	}
	// Reconnect to the same registered shell, then queue a durable offline end.
	stop = connect()
	if command("list-panes", "-a", "-F", "#{pane_pid}") != pid {
		t.Fatal("agent restart replaced shell")
	}
	stop()
	stop = func() {}
	code, data = request("POST", "/api/v2/terminal/sessions/"+session.ID+"/end", map[string]string{"requestId": terminalbroker.ID()})
	if code != 200 || !strings.Contains(string(data), "end_requested") {
		t.Fatalf("offline end %d %s", code, data)
	}
	stop = connect()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, _ := st.TerminalSessionFor(uid, session.ID)
		if current.State == terminalproto.Ended {
			if command("list-sessions", "-F", "#{session_name}") != "" {
				t.Fatal("ended shell survived")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(fmt.Sprintf("pending end not dispatched for %s", session.ID))
}
