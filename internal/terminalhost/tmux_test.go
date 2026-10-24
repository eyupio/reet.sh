package terminalhost

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestParseTmuxVersion(t *testing.T) {
	for _, tc := range []struct {
		value string
		major int
		minor int
		ok    bool
	}{
		{"tmux 3.2a", 3, 2, true},
		{"tmux 3.4", 3, 4, true},
		{"tmux 3.4a", 3, 4, true},
		{"tmux next-3.5", 3, 5, true},
		{"tmux 2.9", 2, 9, true},
		{"tmux", 0, 0, false},
		{"nonsense 3.4", 0, 0, false},
	} {
		major, minor, err := parseTmuxVersion(tc.value)
		if tc.ok && (err != nil || major != tc.major || minor != tc.minor) {
			t.Fatalf("%q: %d %d %v", tc.value, major, minor, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%q accepted", tc.value)
		}
	}
}

func TestSupportedTmuxVersion(t *testing.T) {
	if supportedTmux(3, 1) || supportedTmux(3, 0) || supportedTmux(2, 9) {
		t.Fatal("tmux older than 3.2 accepted")
	}
	if !supportedTmux(3, 2) || !supportedTmux(3, 3) || !supportedTmux(4, 0) {
		t.Fatal("supported tmux versions rejected")
	}
}

func TestFindTmuxAcceptsMinimumVersion(t *testing.T) {
	dir := t.TempDir()
	program := "#!/bin/sh\nprintf 'tmux 3.2a\\n'\n"
	if err := os.WriteFile(dir+"/tmux", []byte(program), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	tmux, err := FindTmux(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tmux.Version != "tmux 3.2a" {
		t.Fatalf("version: %q", tmux.Version)
	}
}

func TestTmuxSessionsAndLocalEnd(t *testing.T) {
	tmux, err := FindTmux(context.Background())
	if err != nil {
		t.Skip(err)
	}
	layout := privateLayout(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := exec.CommandContext(ctx, tmux.Path, "-D", "-S", layout.Socket, "-f", layout.Config)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
		_ = server.Wait()
	})
	deadline := time.Now().Add(3 * time.Second)
	for !SocketActive(layout) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !SocketActive(layout) {
		t.Fatal("tmux did not create its socket")
	}
	if err := exec.Command(tmux.Path, "-S", layout.Socket, "new-session", "-d", "-s", "reet-test", "sleep", "30").Run(); err != nil {
		t.Fatal(err)
	}
	sessions, err := Sessions(context.Background(), tmux, layout)
	if err != nil || len(sessions) != 1 || sessions[0].Name != "reet-test" || sessions[0].Created == 0 || sessions[0].Windows != 1 {
		t.Fatalf("sessions: %+v %v", sessions, err)
	}
	if err := EndSession(context.Background(), tmux, layout, "reet-test"); err != nil {
		t.Fatal(err)
	}
	sessions, err = Sessions(context.Background(), tmux, layout)
	if err != nil && !errors.Is(err, ErrNotRunning) {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("session survived local end: %+v", sessions)
	}
	if err := EndSession(context.Background(), tmux, layout, "-bad"); err == nil {
		t.Fatal("invalid session name accepted")
	}
}

func TestSessionsWithoutService(t *testing.T) {
	tmux, err := FindTmux(context.Background())
	if err != nil {
		t.Skip(err)
	}
	layout := privateLayout(t)
	if err := os.Remove(layout.Socket); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := Sessions(context.Background(), tmux, layout); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("missing service: %v", err)
	}
}

func TestEndSessionDoesNotResolvePrefixToAnotherSession(t *testing.T) {
	tmux, err := FindTmux(context.Background())
	if err != nil {
		t.Skip(err)
	}
	layout := privateLayout(t)
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
	for !SocketActive(layout) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := exec.Command(tmux.Path, "-S", layout.Socket, "new-session", "-d", "-s", "reet-longer", "sleep", "30").Run(); err != nil {
		t.Fatal(err)
	}
	if err := EndSession(context.Background(), tmux, layout, "reet"); err == nil {
		t.Fatal("prefix matched an unrelated session")
	}
	sessions, err := Sessions(context.Background(), tmux, layout)
	if err != nil || len(sessions) != 1 || sessions[0].Name != "reet-longer" {
		t.Fatal("unrelated local work ended", sessions, err)
	}
}

func TestReadinessDistinguishesConsentTmuxAndSupervisor(t *testing.T) {
	layout := privateLayout(t)
	if readiness := Readiness(context.Background(), layout, false); readiness != "disabled_locally" {
		t.Fatalf("disabled readiness: %s", readiness)
	}
	tmux, err := FindTmux(context.Background())
	if err != nil {
		t.Skip(err)
	}
	if readiness := Readiness(context.Background(), layout, true); readiness != "supervisor_unavailable" {
		t.Fatalf("missing service readiness: %s", readiness)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := exec.CommandContext(ctx, tmux.Path, "-D", "-S", layout.Socket, "-f", layout.Config)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
		_ = server.Wait()
	})
	deadline := time.Now().Add(3 * time.Second)
	for !SocketActive(layout) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if readiness := Readiness(context.Background(), layout, true); readiness != "ready" {
		t.Fatalf("running service readiness: %s", readiness)
	}
}
