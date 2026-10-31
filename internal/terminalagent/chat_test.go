//go:build linux || darwin

package terminalagent

import (
	"bytes"
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

type fakeChat struct {
	mu        sync.Mutex
	onEvent   func(acpagent.Event)
	done      chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
	allow     []bool
	prompted  string
	images    []acpagent.Attachment
	busy      bool
	cancelled bool
}

func (f *fakeChat) Prompt(ctx context.Context, text string) error {
	return f.PromptWithAttachments(ctx, text, nil)
}

func (f *fakeChat) PromptWithAttachments(ctx context.Context, text string, images []acpagent.Attachment) error {
	f.mu.Lock()
	f.prompted = text
	f.images = images
	f.busy = true
	f.mu.Unlock()
	select {
	case <-ctx.Done():
	case <-f.closed:
	}
	f.mu.Lock()
	f.busy = false
	f.mu.Unlock()
	return ctx.Err()
}

func (f *fakeChat) ImagesSupported() bool { return true }
func (f *fakeChat) Cancel() error {
	f.mu.Lock()
	f.cancelled = true
	f.mu.Unlock()
	return nil
}
func (f *fakeChat) Allow(id string, allow bool) error {
	f.mu.Lock()
	f.allow = append(f.allow, allow)
	onEvent := f.onEvent
	f.mu.Unlock()
	if onEvent != nil {
		answer := allow
		onEvent(acpagent.Event{Kind: "permission_resolved", PermissionID: id, Allow: &answer})
	}
	return nil
}
func (f *fakeChat) SessionID() string { return "ses_fake000000000000000000000001" }
func (f *fakeChat) Working() bool     { f.mu.Lock(); defer f.mu.Unlock(); return f.busy }
func (f *fakeChat) Close() {
	f.closeOnce.Do(func() {
		close(f.closed)
		f.done <- struct{}{}
	})
}
func (f *fakeChat) Done() <-chan struct{} { return f.done }

type sent struct {
	mu   sync.Mutex
	msgs []terminalproto.Message
}

func (s *sent) send(_ context.Context, m terminalproto.Message) error {
	s.mu.Lock()
	s.msgs = append(s.msgs, m)
	s.mu.Unlock()
	return nil
}
func (s *sent) hasSession(state terminalproto.SessionState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.msgs {
		if c, ok := m.Body.(*terminalproto.SessionChanged); ok && c.State == state {
			return true
		}
	}
	return false
}

func testChatHost(t *testing.T) (*chatHost, *fakeChat, *sent) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	home := t.TempDir()
	dir := t.TempDir()
	out := &sent{}
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: home})
	h := sup.attach(ctx, 1, out.send, true, true)
	h.sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		proc.onEvent = opts.OnEvent
		return proc, nil
	}
	_ = dir
	return h, proc, out
}

func TestChatHostYoloAllowsOnce(t *testing.T) {
	h, proc, out := testChatHost(t)
	h.sup.opts.YoloConsent = func() error { return nil }
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Yolo: true, Dir: h.sup.opts.Home,
	}); err != nil {
		t.Fatal(err)
	}
	proc.onEvent(acpagent.Event{Kind: "permission", PermissionID: "permission-0123456789", Title: "run tests"})
	proc.mu.Lock()
	defer proc.mu.Unlock()
	if len(proc.allow) != 1 || !proc.allow[0] {
		t.Fatalf("allow: %v", proc.allow)
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	var permission, resolved bool
	for _, m := range out.msgs {
		if e, ok := m.Body.(*terminalproto.ACPEvent); ok {
			if e.Kind == "permission" {
				permission = true
			}
			if e.Kind == "permission_resolved" && e.Allow != nil && *e.Allow {
				resolved = true
			}
		}
	}
	if !permission || !resolved {
		t.Fatalf("events: permission=%v resolved=%v", permission, resolved)
	}
}

