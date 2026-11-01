package terminalhost

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// branchPruneTimeout bounds one plan or prune: every git call it makes.
const branchPruneTimeout = 20 * time.Second

// PlanBranchPrune lists the local branches of the repository whose working
// tree top is req.Dir and says whether each is safe to delete, compared
// against req.Base (or the repository's default when Base is empty).
//
// It follows the same hostile-controller rules as ListBranches: the folder is
// resolved under home and must be a repository root, git runs as an argument
// vector with the hardened environment pinned to that repository, and only
// plain branch names the protocol can carry are answered.
//
// A branch is safe only when its commits are contained in the base
// (`rev-list --count base..branch` is 0) or are all patch-equivalent to the
// base (`git cherry base branch` lists no "+" commit), it is not the current
// branch, the default, the base, or checked out in any worktree, and either it
// has no upstream or its upstream has every local commit. Anything that cannot
// be proven is skipped, never deleted.
func PlanBranchPrune(ctx context.Context, home string, req terminalproto.BranchPrunePlan, git GitRunner) terminalproto.BranchPrunePlanResult {
	ctx, cancel := context.WithTimeout(ctx, branchPruneTimeout)
	defer cancel()
	dir, reason := branchesRepo(home, req.Dir)
	if reason != "" {
		return terminalproto.BranchPrunePlanResult{Reason: reason}
	}
	if git == nil {
		return terminalproto.BranchPrunePlanResult{Reason: terminalproto.BranchesGitFailed}
	}
	ctx = pinGitDir(ctx, dir)

	refs, err := git(ctx, dir, "for-each-ref", "--count=201", "--format=%(refname) %(objectname)", "refs/heads")
	if err != nil || ctx.Err() != nil {
		return terminalproto.BranchPrunePlanResult{Reason: terminalproto.BranchesGitFailed}
	}
	oids := map[string]string{}
	var order []string
	more := false
	for _, line := range strings.Split(string(refs), "\n") {
		if line == "" {
			continue
		}
		ref, oid, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		name, ok := strings.CutPrefix(ref, "refs/heads/")
		if !ok || name == "" || !terminalproto.ValidRefName(name) || !isOID(oid) {
			continue
		}
		if _, seen := oids[name]; seen {
			continue
		}
		if len(oids) == terminalproto.MaxBranchPruneList {
			more = true
			break
		}
		oids[name] = oid
		order = append(order, name)
	}

	current := ""
	if out, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		current = strings.TrimSuffix(string(out), "\n")
		if !terminalproto.ValidRefName(current) {
			current = ""
		}
	}
	deflt := ""
	if out, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if name, ok := strings.CutPrefix(strings.TrimSuffix(string(out), "\n"), "origin/"); ok && terminalproto.ValidRefName(name) {
			deflt = name
		}
	}
	checkedOut := map[string]bool{}
	if out, err := git(ctx, dir, "worktree", "list", "--porcelain"); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if name, ok := strings.CutPrefix(line, "branch refs/heads/"); ok && terminalproto.ValidRefName(name) {
				checkedOut[name] = true
			}
		}
	}

	base := req.Base
	if base == "" {
		base = deflt
		if base == "" {
			if _, ok := oids["main"]; ok {
				base = "main"
			} else if _, ok := oids["master"]; ok {
				base = "master"
			}
		}
	}
	baseRef := ""
	if base != "" {
		if _, ok := oids[base]; ok {
			baseRef = "refs/heads/" + base
		} else {
			baseRef = "refs/remotes/origin/" + base
		}
		// A base that names no ref proves nothing: nothing is called merged.
		if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", baseRef); err != nil || ctx.Err() != nil {
			baseRef, base = "", ""
		}
	}

	result := terminalproto.BranchPrunePlanResult{Current: current, Default: deflt, Base: base, More: more}
	// The encoded answer must fit the protocol's reply limit, or the machine
	// could not send it: stop adding branches when the budget is spent.
	size := branchPrunePlanBaseSize(result)
	for _, name := range order {
		if ctx.Err() != nil {
			result.More = true
			break
		}
		skip := terminalproto.BranchPruneSkip("")
		switch {
		case name == current:
			skip = terminalproto.BranchPruneCurrent
		case name == deflt:
			skip = terminalproto.BranchPruneDefault
		case name == base:
			skip = terminalproto.BranchPruneBase
		case checkedOut[name]:
			skip = terminalproto.BranchPruneCheckedOut
		case baseRef == "":
			skip = terminalproto.BranchPruneNotMerged
		}
		if skip == "" {
			merged, failed := branchMerged(ctx, dir, baseRef, "refs/heads/"+name, git)
			if failed {
				skip = terminalproto.BranchPruneGitFailed
			} else if !merged.merged {
				skip = terminalproto.BranchPruneNotMerged
			} else if merged.uniqueCommits {
				if unpushed, failed := branchUnpushed(ctx, dir, "refs/heads/"+name, git); failed {
					skip = terminalproto.BranchPruneGitFailed
				} else if unpushed {
					skip = terminalproto.BranchPruneUnpushed
				}
			}
		}
		entry := terminalproto.BranchPruneBranch{Name: name, OID: oids[name], Skip: skip}
		data, _ := json.Marshal(entry)
		if size+len(data)+1 > terminalproto.MaxBrowseReplyBytes {
			result.More = true
			break
		}
		size += len(data) + 1
		result.Branches = append(result.Branches, entry)
	}
	return result
}

