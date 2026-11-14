package terminalhost

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ErrWorktreeRefused means a worktree operation (create, status or remove)
// was refused because it could not be done safely. The error is a
// *WorktreeError carrying the reason; the connector answers it as
// terminalproto.WorktreeRefused with that reason (see WorktreeResult).
var ErrWorktreeRefused = errors.New("the worktree operation was refused")

// WorktreeError is the refusal: Reason is the wire enum, Detail is plain text
// (at most terminalproto.MaxWorktreeDetailBytes, no control characters).
type WorktreeError struct {
	Reason terminalproto.WorktreeReason
	Detail string
}

func (e *WorktreeError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s: %s", ErrWorktreeRefused, e.Reason)
	}
	return fmt.Sprintf("%s: %s: %s", ErrWorktreeRefused, e.Reason, e.Detail)
}
func (e *WorktreeError) Is(target error) bool { return target == ErrWorktreeRefused }

func refuseWorktree(reason terminalproto.WorktreeReason, detail string) error {
	return &WorktreeError{Reason: reason, Detail: worktreeDetail([]byte(detail))}
}

// WorktreeResult is the single mapping point from a host error to a
// worktree_refused answer; ok is false for any other error.
func WorktreeResult(err error) (terminalproto.Result, bool) {
	var w *WorktreeError
	if !errors.As(err, &w) {
		return terminalproto.Result{}, false
	}
	r := terminalproto.Result{Code: terminalproto.WorktreeRefused, Reason: w.Reason, Detail: w.Detail}
	if !w.Reason.Valid() {
		r.Reason, r.Detail = "", ""
	}
	if r.Reason == "" {
		r.Detail = ""
	}
	return r, true
}

// GitRunner runs `git -C dir args...` and returns its combined output. The
// real one is ExecGit; tests record or fail calls through it.
type GitRunner func(ctx context.Context, dir string, args ...string) ([]byte, error)

const (
	gitTimeout     = 60 * time.Second
	gitOutputLimit = 4 << 10
	// gitListLimit bounds the read-only listings that grow with the
	// repository (worktree list, status, ls-files, for-each-ref); output this long may have
	// been cut short and is never trusted as complete.
	gitListLimit = 4 << 20
)

// outputLimit is gitListLimit for the read-only listings (including every
// for-each-ref, which only reads) and gitOutputLimit
// for every other git call. Leading "-c name=value" arguments are looked
// past, so a hardened listing keeps its bound.
func outputLimit(args []string) int {
	for len(args) >= 2 && args[0] == "-c" {
		args = args[2:]
	}
	isListing := len(args) > 0 &&
		(args[0] == "status" || args[0] == "ls-files" || args[0] == "for-each-ref")
	isWorktreeList := len(args) > 1 && args[0] == "worktree" && args[1] == "list"
	if isListing || isWorktreeList {
		return gitListLimit
	}
	return gitOutputLimit
}

var errGitMissing = errors.New("git is not installed")

// readOnlyGitArgs keep a git call that only looks at a repository from
// running anything the repository's or the person's configuration names,
// and from trusting what such a program cached: no file-system monitor
// hook (which could also hide an edit from status), no untracked cache, no
// hooks, no external diff, no attributes file from outside the repository.
// filterKeysArgs lists, NUL-separated, the name of every setting of every
// configured content filter; filterOffArgs turns the listing into the -c
// arguments that switch them all off.
var filterKeysArgs = []string{"config", "-z", "--name-only", "--get-regexp", `^filter\..*\.(clean|smudge|process|required)$`}

// filterOffArgs is, for each filter setting in keys (the output of
// filterKeysArgs), a -c argument that empties it, or sets required to
// false: git then runs no clean, smudge or process command, whatever the
// repository's attributes select. The output is git's stdout and stderr
// together, so anything in it that is not a filter setting (a warning) may
// have joined a setting and hidden it: ok is then false and no arguments
// are returned.
func filterOffArgs(keys []byte) (args []string, ok bool) {
	for _, key := range strings.Split(string(keys), "\x00") {
		if key == "" {
			continue
		}
		if !strings.HasPrefix(key, "filter.") {
			return nil, false
		}
		value := ""
		if strings.HasSuffix(key, ".required") {
			value = "false"
		}
		args = append(args, "-c", key+"="+value)
	}
	return args, true
}

var readOnlyGitArgs = []string{"-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.hooksPath=/dev/null", "-c", "diff.external=", "-c", "core.attributesFile=/dev/null"}

// ExecGit runs git found by exec.LookPath as an argument vector, never through
// a shell: `git -C dir args...`, stdin empty, environment exactly HOME, PATH
// and LANG from this process (every GIT_* and every other variable removed),
// a 60-second timeout and stdout and stderr together capped at 4 KiB (4 MiB
// for the read-only listings `worktree list`, `status` and `ls-files`, see
// outputLimit; the excess is discarded while reading, never buffered).
func ExecGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	path, err := exec.LookPath("git")
	if err != nil || !filepath.IsAbs(path) {
		return nil, errGitMissing
	}
	return runGit(ctx, path, gitTimeout, outputLimit(args), dir, args...)
}

// newExecGit is ExecGit with another per-call timeout, for tests.
func newExecGit(timeout time.Duration) GitRunner {
	return func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		path, err := exec.LookPath("git")
		if err != nil || !filepath.IsAbs(path) {
			return nil, errGitMissing
		}
		return runGit(ctx, path, timeout, outputLimit(args), dir, args...)
	}
}

const cleanupTimeout = 30 * time.Second

func runGit(ctx context.Context, path string, timeout time.Duration, limit int, dir string, args ...string) ([]byte, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("git folder is not absolute")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, append([]string{"-C", dir}, args...)...)
	cmd.Env = append(gitEnvironment(os.Environ()), pinnedGitEnv(ctx)...)
	out := &truncatingBuffer{limit: limit}
	cmd.Stdout, cmd.Stderr = out, out
	killGroupOnCancel(cmd)
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	if ctx.Err() != nil && err != nil {
		return out.Bytes(), fmt.Errorf("git timed out: %w", ctx.Err())
	}
	return out.Bytes(), err
}

// pinnedGitKey carries, in a context, the repository a git call is pinned
// to (see pinGitDir).
type pinnedGitKey struct{}

// pinGitDir returns ctx with git calls made through ExecGit pinned to the
// repository at dir: GIT_DIR=<dir>/.git and GIT_CEILING_DIRECTORIES=<parent
// of dir>, added to the hardened environment. git then reads that .git or
// fails; it never searches the folders above dir for another repository,
// which it would do for a .git it does not accept as a git directory.
func pinGitDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, pinnedGitKey{}, dir)
}

// gitEnvKey carries, in a context, extra variables for a git call (see
// withGitEnv).
type gitEnvKey struct{}

