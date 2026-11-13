package terminalhost

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	// repoCloneTimeout bounds one clone request on the machine: the sign-in
	// check, the clone itself and every other git call it makes.
	repoCloneTimeout = 10 * time.Minute
	// ghTimeout bounds one gh call.
	ghTimeout = 15 * time.Second
	// cloneMarker is the file reet writes into <clone>/.git when it made the
	// clone itself; its content is "owner/repo\n".
	cloneMarker = "reet-clone"
	// leftoverAge is how long a temporary clone folder is left alone: longer
	// than any clone runs (repoCloneTimeout), so it is never one in progress.
	leftoverAge = 15 * time.Minute
)

var errGHMissing = errors.New("gh is not installed")

// removeLeftover removes a leftover temporary folder; tests replace it.
var removeLeftover = os.RemoveAll

// leftoverGit reads a leftover candidate's origin: the ordinary per-call git
// runner, not the clone's, so a slow candidate cannot hold the clone up for
// long. Tests replace it.
var leftoverGit GitRunner = ExecGit

// GHRunner runs `gh args...` and returns its combined output. The real one
// is ExecGH; tests fake it.
type GHRunner func(ctx context.Context, args ...string) ([]byte, error)

// ExecGH runs gh found by exec.LookPath, only at an absolute path, as an
// argument vector, never through a shell: stdin empty, environment exactly
// HOME, PATH and LANG from this process (as git gets), a 15-second timeout
// and stdout and stderr together capped at 4 KiB.
func ExecGH(ctx context.Context, args ...string) ([]byte, error) {
	path, err := exec.LookPath("gh")
	if err != nil || !filepath.IsAbs(path) {
		return nil, errGHMissing
	}
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = gitEnvironment(os.Environ())
	out := &truncatingBuffer{limit: gitOutputLimit}
	cmd.Stdout, cmd.Stderr = out, out
	killGroupOnCancel(cmd)
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	if ctx.Err() != nil && err != nil {
		return out.Bytes(), fmt.Errorf("gh timed out: %w", ctx.Err())
	}
	return out.Bytes(), err
}

// ExecCloneGit is ExecGit with the clone's own bound in place of the usual
// per-call one: a full clone of a large repository takes minutes.
func ExecCloneGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return newExecGit(repoCloneTimeout)(ctx, dir, args...)
}

