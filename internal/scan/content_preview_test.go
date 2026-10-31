//go:build linux || darwin

package scan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillpreview"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

type previewController struct {
	t      *testing.T
	st     *store.Store
	uid    int64
	url    string
	client *http.Client
	bundle store.PrivateSkillResult
}

func newPreviewController(t *testing.T) *previewController {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "preview", auth.HashToken("preview-personal")); err != nil {
		t.Fatal(err)
	}
	server := api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthAccounts))
	t.Cleanup(func() { server.Close() })
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	empty := ""
	bundle, err := st.SavePrivateSkillRevisionFor(uid, "", store.PrivateSkillWrite{RequestID: "scan_preview_bundle_0001", ExpectedBase: &empty, Name: "demo", Files: []skillbundle.ContentFile{
		{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Library copy\n---\nLIBRARY_INSTRUCTIONS\n")},
		{Path: "scripts/run.sh", Data: []byte("#!/bin/sh\n"), Executable: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return &previewController{t: t, st: st, uid: uid, url: srv.URL, client: srv.Client(), bundle: bundle}
}

func (p *previewController) browser(method, path string, body, out any) int {
	p.t.Helper()
	raw, _ := json.Marshal(body)
	request, err := http.NewRequest(method, p.url+path, strings.NewReader(string(raw)))
	if err != nil {
		p.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Reet-Token", "preview-personal")
	response, err := p.client.Do(request)
	if err != nil {
		p.t.Fatal(err)
	}
	defer response.Body.Close()
	if out != nil {
		if err = json.NewDecoder(response.Body).Decode(out); err != nil {
			p.t.Fatal(err)
		}
	}
	return response.StatusCode
}

// machine enrols a connector with its own synthetic home and pins the library
// revision to the given reported scope.
func (p *previewController) machine(name, agent, project string, projects ...string) (agentConfig, store.SkillContentTarget) {
	p.t.Helper()
	home, err := filepath.EvalSymlinks(p.t.TempDir())
	if err != nil {
		p.t.Fatal(err)
	}
	a, secret, err := p.st.EnrollSyncAgent(p.uid, name, "test")
	if err != nil {
		p.t.Fatal(err)
	}
	if err = p.st.TouchEnvironmentFor(p.uid, name, agent, project, ""); err != nil {
		p.t.Fatal(err)
	}
	env := store.EnvironmentIDFor(p.uid, name, agent, project)
	if err = p.st.RecordInventoryFor(p.uid, env, true, store.MachineInfo{}); err != nil {
		p.t.Fatal(err)
	}
	empty := ""
	pin, err := p.st.SaveSkillContentTargetFor(p.uid, store.SkillContentTargetWrite{EnvironmentID: env, AgentID: a.ID, BundleID: p.bundle.Bundle.ID, RevisionID: p.bundle.Revision.ID, Mode: "additive", ExpectedVersion: &empty})
	if err != nil {
		p.t.Fatal(err)
	}
	return agentConfig{ID: a.ID, Secret: secret, Home: home, Host: name, Server: p.url, Token: "preview-personal", Projects: projects}, pin
}

func (p *previewController) request(pin store.SkillContentTarget, key string) store.SkillContentPreview {
	p.t.Helper()
	var out store.SkillContentPreview
	body := store.SkillContentPreviewCreate{RequestKey: key, TargetID: pin.ID, ExpectedTargetVersion: pin.Version}
	if code := p.browser("POST", "/api/v2/skill-content-previews", body, &out); code != 200 {
		p.t.Fatal(code, out)
	}
	return out
}

func (p *previewController) get(id string) store.SkillContentPreview {
	p.t.Helper()
	var out store.SkillContentPreview
	if code := p.browser("GET", "/api/v2/skill-content-previews/"+id, nil, &out); code != 200 {
		p.t.Fatal(code)
	}
	return out
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(data), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
}

func tree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		b.WriteString(rel + "|" + info.Mode().String() + "|" + info.ModTime().UTC().String() + "|")
		if info.Mode().IsRegular() {
			data, _ := os.ReadFile(path)
			b.WriteString(string(data))
		}
		b.WriteString("\n")
		return nil
	})
	return b.String()
}

func negotiated(t *testing.T, c agentConfig, instance string) {
	t.Helper()
	if handled, err := processContentPreviewWith(context.Background(), c, "test", instance); err != nil || handled {
		t.Fatal("negotiation must report no work and read nothing", handled, err)
	}
}

func TestContentPreviewTwoMachinesThroughARealControllerLeavesFilesUntouched(t *testing.T) {
	p := newPreviewController(t)
	instance := strings.Repeat("a", 48)

	// Machine one already has the skill folder: the library's two files differ from
	// its own, and it holds a note reet never put there.
	one, pinOne := p.machine("one", "agents", "")
	writeFiles(t, filepath.Join(one.Home, ".agents", "skills", "demo"), map[string]string{
		"SKILL.md":       "---\nname: demo\ndescription: Local copy\n---\nLOCAL_SECRET_INSTRUCTIONS\n",
		"notes/mine.txt": "LOCAL_SECRET_NOTE",
		"scripts/run.sh": "#!/bin/sh\necho local\n",
	})
	// Machine two has the scope root but no demo folder.
	two, pinTwo := p.machine("two", "agents", "")
	if err := os.MkdirAll(filepath.Join(two.Home, ".agents", "skills", "someone-else"), 0o755); err != nil {
		t.Fatal(err)
	}
	beforeOne, beforeTwo := tree(t, one.Home), tree(t, two.Home)

	for _, c := range []agentConfig{one, two} {
		negotiated(t, c, instance)
	}
	reqOne, reqTwo := p.request(pinOne, "scan_preview_request_one1"), p.request(pinTwo, "scan_preview_request_two2")
	for _, c := range []agentConfig{one, two} {
		if handled, err := processContentPreviewWith(context.Background(), c, "test", instance); err != nil || !handled {
			t.Fatal(handled, err)
		}
	}

	gotOne, gotTwo := p.get(reqOne.ID), p.get(reqTwo.ID)
	if gotOne.State != "captured" || gotOne.Capture == nil || gotOne.Capture.Presence != "present" || gotOne.Capture.RootOwnership != "exclusive" || len(gotOne.Capture.Manifest.Files) != 3 {
		t.Fatalf("%+v", gotOne.Capture)
	}
	// Existing files, even ones that look like the library's, are not owned.
	if gotOne.Plan.Ready || len(gotOne.Plan.Actions) != 0 || len(gotOne.Plan.Conflicts) != 1 || gotOne.Plan.Conflicts[0].Code != "adoption_required" {
		t.Fatalf("%+v", gotOne.Plan)
	}
	if gotTwo.State != "captured" || gotTwo.Capture.Presence != "absent" || !gotTwo.Plan.Ready || len(gotTwo.Plan.Actions) != 2 {
		t.Fatalf("%+v %+v", gotTwo.Capture, gotTwo.Plan)
	}
	if gotOne.Capture.RootID == gotTwo.Capture.RootID {
		t.Fatal("two machines' roots share an identity")
	}
	for _, got := range []store.SkillContentPreview{gotOne, gotTwo} {
		raw, _ := json.Marshal(got)
		for _, leaked := range []string{"LOCAL_SECRET_INSTRUCTIONS", "LOCAL_SECRET_NOTE", "LIBRARY_INSTRUCTIONS", one.Home, two.Home, `"data":`} {
			if strings.Contains(string(raw), leaked) {
				t.Fatalf("review exposes %q", leaked)
			}
		}
	}
	if tree(t, one.Home) != beforeOne || tree(t, two.Home) != beforeTwo {
		t.Fatal("a preview changed a machine's files")
	}
	if jobs, err := p.st.ClaimSyncJob(p.uid, store.SyncAgent{ID: one.ID, Host: "one"}); err != nil || jobs != nil {
		t.Fatal("preview produced legacy work", jobs, err)
	}
}

func TestContentPreviewProjectScopeUsesOnlyEnrolledProjects(t *testing.T) {
	p := newPreviewController(t)
	instance := strings.Repeat("b", 48)
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, pin := p.machine("projects", "claude", project, project)
	if err = os.MkdirAll(filepath.Join(project, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	negotiated(t, c, instance)
	req := p.request(pin, "scan_preview_project_scope")
	if handled, err := processContentPreviewWith(context.Background(), c, "test", instance); err != nil || !handled {
		t.Fatal(handled, err)
	}
	got := p.get(req.ID)
	if got.State != "captured" || got.Capture.Presence != "absent" || got.Project != project || !got.Plan.Ready {
		t.Fatalf("%+v", got)
	}

	// The same pin on a connector that has not enrolled that project locally is
	// refused by the connector, not looked up.
	other, otherPin := p.machine("not-enrolled", "claude", project)
	if err = os.MkdirAll(filepath.Join(project, ".claude", "skills", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	negotiated(t, other, instance)
	refused := p.request(otherPin, "scan_preview_not_enrolled")
	if handled, err := processContentPreviewWith(context.Background(), other, "test", instance); err != nil || !handled {
		t.Fatal(handled, err)
	}
	if got = p.get(refused.ID); got.State != "failed" || got.Error != "scope_unavailable" || got.Capture != nil {
		t.Fatalf("%+v", got)
	}
}

func TestContentPreviewSharedAndLinkedLocationsAreReportedNeverRead(t *testing.T) {
	p := newPreviewController(t)
	instance := strings.Repeat("c", 48)

	// ~/.claude/skills is a link to ~/.agents/skills: one directory, two names.
	linked, pinLinked := p.machine("linked", "claude", "")
	agents := filepath.Join(linked.Home, ".agents", "skills")
	writeFiles(t, filepath.Join(agents, "demo"), map[string]string{"SKILL.md": "---\nname: demo\n---\nBEHIND_THE_LINK\n"})
	if err := os.MkdirAll(filepath.Join(linked.Home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(agents, filepath.Join(linked.Home, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}
	// A skill folder that is itself a link (as skill installers often create).
	symlinked, pinSymlinked := p.machine("symlinked", "agents", "")
	elsewhere := t.TempDir()
	writeFiles(t, elsewhere, map[string]string{"SKILL.md": "---\nname: demo\n---\nBEHIND_THE_FOLDER_LINK\n"})
	if err := os.MkdirAll(filepath.Join(symlinked.Home, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(symlinked.Home, ".agents", "skills", "demo")); err != nil {
		t.Fatal(err)
	}
	// A scope root that does not exist yet.
	missing, pinMissing := p.machine("missing-root", "agents", "")

	for _, tc := range []struct {
		name      string
		c         agentConfig
		pin       store.SkillContentTarget
		ownership string
		blocked   string
		code      string
	}{
		{"aliased scope root", linked, pinLinked, "shared", "root_shared", "scope_root_symlink"},
		{"linked skill folder", symlinked, pinSymlinked, "exclusive", "observation_unknown", "skill_symlink"},
		{"scope root not created", missing, pinMissing, "unknown", "root_unknown", "scope_root_missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			negotiated(t, tc.c, instance)
			req := p.request(tc.pin, "scan_preview_"+strings.ReplaceAll(tc.name, " ", "_"))
			if handled, err := processContentPreviewWith(context.Background(), tc.c, "test", instance); err != nil || !handled {
				t.Fatal(handled, err)
			}
			got := p.get(req.ID)
			raw, _ := json.Marshal(got)
			if got.State != "captured" || got.Capture.Presence != "unknown" || got.Capture.RootOwnership != tc.ownership || got.Capture.Manifest != nil || got.Plan.Ready || len(got.Plan.Actions) != 0 {
				t.Fatalf("%+v", got)
			}
			if len(got.Plan.Conflicts) != 1 || got.Plan.Conflicts[0].Code != tc.blocked || got.Capture.Issues[0].Code != tc.code {
				t.Fatalf("%+v %+v", got.Plan.Conflicts, got.Capture.Issues)
			}
			if strings.Contains(string(raw), "BEHIND_THE") {
				t.Fatal("read through a link")
			}
		})
	}
}

func TestContentPreviewUnsupportedOrFutureControllersNeverTriggerFileReads(t *testing.T) {
	home := t.TempDir()
	canary := filepath.Join(home, ".agents", "skills", "demo")
	writeFiles(t, canary, map[string]string{"SKILL.md": "x"})
	before := tree(t, home)
	for name, handler := range map[string]http.HandlerFunc{
		"not found":      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) },
		"method refused": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(405) },
		"accounts only":  func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) },
		// Paused, between enrolments or not ready: a normal state, not an error to report every cycle.
		"connector paused": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(409) },
		"future schema": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 2, "work": map[string]any{"id": strings.Repeat("a", 48), "instanceId": strings.Repeat("a", 48), "agent": "agents", "name": "demo"}})
		},
		"no work": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "work": nil})
		},
	} {
		t.Run(name, func(t *testing.T) {
			var posts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				if r.URL.Path != "/api/v2/sync/skill-content-preview/work" {
					t.Errorf("sent %s to a controller that offered no work", r.URL.Path)
				}
				handler(w, r)
			}))
			defer srv.Close()
			handled, err := processContentPreviewWith(context.Background(), agentConfig{Home: home, Server: srv.URL}, "test", strings.Repeat("a", 48))
			if err != nil || handled || posts.Load() != 1 {
				t.Fatal(handled, err, posts.Load())
			}
		})
	}
	if tree(t, home) != before {
		t.Fatal("home changed")
	}
}