// withGitEnv returns ctx with env added to the hardened environment of git
// calls made through ExecGit, for the few variables one call needs and the
// others must not have (GIT_TERMINAL_PROMPT=0 on a fetch, see networkGitEnv).
// It replaces what an earlier withGitEnv added.
func withGitEnv(ctx context.Context, env ...string) context.Context {
	return context.WithValue(ctx, gitEnvKey{}, env)
}

// pinnedGitEnv is the environment pinGitDir and withGitEnv add, or nothing.
func pinnedGitEnv(ctx context.Context) []string {
	var env []string
	if dir, ok := ctx.Value(pinnedGitKey{}).(string); ok && filepath.IsAbs(dir) {
		// GIT_NO_LAZY_FETCH: in a partial clone, git never fetches a missing
		// object (which would run the repository's configured ssh command or
		// remote helper). Older gits ignore it; ListBranches then also avoids
		// reading objects in such a repository.
		env = append(env, "GIT_DIR="+filepath.Join(dir, ".git"), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir), "GIT_NO_LAZY_FETCH=1")
	}
	if extra, ok := ctx.Value(gitEnvKey{}).([]string); ok {
		env = append(env, extra...)
	}
	return env
}

// gitEnvironment keeps only HOME, PATH and LANG (the first of each). No
// GIT_* variable (GIT_DIR, GIT_WORK_TREE, GIT_CONFIG_*, ...) reaches git.
func gitEnvironment(environ []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, kv := range environ {
		key, _, ok := strings.Cut(kv, "=")
		allowedKey := key == "HOME" || key == "PATH" || key == "LANG"
		valid := ok && !seen[key] && allowedKey
		if !valid {
			continue
		}
		seen[key] = true
		out = append(out, kv)
	}
	return out
}

