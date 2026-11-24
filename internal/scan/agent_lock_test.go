package scan

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func lockedMachine(t *testing.T, server string) string {
	t.Helper()
	config := enrolledMachine(t, server)
	c := loaded(t, config)
	c.Locked = true
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestAgentLockToggle(t *testing.T) {
	config := enrolledMachine(t, "https://example.invalid")
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := RunAgent(append(args, "--config", config), &out, "test"); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}
	if out := run("lock", "status"); !strings.Contains(out, "Locked on this machine: off") {
		t.Fatalf("status: %q", out)
	}
	if out := run("lock"); !strings.Contains(out, "reet agent unlock") {
		t.Fatalf("lock: %q", out)
	}
	// What is saved is what the agent reads again after a restart.
	if !loaded(t, config).Locked {
		t.Fatal("lock was not saved")
	}
	if out := run("lock", "status"); !strings.Contains(out, "Locked on this machine: on") {
		t.Fatalf("status: %q", out)
	}
	if out := run("status"); !strings.Contains(out, "Locked on this machine: on") {
		t.Fatalf("agent status: %q", out)
	}
	if out := run("unlock"); !strings.Contains(out, "Unlocked") {
		t.Fatalf("unlock: %q", out)
	}
	if loaded(t, config).Locked {
		t.Fatal("unlock was not saved")
	}
	if out := run("status"); !strings.Contains(out, "Locked on this machine: off") {
		t.Fatalf("agent status: %q", out)
	}
	for _, args := range [][]string{{"lock", "maybe"}, {"lock", "status", "extra"}, {"unlock", "status"}, {"lock", "on"}} {
		if err := RunAgent(append(args, "--config", config), io.Discard, "test"); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
	var help bytes.Buffer
	if err := RunAgent([]string{"lock", "--help"}, &help, "test"); err != nil || !strings.Contains(help.String(), "reet agent lock|unlock") {
		t.Fatalf("help: %q %v", help.String(), err)
	}
	if err := RunAgent(nil, io.Discard, "test"); err == nil || !strings.Contains(err.Error(), "lock|unlock") {
		t.Fatalf("usage: %v", err)
	}
}

func TestTheLockSurvivesFollowingAMove(t *testing.T) {
	config := lockedMachine(t, "https://old.example.com")
	next := honest(t)
	var out bytes.Buffer
	if !(&moveFollower{}).follow(context.Background(), movedRefusal(next.URL), loaded(t, config), config, &out, "test") {
		t.Fatalf("not followed: %s", out.String())
	}
	if now := loaded(t, config); !now.Locked || now.Server != next.URL {
		t.Fatalf("saved %+v", now)
	}
}

func TestTheLockSurvivesSetServer(t *testing.T) {
	config := lockedMachine(t, "https://old.example.com")
	withRestart(t, nil)
	next := honest(t)
	if out, err := setServer(t, config, next.URL); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !loaded(t, config).Locked {
		t.Fatal("set-server dropped the lock")
	}
}

func TestTheLockSurvivesEnrolAgain(t *testing.T) {
	c := newCodeController(t)
	config, _ := tokenEnrolled(t, c, false)
	saved := loaded(t, config)
	saved.Locked = true
	if err := writePrivate(config, saved); err != nil {
		t.Fatal(err)
	}
	if err := RunAgentInput([]string{"enroll", "--again", "--code", c.code(t), "--config", config}, strings.NewReader(""), io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	if !loaded(t, config).Locked {
		t.Fatal("enrol --again dropped the lock")
	}
}

// Running enrol again on an enrolled machine reuses the file it has.
func TestTheLockSurvivesEnrolIfNeeded(t *testing.T) {
	config := lockedMachine(t, "https://example.invalid")
	c := loaded(t, config)
	if err := RunAgent([]string{"enroll", "--if-needed", "--allow-apply", "--server", c.Server, "--env", c.Host, "--config", config, "--skip-terminal-setup"}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	if now := loaded(t, config); !now.Locked || !now.AllowApply {
		t.Fatalf("saved %+v", now)
	}
}

// A new build proving itself after an update writes only its own marker and
// result, never the enrolment, so the lock is still on when it runs.
func TestTheLockSurvivesAnUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"agent-1"}`))
	}))
	defer srv.Close()
	config := lockedMachine(t, srv.URL)
	c := loaded(t, config)
	if err := savePendingUpdate(config, pendingUpdate{JobID: "job-1", From: "v1.0.0", To: "v1.1.0", Target: "/tmp/reet", Previous: "/tmp/reet.previous"}); err != nil {
		t.Fatal(err)
	}
	if handled, err := checkPendingUpdate(c, config, "v1.1.0"); err != nil || !handled {
		t.Fatalf("check: %t %v", handled, err)
	}
	if !loaded(t, config).Locked {
		t.Fatal("the update dropped the lock")
	}
}

// The running agent's reader sees the lock as soon as it is saved, and a
// file it cannot read counts as locked.
func TestLockedNowReadsTheFileAndFailsClosed(t *testing.T) {
	config := enrolledMachine(t, "https://example.invalid")
	locked := lockedNow(config)
	if locked() {
		t.Fatal("an unlocked machine read as locked")
	}
	if err := RunAgent([]string{"lock", "--config", config}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	if !locked() {
		t.Fatal("the lock was not read again")
	}
	if !lockedNow(config + ".missing")() {
		t.Fatal("an unreadable enrolment read as unlocked")
	}
}