func TestContentPreviewRefusesRedirectsAndInsecureControllers(t *testing.T) {
	var followed atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Add(1) }))
	defer target.Close()
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/capture-my-credentials", http.StatusTemporaryRedirect)
	}))
	defer redirecting.Close()
	c := agentConfig{Server: redirecting.URL, Token: "secret-token", ID: "agent", Secret: "agent-secret", Home: t.TempDir()}
	if handled, err := processContentPreviewWith(context.Background(), c, "test", strings.Repeat("a", 48)); handled || err == nil {
		t.Fatal("a redirect should be an error, not work", handled, err)
	}
	if followed.Load() != 0 {
		t.Fatal("followed a redirect with enrolment credentials")
	}
	c.Server = "http://controller.example.test"
	if handled, err := processContentPreviewWith(context.Background(), c, "test", strings.Repeat("a", 48)); handled || err == nil {
		t.Fatal("sent enrolment credentials over cleartext HTTP", handled, err)
	}
}

func TestContentPreviewIgnoresWorkForAnotherProcessOrAnUnsupportedScope(t *testing.T) {
	home := t.TempDir()
	writeFiles(t, filepath.Join(home, ".codex", "skills", "demo"), map[string]string{"SKILL.md": "x"})
	instance := strings.Repeat("a", 48)
	for name, work := range map[string]map[string]any{
		"another process":    {"id": strings.Repeat("1", 48), "instanceId": strings.Repeat("b", 48), "agent": "agents", "project": "", "name": "demo"},
		"scan-only agent":    {"id": strings.Repeat("2", 48), "instanceId": instance, "agent": "codex", "project": "", "name": "demo"},
		"traversing name":    {"id": strings.Repeat("3", 48), "instanceId": instance, "agent": "agents", "project": "", "name": "../.codex/skills/demo"},
		"unenrolled project": {"id": strings.Repeat("4", 48), "instanceId": instance, "agent": "claude", "project": "/etc", "name": "demo"},
		"home agent project": {"id": strings.Repeat("5", 48), "instanceId": instance, "agent": "cursor", "project": "/tmp", "name": "demo"},
	} {
		t.Run(name, func(t *testing.T) {
			var reports, failures atomic.Int32
			var code atomic.Value
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/sync/skill-content-preview/work":
					json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "work": work})
				case "/api/v2/sync/skill-content-preview/report":
					reports.Add(1)
				case "/api/v2/sync/skill-content-preview/failure":
					failures.Add(1)
					var in struct{ Code string }
					json.NewDecoder(r.Body).Decode(&in)
					code.Store(in.Code)
					json.NewEncoder(w).Encode(map[string]bool{"recorded": true})
				}
			}))
			defer srv.Close()
			handled, err := processContentPreviewWith(context.Background(), agentConfig{Home: home, Server: srv.URL}, "test", instance)
			if err != nil || !handled || reports.Load() != 0 {
				t.Fatal(handled, err, reports.Load())
			}
			if name == "another process" {
				if failures.Load() != 0 {
					t.Fatal("reported on work that belongs to another process")
				}
				return
			}
			if failures.Load() != 1 || code.Load() != "scope_unavailable" {
				t.Fatal("expected one bounded failure", failures.Load(), code.Load())
			}
		})
	}
}

