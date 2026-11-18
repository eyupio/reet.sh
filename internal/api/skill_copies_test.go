package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillfetch"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

const findSkillsOrigin = "https://github.com/vercel-labs/skills/tree/HEAD/skills/find-skills"

// stubFetcher answers every fetch with what the test last set.
type stubFetcher struct {
	mu    sync.Mutex
	body  string
	err   error
	gate  chan struct{} // when set, a fetch waits for it to close
	calls int
}

func (f *stubFetcher) set(body string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.body, f.err = body, err
}

func (f *stubFetcher) Fetch(ctx context.Context, r ref.Ref) (skillfetch.Result, error) {
	f.mu.Lock()
	gate, body, err := f.gate, f.body, f.err
	f.calls++
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return skillfetch.Result{}, ctx.Err()
		}
	}
	if err != nil {
		return skillfetch.Result{}, err
	}
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: " + r.Slug + "\ndescription: Finds skills\n---\n" + body + "\n")}}
	m, e := skillbundle.FromContents(files)
	return skillfetch.Result{Files: files, Manifest: m, Origin: findSkillsOrigin}, e
}

type copyEnv struct {
	srv   *httptest.Server
	api   *Server
	st    *store.Store
	uid   int64
	fetch *stubFetcher
}

func newCopyEnv(t *testing.T) copyEnv {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	f := &stubFetcher{body: "v1"}
	h := New(st, deadFetcher(t), "", WithSkillFetcher(f))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	uid, _ := st.DefaultOwner()
	return copyEnv{srv, h, st, uid, f}
}

// settle waits for the background copy runs to finish.
func (e copyEnv) settle() { e.api.s.skillCopies.Wait() }

func (e copyEnv) item(t *testing.T) string {
	t.Helper()
	health, err := e.st.SkillHealthFor(e.uid, time.Now())
	if err != nil || len(health) != 1 {
		t.Fatalf("health: %+v %v", health, err)
	}
	for id := range health {
		return id
	}
	return ""
}

func (e copyEnv) health(t *testing.T) store.SkillHealth {
	t.Helper()
	m, err := e.st.SkillHealthFor(e.uid, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return m[e.item(t)]
}

func TestAddByLinkFetchesContent(t *testing.T) {
	e := newCopyEnv(t)
	code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now())
	if err != nil || len(d.Active) != 1 || d.Active[0].Name != "find-skills" || d.Active[0].Revision == "" {
		t.Fatalf("desired: %+v %v", d, err)
	}
	origin, reference, err := e.st.SkillOriginFor(e.uid, d.Active[0].Item)
	if err != nil || origin != "imported" || !strings.HasPrefix(reference, "https://github.com/") {
		t.Fatalf("origin %q reference %q %v", origin, reference, err)
	}
	if h := e.health(t); h.State == "getting" || h.State == "fetch-failed" {
		t.Fatalf("health: %+v", h)
	}
}

func TestAddByLinkShowsGettingWhileFetching(t *testing.T) {
	e := newCopyEnv(t)
	e.fetch.gate = make(chan struct{})
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if h := e.health(t); h.State != "getting" {
		t.Fatalf("while the fetch is pending: %+v", h)
	}
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now()); len(d.Active) != 0 {
		t.Fatalf("a skill with no content is not spread: %+v", d)
	}
	close(e.fetch.gate)
	e.settle()
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now()); len(d.Active) != 1 {
		t.Fatalf("desired after the fetch: %+v", d)
	}
}

func TestAddByLinkFetchFailureShowsProblem(t *testing.T) {
	e := newCopyEnv(t)
	e.fetch.set("", skillfetch.ErrNotFound)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	if h := e.health(t); h.State != "fetch-failed" || h.Reason == "" {
		t.Fatalf("health: %+v", h)
	}
	e.fetch.mu.Lock()
	calls := e.fetch.calls
	e.fetch.mu.Unlock()
	if calls != 1 {
		t.Fatalf("a failed skill is not fetched again until Retry: %d calls", calls)
	}
	e.fetch.set("v1", nil)
	for _, bad := range []string{`{}`, `{"items":[]}`, `nope`} {
		if code, _ := do(t, e.srv, "POST", "/api/v2/skills/retry", bad); code != 400 {
			t.Fatalf("retry with %q: %d", bad, code)
		}
	}
	code, body := do(t, e.srv, "POST", "/api/v2/skills/retry", `{"items":["`+e.item(t)+`"]}`)
	if code != 200 || !strings.Contains(body, `"retried":1`) {
		t.Fatalf("retry: %d %s", code, body)
	}
	e.settle()
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now()); len(d.Active) != 1 {
		t.Fatalf("desired after retry: %+v", d)
	}
}

