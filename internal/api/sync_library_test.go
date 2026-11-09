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
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

func TestScopedLibrarySyncPreservesOtherMachinePolicies(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "owner", auth.HashToken("scoped-owner")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		rf, err := ref.Parse("a/b/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = st.UpsertFor(uid, rf, store.Patch{}); err != nil {
			t.Fatal(err)
		}
	}
	agents := map[string]store.SyncAgent{}
	for _, host := range []string{"laptop", "desktop"} {
		if err = st.TouchEnvironmentFor(uid, host, "agents", "", ""); err != nil {
			t.Fatal(err)
		}
		id := store.EnvironmentIDFor(uid, host, "agents", "")
		if err = st.RecordInventoryFor(uid, id, true, store.MachineInfo{}); err != nil {
			t.Fatal(err)
		}
		a, _, err := st.EnrollSyncAgent(uid, host, "test")
		if err != nil {
			t.Fatal(err)
		}
		agents[host] = a
	}
	if err = st.ControlSyncAgent(uid, agents["laptop"].ID, "keep-sync-on"); err != nil {
		t.Fatal(err)
	}
	desktopID := store.EnvironmentIDFor(uid, "desktop", "agents", "")
	if err = st.SaveDesiredFor(uid, "owner", store.DesiredSelection{TargetID: desktopID, SourceID: "shelf", Mode: "mirror", SkillIDs: []string{"a/b/one"}}); err != nil {
		t.Fatal(err)
	}
	before, err := st.SyncTargetsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts)))
	defer srv.Close()
	body, _ := json.Marshal(map[string]string{"agentId": agents["laptop"].ID})
	code, response := doClient(t, srv.Client(), srv, "POST", "/api/v2/sync/library", string(body), "X-Reet-Token", "scoped-owner")
	if code != 200 {
		t.Fatal(code, response)
	}
	afterAgents, _ := st.SyncAgentsFor(uid)
	for _, a := range afterAgents {
		if a.Host == "desktop" && a.KeepSynced {
			t.Fatal("scoped add enabled automatic sync on another machine")
		}
	}
	after, _ := st.SyncTargetsFor(uid)
	if len(after) != 2 {
		t.Fatalf("expected the selected machine's library plan and the existing custom plan: %+v", after)
	}
	for _, target := range after {
		if target.Target.ID == desktopID && (target.Revision != before[0].Revision || target.SourceID != "shelf" || target.Mode != "mirror" || len(target.Desired) != 1) {
			t.Fatalf("scoped add replaced another machine's plan: %+v", target)
		}
		if target.Target.Host == "laptop" && (target.SourceID != "library" || target.Mode != "additive" || !target.FollowSource || len(target.Desired) != 2) {
			t.Fatalf("selected machine did not follow the library: %+v", target)
		}
	}
	jobs, _ := st.SyncJobsFor(uid)
	if len(jobs) != 1 {
		t.Fatalf("expected one install for the selected machine: %+v", jobs)
	}
	for _, job := range jobs {
		if job.AgentID != agents["laptop"].ID {
			t.Fatalf("scoped add queued work for another machine: %+v", job)
		}
	}
	if code, response = doClient(t, srv.Client(), srv, "POST", "/api/v2/sync/library", `{"agentId":"not-owned"}`, "X-Reet-Token", "scoped-owner"); code != 409 {
		t.Fatal("unknown enrollment accepted", code, response)
	}
}

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
	items, err := st.LibraryFor(uid)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	item := items[0]
	if err := st.SetLibraryStatusFor(uid, item.ID, store.LibraryInUse, item.Version); err != nil {
		t.Fatal(err)
	}
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
	selectFor := func(m machine) {
		t.Helper()
		tick(m)
		path := filepath.Join(m.home, ".agents", "skills", "reet-demo", "SKILL.md")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("new machine received content before selection", err)
		}
		if _, err := st.SaveLibrarySelectionFor(uid, store.LibrarySelection{TargetID: store.EnvironmentIDFor(uid, m.name, "agents", ""), ItemIDs: []string{item.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	laptop := newMachine("laptop")
	selectFor(laptop)
	for range 5 {
		tick(laptop)
	}
	assertFiles(laptop, content)
	if _, found, err := st.GetFor(uid, "local/reet-demo"); err != nil || found {
		t.Fatal("private written skill created a duplicate local shelf card", err)
	}
	desktop := newMachine("desktop")
	selectFor(desktop)
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
	// Later additions require explicit selection; neither machine subscribes.
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
	tick(laptop)
	for _, m := range []machine{laptop, desktop} {
		if _, err := os.Stat(filepath.Join(m.home, ".agents", "skills", "reet-added", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatal("future addition was implicitly selected", err)
		}
	}
	t.Setenv("HOME", laptop.home)
	t.Chdir(laptop.home)
	if err := scan.RunAgentInput([]string{"library-import", "--config", laptop.config}, strings.NewReader(""), io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	imported, err := st.LibraryFor(uid)
	if err != nil || len(imported) != 3 {
		t.Fatal("capture duplicated a managed copy", imported, err)
	}
	selected := []string{}
	for _, item := range imported {
		if item.Status != store.LibraryInUse {
			if err := st.SetLibraryStatusFor(uid, item.ID, store.LibraryInUse, item.Version); err != nil {
				t.Fatal(err)
			}
		}
		selected = append(selected, item.ID)
	}
	for _, m := range []machine{laptop, desktop} {
		target := store.EnvironmentIDFor(uid, m.name, "agents", "")
		previous, err := st.LibrarySelectionFor(uid, target)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.SaveLibrarySelectionFor(uid, store.LibrarySelection{TargetID: target, ItemIDs: selected, Version: previous.Version}); err != nil {
			t.Fatal(err)
		}
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
		if len(target.Missing) != 0 || target.SourceID != "library" || target.FollowSource || !target.Canonical || len(target.Desired) != 3 {
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

func TestDedupeLibraryEndpointPreviewsThenRemoves(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "owner", auth.HashToken("dedupe-owner")); err != nil {
		t.Fatal(err)
	}
	firstRef, err := ref.Parse("a/b/improve")
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.UpsertFor(uid, firstRef, store.Patch{})
	if err != nil {
		t.Fatal(err)
	}
	secondRef, err := ref.Parse("c/d/improve")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.UpsertFor(uid, secondRef, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts)))
	defer srv.Close()

	code, response := doClient(t, srv.Client(), srv, "POST", "/api/v2/sync/library/dedupe", `{}`, "X-Reet-Token", "dedupe-owner")
	if code != 200 {
		t.Fatal(code, response)
	}
	var preview struct {
		Groups  []store.SyncDuplicateGroup `json:"groups"`
		Removed []string                   `json:"removed"`
	}
	if err = json.Unmarshal([]byte(response), &preview); err != nil {
		t.Fatal(err, response)
	}
	if len(preview.Groups) != 1 || len(preview.Removed) != 1 || preview.Groups[0].Keep.ID != first.ID {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if _, ok, err := st.GetFor(uid, first.ID); err != nil || !ok {
		t.Fatal("preview removed a skill")
	}

	code, response = doClient(t, srv.Client(), srv, "POST", "/api/v2/sync/library/dedupe", `{"confirm":true}`, "X-Reet-Token", "dedupe-owner")
	if code != 200 {
		t.Fatal(code, response)
	}
	if err = json.Unmarshal([]byte(response), &preview); err != nil {
		t.Fatal(err, response)
	}
	if len(preview.Removed) != 1 {
		t.Fatalf("nothing removed: %+v", preview)
	}
	if _, ok, err := st.GetFor(uid, preview.Removed[0]); err != nil || ok {
		t.Fatal("duplicate survived confirm")
	}
}

// Sync all machines is one action that works. A name two sources share is settled
// by the controller instead of failing the request with "More than one source uses
// the name", and the library read says which copy was left out so the page can
// mention it without calling it an error.
func TestSyncAllMachinesSucceedsWhenANameIsShared(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "owner", auth.HashToken("shared-name-owner")); err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"a/b/improve", "c/d/improve", "e/f/other"} {
		rf, err := ref.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = st.UpsertFor(uid, rf, store.Patch{}); err != nil {
			t.Fatal(err)
		}
	}
	if err = st.TouchEnvironmentFor(uid, "laptop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = st.RecordInventoryFor(uid, store.EnvironmentIDFor(uid, "laptop", "agents", ""), true, store.MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = st.EnrollSyncAgent(uid, "laptop", "test"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts)))
	defer srv.Close()

	if code, response := doClient(t, srv.Client(), srv, "POST", "/api/v2/sync/library", `{}`, "X-Reet-Token", "shared-name-owner"); code != 200 {
		t.Fatalf("a shared name failed Sync all machines: %d %s", code, response)
	}
	agents, err := st.SyncAgentsFor(uid)
	if err != nil || len(agents) != 1 || !agents[0].KeepSynced {
		t.Fatalf("automatic sync was not switched on: %+v %v", agents, err)
	}

	code, response := doClient(t, srv.Client(), srv, "GET", "/api/v2/sync/candidates?collection=library", "", "X-Reet-Token", "shared-name-owner")
	if code != 200 {
		t.Fatal(code, response)
	}
	var read struct {
		Skills     []store.SyncSkill          `json:"skills"`
		Duplicates []store.SyncDuplicateGroup `json:"duplicates"`
	}
	if err = json.Unmarshal([]byte(response), &read); err != nil {
		t.Fatal(err, response)
	}
	improve := 0
	for _, sk := range read.Skills {
		if sk.Problem != "" {
			t.Fatalf("a shared name is still reported as a problem: %+v", sk)
		}
		if sk.Slug == "improve" {
			improve++
		}
	}
	if improve != 1 || len(read.Skills) != 2 {
		t.Fatalf("the library should hold one copy of each name: %+v", read.Skills)
	}
	if len(read.Duplicates) != 1 || read.Duplicates[0].Slug != "improve" || len(read.Duplicates[0].Remove) != 1 {
		t.Fatalf("the copy that was left out is not reported: %s", response)
	}
}

// A skill another agent already holds from a different source used to stop
// sync with no way forward. The endpoint adopts the installed source and the
// next preview installs it without overwriting the other agent's copy.
func TestResolveSourceConflictEndpointFollowsInstalledCopy(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "owner", auth.HashToken("source-fix-owner")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = st.EnrollSyncAgent(uid, "laptop", "test"); err != nil {
		t.Fatal(err)
	}
	if err = st.TouchEnvironmentFor(uid, "laptop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	target := store.EnvironmentIDFor(uid, "laptop", "agents", "")
	if err = st.RecordInventoryFor(uid, target, true, store.MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	if err = st.TouchEnvironmentFor(uid, "laptop", "claude", "", ""); err != nil {
		t.Fatal(err)
	}
	claude := store.EnvironmentIDFor(uid, "laptop", "claude", "")
	if err = st.RecordInventoryFor(uid, claude, true, store.MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	installedRef, _ := ref.Parse("x/y/one")
	installed, err := st.UpsertFor(uid, installedRef, store.Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.RecordEnvironmentFor(uid, installed.ID, "laptop", "claude", "", "", ""); err != nil {
		t.Fatal(err)
	}
	wantedRef, _ := ref.Parse("a/b/one")
	if _, err = st.UpsertFor(uid, wantedRef, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	candidates, err := st.SyncCandidatesFor(uid, "owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	selected := ""
	for _, candidate := range candidates {
		if candidate.ID == "a/b/one" {
			selected = candidate.ID
		}
	}
	if err = st.SaveDesiredFor(uid, "owner", store.DesiredSelection{TargetID: target, SourceID: "shelf", SkillIDs: []string{selected}, Mode: "additive"}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts)))
	defer srv.Close()

	body := `{"targetIds":["` + target + `"]}`
	code, response := doClient(t, srv.Client(), srv, "POST", "/api/v2/sync/source-conflicts/resolve", body, "X-Reet-Token", "source-fix-owner")
	if code != 200 {
		t.Fatal(code, response)
	}
	var preview struct {
		Changes []store.SyncSourceFix `json:"changes"`
	}
	if err = json.Unmarshal([]byte(response), &preview); err != nil {
		t.Fatal(err, response)
	}
	if len(preview.Changes) != 1 || preview.Changes[0].From != "a/b" || preview.Changes[0].To != "x/y" || preview.Changes[0].Agent != "claude" {
		t.Fatalf("unexpected preview: %+v", preview)
	}

	code, response = doClient(t, srv.Client(), srv, "POST", "/api/v2/sync/source-conflicts/resolve", `{"targetIds":["`+target+`"],"confirm":true}`, "X-Reet-Token", "source-fix-owner")
	if code != 200 {
		t.Fatal(code, response)
	}
	targets, err := st.SyncTargetsFor(uid)
	if err != nil || len(targets) != 1 || targets[0].Desired[0].Source != "x/y" {
		t.Fatalf("plan not resolved: %+v %v", targets, err)
	}
}
