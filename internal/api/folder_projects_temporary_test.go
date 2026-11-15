package api

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// temporaryProject saves a project reet cloned from acme/site for one start.
func (f *browseAPI) temporaryProject(t *testing.T) store.FolderProject {
	t.Helper()
	p, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: siteCloneDir, IsRepo: true, BaseBranch: "main", Origin: "acme/site", Cloned: true, Temporary: true})
	if err != nil || !p.Temporary {
		t.Fatal(err, p)
	}
	return p
}

// later is when the grace for a project made now is over.
func later() time.Time { return time.Now().Add(temporaryGrace + time.Minute) }

func TestTemporaryProjectGoesWithItsCloneOnceDoneWith(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	p := f.temporaryProject(t)
	ordinary := f.clonedProject2(t)
	f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
	f.srv.s.sweepTemporaryProjects(later())
	if sent := f.removesSent(); len(sent) != 1 || sent[0] != (terminalproto.RepoCloneRemove{Dir: siteCloneDir, Origin: "acme/site", Fingerprint: cloneFingerprint}) {
		t.Fatalf("remove sent %+v", sent)
	}
	if _, err := f.st.FolderProjectFor(f.uid, p.ID); err == nil {
		t.Fatal("the temporary project is still there")
	}
	rows := projectEvents(t, f.st, f.uid, "project-remove")
	if len(rows) != 1 || rows[0]["temporary"] != true || rows[0]["cloneDeleted"] != true || rows[0]["clonePath"] != siteCloneDir || rows[0]["project"] != p.ID {
		t.Fatalf("audit %v", rows)
	}
	// An ordinary project from GitHub is never touched.
	if _, err := f.st.FolderProjectFor(f.uid, ordinary.ID); err != nil {
		t.Fatal("an ordinary project was removed", err)
	}
	for _, s := range f.statusesSent() {
		if s.Dir != siteCloneDir {
			t.Fatalf("an ordinary project's machine was asked: %+v", s)
		}
	}
}

// clonedProject2 is an ordinary project reet cloned from acme/docs.
func (f *browseAPI) clonedProject2(t *testing.T) store.FolderProject {
	t.Helper()
	p, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: browseTestHome + "/reet/repos/acme/docs", IsRepo: true, Origin: "acme/docs", Cloned: true})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// While it is new or something uses it, nothing is asked and it stays quietly.
func TestTemporaryProjectWaitsWhileNewOrInUse(t *testing.T) {
	t.Run("new", func(t *testing.T) {
		f := browseAPIFixture(t, cloneRemoveCaps())
		p := f.temporaryProject(t)
		f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
		f.srv.s.sweepTemporaryProjects(time.Now())
		if sent := f.sentRequests(); len(sent) != 0 {
			t.Fatalf("asked within the grace: %+v", sent)
		}
		f.projectKept(t, p.ID)
	})
	t.Run("a session open in it", func(t *testing.T) {
		f := browseAPIFixture(t, cloneRemoveCaps())
		p := f.temporaryProject(t)
		f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
		f.openSession(t, f.agent, siteCloneDir, terminalproto.Running, "")
		f.srv.s.sweepTemporaryProjects(later())
		if sent := f.sentRequests(); len(sent) != 0 {
			t.Fatalf("asked while in use: %+v", sent)
		}
		f.projectKept(t, p.ID)
		if got, _ := f.st.FolderProjectFor(f.uid, p.ID); got.KeptBecause != "" {
			t.Fatalf("in use is not a reason to show: %q", got.KeptBecause)
		}
	})
	t.Run("a Factory outcome still running on it", func(t *testing.T) {
		f := browseAPIFixture(t, cloneRemoveCaps())
		p := f.temporaryProject(t)
		f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
		if _, err := f.st.CreateGafferOutcomeFor(f.uid, store.GafferOutcome{Objective: "Ship it", State: "starting", ProjectID: p.ID, AgentID: f.agent, Dir: p.Dir}); err != nil {
			t.Fatal(err)
		}
		f.srv.s.sweepTemporaryProjects(later())
		if sent := f.sentRequests(); len(sent) != 0 {
			t.Fatalf("asked while an outcome runs: %+v", sent)
		}
		f.projectKept(t, p.ID)
	})
}

