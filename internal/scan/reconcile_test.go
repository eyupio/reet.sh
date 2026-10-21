package scan

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestReconcileRefreshUsesHostnameAndDoesNotTouchFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REET_ENV", "")
	mkSkill(t, filepath.Join(os.Getenv("HOME"), ".agents", "skills"), "alpha", true)
	hostname, _ := os.Hostname()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("X-Reet-Token") != "private-token" {
			t.Error("credential missing")
		}
		if r.URL.Path == "/api/v2/machines/merge" {
			var v struct{ From, To string }
			if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
				t.Error(err)
			}
			if v.From != hostname || v.To != "PROD_Snicket" {
				t.Errorf("identity: %+v", v)
			}
			w.Write([]byte(`{}`))
			return
		}
		var v struct {
			Prune   bool
			Items   []struct{ Name, Host string }
			Machine struct{ Host string }
		}
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			t.Error(err)
		}
		if !v.Prune || v.Machine.Host != "PROD_Snicket" {
			t.Errorf("inventory: %+v", v)
		}
		if len(v.Items) == 0 {
			t.Error("installed skill not reported")
		}
		w.Write([]byte(`{"imported":1,"errors":[]}`))
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := RunVersion([]string{"--server", srv.URL, "--token", "private-token", "--env", "PROD_Snicket", "--prune", "--reconcile"}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/api/v2/machines/merge,/api/import" {
		t.Fatalf("repair must precede report: %v", paths)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".agents", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatal("refresh changed installed files")
	}
	if strings.Contains(out.String(), "private-token") {
		t.Fatal("token leaked")
	}
	paths = nil
	if err := RunVersion([]string{"--server", srv.URL, "--token", "private-token", "--env", "PROD_Snicket", "--prune", "--reconcile", "--dry-run"}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatal("dry run changed controller state")
	}
	if err := RunVersion([]string{"--server", srv.URL, "--reconcile"}, &out, "test"); err == nil {
		t.Fatal("repair accepted an implicit preferred name")
	}
	if len(paths) != 0 {
		t.Fatal("missing explicit name still sent requests")
	}
}

func TestReconcileConflictStopsBeforeInventoryImport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"conflicting references"}`))
	}))
	defer srv.Close()
	err := RunVersion([]string{"--server", srv.URL, "--env", "preferred", "--reconcile", "--prune"}, &bytes.Buffer{}, "test")
	if err == nil || !strings.Contains(err.Error(), "conflicting references") || requests != 1 {
		t.Fatalf("conflict not stopped: %v %d", err, requests)
	}
}

func TestSyncAcceptsMergedHostAndTargetAliases(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".claude", "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	posted := false
	target := store.SyncTarget{Target: store.Environment{ID: "new-target", Host: "preferred", Agent: "claude", Name: "preferred · Claude Code"}, HostAliases: []string{"old-host"}, TargetAliases: []string{"old-target"}, Desired: []store.SyncSkill{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"targets": []store.SyncTarget{target}, "canWrite": true})
			return
		}
		var v struct{ Machine store.MachineInfo }
		json.NewDecoder(r.Body).Decode(&v)
		if v.Machine.Host != "preferred" {
			t.Error("report did not use preferred name")
		}
		posted = true
		w.Write([]byte(`{"imported":0,"errors":[]}`))
	}))
	defer srv.Close()
	if err := runSync([]string{"--server", srv.URL, "--env", "old-host", "--target", "old-target"}, strings.NewReader(""), &bytes.Buffer{}, func([]string, string) error { t.Fatal("must not install/remove skills"); return nil }, "test"); err != nil {
		t.Fatal(err)
	}
	if !posted {
		t.Fatal("inventory was not refreshed")
	}
	if err := runSync([]string{"--server", srv.URL, "--env", "unrelated-host", "--target", "old-target"}, strings.NewReader(""), &bytes.Buffer{}, func([]string, string) error { t.Fatal("must not execute"); return nil }, "test"); err == nil {
		t.Fatal("unrelated host accepted")
	}
}
