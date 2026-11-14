package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// cloneRemoveCaps is a machine that can report on a clone and delete it.
func cloneRemoveCaps() terminalproto.Capabilities {
	caps := repoCloneCaps()
	caps.RepoCloneRemove = true
	return caps
}

const (
	cloneFingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	staleFingerprint = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	siteCloneDir     = browseTestHome + "/reet/repos/acme/site"
)

// cleanClone is the machine's answer for a clone reet made with nothing in it
// that is not on GitHub.
func cleanClone() terminalproto.RepoCloneState {
	return terminalproto.RepoCloneState{Marked: true, Fingerprint: cloneFingerprint}
}

// clonedProject saves a project reet cloned from acme/site on the machine.
func (f *browseAPI) clonedProject(t *testing.T) store.FolderProject {
	t.Helper()
	p, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: siteCloneDir, IsRepo: true, BaseBranch: "main", Origin: "acme/site", Cloned: true})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// machineClone answers a status with state and a remove with removed.
func (f *browseAPI) machineClone(state terminalproto.RepoCloneState, removed terminalproto.RepoCloneRemoved) {
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.RepoCloneRemove); ok {
			return removed
		}
		return state
	})
}

func (f *browseAPI) cloneRoute(t *testing.T, cookie, id string) (int, map[string]any, string) {
	t.Helper()
	f.freshWrites()
	return f.get(t, cookie, "/api/v2/folder-projects/"+id+"/clone")
}

// remove sends DELETE for project id as cookie, with body when not "".
func (f *browseAPI) remove(t *testing.T, cookie, id, body, ifMatch string) (int, map[string]any) {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest("DELETE", "/api/v2/folder-projects/"+id, nil)
	} else {
		r = httptest.NewRequest("DELETE", "/api/v2/folder-projects/"+id, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Origin", "https://reet.example")
	if ifMatch != "" {
		r.Header.Set("If-Match", ifMatch)
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func deleteCloneBody(fingerprint string) string {
	raw, _ := json.Marshal(map[string]any{"deleteClone": true, "fingerprint": fingerprint})
	return string(raw)
}

func (f *browseAPI) statusesSent() []terminalproto.RepoCloneStatus {
	var out []terminalproto.RepoCloneStatus
	for _, body := range f.sentRequests() {
		if c, ok := body.(terminalproto.RepoCloneStatus); ok {
			out = append(out, c)
		}
	}
	return out
}

func (f *browseAPI) removesSent() []terminalproto.RepoCloneRemove {
	var out []terminalproto.RepoCloneRemove
	for _, body := range f.sentRequests() {
		if c, ok := body.(terminalproto.RepoCloneRemove); ok {
			out = append(out, c)
		}
	}
	return out
}

func (f *browseAPI) projectKept(t *testing.T, id string) {
	t.Helper()
	if _, err := f.st.FolderProjectFor(f.uid, id); err != nil {
		t.Fatalf("the project is gone: %v", err)
	}
	if rows := projectEvents(t, f.st, f.uid, "project-remove"); len(rows) != 0 {
		t.Fatalf("a removal that did not happen was audited: %v", rows)
	}
}

func TestRemoveProjectDeletesItsCloneWhenAsked(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	p := f.clonedProject(t)
	f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})

	code, state, raw := f.cloneRoute(t, f.cookie, p.ID)
	if code != 200 || state["removable"] != true || state["fingerprint"] != cloneFingerprint || state["sentence"] != "" {
		t.Fatalf("clone = %d %s", code, raw)
	}
	for _, count := range []string{"worktrees", "changed", "stashes", "unpushed"} {
		if state[count] != float64(0) {
			t.Fatalf("%s = %v in %s", count, state[count], raw)
		}
	}
	if sent := f.statusesSent(); len(sent) != 1 || sent[0] != (terminalproto.RepoCloneStatus{Dir: siteCloneDir, Origin: "acme/site"}) {
		t.Fatalf("status sent %+v", sent)
	}

	f.freshWrites()
	code, out := f.remove(t, f.cookie, p.ID, deleteCloneBody(cloneFingerprint), `"1"`)
	if code != 200 || out["ok"] != true || out["cloneDeleted"] != true || out["message"] != "Project removed. ~/reet/repos/acme/site was deleted from studio." {
		t.Fatalf("remove = %d %v", code, out)
	}
	if sent := f.removesSent(); len(sent) != 1 || sent[0] != (terminalproto.RepoCloneRemove{Dir: siteCloneDir, Origin: "acme/site", Fingerprint: cloneFingerprint}) {
		t.Fatalf("remove sent %+v", sent)
	}
	if _, err := f.st.FolderProjectFor(f.uid, p.ID); err == nil {
		t.Fatal("the project is still there")
	}
	rows := projectEvents(t, f.st, f.uid, "project-remove")
	if len(rows) != 1 || rows[0]["cloneDeleted"] != true || rows[0]["clonePath"] != siteCloneDir || rows[0]["project"] != p.ID || rows[0]["host"] != "studio" || rows[0]["folder"] != siteCloneDir {
		t.Fatalf("audit %v", rows)
	}
}

// Review Focus 5: the clone gained work between the dialog and the click.
func TestRemoveProjectKeepsTheProjectWhenTheCloneChanged(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	p := f.clonedProject(t)
	f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneChanged})
	code, out := f.remove(t, f.cookie, p.ID, deleteCloneBody(cloneFingerprint), `"1"`)
	if code != http.StatusConflict || out["code"] != "clone_changed" || out["error"] != "The clone changed just now, so nothing was removed. Check again." {
		t.Fatalf("remove = %d %v", code, out)
	}
	if len(f.removesSent()) != 1 {
		t.Fatal("the machine was not asked")
	}
	f.projectKept(t, p.ID)

	// Removable when shown, not removable when asked: also "changed".
	f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneNotRemovable})
	f.freshWrites()
	code, out = f.remove(t, f.cookie, p.ID, deleteCloneBody(cloneFingerprint), "")
	if code != http.StatusConflict || out["code"] != "clone_changed" || out["error"] != "The clone changed just now, so nothing was removed. Check again." {
		t.Fatalf("not removable = %d %v", code, out)
	}
	f.projectKept(t, p.ID)
}

