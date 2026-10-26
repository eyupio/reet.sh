package terminalproto

import (
	"encoding/json"
	"strings"
)

// BrowseFeature lets the controller list one level of folders on a machine.
const BrowseFeature = "browse-v1"

const (
	// MaxBrowseEntries is the most folders one browse answer lists.
	MaxBrowseEntries = 200
	// MaxBrowseReplyBytes bounds an encoded BrowseResult body, well inside
	// MaxControlBytes so the envelope always fits.
	MaxBrowseReplyBytes = 12 << 10
	// MaxBrowseNameBytes bounds a folder name and the After cursor.
	MaxBrowseNameBytes = 255
	// MaxBrowseFilterBytes bounds a Browse filter.
	MaxBrowseFilterBytes = 64
)

// SupportsBrowse reports whether the peer advertised browse-v1 in
// X-Reet-Terminal-Features. Browse messages are never sent without it.
func SupportsBrowse(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == BrowseFeature {
			return true
		}
	}
	return false
}

// BrowseReason says why a folder could not be listed (BrowseResult.Reason).
type BrowseReason string

const (
	// BrowseNotFound: the folder does not exist.
	BrowseNotFound BrowseReason = "not_found"
	// BrowseNotDir: the path is not a folder.
	BrowseNotDir BrowseReason = "not_dir"
	// BrowseOutsideHome: the folder, or where a link in it leads, is outside home.
	BrowseOutsideHome BrowseReason = "outside_home"
	// BrowseDenied: this user may not read the folder.
	BrowseDenied BrowseReason = "denied"
	// BrowseUnreadable: the folder could not be read.
	BrowseUnreadable BrowseReason = "unreadable"
	// BrowseConflict: the browse could not run now (another one is running).
	BrowseConflict BrowseReason = "conflict"
)

func (r BrowseReason) valid() bool {
	switch r {
	case BrowseNotFound, BrowseNotDir, BrowseOutsideHome, BrowseDenied, BrowseUnreadable, BrowseConflict:
		return true
	}
	return false
}

// BrowseRepo says how a folder relates to a git working tree.
type BrowseRepo string

const (
	// BrowseRepoRoot: the folder is the top of a working tree.
	BrowseRepoRoot BrowseRepo = "root"
	// BrowseRepoInside: the folder lies inside a working tree (BrowseResult.RepoTop).
	BrowseRepoInside BrowseRepo = "inside"
	// BrowseRepoNone: the folder is in no working tree.
	BrowseRepoNone BrowseRepo = "none"
)

func (r BrowseRepo) valid() bool {
	return r == BrowseRepoRoot || r == BrowseRepoInside || r == BrowseRepoNone
}

// Browse asks for the folders directly inside Path, one level and folders
// only. An empty Path means the user's home. After is the last name of the
// previous page and Filter a case-insensitive substring of names; Hidden
// includes folders whose name starts with a dot.
type Browse struct {
	Path   string `json:"path,omitempty"`
	After  string `json:"after,omitempty"`
	Filter string `json:"filter,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

func (Browse) kind() string { return "browse" }
func (b Browse) valid() bool {
	return (b.Path == "" || validDir(b.Path)) && validBrowseText(b.After, MaxBrowseNameBytes) && validBrowseText(b.Filter, MaxBrowseFilterBytes)
}

func validBrowseText(s string, max int) bool { return len(s) <= max && plainText(s, max) }

// BrowseEntry is one folder. RepoRoot marks the top of a git working tree.
type BrowseEntry struct {
	Name     string `json:"name"`
	Dir      bool   `json:"dir,omitempty"`
	RepoRoot bool   `json:"repoRoot,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
}

func (e BrowseEntry) valid() bool {
	return e.Name != "" && e.Name != "." && e.Name != ".." && !strings.Contains(e.Name, "/") && validBrowseText(e.Name, MaxBrowseNameBytes)
}

// BrowseResult answers a Browse. With Reason set nothing else is; otherwise
// Path, Home and Repo are always set. Parent is empty when Path has no parent
// inside home. RepoTop is the working tree's top and is set exactly when Repo
// is root or inside. More says another page follows (send the last name as
// After). Partial says the listing stopped early, with Scanned entries read and
// Omitted not listed.
type BrowseResult struct {
	Path    string        `json:"path,omitempty"`
	Home    string        `json:"home,omitempty"`
	Parent  string        `json:"parent,omitempty"`
	Repo    BrowseRepo    `json:"repo,omitempty"`
	RepoTop string        `json:"repoTop,omitempty"`
	Entries []BrowseEntry `json:"entries,omitempty"`
	More    bool          `json:"more,omitempty"`
	Partial bool          `json:"partial,omitempty"`
	Scanned int           `json:"scanned,omitempty"`
	Omitted int           `json:"omitted,omitempty"`
	Reason  BrowseReason  `json:"reason,omitempty"`
}

func (BrowseResult) kind() string { return "browse_result" }
func (r BrowseResult) valid() bool {
	if r.Reason != "" {
		return r.Reason.valid() && r.Path == "" && r.Home == "" && r.Parent == "" && r.Repo == "" && r.RepoTop == "" &&
			len(r.Entries) == 0 && !r.More && !r.Partial && r.Scanned == 0 && r.Omitted == 0
	}
	if !validDir(r.Path) || !validDir(r.Home) || r.Parent != "" && !validDir(r.Parent) || !r.Repo.valid() || len(r.Entries) > MaxBrowseEntries ||
		!validWorktreeCount(r.Scanned) || !validWorktreeCount(r.Omitted) {
		return false
	}
	if (r.Repo == BrowseRepoNone) != (r.RepoTop == "") || r.RepoTop != "" && !validDir(r.RepoTop) {
		return false
	}
	for _, e := range r.Entries {
		if !e.valid() {
			return false
		}
	}
	data, err := json.Marshal(r)
	return err == nil && len(data) <= MaxBrowseReplyBytes
}
