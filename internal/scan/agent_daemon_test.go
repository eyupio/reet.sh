//go:build linux || darwin

package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/filelock"
	"golang.org/x/sys/unix"
)

func TestAgentDaemonSubprocess(t *testing.T) {
	if config := os.Getenv("REET_TEST_AGENT_DAEMON_CONFIG"); config != "" {
		err := RunAgent([]string{"run", "--foreground", "--config", config}, io.Discard, "daemon-test")
		if err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("/tmp", "reet-daemon-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	pollEntered := make(chan struct{}, 1)
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/sync/poll" {
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case pollEntered <- struct{}{}:
			default:
			}
			<-r.Context().Done()
			return
		}
		http.Error(w, "private relay unavailable", 503)
	}))
	defer controller.Close()
	config := filepath.Join(dir, "agent.json")
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	if err = writePrivate(config, agentConfig{Server: controller.URL, ID: "daemon0123456789abcdef", Secret: "test-secret", Host: "daemon", Hostname: hostname, Home: home}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REET_TEST_AGENT_DAEMON_CONFIG", config)
	binary, _ := os.Executable()
	cmd := exec.Command(binary, "-test.run=^TestAgentDaemonSubprocess$")
	start := time.Now()
	pid, err := launchAgentDaemon(config, cmd)
	if err != nil {
		t.Fatal(err)
	}
	process, _ := os.FindProcess(pid)
	defer process.Wait()
	defer func() { _ = localAgentCall(config, "POST", "/stop", nil) }()
	if time.Since(start) > 5*time.Second {
		t.Fatal("daemon start blocked the calling terminal")
	}
	sid, err := unix.Getsid(pid)
	if err != nil || sid != pid {
		t.Fatal("daemon still belongs to caller's terminal session", sid, err)
	}
	var status agentLocalStatus
	if err = localAgentCall(config, "GET", "/status", &status); err != nil || status.PID != pid {
		t.Fatal("daemon status missing", status, err)
	}
	if status.Connected {
		t.Fatal("failed private bootstrap advertised a live connection")
	}
	select {
	case <-pollEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not begin background polling")
	}
	var output bytes.Buffer
	if err = RunAgent([]string{"status", "--config", config}, &output, "test"); err != nil || !strings.Contains(output.String(), "running") {
		t.Fatal(output.String(), err)
	}
	// Starting twice must not launch another process or fight for the lifetime lock.
	output.Reset()
	if err = RunAgent([]string{"start", "--config", config}, &output, "test"); err != nil || !strings.Contains(output.String(), "already running") {
		t.Fatal(output.String(), err)
	}
	if err = RunAgent([]string{"stop", "--config", config}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	lock, err := filelock.Acquire(config + ".lock")
	if err != nil {
		t.Fatal("stopped daemon held its lock", err)
	}
	lock.Close()
	output.Reset()
	if err = RunAgent([]string{"status", "--config", config}, &output, "test"); err != nil || !strings.Contains(output.String(), "stopped") {
		t.Fatal(output.String(), err)
	}
	dirPath, _, _ := daemonPaths(config)
	info, err := os.Stat(filepath.Join(dirPath, "agent.log"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("daemon log was not private", err)
	}
}

func TestAgentControlNeverSignalsAnUnverifiedPID(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "reet-control-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	config := filepath.Join(dir, "agent.json")
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	if err = writePrivate(config, agentConfig{Hostname: hostname, Home: home}); err != nil {
		t.Fatal(err)
	}
	lock, err := filelock.Acquire(config + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = stopLocalAgent(config, "reet", io.Discard); err == nil || !strings.Contains(err.Error(), "without local control") {
		t.Fatal("unsafe stop accepted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &agentRuntime{started: time.Now().UnixMilli(), version: "test"}
	closeControl, err := serveAgentControl(config, state, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer closeControl()
	state.connected.Store(true)
	var status agentLocalStatus
	if err = localAgentCall(config, "GET", "/status", &status); err != nil || !status.Connected {
		t.Fatal(status, err)
	}
	if err = localAgentCall(config, "POST", "/stop", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel exact process")
	}
	raw, _ := json.Marshal(status)
	if strings.Contains(string(raw), "secret") {
		t.Fatal("credentials entered local status")
	}
}
