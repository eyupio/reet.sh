package terminalhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	// cloneStatusTimeout bounds everything CloneStatus does, and every check
	// RemoveClone makes before it deletes.
	cloneStatusTimeout = 10 * time.Second
	// maxCloneTips bounds the distinct commits checked against origin.
	maxCloneTips = 1000
)

// removeCloneDir deletes a clone's folder; tests replace it.
var removeCloneDir = os.RemoveAll

// cloneCheckArgs go before every git call a clone status or remove makes:
// readOnlyGitArgs, and no LFS filter in either direction (cloneFilterArgs
// and an empty clean), so looking at a clone runs no program its
// configuration names. A file LFS would have cleaned may then read as
// changed, which only keeps the clone.
var cloneCheckArgs = append(append(append([]string{}, readOnlyGitArgs...), cloneFilterArgs...), "-c", "filter.lfs.clean=")

// hardenedCloneGit is git with cloneCheckArgs before every call.
func hardenedCloneGit(git GitRunner) GitRunner {
	return func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return git(ctx, dir, append(append([]string{}, cloneCheckArgs...), args...)...)
	}
}

// errCloneNotRemovable means the clone holds something the four counts do
// not show and that deleting it could lose.
var errCloneNotRemovable = errors.New("the clone holds what cannot be checked")

// cloneAt is a clone's folder once its path has been checked.
type cloneAt struct {
	dest, ownerDir string
	// info is the folder as it was when the path was checked.
	info os.FileInfo
}

// CloneStatus reports what the clone at req.Dir holds that deleting it would
// lose. It changes nothing, in the clone or anywhere else, and never reaches
// the network: what is on GitHub is read from the remote-tracking branches
// the clone already has. It never returns a Go error: every outcome is a
// RepoCloneState.
//
// req.Dir must be exactly <home>/reet/repos/<owner>/<repo> for req.Origin
// ("owner/repo"), with home resolved, and every folder from reet down to it
// a real directory of this user's (no symlink; the three above the clone not
// writable by others): anything else, a missing folder included, answers
// denied, and git is not run.
//
// The folder must be a plain clone of that repository: .git a real directory
// with its own regular config and no commondir or config.worktree
// (ownConfig), and `git config --local --get-all remote.origin.url` exiting
// 0 with exactly one origin that names <owner>/<repo> on github.com. Anything
// else answers exists: something else is there. So does a clone whose
// pushes do not go to that repository: a remote.origin.pushurl of its own,
// or an address `git remote get-url --all --push origin` gives (the person's
// insteadOf and pushInsteadOf rewrites applied) that is not exactly one
// naming <owner>/<repo> on github.com. github.com over ssh is still GitHub.
//
// Marked is true only when the marker CloneRepo writes is there and names
// the repository. Without it the folder is a person's own: nothing is
// counted and the answer is the empty state.
//
// For a marked clone, with every git call pinned to the folder and told not
// to write (GIT_OPTIONAL_LOCKS=0):
//
//   - Worktrees is the entries of `git worktree list --porcelain` after the
//     clone itself, which must come first. One whose folder is gone still
//     counts until git forgets it.
//   - Changed is the entries of `git status --porcelain`, untracked files
//     included. Ignored files are not counted, and are deleted with the
//     clone.
//   - Stashes is the stash entries.
//   - Unpushed is the local branches whose tip is not reachable from any
//     refs/remotes/origin/* ref, plus a detached HEAD on such a commit, plus
//     each ref of a bisect or rebase in progress on one. Nothing is fetched,
//     so a branch pushed from elsewhere, or deleted on GitHub and pruned
//     here, counts. Tags, notes and commits only a reflog holds are not
//     looked at.
//   - Fingerprint is a SHA-256 over the worktree list, the status, every
//     ref with its tip (branches, tags, remote-tracking branches, the
//     stash), HEAD, the stash count and, when anything is changed, the
//     content of every changed path (changeFingerprint).
//
// A count git did not give is never zero: a failed or unreadable git call
// answers failed, a request out of time timed_out. A clone that holds what
// the counts cannot show answers not_removable:
//
//   - a file marked assume-unchanged or skip-worktree (status does not look
//     at it);
//   - a gitlink (mode 160000) in the index, whether or not its folder is
//     there, and any entry named .git below the clone's own, found by a walk
//     that follows no symlink (nestedGit): another repository, in an ignored
//     folder too, may hold history that is nowhere else;
//   - a submodule's repository under .git/modules, or a rebase, a patch
//     series or a merge in progress that may have set changes aside
//     (.git/rebase-merge, .git/rebase-apply, .git/MERGE_AUTOSTASH);
//   - a lock file in .git or under .git/refs: git is writing (gitWriting).
//
// A working tree that is not the folder itself (core.worktree, a bare
// repository) and a folder the walk cannot read answer failed.
//
// Looking runs nothing the clone's or the person's configuration names:
// every git call carries cloneCheckArgs (no fsmonitor hook, which could
// also hide an edit, no untracked cache, no hooks, no external diff, no
// outside attributes file) and switches off every content filter the clone's
// config or the person's names (filterOffArgs), LFS included. If the
// filters cannot be listed the answer is failed.
//
// A clone is also kept, on the safe side, when it is too large to look at:
// the walk for nested repositories has no entry limit and answers timed_out
// when the 10 seconds of a status run out, and a clone whose file listing
// passes 4 MiB (some tens of thousands of tracked files) answers failed.
func CloneStatus(ctx context.Context, home string, req terminalproto.RepoCloneStatus, git GitRunner) terminalproto.RepoCloneState {
	ctx, cancel := context.WithTimeout(ctx, cloneStatusTimeout)
	defer cancel()
	state, _ := cloneState(ctx, home, req.Dir, req.Origin, git)
	return state
}

