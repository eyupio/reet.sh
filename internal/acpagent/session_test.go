//go:build linux || darwin

package acpagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func fakeACP(t *testing.T, scenario, version string) string {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "opencode")
	body := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo %s; exit 0; fi\nGO_WANT_ACP_HELPER=1 ACP_SCENARIO=%s ACP_VERSION=%s ACP_ORPHAN_FILE=%q exec %q -test.run=TestFakeACP\n", version, scenario, version, script+".pid", bin)
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	return script
}

// The test binary is also the fake ACP child. The generated script runs it
// with -test.run so it speaks JSON lines instead of running the suite again.
func TestFakeACP(t *testing.T) {
	if os.Getenv("GO_WANT_ACP_HELPER") != "1" {
		t.Skip("fake child")
	}
	scenario, version := os.Getenv("ACP_SCENARIO"), os.Getenv("ACP_VERSION")
	in := bufio.NewScanner(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	send := func(v any) {
		data, _ := json.Marshal(v)
		_, _ = out.Write(append(data, '\n'))
		_ = out.Flush()
	}
	reply := func(id json.RawMessage, result any) {
		send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	update := func(session, kind string, fields map[string]any) {
		fields["sessionUpdate"] = kind
		send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": session, "update": fields}})
	}
	var promptID json.RawMessage
	var session string
	for in.Scan() {
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &message) != nil {
			continue
		}
		switch message.Method {
		case "initialize":
			name, protocol := "OpenCode", 1
			if scenario == "wrong-name" {
				name = "Another Agent"
			}
			if scenario == "wrong-protocol" {
				protocol = 2
			}
			reply(message.ID, map[string]any{"protocolVersion": protocol, "agentInfo": map[string]string{"name": name, "version": version}})
		case "session/load":
			var params struct {
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(message.Params, &params)
			session = params.SessionID
			if scenario == "load-plan" {
				reply(message.ID, map[string]any{"configOptions": []map[string]any{{"id": "mode", "currentValue": "plan"}}})
				continue
			}
			update(session, "user_message_chunk", map[string]any{"messageId": "msg_old", "content": map[string]string{"type": "text", "text": "old question"}})
			update(session, "tool_call", map[string]any{"toolCallId": "call_old", "title": "old command", "kind": "execute", "status": "in_progress"})
			update(session, "agent_message_chunk", map[string]any{"messageId": "msg_old_reply", "content": map[string]string{"type": "text", "text": "old answer"}})
			reply(message.ID, map[string]any{"configOptions": []map[string]any{{"id": "model", "currentValue": "stub/model"}, {"id": "mode", "currentValue": "build"}}})
		case "session/new":
			mode := "build"
			if scenario == "mode-plan" {
				mode = "plan"
			}
			session = "ses_fake_1"
			reply(message.ID, map[string]any{"sessionId": session, "configOptions": []map[string]any{
				{"id": "model", "currentValue": "stub/model"},
				{"id": "mode", "currentValue": mode},
			}})
			if scenario == "orphan" {
				child := exec.Command("/bin/sh", "-c", "trap '' TERM; sleep 60")
				if child.Start() == nil {
					_ = os.WriteFile(os.Getenv("ACP_ORPHAN_FILE"), []byte(fmt.Sprint(child.Process.Pid)), 0600)
				}
				os.Exit(0)
			}
		case "session/prompt":
			var params struct {
				SessionID string `json:"sessionId"`
				Prompt    []struct {
					Text string `json:"text"`
				} `json:"prompt"`
			}
			_ = json.Unmarshal(message.Params, &params)
			session = params.SessionID
			promptID = message.ID
			switch scenario {
			case "basic":
				update(session, "agent_message_chunk", map[string]any{"messageId": "msg_1", "content": map[string]string{"type": "text", "text": "hello "}})
				update(session, "agent_message_chunk", map[string]any{"messageId": "msg_1", "content": map[string]string{"type": "text", "text": "there"}})
				reply(promptID, map[string]any{"stopReason": "end_turn"})
			case "permission", "timeout", "cancel":
				update(session, "tool_call", map[string]any{"toolCallId": "call_1", "title": "write hello.txt", "kind": "edit", "status": "pending"})
				update(session, "tool_call_update", map[string]any{"toolCallId": "call_1", "title": "write hello.txt", "kind": "edit", "status": "in_progress", "locations": []map[string]string{{"path": "/tmp/hello.txt"}}})
				send(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": map[string]any{
					"sessionId": session,
					"toolCall": map[string]any{"toolCallId": "call_1", "title": "write hello.txt", "kind": "edit", "status": "pending",
						"locations": []map[string]string{{"path": "/tmp/hello.txt"}},
						"rawInput":  map[string]string{"filepath": "/tmp/hello.txt", "diff": "@@ -0,0 +1 @@\n+hello\n"}},
					"options": []map[string]string{{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "always", "kind": "allow_always", "name": "Always allow"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}},
				}})
			case "watchdog":
				time.Sleep(120 * time.Millisecond)
				reply(promptID, map[string]any{"stopReason": "end_turn"})
			case "fs":
				send(map[string]any{"jsonrpc": "2.0", "id": 9, "method": "fs/write_text_file", "params": map[string]string{"sessionId": session, "path": "/tmp/mirror.txt", "content": "mirror"}})
			}
		case "session/cancel":
			if len(promptID) > 0 {
				reply(promptID, map[string]any{"stopReason": "cancelled"})
				promptID = nil
			}
		default:
			if len(message.ID) > 0 && string(message.ID) != "null" {
				// A permission answer, the fs mirror result, or an unknown
				// request. A permission answer ends the fake turn.
				var answer struct {
					Result struct {
						Outcome struct {
							Outcome  string `json:"outcome"`
							OptionID string `json:"optionId"`
						} `json:"outcome"`
					} `json:"result"`
				}
				_ = json.Unmarshal(in.Bytes(), &answer)
				if answer.Result.Outcome.OptionID == "once" || answer.Result.Outcome.OptionID == "reject" {
					status := "completed"
					if answer.Result.Outcome.OptionID == "reject" {
						status = "failed"
					}
					update(session, "tool_call_update", map[string]any{"toolCallId": "call_1", "title": "write hello.txt", "kind": "edit", "status": status})
					reply(promptID, map[string]any{"stopReason": "end_turn"})
					promptID = nil
				} else if scenario == "fs" {
					update(session, "agent_message_chunk", map[string]any{"messageId": "msg_fs", "content": map[string]string{"type": "text", "text": "mirrored"}})
					reply(promptID, map[string]any{"stopReason": "end_turn"})
					promptID = nil
				}
			}
		}
	}
	os.Exit(0)
}

