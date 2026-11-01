//go:build linux || darwin

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func branchesHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	return home
}

func branchesGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	env := []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func branchesGitErr(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	env := []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	return cmd.Run()
}

// branchesRepo makes a repository at home/site with main, a merged branch
// "old", and an unmerged branch "wip" with one commit of its own.
func branchesRepo(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, "site")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	branchesGit(t, dir, "init", "-q", "-b", "main")
	branchesGit(t, dir, "config", "user.name", "Reet Test")
	branchesGit(t, dir, "config", "user.email", "test@example.invalid")
	branchesGit(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	branchesGit(t, dir, "add", "README")
	branchesGit(t, dir, "commit", "-q", "-m", "first")
	branchesGit(t, dir, "branch", "old")
	branchesGit(t, dir, "checkout", "-q", "-b", "wip")
	if err := os.WriteFile(filepath.Join(dir, "work"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	branchesGit(t, dir, "add", "work")
	branchesGit(t, dir, "commit", "-q", "-m", "work")
	branchesGit(t, dir, "checkout", "-q", "main")
	return dir
}

func branchesRun(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := runBranchesInput(args, strings.NewReader(""), &out)
	return code, out.String()
}

func TestBranchesStatusJSON(t *testing.T) {
	home := branchesHome(t)
	repo := branchesRepo(t, home)
	code, out := branchesRun(t, "status", "--dir", repo, "--json")
	if code != branchesOK {
		t.Fatalf("status exit %d: %s", code, out)
	}
	var plan terminalproto.BranchPrunePlanResult
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err, out)
	}
	if plan.Base != "main" || plan.Current != "main" {
		t.Fatalf("plan base/current = %q/%q", plan.Base, plan.Current)
	}
	got := map[string]terminalproto.BranchPruneSkip{}
	for _, b := range plan.Branches {
		got[b.Name] = b.Skip
	}
	if got["old"] != "" || got["wip"] != terminalproto.BranchPruneNotMerged || got["main"] != terminalproto.BranchPruneCurrent {
		t.Fatalf("skips = %v", got)
	}
}

func TestBranchesPruneYesDeletesOnlyTheSafe(t *testing.T) {
	home := branchesHome(t)
	repo := branchesRepo(t, home)
	code, out := branchesRun(t, "prune", "--dir", repo, "--yes", "--json")
	if code != branchesOK {
		t.Fatalf("prune exit %d: %s", code, out)
	}
	var result terminalproto.BranchPruneResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err, out)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != "old" {
		t.Fatalf("deleted = %v", result.Deleted)
	}
	if err := branchesGitErr(repo, "rev-parse", "--verify", "--quiet", "refs/heads/old"); err == nil {
		t.Fatal("old was not deleted")
	}
	if err := branchesGitErr(repo, "rev-parse", "--verify", "--quiet", "refs/heads/wip"); err != nil {
		t.Fatal("wip was deleted")
	}
}

func TestBranchesPruneWithoutYesChangesNothing(t *testing.T) {
	home := branchesHome(t)
	repo := branchesRepo(t, home)
	code, out := branchesRun(t, "prune", "--dir", repo)
	if code != branchesHaveWork {
		t.Fatalf("prune without --yes exit %d, want %d: %s", code, branchesHaveWork, out)
	}
	if !strings.Contains(out, "--yes") {
		t.Fatalf("no hint about --yes: %s", out)
	}
	if err := branchesGitErr(repo, "rev-parse", "--verify", "--quiet", "refs/heads/old"); err != nil {
		t.Fatal("old was deleted without --yes")
	}
}

func TestBranchesDryRunReportsWork(t *testing.T) {
	home := branchesHome(t)
	repo := branchesRepo(t, home)
	code, out := branchesRun(t, "prune", "--dir", repo, "--dry-run", "--json")
	if code != branchesHaveWork {
		t.Fatalf("dry run exit %d, want %d: %s", code, branchesHaveWork, out)
	}
	var plan terminalproto.BranchPrunePlanResult
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err, out)
	}
	if plan.Base != "main" {
		t.Fatalf("dry run did not print the plan: %s", out)
	}
	if err := branchesGitErr(repo, "rev-parse", "--verify", "--quiet", "refs/heads/old"); err != nil {
		t.Fatal("dry run deleted a branch")
	}
}

func TestBranchesOutsideHomeIsRefused(t *testing.T) {
	branchesHome(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(outside, "site")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	branchesGit(t, repo, "init", "-q", "-b", "main")
	code, _ := branchesRun(t, "status", "--dir", repo)
	if code != branchesRefused {
		t.Fatalf("outside home exit %d, want %d", code, branchesRefused)
	}
}

func TestBranchesInvalidBaseIsUsage(t *testing.T) {
	home := branchesHome(t)
	repo := branchesRepo(t, home)
	if code, _ := branchesRun(t, "status", "--dir", repo, "--base", "-x"); code != branchesUsage {
		t.Fatalf("invalid base exit %d, want %d", code, branchesUsage)
	}
}
