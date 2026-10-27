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
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/terminalagent"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// recordingAgent is a fake agent CLI. Each start writes its own directory
// holding $0, the physical working directory and every argument in its own
// file (so newlines and quotes survive), then stays alive as the pane command.
const recordingAgent = `#!/bin/sh
rec="RECORDS/$$"
mkdir -p "$rec"
printf '%s' "$0" > "$rec/argv0"
pwd -P > "$rec/pwd"
i=0
for a in "$@"; do
	i=$((i+1))
	printf '%s' "$a" > "$rec/arg.$i"
done
printf '%s' "$i" > "$rec/argc"
exec sleep 600
`

type recordedStart struct {
	argv0 string
	dir   string
	args  []string
}

func readStarts(t *testing.T, root string) []recordedStart {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []recordedStart
	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		count, err := os.ReadFile(filepath.Join(dir, "argc"))
		if err != nil {
			continue // still starting
		}
		n, _ := strconv.Atoi(string(count))
		argv0, _ := os.ReadFile(filepath.Join(dir, "argv0"))
		pwd, _ := os.ReadFile(filepath.Join(dir, "pwd"))
		start := recordedStart{argv0: string(argv0), dir: strings.TrimSuffix(string(pwd), "\n"), args: []string{}}
		for i := 1; i <= n; i++ {
			arg, err := os.ReadFile(filepath.Join(dir, "arg."+strconv.Itoa(i)))
			if err != nil {
				t.Fatal(err)
			}
			start.args = append(start.args, string(arg))
		}
		out = append(out, start)
	}
	return out
}