// truncatingBuffer keeps the first limit bytes and discards the rest without
// failing the writer, so git is never blocked or killed by a full pipe.
// It deliberately has no ReadFrom, so io.Copy always goes through Write.
type truncatingBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (c *truncatingBuffer) Write(p []byte) (int, error) {
	if room := c.limit - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func (c *truncatingBuffer) Bytes() []byte { return c.buf.Bytes() }

// worktreeDetail is the plain-text detail of a refusal: the first "fatal:" or
// "error:" line of git's output, else its first non-empty line, with invalid
// UTF-8 and every control or format character removed, at most 200 bytes.
func worktreeDetail(out []byte) string {
	line := ""
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 1024), len(out)+1)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "fatal:") || strings.HasPrefix(text, "error:") {
			line = text
			break
		}
		if line == "" {
			line = text
		}
	}
	var b strings.Builder
	for _, r := range strings.ToValidUTF8(line, "") {
		isControl := unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
		isSeparator := r == '\u2028' || r == '\u2029'
		if isControl || isSeparator {
			continue
		}
		if b.Len()+utf8.RuneLen(r) > terminalproto.MaxWorktreeDetailBytes {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// Worktree is a run's worktree. Created is true only when this call made it;
// Undo then removes it and its branch. An adopted worktree (Created false,
// from a retry) is never undone, and has no Base: only the call that made the
// worktree resolved one.
type Worktree struct {
	Path, Branch string
	Created      bool
	// Base is what this call started the worktree from (see ResolveBase),
	// set whenever Created is true.
	Base terminalproto.BaseState
	repo string
}

// Undo removes a worktree this call created, after a later step failed before
// the agent started: `git worktree remove --force <path>` (the only --force
// reet uses; the worktree is untouched because nothing ran in it) and then
// `git branch -D <branch>` for the branch the same call created. It does
// nothing when the worktree was adopted.
func (w Worktree) Undo(ctx context.Context, git GitRunner) error {
	if !w.Created || w.repo == "" {
		return nil
	}
	var errs []error
	if out, err := git(ctx, w.repo, "worktree", "remove", "--force", w.Path); err != nil {
		errs = append(errs, fmt.Errorf("remove worktree: %w: %s", err, worktreeDetail(out)))
	}
	if out, err := git(ctx, w.repo, "branch", "-D", w.Branch); err != nil {
		errs = append(errs, fmt.Errorf("delete branch: %w: %s", err, worktreeDetail(out)))
	}
	return errors.Join(errs...)
}

// busyMarkers are the files git keeps while an operation is half done.
var busyMarkers = []string{"MERGE_HEAD", "rebase-merge", "rebase-apply", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG", "sequencer"}

// PrepareWorktree makes, or on a retry adopts, the worktree for one run of a
// Project in repo:
//
//  1. repo is resolved with ResolveDir (absolute, symlinks followed, a
//     directory inside home) and must be the root of a normal working tree:
//     `.git` a real directory (not a file, not a symlink, not bare), equal to
//     `git rev-parse --show-toplevel`, no merge, rebase, cherry-pick, revert or
//     bisect in progress, and HEAD a commit. A dirty tree is allowed. On a
//     connection that negotiated base-fresh-v1 the base is resolved with
//     ResolveBase, from the fetch the plan made: GitHub's commit unless the
//     repository's own branch is ahead of it; with no github.com origin it is
//     the local branch, then the remote-tracking branch, as it has always
//     been. A base naming GitHub's own branch, origin/<name>, is started from
//     whatever the local <name> holds, and any other remote's branch
//     (upstream/main) still resolves to refs/remotes/<base>. What it chose is
//     Base. With no base named, ResolveBase takes the repository's default
//     branch (origin/HEAD, then a local main or master), so the run starts
//     from GitHub's default branch, or, when the fetch failed, from the last
//     known one; only a repository with no github.com origin, or none with a
//     default branch at all, starts at HEAD, as it always has.
//     Without base-fresh-v1 (a zero BasePlan) nothing is fetched and the base
//     is the lookup a create has always made: refs/heads/<base>, then
//     refs/remotes/<base>, and the folder's HEAD with no base named. Such a
//     worktree has no Base to report.
//  2. The destination is <home>/.reet/worktrees/<project slug>/<repo slug>/<run>
//     and the branch reet/<project slug>-<run>. Every folder up to the
//     destination's parent must be a real directory owned by this user (new
//     ones are made 0700); a symlink anywhere is refused.
//  3. If the destination or branch exists it is adopted only when git lists
//     exactly that path on exactly that branch for this repository (Created
//     false); anything else is refused.
//  4. Otherwise it runs exactly
//     `git -C <repo> -c core.hooksPath=/dev/null worktree add -b <branch> <destination> <commit>`
//     (the literal HEAD when no base was named and no default branch is known).
//
// Refusals are *WorktreeError (errors.Is ErrWorktreeRefused); a folder that
// ResolveDir refuses keeps its ErrLaunchUnavailable error.
//
// PrepareWorktree is the whole thing in one call, for a caller that may reach
// the network where it stands: it plans the base itself, as a connection that
// negotiated base-fresh-v1. A caller that holds a lock must not — it calls
// PlanBase first and then PrepareWorktreeFrom.
func PrepareWorktree(ctx context.Context, home, repo, project, run, base string, git GitRunner) (Worktree, error) {
	plan := PlanBase(ctx, home, repo, project, run, base, true, git)
	return PrepareWorktreeFrom(ctx, home, repo, project, run, base, plan, git)
}

// PrepareWorktreeFrom is PrepareWorktree with the base already planned (see
// PlanBase): it makes the worktree without one git call that reaches the
// network, so it is safe to run under a lock. Its own budget covers only the
// calls it makes; the plan's fetch had its own.
func PrepareWorktreeFrom(ctx context.Context, home, repo, project, run, base string, plan BasePlan, git GitRunner) (Worktree, error) {
	return prepareWorktree(ctx, home, repo, project, run, base, plan, git, gitTimeout, cleanupTimeout)
}

// prepareWorktree bounds every git call it makes by one budget (60 s in
// PrepareWorktree; runGit also caps each call); cleaning up a failed add has
// its own budget so it still runs after that one has ended.
func prepareWorktree(ctx context.Context, home, repo, project, run, base string, plan BasePlan, git GitRunner, budget, cleanupBudget time.Duration) (Worktree, error) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	validBase := base == "" || terminalproto.ValidRefName(base)
	valid := terminalproto.ValidRunToken(run) && git != nil && validBase
	if !valid {
		return Worktree{}, terminalproto.ErrInvalid
	}
	repoPath, realHome, err := worktreeRepoPath(home, repo)
	if err != nil {
		return Worktree{}, err
	}
	head, err := checkWorktreeRepo(ctx, git, repoPath)
	if err != nil {
		return Worktree{}, err
	}

	parent, dest, branch := worktreePlace(realHome, repoPath, project, run)
	if hasControl(dest) {
		return Worktree{}, refuseWorktree(terminalproto.WorktreeOutsideHome, "the home folder's path cannot hold a worktree")
	}
	if err := ensureWorktreeDirs(realHome, parent); err != nil {
		return Worktree{}, err
	}
	w := Worktree{Path: dest, Branch: branch, repo: repoPath}

	destInfo, destErr := os.Lstat(dest)
	if destErr != nil && !os.IsNotExist(destErr) {
		return Worktree{}, refuseWorktree(terminalproto.WorktreeOutsideHome, "the destination cannot be read")
	}
	destExists := destErr == nil
	branchExists, err := gitBranchExists(ctx, git, repoPath, branch)
	if err != nil {
		return Worktree{}, err
	}
	listed, adoptable, err := gitWorktreeListed(ctx, git, repoPath, dest, branch)
	if err != nil {
		return Worktree{}, err
	}
	exists := destExists || branchExists
	if exists || listed {
		validDest := destExists && destInfo.IsDir() && destInfo.Mode()&os.ModeSymlink == 0
		canAdopt := validDest && branchExists
		if canAdopt && adoptable {
			return w, nil
		}
		return Worktree{}, refuseWorktree(terminalproto.WorktreeExists, "")
	}

	// A retry adopts above without depending on whether the original base
	// branch still exists. New worktrees pin the chosen branch to a commit.
	start := ""
	if !plan.fresh {
		// A connection that did not negotiate base-fresh-v1 never asked for a
		// fetched base, so nothing was fetched and the worktree starts where
		// a create has always started it. It has no base to report either.
		if start, err = baseAsBefore(ctx, git, repoPath, base); err != nil {
			return Worktree{}, err
		}
	} else {
		state, err := ResolveBaseFrom(ctx, repoPath, base, plan, git)
		if err != nil {
			return Worktree{}, err
		}
		start = state.Start
		// With no base named and no default branch to go by — no github.com
		// origin, or none known — the run begins where the repository stands,
		// exactly as it always has. A fetch that failed still leaves the last
		// known default branch, which ResolveBase has already chosen.
		if base == "" && state.Relation == terminalproto.BaseNoRemote {
			state = terminalproto.BaseState{Relation: terminalproto.BaseNoRemote, Start: head}
			start = "HEAD"
		}
		w.Base = state
	}

	out, err := git(ctx, repoPath, "-c", "core.hooksPath=/dev/null", "worktree", "add", "-b", branch, dest, start)
	if err != nil {
		cleanupFailedAdd(ctx, git, w, cleanupBudget)
		return Worktree{}, gitFailed(out, err)
	}
	w.Created = true
	return w, nil
}

// worktreeRepoPath resolves the repository a worktree create works from, and
// the home it must be inside: ResolveDir, both with their symlinks followed.
// A repository that contains ~/.reet (home itself, or a parent of it) is
// refused, because it would hold its own worktrees as untracked files that a
// clean deletes.
func worktreeRepoPath(home, repo string) (string, string, error) {
	repoPath, err := ResolveDir(home, repo)
	if err != nil {
		return "", "", err
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", "", launchUnavailable("home is unavailable")
	}
	if repoPath, err = filepath.EvalSymlinks(repoPath); err != nil {
		return "", "", launchUnavailable("folder is unavailable")
	}
	if inside(repoPath, filepath.Join(realHome, ".reet")) {
		return "", "", refuseWorktree(terminalproto.WorktreeNotRepoRoot, "worktrees cannot be made inside the repository they come from")
	}
	return repoPath, realHome, nil
}

// checkWorktreeRepo makes the checks every worktree create makes on the
// repository before it changes anything, and answers the commit HEAD names:
// `.git` is a real directory of its own (not a file, not a symlink), no
// merge, rebase, cherry-pick, revert or bisect is in progress, the folder is
// the top of its working tree, and HEAD is a commit.
//
// PlanBase makes exactly these checks before it fetches, so a create that
// will be refused never reaches github.com for a base it cannot use.
func checkWorktreeRepo(ctx context.Context, git GitRunner, repoPath string) (string, error) {
	gitDir := filepath.Join(repoPath, ".git")
	info, err := os.Lstat(gitDir)
	validGitDir := err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
	if !validGitDir {
		return "", refuseWorktree(terminalproto.WorktreeNotRepoRoot, ".git is not a folder in this repository")
	}
	for _, marker := range busyMarkers {
		if _, err := os.Lstat(filepath.Join(gitDir, marker)); err == nil {
			return "", refuseWorktree(terminalproto.WorktreeRepoBusy, marker+" is present")
		}
	}
	out, err := git(ctx, repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, errGitMissing) || ctx.Err() != nil {
			return "", gitFailed(out, err)
		}
		return "", refuseWorktree(terminalproto.WorktreeNotRepoRoot, worktreeDetail(out))
	}
	top, err := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n"))
	if err != nil || top != repoPath {
		return "", refuseWorktree(terminalproto.WorktreeNotRepoRoot, "the folder is not the top of its working tree")
	}
	head, err := gitCommit(ctx, git, repoPath, "HEAD")
	if err != nil {
		return "", err
	}
	if head == "" {
		return "", refuseWorktree(terminalproto.WorktreeNotRepoRoot, "HEAD does not point at a commit yet")
	}
	return head, nil
}

// worktreePlace is where a run's worktree goes under a resolved home and
// repository: the folder that holds that project's worktrees for that
// repository, the destination itself and the branch. PlanBase works out the
// same destination, so a retry that will adopt one fetches nothing.
func worktreePlace(realHome, repoPath, project, run string) (parent, dest, branch string) {
	projectSlug := terminalproto.Slug(project, "project")
	repoSlug := terminalproto.Slug(filepath.Base(repoPath), "repo")
	parent = filepath.Join(realHome, ".reet", "worktrees", projectSlug, repoSlug)
	return parent, filepath.Join(parent, run), "reet/" + projectSlug + "-" + run
}

// baseAsBefore is the commit a worktree starts from on a connection that did
// not negotiate base-fresh-v1: refs/heads/<base>, then refs/remotes/<base>,
// and the literal HEAD when no base was named. It reaches nothing and is the
// lookup a create made before any base was ever fetched. A base neither place
// has is refuseWorktree(WorktreeBaseMissing, "").
func baseAsBefore(ctx context.Context, git GitRunner, repo, base string) (string, error) {
	if base == "" {
		return "HEAD", nil
	}
	for _, prefix := range []string{"refs/heads/", "refs/remotes/"} {
		commit, err := gitCommit(ctx, git, repo, prefix+base)
		if err != nil {
			return "", err
		}
		if commit != "" {
			return commit, nil
		}
	}
	return "", refuseWorktree(terminalproto.WorktreeBaseMissing, "")
}

func gitFailed(out []byte, err error) error {
	detail := worktreeDetail(out)
	if detail == "" && err != nil {
		known := errors.Is(err, errGitMissing) || errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, context.Canceled)
		if known {
			detail = err.Error()
		}
	}
	return refuseWorktree(terminalproto.WorktreeGitFailed, detail)
}

