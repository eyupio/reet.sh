package api

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// Real controller routes and the binary's enrollment/poll/apply code, with two
// isolated home directories. No browser orchestrates the sync and no public
// installer or network skill source is involved.
func TestLibrarySyncNewMachinesDraftEditsAndReconnect(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "owner", auth.HashToken("library-owner")); err != nil {
		t.Fatal(err)
	}
	if err = st.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: reet-demo\ndescription: A private draft\n---\nFirst version.\n"
	save := func(body string) {
		t.Helper()
		if _, err := st.SaveHostedSkill(uid, store.HostedSkill{Slug: "reet-demo", Content: body}); err != nil {
			t.Fatal(err)
		}
	}
	save(content)
	srv := httptest.NewServer(New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts)))
	defer srv.Close()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("REET_TOKEN", "")
	type machine struct{ name, home, config string }
	newMachine := func(name string) machine {
		t.Helper()
		m := machine{name: name, home: t.TempDir()}
		m.config = filepath.Join(m.home, ".config", "reet", "agent.json")
		t.Setenv("HOME", m.home)
		t.Chdir(m.home)
		err := scan.RunAgentInput([]string{"enroll", "--server", srv.URL, "--token", "library-owner", "--env", name, "--config", m.config, "--allow-apply", "--skip-terminal-setup"}, strings.NewReader(""), io.Discard, "test")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	tick := func(m machine) {
		t.Helper()
		t.Setenv("HOME", m.home)
		t.Chdir(m.home)
		if err := scan.RunAgentInput([]string{"run", "--once", "--config", m.config}, strings.NewReader(""), io.Discard, "test"); err != nil {
			t.Fatal(err)
		}
	}
	assertFiles := func(m machine, want string) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(m.home, ".agents", "skills", "reet-demo", "SKILL.md"))
		if err != nil || string(got) != want {
			t.Fatalf("%s: got %q, want %q: %v", m.name, got, want, err)
		}
	}
	laptop := newMachine("laptop")
	for range 5 {
		tick(laptop)
	}
	assertFiles(laptop, content)
	if _, found, err := st.GetFor(uid, "local/reet-demo"); err != nil || found {
		t.Fatal("private written skill created a duplicate local shelf card", err)
	}
	desktop := newMachine("desktop")
	for range 5 {
		tick(desktop)
	}
	assertFiles(desktop, content)
	content = strings.Replace(content, "First version.", "An edited private draft.", 1)
	save(content)
	for range 4 {
		tick(laptop)
	}
	assertFiles(laptop, content)
	// Desktop did not poll during the edit. Reconnection alone catches it up.
	for range 4 {
		tick(desktop)
	}
	assertFiles(desktop, content)
	// A skill written after both machines joined follows the same policy.
	added := "---\nname: reet-added\ndescription: Added later\n---\nNew instructions.\n"
	if _, err := st.SaveHostedSkill(uid, store.HostedSkill{Slug: "reet-added", Content: added}); err != nil {
		t.Fatal(err)
	}
	// A custom local skill, with a supporting file, takes the authenticated
	// machine -> private library -> machine path without publishing.
	local := "---\nname: personal\ndescription: A machine-only skill\n---\nPersonal instructions.\n"
	localDir := filepath.Join(laptop.home, ".agents", "skills", "personal")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "SKILL.md"), []byte(local), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "example.txt"), []byte("Supporting content"), 0644); err != nil {
		t.Fatal(err)
	}
	for range 12 {
		tick(laptop)
		tick(desktop)
	}
	for _, m := range []machine{laptop, desktop} {
		for name, want := range map[string]string{"reet-added/SKILL.md": added, "personal/SKILL.md": local, "personal/example.txt": "Supporting content"} {
			got, err := os.ReadFile(filepath.Join(m.home, ".agents", "skills", filepath.FromSlash(name)))
			if err != nil || string(got) != want {
				t.Fatalf("%s: %s = %q: %v", m.name, name, got, err)
			}
		}
	}
	jobs, _ := st.SyncJobsFor(uid)
	before := len(jobs)
	for range 2 {
		tick(laptop)
		tick(desktop)
	}
	jobs, _ = st.SyncJobsFor(uid)
	if len(jobs) != before {
		t.Fatalf("matching private content queued repeated jobs: %d -> %d", before, len(jobs))
	}
	targets, _ := st.SyncTargetsFor(uid)
	if len(targets) != 2 {
		t.Fatalf("want automatic home-folder policies for both machines: %+v", targets)
	}
	for _, target := range targets {
		if len(target.Missing) != 0 || target.SourceID != "library" || !target.FollowSource {
			t.Fatalf("not converged: %+v", target)
		}
	}
	viewer, _ := st.CreateUser("viewer@library.test", "Viewer", "viewer", "")
	st.CreateAPIToken(viewer.ID, "viewer", auth.HashToken("library-viewer"))
	if code, body := doClient(t, srv.Client(), srv, "POST", "/api/v2/sync/library", `{}`, "X-Reet-Token", "library-viewer"); code != 403 {
		t.Fatal(code, body)
	}
	code, body := doClient(t, srv.Client(), srv, "GET", "/api/v2/sync/candidates?collection=library", "", "X-Reet-Token", "library-viewer")
	var listed struct{ Skills []store.SyncSkill }
	if err := json.Unmarshal([]byte(body), &listed); err != nil || code != 200 || len(listed.Skills) != 0 {
		t.Fatal("another account read private library", code, body, err)
	}
}
