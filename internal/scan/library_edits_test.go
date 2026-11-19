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
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestLibraryCapturePreservesManagedFileDeletion(t *testing.T) {
	path := t.TempDir()
	skill := "---\nname: demo\ndescription: Fixture\n---\noriginal\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(skill), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "reference.txt"), []byte("keep me"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest, err := skillbundle.Scan(path, skillbundle.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	state := libraryLocalState{CalibratedAt: time.Now().UnixMilli(), Bindings: []libraryBinding{{Path: path, ItemID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), Digest: manifest.Digest, Paths: []string{"SKILL.md", "reference.txt"}}}}
	if err := os.Remove(filepath.Join(path, "reference.txt")); err != nil {
		t.Fatal(err)
	}
	if err := captureLibraryEdits(&state, homeFolders{}); err != nil {
		t.Fatal(err)
	}
	if len(state.Pending) != 0 || state.Bindings[0].Digest != manifest.Digest {
		t.Fatal("local absence became a central content deletion", state)
	}
}

func TestLibraryConsentRefreshesWithoutRestartAndKeepsPendingBytes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	uploads, negotiations := 0, 0
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/library/protocol":
			negotiations++
			_ = json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "serverTime": time.Now().UnixMilli()})
		case "/api/v2/library/edits":
			uploads++
			var event store.LibraryEdit
			if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(store.LibraryEditReceipt{EventID: event.EventID, RevisionID: strings.Repeat("c", 32), Won: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer controller.Close()
	hostname, _ := os.Hostname()
	config := filepath.Join(home, "agent.json")
	c := agentConfig{Server: controller.URL, Home: home, Hostname: hostname, ID: strings.Repeat("a", 32), Token: "fixture", Secret: "fixture", AllowApply: false}
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	state := libraryLocalState{Server: c.Server, Enrollment: c.ID, CalibratedAt: time.Now().UnixMilli(), Pending: []store.LibraryEdit{{ItemID: strings.Repeat("a", 32), Base: strings.Repeat("b", 32), EventID: strings.Repeat("d", 32), SavedAt: time.Now().UnixMilli(), Files: []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("pending exact bytes")}}}}}
	if err := writePrivate(config+".library", state); err != nil {
		t.Fatal(err)
	}
	if err := syncLibraryEdits(context.Background(), c, config, "test", io.Discard); err != nil {
		t.Fatal(err)
	}
	if negotiations != 1 || uploads != 0 {
		t.Fatal("consent-off uploaded or failed to negotiate", negotiations, uploads)
	}
	retained, err := loadLibraryState(config, c)
	if err != nil || len(retained.Pending) != 1 {
		t.Fatal("consent-off lost pending edit", retained, err)
	}
	local := c
	local.AllowApply = true
	if err := writePrivate(config, local); err != nil {
		t.Fatal(err)
	}
	// Pass the old in-memory config, as the long-running daemon does.
	if err := syncLibraryEdits(context.Background(), c, config, "test", io.Discard); err != nil {
		t.Fatal(err)
	}
	retained, err = loadLibraryState(config, c)
	if err != nil || uploads != 1 || len(retained.Pending) != 0 {
		t.Fatal("local consent needed a daemon restart", uploads, retained, err)
	}
}

