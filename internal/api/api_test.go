package api

import (
	"encoding/json"
	"github.com/eyupio/reet.sh/internal/auth"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

const findSkills = "https://www.skills.sh/vercel-labs/skills/find-skills"

type listResp struct {
	Skills []store.Skill `json:"skills"`
	Counts store.Counts  `json:"counts"`
}

func newSrv(t *testing.T, f meta.Fetcher, token string) *httptest.Server {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(s, f, token))
	t.Cleanup(func() { srv.Close(); s.Close() })
	return srv
}

func newAccountsSrvStore(t *testing.T, token string) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(s, deadFetcher(t), token, WithAuthMode(AuthAccounts)))
	t.Cleanup(func() { srv.Close(); s.Close() })
	return srv, s
}

func newAccountsSrv(t *testing.T, token string) *httptest.Server {
	t.Helper()
	srv, _ := newAccountsSrvStore(t, token)
	return srv
}

func doClient(t *testing.T, client *http.Client, srv *httptest.Server, method, path, body string, hdr ...string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func deadFetcher(t *testing.T) meta.Fetcher {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) }))
	t.Cleanup(dead.Close)
	return meta.Fetcher{Client: dead.Client(), PageBase: dead.URL + "/"}
}

func do(t *testing.T, srv *httptest.Server, method, path, body string, hdr ...string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func list(t *testing.T, srv *httptest.Server, qs string) listResp {
	t.Helper()
	code, body := do(t, srv, "GET", "/api/skills"+qs, "")
	var l listResp
	if code != 200 || json.Unmarshal([]byte(body), &l) != nil {
		t.Fatalf("list: %d %s", code, body)
	}
	return l
}

func TestAddByURLAndList(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	code, body := do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`","liked":true}`)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	l := list(t, srv, "?tab=liked")
	if len(l.Skills) != 1 || l.Skills[0].ID != "vercel-labs/skills/find-skills" || !l.Skills[0].Liked || l.Counts.Liked != 1 {
		t.Fatalf("%+v", l)
	}
}

func TestAddSaysWhetherCreated(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	if _, body := do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); !strings.Contains(body, `"created":true`) {
		t.Fatalf("first add: %s", body)
	}
	if _, body := do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); !strings.Contains(body, `"created":false`) {
		t.Fatalf("second add: %s", body)
	}
}

func TestAddRejectsForeignURL(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	code, body := do(t, srv, "POST", "/api/skills", `{"input":"https://github.com/a/b/c"}`)
	if code != 400 || !strings.Contains(body, "error") {
		t.Fatalf("%d %s", code, body)
	}
	if l := list(t, srv, ""); len(l.Skills) != 0 {
		t.Fatalf("saved something: %+v", l)
	}
}

func TestAddSurvivesMetaFailure(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`)
	time.Sleep(100 * time.Millisecond)
	if l := list(t, srv, ""); len(l.Skills) != 1 || l.Skills[0].Name != "find-skills" {
		t.Fatalf("%+v", l)
	}
}

func TestMetaAppliedInBackground(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<meta name="description" content="Find skills fast">
			<script type="application/ld+json">{"@type":"SoftwareApplication","name":"Find Skills","interactionStatistic":{"userInteractionCount":1234}}</script>`))
	}))
	defer page.Close()
	srv := newSrv(t, meta.Fetcher{Client: page.Client(), PageBase: page.URL + "/"}, "")
	do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`)
	for i := 0; i < 40; i++ {
		l := list(t, srv, "")
		if l.Skills[0].Name == "Find Skills" && l.Skills[0].Description == "Find skills fast" && l.Skills[0].Installs == 1234 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("metadata never arrived")
}

func TestAddInstallCommandFetchesDetailsAndDeduplicates(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropics/skills/mcp-builder" {
			t.Errorf("unexpected metadata path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`<meta name="description" content="Build MCP servers">
			<script type="application/ld+json">{"@type":"SoftwareApplication","name":"MCP Builder","interactionStatistic":{"userInteractionCount":123}}</script>`))
	}))
	defer page.Close()
	srv := newSrv(t, meta.Fetcher{Client: page.Client(), PageBase: page.URL + "/"}, "")
	code, body := do(t, srv, "POST", "/api/skills", `{"input":"npx skills add https://github.com/anthropics/skills --skill mcp-builder","liked":true}`)
	if code != 200 || !strings.Contains(body, `"created":true`) {
		t.Fatalf("add command: %d %s", code, body)
	}
	code, body = do(t, srv, "POST", "/api/skills", `{"input":"https://skills.sh/anthropics/skills/mcp-builder"}`)
	if code != 200 || !strings.Contains(body, `"created":false`) {
		t.Fatalf("add URL: %d %s", code, body)
	}
	for i := 0; i < 40; i++ {
		l := list(t, srv, "")
		if len(l.Skills) == 1 {
			s := l.Skills[0]
			if s.ID == "anthropics/skills/mcp-builder" && s.Source == "anthropics/skills" && s.Slug == "mcp-builder" && s.Liked && s.Name == "MCP Builder" && s.Description == "Build MCP servers" && s.Installs == 123 {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("command did not resolve to one skill with metadata")
}

func TestPatchFlagsAndNotes(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`","liked":true}`)
	code, _ := do(t, srv, "PATCH", "/api/skill", `{"id":"vercel-labs/skills/find-skills","liked":false,"bookmarked":true,"notes":"hi"}`)
	if code != 200 {
		t.Fatal(code)
	}
	s := list(t, srv, "").Skills[0]
	if s.Liked || !s.Bookmarked || s.Notes != "hi" {
		t.Fatalf("%+v", s)
	}
	if code, _ := do(t, srv, "PATCH", "/api/skill", `{"id":"nope/x/y","liked":true}`); code != 404 {
		t.Fatalf("unknown id: %d", code)
	}
}

func TestImportLocalAndSourced(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	body := `{"items":[{"name":"plain"},{"name":"find-skills","source":"vercel-labs/skills"}],"text":"# comment\n- https://www.skills.sh/anthropics/skills/pptx\nrawname\n"}`
	if code, b := do(t, srv, "POST", "/api/import", body); code != 200 {
		t.Fatalf("%d %s", code, b)
	}
	l := list(t, srv, "?tab=used")
	ids := map[string]bool{}
	for _, s := range l.Skills {
		ids[s.ID] = true
	}
	for _, want := range []string{"local/plain", "vercel-labs/skills/find-skills", "anthropics/skills/pptx", "local/rawname"} {
		if !ids[want] {
			t.Errorf("missing %s in %v", want, ids)
		}
	}
}

