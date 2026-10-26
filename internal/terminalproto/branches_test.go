package terminalproto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestBranchesFeatureToken(t *testing.T) {
	for _, value := range []string{"", "worktree-v1", "branches-v10", "prefix-branches-v1"} {
		if SupportsBranches(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsBranches("worktree-v1, branches-v1") || BranchesFeature != "branches-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestBranchesRoundTripAndLimits(t *testing.T) {
	for _, body := range []Body{
		Branches{Dir: "/home/u/api", Filter: "fea"},
		BranchesResult{Current: "main", Default: "main", Branches: []string{"main", "feature/x", "release-1.2"}, More: true},
		BranchesResult{Branches: []string{}},
		BranchesResult{Reason: BranchesNotRepoRoot},
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
	encoded, _ := Encode(testMessage(Branches{Dir: "/home/u/api"}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"dir":`), []byte(`"remote":"o","dir":`), 1)); err == nil {
		t.Fatal("unknown field accepted")
	}

	for name, b := range map[string]Branches{
		"no dir": {}, "relative dir": {Dir: "api"}, "control in dir": {Dir: "/a\n"},
		"filter 65 bytes": {Dir: "/a", Filter: strings.Repeat("a", 65)}, "control in filter": {Dir: "/a", Filter: "a\x00"},
	} {
		if b.valid() {
			t.Error("accepted", name)
		}
	}
	if !(Branches{Dir: "/a", Filter: strings.Repeat("a", 64)}).valid() {
		t.Error("64 byte filter refused")
	}

	names := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("b%d", i)
		}
		return out
	}
	if !(BranchesResult{Current: "main", Default: "main", Branches: names(MaxBranchNames)}).valid() {
		t.Error("200 names refused")
	}
	invalid := map[string]BranchesResult{
		"201 names":           {Branches: names(MaxBranchNames + 1)},
		"a..b":                {Branches: []string{"a..b"}},
		"empty name":          {Branches: []string{""}},
		"leading dash":        {Branches: []string{"-x"}},
		"space":               {Branches: []string{"a b"}},
		"control":             {Branches: []string{"a\nb"}},
		"trailing slash":      {Branches: []string{"a/"}},
		"trailing dot":        {Branches: []string{"a."}},
		"lock suffix":         {Branches: []string{"a.lock"}},
		"hidden component":    {Branches: []string{"a/.b"}},
		"reflog syntax":       {Branches: []string{"a@{1}"}},
		"glob":                {Branches: []string{"a*"}},
		"256 bytes":           {Branches: []string{strings.Repeat("a", 256)}},
		"bad current":         {Current: "a..b"},
		"bad default":         {Default: "a b"},
		"unknown reason":      {Reason: "boom"},
		"reason with names":   {Reason: BranchesGitFailed, Branches: []string{"main"}},
		"reason with current": {Reason: BranchesGitFailed, Current: "main"},
		"reason with more":    {Reason: BranchesGitFailed, More: true},
		"over reply budget": {Branches: func() []string {
			s := make([]string, MaxBranchNames)
			for i := range s {
				s[i] = strings.Repeat("a", 200)
			}
			return s
		}()},
	}
	for name, r := range invalid {
		if r.valid() {
			t.Error("accepted", name)
		}
	}
	for _, reason := range []BranchesReason{BranchesNotRepoRoot, BranchesOutsideHome, BranchesGitFailed, BranchesConflict} {
		if !(BranchesResult{Reason: reason}).valid() {
			t.Error("refused", reason)
		}
	}
	for _, name := range []string{"main", "feature/x", "release-1.2", "v1.0.0", "a@b", "ünï"} {
		if !validRefName(name) {
			t.Error("refused", name)
		}
	}
}

func TestBranchesOmitsEmptyKeys(t *testing.T) {
	data, _ := json.Marshal(BranchesResult{})
	if bytes.Contains(data, []byte("reason")) || bytes.Contains(data, []byte("more")) {
		t.Fatal(string(data))
	}
}

func TestCapabilitiesBranchesRequiresWorktree(t *testing.T) {
	c := testCapabilities()
	legacy, _ := json.Marshal(c)
	if bytes.Contains(legacy, []byte("branches")) {
		t.Fatal("new field sent to old server", string(legacy))
	}
	c.Branches = true
	if c.Validate() == nil {
		t.Fatal("branches without worktree accepted")
	}
	c.TerminalControl, c.Launch = true, true
	if c.Validate() == nil {
		t.Fatal("branches without worktree accepted")
	}
	c.Worktree = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
