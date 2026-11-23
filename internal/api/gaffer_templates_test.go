package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"github.com/eyupio/reet.sh/internal/store"
)

func templateServer(t *testing.T) (*store.Store, *httptest.Server, *http.Client) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup %d", code)
	}
	return st, srv, owner
}

// doneOutcome is a finished one-pipeline plan of the account's, with a skill
// on its review stage that is not in the account's library.
func doneOutcome(t *testing.T, st *store.Store, uid int64) store.GafferOutcome {
	t.Helper()
	o, err := st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: strings.Repeat("e", 32), Objective: "Ship CSV export.", AgentID: "machine-1", ProjectID: "project-1", Dir: "/work/billing", Tool: "opencode", MaxReviews: 8, Hours: 24,
		Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Export API", Prompt: "Add the export.", RequestID: strings.Repeat("c", 32),
			Criteria: []store.GafferCriterion{{ID: strings.Repeat("d", 32), Text: "It downloads as CSV.", Revision: 1}}, CriteriaRevision: 1,
			Stages: []store.GafferOutcomeStage{{Kind: "code", Tool: "opencode", Tier: "standard"}, {Kind: "review", Tool: "opencode", Tier: "heavy", SkillGuidance: []skillbuddy.Skill{{ID: "library:gone", Name: "Gone", Source: "personal", Revision: "r1"}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	o, err = st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Sessions[0].State = "settled", "done"
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// Templates are the owning browser account's: not signed out, not with an
// API token, not from another origin, not for a read-only account, and
// another account's template is not found. Bodies are strict.
func TestGafferTemplateRoutesNeedTheOwningBrowser(t *testing.T) {
	t.Parallel()
	st, srv, owner := templateServer(t)
	if _, err := st.CreateAPIToken(1, "Template refusal test", auth.HashToken("synthetic-api-token")); err != nil {
		t.Fatal(err)
	}
	o := doneOutcome(t, st, 1)
	code, body := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/templates", `{"requestId":"`+strings.Repeat("1", 32)+`","outcomeId":"`+o.ID+`","name":"CSV export"}`)
	if code != 201 {
		t.Fatalf("save: %d %s", code, body)
	}
	var saved struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &saved)
	base := "/api/v2/gaffer/templates/" + saved.ID
	routes := []struct{ method, path, body string }{
		{"GET", "/api/v2/gaffer/templates", ""},
		{"POST", "/api/v2/gaffer/templates", `{"requestId":"` + strings.Repeat("2", 32) + `","outcomeId":"` + o.ID + `","name":"Again"}`},
		{"GET", base, ""},
		{"GET", base + "/versions/1", ""},
		{"PUT", base, `{"requestId":"` + strings.Repeat("3", 32) + `","baseVersion":1,"content":{"name":"x"}}`},
		{"GET", base + "/diff", ""},
		{"POST", base + "/launch", `{"requestId":"` + strings.Repeat("4", 32) + `","version":1,"projectId":"p","tool":"opencode","confirmed":true}`},
		{"DELETE", base + "?version=1", ""},
	}
	password, _ := auth.HashPassword("long-enough-password")
	clients := map[string]*http.Client{}
	for _, role := range []string{"member", "viewer"} {
		user, err := st.CreateUser(role+"@example.test", role, role, password)
		if err != nil {
			t.Fatal(err)
		}
		clients[role] = &http.Client{Jar: mustJar(t)}
		if code, _ := gafferClient(t, clients[role], srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+user.Email+`","password":"long-enough-password"}`); code != 200 {
			t.Fatal(code)
		}
	}
	for _, route := range routes {
		if code, _ := gafferClient(t, &http.Client{}, srv, route.method, route.path, route.body); code != 401 {
			t.Fatalf("signed out %s %s: %d", route.method, route.path, code)
		}
		if code, _ := gafferClient(t, &http.Client{}, srv, route.method, route.path, route.body, "Authorization", "Bearer synthetic-api-token"); code != 403 {
			t.Fatalf("API token %s %s: %d", route.method, route.path, code)
		}
		if code, _ := gafferClient(t, owner, srv, route.method, route.path, route.body, "Authorization", "Bearer synthetic-api-token"); code != 403 {
			t.Fatalf("API token beside the browser %s %s: %d", route.method, route.path, code)
		}
		if code, _ := gafferClient(t, clients["viewer"], srv, route.method, route.path, route.body); code != 403 {
			t.Fatalf("read-only %s %s: %d", route.method, route.path, code)
		}
		if route.method != "GET" {
			if code, _ := doClient(t, owner, srv, route.method, route.path, route.body, "Origin", "https://different.example"); code != 403 {
				t.Fatalf("foreign origin %s %s: %d", route.method, route.path, code)
			}
		}
		if route.body != "" {
			if code, _ := gafferClient(t, owner, srv, route.method, route.path, strings.Replace(route.body, "{", `{"machine":"m",`, 1)); code != 400 {
				t.Fatalf("unknown field %s %s: %d", route.method, route.path, code)
			}
		}
	}
	// Another account: its own empty list, and nothing of this one's.
	if code, body := gafferClient(t, clients["member"], srv, "GET", "/api/v2/gaffer/templates", ""); code != 200 || !strings.Contains(body, `"templates":[]`) {
		t.Fatalf("other account's list: %d %s", code, body)
	}
	for _, route := range routes[1:] {
		if strings.HasSuffix(route.path, "/launch") {
			// A launch is refused first where remote sessions are off, as here.
			continue
		}
		if code, body := gafferClient(t, clients["member"], srv, route.method, route.path, route.body); code != 404 || strings.Contains(body, "CSV") {
			t.Fatalf("other account %s %s: %d %s", route.method, route.path, code, body)
		}
	}
	if code, _ := gafferClient(t, owner, srv, "GET", base, ""); code != 200 {
		t.Fatalf("owner after the refusals: %d", code)
	}
}

// Through the routes: save, read with the skill that is no longer approved,
// edit on the version seen, compare, refuse a stale or repeated write, and
// delete. A launch is refused where remote sessions are off.
func TestGafferTemplateRoutesSaveEditCompareAndDelete(t *testing.T) {
	t.Parallel()
	st, srv, owner := templateServer(t)
	o := doneOutcome(t, st, 1)
	request := func(c byte) string { return strings.Repeat(string(c), 32) }
	save := `{"requestId":"` + request('1') + `","outcomeId":"` + o.ID + `","name":"CSV export"}`
	code, body := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/templates", save)
	if code != 201 {
		t.Fatalf("save: %d %s", code, body)
	}
	var view struct {
		ID        string                      `json:"id"`
		Version   int                         `json:"version"`
		Latest    int                         `json:"latest"`
		Content   store.GafferTemplateContent `json:"content"`
		SkillGaps []struct {
			Status, Key string
		} `json:"skillGaps"`
	}
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatal(err)
	}
	if view.Version != 1 || view.Latest != 1 || len(view.SkillGaps) != 1 || view.SkillGaps[0].Status != "unavailable" || view.SkillGaps[0].Key != "0:1:library:gone" || strings.Contains(body, "machine-1") || strings.Contains(body, "/work/billing") {
		t.Fatalf("saved view: %s", body)
	}
	if code, again := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/templates", save); code != 201 || !strings.Contains(again, `"id":"`+view.ID+`"`) {
		t.Fatalf("repeated save: %d %s", code, again)
	}
	// A plan that has not succeeded is refused with its code.
	if _, err := st.UpdateGafferOutcomeFor(1, o.ID, o.Revision, func(o *store.GafferOutcome) bool { o.Sessions[0].State = "failed"; return true }); err != nil {
		t.Fatal(err)
	}
	if code, body := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/templates", `{"requestId":"`+request('2')+`","outcomeId":"`+o.ID+`","name":"Failed"}`); code != 409 || !strings.Contains(body, `"code":"template_not_successful"`) {
		t.Fatalf("unsuccessful plan: %d %s", code, body)
	}
	view.Content.Pipelines[0].Prompt = "Add the export, streamed."
	content, _ := json.Marshal(view.Content)
	path := "/api/v2/gaffer/templates/" + view.ID
	// Two edits on version 1 at once: one becomes version 2, the other is told the template changed.
	var wg sync.WaitGroup
	codes := make([]int, 2)
	bodies := make([]string, 2)
	for i, c := range []byte{'3', '4'} {
		wg.Add(1)
		go func(i int, c byte) {
			defer wg.Done()
			codes[i], bodies[i] = gafferClient(t, owner, srv, "PUT", path, `{"requestId":"`+request(c)+`","baseVersion":1,"content":`+string(content)+`}`)
		}(i, c)
	}
	wg.Wait()
	if codes[0]+codes[1] != 200+409 || !strings.Contains(bodies[0]+bodies[1], `"code":"template_changed"`) {
		t.Fatalf("concurrent edits: %v %v", codes, bodies)
	}
	winner := '3'
	if codes[1] == 200 {
		winner = '4'
	}
	// The winning request again is answered with its version, not a third one.
	if code, body := gafferClient(t, owner, srv, "PUT", path, `{"requestId":"`+request(byte(winner))+`","baseVersion":1,"content":`+string(content)+`}`); code != 200 || !strings.Contains(body, `"version":2`) || !strings.Contains(body, `"versions":[1,2]`) {
		t.Fatalf("repeated edit: %d %s", code, body)
	}
	if code, body := gafferClient(t, owner, srv, "GET", path+"/diff", ""); code != 200 || !strings.Contains(body, `"from":1,"to":2`) || !strings.Contains(body, `"promptChanged":true`) {
		t.Fatalf("diff: %d %s", code, body)
	}
	for _, bad := range []struct{ method, path string }{{"GET", path + "/diff?from=x"}, {"GET", path + "/diff?to=0"}, {"DELETE", path + "?version=x"}, {"DELETE", path}} {
		if code, body := gafferClient(t, owner, srv, bad.method, bad.path, ""); code != 422 || !strings.Contains(body, `"field":"version"`) {
			t.Fatalf("%s %s: %d %s", bad.method, bad.path, code, body)
		}
	}
	if code, body := gafferClient(t, owner, srv, "GET", path+"/versions/1", ""); code != 200 || !strings.Contains(body, `"prompt":"Add the export."`) {
		t.Fatalf("version 1 after the edit: %d %s", code, body)
	}
	if code, body := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/templates", ""); code != 200 || !strings.Contains(body, `"version":2`) || !strings.Contains(body, `"lastUsedVersion":0`) {
		t.Fatalf("list: %d %s", code, body)
	}
	if code, body := gafferClient(t, owner, srv, "POST", path+"/launch", `{"requestId":"`+request('5')+`","version":2,"projectId":"p","tool":"opencode","confirmed":true}`); code != 403 || !strings.Contains(body, "Remote sessions are disabled") {
		t.Fatalf("launch with remote sessions off: %d %s", code, body)
	}
	if code, body := gafferClient(t, owner, srv, "DELETE", path+"?version=1", ""); code != 409 || !strings.Contains(body, `"code":"template_changed"`) {
		t.Fatalf("stale delete: %d %s", code, body)
	}
	if code, _ := gafferClient(t, owner, srv, "DELETE", path+"?version=2", ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "GET", path, ""); code != 404 {
		t.Fatalf("after delete: %d", code)
	}
	if after, err := st.GafferOutcomeFor(1, o.ID); err != nil || after.Sessions[0].Title != "Export API" {
		t.Fatalf("the outcome after its template went: %+v %v", after, err)
	}
}

// A launch refused because the skills changed says so with its code and the
// gaps as they are now, so the page need not read the template again.
func TestTemplateSkillsChangedCarriesItsGaps(t *testing.T) {
	w := httptest.NewRecorder()
	gafferTemplateError(w, &gaffer.TemplateSkillsChanged{Gaps: []gaffer.TemplateSkillGap{{Pipeline: 0, Stage: 1, ID: "library:gone", Name: "Gone", Revision: "r1", Status: "unavailable", Key: "0:1:library:gone"}}})
	var body struct {
		Code string                    `json:"code"`
		Gaps []gaffer.TemplateSkillGap `json:"gaps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 409 || body.Code != "template_skills_changed" || len(body.Gaps) != 1 || body.Gaps[0].Key != "0:1:library:gone" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
