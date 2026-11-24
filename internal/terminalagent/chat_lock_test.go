//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// While the machine is locked its connection refuses to stream chat events
// (agenthealth holds them). The chat keeps running and keeps what it said;
// after unlock the agent reconnects, the held events are delivered and the
// chat takes prompts again. Nothing is deleted on the way.
func TestAChatHeldByTheLockResumesAfterUnlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: t.TempDir(), Consent: func() error { return nil }})
	proc := &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}
	sup.lookPath = func(string) (string, error) { return "/bin/true", nil }
	sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
		proc.onEvent = opts.OnEvent
		return proc, nil
	}
	var locked atomic.Bool
	firstOut := &sent{}
	first := sup.attach(ctx, 1, func(ctx context.Context, m terminalproto.Message) error {
		if _, ok := m.Body.(*terminalproto.ACPEvent); ok && locked.Load() {
			return errors.New("machine is locked")
		}
		return firstOut.send(ctx, m)
	}, true, true)
	const session = "session-0123456789"
	if _, err := first.open("request-0123456789", session, terminalproto.Create{
		Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: sup.opts.Home,
	}, true); err != nil {
		t.Fatal(err)
	}
	locked.Store(true)
	proc.onEvent(acpagent.Event{Kind: "agent", MessageID: "msg_locked", Text: "said while locked"})
	if firstOut.chatText("said while locked") {
		t.Fatal("a locked machine streamed a chat event")
	}
	if snapshot, ok := first.snapshot(session); !ok || snapshot.State != terminalproto.Running {
		t.Fatalf("the lock stopped the chat: %+v %t", snapshot, ok)
	}
	locked.Store(false)
	first.close()
	secondOut := &sent{}
	second := sup.attach(ctx, 2, secondOut.send, true, true)
	defer second.close()
	deadline := time.Now().Add(2 * time.Second)
	for !secondOut.chatText("said while locked") {
		if time.Now().After(deadline) {
			t.Fatalf("the held event was not delivered after unlock: %+v", secondOut.msgs)
		}
		time.Sleep(time.Millisecond)
	}
	go func() { _ = second.prompt(session, "carry on", nil) }()
	for {
		proc.mu.Lock()
		prompted := proc.prompted
		proc.mu.Unlock()
		if prompted == "carry on" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the chat did not take a prompt after unlock")
		}
		time.Sleep(time.Millisecond)
	}
}
