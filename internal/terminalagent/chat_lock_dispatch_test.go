//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type chatDispatchRig struct {
	ctx    context.Context
	home   string
	off    atomic.Bool
	locked atomic.Bool
	starts atomic.Int32
	proc   *fakeChat
	sup    *chatSupervisor
	out    *sent
	handle func(context.Context, terminalproto.Message) error
}

func newChatDispatchRig(t *testing.T) *chatDispatchRig {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	r := &chatDispatchRig{ctx: ctx, home: t.TempDir(), out: &sent{}}
	opts := terminalhost.ManagerOptions{Home: r.home, Locked: r.locked.Load, Consent: func() error {
		if r.off.Load() {
			return terminalhost.ErrTerminalDisabled
		}
		return nil
	}}
	r.sup = newChatSupervisor(ctx, opts)
	r.proc = &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	r.sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	r.sup.start = func(_ context.Context, o acpagent.Options) (chatProcess, error) {
		r.starts.Add(1)
		r.proc.onEvent = o.OnEvent
		return r.proc, nil
	}
	headers := http.Header{terminalproto.FeatureHeader: {joinFeatures([]string{swapFeatures(), terminalproto.ACPLiveControlsFeature,
		terminalproto.ACPQuestionFeature, terminalproto.ChatReattachFeature, terminalproto.ACPApprovalFeature})}}
	handle, cleanup := FactoryWithChats(opts, &ChatSupervisor{s: r.sup})(ctx, http.DefaultClient, headers, 1, r.out.send)
	t.Cleanup(cleanup)
	r.handle = handle
	return r
}

func (r *chatDispatchRig) ask(t *testing.T, id, session string, body terminalproto.Body) terminalproto.Message {
	t.Helper()
	if err := r.handle(r.ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: id, SessionID: session, Generation: 1, Body: body}); err != nil {
		t.Fatal(id, err)
	}
	return r.out.waitFor(t, func(m terminalproto.Message) bool { return m.RequestID == id })
}

const lockedChat = "session-0123456789"

func (r *chatDispatchRig) startChat(t *testing.T) {
	t.Helper()
	r.ask(t, "request-0123456780", lockedChat, &terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: r.home})
	r.out.waitFor(t, func(m terminalproto.Message) bool {
		changed, ok := m.Body.(terminalproto.SessionChanged)
		return ok && changed.State == terminalproto.Running
	})
}

func refusedAs(t *testing.T, what string, reply terminalproto.Message) {
	t.Helper()
	if r, ok := reply.Body.(terminalproto.Result); !ok || r.Code != terminalproto.Denied {
		t.Errorf("%s answered %+v", what, reply.Body)
	}
}

// A session-mode swap starts or restarts a chat (terminal to chat) or ends
// one for a terminal (chat to terminal), so with terminal access off it is
// refused either way and nothing is started.
func TestSessionModeNeedsLocalTerminalConsent(t *testing.T) {
	r := newChatDispatchRig(t)
	r.startChat(t)
	r.off.Store(true)
	refusedAs(t, "terminal to chat", r.ask(t, "request-01234567a1", "session-012345678b", &terminalproto.SessionMode{To: terminalproto.SessionModeChat, Tool: "opencode", Dir: r.home, ACPSessionID: "ses_0123456789abcdef", Title: "Chat", Trust: true}))
	refusedAs(t, "chat to terminal", r.ask(t, "request-01234567a2", lockedChat, &terminalproto.SessionMode{To: terminalproto.SessionModeTerminal, Tool: "opencode", Dir: r.home, ACPSessionID: "ses_0123456789abcdef", Title: "Chat", Cols: 100, Rows: 30}))
	if r.starts.Load() != 1 {
		t.Fatalf("a chat was started with terminal access off: %d", r.starts.Load())
	}
	if s, ok := (&chatHost{sup: r.sup, link: r.sup.link}).snapshot(lockedChat); !ok || s.State != terminalproto.Running {
		t.Fatalf("the swap stopped the chat: %+v", s)
	}
}

// A chat running on a locked machine, whether it was running when the lock
// was taken or resumed after the agent restarted, takes nothing: no prompt,
// no answer, no control, no swap, and it cannot be started again. Cancelling
// it still works.
func TestAChatOnALockedMachineTakesNothing(t *testing.T) {
	r := newChatDispatchRig(t)
	r.startChat(t)
	r.locked.Store(true)
	for name, body := range map[string]terminalproto.Body{
		"prompt":     &terminalproto.ACPPrompt{Text: "rm -rf"},
		"control":    &terminalproto.ACPControl{Action: "steer", Text: "rm -rf"},
		"permission": &terminalproto.ACPPermission{PermissionID: "permission-0123456789", Allow: true},
		"answer":     &terminalproto.ACPQuestionAnswer{QuestionID: "question-0123456789", Action: "cancel"},
		"mode":       &terminalproto.ACPMode{Yolo: true},
		"swap":       &terminalproto.SessionMode{To: terminalproto.SessionModeTerminal, Tool: "opencode", Dir: r.home, ACPSessionID: "ses_0123456789abcdef", Title: "Chat", Cols: 100, Rows: 30},
	} {
		refusedAs(t, name, r.ask(t, "request-0123"+name[:1]+"456789xx", lockedChat, body))
	}
	proc := r.proc
	proc.mu.Lock()
	prompted := proc.prompted
	proc.mu.Unlock()
	if prompted != "" {
		t.Fatalf("a locked machine's chat was prompted: %q", prompted)
	}
	h := &chatHost{sup: r.sup, link: r.sup.link}
	if _, err := h.open("request-0123456781", "session-012345678c", terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: r.home}, false); !errors.Is(err, terminalhost.ErrLocked) {
		t.Fatalf("a chat opened on a locked machine: %v", err)
	}
	if reply := r.ask(t, "request-0123456799", lockedChat, &terminalproto.ACPCancel{}); reply.Body.(terminalproto.Result).Code != terminalproto.OK {
		t.Fatalf("cancel on a locked machine: %+v", reply.Body)
	}
}
