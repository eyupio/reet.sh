package terminalproto

import (
	"regexp"
	"strings"
	"unicode"
)

// RepoCloneFeature lets the controller ask a machine to clone a GitHub
// repository into its projects folder. It is negotiated separately from
// worktree-v1 because answering it runs `git clone` against github.com.
const RepoCloneFeature = "repo-clone-v1"

// SupportsRepoClone reports whether the peer advertised repo-clone-v1 in
// X-Reet-Terminal-Features. A RepoClone is never sent without it.
func SupportsRepoClone(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == RepoCloneFeature {
			return true
		}
	}
	return false
}

// RepoCloneRemoveFeature lets the controller ask a machine what a clone reet
// made holds, and to delete it. It is its own token because a machine that
// has repo-clone-v1 alone decodes strictly and would refuse these kinds.
const RepoCloneRemoveFeature = "repo-clone-remove-v1"

// SupportsRepoCloneRemove reports whether the peer advertised
// repo-clone-remove-v1 in X-Reet-Terminal-Features. A RepoCloneStatus or a
// RepoCloneRemove is never sent without it.
func SupportsRepoCloneRemove(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == RepoCloneRemoveFeature {
			return true
		}
	}
	return false
}

// ghOwner matches a GitHub owner (user or organization) segment: letters,
// digits and hyphens, at most 39 characters, never starting or ending with a
// hyphen.
var ghOwner = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

// ghRepo matches a GitHub repository name segment: letters, digits, dots,
// hyphens and underscores, at most 100 characters.
var ghRepo = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// ValidGitHubOwner reports whether s is a GitHub owner name.
func ValidGitHubOwner(s string) bool { return ghOwner.MatchString(s) }

// ValidGitHubRepo reports whether s is a GitHub repository name that has
// already been stripped of its ".git" suffix: callers strip one trailing
// ".git" from a URL or remote first, so a name that still ends in ".git" is
// refused. A name made only of dots and a name ending in a dot are refused
// too, because the name becomes a folder and Windows path handling drops
// trailing dots; a leading dot or dash is allowed.
func ValidGitHubRepo(s string) bool {
	return ghRepo.MatchString(s) && strings.Trim(s, ".") != "" &&
		!strings.HasSuffix(s, ".") && !strings.HasSuffix(s, ".git")
}

// ParseGitHubRepo reads what a person typed to name a repository on
// github.com: "owner/repo" or "https://github.com/owner/repo", each with an
// optional ".git" and, on the URL only, one trailing slash. Surrounding whitespace is ignored and the host is matched without
// regard to case. Every other form (other schemes, userinfo, ports, query
// strings, fragments, extra path segments, control characters) is refused:
// ok is false and owner, repo are empty.
func ParseGitHubRepo(input string) (owner, repo string, ok bool) {
	s := strings.TrimSpace(input)
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", "", false
	}
	const prefix = "https://github.com/"
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		s = strings.TrimSuffix(s[len(prefix):], "/")
	}
	owner, repo, found := strings.Cut(s, "/")
	if !found {
		return "", "", false
	}
	repo = strings.TrimSuffix(repo, ".git")
	if !ValidGitHubOwner(owner) || !ValidGitHubRepo(repo) {
		return "", "", false
	}
	return owner, repo, true
}

// RepoClone asks the machine to clone github.com/Owner/Repo into its
// projects folder. It is sent only to a connection that negotiated
// repo-clone-v1.
type RepoClone struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

func (RepoClone) kind() string { return "repo_clone" }
func (c RepoClone) valid() bool {
	return ValidGitHubOwner(c.Owner) && ValidGitHubRepo(c.Repo)
}

// RepoCloneReason says why a clone did not leave a repository folder.
type RepoCloneReason string