func TestRemoveProjectKeepsBothForEachRefusal(t *testing.T) {
	for _, c := range []struct {
		reason         terminalproto.RepoCloneReason
		code, sentence string
	}{
		{terminalproto.RepoCloneBusy, "clone_kept", "The clone on studio is kept: studio can't be asked right now."},
		{terminalproto.RepoCloneDenied, "clone_kept", "The clone on studio is kept: reet can't find it at ~/reet/repos/acme/site."},
		{terminalproto.RepoCloneExists, "clone_kept", "The clone on studio is kept: the folder there is no longer a plain clone of acme/site."},
		// A delete may have started: neither outcome is claimed.
		{terminalproto.RepoCloneFailed, "clone_unconfirmed", "reet couldn't confirm whether the clone on studio was removed. Check again."},
		{terminalproto.RepoCloneTimedOut, "clone_unconfirmed", "reet couldn't confirm whether the clone on studio was removed. Check again."},
	} {
		t.Run(string(c.reason), func(t *testing.T) {
			f := browseAPIFixture(t, cloneRemoveCaps())
			p := f.clonedProject(t)
			f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Reason: c.reason})
			code, out := f.remove(t, f.cookie, p.ID, deleteCloneBody(cloneFingerprint), `"1"`)
			if code != http.StatusConflict || out["code"] != c.code || out["error"] != c.sentence {
				t.Fatalf("remove = %d %v", code, out)
			}
			f.projectKept(t, p.ID)
		})
	}
}

// A lost answer says nothing about the folder: the project stays and neither
// outcome is claimed.
func TestRemoveProjectClaimsNothingWhenTheAnswerIsLost(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	p := f.clonedProject(t)
	f.answering(func(terminalproto.Body) terminalproto.Body { return nil })
	type answer struct {
		code int
		out  map[string]any
	}
	done := make(chan answer, 1)
	go func() {
		code, out := f.remove(t, f.cookie, p.ID, deleteCloneBody(cloneFingerprint), `"1"`)
		done <- answer{code, out}
	}()
	f.waitHeld(t, 1)
	f.peers[f.agent].Close()
	got := <-done
	if got.code != http.StatusConflict || got.out["code"] != "clone_unconfirmed" || got.out["error"] != "reet couldn't confirm whether the clone on studio was removed. Check again." {
		t.Fatalf("remove = %d %v", got.code, got.out)
	}
	f.projectKept(t, p.ID)
}