func TestKickSkillCopiesRunsOnePerAccount(t *testing.T) {
	e := newCopyEnv(t)
	e.fetch.gate = make(chan struct{})
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	for range 5 {
		e.api.s.kickSkillCopies(e.uid)
	}
	close(e.fetch.gate)
	e.settle()
	e.fetch.mu.Lock()
	calls := e.fetch.calls
	e.fetch.mu.Unlock()
	if calls > 2 {
		t.Fatalf("kicks while a run is going share one run and one more: %d fetches", calls)
	}
}

func TestMachineOnlySkillIsCopiedFromItsMachine(t *testing.T) {
	e := newCopyEnv(t)
	if _, err := e.st.UpsertLocalFor(e.uid, "kept", store.Patch{}); err != nil {
		t.Fatal(err)
	}
	a, _, err := e.st.EnrollSyncAgentBuild(e.uid, "elitebook", "1.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.st.SetAgentSkillsProtocolFor(e.uid, a.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err = e.st.RecordEnvironmentFor(e.uid, "local/kept", "elitebook", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.ClaimRemoteImportFor(e.uid, a.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	report := store.SkillReport{Version: "v1", Entries: []store.SkillReportEntry{{Folder: "~/.agents/skills", Name: "kept", State: "installed", Digest: "d1"}}}
	if err = e.st.RecordSkillReportFor(e.uid, a.ID, report, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.api.s.kickSkillCopies(e.uid)
	e.settle()
	requests, _, err := e.st.RemoteImportsFor(e.uid)
	if err != nil || len(requests) != 1 || requests[0].SyncSkillID != "local/kept" || requests[0].AgentID != a.ID {
		t.Fatalf("a capture must be asked of the machine that has it: %+v %v", requests, err)
	}
	if h := e.health(t); h.State != "copying" {
		t.Fatalf("health: %+v", h)
	}
}

func TestOriginalChangesShowsDiffAndEdited(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	type reply struct {
		Changes  []struct{ Path, Kind string }
		State    string
		Digest   string
		Revision string
		Edited   bool
	}
	get := func() reply {
		t.Helper()
		code, body := do(t, e.srv, "GET", "/api/v2/skills/"+item+"/original", "")
		var r reply
		if code != 200 || json.Unmarshal([]byte(body), &r) != nil {
			t.Fatalf("GET original: %d %s", code, body)
		}
		return r
	}
	post := func(r reply) string { return `{"digest":"` + r.Digest + `","revision":"` + r.Revision + `"}` }
	if r := get(); r.Revision == "" {
		t.Fatalf("GET must return the revision it compared against: %+v", r)
	}
	if r := get(); r.State != "equal" || len(r.Changes) != 0 || r.Edited || r.Digest == "" {
		t.Fatalf("nothing changed: %+v", r)
	}
	// Posting when nothing changed makes no revision.
	if code, body := do(t, e.srv, "POST", "/api/v2/skills/"+item+"/original", post(get())); code != 200 || !strings.Contains(body, `"updated":false`) {
		t.Fatalf("no-op update: %d %s", code, body)
	}
	e.fetch.set("v2", nil)
	r := get()
	if r.State != "changed" || len(r.Changes) != 1 || r.Changes[0].Path != "SKILL.md" || r.Changes[0].Kind != "modified" || r.Edited {
		t.Fatalf("changed: %+v", r)
	}
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: find-skills\ndescription: Mine\n---\nedited\n")}}
	if _, err := e.st.SetSkillContentFor(e.uid, item, files, store.PrivateSkillOrigin{Kind: "edit"}); err != nil {
		t.Fatal(err)
	}
	if r = get(); !r.Edited {
		t.Fatalf("after an edit: %+v", r)
	}
	if code, _ := do(t, e.srv, "POST", "/api/v2/skills/"+item+"/original", `{"digest":"stale","revision":"`+r.Revision+`"}`); code != 409 {
		t.Fatalf("a stale digest: %d", code)
	}
	if edited, _ := e.st.SkillEditedSinceImportFor(e.uid, item); !edited {
		t.Fatal("a refused update must change nothing")
	}
	code, body := do(t, e.srv, "POST", "/api/v2/skills/"+item+"/original", post(r))
	if code != 200 || !strings.Contains(body, `"updated":true`) || !strings.Contains(body, `"edited":true`) {
		t.Fatalf("confirmed update: %d %s", code, body)
	}
	if after := get(); after.State != "equal" || after.Edited {
		t.Fatalf("after the update: %+v", after)
	}
	if code, _ := do(t, e.srv, "GET", "/api/v2/skills/nosuchitem/original", ""); code != 404 {
		t.Fatalf("unknown item: %d", code)
	}
}

func TestOriginalRefusesWhenTheSkillChangedDuringTheDownload(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	e.fetch.set("v2", nil)
	var r struct{ Digest, Revision string }
	code, body := do(t, e.srv, "GET", "/api/v2/skills/"+item+"/original", "")
	if code != 200 || json.Unmarshal([]byte(body), &r) != nil || r.Revision == "" {
		t.Fatalf("GET original: %d %s", code, body)
	}
	url := "/api/v2/skills/" + item + "/original"
	for _, in := range []string{`{"digest":"` + r.Digest + `"}`, `{"digest":"` + r.Digest + `","revision":""}`} {
		if code, body := do(t, e.srv, "POST", url, in); code != 400 {
			t.Fatalf("a missing revision is a 400: %d %s", code, body)
		}
	}
	// An edit lands after the person looked, before they confirmed.
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: find-skills\ndescription: Mine\n---\nedited during the download\n")}}
	edit, err := e.st.SetSkillContentFor(e.uid, item, files, store.PrivateSkillOrigin{Kind: "edit"})
	if err != nil {
		t.Fatal(err)
	}
	code, body = do(t, e.srv, "POST", url, `{"digest":"`+r.Digest+`","revision":"`+r.Revision+`"}`)
	if code != 409 || !strings.Contains(body, "changed") {
		t.Fatalf("an edit made since the revision was read must be refused: %d %s", code, body)
	}
	li, err := e.st.LibraryItemFor(e.uid, item)
	if err != nil || li.RevisionID != edit.Revision.ID {
		t.Fatalf("the edit must stay the head: %+v %v", li, err)
	}
	// Reading again gives the new revision, and that one is accepted.
	var again struct{ Digest, Revision string }
	code, body = do(t, e.srv, "GET", url, "")
	if code != 200 || json.Unmarshal([]byte(body), &again) != nil || again.Revision != edit.Revision.ID {
		t.Fatalf("GET after the edit: %d %s", code, body)
	}
	if code, body = do(t, e.srv, "POST", url, `{"digest":"`+again.Digest+`","revision":"`+again.Revision+`"}`); code != 200 || !strings.Contains(body, `"updated":true`) {
		t.Fatalf("the right revision succeeds: %d %s", code, body)
	}
}

func TestOriginalIsRefusedForASkillWithoutALink(t *testing.T) {
	e := newCopyEnv(t)
	item, err := e.st.UpsertLocalFor(e.uid, "mine", store.Patch{})
	if err != nil {
		t.Fatal(err)
	}
	_ = item
	health, _ := e.st.SkillHealthFor(e.uid, time.Now())
	for id := range health {
		if code, _ := do(t, e.srv, "GET", "/api/v2/skills/"+id+"/original", ""); code != 404 {
			t.Fatalf("a machine skill has no original: %d", code)
		}
	}
}

func TestSkillCopyDefaultFetchesNothing(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, deadFetcher(t), "")
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close() })
	if _, ok := h.s.skillFetcher.(noSkillFetcher); !ok {
		t.Fatalf("a server built without WithSkillFetcher must not reach the network: %T", h.s.skillFetcher)
	}
	if code, body := do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	h.s.skillCopies.Wait()
	uid, _ := st.DefaultOwner()
	m, _ := st.SkillHealthFor(uid, time.Now())
	for _, v := range m {
		if v.State != "getting" || v.Reason != "" {
			t.Fatalf("an unconfigured fetcher leaves the skill pending, not failed: %+v", v)
		}
	}
}
