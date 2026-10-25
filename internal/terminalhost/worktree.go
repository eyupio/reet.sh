package terminalhost

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
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
	// repository (worktree list, status, ls-files); output this long may have
	// been cut short and is never trusted as complete.
	gitListLimit = 4 << 20
)

// outputLimit is gitListLimit for the read-only listings and gitOutputLimit
// for every other git call.
func outputLimit(args []string) int {
	if len(args) > 0 && (args[0] == "status" || args[0] == "ls-files" || len(args) > 1 && args[0] == "worktree" && args[1] == "list") {
		return gitListLimit
	}
	return gitOutputLimit
}

var errGitMissing = errors.New("git is not installed")

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
	cmd.Env = gitEnvironment(os.Environ())
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

// gitEnvironment keeps only HOME, PATH and LANG (the first of each). No
// GIT_* variable (GIT_DIR, GIT_WORK_TREE, GIT_CONFIG_*, ...) reaches git.
func gitEnvironment(environ []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, kv := range environ {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || seen[key] || key != "HOME" && key != "PATH" && key != "LANG" {
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
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ' {
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
// from a retry) is never undone.
type Worktree struct {
	Path, Branch string
	Created      bool
	repo         string
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
//     bisect in progress, and HEAD a commit. A dirty tree is allowed.
//  2. The destination is <home>/.reet/worktrees/<project slug>/<repo slug>/<run>
//     and the branch reet/<project slug>-<run>. Every folder up to the
//     destination's parent must be a real directory owned by this user (new
//     ones are made 0700); a symlink anywhere is refused.
//  3. If the destination or branch exists it is adopted only when git lists
//     exactly that path on exactly that branch for this repository (Created
//     false); anything else is refused.
//  4. Otherwise it runs exactly
//     `git -C <repo> -c core.hooksPath=/dev/null worktree add -b <branch> <destination> HEAD`.
//
// Refusals are *WorktreeError (errors.Is ErrWorktreeRefused); a folder that
// ResolveDir refuses keeps its ErrLaunchUnavailable error.
func PrepareWorktree(ctx context.Context, home, repo, project, run string, git GitRunner) (Worktree, error) {
	return prepareWorktree(ctx, home, repo, project, run, git, gitTimeout, cleanupTimeout)
}

// prepareWorktree bounds every git call it makes by one budget (60 s in
// PrepareWorktree; runGit also caps each call); cleaning up a failed add has
// its own budget so it still runs after that one has ended.
func prepareWorktree(ctx context.Context, home, repo, project, run string, git GitRunner, budget, cleanupBudget time.Duration) (Worktree, error) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if !terminalproto.ValidRunToken(run) || git == nil {
		return Worktree{}, terminalproto.ErrInvalid
	}
	repoPath, err := ResolveDir(home, repo)
	if err != nil {
		return Worktree{}, err
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return Worktree{}, launchUnavailable("home is unavailable")
	}
	if repoPath, err = filepath.EvalSymlinks(repoPath); err != nil {
		return Worktree{}, launchUnavailable("folder is unavailable")
	}
	base := filepath.Join(realHome, ".reet")
	// A repository that contains ~/.reet (home itself, or a parent of it) would
	// hold its own worktrees as untracked files that a clean deletes.
	if inside(repoPath, base) {
		return Worktree{}, refuseWorktree(terminalproto.WorktreeNotRepoRoot, "worktrees cannot be made inside the repository they come from")
	}
	gitDir := filepath.Join(repoPath, ".git")
	info, err := os.Lstat(gitDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Worktree{}, refuseWorktree(terminalproto.WorktreeNotRepoRoot, ".git is not a folder in this repository")
	}
	for _, marker := range busyMarkers {
		if _, err := os.Lstat(filepath.Join(gitDir, marker)); err == nil {
			return Worktree{}, refuseWorktree(terminalproto.WorktreeRepoBusy, marker+" is present")
		}
	}
	out, err := git(ctx, repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, errGitMissing) || ctx.Err() != nil {
			return Worktree{}, gitFailed(out, err)
		}
		return Worktree{}, refuseWorktree(terminalproto.WorktreeNotRepoRoot, worktreeDetail(out))
	}
	top, err := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n"))
	if err != nil || top != repoPath {
		return Worktree{}, refuseWorktree(terminalproto.WorktreeNotRepoRoot, "the folder is not the top of its working tree")
	}
	if out, err := git(ctx, repoPath, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err != nil || strings.TrimSpace(string(out)) == "" {
		if errors.Is(err, errGitMissing) || ctx.Err() != nil {
			return Worktree{}, gitFailed(out, err)
		}
		return Worktree{}, refuseWorktree(terminalproto.WorktreeNotRepoRoot, "HEAD does not point at a commit yet")
	}

	projectSlug := terminalproto.Slug(project, "project")
	repoSlug := terminalproto.Slug(filepath.Base(repoPath), "repo")
	parent := filepath.Join(base, "worktrees", projectSlug, repoSlug)
	dest := filepath.Join(parent, run)
	branch := "reet/" + projectSlug + "-" + run
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
	if destExists || branchExists || listed {
		if destExists && destInfo.IsDir() && destInfo.Mode()&os.ModeSymlink == 0 && branchExists && adoptable {
			return w, nil
		}
		return Worktree{}, refuseWorktree(terminalproto.WorktreeExists, "")
	}

	out, err = git(ctx, repoPath, "-c", "core.hooksPath=/dev/null", "worktree", "add", "-b", branch, dest, "HEAD")
	if err != nil {
		cleanupFailedAdd(ctx, git, w, cleanupBudget)
		return Worktree{}, gitFailed(out, err)
	}
	w.Created = true
	return w, nil
}

func gitFailed(out []byte, err error) error {
	detail := worktreeDetail(out)
	if detail == "" && err != nil {
		if errors.Is(err, errGitMissing) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
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
	if !ok || len(hash) != 40 && len(hash) != 64 {
		return false
	}
	for i := 0; i < len(hash); i++ {
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
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
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
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || ownedByCurrentUser(info) != nil || info.Mode().Perm()&0o022 != 0 {
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
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
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
type WorktreeCounts struct{ Changed, Unpushed int }

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
	if state.Unpushed, err = unpushedCount(ctx, w, git); err != nil {
		return terminalproto.WorktreeState{}, err
	}
	if state.Unreferenced, err = gitCount(ctx, git, w.path, "rev-list", "--count", "HEAD", "--not", "--branches", "--remotes", "--tags"); err != nil {
		return terminalproto.WorktreeState{}, err
	}
	out, err = git(ctx, w.path, "ls-files", "-v", "-z")
	if err != nil {
		return terminalproto.WorktreeState{}, gitFailed(out, err)
	}
	state.Hidden = hiddenEntries(out)
	if len(out) >= gitListLimit {
		state.Incomplete = true
	}
	nested, incomplete := scanWorktree(w.path, maxScanEntries, maxScanDepth)
	state.Nested = nested
	state.Incomplete = state.Incomplete || incomplete
	return state, nil
}

// hiddenEntries counts `ls-files -v -z` entries ("T path\0") whose tag is
// "S" (skip-worktree, including everything outside a sparse checkout) or a
// lower-case letter (assume-unchanged): git does not look at these files for
// edits, so neither status nor `worktree remove` would see one.
func hiddenEntries(out []byte) int {
	n := 0
	for _, entry := range strings.Split(string(out), "\x00") {
		if len(entry) >= 3 && entry[1] == ' ' && (entry[0] == 'S' || entry[0] >= 'a' && entry[0] <= 'z') {
			n++
		}
	}
	return n
}

const (
	maxScanEntries = 200_000
	maxScanDepth   = 256
)

// scanWorktree walks the worktree without following symlinks and reports a
// `.git` entry (file, folder or symlink) anywhere below its top level: another
// repository or worktree, possibly ignored, that a remove would delete with
// its own uncommitted work and commits. Only the worktree's own top-level
// `.git` file is skipped. More than maxEntries entries, a path deeper than
// maxDepth, or a folder that cannot be read makes the scan incomplete.
func scanWorktree(path string, maxEntries, maxDepth int) (nested, incomplete bool) {
	entries := 0
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
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
		if d.Name() == ".git" {
			if rel == ".git" {
				return nil
			}
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
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' || n > 1<<30 {
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
	if ref := strings.TrimSuffix(string(out), "\n"); err == nil && strings.HasPrefix(ref, "refs/heads/") && !strings.ContainsAny(ref, "*?[\\\n") {
		if branch := strings.TrimPrefix(ref, "refs/heads/"); branch != w.branch {
			args = append(args, "--exclude="+branch)
		}
	} else if err == nil || ctx.Err() != nil || errors.Is(err, errGitMissing) {
		// symbolic-ref succeeded with something unexpected, or git itself failed.
		return 0, gitFailed(out, err)
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
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		count++
		if len(paths) < terminalproto.MaxWorktreePaths {
			paths = append(paths, plainPath(entry[3:]))
		}
		if entry[0] == 'R' || entry[0] == 'C' || entry[1] == 'R' || entry[1] == 'C' {
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
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ' {
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
//  3. Any uncommitted change (Changed > 0) is refused as has_changes: git
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
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	w, err := locateWorktree(ctx, home, project, repo, run, git)
	if err != nil {
		return err
	}
	state, err := worktreeState(ctx, w, git)
	if err != nil {
		return err
	}
	if state.Changed != shown.Changed || state.Unpushed != shown.Unpushed {
		return ErrWorktreeChanged
	}
	switch {
	case state.Changed > 0:
		return refuseWorktree(terminalproto.WorktreeHasChanges, "")
	case state.Hidden > 0:
		return refuseWorktree(terminalproto.WorktreeHiddenChanges, "")
	case state.Unreferenced > 0:
		return refuseWorktree(terminalproto.WorktreeUnreferencedCommits, "")
	case state.Nested:
		return refuseWorktree(terminalproto.WorktreeNestedRepository, "")
	case state.Incomplete:
		return refuseWorktree(terminalproto.WorktreeCheckIncomplete, "")
	}
	if out, err := git(ctx, w.root, "worktree", "remove", w.path); err != nil {
		return gitFailed(out, err)
	}
	return nil
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
	if !terminalproto.ValidRunToken(run) || !terminalproto.ValidSlug(project) || repo == "" || git == nil {
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
	if err != nil || !inside(worktrees, resolved) || resolved == worktrees {
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
	if w.root == path || inside(worktrees, w.root) || terminalproto.Slug(filepath.Base(w.root), "repo") != repoSlug {
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
			if line == "prunable" || strings.HasPrefix(line, "prunable ") || line == "bare" {
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
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
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
		if !info.IsDir() || ownedByCurrentUser(info) != nil || info.Mode().Perm()&0o022 != 0 {
			return refuseWorktree(terminalproto.WorktreeOutsideHome, shown+" is not a folder of yours")
		}
	}
	return nil
}
