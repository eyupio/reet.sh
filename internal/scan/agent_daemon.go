//go:build linux || darwin

package scan

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/eyupio/reet.sh/internal/filelock"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"golang.org/x/sys/unix"
)

type agentRuntime struct {
	connected              atomic.Bool
	lastCycle              atomic.Int64
	started                int64
	version, servicePrefix string
	supervised             bool
}
type agentLocalStatus struct {
	PID           int    `json:"pid"`
	StartedAt     int64  `json:"startedAt"`
	Version       string `json:"version"`
	Connected     bool   `json:"privateConnected"`
	LastCycle     int64  `json:"lastCycleAt"`
	Supervised    bool   `json:"supervised"`
	ServicePrefix string `json:"servicePrefix"`
}

func (r *agentRuntime) snapshot() agentLocalStatus {
	return agentLocalStatus{os.Getpid(), r.started, r.version, r.connected.Load(), r.lastCycle.Load(), r.supervised, r.servicePrefix}
}

func daemonPaths(config string) (string, string, error) {
	sum := sha256.Sum256([]byte(config))
	dir := filepath.Join(filepath.Dir(config), "agent-"+hex.EncodeToString(sum[:6]))
	socket := filepath.Join(dir, "control.sock")
	if len(socket) > 100 {
		return "", "", errors.New("agent control socket path is too long; move enrollment to a shorter config directory")
	}
	return dir, socket, nil
}
func daemonDir(dir string) error {
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return errors.New("agent runtime directory must be private and owned by this user")
	}
	return nil
}
func localAgentCall(config, method, path string, out any) error {
	dir, socket, err := daemonPaths(config)
	if err != nil {
		return err
	}
	if err := daemonDir(dir); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer tr.CloseIdleConnections()
	req, _ := http.NewRequestWithContext(ctx, method, "http://agent.invalid"+path, nil)
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		return errors.New("agent local control is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("agent local control rejected request")
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(out)
	}
	return nil
}
func agentRunning(config string) (bool, error) {
	lock, err := filelock.Acquire(config + ".lock")
	if errors.Is(err, filelock.ErrLocked) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	lock.Close()
	return false, nil
}

// The control socket is reachable only through a private owner-checked local
// directory. Stop addresses this exact process, never a potentially reused PID.
func serveAgentControl(config string, state *agentRuntime, stop context.CancelFunc) (func(), error) {
	dir, socket, err := daemonPaths(config)
	if err != nil {
		return nil, err
	}
	if err = daemonDir(dir); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("agent control path is not a socket")
		}
		if err = os.Remove(socket); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(socket, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state.snapshot())
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		stop()
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: 2 * time.Second, MaxHeaderBytes: 1024}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	return func() { _ = server.Close(); <-done; _ = os.Remove(socket) }, nil
}
func notifyAgentReady() error {
	if os.Getenv("REET_AGENT_DAEMON_CHILD") != "1" {
		return nil
	}
	os.Unsetenv("REET_AGENT_DAEMON_CHILD")
	file := os.NewFile(3, "agent-ready")
	if file == nil {
		return errors.New("missing agent readiness pipe")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		return errors.New("invalid agent readiness pipe")
	}
	_, err = io.WriteString(file, "READY\n")
	return err
}