func TestImportKeepsLikes(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`","liked":true,"bookmarked":true}`)
	do(t, srv, "PATCH", "/api/skill", `{"id":"vercel-labs/skills/find-skills","notes":"keep me"}`)
	do(t, srv, "POST", "/api/import", `{"items":[{"name":"find-skills","source":"vercel-labs/skills"}]}`)
	s := list(t, srv, "").Skills[0]
	if !s.Liked || !s.Bookmarked || s.Used || s.Notes != "keep me" {
		t.Fatalf("%+v", s)
	}
}

func TestResolveAndDelete(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	do(t, srv, "POST", "/api/import", `{"items":[{"name":"find-skills"}]}`)
	code, b := do(t, srv, "POST", "/api/skill/resolve", `{"id":"local/find-skills","input":"`+findSkills+`"}`)
	if code != 200 {
		t.Fatalf("%d %s", code, b)
	}
	l := list(t, srv, "")
	if len(l.Skills) != 1 || l.Skills[0].ID != "vercel-labs/skills/find-skills" || !l.Skills[0].Used {
		t.Fatalf("%+v", l)
	}
	if code, _ := do(t, srv, "DELETE", "/api/skill?id=vercel-labs/skills/find-skills", ""); code != 204 {
		t.Fatal(code)
	}
	if l := list(t, srv, ""); len(l.Skills) != 0 {
		t.Fatalf("%+v", l)
	}
}

func TestQuickLike(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	code, body := do(t, srv, "GET", "/quick?action=like&url="+findSkills, "")
	if code != 200 || !strings.Contains(body, "Liked") {
		t.Fatalf("%d %s", code, body)
	}
	if l := list(t, srv, "?tab=liked"); len(l.Skills) != 1 {
		t.Fatalf("%+v", l)
	}
	if _, body := do(t, srv, "GET", "/quick?action=like&url="+findSkills, ""); !strings.Contains(body, "Already liked") {
		t.Fatalf("second like: %s", body)
	}
	if code, _ := do(t, srv, "GET", "/quick?action=like&url=https://example.com/a/b/c", ""); code != 400 {
		t.Fatalf("foreign url: %d", code)
	}
}

func TestHealthAndVersion(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`)
	code, body := do(t, srv, "GET", "/healthz", "")
	var h struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		Skills  int    `json:"skills"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &h) != nil || !h.OK || h.Version == "" || h.Skills != 1 {
		t.Fatalf("healthz: %d %s", code, body)
	}
	code, body = do(t, srv, "GET", "/api/version", "")
	var v struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &v) != nil || v.Version == "" || v.Commit == "" {
		t.Fatalf("version: %d %s", code, body)
	}
}