type events struct {
	mu   sync.Mutex
	all  []Event
	wait chan Event
}

func (e *events) add(event Event) {
	e.mu.Lock()
	e.all = append(e.all, event)
	e.mu.Unlock()
	select {
	case e.wait <- event:
	default:
	}
}
func (e *events) next(t *testing.T) Event {
	t.Helper()
	select {
	case event := <-e.wait:
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("no event")
		return Event{}
	}
}
func (e *events) has(kind string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, event := range e.all {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

const testVersion = "1.19.5"

func options(t *testing.T, scenario, version string) (Options, *events) {
	t.Helper()
	home, dir := t.TempDir(), t.TempDir()
	got := &events{wait: make(chan Event, 32)}
	return Options{Agent: OpenCode, Binary: fakeACP(t, scenario, version), Dir: dir, Home: home,
		BaseEnv:           []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"},
		PermissionTimeout: time.Second, StillWorkingAfter: 40 * time.Millisecond, TurnTimeout: 2 * time.Second,
		OnEvent: got.add}, got
}

func TestPermissionAllowDeny(t *testing.T) {
	for _, allow := range []bool{true, false} {
		t.Run(fmt.Sprint(allow), func(t *testing.T) {
			opts, got := options(t, "permission", testVersion)
			session, err := Start(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			done := make(chan error, 1)
			go func() { done <- session.Prompt(context.Background(), "write a file") }()
			event := got.next(t)
			for event.Kind != "permission" {
				event = got.next(t)
			}
			if event.Command != "" || !strings.Contains(event.Diff, "hello") || event.Truncated {
				t.Fatalf("permission %+v", event)
			}
			if err := session.Allow(event.PermissionID, allow); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			resolved := false
			for e := range got.wait {
				if e.Kind == "permission_resolved" {
					resolved = e.Allow != nil && *e.Allow == allow
					break
				}
			}
			if !resolved {
				t.Fatal("no resolution")
			}
		})
	}
}

func TestCancelClosesPermissionCard(t *testing.T) {
	opts, got := options(t, "permission", testVersion)
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), "write a file") }()
	for event := got.next(t); event.Kind != "permission"; event = got.next(t) {
	}
	if err := session.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not end after cancel")
	}
	if !got.has("permission_cancelled") {
		t.Fatal("card was not closed")
	}
}

func TestPermissionTimeout(t *testing.T) {
	opts, got := options(t, "timeout", testVersion)
	opts.PermissionTimeout = 30 * time.Millisecond
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), "write a file") }()
	for event := got.next(t); event.Kind != "permission"; event = got.next(t) {
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not end after timeout")
	}
	found := false
	for e := range got.wait {
		if e.Kind == "permission_cancelled" && e.Reason == "timeout" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("timeout did not close the card")
	}
}

