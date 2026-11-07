//go:build linux || darwin

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalagent"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// No provider call: exercise the real HTTP authorization, ticket, broker,
// private connector, supervisor-owned ACP worker and disposable native PTY.
func TestNativeViewPrivateLifecyclePreservesActualChat(t *testing.T) {
	binary := os.Getenv("REET_OPENCODE_NATIVE")
	if binary == "" {
		t.Skip("set REET_OPENCODE_NATIVE to the reviewed executable")
	}
	tmux, err := terminalhost.FindTmux(t.Context())
	if err != nil {
		t.Fatal("native connector integration requires tmux", err)
	}
	home, err := os.MkdirTemp("/tmp", "reet-native-api-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, filepath.Join(bin, "opencode")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo := filepath.Join(home, "repo")
	git := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return out
	}
	git("init", "-b", "main", repo)
	git("-C", repo, "-c", "user.name=Native fixture", "-c", "user.email=fixture@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "fixture")
	worktree := filepath.Join(home, ".reet", "worktrees", "native", "repo", "0123456789abcdef0123456789abcdef")
	if err := os.MkdirAll(filepath.Dir(worktree), 0700); err != nil {
		t.Fatal(err)
	}
	git("-C", repo, "worktree", "add", "-b", "native-view-fixture", worktree, "HEAD")
	worktreesBefore := git("-C", repo, "worktree", "list", "--porcelain")
	srv, st, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	defer srv.Close()
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	layout, err := terminalhost.NewEnrollmentLayout(filepath.Join(home, "agent.json"), agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := terminalhost.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	daemon := exec.Command(tmux.Path, "-D", "-S", layout.Socket, "-f", layout.Config)
	daemon.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "SHELL=/bin/sh", "TERM=xterm-256color"}
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
		_ = daemon.Process.Kill()
		_ = daemon.Wait()
	}()
	for deadline := time.Now().Add(3 * time.Second); !terminalhost.SocketActive(layout) && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	opts := terminalhost.ManagerOptions{EnrollmentID: agent.ID, Layout: layout, Home: home, Consent: func() error { return nil }}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	supervisor := terminalagent.NewChatSupervisor(ctx, opts)
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	defer private.Close()
	public := httptest.NewServer(srv)
	defer public.Close()
	srv.s.publicURL = public.URL
	headers := http.Header{"X-Reet-Agent": {agent.ID}, "X-Reet-Agent-Key": {secret}, "X-Reet-Version": {"test"}}
	peer, response, err := websocket.Dial(ctx, private.URL+"/private/v1/control", &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.CloseNow()
	features := response.Header.Get(terminalproto.FeatureHeader)
	headers.Set(terminalproto.FeatureHeader, features)
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalControl: true, TerminalEnabled: true, HealthEnabled: true, Readiness: terminalproto.Ready, Launch: true, Tools: []string{"opencode"}, ACP: true, ACPHistory: true, ACPApproval: true, NativeView: true}
	instance := terminalbroker.ID()
	handshake, _ := terminalproto.EncodeHandshake(terminalproto.Handshake{ProtocolVersion: 1, RequestID: terminalbroker.ID(), AgentInstanceID: instance, Capabilities: caps})
	if err := peer.Write(ctx, websocket.MessageText, handshake); err != nil {
		t.Fatal(err)
	}
	_, raw, err := peer.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ack, err := terminalproto.DecodeHandshakeReply(raw)
	if err != nil {
		t.Fatal(err)
	}
	var writeMu sync.Mutex
	send := func(ctx context.Context, message terminalproto.Message) error {
		data, err := terminalproto.Encode(message)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		return peer.Write(ctx, websocket.MessageText, data)
	}
	base, _ := url.Parse(private.URL)
	handle, cleanup := terminalagent.FactoryWithChats(opts, supervisor)(ctx, &http.Client{Transport: terminalTestTransport{base, http.DefaultTransport}}, headers, ack.Generation, agenthealth.Sender(send))
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cleanup()
		for {
			_, data, err := peer.Read(ctx)
			if err != nil {
				return
			}
			message, err := terminalproto.Decode(data)
			if err != nil {
				return
			}
			if probe, ok := message.Body.(*terminalproto.Probe); ok {
				message.Body = terminalproto.ProbeResult(*probe)
				if send(ctx, message) != nil {
					return
				}
			} else if handle(ctx, message) != nil {
				return
			}
		}
	}()
	defer func() {
		cancel()
		peer.CloseNow()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("native connector did not stop")
		}
	}()
	cpu, total, available, disk, free, uptime := 1.0, float64(4<<30), float64(2<<30), float64(100<<30), float64(50<<30), 100.0
	if err := send(ctx, terminalproto.Message{Version: 1, RequestID: terminalbroker.ID(), Generation: ack.Generation, Body: terminalproto.HealthReport{SchemaVersion: 1, Sequence: 1, AgentInstanceID: instance, SampledAt: time.Now().UnixMilli(), Scope: "host", CPUPercent: terminalproto.Reading{Value: &cpu}, Memory: terminalproto.CapacityReading{Total: terminalproto.Reading{Value: &total}, Available: terminalproto.Reading{Value: &available}}, Disk: terminalproto.CapacityReading{Total: terminalproto.Reading{Value: &disk}, Available: terminalproto.Reading{Value: &free}}, HostUptimeSeconds: terminalproto.Reading{Value: &uptime}}}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(3 * time.Second); !srv.s.terminals.Ready(uid, agent.ID) && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if !srv.s.terminals.Ready(uid, agent.ID) {
		t.Fatal("native connector never became ready")
	}
	request := func(method, path string, body any) (int, []byte) {
		t.Helper()
		data, _ := json.Marshal(body)
		r, _ := http.NewRequestWithContext(ctx, method, public.URL+path, strings.NewReader(string(data)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", public.URL)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		response, err := public.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		out, _ := io.ReadAll(response.Body)
		return response.StatusCode, out
	}
	create := map[string]any{"agentId": agent.ID, "requestId": terminalbroker.ID(), "title": "Native shared view", "cols": 80, "rows": 24, "tool": "opencode", "structured": true, "trust": true, "dir": worktree}
	status, data := request("POST", "/api/v2/terminal/sessions", create)
	if status != 200 {
		t.Fatalf("native create %d %s", status, data)
	}
	var session store.TerminalSession
	if err := json.Unmarshal(data, &session); err != nil || session.ACPSessionID == "" {
		t.Fatalf("native chat not admitted: %s %v", data, err)
	}
	ready := func() {
		t.Helper()
		status, data := request("GET", "/api/v2/terminal/sessions/"+session.ID+"/native-view", nil)
		if status != 200 || !strings.Contains(string(data), `"available":true`) {
			t.Fatalf("native availability %d %s", status, data)
		}
	}
	ready()
	if status, _ := request("POST", "/api/v2/terminal/sessions/"+session.ID+"/attachments", terminalbroker.Options{Cols: 80, Rows: 24}); status == 200 {
		t.Fatal("ordinary ticket attached a structured worker")
	}
	attach := func() *websocket.Conn {
		t.Helper()
		status, data := request("POST", "/api/v2/terminal/sessions/"+session.ID+"/attachments", terminalbroker.Options{Cols: 80, Rows: 24, NativeView: true})
		if status != 200 {
			t.Fatalf("native ticket %d %s", status, data)
		}
		var ticket struct{ Ticket string }
		_ = json.Unmarshal(data, &ticket)
		socket, _, err := websocket.Dial(ctx, public.URL+"/api/v2/terminal/socket", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {public.URL}, "Cookie": {sessionCookie + "=" + cookie}}})
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(map[string]string{"ticket": ticket.Ticket})
		if err := socket.Write(ctx, websocket.MessageText, body); err != nil {
			t.Fatal(err)
		}
		kind, frame, err := socket.Read(ctx)
		if err != nil || kind != websocket.MessageText || !strings.Contains(string(frame), "ready") {
			socket.CloseNow()
			t.Fatalf("native socket not ready: %q %v", frame, err)
		}
		for {
			kind, frame, err = socket.Read(ctx)
			if err != nil {
				socket.CloseNow()
				t.Fatal(err)
			}
			if kind == websocket.MessageBinary && len(frame) > 0 {
				break
			}
		}
		return socket
	}
	first := attach()
	_ = first.Close(websocket.StatusNormalClosure, "detach native view")
	ready()
	second := attach()
	defer second.CloseNow()
	ready()
	unchanged, err := st.TerminalSessionFor(uid, session.ID)
	if err != nil || !unchanged.Structured || unchanged.State != terminalproto.Running || unchanged.ACPSessionID != session.ACPSessionID || unchanged.Dir != session.Dir {
		t.Fatal("native attachment replaced the session", err)
	}
	if !bytes.Equal(worktreesBefore, git("-C", repo, "worktree", "list", "--porcelain")) {
		t.Fatal("changing native views allocated or changed a worktree")
	}
	if unchanged.Dir != worktree {
		t.Fatal("native view changed the worktree directory")
	}
	status, data = request("POST", "/api/v2/terminal/sessions/"+session.ID+"/end", map[string]any{"requestId": terminalbroker.ID()})
	if status != 200 {
		t.Fatalf("end native worker %d %s", status, data)
	}
	endedCtx, endCancel := context.WithTimeout(ctx, 5*time.Second)
	defer endCancel()
	for {
		_, _, err := second.Read(endedCtx)
		if err != nil {
			if endedCtx.Err() != nil {
				t.Fatal("ended worker left its native client attached")
			}
			break
		}
	}
	t.Log("Real native TUI traversed HTTP ticket, broker socket, private connector and host PTY; detach/reattach preserved the structured conversation")
}
