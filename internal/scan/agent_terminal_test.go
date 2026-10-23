package scan

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
)

func TestAgentTerminalConsentKeepsLocalSessions(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "agent.json")
	c := agentConfig{Server: "https://example.invalid", Token: "token", ID: "agent0123456789abcdef", Secret: "secret0123456789abcdef", Host: "fixture", Hostname: hostname, Home: home}
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	original := rootUser
	rootUser = func() bool { return false }
	defer func() { rootUser = original }()
	var out bytes.Buffer
	if err := runAgentTerminal([]string{"enable", "--config", config}, &out); err != nil {
		t.Skipf("tmux is unavailable: %v", err)
	}
	loaded, err := loadAgentConfig(config)
	if err != nil || !loaded.TerminalEnabled {
		t.Fatalf("consent was not stored: %+v %v", loaded, err)
	}
	out.Reset()
	if err := runAgentTerminal([]string{"status", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Terminal access: true") {
		t.Fatalf("status: %s", out.String())
	}

	layout, err := terminalhost.NewLayout(config)
	if err != nil {
		t.Fatal(err)
	}
	tmux, err := terminalhost.FindTmux(context.Background())
	if err != nil {
		t.Skip(err)
	}
	server := exec.Command(tmux.Path, "-D", "-S", layout.Socket, "-f", layout.Config)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
		_ = server.Process.Kill()
		_ = server.Wait()
	})
	deadline := time.Now().Add(3 * time.Second)
	for !terminalhost.SocketActive(layout) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !terminalhost.SocketActive(layout) {
		t.Fatal("tmux did not start")
	}
	if err := exec.Command(tmux.Path, "-S", layout.Socket, "new-session", "-d", "-s", "reet-local", "sleep", "30").Run(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runAgentTerminal([]string{"sessions", "--config", config}, &out); err != nil || !strings.Contains(out.String(), "reet-local") {
		t.Fatalf("sessions: %s %v", out.String(), err)
	}
	out.Reset()
	if err := runAgentTerminal([]string{"disable", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	sessions, err := terminalhost.Sessions(context.Background(), tmux, layout)
	if err != nil || len(sessions) != 1 || sessions[0].Name != "reet-local" {
		t.Fatalf("disable ended local sessions: %+v %v", sessions, err)
	}
	out.Reset()
	if err := runAgentTerminal([]string{"end", "--session", "reet-local", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if sessions, _ := terminalhost.Sessions(context.Background(), tmux, layout); len(sessions) != 0 {
		t.Fatalf("local end left sessions: %+v", sessions)
	}
}

func TestAgentTerminalEnableRefusesRoot(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	original := rootUser
	rootUser = func() bool { return true }
	defer func() { rootUser = original }()
	config := filepath.Join(t.TempDir(), "agent.json")
	if err := writePrivate(config, agentConfig{Server: "https://example.invalid", Token: "token", ID: "agent0123456789abcdef", Secret: "secret0123456789abcdef", Host: "fixture", Hostname: hostname, Home: home}); err != nil {
		t.Fatal(err)
	}
	if err := runAgentTerminal([]string{"enable", "--config", config}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "non-root") {
		t.Fatalf("root enable: %v", err)
	}
}
