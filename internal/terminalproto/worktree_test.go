package terminalproto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const testRun = "0123456789abcdef0123456789abcdef"

func worktreeCreate() Create {
	c := launchCreate()
	c.Worktree = testRun
	c.Project = "reet-sh"
	return c
}

func TestWorktreeFeatureToken(t *testing.T) {
	for _, value := range []string{"", "launch-v1", "worktree-v10", "prefix-worktree-v1"} {
		if SupportsWorktree(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsWorktree("launch-v1, worktree-v1") || WorktreeFeature != "worktree-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestWorktreeCreateRoundTripsStrictly(t *testing.T) {
	encoded, err := Encode(testMessage(worktreeCreate()))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || *decoded.Body.(*Create) != worktreeCreate() {
		t.Fatal(decoded, err)
	}
	if _, err = Decode(bytes.Replace(encoded, []byte(`"worktree":`), []byte(`"branch":"x","worktree":`), 1)); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestPlainAndLaunchCreateOmitWorktreeKey(t *testing.T) {
	for _, c := range []Create{{Title: "Shell", Cols: 80, Rows: 24}, launchCreate()} {
		data, _ := json.Marshal(c)
		if bytes.Contains(data, []byte(`"worktree"`)) {
			t.Fatal("create carries worktree", string(data))
		}
	}
}

func TestWorktreeTokens(t *testing.T) {
	for name, token := range map[string]string{
		"uppercase": strings.ToUpper(testRun),
		"15 chars":  strings.Repeat("a", 15),
		"49 chars":  strings.Repeat("a", 49),
		"non-hex":   strings.Repeat("g", 20),
		"empty":     "",
		"dash":      strings.Repeat("a", 15) + "-",
	} {
		if ValidRunToken(token) {
			t.Error("token accepted:", name)
		}
		if token == "" {
			continue
		}
		c := worktreeCreate()
		c.Worktree = token
		if c.Validate() == nil {
			t.Error("create accepted:", name)
		}
	}
	for _, token := range []string{strings.Repeat("a", 16), strings.Repeat("0", 48), testRun} {
		if !ValidRunToken(token) {
			t.Error("good token refused", token)
		}
	}
	c := worktreeCreate()
	c.Worktree, c.Project = "", ""
	if c.Validate() != nil {
		t.Fatal("launch create without worktree invalid")
	}
}

func TestWorktreeNeedsDir(t *testing.T) {
	c := worktreeCreate()
	c.Dir = ""
	if c.Validate() == nil {
		t.Fatal("worktree without dir accepted")
	}
}

func TestWorktreeNeedsTool(t *testing.T) {
	c := Create{Title: "A", Cols: 80, Rows: 24, Worktree: testRun}
	if c.Validate() == nil {
		t.Fatal("worktree without tool accepted")
	}
}

func TestCreateComparableOnWorktree(t *testing.T) {
	a, b := worktreeCreate(), worktreeCreate()
	if a != b {
		t.Fatal("identical creates differ")
	}
	b.Worktree = strings.Repeat("a", 16)
	if a == b {
		t.Fatal("worktree difference not seen")
	}
}

func TestWorktreeCapability(t *testing.T) {
	c := testCapabilities()
	legacy, _ := json.Marshal(c)
	if bytes.Contains(legacy, []byte("worktree")) {
		t.Fatal("new field sent to old server", string(legacy))
	}
	c.Worktree = true
	if c.Validate() == nil {
		t.Fatal("worktree without launch accepted")
	}
	c.TerminalControl, c.Launch = true, true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidRepoName(t *testing.T) {
	for _, s := range []string{"", ".", "..", "-x", "--help", "-", "a/b", "a b", "a\x00", "é", strings.Repeat("a", 101), "a;b"} {
		if validRepoName(s) {
			t.Error("accepted", s)
		}
	}
	for _, s := range []string{"reet.sh", "a_b-c", ".hidden", "...", strings.Repeat("a", 100)} {
		if !validRepoName(s) {
			t.Error("refused", s)
		}
	}
}

func TestWorktreeMessagesStrict(t *testing.T) {
	bodies := []Body{
		WorktreeStatus{Run: testRun, Project: "my-proj", Repo: "reet.sh"},
		WorktreeRemove{Run: testRun, Project: "my-proj", Repo: "reet.sh", Changed: 2, Unpushed: 1},
		WorktreeState{Path: "/home/a/.reet/worktrees/p/r/" + testRun, Branch: "reet/p-" + testRun, Changed: 1, Unpushed: 0, Paths: []string{"a.txt"}},
		WorktreeState{Path: "/home/a/w", Branch: "b"},
		WorktreeResult{Code: WorktreeChanged},
		WorktreeResult{Code: WorktreeRefused},
	}
	for _, body := range bodies {
		encoded, err := Encode(testMessage(body))
		if err != nil {
			t.Fatalf("%T: %v", body, err)
		}
		if _, err = Decode(encoded); err != nil {
			t.Fatalf("%T: %v", body, err)
		}
		var raw map[string]any
		var env struct{ Body json.RawMessage }
		_ = json.Unmarshal(encoded, &env)
		_ = json.Unmarshal(env.Body, &raw)
		raw["extra"] = 1
		body2, _ := json.Marshal(raw)
		var full map[string]any
		_ = json.Unmarshal(encoded, &full)
		full["body"] = json.RawMessage(body2)
		bad, _ := json.Marshal(full)
		if _, err = Decode(bad); err == nil {
			t.Fatalf("%T accepted an unknown field", body)
		}
	}
	for name, body := range map[string]Body{
		"status bad run":        WorktreeStatus{Run: "ABC", Project: "p", Repo: "r"},
		"status bad repo":       WorktreeStatus{Run: testRun, Project: "p", Repo: ".."},
		"remove bad run":        WorktreeRemove{Run: "", Project: "p", Repo: "r"},
		"remove bad repo":       WorktreeRemove{Run: testRun, Project: "p", Repo: "a/b"},
		"remove negative":       WorktreeRemove{Run: testRun, Project: "p", Repo: "r", Changed: -1},
		"remove negative u":     WorktreeRemove{Run: testRun, Project: "p", Repo: "r", Unpushed: -1},
		"state negative":        WorktreeState{Path: "/a", Branch: "b", Changed: -1},
		"state no path":         WorktreeState{Branch: "b"},
		"state relative":        WorktreeState{Path: "a", Branch: "b"},
		"state control":         WorktreeState{Path: "/a\n", Branch: "b"},
		"state branch ctrl":     WorktreeState{Path: "/a", Branch: "b\x1b"},
		"status no project":     WorktreeStatus{Run: testRun, Repo: "r"},
		"status bad project":    WorktreeStatus{Run: testRun, Project: "My Proj", Repo: "r"},
		"status dot project":    WorktreeStatus{Run: testRun, Project: "..", Repo: "r"},
		"remove no project":     WorktreeRemove{Run: testRun, Repo: "r"},
		"remove bad project":    WorktreeRemove{Run: testRun, Project: "a/b", Repo: "r"},
		"remove long project":   WorktreeRemove{Run: testRun, Project: strings.Repeat("a", 41), Repo: "r"},
		"status dash repo":      WorktreeStatus{Run: testRun, Project: "p", Repo: "--help"},
		"remove dash repo":      WorktreeRemove{Run: testRun, Project: "p", Repo: "-x"},
		"status count":          WorktreeRemove{Run: testRun, Project: "p", Repo: "r", Changed: 1<<30 + 1},
		"remove unpushed count": WorktreeRemove{Run: testRun, Project: "p", Repo: "r", Unpushed: 1<<30 + 1},
		"state changed count":   WorktreeState{Path: "/a", Branch: "b", Changed: 1<<30 + 1},
		"state unpushed count":  WorktreeState{Path: "/a", Branch: "b", Unpushed: 1<<30 + 1},
		"state empty branch":    WorktreeState{Path: "/a", Branch: ""},
		"result unknown":        WorktreeResult{Code: "nope"},
	} {
		if testMessage(body).Validate() == nil {
			t.Error("accepted:", name)
		}
	}
	if (WorktreeChanged != "worktree_changed") || (WorktreeRefused != "worktree_refused") || !WorktreeChanged.Valid() || !WorktreeRefused.Valid() || ResultCode("worktree_other").Valid() {
		t.Fatal("result codes")
	}
}

func TestWorktreePaths(t *testing.T) {
	state := func(paths []string) Message {
		return testMessage(WorktreeState{Path: "/a", Branch: "b", Paths: paths})
	}
	twenty := make([]string, 20)
	for i := range twenty {
		twenty[i] = "f"
	}
	if state(twenty).Validate() != nil || state([]string{strings.Repeat("a", 255)}).Validate() != nil || state([]string{"é.txt"}).Validate() != nil {
		t.Fatal("good paths refused")
	}
	for name, paths := range map[string][]string{
		"21 entries":    append(append([]string{}, twenty...), "f"),
		"256 bytes":     {strings.Repeat("a", 256)},
		"newline":       {"a\nb"},
		"escape":        {"a\x1b[31m"},
		"NUL":           {"a\x00"},
		"C1":            {"a\u0085b"},
		"bad utf8":      {"a\xffb"},
		"empty":         {""},
		"bidi override": {"a\u202eb"},
		"zero width":    {"a\u200bb"},
		"isolate":       {"a\u2066b"},
		"isolate end":   {"a\u2069b"},
		"BOM":           {"\ufeffa"},
		"line sep":      {"a\u2028b"},
	} {
		if state(paths).Validate() == nil {
			t.Error("accepted:", name)
		}
	}
}

func TestWorktreeProjectField(t *testing.T) {
	c := worktreeCreate()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Project = ""
	if c.Validate() == nil {
		t.Fatal("worktree without project accepted")
	}
	for _, bad := range []string{"Reet", "reet.sh", "a/b", "-a", "a-", "a--b", "..", strings.Repeat("a", MaxSlugBytes+1), "é", "a b"} {
		c.Project = bad
		if c.Validate() == nil {
			t.Error("bad project slug accepted", bad)
		}
	}
	plain := launchCreate()
	plain.Project = "reet-sh"
	if plain.Validate() == nil {
		t.Fatal("project without worktree accepted")
	}
	for _, c := range []Create{{Title: "Shell", Cols: 80, Rows: 24}, launchCreate()} {
		data, _ := json.Marshal(c)
		if bytes.Contains(data, []byte(`"project"`)) {
			t.Fatal("create carries project", string(data))
		}
	}
	a, b := worktreeCreate(), worktreeCreate()
	b.Project = "other"
	if a == b {
		t.Fatal("project difference not seen")
	}
	encoded, err := Encode(testMessage(worktreeCreate()))
	if err != nil || !bytes.Contains(encoded, []byte(`"project":"reet-sh"`)) {
		t.Fatal(string(encoded), err)
	}
}

func TestSlug(t *testing.T) {
	long := strings.Repeat("abcdefghij", 10)
	cases := map[string]struct{ name, fallback, want string }{
		"plain":       {"reet", "project", "reet"},
		"upper":       {"Reet.SH", "project", "reet-sh"},
		"dots":        {"..", "repo", ""},
		"dot segment": {"../etc", "repo", "etc"},
		"separators":  {"a/b\\c", "repo", "a-b-c"},
		"spaces":      {"  My   Project  ", "project", "my-project"},
		"unicode mix": {"Café Ünïcode", "project", "caf-n-code"},
		"empty":       {"", "project", "project"},
		"hyphens":     {"--a--b--", "repo", "a-b"},
		"underscore":  {"my_repo", "repo", "my-repo"},
	}
	for name, tc := range cases {
		got := Slug(tc.name, tc.fallback)
		if tc.want != "" && got != tc.want {
			t.Errorf("%s: Slug(%q) = %q, want %q", name, tc.name, got, tc.want)
		}
		if !ValidSlug(got) {
			t.Errorf("%s: Slug(%q) = %q is not a valid slug", name, tc.name, got)
		}
	}
	// Names with nothing usable fall back to the fallback plus a hash of the
	// name, so two different unicode names do not share a folder.
	a, b := Slug("日本", "repo"), Slug("中国", "repo")
	if !strings.HasPrefix(a, "repo-") || a == b || !ValidSlug(a) || !ValidSlug(b) {
		t.Fatal("unicode fallback", a, b)
	}
	if Slug("..", "repo") != Slug("..", "repo") || !strings.HasPrefix(Slug("..", "repo"), "repo-") {
		t.Fatal("fallback not deterministic")
	}
	// Long names are bounded and keep a hash so they stay distinct.
	x, y := Slug(long+"x", "repo"), Slug(long+"y", "repo")
	if len(x) > MaxSlugBytes || x == y || !ValidSlug(x) || !strings.HasPrefix(x, "abcdefghij") {
		t.Fatal("long names", x, y)
	}
	exact := strings.Repeat("a", MaxSlugBytes)
	if Slug(exact, "repo") != exact {
		t.Fatal("a name at the limit changed", Slug(exact, "repo"))
	}
	for _, s := range []string{"a", "a-b", "0", strings.Repeat("z", MaxSlugBytes)} {
		if !ValidSlug(s) {
			t.Error("valid slug refused", s)
		}
	}
}

func TestResultWorktreeReason(t *testing.T) {
	good := []Result{
		{Code: WorktreeRefused, Reason: WorktreeNotRepoRoot},
		{Code: WorktreeRefused, Reason: WorktreeRepoBusy},
		{Code: WorktreeRefused, Reason: WorktreeExists},
		{Code: WorktreeRefused, Reason: WorktreeGitFailed, Detail: "fatal: invalid reference: HEAD"},
		{Code: WorktreeRefused, Reason: WorktreeOutsideHome},
		{Code: WorktreeRefused},
	}
	for _, r := range good {
		encoded, err := Encode(testMessage(r))
		if err != nil {
			t.Fatal(r, err)
		}
		decoded, err := Decode(encoded)
		if err != nil || decoded.Body.(*Result).Reason != r.Reason || decoded.Body.(*Result).Detail != r.Detail {
			t.Fatal(r, decoded, err)
		}
	}
	for name, r := range map[string]Result{
		"reason with other code": {Code: LaunchFailed, Reason: WorktreeExists},
		"unknown reason":         {Code: WorktreeRefused, Reason: "nope"},
		"detail without reason":  {Code: WorktreeRefused, Detail: "x"},
		"detail too long":        {Code: WorktreeRefused, Reason: WorktreeGitFailed, Detail: strings.Repeat("a", MaxWorktreeDetailBytes+1)},
		"detail control":         {Code: WorktreeRefused, Reason: WorktreeGitFailed, Detail: "a\nb"},
		"detail escape":          {Code: WorktreeRefused, Reason: WorktreeGitFailed, Detail: "a\x1b[2J"},
	} {
		if _, err := Encode(testMessage(r)); err == nil {
			t.Error("invalid result accepted:", name)
		}
	}
	data, _ := json.Marshal(Result{Code: OK})
	if string(data) != `{"code":"ok"}` {
		t.Fatal("plain result changed", string(data))
	}
}

func TestWorktreeResultCarriesReason(t *testing.T) {
	good := []WorktreeResult{
		{Code: OK},
		{Code: NotFound},
		{Code: WorktreeChanged},
		{Code: WorktreeRefused, Reason: WorktreeOutsideHome},
		{Code: WorktreeRefused, Reason: WorktreeNotRegistered},
		{Code: WorktreeRefused, Reason: WorktreeHasChanges},
		{Code: WorktreeRefused, Reason: WorktreeGitFailed, Detail: "fatal: 'x' is locked"},
	}
	for _, r := range good {
		encoded, err := Encode(Message{Version: Version, RequestID: "request-0123456789", Generation: 1, Body: r})
		if err != nil {
			t.Fatal(r, err)
		}
		decoded, err := Decode(encoded)
		if err != nil || *decoded.Body.(*WorktreeResult) != r {
			t.Fatal(r, decoded, err)
		}
	}
	for name, r := range map[string]WorktreeResult{
		"reason with ok":        {Code: OK, Reason: WorktreeHasChanges},
		"reason with changed":   {Code: WorktreeChanged, Reason: WorktreeHasChanges},
		"unknown reason":        {Code: WorktreeRefused, Reason: "nope"},
		"detail without reason": {Code: WorktreeRefused, Detail: "x"},
		"detail control":        {Code: WorktreeRefused, Reason: WorktreeGitFailed, Detail: "a\nb"},
		"detail bidi":           {Code: WorktreeRefused, Reason: WorktreeGitFailed, Detail: "a‮b"},
		"detail too long":       {Code: WorktreeRefused, Reason: WorktreeGitFailed, Detail: strings.Repeat("a", MaxWorktreeDetailBytes+1)},
	} {
		if _, err := Encode(Message{Version: Version, RequestID: "request-0123456789", Generation: 1, Body: r}); err == nil {
			t.Error("invalid worktree result accepted:", name)
		}
	}
	data, _ := json.Marshal(WorktreeResult{Code: NotFound})
	if string(data) != `{"code":"not_found"}` {
		t.Fatal("plain worktree result changed", string(data))
	}
}

func TestWorktreeStateReportsWhatBlocksRemoval(t *testing.T) {
	msg := func(b Body) Message {
		return Message{Version: Version, RequestID: "request-0123456789", Generation: 1, Body: b}
	}
	full := WorktreeState{Path: "/a", Branch: "b", Unreferenced: 2, Hidden: 3, Nested: true, Incomplete: true}
	encoded, err := Encode(msg(full))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Body.(*WorktreeState); got.Unreferenced != 2 || got.Hidden != 3 || !got.Nested || !got.Incomplete {
		t.Fatal(got)
	}
	data, _ := json.Marshal(WorktreeState{Path: "/a", Branch: "b"})
	if string(data) != `{"path":"/a","branch":"b","changed":0,"unpushed":0}` {
		t.Fatal("a clean state gained fields", string(data))
	}
	for name, s := range map[string]WorktreeState{
		"negative unreferenced": {Path: "/a", Branch: "b", Unreferenced: -1},
		"huge unreferenced":     {Path: "/a", Branch: "b", Unreferenced: 1<<30 + 1},
		"negative hidden":       {Path: "/a", Branch: "b", Hidden: -1},
		"huge hidden":           {Path: "/a", Branch: "b", Hidden: 1<<30 + 1},
	} {
		if _, err := Encode(msg(s)); err == nil {
			t.Error("invalid state accepted:", name)
		}
	}
	for _, r := range []WorktreeReason{WorktreeUnreferencedCommits, WorktreeNestedRepository, WorktreeHiddenChanges, WorktreeCheckIncomplete} {
		if !r.Valid() {
			t.Fatal(r)
		}
		if _, err := Encode(msg(WorktreeResult{Code: WorktreeRefused, Reason: r})); err != nil {
			t.Fatal(r, err)
		}
	}
}