// cleanupFailedAdd removes what a failed `worktree add` may have left: the
// destination when git lists it, an empty destination folder, and the
// branch, which did not exist before this call.
//
// The destination did not exist before this call, so a worktree git left
// there (possibly still "locked initializing" after a killed add) is this
// call's: it is unlocked and then force-removed. `branch -D` is safe only
// because the branch name contains the run token, so it can be no one
// else's branch, and it did not exist when this call checked.
func cleanupFailedAdd(ctx context.Context, git GitRunner, w Worktree, budget time.Duration) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	defer cancel()
	if listed, _, err := gitWorktreeListed(ctx, git, w.repo, w.Path, w.Branch); err == nil && listed {
		_, _ = git(ctx, w.repo, "worktree", "unlock", w.Path)
		_, _ = git(ctx, w.repo, "worktree", "remove", "--force", w.Path)
	} else {
		_ = os.Remove(w.Path) // only an empty folder
	}
	if exists, err := gitBranchExists(ctx, git, w.repo, w.Branch); err == nil && exists {
		_, _ = git(ctx, w.repo, "branch", "-D", w.Branch)
	}
}

func gitBranchExists(ctx context.Context, git GitRunner, repo, branch string) (bool, error) {
	ref := "refs/heads/" + branch
	out, err := git(ctx, repo, "for-each-ref", "--format=%(refname)", ref)
	if err != nil {
		return false, gitFailed(out, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == ref {
			return true, nil
		}
	}
	return false, nil
}

// gitWorktreeListed reports whether `git worktree list --porcelain` has a
// line naming path, and whether it may be adopted: the whole listing parses
// as records that each start with a "worktree" line, exactly one record names
// path, and that record is exactly worktree, HEAD <hash> and branch lines on
// branch: not locked, not prunable, not detached. The listing is not -z
// (git 2.34 is supported), so a worktree path containing a newline can print
// extra lines or records; the strict shape keeps those from ever making a
// path adoptable, and a path with a newline is never adoptable itself.
func gitWorktreeListed(ctx context.Context, git GitRunner, repo, path, branch string) (listed, adoptable bool, err error) {
	out, err := git(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return false, false, gitFailed(out, err)
	}
	if len(out) >= gitListLimit {
		// Possibly cut short: the path may be missing or cut mid-line.
		return false, false, refuseWorktree(terminalproto.WorktreeGitFailed, "git listed too many worktrees to read")
	}
	wellFormed := true
	matches := 0
	exact := false
	var record []string
	finish := func() {
		if len(record) == 0 {
			return
		}
		if !strings.HasPrefix(record[0], "worktree ") {
			wellFormed = false
		} else if strings.TrimPrefix(record[0], "worktree ") == path {
			matches++
			exact = len(record) == 3 && validHeadLine(record[1]) && record[2] == "branch refs/heads/"+branch
		}
		record = nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "worktree "+path {
			listed = true
		}
		if line == "" {
			finish()
			continue
		}
		record = append(record, line)
	}
	finish()
	return listed, wellFormed && matches == 1 && exact && !hasControl(path), nil
}

func validHeadLine(line string) bool {
	hash, ok := strings.CutPrefix(line, "HEAD ")
	validLength := len(hash) == 40 || len(hash) == 64
	if !ok || !validLength {
		return false
	}
	for i := range hash {
		if !(hash[i] >= '0' && hash[i] <= '9' || hash[i] >= 'a' && hash[i] <= 'f') {
			return false
		}
	}
	return true
}

// ensureWorktreeDirs makes every folder from home down to dir, refusing a
// symlink, a non-directory, a folder another user owns, or one that group or
// others can write, at any step.
func ensureWorktreeDirs(home, dir string) error {
	rel, err := filepath.Rel(home, dir)
	valid := err == nil && rel != "." && filepath.IsLocal(rel)
	if !valid {
		return refuseWorktree(terminalproto.WorktreeOutsideHome, "")
	}
	current := home
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o700); err != nil && !os.IsExist(err) {
				return refuseWorktree(terminalproto.WorktreeOutsideHome, "a folder for worktrees cannot be made")
			}
			info, err = os.Lstat(current)
		}
		invalid := err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() ||
			ownedByCurrentUser(info) != nil || info.Mode().Perm()&0o022 != 0
		if invalid {
			return refuseWorktree(terminalproto.WorktreeOutsideHome, filepath.Join("~", strings.TrimPrefix(current, home))+" is not a folder of yours")
		}
	}
	return nil
}