func TestRemoveProjectWithoutTheFlagNeverTouchesTheFolder(t *testing.T) {
	for name, body := range map[string]string{"no body": "", "empty object": "{}", "flag off": `{"deleteClone":false}`, "flag off with a fingerprint": `{"deleteClone":false,"fingerprint":"` + cloneFingerprint + `"}`} {
		t.Run(name, func(t *testing.T) {
			f := browseAPIFixture(t, cloneRemoveCaps())
			p := f.clonedProject(t)
			f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
			code, out := f.remove(t, f.cookie, p.ID, body, `"1"`)
			if code != 200 || out["message"] != "Project removed. Its sessions and any worktrees stay on studio." {
				t.Fatalf("remove = %d %v", code, out)
			}
			if _, present := out["cloneDeleted"]; present {
				t.Fatalf("answer = %v", out)
			}
			if sent := f.sentRequests(); len(sent) != 0 {
				t.Fatalf("the machine was asked %+v", sent)
			}
			rows := projectEvents(t, f.st, f.uid, "project-remove")
			if len(rows) != 1 {
				t.Fatalf("audit %v", rows)
			}
			if _, present := rows[0]["cloneDeleted"]; present {
				t.Fatalf("audit %v", rows[0])
			}
			if _, present := rows[0]["clonePath"]; present {
				t.Fatalf("audit %v", rows[0])
			}
		})
	}
}

func TestRemoveAnOrdinaryProjectIgnoresDeleteClone(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
	// One a person added by path, and one adopted from GitHub without reet's marker.
	plain, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: browseTestHome + "/code/site", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	adopted, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: siteCloneDir, IsRepo: true, Origin: "acme/site"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []store.FolderProject{plain, adopted} {
		f.freshWrites()
		code, out := f.remove(t, f.cookie, p.ID, deleteCloneBody(cloneFingerprint), `"1"`)
		if code != http.StatusUnprocessableEntity || out["code"] != "not_cloned" || out["error"] != "reet didn't clone that project's folder, so it won't delete it." {
			t.Fatalf("remove = %d %v", code, out)
		}
		code, out, raw := f.cloneRoute(t, f.cookie, p.ID)
		if code != http.StatusNotFound || out["code"] != "not_cloned" {
			t.Fatalf("clone = %d %s", code, raw)
		}
		if _, err := f.st.FolderProjectFor(f.uid, p.ID); err != nil {
			t.Fatalf("the project is gone: %v", err)
		}
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatalf("the machine was asked %+v", sent)
	}
	if rows := projectEvents(t, f.st, f.uid, "project-remove"); len(rows) != 0 {
		t.Fatalf("audit %v", rows)
	}
}

