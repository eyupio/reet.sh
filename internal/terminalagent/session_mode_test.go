//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	swapSession = "session-0123456789"
	swapACP     = "ses_fake000000000000000000000001"
)

// fakeShells stands in for the tmux manager: a registry of sessions that
// refuses a reused id the way Manager.Create does.
type fakeShells struct {
	mu        sync.Mutex
	sessions  map[string]terminalhost.ManagedSession
	creates   []terminalproto.Create
	ends      int
	createErr error
	// gate, when set, holds every Create until it is closed.
	gate chan struct{}
	// entered is signalled when a Create starts waiting on gate.
	entered chan struct{}
	// forgetGate, when set, holds Forget (after End) until it is closed.
	forgetGate    chan struct{}
	forgetEntered chan struct{}
}

func newFakeShells() *fakeShells {
	return &fakeShells{sessions: map[string]terminalhost.ManagedSession{}}
}

func (f *fakeShells) Create(_ context.Context, requestID, sessionID string, in terminalproto.Create) (terminalhost.ManagedSession, error) {
	if !terminalproto.ValidID(requestID) || in.Validate() != nil {
		return terminalhost.ManagedSession{}, terminalproto.ErrInvalid
	}
	f.mu.Lock()
	gate, entered := f.gate, f.entered
	f.mu.Unlock()
	if gate != nil {
		if entered != nil {
			entered <- struct{}{}
		}
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sessions[sessionID]; ok {
		return terminalhost.ManagedSession{}, terminalhost.ErrOperationConflict
	}
	f.creates = append(f.creates, in)
	if f.createErr != nil {
		return terminalhost.ManagedSession{}, f.createErr
	}
	s := terminalhost.ManagedSession{ID: sessionID, Title: in.Title, Tool: in.Tool, State: terminalproto.Running, CreatedAt: 1, UpdatedAt: 1, CreateRequestID: requestID}
	f.sessions[sessionID] = s
	return s, nil
}

func (f *fakeShells) End(_ context.Context, requestID, sessionID string) (terminalhost.ManagedSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[sessionID]
	if !ok {
		return s, terminalhost.ErrSessionMissing
	}
	f.ends++
	s.State = terminalproto.Ended
	f.sessions[sessionID] = s
	return s, nil
}

func (f *fakeShells) Forget(_ context.Context, sessionID string) error {
	f.mu.Lock()
	gate, entered := f.forgetGate, f.forgetEntered
	f.mu.Unlock()
	if gate != nil {
		if entered != nil {
			entered <- struct{}{}
		}
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[sessionID]
	if !ok {
		return terminalhost.ErrSessionMissing
	}
	if s.State.ReservesQuota() {
		return terminalhost.ErrOperationConflict
	}
	delete(f.sessions, sessionID)
	return nil
}

// PaneProcesses reports no processes: a fake terminal has none to wait for.
func (f *fakeShells) PaneProcesses(context.Context, string) ([]int, error) { return nil, nil }

func (f *fakeShells) List(context.Context) ([]terminalhost.ManagedSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]terminalhost.ManagedSession, 0, len(f.sessions))
	for _, s := range f.sessions {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeShells) createCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.creates)
}

func toTerminal(dir string) terminalproto.SessionMode {
	return terminalproto.SessionMode{To: terminalproto.SessionModeTerminal, Tool: "opencode", Dir: dir, Model: "anthropic/claude-sonnet-4", ACPSessionID: swapACP, Title: "Chat", Cols: 100, Rows: 30}
}

func toChat(dir string) terminalproto.SessionMode {
	return terminalproto.SessionMode{To: terminalproto.SessionModeChat, Tool: "opencode", Dir: dir, ACPSessionID: swapACP, Title: "Chat", Trust: true}
}

// openSwapChat opens the chat the swap tests move, and a switch over it.
func openSwapChat(t *testing.T, h *chatHost) (*agentSwitch, *fakeShells) {
	t.Helper()
	if _, err := h.open("request-0123456789", swapSession, terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home,
	}); err != nil {
		t.Fatal(err)
	}
	shells := newFakeShells()
	return &agentSwitch{chats: h, shells: shells}, shells
}

func (s *sent) sessionStates() []terminalproto.SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []terminalproto.SessionState
	for _, m := range s.msgs {
		if c, ok := m.Body.(*terminalproto.SessionChanged); ok {
			out = append(out, c.State)
		}
	}
	return out
}

