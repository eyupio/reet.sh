//go:build linux || darwin

package acpagent

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// runClaudeHelper is the fake Claude Code adapter. It speaks the frames the slice 1 probe recorded from
// claude-agent-acp 0.99.1: a model catalogue, a thinking-level option that depends on the model, and a "Resource not
// found" refusal for a conversation it never stored.
//
//	claudecode-no-store      session/load answers -32002 Resource not found
//	claudecode-effort-stuck  a thinking-level change is answered with the old value (not confirmed)
//	claudecode-effort-dropped choosing another model drops the thinking level from the snapshot
//	claudecode-model-reset   an unknown model is accepted and silently reset to "default", as the adapter does
//	claudecode-image         the prompt is checked for a PNG image block
func runClaudeHelper(in *bufio.Scanner, out *bufio.Writer, scenario, version string) {
	send := func(v any) { raw, _ := json.Marshal(v); _, _ = out.Write(append(raw, '\n')); _ = out.Flush() }
	reply := func(id json.RawMessage, result any) {
		send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	refuse := func(id json.RawMessage, code int, text string) {
		send(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": text}})
	}
	native := "5d0a3b57-7c2e-4f58-9d1b-3b8a2c1e9f40"
	model, effort := "default", "default"
	listed := func(values []string, v string) bool {
		for _, x := range values {
			if x == v {
				return true
			}
		}
		return false
	}
	models := []string{"default", "sonnet", "opus"}
	efforts := []string{"default", "low", "medium", "high", "xhigh", "max"}
	choices := func(values []string) []map[string]string {
		out := make([]map[string]string, len(values))
		for i, v := range values {
			out[i] = map[string]string{"value": v}
		}
		return out
	}
	// Which options exist depends on the model: sonnet offers no "fast".
	snapshot := func() []map[string]any {
		options := []map[string]any{
			{"id": "mode", "category": "mode", "currentValue": "default", "options": choices([]string{"default", "acceptEdits", "plan"})},
			{"id": "model", "category": "model", "currentValue": model, "options": choices(models)},
		}
		dropped := scenario == "claudecode-effort-dropped" && model != "default"
		if !dropped {
			options = append(options, map[string]any{"id": "effort", "category": "thought_level", "currentValue": effort, "options": choices(efforts)})
		}
		if model != "sonnet" {
			options = append(options, map[string]any{"id": "fast", "category": "model_config", "currentValue": "off", "options": choices([]string{"on", "off"})})
		}
		return append(options, map[string]any{"id": "ultracode", "category": "model_config", "currentValue": "off", "options": choices([]string{"on", "off"})})
	}
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
			reply(message.ID, map[string]any{"protocolVersion": 1,
				"agentInfo":         map[string]string{"name": "@agentclientprotocol/claude-agent-acp", "version": version},
				"agentCapabilities": map[string]any{"loadSession": true, "promptCapabilities": map[string]bool{"image": true}}})
		case "session/new":
			reply(message.ID, map[string]any{"sessionId": native, "configOptions": snapshot()})
		case "session/load":
			var params struct {
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(message.Params, &params)
			if scenario == "claudecode-no-store" {
				refuse(message.ID, -32002, "Resource not found: "+params.SessionID)
				continue
			}
			native = params.SessionID
			reply(message.ID, map[string]any{"configOptions": snapshot()})
		case "session/set_config_option":
			var params struct {
				ConfigID string `json:"configId"`
				Value    string `json:"value"`
			}
			_ = json.Unmarshal(message.Params, &params)
			switch {
			case params.ConfigID == "model" && listed(models, params.Value):
				model = params.Value
			case params.ConfigID == "model" && scenario == "claudecode-model-reset":
				model = "default"
			case params.ConfigID == "effort" && listed(efforts, params.Value):
				if scenario != "claudecode-effort-stuck" {
					effort = params.Value
				}
			default:
				refuse(message.ID, -32602, "Invalid value for config option "+params.ConfigID)
				continue
			}
			reply(message.ID, map[string]any{"configOptions": snapshot()})
		case "session/prompt":
			var params struct {
				Prompt []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					MIMEType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"prompt"`
			}
			_ = json.Unmarshal(message.Params, &params)
			text := "no image"
			for _, block := range params.Prompt {
				data, err := base64.StdEncoding.DecodeString(block.Data)
				if block.Type == "image" && block.MIMEType == "image/png" && err == nil && bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
					text = "image ok"
				}
			}
			send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": native,
				"update": map[string]any{"sessionUpdate": "agent_message_chunk", "messageId": "msg_1", "content": map[string]string{"type": "text", "text": text}}}})
			reply(message.ID, map[string]any{"stopReason": "end_turn"})
		case "authenticate":
			// Reet never starts a login flow.
			return
		}
	}
}

func isClaudeHelperScenario(scenario string) bool { return strings.HasPrefix(scenario, "claudecode") }