func TestCloneRouteGivesEachSentence(t *testing.T) {
	reason := func(r terminalproto.RepoCloneReason) terminalproto.RepoCloneState {
		return terminalproto.RepoCloneState{Reason: r}
	}
	counted := func(worktrees, changed, stashes, unpushed int) terminalproto.RepoCloneState {
		return terminalproto.RepoCloneState{Marked: true, Worktrees: worktrees, Changed: changed, Stashes: stashes, Unpushed: unpushed, Fingerprint: cloneFingerprint}
	}
	const cantAsk = "The clone on studio is kept: studio can't be asked right now."
	const work = "The clone on studio is kept: it holds work that isn't on GitHub."
	for _, c := range []struct {
		name     string
		state    terminalproto.RepoCloneState
		sentence string
	}{
		{"busy", reason(terminalproto.RepoCloneBusy), cantAsk},
		{"timed out", reason(terminalproto.RepoCloneTimedOut), cantAsk},
		{"failed", reason(terminalproto.RepoCloneFailed), cantAsk},
		{"denied", reason(terminalproto.RepoCloneDenied), "The clone on studio is kept: reet can't find it at ~/reet/repos/acme/site."},
		{"exists", reason(terminalproto.RepoCloneExists), "The clone on studio is kept: the folder there is no longer a plain clone of acme/site."},
		{"one worktree", counted(1, 0, 0, 0), "The clone on studio is kept: 1 session still has a worktree from it. Remove that session first."},
		{"two worktrees", counted(2, 0, 0, 0), "The clone on studio is kept: 2 sessions still have worktrees from it. Remove those sessions first."},
		{"worktrees before work", counted(3, 1, 1, 1), "The clone on studio is kept: 3 sessions still have worktrees from it. Remove those sessions first."},
		{"changed", counted(0, 1, 0, 0), work},
		{"stashes", counted(0, 0, 2, 0), work},
		{"unpushed", counted(0, 0, 0, 1), work},
		// What the counts cannot show (a nested repository, a rebase under
		// way, a lock): reet cannot say there is work, only that there may be.
		{"not removable", reason(terminalproto.RepoCloneNotRemovable), "The clone on studio is kept: it may hold work that isn't on GitHub."},
		{"not marked", terminalproto.RepoCloneState{}, "The clone on studio is kept: reet didn't make that folder."},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := browseAPIFixture(t, cloneRemoveCaps())
			p := f.clonedProject(t)
			f.machineClone(c.state, terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
			code, out, raw := f.cloneRoute(t, f.cookie, p.ID)
			if code != 200 || out["removable"] != false || out["sentence"] != c.sentence {
				t.Fatalf("clone = %d %s", code, raw)
			}
			// A clone that stays gives no fingerprint to delete it with.
			if _, present := out["fingerprint"]; present {
				t.Fatalf("a kept clone gave a fingerprint: %s", raw)
			}
			for count, want := range map[string]int{"worktrees": c.state.Worktrees, "changed": c.state.Changed, "stashes": c.state.Stashes, "unpushed": c.state.Unpushed} {
				if out[count] != float64(want) {
					t.Fatalf("%s = %v, want %d (%s)", count, out[count], want, raw)
				}
			}
			f.projectKept(t, p.ID)
		})
	}

	t.Run("offline", func(t *testing.T) {
		f := browseAPIFixture(t, cloneRemoveCaps())
		p := f.clonedProject(t)
		f.peers[f.agent].Close()
		code, out, raw := f.cloneRoute(t, f.cookie, p.ID)
		if code != 200 || out["removable"] != false || out["sentence"] != cantAsk {
			t.Fatalf("clone = %d %s", code, raw)
		}
		f.freshWrites()
		code, out = f.remove(t, f.cookie, p.ID, deleteCloneBody(cloneFingerprint), `"1"`)
		if code != http.StatusConflict || out["code"] != "clone_kept" || out["error"] != cantAsk {
			t.Fatalf("remove = %d %v", code, out)
		}
		f.projectKept(t, p.ID)
	})

	t.Run("older machine", func(t *testing.T) {
		f := browseAPIFixture(t, repoCloneCaps()) // repo-clone-v1 only
		p := f.clonedProject(t)
		code, out, raw := f.cloneRoute(t, f.cookie, p.ID)
		if code != 200 || out["removable"] != false || out["sentence"] != cantAsk {
			t.Fatalf("clone = %d %s", code, raw)
		}
		f.freshWrites()
		code, out = f.remove(t, f.cookie, p.ID, deleteCloneBody(cloneFingerprint), `"1"`)
		if code != http.StatusConflict || out["code"] != "clone_kept" || out["error"] != cantAsk {
			t.Fatalf("remove = %d %v", code, out)
		}
		if sent := f.sentRequests(); len(sent) != 0 {
			t.Fatalf("an older machine was asked %+v", sent)
		}
		f.projectKept(t, p.ID)
	})

	t.Run("a machine answer reet cannot read", func(t *testing.T) {
		f := browseAPIFixture(t, cloneRemoveCaps())
		p := f.clonedProject(t)
		f.answering(func(terminalproto.Body) terminalproto.Body {
			return terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed}
		})
		code, out, raw := f.cloneRoute(t, f.cookie, p.ID)
		if code != 200 || out["removable"] != false || out["sentence"] != cantAsk {
			t.Fatalf("clone = %d %s", code, raw)
		}
	})

	// The machine answers denied as its terminal access is switched off; the
	// controller hears of the switch and says so.
	t.Run("terminal access off", func(t *testing.T) {
		f := browseAPIFixture(t, cloneRemoveCaps())
		p := f.clonedProject(t)
		f.answering(func(terminalproto.Body) terminalproto.Body {
			record, err := f.st.RemoteCapabilitiesRawFor(f.uid, f.agent)
			if err != nil {
				t.Error(err)
			}
			off := record.Capabilities
			off.TerminalEnabled, off.Readiness = false, terminalproto.DisabledLocally
			if err := f.st.UpdateRemoteCapabilitiesFor(f.uid, f.agent, record.InstanceID, record.Generation, off); err != nil {
				t.Error(err)
			}
			return terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneDenied}
		})
		code, out, raw := f.cloneRoute(t, f.cookie, p.ID)
		if code != 200 || out["removable"] != false || out["sentence"] != "The clone on studio is kept: terminal access is off on studio." {
			t.Fatalf("clone = %d %s", code, raw)
		}
	})
}

