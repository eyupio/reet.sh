package scan

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

// Exercise the real executor, staging rename, pre-write pin and post-write
// journal together. In particular a valid pin must not prove a failed install.
func TestLibraryDeploymentOwnershipRequiresCompletedInstall(t *testing.T) {
	for _, failure := range []string{"", "pin", "prepared", "rename", "verify"} {
		t.Run("failure_"+failure, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			directory := filepath.Join(home, ".agents", "skills")
			config := filepath.Join(home, "agent.json")
			files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Journal fixture\n---\nInstructions.\n")}}
			manifest, err := skillbundle.FromContents(files)
			if err != nil {
				t.Fatal(err)
			}
			revision := store.PrivateSkillRevision{ID: strings.Repeat("b", 32), BundleID: strings.Repeat("c", 32), ManifestDigest: manifest.Digest, Files: files}
			skill := store.SyncSkill{ID: "local/demo", ItemID: strings.Repeat("a", 32), Name: "demo", Slug: "demo", SourceType: "private", Source: "private:" + revision.BundleID + ":" + revision.ID + ":" + manifest.Digest}
			target := store.SyncTarget{Canonical: true, Target: store.Environment{ID: "target", Name: "Desktop", Host: "desktop", Agent: "agents"}, Mode: "additive", Revision: "revision", Desired: []store.SyncSkill{skill}}
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api/v2/library/bases":
					if failure == "pin" {
						http.Error(w, "pin refused", http.StatusConflict)
						return
					}
					var base store.LibraryBase
					if err := json.NewDecoder(r.Body).Decode(&base); err != nil {
						t.Error(err)
					}
					_ = json.NewEncoder(w).Encode(base)
				case strings.HasPrefix(r.URL.Path, "/api/v2/skill-bundles/"):
					_ = json.NewEncoder(w).Encode(revision)
				case r.Method == "GET":
					_ = json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{target}})
				default:
					_, _ = w.Write([]byte(`{"errors":[]}`))
				}
			}))
			defer controller.Close()
			hostname, _ := os.Hostname()
			c := agentConfig{Server: controller.URL, ID: strings.Repeat("d", 32), Token: "fixture", Secret: "fixture", AllowApply: true, Home: home, Hostname: hostname}
			if err := writePrivate(config, c); err != nil {
				t.Fatal(err)
			}
			verified := 0
			opts := syncOptions{onInstalled: func(s store.SyncSkill, path string, files []skillbundle.ContentFile) error {
				if err := prepareLibraryInstall(context.Background(), config, c, s, path, files, "test", target.Target.ID); err != nil {
					return err
				}
				if failure == "prepared" {
					return errors.New("interrupted before rename")
				}
				if failure == "rename" {
					if err := os.Mkdir(filepath.Join(path, s.Slug), 0755); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(path, s.Slug, "foreign.txt"), []byte("preserve"), 0644)
				}
				return nil
			}, onInstallVerified: func(s store.SyncSkill, path string) error {
				verified++
				if failure == "verify" {
					if err := os.WriteFile(filepath.Join(path, s.Slug, "SKILL.md"), []byte("changed after rename"), 0644); err != nil {
						return err
					}
				}
				return finishLibraryInstall(config, c, s, path, target.Target.ID)
			}}
			args := []string{"--server", c.Server, "--token", c.Token, "--env", "desktop", "--target", target.Target.ID, "--apply"}
			err = runSyncWithOptions(args, strings.NewReader("yes\n"), io.Discard, func([]string, string) error { t.Fatal("private install invoked npx"); return nil }, "test", opts)
			if (err != nil) != (failure != "") {
				t.Fatalf("failure=%q result=%v", failure, err)
			}
			state, err := loadLibraryState(config, c)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "pin" {
				if len(state.Bindings) != 0 {
					t.Fatal("refused pin acquired a journal binding", state)
				}
			} else {
				if len(state.Bindings) != 1 || state.Bindings[0].Deployment == nil {
					t.Fatal("prepared install lost durable provenance", state)
				}
				deployment := state.Bindings[0].Deployment
				if deployment.TargetID != "target" || deployment.Revision != revision.ID || deployment.Digest != manifest.Digest || deployment.Verified != (failure == "") {
					t.Fatal("incorrect ownership receipt", deployment)
				}
			}
			if failure == "pin" || failure == "prepared" || failure == "rename" {
				if verified != 0 {
					t.Fatal("failed install reached verification", verified)
				}
			}
			if failure == "rename" {
				data, err := os.ReadFile(filepath.Join(directory, "demo", "foreign.txt"))
				if err != nil || string(data) != "preserve" {
					t.Fatal("failed rename changed foreign bytes", string(data), err)
				}
			}
		})
	}
}

