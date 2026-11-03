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

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	reattachSessionID = "session-reattach0123456789"
	reattachRequestID = "request-reattach0123456789"
)

// reattachLoads records every start the supervisor asks for: a non-empty
// Resume is the session/load the real acpagent sends for it.
type reattachLoads struct {
	mu    sync.Mutex
	loads []string
	dirs  []string
}

func (r *reattachLoads) record(opts acpagent.Options) {
	r.mu.Lock()
	r.loads = append(r.loads, opts.Resume)
	r.dirs = append(r.dirs, opts.Dir)
	r.mu.Unlock()
}

func (r *reattachLoads) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.loads)
}

func reattachHost(t *testing.T) (*chatHost, *fakeChat, *sent, *reattachLoads) {
	t.Helper()
	h, proc, out := testChatHost(t)
	t.Cleanup(h.sup.Close)
	loads := &reattachLoads{}
	h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		loads.record(opts)
		proc.mu.Lock()
		proc.onEvent = opts.OnEvent
		proc.mu.Unlock()
		return proc, nil
	}
	return h, proc, out, loads
}

func reattachBody(h *chatHost, proc *fakeChat) terminalproto.ChatReattach {
	return terminalproto.ChatReattach{Tool: "opencode", Dir: h.sup.opts.Home, ACPSessionID: proc.SessionID(), Title: "Lost chat", Trust: true}
}

func TestReattachStartsLoadUnderSameSessionID(t *testing.T) {
	h, proc, _, loads := reattachHost(t)
	changed, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc))
	if err != nil {
		t.Fatal(err)
	}
	if changed.State != terminalproto.Running || changed.ACPSessionID != proc.SessionID() {
		t.Fatalf("reattach reply %+v", changed)
	}
	if loads.count() != 1 || loads.loads[0] != proc.SessionID() {
		t.Fatalf("session/load ids %v, want one load of %q", loads.loads, proc.SessionID())
	}
	snapshot, ok := h.snapshot(reattachSessionID)
	if !ok || snapshot.State != terminalproto.Running || snapshot.Title != "Lost chat" {
		t.Fatalf("reattached chat not under its own id: %+v present=%v", snapshot, ok)
	}
}

// The controller keeps its own transcript, so what session/load replays must
// not reach it a second time. What Start emits after the replay must: the
// close of a tool call the crash interrupted (or its card keeps spinning) and
// the idle status with the confirmed model (or the row keeps showing the
// crashed turn's working state). Later events are forwarded as usual.
func TestReattachDropsReplay(t *testing.T) {
	h, proc, out, loads := reattachHost(t)
	h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		loads.record(opts)
		// session/load replay, ending in a tool call that was still running.
		opts.OnEvent(acpagent.Event{Kind: "user", Text: "old question"})
		opts.OnEvent(acpagent.Event{Kind: "agent", MessageID: "msg_old1", Text: "old answer"})
		opts.OnEvent(acpagent.Event{Kind: "user", Text: "older follow-up"})
		opts.OnEvent(acpagent.Event{Kind: "tool", ToolCallID: "call_old", Status: "in_progress", ToolKind: "execute", Title: "make test"})
		opts.OnEvent(acpagent.Event{Kind: "status", State: "working"})
		// What acpagent.Start emits once the load has returned.
		if opts.OnReady != nil {
			opts.OnReady()
		}
		opts.OnEvent(acpagent.Event{Kind: "tool", ToolCallID: "call_old", Status: "failed", ToolKind: "other",
			Title: "Interrupted tool call", Text: "Interrupted before this chat was reopened."})
		opts.OnEvent(acpagent.Event{Kind: "status", State: "idle", Detail: "test/model"})
		proc.mu.Lock()
		proc.onEvent = opts.OnEvent
		proc.mu.Unlock()
		return proc, nil
	}
	body := reattachBody(h, proc)
	body.Model = "test/model"
	if _, err := h.reattach(reattachRequestID, reattachSessionID, body); err != nil {
		t.Fatal(err)
	}
	if replayed := append(out.events("user"), out.events("agent")...); len(replayed) != 0 {
		t.Fatalf("session/load replay was forwarded: %+v", replayed)
	}
	tools := out.events("tool")
	if len(tools) != 1 || tools[0].ToolCallID != "call_old" || tools[0].Status != "failed" {
		t.Fatalf("interrupted tool close not forwarded alone: %+v", tools)
	}
	statuses := out.events("status")
	if len(statuses) != 1 || statuses[0].State != "idle" || statuses[0].Detail != "test/model" {
		t.Fatalf("post-load status not forwarded alone: %+v", statuses)
	}
	h.sup.mu.Lock()
	cs := h.sup.sessions[reattachSessionID]
	var lastStatus *terminalproto.ACPEvent
	if cs != nil {
		lastStatus = cs.lastStatus
	}
	h.sup.mu.Unlock()
	if lastStatus == nil || lastStatus.State != "idle" {
		t.Fatalf("machine kept no status for a later reconnect: %+v", lastStatus)
	}
	proc.onEvent(acpagent.Event{Kind: "agent", MessageID: "msg_new1", Text: "new answer"})
	agent := out.events("agent")
	if len(agent) != 1 || agent[0].Text != "new answer" {
		t.Fatalf("post-start event not forwarded exactly once: %+v", agent)
	}
}

