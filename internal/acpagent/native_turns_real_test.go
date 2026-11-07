//go:build linux || darwin

package acpagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// Runs the real pinned agent and TUI against a deterministic loopback model.
// No account credentials or external provider calls are used.
func TestNativeRealBidirectionalTurns(t *testing.T) {
	binary := os.Getenv("REET_OPENCODE_NATIVE")
	if binary == "" {
		t.Skip("set REET_OPENCODE_NATIVE to the verified pinned executable")
	}
	var requestedMu sync.Mutex
	requestedModels := map[string]string{}
	var offeredQuestions atomic.Bool
	stopStarted := make(chan struct{}, 1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
			Stream   bool `json:"stream"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		for _, tool := range body.Tools {
			if tool.Function.Name == "question" {
				offeredQuestions.Store(true)
			}
		}
		answer := "fixture reply"
		origin := ""
		hasTool := false
		for _, m := range body.Messages {
			if m.Role == "tool" {
				hasTool = true
			}
			if m.Role == "user" {
				hasTool = false
				for _, marker := range []string{"native-permission-origin", "chat-permission-origin", "native-question-origin", "native-stop-origin"} {
					if strings.Contains(string(m.Content), marker) {
						origin = marker
						answer = marker + " reply"
					}
				}
				if strings.Contains(string(m.Content), "chat-origin") {
					answer = "chat-origin reply"
				}
				if strings.Contains(string(m.Content), "terminal-origin") {
					answer = "terminal-origin reply"
				}
			}
		}
		if !body.Stream {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": answer}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}})
			return
		}

		requestKey := origin
		if requestKey == "" {
			requestKey = strings.TrimSuffix(answer, " reply")
		}
		requestedMu.Lock()
		requestedModels[requestKey] = body.Model
		requestedMu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if origin == "native-stop-origin" {
			_, _ = io.WriteString(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"stop-started\"},\"finish_reason\":null}]}\n\n")
			w.(http.Flusher).Flush()
			select {
			case stopStarted <- struct{}{}:
			default:
			}
			<-r.Context().Done()
			return
		}
		if origin != "" && !hasTool {
			arguments, _ := json.Marshal(map[string]string{"command": "printf native-approval-fixture", "description": "Print the approval fixture marker"})
			tool := "bash"
			if strings.Contains(origin, "question") {
				tool = "question"
				arguments, _ = json.Marshal(map[string]any{"questions": []any{map[string]any{"header": "Fixture choice", "question": "Which fixture option should be used?", "options": []any{map[string]string{"label": "First", "description": "Fixture first choice"}, map[string]string{"label": "Second", "description": "Fixture second choice"}}}}})
			}
			chunk, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "model": "fixture", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": origin, "type": "function", "function": map[string]string{"name": tool, "arguments": string(arguments)}}}}, "finish_reason": nil}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
			_, _ = io.WriteString(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		for _, part := range []string{answer[:len(answer)/2], answer[len(answer)/2:]} {
			chunk, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "model": "fixture", "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"role": "assistant", "content": part}, "finish_reason": nil}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
			w.(http.Flusher).Flush()
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer provider.Close()
	spec := OpenCode
	spec.ConfigEnv = func(launch LaunchEnv) map[string]string {
		env := openCodeConfigEnv(launch)
		var config map[string]any
		_ = json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &config)
		config["provider"] = map[string]any{"native-fixture": map[string]any{"npm": "@ai-sdk/openai-compatible", "name": "Native fixture", "options": map[string]any{"baseURL": provider.URL + "/v1", "apiKey": "fixture-only"}, "models": map[string]any{"fixture": map[string]any{"name": "Fixture", "limit": map[string]int{"context": 32000, "output": 1024}}, "alternate": map[string]any{"name": "Alternate", "limit": map[string]int{"context": 32000, "output": 1024}}}}}
		raw, _ := json.Marshal(config)
		env["OPENCODE_CONFIG_CONTENT"] = string(raw)
		return env
	}
	var mu sync.Mutex
	var events []Event
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	session, err := Start(ctx, Options{Agent: spec, Binary: binary, Dir: t.TempDir(), Home: t.TempDir(), Trust: true, NativeTerminal: true, Model: "native-fixture/fixture", TurnTimeout: 15 * time.Second, BaseEnv: []string{"PATH=" + os.Getenv("PATH")}, OnEvent: func(e Event) { mu.Lock(); events = append(events, e); mu.Unlock() }})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Prompt(ctx, "chat-origin"); err != nil {
		t.Fatalf("chat turn: %v", err)
	}
	mu.Lock()
	var reply string
	for _, e := range events {
		if e.Kind == "agent" {
			reply += e.Text
		}
	}
	mu.Unlock()
	if !strings.Contains(reply, "chat-origin reply") {
		t.Fatalf("chat reply missing: %q", reply)
	}
	for deadline := time.Now().Add(5 * time.Second); session.Working() && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	if err := session.SetModel(ctx, "native-fixture/alternate"); err != nil {
		t.Fatal("chat model change", err)
	}
	native, ok := session.NativeTerminal()
	if !ok {
		t.Fatal("native view unavailable")
	}
	pid := session.cmd.Process.Pid
	cmd := exec.CommandContext(ctx, native.Binary, native.Args...)
	cmd.Dir = native.Dir
	cmd.Env = native.Env
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 100, Rows: 35})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = terminal.Close(); _ = cmd.Wait() }()
	var output strings.Builder
	go func() {
		buf := make([]byte, 16384)
		for {
			n, err := terminal.Read(buf)
			if n > 0 {
				mu.Lock()
				output.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	wait := func(label string, condition func() bool) {
		t.Helper()
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
			if condition() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		mu.Lock()
		raw := output.String()
		mu.Unlock()
		if len(raw) > 6000 {
			raw = raw[len(raw)-6000:]
		}
		t.Fatalf("%s: %q", label, raw)
	}
	wait("native history", func() bool { mu.Lock(); defer mu.Unlock(); return strings.Contains(output.String(), "chat-origin") })
	_, _ = terminal.Write([]byte("terminal-origin\r"))
	wait("native prompt mirrored into chat", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, e := range events {
			if e.Kind == "user" && e.Text == "terminal-origin" {
				return true
			}
		}
		return false
	})
	wait("native reply streamed into chat", func() bool {
		mu.Lock()
		defer mu.Unlock()
		var text string
		for _, e := range events {
			if e.Kind == "agent" {
				text += e.Text
			}
		}
		return strings.Contains(text, "terminal-origin reply")
	})
	if session.cmd.Process.Pid != pid {
		t.Fatal("turn changed the worker")
	}

	requestedMu.Lock()
	nativeModel := requestedModels["terminal-origin"]
	chatModel := requestedModels["chat-origin"]
	requestedMu.Unlock()
	if nativeModel != "fixture" || chatModel != "fixture" || session.Model() != "native-fixture/alternate" {
		t.Fatalf("view selection changed model scope: chat=%s native=%s current=%s", chatModel, nativeModel, session.Model())
	}
	wait("native turn idle", func() bool { return !session.Working() })
	mu.Lock()
	since := len(events)
	mu.Unlock()
	_, _ = terminal.Write([]byte("native-permission-origin\r"))
	permission := func() string {
		mu.Lock()
		defer mu.Unlock()
		for _, e := range events[since:] {
			if e.Kind == "permission" {
				return e.PermissionID
			}
		}
		return ""
	}
	wait("native permission mirrored into chat", func() bool { return permission() != "" })
	first := permission()
	if err := session.Allow(first, true); err != nil {
		t.Fatal(err)
	}
	wait("native permission resolved", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, e := range events[since:] {
			if e.Kind == "permission_resolved" && e.PermissionID == first && e.Allow != nil && *e.Allow {
				return true
			}
		}
		return false
	})
	if err := session.Allow(first, false); !errors.Is(err, ErrNoPermission) {
		t.Fatalf("late reply accepted: %v", err)
	}
	wait("native permission turn idle", func() bool { return !session.Working() })
	mu.Lock()
	since = len(events)
	output.Reset()
	mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- session.Prompt(ctx, "chat-permission-origin") }()
	wait("chat permission", func() bool { return permission() != "" })
	second := permission()
	wait("native permission dialog", func() bool {
		mu.Lock()
		defer mu.Unlock()
		raw := output.String()
		return strings.LastIndex(raw, "Allow once") > strings.LastIndex(raw, "chat-permission-origin") && strings.Contains(raw, "chat-permission-origin")
	})
	_, _ = terminal.Write([]byte("\r"))
	wait("terminal answer retires chat permission", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, e := range events[since:] {
			if e.Kind == "permission_resolved" && e.PermissionID == second && e.Allow != nil && *e.Allow {
				return true
			}
		}
		return false
	})
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("chat permission turn did not complete")
	}
	if err := session.Allow(second, false); !errors.Is(err, ErrNoPermission) {
		t.Fatalf("late terminal reply accepted: %v", err)
	}
	requestedMu.Lock()
	nextChatModel := requestedModels["chat-permission-origin"]
	requestedMu.Unlock()
	if nextChatModel != "alternate" {
		t.Fatalf("native turn overwrote Chat's selected model: %s", nextChatModel)
	}
	if session.cmd.Process.Pid != pid {
		t.Fatal("approval changed the worker")
	}

	wait("approval turn idle", func() bool { return !session.Working() })
	mu.Lock()
	since = len(events)
	mu.Unlock()
	_, _ = terminal.Write([]byte("native-question-origin\r"))
	wait("unsupported question tool is refused without hanging", func() bool {
		mu.Lock()
		defer mu.Unlock()
		var text string
		for _, e := range events[since:] {
			if e.Kind == "agent" {
				text += e.Text
			}
		}
		return strings.Contains(text, "native-question-origin reply")
	})
	if offeredQuestions.Load() {
		t.Fatal("reviewed ACP adapter unexpectedly offered typed questions")
	}
	wait("refused question turn idle", func() bool { return !session.Working() })
	_, _ = terminal.Write([]byte("native-stop-origin\r"))
	select {
	case <-stopStarted:
	case <-ctx.Done():
		t.Fatal("native stop turn did not start")
	}
	wait("native stop reports working", session.Working)
	if err := session.Prompt(ctx, "must-not-overlap-native-turn"); !errors.Is(err, ErrBusy) {
		t.Fatalf("concurrent chat turn accepted: %v", err)
	}
	if err := session.Cancel(); err != nil {
		t.Fatal(err)
	}
	wait("native turn stopped", func() bool { return !session.Working() })
	select {
	case <-session.Done():
		t.Fatal("native cancellation killed the worker")
	default:
	}
	if session.cmd.Process.Pid != pid {
		t.Fatal("stop changed the worker")
	}
	t.Log("Real ACP and native TUI exchanged both turns through one worker using a loopback model fixture")
}
