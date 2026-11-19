package scan

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// A run of the agent leaves the process's working directory as it was, and
// an enrollment copied to another machine is refused.
func TestEnrolledAgentRunKeepsItsDirectoryAndRefusesACopiedEnrollment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REET_TOKEN", "")
	project := t.TempDir()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := httptest.NewServer(api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthNone)))
	defer srv.Close()
	cfg := filepath.Join(t.TempDir(), "agent.json")
	if err = RunAgent([]string{"enroll", "--server", srv.URL, "--env", "fixture", "--project", project, "--config", cfg, "--allow-apply", "--skip-terminal-setup"}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunAgent([]string{"run", "--once", "--config", cfg}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Getwd()
	if err != nil || after != before {
		t.Fatalf("agent run changed the process working directory: %q -> %q (%v)", before, after, err)
	}
	c, _ := loadAgentConfig(cfg)
	c.Hostname = "another-machine"
	writePrivate(cfg, c)
	if err = RunAgent([]string{"run", "--once", "--config", cfg}, io.Discard, "test"); err == nil {
		t.Fatal("copied enrollment accepted on another machine")
	}
}

// olderAgent serves h as a controller sees an agent from before the skills
// protocol.
func olderAgent(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-Reet-Skills-Protocol")
		h.ServeHTTP(w, r)
	})
}

func TestAgentDropsResultTheControllerRefuses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REET_TOKEN", "")
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	srv := httptest.NewServer(olderAgent(api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthNone))))
	defer srv.Close()
	cfg := filepath.Join(t.TempDir(), "agent.json")
	if err = RunAgent([]string{"enroll", "--server", srv.URL, "--env", "fixture", "--config", cfg, "--skip-terminal-setup"}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	agents, _ := st.SyncAgentsFor(uid)
	if len(agents) != 1 {
		t.Fatalf("enrollment: %+v", agents)
	}
	if err = os.WriteFile(cfg+".result", []byte(`{"id":"closed-job","status":"succeeded"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = RunAgent([]string{"run", "--once", "--config", cfg}, io.Discard, "test"); err == nil {
		t.Fatal("refused result reported success")
	}
	if _, err = os.Stat(cfg + ".result"); !os.IsNotExist(err) {
		t.Fatal("a result the controller will never take kept blocking the agent")
	}
	if _, err = st.QueueSyncJob(uid, agents[0].ID, "", "scan"); err != nil {
		t.Fatal(err)
	}
	if err = RunAgent([]string{"run", "--once", "--config", cfg}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	jobs, _ := st.SyncJobsFor(uid)
	if len(jobs) != 1 || jobs[0].Kind != "scan" || jobs[0].Status != "succeeded" {
		t.Fatalf("agent did not resume polling: %+v", jobs)
	}
}

// The restore command for an old sync job's backup names the exact backup: it
// puts that backup back where it was taken from, a project's folder included,
// and never over what is there now.
func TestAgentRestoreExactJobBackup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REET_TOKEN", "")
	project := t.TempDir()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	srv := httptest.NewServer(api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthNone)))
	defer srv.Close()
	cfg := filepath.Join(t.TempDir(), "agent.json")
	if err = RunAgent([]string{"enroll", "--server", srv.URL, "--env", "fixture", "--project", project, "--config", cfg, "--allow-apply", "--skip-terminal-setup"}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	c, _ := loadAgentConfig(cfg)
	target := store.EnvironmentIDFor(uid, "fixture", "claude", c.Projects[0])
	if err = st.TouchEnvironmentFor(uid, "fixture", "claude", c.Projects[0], filepath.Join(c.Projects[0], ".claude", "skills")); err != nil {
		t.Fatal(err)
	}
	if err = st.RecordInventoryFor(uid, target, true, store.MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	// The job's saved set is gone: the backup's own path says where it goes.
	backup := filepath.Join(c.Projects[0], ".claude", ".reet-sync-backups", "removed-20261101", "extra")
	writeSkill(t, backup)
	restore := func(args ...string) error {
		var out bytes.Buffer
		return runAgentRestore(append([]string{"--config", cfg}, args...), &out)
	}
	if err = restore("--target", target, "--backup", backup); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(c.Projects[0], ".claude", "skills", "extra", "SKILL.md")); err != nil {
		t.Fatal("the job backup was not restored to its project folder")
	}
	// Never over what is there now.
	writeSkill(t, backup)
	if err = restore("--target", target, "--backup", backup); err == nil {
		t.Fatal("restored over an existing skill")
	}
	if _, err = os.Stat(filepath.Join(backup, "SKILL.md")); err != nil {
		t.Fatal("a refused restore moved the backup")
	}
	// Only a backup in a backups folder of that target, named plainly.
	for _, bad := range []string{
		filepath.Join(c.Projects[0], ".claude", "skills", "extra"),
		filepath.Join(c.Projects[0], ".claude", ".reet-sync-backups", "removed-20261101", "..", "removed-20261101", "extra"),
		filepath.Join(t.TempDir(), ".reet-sync-backups", "removed-1", "extra"),
	} {
		if err = restore("--target", target, "--backup", bad); err == nil {
			t.Fatalf("restored from %s", bad)
		}
	}
	// Sync targets are gone: --target, still taken from commands copied
	// before, checks nothing, and the backup's own path says where it goes.
	other := filepath.Join(c.Projects[0], ".claude", ".reet-sync-backups", "removed-20261102", "other")
	writeSkill(t, other)
	if err = restore("--target", "some-other-target", "--backup", other); err != nil {
		t.Fatalf("restore with a target named: %v", err)
	}
	if _, err = os.Stat(filepath.Join(c.Projects[0], ".claude", "skills", "other", "SKILL.md")); err != nil {
		t.Fatal("the backup did not go back to the folder its path names")
	}
	// A backup beside no skills folder this machine has is refused plainly.
	stray := filepath.Join(t.TempDir(), "kit", ".reet-sync-backups", "removed-1", "extra")
	writeSkill(t, stray)
	if err = restore("--target", target, "--backup", stray); err == nil || !strings.Contains(err.Error(), "can't tell which skills folder") {
		t.Fatalf("a stray backup: %v", err)
	}
	// The name form still works.
	homeFolder := filepath.Join(c.Home, ".agents", "skills")
	writeSkill(t, filepath.Join(homeFolder, "alpha"))
	if _, err = backupSkill(homeFolder, "alpha", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = restore("alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(homeFolder, "alpha", "SKILL.md")); err != nil {
		t.Fatal("the name form did not restore")
	}
}