// The controller may mark a row lost while the machine still runs its chat.
// That process is adopted: no restart, no second session/load.
func TestReattachAdoptsLiveProcess(t *testing.T) {
	h, proc, _, loads := reattachHost(t)
	if _, err := h.open("request-first0123456789", reattachSessionID, terminalproto.Create{
		Title: "Chat", Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home, ResumeACP: proc.SessionID(),
	}); err != nil {
		t.Fatal(err)
	}
	cs := h.find(reattachSessionID)
	changed, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc))
	if err != nil {
		t.Fatal(err)
	}
	if changed.State != terminalproto.Running || changed.ACPSessionID != proc.SessionID() {
		t.Fatalf("adopt reply %+v", changed)
	}
	if loads.count() != 1 {
		t.Fatalf("session/load sent %d times, want 1", loads.count())
	}
	if h.find(reattachSessionID) != cs {
		t.Fatal("the live chat was replaced instead of adopted")
	}
	select {
	case <-proc.closed:
		t.Fatal("the live process was closed")
	default:
	}
}

func TestReattachRefusedWhenAnotherRowOwnsConversation(t *testing.T) {
	h, proc, _, loads := reattachHost(t)
	if _, err := h.open("request-other0123456789", "session-other0123456789", terminalproto.Create{
		Title: "Other", Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home, ResumeACP: proc.SessionID(),
	}); err != nil {
		t.Fatal(err)
	}
	_, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc))
	if !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("reattach of a conversation another row owns answered %v", err)
	}
	if loads.count() != 1 {
		t.Fatalf("session/load sent %d times, want 1", loads.count())
	}
	if _, ok := h.snapshot(reattachSessionID); ok {
		t.Fatal("refused reattach left a session behind")
	}
}

func TestReattachIsIdempotentPerRequestID(t *testing.T) {
	h, proc, _, loads := reattachHost(t)
	first, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc))
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc))
	if err != nil {
		t.Fatalf("retrying the same reattach was refused: %v", err)
	}
	if again != first {
		t.Fatalf("retry answered %+v, first %+v", again, first)
	}
	if loads.count() != 1 {
		t.Fatalf("session/load sent %d times, want 1", loads.count())
	}
	if _, err := h.reattach(reattachRequestID, "session-another0123456789", reattachBody(h, proc)); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("a request id reused for another session answered %v", err)
	}
}

func TestReattachFailureLeavesNothingRunning(t *testing.T) {
	h, proc, _, loads := reattachHost(t)
	h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		loads.record(opts)
		return nil, errors.New("session/load failed")
	}
	h.sup.mu.Lock()
	quota := h.sup.quotaLocked()
	h.sup.mu.Unlock()
	if _, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc)); err == nil {
		t.Fatal("a failed session/load reported success")
	}
	if _, ok := h.snapshot(reattachSessionID); ok {
		t.Fatal("failed reattach left a session behind")
	}
	h.sup.mu.Lock()
	after := h.sup.quotaLocked()
	_, receipt := h.sup.requests[reattachRequestID]
	h.sup.mu.Unlock()
	if after != quota || receipt {
		t.Fatalf("failed reattach leaked: quota %d -> %d, receipt=%v", quota, after, receipt)
	}
	// The failure is retryable under the same session id.
	h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		loads.record(opts)
		return proc, nil
	}
	if _, err := h.reattach("request-retry0123456789", reattachSessionID, reattachBody(h, proc)); err != nil {
		t.Fatalf("retry after a failed reattach: %v", err)
	}
}

