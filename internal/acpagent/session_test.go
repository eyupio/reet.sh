//go:build linux || darwin

package acpagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
			capabilities := map[string]any{"promptCapabilities": map[string]bool{"image": true}}
			if scenario == "no-images" {
				capabilities = map[string]any{"promptCapabilities": map[string]bool{"image": false}}
			}
			reply(message.ID, map[string]any{"protocolVersion": protocol, "agentInfo": map[string]string{"name": name, "version": version},
				"agentCapabilities": capabilities})
		case "session/set_config_option":
			var params struct {
				ConfigID  string `json:"configId"`
				Value     string `json:"value"`
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(message.Params, &params)
			if params.ConfigID == "mode" {
				// A profile's StartMode switch: confirmed in the reply, silently ignored, or refused.
				switch scenario {
				case "mode-switch", "mode-live":
					reply(message.ID, map[string]any{"configOptions": []map[string]any{{"id": "mode", "currentValue": params.Value}}})
				case "mode-ignored":
					reply(message.ID, map[string]any{"configOptions": []map[string]any{{"id": "mode", "currentValue": "plan"}}})
				case "mode-silent":
					reply(message.ID, map[string]any{})
				default:
					send(map[string]any{"jsonrpc": "2.0", "id": message.ID, "error": map[string]any{"code": -32602, "message": "no"}})
				}
				continue
			}
			if params.ConfigID != "model" || params.SessionID != session || params.Value == "reject" {
				send(map[string]any{"jsonrpc": "2.0", "id": message.ID, "error": map[string]any{"code": -32602, "message": "invalid model"}})
			} else if params.Value == "silently-reset" {
				// The Claude adapter accepts an unknown model and resets to default.
				reply(message.ID, map[string]any{"configOptions": []map[string]any{{"id": "model", "currentValue": "default"}}})
			} else if params.Value == "echo-list" {
				reply(message.ID, map[string]any{"configOptions": []map[string]any{{"id": "mode", "currentValue": "build"}, {"id": "model", "currentValue": "echo-list"}}})
			} else {
				reply(message.ID, map[string]any{})
			}
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
			if scenario == "mode-plan" || scenario == "mode-switch" || scenario == "mode-ignored" || scenario == "mode-silent" {
				mode = "plan"
			}
			session = "ses_fake_1"
			if scenario == "question-early" {
				// Asked while the chat is still starting, before session/new has answered.
				send(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "elicitation/create", "params": map[string]any{
					"mode": "form", "sessionId": session, "message": "Too early?",
					"requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string", "title": "A"}}}}})
				time.Sleep(50 * time.Millisecond)
			}
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
					Type     string `json:"type"`
					Text     string `json:"text"`
					MIMEType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"prompt"`
			}
			_ = json.Unmarshal(message.Params, &params)
			session = params.SessionID
			promptID = message.ID
			switch scenario {
			case "image":
				ok := false
				for _, block := range params.Prompt {
					if block.Type != "image" || block.MIMEType != "image/png" || len(block.Text) != 0 {
						continue
					}
					data, err := base64.StdEncoding.DecodeString(block.Data)
					if err == nil && bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
						ok = true
					}
				}
				text := "image missing"
				if ok {
					text = "image ok"
				}
				update(session, "agent_message_chunk", map[string]any{"messageId": "msg_image", "content": map[string]string{"type": "text", "text": text}})
				reply(promptID, map[string]any{"stopReason": "end_turn"})
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
			case "question", "question-bad", "question-off", "question-big":
				properties := map[string]any{
					"question_0": map[string]any{"type": "string", "title": "Probe", "oneOf": []map[string]string{
						{"const": "Option A", "title": "Option A", "description": "First"}, {"const": "Option B", "title": "Option B"}}},
					"question_0_custom": map[string]any{"type": "string", "title": "Other"},
				}
				if scenario == "question-big" {
					// Valid for the machine parser (under 16 KiB) but over the wire bound.
					properties = map[string]any{}
					for f := 0; f < 6; f++ {
						var options []map[string]string
						for o := 0; o < 12; o++ {
							options = append(options, map[string]string{"const": fmt.Sprintf("v%02d%02d", f, o), "title": strings.Repeat("t", 150)})
						}
						properties[fmt.Sprintf("question_%d", f)] = map[string]any{"type": "string", "oneOf": options}
					}
				}
				if scenario == "question-bad" {
					properties["question_0_custom"] = map[string]any{"type": "string", "pattern": "^a$"}
				}
				send(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "elicitation/create", "params": map[string]any{
					"mode": "form", "sessionId": session, "toolCallId": "toolu_q", "message": "Which probe option?",
					"requestedSchema": map[string]any{"type": "object", "properties": properties, "required": []string{"question_0"}}}})
			case "tool-details":
				update(session, "tool_call", map[string]any{"toolCallId": "call_run", "title": "go test ./...", "kind": "execute", "status": "pending",
					"rawInput": map[string]string{"command": "go test ./..."}})
				update(session, "tool_call", map[string]any{"toolCallId": "call_edit", "title": "notes.txt", "kind": "edit", "status": "pending",
					"locations": []map[string]string{{"path": "/work/notes.txt"}}})
				time.Sleep(8 * time.Millisecond)
				update(session, "tool_call_update", map[string]any{"toolCallId": "call_run", "title": "go test ./...", "kind": "execute", "status": "completed",
					"rawInput": map[string]string{"command": "go test ./..."},
					"content":  []map[string]any{{"type": "content", "content": map[string]string{"type": "text", "text": "ok"}}}})
				update(session, "tool_call_update", map[string]any{"toolCallId": "call_edit", "title": "notes.txt", "kind": "edit", "status": "completed",
					"locations": []map[string]string{{"path": "/work/notes.txt"}},
					"rawInput":  map[string]string{"filePath": "/work/notes.txt"},
					"rawOutput": map[string]any{"metadata": map[string]any{"filediff": map[string]any{"patch": "@@ -1 +1 @@\n-alpha\n+ALPHA\n", "file": "/work/notes.txt", "additions": 1, "deletions": 1}}}})
				reply(promptID, map[string]any{"stopReason": "end_turn"})
			case "watchdog":
				time.Sleep(120 * time.Millisecond)
				reply(promptID, map[string]any{"stopReason": "end_turn"})
			case "silent-error":
				logDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "opencode", "log")
				_ = os.MkdirAll(logDir, 0700)
				line := `timestamp=` + time.Now().UTC().Format(time.RFC3339Nano) +
					` level=ERROR run=x message="stream error" providerID=opencode modelID=stub session.id=` + session +
					` small=false agent=build mode=primary error.error="AI_APICallError: quota exhausted"` + "\n"
				_ = os.WriteFile(filepath.Join(logDir, "opencode.log"), []byte(line), 0600)
			case "progress-first":
				update(session, "agent_message_chunk", map[string]any{"messageId": "msg_progress", "content": map[string]string{"type": "text", "text": "still here"}})
				time.Sleep(120 * time.Millisecond)
				reply(promptID, map[string]any{"stopReason": "end_turn"})
			case "progress-after-silence":
				time.Sleep(60 * time.Millisecond)
				update(session, "agent_message_chunk", map[string]any{"messageId": "msg_progress", "content": map[string]string{"type": "text", "text": "still here"}})
				time.Sleep(60 * time.Millisecond)
				reply(promptID, map[string]any{"stopReason": "end_turn"})
			case "progress-after-silence-hang":
				time.Sleep(60 * time.Millisecond)
				update(session, "agent_message_chunk", map[string]any{"messageId": "msg_progress", "content": map[string]string{"type": "text", "text": "still here"}})
			case "question-late":
				// The turn ends, and only then does the agent ask a question.
				reply(promptID, map[string]any{"stopReason": "end_turn"})
				promptID = nil
				time.Sleep(100 * time.Millisecond)
				send(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "elicitation/create", "params": map[string]any{
					"mode": "form", "sessionId": session, "toolCallId": "toolu_late", "message": "Too late?",
					"requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string", "title": "A"}}}}})
			case "late-permission":
				// The turn ends, and only then does the agent ask for a permission.
				reply(promptID, map[string]any{"stopReason": "end_turn"})
				promptID = nil
				time.Sleep(100 * time.Millisecond)
				send(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": map[string]any{
					"sessionId": session,
					"toolCall": map[string]any{"toolCallId": "call_late", "title": "write late.txt", "kind": "edit", "status": "pending",
						"rawInput": map[string]string{"filepath": "/tmp/late.txt", "diff": "@@ -0,0 +1 @@\n+late\n"}},
					"options": []map[string]string{{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}},
				}})
			case "permission-on-cancel":
				// Hangs until cancelled; see the session/cancel handler.
			case "running-command":
				// A command is started and never reported finished; the prompt waits for session/cancel.
				update(session, "tool_call", map[string]any{"toolCallId": "call_run", "title": "sleep 47", "kind": "execute", "status": "in_progress"})
			case "ignore-cancel":
				// Never answers the prompt and ignores session/cancel.
			case "steady-progress":
				for i := 0; i < 10; i++ {
					time.Sleep(30 * time.Millisecond)
					update(session, "agent_message_chunk", map[string]any{"messageId": "msg_steady", "content": map[string]string{"type": "text", "text": "."}})
				}
				reply(promptID, map[string]any{"stopReason": "end_turn"})
			case "fs":
				send(map[string]any{"jsonrpc": "2.0", "id": 9, "method": "fs/write_text_file", "params": map[string]string{"sessionId": session, "path": "/tmp/mirror.txt", "content": "mirror"}})
			}
		case "session/cancel":
			if scenario == "ignore-cancel" {
				break
			}
			if scenario == "permission-on-cancel" {
				// Winding down, the agent still asks for a permission before it finishes.
				send(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "session/request_permission", "params": map[string]any{
					"sessionId": session,
					"toolCall": map[string]any{"toolCallId": "call_cancel", "title": "write cancel.txt", "kind": "edit", "status": "pending",
						"rawInput": map[string]string{"filepath": "/tmp/cancel.txt", "diff": "@@ -0,0 +1 @@\n+x\n"}},
					"options": []map[string]string{{"optionId": "once", "kind": "allow_once", "name": "Allow once"}, {"optionId": "reject", "kind": "reject_once", "name": "Reject"}},
				}})
				time.Sleep(100 * time.Millisecond)
			}
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
				} else if scenario == "late-permission" || scenario == "permission-on-cancel" {
					var raw struct {
						Result json.RawMessage `json:"result"`
					}
					_ = json.Unmarshal(in.Bytes(), &raw)
					update(session, "agent_message_chunk", map[string]any{"messageId": "msg_late", "content": map[string]string{"type": "text", "text": "late answer: " + string(raw.Result)}})
				} else if strings.HasPrefix(scenario, "question") {
					// Echo exactly what the agent received so the test can assert it.
					var raw struct {
						Result json.RawMessage `json:"result"`
					}
					_ = json.Unmarshal(in.Bytes(), &raw)
					update(session, "agent_message_chunk", map[string]any{"messageId": "msg_q", "content": map[string]string{"type": "text", "text": string(raw.Result)}})
					if len(promptID) > 0 {
						reply(promptID, map[string]any{"stopReason": "end_turn"})
						promptID = nil
					}
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

func (e *events) text(text string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, event := range e.all {
		if event.Text == text {
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

func TestToolDetailsForTheRow(t *testing.T) {
	opts, got := options(t, "tool-details", testVersion)
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Prompt(context.Background(), "run the tests and edit the notes"); err != nil {
		t.Fatal(err)
	}
	var run, edit Event
	deadline := time.After(3 * time.Second)
	for run.ToolCallID == "" || edit.ToolCallID == "" {
		select {
		case event := <-got.wait:
			if event.Kind != "tool" || event.Status != "completed" {
				continue
			}
			switch event.ToolCallID {
			case "call_run":
				run = event
			case "call_edit":
				edit = event
			}
		case <-deadline:
			t.Fatalf("tool details not seen: run=%+v edit=%+v", run, edit)
		}
	}
	if run.Command != "go test ./..." || run.DurationMs <= 0 {
		t.Fatalf("run details %+v", run)
	}
	if edit.Path != "/work/notes.txt" || !strings.Contains(edit.Diff, "-alpha") || edit.Additions != 1 || edit.Deletions != 1 || edit.DurationMs <= 0 {
		t.Fatalf("edit details %+v", edit)
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

func TestPromptEmbedsImagesAndHonoursTheCapability(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n")
	opts, got := options(t, "image", testVersion)
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if !session.ImagesSupported() {
		t.Fatal("image capability was not read")
	}
	if err := session.PromptWithAttachments(context.Background(), "what is wrong?", []Attachment{{MIME: "image/png", Data: png}}); err != nil {
		t.Fatal(err)
	}
	if !got.text("image ok") {
		t.Fatal("the image content block did not reach the agent")
	}

	opts, _ = options(t, "no-images", testVersion)
	session, err = Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.ImagesSupported() {
		t.Fatal("a false capability was read as true")
	}
	if err := session.PromptWithAttachments(context.Background(), "look", []Attachment{{MIME: "image/png", Data: png}}); !errors.Is(err, ErrImages) {
		t.Fatalf("image without the capability: %v", err)
	}
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

func TestSilentProviderErrorStopsTheTurn(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	opts, got := options(t, "silent-error", testVersion)
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), "say hello") }()
	var reported Event
	for event := got.next(t); ; event = got.next(t) {
		if event.Kind == "status" && event.State == "error" {
			reported = event
			break
		}
	}
	if !strings.Contains(reported.Detail, "quota exhausted") {
		t.Fatalf("error detail %q", reported.Detail)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("prompt: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not stop after the provider error")
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	for _, event := range got.all {
		if event.Kind == "status" && event.State == "cancelled" {
			t.Fatal("the provider error was replaced by a cancelled status")
		}
	}
}

func TestSilentRecoveryDoesNotOutliveTheTurn(t *testing.T) {
	opts, got := options(t, "watchdog", testVersion)
	opts.StillWorkingAfter = 20 * time.Millisecond
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	opts.Agent.RecoverLive = func(_, _ string, _ int64) string {
		once.Do(func() { close(entered) })
		<-release
		return "late provider error"
	}
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), "slow") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("the watchdog never read the live error source")
	}
	for event := got.next(t); ; event = got.next(t) {
		if event.Kind == "status" && event.State == "idle" && event.Detail == "" {
			break
		}
	}
	// Let Prompt reach its deferred join, which closes watchDone.
	time.Sleep(20 * time.Millisecond)
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("prompt: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt waited for the watchdog after the turn")
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	for _, event := range got.all {
		if event.Kind == "status" && (event.State == "error" || event.State == "cancelled") {
			t.Fatalf("the finished turn was reported as %s: %+v", event.State, event)
		}
	}
}

func TestSilentRecoverySkipsTurnsWithProgress(t *testing.T) {
	t.Run("progress before the check", func(t *testing.T) {
		opts, got := options(t, "progress-first", testVersion)
		opts.StillWorkingAfter = 20 * time.Millisecond
		var called atomic.Bool
		opts.Agent.RecoverLive = func(_, _ string, _ int64) string {
			called.Store(true)
			return "stale provider error"
		}
		session, err := Start(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		if err := session.Prompt(context.Background(), "working"); err != nil {
			t.Fatal(err)
		}
		if called.Load() {
			t.Fatal("live recovery ran for a turn that streamed progress")
		}
		got.mu.Lock()
		defer got.mu.Unlock()
		for _, event := range got.all {
			if event.Kind == "status" && event.State == "error" {
				t.Fatalf("healthy progress was stopped: %+v", event)
			}
		}
	})

	t.Run("progress after the read", func(t *testing.T) {
		opts, got := options(t, "progress-after-silence", testVersion)
		opts.StillWorkingAfter = 20 * time.Millisecond
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		opts.Agent.RecoverLive = func(_, _ string, _ int64) string {
			once.Do(func() { close(entered) })
			<-release
			return "stale provider error"
		}
		session, err := Start(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		done := make(chan error, 1)
		go func() { done <- session.Prompt(context.Background(), "working") }()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("the watchdog never read the live error source")
		}
		for event := got.next(t); ; event = got.next(t) {
			if event.Kind == "agent" {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		close(release)
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("prompt: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("prompt did not finish")
		}
		got.mu.Lock()
		defer got.mu.Unlock()
		for _, event := range got.all {
			if event.Kind == "status" && event.State == "error" {
				t.Fatalf("late progress was stopped: %+v", event)
			}
		}
	})

	t.Run("progress while a stale error is read", func(t *testing.T) {
		opts, got := options(t, "progress-after-silence-hang", testVersion)
		opts.StillWorkingAfter = 20 * time.Millisecond
		opts.TurnTimeout = 150 * time.Millisecond
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		opts.Agent.RecoverLive = func(_, _ string, _ int64) string {
			once.Do(func() { close(entered) })
			<-release
			return "stale provider error"
		}
		session, err := Start(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		done := make(chan error, 1)
		go func() { done <- session.Prompt(context.Background(), "working") }()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("the watchdog never read the live error source")
		}
		for event := got.next(t); ; event = got.next(t) {
			if event.Kind == "agent" {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		close(release)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("prompt did not finish")
		}
		got.mu.Lock()
		defer got.mu.Unlock()
		stopped := false
		for _, event := range got.all {
			if event.Kind == "status" && event.State == "error" {
				t.Fatalf("stale error stopped healthy work: %+v", event)
			}
			if event.Kind == "notice" && strings.Contains(event.Text, "took too long") {
				stopped = true
			}
		}
		if !stopped {
			t.Fatal("the watchdog stopped watching after the stale read")
		}
	})
}

func TestActiveTurnOutlivesTheIdleTimeout(t *testing.T) {
	opts, got := options(t, "steady-progress", testVersion)
	opts.StillWorkingAfter = 20 * time.Millisecond
	opts.TurnTimeout = 150 * time.Millisecond
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Prompt(context.Background(), "working"); err != nil {
		t.Fatalf("an active turn was stopped: %v", err)
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	for _, event := range got.all {
		if event.Kind == "notice" && strings.Contains(event.Text, "took too long") {
			t.Fatalf("an active turn was timed out: %+v", event)
		}
	}
}

func TestSilentTurnIsAbandonedWhenCancelIsIgnored(t *testing.T) {
	old := cancelGrace
	cancelGrace = 100 * time.Millisecond
	defer func() { cancelGrace = old }()
	opts, _ := options(t, "ignore-cancel", testVersion)
	opts.StillWorkingAfter = 20 * time.Millisecond
	opts.TurnTimeout = 100 * time.Millisecond
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), "hang") }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a silent turn stayed busy after the adapter ignored cancel")
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

func TestAllowAllPendingAnswersTheCardOnScreen(t *testing.T) {
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
	if err := session.AllowAllPending(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for e := range got.wait {
		if e.Kind == "permission_resolved" {
			if e.Allow == nil || !*e.Allow {
				t.Fatalf("resolved %+v", e)
			}
			return
		}
	}
	t.Fatal("no resolution")
}

func TestSwitchModelPreservesSessionAndFailureKeepsModel(t *testing.T) {
	opts, _ := options(t, "", testVersion)
	s, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := s.SessionID()
	if err := s.SetModel(context.Background(), "provider/other"); err != nil {
		t.Fatal(err)
	}
	if s.Model() != "provider/other" || s.SessionID() != id {
		t.Fatal("model switch replaced history or failed")
	}
	if err := s.SetModel(context.Background(), "reject"); err == nil {
		t.Fatal("accepted rejected model")
	}
	if s.Model() != "provider/other" {
		t.Fatal("failed switch changed model")
	}
	s.turnMu.Lock()
	s.working = true
	s.turnMu.Unlock()
	if err := s.SetModel(context.Background(), "provider/busy"); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy switch: %v", err)
	}
	s.turnMu.Lock()
	s.working = false
	s.turnMu.Unlock()
}

func TestTypedDiffFromClaudeWrite(t *testing.T) {
	// Shape captured from the Claude adapter's Write permission request.
	const raw = `{"sessionId":"0b9f6a1c-3d2e-4f5a-8b7c-1234567890ab","toolCall":{"toolCallId":"t1","kind":"edit",
		"rawInput":{"file_path":"/p/m.txt","content":"reet-probe"},
		"content":[{"type":"diff","path":"/p/m.txt","oldText":null,"newText":"reet-probe"}]}}`
	var params permissionParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatal(err)
	}
	diff, path, truncated, ok := typedDiff(params.ToolCall.Content)
	if !ok || truncated || path != "/p/m.txt" || params.ToolCall.RawInput.SnakePath != "/p/m.txt" ||
		diff != "--- /p/m.txt\n+++ /p/m.txt\n+reet-probe\n" {
		t.Fatalf("diff %q path %q ok %v", diff, path, ok)
	}
	old, next := "a\nb\n", "a\nc\n"
	diff, _, _, ok = typedDiff([]permissionContent{{Type: "diff", Path: "/x", OldText: &old, NewText: &next}})
	if !ok || !strings.Contains(diff, "-b\n") || !strings.Contains(diff, "+c\n") {
		t.Fatalf("replace diff %q", diff)
	}
}

func TestTypedDiffRefusesUnusableAndBounds(t *testing.T) {
	text := "x"
	for name, blocks := range map[string][]permissionContent{
		"none":        nil,
		"wrong type":  {{Type: "text", Path: "/x", NewText: &text}},
		"no new text": {{Type: "diff", Path: "/x"}},
		"no path":     {{Type: "diff", NewText: &text}},
	} {
		if _, _, _, ok := typedDiff(blocks); ok {
			t.Fatalf("%s accepted", name)
		}
	}
	big := strings.Repeat("line of text\n", 5000)
	diff, _, truncated, ok := typedDiff([]permissionContent{{Type: "diff", Path: "/x", NewText: &big}})
	if !ok || !truncated || len(diff) > maxDiffBytes+4 {
		t.Fatalf("bound: ok=%v truncated=%v len=%d", ok, truncated, len(diff))
	}
	one := strings.Repeat("y", 1<<20)
	diff, _, truncated, _ = typedDiff([]permissionContent{{Type: "diff", Path: "/x", NewText: &one}})
	if !truncated || len(diff) > maxDiffBytes+4 {
		t.Fatalf("single line bound: %v %d", truncated, len(diff))
	}
}

func TestCloseConfirmsGroupRetired(t *testing.T) {
	opts, _ := options(t, "permission", testVersion)
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if session.GroupRetired() {
		t.Fatal("retired before Close")
	}
	session.Close()
	if !session.GroupRetired() {
		t.Fatal("Close did not confirm the group gone")
	}
}

func TestSubagentSummary(t *testing.T) {
	text, cut := subagentSummary("general-purpose", "Do the thing")
	if text != "[general-purpose] Do the thing" || cut {
		t.Fatalf("summary %q %v", text, cut)
	}
	if text, _ := subagentSummary("x", "  \n"); text != "" {
		t.Fatalf("blank prompt reviewable: %q", text)
	}
	text, cut = subagentSummary("", strings.Repeat("p", maxDiffBytes*2))
	if !cut || len(text) > maxDiffBytes {
		t.Fatalf("bound %v %d", cut, len(text))
	}
	var params permissionParams
	raw := `{"toolCall":{"kind":"think","title":"probe","rawInput":{"description":"probe","prompt":"Do nothing","subagent_type":"general-purpose"}}}`
	if err := json.Unmarshal([]byte(raw), &params); err != nil || params.ToolCall.RawInput.Prompt != "Do nothing" || params.ToolCall.RawInput.SubagentType != "general-purpose" {
		t.Fatalf("decode %+v %v", params, err)
	}
}

func TestSetModelReportsAppliedState(t *testing.T) {
	opts, _ := options(t, "permission", testVersion)
	s, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SetModel(context.Background(), "echo-list"); err != nil || s.Model() != "echo-list" {
		t.Fatalf("acknowledged model: %v %q", err, s.Model())
	}
	if err := s.SetModel(context.Background(), "silently-reset"); !errors.Is(err, ErrModel) || s.Model() != "default" {
		t.Fatalf("silent reset: %v %q", err, s.Model())
	}
}

func TestSubagentPermissionCarriesParentLink(t *testing.T) {
	// Shape captured from the nested subagent probe.
	const raw = `{"sessionId":"s","toolCall":{"toolCallId":"toolu_child","kind":"execute","title":"printf nested",
		"_meta":{"claudeCode":{"toolName":"Bash","parentToolUseId":"toolu_probe"}},"rawInput":{"command":"printf nested"}}}`
	var params permissionParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil || params.ToolCall.Meta.ClaudeCode.ParentToolUseID != "toolu_probe" {
		t.Fatalf("decode %+v %v", params, err)
	}
}

func TestPermissionTitle(t *testing.T) {
	for _, tc := range []struct{ title, parent, want string }{
		{"ls", "", "ls"},
		{"", "", "Claude Code tool call"},
		{"ls", "toolu_1", "Subagent: ls"},
	} {
		if got := permissionTitle(tc.title, tc.parent, "Claude Code"); got != tc.want {
			t.Fatalf("%+v -> %q", tc, got)
		}
	}
}

// An agent that ignores session/cancel must not leave the chat busy until the
// turn timeout after the person was told it stopped: after the grace period the
// chat is ended explicitly and the process group is confirmed gone.
func TestCancelIgnoredByTheAgentEndsTheChatAfterTheGracePeriod(t *testing.T) {
	opts, got := options(t, "ignore-cancel", testVersion)
	opts.TurnTimeout = time.Minute
	opts.ForceStopGrace = 300 * time.Millisecond
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), "start") }()
	for deadline := time.Now().Add(5 * time.Second); !session.Working(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the turn never started")
		}
	}
	if err := session.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("an ignored cancel left the chat running")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the turn never returned after the chat was ended")
	}
	// Done closes when the process exits; Close records the retirement just after.
	for deadline := time.Now().Add(10 * time.Second); !session.GroupRetired(); {
		if time.Now().After(deadline) {
			t.Fatal("the process group was not confirmed gone")
		}
		time.Sleep(20 * time.Millisecond)
	}
	notice := false
	for {
		select {
		case event := <-got.wait:
			if event.Kind == "notice" && strings.Contains(event.Text, "did not stop when asked") && event.Bad {
				notice = true
			}
			continue
		default:
		}
		break
	}
	if !notice {
		t.Fatal("the person was not told the chat was ended")
	}
}

// A cancel the agent honours must not end the chat, and the force-stop armed for
// that turn must not touch the next turn that is running when it fires.
func TestForceStopIsBoundToTheCancelledTurn(t *testing.T) {
	opts, got := options(t, "progress-after-silence-hang", testVersion)
	opts.TurnTimeout = time.Minute
	opts.ForceStopGrace = 300 * time.Millisecond
	opts.StillWorkingAfter = time.Minute
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	first := make(chan error, 1)
	go func() { first <- session.Prompt(context.Background(), "one") }()
	for event := got.next(t); event.Kind != "agent"; event = got.next(t) {
	}
	if err := session.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first:
	case <-time.After(5 * time.Second):
		t.Fatal("the honoured cancel did not end the first turn")
	}
	second := make(chan error, 1)
	go func() { second <- session.Prompt(context.Background(), "two") }()
	time.Sleep(900 * time.Millisecond) // well past the first turn's grace period
	select {
	case <-session.Done():
		t.Fatal("the first turn's force-stop ended the second turn")
	default:
	}
	if !session.Working() {
		t.Fatal("the second turn is not running")
	}
	_ = session.Cancel()
	select {
	case <-second:
	case <-time.After(5 * time.Second):
		t.Fatal("the second turn did not end")
	}
}

// Once the force-stop has committed to ending the chat, a turn that finishes a
// moment later must not be followed by a new one that the close would then kill.
func TestForceStopRefusesNewTurnsOnceCommitted(t *testing.T) {
	opts, got := options(t, "progress-after-silence-hang", testVersion)
	opts.TurnTimeout = time.Minute
	opts.ForceStopGrace = time.Minute // the timer never fires; the commit is driven by hand
	opts.StillWorkingAfter = time.Minute
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	first := make(chan error, 1)
	go func() { first <- session.Prompt(context.Background(), "one") }()
	for event := got.next(t); event.Kind != "agent"; event = got.next(t) {
	}
	session.turnMu.Lock()
	seq := session.turnSeq
	session.turnMu.Unlock()
	if !session.commitForceStop(seq) {
		t.Fatal("a running turn could not be committed to a force-stop")
	}
	// The agent now finishes the cancelled turn, before the close runs.
	if err := session.sendCancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn did not end")
	}
	if err := session.Prompt(context.Background(), "two"); !errors.Is(err, ErrClosed) {
		t.Fatalf("a new turn started after the force-stop committed: %v", err)
	}
	// A stale commit for an older turn is refused.
	if session.commitForceStop(seq - 1) {
		t.Fatal("a commit for another turn was accepted")
	}
}

// SC5 audit, gap 3: a permission request that arrives after the turn has ended
// belongs to no turn the person can see. It must be answered as cancelled and
// never raise an approval card or a "waiting" status on an idle chat.
func TestPermissionRequestAfterTheTurnEndedIsCancelled(t *testing.T) {
	opts, got := options(t, "late-permission", testVersion)
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Prompt(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	var answered string
	deadline := time.After(3 * time.Second)
wait:
	for {
		select {
		case event := <-got.wait:
			if event.Kind == "permission" {
				t.Fatalf("an approval card was raised for a turn that had ended: %+v", event)
			}
			if event.Kind == "status" && event.State == "waiting" {
				t.Fatal("an idle chat was shown as waiting for an approval")
			}
			if event.Kind == "agent" && strings.HasPrefix(event.Text, "late answer: ") {
				answered = event.Text
				break wait
			}
		case <-deadline:
			t.Fatal("the late permission request was never answered")
		}
	}
	if !strings.Contains(answered, "cancelled") {
		t.Fatalf("the agent was told %q, want a cancelled outcome", answered)
	}
}

// SC5 audit, gap 3: a permission request that arrives while the cancelled turn
// is still winding down must not raise a card either.
func TestPermissionRequestAfterCancelIsCancelled(t *testing.T) {
	opts, got := options(t, "permission-on-cancel", testVersion)
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), "go") }()
	for deadline := time.Now().Add(5 * time.Second); !session.Working(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the turn never started")
		}
	}
	if err := session.Cancel(); err != nil {
		t.Fatal(err)
	}
	answered := ""
	deadline := time.After(5 * time.Second)
wait:
	for {
		select {
		case event := <-got.wait:
			if event.Kind == "permission" {
				t.Fatalf("a card was raised for a cancelled turn: %+v", event)
			}
			if event.Kind == "agent" && strings.HasPrefix(event.Text, "late answer: ") {
				answered = event.Text
				break wait
			}
		case <-deadline:
			t.Fatal("the permission request sent during the cancel was never answered")
		}
	}
	if !strings.Contains(answered, "cancelled") {
		t.Fatalf("the agent was told %q, want a cancelled outcome", answered)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled turn did not end")
	}
}

// A profile with a StartMode moves a session that starts elsewhere into it, but
// only when the adapter's reply confirms the new mode. An ignored, unreported or
// refused switch refuses the chat, so a chat can never run in the wrong mode.
func TestStartModeIsConfirmedOrTheChatIsRefused(t *testing.T) {
	for scenario, wantStart := range map[string]bool{"mode-switch": true, "mode-ignored": false, "mode-silent": false, "mode-plan": false} {
		opts, _ := options(t, scenario, testVersion)
		opts.Agent.StartMode = "build"
		session, err := Start(context.Background(), opts)
		if (err == nil) != wantStart {
			t.Fatalf("%s: err %v, want start %v", scenario, err, wantStart)
		}
		if session != nil {
			session.Close()
		}
	}
	// Without a StartMode the old rule holds: a session in another mode is refused.
	opts, _ := options(t, "mode-switch", testVersion)
	if _, err := Start(context.Background(), opts); err == nil {
		t.Fatal("plan mode accepted without a StartMode")
	}
}

// For an adapter whose cancel leaves a command alive, Stop with a command running
// ends the chat, retires the process group and says so; Stop with nothing running
// only cancels the turn and leaves the chat usable.
func TestStopEndsTheChatOnlyWhileACommandRunsWhenTheProfileAsksFor(t *testing.T) {
	for _, tc := range []struct {
		scenario string
		flag     bool
		ended    bool
	}{{"running-command", true, true}, {"running-command", false, false}, {"permission-on-cancel", true, false}} {
		opts, got := options(t, tc.scenario, testVersion)
		opts.Agent.EndChatOnStopWhileToolRuns = tc.flag
		opts.TurnTimeout = time.Minute
		session, err := Start(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- session.Prompt(context.Background(), "go") }()
		for deadline := time.Now().Add(5 * time.Second); !session.Working(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal("the turn never started")
			}
		}
		if tc.scenario == "running-command" {
			for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
				session.mu.Lock()
				n := len(session.toolStart)
				session.mu.Unlock()
				if n > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("the command never started")
				}
			}
		}
		if err := session.Cancel(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: the turn never returned", tc.scenario)
		}
		notice := false
		for {
			select {
			case event := <-got.wait:
				if event.Kind == "notice" && strings.Contains(event.Text, "was running a command when you pressed Stop") && event.Bad {
					notice = true
				}
				continue
			default:
			}
			break
		}
		select {
		case <-session.Done():
			if !tc.ended {
				t.Fatalf("%s flag=%v: the chat was ended", tc.scenario, tc.flag)
			}
			if !session.GroupRetired() {
				t.Fatalf("%s: the process group was not confirmed gone", tc.scenario)
			}
		case <-time.After(300 * time.Millisecond):
			if tc.ended {
				t.Fatalf("%s: the chat was not ended", tc.scenario)
			}
		}
		if notice != tc.ended {
			t.Fatalf("%s flag=%v: notice=%v want %v", tc.scenario, tc.flag, notice, tc.ended)
		}
		session.Close()
	}
}

// A tool the replayed history left unfinished is marked interrupted when the chat
// is reopened; it must not count as a running command, or Stop in a later
// text-only turn would end the chat and claim a command was running.
func TestInterruptedReplayToolsDoNotMakeStopEndTheChat(t *testing.T) {
	opts, got := options(t, "ignore-cancel", testVersion)
	opts.Resume = "ses_fake000000000000000000000001"
	opts.Agent.EndChatOnStopWhileToolRuns = true
	opts.TurnTimeout = time.Minute
	opts.ForceStopGrace = time.Minute
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if !got.has("tool") {
		t.Fatal("the replay did not include the unfinished tool")
	}
	go func() { _ = session.Prompt(context.Background(), "just text") }()
	for deadline := time.Now().Add(5 * time.Second); !session.Working(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the turn never started")
		}
	}
	if err := session.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Done():
		t.Fatal("Stop ended the chat although no command was running")
	case <-time.After(400 * time.Millisecond):
	}
	for {
		select {
		case event := <-got.wait:
			if event.Kind == "notice" && strings.Contains(event.Text, "was running a command") {
				t.Fatal("the person was told a command was running")
			}
			continue
		default:
		}
		break
	}
}

// A live mode switch is limited to the profile's choosable modes, refused while a
// turn works, and reported as applied only when the agent's reply names the mode.
func TestSetModeIsLimitedAndConfirmed(t *testing.T) {
	opts, _ := options(t, "mode-live", testVersion)
	opts.Agent.RequireMode = ""
	opts.Agent.ChoosableModes = []string{"build", "plan"}
	session, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if got := session.Mode(); got != "build" {
		t.Fatalf("starting mode %q", got)
	}
	if err := session.SetMode(context.Background(), "plan"); err != nil {
		t.Fatalf("allowed switch: %v", err)
	}
	if got := session.Mode(); got != "plan" {
		t.Fatalf("mode after switch %q", got)
	}
	if err := session.SetMode(context.Background(), "full-access"); !errors.Is(err, ErrMode) {
		t.Fatalf("a mode outside the list: %v", err)
	}
	if got := session.Mode(); got != "plan" {
		t.Fatalf("a refused switch changed the mode to %q", got)
	}
	opts, _ = options(t, "mode-ignored", testVersion)
	opts.Agent.RequireMode = ""
	opts.Agent.ChoosableModes = []string{"build", "plan"}
	ignored, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer ignored.Close()
	if err := ignored.SetMode(context.Background(), "build"); !errors.Is(err, ErrMode) {
		t.Fatalf("an ignored switch was reported as applied: %v", err)
	}
	opts, _ = options(t, "ignore-cancel", testVersion)
	opts.Agent.ChoosableModes = []string{"build"}
	opts.TurnTimeout = time.Minute
	opts.ForceStopGrace = time.Minute
	busy, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	go func() { _ = busy.Prompt(context.Background(), "work") }()
	for deadline := time.Now().Add(5 * time.Second); !busy.Working(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the turn never started")
		}
	}
	if err := busy.SetMode(context.Background(), "build"); !errors.Is(err, ErrBusy) {
		t.Fatalf("a switch during a turn: %v", err)
	}
}
