//go:build linux || darwin

package terminalagent

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// gatedGit runs real git, except that the first `status` waits for release
// (or for its context to end) after saying it started.
type gatedGit struct {
	once    sync.Once
	opened  sync.Once
	started chan struct{}
	release chan struct{}
}

func newGatedGit() *gatedGit {
	return &gatedGit{started: make(chan struct{}), release: make(chan struct{})}
}

func (g *gatedGit) open() { g.opened.Do(func() { close(g.release) }) }

func (g *gatedGit) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "status" {
		first := false
		g.once.Do(func() { first = true; close(g.started) })
		if first {
			select {
			case <-g.release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return terminalhost.ExecGit(ctx, dir, args...)
}

// connectorWorktree makes a real repository with one run worktree and a
// connector whose manager uses git.
func connectorWorktree(t *testing.T, git terminalhost.GitRunner) (func(context.Context, terminalproto.Message) error, replies, func(), string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH; the worktree tests need real git")
	}
	if _, err := terminalhost.FindTmux(t.Context()); err != nil {
		t.Skip(err)
	}
	base, err := os.MkdirTemp("/tmp", "reet-agent-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	if base, err = filepath.EvalSymlinks(base); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	repo := filepath.Join(home, "code", "site")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	gitT(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", "README")
	gitT(t, repo, "-c", "user.name=T", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "first")
	w, err := terminalhost.PrepareWorktree(t.Context(), home, repo, "p", testRun, terminalhost.ExecGit)
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(base, "agent.json")
	if err := os.WriteFile(config, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	id := "enrollment-0123456789"
	layout, err := terminalhost.NewEnrollmentLayout(config, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := terminalhost.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	got := make(replies, 16)
	headers := http.Header{}
	headers.Set(terminalproto.FeatureHeader, terminalproto.LaunchFeature+","+terminalproto.WorktreeFeature)
	handle, cleanup := Factory(terminalhost.ManagerOptions{EnrollmentID: id, Layout: layout, Home: home, Consent: func() error { return nil }, Git: git})(t.Context(), http.DefaultClient, headers, 1, got.send)
	return handle, got, cleanup, w.Path
}

func gitT(t *testing.T, dir string, args ...string) { t.Helper(); git(t, dir, args...) }

func message(id string, body terminalproto.Body) terminalproto.Message {
	return terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: id, Body: body}
}

// A worktree request returns the dispatcher at once and answers from its own
// goroutine; while it runs, a second worktree request is refused as busy
// immediately and other messages are still handled.
func TestWorktreeRequestRunsAsideAndASecondIsBusy(t *testing.T) {
	gate := newGatedGit()
	handle, got, cleanup, path := connectorWorktree(t, gate.run)
	defer cleanup()
	defer gate.open()
	status := &terminalproto.WorktreeStatus{Run: testRun, Project: "p", Repo: "site"}
	start := time.Now()
	if err := handle(t.Context(), message("request-first-00001", status)); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("the dispatcher waited for the worktree request")
	}
	select {
	case <-gate.started:
	case <-time.After(10 * time.Second):
		t.Fatal("status never reached git")
	}
	for i, body := range []terminalproto.Body{status, &terminalproto.WorktreeRemove{Run: testRun, Project: "p", Repo: "site"}} {
		id := []string{"request-second-0001", "request-third-00001"}[i]
		start := time.Now()
		if err := handle(t.Context(), message(id, body)); err != nil {
			t.Fatal(err)
		}
		select {
		case reply := <-got:
			if reply.RequestID != id || reply.Body != (terminalproto.WorktreeResult{Code: terminalproto.Conflict}) {
				t.Fatal("second worktree request", reply)
			}
			if _, err := terminalproto.Encode(reply); err != nil {
				t.Fatal("busy reply is not a valid message", err)
			}
		case <-time.After(time.Second):
			t.Fatal("a second worktree request was not refused at once")
		}
		if time.Since(start) > time.Second {
			t.Fatal("busy took too long")
		}
	}
	// Other messages are still served while the worktree request runs.
	if err := handle(t.Context(), message("request-reconcile-1", &terminalproto.Reconcile{})); err != nil {
		t.Fatal(err)
	}
	select {
	case reply := <-got:
		// No tmux server runs here, so the answer may be unavailable; what
		// matters is that it came while git was still held.
		if reply.RequestID != "request-reconcile-1" {
			t.Fatal("reconcile while a worktree request ran", reply)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("reconcile waited for the worktree request")
	}
	gate.open()
	select {
	case reply := <-got:
		if s, ok := reply.Body.(terminalproto.WorktreeState); !ok || reply.RequestID != "request-first-00001" || s.Path != path {
			t.Fatal("first worktree reply", reply)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the first worktree request never answered")
	}
	// The slot is free again.
	if body := worktreeCall(t, handle, got, status); body == (terminalproto.WorktreeResult{Code: terminalproto.Conflict}) {
		t.Fatal("the slot was not released")
	}
}

// When the connection ends, a running worktree request is cancelled, sends
// nothing, and cleanup waits for it.
func TestWorktreeRequestIsCancelledWithTheConnection(t *testing.T) {
	gate := newGatedGit()
	handle, got, cleanup, path := connectorWorktree(t, gate.run)
	ctx, cancel := context.WithCancel(t.Context())
	if err := handle(ctx, message("request-first-00001", &terminalproto.WorktreeStatus{Run: testRun, Project: "p", Repo: "site"})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.started:
	case <-time.After(10 * time.Second):
		t.Fatal("status never reached git")
	}
	cancel()
	done := make(chan struct{})
	go func() { cleanup(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cleanup did not wait for, or the request did not stop on, the closed connection")
	}
	select {
	case reply := <-got:
		t.Fatal("a cancelled worktree request still answered", reply)
	default:
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("a cancelled status touched the worktree", err)
	}
}
