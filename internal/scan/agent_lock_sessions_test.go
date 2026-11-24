package scan

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type fakeSessions struct {
	t       *testing.T
	config  string
	count   int
	ended   int
	endedAt bool // the machine was locked when the sessions were ended
}

func (f *fakeSessions) running() (int, error) { return f.count, nil }
func (f *fakeSessions) endAll() (int, error) {
	f.endedAt = loaded(f.t, f.config).Locked
	n := f.count
	f.ended, f.count = n, 0
	return n, nil
}

func withSessions(t *testing.T, config string, count int) *fakeSessions {
	t.Helper()
	f := &fakeSessions{t: t, config: config, count: count}
	old := sessionsFor
	sessionsFor = func(string, agentConfig) machineSessions { return f }
	t.Cleanup(func() { sessionsFor = old })
	return f
}

const stillRunning = "2 sessions are still running and can run commands on this machine, including `reet agent unlock`. End them with `reet agent lock --end-sessions`, or from reet."

// A lock does not contain what already runs: the command says so and how
// many, and lock status counts them.
func TestLockSaysWhichSessionsStillRun(t *testing.T) {
	config := enrolledMachine(t, "https://example.invalid")
	withSessions(t, config, 2)
	var out bytes.Buffer
	if err := RunAgent([]string{"lock", "--config", config}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), stillRunning) {
		t.Fatalf("lock: %q", out.String())
	}
	out.Reset()
	if err := RunAgent([]string{"lock", "status", "--config", config}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Locked on this machine: on") || !strings.Contains(out.String(), "Sessions running: 2") {
		t.Fatalf("status: %q", out.String())
	}
	quiet := enrolledMachine(t, "https://example.invalid")
	withSessions(t, quiet, 0)
	out.Reset()
	if err := RunAgent([]string{"lock", "--config", quiet}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "still running") {
		t.Fatalf("lock with nothing running: %q", out.String())
	}
}

// --end-sessions locks first, so nothing new starts, then ends every
// session and chat and says how many.
func TestLockEndSessionsEndsThemAfterLocking(t *testing.T) {
	config := enrolledMachine(t, "https://example.invalid")
	f := withSessions(t, config, 2)
	var out bytes.Buffer
	if err := RunAgent([]string{"lock", "--end-sessions", "--config", config}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if f.ended != 2 || !f.endedAt || !loaded(t, config).Locked {
		t.Fatalf("ended %d, locked first %t", f.ended, f.endedAt)
	}
	if !strings.Contains(out.String(), "Ended 2 sessions on this machine.") || strings.Contains(out.String(), "still running") {
		t.Fatalf("output: %q", out.String())
	}
	for _, args := range [][]string{{"unlock", "--end-sessions"}, {"lock", "status", "--end-sessions"}, {"status", "--end-sessions"}} {
		if err := RunAgent(append(args, "--config", config), io.Discard, "test"); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}
