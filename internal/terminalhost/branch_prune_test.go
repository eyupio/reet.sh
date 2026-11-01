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

// A branch with many patch-equivalent commits and one of its own: git cherry's
// output is cut by ExecGit before the "+" line, so the list is incomplete and
// the branch must not be called merged.
func TestPlanBranchPruneLongEquivalentHistoryWithUniqueCommitIsNotMerged(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "prune-long")
	testGit(t, repo, "branch", "eq")
	for i := 0; i < 100; i++ {
		name := "f" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + strings.Repeat("y", i/26)
		writeFile(t, filepath.Join(repo, name), "x\n")
		testGit(t, repo, "add", name)
		testGit(t, repo, "commit", "-q", "-m", name)
	}
	testGit(t, repo, "checkout", "-q", "eq")
	testGit(t, repo, "cherry-pick", "main~99..main")
	writeFile(t, filepath.Join(repo, "unique"), "mine\n")
	testGit(t, repo, "add", "unique")
	testGit(t, repo, "commit", "-q", "-m", "unique work")
	testGit(t, repo, "checkout", "-q", "main")

	for _, b := range planPrune(t, home, repo).Branches {
		if b.Name == "eq" && b.Skip != terminalproto.BranchPruneNotMerged {
			t.Fatalf("eq skip = %q, want not_merged", b.Skip)
		}
	}
}

// A merge commit only this branch has can hold a conflict resolution that
// git cherry never looks at.
func TestPlanBranchPruneUniqueMergeCommitIsNotMerged(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "prune-merge")
	writeFile(t, filepath.Join(repo, "a"), "a\n")
	testGit(t, repo, "add", "a")
	testGit(t, repo, "commit", "-q", "-m", "a")
	testGit(t, repo, "checkout", "-q", "-b", "m", "main~1")
	testGit(t, repo, "merge", "--no-ff", "--no-commit", "main")
	writeFile(t, filepath.Join(repo, "resolution"), "only here\n")
	testGit(t, repo, "add", "resolution")
	testGit(t, repo, "commit", "-q", "-m", "merge with extra work")
	testGit(t, repo, "checkout", "-q", "main")

	for _, b := range planPrune(t, home, repo).Branches {
		if b.Name == "m" && b.Skip != terminalproto.BranchPruneNotMerged {
			t.Fatalf("m skip = %q, want not_merged", b.Skip)
		}
	}
}

// Two hundred safe branches with long names do not fit one reply: the plan
// stops at the byte budget and says there is more, and it still encodes.
func TestPlanBranchPruneStaysWithinReplyBudget(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "prune-many")
	for i := 0; i < terminalproto.MaxBranchPruneList; i++ {
		testGit(t, repo, "branch", strings.Repeat("n", 90)+strings.Repeat("a", i/26)+string(rune('a'+i%26)))
	}
	plan := planPrune(t, home, repo)
	if !plan.More || len(plan.Branches) == 0 || len(plan.Branches) >= terminalproto.MaxBranchPruneList {
		t.Fatalf("got %d branches, More %t", len(plan.Branches), plan.More)
	}
}