// branchPrunePlanBaseSize is an upper bound on the encoded answer without
// branches: More set and the branches' brackets.
func branchPrunePlanBaseSize(r terminalproto.BranchPrunePlanResult) int {
	r.More, r.Branches = true, nil
	data, _ := json.Marshal(r)
	return len(data) + len(`,"branches":[]`)
}

type mergeState struct {
	merged        bool
	uniqueCommits bool
}

// branchMerged reports whether branch has nothing the base does not already
// have, either contained or patch-equivalent. uniqueCommits says the branch
// has commits the base does not contain, even if their patches are applied.
func branchMerged(ctx context.Context, dir, base, branch string, git GitRunner) (mergeState, bool) {
	out, err := git(ctx, dir, "rev-list", "--count", base+".."+branch)
	if err != nil {
		return mergeState{}, true
	}
	if strings.TrimSpace(string(out)) == "0" {
		return mergeState{merged: true}, false
	}
	// git cherry skips merge commits, so a merge only this branch has could
	// carry work (a conflict resolution) that no patch comparison sees: it is
	// not proven preserved.
	merges, err := git(ctx, dir, "rev-list", "--count", "--merges", base+".."+branch)
	if err != nil {
		return mergeState{}, true
	}
	if strings.TrimSpace(string(merges)) != "0" {
		return mergeState{merged: false}, false
	}
	plain, err := git(ctx, dir, "rev-list", "--count", "--no-merges", base+".."+branch)
	if err != nil {
		return mergeState{}, true
	}
	want, err := strconv.Atoi(strings.TrimSpace(string(plain)))
	if err != nil {
		return mergeState{}, true
	}
	cherry, err := git(ctx, dir, "cherry", base, branch)
	if err != nil {
		return mergeState{}, true
	}
	// ExecGit cuts long output without an error. Every commit must be
	// accounted for: a list shorter than the commit count is incomplete and
	// proves nothing, whatever it shows.
	listed := 0
	for _, line := range strings.Split(string(cherry), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			return mergeState{merged: false}, false
		}
		listed++
	}
	if listed != want {
		return mergeState{merged: false}, false
	}
	return mergeState{merged: true, uniqueCommits: true}, false
}

// branchUnpushed reports whether the branch has commits its upstream lacks.
// A branch with no upstream has none to compare and is not called unpushed.
func branchUnpushed(ctx context.Context, dir, branch string, git GitRunner) (bool, bool) {
	out, err := git(ctx, dir, "for-each-ref", "--format=%(upstream:short)", branch)
	if err != nil {
		return false, true
	}
	up := strings.TrimSuffix(string(out), "\n")
	if up == "" {
		return false, false
	}
	count, err := git(ctx, dir, "rev-list", "--count", up+".."+branch)
	if err != nil {
		return false, true
	}
	return strings.TrimSpace(string(count)) != "0", false
}

// PruneBranches deletes the requested branches when each is still safe by the
// same rules as PlanBranchPrune and still points at the tip the person was
// shown. It deletes a ref with `git update-ref -d refs/heads/<name> <oid>`, so
// a branch that moved is skipped rather than force-deleted, and it never
// checks out or touches a working tree.
func PruneBranches(ctx context.Context, home string, req terminalproto.BranchPrune, git GitRunner) terminalproto.BranchPruneResult {
	ctx, cancel := context.WithTimeout(ctx, branchPruneTimeout)
	defer cancel()
	plan := PlanBranchPrune(ctx, home, terminalproto.BranchPrunePlan{Dir: req.Dir, Base: req.Base}, git)
	if plan.Reason != "" {
		return terminalproto.BranchPruneResult{Reason: plan.Reason}
	}
	byName := make(map[string]terminalproto.BranchPruneBranch, len(plan.Branches))
	for _, b := range plan.Branches {
		byName[b.Name] = b
	}
	dir, reason := branchesRepo(home, req.Dir)
	if reason != "" {
		return terminalproto.BranchPruneResult{Reason: reason}
	}
	ctx = pinGitDir(ctx, dir)
	result := terminalproto.BranchPruneResult{}
	for _, target := range req.Branches {
		planned, ok := byName[target.Name]
		switch {
		case !ok || planned.OID != target.OID:
			result.Skipped = append(result.Skipped, terminalproto.BranchPruneSkipped{Name: target.Name, Skip: terminalproto.BranchPruneMoved})
			continue
		case planned.Skip != "":
			result.Skipped = append(result.Skipped, terminalproto.BranchPruneSkipped{Name: target.Name, Skip: planned.Skip})
			continue
		}
		if ctx.Err() != nil {
			result.Skipped = append(result.Skipped, terminalproto.BranchPruneSkipped{Name: target.Name, Skip: terminalproto.BranchPruneGitFailed})
			continue
		}
		if _, err := git(ctx, dir, "update-ref", "-d", "refs/heads/"+target.Name, target.OID); err != nil {
			result.Skipped = append(result.Skipped, terminalproto.BranchPruneSkipped{Name: target.Name, Skip: terminalproto.BranchPruneGitFailed})
			continue
		}
		result.Deleted = append(result.Deleted, target.Name)
	}
	return result
}

// isOID reports whether s is a full git object id: 40 or 64 lower-case hex.
func isOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}