func TestChatHostYoloRefusedWithoutMachineConsent(t *testing.T) {
	h, _, _ := testChatHost(t)
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Yolo: true, Dir: h.sup.opts.Home,
	}); !errors.Is(err, terminalhost.ErrYoloRefused) {
		t.Fatalf("refused: %v", err)
	}
	h.sup.opts.YoloConsent = func() error { return errors.New("off") }
	if _, err := h.open("request-9876543210", "session-9876543210", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Yolo: true, Dir: h.sup.opts.Home,
	}); !errors.Is(err, terminalhost.ErrYoloRefused) {
		t.Fatalf("refused off: %v", err)
	}
}

func TestChatHostCreatePromptPermissionEnd(t *testing.T) {
	h, proc, out := testChatHost(t)
	changed, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home,
	})
	if err != nil || changed.State != terminalproto.Running {
		t.Fatalf("open: %+v %v", changed, err)
	}
	if snap, ok := h.snapshot("session-0123456789"); !ok || snap.State != terminalproto.Running {
		t.Fatalf("snapshot: %+v %v", snap, ok)
	}
	if err := h.prompt("session-0123456789", "hello", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		proc.mu.Lock()
		prompted := proc.prompted
		proc.mu.Unlock()
		if prompted == "hello" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("prompted %q", prompted)
		}
		time.Sleep(time.Millisecond)
	}
	if err := h.prompt("session-0123456789", "again", nil); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("busy prompt: %v", err)
	}
	proc.onEvent(acpagent.Event{Kind: "permission", PermissionID: "permission-0123456789", Title: "write", ToolKind: "edit", Path: "/home/u/a", Diff: "@@ -0,0 +1 @@\n+x"})
	if err := h.permission("session-0123456789", "permission-0123456789", true); err != nil {
		t.Fatal(err)
	}
	proc.mu.Lock()
	if len(proc.allow) != 1 || !proc.allow[0] {
		t.Fatalf("allow %v", proc.allow)
	}
	proc.mu.Unlock()
	if _, err := h.end("session-0123456789"); err != nil {
		t.Fatal(err)
	}
	if snap, ok := h.snapshot("session-0123456789"); !ok || snap.State != terminalproto.Exited {
		t.Fatalf("ended snapshot %+v %v", snap, ok)
	}
	if !out.hasSession(terminalproto.Exited) {
		out.mu.Lock()
		t.Fatalf("end was not reported: %+v", out.msgs)
	}
}

func TestChatHostTrustAndVersionErrors(t *testing.T) {
	for name, err := range map[string]error{
		"trust":   &acpagent.TrustError{Files: []string{"plugin.ts"}},
		"version": acpagent.ErrVersion,
	} {
		r := chatResult(err)
		if r.Code == "" {
			t.Fatalf("%s: no code", name)
		}
	}
}

func TestChatHostEndKeepsItsReceipt(t *testing.T) {
	h, _, _ := testChatHost(t)
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.end("session-0123456789"); err != nil {
		t.Fatal(err)
	}
	changed, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home})
	if err != nil || changed.State != terminalproto.Exited {
		t.Fatalf("replay: %+v %v", changed, err)
	}
}

func TestWireEventTrimsAndValidates(t *testing.T) {
	event := acpagent.Event{Kind: "permission", PermissionID: "permission-0123456789", Title: "write", Diff: string(make([]byte, 12000))}
	wire, ok := wireEvent(event)
	if !ok || !wire.Truncated || wire.Kind != "permission" || wire.Diff != "" {
		t.Fatalf("oversize card: %+v %v", wire, ok)
	}
	event.Diff = "x"
	wire, ok = wireEvent(event)
	if !ok || wire.Kind != "permission" || wire.PermissionID != "permission-0123456789" {
		t.Fatalf("wire %+v %v", wire, ok)
	}
	_ = time.Second
}

func TestReconcileSnapshotKeepsLiveChats(t *testing.T) {
	shells := make([]terminalproto.SessionSnapshot, 32)
	for i := range shells {
		shells[i] = terminalproto.SessionSnapshot{ID: "shell-" + string(rune('a'+i)), Title: "Shell", State: terminalproto.Exited}
	}
	chat := terminalproto.SessionSnapshot{ID: "chat-0123456789", Title: "Chat", State: terminalproto.Running}
	got := reconcileSnapshot(shells, []terminalproto.SessionSnapshot{chat})
	if len(got.Sessions) != 32 || got.Sessions[0].ID != chat.ID {
		t.Fatalf("live chat was omitted: %+v", got.Sessions[:2])
	}
}

