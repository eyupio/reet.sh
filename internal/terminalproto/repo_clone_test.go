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
		"ssh rewrite":         {RepoCloneResult{Reason: RepoCloneSSHRewrite}, true},
		"ssh rewrite and dir": {RepoCloneResult{Reason: RepoCloneSSHRewrite, Dir: "/home/u/x"}, false},
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
	for _, reason := range []RepoCloneReason{RepoCloneNotSignedIn, RepoCloneNoAccess, RepoCloneExists, RepoCloneTimedOut, RepoCloneFailed, RepoCloneDenied, RepoCloneBusy, RepoCloneSSHRewrite} {
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

const testFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRepoCloneRemoveFeatureToken(t *testing.T) {
	for _, value := range []string{"", "repo-clone-v1", "repo-clone-remove-v10", "prefix-repo-clone-remove-v1"} {
		if SupportsRepoCloneRemove(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsRepoCloneRemove("repo-clone-v1, repo-clone-remove-v1") || RepoCloneRemoveFeature != "repo-clone-remove-v1" {
		t.Fatal("feature not recognized")
	}
	if SupportsRepoClone(RepoCloneRemoveFeature) {
		t.Fatal("the remove token turned cloning on")
	}
}

func TestRepoCloneStatusAndRemoveValid(t *testing.T) {
	for name, c := range map[string]struct {
		dir, origin string
		want        bool
	}{
		"ok":               {"/home/u/reet/repos/eyupio/reet.sh", "eyupio/reet.sh", true},
		"relative dir":     {"reet/repos/a/b", "a/b", false},
		"empty dir":        {"", "a/b", false},
		"control in dir":   {"/home/u/a\n", "a/b", false},
		"empty origin":     {"/home/u/a", "", false},
		"no slash":         {"/home/u/a", "ab", false},
		"bad owner":        {"/home/u/a", "-a/b", false},
		"bad repo":         {"/home/u/a", "a/..", false},
		"git suffix":       {"/home/u/a", "a/b.git", false},
		"url as an origin": {"/home/u/a", "https://github.com/a/b", false},
		"three segments":   {"/home/u/a", "a/b/c", false},
	} {
		if got := (RepoCloneStatus{Dir: c.dir, Origin: c.origin}).valid(); got != c.want {
			t.Errorf("status %s: valid() = %v, want %v", name, got, c.want)
		}
		if got := (RepoCloneRemove{Dir: c.dir, Origin: c.origin, Fingerprint: testFingerprint}).valid(); got != c.want {
			t.Errorf("remove %s: valid() = %v, want %v", name, got, c.want)
		}
	}
	for _, fingerprint := range []string{"", "abc", testFingerprint[:63], testFingerprint + "0", strings.ToUpper(testFingerprint), testFingerprint[:63] + "g"} {
		if (RepoCloneRemove{Dir: "/home/u/a", Origin: "a/b", Fingerprint: fingerprint}).valid() {
			t.Errorf("remove accepted the fingerprint %q", fingerprint)
		}
	}
}

func TestRepoCloneStateValidAndRemovable(t *testing.T) {
	clean := RepoCloneState{Marked: true, Fingerprint: testFingerprint}
	cases := map[string]struct {
		s               RepoCloneState
		valid, removeOK bool
	}{
		"clean":                     {clean, true, true},
		"not marked":                {RepoCloneState{}, true, false},
		"worktree":                  {RepoCloneState{Marked: true, Worktrees: 1, Fingerprint: testFingerprint}, true, false},
		"changed":                   {RepoCloneState{Marked: true, Changed: 2, Fingerprint: testFingerprint}, true, false},
		"stash":                     {RepoCloneState{Marked: true, Stashes: 1, Fingerprint: testFingerprint}, true, false},
		"unpushed":                  {RepoCloneState{Marked: true, Unpushed: 3, Fingerprint: testFingerprint}, true, false},
		"marked, no fingerprint":    {RepoCloneState{Marked: true}, false, true},
		"bad fingerprint":           {RepoCloneState{Marked: true, Fingerprint: "xyz"}, false, true},
		"negative count":            {RepoCloneState{Marked: true, Changed: -1, Fingerprint: testFingerprint}, false, false},
		"huge count":                {RepoCloneState{Marked: true, Unpushed: maxRepoCloneCount + 1, Fingerprint: testFingerprint}, false, false},
		"unmarked with a count":     {RepoCloneState{Changed: 1}, false, false},
		"unmarked, fingerprint":     {RepoCloneState{Fingerprint: testFingerprint}, false, false},
		"reason with marked":        {RepoCloneState{Reason: RepoCloneFailed, Marked: true}, false, false},
		"reason with a count":       {RepoCloneState{Reason: RepoCloneFailed, Stashes: 1}, false, false},
		"reason with a fingerprint": {RepoCloneState{Reason: RepoCloneFailed, Fingerprint: testFingerprint}, false, false},
		"unknown reason":            {RepoCloneState{Reason: "boom"}, false, false},
		"changed is not a state":    {RepoCloneState{Reason: RepoCloneChanged}, false, false},
	}
	for _, reason := range []RepoCloneReason{RepoCloneDenied, RepoCloneBusy, RepoCloneTimedOut, RepoCloneFailed, RepoCloneExists, RepoCloneNotRemovable} {
		cases["reason "+string(reason)] = struct {
			s               RepoCloneState
			valid, removeOK bool
		}{RepoCloneState{Reason: reason}, true, false}
	}
	// Reasons of a clone, and of a remove alone, are not a state's.
	for _, reason := range []RepoCloneReason{RepoCloneNotSignedIn, RepoCloneNoAccess, RepoCloneSSHRewrite, RepoCloneChanged} {
		cases["reason "+string(reason)] = struct {
			s               RepoCloneState
			valid, removeOK bool
		}{RepoCloneState{Reason: reason}, false, false}
	}
	for name, c := range cases {
		if got := c.s.valid(); got != c.valid {
			t.Errorf("%s: valid() = %v, want %v", name, got, c.valid)
		}
		if got := c.s.Removable(); got != c.removeOK {
			t.Errorf("%s: Removable() = %v, want %v", name, got, c.removeOK)
		}
	}
}

func TestRepoCloneRemovedValid(t *testing.T) {
	cases := map[string]struct {
		r    RepoCloneRemoved
		want bool
	}{
		"removed":        {RepoCloneRemoved{Dir: "/home/u/reet/repos/a/b"}, true},
		"empty":          {RepoCloneRemoved{}, false},
		"relative dir":   {RepoCloneRemoved{Dir: "a/b"}, false},
		"dir and reason": {RepoCloneRemoved{Dir: "/home/u/a", Reason: RepoCloneChanged}, false},
		"unknown reason": {RepoCloneRemoved{Reason: "boom"}, false},
	}
	for _, reason := range []RepoCloneReason{RepoCloneChanged, RepoCloneNotRemovable, RepoCloneDenied, RepoCloneBusy, RepoCloneTimedOut, RepoCloneFailed, RepoCloneExists} {
		cases["reason "+string(reason)] = struct {
			r    RepoCloneRemoved
			want bool
		}{RepoCloneRemoved{Reason: reason}, true}
	}
	// Reasons of a clone are not a remove's.
	for _, reason := range []RepoCloneReason{RepoCloneNotSignedIn, RepoCloneNoAccess, RepoCloneSSHRewrite} {
		cases["reason "+string(reason)] = struct {
			r    RepoCloneRemoved
			want bool
		}{RepoCloneRemoved{Reason: reason}, false}
	}
	for name, c := range cases {
		if got := c.r.valid(); got != c.want {
			t.Errorf("%s: valid() = %v, want %v", name, got, c.want)
		}
	}
}

// The two reasons a remove adds are never an answer to a clone.
func TestRepoCloneResultRefusesTheRemoveReasons(t *testing.T) {
	for _, reason := range []RepoCloneReason{RepoCloneChanged, RepoCloneNotRemovable} {
		if !reason.valid() {
			t.Errorf("%s is not a reason", reason)
		}
		if (RepoCloneResult{Reason: reason}).valid() {
			t.Errorf("a clone result carried %s", reason)
		}
	}
	if RepoCloneChanged != "changed" || RepoCloneNotRemovable != "not_removable" {
		t.Fatal(RepoCloneChanged, RepoCloneNotRemovable)
	}
}

func TestRepoCloneRemoveRoundTripAndLimits(t *testing.T) {
	dir := "/home/u/reet/repos/eyupio/reet.sh"
	for _, body := range []Body{
		RepoCloneStatus{Dir: dir, Origin: "eyupio/reet.sh"},
		RepoCloneState{Marked: true, Worktrees: 1, Changed: 2, Stashes: 3, Unpushed: 4, Fingerprint: testFingerprint},
		RepoCloneState{},
		RepoCloneState{Reason: RepoCloneNotRemovable},
		RepoCloneRemove{Dir: dir, Origin: "eyupio/reet.sh", Fingerprint: testFingerprint},
		RepoCloneRemoved{Dir: dir},
		RepoCloneRemoved{Reason: RepoCloneChanged},
		RepoCloneRemoved{Reason: RepoCloneNotRemovable},
	} {
		encoded, err := Encode(testMessage(body))
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		if decoded.Body.kind() != body.kind() {
			t.Fatalf("%s decoded as %s", body.kind(), decoded.Body.kind())
		}
		again, err := Encode(decoded)
		if err != nil || !bytes.Equal(again, encoded) {
			t.Fatalf("%s did not round-trip: %s / %s (%v)", body.kind(), encoded, again, err)
		}
	}
	for kind, body := range map[string]Body{
		"repo_clone_status":  RepoCloneStatus{Dir: dir, Origin: "a/b"},
		"repo_clone_state":   RepoCloneState{Reason: RepoCloneFailed},
		"repo_clone_remove":  RepoCloneRemove{Dir: dir, Origin: "a/b", Fingerprint: testFingerprint},
		"repo_clone_removed": RepoCloneRemoved{Reason: RepoCloneFailed},
	} {
		if body.kind() != kind {
			t.Fatalf("kind %s, want %s", body.kind(), kind)
		}
		encoded, err := Encode(testMessage(body))
		if err != nil {
			t.Fatal(kind, err)
		}
		if _, err := Decode(bytes.Replace(encoded, []byte(`"body":{`), []byte(`"body":{"force":true,`), 1)); err == nil {
			t.Fatalf("%s: unknown field accepted", kind)
		}
	}
	for _, body := range []Body{
		RepoCloneStatus{Dir: dir, Origin: "a"},
		RepoCloneState{Marked: true},
		RepoCloneRemove{Dir: dir, Origin: "a/b"},
		RepoCloneRemoved{},
	} {
		if _, err := Encode(testMessage(body)); err == nil {
			t.Fatalf("invalid %s encoded", body.kind())
		}
	}
}

func TestRepoCloneRemoveCapability(t *testing.T) {
	feats := WorktreeFeature + "," + RepoCloneRemoveFeature
	ok := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "u", TerminalEnabled: true, Readiness: Ready, TerminalControl: true, Launch: true, Worktree: true, RepoCloneRemove: true}
	if err := ok.ValidateFor(feats); err != nil {
		t.Fatal(err)
	}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ok.ValidateFor(WorktreeFeature + "," + RepoCloneFeature); err == nil {
		t.Fatal("repoCloneRemove accepted without the negotiated feature")
	}
	if err := (Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "u", TerminalEnabled: true, Readiness: Ready, TerminalControl: true, Launch: true, RepoCloneRemove: true}).ValidateFor(feats); err == nil {
		t.Fatal("repoCloneRemove accepted without worktree")
	}
}
