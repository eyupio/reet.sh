package scan

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkSkill(t *testing.T, dir, name string, withFile bool) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if withFile {
		if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func names(f []Found) map[string]string {
	m := map[string]string{}
	for _, x := range f {
		m[x.Name] = x.Source
	}
	return m
}

func TestCollectFromSkillDirs(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	mkSkill(t, filepath.Join(home, ".claude", "skills"), "alpha", true)
	mkSkill(t, filepath.Join(home, ".claude", "skills"), "not-a-skill", false)
	mkSkill(t, filepath.Join(home, ".agents", "skills"), "alpha", true)
	mkSkill(t, filepath.Join(home, ".agents", "skills"), "beta", true)
	mkSkill(t, filepath.Join(cwd, ".claude", "skills"), "gamma", true)
	extra := t.TempDir()
	mkSkill(t, extra, "delta", true)
	got := Collect(home, cwd, []string{extra})
	m := names(got)
	if len(got) != 4 {
		t.Fatalf("want alpha beta gamma delta once each, got %+v", got)
	}
	for _, n := range []string{"alpha", "beta", "gamma", "delta"} {
		if _, ok := m[n]; !ok {
			t.Errorf("missing %s", n)
		}
	}
}

func TestCollectUsesLockFileSource(t *testing.T) {
	home := t.TempDir()
	mkSkill(t, filepath.Join(home, ".agents", "skills"), "find-skills", true)
	lock := `{"version":1,"skills":{"find-skills":{"source":"vercel-labs/skills"},"only-in-lock":{"source":"o/r"}}}`
	if err := os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	m := names(Collect(home, t.TempDir(), nil))
	if m["find-skills"] != "vercel-labs/skills" || m["only-in-lock"] != "o/r" {
		t.Fatalf("%+v", m)
	}
}

func TestCollectToleratesBadLockFile(t *testing.T) {
	home := t.TempDir()
	mkSkill(t, filepath.Join(home, ".claude", "skills"), "alpha", true)
	os.MkdirAll(filepath.Join(home, ".agents"), 0o755)
	os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte("{not json"), 0o644)
	if m := names(Collect(home, t.TempDir(), nil)); len(m) != 1 {
		t.Fatalf("%+v", m)
	}
}

func TestRunPostsItemsWithToken(t *testing.T) {
	var gotTok string
	var body struct {
		Items []struct{ Name, Source string }
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTok = r.Header.Get("X-Reet-Token")
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		w.Write([]byte(`{"imported":1,"errors":[]}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	mkSkill(t, filepath.Join(home, ".claude", "skills"), "alpha", true)
	var out bytes.Buffer
	if err := Run([]string{"--server", srv.URL, "--token", "tok"}, &out); err != nil {
		t.Fatal(err)
	}
	if gotTok != "tok" || len(body.Items) != 1 || body.Items[0].Name != "alpha" {
		t.Fatalf("tok=%q body=%+v", gotTok, body)
	}
	if !strings.Contains(out.String(), "alpha") {
		t.Fatalf("output: %s", out.String())
	}
}

func TestRunDryRunDoesNotPost(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	mkSkill(t, filepath.Join(home, ".claude", "skills"), "alpha", true)
	var out bytes.Buffer
	if err := Run([]string{"--server", srv.URL, "--dry-run"}, &out); err != nil || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestCollectScanEnvironments(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	mkSkill(t, filepath.Join(home, ".claude", "skills"), "alpha", true)
	mkSkill(t, filepath.Join(home, ".cursor", "skills"), "alpha", true)
	mkSkill(t, filepath.Join(home, ".agents", "skills"), "beta", true)
	scan := CollectScan(home, cwd, nil, "laptop")
	if len(scan.Found) != 3 {
		t.Fatalf("found: %+v", scan.Found)
	}
	if len(scan.Envs) != 3 {
		t.Fatalf("envs: %+v", scan.Envs)
	}
	byAgent := map[string]ScannedEnv{}
	for _, env := range scan.Envs {
		byAgent[env.Agent] = env
	}
	if byAgent["claude"].Host != "laptop" || len(byAgent["claude"].Skills) != 1 || byAgent["claude"].Skills[0] != "alpha" {
		t.Fatalf("claude: %+v", byAgent["claude"])
	}
	if len(byAgent["cursor"].Skills) != 1 || len(byAgent["agents"].Skills) != 1 {
		t.Fatalf("envs: %+v", byAgent)
	}
}

func TestRunPostsEnvironmentsAndPrune(t *testing.T) {
	var body struct {
		Items []struct {
			Name  string
			Host  string
			Agent string
		}
		Prune        bool
		Environments []struct {
			Host, Agent string
			Skills      []string
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		w.Write([]byte(`{"imported":1,"created":1,"ignored":0,"pruned":2,"errors":[]}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	mkSkill(t, filepath.Join(home, ".claude", "skills"), "alpha", true)
	var out bytes.Buffer
	if err := Run([]string{"--server", srv.URL, "--env", "laptop", "--prune"}, &out); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].Host != "laptop" || body.Items[0].Agent != "claude" {
		t.Fatalf("items: %+v", body.Items)
	}
	if !body.Prune || len(body.Environments) != 1 || body.Environments[0].Host != "laptop" || len(body.Environments[0].Skills) != 1 {
		t.Fatalf("payload: %+v", body)
	}
	if !strings.Contains(out.String(), "1 new") || !strings.Contains(out.String(), "2 pruned") {
		t.Fatalf("output: %s", out.String())
	}
}

func TestScanKeepsGlobalAndProjectSourcesSeparate(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	mkSkill(t, filepath.Join(home, ".claude", "skills"), "alpha", true)
	mkSkill(t, filepath.Join(cwd, ".claude", "skills"), "alpha", true)
	os.MkdirAll(filepath.Join(home, ".agents"), 0755)
	os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(`{"skills":{"alpha":{"source":"global/repo"}}}`), 0600)
	os.WriteFile(filepath.Join(cwd, "skills-lock.json"), []byte(`{"skills":{"alpha":{"source":"project/repo"}}}`), 0600)
	result := CollectScan(home, cwd, nil, "machine")
	foundGlobal, foundProject := false, false
	for _, found := range result.Found {
		if found.Name != "alpha" {
			continue
		}
		if found.Project == "" {
			foundGlobal = found.Source == "global/repo"
		} else {
			foundProject = found.Source == "project/repo" && filepath.IsAbs(found.Project)
		}
	}
	if !foundGlobal || !foundProject {
		t.Fatalf("sources/scopes mixed: %+v", result.Found)
	}
}

func TestMissingSkillsFolderUnderKnownAgentIsEmpty(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	result := CollectScan(home, t.TempDir(), nil, "machine")
	if len(result.Envs) != 1 || result.Envs[0].Agent != "claude" || len(result.Envs[0].Skills) != 0 {
		t.Fatalf("missing directory not reported empty: %+v", result)
	}
}
