//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestStructuredChatWorktreeLifecycle(t *testing.T) {
	for _, scenario := range []string{"created", "refused-new", "refused-adopted"} {
		t.Run(scenario, func(t *testing.T) {
			h, proc, _ := testChatHost(t)
			t.Cleanup(h.sup.Close)
			home, err := filepath.EvalSymlinks(h.sup.opts.Home)
			if err != nil {
				t.Fatal(err)
			}
			h.sup.opts.Home = home
			h.sup.registry = newChatRegistry(filepath.Join(home, "structured-chats.json"))
			repo := filepath.Join(h.sup.opts.Home, "repo")
			if err := os.MkdirAll(repo, 0700); err != nil {
				t.Fatal(err)
			}
			git(t, repo, "init", "-q", "-b", "main")
			git(t, repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-q", "-m", "fixture")
			run := "0123456789abcdef0123456789abcdef"
			path := filepath.Join(h.sup.opts.Home, ".reet", "worktrees", "project", "repo", run)
			if scenario == "refused-adopted" {
				if _, err := terminalhost.PrepareWorktree(t.Context(), h.sup.opts.Home, repo, "project", run, "", terminalhost.ExecGit); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "keep.txt"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			starts := 0
			h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
				starts++
				if opts.Dir != path {
					t.Fatalf("chat started outside its worktree: %s", opts.Dir)
				}
				if scenario != "created" {
					return nil, errors.New("startup refused")
				}
				proc.onEvent = opts.OnEvent
				return proc, nil
			}
			create := terminalproto.Create{Title: "Isolated chat", Tool: "opencode", Structured: true, Trust: true, Dir: repo, Cols: 80, Rows: 24, Worktree: run, Project: "project"}
			changed, err := h.open("request-worktree-0001", "session-worktree-0001", create)
			if scenario == "refused-new" {
				if err == nil {
					t.Fatal("start unexpectedly succeeded")
				}
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed start retained its new worktree", err)
				}
				if out, err := exec.Command("git", "-C", repo, "show-ref", "--verify", "refs/heads/reet/project-"+run).CombinedOutput(); err == nil {
					t.Fatalf("failed start retained its new branch: %s", out)
				}
				return
			}
			if scenario == "refused-adopted" {
				if err == nil {
					t.Fatal("start unexpectedly succeeded")
				}
				if data, err := os.ReadFile(filepath.Join(path, "keep.txt")); err != nil || string(data) != "keep" {
					t.Fatal("failed start removed adopted work", err)
				}
				return
			}
			if err != nil || changed.WorktreePath != path || changed.WorktreeRepo != "repo" {
				t.Fatalf("chat worktree metadata missing: %+v %v", changed, err)
			}
			records := newChatRegistry(filepath.Join(home, "structured-chats.json")).snapshot()
			if len(records) != 1 || records[0].Dir != path || records[0].WorktreePath != path || records[0].WorktreeRepo != "repo" {
				t.Fatalf("restart record lost worktree identity: %+v", records)
			}
			again, err := h.open("request-worktree-0001", "session-worktree-0001", create)
			if err != nil || again.WorktreePath != path || starts != 1 {
				t.Fatalf("retry recreated chat/worktree: %+v %v starts=%d", again, err, starts)
			}
			if _, err := h.end("session-worktree-0001"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("ending chat deleted its worktree", err)
			}
			if !strings.Contains(git(t, repo, "worktree", "list", "--porcelain"), path) {
				t.Fatal("worktree no longer registered")
			}
		})
	}
}
