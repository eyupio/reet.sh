//go:build linux || darwin

package terminalhost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/sync/poll" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"agent":{"host":"native","paused":true}}`)
			return
		}
		http.Error(w, "not configured", http.StatusServiceUnavailable)
	}))
	defer controller.Close()
	data, _ := json.Marshal(map[string]any{"server": controller.URL, "token": "native-test-token", "id": "native0123456789abcdef", "secret": "native-test-secret", "host": "native", "hostname": hostname, "home": home})
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	agentBinary := os.Getenv("REET_NATIVE_AGENT_BINARY")
	if agentBinary == "" {
		t.Fatal("REET_NATIVE_AGENT_BINARY must point to the built reet binary; a placeholder process does not verify real agent recovery")
	}
	layout, err := NewEnrollmentLayout(config, "native0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(layout); err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("reet-native-%d", os.Getpid())
	plan, err := PlanServices(context.Background(), ServiceOptions{
		Prefix: prefix, Binary: agentBinary, ConfigPath: config, Layout: layout,
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
	if !waitForNative(t, 20*time.Second, func() bool {
		return SocketActive(layout) && ReadStatus(context.Background(), plan).Active
	}) {
		debugServices(t, plan, layout)
		t.Fatal("timed out waiting for services to start")
	}
	// The real agent must have entered its run loop and acquired its lock before
	// it is killed. Then prove it stays up after the manager restarts it.
	if !waitForNative(t, 10*time.Second, func() bool { _, err := os.Stat(config + ".lock"); return err == nil }) {
		t.Fatal("real agent did not acquire enrollment lock")
	}
	if err := exec.Command(tmux.Path, "-S", layout.Socket, "new-session", "-d", "-s", "reet-native", "sleep", "300").Run(); err != nil {
		t.Fatal(err)
	}
	pid := panePID(t, tmux.Path, layout.Socket)
	if runtime.GOOS == "linux" {
		restricted, err := TerminalNoNewPrivileges(context.Background(), plan)
		if err != nil || restricted {
			t.Fatalf("native tmux prevents normal sudo: restricted=%t err=%v", restricted, err)
		}
		for _, process := range []int{pid, mainPID(t, plan)} {
			restricted, err := processNoNewPrivileges(process)
			if err != nil || restricted {
				t.Fatalf("native child inherited no-new-privileges: pid=%d restricted=%t err=%v", process, restricted, err)
			}
		}
	}

	// Refreshing agent definitions must preserve the independently running
	// server as well. This used to restart tmux as an install side effect.
	if err := Install(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if !processAlive(pid) || panePID(t, tmux.Path, layout.Socket) != pid {
		t.Fatal("service refresh killed the original process")
	}
	// Exercise the real CLI lifecycle too. Native stop must stop just the agent
	// (including launchd KeepAlive), and restart must preserve the same pane PID.
	if !waitForNative(t, 10*time.Second, func() bool {
		output, err := exec.Command(agentBinary, "agent", "status", "--config", config).CombinedOutput()
		return err == nil && strings.Contains(string(output), "Agent: running")
	}) {
		t.Fatal("agent local control did not become ready")
	}
	for _, action := range []string{"status", "stop", "start", "restart"} {
		cmd := exec.Command(agentBinary, "agent", action, "--config", config, "--service-prefix", prefix)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("agent %s: %s (%v)", action, output, err)
		}
		if action == "stop" && mainPID(t, plan) != 0 {
			t.Fatal("native service restarted after stop")
		}
		if !processAlive(pid) || panePID(t, tmux.Path, layout.Socket) != pid {
			t.Fatalf("agent %s killed original shell", action)
		}
	}
	firstAgent := mainPID(t, plan)
	// Reproduce an installed service competing with an older standalone agent.
	// CLI installation must hand off the enrollment lock without touching tmux.
	if err := ControlAgentService(context.Background(), plan, "stop"); err != nil {
		t.Fatal(err)
	}
	standalone := exec.Command(agentBinary, "agent", "run", "--foreground", "--config", config)
	standalone.Stdout = io.Discard
	standalone.Stderr = io.Discard
	if err := standalone.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { standalone.Process.Kill(); standalone.Wait() })
	if !waitForNative(t, 10*time.Second, func() bool {
		output, err := exec.Command(agentBinary, "agent", "status", "--config", config).CombinedOutput()
		return err == nil && strings.Contains(string(output), "background/foreground process")
	}) {
		t.Fatal("standalone regression fixture did not start")
	}
	output, err := exec.Command(agentBinary, "agent", "service", "install", "--config", config, "--service-prefix", prefix).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Verified: native agent") {
		t.Fatalf("standalone handoff: %s (%v)", output, err)
	}
	if !processAlive(pid) || panePID(t, tmux.Path, layout.Socket) != pid {
		t.Fatal("standalone migration ended tmux work")
	}
	firstAgent = mainPID(t, plan)
	if firstAgent == 0 {
		t.Fatal("agent service has no main process")
	}
	if err := syscall.Kill(firstAgent, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if !waitForNative(t, 25*time.Second, func() bool {
		next := mainPID(t, plan)
		return next != 0 && next != firstAgent
	}) {
		debugServices(t, plan, layout)
		t.Fatal("timed out waiting for the agent service to restart")
	}
	restarted := mainPID(t, plan)
	time.Sleep(2 * time.Second)
	if restarted == 0 || mainPID(t, plan) != restarted {
		debugServices(t, plan, layout)
		t.Fatal("real agent could not remain running after restart")
	}
	if !SocketActive(layout) || !processAlive(pid) {
		t.Fatal("killing the agent ended tmux or the running test process")
	}
	if sessions, err := Sessions(context.Background(), tmux, layout); err != nil || len(sessions) != 1 || sessions[0].Name != "reet-native" {
		t.Fatalf("tmux session did not survive: %+v %v", sessions, err)
	}
	if runtime.GOOS == "darwin" {
		if active := runServiceCommand(context.Background(), "launchctl", "print", macServiceDomain()+"/"+plan.TerminalLabel); active != nil {
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
	if runtime.GOOS == "darwin" {
		output, err := runServiceOutput(context.Background(), "launchctl", "print", macServiceDomain()+"/"+plan.AgentLabel)
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
	output, err := runServiceOutput(context.Background(), "systemctl", "--user", "show", plan.AgentName, "--property=MainPID", "--value")
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return pid
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitForNative(t *testing.T, timeout time.Duration, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func debugServices(t *testing.T, plan ServicePlan, layout Layout) {
	t.Helper()
	t.Logf("layout socket active=%t status=%+v", SocketActive(layout), ReadStatus(context.Background(), plan))
	if runtime.GOOS == "darwin" {
		for _, label := range []string{plan.AgentLabel, plan.TerminalLabel} {
			out, err := runServiceOutput(context.Background(), "launchctl", "print", macServiceDomain()+"/"+label)
			t.Logf("%s print err=%v:\n%s", label, err, out)
		}
		return
	}
	for _, name := range []string{plan.AgentName, plan.TerminalName} {
		out, err := runServiceOutput(context.Background(), "systemctl", "--user", "status", name, "--no-pager", "-l")
		t.Logf("%s status err=%v:\n%s", name, err, out)
		journal, jerr := runServiceOutput(context.Background(), "journalctl", "--user", "-u", name, "--no-pager", "-n", "40")
		if jerr == nil {
			t.Logf("%s journal:\n%s", name, journal)
		}
	}
}
