package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/desk"
)

type deskFixture struct {
	t      *testing.T
	srv    *Server
	cookie string
}

func newDeskFixture(t *testing.T) *deskFixture {
	t.Helper()
	srv, st, _, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	return &deskFixture{t, srv, terminalLogin(t, st, uid)}
}

func (f *deskFixture) do(method, path, body string, edit ...func(*http.Request)) (int, map[string]any) {
	f.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://reet.example")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	for _, e := range edit {
		e(r)
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

const deskTwoPaneDoc = `{"tabs":[{"id":"t1","name":"Main","colour":"","root":{"split":"right","ratio":0.5,"a":{"pane":"p1"},"b":{"pane":"p2"}},"zoomed":"","active":"p1","panes":{
"p1":{"id":"p1","name":"api","colour":"","host":"m1","session":"s1","kind":"shell","dir":"/a","dirAt":0,"start":"make dev"},
"p2":{"id":"p2","name":"web","colour":"","host":"m2","session":"","kind":"agent","dir":"/b","dirAt":0,"start":""}}}],"active":"t1"}`

func TestDeskRoutesNeedAccount(t *testing.T) {
	f := newDeskFixture(t)
	for _, c := range [][2]string{{"GET", "/api/v2/desks"}, {"POST", "/api/v2/desks"}, {"GET", "/api/v2/desks/x"}, {"PATCH", "/api/v2/desks/x"}, {"DELETE", "/api/v2/desks/x"}, {"POST", "/api/v2/desks/reorder"}} {
		r := httptest.NewRequest(c[0], c[1], strings.NewReader(`{}`))
		r.Header.Set("Origin", "https://reet.example")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		f.srv.ServeHTTP(w, r)
		if w.Code != 401 && w.Code != 403 {
			t.Fatalf("%s %s without a browser session: %d", c[0], c[1], w.Code)
		}
	}
	// An API token is not a browser session either.
	code, _ := f.do("GET", "/api/v2/desks", "", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer fixture-owner")
	})
	if code != 403 {
		t.Fatalf("API token: %d", code)
	}
}

func TestDeskCreatePatchGet(t *testing.T) {
	f := newDeskFixture(t)
	code, d := f.do("POST", "/api/v2/desks", `{"name":"Work","colour":"blue"}`)
	if code != 201 || d["name"] != "Work" || d["version"] != float64(1) || d["schema"] != float64(1) {
		t.Fatalf("create: %d %v", code, d)
	}
	id := d["id"].(string)
	code, d = f.do("PATCH", "/api/v2/desks/"+id, `{"expect":1,"name":"Work 2","doc":`+deskTwoPaneDoc+`}`)
	if code != 200 || d["version"] != float64(2) || d["name"] != "Work 2" || d["colour"] != "blue" {
		t.Fatalf("patch: %d %v", code, d)
	}
	code, d = f.do("GET", "/api/v2/desks/"+id, "")
	if code != 200 || d["version"] != float64(2) || len(d["doc"].(map[string]any)["tabs"].([]any)) != 1 {
		t.Fatalf("get: %d %v", code, d)
	}
	code, d = f.do("GET", "/api/v2/desks", "")
	list := d["desks"].([]any)
	if code != 200 || len(list) != 1 {
		t.Fatalf("list: %d %v", code, d)
	}
	row := list[0].(map[string]any)
	if row["tabs"] != float64(1) || row["panes"] != float64(2) || row["sessions"].([]any)[0] != "s1" {
		t.Fatalf("summary: %v", row)
	}
	if h := row["hosts"].([]any); len(h) != 2 || h[0] != "m1" || h[1] != "m2" {
		t.Fatalf("hosts: %v", h)
	}
	if code, d = f.do("GET", "/api/v2/desks/nope", ""); code != 404 || d["code"] != "desk_missing" {
		t.Fatalf("missing: %d %v", code, d)
	}
	if code, d = f.do("POST", "/api/v2/desks", `{"name":"work 2"}`); code != 409 || d["code"] != "desk_name_taken" {
		t.Fatalf("name taken: %d %v", code, d)
	}
	if code, d = f.do("POST", "/api/v2/desks/reorder", `{"ids":["`+id+`"]}`); code != 200 {
		t.Fatalf("reorder: %d %v", code, d)
	}
}

func TestDeskStaleWriteReturnsCurrent(t *testing.T) {
	f := newDeskFixture(t)
	_, d := f.do("POST", "/api/v2/desks", `{"name":"Work"}`)
	id := d["id"].(string)
	f.do("PATCH", "/api/v2/desks/"+id, `{"expect":1,"name":"Mine"}`)
	code, d := f.do("PATCH", "/api/v2/desks/"+id, `{"expect":1,"name":"Theirs"}`)
	cur, _ := d["desk"].(map[string]any)
	if code != 409 || d["error"] != "desk_stale" || cur["name"] != "Mine" || cur["version"] != float64(2) {
		t.Fatalf("stale patch: %d %v", code, d)
	}
	if code, d = f.do("DELETE", "/api/v2/desks/"+id, `{"expect":1,"endSessions":false}`); code != 409 || d["error"] != "desk_stale" {
		t.Fatalf("stale delete: %d %v", code, d)
	}
	if code, _ = f.do("PATCH", "/api/v2/desks/"+id, `{"name":"No expect"}`); code != 409 {
		t.Fatalf("missing expect: %d", code)
	}
}