func TestChatHostOmitsHistoryFieldsForAnOldController(t *testing.T) {
	h, _, out := testChatHost(t)
	h.link.history = false
	changed, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home})
	if err != nil {
		t.Fatal(err)
	}
	if changed.ACPSessionID != "" {
		t.Fatal("create reported an ACP id without acp-history-v1")
	}
	if snap, _ := h.snapshot("session-0123456789"); snap.ACPSessionID != "" {
		t.Fatal("snapshot reported an ACP id")
	}
	if _, err := h.end("session-0123456789"); err != nil {
		t.Fatal(err)
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	for _, m := range out.msgs {
		if s, ok := m.Body.(*terminalproto.SessionChanged); ok && s.ACPSessionID != "" {
			t.Fatal("session change reported an ACP id")
		}
	}
}

func TestChatHostReportsCreatingInSnapshots(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	out := &sent{}
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: t.TempDir()})
	h := sup.attach(ctx, 1, out.send, true, true)
	h.sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	gate := make(chan struct{})
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	h.sup.start = func(_ context.Context, _ acpagent.Options) (chatProcess, error) {
		<-gate
		return proc, nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if snap, ok := h.snapshot("session-0123456789"); ok {
			if snap.State != terminalproto.Creating {
				t.Fatalf("state %q", snap.State)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("creating session was not in the snapshot")
		}
		time.Sleep(time.Millisecond)
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	h.close()
}

func (s *sent) chatText(text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.msgs {
		if event, ok := m.Body.(*terminalproto.ACPEvent); ok && event.Text == text {
			return true
		}
	}
	return false
}

func TestChatSurvivesReconnectAndFlushesDetachedEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: t.TempDir()})
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		proc.onEvent = opts.OnEvent
		return proc, nil
	}

	first := &sent{}
	a := sup.attach(ctx, 1, first.send, true, true)
	defer a.close()
	changed, err := a.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: sup.opts.Home,
	})
	if err != nil || changed.State != terminalproto.Running {
		t.Fatalf("open: %+v %v", changed, err)
	}

	// The control connection drops. The ACP process must stay alive and the
	// session must remain in reconciliation so the controller does not mark it
	// lost.
	a.close()
	select {
	case <-proc.done:
		t.Fatal("detaching the control connection closed the chat")
	case <-time.After(30 * time.Millisecond):
	}
	proc.onEvent(acpagent.Event{Kind: "agent", MessageID: "msg_1", Text: "while away"})
	if snap, ok := a.snapshot("session-0123456789"); !ok || snap.State != terminalproto.Running {
		t.Fatalf("snapshot after detach: %+v %v", snap, ok)
	}

	// The next connection adopts the same session and receives the event that
	// was produced while nothing was attached.
	second := &sent{}
	b := sup.attach(ctx, 2, second.send, true, true)
	defer b.close()
	if snap, ok := b.snapshot("session-0123456789"); !ok || snap.State != terminalproto.Running || snap.ACPSessionID == "" {
		t.Fatalf("snapshot after reattach: %+v %v", snap, ok)
	}
	if !second.chatText("while away") {
		t.Fatalf("buffered event was not delivered after reattach: %+v", second.msgs)
	}
	if err := b.prompt("session-0123456789", "again", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		proc.mu.Lock()
		prompted := proc.prompted
		proc.mu.Unlock()
		if prompted == "again" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("prompted %q", prompted)
		}
		time.Sleep(time.Millisecond)
	}
	sup.Close()
	select {
	case <-proc.closed:
	case <-time.After(time.Second):
		t.Fatal("supervisor close did not close the chat")
	}
}

func (s *sent) waitFor(t *testing.T, match func(terminalproto.Message) bool) terminalproto.Message {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, message := range s.msgs {
			if match(message) {
				s.mu.Unlock()
				return message
			}
		}
		s.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("expected control message was not sent")
	return terminalproto.Message{}
}