// RemoveClone deletes the clone at req.Dir only when it is proven to be a
// clone reet made with nothing in it that is not on GitHub. It never returns
// a Go error: every outcome is a RepoCloneRemoved.
//
// It looks at the folder again, exactly as CloneStatus does, immediately
// before deleting, and refuses, leaving everything untouched:
//
//   - with the state's own reason (denied, exists, failed, timed_out,
//     not_removable) when the state has one;
//   - not_removable when the folder is not marked as reet's;
//   - changed when the fingerprint is not req.Fingerprint, the one the
//     person was shown: work that arrived since is never deleted;
//   - not_removable when any count is not zero;
//   - denied when the folder is no longer the one that was looked at.
//
// Then the folder is deleted with everything in it, ignored files included,
// and <home>/reet/repos/<owner> is removed when that left it empty (a plain
// rmdir, which fails harmlessly otherwise). <home>/reet/repos and everything
// above it are never touched. A delete that fails answers failed; it may
// have removed part of the folder. Once started, the delete is not cut
// short by the end of ctx.
func RemoveClone(ctx context.Context, home string, req terminalproto.RepoCloneRemove, git GitRunner) terminalproto.RepoCloneRemoved {
	refuse := func(reason terminalproto.RepoCloneReason) terminalproto.RepoCloneRemoved {
		return terminalproto.RepoCloneRemoved{Reason: reason}
	}
	checkCtx, cancel := context.WithTimeout(ctx, cloneStatusTimeout)
	defer cancel()
	state, at := cloneState(checkCtx, home, req.Dir, req.Origin, git)
	switch {
	case state.Reason != "":
		return refuse(state.Reason)
	case !state.Marked:
		return refuse(terminalproto.RepoCloneNotRemovable)
	case state.Fingerprint == "" || state.Fingerprint != req.Fingerprint:
		return refuse(terminalproto.RepoCloneChanged)
	case !state.Removable():
		return refuse(terminalproto.RepoCloneNotRemovable)
	case checkCtx.Err() != nil:
		return refuse(terminalproto.RepoCloneTimedOut)
	}
	// The folder about to be deleted must still be the one that was checked.
	info, err := os.Lstat(at.dest)
	if err != nil || !info.IsDir() || !os.SameFile(info, at.info) {
		return refuse(terminalproto.RepoCloneDenied)
	}
	if err := removeCloneDir(at.dest); err != nil {
		return refuse(terminalproto.RepoCloneFailed)
	}
	// os.Remove never removes a folder that holds anything.
	_ = os.Remove(at.ownerDir)
	return terminalproto.RepoCloneRemoved{Dir: at.dest}
}

