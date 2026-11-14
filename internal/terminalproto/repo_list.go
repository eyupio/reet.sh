package terminalproto

import "strings"

// RepoListFeature lets the controller ask a machine which GitHub repositories
// its gh sign-in can reach, to offer them when a project is added from
// GitHub. A peer that has repo-clone-v1 alone decodes strictly and would
// refuse these kinds.
const RepoListFeature = "repo-list-v1"

// SupportsRepoList reports whether the peer advertised repo-list-v1 in
// X-Reet-Terminal-Features. A RepoList is never sent without it.
func SupportsRepoList(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == RepoListFeature {
			return true
		}
	}
	return false
}

// MaxRepoListEntries bounds a RepoListResult: the most recently pushed
// repositories, at most this many. 200 of the longest names GitHub allows
// still fit in one control message.
const MaxRepoListEntries = 200

// RepoList asks the machine for the GitHub repositories its gh sign-in can
// reach (its own, its organizations' and those shared with it), most
// recently pushed first. It carries nothing: the machine's sign-in decides.
// It is sent only to a connection that negotiated repo-list-v1.
type RepoList struct{}

func (RepoList) kind() string { return "repo_list" }
func (RepoList) valid() bool  { return true }

// RepoListEntry is one repository: Name is "owner/repo"; PushedAt is when it
// was last pushed to, in Unix seconds, 0 when GitHub gave none.
type RepoListEntry struct {
	Name     string `json:"name"`
	Private  bool   `json:"private,omitempty"`
	PushedAt int64  `json:"pushedAt,omitempty"`
}

// RepoListReason says why a RepoList has no list.
type RepoListReason string

const (
	// RepoListNotSignedIn: gh on the machine has no usable GitHub sign-in.
	RepoListNotSignedIn RepoListReason = "not_signed_in"
	// RepoListDenied: terminal access is off there, or gh is missing.
	RepoListDenied RepoListReason = "denied"
	// RepoListTimedOut: GitHub did not answer in time.
	RepoListTimedOut RepoListReason = "timed_out"
	// RepoListFailed: gh failed for another reason (GitHub unreachable, a
	// reply that could not be read).
	RepoListFailed RepoListReason = "failed"
	// RepoListBusy: the machine is already listing; nothing was started.
	RepoListBusy RepoListReason = "busy"
)

func (r RepoListReason) valid() bool {
	switch r {
	case RepoListNotSignedIn, RepoListDenied, RepoListTimedOut, RepoListFailed, RepoListBusy:
		return true
	}
	return false
}

// RepoListResult answers a RepoList. With Reason set there is no list and
// Repos is empty; otherwise Repos holds at most MaxRepoListEntries distinct
// repositories, most recently pushed first. More says GitHub had more than
// were sent.
type RepoListResult struct {
	Reason RepoListReason  `json:"reason,omitempty"`
	Repos  []RepoListEntry `json:"repos,omitempty"`
	More   bool            `json:"more,omitempty"`
}

func (RepoListResult) kind() string { return "repo_list_result" }
func (r RepoListResult) valid() bool {
	if r.Reason != "" {
		return r.Reason.valid() && len(r.Repos) == 0 && !r.More
	}
	if len(r.Repos) > MaxRepoListEntries {
		return false
	}
	seen := make(map[string]bool, len(r.Repos))
	for _, e := range r.Repos {
		key := strings.ToLower(e.Name)
		if !validRepoOrigin(e.Name) || e.PushedAt < 0 || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}
