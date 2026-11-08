package scan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

func TestLibrarySkillLocationsHonourOverridesAndRemainDistinct(t *testing.T) {
	home, project, custom := t.TempDir(), t.TempDir(), t.TempDir()
	values := map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(home, "claude-custom"), "CODEX_HOME": filepath.Join(home, "codex-custom"), "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "OPENCODE_CONFIG_DIR": filepath.Join(home, "opencode-custom")}
	locations, err := CollectLibrarySkillLocations(home, []string{project}, []string{custom, custom}, "agy", func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, location := range locations {
		if seen[location.ID] {
			t.Fatal("root identity collision")
		}
		seen[location.ID] = true
	}
	for _, want := range []string{filepath.Join(home, ".agents", "skills"), filepath.Join(values["CLAUDE_CONFIG_DIR"], "skills"), filepath.Join(values["CODEX_HOME"], "skills"), filepath.Join(values["OPENCODE_CONFIG_DIR"], "skills"), filepath.Join(project, ".agents", "skills"), custom} {
		found := false
		for _, location := range locations {
			found = found || location.Path == want
		}
		if !found {
			t.Fatal("missing explicit/default/override root", want)
		}
	}
	if len(locations) != 9 {
		t.Fatalf("duplicate custom root retained: %d", len(locations))
	}
	values["CLAUDE_CONFIG_DIR"] = "relative/path"
	valid, err := CollectLibrarySkillLocations(home, nil, nil, "shared", func(key string) string { return values[key] })
	if err == nil || len(valid) != 3 {
		t.Fatal("invalid override was accepted or hid healthy app roots", valid, err)
	}
}

func TestLibraryCapturedSkillRestoresExactBytesWithoutSource(t *testing.T) {
	s, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := s.DefaultOwner()
	agent, _, err := s.EnrollSyncAgent(uid, "source", "test")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	skillDir := filepath.Join(root, "demo")
	if err = os.MkdirAll(filepath.Join(skillDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\r\nname: demo\r\ndescription: Captured bytes\r\n---\r\nExact instructions.\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(skillDir, "bin", "run"), []byte{0, 255, 3}, 0755); err != nil {
		t.Fatal(err)
	}
	expected, err := skillbundle.Scan(skillDir, skillbundle.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	// The integration package separately covers the production routes/auth.
	// Here the real ingestion store and production installer share exact bytes.
	mux.HandleFunc("POST /api/v2/library/locations", func(w http.ResponseWriter, r *http.Request) {
		var location store.LibraryLocation
		_ = json.NewDecoder(r.Body).Decode(&location)
		items, e := s.RegisterLibraryLocationFor(uid, agent.ID, location)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "items": items})
	})
	mux.HandleFunc("POST /api/v2/library/locations/content", func(w http.ResponseWriter, r *http.Request) {
		var in store.LibraryLocationImport
		_ = json.NewDecoder(r.Body).Decode(&in)
		result, e := s.ImportLibraryLocationFor(uid, agent.ID, in)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("GET /api/v2/skill-bundles/{bundle}/revisions/{revision}", func(w http.ResponseWriter, r *http.Request) {
		revision, e := s.PrivateSkillRevisionFor(uid, r.PathValue("bundle"), r.PathValue("revision"))
		if e != nil {
			http.Error(w, e.Error(), 404)
			return
		}
		_ = json.NewEncoder(w).Encode(revision)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	c := agentConfig{Server: server.URL, ID: agent.ID, Token: "disposable"}
	location := store.LibraryLocation{ID: strings.Repeat("a", 32), Application: "shared", Path: root}
	result := importLibraryLocations(context.Background(), c, []store.LibraryLocation{location}, nil, "test")
	if result.Imported != 1 || len(result.Issues) != 0 {
		t.Fatal(result)
	}
	again := importLibraryLocations(context.Background(), c, []store.LibraryLocation{location}, nil, "test")
	if again.Imported != 0 || again.Unchanged != 1 || len(again.Issues) != 0 {
		t.Fatal("unchanged content was recaptured", again)
	}
	items, err := s.LibraryFor(uid)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	item := items[0]
	if err = os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	source := "private:" + item.BundleID + ":" + item.RevisionID + ":" + item.Digest
	if err = installPrivateSkill(server.URL, c.Token, store.SyncSkill{Slug: "demo", Source: source, SourceType: "private"}, destination); err != nil {
		t.Fatal(err)
	}
	restored, err := skillbundle.Scan(filepath.Join(destination, "demo"), skillbundle.Limits{})
	if err != nil || restored.Digest != expected.Digest {
		t.Fatal("restore changed bytes or executable flags", restored.Digest, expected.Digest, err)
	}
	missing := importLibraryLocations(context.Background(), c, []store.LibraryLocation{location}, []string{root}, "test")
	if len(missing.Issues) != 1 || len(items) != 1 {
		t.Fatal("missing root was treated as content deletion", missing)
	}
	current, err := s.LibraryFor(uid)
	if err != nil || len(current) != 1 || current[0].RevisionID != item.RevisionID {
		t.Fatal("incomplete observation removed previous content", err)
	}
}