// cloneState is what CloneStatus documents, with the checked folder for a
// remove. The folder is set only when the state has no reason.
func cloneState(ctx context.Context, home, dir, origin string, git GitRunner) (terminalproto.RepoCloneState, cloneAt) {
	refuse := func(reason terminalproto.RepoCloneReason) (terminalproto.RepoCloneState, cloneAt) {
		return terminalproto.RepoCloneState{Reason: reason}, cloneAt{}
	}
	if git == nil {
		return refuse(terminalproto.RepoCloneFailed)
	}
	git = hardenedCloneGit(git)
	owner, repo, ok := strings.Cut(origin, "/")
	if !ok || !terminalproto.ValidGitHubOwner(owner) || !terminalproto.ValidGitHubRepo(repo) {
		return refuse(terminalproto.RepoCloneDenied)
	}
	req := terminalproto.RepoClone{Owner: owner, Repo: repo}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil || !filepath.IsAbs(realHome) {
		return refuse(terminalproto.RepoCloneFailed)
	}
	at, ok := checkClonePath(realHome, dir, req)
	if !ok {
		return refuse(terminalproto.RepoCloneDenied)
	}
	if ctx.Err() != nil {
		return refuse(terminalproto.RepoCloneTimedOut)
	}
	// failed words an error: out of time, something the counts cannot show,
	// or git itself.
	failed := func(err error) terminalproto.RepoCloneReason {
		switch {
		case ctx.Err() != nil:
			return terminalproto.RepoCloneTimedOut
		case errors.Is(err, errCloneNotRemovable):
			return terminalproto.RepoCloneNotRemovable
		}
		return terminalproto.RepoCloneFailed
	}
	if !ownConfig(filepath.Join(at.dest, ".git")) {
		return refuse(terminalproto.RepoCloneExists)
	}
	// No content filter runs while the clone is looked at: every filter the
	// clone's config or the person's names is switched off for the calls
	// that follow, as InspectSession does. cloneCheckArgs already names the
	// LFS filter, so the listing is never empty: a call that fails, lists
	// nothing or may be cut short means the filters are not known.
	keys, err := git(pinGitDir(ctx, at.dest), at.dest, filterKeysArgs...)
	if err != nil || len(keys) == 0 || len(keys) >= gitOutputLimit {
		return refuse(failed(err))
	}
	checked, off := git, filterOffArgs(keys)
	git = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return checked(ctx, dir, append(append([]string{}, off...), args...)...)
	}
	// As adoptClone reads it: the folder's own config only, no rewrite, and
	// exactly one origin. git's exit 1 is never taken as "no origin".
	out, err := git(pinGitDir(ctx, at.dest), at.dest, "config", "--local", "--get-all", "remote.origin.url")
	switch {
	case err != nil && (ctx.Err() != nil || errors.Is(err, errGitMissing)):
		return refuse(failed(err))
	case err != nil || !originNames(out, req):
		return refuse(terminalproto.RepoCloneExists)
	}
	// Pushes must go to the same repository, or origin's remote-tracking
	// branches say nothing about GitHub. A push address of the clone's own
	// is refused whatever it names. Then the address git would really push
	// to, with the person's own insteadOf and pushInsteadOf rewrites applied,
	// must be exactly one and name the repository on github.com (over https
	// or ssh). git's exit 1 for "no pushurl" is not relied on: the second
	// call must succeed and prove the address.
	_, err = git(pinGitDir(ctx, at.dest), at.dest, "config", "--local", "--get-all", "remote.origin.pushurl")
	switch {
	case err == nil:
		return refuse(terminalproto.RepoCloneExists)
	case ctx.Err() != nil || errors.Is(err, errGitMissing):
		return refuse(failed(err))
	}
	out, err = git(pinGitDir(ctx, at.dest), at.dest, "remote", "get-url", "--all", "--push", "origin")
	switch {
	case err != nil && (ctx.Err() != nil || errors.Is(err, errGitMissing)):
		return refuse(failed(err))
	case err != nil || !originNames(out, req):
		return refuse(terminalproto.RepoCloneExists)
	}
	if marker, marked := cloneMarked(at.dest); !marked || !sameRepo(marker, req) {
		return terminalproto.RepoCloneState{}, at
	}
	state, err := cloneCounts(ctx, at.dest, git)
	if err != nil || ctx.Err() != nil {
		return refuse(failed(err))
	}
	return state, at
}

// checkClonePath reports whether dir is exactly <realHome>/reet/repos/<owner>/<repo>
// for req and every folder from reet down to it is a real directory of this
// user's, the three above the clone not writable by group or others. It
// makes nothing and follows no symlink.
func checkClonePath(realHome, dir string, req terminalproto.RepoClone) (cloneAt, bool) {
	ownerDir := filepath.Join(realHome, "reet", "repos", req.Owner)
	dest := filepath.Join(ownerDir, req.Repo)
	if dir != dest {
		return cloneAt{}, false
	}
	for _, path := range []string{filepath.Join(realHome, "reet"), filepath.Join(realHome, "reet", "repos"), ownerDir} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || ownedByCurrentUser(info) != nil || info.Mode().Perm()&0o022 != 0 {
			return cloneAt{}, false
		}
	}
	info, err := os.Lstat(dest)
	if err != nil || !info.IsDir() || ownedByCurrentUser(info) != nil {
		return cloneAt{}, false
	}
	return cloneAt{dest: dest, ownerDir: ownerDir, info: info}, true
}

