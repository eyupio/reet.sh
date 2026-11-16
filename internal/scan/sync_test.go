package scan

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/noderuntime"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestChooseSyncMode(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		apply       bool
	}{
		{name: "apply", input: "apply\n", apply: true},
		{name: "apply shorthand", input: "a\n", apply: true},
		{name: "apply number", input: "2\n", apply: true},
		{name: "uppercase apply", input: "APPLY\n", apply: true},
		{name: "preview", input: "preview\n"},
		{name: "preview shorthand", input: "p\n"},
		{name: "preview number", input: "1\n"},
		{name: "empty answers preview", input: "\n"},
		{name: "unknown then preview", input: "later\npreview\n"},
		{name: "unknown then apply", input: "yes please\napply\n", apply: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			apply, err := chooseSyncMode(strings.NewReader(tc.input), &out)
			if err != nil {
				t.Fatal(err)
			}
			if apply != tc.apply {
				t.Fatalf("apply = %t, want %t: %s", apply, tc.apply, out.String())
			}
			if strings.Contains(tc.input, "later") && !strings.Contains(out.String(), "Enter preview or apply.") {
				t.Fatalf("unknown answer was not explained: %s", out.String())
			}
		})
	}
}

func TestRequireNpxOnlyForInstalls(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	install := []SyncAction{{Add: true, Skill: store.SyncSkill{SourceType: "github"}}}
	err := requireNpx(install)
	if err == nil || !strings.Contains(err.Error(), "Node.js") {
		t.Fatalf("missing npx = %v", err)
	}
	if err := requireNpx([]SyncAction{{Add: true, Skill: store.SyncSkill{SourceType: "private"}}}); err != nil {
		t.Fatalf("private install should not need npx: %v", err)
	}
	if err := requireNpx([]SyncAction{{Add: false, Skill: store.SyncSkill{SourceType: "github"}}}); err != nil {
		t.Fatalf("removal should not need npx: %v", err)
	}
	bin := t.TempDir()
	for _, name := range []string{"npx", "node"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	if err := requireNpx(install); err != nil {
		t.Fatalf("present npx = %v", err)
	}
	managed := filepath.Join(noderuntime.Dir(home), "bin")
	if err := os.MkdirAll(managed, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"npx", "node"} {
		if err := os.WriteFile(filepath.Join(managed, name), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if err := requireNpx(install); err != nil {
		t.Fatalf("managed runtime = %v", err)
	}
}

func TestSyncPreviewConfirmationAndApply(t *testing.T) {
	for _, tc := range []struct {
		name, input, problem         string
		apply, fail, changed, locked bool
	}{
		{name: "preview"},
		{name: "confirmation declined", apply: true, input: "no\n", problem: "cancelled"},
		{name: "apply install and removal", apply: true, input: "Laptop Claude\n"},
		{name: "apply accepts yes", apply: true, input: "yes\n"},
		{name: "command fails and reports inventory", apply: true, input: "Laptop Claude\n", fail: true, problem: "sync stopped"},
		{name: "controller changed", apply: true, input: "Laptop Claude\n", changed: true, problem: "controller target changed"},
		{name: "concurrent local sync", apply: true, input: "Laptop Claude\n", locked: true, problem: "cannot lock target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, ".claude", "skills")
			mkSkill(t, dir, "extra", true)
			lockDir := filepath.Join(home, ".agents")
			os.MkdirAll(lockDir, 0755)
			writeLock := func() {
				t.Helper()
				if err := os.WriteFile(filepath.Join(lockDir, ".skill-lock.json"), []byte(`{"skills":{"extra":{"source":"o/r"},"beta":{"source":"o/r"}}}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			writeLock()
			target := store.SyncTarget{Target: store.Environment{ID: "1/laptop/claude", Name: "Laptop Claude", Host: "laptop", Agent: "claude"}, SourceName: "Reference", Revision: "one", Desired: []store.SyncSkill{{ID: "o/r/beta", Name: "beta", Slug: "beta", Source: "o/r", SourceType: "github"}}}
			gets, posts, executed := 0, 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Reet-Token") != "secret" {
					t.Error("missing account token")
				}
				if r.Method == "GET" {
					gets++
					current := target
					if tc.changed && gets > 1 {
						current.Revision = "two"
					}
					json.NewEncoder(w).Encode(map[string]any{"targets": []store.SyncTarget{current}, "canWrite": true})
					return
				}
				posts++
				var body struct{ Machine store.MachineInfo }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Machine.Version != "vtest" {
					t.Error("scan lost binary version")
				}
				w.Write([]byte(`{"imported":1,"errors":[]}`))
			}))
			defer srv.Close()
			if tc.locked {
				os.WriteFile(filepath.Join(dir, ".reet-sync.lock"), nil, 0600)
			}
			args := []string{"--server", srv.URL, "--token", "secret", "--env", "laptop", "--target", target.Target.ID}
			if tc.apply {
				args = append(args, "--apply")
			}
			var out bytes.Buffer
			err := runSync(args, strings.NewReader(tc.input), &out, func(args []string, cwd string) error {
				executed++
				if tc.fail {
					return errors.New("fixture failure")
				}
				if !strings.Contains(strings.Join(args, " "), "--agent claude-code --yes --global") {
					t.Errorf("incorrect target scope: %v", args)
				}
				if args[1] != "skills@1.7.4" {
					t.Errorf("executor version is not pinned: %v", args)
				}
				if args[2] == "add" {
					mkSkill(t, dir, "beta", true)
				} else {
					t.Errorf("unexpected operation: %v", args)
				}
				return nil
			}, "vtest")
			if tc.problem != "" {
				if err == nil || !strings.Contains(err.Error(), tc.problem) {
					t.Fatalf("expected %q, got %v: %s", tc.problem, err, out.String())
				}
			} else if err != nil {
				t.Fatalf("sync failed: %v: %s", err, out.String())
			}
			expected := 0
			if tc.apply && (tc.problem == "" || tc.fail) {
				expected = 1
			}
			if executed != expected {
				t.Fatalf("executed %d operations, wanted %d", executed, expected)
			}
			if (tc.apply && (tc.problem == "" || tc.fail)) != (posts == 1) {
				t.Fatalf("unexpected inventory reports: %d", posts)
			}
			if tc.apply && tc.problem == "" {
				backups, err := filepath.Glob(filepath.Join(home, ".claude", ".reet-sync-backups", "removed-*", "extra", "SKILL.md"))
				if err != nil || len(backups) != 1 {
					t.Fatalf("removed entry was not backed up: %v %v", backups, err)
				}
			}
			if strings.Contains(out.String(), "secret") {
				t.Fatal("token leaked to output")
			}
		})
	}
}

func TestSyncRejectsConflictsAndUninstallableSkills(t *testing.T) {
	beta := store.SyncSkill{ID: "o/r/beta", Name: "beta", Slug: "beta", Source: "o/r", SourceType: "github"}
	for _, tc := range []struct {
		name    string
		desired []store.SyncSkill
		actual  []Found
	}{
		{"source conflict", []store.SyncSkill{beta}, []Found{{Name: "beta", Source: "different/repo"}}},
		{"local-only removal", nil, []Found{{Name: "custom"}}},
		{"duplicate name", []store.SyncSkill{beta, beta}, nil},
		{"invalid source", []store.SyncSkill{{Slug: "beta", Source: "--evil", SourceType: "github"}}, nil},
		{"invalid slug", []store.SyncSkill{{Slug: "../bad", Source: "o/r", SourceType: "github"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := planSync(tc.desired, tc.actual); err == nil {
				t.Fatal("unsafe plan accepted")
			}
		})
	}
}

func TestEmptyScanRegistersMachine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var machine store.MachineInfo
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Machine store.MachineInfo }
		json.NewDecoder(r.Body).Decode(&in)
		machine = in.Machine
		w.Write([]byte(`{"errors":[]}`))
	}))
	defer srv.Close()
	if err := RunVersion([]string{"--server", srv.URL, "--env", "server", "--server-host", "--prune"}, &bytes.Buffer{}, "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if machine.Host != "server" || !machine.Server || machine.Version != "v1.2.3" || machine.OS == "" {
		t.Fatalf("missing host info: %+v", machine)
	}
}

func TestSyncRejectsWrongMachineAndViewer(t *testing.T) {
	for _, writer := range []bool{true, false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"canWrite": writer, "targets": []store.SyncTarget{{Target: store.Environment{ID: "target", Host: "different-server"}}}})
		}))
		var out bytes.Buffer
		called := false
		err := runSync([]string{"--server", srv.URL, "--target", "target", "--env", "local"}, strings.NewReader(""), &out, func([]string, string) error { called = true; return nil }, "dev")
		srv.Close()
		if err == nil || called {
			t.Fatalf("wrong machine/viewer allowed: %v", err)
		}
	}
}

func TestSyncRefusesRemovingSharedCanonicalSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	canonical := filepath.Join(home, ".agents", "skills")
	mkSkill(t, canonical, "shared", true)
	os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(`{"skills":{"shared":{"source":"o/r"}}}`), 0600)
	os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0755)
	if err := os.Symlink(filepath.Join(canonical, "shared"), filepath.Join(home, ".claude", "skills", "shared")); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{{Target: store.Environment{ID: "target", Host: "local", Agent: "agents"}, Desired: []store.SyncSkill{}, Revision: "one"}}})
	}))
	defer srv.Close()
	err := runSync([]string{"--server", srv.URL, "--target", "target", "--env", "local"}, strings.NewReader(""), &bytes.Buffer{}, func([]string, string) error { t.Fatal("must not install anything"); return nil }, "dev")
	if err == nil || !strings.Contains(err.Error(), "is shared with claude") {
		t.Fatalf("shared canonical removal accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(canonical, "shared", "SKILL.md")); err != nil {
		t.Fatal("canonical content lost")
	}
}

// A skill another app already has from elsewhere is left alone and reported on
// that skill. The rest of the folder still syncs, and an exact-set plan does not
// remove the copy it left alone.
func TestSyncLeavesAConflictingSkillAloneAndSyncsTheRest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	shared := filepath.Join(home, ".agents", "skills")
	mkSkill(t, shared, "held", true)
	os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(`{"skills":{"held":{"source":"o/r"}}}`), 0600)
	claude := filepath.Join(home, ".claude", "skills")
	mkSkill(t, claude, "held", true)
	desired := []store.SyncSkill{
		{ID: "local/held", Name: "held", Slug: "held", SourceType: "private", Source: "private:" + strings.Repeat("a", 32) + ":" + strings.Repeat("b", 32) + ":" + strings.Repeat("c", 64)},
		{ID: "x/y/other", Name: "other", Slug: "other", SourceType: "github", Source: "x/y"},
	}
	for _, mode := range []string{"additive", "mirror"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{{Target: store.Environment{ID: "target", Host: "local", Agent: "claude"}, Mode: mode, Desired: desired, Revision: "one"}}})
		}))
		var plan SyncPlan
		err := runSyncWithOptions([]string{"--server", srv.URL, "--target", "target", "--env", "local"}, strings.NewReader(""), &bytes.Buffer{}, func([]string, string) error { t.Fatal("a preview must not install"); return nil }, "dev", syncOptions{onPlan: func(p SyncPlan) { plan = p }})
		srv.Close()
		if err != nil {
			t.Fatalf("%s: one conflicting name stopped the whole folder: %v", mode, err)
		}
		if !strings.Contains(plan.Problems["held"], "The shared skills folder already has this skill from o/r") {
			t.Fatalf("%s: the skill left alone was not reported: %+v", mode, plan.Problems)
		}
		if len(plan.Actions) != 1 || plan.Actions[0].Skill.Slug != "other" || !plan.Actions[0].Add {
			t.Fatalf("%s: the plan must install the other skill and touch nothing else: %+v", mode, plan.Actions)
		}
	}
	if _, err := os.Stat(filepath.Join(claude, "held", "SKILL.md")); err != nil {
		t.Fatal("the copy left alone was changed")
	}
}

func TestHelpNeverPrintsEnvironmentToken(t *testing.T) {
	t.Setenv("REET_TOKEN", "private-token-do-not-print")
	for _, cmd := range []string{"scan", "sync"} {
		var out bytes.Buffer
		var err error
		if cmd == "scan" {
			err = Run([]string{"--help"}, &out)
		} else {
			err = RunSync([]string{"--help"}, strings.NewReader(""), &out)
		}
		if err != nil {
			t.Fatalf("help failed: %v", err)
		}
		if strings.Contains(out.String(), "private-token-do-not-print") {
			t.Fatal("help exposed the inherited token")
		}
	}
}

// A machine with no skills folder yet is an empty scope. Preview works, and
// apply makes the folder so the lock and the install have somewhere to go.
func TestSyncCreatesMissingTargetDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := filepath.Join(home, ".config", "opencode", "skills")
	target := store.SyncTarget{Target: store.Environment{ID: "1/laptop/opencode", Name: "Laptop OpenCode", Host: "laptop", Agent: "opencode"}, SourceName: "Reference", Revision: "one", Desired: []store.SyncSkill{{ID: "owner/skills/improve", Name: "improve", Slug: "improve", Source: "owner/skills", SourceType: "github"}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"targets": []store.SyncTarget{target}, "canWrite": true})
			return
		}
		w.Write([]byte(`{"imported":1,"errors":[]}`))
	}))
	defer srv.Close()
	for _, apply := range []bool{false, true} {
		args := []string{"--server", srv.URL, "--token", "secret", "--env", "laptop", "--target", target.Target.ID}
		input := ""
		if apply {
			args = append(args, "--apply")
			input = "Laptop OpenCode\n"
		}
		var out bytes.Buffer
		err := runSync(args, strings.NewReader(input), &out, func([]string, string) error {
			if !apply {
				t.Fatal("preview ran an install command")
			}
			mkSkill(t, dir, "improve", true)
			if err := os.MkdirAll(filepath.Join(home, ".agents"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(`{"skills":{"improve":{"source":"owner/skills"}}}`), 0600); err != nil {
				t.Fatal(err)
			}
			return nil
		}, "dev")
		if err != nil {
			t.Fatalf("apply=%t: %v: %s", apply, err, out.String())
		}
		if !apply {
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("preview created the skills folder")
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "improve", "SKILL.md")); err != nil {
			t.Fatalf("apply did not create the folder: %v", err)
		}
	}
}

func TestSyncReplacesLocalDuplicateAndBacksItUp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude", "skills")
	mkSkill(t, dir, "improve", true)
	target := store.SyncTarget{Target: store.Environment{ID: "1/laptop/claude", Name: "Laptop Claude", Host: "laptop", Agent: "claude"}, SourceName: "Reference", Revision: "one", Desired: []store.SyncSkill{{ID: "owner/skills/improve", Name: "improve", Slug: "improve", Source: "owner/skills", SourceType: "github"}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"targets": []store.SyncTarget{target}, "canWrite": true})
			return
		}
		w.Write([]byte(`{"imported":1,"errors":[]}`))
	}))
	defer srv.Close()
	installed := false
	var out bytes.Buffer
	err := runSync([]string{"--server", srv.URL, "--token", "secret", "--env", "laptop", "--target", target.Target.ID, "--apply"}, strings.NewReader("Laptop Claude\n"), &out, func(args []string, cwd string) error {
		if !strings.Contains(strings.Join(args, " "), "add https://github.com/owner/skills") {
			t.Fatalf("unexpected command: %v", args)
		}
		mkSkill(t, dir, "improve", true)
		lockDir := filepath.Join(home, ".agents")
		os.MkdirAll(lockDir, 0755)
		if err := os.WriteFile(filepath.Join(lockDir, ".skill-lock.json"), []byte(`{"skills":{"improve":{"source":"owner/skills","sourceType":"github"}}}`), 0600); err != nil {
			t.Fatal(err)
		}
		installed = true
		return nil
	}, "dev")
	if err != nil {
		t.Fatalf("sync failed: %v: %s", err, out.String())
	}
	if !installed || !strings.Contains(out.String(), "REPLACE improve") {
		t.Fatalf("local duplicate was not replaced: %s", out.String())
	}
	backups, err := filepath.Glob(filepath.Join(home, ".claude", ".reet-sync-backups", "removed-*", "improve", "SKILL.md"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("local copy was not backed up: %v %v", backups, err)
	}
}

func TestNetworkFailureRecognisesOfflineErrors(t *testing.T) {
	for _, message := range []string{"getaddrinfo ENOTFOUND github.com", "npm ERR! network request failed", "proxyconnect tcp: connection refused"} {
		if !networkFailure(errors.New(message)) {
			t.Fatalf("%q was not treated as a network failure", message)
		}
	}
	if !networkFailure(&commandOutputError{err: errors.New("exit status 1"), output: "npm ERR! code ENOTFOUND"}) {
		t.Fatal("captured command output was not classified")
	}
	if networkFailure(errors.New("source conflict for alpha")) {
		t.Fatal("ordinary refusal looked like a network failure")
	}
}

// The skills CLI selects a skill by the name in its SKILL.md. A skill whose
// name is not its folder name ("SVG Logo Designer" in svg-logo-designer) was
// asked for by folder name only, so every install of it failed.
func TestSkillsAddCommandOffersTheSkillsOwnName(t *testing.T) {
	for _, tc := range []struct {
		label string
		skill store.SyncSkill
		names []string
	}{
		{"name is the folder", store.SyncSkill{Name: "beta", Slug: "beta"}, []string{"beta"}},
		{"display name", store.SyncSkill{Name: "SVG Logo Designer", Slug: "svg-logo-designer"}, []string{"svg-logo-designer", "SVG Logo Designer"}},
		{"only case differs", store.SyncSkill{Name: "Beta", Slug: "beta"}, []string{"beta"}},
		{"no name", store.SyncSkill{Slug: "beta"}, []string{"beta"}},
		{"a name that reads as a flag", store.SyncSkill{Name: "--all", Slug: "beta"}, []string{"beta"}},
		{"a name that selects everything", store.SyncSkill{Name: "*", Slug: "beta"}, []string{"beta"}},
		{"a name with a line break", store.SyncSkill{Name: "Beta\nTwo", Slug: "beta"}, []string{"beta"}},
	} {
		want := append([]string{"--yes", "skills@1.7.4", "add", "https://github.com/o/r", "--skill"}, tc.names...)
		want = append(want, "--agent", "codex", "--yes", "--global")
		got := skillsAddCommand("https://github.com/o/r", tc.skill, "codex", true)
		if !slices.Equal(got, want) {
			t.Errorf("%s:\n got %q\nwant %q", tc.label, got, want)
		}
	}
	project := strings.Join(skillsAddCommand("https://github.com/o/r", store.SyncSkill{Slug: "beta"}, "codex", false), " ")
	if strings.Contains(project, "--global") {
		t.Errorf("a project install must not be global: %s", project)
	}
}

// A failed install says what the installer said, not only its exit status, and
// never tells the person to preview again: nothing about the plan was stale.
func TestInstallFailureSaysWhatTheInstallerSaid(t *testing.T) {
	output := "\x1b[?25l|\n\x1b[32mo\x1b[39m  Found 10 skills\n|\n\x1b[31m■\x1b[39m  No matching skills found for: svg-logo-designer\n|\n•  Available skills:\n|    - SVG Logo Designer\nnpm notice\nnpm notice New major version of npm available!\n"
	err := installFailure("svg-logo-designer", &commandOutputError{err: errors.New("exit status 1"), output: output})
	if got := err.Error(); got != "sync stopped at svg-logo-designer: No matching skills found for: svg-logo-designer" {
		t.Fatalf("got %q", got)
	}
	if ErrReplanWorthAttempting(err) || strings.Contains(err.Error(), "preview again") {
		t.Fatalf("an install failure is not a stale plan: %v", err)
	}
	plain := installFailure("beta", errors.New("fixture failure"))
	if got := plain.Error(); got != "sync stopped at beta: fixture failure" {
		t.Fatalf("got %q", got)
	}
	offline := installFailure("beta", &commandOutputError{err: errors.New("exit status 1"), output: "npm error code ENOTFOUND\nnpm error network request to https://registry.npmjs.org/skills failed\n"})
	if got := offline.Error(); !strings.Contains(got, "npm error network request") || !strings.Contains(got, "check network access") {
		t.Fatalf("got %q", got)
	}
}

// The catalogue calls "UI/UX Design Review" uiux-design-review; the skills CLI
// installs it into ui-ux-design-review. The install worked and the plan never
// settled: the agent kept seeing the catalogued name as missing.
func TestSyncAcceptsTheFolderTheInstallerMade(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	skill := store.SyncSkill{ID: "rknall/claude-skills/uiux-design-review", Name: "UI/UX Design Review", Slug: "uiux-design-review", Source: "rknall/claude-skills", SourceType: "github"}
	for _, mode := range []string{"additive", "mirror"} {
		target := store.SyncTarget{Target: store.Environment{ID: "1/laptop/agents", Name: "Laptop shared", Host: "laptop", Agent: "agents"}, SourceName: "Reference", Revision: "one", Mode: mode, Desired: []store.SyncSkill{skill}}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" {
				json.NewEncoder(w).Encode(map[string]any{"targets": []store.SyncTarget{target}, "canWrite": true})
				return
			}
			w.Write([]byte(`{"imported":1,"errors":[]}`))
		}))
		installs := 0
		run := func(apply bool) (string, error) {
			args := []string{"--server", srv.URL, "--token", "secret", "--env", "laptop", "--target", target.Target.ID}
			if apply {
				args = append(args, "--apply")
			}
			var out bytes.Buffer
			err := runSync(args, strings.NewReader("yes\n"), &out, func(args []string, cwd string) error {
				installs++
				// What the real CLI does: a folder made from the skill's name, and a
				// lock entry keyed by that name that points at the repository folder.
				mkSkill(t, dir, "ui-ux-design-review", true)
				lock := `{"version":3,"skills":{"UI/UX Design Review":{"source":"rknall/claude-skills","sourceType":"github","skillPath":"ui-design-review/SKILL.md"}}}`
				return os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(lock), 0600)
			}, "vtest")
			return out.String(), err
		}
		if out, err := run(true); err != nil {
			t.Fatalf("%s: apply did not settle: %v\n%s", mode, err, out)
		}
		out, err := run(false)
		if err != nil || !strings.Contains(out, "Already in sync.") || strings.Contains(out, "REMOVE") {
			t.Fatalf("%s: a second look still plans changes: %v\n%s", mode, err, out)
		}
		if installs != 1 {
			t.Fatalf("%s: installed %d times", mode, installs)
		}
		srv.Close()
		os.RemoveAll(filepath.Join(dir, "ui-ux-design-review"))
		os.Remove(filepath.Join(home, ".agents", ".skill-lock.json"))
	}
}

// The skills CLI writes a skill into a folder made from its name, which is not
// always the catalogue's slug. The checks that protect what is already on the
// machine compared slugs only, so a copy another app held under the installed
// folder, from another source, was not seen and would have been overwritten.
func TestSyncLeavesACopyUnderTheInstalledFolderAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	shared := filepath.Join(home, ".agents", "skills")
	mkSkill(t, shared, "ui-ux-design-review", true)
	os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(`{"skills":{"ui-ux-design-review":{"source":"o/r"}}}`), 0600)
	claude := filepath.Join(home, ".claude", "skills")
	mkSkill(t, claude, "ui-ux-design-review", true)
	desired := []store.SyncSkill{
		{ID: "rknall/claude-skills/uiux-design-review", Name: "UI/UX Design Review", Slug: "uiux-design-review", SourceType: "github", Source: "rknall/claude-skills"},
		{ID: "x/y/other", Name: "other", Slug: "other", SourceType: "github", Source: "x/y"},
	}
	for _, mode := range []string{"additive", "mirror"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{{Target: store.Environment{ID: "target", Host: "local", Agent: "claude"}, Mode: mode, Desired: desired, Revision: "one"}}})
		}))
		var plan SyncPlan
		err := runSyncWithOptions([]string{"--server", srv.URL, "--target", "target", "--env", "local"}, strings.NewReader(""), &bytes.Buffer{}, func([]string, string) error { t.Fatal("a preview must not install"); return nil }, "dev", syncOptions{onPlan: func(p SyncPlan) { plan = p }})
		srv.Close()
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if !strings.Contains(plan.Problems["uiux-design-review"], "The shared skills folder already has this skill from o/r") {
			t.Fatalf("%s: the copy under the installed folder was not seen: problems %+v actions %+v", mode, plan.Problems, plan.Actions)
		}
		if len(plan.Actions) != 1 || plan.Actions[0].Skill.Slug != "other" || !plan.Actions[0].Add {
			t.Fatalf("%s: the plan must install the other skill and touch nothing else: %+v", mode, plan.Actions)
		}
	}
}

// An entry that is not a skill, sitting where the CLI would write, stops the
// install whether it carries the catalogue's slug or the installed folder name.
func TestSyncRefusesAnUnrecognisedEntryUnderTheInstalledFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills", "ui-ux-design-review"), 0755); err != nil {
		t.Fatal(err)
	}
	desired := []store.SyncSkill{{ID: "rknall/claude-skills/uiux-design-review", Name: "UI/UX Design Review", Slug: "uiux-design-review", SourceType: "github", Source: "rknall/claude-skills"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{{Target: store.Environment{ID: "target", Host: "local", Agent: "agents"}, Mode: "additive", Desired: desired, Revision: "one"}}})
	}))
	defer srv.Close()
	err := runSync([]string{"--server", srv.URL, "--target", "target", "--env", "local"}, strings.NewReader(""), &bytes.Buffer{}, func([]string, string) error { t.Fatal("must not install over an unrecognised entry"); return nil }, "dev")
	if err == nil || !strings.Contains(err.Error(), "unrecognised existing entry") || !strings.Contains(err.Error(), "ui-ux-design-review") {
		t.Fatalf("an unrecognised entry under the installed folder was accepted: %v", err)
	}
}

// In the scope being synced, a copy under the installed folder gets the same
// care as one under the slug: another source is a conflict that stops the plan,
// and a copy with no source is backed up before it is replaced.
func TestSyncTreatsTheInstalledFolderInItsOwnScopeAsTheSkill(t *testing.T) {
	skill := store.SyncSkill{ID: "rknall/claude-skills/uiux-design-review", Name: "UI/UX Design Review", Slug: "uiux-design-review", SourceType: "github", Source: "rknall/claude-skills"}
	serve := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" {
				json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{{Target: store.Environment{ID: "target", Name: "Local shared", Host: "local", Agent: "agents"}, Mode: "additive", Desired: []store.SyncSkill{skill}, Revision: "one"}}})
				return
			}
			w.Write([]byte(`{"imported":1,"errors":[]}`))
		}))
	}
	t.Run("another source", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		mkSkill(t, filepath.Join(home, ".agents", "skills"), "ui-ux-design-review", true)
		os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(`{"skills":{"ui-ux-design-review":{"source":"o/r"}}}`), 0600)
		srv := serve()
		defer srv.Close()
		err := runSync([]string{"--server", srv.URL, "--target", "target", "--env", "local", "--apply"}, strings.NewReader("yes\n"), &bytes.Buffer{}, func([]string, string) error { t.Fatal("must not install over another source's copy"); return nil }, "dev")
		if err == nil || !strings.Contains(err.Error(), "source conflict for uiux-design-review") {
			t.Fatalf("another source's copy under the installed folder was accepted: %v", err)
		}
	})
	t.Run("no source", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		dir := filepath.Join(home, ".agents", "skills")
		mkSkill(t, dir, "ui-ux-design-review", true)
		if err := os.WriteFile(filepath.Join(dir, "ui-ux-design-review", "mine.txt"), []byte("local work"), 0600); err != nil {
			t.Fatal(err)
		}
		srv := serve()
		defer srv.Close()
		var out bytes.Buffer
		err := runSync([]string{"--server", srv.URL, "--target", "target", "--env", "local", "--apply"}, strings.NewReader("yes\n"), &out, func([]string, string) error {
			if _, err := os.Stat(filepath.Join(dir, "ui-ux-design-review")); !os.IsNotExist(err) {
				t.Error("the unlinked copy was still in place when the install ran")
			}
			mkSkill(t, dir, "ui-ux-design-review", true)
			lock := `{"version":3,"skills":{"UI/UX Design Review":{"source":"rknall/claude-skills","skillPath":"ui-design-review/SKILL.md"}}}`
			return os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(lock), 0600)
		}, "dev")
		if err != nil || !strings.Contains(out.String(), "REPLACE uiux-design-review") {
			t.Fatalf("an unlinked copy was not replaced with a backup: %v\n%s", err, out.String())
		}
		kept, _ := filepath.Glob(filepath.Join(home, ".agents", ".reet-sync-backups", "removed-*", "ui-ux-design-review", "mine.txt"))
		if len(kept) != 1 {
			t.Fatalf("the unlinked copy was not backed up: %v\n%s", kept, out.String())
		}
	})
}