// CloneRepo clones github.com/<owner>/<repo> into <home>/reet/repos/<owner>/<repo>,
// with the machine's own gh sign-in, and answers where it is. It never
// returns a Go error: every outcome is a RepoCloneResult.
//
// The gh sign-in must answer for github.com first; otherwise nothing is
// made. Every folder from home down to the owner folder must be a real
// directory of this user's (new ones are made 0700), and no other spelling
// of the owner or the repository, differing only in case, may be there.
//
// A destination that is already there is adopted, untouched, only when it
// is a clone of the same repository (.git a real directory, origin naming
// it on github.com); anything else answers exists. Otherwise the clone is
// made in a temporary folder next to it, the marker written into it, and the
// folder renamed into place without replacing anything. A clone that fails
// removes its temporary folder, and the owner folder when this call made it
// and it is empty. Before cloning, the temporary folders earlier clones of
// the same repository left behind when cut off are removed, under the exact
// conditions removeLeftovers states; nothing else is ever removed.
//
// The clone is a full, normal one: its remote is named origin whatever the
// person's clone.defaultRemoteName says, and tags are fetched as in any
// clone. A git setting that rewrites github.com to ssh answers ssh_rewrite.
//
// The clone reaches github.com only and runs no LFS filter (cloneFilterArgs).
// Any other smudge filter the person's own git configuration names still runs
// during the clone's checkout, on the paths the repository's .gitattributes
// select, as it would for any `git clone` that person ran.
func CloneRepo(ctx context.Context, home string, req terminalproto.RepoClone, git GitRunner, gh GHRunner) terminalproto.RepoCloneResult {
	refuse := func(reason terminalproto.RepoCloneReason) terminalproto.RepoCloneResult {
		return terminalproto.RepoCloneResult{Reason: reason}
	}
	if !terminalproto.ValidGitHubOwner(req.Owner) || !terminalproto.ValidGitHubRepo(req.Repo) || git == nil || gh == nil {
		return refuse(terminalproto.RepoCloneFailed)
	}
	ctx, cancel := context.WithTimeout(ctx, repoCloneTimeout)
	defer cancel()

	if _, err := gh(ctx, "auth", "status", "--hostname", "github.com"); err != nil {
		switch {
		case errors.Is(err, errGHMissing):
			return refuse(terminalproto.RepoCloneFailed)
		case ctx.Err() != nil:
			return refuse(terminalproto.RepoCloneTimedOut)
		}
		return refuse(terminalproto.RepoCloneNotSignedIn)
	}

	realHome, err := filepath.EvalSymlinks(home)
	if err != nil || !filepath.IsAbs(realHome) {
		return refuse(terminalproto.RepoCloneFailed)
	}
	repos := filepath.Join(realHome, "reet", "repos")
	if ensureWorktreeDirs(realHome, repos) != nil || hasCaseTwin(repos, req.Owner) {
		return refuse(terminalproto.RepoCloneFailed)
	}
	ownerDir := filepath.Join(repos, req.Owner)
	_, err = os.Lstat(ownerDir)
	madeOwner := os.IsNotExist(err)
	// removeOwner takes away the owner folder this call made, once it is
	// empty again; os.Remove never removes a folder that holds anything.
	removeOwner := func() {
		if madeOwner {
			_ = os.Remove(ownerDir)
		}
	}
	if ensureWorktreeDirs(realHome, ownerDir) != nil {
		return refuse(terminalproto.RepoCloneFailed)
	}
	if hasCaseTwin(ownerDir, req.Repo) {
		removeOwner()
		return refuse(terminalproto.RepoCloneFailed)
	}
	dest := filepath.Join(ownerDir, req.Repo)
	if _, err := os.Lstat(dest); err == nil {
		return adoptClone(ctx, dest, req, git)
	} else if !os.IsNotExist(err) {
		return refuse(terminalproto.RepoCloneFailed)
	}

	removeLeftovers(ctx, ownerDir, req)

	// The temporary folder is made here, exclusively, so it is this call's
	// and nobody else's; git clones into an empty folder that exists.
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		removeOwner()
		return refuse(terminalproto.RepoCloneFailed)
	}
	temp := filepath.Join(ownerDir, "."+req.Repo+".cloning-"+hex.EncodeToString(suffix[:]))
	if err := os.Mkdir(temp, 0o700); err != nil {
		removeOwner()
		return refuse(terminalproto.RepoCloneFailed)
	}
	// Removing the temporary folder needs no context, so it runs after the
	// request's time is up as well.
	discard := func() {
		_ = os.RemoveAll(temp)
		removeOwner()
	}

	// LANG=C so the messages noAccess matches are git's own English ones;
	// LC_ALL and LC_MESSAGES never reach git (gitEnvironment).
	// GIT_LFS_SKIP_SMUDGE and the empty filter.lfs settings keep git-lfs
	// from running on this checkout at all (see cloneFilterArgs).
	cloneCtx := withGitEnv(ctx, append(networkGitEnv(), "LANG=C", "GIT_LFS_SKIP_SMUDGE=1")...)
	args := append(networkGitArgs(), cloneFilterArgs...)
	args = append(args, "clone", "--origin", "origin", "https://github.com/"+req.Owner+"/"+req.Repo+".git", temp)
	out, err := git(cloneCtx, ownerDir, args...)
	if err != nil || ctx.Err() != nil {
		discard()
		switch {
		case ctx.Err() != nil:
			return refuse(terminalproto.RepoCloneTimedOut)
		case errors.Is(err, errGitMissing):
			return refuse(terminalproto.RepoCloneFailed)
		case sshRefused(out):
			return refuse(terminalproto.RepoCloneSSHRewrite)
		case noAccess(out):
			return refuse(terminalproto.RepoCloneNoAccess)
		}
		return refuse(terminalproto.RepoCloneFailed)
	}
	if err := writeCloneMarker(temp, req); err != nil {
		discard()
		return refuse(terminalproto.RepoCloneFailed)
	}
	if err := renameNoReplace(temp, dest); err != nil {
		_ = os.RemoveAll(temp)
		if _, statErr := os.Lstat(dest); statErr == nil {
			// Somebody made the destination while the clone ran.
			return adoptClone(ctx, dest, req, git)
		}
		removeOwner()
		return refuse(terminalproto.RepoCloneFailed)
	}
	return terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: cloneDefaultBranch(ctx, dest, git), Marked: true}
}