// cloneCounts counts what the marked clone at dest holds (see CloneStatus).
// Any error means the counts are not known.
func cloneCounts(ctx context.Context, dest string, git GitRunner) (terminalproto.RepoCloneState, error) {
	none := terminalproto.RepoCloneState{}
	unreadable := errors.New("git answered unreadably")
	// GIT_OPTIONAL_LOCKS=0: status must not rewrite the index it reads.
	ctx = withGitEnv(pinGitDir(ctx, dest), "GIT_OPTIONAL_LOCKS=0")
	run := func(args ...string) ([]byte, error) {
		out, err := git(ctx, dest, args...)
		if err != nil {
			return nil, gitFailed(out, err)
		}
		return out, nil
	}

	// The working tree git looks at must be this folder: core.worktree could
	// point status somewhere else, and a bare repository has none.
	top, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		return none, err
	}
	if strings.TrimSuffix(string(top), "\n") != dest {
		return none, unreadable
	}

	worktrees, err := run("worktree", "list", "--porcelain")
	if err != nil {
		return none, err
	}
	linked, ok := linkedWorktrees(worktrees, dest)
	if !ok {
		return none, unreadable
	}

	status, err := run("status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return none, err
	}
	changed, _ := parseStatus(status)

	refs, err := run("for-each-ref", "--format=%(objectname) %(objecttype) %(refname)")
	if err != nil {
		return none, err
	}
	if len(refs) >= gitListLimit {
		return none, unreadable
	}
	// tips are the commits deleting the clone would lose unless origin has
	// them, one entry for each ref that holds one.
	var tips []string
	stashed := false
	for _, line := range strings.Split(strings.TrimSuffix(string(refs), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, " ", 3)
		if len(fields) != 3 || !validHeadLine("HEAD "+fields[0]) || fields[2] == "" {
			return none, unreadable
		}
		name := fields[2]
		switch {
		case name == "refs/stash":
			stashed = true
		case strings.HasPrefix(name, "refs/heads/"), strings.HasPrefix(name, "refs/bisect/"),
			strings.HasPrefix(name, "refs/rewritten/"), strings.HasPrefix(name, "refs/worktree/"):
			tips = append(tips, fields[0])
		}
	}

	// HEAD on a branch is covered by that branch. A failed symbolic-ref means
	// a detached HEAD only when git itself ran, and then HEAD must name a
	// commit: it is never taken as nothing.
	head, err := git(ctx, dest, "symbolic-ref", "-q", "HEAD")
	headLine := strings.TrimSuffix(string(head), "\n")
	switch {
	case err == nil && !strings.HasPrefix(headLine, "refs/heads/"):
		return none, unreadable
	case err != nil && (ctx.Err() != nil || errors.Is(err, errGitMissing)):
		return none, gitFailed(head, err)
	case err != nil:
		out, err := run("rev-parse", "--verify", "HEAD^{commit}")
		if err != nil {
			return none, err
		}
		oid := strings.TrimSuffix(string(out), "\n")
		if !validHeadLine("HEAD " + oid) {
			return none, unreadable
		}
		tips = append(tips, oid)
		headLine = "detached " + oid
	}

	unpushed, err := unpushedTips(ctx, dest, git, tips)
	if err != nil {
		return none, err
	}

	stashes := 0
	if stashed {
		if stashes, err = gitCount(ctx, git, dest, "rev-list", "--walk-reflogs", "--count", "refs/stash"); err != nil {
			return none, err
		}
		// The ref is there, so at least one stash is.
		stashes = max(stashes, 1)
	}

	// What status does not look at, and what the refs above do not hold.
	hidden, err := run("ls-files", "-v", "-z")
	if err != nil {
		return none, err
	}
	if len(hidden) >= gitListLimit {
		return none, unreadable
	}
	if hiddenEntries(hidden, dest) > 0 {
		return none, errCloneNotRemovable
	}
	// A gitlink is another repository's commit: the repository it came from
	// may live only inside this folder. This is the last git call; what
	// follows reads the disk.
	staged, err := run("ls-files", "--stage", "-z")
	if err != nil {
		return none, err
	}
	if len(staged) >= gitListLimit {
		return none, unreadable
	}
	for _, entry := range strings.Split(string(staged), "\x00") {
		if strings.HasPrefix(entry, "160000 ") {
			return none, errCloneNotRemovable
		}
	}
	if writing, err := gitWriting(filepath.Join(dest, ".git")); err != nil {
		return none, err
	} else if writing {
		return none, errCloneNotRemovable
	}
	if nested, err := nestedGit(ctx, dest); err != nil {
		return none, err
	} else if nested {
		return none, errCloneNotRemovable
	}
	// A submodule's repository, and what a rebase, a patch series or a merge
	// in progress set aside (its autostash): work kept in .git itself.
	for _, name := range []string{"modules", "rebase-merge", "rebase-apply", "MERGE_AUTOSTASH"} {
		if _, err := os.Lstat(filepath.Join(dest, ".git", name)); !os.IsNotExist(err) {
			return none, errCloneNotRemovable
		}
	}

	h := sha256.New()
	for _, part := range [][]byte{[]byte("reet-clone-state-v1"), worktrees, status, refs, []byte(headLine), []byte(fmt.Sprint(stashes))} {
		fmt.Fprintf(h, "%d:", len(part))
		h.Write(part)
	}
	if changed > 0 {
		content, err := changeFingerprint(ctx, worktreeAt{path: dest}, git, status)
		if err != nil {
			return none, err
		}
		h.Write([]byte(content))
	}
	return terminalproto.RepoCloneState{
		Marked:      true,
		Worktrees:   linked,
		Changed:     changed,
		Stashes:     stashes,
		Unpushed:    unpushed,
		Fingerprint: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

// linkedWorktrees counts the worktrees `git worktree list --porcelain` lists
// after the clone itself, which must be the first one. ok is false for a
// listing that may be cut short or does not start with the clone.
func linkedWorktrees(out []byte, dest string) (linked int, ok bool) {
	if len(out) >= gitListLimit {
		return 0, false
	}
	lines := strings.Split(string(out), "\n")
	if lines[0] != "worktree "+dest {
		return 0, false
	}
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "worktree ") {
			linked++
		}
	}
	return linked, true
}