func TestAuthRequired(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "s3cret")
	if code, _ := do(t, srv, "GET", "/api/skills", ""); code != 401 {
		t.Fatalf("api no token: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/quick?action=like&url="+findSkills, ""); code != 401 {
		t.Fatalf("quick no token: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/skills", "", "Authorization", "Bearer wrong"); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/skills", "", "Authorization", "Bearer s3cret"); code != 200 {
		t.Fatalf("bearer: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/skills", "", "X-Reet-Token", "s3cret"); code != 200 {
		t.Fatalf("header: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/quick?t=s3cret&action=save&url="+findSkills, ""); code != 200 {
		t.Fatalf("quick query token: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/healthz", ""); code != 200 {
		t.Fatalf("healthz: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/version", ""); code != 200 {
		t.Fatalf("version should be open: %d", code)
	}
}

func TestExportRestoreRoundTrip(t *testing.T) {
	a := newSrv(t, deadFetcher(t), "")
	do(t, a, "POST", "/api/skills", `{"input":"`+findSkills+`","liked":true}`)
	code, exported := do(t, a, "GET", "/api/export", "")
	if code != 200 {
		t.Fatal(code)
	}
	b := newSrv(t, deadFetcher(t), "")
	if code, body := do(t, b, "POST", "/api/restore", exported); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if l := list(t, b, "?tab=liked"); len(l.Skills) != 1 {
		t.Fatalf("%+v", l)
	}
}

func TestCrossSiteWriteRejected(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	code, _ := do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`, "Sec-Fetch-Site", "cross-site")
	if code != 403 {
		t.Fatalf("cross-site POST: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", "/api/skill?id=a/b/c", "", "Sec-Fetch-Site", "cross-site"); code != 403 {
		t.Fatalf("cross-site DELETE: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`, "Sec-Fetch-Site", "same-origin"); code != 200 {
		t.Fatalf("same-origin POST: %d", code)
	}
	if l := list(t, srv, ""); len(l.Skills) != 1 {
		t.Fatalf("only the same-origin write should have landed: %+v", l)
	}
}

func TestWriteRequiresJSONContentType(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	code, _ := do(t, srv, "POST", "/api/restore", `{"skills":[{"id":"a/b/c"}]}`, "Content-Type", "text/plain")
	if code != 415 {
		t.Fatalf("text/plain POST: %d", code)
	}
	if l := list(t, srv, ""); len(l.Skills) != 0 {
		t.Fatalf("nothing should be saved: %+v", l)
	}
}

func TestImportEnvironmentAndPrune(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	body := `{"items":[{"name":"alpha","host":"laptop","agent":"claude","path":".claude/skills"}],"environments":[{"host":"laptop","agent":"claude","path":".claude/skills","skills":["alpha"]}],"prune":true}`
	if code, b := do(t, srv, "POST", "/api/import", body); code != 200 {
		t.Fatalf("%d %s", code, b)
	}
	code, b := do(t, srv, "GET", "/api/environments", "")
	var envs struct {
		Environments []store.Environment `json:"environments"`
	}
	if code != 200 || json.Unmarshal([]byte(b), &envs) != nil || len(envs.Environments) != 1 {
		t.Fatalf("environments: %d %s", code, b)
	}
	envPath := "/api/skills?env=" + url.QueryEscape(envs.Environments[0].ID)
	code, b = do(t, srv, "GET", envPath, "")
	if code != 200 || !strings.Contains(b, `"name":"alpha"`) {
		t.Fatalf("env filter: %d %s", code, b)
	}
	body = `{"items":[],"environments":[{"host":"laptop","agent":"claude","path":".claude/skills","skills":[]}],"prune":true}`
	code, b = do(t, srv, "POST", "/api/import", body)
	if code != 200 || !strings.Contains(b, `"pruned":1`) {
		t.Fatalf("prune: %d %s", code, b)
	}
	if code, b := do(t, srv, "GET", envPath, ""); code != 200 || !strings.Contains(b, `"skills":[]`) {
		t.Fatalf("after prune: %d %s", code, b)
	}
}

func TestTagsEventsAndIgnore(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	do(t, srv, "POST", "/api/skills", `{"input":"anthropics/skills/frontend-design"}`)
	code, b := do(t, srv, "POST", "/api/skill/tags", `{"id":"anthropics/skills/frontend-design","add":["client work"],"remove":[]}`)
	if code != 200 || !strings.Contains(b, "client work") {
		t.Fatalf("tags: %d %s", code, b)
	}
	if code, b := do(t, srv, "GET", `/api/skills?q=tag%3A%22client%20work%22`, ""); code != 200 || !strings.Contains(b, "frontend-design") {
		t.Fatalf("tag search: %d %s", code, b)
	}
	if code, b := do(t, srv, "GET", "/api/events?skill=anthropics%2Fskills%2Ffrontend-design", ""); code != 200 || !strings.Contains(b, `"action":"tag"`) {
		t.Fatalf("events: %d %s", code, b)
	}
	if code, _ := do(t, srv, "POST", "/api/skill/install-copy", `{"id":"anthropics/skills/frontend-design"}`); code != 204 {
		t.Fatalf("install copy event: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", "/api/skill?id=anthropics%2Fskills%2Ffrontend-design&ignore=1", ""); code != 204 {
		t.Fatalf("ignore: %d", code)
	}
	if code, b := do(t, srv, "GET", "/api/ignored", ""); code != 200 || !strings.Contains(b, "frontend-design") {
		t.Fatalf("ignored: %d %s", code, b)
	}
	code, b = do(t, srv, "POST", "/api/import", `{"items":[{"name":"frontend-design","source":"anthropics/skills"}]}`)
	if code != 200 || !strings.Contains(b, `"ignored":1`) {
		t.Fatalf("import ignored: %d %s", code, b)
	}
}

func TestRenameAndMergeTags(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	do(t, srv, "POST", "/api/skills", `{"input":"a/b/one"}`)
	do(t, srv, "POST", "/api/skills", `{"input":"a/b/two"}`)
	do(t, srv, "POST", "/api/skill/tags", `{"id":"a/b/one","add":["first"],"remove":[]}`)
	do(t, srv, "POST", "/api/skill/tags", `{"id":"a/b/two","add":["second"],"remove":[]}`)
	code, b := do(t, srv, "GET", "/api/tags", "")
	var tags struct {
		Tags []store.Tag `json:"tags"`
	}
	if code != 200 || json.Unmarshal([]byte(b), &tags) != nil {
		t.Fatalf("tags: %d %s", code, b)
	}
	ids := map[string]int64{}
	for _, tag := range tags.Tags {
		ids[tag.Name] = tag.ID
	}
	if code, b := do(t, srv, "PATCH", "/api/tag", `{"id":`+strconv.FormatInt(ids["first"], 10)+`,"name":"renamed"}`); code != 200 {
		t.Fatalf("rename: %d %s", code, b)
	}
	if code, b := do(t, srv, "POST", "/api/tags/merge", `{"from":`+strconv.FormatInt(ids["first"], 10)+`,"into":`+strconv.FormatInt(ids["second"], 10)+`}`); code != 200 {
		t.Fatalf("merge: %d %s", code, b)
	}
	if code, b := do(t, srv, "GET", `/api/skills?q=tag%3Asecond`, ""); code != 200 || !strings.Contains(b, `"name":"one"`) || !strings.Contains(b, `"name":"two"`) {
		t.Fatalf("merged filter: %d %s", code, b)
	}
}

func TestManualRefreshReportsChange(t *testing.T) {
	var mu sync.Mutex
	body := ""
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Write([]byte(body))
	}))
	defer page.Close()
	srv := newSrv(t, meta.Fetcher{Client: page.Client(), PageBase: page.URL + "/"}, "")
	do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`)
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	body = `<meta name="description" content="Changed">`
	mu.Unlock()
	var res struct {
		Changed   bool `json:"changed"`
		Refreshed bool `json:"refreshed"`
	}
	code, b := do(t, srv, "POST", "/api/skill/refresh", `{"id":"vercel-labs/skills/find-skills"}`)
	if code != 200 || json.Unmarshal([]byte(b), &res) != nil || !res.Changed || !res.Refreshed {
		t.Fatalf("first refresh: %d %s", code, b)
	}
	code, b = do(t, srv, "POST", "/api/skill/refresh", `{"id":"vercel-labs/skills/find-skills"}`)
	if code != 200 || json.Unmarshal([]byte(b), &res) != nil || res.Changed || !res.Refreshed {
		t.Fatalf("second refresh: %d %s", code, b)
	}
}

func TestBulkPatchTagAndRemove(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	ids := []string{"a/b/one", "a/b/two", "a/b/three"}
	for _, id := range ids {
		do(t, srv, "POST", "/api/skills", `{"input":"`+id+`"}`)
	}
	mkBody := func(extra string) string {
		return `{"ids":["a/b/one","a/b/two","a/b/three"],` + extra + `}`
	}
	if code, b := do(t, srv, "PATCH", "/api/skills", mkBody(`"used":true`)); code != 200 || !strings.Contains(b, `"updated":3`) {
		t.Fatalf("bulk stage: %d %s", code, b)
	}
	if l := list(t, srv, "?tab=used"); len(l.Skills) != 3 {
		t.Fatalf("used: %+v", l)
	}
	if code, b := do(t, srv, "POST", "/api/skills/tags", mkBody(`"add":["bulk"],"remove":[]`)); code != 200 || !strings.Contains(b, `"tagged":3`) {
		t.Fatalf("bulk tag: %d %s", code, b)
	}
	if code, b := do(t, srv, "GET", "/api/skills?q=tag%3Abulk", ""); code != 200 || !strings.Contains(b, "a/b/one") || !strings.Contains(b, "a/b/three") {
		t.Fatalf("bulk tag filter: %d %s", code, b)
	}
	if code, b := do(t, srv, "POST", "/api/skills/remove", mkBody(`"ignore":false`)); code != 200 || !strings.Contains(b, `"removed":3`) {
		t.Fatalf("bulk remove: %d %s", code, b)
	}
	if l := list(t, srv, ""); len(l.Skills) != 0 {
		t.Fatalf("after remove: %+v", l)
	}
	if code, b := do(t, srv, "GET", "/api/events", ""); code != 200 || !strings.Contains(b, "bulk-stage") || !strings.Contains(b, "bulk-tag") || !strings.Contains(b, "bulk-remove") {
		t.Fatalf("bulk events: %d %s", code, b)
	}
}

func TestAdminStatusAndSettings(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	do(t, srv, "POST", "/api/skills", `{"input":"a/b/one"}`)
	code, b := do(t, srv, "GET", "/api/admin/status", "")
	var status struct {
		Version string       `json:"version"`
		Counts  store.Counts `json:"counts"`
		DB      struct {
			Path string
			Size int64
		} `json:"db"`
		Settings struct {
			RefreshEnabled bool   `json:"refreshEnabled"`
			TopicsEnabled  bool   `json:"topicsEnabled"`
			InstanceName   string `json:"instanceName"`
		} `json:"settings"`
	}
	if code != 200 || json.Unmarshal([]byte(b), &status) != nil || status.Counts.All != 1 || status.DB.Path == "" || !status.Settings.RefreshEnabled {
		t.Fatalf("status: %d %s", code, b)
	}
	if code, b := do(t, srv, "PATCH", "/api/admin/settings", `{"refreshEnabled":false,"topicsEnabled":false,"instanceName":"Team shelf"}`); code != 200 {
		t.Fatalf("settings: %d %s", code, b)
	}
	code, b = do(t, srv, "GET", "/api/admin/status", "")
	if code != 200 || json.Unmarshal([]byte(b), &status) != nil || status.Settings.RefreshEnabled || status.Settings.TopicsEnabled || status.Settings.InstanceName != "Team shelf" {
		t.Fatalf("updated status: %d %s", code, b)
	}
	if code, b := do(t, srv, "GET", "/api/events?limit=20", ""); code != 200 || !strings.Contains(b, `"action":"settings"`) {
		t.Fatalf("settings event: %d %s", code, b)
	}
}

func TestFilteredMarkdownAndCSV(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	do(t, srv, "POST", "/api/skills", `{"input":"a/b/one"}`)
	do(t, srv, "POST", "/api/skills", `{"input":"a/b/two"}`)
	do(t, srv, "POST", "/api/skill/tags", `{"id":"a/b/one","add":["bulk"],"remove":[]}`)
	do(t, srv, "POST", "/api/import", `{"items":[{"name":"one","source":"a/b","host":"laptop","agent":"claude"}]}`)
	code, md := do(t, srv, "GET", "/api/export?format=md&q=one", "")
	if code != 200 || !strings.Contains(md, "## On the shelf") || !strings.Contains(md, "npx skills add https://github.com/a/b --skill one") {
		t.Fatalf("markdown: %d %s", code, md)
	}
	code, csvOut := do(t, srv, "GET", "/api/export?format=csv&q=tag%3Abulk", "")
	if code != 200 || !strings.Contains(csvOut, "id,name,stage") || !strings.Contains(csvOut, "a/b/one") || !strings.Contains(csvOut, "bulk") {
		t.Fatalf("csv: %d %s", code, csvOut)
	}
}

func TestAccountsSetupSignInAndSessions(t *testing.T) {
	srv := newAccountsSrv(t, "bootstrap")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	code, body := doClient(t, client, srv, "GET", "/api/v2/auth/status", "")
	if code != 200 || !strings.Contains(body, `"setupRequired":true`) || !strings.Contains(body, `"bootstrapRequired":true`) {
		t.Fatalf("status: %d %s", code, body)
	}
	if code, _ := doClient(t, client, srv, "GET", "/api/skills", ""); code != 401 {
		t.Fatalf("pre-setup shelf: %d", code)
	}
	if code, _ := doClient(t, client, srv, "POST", "/api/v2/auth/setup", `{"name":"Sam","email":"sam@example.com","password":"long-enough-password"}`); code != 401 {
		t.Fatalf("setup without bootstrap: %d", code)
	}
	code, body = doClient(t, client, srv, "POST", "/api/v2/auth/setup", `{"name":"Sam","email":"sam@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap")
	if code != 200 || !strings.Contains(body, `"role":"owner"`) || strings.Contains(body, "password") {
		t.Fatalf("setup: %d %s", code, body)
	}
	if code, body := doClient(t, client, srv, "GET", "/api/skills", ""); code != 200 || !strings.Contains(body, `"skills"`) {
		t.Fatalf("session shelf: %d %s", code, body)
	}
	if code, body := doClient(t, client, srv, "GET", "/api/v2/auth/status", ""); code != 200 || !strings.Contains(body, `"email":"sam@example.com"`) {
		t.Fatalf("status with session: %d %s", code, body)
	}
	if code, _ := doClient(t, client, srv, "POST", "/api/v2/auth/setup", `{"name":"Other","email":"other@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 409 {
		t.Fatalf("second setup: %d", code)
	}
	if code, _ := doClient(t, client, srv, "POST", "/api/v2/auth/sign-out", `{}`); code != 204 {
		t.Fatalf("sign out: %d", code)
	}
	if code, _ := doClient(t, client, srv, "GET", "/api/skills", ""); code != 401 {
		t.Fatalf("signed-out shelf: %d", code)
	}
	if code, _ := doClient(t, client, srv, "POST", "/api/v2/auth/sign-in", `{"email":"sam@example.com","password":"wrong"}`); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	if code, _ := doClient(t, client, srv, "POST", "/api/v2/auth/sign-in", `{"email":"sam@example.com","password":"long-enough-password"}`); code != 200 {
		t.Fatalf("sign in: %d", code)
	}
	if code, body := doClient(t, client, srv, "GET", "/api/skills", ""); code != 200 || !strings.Contains(body, `"skills"`) {
		t.Fatalf("signed-in shelf: %d %s", code, body)
	}
	if code, body := doClient(t, client, srv, "GET", "/api/events?limit=20", ""); code != 200 || !strings.Contains(body, "sign-in") || !strings.Contains(body, "setup") {
		t.Fatalf("auth events: %d %s", code, body)
	}
	if code, _ := doClient(t, client, srv, "GET", "/api/admin/status", ""); code != 200 {
		t.Fatalf("admin with session: %d", code)
	}
}

func TestAccountsSetupWithoutBootstrap(t *testing.T) {
	srv := newAccountsSrv(t, "")
	code, body := do(t, srv, "POST", "/api/v2/auth/setup", `{"email":"sam@example.com","password":"long-enough-password"}`)
	if code != 200 || !strings.Contains(body, `"role":"owner"`) {
		t.Fatalf("open setup: %d %s", code, body)
	}
}

func TestAuthNoneAllowsRequests(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	srv := httptest.NewServer(New(s, deadFetcher(t), "", WithAuthMode(AuthNone)))
	defer srv.Close()
	if code, _ := do(t, srv, "GET", "/api/skills", ""); code != 200 {
		t.Fatalf("none mode shelf: %d", code)
	}
	if code, body := do(t, srv, "GET", "/api/v2/auth/status", ""); code != 200 || !strings.Contains(body, `"mode":"none"`) {
		t.Fatalf("none status: %d %s", code, body)
	}
}

func TestBootstrapTokenOnlyImports(t *testing.T) {
	srv := newAccountsSrv(t, "bootstrap")
	if code, _ := do(t, srv, "POST", "/api/v2/auth/setup", `{"email":"sam@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/skills", "", "X-Reet-Token", "bootstrap"); code != 401 {
		t.Fatalf("bootstrap should not read the shelf: %d", code)
	}
	if code, body := do(t, srv, "POST", "/api/import", `{"items":[{"name":"alpha"}]}`, "X-Reet-Token", "bootstrap"); code != 200 || !strings.Contains(body, `"imported":1`) {
		t.Fatalf("bootstrap import: %d %s", code, body)
	}
}

func TestPerUserIsolationAndRoles(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "bootstrap")
	ownerClient := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, ownerClient, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	hash, _ := auth.HashPassword("long-enough-password")
	if _, err := st.CreateUser("member@example.com", "Member", "member", hash); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("viewer@example.com", "Viewer", "viewer", hash); err != nil {
		t.Fatal(err)
	}
	memberClient := &http.Client{Jar: mustJar(t)}
	viewerClient := &http.Client{Jar: mustJar(t)}
	if code, body := doClient(t, memberClient, srv, "POST", "/api/v2/auth/sign-in", `{"email":"member@example.com","password":"long-enough-password"}`); code != 200 {
		t.Fatalf("member sign-in: %d %s", code, body)
	}
	if code, body := doClient(t, viewerClient, srv, "POST", "/api/v2/auth/sign-in", `{"email":"viewer@example.com","password":"long-enough-password"}`); code != 200 {
		t.Fatalf("viewer sign-in: %d %s", code, body)
	}
	if code, body := doClient(t, memberClient, srv, "POST", "/api/skills", `{"input":"a/b/member-skill"}`); code != 200 {
		t.Fatalf("member add: %d %s", code, body)
	}
	if _, body := doClient(t, memberClient, srv, "GET", "/api/skills", ""); !strings.Contains(body, "member-skill") {
		t.Fatalf("member shelf: %s", body)
	}
	if _, body := doClient(t, viewerClient, srv, "GET", "/api/skills", ""); strings.Contains(body, "member-skill") {
		t.Fatalf("viewer saw member shelf: %s", body)
	}
	if _, body := doClient(t, ownerClient, srv, "GET", "/api/skills", ""); strings.Contains(body, "member-skill") {
		t.Fatalf("owner saw member shelf: %s", body)
	}
	if code, _ := doClient(t, viewerClient, srv, "POST", "/api/skills", `{"input":"a/b/viewer-skill"}`); code != 403 {
		t.Fatalf("viewer write: %d", code)
	}
	if code, _ := doClient(t, viewerClient, srv, "GET", "/api/export", ""); code != 403 {
		t.Fatalf("viewer export: %d", code)
	}
	if code, _ := doClient(t, viewerClient, srv, "GET", "/api/admin/status", ""); code != 403 {
		t.Fatalf("viewer admin: %d", code)
	}
	if code, _ := doClient(t, memberClient, srv, "GET", "/api/admin/status", ""); code != 403 {
		t.Fatalf("member admin: %d", code)
	}
	if code, _ := doClient(t, ownerClient, srv, "GET", "/api/admin/status", ""); code != 200 {
		t.Fatalf("owner admin: %d", code)
	}
	if code, _ := doClient(t, memberClient, srv, "PATCH", "/api/tag", `{"id":1,"name":"nope"}`); code != 403 {
		t.Fatalf("member tag admin: %d", code)
	}
}

func mustJar(t *testing.T) *cookiejar.Jar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return jar
}

