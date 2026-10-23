//go:build linux || darwin

package terminalhost

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestNativeAgentKillKeepsTmuxSession uses the real per-user service manager.
// It is opt-in because hosted runners and developer shells may not have a user
// manager. When enabled, an unavailable manager is a failure, not a skip.
func TestNativeAgentKillKeepsTmuxSession(t *testing.T) {
	if os.Getenv("REET_NATIVE_SERVICE_TESTS") != "1" {
		t.Skip("set REET_NATIVE_SERVICE_TESTS=1 on a host with a working user service manager")
	}
	tmux, err := FindTmux(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("/tmp", "reet-native-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	config := filepath.Join(dir, "agent.json")
	if err := os.WriteFile(config, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	agentScript := filepath.Join(dir, "fake-agent")
	if err := os.WriteFile(agentScript, []byte("#!/bin/sh\nwhile :; do sleep 1; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	layout, err := NewLayout(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(layout); err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("reet-native-%d", os.Getpid())
	plan, err := PlanServices(context.Background(), ServiceOptions{
		Prefix: prefix, Binary: agentScript, ConfigPath: config, Layout: layout,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = Uninstall(context.Background(), plan)
		_ = exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
	})
	if err := Install(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	waitForNative(t, 20*time.Second, func() bool {
		return SocketActive(layout) && ReadStatus(context.Background(), plan).Active
	}, "services to start")
	if err := exec.Command(tmux.Path, "-S", layout.Socket, "new-session", "-d", "-s", "reet-native", "sleep", "300").Run(); err != nil {
		t.Fatal(err)
	}
	pid := panePID(t, tmux.Path, layout.Socket)
	firstAgent := mainPID(t, plan)
	if firstAgent == 0 {
		t.Fatal("agent service has no main process")
	}
	if err := syscall.Kill(firstAgent, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitForNative(t, 25*time.Second, func() bool {
		next := mainPID(t, plan)
		return next != 0 && next != firstAgent
	}, "agent service to restart")
	if !SocketActive(layout) || !processAlive(pid) {
		t.Fatal("killing the agent ended tmux or the running test process")
	}
	if sessions, err := Sessions(context.Background(), tmux, layout); err != nil || len(sessions) != 1 || sessions[0].Name != "reet-native" {
		t.Fatalf("tmux session did not survive: %+v %v", sessions, err)
	}
	if runtime.GOOS == "darwin" {
		if active := runServiceCommand(context.Background(), "launchctl", "print", "gui/"+strconv.Itoa(os.Getuid())+"/"+plan.TerminalLabel); active != nil {
			t.Fatal("terminal LaunchAgent stopped")
		}
	} else {
		if active, err := serviceActive(context.Background(), plan.TerminalName); err != nil || !active {
			t.Fatalf("terminal unit stopped: %t %v", active, err)
		}
	}
}

func panePID(t *testing.T, tmuxPath, socket string) int {
	t.Helper()
	output, err := exec.Command(tmuxPath, "-S", socket, "list-panes", "-t", "reet-native", "-F", "#{pane_pid}").Output()
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || pid <= 0 {
		t.Fatalf("pane pid: %q", output)
	}
	return pid
}

func mainPID(t *testing.T, plan ServicePlan) int {
	t.Helper()
	uid := strconv.Itoa(os.Getuid())
	if runtime.GOOS == "darwin" {
		output, err := exec.Command("launchctl", "print", "gui/"+uid+"/"+plan.AgentLabel).CombinedOutput()
		if err != nil {
			return 0
		}
		match := regexp.MustCompile(`(?m)^\s*pid = (\d+)$`).FindStringSubmatch(string(output))
		if len(match) != 2 {
			return 0
		}
		pid, _ := strconv.Atoi(match[1])
		return pid
	}
	output, err := exec.Command("systemctl", "--user", "show", plan.AgentName, "--property=MainPID", "--value").Output()
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return pid
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitForNative(t *testing.T, timeout time.Duration, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
