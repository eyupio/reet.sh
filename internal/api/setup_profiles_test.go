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
	code, response = doClient(t, srv.Client(), srv, "DELETE", "/api/v2/setup/profiles/work", "", "X-Reet-Token", "tok")
	if code != 200 {
		t.Fatalf("delete: %d %s", code, response)
	}
	code, response = doClient(t, srv.Client(), srv, "GET", "/api/v2/setup/profiles/work", "", "X-Reet-Token", "tok")
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
