package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/setup"
)

func testProfile() setup.SetupProfile {
	return setup.SetupProfile{SchemaVersion: 1, Name: "work", Revisions: []setup.SetupProfileRevision{{
		Revision: 1, CreatedAt: time.Now().UTC(), SnapshotID: "setup-20261022T120000Z-abcdef01",
		SnapshotDigest: "digest", TakenAt: time.Now().UTC(),
		Preview: setup.SnapshotPreview{SchemaVersion: 1, OS: "linux", Architecture: "amd64"},
	}}}
}

func TestProfileRemotePushAndPull(t *testing.T) {
	profile := testProfile()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Reet-Token") != "tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPut:
			if r.URL.Path != "/api/v2/setup/profiles/work" {
				http.NotFound(w, r)
				return
			}
			var got setup.SetupProfile
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got.Name != "work" {
				http.Error(w, "bad profile", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if r.URL.Path != "/api/v2/setup/profiles/work" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(profile)
		default:
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	remote := profileRemote{}
	if err := remote.Push(server.URL, "tok", profile); err != nil {
		t.Fatal(err)
	}
	got, err := remote.Pull(server.URL, "tok", "work")
	if err != nil || got.Name != "work" || len(got.Revisions) != 1 {
		t.Fatalf("pull: %+v %v", got, err)
	}
	if err := remote.Push(server.URL, "wrong", profile); err == nil {
		t.Fatal("unauthorized push accepted")
	}
	if _, err := remote.Pull("", "tok", "work"); err == nil {
		t.Fatal("missing server accepted")
	}
}
