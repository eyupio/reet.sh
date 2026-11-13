package terminalhost

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ghOwner matches a GitHub owner (user or organization) segment: letters,
// digits and hyphens, at most 39 characters, never starting or ending with a
// hyphen.
var ghOwner = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

// ghRepo matches a GitHub repository name segment: letters, digits, dots,
// hyphens and underscores, at most 100 characters.
var ghRepo = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// ghRemote matches the owner and repo path of a github.com remote URL, in
// any of the forms git, ssh or https deals in:
// "github.com/OWNER/REPO", "github.com:OWNER/REPO".
var ghRemote = regexp.MustCompile(`^(?:https://|ssh://(?:[^@/]+@)?|[^@/]+@)?github\.com[:/]([^/]+)/([^/]+?)/?$`)

// GitHubRemote reports the owner and repo a remote URL names, only for
// github.com: "https://github.com/OWNER/REPO[.git]", "git@github.com:OWNER/REPO.git"
// and "ssh://git@github.com/OWNER/REPO" forms. Any other host (including a
// look-alike such as github.example.com or github.com.evil.example), a
// malformed path, or an owner or repo that fails validation (including "."
// or "..") is rejected: ok is false and owner, repo are empty.
func GitHubRemote(url string) (owner, repo string, ok bool) {
	m := ghRemote.FindStringSubmatch(url)
	if m == nil {
		return "", "", false
	}
	owner, repo = m[1], m[2]
	repo = strings.TrimSuffix(repo, ".git")
	if repo == "." || repo == ".." || !ghOwner.MatchString(owner) || !ghRepo.MatchString(repo) {
		return "", "", false
	}
	return owner, repo, true
}

// networkGitEnv is the extra environment a git call that may reach
// github.com needs on top of gitEnvironment: GIT_TERMINAL_PROMPT=0 so a
// missing or expired gh sign-in fails instead of blocking on a prompt that
// has nowhere to go.
func networkGitEnv() []string {
	return []string{"GIT_TERMINAL_PROMPT=0"}
}

// ghCredentialArgs returns the git -c arguments that reset every inherited
// credential helper and then name gh's own, for github.com only, so git
// authenticates a fetch from github.com with the machine's `gh` sign-in and
// nowhere else. It returns nil when gh is not found on PATH, or is found
// only as a relative path (a LookPath quirk for "."-containing PATH
// entries, never a real install).
func ghCredentialArgs() []string {
	gh, err := exec.LookPath("gh")
	if err != nil || !filepath.IsAbs(gh) {
		return nil
	}
	return []string{
		"-c", "credential.helper=",
		"-c", "credential.https://github.com.helper=!" + gh + " auth git-credential",
	}
}