func TestLibraryDeploymentNeverPromotesSourcesOrLegacyBases(t *testing.T) {
	for _, source := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "source"}[source], func(t *testing.T) {
			home := t.TempDir()
			c := agentConfig{Server: "https://example.test", ID: strings.Repeat("a", 32)}
			config := filepath.Join(home, "agent.json")
			files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Fixture\n---\nOriginal.\n")}}
			manifest, _ := skillbundle.FromContents(files)
			skill := store.SyncSkill{ItemID: strings.Repeat("b", 32), Slug: "demo", Source: "private:" + strings.Repeat("c", 32) + ":" + strings.Repeat("d", 32) + ":" + manifest.Digest}
			state := libraryLocalState{Server: c.Server, Enrollment: c.ID, Bindings: []libraryBinding{{Path: filepath.Join(home, "demo"), ItemID: skill.ItemID, Revision: strings.Repeat("d", 32), Digest: manifest.Digest, Source: source}}}
			if err := writePrivate(config+".library", state); err != nil {
				t.Fatal(err)
			}
			if err := recordLibraryInstall(config, c, skill, home, files, "target"); err != nil {
				t.Fatal(err)
			}
			if err := finishLibraryInstall(config, c, skill, home, "target"); err != nil {
				t.Fatal(err)
			}
			got, err := loadLibraryState(config, c)
			if err != nil || got.Bindings[0].Deployment != nil || got.Bindings[0].Source != source {
				t.Fatal("an edit base was promoted to removable ownership", got, err)
			}
		})
	}
}

func TestLibraryDeploymentVerificationFences(t *testing.T) {
	for _, changed := range []string{"target", "item", "revision", "consent", "enrollment", "local_reenrollment", "link", "source_import"} {
		t.Run(changed, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			hostname, _ := os.Hostname()
			c := agentConfig{Server: "https://example.test", ID: strings.Repeat("a", 32), Hostname: hostname, Home: home, AllowApply: true}
			config := filepath.Join(home, "agent.json")
			if err := writePrivate(config, c); err != nil {
				t.Fatal(err)
			}
			files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Fixture\n---\nOriginal.\n")}}
			manifest, _ := skillbundle.FromContents(files)
			skill := store.SyncSkill{ItemID: strings.Repeat("b", 32), Slug: "demo", Source: "private:" + strings.Repeat("c", 32) + ":" + strings.Repeat("d", 32) + ":" + manifest.Digest}
			if err := recordLibraryInstall(config, c, skill, home, files, "target"); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(home, skill.Slug)
			if err := os.Mkdir(destination, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(destination, "SKILL.md"), files[0].Data, 0644); err != nil {
				t.Fatal(err)
			}
			target := "target"
			verifySkill, verifyConfig := skill, c
			switch changed {
			case "target":
				target = "other"
			case "item":
				verifySkill.ItemID = strings.Repeat("e", 32)
			case "revision":
				verifySkill.Source = strings.Replace(skill.Source, strings.Repeat("d", 32), strings.Repeat("e", 32), 1)
			case "consent":
				local := c
				local.AllowApply = false
				if err := writePrivate(config, local); err != nil {
					t.Fatal(err)
				}
			case "enrollment":
				verifyConfig.ID = strings.Repeat("e", 32)
			case "local_reenrollment":
				local := c
				local.ID = strings.Repeat("e", 32)
				if err := writePrivate(config, local); err != nil {
					t.Fatal(err)
				}
			case "link":
				other := filepath.Join(home, "other")
				if err := os.Rename(destination, other); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, destination); err != nil {
					t.Fatal(err)
				}
			case "source_import":
				if err := recordLibrarySource(config, c, destination, skill.ItemID, strings.Repeat("d", 32), manifest); err != nil {
					t.Fatal(err)
				}
			}
			err := finishLibraryInstall(config, verifyConfig, verifySkill, home, target)
			if changed != "source_import" && err == nil {
				t.Fatal("changed identity/consent/path was verified")
			}
			state, err := loadLibraryState(config, c)
			if err != nil {
				t.Fatal(err)
			}
			if deployment := state.Bindings[0].Deployment; deployment != nil && deployment.Verified {
				t.Fatal("unsafe deployment acquired ownership", deployment)
			}
			data, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
			if err != nil || string(data) != string(files[0].Data) {
				t.Fatal("verification changed local content", string(data), err)
			}
		})
	}
}
