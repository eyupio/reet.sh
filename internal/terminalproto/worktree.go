package terminalproto

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"
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
)

func (r WorktreeReason) Valid() bool {
	return r == WorktreeNotRepoRoot || r == WorktreeRepoBusy || r == WorktreeExists || r == WorktreeGitFailed || r == WorktreeOutsideHome
}

func validWorktreeDetail(s string) bool {
	return len(s) <= MaxWorktreeDetailBytes && plainText(s, MaxWorktreeDetailBytes)
}

// validWorktree: a worktree needs a tool, a run token and a Project slug; a
// Project slug is meaningless without a worktree.
func (c Create) validWorktree() bool {
	if c.Worktree == "" {
		return c.Project == ""
	}
	return c.Tool != "" && ValidRunToken(c.Worktree) && ValidSlug(c.Project)
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
// '.', '_' and '-', never "." or "..".
func validRepoName(s string) bool {
	if s == "" || len(s) > 100 || s == "." || s == ".." {
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
	Run  string `json:"run"`
	Repo string `json:"repo"`
}

func (WorktreeStatus) kind() string  { return "worktree_status" }
func (w WorktreeStatus) valid() bool { return ValidRunToken(w.Run) && validRepoName(w.Repo) }

// WorktreeRemove carries the counts the person was shown so that changes made
// after they looked cannot be thrown away.
type WorktreeRemove struct {
	Run      string `json:"run"`
	Repo     string `json:"repo"`
	Changed  int    `json:"changed"`
	Unpushed int    `json:"unpushed"`
}

func (WorktreeRemove) kind() string { return "worktree_remove" }
func (w WorktreeRemove) valid() bool {
	return ValidRunToken(w.Run) && validRepoName(w.Repo) && validWorktreeCount(w.Changed) && validWorktreeCount(w.Unpushed)
}

type WorktreeState struct {
	Path     string   `json:"path"`
	Branch   string   `json:"branch"`
	Changed  int      `json:"changed"`
	Unpushed int      `json:"unpushed"`
	Paths    []string `json:"paths,omitempty"`
}

func (WorktreeState) kind() string { return "worktree_state" }
func (w WorktreeState) valid() bool {
	if !validDir(w.Path) || !plainText(w.Branch, 255) || w.Branch == "" || !validWorktreeCount(w.Changed) || !validWorktreeCount(w.Unpushed) || len(w.Paths) > MaxWorktreePaths {
		return false
	}
	for _, p := range w.Paths {
		if p == "" || len(p) > MaxWorktreePathBytes || !utf8.ValidString(p) {
			return false
		}
		for _, r := range p {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}

// WorktreeResult answers a remove (ok, worktree_changed, worktree_refused,
// not_found, failed) or refuses a status.
type WorktreeResult struct {
	Code ResultCode `json:"code"`
}

func (WorktreeResult) kind() string  { return "worktree_result" }
func (w WorktreeResult) valid() bool { return w.Code.Valid() }
