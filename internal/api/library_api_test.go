package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// The canonical library's own routes keep an item's status behind its
// version, keep it across publishing, and stay private to the account.
func TestCanonicalLibraryAPIPreservesAccountAndLifecycleBoundaries(t *testing.T) {
	e := newReconcileEnv(t)
	e.writtenSkill(t, "lifecycle")
	items, err := e.st.LibraryFor(e.uid)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	item := items[0]
	patch := func(status string, version int64) int {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"status": status, "expectedVersion": version})
		code, _ := e.owner(t, http.MethodPatch, "/api/v2/library/"+item.ID, string(body))
		return code
	}
	if code := patch("archived", item.Version); code != 200 {
		t.Fatal("canonical lifecycle patch failed", code)
	}
	if code := patch("active", item.Version); code != 409 {
		t.Fatal("stale lifecycle patch accepted", code)
	}
	// A first publication, repeated save and unpublication each keep an
	// Archived status.
	hosted, err := e.st.HostedSkillsFor(e.uid)
	if err != nil || len(hosted) != 1 {
		t.Fatal(hosted, err)
	}
	for _, published := range []bool{true, true, false} {
		body, _ := json.Marshal(store.HostedSkill{Content: hosted[0].Content, Published: published})
		if code, raw := e.owner(t, http.MethodPut, "/api/v2/hosting/skills/"+hosted[0].Slug, string(body)); code != 200 {
			t.Fatalf("save: %d %s", code, raw)
		}
		current, err := e.st.LibraryFor(e.uid)
		if err != nil || len(current) != 1 || current[0].ID != item.ID || current[0].Status != store.SkillArchived || current[0].Published != published {
			t.Fatal("visibility reset canonical lifecycle", current, err)
		}
	}
	response, err := http.Get(e.srv.URL + "/api/v2/library")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == 200 {
		t.Fatal("anonymous request read private library")
	}
	viewer, err := e.st.CreateUser("viewer-library@example.com", "Reader", "viewer", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.LibraryFor(viewer.ID); err == nil {
		t.Fatal("viewer obtained private library access")
	}
	other, err := e.st.CreateUser("other-library@example.com", "Other", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.LibraryItemFor(other.ID, item.ID); err == nil {
		t.Fatal("other account read private item")
	}
}

// Typed content with no SKILL.md, such as an agent definition, is saved and
// listed in the canonical library.
func TestCanonicalLibraryTypedPrivateAPI(t *testing.T) {
	e := newReconcileEnv(t)
	payload, _ := json.Marshal(map[string]any{
		"name": "review", "requestId": "typed_api_review_0001", "expectedBase": "",
		"itemType": "agent_definition", "format": "markdown", "application": "claude",
		"files": []map[string]any{{"path": "review.md", "data": []byte("---\nname: review\ndescription: Review code\n---\nInspect the diff.\n")}},
	})
	code, body := e.owner(t, http.MethodPost, "/api/v2/skill-bundles", string(payload))
	if code != 200 {
		t.Fatalf("typed agent content without SKILL.md was rejected: %d %s", code, body)
	}
	var library struct {
		Items []map[string]any `json:"items"`
	}
	code, body = e.owner(t, http.MethodGet, "/api/v2/library", "")
	if code != 200 || json.Unmarshal([]byte(body), &library) != nil || len(library.Items) != 1 || library.Items[0]["itemType"] != "agent_definition" || library.Items[0]["status"] != "active" {
		t.Fatalf("typed capture missing from canonical library: %d %s", code, body)
	}
}
