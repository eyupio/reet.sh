//go:build linux || darwin

package terminalhost

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const testEnrollment = "enrollment0123456789abcdef"

func testSessionID(n int) string { return fmt.Sprintf("session0123456789-%08d", n) }
func testRequestID(n int) string { return fmt.Sprintf("request0123456789-%08d", n) }

func managerFixture(t *testing.T) (*Manager, ManagerOptions) {
	t.Helper()
	tmux, err := FindTmux(context.Background())
	if err != nil {
		t.Skip(err)
	}
	layout := privateLayout(t)
	home := shortTempDir(t)
	server := exec.Command(tmux.Path, "-D", "-S", layout.Socket, "-f", layout.Config)
	server.Env = validateEnvironment(append(safeEnvironment(), "HOME="+home))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
		_ = server.Process.Kill()
		_ = server.Wait()
	})
	deadline := time.Now().Add(3 * time.Second)
	for !responsive(context.Background(), tmux, layout) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	opts := ManagerOptions{EnrollmentID: testEnrollment, Layout: layout, Home: home, Consent: func() error { return nil }}
	m, err := NewManager(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	// A controlled non-login shell keeps tests independent of developer/runner
	// startup files. Production Create has no command field and never does this.
	if _, err := m.command(context.Background(), 0, "set-option", "-g", "default-command", "PS1='stage5-ready> ' exec /bin/sh -i"); err != nil {
		t.Fatal(err)
	}
	return m, opts
}

func TestManagerProcessDeathRecoversSameShell(t *testing.T) {
	if dir := os.Getenv("REET_TEST_MANAGER_CHILD"); dir != "" {
		layout := Layout{Dir: dir, Socket: dir + "/tmux.sock", Config: dir + "/tmux.conf", Logs: dir + "/logs"}
		m, err := NewManager(context.Background(), ManagerOptions{EnrollmentID: testEnrollment, Layout: layout, Home: os.Getenv("REET_TEST_MANAGER_HOME"), Consent: func() error { return nil }})
		if err != nil {
			fmt.Fprintln(os.Stdout, "failed")
			os.Exit(2)
		}
		if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), terminalproto.Create{Title: "Build", Cols: 80, Rows: 24}); err != nil {
			fmt.Fprintln(os.Stdout, "failed")
			os.Exit(2)
		}
		out, err := m.command(context.Background(), 1024, "list-panes", "-t", "="+m.name(testSessionID(1)), "-F", "#{pane_pid}")
		if err != nil {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stdout, strings.TrimSpace(string(out)))
		select {}
	}
	m, opts := managerFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestManagerProcessDeathRecoversSameShell$")
	child.Env = append(os.Environ(), "REET_TEST_MANAGER_CHILD="+opts.Layout.Dir, "REET_TEST_MANAGER_HOME="+opts.Home)
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	reader := bufio.NewScanner(output)
	if !reader.Scan() || reader.Text() == "failed" {
		t.Fatal("manager child failed to create shell")
	}
	pid := reader.Text()
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	s, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), terminalproto.Create{Title: "Build", Cols: 80, Rows: 24})
	if err != nil || s.State != terminalproto.Running || managedPID(t, m, s.ID) != pid {
		t.Fatal("killing manager replaced process", s, err)
	}
}

func TestConcurrentManagersReserveOneQuotaAndPreserveReceipts(t *testing.T) {
	m, opts := managerFixture(t)
	second, err := NewManager(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 1; i <= 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			manager := m
			if i%2 == 0 {
				manager = second
			}
			_, err := manager.Create(context.Background(), testRequestID(i), testSessionID(i), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrSessionCapacity) {
			t.Fatal(err)
		}
	}
	if accepted != 8 {
		t.Fatal("concurrent quota reservation", accepted)
	}
	sessions, err := m.List(context.Background())
	if err != nil || len(sessions) != 8 {
		t.Fatal("registry lost a concurrent writer", len(sessions), err)
	}
}

