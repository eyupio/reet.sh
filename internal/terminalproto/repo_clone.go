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

// ghOwner matches a GitHub owner (user or organization) segment: letters,
// digits and hyphens, at most 39 characters, never starting or ending with a
// hyphen.
var ghOwner = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

// ghRepo matches a GitHub repository name segment: letters, digits, dots,
// hyphens and underscores, at most 100 characters.
var ghRepo = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// ValidGitHubOwner reports whether s is a GitHub owner name.
func ValidGitHubOwner(s string) bool { return ghOwner.MatchString(s) }

// ValidGitHubRepo reports whether s is a GitHub repository name as it is
// stored, without a ".git" suffix: callers strip the suffix from a URL or
// remote first. "." and ".." are never names, and neither is ".git".
func ValidGitHubRepo(s string) bool {
	return ghRepo.MatchString(s) && s != "." && s != ".." && !strings.HasSuffix(s, ".git")
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
)

func (r RepoCloneReason) valid() bool {
	switch r {
	case RepoCloneNotSignedIn, RepoCloneNoAccess, RepoCloneExists, RepoCloneTimedOut, RepoCloneFailed, RepoCloneDenied:
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
		return r.Reason.valid() && r.Dir == "" && r.DefaultBranch == "" && !r.Adopted && !r.Marked
	}
	return validDir(r.Dir) && (r.DefaultBranch == "" || validRefName(r.DefaultBranch))
}
