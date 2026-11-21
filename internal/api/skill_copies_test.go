package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
	name  string        // when set, the front-matter name of the fetched SKILL.md
	calls int
}

func (f *stubFetcher) setGate(g chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gate = g
}

func (f *stubFetcher) gateChan() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gate
}

func (f *stubFetcher) setName(n string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.name = n
}

func (f *stubFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *stubFetcher) set(body string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.body, f.err = body, err
}

func (f *stubFetcher) Fetch(ctx context.Context, r ref.Ref) (skillfetch.Result, error) {
	f.mu.Lock()
	gate, body, err, name := f.gate, f.body, f.err, f.name
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
	if name == "" {
		name = r.Slug
	}
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: " + name + "\ndescription: Finds skills\n---\n" + body + "\n")}}
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
	h.s.originalTTL = 0 // a GET fetches every time unless a test turns the cache on
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
	e.fetch.setGate(make(chan struct{}))
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if h := e.health(t); h.State != "getting" {
		t.Fatalf("while the fetch is pending: %+v", h)
	}
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now()); len(d.Active) != 0 {
		t.Fatalf("a skill with no content is not spread: %+v", d)
	}
	close(e.fetch.gateChan())
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
	calls := e.fetch.callCount()
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
	e.fetch.setGate(make(chan struct{}))
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	for range 5 {
		e.api.s.kickSkillCopies(e.uid)
	}
	close(e.fetch.gateChan())
	e.settle()
	calls := e.fetch.callCount()
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
	if code, body := do(t, e.srv, "POST", url, `{"digest":"`+r.Digest+`"}`); code != 400 {
		t.Fatalf("a missing revision is a 400: %d %s", code, body)
	}
	// Revision "" says the person looked at a skill with no content; this one has some (ruling R38).
	if code, body := do(t, e.srv, "POST", url, `{"digest":"`+r.Digest+`","revision":""}`); code != 409 {
		t.Fatalf("revision \"\" for a skill with content is stale: %d %s", code, body)
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
	if _, err := e.st.UpsertLocalFor(e.uid, "mine", store.Patch{}); err != nil {
		t.Fatal(err)
	}
	health, err := e.st.SkillHealthFor(e.uid, time.Now())
	if err != nil || len(health) != 1 {
		t.Fatalf("the machine skill must be in the library: %+v %v", health, err)
	}
	for id := range health {
		for _, method := range []string{"GET", "POST"} {
			body := ""
			if method == "POST" {
				body = `{"digest":"d","revision":"r"}`
			}
			if code, _ := do(t, e.srv, method, "/api/v2/skills/"+id+"/original", body); code != 404 {
				t.Fatalf("%s: a machine skill has no original: %d", method, code)
			}
		}
	}
	if e.fetch.callCount() != 0 {
		t.Fatal("a skill without a link must not be fetched")
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

func TestKickSkillCopiesRacingCloseIsSafe(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	f := &stubFetcher{body: "v1"}
	h := New(st, deadFetcher(t), "", WithSkillFetcher(f))
	t.Cleanup(func() { h.Close(); st.Close() })
	uid, _ := st.DefaultOwner()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 20 {
				h.s.kickSkillCopies(uid)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		h.Close()
	}()
	close(start)
	wg.Wait()
	h.s.skillCopies.Wait()
	// Once closed, a kick starts nothing.
	h.s.kickSkillCopies(uid)
	h.s.skillCopyMu.Lock()
	running := h.s.skillCopyRunning[uid]
	h.s.skillCopyMu.Unlock()
	if running {
		t.Fatal("a kick after Close started a run")
	}
}

func TestOriginalStoreFailureIsNotAnUpstreamFailure(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"fetch failure", errors.New("connection reset"), 502},
		{"not found", skillfetch.ErrNotFound, 502},
		{"too large", skillfetch.ErrTooLarge, 502},
		{"archive too large", skillfetch.ErrArchiveTooLarge, 502},
	} {
		e.fetch.set("v1", tc.err)
		if code, body := do(t, e.srv, "GET", "/api/v2/skills/"+item+"/original", ""); code != tc.want {
			t.Fatalf("%s: %d %s", tc.name, code, body)
		}
	}
	// A store failure is ours, not the original's.
	rec := httptest.NewRecorder()
	originalError(rec, errors.New("sql: database is locked"))
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "original") {
		t.Fatalf("store failure: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	originalError(rec, fetchError{errors.New("dial tcp: refused")})
	if rec.Code != 502 {
		t.Fatalf("fetch failure: %d", rec.Code)
	}
}

func TestOriginalUpdateNeedsDigestAndRevision(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	url := "/api/v2/skills/" + e.item(t) + "/original"
	for _, in := range []string{`{"revision":"r"}`, `{"digest":"","revision":"r"}`} {
		if code, body := do(t, e.srv, "POST", url, in); code != 400 {
			t.Fatalf("an empty digest is a 400, like a missing revision: %q %d %s", in, code, body)
		}
	}
}

func TestSkillCopyBodiesAreSmallAndStrict(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	big := strings.Repeat("a", 100<<10)
	for name, c := range map[string]struct{ url, body string }{
		"retry too big":       {"/api/v2/skills/retry", `{"items":["` + big + `"]}`},
		"retry unknown field": {"/api/v2/skills/retry", `{"items":["` + item + `"],"extra":1}`},
		"retry trailing data": {"/api/v2/skills/retry", `{"items":["` + item + `"]} {}`},
		"update too big":      {"/api/v2/skills/" + item + "/original", `{"digest":"` + big + `","revision":"r"}`},
		"update unknown":      {"/api/v2/skills/" + item + "/original", `{"digest":"d","revision":"r","x":1}`},
	} {
		if code, body := do(t, e.srv, "POST", c.url, c.body); code != 400 {
			t.Fatalf("%s: %d %s", name, code, body)
		}
	}
	if code, _ := do(t, e.srv, "POST", "/api/v2/skills/retry", `{"items":["`+item+`"]}`); code != 200 {
		t.Fatalf("a normal retry still works: %d", code)
	}
	e.settle()
}

func TestOriginalGetIsCachedBrieflyAndPostRefetches(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	now := time.Now()
	e.api.s.originalTTL = time.Minute
	e.api.s.original.now = func() time.Time { return now }
	url := "/api/v2/skills/" + item + "/original"
	var r struct{ Digest, Revision, State string }
	get := func() {
		t.Helper()
		code, body := do(t, e.srv, "GET", url, "")
		if code != 200 || json.Unmarshal([]byte(body), &r) != nil {
			t.Fatalf("GET: %d %s", code, body)
		}
	}
	before := e.fetch.callCount()
	get()
	get()
	get()
	if n := e.fetch.callCount() - before; n != 1 {
		t.Fatalf("three GETs inside the window made %d fetches", n)
	}
	// A POST verifies against a fresh fetch.
	if code, body := do(t, e.srv, "POST", url, `{"digest":"`+r.Digest+`","revision":"`+r.Revision+`"}`); code != 200 {
		t.Fatalf("POST: %d %s", code, body)
	}
	if n := e.fetch.callCount() - before; n != 2 {
		t.Fatalf("POST must fetch again: %d fetches", n)
	}
	// After the window, a GET fetches again and sees the change.
	e.fetch.set("v2", nil)
	get()
	if r.State != "equal" {
		t.Fatalf("inside the window the answer is the cached one: %+v", r)
	}
	now = now.Add(2 * time.Minute)
	get()
	if r.State != "changed" {
		t.Fatalf("after the window the original is fetched again: %+v", r)
	}
}

func TestSkillCopyProblemWording(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{skillfetch.ErrTooLarge, "over 8 MiB or 256 files"},
		{skillfetch.ErrArchiveTooLarge, "repository is too large"},
		{fmt.Errorf("skill archive: %w", skillfetch.ErrArchiveTooLarge), "repository is too large"},
		{fmt.Errorf("wrapped: %w", skillfetch.ErrTooLarge), "over 8 MiB or 256 files"},
		{store.ErrPrivateSkillInvalid, "did not pass reet's checks"},
		{skillfetch.ErrNotFound, "no longer at its original link"},
	} {
		if got := skillCopyProblem(tc.err); !strings.Contains(got, tc.want) {
			t.Errorf("%v: %q does not say %q", tc.err, got, tc.want)
		}
	}
}

