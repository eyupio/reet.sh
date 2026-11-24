//go:build linux || darwin

package scan

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func lock(t *testing.T, config string) {
	t.Helper()
	c := loaded(t, config)
	c.Locked = true
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
}

// A locked machine refuses the controller's update job with the sentence,
// even in the long-running agent that could restart, and keeps scanning.
func TestALockedMachineRefusesUpdatesAndKeepsScanning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REET_TOKEN", "")
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	srv := httptest.NewServer(api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthNone)))
	defer srv.Close()
	config := filepath.Join(t.TempDir(), "agent.json")
	if err = RunAgent([]string{"enroll", "--server", srv.URL, "--env", "fixture", "--project", t.TempDir(), "--config", config, "--skip-terminal-setup"}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	lock(t, config)
	agents, _ := st.SyncAgentsFor(uid)
	if _, err = st.QueueAgentUpdate(uid, agents[0].ID, "latest"); err != nil {
		t.Fatal(err)
	}
	// The agent was started before the lock: the cycle reads it again.
	started := loaded(t, config)
	started.Locked = false
	last := time.Now()
	if err = agentCycle(context.Background(), started, config, io.Discard, "test", &last, true); err != nil {
		t.Fatal(err)
	}
	jobs, _ := st.SyncJobsFor(uid)
	if len(jobs) != 1 || jobs[0].Kind != "update" || jobs[0].Status != "failed" || jobs[0].Message != terminalproto.LockedSentence {
		t.Fatalf("update job: %+v", jobs)
	}
	envs, _ := st.SyncEnvironmentsFor(uid)
	before := len(envs)
	last = time.Time{}
	if err = agentCycle(context.Background(), started, config, io.Discard, "test", &last, true); err != nil {
		t.Fatal(err)
	}
	if envs, _ = st.SyncEnvironmentsFor(uid); len(envs) == 0 || len(envs) < before || last.IsZero() {
		t.Fatalf("a locked machine stopped its inventory scan: %d environments", len(envs))
	}
}

// Skills approved for this machine are not installed while it is locked,
// even with allow-apply on; the report says why.
func TestALockedMachineInstallsNoSkills(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	lock(t, config)
	home := loaded(t, config).Home
	shared := filepath.Join(home, ".agents", "skills")
	a, aFiles := skillBundle(t, "alpha", "first")
	_, old := skillBundle(t, "alpha", "old")
	placeFiles(t, filepath.Join(shared, "alpha"), old)
	a.Bundle, a.Revision = strings.Repeat("1", 32), strings.Repeat("2", 32)
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}})
	before := homeTree(t, shared)
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	sameTree(t, "skills", before, homeTree(t, shared))
	if _, _, _, fetches := srv.counts(); fetches != 0 {
		t.Fatalf("fetched %d bundles while locked", fetches)
	}
	rep := srv.lastReport(t)
	if rep.Blocked != "locked" {
		t.Fatalf("report %+v", rep)
	}
	if e := entryOf(t, rep, "~/.agents/skills", "alpha"); e.State != "failed" || e.Detail != terminalproto.LockedSentence {
		t.Fatalf("entry %+v", e)
	}
}

// A locked machine reads no folder for a remote import: it tells the
// controller why, and the cycle goes on to poll.
func TestALockedMachineRefusesARemoteImport(t *testing.T) {
	instance := strings.Repeat("a", 48)
	var failures []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/sync/skill-import/work":
			work := &store.RemoteImport{ID: "locked-fixture", Agent: "agents", Name: "demo", Instance: instance, State: "previewing", ExpiresAt: time.Now().Add(time.Minute).UnixMilli()}
			json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "work": work})
		case "/api/v2/sync/skill-import/failure":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			failures = append(failures, body)
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		default:
			t.Errorf("a locked machine asked for %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	handled, err := processRemoteImportWith(context.Background(), agentConfig{Home: "/unavailable", Server: srv.URL, Locked: true}, "test", instance)
	if err != nil || handled {
		t.Fatal(handled, err)
	}
	if len(failures) != 1 || failures[0]["code"] != "machine_locked" || failures[0]["id"] != "locked-fixture" {
		t.Fatalf("failures: %v", failures)
	}
}
