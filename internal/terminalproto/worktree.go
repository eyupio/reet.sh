package terminalproto

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const WorktreeFeature = "worktree-v1"
const MaxWorktreePaths = 20
const MaxWorktreePathBytes = 255
const maxWorktreeCount = 1 << 30

// MaxSlugBytes bounds a Project or repository slug.
const MaxSlugBytes = 40

// MaxWorktreeDetailBytes bounds the detail of a refused worktree.
const MaxWorktreeDetailBytes = 200

// WorktreeReason says why a worktree was refused (Result.Reason).
type WorktreeReason string

const (
	// WorktreeNotRepoRoot: the folder is not the top of a normal git working tree.
	WorktreeNotRepoRoot WorktreeReason = "not_repo_root"
	// WorktreeRepoBusy: a merge, rebase, cherry-pick, revert or bisect is in progress.
	WorktreeRepoBusy WorktreeReason = "repo_busy"
	// WorktreeExists: the destination or branch exists and is not this run's.
	WorktreeExists WorktreeReason = "exists"
	// WorktreeGitFailed: git could not make the worktree (Detail says why).
	WorktreeGitFailed WorktreeReason = "git_failed"
	// WorktreeOutsideHome: reet's worktree folder is not a real directory of
	// this user inside home (a symlink, another owner, a file).
	WorktreeOutsideHome WorktreeReason = "outside_home"
	// WorktreeNotRegistered: something is at the run's worktree path but git
	// does not list it as a worktree of its repository (status and remove).
	WorktreeNotRegistered WorktreeReason = "not_registered"
	// WorktreeHasChanges: the worktree has uncommitted changes (modified,
	// staged or untracked files), so it is never removed (remove only).
	WorktreeHasChanges WorktreeReason = "has_changes"
	// WorktreeUnreferencedCommits: HEAD has commits that no branch, tag or
	// remote-tracking branch holds (a detached HEAD); removing the worktree
	// would leave them unreachable (remove only).
	WorktreeUnreferencedCommits WorktreeReason = "unreferenced_commits"
	// WorktreeNestedRepository: another repository or worktree (a .git entry)
	// lies inside the worktree, perhaps ignored, and would be deleted with it.
	WorktreeNestedRepository WorktreeReason = "nested_repository"
	// WorktreeHiddenChanges: files are marked skip-worktree or
	// assume-unchanged (or are outside a sparse checkout), so git does not
	// check them for edits.
	WorktreeHiddenChanges WorktreeReason = "hidden_changes"
	// WorktreeCheckIncomplete: reet could not check the whole worktree (too
	// many files, too deep, an unreadable folder, or too much for git to list).
	WorktreeCheckIncomplete WorktreeReason = "check_incomplete"
)

func (r WorktreeReason) Valid() bool {
	switch r {
	case WorktreeNotRepoRoot, WorktreeRepoBusy, WorktreeExists, WorktreeGitFailed, WorktreeOutsideHome, WorktreeNotRegistered, WorktreeHasChanges,
		WorktreeUnreferencedCommits, WorktreeNestedRepository, WorktreeHiddenChanges, WorktreeCheckIncomplete:
		return true
	}
	return false
}

func validWorktreeDetail(s string) bool {
	return len(s) <= MaxWorktreeDetailBytes && plainText(s, MaxWorktreeDetailBytes)
}

// validWorktree: a worktree needs a tool, a folder (the repository root), a run token and a Project slug; a
// Project slug is meaningless without a worktree.
func (c Create) validWorktree() bool {
	if c.Worktree == "" {
		return c.Project == ""
	}
	return c.Tool != "" && c.Dir != "" && ValidRunToken(c.Worktree) && ValidSlug(c.Project)
}

// ValidSlug accepts 1 to MaxSlugBytes of a-z and 0-9 in hyphen-separated
// words: no dot, separator, leading, trailing or doubled hyphen.
func ValidSlug(s string) bool {
	if s == "" || len(s) > MaxSlugBytes || s[0] == '-' || s[len(s)-1] == '-' || strings.Contains(s, "--") {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-') {
			return false
		}
	}
	return true
}

// Slug makes a folder- and branch-safe name: ASCII letters are lower-cased,
// digits kept, every other character (dots, separators, spaces, non-ASCII)
// becomes one hyphen, and hyphens are trimmed. A name longer than
// MaxSlugBytes keeps its first part and gains 8 hex digits of its SHA-256; a
// name with nothing usable becomes fallback plus those 8 digits (or fallback
// alone when name is empty). The result always satisfies ValidSlug when
// fallback does.
func Slug(name, fallback string) string {
	var b strings.Builder
	dash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 'a' - 'A')
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = true
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if name == "" {
		return fallback
	}
	sum := sha256.Sum256([]byte(name))
	tag := hex.EncodeToString(sum[:4])
	if s == "" {
		return fallback + "-" + tag
	}
	if len(s) > MaxSlugBytes {
		s = strings.TrimRight(s[:MaxSlugBytes-len(tag)-1], "-") + "-" + tag
	}
	return s
}

