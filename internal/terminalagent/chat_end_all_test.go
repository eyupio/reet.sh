//go:build linux || darwin

package terminalagent

import (
	"context"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// reet agent lock --end-sessions ends every chat the running agent holds,
// as the controller's end does, and counts them.
func TestEndAllChatsEndsEveryRunningChat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: t.TempDir(), Consent: func() error { return nil }})
	var procs []*fakeChat
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup.start = func(_ context.Context, o acpagent.Options) (chatProcess, error) {
		p := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
		p.onEvent = o.OnEvent
		procs = append(procs, p)
		return p, nil
	}
	h := sup.attach(ctx, 1, (&sent{}).send, true, true)
	for _, id := range []string{"session-0123456781", "session-0123456782"} {
		if _, err := h.open("request-"+id[8:], id, terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: sup.opts.Home}, false); err != nil {
			t.Fatal(err)
		}
	}
	c := &ChatSupervisor{s: sup}
	if n := c.Running(); n != 2 {
		t.Fatalf("running: %d", n)
	}
	if n := c.EndAll(); n != 2 {
		t.Fatalf("ended: %d", n)
	}
	for _, p := range procs {
		select {
		case <-p.closed:
		default:
			t.Fatal("a chat process was left running")
		}
	}
	if n := c.Running(); n != 0 {
		t.Fatalf("still running: %d", n)
	}
	var none *ChatSupervisor
	if none.Running() != 0 || none.EndAll() != 0 {
		t.Fatal("no supervisor has chats")
	}
}

// With the agent stopped, the chats it would resume are counted from its
// record and forgotten, so none comes back when it starts.
func TestRecordedChatsAreCountedAndForgotten(t *testing.T) {
	dir := t.TempDir()
	r := newChatRegistry(dir + "/structured-chats.json")
	r.remember(chatRecord{ID: "session-0123456781", Title: "Chat", Tool: "opencode", Dir: "/tmp", ACPSID: "ses_0123456789abcdef"})
	if n := RecordedChats(dir); n != 1 {
		t.Fatalf("recorded: %d", n)
	}
	if n := ForgetRecordedChats(dir); n != 1 {
		t.Fatalf("forgotten: %d", n)
	}
	if n := RecordedChats(dir); n != 0 {
		t.Fatalf("left: %d", n)
	}
}