// The stored folder is never trusted: only <home>/reet/repos/<owner>/<repo>
// for the stored origin is sent to a machine.
func TestCloneRouteNeverSendsAFolderThatIsNotTheClone(t *testing.T) {
	for name, p := range map[string]store.FolderProject{
		"another folder":     {Dir: browseTestHome + "/code/site", Origin: "acme/site"},
		"another repository": {Dir: browseTestHome + "/reet/repos/acme/docs", Origin: "acme/site"},
		"another case":       {Dir: browseTestHome + "/reet/repos/Acme/site", Origin: "acme/site"},
		"below the clone":    {Dir: siteCloneDir + "/sub", Origin: "acme/site"},
		"no home":            {Dir: "/reet/repos/acme/site", Origin: "acme/site"},
	} {
		t.Run(name, func(t *testing.T) {
			f := browseAPIFixture(t, cloneRemoveCaps())
			f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
			p.Host, p.IsRepo, p.Cloned = f.agent, true, true
			saved, err := f.st.CreateFolderProjectFor(f.uid, p)
			if err != nil {
				t.Fatal(err)
			}
			code, out, raw := f.cloneRoute(t, f.cookie, saved.ID)
			if code != 200 || out["removable"] != false || !strings.HasPrefix(out["sentence"].(string), "The clone on studio is kept: reet can't find it at ") {
				t.Fatalf("clone = %d %s", code, raw)
			}
			f.freshWrites()
			code, out = f.remove(t, f.cookie, saved.ID, deleteCloneBody(cloneFingerprint), `"1"`)
			if code != http.StatusConflict || out["code"] != "clone_kept" {
				t.Fatalf("remove = %d %v", code, out)
			}
			if sent := f.sentRequests(); len(sent) != 0 {
				t.Fatalf("the machine was asked %+v", sent)
			}
			f.projectKept(t, saved.ID)
		})
	}
}

func TestRemoveProjectWithItsCloneChecksTheRequestFirst(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	p := f.clonedProject(t)
	f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
	for _, c := range []struct {
		name, body, ifMatch string
		status              int
		code, sentence      string
	}{
		{"no fingerprint", `{"deleteClone":true}`, `"1"`, 422, "invalid_fingerprint", "Check the clone again before deleting it."},
		{"a fingerprint that is not one", deleteCloneBody("abc"), `"1"`, 422, "invalid_fingerprint", "Check the clone again before deleting it."},
		{"upper case", deleteCloneBody(strings.ToUpper(cloneFingerprint)), `"1"`, 422, "invalid_fingerprint", "Check the clone again before deleting it."},
		{"a path from the browser", `{"deleteClone":true,"fingerprint":"` + cloneFingerprint + `","dir":"/home/tester/code"}`, `"1"`, 400, "invalid_request", "That request isn't a project reet can read."},
		{"two values", deleteCloneBody(cloneFingerprint) + deleteCloneBody(cloneFingerprint), `"1"`, 400, "invalid_request", "That request isn't a project reet can read."},
		{"a stale version", deleteCloneBody(cloneFingerprint), `"7"`, 409, "project_changed", "Someone changed this project. Reload it, then try again."},
	} {
		f.freshWrites()
		code, out := f.remove(t, f.cookie, p.ID, c.body, c.ifMatch)
		if code != c.status || out["code"] != c.code || out["error"] != c.sentence {
			t.Fatalf("%s = %d %v", c.name, code, out)
		}
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatalf("a refused request reached the machine: %+v", sent)
	}
	f.projectKept(t, p.ID)
}

