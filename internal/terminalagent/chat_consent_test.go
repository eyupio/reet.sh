//go:build linux || darwin

package terminalagent

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A chat runs an agent as a shell does, so with terminal access switched off
// on the machine every request that starts, resumes, drives or answers a
// chat is refused there, whatever the controller believes. Cancelling and
// ending one still work.
func TestChatRequestsNeedLocalTerminalConsent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	home := t.TempDir()
	var off atomic.Bool
	opts := terminalhost.ManagerOptions{Home: home, Consent: func() error {
		if off.Load() {
			return terminalhost.ErrTerminalDisabled
		}
		return nil
	}}
	sup := newChatSupervisor(ctx, opts)
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	var starts atomic.Int32
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup.start = func(_ context.Context, o acpagent.Options) (chatProcess, error) {
		starts.Add(1)
		proc.onEvent = o.OnEvent
		return proc, nil
	}
	features := []string{terminalproto.LaunchFeature, terminalproto.ACPFeature, terminalproto.ACPHistoryFeature, terminalproto.ACPLiveControlsFeature,
		terminalproto.ACPQuestionFeature, terminalproto.ACPModelFeature, terminalproto.ACPAgentModeFeature, terminalproto.ChatReattachFeature, terminalproto.ACPApprovalFeature}
	headers := http.Header{terminalproto.FeatureHeader: {joinFeatures(features)}}
	out := &sent{}
	handle, cleanup := FactoryWithChats(opts, &ChatSupervisor{s: sup})(ctx, http.DefaultClient, headers, 1, out.send)
	t.Cleanup(cleanup)
	const session = "session-0123456789"
	create := terminalproto.Message{Version: terminalproto.Version, RequestID: "request-0123456780", SessionID: session, Generation: 1,
		Body: &terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: home}}
	if err := handle(ctx, create); err != nil {
		t.Fatal(err)
	}
	out.waitFor(t, func(m terminalproto.Message) bool {
		changed, ok := m.Body.(terminalproto.SessionChanged)
		return ok && changed.State == terminalproto.Running
	})
	off.Store(true)
	refused := map[string]terminalproto.Body{
		"create":     &terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: home},
		"reattach":   &terminalproto.ChatReattach{Tool: "opencode", Dir: home, ACPSessionID: "ses_0123456789abcdef", Title: "Chat", Trust: true},
		"prompt":     &terminalproto.ACPPrompt{Text: "rm -rf"},
		"control":    &terminalproto.ACPControl{Action: "steer", Text: "rm -rf"},
		"permission": &terminalproto.ACPPermission{PermissionID: "permission-0123456789", Allow: true},
		"answer":     &terminalproto.ACPQuestionAnswer{QuestionID: "question-0123456789", Action: "cancel"},
		"model":      &terminalproto.ACPModel{Model: "anthropic/claude"},
		"mode":       &terminalproto.ACPMode{Yolo: true},
		"agent mode": &terminalproto.ACPAgentMode{Mode: "agent"},
	}
	i := 0
	for name, body := range refused {
		i++
		id := "request-01234567" + string(rune('a'+i)) + "x"
		target := session
		if name == "create" || name == "reattach" {
			target = "session-012345678" + string(rune('a'+i))
		}
		if err := handle(ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: id, SessionID: target, Generation: 1, Body: body}); err != nil {
			t.Fatal(name, err)
		}
		reply := out.waitFor(t, func(m terminalproto.Message) bool { return m.RequestID == id })
		if r, ok := reply.Body.(terminalproto.Result); !ok || r.Code != terminalproto.Denied {
			t.Errorf("%s with terminal access off answered %+v", name, reply.Body)
		}
	}
	if starts.Load() != 1 {
		t.Fatalf("a chat was started with terminal access off: %d starts", starts.Load())
	}
	proc.mu.Lock()
	prompted := proc.prompted
	proc.mu.Unlock()
	if prompted != "" {
		t.Fatalf("a prompt reached the agent: %q", prompted)
	}
	cancelID := "request-0123456799"
	if err := handle(ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: cancelID, SessionID: session, Generation: 1, Body: &terminalproto.ACPCancel{}}); err != nil {
		t.Fatal(err)
	}
	if reply := out.waitFor(t, func(m terminalproto.Message) bool { return m.RequestID == cancelID }); reply.Body.(terminalproto.Result).Code == terminalproto.Denied {
		t.Fatal("cancelling a chat needed terminal access")
	}
}

func joinFeatures(features []string) string {
	joined := ""
	for i, f := range features {
		if i > 0 {
			joined += ","
		}
		joined += f
	}
	return joined
}
