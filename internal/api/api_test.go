package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/meta"
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
		w.Write([]byte(`<meta name="description" content="Find skills fast">`))
	}))
	defer page.Close()
	srv := newSrv(t, meta.Fetcher{Client: page.Client(), PageBase: page.URL + "/"}, "")
	do(t, srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`)
	for i := 0; i < 40; i++ {
		if l := list(t, srv, ""); l.Skills[0].Description == "Find skills fast" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("description never arrived")
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
	if !s.Liked || !s.Bookmarked || !s.Used || s.Notes != "keep me" {
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
	if code, _ := do(t, srv, "GET", "/quick?action=like&url=https://example.com/a/b/c", ""); code != 400 {
		t.Fatalf("foreign url: %d", code)
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
