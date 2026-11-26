//go:build linux || darwin

package acpagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scriptAgent is a shell agent that opens session s1 and then runs tail. It is
// for behaviour the Go fake cannot show, such as no longer reading stdin.
func scriptAgent(t *testing.T, tail string) Options {
	t.Helper()
	script := filepath.Join(t.TempDir(), "agent")
	body := "#!/bin/sh\nread -r line\n" +
		`echo '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentInfo":{"name":"script","version":"1.0.0"}}}'` +
		"\nread -r line\n" + `echo '{"jsonrpc":"2.0","id":2,"result":{"sessionId":"s1"}}'` + "\n" + tail + "\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	return Options{Agent: Spec{Key: "script", Command: "script"}, Binary: script, Dir: t.TempDir(), Home: t.TempDir(),
		BaseEnv: []string{"PATH=" + os.Getenv("PATH")}}
}

// An agent that is alive but no longer reads stdin must not hold the chat: a
// prompt larger than the pipe gives up, and the chat ends instead of blocking
// Stop, the turn deadlines and End behind the write.
func TestAnAgentThatStopsReadingIsEnded(t *testing.T) {
	old := writeTimeout
	writeTimeout = 200 * time.Millisecond
	defer func() { writeTimeout = old }()
	session, err := Start(context.Background(), scriptAgent(t, "exec sleep 600"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- session.Prompt(context.Background(), strings.Repeat("a", 1<<20)) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the prompt is still blocked on an agent that stopped reading")
	}
	select {
	case <-session.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the chat was not ended")
	}
	session.Close()
	if !session.GroupRetired() {
		t.Fatal("the process group was not confirmed gone")
	}
}