func launchAgentDaemon(config string, cmd *exec.Cmd) (int, error) {
	dir, _, err := daemonPaths(config)
	if err != nil {
		return 0, err
	}
	if err = daemonDir(dir); err != nil {
		return 0, err
	}
	logPath := filepath.Join(dir, "agent.log")
	if err = terminalhost.CheckEnrollmentConfig(logPath); err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	fd, err := unix.Open(logPath, unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return 0, err
	}
	log := os.NewFile(uintptr(fd), logPath)
	defer log.Close()
	if err = terminalhost.CheckEnrollmentConfig(logPath); err != nil {
		return 0, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	defer writer.Close()
	cmd.Stdin = nil
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.ExtraFiles = []*os.File{writer}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = append(os.Environ(), "REET_AGENT_DAEMON_CHILD=1")
	if err = cmd.Start(); err != nil {
		return 0, err
	}
	writer.Close()
	ready := make(chan bool, 1)
	go func() {
		line, _ := bufio.NewReader(io.LimitReader(reader, 64)).ReadString('\n')
		ready <- line == "READY\n"
	}()
	select {
	case ok := <-ready:
		if ok {
			pid := cmd.Process.Pid
			_ = cmd.Process.Release()
			return pid, nil
		}
	case <-time.After(5 * time.Second):
	}
	cmd.Process.Kill()
	_ = cmd.Wait()
	reader.Close()
	return 0, fmt.Errorf("background agent did not become ready; inspect %s", logPath)
}

func nativeAgentPlan(config, prefix string) (terminalhost.ServicePlan, bool, error) {
	c, err := loadAgentConfig(config)
	if err != nil {
		return terminalhost.ServicePlan{}, false, err
	}
	layout, err := terminalhost.NewEnrollmentLayout(config, c.ID)
	if err != nil {
		return terminalhost.ServicePlan{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	plan, err := terminalhost.PlanServices(ctx, terminalhost.ServiceOptions{Prefix: prefix, ConfigPath: config, Layout: layout, AllowMissingTmux: true})
	if err != nil {
		return plan, false, err
	}
	_, err = os.Lstat(plan.AgentPath)
	if os.IsNotExist(err) {
		return plan, false, nil
	}
	return plan, err == nil, err
}

func controlAgent(action, config, prefix string, out io.Writer) error {
	// Serialize CLI starts/restarts independently of the daemon's lifetime lock.
	startup, err := filelock.Acquire(config + ".start.lock")
	if err != nil {
		return fmt.Errorf("another agent lifecycle command is running: %w", err)
	}
	defer startup.Close()
	if action == "restart" {
		if err = stopLocalAgent(config, prefix, out); err != nil {
			return err
		}
		action = "start"
	}
	if action == "stop" {
		return stopLocalAgent(config, prefix, out)
	}
	running, err := agentRunning(config)
	if err != nil {
		return err
	}
	if running {
		fmt.Fprintln(out, "Agent is already running. Check it with: reet agent status")
		return nil
	}
	plan, installed, err := nativeAgentPlan(config, prefix)
	if err != nil {
		return err
	}
	if installed {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err = terminalhost.ControlAgentService(ctx, plan, "start"); err != nil {
			return err
		}
		if err = waitAgent(config, true); err != nil {
			return err
		}
		fmt.Fprintln(out, "Background agent service started. The terminal prompt is free; tmux sessions are preserved.")
		return nil
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	pid, err := launchAgentDaemon(config, exec.Command(binary, "agent", "run", "--foreground", "--config", config))
	if err != nil {
		return err
	}
	dir, _, _ := daemonPaths(config)
	fmt.Fprintf(out, "Background agent started (PID %d). You can close this terminal.\nStatus: reet agent status --config %q\nStop: reet agent stop --config %q\nLogs: %s\n", pid, config, config, filepath.Join(dir, "agent.log"))
	fmt.Fprintln(out, "To start automatically after login/reboot, install the native agent service.")
	return nil
}

// Installation owns the same start lock as lifecycle commands, so an old
// detached process cannot race the native service for the enrollment lock.
func installAgentServices(plan terminalhost.ServicePlan, out io.Writer) error {
	startup, err := filelock.Acquire(plan.ConfigPath + ".start.lock")
	if err != nil {
		return err
	}
	defer startup.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err = terminalhost.CheckInstall(ctx, plan); err != nil {
		return err
	}
	running, err := agentRunning(plan.ConfigPath)
	if err != nil {
		return err
	}
	if running {
		var state agentLocalStatus
		if err = localAgentCall(plan.ConfigPath, "GET", "/status", &state); err != nil {
			return errors.New("an older agent has no local control; stop that foreground process before installing services")
		}
		if state.Supervised && state.ServicePrefix != plan.Prefix {
			return errors.New("this enrollment is supervised under another service prefix; use its --service-prefix")
		}
		if !state.Supervised {
			fmt.Fprintln(out, "Switching the existing standalone agent to native supervision. Running tmux sessions are preserved.")
			if err = stopLocalAgent(plan.ConfigPath, plan.Prefix, out); err != nil {
				return err
			}
		}
	}
	if err = terminalhost.Install(ctx, plan); err != nil {
		return err
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var state agentLocalStatus
		if localAgentCall(plan.ConfigPath, "GET", "/status", &state) == nil && state.Supervised && state.ServicePrefix == plan.Prefix {
			if !terminalhost.ReadStatus(ctx, plan).Active {
				return errors.New("agent started but the terminal supervisor is not active; inspect service status")
			}
			fmt.Fprintln(out, "Verified: native agent and independent terminal supervisor are active.")
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("services were installed but the native agent did not become ready; no successful handoff was verified")
}
func localTerminalAgentNative(config string) (bool, error) {
	running, err := agentRunning(config)
	if err != nil || !running {
		return false, err
	}
	var state agentLocalStatus
	if err = localAgentCall(config, "GET", "/status", &state); err != nil {
		return false, errors.New("the existing agent has no local control; stop its older foreground process before continuing setup")
	}
	return state.Supervised, nil
}
func localTerminalAgentConnected(config string) bool {
	var state agentLocalStatus
	return localAgentCall(config, "GET", "/status", &state) == nil && state.Supervised && state.Connected
}
func stopLocalAgent(config, prefix string, out io.Writer) error {
	running, err := agentRunning(config)
	if err != nil {
		return err
	}
	if !running {
		fmt.Fprintln(out, "Agent is stopped.")
		return nil
	}
	var state agentLocalStatus
	if err = localAgentCall(config, "GET", "/status", &state); err != nil {
		return errors.New("agent is running without local control; stop the older foreground process once, then use reet agent start")
	}
	if state.Supervised {
		plan, installed, err := nativeAgentPlan(config, state.ServicePrefix)
		if err != nil {
			return err
		}
		if !installed {
			return errors.New("agent service definition is missing; inspect its service manager")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err = terminalhost.ControlAgentService(ctx, plan, "stop"); err != nil {
			return err
		}
	} else {
		if err = localAgentCall(config, "POST", "/stop", nil); err != nil {
			return err
		}
	}
	if err = waitAgent(config, false); err != nil {
		return err
	}
	fmt.Fprintln(out, "Agent stopped. Local terminal sessions continue running.")
	return nil
}
func waitAgent(config string, running bool) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := agentRunning(config)
		if err != nil {
			return err
		}
		if current == running {
			if !running {
				return nil
			}
			if localAgentCall(config, "GET", "/status", nil) == nil {
				return nil
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("agent did not reach the requested state; inspect its local logs")
}
func printAgentStatus(config string, out io.Writer) error {
	running, err := agentRunning(config)
	if err != nil {
		return err
	}
	if !running {
		fmt.Fprintln(out, "Agent: stopped\nPrivate connection: disconnected")
		return nil
	}
	var status agentLocalStatus
	if err = localAgentCall(config, "GET", "/status", &status); err != nil {
		fmt.Fprintln(out, "Agent: running (local control unavailable; older process)")
		return nil
	}
	connection := "reconnecting"
	if status.Connected {
		connection = "connected"
	}
	mode := "background/foreground process"
	if status.Supervised {
		mode = "native service"
	}
	fmt.Fprintf(out, "Agent: running (PID %d, %s)\nPrivate connection: %s\nStarted: %s\nVersion: %s\n", status.PID, mode, connection, time.UnixMilli(status.StartedAt).Format(time.RFC3339), status.Version)
	if status.LastCycle > 0 {
		fmt.Fprintf(out, "Last successful inventory/job cycle: %s\n", time.UnixMilli(status.LastCycle).Format(time.RFC3339))
	}
	return nil
}
