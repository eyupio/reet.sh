package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const fromGitHubPath = "/api/v2/folder-projects/from-github"

// repoCloneCaps is a machine that can clone from GitHub.
func repoCloneCaps() terminalproto.Capabilities {
	caps := browseCaps()
	caps.RepoClone = true
	return caps
}

// cloned is the machine's answer for a fresh clone of owner/repo.
func cloned(owner, repo string) terminalproto.RepoCloneResult {
	return terminalproto.RepoCloneResult{Dir: browseTestHome + "/reet/repos/" + owner + "/" + repo, DefaultBranch: "main", Marked: true}
}

func fromGitHubBody(host, repo, name string) string {
	raw, _ := json.Marshal(map[string]string{"host": host, "repo": repo, "name": name})
	return string(raw)
}

// cloneStatus reads one clone job as cookie.
func (f *browseAPI) cloneStatus(t *testing.T, cookie, id string) (int, map[string]any, string) {
	t.Helper()
	return f.get(t, cookie, "/api/v2/folder-projects/clones/"+id)
}

// startClone asks for a clone and expects it to start.
func (f *browseAPI) startClone(t *testing.T, repo, name string) (id string, out map[string]any) {
	t.Helper()
	f.freshWrites()
	code, out := f.post(t, f.cookie, fromGitHubPath, fromGitHubBody(f.agent, repo, name))
	if code != http.StatusAccepted {
		t.Fatalf("start = %d %v", code, out)
	}
	id, _ = out["cloneId"].(string)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Fatalf("cloneId = %q", id)
	}
	return id, out
}

// freshWrites gives the account a fresh folder-project write allowance.
func (f *browseAPI) freshWrites() {
	f.srv.s.folderWritesMu.Lock()
	f.srv.s.folderWrites = auth.NewLimiter(folderProjectWriteLimit, time.Minute)
	f.srv.s.folderWritesMu.Unlock()
}

// waitClone polls the job until it is no longer cloning.
func (f *browseAPI) waitClone(t *testing.T, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		code, out, raw := f.cloneStatus(t, f.cookie, id)
		if code != 200 {
			t.Fatalf("status = %d %s", code, raw)
		}
		if out["state"] != "cloning" {
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("the clone never finished: %s", raw)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitHeld waits until n requests are held by the machine.
func (f *browseAPI) waitHeld(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for f.heldCount() < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d requests reached the machine", f.heldCount(), n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// release answers the first held request with body.
func (f *browseAPI) release(t *testing.T, body terminalproto.Body) {
	t.Helper()
	f.mu.Lock()
	reply := f.held[0]
	f.held = f.held[1:]
	f.mu.Unlock()
	reply.Body = body
	data, err := terminalproto.Encode(reply)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := terminalproto.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.peers[f.agent].Receive(decoded); err != nil {
		t.Fatal(err)
	}
}

func (f *browseAPI) clonesSent() []terminalproto.RepoClone {
	var out []terminalproto.RepoClone
	for _, body := range f.sentRequests() {
		if c, ok := body.(terminalproto.RepoClone); ok {
			out = append(out, c)
		}
	}
	return out
}

func TestControllerAdvertisesRepoClone(t *testing.T) {
	if !terminalproto.SupportsRepoClone(privateControlFeatures) || !terminalproto.SupportsRepoClone(agreedControlFeatures(terminalproto.Capabilities{})) {
		t.Fatalf("repo-clone-v1 is not advertised: %q", privateControlFeatures)
	}
	if !isPeerReply(&terminalproto.RepoCloneResult{}) {
		t.Fatal("a clone's answer would end the machine's connection")
	}
}

func TestAddFromGitHubClonesThenSavesARepoProject(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	// A project already called "site" makes the new one "site (2)".
	if _, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: browseTestHome + "/site", Name: "site"}); err != nil {
		t.Fatal(err)
	}
	f.answering(func(terminalproto.Body) terminalproto.Body { return cloned("acme", "site") })
	id, out := f.startClone(t, " https://github.com/acme/site.git ", "")
	if out["dir"] != "~/reet/repos/acme/site" || len(out) != 2 {
		t.Fatalf("start answer = %v", out)
	}
	done := f.waitClone(t, id)
	if done["state"] != "done" {
		t.Fatalf("job = %v", done)
	}
	if sent := f.clonesSent(); len(sent) != 1 || sent[0] != (terminalproto.RepoClone{Owner: "acme", Repo: "site"}) {
		t.Fatalf("sent %+v", sent)
	}
	project, _ := done["project"].(map[string]any)
	want := map[string]any{"host": f.agent, "machine": "studio", "dir": browseTestHome + "/reet/repos/acme/site", "name": "site (2)", "isRepo": true,
		"baseBranch": "main", "origin": "acme/site", "cloned": true}
	for k, v := range want {
		if project[k] != v {
			t.Fatalf("project %s = %v, want %v (%v)", k, project[k], v, project)
		}
	}
	saved, err := f.st.FolderProjectFor(f.uid, project["id"].(string))
	if err != nil || saved.Origin != "acme/site" || !saved.Cloned || !saved.IsRepo || saved.BaseBranch != "main" || saved.Dir != browseTestHome+"/reet/repos/acme/site" {
		t.Fatalf("stored %+v %v", saved, err)
	}
	// The list shows the same facts.
	code, list, raw := f.get(t, f.cookie, "/api/v2/folder-projects")
	if code != 200 || !strings.Contains(raw, `"origin":"acme/site"`) || len(list["projects"].([]any)) != 2 {
		t.Fatalf("list = %d %s", code, raw)
	}
	rows := projectEvents(t, f.st, f.uid, "project-add")
	if len(rows) != 1 {
		t.Fatalf("audit rows %v", rows)
	}
	if rows[0]["origin"] != "acme/site" || rows[0]["cloned"] != true || rows[0]["folder"] != saved.Dir || rows[0]["project"] != saved.ID || rows[0]["host"] != "studio" {
		t.Fatalf("audit row %v", rows[0])
	}
}

func TestAddFromGitHubTakesTheNameGiven(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return cloned("acme", "site") })
	id, _ := f.startClone(t, "acme/site", "  Front  ")
	done := f.waitClone(t, id)
	if project, _ := done["project"].(map[string]any); project["name"] != "Front" {
		t.Fatalf("job = %v", done)
	}
}