func TestSwapChatToTerminalKeepsIDAndReleasesChat(t *testing.T) {
	h, proc, out := testChatHost(t)
	sw, shells := openSwapChat(t, h)
	changed, err := sw.swap(t.Context(), "request-swap-000001", swapSession, toTerminal(h.sup.opts.Home))
	if err != nil {
		t.Fatal(err)
	}
	if changed.State != terminalproto.Running {
		t.Fatalf("state = %s", changed.State)
	}
	select {
	case <-proc.closed:
	default:
		t.Fatal("the chat process was not closed")
	}
	if _, ok := h.snapshot(swapSession); ok {
		t.Fatal("the chat receipt was kept after the swap")
	}
	shells.mu.Lock()
	defer shells.mu.Unlock()
	if len(shells.creates) != 1 {
		t.Fatalf("creates = %d", len(shells.creates))
	}
	in := shells.creates[0]
	if in.LaunchSession != swapACP || in.Tool != "opencode" || in.Model != "anthropic/claude-sonnet-4" || in.Cols != 100 || in.Rows != 30 || in.Structured {
		t.Fatalf("terminal create = %+v", in)
	}
	if s, ok := shells.sessions[swapSession]; !ok || s.State != terminalproto.Running {
		t.Fatalf("terminal not running under the same id: %+v", shells.sessions)
	}
	for _, state := range out.sessionStates() {
		if !state.ReservesQuota() {
			t.Fatalf("the controller was told %s during the swap", state)
		}
	}
}

func TestSwapTerminalToChatResumesStoredSession(t *testing.T) {
	h, _, out := testChatHost(t)
	var resume string
	var trust bool
	start := h.sup.start
	h.sup.start = func(ctx context.Context, opts acpagent.Options) (chatProcess, error) {
		resume, trust = opts.Resume, opts.Trust
		return start(ctx, opts)
	}
	shells := newFakeShells()
	if _, err := shells.Create(t.Context(), "request-0123456789", swapSession, terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", LaunchSession: swapACP}); err != nil {
		t.Fatal(err)
	}
	sw := &agentSwitch{chats: h, shells: shells}
	changed, err := sw.swap(t.Context(), "request-swap-000001", swapSession, toChat(h.sup.opts.Home))
	if err != nil {
		t.Fatal(err)
	}
	if resume != swapACP || !trust {
		t.Fatalf("chat started with resume %q trust %v", resume, trust)
	}
	if changed.State != terminalproto.Running || changed.ACPSessionID != swapACP {
		t.Fatalf("reply = %+v", changed)
	}
	if sessions, _ := shells.List(t.Context()); len(sessions) != 0 {
		t.Fatalf("the terminal entry was kept: %+v", sessions)
	}
	if s, ok := h.snapshot(swapSession); !ok || s.State != terminalproto.Running {
		t.Fatalf("chat = %+v %v", s, ok)
	}
	for _, state := range out.sessionStates() {
		if !state.ReservesQuota() {
			t.Fatalf("the controller was told %s during the swap", state)
		}
	}
}

func TestSwapRefusedWhileTurnRunning(t *testing.T) {
	h, proc, _ := testChatHost(t)
	sw, shells := openSwapChat(t, h)
	if err := h.prompt(swapSession, "hello", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !proc.Working() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if h.idle(swapSession) {
		t.Fatal("a running turn reads as idle")
	}
	if _, err := sw.swap(t.Context(), "request-swap-000001", swapSession, toTerminal(h.sup.opts.Home)); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("err = %v", err)
	}
	if shells.createCount() != 0 {
		t.Fatal("a terminal was started")
	}
	if s, ok := h.snapshot(swapSession); !ok || s.State != terminalproto.Running {
		t.Fatal("the chat was disturbed")
	}
}

func TestSwapRefusedWithPendingApproval(t *testing.T) {
	h, proc, _ := testChatHost(t)
	sw, shells := openSwapChat(t, h)
	if !h.idle(swapSession) {
		t.Fatal("a fresh chat is not idle")
	}
	proc.onEvent(acpagent.Event{Kind: "permission", PermissionID: "permission-0123456789", Title: "run tests"})
	if h.idle(swapSession) {
		t.Fatal("a pending approval reads as idle")
	}
	if _, err := sw.swap(t.Context(), "request-swap-000001", swapSession, toTerminal(h.sup.opts.Home)); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("err = %v", err)
	}
	if shells.createCount() != 0 {
		t.Fatal("a terminal was started")
	}
}

func TestSwapRefusedWithPendingQuestion(t *testing.T) {
	h, proc, _ := questionHost(t, true)
	shells := newFakeShells()
	sw := &agentSwitch{chats: h, shells: shells}
	proc.onEvent(acpagent.Event{Kind: "question", QuestionID: connectorQuestionID, Question: sampleAgentQuestion()})
	if h.idle(swapSession) {
		t.Fatal("a pending question reads as idle")
	}
	if _, err := sw.swap(t.Context(), "request-swap-000001", swapSession, toTerminal(h.sup.opts.Home)); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("err = %v", err)
	}
	if shells.createCount() != 0 {
		t.Fatal("a terminal was started")
	}
	proc.onEvent(acpagent.Event{Kind: "question_resolved", QuestionID: connectorQuestionID, Reason: "accept"})
	if !h.idle(swapSession) {
		t.Fatal("a resolved question still blocks the swap")
	}
}