// inside reports whether path is dir or below it.
func inside(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

// hasControl reports control characters and invisible format characters
// (bidi overrides and the like), which a path shown to a person must not hold.
func hasControl(s string) bool {
	for _, r := range s {
		isControl := unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
		isSeparator := r == '\u2028' || r == '\u2029'
		if isControl || isSeparator {
			return true
		}
	}
	return false
}

// ErrWorktreeNotFound means nothing is at the run's worktree path: it was
// never made, or it has already been removed (by reet, by git or by hand). For
// a remove the controller may treat it as already done; something that is at
// the path but is not a registered worktree is refused instead
// (not_registered), never reported as not found.
var ErrWorktreeNotFound = errors.New("the worktree is not there")

// ErrWorktreeChanged means a remove was refused because the worktree's counts
// are no longer the ones the person was shown.
var ErrWorktreeChanged = errors.New("the worktree changed since it was shown")

// WorktreeCounts are the two numbers a person is shown before a remove.
type WorktreeCounts struct {
	Changed, Unpushed int
	// Discard is the person's explicit confirmation to delete uncommitted
	// changes, and Fingerprint the status fingerprint they were shown.
	Discard     bool
	Fingerprint string
}

// worktreeAt is a run's worktree that passed every check in locateWorktree.
type worktreeAt struct {
	path, branch string
	// root is the working tree of the repository the worktree belongs to.
	root string
}

// WorktreeStatus reports a run's worktree without changing it:
//
//   - Changed counts the entries of
//     `git -C <path> status --porcelain=v1 -z --untracked-files=all --ignore-submodules=none`
//     (modified, staged, deleted, renamed and untracked files each count;
//     ignored files do not). The two flags override a repository's
//     status.showUntrackedFiles and submodule settings, which would otherwise
//     hide files that a remove deletes. Output beyond 4 MiB (gitListLimit)
//     is not read, so past tens of thousands of entries Changed is a lower
//     bound; it is never 0 when anything changed.
//   - Paths are the first 20 of those paths, each at most 255 bytes, with
//     invalid UTF-8 and control or format characters replaced by U+FFFD.
//     File contents are never read.
//   - Unpushed is `git -C <path> rev-list --count HEAD --not --remotes
//     --exclude=<run's branch> [--exclude=<HEAD's branch>] --branches`:
//     commits reachable from the worktree's HEAD that are on no
//     remote-tracking branch and on no local branch other than the run's own
//     branch and the branch checked out there (if any). A fresh
//     worktree is 0, because its starting commit is on the branch it was made
//     from; in a repository with no remote it is the commits made in the
//     worktree since then. Commits the person also put on another local
//     branch are not counted; neither is anything on a branch that removal
//     keeps at risk, since the branch is never deleted.
//   - Unreferenced is `rev-list --count HEAD --not --branches --remotes
//     --tags`: commits (on a detached HEAD) that nothing but the worktree
//     holds and that a remove would leave unreachable.
//   - Hidden counts `ls-files -v -z` entries marked skip-worktree (also every
//     file outside a sparse checkout) or assume-unchanged: edits to them are
//     invisible to status and to `worktree remove`.
//   - Nested is a `.git` entry below the top level (see scanWorktree): an
//     ignored clone or worktree whose work a remove would delete.
//   - Incomplete: the scan or the ls-files listing could not be read whole.
//
// The path is ~/.reet/worktrees/<project>/<Slug(repo)>/<run>, derived exactly
// as PrepareWorktree derives it, and must be a registered worktree of its
// repository (see locateWorktree). Nothing at the path: ErrWorktreeNotFound.
func WorktreeStatus(ctx context.Context, home, project, repo, run string, git GitRunner) (terminalproto.WorktreeState, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	w, err := locateWorktree(ctx, home, project, repo, run, git)
	if err != nil {
		return terminalproto.WorktreeState{}, err
	}
	return worktreeState(ctx, w, git)
}

func worktreeState(ctx context.Context, w worktreeAt, git GitRunner) (terminalproto.WorktreeState, error) {
	out, err := git(ctx, w.path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return terminalproto.WorktreeState{}, gitFailed(out, err)
	}
	changed, paths := parseStatus(out)
	state := terminalproto.WorktreeState{Path: w.path, Branch: w.branch, Changed: changed, Paths: paths}
	if changed > 0 {
		if state.Fingerprint, err = changeFingerprint(ctx, w, git, out); err != nil {
			return terminalproto.WorktreeState{}, err
		}
	}
	if state.Unpushed, err = unpushedCount(ctx, w, git); err != nil {
		return terminalproto.WorktreeState{}, err
	}
	tips, stash, complete, err := worktreeRefTips(ctx, git, w.path)
	if err != nil {
		return terminalproto.WorktreeState{}, err
	}
	state.Incomplete = !complete
	args := append(append([]string{"rev-list", "--count", "HEAD"}, tips...), "--not", "--branches", "--remotes", "--tags")
	args = append(args, stash...)
	if state.Unreferenced, err = gitCount(ctx, git, w.path, args...); err != nil {
		return terminalproto.WorktreeState{}, err
	}
	out, err = git(ctx, w.path, "ls-files", "-v", "-z")
	if err != nil {
		return terminalproto.WorktreeState{}, gitFailed(out, err)
	}
	state.Hidden = hiddenEntries(out, w.path)
	if len(out) >= gitListLimit {
		state.Incomplete = true
	}
	nested, incomplete := scanWorktree(ctx, w.path, scanEntryLimit, scanDepthLimit)
	state.Nested = nested
	state.Incomplete = state.Incomplete || incomplete
	return state, nil
}

// hiddenEntries counts `ls-files -v -z` entries ("T path\0") that git does
// not look at for edits, so neither status nor `worktree remove` would see
// one: a lower-case tag (assume-unchanged) always, and "S" (skip-worktree)
// when the file is on disk in the worktree. A skip-worktree file that is not
// there (the usual case outside a sparse checkout) holds no edit.
func hiddenEntries(out []byte, worktree string) int {
	n := 0
	for _, entry := range strings.Split(string(out), "\x00") {
		if len(entry) < 3 || entry[1] != ' ' {
			continue
		}
		switch {
		case entry[0] >= 'a' && entry[0] <= 'z':
			n++
		case entry[0] == 'S':
			if _, err := os.Lstat(filepath.Join(worktree, filepath.FromSlash(entry[2:]))); err == nil {
				n++
			}
		}
	}
	return n
}

// maxWorktreeRefTips bounds the per-worktree refs read as extra tips.
const maxWorktreeRefTips = 1000

// worktreeRefTips reads `for-each-ref --format=%(objectname) %(objecttype)
// %(refname) refs/worktree refs/bisect refs/rewritten refs/stash` in the
// worktree. Commits and tags held by refs/worktree/*, refs/bisect/* and
// refs/rewritten/* (the three per-worktree ref namespaces git documents,
// deleted with it) are returned as tips whose commits count as unreferenced;
// refs/stash (shared, it survives the remove) is returned as stash, a commit
// it holds is protected. complete is false when the listing may be cut short
// or there are more than maxWorktreeRefTips tips.
func worktreeRefTips(ctx context.Context, git GitRunner, worktree string) (tips, stash []string, complete bool, err error) {
	out, err := git(ctx, worktree, "for-each-ref", "--format=%(objectname) %(objecttype) %(refname)", "refs/worktree", "refs/bisect", "refs/rewritten", "refs/stash")
	if err != nil {
		return nil, nil, false, gitFailed(out, err)
	}
	if len(out) >= gitListLimit {
		return nil, nil, false, nil
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, " ", 3)
		if len(fields) != 3 || !validHeadLine("HEAD "+fields[0]) {
			return nil, nil, false, refuseWorktree(terminalproto.WorktreeGitFailed, "git listed refs unreadably")
		}
		if fields[1] != "commit" && fields[1] != "tag" {
			continue
		}
		if fields[2] == "refs/stash" {
			stash = append(stash, fields[0])
		} else {
			tips = append(tips, fields[0])
		}
	}
	if len(tips) > maxWorktreeRefTips {
		return nil, nil, false, nil
	}
	return tips, stash, true, nil
}