// An exited receipt for the same id lingers in the supervisor (end keeps it
// to answer a lost reply). A reattach replaces it rather than conflicting.
func TestReattachReplacesExitedReceipt(t *testing.T) {
	h, proc, _, loads := reattachHost(t)
	h.sup.sessions[reattachSessionID] = &chatSession{id: reattachSessionID, tool: "opencode", acpID: proc.SessionID(), state: terminalproto.Exited}
	h.sup.requests["request-old0123456789"] = reattachSessionID
	changed, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc))
	if err != nil {
		t.Fatal(err)
	}
	if changed.State != terminalproto.Running || loads.count() != 1 {
		t.Fatalf("exited receipt not replaced: %+v loads=%d", changed, loads.count())
	}
	if _, stale := h.sup.requests["request-old0123456789"]; stale {
		t.Fatal("the replaced receipt's request id still names the session")
	}
}

// A retired receipt whose process group could not be confirmed gone still
// owns the conversation: nothing else may load it.
func TestReattachRefusesStrayReceipt(t *testing.T) {
	h, proc, _, loads := reattachHost(t)
	h.sup.sessions[reattachSessionID] = &chatSession{id: reattachSessionID, tool: "opencode", acpID: proc.SessionID(), state: terminalproto.Exited, stray: true}
	if _, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc)); !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("reattach over a stray process answered %v", err)
	}
	if loads.count() != 0 {
		t.Fatalf("session/load sent %d times", loads.count())
	}
}

// The controller checks only ValidDir (absolute, no control characters, ".."
// allowed). The machine's ResolveDir confines it to home, as for a create.
func TestReattachConfinesDirToHome(t *testing.T) {
	h, proc, _, loads := reattachHost(t)
	home, err := filepath.EvalSymlinks(h.sup.opts.Home)
	if err != nil {
		t.Fatal(err)
	}
	body := reattachBody(h, proc)
	body.Dir = h.sup.opts.Home + "/../.."
	if !terminalproto.ValidDir(body.Dir) {
		t.Fatalf("test dir %q is not wire-valid", body.Dir)
	}
	if _, err := h.reattach(reattachRequestID, reattachSessionID, body); err == nil {
		t.Fatal("a dir escaping home was accepted")
	}
	if loads.count() != 0 {
		t.Fatal("a process was started outside home")
	}
	if _, ok := h.snapshot(reattachSessionID); ok {
		t.Fatal("refused reattach left a session behind")
	}

	if err := os.Mkdir(filepath.Join(h.sup.opts.Home, "work"), 0700); err != nil {
		t.Fatal(err)
	}
	body.Dir = h.sup.opts.Home + "/work/.."
	if _, err := h.reattach("request-inside0123456789", reattachSessionID, body); err != nil {
		t.Fatal(err)
	}
	if loads.count() != 1 || loads.dirs[0] != home {
		t.Fatalf("dir inside home not normalized: %v", loads.dirs)
	}
}

// The control connection answers chat_reattach from its own goroutine with the
// session_changed the machine reached, under the request's id.
func TestReattachDispatchAnswersRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	home := t.TempDir()
	opts := terminalhost.ManagerOptions{Home: home}
	sup := newChatSupervisor(ctx, opts)
	t.Cleanup(sup.Close)
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	loads := &reattachLoads{}
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		loads.record(opts)
		return proc, nil
	}
	headers := http.Header{terminalproto.FeatureHeader: {terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," +
		terminalproto.ACPHistoryFeature + "," + terminalproto.ChatReattachFeature}}
	out := &sent{}
	handle, cleanup := FactoryWithChats(opts, &ChatSupervisor{s: sup})(ctx, http.DefaultClient, headers, 1, out.send)
	defer cleanup()
	msg := terminalproto.Message{Version: terminalproto.Version, RequestID: reattachRequestID, SessionID: reattachSessionID, Generation: 1,
		Body: &terminalproto.ChatReattach{Tool: "opencode", Dir: home, ACPSessionID: proc.SessionID(), Title: "Lost chat", Trust: true}}
	if err := msg.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := handle(ctx, msg); err != nil {
		t.Fatal(err)
	}
	reply := out.waitFor(t, func(m terminalproto.Message) bool { return m.RequestID == reattachRequestID })
	changed, ok := reply.Body.(terminalproto.SessionChanged)
	if !ok || changed.State != terminalproto.Running || changed.ACPSessionID != proc.SessionID() {
		t.Fatalf("reattach reply %#v", reply.Body)
	}
	if loads.count() != 1 || loads.loads[0] != proc.SessionID() {
		t.Fatalf("session/load ids %v", loads.loads)
	}
}

