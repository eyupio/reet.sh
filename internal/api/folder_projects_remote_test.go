package api

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func inspectCaps() terminalproto.Capabilities {
	caps := browseCaps()
	caps.Inspect = true
	return caps
}

func (f *browseAPI) folderRepo(t *testing.T) store.FolderProject {
	t.Helper()
	p, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: browseTestHome + "/code/site", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *browseAPI) inspectsSent() []terminalproto.Inspect {
	var out []terminalproto.Inspect
	for _, body := range f.sentRequests() {
		if c, ok := body.(terminalproto.Inspect); ok {
			out = append(out, c)
		}
	}
	return out
}

// A repository added as a folder learns its GitHub repository from its
// machine, and the projects list shows it.
func TestFolderRepoLearnsItsGitHubRepository(t *testing.T) {
	f := browseAPIFixture(t, inspectCaps())
	p := f.folderRepo(t)
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.InspectResult{Repo: true, Branch: "main", Remote: "acme/site"}
	})
	f.srv.s.learnFolderRemotes(time.Now())
	if sent := f.inspectsSent(); len(sent) != 1 || sent[0] != (terminalproto.Inspect{Dir: p.Dir, View: "status"}) {
		t.Fatalf("asked %+v", sent)
	}
	got, _ := f.st.FolderProjectFor(f.uid, p.ID)
	if got.Remote != "acme/site" || got.RemoteCheckedAt == 0 || got.Version != p.Version {
		t.Fatalf("recorded %+v", got)
	}
	code, _, raw := f.get(t, f.cookie, "/api/v2/folder-projects")
	if code != 200 || !strings.Contains(raw, `"remote":"acme/site"`) {
		t.Fatal(code, raw)
	}
	// Asked, so not asked again within the day.
	f.srv.s.learnFolderRemotes(time.Now())
	if sent := f.inspectsSent(); len(sent) != 1 {
		t.Fatalf("asked again at once: %+v", sent)
	}
	// A day later it is asked again, and a folder with no GitHub origin now says so.
	f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.InspectResult{Repo: true} })
	f.srv.s.learnFolderRemotes(time.Now().Add(folderRemoteEvery + time.Minute))
	if again, _ := f.st.FolderProjectFor(f.uid, p.ID); again.Remote != "" || again.RemoteCheckedAt <= got.RemoteCheckedAt {
		t.Fatalf("after the origin went: %+v", again)
	}
}

// A machine that cannot answer is asked again later: nothing is recorded.
func TestFolderRepoRemoteWaitsForAnAnswer(t *testing.T) {
	t.Run("busy", func(t *testing.T) {
		f := browseAPIFixture(t, inspectCaps())
		p := f.folderRepo(t)
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.InspectResult{Reason: "busy"} })
		now := time.Now()
		f.srv.s.learnFolderRemotes(now)
		if got, _ := f.st.FolderProjectFor(f.uid, p.ID); got.RemoteCheckedAt != 0 {
			t.Fatalf("a busy answer was recorded: %+v", got)
		}
		// Not asked again on the next pass, but once the retry time has passed.
		f.srv.s.learnFolderRemotes(now.Add(time.Minute))
		if sent := f.inspectsSent(); len(sent) != 1 {
			t.Fatalf("asked again at once: %d", len(sent))
		}
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.InspectResult{Remote: "acme/site"} })
		f.srv.s.learnFolderRemotes(now.Add(folderRemoteRetry + time.Minute))
		if got, _ := f.st.FolderProjectFor(f.uid, p.ID); got.Remote != "acme/site" {
			t.Fatalf("not asked again after the retry time: %+v", got)
		}
	})
	t.Run("older machine", func(t *testing.T) {
		f := browseAPIFixture(t, browseCaps())
		p := f.folderRepo(t)
		f.srv.s.learnFolderRemotes(time.Now())
		if sent := f.inspectsSent(); len(sent) != 0 {
			t.Fatalf("an older machine was asked: %+v", sent)
		}
		if got, _ := f.st.FolderProjectFor(f.uid, p.ID); got.RemoteCheckedAt != 0 {
			t.Fatalf("recorded %+v", got)
		}
	})
}