func TestTerminalPrivateLaunchStartsTheRecordedAgent(t *testing.T) {
	tmux, err := terminalhost.FindTmux(t.Context())
	if err != nil {
		if os.Getenv("REET_BROWSER_TESTS") == "1" {
			t.Fatal("launch gate requires installed tmux", err)
		}
		t.Skip(err)
	}
	dir, err := os.MkdirTemp("/tmp", "reet-launch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	home := filepath.Join(dir, "home")
	site := filepath.Join(home, "work", "site")
	records := filepath.Join(dir, "records")
	bin := filepath.Join(dir, "bin")
	outside := filepath.Join(dir, "outside")
	for _, d := range []string{site, records, bin, outside} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(home, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"claude", "gemini"} {
		script := strings.Replace(recordingAgent, "RECORDS", records, 1)
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	// The connector finds agents on its own PATH: only the recording fakes,
	// tmux and the base system, never a real agent CLI installed here.
	t.Setenv("PATH", bin+":"+filepath.Dir(tmux.Path)+":/usr/bin:/bin")

	srv, st, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	layout, err := terminalhost.NewEnrollmentLayout(filepath.Join(dir, "agent.json"), agent.ID)
	if err != nil {
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
	tmuxOut := func(args ...string) string {
		t.Helper()
		out, _ := exec.Command(tmux.Path, append([]string{"-S", layout.Socket}, args...)...).Output()
		return strings.TrimSpace(string(out))
	}
	deadline := time.Now().Add(3 * time.Second)
	for !terminalhost.SocketActive(layout) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	tmuxOut("set-option", "-g", "default-command", "exec /bin/sh -i")
	sessionNames := func() []string {
		out := tmuxOut("list-sessions", "-F", "#{session_name}")
		if out == "" {
			return nil
		}
		return strings.Split(out, "\n")
	}

	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	defer private.Close()
	public := httptest.NewServer(srv)
	defer public.Close()
	srv.s.publicURL = public.URL
	base, _ := url.Parse(private.URL)
	privateClient := &http.Client{Transport: terminalTestTransport{base, http.DefaultTransport}}

	// The connector side negotiates exactly as agenthealth does: it claims
	// launch only when the controller advertised it, and reports the tools its
	// built-in presets find on PATH.
	headers := http.Header{"X-Reet-Agent": {agent.ID}, "X-Reet-Agent-Key": {secret}, "X-Reet-Version": {"test"}}
	ctx, cancel := context.WithCancel(t.Context())
	c, response, err := websocket.Dial(ctx, private.URL+"/private/v1/control", &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	advertised := response.Header.Get(terminalproto.FeatureHeader)
	if !terminalproto.SupportsLaunch(advertised) || !terminalproto.SupportsCommandHistory(advertised) {
		t.Fatal("controller did not advertise launch", advertised)
	}
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalControl: true, TerminalEnabled: true, HealthEnabled: true, Readiness: terminalproto.Ready, CommandHistory: true, Launch: true, Tools: terminalhost.InstalledTools(exec.LookPath)}
	if !slices.Equal(caps.Tools, []string{"claude", "gemini"}) {
		t.Fatal("installed tools", caps.Tools)
	}
	headers.Set(terminalproto.FeatureHeader, terminalproto.CommandHistoryFeature+","+terminalproto.LaunchFeature)
	handshake, _ := terminalproto.EncodeHandshake(terminalproto.Handshake{ProtocolVersion: 1, RequestID: terminalbroker.ID(), AgentInstanceID: terminalbroker.ID(), Capabilities: caps})
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
	factory := terminalagent.Factory(terminalhost.ManagerOptions{EnrollmentID: agent.ID, Layout: layout, Home: home, Consent: func() error { return nil }})
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
	defer func() {
		cancel()
		c.CloseNow()
		<-done
	}()
	deadline = time.Now().Add(3 * time.Second)
	for !srv.s.terminals.Ready(uid, agent.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !srv.s.terminals.Ready(uid, agent.ID) {
		t.Fatal("private connection never became terminal-ready")
	}

	request := func(in any) (int, map[string]any, string) {
		t.Helper()
		data, _ := json.Marshal(in)
		r, _ := http.NewRequestWithContext(t.Context(), "POST", public.URL+"/api/v2/terminal/sessions", strings.NewReader(string(data)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", public.URL)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		resp, err := public.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out, string(raw)
	}
	waitStarts := func(n int) []recordedStart {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			starts := readStarts(t, records)
			if len(starts) >= n || time.Now().After(deadline) {
				return starts
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	// A prompt shaped like flags, separators and shell syntax reaches the agent
	// as one argument, in the chosen folder.
	prompt := "fix the bug -- --help \"quoted\" $(id) `id`\nthen -rf ; exit"
	launchID := terminalbroker.ID()
	launch := map[string]any{"agentId": agent.ID, "requestId": launchID, "title": "Claude", "cols": 80, "rows": 24, "tool": "claude", "model": "sonnet", "dir": site, "prompt": prompt}
	code, first, raw := request(launch)
	if code != 200 || first["state"] != "running" {
		t.Fatal("launch", code, raw)
	}
	starts := waitStarts(1)
	if len(starts) != 1 {
		t.Fatal("agent did not start once", starts)
	}
	realSite, _ := filepath.EvalSymlinks(site)
	realHome, _ := filepath.EvalSymlinks(home) // /tmp is a symlink on macOS
	if starts[0].argv0 != filepath.Join(bin, "claude") || starts[0].dir != realSite || !slices.Equal(starts[0].args, []string{"--model", "sonnet", "--", prompt}) {
		t.Fatalf("recorded start %#v", starts[0])
	}
	if names := sessionNames(); len(names) != 1 {
		t.Fatal("tmux sessions after launch", names)
	}

	// A retry of the same request starts no second process.
	code, again, raw := request(launch)
	if code != 200 || again["id"] != first["id"] {
		t.Fatal("retry", code, raw)
	}
	time.Sleep(300 * time.Millisecond)
	if starts := readStarts(t, records); len(starts) != 1 || len(sessionNames()) != 1 {
		t.Fatal("retry started another agent", len(starts), sessionNames())
	}
	changed := map[string]any{}
	for k, v := range launch {
		changed[k] = v
	}
	changed["model"] = "opus"
	if code, _, raw := request(changed); code != 409 {
		t.Fatal("changed retry", code, raw)
	}

	// A plain create still opens the user's shell in home.
	code, plain, raw := request(map[string]any{"agentId": agent.ID, "requestId": terminalbroker.ID(), "title": "Shell", "cols": 80, "rows": 24})
	if code != 200 || plain["state"] != "running" {
		t.Fatal("plain create", code, raw)
	}
	deadline = time.Now().Add(3 * time.Second)
	var panes []string
	for time.Now().Before(deadline) {
		panes = strings.Split(tmuxOut("list-panes", "-a", "-F", "#{pane_current_command} #{pane_current_path}"), "\n")
		sort.Strings(panes)
		if plainAndAgentPanes(panes, realHome, realSite) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !plainAndAgentPanes(panes, realHome, realSite) {
		t.Fatal("plain shell and agent panes", panes)
	}
	if len(readStarts(t, records)) != 1 {
		t.Fatal("plain create started an agent")
	}

	// Refusals leave no tmux session and start nothing.
	before, _ := st.TerminalSessionsFor(uid)
	for _, tc := range []struct {
		name   string
		fields map[string]any
		status int
		code   string
	}{
		{"escaping symlink", map[string]any{"tool": "claude", "dir": filepath.Join(home, "escape")}, 422, "folder_unavailable"},
		{"outside home", map[string]any{"tool": "gemini", "dir": outside}, 422, "folder_unavailable"},
		{"missing folder", map[string]any{"tool": "claude", "dir": filepath.Join(home, "missing")}, 422, "folder_unavailable"},
		{"not installed", map[string]any{"tool": "codex"}, 422, "tool_not_installed"},
		{"one-word prompt", map[string]any{"tool": "claude", "prompt": "purge"}, 422, "prompt_unsupported"},
	} {
		body := map[string]any{"agentId": agent.ID, "requestId": terminalbroker.ID(), "title": "Refused", "cols": 80, "rows": 24}
		for k, v := range tc.fields {
			body[k] = v
		}
		code, out, raw := request(body)
		if code != tc.status || out["code"] != tc.code {
			t.Fatal(tc.name, code, raw)
		}
		if names := sessionNames(); len(names) != 2 {
			t.Fatal(tc.name, "left a tmux session", names)
		}
	}
	after, _ := st.TerminalSessionsFor(uid)
	// The one-word prompt and the agent the machine does not advertise never
	// reached the connector or the store; the connector-refused launches are
	// recorded as lost, never running.
	if len(after) != len(before)+3 {
		t.Fatal("refusals recorded", len(before), len(after))
	}
	for _, s := range after {
		if s.Title == "Refused" && s.State != terminalproto.Lost {
			t.Fatal("refused launch recorded as", s.State)
		}
	}
	if len(readStarts(t, records)) != 1 {
		t.Fatal("a refused launch started an agent")
	}

	events := launchEvents(t, st, uid)
	if len(events) != 1 || !strings.Contains(events[0].Detail, `"tool":"claude"`) || strings.Contains(events[0].Detail, "fix the bug") {
		t.Fatal("launch audit", events)
	}
}

// plainAndAgentPanes reports whether the panes are exactly one interactive
// shell in home and the recorded agent in the launch folder. /bin/sh is dash on
// some systems and bash on others, so the shell's own name is not compared.
func plainAndAgentPanes(panes []string, home, site string) bool {
	if len(panes) != 2 {
		return false
	}
	shell, agent := panes[0], panes[1]
	if strings.HasPrefix(shell, "sleep ") {
		shell, agent = agent, shell
	}
	name, path, ok := strings.Cut(shell, " ")
	return ok && name != "sleep" && path == home && agent == "sleep "+site
}