// cloneFilterArgs turn off the LFS filter for the clone's own checkout,
// whatever the person's git configuration installs (`git lfs install`):
// git-lfs follows the repository's committed .lfsconfig to any host, over
// ssh too, and asks the machine's credential helpers for it, so a tree nobody
// has looked at yet must not start it. LFS-tracked files stay pointer files
// in this checkout, which is not worked in; each run's worktree is checked out
// later under the person's own configuration. They are for the clone only:
// a fetch checks nothing out.
var cloneFilterArgs = []string{"-c", "filter.lfs.process=", "-c", "filter.lfs.smudge=", "-c", "filter.lfs.required=false"}

// noAccess reports git's own words for a repository that is missing or not
// visible to the sign-in: GitHub answers both the same way.
func noAccess(out []byte) bool {
	return bytes.Contains(out, []byte("Repository not found")) || bytes.Contains(out, []byte("could not read Username"))
}

// sshRefused reports git's own words for refusing the ssh transport: the
// person's git settings rewrote the github.com URL to ssh (url.<base>.insteadOf),
// which the clone does not allow.
func sshRefused(out []byte) bool {
	return bytes.Contains(out, []byte("transport 'ssh' not allowed"))
}

// removeLeftovers removes the temporary folders earlier clones of req's
// repository left in ownerDir when they were cut off (the connector stopped,
// the machine lost power). A folder is a leftover only when all six hold:
//
//  1. it is directly in ownerDir;
//  2. its name is exactly ".<repo>.cloning-" and 16 lower-case hex
//     characters;
//  3. it is a real directory, not a symlink, owned by this user;
//  4. it last changed more than leftoverAge ago;
//  5. it is not a clone reet made under that name: its marker does not name
//     <owner>/<its own name>;
//  6. it is not a clone of the repository named like it: when it has a .git,
//     its configured remote.origin.url (read as adoptClone reads it, pinned
//     to the folder, no rewrites, with the ordinary per-call bound) is read
//     and does not name github.com/<owner>/<its own name>, ignoring case. A
//     .git whose origin cannot be read (git fails, times out, the config is
//     unreadable or has no origin) keeps the folder.
//
// A real leftover of <owner>/<repo> names <owner>/<repo> as its origin, or
// has no .git yet. Only that folder is removed, and a failure to remove it is
// ignored.
func removeLeftovers(ctx context.Context, ownerDir string, req terminalproto.RepoClone) {
	list, err := os.ReadDir(ownerDir)
	if err != nil {
		return
	}
	prefix := "." + req.Repo + ".cloning-"
	for _, entry := range list {
		name := entry.Name()
		suffix, ok := strings.CutPrefix(name, prefix)
		if !ok || !lowerHex16(suffix) {
			continue
		}
		path := filepath.Join(ownerDir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || ownedByCurrentUser(info) != nil {
			continue
		}
		if time.Since(info.ModTime()) <= leftoverAge {
			continue
		}
		if origin, marked := cloneMarked(path); marked && strings.EqualFold(origin, req.Owner+"/"+name) {
			continue
		}
		if clonedAs(ctx, path, req.Owner+"/"+name) {
			continue
		}
		_ = removeLeftover(path)
	}
}

// clonedAs reports whether the folder at path may be a clone of repo
// ("owner/name"): it has a .git and its configured origin names repo on
// github.com, or its origin cannot be read. A folder without .git is not.
func clonedAs(ctx context.Context, path, repo string) bool {
	if _, err := os.Lstat(filepath.Join(path, ".git")); os.IsNotExist(err) {
		return false
	} else if err != nil {
		return true
	}
	// `git config --get` applies no url.<base>.insteadOf rewrite: this is the
	// origin as configured.
	out, err := leftoverGit(pinGitDir(ctx, path), path, "config", "--get", "remote.origin.url")
	if err != nil {
		return true
	}
	owner, name, ok := GitHubRemote(strings.TrimSuffix(string(out), "\n"))
	return ok && strings.EqualFold(owner+"/"+name, repo)
}

// lowerHex16 reports whether s is exactly 16 lower-case hex characters, the
// suffix CloneRepo gives its temporary folder.
func lowerHex16(s string) bool {
	if len(s) != 16 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// hasCaseTwin reports whether dir holds an entry whose name equals name
// ignoring case but is not name itself, or cannot be read: two spellings
// must never share or shadow one folder on a file system that ignores case.
func hasCaseTwin(dir, name string) bool {
	list, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, entry := range list {
		if entry.Name() != name && strings.EqualFold(entry.Name(), name) {
			return true
		}
	}
	return false
}

// adoptClone answers for a destination that is already there: adopted when
// it is a clone of the same repository on github.com, exists otherwise. It
// changes nothing in the folder, and never writes a marker into it.
func adoptClone(ctx context.Context, dest string, req terminalproto.RepoClone, git GitRunner) terminalproto.RepoCloneResult {
	exists := terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneExists}
	if !realDir(dest) || !realDir(filepath.Join(dest, ".git")) {
		return exists
	}
	// `git config --get` applies no url.<base>.insteadOf rewrite: this is
	// the origin as configured.
	out, err := git(pinGitDir(ctx, dest), dest, "config", "--get", "remote.origin.url")
	if err != nil {
		return exists
	}
	owner, repo, ok := GitHubRemote(strings.TrimSuffix(string(out), "\n"))
	if !ok || !sameRepo(owner+"/"+repo, req) {
		return exists
	}
	origin, marked := cloneMarked(dest)
	return terminalproto.RepoCloneResult{
		Dir:           dest,
		DefaultBranch: cloneDefaultBranch(ctx, dest, git),
		Adopted:       true,
		Marked:        marked && sameRepo(origin, req),
	}
}

// sameRepo reports whether origin ("owner/repo") names req's repository;
// GitHub names ignore case.
func sameRepo(origin string, req terminalproto.RepoClone) bool {
	return strings.EqualFold(origin, req.Owner+"/"+req.Repo)
}

// realDir reports whether path is a directory itself, not a symlink to one.
func realDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

// writeCloneMarker writes the marker into the new clone at dir; it refuses
// a .git that is not a real directory and a marker that is already there.
func writeCloneMarker(dir string, req terminalproto.RepoClone) error {
	gitDir := filepath.Join(dir, ".git")
	if !realDir(gitDir) {
		return errors.New("the clone has no .git directory")
	}
	f, err := os.OpenFile(filepath.Join(gitDir, cloneMarker), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(req.Owner + "/" + req.Repo + "\n")
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// cloneMarked answers the "owner/repo" the reet marker in dir names, only
// when .git is a real directory, the marker a regular file (not a symlink)
// and its content one valid "owner/repo" line.
func cloneMarked(dir string) (origin string, ok bool) {
	gitDir := filepath.Join(dir, ".git")
	if !realDir(gitDir) {
		return "", false
	}
	path := filepath.Join(gitDir, cloneMarker)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	// The file opened must be the one looked at, not a symlink swapped in.
	if opened, err := f.Stat(); err != nil || !os.SameFile(info, opened) {
		return "", false
	}
	data, err := io.ReadAll(io.LimitReader(f, 256))
	if err != nil {
		return "", false
	}
	line, found := strings.CutSuffix(string(data), "\n")
	owner, repo, slash := strings.Cut(line, "/")
	if !found || !slash || !terminalproto.ValidGitHubOwner(owner) || !terminalproto.ValidGitHubRepo(repo) {
		return "", false
	}
	return line, true
}

// cloneDefaultBranch is the repository's default branch (syncDefaultBase),
// else the branch its working tree has checked out, else "". A clone whose
// HEAD has no commit (an empty repository) has none: its HEAD names a branch
// that does not exist yet.
func cloneDefaultBranch(ctx context.Context, dir string, git GitRunner) string {
	ctx = pinGitDir(ctx, dir)
	if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err != nil {
		return ""
	}
	if name := syncDefaultBase(ctx, dir, git); name != "" {
		return name
	}
	out, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if name := strings.TrimSuffix(string(out), "\n"); err == nil && terminalproto.ValidRefName(name) {
		return name
	}
	return ""
}

// renameIfAbsent renames from to to unless to is there, for systems
// without an atomic rename that refuses to replace: the check and the rename
// are two steps, so a folder made between them could still be replaced if
// it is empty.
func renameIfAbsent(from, to string) error {
	if _, err := os.Lstat(to); !os.IsNotExist(err) {
		return os.ErrExist
	}
	return os.Rename(from, to)
}
