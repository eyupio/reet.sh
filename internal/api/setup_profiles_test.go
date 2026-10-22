package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/setup"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestSetupProfileAPI(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := httptest.NewServer(New(st, deadFetcher(t), "tok"))
	defer srv.Close()
	profile := setup.SetupProfile{
		SchemaVersion: 1, Name: "work",
		Revisions: []setup.SetupProfileRevision{{
			Revision: 1, CreatedAt: time.Now().UTC(), SnapshotID: "setup-20261022T120000Z-abcdef01",
			SnapshotDigest: "digest", TakenAt: time.Now().UTC(),
			Preview: setup.SnapshotPreview{SchemaVersion: 1, OS: "linux", Architecture: "amd64"},
		}},
	}
	body, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	code, response := doClient(t, srv.Client(), srv, "PUT", "/api/v2/setup/profiles/work", string(body), "X-Reet-Token", "tok")
	if code != 200 || response == "" {
		t.Fatalf("save: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "GET", "/api/v2/setup/profiles", "", "X-Reet-Token", "tok")
	if code != 200 || !containsAll(response, `"work"`, `"revision":1`, `"setup-20261022T120000Z-abcdef01"`) {
		t.Fatalf("list: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "GET", "/api/v2/setup/profiles/work", "", "X-Reet-Token", "tok")
	if code != 200 {
		t.Fatalf("get: %d %s", code, response)
	}
	var got setup.SetupProfile
	if err := json.Unmarshal([]byte(response), &got); err != nil || got.Name != "work" || len(got.Revisions) != 1 {
		t.Fatalf("profile payload: %+v %v", got, err)
	}
	for _, tc := range []struct {
		method, path, body, want string
	}{
		{"PUT", "/api/v2/setup/profiles/bad name", string(body), "profile names"},
		{"PUT", "/api/v2/setup/profiles/work", `{"schema_version":1,"name":"work"}`, "incomplete"},
		{"GET", "/api/v2/setup/profiles/missing", "", "not found"},
	} {
		code, response = doClient(t, srv.Client(), srv, tc.method, tc.path, tc.body, "X-Reet-Token", "tok")
		if code < 400 || !containsAll(response, tc.want) {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, code, response)
		}
	}
	other := profile
	other.Name = "other"
	otherBody, err := json.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	code, response = doClient(t, srv.Client(), srv, "PUT", "/api/v2/setup/profiles/other", string(otherBody), "X-Reet-Token", "tok")
	if code != 200 {
		t.Fatalf("save other: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "PATCH", "/api/v2/setup/profiles/work", `{"name":"personal"}`, "X-Reet-Token", "tok")
	if code != 200 || !containsAll(response, `"name":"personal"`) {
		t.Fatalf("rename: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "GET", "/api/v2/setup/profiles/personal", "", "X-Reet-Token", "tok")
	if code != 200 || !containsAll(response, `"name":"personal"`) {
		t.Fatalf("get renamed: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "GET", "/api/v2/setup/profiles/work", "", "X-Reet-Token", "tok")
	if code != 404 {
		t.Fatalf("old name still exists: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "PATCH", "/api/v2/setup/profiles/personal", `{"name":"other"}`, "X-Reet-Token", "tok")
	if code != 409 || !containsAll(response, "already exists") {
		t.Fatalf("rename conflict: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "PATCH", "/api/v2/setup/profiles/personal", `{"name":"bad name"}`, "X-Reet-Token", "tok")
	if code != 400 || !containsAll(response, "profile names") {
		t.Fatalf("rename invalid name: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "PATCH", "/api/v2/setup/profiles/personal", `{"overrides":{"exclude_kinds":["memory"],"exclude_sources":["/project"]}}`, "X-Reet-Token", "tok")
	if code != 200 {
		t.Fatalf("override edit: %d %s", code, response)
	}
	var edited setup.SetupProfile
	if err := json.Unmarshal([]byte(response), &edited); err != nil || len(edited.Revisions) != 2 || edited.Revisions[1].Overrides == nil || edited.Revisions[1].SnapshotID != edited.Revisions[0].SnapshotID || !containsAll(response, "memory") {
		t.Fatalf("override edit payload: %+v %v", edited, err)
	}
	code, response = doClient(t, srv.Client(), srv, "PATCH", "/api/v2/setup/profiles/personal", `{"overrides":{"exclude_kinds":["bogus"]}}`, "X-Reet-Token", "tok")
	if code != 400 || !containsAll(response, "excluded kind") {
		t.Fatalf("invalid override: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "PATCH", "/api/v2/setup/profiles/personal", `{"name":"other","overrides":{}}`, "X-Reet-Token", "tok")
	if code != 400 || !containsAll(response, "not both") {
		t.Fatalf("combined edit: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "DELETE", "/api/v2/setup/profiles/personal", "", "X-Reet-Token", "tok")
	if code != 200 {
		t.Fatalf("delete: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "DELETE", "/api/v2/setup/profiles/other", "", "X-Reet-Token", "tok")
	if code != 200 {
		t.Fatalf("delete other: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "GET", "/api/v2/setup/profiles/personal", "", "X-Reet-Token", "tok")
	if code != 404 {
		t.Fatalf("get after delete: %d %s", code, response)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