const (
	// RepoCloneNotSignedIn: the machine has no usable GitHub sign-in.
	RepoCloneNotSignedIn RepoCloneReason = "not_signed_in"
	// RepoCloneNoAccess: the repository is missing, or not visible to this
	// sign-in.
	RepoCloneNoAccess RepoCloneReason = "no_access"
	// RepoCloneExists: the folder is there and is something else.
	RepoCloneExists RepoCloneReason = "exists"
	// RepoCloneTimedOut: the clone ran out of time.
	RepoCloneTimedOut RepoCloneReason = "timed_out"
	// RepoCloneFailed: git failed for another reason.
	RepoCloneFailed RepoCloneReason = "failed"
	// RepoCloneDenied: the machine's owner does not allow cloning.
	RepoCloneDenied RepoCloneReason = "denied"
	// RepoCloneBusy: the machine is already cloning; nothing was started.
	RepoCloneBusy RepoCloneReason = "busy"
	// RepoCloneSSHRewrite: the machine's git settings send github.com over
	// ssh, which the clone refuses.
	RepoCloneSSHRewrite RepoCloneReason = "ssh_rewrite"
	// RepoCloneChanged: a remove was refused because the clone is no longer
	// what the person was shown. Only a RepoCloneRemoved carries it.
	RepoCloneChanged RepoCloneReason = "changed"
	// RepoCloneNotRemovable: the clone holds something that deleting it
	// would lose, or reet did not make it. Never an answer to a RepoClone.
	RepoCloneNotRemovable RepoCloneReason = "not_removable"
)

func (r RepoCloneReason) valid() bool {
	switch r {
	case RepoCloneNotSignedIn, RepoCloneNoAccess, RepoCloneExists, RepoCloneTimedOut, RepoCloneFailed, RepoCloneDenied, RepoCloneBusy, RepoCloneSSHRewrite,
		RepoCloneChanged, RepoCloneNotRemovable:
		return true
	}
	return false
}

// removeOnly reports the reasons only the status and remove answers carry.
func (r RepoCloneReason) removeOnly() bool {
	return r == RepoCloneChanged || r == RepoCloneNotRemovable
}

// stateReason reports the reasons a RepoCloneState may carry; a
// RepoCloneRemoved may carry these and changed.
func (r RepoCloneReason) stateReason() bool {
	switch r {
	case RepoCloneDenied, RepoCloneExists, RepoCloneFailed, RepoCloneTimedOut, RepoCloneBusy, RepoCloneNotRemovable:
		return true
	}
	return false
}

// RepoCloneResult answers a RepoClone. With Reason set nothing was cloned and
// every other field is empty; otherwise Dir is the repository folder.
// Adopted says the folder was already a clone of the repository and was
// kept; Marked says the reet marker is present in it.
type RepoCloneResult struct {
	Reason        RepoCloneReason `json:"reason,omitempty"`
	Dir           string          `json:"dir,omitempty"`
	DefaultBranch string          `json:"defaultBranch,omitempty"`
	Adopted       bool            `json:"adopted,omitempty"`
	Marked        bool            `json:"marked,omitempty"`
}

func (RepoCloneResult) kind() string { return "repo_clone_result" }
func (r RepoCloneResult) valid() bool {
	if r.Reason != "" {
		return r.Reason.valid() && !r.Reason.removeOnly() && r.Dir == "" && r.DefaultBranch == "" && !r.Adopted && !r.Marked
	}
	return validDir(r.Dir) && (r.DefaultBranch == "" || validRefName(r.DefaultBranch))
}

// maxRepoCloneCount bounds each count of a RepoCloneState.
const maxRepoCloneCount = 1 << 30

// validRepoOrigin reports whether s is "owner/repo", both GitHub names.
func validRepoOrigin(s string) bool {
	owner, repo, ok := strings.Cut(s, "/")
	return ok && ValidGitHubOwner(owner) && ValidGitHubRepo(repo)
}

// validCloneFingerprint reports whether s is a SHA-256 in lower-case hex.
func validCloneFingerprint(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := range len(s) {
		if (s[i] < '0' || s[i] > '9') && (s[i] < 'a' || s[i] > 'f') {
			return false
		}
	}
	return true
}