func TestContentPreviewChoosesItsOwnRootAndSendsNothingOnceCancelled(t *testing.T) {
	home := t.TempDir()
	writeFiles(t, filepath.Join(home, ".agents", "skills", "demo"), map[string]string{"SKILL.md": "x"})
	instance := strings.Repeat("a", 48)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var reports atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/report") {
			reports.Add(1)
		}
		json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "work": map[string]any{"id": strings.Repeat("9", 48), "instanceId": instance, "agent": "agents", "project": "", "name": "demo", "expiresAt": time.Now().Add(time.Minute).UnixMilli(), "path": "/etc"}})
	}))
	defer srv.Close()
	var chosen skillpreview.Roots
	observe := func(roots skillpreview.Roots, name string) skillpreview.Capture {
		chosen = roots
		cancel() // the connector is told to stop while it is still reading
		return skillpreview.Observe(roots, name)
	}
	handled, err := processContentPreviewUsing(ctx, agentConfig{Home: home, Server: srv.URL}, "test", instance, observe)
	if !handled || err == nil || reports.Load() != 0 {
		t.Fatal(handled, err, reports.Load())
	}
	if want := filepath.Join(home, ".agents", "skills"); chosen.Target != want {
		t.Fatalf("root came from %q, not the connector's own configuration %q", chosen.Target, want)
	}
	for _, other := range chosen.Others {
		if other == chosen.Target {
			t.Fatal("the target was listed as its own alias")
		}
	}
	if len(chosen.Others) < 4 || !chosen.OthersComplete {
		t.Fatal("alias detection should cover every other configured home scope", chosen.Others)
	}
}

