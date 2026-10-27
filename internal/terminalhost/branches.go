package terminalhost

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// branchesTimeout bounds one listing: its three git calls together.
const branchesTimeout = 5 * time.Second

// branchesCount is what for-each-ref is asked for: one more than a listing
// returns, so a cut list is known to be cut.
const branchesCount = terminalproto.MaxBranchNames + 1

// ListBranches lists the branches of the repository whose working tree top
// is req.Dir, for a remote controller that must be treated as hostile:
//
//   - req.Dir is cleaned lexically and resolved by ResolveDir (symlinks
//     followed, home or inside it). A path that leaves home at any point is
//     outside_home whether or not anything exists there (the browse walk
//     tells why); anything else that is not a folder is not_repo_root.
//   - The folder must be a repository root as browse and worktree-v1 see
//     one: <dir>/.git a real directory (never a link or a .git file) holding
//     a regular HEAD file, both lstat'ed beneath an os.Root at the real home.
//   - Only then git runs, through git (ExecGit: argument vector, no shell,
//     the worktree environment of HOME, PATH and LANG only), in the resolved
//     folder, exactly
//     `for-each-ref --count=201 --sort=-committerdate --format=%(refname) refs/heads refs/remotes`,
//     `symbolic-ref --quiet --short HEAD` and
//     `symbolic-ref --quiet --short refs/remotes/origin/HEAD`, within 5
//     seconds in all. req.Filter never reaches git: it is applied here.
//   - Branches are short names (main, origin/main), newest commit first. A
//     remote's HEAD and a branch called HEAD are left out, as is any name the
//     protocol cannot carry (terminalproto.ValidRefName) or a repeat of a
//     short name already listed: one such name would make the whole answer
//     invalid. At most 200 are listed and the answer always fits
//     terminalproto.MaxBrowseReplyBytes; More is set when the list was cut,
//     by git's count or by either bound.
//   - Current is the checked-out branch, empty on a detached HEAD. Default
//     is the branch refs/remotes/origin/HEAD points at, without "origin/",
//     empty when there is none.
//   - A git failure is git_failed with nothing of git's output.
//
// The answer is always a valid branches_result.
func ListBranches(ctx context.Context, home string, req terminalproto.Branches, git GitRunner) terminalproto.BranchesResult {
	ctx, cancel := context.WithTimeout(ctx, branchesTimeout)
	defer cancel()
	dir, reason := branchesRepo(home, req.Dir)
	if reason != "" {
		return terminalproto.BranchesResult{Reason: reason}
	}
	if git == nil {
		return terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed}
	}
	out, err := git(ctx, dir, "for-each-ref", "--count=201", "--sort=-committerdate", "--format=%(refname)", "refs/heads", "refs/remotes")
	if err != nil || ctx.Err() != nil {
		return terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed}
	}
	refs := string(out)
	cut := len(out) >= gitListLimit
	r := terminalproto.BranchesResult{}
	if out, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		r.Current = branchName(strings.TrimSuffix(string(out), "\n"))
	}
	if out, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if name, ok := strings.CutPrefix(strings.TrimSuffix(string(out), "\n"), "origin/"); ok {
			r.Default = branchName(name)
		}
	}
	// symbolic-ref fails on a detached HEAD or with no origin/HEAD; only a
	// listing cut short by the deadline is a failure.
	if ctx.Err() != nil {
		return terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed}
	}
	// The folder git read must still be the repository root that was
	// checked: one swapped for a link meanwhile is not answered for.
	if again, reason := branchesRepo(home, req.Dir); reason != "" || again != dir {
		return terminalproto.BranchesResult{Reason: terminalproto.BranchesOutsideHome}
	}

	lines := strings.Split(refs, "\n")
	if cut && len(lines) > 0 {
		lines = lines[:len(lines)-1] // the last line may be cut short
	}
	filter := strings.ToLower(req.Filter)
	seen := map[string]bool{}
	size := branchesBaseSize(r)
	listed := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		listed++
		name, ok := shortRef(line)
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		if !strings.Contains(strings.ToLower(name), filter) {
			continue
		}
		if len(r.Branches) == terminalproto.MaxBranchNames {
			r.More = true
			break
		}
		n := branchEntrySize(name)
		if size+n > terminalproto.MaxBrowseReplyBytes {
			r.More = true
			break
		}
		size += n
		r.Branches = append(r.Branches, name)
	}
	if cut || listed >= branchesCount {
		r.More = true
	}
	if !branchesResultValid(r) {
		return terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed}
	}
	return r
}

// branchesRepo resolves dir and checks it is a repository root inside home.
func branchesRepo(home, dir string) (string, terminalproto.BranchesReason) {
	realHome, err := ResolveDir(home, home)
	if err != nil {
		return "", terminalproto.BranchesOutsideHome
	}
	rooted, done, err := rootBeneath(OSFS, realHome)
	if err != nil {
		return "", terminalproto.BranchesOutsideHome
	}
	defer done()
	b := &browser{fsys: rooted, expired: func() bool { return false }}
	if !filepath.IsAbs(dir) {
		return "", terminalproto.BranchesOutsideHome
	}
	// ".." is taken lexically, as in a browse, so a link out of home is
	// never followed back in.
	path := filepath.Clean(dir)
	resolved, err := ResolveDir(realHome, path)
	if err != nil {
		if b.whyNot(home, realHome, path) == terminalproto.BrowseOutsideHome {
			return "", terminalproto.BranchesOutsideHome
		}
		return "", terminalproto.BranchesNotRepoRoot
	}
	if !b.repoRoot(resolved) {
		return "", terminalproto.BranchesNotRepoRoot
	}
	return resolved, ""
}

// shortRef turns a for-each-ref line into the name a listing shows, or
// false for a line that is not a branch that can be sent.
func shortRef(line string) (string, bool) {
	if name, ok := strings.CutPrefix(line, "refs/heads/"); ok {
		name = branchName(name)
		return name, name != ""
	}
	if name, ok := strings.CutPrefix(line, "refs/remotes/"); ok {
		// refs/remotes/<remote>/HEAD is that remote's default, not a branch.
		if _, rest, _ := strings.Cut(name, "/"); rest == "HEAD" {
			return "", false
		}
		name = branchName(name)
		return name, name != ""
	}
	return "", false
}

// branchName is name when the protocol can carry it as a branch, else "".
// HEAD names no branch.
func branchName(name string) string {
	if name == "HEAD" || !terminalproto.ValidRefName(name) {
		return ""
	}
	return name
}

// branchesBaseSize is an upper bound on the encoded answer without names:
// More set and the names' brackets.
func branchesBaseSize(r terminalproto.BranchesResult) int {
	r.More, r.Branches = true, nil
	data, _ := json.Marshal(r)
	return len(data) + len(`,"branches":[]`)
}

// branchEntrySize is what one name adds to the encoded answer, with its
// separating comma.
func branchEntrySize(name string) int {
	data, _ := json.Marshal(name)
	return len(data) + 1
}

// branchesResultValid reports whether r encodes as a branches_result.
func branchesResultValid(r terminalproto.BranchesResult) bool {
	_, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: "branches-result-check", Generation: 1, Body: r})
	return err == nil
}