// RepoCloneStatus asks what the clone at Dir holds that deleting it would
// lose. Origin is the "owner/repo" the clone was made from; Dir must be the
// folder the machine clones that repository into. It is sent only to a
// connection that negotiated repo-clone-remove-v1.
type RepoCloneStatus struct {
	Dir    string `json:"dir"`
	Origin string `json:"origin"`
}

func (RepoCloneStatus) kind() string  { return "repo_clone_status" }
func (s RepoCloneStatus) valid() bool { return validDir(s.Dir) && validRepoOrigin(s.Origin) }

// RepoCloneState answers a RepoCloneStatus. With Reason set nothing could be
// said about the folder and every other field is empty. Marked says the
// folder is a clone reet made of the repository asked about; without it
// nothing was counted and the folder is never removed. Otherwise the four
// counts are what deleting the clone would lose, and Fingerprint names
// exactly what was looked at: a RepoCloneRemove must repeat it.
type RepoCloneState struct {
	Reason RepoCloneReason `json:"reason,omitempty"`
	Marked bool            `json:"marked,omitempty"`
	// Worktrees is the linked worktrees, not counting the clone itself.
	Worktrees int `json:"worktrees,omitempty"`
	// Changed is the uncommitted and untracked entries. Ignored files are
	// not counted; they are deleted with the clone.
	Changed int `json:"changed,omitempty"`
	Stashes int `json:"stashes,omitempty"`
	// Unpushed is the local branches (and a detached HEAD) with commits on
	// no remote-tracking branch of origin.
	Unpushed    int    `json:"unpushed,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

func (RepoCloneState) kind() string { return "repo_clone_state" }
func (s RepoCloneState) valid() bool {
	counted := s.Worktrees != 0 || s.Changed != 0 || s.Stashes != 0 || s.Unpushed != 0 || s.Fingerprint != ""
	if s.Reason != "" {
		return s.Reason.stateReason() && !s.Marked && !counted
	}
	if !s.Marked {
		return !counted
	}
	for _, n := range []int{s.Worktrees, s.Changed, s.Stashes, s.Unpushed} {
		if n < 0 || n > maxRepoCloneCount {
			return false
		}
	}
	return validCloneFingerprint(s.Fingerprint)
}

// Removable reports whether the clone may be deleted: reet made it and
// nothing in it would be lost.
func (s RepoCloneState) Removable() bool {
	return s.Reason == "" && s.Marked && s.Worktrees == 0 && s.Changed == 0 && s.Stashes == 0 && s.Unpushed == 0
}

// RepoCloneRemove asks the machine to delete the clone at Dir. Fingerprint
// is the one the RepoCloneState shown to the person carried; the machine
// looks again and refuses when it differs. It is sent only to a connection
// that negotiated repo-clone-remove-v1.
type RepoCloneRemove struct {
	Dir         string `json:"dir"`
	Origin      string `json:"origin"`
	Fingerprint string `json:"fingerprint"`
}

func (RepoCloneRemove) kind() string { return "repo_clone_remove" }
func (r RepoCloneRemove) valid() bool {
	return validDir(r.Dir) && validRepoOrigin(r.Origin) && validCloneFingerprint(r.Fingerprint)
}

// RepoCloneRemoved answers a RepoCloneRemove. With Reason set the folder was
// kept (failed alone may mean a delete that started and stopped part way);
// otherwise Dir is the folder that was deleted.
type RepoCloneRemoved struct {
	Reason RepoCloneReason `json:"reason,omitempty"`
	Dir    string          `json:"dir,omitempty"`
}

func (RepoCloneRemoved) kind() string { return "repo_clone_removed" }
func (r RepoCloneRemoved) valid() bool {
	if r.Reason != "" {
		return (r.Reason.stateReason() || r.Reason == RepoCloneChanged) && r.Dir == ""
	}
	return validDir(r.Dir)
}
