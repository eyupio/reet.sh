//go:build linux || darwin

package acpagent

import (
	"bufio"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// These shapes were measured from the official AGY 1.5.0 executable using a
// local Gemini stub. The command's capitalized CommandLine must reach a complete
// approval card; a title alone must never make an unreviewable command allowable.
func TestAntigravityChatWorkflow(t *testing.T) {
	for _, kind := range []string{"command", "write"} {
		for _, allow := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "-deny", true: "-allow"}[allow], func(t *testing.T) {
				opts, got := options(t, "antigravity-"+kind, "1.5.0")
				opts.Agent, opts.Trust = Antigravity, true
				opts.Model, opts.Effort = "gemini-4-argon", "medium"
				session, err := Start(t.Context(), opts)
				if err != nil {
					t.Fatal(err)
				}
				defer session.Close()
				if session.Mode() != "default" || session.Model() != opts.Model || !terminalproto.ValidACPSessionIDFor("antigravity", session.SessionID()) {
					t.Fatalf("session mode=%s model=%s id=%s", session.Mode(), session.Model(), session.SessionID())
				}
				controls := session.Controls()
				if len(controls.Options) != 2 || controls.Options[1].ID != "thought_level" || controls.Options[1].Current != "medium" {
					t.Fatalf("effort not confirmed: %+v", controls)
				}
				done := make(chan error, 1)
				go func() { done <- session.Prompt(t.Context(), "write the probe marker") }()
				var permission Event
				for permission.Kind != "permission" {
					permission = got.next(t)
				}
				if permission.Truncated {
					t.Fatalf("complete native card was truncated: %+v", permission)
				}
				if kind == "command" && permission.Command != "printf marker" {
					t.Fatalf("command: %+v", permission)
				}
				if kind == "write" && (permission.Path != "/project/marker.txt" || !strings.Contains(permission.Diff, "+marker")) {
					t.Fatalf("edit: %+v", permission)
				}
				if err := session.Allow(permission.PermissionID, allow); err != nil {
					t.Fatal(err)
				}
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestAntigravityRefusesUntrustedFoldersBeforeLaunching(t *testing.T) {
	opts, _ := options(t, "antigravity-command", "1.5.0")
	opts.Agent, opts.Binary = Antigravity, "/not-installed"
	var trust *TrustError
	if session, err := Start(t.Context(), opts); session != nil || !errors.As(err, &trust) {
		t.Fatalf("untrusted start: %v %v", session, err)
	}
}

func TestAntigravityDoesNotAnnounceUnconfirmedStarts(t *testing.T) {
	for _, scenario := range []string{"login", "stale-model", "mode-refused"} {
		t.Run(scenario, func(t *testing.T) {
			opts, _ := options(t, "antigravity-"+scenario, "1.5.0")
			opts.Agent, opts.Trust, opts.Model = Antigravity, true, "gemini-4-argon"
			ready := false
			opts.OnReady = func() { ready = true }
			session, err := Start(t.Context(), opts)
			if session != nil {
				session.Close()
				t.Fatal("refused start returned a session")
			}
			if err == nil || ready {
				t.Fatalf("unconfirmed start announced: %v ready=%v", err, ready)
			}
			if scenario == "login" && !errors.Is(err, ErrAntigravityLogin) {
				t.Fatalf("missing login: %v", err)
			}
			if scenario == "stale-model" && !errors.Is(err, ErrModel) {
				t.Fatalf("stale model: %v", err)
			}
		})
	}
}

func TestAntigravityDescriptorAndEnvironment(t *testing.T) {
	entry := Describe("antigravity", true, "1.5.0")
	if entry.State != "available" || !entry.Valid() || !entry.Model || !entry.Approval || !entry.History || !entry.Images || len(entry.Modes) != 0 {
		t.Fatalf("descriptor: %+v", entry)
	}
	if old := Describe("antigravity", true, "1.4.9"); old.State != "incompatible" {
		t.Fatalf("old: %+v", old)
	}
	env := "\n" + strings.Join(agentEnvironment(Antigravity, "/machine/home", []string{"PATH=/bin", "GEMINI_API_KEY=placeholder", "ANTIGRAVITY_PERM_GRANTS=all", "GOOGLE_GEMINI_BASE_URL=https://other.example", "OPENAI_API_KEY=other", "SSH_AUTH_SOCK=/socket"}, LaunchEnv{}), "\n") + "\n"
	if !strings.Contains(env, "\nHOME=/machine/home\n") || !strings.Contains(env, "\nGEMINI_API_KEY=placeholder\n") {
		t.Fatal("AGY's own login environment missing")
	}
	for _, key := range []string{"ANTIGRAVITY_PERM_GRANTS", "GOOGLE_GEMINI_BASE_URL", "OPENAI_API_KEY", "SSH_AUTH_SOCK"} {
		if strings.Contains(env, key+"=") {
			t.Fatalf("inherited %s", key)
		}
	}
}

// A deterministic ACP peer reproduces the measured AGY contract in ordinary CI.
// The optional real-binary test exercises the native engine against a local stub.
func runAntigravityHelper(in *bufio.Scanner, out *bufio.Writer, scenario, version string) {
	send := func(v any) { raw, _ := json.Marshal(v); _, _ = out.Write(append(raw, '\n')); _ = out.Flush() }
	reply := func(id json.RawMessage, result any) {
		send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	native := "123e4567-e89b-12d3-a456-426614174000"
	model, effort, mode := "gemini-3.1-pro", "low", "default"
	if scenario == "antigravity-mode-refused" {
		mode = "accept-edits"
	}
	settings := func() []map[string]any {
		return []map[string]any{
			{"id": "model", "category": "model", "currentValue": model, "options": []map[string]string{{"value": "gemini-3.1-pro"}, {"value": "gemini-4-argon"}}},
			{"id": "thought_level", "category": "thought_level", "currentValue": effort, "options": []map[string]string{{"value": "low"}, {"value": "medium"}, {"value": "high"}}},
			{"id": "mode", "category": "mode", "currentValue": mode},
		}
	}
	update := func(kind string, values map[string]any) {
		values["sessionUpdate"] = kind
		send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": native, "update": values}})
	}
	var prompt json.RawMessage
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
			reply(message.ID, map[string]any{"protocolVersion": 1, "agentInfo": map[string]string{"name": "Antigravity ACP Server", "version": version}, "agentCapabilities": map[string]any{"loadSession": true, "promptCapabilities": map[string]bool{"image": true}}})
		case "session/new", "session/load":
			if scenario == "antigravity-login" {
				send(map[string]any{"jsonrpc": "2.0", "id": message.ID, "error": map[string]any{"code": -32000, "message": "Authentication required"}})
				continue
			}
			reply(message.ID, map[string]any{"sessionId": native, "configOptions": settings()})
		case "session/set_config_option":
			var params struct {
				ConfigID string `json:"configId"`
				Value    string `json:"value"`
			}
			_ = json.Unmarshal(message.Params, &params)
			switch params.ConfigID {
			case "mode":
				if scenario != "antigravity-mode-refused" {
					mode = params.Value
				}
			case "model":
				if scenario != "antigravity-stale-model" {
					model = params.Value
				}
			case "thought_level":
				effort = params.Value
			}
			reply(message.ID, map[string]any{"configOptions": settings()})
		case "session/prompt":
			prompt = append(json.RawMessage(nil), message.ID...)
			update("agent_message_chunk", map[string]any{"content": map[string]string{"type": "text", "text": "hello from AGY"}})
			tool := map[string]any{"toolCallId": "agy-tool", "kind": "execute", "title": "Write marker", "rawInput": map[string]string{"CommandLine": "printf marker"}}
			if scenario == "antigravity-write" {
				tool["kind"] = "edit"
				tool["rawInput"] = map[string]string{"Path": "/project/marker.txt", "Content": "marker"}
				tool["content"] = []map[string]string{{"type": "diff", "path": "/project/marker.txt", "newText": "marker"}}
			}
			send(map[string]any{"jsonrpc": "2.0", "id": "agy-permission", "method": "session/request_permission", "params": map[string]any{"sessionId": native, "toolCall": tool, "options": []map[string]string{{"optionId": "allow", "kind": "allow_once"}, {"optionId": "deny", "kind": "reject_once"}}}})
		case "":
			if string(message.ID) == `"agy-permission"` {
				reply(prompt, map[string]string{"stopReason": "end_turn"})
			}
		case "session/cancel":
			if len(prompt) > 0 {
				reply(prompt, map[string]string{"stopReason": "cancelled"})
			}
		default:
			reply(message.ID, map[string]any{})
		}
	}
}