func TestInvalidRegistryFailsClosedAndPreservesLocalWork(t *testing.T) {
	m, opts := managerFixture(t)
	s := createManaged(t, m, 1)
	pid := managedPID(t, m, s.ID)
	if err := os.WriteFile(m.registryPath(), []byte(`{"version":1,"enrollmentId":"foreign"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.History(context.Background(), s.ID, 10); !errors.Is(err, ErrRegistry) {
		t.Fatal("invalid registry accepted", err)
	}
	if other, err := NewManager(context.Background(), opts); !errors.Is(err, ErrRegistry) {
		if other != nil {
			other.Close()
		}
		t.Fatal("invalid state replaced", err)
	}
	if managedPID(t, m, s.ID) != pid {
		t.Fatal("bad metadata killed local work")
	}
}
func createManaged(t *testing.T, m *Manager, n int) ManagedSession {
	t.Helper()
	s, err := m.Create(context.Background(), testRequestID(n), testSessionID(n), terminalproto.Create{Title: "Build", Cols: 80, Rows: 24})
	if err != nil || s.State != terminalproto.Running {
		t.Fatalf("create: %+v %v", s, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		out, err := m.History(context.Background(), s.ID, 100)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "stage5-ready>") {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("controlled shell did not become ready")
	return s
}
func managedPID(t *testing.T, m *Manager, id string) string {
	t.Helper()
	out, err := m.command(context.Background(), 1024, "list-panes", "-t", "="+m.name(id), "-F", "#{pane_pid}")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestManagerRetryAndRestartKeepOriginalProcess(t *testing.T) {
	m, opts := managerFixture(t)
	s := createManaged(t, m, 1)
	pid := managedPID(t, m, s.ID)
	if err := m.Rename(context.Background(), s.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	_ = m.Close()
	next, err := NewManager(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	again, err := next.Create(context.Background(), testRequestID(1), s.ID, terminalproto.Create{Title: "Build", Cols: 80, Rows: 24})
	if err != nil || again.ID != s.ID || again.Title != "Renamed" || managedPID(t, next, s.ID) != pid {
		t.Fatal("retry/restart replaced the shell", again, err)
	}
	names, err := next.sessionNames(context.Background())
	if err != nil || len(names) != 1 {
		t.Fatal("duplicate shell", names, err)
	}
	if _, err := next.Create(context.Background(), testRequestID(1), s.ID, terminalproto.Create{Title: "Changed", Cols: 80, Rows: 24}); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("changed request accepted", err)
	}
	ended, err := next.End(context.Background(), testRequestID(2), s.ID)
	if err != nil || ended.State != terminalproto.Ended {
		t.Fatal("end not confirmed", ended, err)
	}
	if _, err := next.Attach(context.Background(), s.ID, AttachOptions{ID: testRequestID(3), Generation: 1, Cols: 80, Rows: 24, Lease: time.Second}); !errors.Is(err, ErrSessionMissing) {
		t.Fatal("attached/created a missing session", err)
	}
}

func TestManagerUncertainCreationReconcilesWithoutRecreating(t *testing.T) {
	m, opts := managerFixture(t)
	s := createManaged(t, m, 1)
	pid := managedPID(t, m, s.ID)
	// Model process death after tmux created the shell but before the receipt
	// was committed. Reopening must recover the existing process.
	r, err := m.loadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	s.State = terminalproto.Creating
	s.PaneID = ""
	r.Sessions[s.ID] = s
	key := receiptKey("create", testRequestID(1))
	op := r.Operations[key]
	op.Complete = false
	r.Operations[key] = op
	if err := m.saveRegistry(r); err != nil {
		t.Fatal(err)
	}
	m.Close()
	next, err := NewManager(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if managedPID(t, next, s.ID) != pid {
		t.Fatal("lost ACK duplicated process")
	}
	// Once actual tmux state is absent, a retry must return Lost, not run new-session.
	if _, err := next.command(context.Background(), 0, "kill-session", "-t", "="+next.name(s.ID)); err != nil {
		t.Fatal(err)
	}
	again, err := next.Create(context.Background(), testRequestID(1), s.ID, terminalproto.Create{Title: "Build", Cols: 80, Rows: 24})
	if err != nil || again.State != terminalproto.Lost {
		t.Fatal("missing session was resurrected", again, err)
	}
	names, err := next.sessionNames(context.Background())
	if err != nil || len(names) != 0 {
		t.Fatal("created a replacement", names, err)
	}
}

func TestManagerConsentQuotaAndEnrollmentIsolation(t *testing.T) {
	m, opts := managerFixture(t)
	for i := 1; i <= 8; i++ {
		createManaged(t, m, i)
	}
	if _, err := m.Create(context.Background(), testRequestID(9), testSessionID(9), terminalproto.Create{Title: "Over quota", Cols: 80, Rows: 24}); !errors.Is(err, ErrSessionCapacity) {
		t.Fatal("quota exceeded", err)
	}
	m.Close()
	opts.Consent = func() error { return ErrTerminalDisabled }
	disabled, err := NewManager(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer disabled.Close()
	if _, err := disabled.History(context.Background(), testSessionID(1), 20); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatal("history bypassed consent", err)
	}
	if _, err := disabled.End(context.Background(), testRequestID(10), testSessionID(1)); err != nil {
		t.Fatal("pending end blocked by disable", err)
	}
	opts.EnrollmentID = "replacement0123456789abcdef"
	if other, err := NewManager(context.Background(), opts); !errors.Is(err, ErrRegistry) {
		if other != nil {
			other.Close()
		}
		t.Fatal("replacement adopted registry", err)
	}
}

func TestManagerExitCodeAndCleanup(t *testing.T) {
	m, _ := managerFixture(t)
	s := createManaged(t, m, 1)
	// send-keys is test-only: production creation never accepts a shell command.
	if _, err := m.command(context.Background(), 0, "send-keys", "-t", s.PaneID, "exit 7", "Enter"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := m.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		sessions, err := m.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if sessions[0].State == terminalproto.Exited {
			// tmux may report a dead pane without an exit status (including
			// signal/PTY shutdown races). Unknown is explicitly nullable.
			if sessions[0].ExitCode != nil && *sessions[0].ExitCode != 7 {
				t.Fatal("exit code missing", sessions)
			}
			names, err := m.sessionNames(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(names) != 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	sessions, _ := m.List(context.Background())
	panes, _ := m.panes(context.Background(), s.ID)
	t.Fatalf("shell exit was not observed: sessions=%+v panes=%+v", sessions, panes)
}

const tmuxCallEnd = "--reet-test-call-end--"

// recordingTmux wraps the real tmux binary and appends each invocation's
// arguments, NUL-separated, to a log, so tests see the exact argv Create sends.
func recordingTmux(t *testing.T, m *Manager) func() [][]string {
	t.Helper()
	dir := shortTempDir(t)
	log := filepath.Join(dir, "tmux.log")
	script := "#!/bin/sh\nprintf '%s\\0' \"$@\" >> '" + log + "'\nprintf '%s\\0' '" + tmuxCallEnd + "' >> '" + log + "'\nexec '" + m.tmux.Path + "' \"$@\"\n"
	wrapper := filepath.Join(dir, "tmux")
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	m.tmux.Path = wrapper
	return func() [][]string {
		data, err := os.ReadFile(log)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		calls, call := [][]string{}, []string{}
		for _, arg := range strings.Split(string(data), "\x00") {
			if arg == tmuxCallEnd {
				calls = append(calls, call)
				call = []string{}
			} else {
				call = append(call, arg)
			}
		}
		return calls
	}
}

func newSessionCalls(calls [][]string) [][]string {
	out := [][]string{}
	for _, c := range calls {
		if len(c) > 2 && c[2] == "new-session" {
			out = append(out, c)
		}
	}
	return out
}

// fakeAgent is a recording stand-in for an agent CLI: it writes its argv
// (NUL-separated) and working directory, then stays alive like a TUI.
func fakeAgent(t *testing.T) (path, record string) {
	t.Helper()
	dir := shortTempDir(t)
	record = filepath.Join(dir, "argv")
	path = filepath.Join(dir, "claude")
	script := "#!/bin/sh\nprintf '%s\\0' \"$@\" > '" + record + ".tmp'\npwd >> '" + record + ".tmp'\nmv '" + record + ".tmp' '" + record + "'\nexec sleep 60\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path, record
}

func TestManagerPlainCreateArgumentsUnchanged(t *testing.T) {
	m, opts := managerFixture(t)
	m.lookPath = func(string) (string, error) { t.Fatal("a plain create must not look up any agent"); return "", nil }
	calls := recordingTmux(t, m)
	s, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil || s.State != terminalproto.Running {
		t.Fatal(s, err)
	}
	created := newSessionCalls(calls())
	want := []string{"-S", opts.Layout.Socket, "new-session", "-d", "-s", m.name(testSessionID(1)), "-x", "80", "-y", "24", "-c", opts.Home, "-e", "HOME=" + opts.Home}
	if len(created) != 1 || !reflect.DeepEqual(created[0], want) {
		t.Fatalf("plain create arguments changed:\n got %q\nwant %q", created, want)
	}
}

func TestManagerLaunchAppendsPresetAfterEnvironment(t *testing.T) {
	m, opts := managerFixture(t)
	agent, record := fakeAgent(t)
	m.lookPath = func(name string) (string, error) {
		if name == "claude" {
			return agent, nil
		}
		return "", errors.New("not found")
	}
	calls := recordingTmux(t, m)
	work := filepath.Join(opts.Home, "work", "site")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	realWork, _ := filepath.EvalSymlinks(work)
	prompt := "x -- --help \"q\" $(id)\nand -rf"
	in := terminalproto.Create{Title: "Claude", Cols: 100, Rows: 30, Tool: "claude", Model: "sonnet", Dir: work + "/", Prompt: prompt}
	s, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), in)
	if err != nil || s.State != terminalproto.Running {
		t.Fatal(s, err)
	}
	want := []string{"-S", opts.Layout.Socket, "new-session", "-d", "-s", m.name(testSessionID(1)), "-x", "100", "-y", "30", "-c", realWork, "-e", "HOME=" + opts.Home, agent, "--model", "sonnet", "--", prompt}
	created := newSessionCalls(calls())
	if len(created) != 1 || !reflect.DeepEqual(created[0], want) {
		t.Fatalf("launch arguments:\n got %q\nwant %q", created, want)
	}
	var data []byte
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err = os.ReadFile(record); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("fake agent did not start", err)
	}
	gotArgv := strings.Split(string(data), "\x00")
	wantArgv := []string{"--model", "sonnet", "--", prompt, realWork + "\n"}
	if !reflect.DeepEqual(gotArgv, wantArgv) {
		t.Fatalf("agent argv/cwd:\n got %q\nwant %q", gotArgv, wantArgv)
	}
	// A retry with identical fields starts nothing new.
	again, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), in)
	if err != nil || again.ID != s.ID || len(newSessionCalls(calls())) != 1 {
		t.Fatal("retry started a second process", again, err)
	}
	changed := in
	changed.Prompt = "something else entirely"
	if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), changed); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("changed launch fields must conflict", err)
	}
}

func TestManagerRefusedLaunchLeavesNothingBehind(t *testing.T) {
	m, opts := managerFixture(t)
	agent, _ := fakeAgent(t)
	m.lookPath = func(name string) (string, error) {
		if name == "claude" {
			return agent, nil
		}
		return "", errors.New("not found")
	}
	calls := recordingTmux(t, m)
	outside := shortTempDir(t)
	if err := os.Symlink(outside, filepath.Join(opts.Home, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.Home, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	refused := []terminalproto.Create{
		{Title: "a", Cols: 80, Rows: 24, Tool: "claude", Dir: filepath.Join(opts.Home, "escape")},
		{Title: "a", Cols: 80, Rows: 24, Tool: "claude", Dir: filepath.Join(opts.Home, "file")},
		{Title: "a", Cols: 80, Rows: 24, Tool: "claude", Dir: filepath.Join(opts.Home, "missing")},
		{Title: "a", Cols: 80, Rows: 24, Tool: "claude", Dir: outside},
		{Title: "a", Cols: 80, Rows: 24, Tool: "gemini"},
		{Title: "a", Cols: 80, Rows: 24, Tool: "claude", Prompt: "purge"},
	}
	for i, in := range refused {
		_, err := m.Create(context.Background(), testRequestID(i+1), testSessionID(i+1), in)
		if !errors.Is(err, ErrLaunchUnavailable) || LaunchResultCode(err) != terminalproto.LaunchFailed {
			t.Fatal("launch must be refused as launch_failed", in, err)
		}
		names, nerr := m.sessionNames(context.Background())
		if nerr != nil || names[m.name(testSessionID(i+1))] {
			t.Fatal("a refused launch left a tmux session", nerr)
		}
	}
	if sessions, err := m.List(context.Background()); err != nil || len(sessions) != 0 {
		t.Fatal("a refused launch left a registry entry", sessions, err)
	}
	if created := newSessionCalls(calls()); len(created) != 0 {
		t.Fatal("a refused launch reached tmux new-session", created)
	}
	// The request ids were never recorded, so a corrected retry may proceed.
	r, err := m.loadRegistry()
	if err != nil || len(r.Operations) != 0 {
		t.Fatal("a refused launch saved an operation receipt", r.Operations, err)
	}
}

func TestManagerLaunchDirectoryIsNotFormatExpanded(t *testing.T) {
	m, opts := managerFixture(t)
	agent, record := fakeAgent(t)
	m.lookPath = func(name string) (string, error) {
		if name == "claude" {
			return agent, nil
		}
		return "", errors.New("not found")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(opts.Home, "a#(touch reet-format-marker)#S#{session_name}#H")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	realFolder, _ := filepath.EvalSymlinks(folder)
	markers := []string{filepath.Join(opts.Home, "reet-format-marker"), filepath.Join(cwd, "reet-format-marker"), filepath.Join(folder, "reet-format-marker"), filepath.Join(opts.Layout.Dir, "reet-format-marker"), "/reet-format-marker"}
	t.Cleanup(func() {
		for _, mk := range markers {
			_ = os.Remove(mk)
		}
	})
	s, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "claude", Dir: folder})
	if err != nil || s.State != terminalproto.Running {
		t.Fatal(s, err)
	}
	var data []byte
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err = os.ReadFile(record); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("fake agent did not start", err)
	}
	// Give a format job, had one been started, time to run.
	time.Sleep(300 * time.Millisecond)
	for _, mk := range markers {
		if _, err := os.Stat(mk); err == nil {
			t.Fatal("tmux ran a format job from the folder name", mk)
		}
	}
	// With no arguments the fake writes one empty argv field before its cwd.
	fields := strings.Split(strings.TrimSuffix(string(data), "\n"), "\x00")
	if got := fields[len(fields)-1]; len(fields) != 2 || got != realFolder {
		t.Fatalf("agent working directory:\n got %q\nwant %q", got, realFolder)
	}
}