func TestFetchedNameThatDiffersFromTheItemSaysSo(t *testing.T) {
	e := newCopyEnv(t)
	e.fetch.setName("other-name")
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	h := e.health(t)
	if h.State != "fetch-failed" || !strings.Contains(h.Reason, "other-name") || !strings.Contains(h.Reason, "find-skills") || strings.Contains(h.Reason, "reet's checks") {
		t.Fatalf("the problem must say the names differ: %+v", h)
	}
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now()); len(d.Active) != 0 {
		t.Fatalf("nothing is saved: %+v", d)
	}
}

// Close must cancel under the lock a kick takes; otherwise a kick that already
// saw a live context can add to the worker group while Close is waiting on it.
func TestCloseCancelsUnderTheKickLock(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, deadFetcher(t), "", WithSkillFetcher(&stubFetcher{body: "v1"}))
	t.Cleanup(func() { h.Close(); st.Close() })
	h.s.skillCopyMu.Lock()
	closed := make(chan struct{})
	go func() { h.Close(); close(closed) }()
	time.Sleep(100 * time.Millisecond) // long enough for an unlocked Close to cancel
	cancelledWhileHeld := h.s.ctx.Err() != nil
	h.s.skillCopyMu.Unlock()
	<-closed
	if cancelledWhileHeld {
		t.Fatal("Close cancelled the context while a kick held the lock")
	}
	if h.s.ctx.Err() == nil {
		t.Fatal("Close must cancel the context")
	}
}

