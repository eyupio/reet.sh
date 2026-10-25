//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const testRun = "0123456789abcdef0123456789abcdef"

func TestWorktreeResultMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want terminalproto.WorktreeResult
	}{
		{nil, terminalproto.WorktreeResult{Code: terminalproto.OK}},
		{fmt.Errorf("x: %w", terminalhost.ErrWorktreeNotFound), terminalproto.WorktreeResult{Code: terminalproto.NotFound}},
		{terminalhost.ErrWorktreeChanged, terminalproto.WorktreeResult{Code: terminalproto.WorktreeChanged}},
		{&terminalhost.WorktreeError{Reason: terminalproto.WorktreeHasChanges}, terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeHasChanges}},
		{&terminalhost.WorktreeError{Reason: terminalproto.WorktreeGitFailed, Detail: "fatal: locked"}, terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeGitFailed, Detail: "fatal: locked"}},
		{terminalhost.ErrTerminalDisabled, terminalproto.WorktreeResult{Code: terminalproto.Denied}},
		{errors.New("other"), terminalproto.WorktreeResult{Code: terminalproto.Unavailable}},
	} {
		if got := worktreeResult(tc.err); got != tc.want {
			t.Fatal(tc.err, got)
		}
	}
}

// replies collects what the connector sends; worktree answers arrive from
// their own goroutine.
type replies chan terminalproto.Message

func (r replies) send(_ context.Context, m terminalproto.Message) error {
	r <- m
	return nil
}

func worktreeCall(t *testing.T, handle func(context.Context, terminalproto.Message) error, got replies, body terminalproto.Body) terminalproto.Body {
	t.Helper()
	msg := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: "request-0123456789", Body: body}
	if err := msg.Validate(); err != nil {
		t.Fatal("request is not a valid message", err)
	}
	if err := handle(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
	var reply terminalproto.Message
	select {
	case reply = <-got:
	case <-time.After(30 * time.Second):
		t.Fatal("no reply")
	}
	if _, err := terminalproto.Encode(reply); err != nil {
		t.Fatalf("reply is not a valid message: %+v", reply.Body)
	}
	if reply.RequestID != msg.RequestID || reply.SessionID != "" {
		t.Fatal("reply envelope", reply)
	}
	return reply.Body
}

// A connection that did not negotiate worktree-v1 is refused plainly and
// never reaches the manager; one that did gets a worktree_result even when
// the manager is unavailable.
func TestWorktreeMessagesRequireNegotiatedFeature(t *testing.T) {
	for _, tc := range []struct {
		features string
		want     terminalproto.ResultCode
	}{
		{"", terminalproto.Denied},
		{terminalproto.LaunchFeature, terminalproto.Denied},
		{terminalproto.CommandHistoryFeature + "," + terminalproto.LaunchFeature, terminalproto.Denied},
		{terminalproto.LaunchFeature + "," + terminalproto.WorktreeFeature, terminalproto.Unavailable},
	} {
		for _, body := range []terminalproto.Body{
			&terminalproto.WorktreeStatus{Run: testRun, Project: "p", Repo: "site"},
			&terminalproto.WorktreeRemove{Run: testRun, Project: "p", Repo: "site"},
		} {
			got := make(replies, 4)
			headers := http.Header{}
			if tc.features != "" {
				headers.Set(terminalproto.FeatureHeader, tc.features)
			}
			handle, cleanup := Factory(terminalhost.ManagerOptions{})(t.Context(), http.DefaultClient, headers, 1, got.send)
			reply := worktreeCall(t, handle, got, body)
			cleanup()
			if r, ok := reply.(terminalproto.WorktreeResult); !ok || r != (terminalproto.WorktreeResult{Code: tc.want}) {
				t.Fatal(tc.features, reply)
			}
		}
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	env := []string{"GIT_CONFIG_NOSYSTEM=1"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Status and remove through the connector with a real manager and real git.
func TestWorktreeStatusAndRemoveThroughTheConnector(t *testing.T) {
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
	git(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "README")
	git(t, repo, "-c", "user.name=T", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "first")
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
	got := make(replies, 4)
	headers := http.Header{}
	headers.Set(terminalproto.FeatureHeader, terminalproto.LaunchFeature+","+terminalproto.WorktreeFeature)
	handle, cleanup := Factory(terminalhost.ManagerOptions{EnrollmentID: id, Layout: layout, Home: home, Consent: func() error { return nil }})(t.Context(), http.DefaultClient, headers, 1, got.send)
	defer cleanup()

	if err := os.WriteFile(filepath.Join(w.Path, "draft"), []byte("d\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reply := worktreeCall(t, handle, got, &terminalproto.WorktreeStatus{Run: testRun, Project: "p", Repo: "site"})
	state, ok := reply.(terminalproto.WorktreeState)
	if !ok || state.Path != w.Path || state.Branch != w.Branch || state.Changed != 1 || state.Unpushed != 0 || len(state.Paths) != 1 || state.Paths[0] != "draft" {
		t.Fatalf("status: %#v", reply)
	}
	reply = worktreeCall(t, handle, got, &terminalproto.WorktreeRemove{Run: testRun, Project: "p", Repo: "site"})
	if reply != (terminalproto.WorktreeResult{Code: terminalproto.WorktreeChanged}) {
		t.Fatalf("stale counts: %#v", reply)
	}
	reply = worktreeCall(t, handle, got, &terminalproto.WorktreeRemove{Run: testRun, Project: "p", Repo: "site", Changed: 1})
	if reply != (terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeHasChanges}) {
		t.Fatalf("uncommitted changes: %#v", reply)
	}
	if err := os.Remove(filepath.Join(w.Path, "draft")); err != nil {
		t.Fatal(err)
	}
	reply = worktreeCall(t, handle, got, &terminalproto.WorktreeRemove{Run: testRun, Project: "p", Repo: "site"})
	if reply != (terminalproto.WorktreeResult{Code: terminalproto.OK}) {
		t.Fatalf("remove: %#v", reply)
	}
	if _, err := os.Lstat(w.Path); !os.IsNotExist(err) {
		t.Fatal("the worktree is still there")
	}
	if git(t, repo, "rev-parse", "--verify", "-q", "refs/heads/"+w.Branch) == "" {
		t.Fatal("the branch was deleted")
	}
	reply = worktreeCall(t, handle, got, &terminalproto.WorktreeRemove{Run: testRun, Project: "p", Repo: "site"})
	if reply != (terminalproto.WorktreeResult{Code: terminalproto.NotFound}) {
		t.Fatalf("already gone: %#v", reply)
	}
	reply = worktreeCall(t, handle, got, &terminalproto.WorktreeStatus{Run: testRun, Project: "p", Repo: "site"})
	if reply != (terminalproto.WorktreeResult{Code: terminalproto.NotFound}) {
		t.Fatalf("status of a removed worktree: %#v", reply)
	}
}