func TestFactoryReattachesProcessScopedChat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	home := t.TempDir()
	opts := terminalhost.ManagerOptions{Home: home}
	sup := newChatSupervisor(ctx, opts)
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		proc.onEvent = opts.OnEvent
		return proc, nil
	}
	headers := http.Header{terminalproto.FeatureHeader: {terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," + terminalproto.ACPHistoryFeature + "," + terminalproto.ACPToolDetailsFeature}}
	factory := FactoryWithChats(opts, &ChatSupervisor{s: sup})

	first := &sent{}
	handle, cleanup := factory(ctx, http.DefaultClient, headers, 1, first.send)
	create := terminalproto.Message{Version: terminalproto.Version, RequestID: "request-0123456789", SessionID: "session-0123456789", Generation: 1,
		Body: &terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: home}}
	if err := handle(ctx, create); err != nil {
		t.Fatal(err)
	}
	first.waitFor(t, func(m terminalproto.Message) bool {
		changed, ok := m.Body.(terminalproto.SessionChanged)
		return ok && changed.State == terminalproto.Running
	})
	cleanup()
	select {
	case <-proc.closed:
		t.Fatal("control cleanup closed the chat process")
	case <-time.After(30 * time.Millisecond):
	}

	// A new control connection with a new generation adopts the live chat.
	proc.onEvent(acpagent.Event{Kind: "agent", MessageID: "msg_away", Text: "away"})
	second := &sent{}
	handle2, cleanup2 := factory(ctx, http.DefaultClient, headers, 2, second.send)
	defer cleanup2()
	reconcile := terminalproto.Message{Version: terminalproto.Version, RequestID: "request-9876543210", Generation: 2, Body: &terminalproto.Reconcile{}}
	if err := handle2(ctx, reconcile); err != nil {
		t.Fatal(err)
	}
	reply := second.waitFor(t, func(m terminalproto.Message) bool {
		_, ok := m.Body.(terminalproto.Reconcile)
		return ok
	})
	snapshot, ok := reply.Body.(terminalproto.Reconcile)
	if !ok {
		t.Fatal("reconcile reply missing")
	}
	found := false
	for _, session := range snapshot.Sessions {
		if session.ID == create.SessionID && session.State == terminalproto.Running && session.ACPSessionID != "" {
			found = true
			if session.ActivityAt != 0 || session.BellAt != 0 {
				t.Fatalf("attention fields sent without attention-v1: %+v", session)
			}
		}
	}
	if !found {
		t.Fatalf("live chat missing from reconcile: %+v", snapshot.Sessions)
	}
	if !second.chatText("away") {
		t.Fatalf("detached event not flushed: %+v", second.msgs)
	}
}

func TestChatStartupCompletesOnTheReplacementLink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: t.TempDir()})
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	gate := make(chan struct{})
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		<-gate
		proc.onEvent = opts.OnEvent
		return proc, nil
	}

	firstOut := &sent{}
	first := sup.attach(ctx, 1, firstOut.send, true, true)
	defer first.close()
	opened := make(chan error, 1)
	go func() {
		_, err := first.open("request-0123456789", "session-0123456789", terminalproto.Create{
			Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: sup.opts.Home,
		})
		opened <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if snapshot, ok := first.snapshot("session-0123456789"); ok && snapshot.State == terminalproto.Creating {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the chat never reached Creating")
		}
		time.Sleep(time.Millisecond)
	}

	// The connection that asked goes away while ACP startup is still running.
	first.close()
	secondOut := &sent{}
	second := sup.attach(ctx, 2, secondOut.send, true, true)
	defer second.close()
	close(gate)
	if err := <-opened; err != nil {
		t.Fatal(err)
	}
	secondOut.waitFor(t, func(m terminalproto.Message) bool {
		switch changed := m.Body.(type) {
		case terminalproto.SessionChanged:
			return changed.State == terminalproto.Running && changed.ACPSessionID != ""
		case *terminalproto.SessionChanged:
			return changed.State == terminalproto.Running && changed.ACPSessionID != ""
		}
		return false
	})
	if snapshot, ok := second.snapshot("session-0123456789"); !ok || snapshot.State != terminalproto.Running {
		t.Fatalf("snapshot: %+v %v", snapshot, ok)
	}
	sup.Close()
}

