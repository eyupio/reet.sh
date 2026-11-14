package api

import (
	"net/url"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// repoListCaps is a machine that can clone and list its GitHub repositories.
func repoListCaps() terminalproto.Capabilities {
	caps := repoCloneCaps()
	caps.RepoList = true
	return caps
}

func githubReposPath(host string) string {
	return "/api/v2/folder-projects/github-repos?" + url.Values{"host": {host}}.Encode()
}

func TestControllerAdvertisesRepoList(t *testing.T) {
	if !terminalproto.SupportsRepoList(privateControlFeatures) || !terminalproto.SupportsRepoList(agreedControlFeatures(terminalproto.Capabilities{})) {
		t.Fatalf("repo-list-v1 is not advertised: %q", privateControlFeatures)
	}
	if !isPeerReply(&terminalproto.RepoListResult{}) {
		t.Fatal("a listing's answer would end the machine's connection")
	}
}

func TestGitHubReposListsWhatTheMachineSignInReaches(t *testing.T) {
	f := browseAPIFixture(t, repoListCaps())
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.RepoList); ok {
			return terminalproto.RepoListResult{Repos: []terminalproto.RepoListEntry{
				{Name: "eyupio/reet.sh", Private: true, PushedAt: 1700000000}, {Name: "someone/shared"},
			}, More: true}
		}
		return nil
	})
	code, _, raw := f.get(t, f.cookie, githubReposPath(f.agent))
	want := `{"more":true,"repos":[{"name":"eyupio/reet.sh","private":true,"pushedAt":1700000000},{"name":"someone/shared","private":false,"pushedAt":0}]}`
	if code != 200 || strings.TrimSpace(raw) != want {
		t.Fatal(code, raw)
	}
	if sent := f.sentRequests(); len(sent) != 1 || sent[0] != (terminalproto.RepoList{}) {
		t.Fatal("requests sent:", sent)
	}
	// An empty list is an empty array, never null.
	f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.RepoListResult{} })
	if code, _, raw := f.get(t, f.cookie, githubReposPath(f.agent)); code != 200 || strings.TrimSpace(raw) != `{"more":false,"repos":[]}` {
		t.Fatal(code, raw)
	}
}

func TestGitHubReposWordsEachRefusal(t *testing.T) {
	for reason, want := range map[terminalproto.RepoListReason]struct {
		status   int
		sentence string
	}{
		terminalproto.RepoListNotSignedIn: {409, "studio isn't signed in to GitHub. Run gh auth login there, then try again."},
		terminalproto.RepoListDenied:      {409, "studio refused to list repositories: terminal access is off there, or the GitHub CLI is missing."},
		terminalproto.RepoListBusy:        {409, "studio is already listing its repositories. Try again in a moment."},
		terminalproto.RepoListTimedOut:    {504, "GitHub didn't answer studio in time. Try again, or type one."},
		terminalproto.RepoListFailed:      {502, "Couldn't list studio's repositories. Type one instead."},
	} {
		f := browseAPIFixture(t, repoListCaps())
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.RepoListResult{Reason: reason} })
		code, out, raw := f.get(t, f.cookie, githubReposPath(f.agent))
		if code != want.status || out["code"] != string(reason) || out["error"] != want.sentence {
			t.Error(reason, code, raw)
		}
	}
}

// A machine that can clone but runs a reet from before the list is never
// asked: the dialog says so and the person types the repository.
func TestGitHubReposOnAnOlderMachine(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	code, out, raw := f.get(t, f.cookie, githubReposPath(f.agent))
	if code != 409 || out["code"] != "repo_list_unsupported" || out["error"] != "Update reet on studio to list its GitHub repositories. You can still type one." {
		t.Fatal(code, raw)
	}
	if len(f.sentRequests()) != 0 {
		t.Fatal("an older machine was asked")
	}
}

func TestGitHubReposRefusesWhatItCannotUse(t *testing.T) {
	f := browseAPIFixture(t, repoListCaps())
	for name, path := range map[string]string{
		"no host":            "/api/v2/folder-projects/github-repos",
		"not an id":          githubReposPath("../x"),
		"not this account's": githubReposPath(strings.Repeat("a", 32)),
		"another parameter":  githubReposPath(f.agent) + "&owner=x",
		"host twice":         githubReposPath(f.agent) + "&host=" + f.agent,
	} {
		code, out, raw := f.get(t, f.cookie, path)
		if code != 422 || out["code"] != "unknown_machine" {
			t.Error(name, code, raw)
		}
	}
	// It makes the machine run gh, so another site may not ask.
	if code, out, raw := f.get(t, f.cookie, githubReposPath(f.agent), "Sec-Fetch-Site", "cross-site"); code != 403 || out["code"] != "origin_refused" {
		t.Fatal(code, raw)
	}
	if len(f.sentRequests()) != 0 {
		t.Fatal("a refused request reached the machine")
	}
}