// lateWriter fails the test if anything writes to it after its handler
// returned.
type lateWriter struct {
	*httptest.ResponseRecorder
	mu       sync.Mutex
	returned bool
	late     int
}

func (w *lateWriter) Header() http.Header { return w.ResponseRecorder.Header() }
func (w *lateWriter) Write(b []byte) (int, error) {
	w.note()
	return w.ResponseRecorder.Write(b)
}
func (w *lateWriter) WriteHeader(code int) {
	w.note()
	w.ResponseRecorder.WriteHeader(code)
}
func (w *lateWriter) note() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.returned {
		w.late++
	}
}

// startDetached sends the start with a context it cancels as soon as the
// handler returns, as a browser that closed the dialog would.
func (f *browseAPI) startDetached(t *testing.T, repo string) (string, *lateWriter) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("POST", fromGitHubPath, strings.NewReader(fromGitHubBody(f.agent, repo, ""))).WithContext(ctx)
	r.Header.Set("Origin", "https://reet.example")
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := &lateWriter{ResponseRecorder: httptest.NewRecorder()}
	f.srv.ServeHTTP(w, r)
	w.mu.Lock()
	w.returned = true
	w.mu.Unlock()
	cancel()
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusAccepted {
		t.Fatalf("start = %d %s", w.Code, w.Body.String())
	}
	return out["cloneId"].(string), w
}

func TestAddFromGitHubSurvivesTheRequestEnding(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return nil })
	id, w := f.startDetached(t, "acme/site")
	f.waitHeld(t, 1)
	// The request is long over before the machine answers.
	time.Sleep(20 * time.Millisecond)
	f.release(t, cloned("acme", "site"))
	if done := f.waitClone(t, id); done["state"] != "done" {
		t.Fatalf("job = %v", done)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.late != 0 {
		t.Fatalf("the job wrote to the finished request %d times", w.late)
	}
}