// Worktree outcomes beyond the generic result codes.
const (
	// WorktreeChanged means the counts shown to the person no longer match.
	WorktreeChanged ResultCode = "worktree_changed"
	// WorktreeRefused means the worktree could not be created or removed safely.
	WorktreeRefused ResultCode = "worktree_refused"
)

// SupportsWorktree reports whether the peer advertised worktree-v1 in
// X-Reet-Terminal-Features. Worktree fields are never sent without it.
func SupportsWorktree(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == WorktreeFeature {
			return true
		}
	}
	return false
}

// ValidRunToken accepts a lower-case hex run token of 16 to 48 characters.
func ValidRunToken(s string) bool {
	if len(s) < 16 || len(s) > 48 {
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

// validRepoName accepts a single directory name: 1-100 of letters, digits,
// '.', '_' and '-', never "." or ".." and never a leading '-' (so git cannot
// read it as an option).
func validRepoName(s string) bool {
	if s == "" || len(s) > 100 || s == "." || s == ".." || s[0] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '.' || b == '_' || b == '-') {
			return false
		}
	}
	return true
}

func validWorktreeCount(n int) bool { return n >= 0 && n <= maxWorktreeCount }

type WorktreeStatus struct {
	Run     string `json:"run"`
	Project string `json:"project"`
	Repo    string `json:"repo"`
}

func (WorktreeStatus) kind() string { return "worktree_status" }
func (w WorktreeStatus) valid() bool {
	return ValidRunToken(w.Run) && ValidSlug(w.Project) && validRepoName(w.Repo)
}

// WorktreeRemove carries the counts the person was shown so that changes made
// after they looked cannot be thrown away.
type WorktreeRemove struct {
	Run      string `json:"run"`
	Project  string `json:"project"`
	Repo     string `json:"repo"`
	Changed  int    `json:"changed"`
	Unpushed int    `json:"unpushed"`
}

func (WorktreeRemove) kind() string { return "worktree_remove" }
func (w WorktreeRemove) valid() bool {
	return ValidRunToken(w.Run) && ValidSlug(w.Project) && validRepoName(w.Repo) && validWorktreeCount(w.Changed) && validWorktreeCount(w.Unpushed)
}

// WorktreeState is a worktree's status. Unreferenced, Hidden, Nested and
// Incomplete each block a remove whatever counts the person was shown; they
// are omitted when zero, so a clean state encodes as before.
type WorktreeState struct {
	Path     string   `json:"path"`
	Branch   string   `json:"branch"`
	Changed  int      `json:"changed"`
	Unpushed int      `json:"unpushed"`
	Paths    []string `json:"paths,omitempty"`
	// Unreferenced counts commits on HEAD that no branch, tag or remote holds.
	Unreferenced int `json:"unreferenced,omitempty"`
	// Hidden counts files git is told not to check (skip-worktree,
	// assume-unchanged, outside a sparse checkout).
	Hidden int `json:"hidden,omitempty"`
	// Nested is true when another repository or worktree lies inside.
	Nested bool `json:"nested,omitempty"`
	// Incomplete is true when the worktree could not be checked completely.
	Incomplete bool `json:"incomplete,omitempty"`
}

func (WorktreeState) kind() string { return "worktree_state" }
func (w WorktreeState) valid() bool {
	if !validDir(w.Path) || !plainText(w.Branch, 255) || w.Branch == "" || !validWorktreeCount(w.Changed) || !validWorktreeCount(w.Unpushed) || !validWorktreeCount(w.Unreferenced) || !validWorktreeCount(w.Hidden) || len(w.Paths) > MaxWorktreePaths {
		return false
	}
	for _, p := range w.Paths {
		if p == "" || len(p) > MaxWorktreePathBytes || !plainText(p, MaxWorktreePathBytes) {
			return false
		}
	}
	return true
}

// WorktreeResult answers a remove (ok, worktree_changed, worktree_refused,
// not_found, ...) or refuses a status. Reason and Detail explain a
// worktree_refused answer exactly as on Result, and are never set otherwise.
type WorktreeResult struct {
	Code   ResultCode     `json:"code"`
	Reason WorktreeReason `json:"reason,omitempty"`
	Detail string         `json:"detail,omitempty"`
	// Path and Branch, set together and only on ok, say which folder a remove
	// deleted and which branch it kept.
	Path   string `json:"path,omitempty"`
	Branch string `json:"branch,omitempty"`
}

func (WorktreeResult) kind() string { return "worktree_result" }
func (w WorktreeResult) valid() bool {
	if (w.Path == "") != (w.Branch == "") || w.Path != "" && (w.Code != OK || !validDir(w.Path) || !plainText(w.Branch, 255)) {
		return false
	}
	return Result{Code: w.Code, Reason: w.Reason, Detail: w.Detail}.valid()
}
