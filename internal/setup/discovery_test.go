package setup

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) environment {
	t.Helper()
	return environment{
		home:     t.TempDir(),
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		lstat:    os.Lstat,
	}
}

func findAgent(t *testing.T, r Report, id string) Agent {
	t.Helper()
	for _, agent := range r.Agents {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("agent %q not reported", id)
	return Agent{}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyHomeReportsAllAdaptersWithoutClaimingRestoreSupport(t *testing.T) {
	r, err := discover(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.SchemaVersion != 1 || r.CapturedAt.IsZero() || r.OS == "" || r.Architecture == "" {
		t.Fatalf("missing inventory identity: %+v", r)
	}
	for _, id := range []string{"claude", "codex", "opencode", "pi", "cursor", "gemini"} {
		a := findAgent(t, r, id)
		if a.State != "not_detected" || a.Executable.State != "missing" || a.Capabilities.Restore != "not_implemented" || a.Capabilities.Backup != "not_implemented" {
			t.Fatalf("unexpected capability or observation: %+v", a)
		}
		for _, loc := range a.Locations {
			if loc.Scope != "global" || loc.State != "missing" {
				t.Fatalf("unexpected location: %+v", loc)
			}
		}
	}
}

func TestConfigAndExecutableAreSeparateEvidence(t *testing.T) {
	e := fixture(t)
	mkdir(t, filepath.Join(e.home, ".claude"))
	e.lookPath = func(name string) (string, error) {
		if name == "codex" {
			return filepath.Join(e.home, "bin", "codex"), nil
		}
		return "", exec.ErrNotFound
	}
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	claude, codex := findAgent(t, r, "claude"), findAgent(t, r, "codex")
	if claude.State != "detected" || claude.Executable.State != "missing" {
		t.Fatalf("configuration incorrectly implies executable: %+v", claude)
	}
	if codex.State != "detected" || codex.Executable.State != "present" || codex.Locations[0].State != "missing" {
		t.Fatalf("binary incorrectly implies configuration: %+v", codex)
	}
}

func TestSharedInstructionsAndSkillsDoNotIdentifyAnAgent(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	mkdir(t, filepath.Join(e.home, ".agents", "skills"))
	if err := os.WriteFile(filepath.Join(e.project, "AGENTS.md"), []byte("Shared instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	if a := findAgent(t, r, "codex"); a.State != "not_detected" {
		t.Fatalf("shared files falsely identify Codex: %+v", a)
	}
}

func TestOverridesAndExplicitProject(t *testing.T) {
	e := fixture(t)
	e.project = t.TempDir()
	overrides := map[string]string{}
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "XDG_CONFIG_HOME", "PI_CODING_AGENT_DIR", "OPENCODE_CONFIG_DIR"} {
		overrides[key] = t.TempDir()
	}
	mkdir(t, filepath.Join(overrides["XDG_CONFIG_HOME"], "opencode"))
	overrides["OPENCODE_CONFIG"] = filepath.Join(t.TempDir(), "custom.json")
	if err := os.WriteFile(overrides["OPENCODE_CONFIG"], []byte("not parsed"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.getenv = func(key string) string { return overrides[key] }
	mkdir(t, filepath.Join(e.project, ".cursor"))
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, a := range r.Agents {
		for _, loc := range a.Locations {
			if _, ok := overrides[loc.Source]; ok {
				seen[loc.Source] = true
				if loc.State != "present" || !strings.HasPrefix(loc.Path, overrides[loc.Source]) {
					t.Fatalf("override not honoured: %+v", loc)
				}
			}
		}
	}
	if len(seen) != len(overrides) || findAgent(t, r, "cursor").State != "detected" || r.Project != e.project {
		t.Fatalf("missing scope/overrides: %+v", r)
	}
}

func TestInvalidOverrideDoesNotFallBackToDefault(t *testing.T) {
	e := fixture(t)
	mkdir(t, filepath.Join(e.home, ".codex"))
	e.getenv = func(key string) string {
		if key == "CODEX_HOME" {
			return "relative/config"
		}
		return ""
	}
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	a := findAgent(t, r, "codex")
	if a.State != "unknown" || a.Locations[0].Issue != "override_must_be_absolute" || a.Locations[0].Path != "" {
		t.Fatalf("invalid override hidden: %+v", a)
	}
}

func TestUnreadableMetadataAndUnexpectedTypeAreNotMissing(t *testing.T) {
	e := fixture(t)
	e.lstat = func(path string) (os.FileInfo, error) {
		if path == filepath.Join(e.home, ".codex") {
			return nil, &os.PathError{Op: "lstat", Path: path, Err: fs.ErrPermission}
		}
		return os.Lstat(path)
	}
	if err := os.WriteFile(filepath.Join(e.home, ".claude"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	if a := findAgent(t, r, "codex"); a.State != "unknown" || a.Locations[0].State != "error" {
		t.Fatalf("permission error hidden: %+v", a)
	}
	if a := findAgent(t, r, "claude"); a.State != "unknown" || a.Locations[0].State != "unsupported" {
		t.Fatalf("unexpected type hidden: %+v", a)
	}
}

func TestSymlinkIsReportedWithoutFollowingTarget(t *testing.T) {
	e := fixture(t)
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(e.home, ".codex")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	if a := findAgent(t, r, "codex"); a.State != "unknown" || a.Locations[0].State != "symlink" {
		t.Fatalf("symlink incorrectly inspected: %+v", a)
	}
}

func TestExecutableLookupErrorIsNotAbsence(t *testing.T) {
	e := fixture(t)
	e.lookPath = func(string) (string, error) { return "", exec.ErrDot }
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	if a := findAgent(t, r, "pi"); a.State != "unknown" || a.Executable.State != "error" {
		t.Fatalf("lookup error hidden: %+v", a)
	}
}

func TestDiscoveryDoesNotReadConfigurationContents(t *testing.T) {
	e := fixture(t)
	const secret = "SYNTHETIC-SECRET-MUST-NOT-APPEAR"
	path := filepath.Join(e.home, ".claude.json")
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil || strings.Contains(string(data), secret) {
		t.Fatalf("report contains content: %v", err)
	}
	if a := findAgent(t, r, "claude"); a.Locations[1].State != "present" {
		t.Fatalf("configuration file not observed: %+v", a)
	}
}

func TestProjectValidationAndNoImplicitProject(t *testing.T) {
	e := fixture(t)
	for _, project := range []string{"relative", filepath.Join(e.home, "missing")} {
		e.project = project
		if _, err := discover(e); err == nil {
			t.Fatalf("accepted invalid project %q", project)
		}
	}
	e.project = ""
	e.home = "relative"
	if _, err := discover(e); err == nil {
		t.Fatal("accepted relative home")
	}
}

func TestMetadataOnlyChecksRegisteredLocations(t *testing.T) {
	e := fixture(t)
	mkdir(t, filepath.Join(e.home, ".pi", "agent", "sessions", "nested"))
	e.lstat = func(path string) (os.FileInfo, error) {
		if strings.Contains(path, "sessions") {
			t.Fatalf("must not inspect history: %s", path)
		}
		return os.Lstat(path)
	}
	r, err := discover(e)
	if err != nil || findAgent(t, r, "pi").State != "detected" {
		t.Fatalf("unexpected scan: %v", err)
	}
}
