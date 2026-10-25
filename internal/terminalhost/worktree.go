package terminalhost

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ErrWorktreeRefused means a run's worktree could not be made safely. The
// error is a *WorktreeError carrying the reason; the connector answers it as
// terminalproto.WorktreeRefused with that reason (see WorktreeResult).
var ErrWorktreeRefused = errors.New("the worktree cannot be made")

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
)

var errGitMissing = errors.New("git is not installed")

// ExecGit runs git found by exec.LookPath as an argument vector, never through
// a shell: `git -C dir args...`, stdin empty, environment exactly HOME, PATH
// and LANG from this process (every GIT_* and every other variable removed),
// a 60-second timeout and stdout and stderr together capped at 4 KiB.
func ExecGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	path, err := exec.LookPath("git")
	if err != nil || !filepath.IsAbs(path) {
		return nil, errGitMissing
	}
	return runGit(ctx, path, gitTimeout, gitOutputLimit, dir, args...)
}

// newExecGit is ExecGit with another per-call timeout, for tests.
func newExecGit(timeout time.Duration) GitRunner {
	return func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		path, err := exec.LookPath("git")
		if err != nil || !filepath.IsAbs(path) {
			return nil, errGitMissing
		}
		return runGit(ctx, path, timeout, gitOutputLimit, dir, args...)
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