func TestSwapConcurrentSecondCallRefused(t *testing.T) {
	h, _, _ := testChatHost(t)
	sw, shells := openSwapChat(t, h)
	shells.gate = make(chan struct{})
	shells.entered = make(chan struct{}, 2)
	first := make(chan error, 1)
	go func() {
		_, err := sw.swap(context.Background(), "request-swap-000001", swapSession, toTerminal(h.sup.opts.Home))
		first <- err
	}()
	select {
	case <-shells.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first swap never started a terminal")
	}
	second := make(chan error, 1)
	go func() {
		_, err := sw.swap(context.Background(), "request-swap-000002", swapSession, toTerminal(h.sup.opts.Home))
		second <- err
	}()
	select {
	case err := <-second:
		if !errors.Is(err, terminalhost.ErrOperationConflict) {
			t.Fatalf("second swap: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second swap waited for the first")
	}
	close(shells.gate)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if n := shells.createCount(); n != 1 {
		t.Fatalf("processes started: %d", n)
	}
}

func TestSwapRestoresPreviousModeWhenNewStartFails(t *testing.T) {
	h, _, out := testChatHost(t)
	var resumes []string
	h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		resumes = append(resumes, opts.Resume)
		proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{}), onEvent: opts.OnEvent}
		return proc, nil
	}
	sw, shells := openSwapChat(t, h)
	shells.createErr = terminalhost.ErrNotRunning
	_, err := sw.swap(t.Context(), "request-swap-000001", swapSession, toTerminal(h.sup.opts.Home))
	if !errors.Is(err, terminalhost.ErrNotRunning) {
		t.Fatalf("err = %v, want the original start error", err)
	}
	if len(resumes) != 2 || resumes[1] != swapACP {
		t.Fatalf("chat starts = %q, want a restore of %s", resumes, swapACP)
	}
	if s, ok := h.snapshot(swapSession); !ok || s.State != terminalproto.Running {
		t.Fatalf("chat not restored: %+v %v", s, ok)
	}
	if sessions, _ := shells.List(t.Context()); len(sessions) != 0 {
		t.Fatalf("terminal left behind: %+v", sessions)
	}
	for _, state := range out.sessionStates() {
		if !state.ReservesQuota() {
			t.Fatalf("the controller was told %s during the swap", state)
		}
	}
	// With the restore failing as well the original error is still returned.
	h.sup.start = func(context.Context, acpagent.Options) (chatProcess, error) {
		return nil, errors.New("restore failed")
	}
	if _, err := sw.swap(t.Context(), "request-swap-000002", swapSession, toTerminal(h.sup.opts.Home)); !errors.Is(err, terminalhost.ErrNotRunning) {
		t.Fatalf("err = %v, want the original start error", err)
	}
}

func TestSwapDoesNotInvalidateReconcile(t *testing.T) {
	h, _, _ := testChatHost(t)
	sw, shells := openSwapChat(t, h)
	snapshot := func() terminalproto.Reconcile {
		sessions, _ := shells.List(t.Context())
		var shellSnaps []terminalproto.SessionSnapshot
		for _, s := range sessions {
			shellSnaps = append(shellSnaps, terminalproto.SessionSnapshot{ID: s.ID, Title: s.Title, State: s.State})
		}
		return reconcileSnapshot(shellSnaps, h.snapshots())
	}
	for i, in := range []terminalproto.SessionMode{toTerminal(h.sup.opts.Home), toChat(h.sup.opts.Home), toTerminal(h.sup.opts.Home)} {
		if _, err := sw.swap(t.Context(), "request-swap-00000"+string(rune('1'+i)), swapSession, in); err != nil {
			t.Fatalf("swap %d: %v", i, err)
		}
		rec := snapshot()
		msg := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: "request-reconcile-01", Body: &rec}
		if err := msg.Validate(); err != nil {
			t.Fatalf("after swap %d the reconcile is invalid: %v (%+v)", i, err, rec.Sessions)
		}
		if len(rec.Sessions) != 1 || rec.Sessions[0].ID != swapSession || rec.Sessions[0].State != terminalproto.Running {
			t.Fatalf("after swap %d: %+v", i, rec.Sessions)
		}
	}
}

