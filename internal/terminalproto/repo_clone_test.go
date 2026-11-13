package terminalproto

import (
	"bytes"
	"strings"
	"testing"
)

func TestRepoCloneFeatureToken(t *testing.T) {
	for _, value := range []string{"", "base-fresh-v1", "repo-clone-v10", "prefix-repo-clone-v1"} {
		if SupportsRepoClone(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsRepoClone("base-fresh-v1, repo-clone-v1") || RepoCloneFeature != "repo-clone-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestParseGitHubRepo(t *testing.T) {
	for in, want := range map[string]string{
		"eyupio/reet.sh":                         "eyupio/reet.sh",
		"https://github.com/eyupio/reet.sh":      "eyupio/reet.sh",
		"https://github.com/eyupio/reet.sh.git":  "eyupio/reet.sh",
		"https://github.com/eyupio/reet.sh/":     "eyupio/reet.sh",
		"https://github.com/eyupio/reet.sh.git/": "eyupio/reet.sh",
		"eyupio/reet.sh.git":                     "eyupio/reet.sh",
		"  eyupio/reet.sh \n":                    "eyupio/reet.sh",
		"https://GitHub.com/eyupio/reet.sh":      "eyupio/reet.sh",
		"a/.github":                              "a/.github",
		"a/-x":                                   "a/-x",
		"a/b":                                    "a/b",
		strings.Repeat("a", 39) + "/" + "r":      strings.Repeat("a", 39) + "/r",
	} {
		o, r, ok := ParseGitHubRepo(in)
		if !ok || o+"/"+r != want {
			t.Errorf("ParseGitHubRepo(%q) = %q, %q, %v; want %s", in, o, r, ok, want)
		}
	}
	for _, in := range []string{
		"", "a", "a/b/c", "a/..", "a/.", "-a/b", "a-/b", "a/b c", "a/.git", "/b", "a/",
		"http://github.com/a/b",
		"https://gitlab.com/a/b",
		"https://github.example.com/a/b",
		"https://github.com.evil.example/a/b",
		"ssh://git@github.com/a/b",
		"git@github.com:a/b.git",
		"github.com/a/b",
		"https://user@github.com/a/b",
		"https://user:pw@github.com/a/b",
		"https://github.com:443/a/b",
		"https://github.com/a/b?x=1",
		"https://github.com/a/b#frag",
		"https://github.com/a/b//",
		"https://github.com/a/b/extra",
		"https://github.com/a",
		"a/b?x=1",
		"a/b#x",
		"a/b/",
		"a/b//",
		strings.Repeat("a", 40) + "/b",
		"a/" + strings.Repeat("b", 101),
		"a/...", "a/....git", "a/b.", "a/b..git", "https://github.com/a/b.",
		"a/b\x00",
		"a\x00/b",
		"a/b\tc",
		"a/b\nc",
		"a\x7f/b",
	} {
		if o, r, ok := ParseGitHubRepo(in); ok {
			t.Errorf("ParseGitHubRepo(%q) accepted as %q/%q", in, o, r)
		}
	}
}

func TestValidGitHubNames(t *testing.T) {
	for _, owner := range []string{"a", "A1", "a-b", strings.Repeat("a", 39)} {
		if !ValidGitHubOwner(owner) {
			t.Error("owner rejected", owner)
		}
	}
	for _, owner := range []string{"", "-a", "a-", "a_b", "a.b", "a/b", strings.Repeat("a", 40)} {
		if ValidGitHubOwner(owner) {
			t.Error("owner accepted", owner)
		}
	}
	for _, repo := range []string{"r", "reet.sh", "a_b-c", ".github", "-x", strings.Repeat("r", 100)} {
		if !ValidGitHubRepo(repo) {
			t.Error("repo rejected", repo)
		}
	}
	for _, repo := range []string{"", ".", "..", "...", "....", "a.", "a..", ".git", "x.git", "a b", "a/b", strings.Repeat("r", 101)} {
		if ValidGitHubRepo(repo) {
			t.Error("repo accepted", repo)
		}
	}
}

func TestRepoCloneValid(t *testing.T) {
	for name, c := range map[string]RepoClone{
		"ok":          {Owner: "eyupio", Repo: "reet.sh"},
		"bad owner":   {Owner: "-a", Repo: "r"},
		"empty owner": {Repo: "r"},
		"bad repo":    {Owner: "a", Repo: ".."},
		"empty repo":  {Owner: "a"},
		"git suffix":  {Owner: "a", Repo: "r.git"},
	} {
		if got, want := c.valid(), name == "ok"; got != want {
			t.Errorf("%s: valid() = %v, want %v", name, got, want)
		}
	}
}

func TestRepoCloneResultValid(t *testing.T) {
	cases := map[string]struct {
		r    RepoCloneResult
		want bool
	}{
		"cloned":              {RepoCloneResult{Dir: "/home/u/eyupio/reet.sh", DefaultBranch: "main", Marked: true}, true},
		"adopted":             {RepoCloneResult{Dir: "/home/u/x", Adopted: true}, true},
		"dir only":            {RepoCloneResult{Dir: "/home/u/x"}, true},
		"each reason":         {RepoCloneResult{Reason: RepoCloneNotSignedIn}, true},
		"no access":           {RepoCloneResult{Reason: RepoCloneNoAccess}, true},
		"exists":              {RepoCloneResult{Reason: RepoCloneExists}, true},
		"timed out":           {RepoCloneResult{Reason: RepoCloneTimedOut}, true},
		"failed":              {RepoCloneResult{Reason: RepoCloneFailed}, true},
		"denied":              {RepoCloneResult{Reason: RepoCloneDenied}, true},
		"busy":                {RepoCloneResult{Reason: RepoCloneBusy}, true},
		"empty":               {RepoCloneResult{}, false},
		"dir and reason":      {RepoCloneResult{Dir: "/home/u/x", Reason: RepoCloneFailed}, false},
		"unknown reason":      {RepoCloneResult{Reason: "boom"}, false},
		"relative dir":        {RepoCloneResult{Dir: "x"}, false},
		"control in dir":      {RepoCloneResult{Dir: "/home/u/x\n"}, false},
		"bad default branch":  {RepoCloneResult{Dir: "/home/u/x", DefaultBranch: "-m"}, false},
		"reason with branch":  {RepoCloneResult{Reason: RepoCloneFailed, DefaultBranch: "main"}, false},
		"reason with adopted": {RepoCloneResult{Reason: RepoCloneFailed, Adopted: true}, false},
		"reason with marked":  {RepoCloneResult{Reason: RepoCloneFailed, Marked: true}, false},
		"branch without dir":  {RepoCloneResult{DefaultBranch: "main"}, false},
		"flags without dir":   {RepoCloneResult{Marked: true}, false},
	}
	for _, reason := range []RepoCloneReason{RepoCloneNotSignedIn, RepoCloneNoAccess, RepoCloneExists, RepoCloneTimedOut, RepoCloneFailed, RepoCloneDenied, RepoCloneBusy} {
		cases["reason "+string(reason)] = struct {
			r    RepoCloneResult
			want bool
		}{RepoCloneResult{Reason: reason}, true}
	}
	for name, c := range cases {
		if got := c.r.valid(); got != c.want {
			t.Errorf("%s: valid() = %v, want %v", name, got, c.want)
		}
	}
}

func TestRepoCloneRoundTripAndLimits(t *testing.T) {
	for _, body := range []Body{
		RepoClone{Owner: "eyupio", Repo: "reet.sh"},
		RepoCloneResult{Dir: "/home/u/eyupio/reet.sh", DefaultBranch: "main", Adopted: true, Marked: true},
		RepoCloneResult{Reason: RepoCloneNoAccess},
	} {
		encoded, err := Encode(testMessage(body))
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		if _, err = Encode(decoded); err != nil {
			t.Fatal(err)
		}
	}
	encoded, _ := Encode(testMessage(RepoClone{Owner: "a", Repo: "b"}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"owner":`), []byte(`"force":true,"owner":`), 1)); err == nil {
		t.Fatal("unknown field accepted")
	}
	encoded, _ = Encode(testMessage(RepoCloneResult{Reason: RepoCloneFailed}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"reason":`), []byte(`"force":true,"reason":`), 1)); err == nil {
		t.Fatal("unknown result field accepted")
	}
	if _, err := Encode(testMessage(RepoClone{Owner: "-a", Repo: "b"})); err == nil {
		t.Fatal("invalid request encoded")
	}
	if _, err := Encode(testMessage(RepoCloneResult{Dir: "/a", Reason: RepoCloneFailed})); err == nil {
		t.Fatal("invalid result encoded")
	}
}

func TestRepoCloneCapability(t *testing.T) {
	feats := WorktreeFeature + "," + RepoCloneFeature
	ok := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "u", TerminalEnabled: true, Readiness: Ready, TerminalControl: true, Launch: true, Worktree: true, RepoClone: true}
	if err := ok.ValidateFor(feats); err != nil {
		t.Fatal(err)
	}
	if err := ok.ValidateFor(WorktreeFeature); err == nil {
		t.Fatal("repoClone accepted without the negotiated feature")
	}
	if err := (Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "u", TerminalEnabled: true, Readiness: Ready, TerminalControl: true, Launch: true, RepoClone: true}).ValidateFor(feats); err == nil {
		t.Fatal("repoClone accepted without worktree")
	}
}
