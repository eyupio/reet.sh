//go:build linux || darwin

package api

import (
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

// No provider call: exercise saved-repository Chat creation and exact worktree
// cleanup through real HTTP, broker, private connector and ACP startup.
func TestStructuredChatWorktreePrivateLifecycle(t *testing.T) {
	binary := os.Getenv("REET_OPENCODE_NATIVE")
	if binary == "" {
		t.Skip("set REET_OPENCODE_NATIVE to the reviewed executable")
	}
	tmux, err := terminalhost.FindTmux(t.Context())
	if err != nil {
		t.Fatal("structured connector integration requires tmux", err)
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
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalControl: true, TerminalEnabled: true, HealthEnabled: true, Readiness: terminalproto.Ready, Launch: true, Tools: []string{"opencode"}, ACP: true, ACPHistory: true, ACPApproval: true, Worktree: true, ACPWorktree: false}
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
		t.Fatal("structured connector never became ready")
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
	project, err := st.CreateFolderProjectFor(uid, store.FolderProject{Host: agent.ID, Dir: repo, Name: "Fixture project", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	create := map[string]any{"agentId": agent.ID, "requestId": terminalbroker.ID(), "title": "Isolated Chat", "cols": 80, "rows": 24, "tool": "opencode", "structured": true, "trust": true, "dir": repo}
	status, data := request("POST", "/api/v2/terminal/sessions", create)
	if status != 409 || !strings.Contains(string(data), "acp_worktree_unsupported") {
		t.Fatalf("old connector was not refused clearly: %d %s", status, data)
	}
	if rows, err := st.TerminalSessionsFor(uid); err != nil || len(rows) != 0 {
		t.Fatalf("old connector reserved a session: %d %v", len(rows), err)
	}
	caps.ACPWorktree = true
	if err := send(ctx, terminalproto.Message{Version: 1, RequestID: terminalbroker.ID(), Generation: ack.Generation, Body: terminalproto.Hello{AgentInstanceID: instance, Capabilities: caps}}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		current, err := st.RemoteCapabilitiesFor(uid, agent.ID)
		if err == nil && current.Capabilities.ACPWorktree && srv.s.terminals.Ready(uid, agent.ID) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, data = request("POST", "/api/v2/terminal/sessions", create)
	if status != 200 {
		t.Fatalf("structured create %d %s", status, data)
	}
	var session store.TerminalSession
	if err := json.Unmarshal(data, &session); err != nil || session.ACPSessionID == "" {
		t.Fatalf("structured chat not admitted: %s %v", data, err)
	}
	if session.WorktreePath == "" || session.WorktreeRepo != "repo" || session.WorktreeBranch == "" {
		t.Fatalf("structured create lost worktree metadata: %+v", session)
	}
	worktree := session.WorktreePath
	if worktree == repo {
		t.Fatal("structured worker ran in shared checkout")
	}
	worktreesBefore := git("-C", repo, "worktree", "list", "--porcelain")
	project.Name = "Renamed project"
	if _, err := st.UpdateFolderProjectFor(uid, project, project.Version); err != nil {
		t.Fatal(err)
	}
	status, data = request("POST", "/api/v2/terminal/sessions", create)
	var retry store.TerminalSession
	_ = json.Unmarshal(data, &retry)
	if status != 200 || retry.ID != session.ID || retry.WorktreePath != worktree {
		t.Fatalf("retry changed isolated create after rename: %d %s", status, data)
	}
	if string(worktreesBefore) != string(git("-C", repo, "worktree", "list", "--porcelain")) {
		t.Fatal("retry allocated a second worktree")
	}
	if err := os.WriteFile(filepath.Join(worktree, "keep.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	status, data = request("POST", "/api/v2/terminal/sessions/"+session.ID+"/archive", nil)
	if status != 409 || !strings.Contains(string(data), "session_running") {
		t.Fatalf("active worktree removed: %d %s", status, data)
	}
	status, data = request("POST", "/api/v2/terminal/sessions/"+session.ID+"/end", map[string]any{"requestId": terminalbroker.ID()})
	if status != 200 {
		t.Fatalf("end native worker %d %s", status, data)
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		row, err := st.TerminalSessionFor(uid, session.ID)
		if err == nil && (row.State == terminalproto.Exited || row.State == terminalproto.Ended) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, data = request("POST", "/api/v2/terminal/sessions/"+session.ID+"/archive", nil)
	if status != 409 || !strings.Contains(string(data), "worktree_unsaved") {
		t.Fatalf("unconfirmed changes removed: %d %s", status, data)
	}
	var evidence struct {
		Worktree struct {
			Changed, Unpushed int
			Fingerprint       string
		}
	}
	if err := json.Unmarshal(data, &evidence); err != nil || evidence.Worktree.Fingerprint == "" {
		t.Fatalf("unsaved evidence missing: %s %v", data, err)
	}
	old := map[string]any{"discard": true, "changed": evidence.Worktree.Changed, "unpushed": evidence.Worktree.Unpushed, "fingerprint": evidence.Worktree.Fingerprint}
	if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("changed since confirmation"), 0600); err != nil {
		t.Fatal(err)
	}
	status, data = request("POST", "/api/v2/terminal/sessions/"+session.ID+"/archive", old)
	if status != 409 {
		t.Fatalf("stale confirmation deleted new work: %d %s", status, data)
	}
	if _, err := os.Stat(filepath.Join(worktree, "keep.txt")); err != nil {
		t.Fatal("stale confirmation removed original work", err)
	}
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	status, data = request("POST", "/api/v2/terminal/sessions/"+session.ID+"/archive", map[string]any{"discard": true, "changed": evidence.Worktree.Changed, "unpushed": evidence.Worktree.Unpushed, "fingerprint": evidence.Worktree.Fingerprint})
	if status != 200 {
		t.Fatalf("confirmed worktree removal failed: %d %s", status, data)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatal("archive left worktree behind", err)
	}
	git("-C", repo, "show-ref", "--verify", "refs/heads/"+session.WorktreeBranch)
	sessions, err := st.TerminalSessionsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range sessions {
		if row.ID == session.ID {
			t.Fatal("archived session remained listed")
		}
	}
	t.Log("Saved-repository Chat launch, immutable retry after rename, active hold, exact unsaved confirmation, cleanup and branch retention passed against the real agent")
}
