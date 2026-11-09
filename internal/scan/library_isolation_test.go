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
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// A failure applying one managed item must not stop an unrelated healthy item
// in the same apply, and must still be reported rather than hidden.
func TestManagedRemovalFailureIsIsolatedPerItem(t *testing.T) {
	for _, mode := range []string{"intent_changed", "post_move_failure", "authority_changed"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			root := filepath.Join(home, ".claude", "skills")
			slugs := []string{"a-failing", "z-healthy"}
			ids := map[string]string{"a-failing": strings.Repeat("1", 32), "z-healthy": strings.Repeat("2", 32)}
			var state libraryLocalState
			digests := map[string]string{}
			for _, slug := range slugs {
				dir := filepath.Join(root, slug)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+slug+"\ndescription: Isolation fixture\n---\nBody for "+slug+".\n"), 0644); err != nil {
					t.Fatal(err)
				}
				digest, err := privateSkillDigest(dir)
				if err != nil {
					t.Fatal(err)
				}
				digests[slug] = digest
				state.Bindings = append(state.Bindings, libraryBinding{Path: dir, Paths: []string{"SKILL.md"}, ItemID: ids[slug], Revision: strings.Repeat("d", 32), Digest: digest, Deployment: &libraryDeployment{TargetID: "target", Revision: strings.Repeat("d", 32), Digest: digest, Verified: true}})
			}
			target := store.SyncTarget{Canonical: true, Target: store.Environment{ID: "target", Name: "Desktop", Host: "desktop", Agent: "claude"}, Mode: "additive", Revision: "revision"}
			var mu sync.Mutex
			seen := map[string]int{}
			var configPath string
			var c agentConfig
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/library/deployment-intent" {
					var check store.LibraryDeploymentCheck
					if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
						t.Error(err)
					}
					mu.Lock()
					seen[check.ItemID]++
					n := seen[check.ItemID]
					mu.Unlock()
					// Reads 1 and 2 are planning and the pre-apply re-check; later reads are execution-time checks.
					if check.ItemID == ids["a-failing"] && n >= 4 {
						if mode == "intent_changed" {
							http.Error(w, "intent changed", http.StatusConflict)
							return
						}
						if mode == "post_move_failure" && n >= 5 {
							http.Error(w, "intent changed after backup", http.StatusConflict)
							return
						}
					}
					_ = json.NewEncoder(w).Encode(store.LibraryDeploymentIntent{Generation: strings.Repeat("a", 64), Action: "remove", Reason: "inactive"})
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
			c = agentConfig{Server: controller.URL, ID: strings.Repeat("b", 32), Hostname: hostname, Home: home, Token: "fixture", Secret: "fixture", AllowApply: true}
			configPath = filepath.Join(home, "agent.json")
			if err := writePrivate(configPath, c); err != nil {
				t.Fatal(err)
			}
			state.Server, state.Enrollment, state.ManagedRemovals, state.CalibratedAt = c.Server, c.ID, managedRemovalVersion, time.Now().UnixMilli()
			if err := writePrivate(configPath+".library", state); err != nil {
				t.Fatal(err)
			}
			var backups []string
			opts := syncOptions{managedRemovals: func(target store.SyncTarget, path string) ([]SyncAction, map[string]string, error) {
				return planLibraryRemovals(context.Background(), configPath, c, target, path, "test")
			}, removeManaged: func(action SyncAction, path string, out io.Writer, onBackup func(string)) error {
				if mode == "authority_changed" && action.Skill.Slug == "a-failing" {
					// Consent is withdrawn locally mid-apply: this is target-wide and must stop everything.
					c2 := c
					c2.AllowApply = false
					if err := writePrivate(configPath, c2); err != nil {
						t.Error(err)
					}
				}
				return removeLibraryDeployment(context.Background(), configPath, c, action, path, "test", out, onBackup)
			}, onBackup: func(path string) { backups = append(backups, path) }}
			args := []string{"--server", c.Server, "--token", c.Token, "--env", "desktop", "--target", "target", "--apply"}
			err := runSyncWithOptions(args, strings.NewReader("yes\n"), io.Discard, func([]string, string) error { t.Fatal("removal invoked npx"); return nil }, "test", opts)
			if err == nil {
				t.Fatal("a failed item was reported as a complete sync")
			}
			_, failingErr := os.Lstat(filepath.Join(root, "a-failing"))
			_, healthyErr := os.Lstat(filepath.Join(root, "z-healthy"))
			if mode == "post_move_failure" && !os.IsNotExist(failingErr) {
				t.Fatal("the post-move fixture never moved its managed copy", failingErr)
			}
			if mode != "post_move_failure" && failingErr != nil {
				t.Fatal("the failing item's files were not retained", failingErr)
			}
			if mode != "authority_changed" {
				wantBackups := 1
				if mode == "post_move_failure" {
					wantBackups = 2
					if strings.Contains(err.Error(), "left as they were") {
						t.Fatal("a post-write failure falsely claimed files were unchanged", err)
					}
				}
				if !os.IsNotExist(healthyErr) || len(backups) != wantBackups {
					t.Fatalf("a healthy item was stopped by an unrelated failure: stat=%v backups=%v run=%v", healthyErr, backups, err)
				}
				if !strings.Contains(err.Error(), "a-failing") || !strings.Contains(err.Error(), "attention") {
					t.Fatal("the failing item is not named in the result", err)
				}
				for _, backup := range backups {
					got, _ := privateSkillDigest(backup)
					if got != digests[filepath.Base(backup)] {
						t.Fatal("recoverable backup changed", backup)
					}
				}
			} else if healthyErr != nil || len(backups) != 0 {
				t.Fatalf("withdrawn consent must stop the whole target: err=%v backups=%v", healthyErr, backups)
			}
		})
	}
}
