//go:build linux || darwin

package acpagent

import (
	"context"
	"encoding/json"
	"github.com/creack/pty"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestNativeViewOnlyOffersTheReviewedSharedService(t *testing.T) {
	for _, test := range []struct {
		tool, version string
		want          bool
	}{{"opencode", "1.19.5", true}, {"opencode", "2.0.0", false}, {"cline", "1.19.5", false}} {
		v, args := prepareNativeView(Options{Agent: Spec{Key: test.tool}, NativeTerminal: true}, test.version, "/local/opencode", "private-password")
		if (v != nil) != test.want {
			t.Fatalf("%s %s: unexpected support", test.tool, test.version)
		}
		if v != nil {
			defer v.cancel()
			if strings.Contains(strings.Join(args, " "), "private-password") || !strings.Contains(strings.Join(args, " "), "127.0.0.1") {
				t.Fatal("native listener was not private")
			}
		}
	}
}
func TestNativeMessagesDoNotEchoChatPromptsOrCrossSessions(t *testing.T) {
	var events []Event
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "opencode" || password != "secret" {
			http.Error(w, "denied", 401)
			return
		}
		role, sid, text := "user", "ses_native_test", "Sent in the native terminal"
		if strings.HasSuffix(r.URL.Path, "own") {
			text = "Sent in chat"
		}
		if strings.HasSuffix(r.URL.Path, "other") {
			sid = "ses_another_test"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"info": map[string]any{"id": r.URL.Path, "role": role, "sessionID": sid, "time": map[string]int64{"created": time.Now().UnixMilli()}}, "parts": []map[string]any{{"type": "text", "text": text}}})
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v := &nativeView{address: server.URL, password: "secret", ctx: ctx, client: server.Client(), own: map[uint64]nativeOwnPrompt{}, seen: map[string]bool{}}
	s := &Session{acpID: "ses_native_test", opts: Options{OnEvent: func(e Event) { events = append(events, e) }}, pending: map[string]*pendingPermission{}}
	v.expectUser("Sent in chat")
	v.user(s, "own")
	v.user(s, "external")
	v.user(s, "external")
	v.user(s, "other")
	if len(events) != 1 || events[0].Text != "Sent in the native terminal" {
		t.Fatalf("unexpected native transcript events: %#v", events)
	}
}

// This probe uses the exact reviewed binary and no model turn or credentials.
// It proves that the native UI attaches to the ACP server instead of replacing it.
func TestNativeTerminalKeepsTheAgentWorker(t *testing.T) {
	binary := os.Getenv("REET_OPENCODE_NATIVE")
	if binary == "" {
		t.Skip("set REET_OPENCODE_NATIVE to the reviewed OpenCode executable")
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".reet", "worktrees", "native", "repo", "0123456789abcdef0123456789abcdef")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var events []Event
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	session, err := Start(ctx, Options{Agent: OpenCode, Binary: binary, Dir: dir, Home: home, Trust: true, NativeTerminal: true, BaseEnv: []string{"PATH=" + os.Getenv("PATH")}, OnEvent: func(event Event) { mu.Lock(); events = append(events, event); mu.Unlock() }})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	native, ok := session.NativeTerminal()
	if !ok {
		t.Fatal("the reviewed agent did not expose its native UI")
	}
	originalPID := session.cmd.Process.Pid
	if native.Dir != dir || session.cmd.Dir != dir {
		t.Fatal("the views did not share the worker worktree")
	}
	cmd := exec.CommandContext(ctx, native.Binary, native.Args...)
	cmd.Dir = native.Dir
	cmd.Env = native.Env
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() }()
	output := make(chan bool, 1)
	go func() { buf := make([]byte, 16384); n, _ := terminal.Read(buf); output <- n > 0 }()
	select {
	case ok := <-output:
		if !ok {
			t.Fatal("native TUI produced no output")
		}
	case <-ctx.Done():
		t.Fatal("native TUI did not attach")
	}
	if session.cmd.Process.Pid != originalPID {
		t.Fatal("view attachment replaced the worker")
	}
	select {
	case <-session.Done():
		t.Fatal("view attachment stopped the agent")
	default:
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	// Closing the presentation must leave the private worker and conversation alive.
	if _, ok := session.NativeTerminal(); !ok {
		t.Fatal("closing the native client stopped its worker")
	}
	t.Log("Native TUI attached to the existing ACP worker; closing the UI left that worker running")
}
