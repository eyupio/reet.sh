package scan

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func syncReportFixture(t *testing.T) (*httptest.Server, store.SyncTarget, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude", "skills")
	mkSkill(t, dir, "extra", true)
	os.MkdirAll(filepath.Join(home, ".agents"), 0755)
	if err := os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(`{"skills":{"extra":{"source":"o/r"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	target := store.SyncTarget{
		Target:     store.Environment{ID: "1/laptop/claude", Name: "Laptop Claude", Host: "laptop", Agent: "claude"},
		SourceName: "Reference",
		Revision:   "one",
		Desired:    []store.SyncSkill{{ID: "o/r/beta", Name: "beta", Slug: "beta", Source: "o/r", SourceType: "github"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"targets": []store.SyncTarget{target}, "canWrite": true})
	}))
	t.Cleanup(srv.Close)
	return srv, target, home
}

func TestRunSyncReportPreview(t *testing.T) {
	srv, target, _ := syncReportFixture(t)
	report, err := RunSyncReport(SyncReportOptions{
		Server:  srv.URL,
		Token:   "secret",
		Target:  target.Target.ID,
		Host:    "laptop",
		Dir:     t.TempDir(),
		Out:     io.Discard,
		Version: "vtest",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.TargetSet || report.Target.Target.Name != target.Target.Name {
		t.Fatalf("target = %+v", report.Target)
	}
	if !report.PlanSet || report.Hash == "" {
		t.Fatal("no plan or hash")
	}
	if len(report.Plan.Actions) != 2 {
		t.Fatalf("actions = %+v", report.Plan.Actions)
	}
	var install, remove bool
	for _, action := range report.Plan.Actions {
		if action.Add && action.Skill.Slug == "beta" {
			install = true
		}
		if !action.Add && action.Skill.Slug == "extra" {
			remove = true
		}
	}
	if !install || !remove {
		t.Fatalf("actions = %+v", report.Plan.Actions)
	}
}

func TestSyncRefusesAChangedApprovedPlan(t *testing.T) {
	srv, target, _ := syncReportFixture(t)
	err := runSyncWithOptions(
		[]string{"--server", srv.URL, "--token", "secret", "--target", target.Target.ID, "--env", "laptop", "--apply"},
		strings.NewReader(""),
		io.Discard,
		func([]string, string) error { t.Fatal("must not execute a changed plan"); return nil },
		"vtest",
		syncOptions{approvedHash: "not-the-plan", cwd: t.TempDir()},
	)
	if err == nil || !strings.Contains(err.Error(), "approved plan no longer matches") {
		t.Fatalf("err = %v", err)
	}
}
