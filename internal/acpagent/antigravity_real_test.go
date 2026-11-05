//go:build linux || darwin

package acpagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestRealAntigravityGaffer runs the official native executable against a local
// Gemini-compatible stub. It uses a synthetic key and isolated home, and makes
// no real provider request. Ordinary CI has no AGY installation and skips it.
func TestRealAntigravityGaffer(t *testing.T) {
	binary := os.Getenv("REET_AGY_150")
	if binary == "" {
		t.Skip("set REET_AGY_150 to the checksum-verified AGY 1.5.0 executable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	home, dir := t.TempDir(), t.TempDir()
	if strings.HasPrefix(dir, "/tmp/") {
		t.Skip("AGY permits temporary-file writes: set TMPDIR to an isolated workspace directory to test edit approval")
	}
	marker := filepath.Join(dir, "marker.txt")
	var action atomic.Value
	action.Store("text")
	var sawImage atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools    []json.RawMessage `json:"tools"`
			Contents []struct {
				Parts []map[string]json.RawMessage `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&request); err != nil {
			http.Error(w, "invalid stub request", 400)
			return
		}
		hasResult := false
		for i, content := range request.Contents {
			for _, part := range content.Parts {
				if i == len(request.Contents)-1 && part["functionResponse"] != nil {
					hasResult = true
				}
				if part["inlineData"] != nil {
					sawImage.Store(true)
				}
			}
		}
		parts := []map[string]any{{"text": "reet-probe-ok"}}
		kind := action.Load().(string)
		target := marker + "." + kind
		if len(request.Tools) > 0 && !hasResult && kind != "text" {
			name := "run_command"
			command := "printf reet-probe > '" + strings.ReplaceAll(target, "'", "'\\''") + "'"
			if kind == "long-command" {
				command = "sleep 30; " + command
			}
			args := map[string]any{"CommandLine": command, "WaitMsBeforeAsync": 500, "toolSummary": "Write probe marker"}
			if kind == "write" {
				name = "write_file"
				args = map[string]any{"Path": target, "Content": "reet-probe"}
			}
			parts = []map[string]any{{"functionCall": map[string]any{"name": name, "args": args}}}
		}
		response := map[string]any{"candidates": []map[string]any{{"content": map[string]any{"role": "model", "parts": parts}, "finishReason": "STOP"}}, "usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2}}
		w.Header().Set("Content-Type", "text/event-stream")
		payload, _ := json.Marshal(response)
		_, _ = w.Write(append(append([]byte("data: "), payload...), '\n', '\n'))
	}))
	defer server.Close()
	// Only this test profile accepts a provider URL override. Production AGY
	// does not inherit one that could redirect the person's credential.
	spec := Antigravity
	spec.EnvAllowlist = append(append([]string(nil), spec.EnvAllowlist...), "GOOGLE_GEMINI_BASE_URL")
	base := []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8", "GEMINI_API_KEY=reet-local-probe-placeholder-only", "GOOGLE_GEMINI_BASE_URL=" + server.URL}
	opts := Options{Agent: spec, Binary: binary, Dir: dir, Home: home, Trust: true, BaseEnv: base, TurnTimeout: 10 * time.Second}
	if session, err := Start(ctx, opts); session != nil || !errors.Is(err, ErrAntigravityLogin) {
		if session != nil {
			session.Close()
		}
		t.Fatalf("missing login: %v", err)
	}
	bootstrapAntigravityTestLogin(t, ctx, binary, agentEnvironment(spec, home, base, LaunchEnv{}), dir)
	got := &events{wait: make(chan Event, 128)}
	opts.OnEvent, opts.Model, opts.Effort = got.add, "gemini-4-argon", "medium"
	session, err := Start(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.Model() != opts.Model || session.Mode() != "default" {
		t.Fatalf("startup model=%s mode=%s", session.Model(), session.Mode())
	}
	if err := session.Prompt(ctx, "Reply with reet-probe-ok."); err != nil {
		t.Fatal(err)
	}
	if !got.text("reet-probe-ok") {
		t.Fatal("no streamed model text")
	}
	for _, kind := range []string{"command", "write"} {
		target := marker + "." + kind
		for _, allow := range []bool{false, true} {
			t.Logf("native %s allow=%v", kind, allow)
			action.Store(kind)
			_ = os.Remove(target)
			done := make(chan error, 1)
			go func() { done <- session.Prompt(ctx, "Write the probe marker.") }()
			var permission Event
			for permission.Kind != "permission" {
				select {
				case permission = <-got.wait:
				case err := <-done:
					got.mu.Lock()
					all := append([]Event(nil), got.all...)
					got.mu.Unlock()
					t.Fatalf("native %s allow=%v ended without an approval: %v; events=%+v", kind, allow, err, all)
				case <-ctx.Done():
					t.Fatal("native approval timed out")
				}
			}
			if permission.Truncated || (kind == "command" && permission.Command == "") || (kind == "write" && permission.Diff == "") {
				t.Fatalf("unreviewable native card: %+v", permission)
			}
			if err := session.Allow(permission.PermissionID, allow); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(target)
			if allow && (err != nil || string(data) != "reet-probe") {
				t.Fatalf("allow %s: %q %v", kind, data, err)
			}
			if !allow && !os.IsNotExist(err) {
				t.Fatalf("deny %s wrote marker: %q %v", kind, data, err)
			}
		}
	}
	action.Store("text")
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if err := session.PromptWithAttachments(ctx, "Describe this image.", []Attachment{{MIME: "image/png", Data: picture.Bytes()}}); err != nil {
		t.Fatal(err)
	}
	if !sawImage.Load() {
		t.Fatal("native image prompt did not reach the local provider")
	}
	native := session.SessionID()
	session.Close()
	replayed := &events{wait: make(chan Event, 128)}
	opts.Resume, opts.OnEvent = native, replayed.add
	resumed, err := Start(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if resumed.SessionID() != native || !replayed.text("reet-probe-ok") {
		t.Fatal("session/load did not replay the conversation after restart")
	}
	// Stop during an approved command retires the whole native process group.
	action.Store("long-command")
	_ = os.Remove(marker + ".long-command")
	done := make(chan error, 1)
	go func() { done <- resumed.Prompt(ctx, "Run the long probe command.") }()
	var permission Event
	for permission.Kind != "permission" {
		permission = replayed.next(t)
	}
	if err := resumed.Allow(permission.PermissionID, true); err != nil {
		t.Fatal(err)
	}
	if err := resumed.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("cancel did not retire the turn")
	}
	if !resumed.GroupRetired() {
		t.Fatal("Stop did not retire AGY's process group")
	}
	if _, err := os.Stat(marker + ".long-command"); !os.IsNotExist(err) {
		t.Fatalf("stopped command wrote marker: %v", err)
	}
}

// The test setup performs the same isolated API-key selection as the raw ACP
// compatibility probe. Production reet never authenticates or changes accounts.
func bootstrapAntigravityTestLogin(t *testing.T, ctx context.Context, binary string, env []string, dir string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, binary, "acp", "--disable-telemetry")
	cmd.Env, cmd.Dir, cmd.SysProcAttr = env, dir, procAttr()
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), maxLineBytes)
	for i, method := range []string{"initialize", "authenticate"} {
		params := map[string]any{"protocolVersion": 1, "clientInfo": map[string]string{"name": "reet-test", "version": "1"}, "clientCapabilities": map[string]any{}}
		if method == "authenticate" {
			params = map[string]any{"methodId": "gemini-api-key"}
		}
		if err := json.NewEncoder(stdin).Encode(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		matched := false
		for scanner.Scan() {
			var reply struct {
				ID    int             `json:"id"`
				Error json.RawMessage `json:"error"`
			}
			if json.Unmarshal(scanner.Bytes(), &reply) == nil && reply.ID == i+1 {
				if len(reply.Error) > 0 {
					t.Fatalf("isolated auth setup: %s", reply.Error)
				}
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("no reply to isolated %s setup", method)
		}
	}
}
