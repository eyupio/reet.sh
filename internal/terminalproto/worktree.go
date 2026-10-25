package terminalproto

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const WorktreeFeature = "worktree-v1"
const MaxWorktreePaths = 20
const MaxWorktreePathBytes = 255
const maxWorktreeCount = 1 << 30

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
