//go:build linux || darwin

package terminalhost

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func planPrune(t *testing.T, home, repo string) terminalproto.BranchPrunePlanResult {
	t.Helper()
	plan := PlanBranchPrune(context.Background(), home, terminalproto.BranchPrunePlan{Dir: repo}, ExecGit)
	if plan.Reason != "" {
		t.Fatalf("plan refused: %+v", plan)
	}
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: "branch-prune-test-0001", Generation: 1, Body: plan}); err != nil {
		t.Fatalf("plan does not encode: %v (%+v)", err, plan)
	}
	return plan
}

func pruneTip(t *testing.T, plan terminalproto.BranchPrunePlanResult, name string) string {
	t.Helper()
	for _, b := range plan.Branches {
		if b.Name == name {
			return b.OID
		}
	}
	t.Fatalf("branch %q missing from plan", name)
	return ""
}

func TestPlanBranchPruneSortsSafeFromRisky(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "prune")
	testGit(t, repo, "branch", "merged")
	testGit(t, repo, "checkout", "-q", "-b", "unmerged")
	writeFile(t, filepath.Join(repo, "work"), "x\n")
	testGit(t, repo, "add", "work")
	testGit(t, repo, "commit", "-q", "-m", "work")
	testGit(t, repo, "checkout", "-q", "main")
	testGit(t, repo, "worktree", "add", "-q", "-b", "checkedout", filepath.Join(home, "co"))

	plan := planPrune(t, home, repo)
	if plan.Base != "main" || plan.Current != "main" {
		t.Fatalf("base/current = %q/%q, want main/main", plan.Base, plan.Current)
	}
	got := map[string]terminalproto.BranchPruneSkip{}
	for _, b := range plan.Branches {
		if b.OID == "" {
			t.Fatalf("branch %q has no tip", b.Name)
		}
		got[b.Name] = b.Skip
	}
	want := map[string]terminalproto.BranchPruneSkip{
		"main":       terminalproto.BranchPruneCurrent,
		"merged":     "",
		"unmerged":   terminalproto.BranchPruneNotMerged,
		"checkedout": terminalproto.BranchPruneCheckedOut,
	}
	for name, skip := range want {
		if got[name] != skip {
			t.Errorf("%s skip = %q, want %q", name, got[name], skip)
		}
	}
}

func TestPruneBranchesDeletesOnlySafeAtShownTip(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "prune-apply")
	testGit(t, repo, "branch", "merged")
	testGit(t, repo, "checkout", "-q", "-b", "unmerged")
	writeFile(t, filepath.Join(repo, "work"), "x\n")
	testGit(t, repo, "add", "work")
	testGit(t, repo, "commit", "-q", "-m", "work")
	testGit(t, repo, "checkout", "-q", "main")

	plan := planPrune(t, home, repo)
	result := PruneBranches(context.Background(), home, terminalproto.BranchPrune{
		Dir: repo, Base: "main",
		Branches: []terminalproto.BranchPruneTarget{
			{Name: "merged", OID: pruneTip(t, plan, "merged")},
			{Name: "unmerged", OID: pruneTip(t, plan, "unmerged")},
			{Name: "main", OID: pruneTip(t, plan, "main")},
		},
	}, ExecGit)
	if result.Reason != "" {
		t.Fatalf("prune refused: %+v", result)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != "merged" {
		t.Fatalf("deleted = %v, want [merged]", result.Deleted)
	}
	skipped := map[string]terminalproto.BranchPruneSkip{}
	for _, s := range result.Skipped {
		skipped[s.Name] = s.Skip
	}
	if skipped["unmerged"] != terminalproto.BranchPruneNotMerged || skipped["main"] != terminalproto.BranchPruneCurrent {
		t.Fatalf("skipped = %v", result.Skipped)
	}
	if _, err := testGitErr(repo, "rev-parse", "--verify", "--quiet", "refs/heads/merged"); err == nil {
		t.Fatal("merged was not deleted")
	}
	if _, err := testGitErr(repo, "rev-parse", "--verify", "--quiet", "refs/heads/unmerged"); err != nil {
		t.Fatal("unmerged was deleted")
	}
}

func TestPruneBranchesSkipsABranchThatMoved(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "prune-moved")
	testGit(t, repo, "branch", "safe")
	plan := planPrune(t, home, repo)
	_ = pruneTip(t, plan, "safe")

	result := PruneBranches(context.Background(), home, terminalproto.BranchPrune{
		Dir: repo, Base: "main",
		Branches: []terminalproto.BranchPruneTarget{{Name: "safe", OID: strings.Repeat("0", 40)}},
	}, ExecGit)
	if result.Reason != "" || len(result.Deleted) != 0 || len(result.Skipped) != 1 || result.Skipped[0].Skip != terminalproto.BranchPruneMoved {
		t.Fatalf("prune of a moved branch = %+v", result)
	}
	if _, err := testGitErr(repo, "rev-parse", "--verify", "--quiet", "refs/heads/safe"); err != nil {
		t.Fatal("moved branch was deleted")
	}
}

func TestPlanBranchPruneRefusals(t *testing.T) {
	home := worktreeHome(t)
	mkdirs(t, home, "plain")
	if r := PlanBranchPrune(context.Background(), home, terminalproto.BranchPrunePlan{Dir: filepath.Join(home, "plain")}, ExecGit); r.Reason != terminalproto.BranchesNotRepoRoot {
		t.Fatalf("plain folder = %+v, want not_repo_root", r)
	}
	if r := PlanBranchPrune(context.Background(), home, terminalproto.BranchPrunePlan{Dir: t.TempDir()}, ExecGit); r.Reason != terminalproto.BranchesOutsideHome {
		t.Fatalf("outside home = %+v, want outside_home", r)
	}
	repo := newRepo(t, home, "prune-nil")
	if r := PlanBranchPrune(context.Background(), home, terminalproto.BranchPrunePlan{Dir: repo}, nil); r.Reason != terminalproto.BranchesGitFailed {
		t.Fatalf("no git = %+v, want git_failed", r)
	}
}