// The fetch picks the folder by a loose match of the name, so a SKILL.md that
// names the skill loosely is its own, and is saved.
func TestFetchedNameThatDiffersOnlyLooselyIsSaved(t *testing.T) {
	e := newCopyEnv(t)
	e.fetch.setName("Find Skills")
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now()); len(d.Active) != 1 {
		t.Fatalf("a loosely matching SKILL.md name was refused: %+v %+v", d, e.health(t))
	}
}

// A skill whose name is not a hosted name (here with an underscore and a
// capital) still gets reet's copy and goes on the machines.
func TestFetchedSkillWithAnyMachineNameGetsACopy(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"vercel-labs/skills/My_Skill"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now()); len(d.Active) != 1 || d.Active[0].Name != "My_Skill" {
		t.Fatalf("desired: %+v health %+v", d, e.health(t))
	}
}

// Adding a skill again straight after deleting it brings it back on the
// machines rather than leaving it deleted.
func TestAddingADeletedSkillAgainPutsItBack(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	if code, body := do(t, e.srv, "POST", "/api/v2/skills/delete", `{"ids":["`+item+`"]}`); code != 200 {
		t.Fatalf("delete: %d %s", code, body)
	}
	code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`)
	if code != 200 || !strings.Contains(body, `"created":true`) {
		t.Fatalf("add again: %d %s", code, body)
	}
	e.settle()
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now().Add(time.Minute)); len(d.Active) != 1 || len(d.Remove) != 0 {
		t.Fatalf("desired after adding again: %+v", d)
	}
}

// Asking for originals downloads each one, so an account gets a couple at a
// time: one more while they run is told to wait, and gets nothing fetched.
func TestOriginalDownloadsAreLimitedPerAccount(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	gate := make(chan struct{})
	e.fetch.setGate(gate)
	before := e.fetch.callCount()
	done := make(chan int, skillOriginalPerAccount)
	for range skillOriginalPerAccount {
		go func() {
			code, _ := do(t, e.srv, "GET", "/api/v2/skills/"+item+"/original", "")
			done <- code
		}()
	}
	for deadline := time.Now().Add(5 * time.Second); e.fetch.callCount() < before+skillOriginalPerAccount; {
		if time.Now().After(deadline) {
			t.Fatalf("the first downloads did not start: %d", e.fetch.callCount()-before)
		}
		time.Sleep(5 * time.Millisecond)
	}
	code, body := do(t, e.srv, "GET", "/api/v2/skills/"+item+"/original", "")
	if code != 429 || e.fetch.callCount() != before+skillOriginalPerAccount {
		t.Fatalf("one more while they run: %d %s, %d fetches", code, body, e.fetch.callCount()-before)
	}
	close(gate)
	for range skillOriginalPerAccount {
		if code := <-done; code != 200 {
			t.Fatalf("a first download: %d", code)
		}
	}
	if code, body := do(t, e.srv, "GET", "/api/v2/skills/"+item+"/original", ""); code != 200 {
		t.Fatalf("after they finished: %d %s", code, body)
	}
}

// An imported skill reet could not fetch has no content, so GET /original
// compares against nothing and answers revision "". Update from original is
// then its first fetch: POST with revision "" is accepted while the skill still
// has no content (ruling R38).
func TestOriginalUpdateIsTheFirstFetchOfASkillWithoutContent(t *testing.T) {
	e := newCopyEnv(t)
	e.fetch.set("", skillfetch.ErrNotFound)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	if li, err := e.st.LibraryItemFor(e.uid, item); err != nil || li.RevisionID != "" {
		t.Fatalf("the failed fetch must leave no content: %+v %v", li, err)
	}
	e.fetch.set("v1", nil)
	url := "/api/v2/skills/" + item + "/original"
	var r struct {
		Changes  []struct{ Path, Kind string }
		Digest   string
		Revision string
	}
	code, body := do(t, e.srv, "GET", url, "")
	if code != 200 || json.Unmarshal([]byte(body), &r) != nil || r.Revision != "" || r.Digest == "" || len(r.Changes) != 1 || r.Changes[0].Kind != "added" {
		t.Fatalf("GET original of a skill with no content: %d %s", code, body)
	}
	// A missing revision is still a 400: the client must say what it looked at.
	if code, body := do(t, e.srv, "POST", url, `{"digest":"`+r.Digest+`"}`); code != 400 {
		t.Fatalf("a missing revision: %d %s", code, body)
	}
	code, body = do(t, e.srv, "POST", url, `{"digest":"`+r.Digest+`","revision":""}`)
	if code != 200 || !strings.Contains(body, `"updated":true`) {
		t.Fatalf("the first fetch through Update from original: %d %s", code, body)
	}
	li, err := e.st.LibraryItemFor(e.uid, item)
	if err != nil || li.RevisionID == "" {
		t.Fatalf("the skill must now have content: %+v %v", li, err)
	}
	// Now that it has content, revision "" says the person looked at nothing.
	if code, body := do(t, e.srv, "POST", url, `{"digest":"`+r.Digest+`","revision":""}`); code != 409 {
		t.Fatalf("revision \"\" once the skill has content: %d %s", code, body)
	}
}

// Content that lands after the person looked at a skill with no content (the
// background fetch, say) makes their revision "" stale: 409, and the content
// that landed stays.
func TestOriginalFirstFetchRefusedWhenContentLandedMeanwhile(t *testing.T) {
	e := newCopyEnv(t)
	e.fetch.set("", skillfetch.ErrNotFound)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	e.fetch.set("v1", nil)
	url := "/api/v2/skills/" + item + "/original"
	var r struct{ Digest, Revision string }
	code, body := do(t, e.srv, "GET", url, "")
	if code != 200 || json.Unmarshal([]byte(body), &r) != nil || r.Revision != "" {
		t.Fatalf("GET original: %d %s", code, body)
	}
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: find-skills\ndescription: Finds skills\n---\nv1\n")}}
	landed, err := e.st.SetSkillContentFor(e.uid, item, files, store.PrivateSkillOrigin{Kind: "original", Reference: findSkillsOrigin})
	if err != nil {
		t.Fatal(err)
	}
	code, body = do(t, e.srv, "POST", url, `{"digest":"`+r.Digest+`","revision":""}`)
	if code != 409 || !strings.Contains(body, "changed") {
		t.Fatalf("content landed meanwhile: %d %s", code, body)
	}
	if li, err := e.st.LibraryItemFor(e.uid, item); err != nil || li.RevisionID != landed.Revision.ID {
		t.Fatalf("the content that landed must stay the head: %+v %v", li, err)
	}
}

// Skills of one repository are fetched from one download of it, not one each.
func TestSkillCopiesDownloadARepositoryOnce(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	slugs := []string{"alpha", "beta", "gamma", "delta"}
	for _, slug := range slugs {
		body := "---\nname: " + slug + "\ndescription: A skill\n---\nv1\n"
		if err := tw.WriteHeader(&tar.Header{Name: "r-abc/skills/" + slug + "/SKILL.md", Mode: 0o644, Typeflag: tar.TypeReg, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	var downloads atomic.Int64
	var up atomic.Bool
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			http.Error(w, "down", 500)
			return
		}
		downloads.Add(1)
		w.Write(buf.Bytes())
	}))
	defer gh.Close()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, deadFetcher(t), "", WithSkillFetcher(skillfetch.Fetcher{Client: gh.Client(), GitHub: gh.URL}))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	uid, _ := st.DefaultOwner()
	// Each skill is added while the repository cannot be read, so all of them
	// wait for one retry.
	for _, slug := range slugs {
		if code, body := do(t, srv, "POST", "/api/skills", `{"input":"o/r/`+slug+`"}`); code >= 300 {
			t.Fatalf("add %d %s", code, body)
		}
		h.s.skillCopies.Wait()
	}
	health, err := st.SkillHealthFor(uid, time.Now())
	if err != nil || len(health) != len(slugs) {
		t.Fatalf("health: %+v %v", health, err)
	}
	var items []string
	for id, sh := range health {
		if sh.State != "fetch-failed" {
			t.Fatalf("before the repository was up: %+v", sh)
		}
		items = append(items, `"`+id+`"`)
	}
	up.Store(true)
	if code, body := do(t, srv, "POST", "/api/v2/skills/retry", `{"items":[`+strings.Join(items, ",")+`]}`); code != 200 {
		t.Fatalf("retry %d %s", code, body)
	}
	h.s.skillCopies.Wait()
	if d, err := st.DesiredSkillsFor(uid, time.Now()); err != nil || len(d.Active) != len(slugs) {
		t.Fatalf("desired after the retry: %+v %v", d, err)
	}
	if n := downloads.Load(); n != 1 {
		t.Fatalf("downloads = %d, want 1 for %d skills of one repository", n, len(slugs))
	}
}