// The scan's caps; variables only so tests can lower them.
var (
	scanEntryLimit = 200_000
	scanDepthLimit = 256
)

// gitLikeName reports a name git or a person would use for a git directory:
// ".git" in any letter case (a case-insensitive file system finds ".Git"
// for ".git") or any name ending in ".git" (a bare repository, "keep.git").
func gitLikeName(name string) bool {
	return len(name) >= 4 && strings.EqualFold(name[len(name)-4:], ".git")
}

// hasGitDirLayout reports a folder laid out like a git directory: a HEAD
// entry and objects and refs folders, as a bare repository of any name has.
func hasGitDirLayout(dir string) bool {
	if _, err := os.Lstat(filepath.Join(dir, "HEAD")); err != nil {
		return false
	}
	for _, sub := range []string{"objects", "refs"} {
		if info, err := os.Lstat(filepath.Join(dir, sub)); err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

// scanWorktree walks the worktree without following symlinks and reports,
// anywhere below its top level, another repository or worktree (possibly
// ignored) that a remove would delete with its own uncommitted work and
// commits: an entry whose name is gitLikeName (file, folder or symlink), or a
// folder with a git directory's layout (hasGitDirLayout). Only the
// worktree's own top-level `.git` file is skipped. More than maxEntries
// entries, a path deeper than maxDepth, a folder that cannot be read, or the
// end of ctx (the operation's budget) makes the scan incomplete.
func scanWorktree(ctx context.Context, path string, maxEntries, maxDepth int) (nested, incomplete bool) {
	entries := 0
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || ctx.Err() != nil {
			incomplete = true
			return fs.SkipAll
		}
		if p == path {
			return nil
		}
		entries++
		rel, _ := filepath.Rel(path, p)
		if entries > maxEntries || strings.Count(rel, string(filepath.Separator))+1 > maxDepth {
			incomplete = true
			return fs.SkipAll
		}
		if rel == ".git" {
			return nil // the worktree's own .git file (checked by locateWorktree)
		}
		if gitDir := d.IsDir() && hasGitDirLayout(p); gitLikeName(d.Name()) || gitDir {
			nested = true
			return fs.SkipAll
		}
		return nil
	})
	return nested, incomplete
}

// gitCount runs a `rev-list --count` and reads its number strictly.
func gitCount(ctx context.Context, git GitRunner, dir string, args ...string) (int, error) {
	out, err := git(ctx, dir, args...)
	if err != nil {
		return 0, gitFailed(out, err)
	}
	text := strings.TrimSuffix(string(out), "\n")
	n := 0
	for i := range text {
		notDigit := text[i] < '0' || text[i] > '9'
		if notDigit || n > 1<<30 {
			return 0, refuseWorktree(terminalproto.WorktreeGitFailed, "git counted commits unreadably")
		}
		n = n*10 + int(text[i]-'0')
	}
	if text == "" || n > 1<<30 {
		return 0, refuseWorktree(terminalproto.WorktreeGitFailed, "git counted commits unreadably")
	}
	return n, nil
}

// unpushedCount: see WorktreeStatus. The run's branch and HEAD's own branch
// are left out of the local branches that hide a commit, so a commit counts
// whether HEAD is on the run's branch, on another branch or detached.
func unpushedCount(ctx context.Context, w worktreeAt, git GitRunner) (int, error) {
	args := []string{"rev-list", "--count", "HEAD", "--not", "--remotes", "--exclude=" + w.branch}
	out, err := git(ctx, w.path, "symbolic-ref", "-q", "HEAD")
	ref := strings.TrimSuffix(string(out), "\n")
	validRef := err == nil && strings.HasPrefix(ref, "refs/heads/") &&
		!strings.ContainsAny(ref, "*?[\\\n")
	if validRef {
		if branch := strings.TrimPrefix(ref, "refs/heads/"); branch != w.branch {
			args = append(args, "--exclude="+branch)
		}
	} else {
		benign := err == nil || ctx.Err() != nil || errors.Is(err, errGitMissing)
		if benign {
			// symbolic-ref succeeded with something unexpected, or git itself failed.
			return 0, gitFailed(out, err)
		}
	}
	args = append(args, "--branches")
	return gitCount(ctx, git, w.path, args...)
}

// parseStatus counts `status --porcelain=v1 -z` entries ("XY path\0", plus
// "orig\0" after a rename or copy) and keeps the first MaxWorktreePaths paths,
// made plain. A truncated last entry still counts.
func parseStatus(out []byte) (int, []string) {
	fields := strings.Split(string(out), "\x00")
	count := 0
	var paths []string
	// Index loop: the body steps past a rename's original path, which range cannot express.
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		count++
		if len(paths) < terminalproto.MaxWorktreePaths {
			paths = append(paths, plainPath(entry[3:]))
		}
		renamed := entry[0] == 'R' || entry[0] == 'C'
		copied := entry[1] == 'R' || entry[1] == 'C'
		if renamed || copied {
			i++ // the original path
		}
	}
	return count, paths
}

