package scan

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestEnrolledAgentPreviewApprovalApply(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REET_TOKEN", "")
	project := t.TempDir()
	mkSkill(t, filepath.Join(project, ".claude", "skills"), "extra", true)
	if err := os.WriteFile(filepath.Join(project, "skills-lock.json"), []byte(`{"skills":{"extra":{"source":"o/r"},"beta":{"source":"o/r"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "npx"), []byte("#!/bin/sh\nmkdir -p .claude/skills/beta\nprintf '%s\\n' '# Beta' > .claude/skills/beta/SKILL.md\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	srv := httptest.NewServer(api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthNone)))
	defer srv.Close()
	cfg := filepath.Join(t.TempDir(), "agent.json")
	if err = RunAgent([]string{"enroll", "--server", srv.URL, "--env", "fixture", "--project", project, "--config", cfg, "--allow-apply"}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	if err = RunAgent([]string{"run", "--once", "--config", cfg}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	rf, _ := ref.Parse("o/r/beta")
	st.UpsertFor(uid, rf, store.Patch{})
	target := store.EnvironmentIDFor(uid, "fixture", "claude", project)
	if err = st.SaveDesiredFor(uid, "owner", store.DesiredSelection{TargetID: target, SourceID: "shelf", SkillIDs: []string{rf.ID}, Mode: "mirror"}); err != nil {
		t.Fatal(err)
	}
	agents, _ := st.SyncAgentsFor(uid)
	j, err := st.QueueSyncJob(uid, agents[0].ID, target, "preview")
	if err != nil {
		t.Fatal(err)
	}
	run := func() {
		t.Helper()
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
	}
	run()
	jobs, _ := st.SyncJobsFor(uid)
	if jobs[0].Status != "awaiting-approval" {
		t.Fatalf("preview: %+v", jobs)
	}
	if _, err = os.Stat(filepath.Join(project, ".claude", "skills", "beta")); !os.IsNotExist(err) {
		t.Fatal("preview installed files")
	}
	if err = st.ControlSyncJob(uid, j.ID, "approve", jobs[0].Hash); err != nil {
		t.Fatal(err)
	}
	run()
	jobs, _ = st.SyncJobsFor(uid)
	if jobs[0].Status != "succeeded" {
		t.Fatalf("apply: %+v", jobs)
	}
	if _, err = os.Stat(filepath.Join(project, ".claude", "skills", "beta", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(filepath.Join(project, ".claude", ".reet-sync-backups", "removed-*", "extra", "SKILL.md"))
	if len(backups) != 1 {
		t.Fatal("removed skill not backed up")
	}
	targets, _ := st.SyncTargetsFor(uid)
	if targets[0].AppliedRevision != targets[0].Revision || len(targets[0].Missing) != 0 || len(targets[0].Extra) != 0 {
		t.Fatalf("not converged: %+v", targets)
	}
	c, _ := loadAgentConfig(cfg)
	if err = restoreSkill(c, target, filepath.Dir(backups[0]), strings.NewReader(targets[0].Target.Name+"\n"), io.Discard, "test"); err != nil {
		t.Fatal("restore:", err)
	}
	if _, err = os.Stat(filepath.Join(project, ".claude", "skills", "extra", "SKILL.md")); err != nil {
		t.Fatal("restore missing", err)
	}
	if err = restoreSkill(c, target, filepath.Dir(backups[0]), strings.NewReader(targets[0].Target.Name+"\n"), io.Discard, "test"); err == nil {
		t.Fatal("restore overwrote destination")
	}
	c.Hostname = "another-machine"
	writePrivate(cfg, c)
	if err = RunAgent([]string{"run", "--once", "--config", cfg}, io.Discard, "test"); err == nil {
		t.Fatal("copied enrollment accepted on another machine")
	}
}
