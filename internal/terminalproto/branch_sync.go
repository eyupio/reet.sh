package terminalproto

import "strings"

// BranchSyncFeature lets the controller ask a repository to bring its base
// branch up to date with the remote after a worktree made from it is removed.
// It is separate from branch-prune-v1 because answering it runs `git fetch`
// and `git merge --ff-only`.
const BranchSyncFeature = "branch-sync-v1"

// SupportsBranchSync reports whether the peer advertised branch-sync-v1 in
// X-Reet-Terminal-Features. Branch-sync messages are never sent without it.
func SupportsBranchSync(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == BranchSyncFeature {
			return true
		}
	}
	return false
}

// BranchSyncStatus says what a sync did to the repository folder.
type BranchSyncStatus string

const (
	// BranchSyncUpdated: the base was fast-forwarded to the remote's tip.
	BranchSyncUpdated BranchSyncStatus = "updated"
	// BranchSyncCurrent: the base already had everything the remote has.
	BranchSyncCurrent BranchSyncStatus = "current"
	// BranchSyncDirty: the folder has uncommitted changes; nothing was changed.
	BranchSyncDirty BranchSyncStatus = "dirty"
	// BranchSyncOffBase: the folder is on another branch (or detached); the
	// remote-tracking branch was fetched but the folder was left alone.
	BranchSyncOffBase BranchSyncStatus = "off_base"
	// BranchSyncDiverged: the local base has commits the remote lacks, so it
	// cannot fast-forward; nothing was changed.
	BranchSyncDiverged BranchSyncStatus = "diverged"
	// BranchSyncBusy: a merge, rebase or similar is in progress in the folder.
	BranchSyncBusy BranchSyncStatus = "busy"
	// BranchSyncNoRemote: the repository has no origin or no such remote branch.
	BranchSyncNoRemote BranchSyncStatus = "no_remote"
	// BranchSyncFetchFailed: git could not fetch (network, credentials, timeout).
	BranchSyncFetchFailed BranchSyncStatus = "fetch_failed"
)

func (s BranchSyncStatus) valid() bool {
	switch s {
	case BranchSyncUpdated, BranchSyncCurrent, BranchSyncDirty, BranchSyncOffBase, BranchSyncDiverged,
		BranchSyncBusy, BranchSyncNoRemote, BranchSyncFetchFailed:
		return true
	}
	return false
}

// BranchSync asks the machine to fetch origin and fast-forward the base branch
// checked out in the repository whose working tree top is Dir. Base empty asks
// for the repository's default branch.
type BranchSync struct {
	Dir  string `json:"dir"`
	Base string `json:"base,omitempty"`
}

func (BranchSync) kind() string { return "branch_sync" }
func (p BranchSync) valid() bool {
	return validDir(p.Dir) && (p.Base == "" || validRefName(p.Base))
}

// BranchSyncResult answers a BranchSync. With Reason set the request failed
// before git was asked and nothing else is set. Otherwise Status says what
// happened to Base; the folder is never changed except by a fast-forward.
type BranchSyncResult struct {
	Reason BranchesReason   `json:"reason,omitempty"`
	Status BranchSyncStatus `json:"status,omitempty"`
	Base   string           `json:"base,omitempty"`
}

func (BranchSyncResult) kind() string { return "branch_sync_result" }
func (r BranchSyncResult) valid() bool {
	if r.Reason != "" {
		return r.Reason.valid() && r.Status == "" && r.Base == ""
	}
	return r.Status.valid() && (r.Base == "" || validRefName(r.Base))
}