func TestAddFromGitHubNeverWritesToAFinishedRequestOnFailure(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return nil })
	id, w := f.startDetached(t, "acme/site")
	f.waitHeld(t, 1)
	f.release(t, terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneNoAccess})
	if done := f.waitClone(t, id); done["state"] != "failed" || done["code"] != "no_access" {
		t.Fatalf("job = %v", done)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.late != 0 {
		t.Fatalf("the job wrote to the finished request %d times", w.late)
	}
}

func TestAddFromGitHubReportsEachRefusal(t *testing.T) {
	for _, c := range []struct {
		reason   terminalproto.RepoCloneReason
		sentence string
	}{
		{terminalproto.RepoCloneNotSignedIn, "studio isn't signed in to GitHub. Run gh auth login there, then try again."},
		{terminalproto.RepoCloneNoAccess, "GitHub has no Acme/Site that studio's sign-in can read."},
		{terminalproto.RepoCloneExists, "~/reet/repos/Acme/Site on studio already holds something else. Move it, or add that folder as a project."},
		{terminalproto.RepoCloneTimedOut, "Cloning Acme/Site took longer than 10 minutes and was stopped. Nothing was kept."},
		{terminalproto.RepoCloneFailed, "Couldn't clone Acme/Site on studio."},
		{terminalproto.RepoCloneBusy, "studio is already cloning a repository. Try again when it finishes."},
		{terminalproto.RepoCloneDenied, "Terminal access is off on studio. Turn it on there, then try again."},
		{terminalproto.RepoCloneSSHRewrite, "studio's git settings send github.com over ssh, which reet doesn't use for cloning. Clone Acme/Site there yourself and add the folder as a project."},
	} {
		t.Run(string(c.reason), func(t *testing.T) {
			f := browseAPIFixture(t, repoCloneCaps())
			f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.RepoCloneResult{Reason: c.reason} })
			id, _ := f.startClone(t, "https://github.com/Acme/Site", "")
			done := f.waitClone(t, id)
			if done["state"] != "failed" || done["code"] != string(c.reason) || done["sentence"] != c.sentence || len(done) != 3 {
				t.Fatalf("job = %v", done)
			}
			if projects, _ := f.st.ListFolderProjectsFor(f.uid); len(projects) != 0 {
				t.Fatalf("a refused clone saved %+v", projects)
			}
		})
	}

	t.Run("project_exists", func(t *testing.T) {
		f := browseAPIFixture(t, repoCloneCaps())
		dir := browseTestHome + "/reet/repos/acme/site"
		if _, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: dir, Name: "Mine"}); err != nil {
			t.Fatal(err)
		}
		f.answering(func(terminalproto.Body) terminalproto.Body { return cloned("acme", "site") })
		id, _ := f.startClone(t, "acme/site", "")
		done := f.waitClone(t, id)
		if done["state"] != "failed" || done["code"] != "project_exists" || done["sentence"] != "studio already has a project for that folder." {
			t.Fatalf("job = %v", done)
		}
		if len(f.clonesSent()) != 1 {
			t.Fatal("the clone was not asked for")
		}
		if projects, _ := f.st.ListFolderProjectsFor(f.uid); len(projects) != 1 || projects[0].Origin != "" {
			t.Fatalf("projects %+v", projects)
		}
		if rows := projectEvents(t, f.st, f.uid, "project-add"); len(rows) != 0 {
			t.Fatalf("a failed save was audited %v", rows)
		}
	})

	t.Run("machine leaves mid-clone", func(t *testing.T) {
		f := browseAPIFixture(t, repoCloneCaps())
		f.answering(func(terminalproto.Body) terminalproto.Body { return nil })
		id, _ := f.startClone(t, "acme/site", "")
		f.waitHeld(t, 1)
		f.peers[f.agent].Close()
		done := f.waitClone(t, id)
		if done["state"] != "failed" || done["code"] != "machine_offline" || done["sentence"] != "studio is offline. Try again when it's back." {
			t.Fatalf("job = %v", done)
		}
	})

	t.Run("immediate", func(t *testing.T) {
		f := browseAPIFixture(t, repoCloneCaps())
		older := f.addMachine(t, "attic", browseCaps())
		offline := f.addMachine(t, "shed", repoCloneCaps())
		f.peers[offline].Close()
		for _, c := range []struct {
			name, body string
			status     int
			code       string
			field      string
			sentence   string
		}{
			{"bare name", fromGitHubBody(f.agent, "site", ""), 422, "invalid_repo", "repo", "Write the repository as owner/name, or paste its github.com link."},
			{"other host", fromGitHubBody(f.agent, "https://gitlab.com/acme/site", ""), 422, "invalid_repo", "repo", "Write the repository as owner/name, or paste its github.com link."},
			{"ssh remote", fromGitHubBody(f.agent, "git@github.com:acme/site.git", ""), 422, "invalid_repo", "repo", "Write the repository as owner/name, or paste its github.com link."},
			{"older machine", fromGitHubBody(older, "acme/site", ""), 409, "repo_clone_unsupported", "", "Update reet on attic to add projects from GitHub. It also needs git and the GitHub CLI."},
			{"offline machine", fromGitHubBody(offline, "acme/site", ""), 409, "machine_offline", "", "shed is offline. Try again when it's back."},
			{"unknown machine", fromGitHubBody("nope", "acme/site", ""), 422, "unknown_machine", "host", "That machine isn't enrolled on this account."},
			{"bad name", fromGitHubBody(f.agent, "acme/site", "a\nb"), 422, "invalid_name", "name", "Give the project a name of 1 to 80 characters, without line breaks or control characters."},
			{"unknown field", `{"host":"` + f.agent + `","repo":"acme/site","origin":"acme/site"}`, 400, "invalid_request", "", "That request isn't a project reet can read."},
		} {
			f.freshWrites()
			code, out := f.post(t, f.cookie, fromGitHubPath, c.body)
			if code != c.status || out["code"] != c.code || out["error"] != c.sentence || (c.field != "" && out["field"] != c.field) {
				t.Fatalf("%s = %d %v", c.name, code, out)
			}
		}
		if sent := f.clonesSent(); len(sent) != 0 {
			t.Fatalf("a refused add reached a machine: %+v", sent)
		}
	})
}

