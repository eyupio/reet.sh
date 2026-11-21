//go:build linux || darwin

package scan

import (
	"context"
	"strings"
	"testing"
)

func TestRestartEndsChatsBeforeReplacingTheProcess(t *testing.T) {
	var order []string
	oldExec := agentExec
	agentExec = func(path string, _ []string, _ []string) error {
		order = append(order, "exec "+path)
		return nil
	}
	t.Cleanup(func() { agentExec = oldExec; onAgentRestart(nil) })
	onAgentRestart(func() { order = append(order, "chats closed") })
	if err := restartAgent("/opt/reet"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, ", "); got != "chats closed, exec /opt/reet" {
		t.Fatalf("restart order: %s", got)
	}
}

func TestTheAgentsChatSupervisorIsClosedOnRestart(t *testing.T) {
	config := enrolledMachine(t, "https://reet.example")
	t.Cleanup(func() { onAgentRestart(nil) })
	onAgentRestart(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if terminalControl(ctx, config, loaded(t, config)) == nil {
		t.Fatal("no terminal control for a valid enrollment")
	}
	beforeRestart.mu.Lock()
	registered := beforeRestart.close != nil
	beforeRestart.mu.Unlock()
	if !registered {
		t.Fatal("the chat supervisor is not closed before a restart")
	}
}