// unpushedTips counts the tips with a commit that no refs/remotes/origin/*
// ref reaches. Nothing is fetched. One call answers for a clone where every
// tip is on origin; otherwise each distinct commit is asked about once.
func unpushedTips(ctx context.Context, dest string, git GitRunner, tips []string) (int, error) {
	if len(tips) == 0 {
		return 0, nil
	}
	missing := map[string]bool{}
	var distinct []string
	for _, tip := range tips {
		if _, seen := missing[tip]; !seen {
			missing[tip] = false
			distinct = append(distinct, tip)
		}
	}
	if len(distinct) > maxCloneTips {
		return 0, errors.New("too many branches to check")
	}
	notOnOrigin := func(oids ...string) (bool, error) {
		args := append([]string{"rev-list", "--count", "--max-count=1"}, oids...)
		n, err := gitCount(ctx, git, dest, append(args, "--not", "--remotes=origin")...)
		return n > 0, err
	}
	some, err := notOnOrigin(distinct...)
	if err != nil || !some {
		return 0, err
	}
	for _, oid := range distinct {
		if missing[oid], err = notOnOrigin(oid); err != nil {
			return 0, err
		}
	}
	count := 0
	for _, tip := range tips {
		if missing[tip] {
			count++
		}
	}
	return count, nil
}

// walkCloneDir walks a clone without following symlinks; tests replace it.
var walkCloneDir = filepath.WalkDir

// gitWriting reports a lock file directly in gitDir (index.lock, HEAD.lock,
// packed-refs.lock, shallow.lock, config.lock) or anywhere under its refs:
// git is writing there, or was cut off while it did.
func gitWriting(gitDir string) (bool, error) {
	list, err := os.ReadDir(gitDir)
	if err != nil {
		return false, err
	}
	for _, entry := range list {
		if strings.HasSuffix(entry.Name(), ".lock") {
			return true, nil
		}
	}
	writing := false
	err = walkCloneDir(filepath.Join(gitDir, "refs"), func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(d.Name(), ".lock") {
			writing = true
			return fs.SkipAll
		}
		return nil
	})
	return writing, err
}

// nestedGit reports an entry named .git (in any letter case; a folder, a
// file or a symlink) anywhere under dest other than dest's own .git folder:
// another repository or worktree, ignored or not, whose history may be
// nowhere else. Symlinks are not followed. A folder that cannot be read, or
// the end of ctx, is an error.
func nestedGit(ctx context.Context, dest string) (bool, error) {
	own := filepath.Join(dest, ".git")
	nested := false
	err := walkCloneDir(dest, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case ctx.Err() != nil:
			return ctx.Err()
		case path == own:
			return fs.SkipDir
		case strings.EqualFold(d.Name(), ".git") && path != dest:
			nested = true
			return fs.SkipAll
		}
		return nil
	})
	return nested, err
}