func TestAddFromGitHubNeverSendsToAnOlderMachine(t *testing.T) {
	f := browseAPIFixture(t, browseCaps()) // no repo-clone-v1
	code, out := f.post(t, f.cookie, fromGitHubPath, fromGitHubBody(f.agent, "acme/site", ""))
	if code != http.StatusConflict || out["code"] != "repo_clone_unsupported" {
		t.Fatalf("older machine = %d %v", code, out)
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatalf("a clone reached an older machine: %+v", sent)
	}
	if n := f.srv.s.clones.running(f.uid); n != 0 {
		t.Fatalf("%d jobs started", n)
	}
}

func TestAddFromGitHubOfAnAdoptedUnmarkedFolderIsNotCloned(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.RepoCloneResult{Dir: browseTestHome + "/reet/repos/acme/site", Adopted: true}
	})
	id, _ := f.startClone(t, "acme/site", "")
	done := f.waitClone(t, id)
	project, _ := done["project"].(map[string]any)
	if done["state"] != "done" || project["origin"] != "acme/site" || project["baseBranch"] != "" {
		t.Fatalf("job = %v", done)
	}
	if _, present := project["cloned"]; present {
		t.Fatalf("an unmarked folder reads as cloned: %v", project)
	}
	saved, err := f.st.FolderProjectFor(f.uid, project["id"].(string))
	if err != nil || saved.Cloned || saved.Origin != "acme/site" {
		t.Fatalf("stored %+v %v", saved, err)
	}
	if rows := projectEvents(t, f.st, f.uid, "project-add"); len(rows) != 1 || rows[0]["cloned"] != false {
		t.Fatalf("audit %v", rows)
	}
}