func TestCloneRouteAndRemoveAreForTheOwnerOnly(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	p := f.clonedProject(t)
	f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
	viewer, err := f.st.CreateUser("clone-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.st.CreateUser("clone-remove-other@example.test", "Other", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewerCookie, otherCookie := terminalLogin(t, f.st, viewer.ID), terminalLogin(t, f.st, other.ID)

	code, out, raw := f.cloneRoute(t, viewerCookie, p.ID)
	if code != http.StatusForbidden || out["code"] != "read_only" || out["error"] != "This account is read-only, so it can't use projects." {
		t.Fatalf("viewer clone = %d %s", code, raw)
	}
	code, out = f.remove(t, viewerCookie, p.ID, deleteCloneBody(cloneFingerprint), "")
	if code != http.StatusForbidden || out["code"] != "read_only" {
		t.Fatalf("viewer remove = %d %v", code, out)
	}
	code, out, raw = f.cloneRoute(t, otherCookie, p.ID)
	if code != http.StatusNotFound || out["code"] != "project_not_found" {
		t.Fatalf("other clone = %d %s", code, raw)
	}
	code, out = f.remove(t, otherCookie, p.ID, deleteCloneBody(cloneFingerprint), "")
	if code != http.StatusNotFound || out["code"] != "project_not_found" {
		t.Fatalf("other remove = %d %v", code, out)
	}
	code, out, raw = f.cloneRoute(t, "", p.ID)
	if code != http.StatusUnauthorized {
		t.Fatalf("signed out clone = %d %s", code, raw)
	}
	if code, _, _ := f.get(t, f.cookie, "/api/v2/folder-projects/"+p.ID+"/elsewhere"); code != http.StatusNotFound {
		t.Fatalf("another path under a project = %d", code)
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatalf("the machine was asked %+v", sent)
	}
	f.projectKept(t, p.ID)
}

// Both routes make a machine run git, so both count as project writes.
func TestCloneRouteAndRemoveAreRateLimited(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	p := f.clonedProject(t)
	f.machineClone(terminalproto.RepoCloneState{}, terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneChanged})
	f.freshWrites()
	path := "/api/v2/folder-projects/" + p.ID + "/clone"
	for i := range folderProjectWriteLimit {
		if code, _, raw := f.get(t, f.cookie, path); code != 200 {
			t.Fatalf("read %d = %d %s", i, code, raw)
		}
	}
	code, out, raw := f.get(t, f.cookie, path)
	if code != http.StatusTooManyRequests || out["code"] != "project_rate_limited" {
		t.Fatalf("read past the limit = %d %s", code, raw)
	}
	code, out = f.remove(t, f.cookie, p.ID, deleteCloneBody(cloneFingerprint), "")
	if code != http.StatusTooManyRequests || out["code"] != "project_rate_limited" {
		t.Fatalf("remove past the limit = %d %v", code, out)
	}
	if n := len(f.statusesSent()); n != folderProjectWriteLimit || len(f.removesSent()) != 0 {
		t.Fatalf("%d statuses and %d removes reached the machine", n, len(f.removesSent()))
	}
}

func TestCloneFolderIsOnlyTheCloneOfTheStoredOrigin(t *testing.T) {
	for _, c := range []struct {
		name string
		p    store.FolderProject
		ok   bool
	}{
		{"the clone", store.FolderProject{Dir: "/home/u/reet/repos/acme/site", Origin: "acme/site", Cloned: true}, true},
		{"home at the root", store.FolderProject{Dir: "/root/reet/repos/a/b", Origin: "a/b", Cloned: true}, true},
		{"not cloned", store.FolderProject{Dir: "/home/u/reet/repos/acme/site", Origin: "acme/site"}, false},
		{"no origin", store.FolderProject{Dir: "/home/u/reet/repos/acme/site", Cloned: true}, false},
		{"an origin with more path", store.FolderProject{Dir: "/home/u/reet/repos/acme/site/x", Origin: "acme/site/x", Cloned: true}, false},
		{"an origin that climbs", store.FolderProject{Dir: "/home/u/reet/repos/acme/..", Origin: "acme/..", Cloned: true}, false},
		{"another folder", store.FolderProject{Dir: "/home/u/code/site", Origin: "acme/site", Cloned: true}, false},
		{"another repository", store.FolderProject{Dir: "/home/u/reet/repos/acme/docs", Origin: "acme/site", Cloned: true}, false},
		{"another case", store.FolderProject{Dir: "/home/u/reet/repos/Acme/site", Origin: "acme/site", Cloned: true}, false},
		{"below the clone", store.FolderProject{Dir: "/home/u/reet/repos/acme/site/sub", Origin: "acme/site", Cloned: true}, false},
		{"no home", store.FolderProject{Dir: "/reet/repos/acme/site", Origin: "acme/site", Cloned: true}, false},
		{"a relative home", store.FolderProject{Dir: "home/u/reet/repos/acme/site", Origin: "acme/site", Cloned: true}, false},
		{"not a clean path", store.FolderProject{Dir: "/home/u/x/../reet/repos/acme/site", Origin: "acme/site", Cloned: true}, false},
		{"a trailing slash", store.FolderProject{Dir: "/home/u/reet/repos/acme/site/", Origin: "acme/site", Cloned: true}, false},
	} {
		dir, origin, ok := cloneFolder(c.p)
		if ok != c.ok || (ok && (dir != c.p.Dir || origin != c.p.Origin)) || (!ok && (dir != "" || origin != "")) {
			t.Errorf("%s: cloneFolder = %q %q %v", c.name, dir, origin, ok)
		}
	}
}
