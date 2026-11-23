package terminalagent

import (
	"context"
	"net/http"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestBackgroundCountFollowsTheNegotiatedFeature(t *testing.T) {
	for _, background := range []bool{true, false} {
		h, proc, out := toolDetailHost(t, true)
		h.link.background = background
		proc.onEvent(acpagent.Event{Kind: "status", State: "idle", Background: 2})
		var seen *terminalproto.ACPEvent
		out.mu.Lock()
		for _, m := range out.msgs {
			if ev, ok := m.Body.(*terminalproto.ACPEvent); ok && ev.Kind == "status" && ev.State == "idle" {
				copy := *ev
				seen = &copy
			}
		}
		out.mu.Unlock()
		if seen == nil {
			t.Fatalf("background=%v: no idle status", background)
		}
		want := 0
		if background {
			want = 2
		}
		if seen.Background != want {
			t.Fatalf("background=%v: sent count %d, want %d", background, seen.Background, want)
		}
	}
}

func TestControlsBackgroundStrippedForAPeerWithoutTheFeature(t *testing.T) {
	state := terminalproto.ACPControls{Background: 3, Options: []terminalproto.ACPSetting{}}
	if got := (&chatHost{link: &chatLink{catalogue: true, provider: true}}).forConnection(state); got.Background != 0 {
		t.Fatalf("an old controller was sent background %d", got.Background)
	}
	if got := (&chatHost{link: &chatLink{catalogue: true, provider: true, background: true}}).forConnection(state); got.Background != 3 {
		t.Fatalf("background = %d, want 3", got.Background)
	}
}

// heldChat is a chat process that reports held work in its controls.
type heldChat struct {
	*fakeChat
	held int
}

func (h *heldChat) Controls() terminalproto.ACPControls {
	return terminalproto.ACPControls{Background: h.held, Options: []terminalproto.ACPSetting{}}
}

func TestBackgroundCountFollowsTheControllersFeatureHeader(t *testing.T) {
	for _, offered := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		home := t.TempDir()
		opts := terminalhost.ManagerOptions{Home: home}
		sup := newChatSupervisor(ctx, opts)
		proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
		sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
		sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
			proc.onEvent = opts.OnEvent
			return proc, nil
		}
		features := terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," + terminalproto.ACPHistoryFeature
		if offered {
			features += "," + terminalproto.ACPBackgroundFeature
		}
		out := &sent{}
		handle, cleanup := FactoryWithChats(opts, &ChatSupervisor{s: sup})(ctx, http.DefaultClient, http.Header{terminalproto.FeatureHeader: {features}}, 1, out.send)
		create := terminalproto.Message{Version: terminalproto.Version, RequestID: "request-0123456789", SessionID: "session-0123456789", Generation: 1,
			Body: &terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: home}}
		if err := handle(ctx, create); err != nil {
			t.Fatal(err)
		}
		out.waitFor(t, func(m terminalproto.Message) bool {
			changed, ok := m.Body.(terminalproto.SessionChanged)
			return ok && changed.State == terminalproto.Running
		})
		proc.onEvent(acpagent.Event{Kind: "status", State: "idle", Background: 2})
		got := out.waitFor(t, func(m terminalproto.Message) bool {
			ev, ok := m.Body.(*terminalproto.ACPEvent)
			return ok && ev.Kind == "status" && ev.State == "idle"
		}).Body.(*terminalproto.ACPEvent)
		want := 0
		if offered {
			want = 2
		}
		if got.Background != want {
			t.Fatalf("offered=%v: sent %d, want %d", offered, got.Background, want)
		}
		cleanup()
		cancel()
	}
}

func TestSettledIdleCarriesTheHeldCount(t *testing.T) {
	h, proc, _ := toolDetailHost(t, true)
	held := &heldChat{fakeChat: proc, held: 2}
	cs := h.find("session-0123456789")
	h.sup.mu.Lock()
	cs.process = held
	h.sup.mu.Unlock()
	proc.onEvent(acpagent.Event{Kind: "status", State: "waiting"})
	proc.onEvent(acpagent.Event{Kind: "permission", PermissionID: "perm-0123456789", Title: "Run", ToolKind: "execute", Command: "ls"})
	allow := true
	proc.onEvent(acpagent.Event{Kind: "permission_resolved", PermissionID: "perm-0123456789", Allow: &allow})
	h.sup.mu.Lock()
	last := cs.lastStatus
	h.sup.mu.Unlock()
	if last == nil || last.State != "idle" || last.Background != 2 {
		t.Fatalf("settled status %+v, want idle with 2 held", last)
	}
}
