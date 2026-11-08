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
	if err := captureLibraryEdits(&state); err != nil {
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
