package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/agentupdate"
	"github.com/eyupio/reet.sh/internal/store"
)

func releaseFixture(t *testing.T, dev, latest string) *agentupdate.Checker {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/dev/dev-build.txt", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(dev)) })
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": latest})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &agentupdate.Checker{Client: srv.Client(), ReleasesAPI: srv.URL + "/releases", DownloadsRoot: srv.URL, TTL: time.Minute}
}

func TestAgentBuildAndAutoUpdateVisibility(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const latestCommit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	srv := httptest.NewServer(New(st, deadFetcher(t), "tok", WithReleaseChecker(releaseFixture(t, latestCommit, "v1.4.2"))))
	defer srv.Close()
	call := func(method, path, body string, headers ...string) (int, string) {
		return doClient(t, srv.Client(), srv, method, path, body, append([]string{"X-Reet-Token", "tok"}, headers...)...)
	}

	code, body := call("POST", "/api/v2/sync/agents", `{"host":"laptop","version":"dev","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	if code != 201 {
		t.Fatalf("enroll: %d %s", code, body)
	}
	var enrolled struct {
		Agent  store.SyncAgent
		Secret string
	}
	if err = json.Unmarshal([]byte(body), &enrolled); err != nil {
		t.Fatal(err)
	}
	if enrolled.Agent.Commit != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || !enrolled.Agent.AutoUpdate {
		t.Fatalf("enrollment did not keep the build and auto-update default: %+v", enrolled.Agent)
	}

	code, body = call("GET", "/api/v2/sync", "")
	var state struct {
		Agents   []store.SyncAgent
		Releases agentupdate.Info
	}
	if err = json.Unmarshal([]byte(body), &state); err != nil {
		t.Fatal(err)
	}
	if code != 200 || len(state.Agents) != 1 || state.Agents[0].Commit != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("build not visible in sync state: %d %s", code, body)
	}
	if state.Releases.Dev.Commit != latestCommit || state.Releases.Latest.Version != "v1.4.2" {
		t.Fatalf("channels not exposed: %+v", state.Releases)
	}

	code, body = call("POST", "/api/v2/sync/poll", `{}`, "X-Reet-Agent", enrolled.Agent.ID, "X-Reet-Agent-Key", enrolled.Secret, "X-Reet-Version", "dev", "X-Reet-Commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if code != 200 {
		t.Fatalf("poll: %d %s", code, body)
	}
	var polled struct {
		Agent store.SyncAgent
		Job   *store.SyncJob
	}
	if err = json.Unmarshal([]byte(body), &polled); err != nil {
		t.Fatal(err)
	}
	if polled.Job == nil || polled.Job.Kind != "update" || polled.Job.Revision != "dev" {
		t.Fatalf("outdated dev build did not receive an update job: %s", body)
	}

	code, body = call("PATCH", "/api/v2/sync/agents", `{"id":"`+enrolled.Agent.ID+`","action":"auto-update-off"}`)
	if code != 200 {
		t.Fatalf("disable auto-update: %d %s", code, body)
	}
	if err = json.Unmarshal([]byte(body), &state); err != nil {
		t.Fatal(err)
	}
	if state.Agents[0].AutoUpdate {
		t.Fatal("auto-update stayed enabled")
	}
}

func TestAutoUpdateLeavesNewerReleaseBuildsAlone(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := httptest.NewServer(New(st, deadFetcher(t), "tok", WithReleaseChecker(releaseFixture(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "v1.4.2"))))
	defer srv.Close()
	call := func(method, path, body string, headers ...string) (int, string) {
		return doClient(t, srv.Client(), srv, method, path, body, append([]string{"X-Reet-Token", "tok"}, headers...)...)
	}
	code, body := call("POST", "/api/v2/sync/agents", `{"host":"server","version":"v1.5.0","commit":"cccccccccccccccccccccccccccccccccccccccc"}`)
	if code != 201 {
		t.Fatalf("enroll: %d %s", code, body)
	}
	var enrolled struct {
		Agent  store.SyncAgent
		Secret string
	}
	json.Unmarshal([]byte(body), &enrolled)
	code, body = call("POST", "/api/v2/sync/poll", `{}`, "X-Reet-Agent", enrolled.Agent.ID, "X-Reet-Agent-Key", enrolled.Secret, "X-Reet-Version", "v1.5.0")
	if code != 200 {
		t.Fatalf("poll: %d %s", code, body)
	}
	var polled struct{ Job *store.SyncJob }
	json.Unmarshal([]byte(body), &polled)
	if polled.Job != nil {
		t.Fatalf("newer local build was downgraded: %+v", polled.Job)
	}
}