// When deleting the clone would lose something, the project stays and says
// why; once it no longer would, the next look removes it.
func TestTemporaryProjectStaysAndSaysWhyWhileWorkWouldBeLost(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	p := f.temporaryProject(t)
	f.machineClone(terminalproto.RepoCloneState{Marked: true, Unpushed: 1, Fingerprint: cloneFingerprint}, terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
	f.srv.s.sweepTemporaryProjects(later())
	f.projectKept(t, p.ID)
	if len(f.removesSent()) != 0 {
		t.Fatal("a remove was sent for a clone with work in it")
	}
	got, _ := f.st.FolderProjectFor(f.uid, p.ID)
	if got.KeptBecause != "The clone on studio is kept: it holds work that isn't on GitHub." || got.Version != p.Version {
		t.Fatalf("kept because %q, version %d", got.KeptBecause, got.Version)
	}
	code, out, raw := f.get(t, f.cookie, "/api/v2/folder-projects")
	if code != 200 || !strings.Contains(raw, `"temporary":true`) || !strings.Contains(raw, `"keptBecause":"The clone on studio is kept: it holds work that isn't on GitHub."`) {
		t.Fatalf("list = %d %v", code, out)
	}
	f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
	f.srv.s.sweepTemporaryProjects(later())
	if _, err := f.st.FolderProjectFor(f.uid, p.ID); err == nil {
		t.Fatal("still there once nothing would be lost")
	}
}

// A machine that cannot be asked is tried again later; nothing is claimed.
func TestTemporaryProjectWaitsForItsMachine(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	p := f.temporaryProject(t)
	f.srv.s.sweepTemporaryProjects(later())
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatalf("an older machine was asked: %+v", sent)
	}
	f.projectKept(t, p.ID)
	if got, _ := f.st.FolderProjectFor(f.uid, p.ID); got.KeptBecause != "" {
		t.Fatalf("said %q", got.KeptBecause)
	}
}

// Keep makes it an ordinary project for good.
func TestKeepATemporaryProject(t *testing.T) {
	f := browseAPIFixture(t, cloneRemoveCaps())
	p := f.temporaryProject(t)
	code, out := f.post(t, f.cookie, "/api/v2/folder-projects/"+p.ID+"/keep", `{}`)
	if code != 200 || out["temporary"] != nil || out["id"] != p.ID {
		t.Fatalf("keep = %d %v", code, out)
	}
	if rows := projectEvents(t, f.st, f.uid, "project-keep"); len(rows) != 1 || rows[0]["project"] != p.ID {
		t.Fatalf("audit %v", rows)
	}
	f.machineClone(cleanClone(), terminalproto.RepoCloneRemoved{Dir: siteCloneDir})
	f.srv.s.sweepTemporaryProjects(later())
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatalf("a kept project's machine was asked: %+v", sent)
	}
	f.projectKept(t, p.ID)
	if code, out := f.post(t, f.cookie, "/api/v2/folder-projects/"+strings.Repeat("0", 32)+"/keep", `{}`); code != 404 {
		t.Fatalf("missing = %d %v", code, out)
	}
}

// Add a project From GitHub saves a temporary project only when asked to.
func TestFromGitHubSavesATemporaryProjectWhenAsked(t *testing.T) {
	f := browseAPIFixture(t, repoCloneCaps())
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.RepoClone); ok {
			return terminalproto.RepoCloneResult{Dir: siteCloneDir, DefaultBranch: "main", Marked: true}
		}
		return nil
	})
	code, out := f.post(t, f.cookie, fromGitHubPath, `{"host":"`+f.agent+`","repo":"acme/site","name":"","temporary":true}`)
	if code != 202 {
		t.Fatalf("start = %d %v", code, out)
	}
	done := f.waitClone(t, out["cloneId"].(string))
	project, _ := done["project"].(map[string]any)
	if done["state"] != "done" || project["temporary"] != true || project["origin"] != "acme/site" {
		t.Fatalf("clone %v", done)
	}
}