func TestAgentCycleServesAPreviewAfterImportAndBeforeLegacyPolling(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	var paths []string
	var mu atomic.Value
	mu.Store(&paths)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := mu.Load().(*[]string)
		*current = append(*current, r.URL.Path)
		switch r.URL.Path {
		case "/api/v2/sync/skill-content-preview/work":
			var in struct {
				InstanceID string `json:"instanceId"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			json.NewEncoder(w).Encode(map[string]any{"schemaVersion": 1, "work": map[string]any{"id": strings.Repeat("7", 48), "instanceId": in.InstanceID, "agent": "agents", "project": "", "name": "demo"}})
		case "/api/v2/sync/skill-content-preview/report":
			json.NewEncoder(w).Encode(map[string]bool{"recorded": true})
		case "/api/v2/sync/skill-import/work":
			w.WriteHeader(404) // an older controller: the cycle must carry on to the preview
		default:
			t.Errorf("the cycle continued to %s although a preview was served", r.URL.Path)
		}
	}))
	defer srv.Close()
	var last time.Time
	c := agentConfig{Server: srv.URL, Home: home, ID: "a", Secret: "s", Token: "t"}
	if err := agentCycle(context.Background(), c, filepath.Join(t.TempDir(), "agent.json"), os.Stderr, "test", &last, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"/api/v2/sync/skill-import/work", "/api/v2/sync/skill-content-preview/work", "/api/v2/sync/skill-content-preview/report"}
	if strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Fatalf("cycle order %v, want %v", paths, want)
	}
}

func TestPreviewRootsListsEveryOtherConfiguredScopeAndFlagsTruncation(t *testing.T) {
	canon := func() string {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return dir
	}
	home, a, b := canon(), canon(), canon()
	c := agentConfig{Home: home, Projects: []string{a, b}}
	contains := func(list []string, want string) bool {
		for _, v := range list {
			if v == want {
				return true
			}
		}
		return false
	}

	roots, err := previewRoots(c, "claude", "")
	if err != nil || roots.Target != filepath.Join(home, ".claude", "skills") || !roots.OthersComplete {
		t.Fatal(roots, err)
	}
	for _, want := range []string{
		filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".cursor", "skills"),
		filepath.Join(home, ".codex", "skills"), // scan-only, but it can still be the same directory
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(a, ".claude", "skills"), filepath.Join(a, ".agents", "skills"),
		filepath.Join(b, ".claude", "skills"), filepath.Join(b, ".agents", "skills"),
	} {
		if !contains(roots.Others, want) {
			t.Fatalf("alias candidates are missing %s: %v", want, roots.Others)
		}
	}
	if contains(roots.Others, roots.Target) {
		t.Fatal("the target was listed as its own alias")
	}

	project, err := previewRoots(c, "claude", a)
	if err != nil || project.Target != filepath.Join(a, ".claude", "skills") {
		t.Fatal(project, err)
	}
	if contains(project.Others, project.Target) || !contains(project.Others, filepath.Join(home, ".claude", "skills")) || !contains(project.Others, filepath.Join(a, ".agents", "skills")) {
		t.Fatalf("a project scope must be compared with the home scope for the same agent: %v", project.Others)
	}

	if _, err = previewRoots(c, "codex", ""); err == nil {
		t.Fatal("a scan-only agent scope was accepted")
	}
	// An enrolled project is matched by canonical directory, not by how it is spelled.
	respelled, err := previewRoots(c, "claude", b+"/../"+filepath.Base(a))
	if err != nil || respelled.Target != project.Target {
		t.Fatal("another spelling of an enrolled project should map to the same root", respelled, err)
	}
	if _, err = previewRoots(c, "claude", filepath.Dir(a)); err == nil {
		t.Fatal("accepted a directory that is not an enrolled project")
	}

	crowded := agentConfig{Home: home}
	for i := 0; i <= maxPreviewProjects; i++ {
		crowded.Projects = append(crowded.Projects, filepath.Join(home, "p", string(rune('a'+i%26)), string(rune('a'+i/26))))
	}
	truncated, err := previewRoots(crowded, "claude", "")
	if err != nil || truncated.OthersComplete {
		t.Fatal("more projects than can be alias-checked must not be reported exclusive-capable", truncated.OthersComplete, err)
	}
	if len(truncated.Others) > 5+2*maxPreviewProjects {
		t.Fatal("alias list is unbounded", len(truncated.Others))
	}
}
