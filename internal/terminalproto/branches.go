package terminalproto

import (
	"encoding/json"
	"strings"
)

// BranchesFeature lets the controller list a repository's branches. It is
// separate from browse-v1 because answering it runs git.
const BranchesFeature = "branches-v1"

// MaxBranchNames is the most branch names one answer lists.
const MaxBranchNames = 200

// SupportsBranches reports whether the peer advertised branches-v1 in
// X-Reet-Terminal-Features. Branches messages are never sent without it.
func SupportsBranches(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == BranchesFeature {
			return true
		}
	}
	return false
}

// BranchesReason says why branches could not be listed (BranchesResult.Reason).
type BranchesReason string

const (
	// BranchesNotRepoRoot: the folder is not the top of a git working tree.
	BranchesNotRepoRoot BranchesReason = "not_repo_root"
	// BranchesOutsideHome: the folder is not a real directory of this user inside home.
	BranchesOutsideHome BranchesReason = "outside_home"
	// BranchesGitFailed: git could not list the branches.
	BranchesGitFailed BranchesReason = "git_failed"
	// BranchesConflict: the listing could not run now (another one is running).
	BranchesConflict BranchesReason = "conflict"
)

func (r BranchesReason) valid() bool {
	switch r {
	case BranchesNotRepoRoot, BranchesOutsideHome, BranchesGitFailed, BranchesConflict:
		return true
	}
	return false
}

// Branches asks for the local branches of the repository whose working tree
// top is Dir. Filter is a case-insensitive substring of names.
type Branches struct {
	Dir    string `json:"dir"`
	Filter string `json:"filter,omitempty"`
}

func (Branches) kind() string { return "branches" }
func (b Branches) valid() bool {
	return validDir(b.Dir) && validBrowseText(b.Filter, MaxBrowseFilterBytes)
}

// BranchesResult answers a Branches. Current is the checked-out branch (empty
// when HEAD is detached) and Default the repository's default branch (empty
// when unknown). More says the list was cut at MaxBranchNames. With Reason set
// nothing else is.
type BranchesResult struct {
	Current  string         `json:"current,omitempty"`
	Default  string         `json:"default,omitempty"`
	Branches []string       `json:"branches,omitempty"`
	More     bool           `json:"more,omitempty"`
	Reason   BranchesReason `json:"reason,omitempty"`
}

func (BranchesResult) kind() string { return "branches_result" }
func (r BranchesResult) valid() bool {
	if r.Reason != "" {
		return r.Reason.valid() && r.Current == "" && r.Default == "" && len(r.Branches) == 0 && !r.More
	}
	if len(r.Branches) > MaxBranchNames || r.Current != "" && !validRefName(r.Current) || r.Default != "" && !validRefName(r.Default) {
		return false
	}
	for _, name := range r.Branches {
		if !validRefName(name) {
			return false
		}
	}
	data, err := json.Marshal(r)
	return err == nil && len(data) <= MaxBrowseReplyBytes
}

// validRefName applies the worktree branch rule (non-empty plain text of at
// most 255 bytes) together with git's own ref-name rules (git
// check-ref-format), so a name the connector sends is one git accepts and can
// never be read as an option.
func validRefName(s string) bool {
	if s == "" || s == "@" || len(s) > MaxBrowseNameBytes || !plainText(s, MaxBrowseNameBytes) ||
		s[0] == '-' || s[0] == '/' || s[len(s)-1] == '/' || s[len(s)-1] == '.' ||
		strings.Contains(s, "..") || strings.Contains(s, "//") || strings.Contains(s, "@{") ||
		strings.ContainsAny(s, " ~^:?*[\\\x7f") {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if part[0] == '.' || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
