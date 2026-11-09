package scan

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestLibraryManagedRemovalExecutor(t *testing.T) {
	for _, mode := range []string{"remove", "dirty", "partial", "old_controller", "future_capability", "source", "unverified", "wrong_target", "maintain", "blocked", "stale_before_move", "stale_after_move", "journal_refused", "backup_link", "edit_during_check", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			defer os.Chmod(home, 0755)
			t.Setenv("HOME", home)
			root := filepath.Join(home, ".claude", "skills")
			path := filepath.Join(root, "demo")
			foreign := filepath.Join(root, "unmanaged")
			for _, dir := range []string{path, foreign} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: Removal fixture\n---\nOriginal instructions.\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(path, "reference.txt"), []byte("preserved support file"), 0644); err != nil {
				t.Fatal(err)
			}
			manifest, err := skillbundle.Scan(path, skillbundle.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "dirty" {
				if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: demo\ndescription: Removal fixture\n---\nUnseen local changes.\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "partial" {
				if err := os.Remove(filepath.Join(path, "reference.txt")); err != nil {
					t.Fatal(err)
				}
			}
			beforeDigest, err := privateSkillDigest(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "backup_link" {
				if err := os.Symlink(foreign, filepath.Join(home, ".claude", ".reet-sync-backups")); err != nil {
					t.Fatal(err)
				}
			}
			target := store.SyncTarget{Canonical: true, Target: store.Environment{ID: "target", Name: "Desktop", Host: "desktop", Agent: "claude"}, Mode: "additive", Revision: "revision"}
			var requests atomic.Int32
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/library/deployment-intent" {
					n := requests.Add(1)
					if mode == "edit_during_check" && n == 4 {
						if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("late edit, must survive"), 0644); err != nil {
							t.Error(err)
						}
					}
					if mode == "journal_refused" && n == 3 {
						if err := os.Chmod(home, 0500); err != nil {
							t.Error(err)
						}
					}
					var check store.LibraryDeploymentCheck
					if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
						t.Error(err)
					}
					if (mode == "stale_before_move" && n >= 4) || (mode == "stale_after_move" && n >= 5) {
						http.Error(w, "intent changed", http.StatusConflict)
						return
					}
					action := "remove"
					if mode == "maintain" || mode == "blocked" {
						action = mode
					}
					if mode == "mixed" && check.ItemID == strings.Repeat("e", 32) {
						action = "blocked"
					}
					_ = json.NewEncoder(w).Encode(store.LibraryDeploymentIntent{Generation: strings.Repeat("a", 64), Action: action, Reason: "inactive"})
					return
				}
				if r.Method == "GET" {
					_ = json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{target}})
				} else {
					_, _ = w.Write([]byte(`{"errors":[]}`))
				}
			}))
			defer controller.Close()
			hostname, _ := os.Hostname()
			c := agentConfig{Server: controller.URL, ID: strings.Repeat("b", 32), Hostname: hostname, Home: home, Token: "fixture", Secret: "fixture", AllowApply: true}
			config := filepath.Join(home, "agent.json")
			if err := writePrivate(config, c); err != nil {
				t.Fatal(err)
			}
			binding := libraryBinding{Path: path, Paths: []string{"SKILL.md", "reference.txt"}, ItemID: strings.Repeat("c", 32), Revision: strings.Repeat("d", 32), Digest: manifest.Digest, Deployment: &libraryDeployment{TargetID: "target", Revision: strings.Repeat("d", 32), Digest: manifest.Digest, Verified: true}}
			state := libraryLocalState{Server: c.Server, Enrollment: c.ID, ManagedRemovals: managedRemovalVersion, CalibratedAt: time.Now().UnixMilli(), Bindings: []libraryBinding{binding}}
			if mode == "mixed" {
				blockedPath := filepath.Join(root, "blocked")
				if err := os.Mkdir(blockedPath, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(blockedPath, "SKILL.md"), []byte("---\nname: blocked\ndescription: Retain me\n---\nCannot sync yet.\n"), 0644); err != nil {
					t.Fatal(err)
				}
				digest, err := privateSkillDigest(blockedPath)
				if err != nil {
					t.Fatal(err)
				}
				state.Bindings = append(state.Bindings, libraryBinding{Path: blockedPath, Paths: []string{"SKILL.md"}, ItemID: strings.Repeat("e", 32), Revision: binding.Revision, Digest: digest, Deployment: &libraryDeployment{TargetID: "target", Revision: binding.Revision, Digest: digest, Verified: true}})
			}
			switch mode {
			case "old_controller":
				state.ManagedRemovals = 0
			case "future_capability":
				state.ManagedRemovals = 2
			case "source":
				state.Bindings[0].Source = true
			case "unverified":
				state.Bindings[0].Deployment.Verified = false
			case "wrong_target":
				state.Bindings[0].Deployment.TargetID = "other"
			}
			if err := writePrivate(config+".library", state); err != nil {
				t.Fatal(err)
			}
			var backups []string
			var plan SyncPlan
			opts := syncOptions{managedRemovals: func(target store.SyncTarget, path string) ([]SyncAction, map[string]string, error) {
				return planLibraryRemovals(context.Background(), config, c, target, path, "test")
			}, removeManaged: func(action SyncAction, path string, out io.Writer, onBackup func(string)) error {
				return removeLibraryDeployment(context.Background(), config, c, action, path, "test", out, onBackup)
			}, onPlan: func(p SyncPlan) { plan = p }, onBackup: func(path string) { backups = append(backups, path) }}
			args := []string{"--server", c.Server, "--token", c.Token, "--env", "desktop", "--target", "target", "--apply"}
			err = runSyncWithOptions(args, strings.NewReader("yes\n"), io.Discard, func([]string, string) error { t.Fatal("removal invoked npx"); return nil }, "test", opts)
			failed := strings.HasPrefix(mode, "stale_") || mode == "journal_refused" || mode == "backup_link" || mode == "edit_during_check"
			if (err != nil) != failed {
				t.Fatalf("mode=%s result=%v", mode, err)
			}
			removed := mode == "remove" || mode == "dirty" || mode == "partial" || mode == "stale_after_move" || mode == "edit_during_check" || mode == "mixed"
			_, pathErr := os.Lstat(path)
			if os.IsNotExist(pathErr) != removed {
				t.Fatalf("wrong filesystem outcome: removed=%v stat=%v", removed, pathErr)
			}
			if removed {
				if len(backups) != 1 {
					t.Fatal("missing recoverable backup", backups)
				}
				got, err := privateSkillDigest(backups[0])
				if err != nil || (mode != "edit_during_check" && got != beforeDigest) {
					t.Fatal("local bytes changed during backup", got, err)
				}
				if mode == "edit_during_check" {
					data, err := os.ReadFile(filepath.Join(backups[0], "SKILL.md"))
					if err != nil || string(data) != "late edit, must survive" {
						t.Fatal("late edit was lost", string(data), err)
					}
				}
			}
			if _, err := os.Stat(filepath.Join(foreign, "SKILL.md")); err != nil {
				t.Fatal("unmanaged copy was modified", err)
			}
			got, err := loadLibraryState(config, c)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "dirty" && (len(got.Pending) != 1 || !strings.Contains(string(got.Pending[0].Files[0].Data), "Unseen local changes")) {
				t.Fatal("unseen edits were not durably captured", got.Pending)
			}
			if mode == "partial" && len(got.Pending) != 0 {
				t.Fatal("partial disappearance became a central edit", got.Pending)
			}
			if mode == "stale_after_move" || mode == "edit_during_check" {
				if got.Bindings[0].Deployment.Removal.Phase != "prepared" || got.Bindings[0].Deployment.Removal.Backup != backups[0] {
					t.Fatal("uncertain outcome lost recovery journal", got)
				}
			} else if removed && (got.Bindings[0].Deployment.Verified || got.Bindings[0].Deployment.Removal.Phase != "removed") {
				t.Fatal("removed directory still claims installed ownership", got)
			}
			if (mode == "blocked" || mode == "mixed") && len(plan.Problems) != 1 {
				t.Fatal("blocked intent was falsely acknowledged", plan)
			}
			if mode == "mixed" {
				if _, err := os.Stat(filepath.Join(root, "blocked", "SKILL.md")); err != nil {
					t.Fatal("blocked deployment was not retained", err)
				}
			}
			if (mode == "old_controller" || mode == "future_capability" || mode == "source" || mode == "unverified" || mode == "wrong_target") && requests.Load() != 0 {
				t.Fatal("unnegotiated or unowned entry requested removal", requests.Load())
			}
		})
	}
}

