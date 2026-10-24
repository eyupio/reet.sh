package scan

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestAnAgentReadsTheBuildItDownloaded(t *testing.T) {
	version, commit, ok := parseAgentBuild("reet v1.4.2 (0123456789abcdef0123456789abcdef01234567)\n")
	if !ok || version != "v1.4.2" || commit != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("build: %q %q %t", version, commit, ok)
	}
	if _, _, ok := parseAgentBuild("reet built from source"); ok {
		t.Fatal("unreadable build accepted")
	}
}

func TestAPendingUpdateMatchesTheBuildItInstalled(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	stable := pendingUpdate{To: "v1.4.2"}
	if !buildMatches(stable, "v1.4.2", "") || buildMatches(stable, "v1.4.1", "") {
		t.Fatal("stable build identity")
	}
	dev := pendingUpdate{To: "dev", Commit: commit}
	if !buildMatches(dev, "dev", commit) || buildMatches(dev, "dev", "ffffffffffffffffffffffffffffffffffffffff") {
		t.Fatal("dev build identity")
	}
	if !buildMatches(dev, "dev", "unknown") {
		t.Fatal("an unlabelled dev build cannot be told apart; the version must stand")
	}
	if !buildMatches(pendingUpdate{To: "dev"}, "dev", "anything") {
		t.Fatal("an older marker without a commit must still match")
	}
}

func TestAnUpdateThatDoesNotTakeSaysSoAndLeavesNoMarker(t *testing.T) {
	config := filepath.Join(t.TempDir(), "agent.json")
	pending := pendingUpdate{JobID: "job-1", From: "v1.0.0", To: "v1.1.0", Target: "/tmp/reet", Previous: "/tmp/reet.previous"}
	if err := savePendingUpdate(config, pending); err != nil {
		t.Fatal(err)
	}
	handled, err := checkPendingUpdate(agentConfig{}, config, "v1.0.0")
	if err != nil || !handled {
		t.Fatalf("check: %t %v", handled, err)
	}
	if _, ok := loadPendingUpdate(config); ok {
		t.Fatal("stale marker kept")
	}
	var result agentResult
	b, err := os.ReadFile(config + ".result")
	if err != nil || json.Unmarshal(b, &result) != nil {
		t.Fatal(err)
	}
	if result.ID != "job-1" || result.Status != "failed" {
		t.Fatalf("result: %+v", result)
	}
}

func TestAnAgentReportsTheStageItIsOn(t *testing.T) {
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/sync/agents/update/stage" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Reet-Agent") != "agent-1" || r.Header.Get("X-Reet-Agent-Key") != "secret" {
			t.Errorf("enrollment headers: %v", r.Header)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	reportAgentStage(agentConfig{Server: srv.URL, Token: "tok", ID: "agent-1", Secret: "secret"}, "job-1", store.UpdateStageVerify, "Checking the build", "v1.0.0")
	if body["id"] != "job-1" || body["stage"] != store.UpdateStageVerify || body["detail"] != "Checking the build" {
		t.Fatalf("stage body: %+v", body)
	}
}

func TestAnUpdateRollsBackWhenRefusedOrAfterTheGraceWindow(t *testing.T) {
	started := time.Now().Add(-time.Minute).UnixMilli()
	if !shouldRollback(errors.New("controller 403 Forbidden: machine enrollment is missing or revoked"), started, time.Now()) {
		t.Fatal("a refusal must roll back at once")
	}
	if shouldRollback(errors.New("dial tcp 10.0.0.1:443: connect: connection refused"), started, time.Now()) {
		t.Fatal("a quiet controller rolled back inside the grace window")
	}
	if !shouldRollback(errors.New("context deadline exceeded"), started, time.Now().Add(updateHealthWindow)) {
		t.Fatal("a controller quiet for the whole window did not roll back")
	}
}

func TestUndoPutThePreviousBinaryBackAndForgotTheMarker(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "agent.json")
	target := filepath.Join(dir, "reet")
	previous := target + ".previous"
	if err := os.WriteFile(target, []byte("new"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(previous, []byte("old"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := savePendingUpdate(config, pendingUpdate{JobID: "job-1", To: "v1.1.0", Target: target, Previous: previous}); err != nil {
		t.Fatal(err)
	}
	undoPendingUpdate(config)
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Fatalf("target: %q", b)
	}
	if _, ok := loadPendingUpdate(config); ok {
		t.Fatal("marker kept")
	}
}

func TestANewBuildThatReachesTheControllerReportsSuccess(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"agent-1"}`))
	}))
	defer srv.Close()
	config := filepath.Join(t.TempDir(), "agent.json")
	if err := savePendingUpdate(config, pendingUpdate{JobID: "job-1", From: "v1.0.0", To: "v1.1.0", Target: "/tmp/reet", Previous: "/tmp/reet.previous"}); err != nil {
		t.Fatal(err)
	}
	c := agentConfig{Server: srv.URL, Token: "tok", ID: "agent-1", Secret: "secret"}
	handled, err := checkPendingUpdate(c, config, "v1.1.0")
	if err != nil || !handled {
		t.Fatalf("check: %t %v", handled, err)
	}
	if _, ok := loadPendingUpdate(config); ok {
		t.Fatal("marker kept after a healthy restart")
	}
	var result agentResult
	b, err := os.ReadFile(config + ".result")
	if err != nil || json.Unmarshal(b, &result) != nil {
		t.Fatal(err)
	}
	if result.ID != "job-1" || result.Status != "succeeded" {
		t.Fatalf("result: %+v", result)
	}
	if len(paths) != 2 || paths[0] != "/api/v2/sync/agents/update/stage" || paths[1] != "/api/v2/sync/identity" {
		t.Fatalf("paths: %v", paths)
	}
}
