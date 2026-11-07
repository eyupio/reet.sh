//go:build linux || darwin

package acpagent

import (
	"context"
	"encoding/json"
	"github.com/creack/pty"
	"io"
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

func TestNativePromptLedgerBoundsAndFailureCleanup(t *testing.T) {
	v := &nativeView{own: map[uint64]nativeOwnPrompt{}}
	failed := v.expectUser("rejected prompt")
	failed(context.Canceled)
	if len(v.own) != 0 {
		t.Fatal("failed request retained an echo expectation")
	}
	for i := 0; i < nativePromptLimit*2; i++ {
		v.expectUser("no native stream")
	}
	if len(v.own) != nativePromptLimit {
		t.Fatalf("unbounded ledger: %d", len(v.own))
	}
	v.mu.Lock()
	for id, p := range v.own {
		p.at = time.Now().Add(-nativePromptMaxAge - time.Second).UnixMilli()
		v.own[id] = p
	}
	v.mu.Unlock()
	v.expectUser("new prompt")
	if len(v.own) != 1 {
		t.Fatal("expired expectations retained without incoming events")
	}
}

func TestNativeReconnectReconcilesMissedIdleAndPreservesStateOnFailure(t *testing.T) {
	var status = `{"ses_native_test":{"type":"busy"}}`
	var refuse bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if refuse {
			http.Error(w, "offline", 503)
			return
		}
		if r.URL.Path == "/session/status" {
			_, _ = w.Write([]byte(status))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v := &nativeView{address: server.URL, ctx: ctx, client: server.Client()}
	s := &Session{acpID: "ses_native_test"}
	if err := v.reconcile(s); err != nil || !s.Working() {
		t.Fatalf("busy snapshot: %v", err)
	}
	refuse = true
	if err := v.reconcile(s); err == nil || !s.Working() {
		t.Fatal("failed snapshot invented an idle session")
	}
	refuse = false
	status = `{}`
	if err := v.reconcile(s); err != nil || s.Working() {
		t.Fatalf("missed idle was not recovered: %v", err)
	}
	status = `null`
	if err := v.reconcile(s); err == nil {
		t.Fatal("missing status map accepted as idle")
	}
	status = `{"ses_native_test":{"type":"unexpected"}}`
	if err := v.reconcile(s); err == nil {
		t.Fatal("unknown status accepted")
	}
}

func TestNativeObserverWithdrawsAvailabilityUntilGapIsReconciled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	snapshot := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case "/session/status":
			close(snapshot)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()
	defer cancel() // Release the stream before waiting for the test server.
	v := &nativeView{address: server.URL, ctx: ctx, client: server.Client()}
	v.ready.Store(true)
	s := &Session{acpID: "ses_native_test", native: v, nativeBusy: true}
	done := make(chan struct{})
	go func() {
		defer close(done)
		v.observe(s, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))})
	}()
	select {
	case <-snapshot:
	case <-time.After(5 * time.Second):
		t.Fatal("observer did not reconnect")
	}
	if _, ok := s.NativeTerminal(); ok {
		t.Fatal("native view advertised before reconciliation")
	}
	if !s.Working() {
		t.Fatal("gap falsely cleared busy state")
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for !v.ready.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !v.ready.Load() || s.Working() {
		t.Fatal("fresh idle snapshot did not restore availability and state")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("observer did not stop with its worker context")
	}
	if v.ready.Load() {
		t.Fatal("stopped observer remained available")
	}
}

func TestNativePermissionRepliesAreSingleUseAndRetired(t *testing.T) {
	v := &nativeView{}
	if tool := v.permissionReplied("unknown", "once"); tool != "" {
		t.Fatal("unknown native request supplied consent")
	}
	v.permissionAsked("request-1", "tool-1")
	if tool := v.permissionReplied("request-1", "once"); tool != "tool-1" {
		t.Fatal("early native answer lost")
	}
	if got := v.takePermissionReply("tool-1"); got != "once" {
		t.Fatal("native answer unavailable")
	}
	if got := v.takePermissionReply("tool-1"); got != "" {
		t.Fatal("native answer consumed twice")
	}
	v.retirePermission("tool-1")
	v.permissionAsked("replayed-request", "tool-1")
	if tool := v.permissionReplied("replayed-request", "once"); tool != "" {
		t.Fatal("retired permission reused")
	}
	v.permissionAsked("request-2", "tool-2")
	v.retirePermission("tool-2") // Chat answered before the native replied event arrived.
	if tool := v.permissionReplied("request-2", "once"); tool != "" {
		t.Fatal("late reply cached after chat resolution")
	}
	if got := v.takePermissionReply("tool-2"); got != "" {
		t.Fatal("late consent escaped request ownership")
	}
}

func TestNativePermissionMustHaveALiveOwner(t *testing.T) {
	var pending = `[]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(pending)) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v := &nativeView{address: server.URL, ctx: ctx, client: server.Client()}
	s := &Session{acpID: "ses_native_test"}
	v.confirmPermissionTurn(s, "late-tool")
	if s.Working() {
		t.Fatal("late permission resurrected an idle turn")
	}
	pending = `[{"sessionID":"ses_other","tool":{"callID":"live-tool"}}]`
	v.confirmPermissionTurn(s, "live-tool")
	if s.Working() {
		t.Fatal("another session supplied permission ownership")
	}
	pending = `[{"sessionID":"ses_native_test","tool":{"callID":"live-tool"}}]`
	v.confirmPermissionTurn(s, "live-tool")
	if !s.Working() {
		t.Fatal("native busy-event race was refused despite the service's live request")
	}
	s.nativeStatus(false)
	s.turnMu.Lock()
	s.nativeCancelled = true
	s.turnMu.Unlock()
	v.confirmPermissionTurn(s, "live-tool")
	if s.Working() {
		t.Fatal("permission resurrected a cancelled turn")
	}
}
