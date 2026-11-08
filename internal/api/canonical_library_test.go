package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestCanonicalLibraryCLIImportsLocationsWithSourceOffline(t *testing.T) {
	h := newSyncContractHarness(t)
	m := h.machine("library-source")
	m.localSkill("twins", "shared copy")
	custom := filepath.Join(m.home, "custom-skills")
	project := filepath.Join(m.home, "project")
	override := filepath.Join(m.home, "claude-override")
	writeSkill := func(root, name, body string) {
		t.Helper()
		dir := filepath.Join(root, syncContractName(name))
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(syncContractContent(name, body)), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", "run"), []byte{0, 255, 1}, 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill(custom, "twins", "different custom copy")
	writeSkill(filepath.Join(project, ".agents", "skills"), "project-copy", "project copy")
	writeSkill(filepath.Join(override, "skills"), "overridden-copy", "app override")
	bad := filepath.Join(custom, "broken")
	if err := os.MkdirAll(bad, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("invalid skill metadata"), 0644); err != nil {
		t.Fatal(err)
	}
	// Preserve the private enrollment while adding an explicitly tracked project.
	data, err := os.ReadFile(m.config)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err = json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	config["projects"] = []string{project}
	data, _ = json.Marshal(config)
	if err = os.WriteFile(m.config, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := m.connectorCommand(ctx, "library-import", "--config", m.config, "--folder", custom)
	command.Env = append(command.Env, "CLAUDE_CONFIG_DIR="+override)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("malformed skill should produce a partial-capture exit")
	}
	if !strings.Contains(string(output), `"imported":4`) || !strings.Contains(string(output), `"import_failed"`) {
		t.Fatalf("healthy imports did not finish independently: %s", output)
	}
	var library struct {
		Items []store.LibraryItem `json:"items"`
	}
	raw := h.mustRequest(http.MethodGet, "/api/v2/library", nil)
	if err = json.Unmarshal([]byte(raw), &library); err != nil {
		t.Fatal(err)
	}
	if len(library.Items) != 4 {
		t.Fatalf("want four independent complete imports, got %d", len(library.Items))
	}
	twins := 0
	m.stop()
	m.link.offline.Store(true)
	for _, item := range library.Items {
		if item.Name == syncContractName("twins") {
			twins++
		}
		if item.Status != store.LibraryToTest || item.RevisionID == "" {
			t.Fatal("new capture was activated or incomplete", item)
		}
		var revision store.PrivateSkillRevision
		body := h.mustRequest(http.MethodGet, "/api/v2/skill-bundles/"+item.BundleID+"/revisions/"+item.RevisionID, nil)
		if err = json.Unmarshal([]byte(body), &revision); err != nil {
			t.Fatal(err)
		}
		if revision.ManifestDigest != item.Digest || len(revision.Files) != 2 {
			t.Fatal("source-offline content was unavailable", item)
		}
		if item.Name != syncContractName("twins") || strings.Contains(string(revision.Files[0].Data), "different custom copy") {
			if revision.Files[1].Path != "bin/run" || !revision.Files[1].Executable || string(revision.Files[1].Data) != string([]byte{0, 255, 1}) {
				t.Fatal("support bytes/executable flag changed", revision.Files[1].Path)
			}
		}
	}
	if twins != 2 {
		t.Fatal("same-name imports were collapsed")
	}
	targets, err := h.st.SyncTargetsFor(h.uid)
	if err != nil || len(targets) != 0 {
		t.Fatal("capture created deployment intent", targets, err)
	}
}

func TestCanonicalLibraryAPIPreservesAccountAndLifecycleBoundaries(t *testing.T) {
	h := newSyncContractHarness(t)
	h.hosted("lifecycle", "owned private contents", false, false)
	items, err := h.st.LibraryFor(h.uid)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	item := items[0]
	code, _ := h.request(http.MethodPatch, "/api/v2/library/"+item.ID, map[string]any{"status": "disabled", "expectedVersion": item.Version})
	if code != 200 {
		t.Fatal("canonical lifecycle patch failed", code)
	}
	code, _ = h.request(http.MethodPatch, "/api/v2/library/"+item.ID, map[string]any{"status": "in_use", "expectedVersion": item.Version})
	if code != 409 {
		t.Fatal("stale lifecycle patch accepted", code)
	}
	// A first publication, repeated save and unpublication must each retain an
	// existing Disabled stage, even when they create/show the legacy shelf row.
	for _, published := range []bool{true, true, false} {
		h.mustRequest(http.MethodPut, "/api/v2/hosting/skills/reet-lifecycle", store.HostedSkill{Content: syncContractContent("lifecycle", "updated private contents"), Published: published})
		current, err := h.st.LibraryFor(h.uid)
		if err != nil || len(current) != 1 || current[0].ID != item.ID || current[0].Status != store.LibraryDisabled || current[0].Published != published {
			t.Fatal("visibility reset canonical lifecycle", current, err)
		}
	}
	request, err := http.NewRequest(http.MethodGet, h.controller.URL+"/api/v2/library", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == 200 {
		t.Fatal("anonymous request read private library")
	}
	viewer, err := h.st.CreateUser("viewer-library@example.com", "Reader", "viewer", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.st.LibraryFor(viewer.ID); err == nil {
		t.Fatal("viewer obtained private library access")
	}
	other, err := h.st.CreateUser("other-library@example.com", "Other", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.st.LibraryItemFor(other.ID, item.ID); err == nil {
		t.Fatal("other account read private item")
	}
}

func TestCanonicalLibraryTypedPrivateAPI(t *testing.T) {
	h := newSyncContractHarness(t)
	payload := map[string]any{
		"name": "review", "requestId": "typed_api_review_0001", "expectedBase": "",
		"itemType": "agent_definition", "format": "markdown", "application": "claude",
		"files": []map[string]any{{"path": "review.md", "data": []byte("---\nname: review\ndescription: Review code\n---\nInspect the diff.\n")}},
	}
	code, body := h.request(http.MethodPost, "/api/v2/skill-bundles", payload)
	if code != 200 {
		t.Fatalf("typed agent content without SKILL.md was rejected: %d %s", code, body)
	}
	var library struct {
		Items []map[string]any `json:"items"`
	}
	code, body = h.request(http.MethodGet, "/api/v2/library", nil)
	if code != 200 || json.Unmarshal([]byte(body), &library) != nil || len(library.Items) != 1 || library.Items[0]["itemType"] != "agent_definition" || library.Items[0]["status"] != "to_test" {
		t.Fatalf("typed capture missing from canonical library: %d %s", code, body)
	}
}

func TestCanonicalLibraryCLIInvalidOverrideKeepsHealthyImports(t *testing.T) {
	h := newSyncContractHarness(t)
	m := h.machine("partial-configuration")
	m.localSkill("healthy-root", "still imported")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := m.connectorCommand(ctx, "library-import", "--config", m.config)
	command.Env = append(command.Env, "CLAUDE_CONFIG_DIR=relative-invalid-folder")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), `"imported":1`) || !strings.Contains(string(output), `"location_configuration_invalid"`) {
		t.Fatalf("invalid app configuration blocked healthy imports: %s", output)
	}
	items, err := h.st.LibraryFor(h.uid)
	if err != nil || len(items) != 1 || items[0].Name != syncContractName("healthy-root") || items[0].RevisionID == "" {
		t.Fatal("healthy capture unavailable", items, err)
	}
}
