package terminalproto

import (
	"encoding/json"
	"strings"
)

// BranchPruneFeature lets the controller ask a repository which local branches
// can be pruned and then delete chosen ones. It is separate from branches-v1
// because answering it runs `git update-ref -d`.
const BranchPruneFeature = "branch-prune-v1"

// MaxBranchPruneList is the most branches one answer carries, and the most a
// single prune may be asked to delete.
const MaxBranchPruneList = 200

// SupportsBranchPrune reports whether the peer advertised branch-prune-v1 in
// X-Reet-Terminal-Features. Branch-prune messages are never sent without it.
func SupportsBranchPrune(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == BranchPruneFeature {
			return true
		}
	}
	return false
}

// BranchPruneSkip says why a branch is not safe to delete. Empty means it is.
type BranchPruneSkip string

const (
	// BranchPruneNotMerged: its commits are neither contained in nor
	// patch-equivalent to the base, so deleting it would lose work.
	BranchPruneNotMerged BranchPruneSkip = "not_merged"
	// BranchPruneUnpushed: it has commits its remote does not have.
	BranchPruneUnpushed BranchPruneSkip = "unpushed"
	// BranchPruneCheckedOut: a worktree has it checked out.
	BranchPruneCheckedOut BranchPruneSkip = "checked_out"
	// BranchPruneCurrent: it is the branch HEAD names.
	BranchPruneCurrent BranchPruneSkip = "current"
	// BranchPruneDefault: it is the repository's default branch.
	BranchPruneDefault BranchPruneSkip = "default"
	// BranchPruneBase: it is the base the prune compares against.
	BranchPruneBase BranchPruneSkip = "base"
	// BranchPruneMoved: it changed since the plan was made, so it is not the
	// branch the person agreed to delete.
	BranchPruneMoved BranchPruneSkip = "moved"
	// BranchPruneGitFailed: git could not delete it (or list it).
	BranchPruneGitFailed BranchPruneSkip = "git_failed"
)

func (s BranchPruneSkip) valid() bool {
	switch s {
	case BranchPruneNotMerged, BranchPruneUnpushed, BranchPruneCheckedOut, BranchPruneCurrent,
		BranchPruneDefault, BranchPruneBase, BranchPruneMoved, BranchPruneGitFailed:
		return true
	}
	return false
}

// validOID is a git object id: 40 or 64 lower-case hex characters (SHA-1 or
// SHA-256), never an abbreviation.
func validOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
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

// BranchPrunePlan asks for the local branches of the repository whose working
// tree top is Dir and whether each is safe to delete. Base names the branch
// merged work is measured against; empty asks the machine to use the
// repository's default.
type BranchPrunePlan struct {
	Dir  string `json:"dir"`
	Base string `json:"base,omitempty"`
}

func (BranchPrunePlan) kind() string { return "branch_prune_plan" }
func (p BranchPrunePlan) valid() bool {
	return validDir(p.Dir) && (p.Base == "" || validRefName(p.Base))
}

// BranchPruneBranch is one local branch in a plan. Skip is empty when deleting
// it is safe; OID is the tip the person was shown, and a prune only deletes a
// branch still at that tip.
type BranchPruneBranch struct {
	Name string          `json:"name"`
	OID  string          `json:"oid"`
	Skip BranchPruneSkip `json:"skip,omitempty"`
}

func (b BranchPruneBranch) valid() bool {
	return validRefName(b.Name) && validOID(b.OID) && (b.Skip == "" || b.Skip.valid())
}

// BranchPrunePlanResult answers a BranchPrunePlan. With Reason set, nothing
// else is. Current and Default are the checked-out and default branches, empty
// when unknown; Base is the branch merged work was measured against (empty
// when none could be found, in which case every branch skips as not_merged).
// More says the list was cut.
type BranchPrunePlanResult struct {
	Reason   BranchesReason      `json:"reason,omitempty"`
	Current  string              `json:"current,omitempty"`
	Default  string              `json:"default,omitempty"`
	Base     string              `json:"base,omitempty"`
	Branches []BranchPruneBranch `json:"branches,omitempty"`
	More     bool                `json:"more,omitempty"`
}

func (BranchPrunePlanResult) kind() string { return "branch_prune_plan_result" }
func (r BranchPrunePlanResult) valid() bool {
	if r.Reason != "" {
		return r.Reason.valid() && r.Current == "" && r.Default == "" && r.Base == "" && len(r.Branches) == 0 && !r.More
	}
	if r.Current != "" && !validRefName(r.Current) || r.Default != "" && !validRefName(r.Default) || r.Base != "" && !validRefName(r.Base) || len(r.Branches) > MaxBranchPruneList {
		return false
	}
	seen := make(map[string]bool, len(r.Branches))
	for _, b := range r.Branches {
		if !b.valid() || seen[b.Name] {
			return false
		}
		seen[b.Name] = true
	}
	data, err := json.Marshal(r)
	return err == nil && len(data) <= MaxBrowseReplyBytes
}

// BranchPruneTarget is one branch the person agreed to delete, at the tip the
// plan showed.
type BranchPruneTarget struct {
	Name string `json:"name"`
	OID  string `json:"oid"`
}

func (t BranchPruneTarget) valid() bool { return validRefName(t.Name) && validOID(t.OID) }

// BranchPrune asks to delete the named branches if each still satisfies the
// plan's safety rules and still points at the tip shown.
type BranchPrune struct {
	Dir      string              `json:"dir"`
	Base     string              `json:"base"`
	Branches []BranchPruneTarget `json:"branches"`
}

func (BranchPrune) kind() string { return "branch_prune" }
func (p BranchPrune) valid() bool {
	if !validDir(p.Dir) || !validRefName(p.Base) || len(p.Branches) == 0 || len(p.Branches) > MaxBranchPruneList {
		return false
	}
	seen := make(map[string]bool, len(p.Branches))
	for _, b := range p.Branches {
		if !b.valid() || seen[b.Name] {
			return false
		}
		seen[b.Name] = true
	}
	return true
}

// BranchPruneSkipped is a branch a prune did not delete, and why.
type BranchPruneSkipped struct {
	Name string          `json:"name"`
	Skip BranchPruneSkip `json:"skip"`
}

func (s BranchPruneSkipped) valid() bool {
	return validRefName(s.Name) && s.Skip != "" && s.Skip.valid()
}

// BranchPruneResult answers a BranchPrune. With Reason set the whole request
// failed and nothing else is. Deleted names the branches removed; Skipped says
// why each other one was kept.
type BranchPruneResult struct {
	Reason  BranchesReason       `json:"reason,omitempty"`
	Deleted []string             `json:"deleted,omitempty"`
	Skipped []BranchPruneSkipped `json:"skipped,omitempty"`
}

func (BranchPruneResult) kind() string { return "branch_prune_result" }
func (r BranchPruneResult) valid() bool {
	if r.Reason != "" {
		return r.Reason.valid() && len(r.Deleted) == 0 && len(r.Skipped) == 0
	}
	if len(r.Deleted) > MaxBranchPruneList || len(r.Skipped) > MaxBranchPruneList {
		return false
	}
	for _, name := range r.Deleted {
		if !validRefName(name) {
			return false
		}
	}
	for _, s := range r.Skipped {
		if !s.valid() {
			return false
		}
	}
	data, err := json.Marshal(r)
	return err == nil && len(data) <= MaxBrowseReplyBytes
}