func TestAddFromGitHubCountsTowardsTheProjectLimit(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	if _, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: "/taken", Name: "Taken"}); err != nil {
		t.Fatal(err)
	}
	code, out := f.post(t, f.cookie, fromGitHubPath, fromGitHubBody(f.agent, "acme/site", "taken"))
	if code != http.StatusConflict || out["code"] != "name_taken" || out["error"] != "Another project on this account already has that name." {
		t.Fatalf("taken name = %d %v", code, out)
	}
	for i := 1; i < store.MaxFolderProjects; i++ {
		if _, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: fmt.Sprintf("/d/%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	code, out = f.post(t, f.cookie, fromGitHubPath, fromGitHubBody(f.agent, "acme/site", ""))
	if code != http.StatusConflict || out["code"] != "project_limit" || out["error"] != "You have 100 projects, the most reet keeps. Remove one first." {
		t.Fatalf("101st = %d %v", code, out)
	}
	if sent := f.clonesSent(); len(sent) != 0 {
		t.Fatalf("a clone was asked for past the limit: %+v", sent)
	}
}

func TestCloneStatusIsPerAccount(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return cloned("acme", "site") })
	id, _ := f.startClone(t, "acme/site", "")
	f.waitClone(t, id)
	other, err := f.st.CreateUser("clone-other@example.test", "Other", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{id, strings.Repeat("0", 32), "not-an-id"} {
		cookie := terminalLogin(t, f.st, other.ID)
		if path != id {
			cookie = f.cookie
		}
		code, out, raw := f.cloneStatus(t, cookie, path)
		if code != 404 || out["code"] != "clone_not_found" || out["error"] != "That clone isn't on this account." {
			t.Fatalf("%s = %d %s", path, code, raw)
		}
	}
	if code, out, _ := f.cloneStatus(t, f.cookie, id); code != 200 || out["state"] != "done" {
		t.Fatalf("owner lost the job: %d %v", code, out)
	}
}

func TestAddFromGitHubTwiceWhileCloningIsOneJob(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return nil })
	first, _ := f.startClone(t, "acme/site", "")
	f.waitHeld(t, 1)
	again, out := f.startClone(t, "https://github.com/ACME/Site.git", "Other name")
	if again != first || out["dir"] != "~/reet/repos/acme/site" {
		t.Fatalf("second start = %v, want job %s", out, first)
	}
	other, _ := f.startClone(t, "acme/docs", "")
	if other == first {
		t.Fatal("another repository joined the first job")
	}
	f.waitHeld(t, 2)
	if sent := f.clonesSent(); len(sent) != 2 {
		t.Fatalf("sent %+v", sent)
	}
	f.release(t, cloned("acme", "site"))
	f.release(t, cloned("acme", "docs"))
	f.waitClone(t, first)
	f.waitClone(t, other)
	// Once it has ended, the same repository starts a new job.
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneExists}
	})
	if later, _ := f.startClone(t, "acme/site", ""); later == first {
		t.Fatal("a finished job was joined")
	}
}

func TestAddFromGitHubRunsAtMostFiveClonesPerAccount(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return nil })
	var ids []string
	for _, repo := range []string{"a", "b", "c", "d", "e"} {
		id, _ := f.startClone(t, "acme/"+repo, "")
		ids = append(ids, id)
	}
	f.waitHeld(t, 5)
	code, out := f.post(t, f.cookie, fromGitHubPath, fromGitHubBody(f.agent, "acme/f", ""))
	if code != http.StatusTooManyRequests || out["code"] != "clone_limit" || out["error"] != "Five repositories are already being cloned. Wait for one to finish." {
		t.Fatalf("sixth = %d %v", code, out)
	}
	if len(f.clonesSent()) != 5 {
		t.Fatal("the sixth clone was sent")
	}
	f.release(t, cloned("acme", "a"))
	f.waitClone(t, ids[0])
	f.startClone(t, "acme/f", "")
	f.waitHeld(t, 5)
	for range 5 {
		f.release(t, terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed})
	}
}

func TestFinishedClonesAreKeptTenMinutes(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	var mu sync.Mutex
	now := time.Now()
	f.srv.s.clones.setClock(func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	f.answering(func(terminalproto.Body) terminalproto.Body { return cloned("acme", "site") })
	id, _ := f.startClone(t, "acme/site", "")
	f.waitClone(t, id)
	mu.Lock()
	now = now.Add(10*time.Minute - time.Second)
	mu.Unlock()
	if code, _, raw := f.cloneStatus(t, f.cookie, id); code != 200 {
		t.Fatalf("dropped early: %d %s", code, raw)
	}
	mu.Lock()
	now = now.Add(2 * time.Second)
	mu.Unlock()
	if code, out, raw := f.cloneStatus(t, f.cookie, id); code != 404 || out["code"] != "clone_not_found" {
		t.Fatalf("kept past ten minutes: %d %s", code, raw)
	}
}

// Closing the server ends a clone that is still running, so its goroutine
// never outlives the server.
func TestServerCloseEndsARunningClone(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body { return nil })
	id, _ := f.startClone(t, "acme/site", "")
	f.waitHeld(t, 1)
	done := make(chan struct{})
	go func() { _ = f.srv.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close waited on a clone")
	}
	if n := f.srv.s.clones.running(f.uid); n != 0 {
		t.Fatalf("%d clones still running after Close", n)
	}
	f.srv.s.clones.mu.Lock()
	state := f.srv.s.clones.jobs[id].state
	f.srv.s.clones.mu.Unlock()
	if state != cloneFailed {
		t.Fatalf("state after Close = %s", state)
	}
}

