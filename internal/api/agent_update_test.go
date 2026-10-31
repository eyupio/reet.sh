package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/agentupdate"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
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
	st, err := storetest.Open(t)
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
	st, err := storetest.Open(t)
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

func TestAnAgentNarratesItsUpdateAndARollbackSettlesIt(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := httptest.NewServer(New(st, deadFetcher(t), "tok", WithReleaseChecker(releaseFixture(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "v1.4.2"))))
	defer srv.Close()
	call := func(method, path, body string, headers ...string) (int, string) {
		return doClient(t, srv.Client(), srv, method, path, body, append([]string{"X-Reet-Token", "tok"}, headers...)...)
	}
	code, body := call("POST", "/api/v2/sync/agents", `{"host":"laptop","version":"v1.0.0"}`)
	if code != 201 {
		t.Fatalf("enroll: %d %s", code, body)
	}
	var enrolled struct {
		Agent  store.SyncAgent
		Secret string
	}
	json.Unmarshal([]byte(body), &enrolled)
	agentHeaders := []string{"X-Reet-Agent", enrolled.Agent.ID, "X-Reet-Agent-Key", enrolled.Secret, "X-Reet-Version", "v1.0.0"}

	code, body = call("POST", "/api/v2/sync/poll", `{}`, agentHeaders...)
	if code != 200 {
		t.Fatalf("poll: %d %s", code, body)
	}
	var polled struct{ Job *store.SyncJob }
	json.Unmarshal([]byte(body), &polled)
	if polled.Job == nil || polled.Job.Kind != "update" || polled.Job.Stage != store.UpdateStageDownload || len(polled.Job.Stages) == 0 {
		t.Fatalf("update job has no timeline: %s", body)
	}

	code, body = call("POST", "/api/v2/sync/agents/update/stage", `{"id":"`+polled.Job.ID+`","stage":"verify","detail":"Checking the downloaded build"}`, agentHeaders...)
	if code != 200 {
		t.Fatalf("stage: %d %s", code, body)
	}
	code, body = call("POST", "/api/v2/sync/agents/update/stage", `{"id":"`+polled.Job.ID+`","stage":"invented"}`, agentHeaders...)
	if code != 409 {
		t.Fatalf("unknown stage accepted: %d %s", code, body)
	}
	code, body = call("POST", "/api/v2/sync/agents/update/stage", `{"id":"`+polled.Job.ID+`","stage":"restart"}`, "X-Reet-Agent", enrolled.Agent.ID, "X-Reet-Agent-Key", "wrong", "X-Reet-Version", "v1.0.0")
	if code != 403 {
		t.Fatalf("a bad key wrote a stage: %d %s", code, body)
	}

	code, body = call("POST", "/api/v2/sync/result", `{"id":"`+polled.Job.ID+`","status":"rolled_back","message":"The new build did not report in; the previous binary is running."}`, agentHeaders...)
	if code != 200 {
		t.Fatalf("result: %d %s", code, body)
	}
	code, body = call("GET", "/api/v2/sync", "")
	var state struct {
		Jobs       []store.SyncJob
		UpdateJobs []store.SyncJob
	}
	json.Unmarshal([]byte(body), &state)
	if code != 200 || len(state.Jobs) != 1 || state.Jobs[0].Status != "rolled_back" || state.Jobs[0].Stage != store.UpdateStageRollback {
		t.Fatalf("rolled-back update not visible: %d %s", code, body)
	}
	if len(state.UpdateJobs) != 1 || state.UpdateJobs[0].ID != polled.Job.ID {
		t.Fatalf("latest update not kept beside the job list: %s", body)
	}
	if state.Jobs[0].Stages[len(state.Jobs[0].Stages)-1].Key != store.UpdateStageRollback {
		t.Fatalf("rollback step missing: %+v", state.Jobs[0].Stages)
	}

	code, body = call("POST", "/api/v2/sync/poll", `{}`, agentHeaders...)
	if code != 200 {
		t.Fatalf("poll after rollback: %d %s", code, body)
	}
	json.Unmarshal([]byte(body), &polled)
	if polled.Job != nil {
		t.Fatalf("auto-update retried immediately after a rollback: %+v", polled.Job)
	}
}