// A chat still starting under the session id (a create, a restore or an
// earlier reattach) is refused as a conflict that says so, so the controller
// does not tell the person the conversation is open elsewhere.
func TestReattachWhileStartingSaysStarting(t *testing.T) {
	h, proc, _, loads := reattachHost(t)
	h.sup.sessions[reattachSessionID] = &chatSession{id: reattachSessionID, tool: "opencode", acpID: proc.SessionID(), state: terminalproto.Creating}
	_, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc))
	if !errors.Is(err, terminalhost.ErrOperationConflict) {
		t.Fatalf("got %v", err)
	}
	if got := chatResult(err); got != (terminalproto.Result{Code: terminalproto.Conflict, Detail: terminalproto.ConflictStarting}) {
		t.Fatalf("result %+v", got)
	}
	if loads.count() != 0 {
		t.Fatalf("session/load sent %d times", loads.count())
	}
}

// Another row owning the conversation stays a plain conflict.
func TestReattachOwnedElsewhereIsPlainConflict(t *testing.T) {
	h, proc, _, _ := reattachHost(t)
	if _, err := h.open("request-other0123456789", "session-other0123456789", terminalproto.Create{
		Title: "Other", Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home, ResumeACP: proc.SessionID(),
	}); err != nil {
		t.Fatal(err)
	}
	_, err := h.reattach(reattachRequestID, reattachSessionID, reattachBody(h, proc))
	if got := chatResult(err); got != (terminalproto.Result{Code: terminalproto.Conflict}) {
		t.Fatalf("result %+v", got)
	}
}

// The still-starting conflict reaches the controller as a valid result that
// carries the starting detail.
func TestReattachDispatchStartingSaysStarting(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	home := t.TempDir()
	opts := terminalhost.ManagerOptions{Home: home}
	sup := newChatSupervisor(ctx, opts)
	t.Cleanup(sup.Close)
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) { return proc, nil }
	sup.mu.Lock()
	sup.sessions[reattachSessionID] = &chatSession{id: reattachSessionID, tool: "opencode", acpID: proc.SessionID(), state: terminalproto.Creating}
	sup.mu.Unlock()
	headers := http.Header{terminalproto.FeatureHeader: {terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," +
		terminalproto.ACPHistoryFeature + "," + terminalproto.ChatReattachFeature}}
	out := &sent{}
	handle, cleanup := FactoryWithChats(opts, &ChatSupervisor{s: sup})(ctx, http.DefaultClient, headers, 1, out.send)
	defer cleanup()
	msg := terminalproto.Message{Version: terminalproto.Version, RequestID: reattachRequestID, SessionID: reattachSessionID, Generation: 1,
		Body: &terminalproto.ChatReattach{Tool: "opencode", Dir: home, ACPSessionID: proc.SessionID(), Title: "Lost chat", Trust: true}}
	if err := handle(ctx, msg); err != nil {
		t.Fatal(err)
	}
	reply := out.waitFor(t, func(m terminalproto.Message) bool { return m.RequestID == reattachRequestID })
	got, ok := reply.Body.(terminalproto.Result)
	if !ok || got.Code != terminalproto.Conflict || got.Detail != terminalproto.ConflictStarting {
		t.Fatalf("reply %#v", reply.Body)
	}
	if err := reply.Validate(); err != nil {
		t.Fatalf("reply does not validate: %v", err)
	}
}
