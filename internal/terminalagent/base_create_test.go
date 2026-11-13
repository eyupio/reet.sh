//go:build linux || darwin

package terminalagent

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// basePair is a repository whose origin is https://github.com/o/r with a
// second repository standing in for the one on GitHub, one commit ahead. The
// GitRunner it gives the manager teaches git where that URL really is, and
// records every call, so a test can see whether a fetch was made at all.
type basePair struct {
	home, repo, remote, local, ahead string

	mu    sync.Mutex
	calls [][]string
}

func newBasePair(t *testing.T, home string) *basePair {
	t.Helper()
	remote := filepath.Join(home, "github")
	if err := os.MkdirAll(remote, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, remote, "init", "-q", "-b", "main")
	git(t, remote, "config", "user.name", "T")
	git(t, remote, "config", "user.email", "t@example.invalid")
	git(t, remote, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(remote, "README"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, remote, "add", "README")
	git(t, remote, "commit", "-q", "-m", "first")
	repo := filepath.Join(home, "code", "site")
	if err := os.MkdirAll(filepath.Dir(repo), 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, home, "clone", "-q", remote, repo)
	git(t, repo, "remote", "set-url", "origin", "https://github.com/o/r")
	p := &basePair{home: home, repo: repo, remote: remote, local: git(t, repo, "rev-parse", "HEAD")}
	// GitHub moves on, so a fetched base and the repository's own differ.
	if err := os.WriteFile(filepath.Join(remote, "NEW"), []byte("y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, remote, "add", "NEW")
	git(t, remote, "commit", "-q", "-m", "on-github")
	p.ahead = git(t, remote, "rev-parse", "HEAD")
	return p
}

func (p *basePair) git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	p.mu.Lock()
	p.calls = append(p.calls, slices.Clone(args))
	p.mu.Unlock()
	rewrite := []string{"-c", "url." + p.remote + ".insteadOf=https://github.com/o/r"}
	return terminalhost.ExecGit(ctx, dir, append(rewrite, args...)...)
}

func (p *basePair) fetched() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, args := range p.calls {
		if slices.Contains(args, "fetch") {
			return true
		}
	}
	return false
}

// fakeTool puts an executable of that name at the front of PATH, so a launch
// finds a binary without running a real agent.
func fakeTool(t *testing.T, base, name string) {
	t.Helper()
	bin := filepath.Join(base, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// agentWithTmux is the connector's dispatcher over a real manager and a real
// tmux server of its own, with the features a connection negotiated.
func agentWithTmux(t *testing.T, home, features string, git terminalhost.GitRunner) (func(context.Context, terminalproto.Message) error, replies) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH; this test needs real git")
	}
	tmux, err := terminalhost.FindTmux(t.Context())
	if err != nil {
		t.Skip(err)
	}
	config := filepath.Join(home, "agent.json")
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
	server := exec.Command(tmux.Path, "-D", "-S", layout.Socket, "-f", layout.Config)
	server.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command(tmux.Path, "-S", layout.Socket, "kill-server").Run()
		_ = server.Process.Kill()
		_ = server.Wait()
	})
	// The server answers only once it is up, and this is also the option a
	// test session runs under: a controlled non-login shell, as the manager's
	// own tests use, so the developer's startup files never reach it.
	ready := func() bool {
		return exec.Command(tmux.Path, "-S", layout.Socket, "set-option", "-g", "default-command", "PS1='t> ' exec /bin/sh -i").Run() == nil
	}
	deadline := time.Now().Add(10 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("the test's tmux server never answered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := make(replies, 32)
	headers := http.Header{}
	headers.Set(terminalproto.FeatureHeader, features)
	handle, cleanup := Factory(terminalhost.ManagerOptions{EnrollmentID: id, Layout: layout, Home: home, Consent: func() error { return nil }, Git: git})(
		t.Context(), http.DefaultClient, headers, 1, got.send)
	t.Cleanup(cleanup)
	return handle, got
}

// sessionChanged sends one create and returns the session_changed it is
// answered with, dropping the launch progress notes that come before it.
func sessionChanged(t *testing.T, handle func(context.Context, terminalproto.Message) error, got replies, in terminalproto.Create) terminalproto.SessionChanged {
	t.Helper()
	msg := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: "request-0123456789", SessionID: "session-0123456789", Body: &in}
	if err := msg.Validate(); err != nil {
		t.Fatal("request is not a valid message", err)
	}
	if err := handle(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(90 * time.Second)
	for {
		select {
		case reply := <-got:
			switch body := reply.Body.(type) {
			case terminalproto.LaunchProgress:
				// A stage note, sent before the one answer; this test is
				// about the answer.
				continue
			case terminalproto.SessionChanged:
				if _, err := terminalproto.Encode(reply); err != nil {
					t.Fatalf("reply is not a valid message: %+v", reply.Body)
				}
				return body
			default:
				t.Fatalf("create was answered with %#v", body)
			}
		case <-deadline:
			t.Fatal("no answer to the create")
		}
	}
}

// A create on a connection that negotiated base-fresh-v1 fetches its base and
// reports it, with launch progress negotiated as well — which is how the
// controller connects. The progress context must be built from the create's
// own, or it drops the negotiated base-fresh and every worktree create
// quietly goes back to the old lookup.
func TestWorktreeCreateCarriesBaseFreshAlongsideProgress(t *testing.T) {
	const launchWorktree = terminalproto.LaunchFeature + "," + terminalproto.WorktreeFeature
	for _, tc := range []struct {
		name     string
		features string
		fresh    bool
	}{
		{"base-fresh and launch progress", launchWorktree + "," + terminalproto.BaseFreshFeature + "," + terminalproto.ProgressFeature, true},
		{"launch progress alone", launchWorktree + "," + terminalproto.ProgressFeature, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, err := os.MkdirTemp("/tmp", "reet-base-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(base) })
			if base, err = filepath.EvalSymlinks(base); err != nil {
				t.Fatal(err)
			}
			home := filepath.Join(base, "home")
			if err := os.MkdirAll(home, 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", home)
			fakeTool(t, base, "claude")
			pair := newBasePair(t, home)
			handle, got := agentWithTmux(t, home, tc.features, pair.git)
			changed := sessionChanged(t, handle, got, terminalproto.Create{
				Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Dir: pair.repo, Worktree: testRun, Project: "p"})
			if changed.WorktreePath == "" || changed.State != terminalproto.Running {
				t.Fatalf("create = %+v", changed)
			}
			head := git(t, changed.WorktreePath, "rev-parse", "HEAD")
			if !tc.fresh {
				if pair.fetched() {
					t.Fatalf("a create without base-fresh-v1 reached github.com: %q", pair.calls)
				}
				if changed.Base != nil {
					t.Fatalf("a create without base-fresh-v1 reported a base: %+v", changed.Base)
				}
				if head != pair.local {
					t.Fatalf("worktree starts at %s, want the repository's own %s", head, pair.local)
				}
				return
			}
			if !pair.fetched() {
				t.Fatalf("the base was not fetched: %q", pair.calls)
			}
			if changed.Base == nil {
				t.Fatal("the create reported no base")
			}
			if changed.Base.Relation != terminalproto.BaseBehind || changed.Base.Start != pair.ahead {
				t.Fatalf("base = %+v, want behind starting from GitHub's %s", changed.Base, pair.ahead)
			}
			if head != pair.ahead {
				t.Fatalf("worktree starts at %s, want GitHub's %s", head, pair.ahead)
			}
			if changed.Base.Base != "main" {
				t.Fatalf("base = %+v, want the repository's default branch", changed.Base)
			}
		})
	}
}
