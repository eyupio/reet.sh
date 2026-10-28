package scan

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestSyncPreviewConfirmationAndApply(t *testing.T) {
	for _, tc := range []struct {
		name, input, problem         string
		apply, fail, changed, locked bool
	}{
		{name: "preview"},
		{name: "confirmation declined", apply: true, input: "no\n", problem: "cancelled"},
		{name: "apply install and removal", apply: true, input: "Laptop Claude\n"},
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