func TestSwapNotNegotiatedIsErrInvalid(t *testing.T) {
	for _, tc := range []struct {
		name     string
		features string
		refused  bool
	}{
		{"none", "", true},
		{"chat only", terminalproto.ACPFeature, true},
		{"session mode", terminalproto.ACPFeature + "," + terminalproto.SessionModeFeature, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := make(replies, 4)
			headers := http.Header{}
			if tc.features != "" {
				headers.Set(terminalproto.FeatureHeader, tc.features)
			}
			handle, cleanup := Factory(terminalhost.ManagerOptions{})(t.Context(), http.DefaultClient, headers, 1, got.send)
			defer cleanup()
			body := toTerminal("/tmp")
			msg := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: "request-0123456789", SessionID: swapSession, Body: &body}
			if err := msg.Validate(); err != nil {
				t.Fatal("not a valid message", err)
			}
			err := handle(t.Context(), msg)
			if tc.refused != errors.Is(err, terminalproto.ErrInvalid) {
				t.Fatalf("features %q: err = %v, refused want %v", tc.features, err, tc.refused)
			}
		})
	}
}

// While a terminal is being swapped out, the session-state observer must not
// see it end: the controller row stays running.
func TestSwapHidesTerminalFromObserver(t *testing.T) {
	h, _, _ := testChatHost(t)
	shells := newFakeShells()
	if _, err := shells.Create(t.Context(), "request-0123456789", swapSession, terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", LaunchSession: swapACP}); err != nil {
		t.Fatal(err)
	}
	shells.forgetGate = make(chan struct{})
	shells.forgetEntered = make(chan struct{}, 1)
	sw := &agentSwitch{chats: h, shells: shells}
	done := make(chan error, 1)
	go func() {
		_, err := sw.swap(context.Background(), "request-swap-000001", swapSession, toChat(h.sup.opts.Home))
		done <- err
	}()
	// The terminal has ended and is not yet forgotten: an unfiltered listing
	// would report it ended.
	<-shells.forgetEntered
	if raw, _ := shells.List(t.Context()); len(raw) != 1 || raw[0].State != terminalproto.Ended {
		t.Fatalf("fixture: %+v", raw)
	}
	sessions, err := observedShells(t.Context(), shells, h.sup)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("observer saw the swapped terminal: %+v", sessions)
	}
	// A reconcile taken now still reports the row, once, as running.
	_, hidden, err := listShells(t.Context(), shells, h.sup)
	if err != nil {
		t.Fatal(err)
	}
	rec := reconcileSnapshot(nil, withSwaps(h.snapshots(), hidden))
	if len(rec.Sessions) != 1 || rec.Sessions[0].ID != swapSession || rec.Sessions[0].State != terminalproto.Running || rec.Sessions[0].Title != "Chat" {
		t.Fatalf("mid-swap reconcile: %+v", rec.Sessions)
	}
	close(shells.forgetGate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A switch to the chat without trust, in a folder with OpenCode plugins, is a
// refusal: the running terminal is not touched and no chat is started. The
// same request with trust proceeds.
func TestSwapToChatWithoutTrustLeavesTerminalUntouched(t *testing.T) {
	h, _, _ := testChatHost(t)
	dir := h.sup.opts.Home
	if err := os.MkdirAll(filepath.Join(dir, ".opencode", "plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	starts := 0
	start := h.sup.start
	h.sup.start = func(ctx context.Context, opts acpagent.Options) (chatProcess, error) {
		starts++
		return start(ctx, opts)
	}
	shells := newFakeShells()
	if _, err := shells.Create(t.Context(), "request-0123456789", swapSession, terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", LaunchSession: swapACP}); err != nil {
		t.Fatal(err)
	}
	sw := &agentSwitch{chats: h, shells: shells}
	in := toChat(dir)
	in.Trust = false
	_, err := sw.swap(t.Context(), "request-swap-000001", swapSession, in)
	var trust *acpagent.TrustError
	if !errors.As(err, &trust) || len(trust.Files) != 1 || trust.Files[0] != "plugin" {
		t.Fatalf("err = %v, want the chat start's trust refusal", err)
	}
	if got := chatResult(err); got.Code != terminalproto.ACPTrustRequired {
		t.Fatalf("answered %+v", got)
	}
	shells.mu.Lock()
	ends, creates := shells.ends, len(shells.creates)
	s := shells.sessions[swapSession]
	shells.mu.Unlock()
	if ends != 0 || creates != 1 || s.State != terminalproto.Running {
		t.Fatalf("the terminal was touched: ends %d creates %d state %s", ends, creates, s.State)
	}
	if starts != 0 {
		t.Fatalf("chat starts = %d", starts)
	}
	if _, ok := h.snapshot(swapSession); ok {
		t.Fatal("a chat was opened")
	}
	in.Trust = true
	if _, err := sw.swap(t.Context(), "request-swap-000002", swapSession, in); err != nil {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Fatalf("chat starts = %d", starts)
	}
	if s, ok := h.snapshot(swapSession); !ok || s.State != terminalproto.Running {
		t.Fatalf("chat = %+v %v", s, ok)
	}
}
