package terminalproto

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestRepoListFeatureToken(t *testing.T) {
	for _, value := range []string{"", "repo-clone-v1", "repo-list-v10", "prefix-repo-list-v1"} {
		if SupportsRepoList(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsRepoList("repo-clone-v1, repo-list-v1") || RepoListFeature != "repo-list-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestRepoListResultValid(t *testing.T) {
	for name, tc := range map[string]struct {
		r  RepoListResult
		ok bool
	}{
		"empty list":         {RepoListResult{}, true},
		"a list":             {RepoListResult{Repos: []RepoListEntry{{Name: "eyupio/reet.sh", Private: true, PushedAt: 1700000000}, {Name: "a/b"}}, More: true}, true},
		"each reason":        {RepoListResult{Reason: RepoListNotSignedIn}, true},
		"unknown reason":     {RepoListResult{Reason: "nope"}, false},
		"reason with a list": {RepoListResult{Reason: RepoListFailed, Repos: []RepoListEntry{{Name: "a/b"}}}, false},
		"reason with more":   {RepoListResult{Reason: RepoListBusy, More: true}, false},
		"not owner/repo":     {RepoListResult{Repos: []RepoListEntry{{Name: "reet.sh"}}}, false},
		"bad owner":          {RepoListResult{Repos: []RepoListEntry{{Name: "-a/b"}}}, false},
		"a path":             {RepoListResult{Repos: []RepoListEntry{{Name: "a/b/c"}}}, false},
		"negative time":      {RepoListResult{Repos: []RepoListEntry{{Name: "a/b", PushedAt: -1}}}, false},
		"repeated, any case": {RepoListResult{Repos: []RepoListEntry{{Name: "a/b"}, {Name: "A/B"}}}, false},
	} {
		if got := tc.r.valid(); got != tc.ok {
			t.Errorf("%s: valid() = %v, want %v", name, got, tc.ok)
		}
	}
	for _, reason := range []RepoListReason{RepoListNotSignedIn, RepoListDenied, RepoListTimedOut, RepoListFailed, RepoListBusy} {
		if !(RepoListResult{Reason: reason}).valid() {
			t.Error("reason refused", reason)
		}
	}
	over := RepoListResult{}
	for i := range MaxRepoListEntries + 1 {
		over.Repos = append(over.Repos, RepoListEntry{Name: fmt.Sprintf("o/r%d", i)})
	}
	if over.valid() {
		t.Fatal("more than the limit accepted")
	}
}

// The limit is chosen so that a full list of the longest names GitHub allows
// still fits in one control message.
func TestRepoListFullOfLongestNamesFitsOneMessage(t *testing.T) {
	full := RepoListResult{More: true}
	owner := strings.Repeat("o", 39)
	for i := range MaxRepoListEntries {
		repo := fmt.Sprintf("%03d", i) + strings.Repeat("r", 97)
		full.Repos = append(full.Repos, RepoListEntry{Name: owner + "/" + repo, Private: true, PushedAt: 9999999999})
	}
	encoded, err := Encode(testMessage(full))
	if err != nil {
		t.Fatal("a full list of the longest names does not encode:", err)
	}
	if _, err := Decode(encoded); err != nil {
		t.Fatal(err)
	}
}

func TestRepoListRoundTripAndStrictness(t *testing.T) {
	for _, body := range []Body{
		RepoList{},
		RepoListResult{Repos: []RepoListEntry{{Name: "eyupio/reet.sh", Private: true, PushedAt: 1700000000}}, More: true},
		RepoListResult{Reason: RepoListNotSignedIn},
		RepoListResult{},
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
	encoded, _ := Encode(testMessage(RepoList{}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"body":{}`), []byte(`"body":{"owner":"x"}`), 1)); err == nil {
		t.Fatal("unknown request field accepted")
	}
	encoded, _ = Encode(testMessage(RepoListResult{Reason: RepoListFailed}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"reason":`), []byte(`"force":true,"reason":`), 1)); err == nil {
		t.Fatal("unknown result field accepted")
	}
	if _, err := Encode(testMessage(RepoListResult{Reason: RepoListFailed, More: true})); err == nil {
		t.Fatal("invalid result encoded")
	}
}

func TestRepoListCapability(t *testing.T) {
	feats := WorktreeFeature + "," + RepoListFeature
	ok := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "u", TerminalEnabled: true, Readiness: Ready, TerminalControl: true, Launch: true, Worktree: true, RepoList: true}
	if err := ok.ValidateFor(feats); err != nil {
		t.Fatal(err)
	}
	if err := ok.ValidateFor(WorktreeFeature + "," + RepoCloneFeature); err == nil {
		t.Fatal("repoList accepted without the negotiated feature")
	}
	if err := (Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "u", TerminalEnabled: true, Readiness: Ready, TerminalControl: true, Launch: true, RepoList: true}).ValidateFor(feats); err == nil {
		t.Fatal("repoList accepted without worktree")
	}
}