func TestLibraryRemovalCapabilityNegotiation(t *testing.T) {
	for _, advertised := range []int{0, 1, 2} {
		t.Run(string(rune('0'+advertised)), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := map[string]any{"schemaVersion": 1, "serverTime": time.Now().UnixMilli()}
				if advertised != 0 {
					response["managedRemovals"] = advertised
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer controller.Close()
			hostname, _ := os.Hostname()
			c := agentConfig{Server: controller.URL, ID: strings.Repeat("a", 32), Hostname: hostname, Home: home, AllowApply: true}
			config := filepath.Join(home, "agent.json")
			if err := writePrivate(config, c); err != nil {
				t.Fatal(err)
			}
			if err := writePrivate(config+".library", libraryLocalState{Server: c.Server, Enrollment: c.ID, ManagedRemovals: 1}); err != nil {
				t.Fatal(err)
			}
			if err := syncLibraryEdits(context.Background(), c, config, "test", io.Discard); err != nil {
				t.Fatal(err)
			}
			state, err := loadLibraryState(config, c)
			if err != nil || state.ManagedRemovals != advertised {
				t.Fatal("capability was inferred or survived a downgrade", state.ManagedRemovals, advertised, err)
			}
		})
	}
}

func TestLibraryEditSyncRefusesReplacedEnrollment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var requests atomic.Int32
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "serverTime": time.Now().UnixMilli()})
	}))
	defer controller.Close()
	hostname, _ := os.Hostname()
	c := agentConfig{Server: controller.URL, ID: strings.Repeat("a", 32), Hostname: hostname, Home: home, AllowApply: true}
	config := filepath.Join(home, "agent.json")
	local := c
	local.ID = strings.Repeat("b", 32)
	if err := writePrivate(config, local); err != nil {
		t.Fatal(err)
	}
	state := libraryLocalState{Server: c.Server, Enrollment: c.ID, Pending: []store.LibraryEdit{{EventID: "retained", Files: []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("offline edit")}}}}}
	if err := writePrivate(config+".library", state); err != nil {
		t.Fatal(err)
	}
	if err := syncLibraryEdits(context.Background(), c, config, "test", io.Discard); err == nil || requests.Load() != 0 {
		t.Fatal("old in-memory enrollment made requests after replacement", err, requests.Load())
	}
	retained, err := loadLibraryState(config, c)
	if err != nil || len(retained.Pending) != 1 || string(retained.Pending[0].Files[0].Data) != "offline edit" {
		t.Fatal("replacement discarded offline bytes", retained, err)
	}
}