func TestPersonalTokenAndBootstrapDisable(t *testing.T) {
	srv, _ := newAccountsSrvStore(t, "bootstrap")
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	code, body := doClient(t, owner, srv, "POST", "/api/v2/tokens", `{"name":"scan"}`)
	var created struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &created) != nil || created.Token == "" {
		t.Fatalf("create token: %d %s", code, body)
	}
	if code, body := doClient(t, owner, srv, "GET", "/api/v2/tokens", ""); code != 200 || strings.Contains(body, created.Token) {
		t.Fatalf("token list leaked the secret: %d %s", code, body)
	}
	if code, body := doClient(t, owner, srv, "POST", "/api/import", `{"items":[{"name":"alpha"}]}`, "X-Reet-Token", created.Token); code != 200 || !strings.Contains(body, `"user":"Owner"`) {
		t.Fatalf("personal token import: %d %s", code, body)
	}
	if code, _ := doClient(t, owner, srv, "POST", "/api/import", `{"items":[{"name":"beta"}]}`, "X-Reet-Token", "bootstrap"); code != 401 {
		t.Fatalf("bootstrap should be disabled after a personal token: %d", code)
	}
	if code, _ := doClient(t, owner, srv, "DELETE", "/api/v2/tokens?id="+strconv.FormatInt(created.ID, 10), ""); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ := doClient(t, owner, srv, "POST", "/api/import", `{"items":[{"name":"beta"}]}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("bootstrap should work again after revoke: %d", code)
	}
}

func TestInviteJoinRoleChangesAndTransfer(t *testing.T) {
	srv, _ := newAccountsSrvStore(t, "bootstrap")
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	code, body := doClient(t, owner, srv, "POST", "/api/v2/admin/invites", `{"email":"viewer@example.com","role":"viewer"}`)
	var invite struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &invite) != nil || invite.Token == "" {
		t.Fatalf("invite: %d %s", code, body)
	}
	if code, body := do(t, srv, "GET", "/api/v2/auth/invite?token="+url.QueryEscape(invite.Token), ""); code != 200 || !strings.Contains(body, "viewer@example.com") {
		t.Fatalf("invite info: %d %s", code, body)
	}
	viewer := &http.Client{Jar: mustJar(t)}
	if code, body := doClient(t, viewer, srv, "POST", "/api/v2/auth/join", `{"token":"`+invite.Token+`","name":"Viewer","password":"long-enough-password"}`); code != 200 || !strings.Contains(body, `"role":"viewer"`) {
		t.Fatalf("join: %d %s", code, body)
	}
	code, body = doClient(t, owner, srv, "GET", "/api/v2/admin/users", "")
	var list struct {
		Users []store.UserMeta `json:"users"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &list) != nil || len(list.Users) != 2 {
		t.Fatalf("users: %d %s", code, body)
	}
	var viewerID int64
	for _, u := range list.Users {
		if u.Email == "viewer@example.com" {
			viewerID = u.ID
		}
	}
	if viewerID == 0 {
		t.Fatal("viewer missing")
	}
	if code, _ := doClient(t, viewer, srv, "POST", "/api/skills", `{"input":"a/b/viewer"}`); code != 403 {
		t.Fatalf("viewer write: %d", code)
	}
	if code, body := doClient(t, owner, srv, "PATCH", "/api/v2/admin/users", `{"id":`+strconv.FormatInt(viewerID, 10)+`,"role":"member"}`); code != 200 || !strings.Contains(body, `"role":"member"`) {
		t.Fatalf("role change: %d %s", code, body)
	}
	if code, _ := doClient(t, viewer, srv, "POST", "/api/skills", `{"input":"a/b/viewer"}`); code != 200 {
		t.Fatalf("member write: %d", code)
	}
	if code, body := doClient(t, owner, srv, "PATCH", "/api/v2/admin/users", `{"id":`+strconv.FormatInt(viewerID, 10)+`,"disabled":true}`); code != 200 {
		t.Fatalf("disable: %d %s", code, body)
	}
	if code, _ := doClient(t, viewer, srv, "GET", "/api/skills", ""); code != 401 {
		t.Fatalf("disabled session: %d", code)
	}
	code, body = doClient(t, owner, srv, "POST", "/api/v2/admin/users/reset", `{"id":`+strconv.FormatInt(viewerID, 10)+`}`)
	var reset struct {
		Password string `json:"password"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &reset) != nil || reset.Password == "" {
		t.Fatalf("reset: %d %s", code, body)
	}
	newViewer := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, newViewer, srv, "POST", "/api/v2/auth/sign-in", `{"email":"viewer@example.com","password":"long-enough-password"}`); code != 401 {
		t.Fatalf("old password after reset: %d", code)
	}
	if code, body := doClient(t, owner, srv, "PATCH", "/api/v2/admin/users", `{"id":`+strconv.FormatInt(viewerID, 10)+`,"disabled":false}`); code != 200 {
		t.Fatalf("re-enable: %d %s", code, body)
	}
	if code, _ := doClient(t, newViewer, srv, "POST", "/api/v2/auth/sign-in", `{"email":"viewer@example.com","password":"`+reset.Password+`"}`); code != 200 {
		t.Fatalf("new password after reset: %d", code)
	}
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/admin/users/transfer", `{"id":`+strconv.FormatInt(viewerID, 10)+`}`); code != 200 || !strings.Contains(body, `"role":"owner"`) {
		t.Fatalf("transfer: %d %s", code, body)
	}
	if code, _ := doClient(t, owner, srv, "PATCH", "/api/admin/settings", `{"instanceName":"nope"}`); code != 403 {
		t.Fatalf("old owner settings after transfer: %d", code)
	}
	if code, _ := doClient(t, owner, srv, "GET", "/api/admin/status", ""); code != 200 {
		t.Fatalf("old owner admin after transfer: %d", code)
	}
}

func TestSharedListsRolesAndPublicLink(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "bootstrap")
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	hash, _ := auth.HashPassword("long-enough-password")
	for _, u := range []struct{ email, name, role string }{
		{"editor@example.com", "Editor", "member"},
		{"viewer@example.com", "Viewer", "viewer"},
	} {
		if _, err := st.CreateUser(u.email, u.name, u.role, hash); err != nil {
			t.Fatal(err)
		}
	}
	editor := &http.Client{Jar: mustJar(t)}
	viewer := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, editor, srv, "POST", "/api/v2/auth/sign-in", `{"email":"editor@example.com","password":"long-enough-password"}`); code != 200 {
		t.Fatalf("editor sign-in: %d", code)
	}
	if code, _ := doClient(t, viewer, srv, "POST", "/api/v2/auth/sign-in", `{"email":"viewer@example.com","password":"long-enough-password"}`); code != 200 {
		t.Fatalf("viewer sign-in: %d", code)
	}

	code, body := doClient(t, owner, srv, "POST", "/api/v2/lists", `{"name":"Launch pack","description":"Before launch","visibility":"private"}`)
	var list store.ListDetail
	if code != 200 || json.Unmarshal([]byte(body), &list) != nil || list.ID == 0 {
		t.Fatalf("create list: %d %s", code, body)
	}
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/skills", `{"input":"a/b/list-skill","note":"start here"}`); code != 200 || !strings.Contains(body, `"note":"start here"`) {
		t.Fatalf("add list skill: %d %s", code, body)
	}
	if code, _ := doClient(t, editor, srv, "GET", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10), ""); code != 404 {
		t.Fatalf("non-member read private list: %d", code)
	}
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/members", `{"email":"editor@example.com","role":"editor"}`); code != 200 || !strings.Contains(body, `"canEdit":true`) {
		t.Fatalf("add editor: %d %s", code, body)
	}
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/members", `{"email":"viewer@example.com","role":"viewer"}`); code != 200 {
		t.Fatalf("add viewer: %d %s", code, body)
	}
	viewerUser, _, _ := st.UserByEmail("viewer@example.com")
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/members", `{"email":"viewer@example.com","role":"editor"}`); code != 400 {
		t.Fatalf("viewer as editor: %d", code)
	}
	if code, _ := doClient(t, owner, srv, "PATCH", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/members", `{"userId":`+strconv.FormatInt(viewerUser.ID, 10)+`,"role":"editor"}`); code != 400 {
		t.Fatalf("promote viewer: %d", code)
	}
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/transfer", `{"userId":`+strconv.FormatInt(viewerUser.ID, 10)+`}`); code != 400 {
		t.Fatalf("viewer as list owner: %d", code)
	}
	// Defence in depth: even a viewer row written directly as editor stays read-only.
	if err := st.AddListMember(list.ID, viewerUser.ID, "editor"); err != nil {
		t.Fatal(err)
	}
	if code, body := doClient(t, viewer, srv, "GET", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10), ""); code != 200 || !strings.Contains(body, `"canEdit":false`) {
		t.Fatalf("viewer edit grant: %d %s", code, body)
	}
	if code, body := doClient(t, editor, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/skills", `{"input":"a/b/editor-skill"}`); code != 200 || !strings.Contains(body, "editor-skill") {
		t.Fatalf("editor add: %d %s", code, body)
	}
	if code, _ := doClient(t, viewer, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/skills", `{"input":"a/b/nope"}`); code != 403 {
		t.Fatalf("viewer add: %d", code)
	}
	if code, body := doClient(t, viewer, srv, "GET", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10), ""); code != 200 || !strings.Contains(body, "list-skill") {
		t.Fatalf("viewer read: %d %s", code, body)
	}
	if code, body := doClient(t, viewer, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/add-to-shelf", `{}`); code != 403 {
		t.Fatalf("viewer add all: %d %s", code, body)
	}
	if code, body := doClient(t, editor, srv, "POST", "/api/skills", `{"input":"a/b/list-skill","used":true}`); code != 200 {
		t.Fatalf("editor seeded shelf: %d %s", code, body)
	}
	if code, body := doClient(t, editor, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/add-to-shelf", `{}`); code != 200 || !strings.Contains(body, `"added":1`) {
		t.Fatalf("editor add all: %d %s", code, body)
	}
	if code, body := doClient(t, editor, srv, "GET", "/api/skills", ""); code != 200 || !strings.Contains(body, "list-skill") {
		t.Fatalf("editor shelf: %d %s", code, body)
	}
	if code, body := doClient(t, editor, srv, "GET", "/api/skills", ""); code != 200 || !strings.Contains(body, `"id":"a/b/list-skill","source"`) || !strings.Contains(body, `"used":true`) {
		t.Fatalf("existing shelf row changed: %d %s", code, body)
	}
	if code, body := doClient(t, editor, srv, "GET", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10), ""); code != 200 || !strings.Contains(body, `"useCount":1`) {
		t.Fatalf("adoption badge: %d %s", code, body)
	}
	editorUser, _, _ := st.UserByEmail("editor@example.com")
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/transfer", `{"userId":`+strconv.FormatInt(editorUser.ID, 10)+`}`); code != 200 {
		t.Fatalf("transfer: %d %s", code, body)
	}
	if code, body := doClient(t, editor, srv, "GET", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10), ""); code != 200 || !strings.Contains(body, `"canManage":true`) || !strings.Contains(body, `"role":"owner"`) {
		t.Fatalf("new owner: %d %s", code, body)
	}
	if code, body := doClient(t, viewer, srv, "GET", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10), ""); code != 200 || !strings.Contains(body, `"ownerName":"Editor"`) {
		t.Fatalf("transfer visible to member: %d %s", code, body)
	}

	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/lists", `{"name":"Public pack","visibility":"link"}`); code != 403 {
		t.Fatalf("link list should be off by default: %d", code)
	}
	if code, _ := doClient(t, owner, srv, "PATCH", "/api/admin/settings", `{"linkListsEnabled":true}`); code != 200 {
		t.Fatalf("turn on links: %d", code)
	}
	code, body = doClient(t, owner, srv, "POST", "/api/v2/lists", `{"name":"Public pack","visibility":"link"}`)
	var public store.ListDetail
	if code != 200 || json.Unmarshal([]byte(body), &public) != nil || public.Slug == "" {
		t.Fatalf("create public list: %d %s", code, body)
	}
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(public.ID, 10)+"/skills", `{"input":"a/b/public-skill","note":"read me"}`); code != 200 {
		t.Fatalf("public add: %d %s", code, body)
	}
	code, body = do(t, srv, "GET", "/api/v2/public/lists/"+public.Slug, "")
	if code != 200 || !strings.Contains(body, "public-skill") || !strings.Contains(body, "read me") {
		t.Fatalf("public read: %d %s", code, body)
	}
	if strings.Contains(body, "owner@example.com") || strings.Contains(body, "editor@example.com") || strings.Contains(body, `"members"`) {
		t.Fatalf("public list leaked people: %s", body)
	}
	if code, _ := doClient(t, owner, srv, "PATCH", "/api/admin/settings", `{"linkListsEnabled":false}`); code != 200 {
		t.Fatalf("turn off links: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/v2/public/lists/"+public.Slug, ""); code != 404 {
		t.Fatalf("public read after off: %d", code)
	}
	if code, body := doClient(t, owner, srv, "PATCH", "/api/v2/lists/"+strconv.FormatInt(public.ID, 10), `{"name":"Public pack renamed"}`); code != 200 || !strings.Contains(body, "Public pack renamed") {
		t.Fatalf("rename while links off: %d %s", code, body)
	}
}

func TestSharedListCardsAndMembersVisibility(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "bootstrap")
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	hash, _ := auth.HashPassword("long-enough-password")
	other, _ := st.CreateUser("other@example.com", "Other", "member", hash)
	otherClient := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, otherClient, srv, "POST", "/api/v2/auth/sign-in", `{"email":"other@example.com","password":"long-enough-password"}`); code != 200 {
		t.Fatalf("other sign-in: %d", code)
	}
	code, body := doClient(t, owner, srv, "POST", "/api/v2/lists", `{"name":"Team picks","visibility":"members"}`)
	var list store.ListDetail
	if code != 200 || json.Unmarshal([]byte(body), &list) != nil {
		t.Fatalf("create: %d %s", code, body)
	}
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/skills", `{"input":"a/b/team-skill"}`); code != 200 {
		t.Fatalf("add: %d %s", code, body)
	}
	if code, body := doClient(t, otherClient, srv, "GET", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10), ""); code != 200 || !strings.Contains(body, "team-skill") || strings.Contains(body, "owner@example.com") {
		t.Fatalf("members read: %d %s", code, body)
	}
	if code, _ := doClient(t, otherClient, srv, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/add-to-shelf", `{}`); code != 200 {
		t.Fatalf("other add all: %d", code)
	}
	if code, body := doClient(t, otherClient, srv, "GET", "/api/skills", ""); code != 200 || !strings.Contains(body, "team-skill") || !strings.Contains(body, "Team picks") {
		t.Fatalf("card list label: %d %s", code, body)
	}
	_ = other
}

func TestAdminRollupAndPrivacy(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "bootstrap")
	ownerClient := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, ownerClient, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	owner, _, _ := st.UserByEmail("owner@example.com")
	hash, _ := auth.HashPassword("long-enough-password")
	member, _ := st.CreateUser("member@example.com", "Member", "member", hash)
	viewer, _ := st.CreateUser("viewer@example.com", "Viewer", "viewer", hash)
	for _, u := range []struct{ email string }{{"member@example.com"}, {"viewer@example.com"}} {
		client := &http.Client{Jar: mustJar(t)}
		if code, body := doClient(t, client, srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+u.email+`","password":"long-enough-password"}`); code != 200 {
			t.Fatalf("sign-in %s: %d %s", u.email, code, body)
		}
	}
	yes := true
	team := ref.Ref{ID: "a/b/team-used", Source: "a/b", Slug: "team-used", SourceType: "github", URL: "https://www.skills.sh/a/b/team-used"}
	unlisted := ref.Ref{ID: "a/b/unlisted", Source: "a/b", Slug: "unlisted", SourceType: "github", URL: "https://www.skills.sh/a/b/unlisted"}
	unused := ref.Ref{ID: "a/b/list-unused", Source: "a/b", Slug: "list-unused", SourceType: "github", URL: "https://www.skills.sh/a/b/list-unused"}
	for _, r := range []ref.Ref{team, unlisted, unused} {
		if err := st.EnsureSkill(r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.UpsertFor(owner.ID, team, store.Patch{Used: &yes}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertFor(member.ID, team, store.Patch{Used: &yes}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertFor(member.ID, unlisted, store.Patch{Used: &yes}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEnvironmentFor(member.ID, team.ID, "member-mbp", "Claude Code", "", "/skills", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEnvironmentFor(viewer.ID, unused.ID, "viewer-mbp", "Cursor", "", "/skills", ""); err != nil {
		t.Fatal(err)
	}
	list, err := st.CreateList(owner.ID, "Team picks", "", "members")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddListSkill(list.ID, team.ID, "", owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddListSkill(list.ID, unused.ID, "", owner.ID); err != nil {
		t.Fatal(err)
	}

	code, body := doClient(t, ownerClient, srv, "GET", "/api/admin/rollup", "")
	var rollup store.Rollup
	if code != 200 || json.Unmarshal([]byte(body), &rollup) != nil {
		t.Fatalf("rollup: %d %s", code, body)
	}
	if !rollup.Individual || rollup.TeamSize != 3 || rollup.NothingInstalled != 1 || len(rollup.Members) != 3 || len(rollup.Skills) != 3 {
		t.Fatalf("rollup shape: %+v", rollup)
	}
	byID := map[string]store.RollupSkill{}
	for _, sk := range rollup.Skills {
		byID[sk.ID] = sk
	}
	if got := byID[team.ID]; got.UsedBy != 2 || got.OnShelf != 2 || got.InstalledBy != 1 || got.EnvironmentLinks != 1 || len(got.Lists) != 1 || len(got.Members) != 3 {
		t.Fatalf("team skill: %+v", got)
	}
	if got := byID[unused.ID]; got.EnvironmentLinks != 1 {
		t.Fatalf("environment links: %+v", got)
	}
	if _, ok := byID[unused.ID]; !ok {
		t.Fatalf("list-only skill missing: %+v", rollup.Skills)
	}
	if len(rollup.Drift.Unlisted) != 1 || rollup.Drift.Unlisted[0] != unlisted.ID {
		t.Fatalf("drift unlisted: %+v", rollup.Drift)
	}
	if len(rollup.Drift.Unused) != 1 || rollup.Drift.Unused[0].SkillID != unused.ID {
		t.Fatalf("drift unused: %+v", rollup.Drift)
	}
	memberClient := &http.Client{Jar: mustJar(t)}
	doClient(t, memberClient, srv, "POST", "/api/v2/auth/sign-in", `{"email":"member@example.com","password":"long-enough-password"}`)
	if code, _ := doClient(t, memberClient, srv, "GET", "/api/admin/rollup", ""); code != 403 {
		t.Fatalf("member rollup: %d", code)
	}
	if code, _ := doClient(t, ownerClient, srv, "PATCH", "/api/admin/settings", `{"rollupIndividual":false}`); code != 200 {
		t.Fatalf("privacy setting: %d", code)
	}
	rollup = store.Rollup{}
	code, body = doClient(t, ownerClient, srv, "GET", "/api/admin/rollup", "")
	if code != 200 || json.Unmarshal([]byte(body), &rollup) != nil || rollup.Individual {
		t.Fatalf("private rollup: %d %s", code, body)
	}
	if rollup.TeamSize != 3 || rollup.NothingInstalled != 1 || len(rollup.Members) != 0 {
		t.Fatalf("private totals: %+v", rollup)
	}
	for _, sk := range rollup.Skills {
		if len(sk.Members) != 0 {
			t.Fatalf("individual rows leaked: %+v", sk)
		}
	}
	if strings.Contains(body, "member@example.com") || strings.Contains(body, "viewer@example.com") {
		t.Fatalf("private rollup leaked people: %s", body)
	}
}