func TestChatPromptReadsOnlyUploadedImages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: t.TempDir(), Layout: terminalhost.Layout{Dir: root}})
	dir := filepath.Join(root, "launch-uploads", "agent-0123456789")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 8)...)
	path := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(path, png, 0600); err != nil {
		t.Fatal(err)
	}
	images, err := sup.readAttachments([]string{path})
	if err != nil || len(images) != 1 || images[0].MIME != "image/png" || !bytes.Equal(images[0].Data, png) {
		t.Fatalf("read uploaded image: %+v %v", images, err)
	}

	outside := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(outside, png, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := sup.readAttachments([]string{outside}); !errors.Is(err, errAttachmentUnavailable) {
		t.Fatalf("outside upload root: %v", err)
	}
	if _, err := sup.readAttachments([]string{dir + "/../shot.png"}); !errors.Is(err, errAttachmentUnavailable) {
		t.Fatalf("traversal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not.png"), []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := sup.readAttachments([]string{filepath.Join(dir, "not.png")}); !errors.Is(err, errAttachmentUnavailable) {
		t.Fatalf("non-image: %v", err)
	}
}

func TestChatPromptPassesImageBytesToACP(t *testing.T) {
	h, proc, _ := testChatHost(t)
	root := t.TempDir()
	h.sup.opts.Layout.Dir = root
	dir := filepath.Join(root, "launch-uploads", "agent-0123456789")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\n")
	path := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(path, png, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.prompt("session-0123456789", "", []string{path}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		proc.mu.Lock()
		images := append([]acpagent.Attachment(nil), proc.images...)
		proc.mu.Unlock()
		if len(images) == 1 {
			if images[0].MIME != "image/png" || !bytes.Equal(images[0].Data, png) {
				t.Fatalf("prompt image: %+v", images[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the image never reached the chat process")
		}
		time.Sleep(time.Millisecond)
	}
}

func toolDetailHost(t *testing.T, details bool) (*chatHost, *fakeChat, *sent) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	out := &sent{}
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: t.TempDir()})
	h := sup.attach(ctx, 1, out.send, true, details)
	h.sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		proc.onEvent = opts.OnEvent
		return proc, nil
	}
	if _, err := h.open("request-0123456789", "session-0123456789", terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home,
	}); err != nil {
		t.Fatal(err)
	}
	return h, proc, out
}

func TestToolDetailsFollowTheNegotiatedFeature(t *testing.T) {
	event := acpagent.Event{Kind: "tool", ToolCallID: "call_1", Title: "go test ./...", ToolKind: "execute", Status: "completed",
		Text: "ok", Command: "go test ./...", Path: "/work", Diff: "@@ -1 +1 @@\n-a\n+b", Additions: 1, Deletions: 1, DurationMs: 12}
	for _, details := range []bool{true, false} {
		_, proc, out := toolDetailHost(t, details)
		proc.onEvent(event)
		var seen *terminalproto.ACPEvent
		out.mu.Lock()
		for _, m := range out.msgs {
			if ev, ok := m.Body.(*terminalproto.ACPEvent); ok && ev.ToolCallID == "call_1" {
				copy := *ev
				seen = &copy
			}
		}
		out.mu.Unlock()
		if seen == nil {
			t.Fatalf("details=%v: no tool event", details)
		}
		if details && (seen.Command != event.Command || seen.Path != event.Path || seen.DurationMs != 12 || seen.Additions != 1) {
			t.Fatalf("details=%v: lost details %+v", details, seen)
		}
		if !details && (seen.Command != "" || seen.Path != "" || seen.Diff != "" || seen.DurationMs != 0 || seen.Additions != 0) {
			t.Fatalf("details=%v: sent details to an old controller %+v", details, seen)
		}
	}
}