func TestDeskFromCopiesWithoutSessions(t *testing.T) {
	f := newDeskFixture(t)
	_, d := f.do("POST", "/api/v2/desks", `{"name":"Work"}`)
	id := d["id"].(string)
	f.do("PATCH", "/api/v2/desks/"+id, `{"expect":1,"doc":`+deskTwoPaneDoc+`}`)
	code, c := f.do("POST", "/api/v2/desks", `{"name":"Copy","template":true,"from":"`+id+`"}`)
	if code != 201 || c["template"] != true {
		t.Fatalf("copy: %d %v", code, c)
	}
	raw, _ := json.Marshal(c["doc"])
	doc, err := desk.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sessions()) != 0 || len(doc.Tabs) != 1 || len(doc.Tabs[0].Panes) != 2 {
		t.Fatalf("copy doc: %s", raw)
	}
	tab := doc.Tabs[0]
	if tab.ID == "t1" || doc.Active != tab.ID {
		t.Fatalf("tab id not fresh: %s", raw)
	}
	var names []string
	for pid, p := range tab.Panes {
		if pid == "p1" || pid == "p2" {
			t.Fatalf("pane id not fresh: %s", raw)
		}
		names = append(names, p.Name+p.Host+p.Dir+p.Start)
	}
	if got := strings.Join(names, "|"); !strings.Contains(got, "apim1/amake dev") || !strings.Contains(got, "webm2/b") {
		t.Fatalf("copy lost fields: %s", got)
	}
	// The source keeps its session.
	_, src := f.do("GET", "/api/v2/desks/"+id, "")
	if !strings.Contains(mustJSON(src["doc"]), `"session":"s1"`) {
		t.Fatal("source lost its session")
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestDeskRejectsOversizeDoc(t *testing.T) {
	f := newDeskFixture(t)
	_, d := f.do("POST", "/api/v2/desks", `{"name":"Work"}`)
	id := d["id"].(string)
	big := `{"tabs":[],"active":"` + strings.Repeat("x", 65<<10) + `"}`
	code, d := f.do("PATCH", "/api/v2/desks/"+id, `{"expect":1,"doc":`+big+`}`)
	if code != 400 || d["code"] != "desk_invalid" {
		t.Fatalf("oversize: %d %v", code, d)
	}
}

func TestDeskRejectsOtherSite(t *testing.T) {
	f := newDeskFixture(t)
	other := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
	if code, _ := f.do("POST", "/api/v2/desks", `{"name":"Work"}`, other); code != 403 {
		t.Fatalf("create from another site: %d", code)
	}
	_, d := f.do("POST", "/api/v2/desks", `{"name":"Work"}`)
	id := d["id"].(string)
	if code, _ := f.do("PATCH", "/api/v2/desks/"+id, `{"expect":1,"name":"X"}`, other); code != 403 {
		t.Fatalf("patch from another site: %d", code)
	}
	if code, _ := f.do("DELETE", "/api/v2/desks/"+id, `{"expect":1,"endSessions":false}`, other); code != 403 {
		t.Fatalf("delete from another site: %d", code)
	}
}

func TestDeskDeleteLeavesSessions(t *testing.T) {
	f := newDeskFixture(t)
	_, d := f.do("POST", "/api/v2/desks", `{"name":"Work"}`)
	id := d["id"].(string)
	f.do("PATCH", "/api/v2/desks/"+id, `{"expect":1,"doc":`+deskTwoPaneDoc+`}`)
	code, d := f.do("DELETE", "/api/v2/desks/"+id, `{"expect":2,"endSessions":false}`)
	if code != 200 || d["ok"] != true || len(d["left"].([]any)) != 0 {
		t.Fatalf("delete: %d %v", code, d)
	}
	if code, _ = f.do("GET", "/api/v2/desks/"+id, ""); code != 404 {
		t.Fatalf("desk still there: %d", code)
	}
	// Asking to end a session reet does not know about deletes the desk and
	// reports nothing left behind.
	_, d = f.do("POST", "/api/v2/desks", `{"name":"Again"}`)
	id = d["id"].(string)
	f.do("PATCH", "/api/v2/desks/"+id, `{"expect":1,"doc":`+deskTwoPaneDoc+`}`)
	if code, d = f.do("DELETE", "/api/v2/desks/"+id, `{"expect":2,"endSessions":true}`); code != 200 || len(d["left"].([]any)) != 0 {
		t.Fatalf("delete ending: %d %v", code, d)
	}
}
