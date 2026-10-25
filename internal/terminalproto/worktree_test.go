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
	c.Worktree = ""
	if c.Validate() != nil {
		t.Fatal("launch create without worktree invalid")
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
	for _, s := range []string{"", ".", "..", "a/b", "a b", "a\x00", "é", strings.Repeat("a", 101), "a;b"} {
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
		WorktreeStatus{Run: testRun, Repo: "reet.sh"},
		WorktreeRemove{Run: testRun, Repo: "reet.sh", Changed: 2, Unpushed: 1},
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
		"status bad run":    WorktreeStatus{Run: "ABC", Repo: "r"},
		"status bad repo":   WorktreeStatus{Run: testRun, Repo: ".."},
		"remove bad run":    WorktreeRemove{Run: "", Repo: "r"},
		"remove bad repo":   WorktreeRemove{Run: testRun, Repo: "a/b"},
		"remove negative":   WorktreeRemove{Run: testRun, Repo: "r", Changed: -1},
		"remove negative u": WorktreeRemove{Run: testRun, Repo: "r", Unpushed: -1},
		"state negative":    WorktreeState{Path: "/a", Branch: "b", Changed: -1},
		"state no path":     WorktreeState{Branch: "b"},
		"state relative":    WorktreeState{Path: "a", Branch: "b"},
		"state control":     WorktreeState{Path: "/a\n", Branch: "b"},
		"state branch ctrl": WorktreeState{Path: "/a", Branch: "b\x1b"},
		"result unknown":    WorktreeResult{Code: "nope"},
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
		"21 entries": append(append([]string{}, twenty...), "f"),
		"256 bytes":  {strings.Repeat("a", 256)},
		"newline":    {"a\nb"},
		"escape":     {"a\x1b[31m"},
		"NUL":        {"a\x00"},
		"C1":         {"a\u0085b"},
		"bad utf8":   {"a\xffb"},
		"empty":      {""},
	} {
		if state(paths).Validate() == nil {
			t.Error("accepted:", name)
		}
	}
}