func TestVersionAndModeRefused(t *testing.T) {
	opts, _ := options(t, "basic", "1.15.13")
	if _, err := Start(context.Background(), opts); !errors.Is(err, ErrVersion) {
		t.Fatalf("version: %v", err)
	}
	opts, _ = options(t, "mode-plan", testVersion)
	if _, err := Start(context.Background(), opts); err == nil {
		t.Fatal("plan mode accepted")
	}
}

func TestHandshakeRequiresTheNamedAgentAndProtocol(t *testing.T) {
	opts, _ := options(t, "wrong-name", testVersion)
	if _, err := Start(context.Background(), opts); !errors.Is(err, ErrVersion) {
		t.Fatalf("wrong name: %v", err)
	}
	var version *VersionError
	opts, _ = options(t, "wrong-name", testVersion)
	if _, err := Start(context.Background(), opts); !errors.As(err, &version) || version.Required != "" {
		t.Fatalf("wrong name error: %v", err)
	}
	opts, _ = options(t, "wrong-protocol", testVersion)
	if _, err := Start(context.Background(), opts); !errors.As(err, &version) || version.Required != "ACP protocol 1" {
		t.Fatalf("wrong protocol error: %v", err)
	}
}

func TestPluginsNeedTrust(t *testing.T) {
	opts, _ := options(t, "basic", testVersion)
	if err := os.MkdirAll(filepath.Join(opts.Dir, ".opencode"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.Dir, ".opencode", "plugin.ts"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(context.Background(), opts); err == nil {
		t.Fatal("plugin folder accepted without trust")
	}
	opts.Trust = true
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	session.Close()
}

func TestWatchdogAndFSMirror(t *testing.T) {
	opts, got := options(t, "watchdog", testVersion)
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "slow"); err != nil {
		t.Fatal(err)
	}
	if !got.has("notice") {
		t.Fatal("watchdog notice missing")
	}
	session.Close()

	opts, got = options(t, "fs", testVersion)
	session, err = Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Prompt(context.Background(), "mirror"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range got.all {
		if event.Kind == "agent" && event.Text == "mirrored" {
			found = true
		}
	}
	if !found {
		t.Fatal("fs mirror did not continue the turn")
	}
}

func TestEnvironmentScrub(t *testing.T) {
	env := agentEnvironment(OpenCode, "/home/u", []string{
		"PATH=/bin", "HOME=/elsewhere", "OPENCODE_PERMISSION={}", "OPENCODE_CONFIG=/tmp/x",
		"OPENCODE_API_KEY=oc_secret", "OPENAI_API_KEY=sk-test", "SSH_AUTH_SOCK=/tmp/ssh", "GH_TOKEN=secret", "LANG=C.UTF-8",
	}, LaunchEnv{Password: "pw"})
	joined := strings.Join(env, "\n")
	for _, bad := range []string{"OPENCODE_PERMISSION", "OPENCODE_CONFIG=", "SSH_AUTH_SOCK", "GH_TOKEN", "HOME=/elsewhere"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("environment kept %s:\n%s", bad, joined)
		}
	}
	for _, good := range []string{"HOME=/home/u", "OPENCODE_SERVER_PASSWORD=pw", "OPENCODE_DISABLE_PROJECT_CONFIG=1", "OPENCODE_DISABLE_MODELS_FETCH=1", "OPENCODE_API_KEY=oc_secret", "OPENAI_API_KEY=sk-test"} {
		if !strings.Contains(joined, good) {
			t.Fatalf("environment missing %s:\n%s", good, joined)
		}
	}
}

func TestCloseKillsOrphanedGroupMembers(t *testing.T) {
	opts, _ := options(t, "orphan", testVersion)
	pidFile := opts.Binary + ".pid"
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	for i := 0; i < 100; i++ {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, _ = strconv.Atoi(string(data))
			if pid > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid <= 0 {
		t.Fatal("fake child did not report its pid")
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	session.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("orphaned child %d survived Close", pid)
}

func TestResumeLoadsHistoryAndMarksInterruptedTools(t *testing.T) {
	opts, got := options(t, "basic", testVersion)
	opts.Resume = "ses_fake000000000000000000000001"
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.SessionID() != opts.Resume {
		t.Fatalf("resumed id %q", session.SessionID())
	}
	if !got.has("agent") || !got.has("user") {
		t.Fatal("replay events missing")
	}
	found := false
	for _, event := range got.all {
		if event.Kind == "tool" && event.ToolCallID == "call_old" && event.Status == "completed" {
			t.Fatal("interrupted tool stayed working")
		}
		if event.Kind == "tool" && event.ToolCallID == "call_old" && event.Status == "failed" {
			found = true
		}
	}
	if !found {
		t.Fatal("interrupted tool was not marked failed")
	}
}

func TestResumePlanModeRefused(t *testing.T) {
	opts, _ := options(t, "load-plan", testVersion)
	opts.Resume = "ses_fake000000000000000000000001"
	if _, err := Start(context.Background(), opts); err == nil {
		t.Fatal("plan mode load accepted")
	}
}