// plainPath makes a path safe to show: invalid UTF-8 and every control or
// format character (newline, ESC, bidi overrides, U+2028/9) become U+FFFD,
// and it is cut to MaxWorktreePathBytes on a rune boundary.
func plainPath(p string) string {
	var b strings.Builder
	for _, r := range strings.ToValidUTF8(p, "�") {
		isControl := unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
		isSeparator := r == '\u2028' || r == '\u2029'
		if isControl || isSeparator {
			r = '�'
		}
		if b.Len()+utf8.RuneLen(r) > terminalproto.MaxWorktreePathBytes {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

// WorktreeRemove removes a run's worktree only when nothing would be lost
// that the person was not shown:
//
//  1. The worktree is located and checked exactly as WorktreeStatus does.
//  2. The status is computed again; unless both counts equal shown, the
//     remove is refused with ErrWorktreeChanged, so changes made after the
//     person looked are never thrown away.
//  3. Any uncommitted change (Changed > 0) is refused as has_changes unless
//     the person confirmed discarding them (shown.Discard, then --force): git
//     would refuse it without --force, and reet never forces. Hidden edits
//     (hidden_changes), commits only HEAD holds (unreferenced_commits), a
//     nested repository (nested_repository) and an incomplete check
//     (check_incomplete) are refused whatever the person was shown.
//  4. It runs exactly `git -C <repository root> worktree remove <path>`,
//     never --force. The repository root comes from the worktree's
//     `rev-parse --git-common-dir`. Git checks again that the worktree is
//     clean and not locked, so a file written after step 2 is still kept.
//
// The branch is never deleted, so commits (Unpushed) stay on it. Ignored
// files are not counted and are deleted with the folder. Nothing at the path:
// ErrWorktreeNotFound. Every git call shares one 60-second budget.
func WorktreeRemove(ctx context.Context, home, project, repo, run string, shown WorktreeCounts, git GitRunner) error {
	_, err := RemoveWorktree(ctx, home, project, repo, run, shown, git)
	return err
}

// A remove is bounded by 60 seconds in all: its checks share
// removeCheckTimeout, then `git worktree remove` gets removeStepTimeout of its
// own. The controller waits longer than a create and a remove back to back.
const (
	removeCheckTimeout = 30 * time.Second
	removeStepTimeout  = 30 * time.Second
)

// WorktreeRemoved says which folder a remove deleted and which branch it kept.
type WorktreeRemoved struct {
	Path   string
	Branch string
}

// RemoveWorktree is WorktreeRemove reporting what it removed. Every check
// runs first, sharing one removeCheckTimeout budget that also ends with ctx;
// the one changing call, `git worktree remove`, then gets removeStepTimeout of
// its own that the end of ctx does not cut short, so a remove that has started
// is never stopped half way through deleting the folder.
func RemoveWorktree(ctx context.Context, home, project, repo, run string, shown WorktreeCounts, git GitRunner) (WorktreeRemoved, error) {
	checkCtx, cancel := context.WithTimeout(ctx, removeCheckTimeout)
	defer cancel()
	w, err := locateWorktree(checkCtx, home, project, repo, run, git)
	if err != nil {
		return WorktreeRemoved{}, err
	}
	state, err := worktreeState(checkCtx, w, git)
	if err != nil {
		return WorktreeRemoved{}, err
	}
	if state.Changed != shown.Changed || state.Unpushed != shown.Unpushed {
		return WorktreeRemoved{}, ErrWorktreeChanged
	}
	switch {
	case state.Changed > 0 && !shown.Discard:
		return WorktreeRemoved{}, refuseWorktree(terminalproto.WorktreeHasChanges, "")
	case state.Changed > 0 && (state.Fingerprint == "" || state.Fingerprint != shown.Fingerprint):
		// The counts match but the edits are not the ones the person saw.
		return WorktreeRemoved{}, ErrWorktreeChanged
	case state.Hidden > 0:
		return WorktreeRemoved{}, refuseWorktree(terminalproto.WorktreeHiddenChanges, "")
	case state.Unreferenced > 0:
		return WorktreeRemoved{}, refuseWorktree(terminalproto.WorktreeUnreferencedCommits, "")
	case state.Nested:
		return WorktreeRemoved{}, refuseWorktree(terminalproto.WorktreeNestedRepository, "")
	case state.Incomplete:
		return WorktreeRemoved{}, refuseWorktree(terminalproto.WorktreeCheckIncomplete, "")
	}
	if checkCtx.Err() != nil {
		return WorktreeRemoved{}, gitFailed(nil, checkCtx.Err())
	}
	removeCtx, cancelRemove := context.WithTimeout(context.WithoutCancel(ctx), removeStepTimeout)
	defer cancelRemove()
	args := []string{"worktree", "remove"}
	if shown.Discard && state.Changed > 0 {
		// Only with the person's explicit confirmation, and only for the
		// changes they were shown: every other check above still refused.
		args = append(args, "--force")
	}
	if out, err := git(removeCtx, w.root, append(args, w.path)...); err != nil {
		return WorktreeRemoved{}, gitFailed(out, err)
	}
	return WorktreeRemoved{Path: w.path, Branch: w.branch}, nil
}

// locateWorktree finds a run's worktree and checks it before anything reads
// or removes it:
//
//   - the path is <real home>/.reet/worktrees/<project>/<Slug(repo)>/<run>,
//     every folder from home down to it a real directory (no symlink), the
//     folders above it owned by this user and not writable by others, and the
//     resolved path inside ~/.reet/worktrees: otherwise outside_home;
//   - nothing at the path (or any folder above it missing): ErrWorktreeNotFound;
//   - the path must be a linked worktree: `.git` a regular file,
//     `rev-parse --show-toplevel` the path itself, `rev-parse --git-common-dir`
//     the `.git` folder of a normal repository whose folder name slugs to
//     Slug(repo), and `worktree list --porcelain` naming that repository first
//     and the path exactly once, not first and not prunable: otherwise
//     not_registered.
func locateWorktree(ctx context.Context, home, project, repo, run string, git GitRunner) (worktreeAt, error) {
	valid := terminalproto.ValidRunToken(run) && terminalproto.ValidSlug(project) &&
		repo != "" && git != nil
	if !valid {
		return worktreeAt{}, terminalproto.ErrInvalid
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil || !filepath.IsAbs(realHome) {
		return worktreeAt{}, refuseWorktree(terminalproto.WorktreeOutsideHome, "the home folder is unavailable")
	}
	worktrees := filepath.Join(realHome, ".reet", "worktrees")
	projectSlug := terminalproto.Slug(project, "project")
	repoSlug := terminalproto.Slug(repo, "repo")
	path := filepath.Join(worktrees, projectSlug, repoSlug, run)
	w := worktreeAt{path: path, branch: "reet/" + projectSlug + "-" + run}
	if hasControl(path) {
		return worktreeAt{}, refuseWorktree(terminalproto.WorktreeOutsideHome, "the home folder's path cannot hold a worktree")
	}
	if err := checkWorktreePath(realHome, path); err != nil {
		return worktreeAt{}, err
	}
	// Redundant guards, kept on purpose: checkWorktreePath has already refused
	// every symlink, so these cannot fail today and a mutant that removes
	// either one alone survives the tests. They keep the "never outside
	// ~/.reet/worktrees" rule true if the walk above is ever changed.
	resolved, err := filepath.EvalSymlinks(path)
	validResolved := err == nil && inside(worktrees, resolved) && resolved != worktrees
	if !validResolved {
		return worktreeAt{}, refuseWorktree(terminalproto.WorktreeOutsideHome, "the worktree is not under ~/.reet/worktrees")
	}
	if resolved != path {
		return worktreeAt{}, refuseWorktree(terminalproto.WorktreeOutsideHome, "the worktree folder is not a real folder")
	}
	notRegistered := func(detail string) error { return refuseWorktree(terminalproto.WorktreeNotRegistered, detail) }
	if info, err := os.Lstat(filepath.Join(path, ".git")); err != nil || !info.Mode().IsRegular() {
		return worktreeAt{}, notRegistered("the folder is not a git worktree")
	}
	readFailed := func(out []byte, err error) error {
		if errors.Is(err, errGitMissing) || ctx.Err() != nil {
			return gitFailed(out, err)
		}
		return notRegistered("git does not see a worktree there")
	}
	out, err := git(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return worktreeAt{}, readFailed(out, err)
	}
	if top, err := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n")); err != nil || top != path {
		return worktreeAt{}, notRegistered("the folder is not the top of a worktree")
	}
	out, err = git(ctx, path, "rev-parse", "--git-common-dir")
	if err != nil {
		return worktreeAt{}, readFailed(out, err)
	}
	common := strings.TrimSuffix(string(out), "\n")
	if !filepath.IsAbs(common) {
		common = filepath.Join(path, common)
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil || filepath.Base(common) != ".git" {
		return worktreeAt{}, notRegistered("the worktree does not belong to a normal repository")
	}
	w.root = filepath.Dir(common)
	invalidRoot := w.root == path || inside(worktrees, w.root)
	validSlug := terminalproto.Slug(filepath.Base(w.root), "repo") == repoSlug
	if invalidRoot || !validSlug {
		return worktreeAt{}, notRegistered("the worktree belongs to another repository")
	}
	out, err = git(ctx, path, "worktree", "list", "--porcelain")
	if err != nil {
		return worktreeAt{}, readFailed(out, err)
	}
	if len(out) >= gitListLimit || !registeredWorktree(string(out), w.root, path) {
		return worktreeAt{}, notRegistered("git does not list this worktree")
	}
	return w, nil
}

// registeredWorktree reports whether a `worktree list --porcelain` listing is
// well formed (every record starts with a worktree line), names root first
// and names path in exactly one later record that is not prunable.
func registeredWorktree(listing, root, path string) bool {
	var records [][]string
	var record []string
	for _, line := range strings.Split(listing, "\n") {
		if line == "" {
			if len(record) > 0 {
				records = append(records, record)
			}
			record = nil
			continue
		}
		record = append(record, line)
	}
	if len(record) > 0 {
		records = append(records, record)
	}
	if len(records) < 2 {
		return false
	}
	matches := 0
	for i, r := range records {
		name, ok := strings.CutPrefix(r[0], "worktree ")
		if !ok {
			return false
		}
		if i == 0 {
			if resolved, err := filepath.EvalSymlinks(name); err != nil || resolved != root {
				return false
			}
			if name == path {
				return false
			}
			continue
		}
		if name != path {
			continue
		}
		matches++
		for _, line := range r[1:] {
			bare := line == "prunable" || line == "bare"
			if bare || strings.HasPrefix(line, "prunable ") {
				return false
			}
		}
	}
	return matches == 1
}

// checkWorktreePath walks from home down to path without creating anything.
// A missing folder is ErrWorktreeNotFound; a symlink anywhere, a non-folder
// above the path, or a folder above it that another user owns or others can
// write, is outside_home. The path itself must be a real folder of this user;
// a file there is not_registered.
func checkWorktreePath(home, path string) error {
	rel, err := filepath.Rel(home, path)
	valid := err == nil && rel != "." && filepath.IsLocal(rel)
	if !valid {
		return refuseWorktree(terminalproto.WorktreeOutsideHome, "")
	}
	parts := strings.Split(rel, string(filepath.Separator))
	current := home
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return ErrWorktreeNotFound
		}
		shown := filepath.Join("~", strings.TrimPrefix(current, home))
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return refuseWorktree(terminalproto.WorktreeOutsideHome, shown+" is not a folder of yours")
		}
		if i == len(parts)-1 {
			if !info.IsDir() {
				return refuseWorktree(terminalproto.WorktreeNotRegistered, "the folder is not a git worktree")
			}
			if ownedByCurrentUser(info) != nil {
				return refuseWorktree(terminalproto.WorktreeOutsideHome, shown+" is not a folder of yours")
			}
			continue
		}
		valid := info.IsDir() && ownedByCurrentUser(info) == nil && info.Mode().Perm()&0o022 == 0
		if !valid {
			return refuseWorktree(terminalproto.WorktreeOutsideHome, shown+" is not a folder of yours")
		}
	}
	return nil
}

// fingerprintFileLimit bounds how much of one changed file is read into the
// fingerprint; a larger file contributes its size and modification time.
const fingerprintFileLimit = 64 << 20

// changeFingerprint summarises the uncommitted changes: git's status, what the
// index holds against HEAD, and for every changed path its kind and content.
// A discard repeats it, so a path edited, reverted or added after the person
// looked gives a different value even when the counts are the same.
func changeFingerprint(ctx context.Context, w worktreeAt, git GitRunner, status []byte) (string, error) {
	h := sha256.New()
	h.Write(status)
	staged, err := git(ctx, w.path, "diff-index", "--cached", "-z", "--no-renames", "HEAD")
	if err != nil {
		return "", gitFailed(staged, err)
	}
	h.Write([]byte{0})
	h.Write(staged)
	fields := strings.Split(string(status), "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		if entry[0] == 'R' || entry[0] == 'C' || entry[1] == 'R' || entry[1] == 'C' {
			i++
		}
		h.Write([]byte{0})
		if err := hashPathContent(h, filepath.Join(w.path, filepath.FromSlash(entry[3:]))); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashPathContent(h interface{ Write([]byte) (int, error) }, p string) error {
	info, err := os.Lstat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		h.Write([]byte("absent"))
		return nil
	case err != nil:
		return refuseWorktree(terminalproto.WorktreeCheckIncomplete, "")
	}
	fmt.Fprintf(h, "%o %d ", info.Mode(), info.Size())
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(p)
		if err != nil {
			return refuseWorktree(terminalproto.WorktreeCheckIncomplete, "")
		}
		h.Write([]byte(target))
	case info.Mode().IsRegular() && info.Size() <= fingerprintFileLimit:
		f, err := os.Open(p)
		if err != nil {
			return refuseWorktree(terminalproto.WorktreeCheckIncomplete, "")
		}
		defer f.Close()
		sum := sha256.New()
		if _, err := io.Copy(sum, f); err != nil {
			return refuseWorktree(terminalproto.WorktreeCheckIncomplete, "")
		}
		h.Write(sum.Sum(nil))
	default:
		fmt.Fprintf(h, "%d", info.ModTime().UnixNano())
	}
	return nil
}