// A machine that follows the library never sends edits made in a managed home
// folder: reet's copy wins there, so the reconcile pass backs the edit up and
// reports it edited. Edits in a project folder are still sent, and edits
// queued for a home copy before this build stay on disk, unsent.
func TestLibraryEditsLeaveManagedHomeCopiesToReconcile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var uploaded []store.LibraryEdit
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/library/protocol":
			_ = json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "serverTime": time.Now().UnixMilli()})
		case "/api/v2/library/edits":
			var event store.LibraryEdit
			if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
				t.Error(err)
			}
			uploaded = append(uploaded, event)
			_ = json.NewEncoder(w).Encode(store.LibraryEditReceipt{EventID: event.EventID, RevisionID: strings.Repeat("c", 32), Won: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer controller.Close()
	hostname, _ := os.Hostname()
	config := filepath.Join(home, "agent.json")
	c := agentConfig{Server: controller.URL, Home: home, Hostname: hostname, ID: strings.Repeat("a", 32), Token: "fixture", Secret: "fixture", AllowApply: true}
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	reets, reetFiles := skillBundle(t, "demo", "reet's copy")
	homeCopy := filepath.Join(home, ".agents", "skills", "demo")
	placeFiles(t, homeCopy, reetFiles)
	project := filepath.Join(t.TempDir(), "app", ".claude", "skills", "tool")
	_, toolFiles := skillBundle(t, "tool", "project copy")
	placeFiles(t, project, toolFiles)
	toolDigest := digestOfFiles(t, toolFiles)
	edit := func(dir, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+filepath.Base(dir)+"\ndescription: edited\n---\n"+body+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	edit(homeCopy, "edited at home")
	edit(project, "edited in the project")
	homeItem, toolItem := strings.Repeat("1", 32), strings.Repeat("2", 32)
	queued := store.LibraryEdit{EventID: strings.Repeat("e", 32), ItemID: homeItem, Base: strings.Repeat("b", 32), SavedAt: time.Now().UnixMilli(), Files: []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("queued before the update")}}}
	state := libraryLocalState{Server: c.Server, Enrollment: c.ID, CalibratedAt: time.Now().UnixMilli(),
		Bindings: []libraryBinding{
			{Path: homeCopy, ItemID: homeItem, Revision: strings.Repeat("b", 32), Digest: reets.Digest, Paths: []string{"SKILL.md", "scripts/run.sh"}},
			{Path: project, ItemID: toolItem, Revision: strings.Repeat("d", 32), Digest: toolDigest, Paths: []string{"SKILL.md", "scripts/run.sh"}},
		},
		Pending: []store.LibraryEdit{queued}}
	if err := writePrivate(config+".library", state); err != nil {
		t.Fatal(err)
	}
	if err := syncLibraryEdits(context.Background(), c, config, "test", io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(uploaded) != 1 || uploaded[0].ItemID != toolItem {
		t.Fatalf("uploaded %d edits: %+v", len(uploaded), uploaded)
	}
	kept, err := loadLibraryState(config, c)
	if err != nil || len(kept.Pending) != 1 || kept.Pending[0].EventID != queued.EventID || kept.Bindings[0].Digest != reets.Digest {
		t.Fatalf("the home edit was captured, or the queued one lost: %+v %v", kept, err)
	}
	// Reconcile then puts reet's copy back and keeps the edit in a backup.
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"demo": reetFiles}}
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{reets}}, map[string]string{".agents/skills/demo": reets.Digest}, stub.fetch, reconcileNow)
	e := entryOf(t, rep, "~/.agents/skills", "demo")
	if e.State != "edited" || e.Backup == "" {
		t.Fatalf("entry %+v", e)
	}
	if got := folderDigest(t, homeCopy); got != reets.Digest {
		t.Fatal("reet's copy was not put back")
	}
	b, err := os.ReadFile(filepath.Join(fromTilde(home, e.Backup), "SKILL.md"))
	if err != nil || !strings.Contains(string(b), "edited at home") {
		t.Fatalf("the backup does not hold the edit: %q %v", b, err)
	}
}

// A controller that recorded this machine below the skills protocol still runs
// the job-based sync, which takes edits made at home: they are sent as before.
// Once a poll says the machine follows the library, they are not.
func TestLibraryEditsFollowWhatTheControllerRecorded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	uploads := 0
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/library/protocol":
			_ = json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "serverTime": time.Now().UnixMilli()})
		case "/api/v2/library/edits":
			uploads++
			var event store.LibraryEdit
			_ = json.NewDecoder(r.Body).Decode(&event)
			_ = json.NewEncoder(w).Encode(store.LibraryEditReceipt{EventID: event.EventID, RevisionID: strings.Repeat("c", 32), Won: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer controller.Close()
	hostname, _ := os.Hostname()
	config := filepath.Join(home, "agent.json")
	c := agentConfig{Server: controller.URL, Home: home, Hostname: hostname, ID: strings.Repeat("a", 32), Token: "fixture", Secret: "fixture", AllowApply: true}
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	reets, reetFiles := skillBundle(t, "demo", "reet's copy")
	homeCopy := filepath.Join(home, ".claude", "skills", "demo")
	placeFiles(t, homeCopy, reetFiles)
	state := libraryLocalState{Server: c.Server, Enrollment: c.ID, CalibratedAt: time.Now().UnixMilli(),
		Bindings: []libraryBinding{{Path: homeCopy, ItemID: strings.Repeat("1", 32), Revision: strings.Repeat("b", 32), Digest: reets.Digest, Paths: []string{"SKILL.md", "scripts/run.sh"}}}}
	if err := writePrivate(config+".library", state); err != nil {
		t.Fatal(err)
	}
	editTo := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(homeCopy, "SKILL.md"), []byte("---\nname: demo\ndescription: edited\n---\n"+body+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	keepControllerSkillsProtocol(config, c, 0, io.Discard)
	editTo("edited for an older controller")
	if err := syncLibraryEdits(context.Background(), c, config, "test", io.Discard); err != nil {
		t.Fatal(err)
	}
	if uploads != 1 {
		t.Fatalf("an older controller got %d uploads, want 1", uploads)
	}
	keepControllerSkillsProtocol(config, c, store.CurrentSkillsProtocol, io.Discard)
	editTo("edited once the machine follows the library")
	if err := syncLibraryEdits(context.Background(), c, config, "test", io.Discard); err != nil {
		t.Fatal(err)
	}
	if uploads != 1 {
		t.Fatalf("a machine that follows the library sent an edit: %d uploads", uploads)
	}
}
