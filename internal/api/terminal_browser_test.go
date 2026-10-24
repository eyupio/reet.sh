//go:build linux || darwin

package api

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// These fixture-only commands are never registered in a production server.
// The browser exercises actual authorization, broker, PTY and tmux; transport
// encryption itself remains covered by the separate local-DERP/native gates.
type terminalBrowserCommand struct {
	action string
	reply  chan map[string]any
}

func runTerminalBrowser(t *testing.T, base, cookie, agent, builder string, commands <-chan terminalBrowserCommand, handle func(string) map[string]any) {
	t.Helper()
	if _, err := os.Stat(filepath.Join("..", "..", "web", "dist", "index.html")); err != nil {
		t.Fatal("build web before REET_BROWSER_TESTS=1", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	script, err := filepath.Abs(filepath.Join("..", "..", "web", "tests", "terminal.e2e.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Env = append(os.Environ(), "REET_BROWSER_URL="+base, "REET_BROWSER_COOKIE="+cookie, "REET_BROWSER_AGENT="+agent, "REET_BROWSER_BUILDER="+builder)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("browser terminal integration: %v\n%s", err, output.String())
			}
			t.Log(output.String())
			return
		case request := <-commands:
			request.reply <- handle(request.action)
		case <-ctx.Done():
			cancel()
			<-done
			t.Fatalf("browser terminal integration timed out\n%s", output.String())
		}
	}
}