// Where a project came from is a stored fact: the ordinary create and edit
// refuse origin and cloned as fields they don't know.
func TestOriginAndClonedAreNotSettableThroughTheNormalRoutes(t *testing.T) {
	f := projectFixture(t)
	id := f.fpCreate(t, nil)["id"].(string)
	for _, field := range []string{`"origin":"acme/site"`, `"cloned":true`, `"cloned":false`, `"origin":""`} {
		body := `{"host":"` + f.agent.ID + `","dir":"/home/tester/elsewhere",` + field + `}`
		for _, rt := range []struct{ method, path string }{{"POST", "/api/v2/folder-projects"}, {"PATCH", "/api/v2/folder-projects/" + id}} {
			f.resetFolderWrites()
			res := f.fpDo(t, f.cookie, rt.method, rt.path, body, "")
			if res.code != 400 || res.out["code"] != "invalid_request" {
				t.Fatal(field, rt.method, res.code, res.raw)
			}
		}
	}
	saved, err := f.st.FolderProjectFor(f.uid, id)
	if err != nil || saved.Origin != "" || saved.Cloned || saved.Version != 1 {
		t.Fatalf("stored %+v %v", saved, err)
	}
	one := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects/"+id, nil, "")
	if strings.Contains(one.raw, `"origin"`) || strings.Contains(one.raw, `"cloned"`) {
		t.Fatalf("a project added by path shows an origin: %s", one.raw)
	}
}

// An empty repository has no default branch: the project is saved with a
// blank base rather than one that does not exist.
func TestAddFromGitHubSavesAnEmptyRepositoryWithABlankBase(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.RepoCloneResult{Dir: browseTestHome + "/reet/repos/acme/empty", Marked: true}
	})
	id, _ := f.startClone(t, "acme/empty", "")
	done := f.waitClone(t, id)
	if done["state"] != "done" {
		t.Fatalf("job = %v", done)
	}
	project, _ := done["project"].(map[string]any)
	if project["baseBranch"] != "" || project["isRepo"] != true || project["cloned"] != true {
		t.Fatalf("project = %v", project)
	}
	saved, err := f.st.FolderProjectFor(f.uid, project["id"].(string))
	if err != nil || saved.BaseBranch != "" || !saved.IsRepo {
		t.Fatalf("stored %+v %v", saved, err)
	}
}

// Editing a cloned project's folder or machine clears where it came from in
// the same write; an edit that leaves both alone keeps it.
func TestEditingAClonedProjectsFolderClearsItsOrigin(t *testing.T) {
	f := projectFixture(t)
	made, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: "/home/tester/reet/repos/acme/site", IsRepo: true, BaseBranch: "main", Origin: "acme/site", Cloned: true})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v2/folder-projects/" + made.ID
	res := f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"name": "Site"}, `"1"`)
	if res.code != 200 || res.out["origin"] != "acme/site" || res.out["cloned"] != true {
		t.Fatalf("rename = %d %s", res.code, res.raw)
	}
	f.resetFolderWrites()
	res = f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"dir": "/home/tester/code/site"}, `"2"`)
	if res.code != 200 || strings.Contains(res.raw, `"origin"`) || strings.Contains(res.raw, `"cloned"`) {
		t.Fatalf("move = %d %s", res.code, res.raw)
	}
	saved, err := f.st.FolderProjectFor(f.uid, made.ID)
	if err != nil || saved.Origin != "" || saved.Cloned || saved.Dir != "/home/tester/code/site" || saved.Name != "Site" {
		t.Fatalf("stored %+v %v", saved, err)
	}
}
