package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/desk"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
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

func TestDeskDeleteEndsLiveSessions(t *testing.T) {
	srv, st, agent, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	f := &deskFixture{t, srv, terminalLogin(t, st, uid)}
	gen, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), terminalproto.Capabilities{
		ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, TerminalControl: true,
		HealthEnabled: true, Readiness: terminalproto.Ready,
	})
	if err != nil {
		t.Fatal(err)
	}
	running := func(title string) string {
		sess, _, err := st.ReserveTerminalCreateFor(uid, agent.ID, terminalbroker.ID(), gen.Generation, terminalproto.Create{Title: title, Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.ObserveTerminalStateFor(uid, agent.ID, sess.ID, gen.Generation, terminalproto.Running, nil); err != nil {
			t.Fatal(err)
		}
		return sess.ID
	}
	good, bad := running("good"), running("bad")
	doc := strings.NewReplacer(`"session":"s1"`, `"session":"`+good+`"`, `"session":""`, `"session":"`+bad+`"`).Replace(deskTwoPaneDoc)

	old := deskEndSession
	t.Cleanup(func() { deskEndSession = old })
	var asked []string
	deskEndSession = func(s *server, ctx context.Context, uid int64, session, request string) error {
		asked = append(asked, session)
		if session == bad {
			return errors.New("machine unreachable")
		}
		return old(s, ctx, uid, session, request)
	}

	_, d := f.do("POST", "/api/v2/desks", `{"name":"Work"}`)
	id := d["id"].(string)
	if code, d := f.do("PATCH", "/api/v2/desks/"+id, `{"expect":1,"doc":`+doc+`}`); code != 200 {
		t.Fatalf("patch: %d %v", code, d)
	}
	// A stale delete ends nothing and leaves the desk.
	if code, _ := f.do("DELETE", "/api/v2/desks/"+id, `{"expect":1,"endSessions":true}`); code != 409 || len(asked) != 0 {
		t.Fatalf("stale delete: %d, asked %v", code, asked)
	}
	code, d := f.do("DELETE", "/api/v2/desks/"+id, `{"expect":2,"endSessions":true}`)
	left, _ := d["left"].([]any)
	if code != 200 || d["ok"] != true || len(left) != 1 || left[0] != bad {
		t.Fatalf("delete: %d %v", code, d)
	}
	if len(asked) != 2 {
		t.Fatalf("asked to end %v, want both sessions", asked)
	}
	if code, _ := f.do("GET", "/api/v2/desks/"+id, ""); code != 404 {
		t.Fatalf("desk still there: %d", code)
	}
	sess, err := st.TerminalSessionFor(uid, good)
	if err != nil || sess.State != terminalproto.EndRequested || !sess.RemoveHistory {
		t.Fatalf("ended session: %+v %v", sess, err)
	}
	if sess, err := st.TerminalSessionFor(uid, bad); err != nil || sess.State != terminalproto.Running {
		t.Fatalf("failed session should be untouched: %+v %v", sess, err)
	}
}

const reopenPane = "0123456789abcdef"

func reopenCaps(deskShell, shellDir bool) terminalproto.Capabilities {
	caps := launchCaps()
	caps.DeskShell, caps.ShellDir = deskShell, shellDir
	return caps
}

// reopenDesk makes a desk with one pane on the fixture's machine, of kind,
// holding session (may be empty), with the folder a browser wrote.
func reopenDesk(t *testing.T, f *launchAPI, kind, session, dir string) (string, float64) {
	t.Helper()
	doc := `{"tabs":[{"id":"t1","name":"Main","colour":"","root":{"pane":"` + reopenPane + `"},"zoomed":"","active":"` + reopenPane + `","panes":{
"` + reopenPane + `":{"id":"` + reopenPane + `","name":"api","colour":"","host":"` + f.agent.ID + `","session":"` + session + `","kind":"` + kind + `","dir":"` + dir + `","dirAt":4102444800000,"start":"make dev"}}}],"active":"t1"}`
	code, d, raw := f.do(t, f.cookie, "POST", "/api/v2/desks", `{"name":"Reopen"}`)
	if code != 201 {
		t.Fatal(code, raw)
	}
	id := d["id"].(string)
	code, d, raw = f.do(t, f.cookie, "PATCH", "/api/v2/desks/"+id, `{"expect":1,"doc":`+doc+`}`)
	if code != 200 {
		t.Fatal(code, raw)
	}
	return id, d["version"].(float64)
}

func reopenPath(id string) string { return "/api/v2/desks/" + id + "/panes/" + reopenPane + "/reopen" }

func reopenedPane(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	doc := out["desk"].(map[string]any)["doc"].(map[string]any)
	return doc["tabs"].([]any)[0].(map[string]any)["panes"].(map[string]any)[reopenPane].(map[string]any)
}

// startShell starts a plain shell through the terminal route and returns its id.
func startShell(t *testing.T, f *launchAPI) string {
	t.Helper()
	code, out, raw := f.create(t, map[string]any{"agentId": f.agent.ID, "requestId": terminalbroker.ID(), "title": "Shell", "cols": 80, "rows": 24})
	if code != 200 {
		t.Fatal(code, raw)
	}
	return out["id"].(string)
}

func TestReopenWritesSessionIntoDoc(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(true, true))
	id, version := reopenDesk(t, f, "agent", "", "")
	code, out, raw := f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": version})
	if code != 200 {
		t.Fatal(code, raw)
	}
	session := out["session"].(map[string]any)
	pane := reopenedPane(t, out)
	if pane["session"] != session["id"] || pane["kind"] != "shell" || out["desk"].(map[string]any)["version"] != version+1 {
		t.Fatalf("desk after reopen: %s", raw)
	}
	if session["title"] != "api" || session["state"] != "running" {
		t.Fatalf("session: %v", session)
	}
	sent := f.sent()
	if len(sent) != 1 || sent[0].Desk != (terminalproto.DeskRef{Pane: reopenPane}) || sent[0].Dir != "" || sent[0].Tool != "" || sent[0].Title != "api" {
		t.Fatalf("sent: %+v", sent)
	}
	found := false
	events, _ := f.st.ListEventsFor(f.uid, 50, "")
	for _, e := range events {
		found = found || e.Action == "desk-pane-reopened"
	}
	if !found {
		t.Fatal("no desk-pane-reopened audit")
	}
	// The shell is live now: a second reopen is refused.
	if code, out, raw := f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": version + 1}); code != 409 || out["code"] != "pane_live" {
		t.Fatal(code, raw)
	}
}

// Only a folder the machine itself reported for the pane goes out as
// Desk.Dir (which may lie outside home); one a browser wrote goes as Dir,
// which keeps the home rule.
func TestReopenOutsideHomeOnlyForRecordedDir(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(true, true))
	id, version := reopenDesk(t, f, "shell", "", "/srv/browser")
	code, out, raw := f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": version})
	if code != 200 {
		t.Fatal(code, raw)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0].Dir != "/srv/browser" || sent[0].Desk != (terminalproto.DeskRef{Pane: reopenPane}) {
		t.Fatalf("browser folder: %+v", sent)
	}
	// The machine reports the shell in /srv/app, then the shell ends.
	sid := out["session"].(map[string]any)["id"].(string)
	if err := f.st.RecordTerminalLiveDirFor(f.uid, f.agent.ID, sid, "/srv/app", 4102444900000); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.s.terminals.End(t.Context(), f.uid, sid, terminalbroker.ID()); err != nil {
		t.Fatal(err)
	}
	version = out["desk"].(map[string]any)["version"].(float64)
	code, out, raw = f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": version})
	if code != 200 {
		t.Fatal(code, raw)
	}
	if sent := f.sent(); len(sent) != 2 || sent[1].Dir != "" || sent[1].Desk != (terminalproto.DeskRef{Pane: reopenPane, Dir: "/srv/app"}) {
		t.Fatalf("reported folder: %+v", sent)
	}
	// A browser that writes another folder (even with a later dirAt) gets the
	// home rule again.
	sid = out["session"].(map[string]any)["id"].(string)
	if _, err := f.srv.s.terminals.End(t.Context(), f.uid, sid, terminalbroker.ID()); err != nil {
		t.Fatal(err)
	}
	d, err := f.st.DeskFor(f.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	p := d.Doc.Tabs[0].Panes[reopenPane]
	p.Dir, p.DirAt = "/etc", 1<<52
	d.Doc.Tabs[0].Panes[reopenPane] = p
	raw2, _ := json.Marshal(d.Doc)
	if code, _, raw := f.do(t, f.cookie, "PATCH", "/api/v2/desks/"+id, `{"expect":`+jsonNumber(d.Version)+`,"doc":`+string(raw2)+`}`); code != 200 {
		t.Fatal(code, raw)
	}
	if code, _, raw := f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": d.Version + 1}); code != 200 {
		t.Fatal(code, raw)
	}
	if sent := f.sent(); len(sent) != 3 || sent[2].Dir != "/etc" || sent[2].Desk.Dir != "" {
		t.Fatalf("browser rewrite: %+v", sent)
	}
}

func jsonNumber(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestReopenLivePaneRefused(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(true, true))
	sid := startShell(t, f)
	id, version := reopenDesk(t, f, "shell", sid, "")
	code, out, raw := f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": version})
	if code != 409 || out["code"] != "pane_live" || out["error"] == "" {
		t.Fatal(code, raw)
	}
	if len(f.sent()) != 1 {
		t.Fatalf("a live pane was reopened: %+v", f.sent())
	}
}

func TestReopenHome(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(true, true))
	id, version := reopenDesk(t, f, "shell", "", "/home/tester/gone")
	code, _, raw := f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": version, "home": true})
	if code != 200 {
		t.Fatal(code, raw)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0].Dir != "" || sent[0].Desk != (terminalproto.DeskRef{Pane: reopenPane}) {
		t.Fatalf("sent: %+v", sent)
	}
}

func TestReopenStale(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(true, true))
	id, version := reopenDesk(t, f, "shell", "", "")
	for _, body := range []any{map[string]any{"expect": version - 1}, map[string]any{}} {
		code, out, raw := f.do(t, f.cookie, "POST", reopenPath(id), body)
		if code != 409 || out["code"] != "desk_stale" || out["desk"].(map[string]any)["version"] != version {
			t.Fatal(code, raw)
		}
	}
	if len(f.sent()) != 0 {
		t.Fatalf("a stale reopen started a shell: %+v", f.sent())
	}
}

// A folder the machine says is gone is answered as folder_gone and the pane
// is left without the session that never started.
func TestReopenFolderGone(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(true, true))
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.LaunchFailed, Detail: terminalproto.FolderGone}
	}
	id, version := reopenDesk(t, f, "shell", "", "/home/tester/old")
	code, out, raw := f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": version})
	if code != 409 || out["code"] != "folder_gone" {
		t.Fatal(code, raw)
	}
	d, _ := f.st.DeskFor(f.uid, id)
	if p := d.Doc.Tabs[0].Panes[reopenPane]; p.Session != "" || p.Dir != "/home/tester/old" {
		t.Fatalf("pane after a failed reopen: %+v", p)
	}
}

// A machine without desk-shell-v1 is sent no desk reference: the folder goes
// as Dir under shell-dir-v1, and without that either it is refused.
func TestReopenOnOlderMachine(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(false, true))
	id, version := reopenDesk(t, f, "shell", "", "/home/tester/app")
	if code, _, raw := f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": version}); code != 200 {
		t.Fatal(code, raw)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0].Dir != "/home/tester/app" || sent[0].Desk != (terminalproto.DeskRef{}) {
		t.Fatalf("sent: %+v", sent)
	}

	old := launchAPIFixture(t, reopenCaps(false, false))
	id, version = reopenDesk(t, old, "shell", "", "/home/tester/app")
	code, out, raw := old.do(t, old.cookie, "POST", reopenPath(id), map[string]any{"expect": version})
	if code != 409 || out["code"] != "shell_dir_unsupported" {
		t.Fatal(code, raw)
	}
	if len(old.sent()) != 0 {
		t.Fatalf("sent: %+v", old.sent())
	}
	if d, _ := old.st.DeskFor(old.uid, id); d.Version != int64(version) || d.Doc.Tabs[0].Panes[reopenPane].Session != "" {
		t.Fatalf("a refused reopen changed the desk: %+v", d)
	}
	// Home needs no folder and works on it.
	if code, _, raw := old.do(t, old.cookie, "POST", reopenPath(id), map[string]any{"expect": version, "home": true}); code != 200 {
		t.Fatal(code, raw)
	}
}

func TestReopenMissingAndGone(t *testing.T) {
	f := launchAPIFixture(t, reopenCaps(true, true))
	id, version := reopenDesk(t, f, "shell", "", "")
	if code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/desks/nope/panes/"+reopenPane+"/reopen", map[string]any{"expect": 1}); code != 404 || out["code"] != "desk_missing" {
		t.Fatal(code, raw)
	}
	if code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/desks/"+id+"/panes/fedcba9876543210/reopen", map[string]any{"expect": version}); code != 404 || out["code"] != "pane_missing" {
		t.Fatal(code, raw)
	}
	d, _ := f.st.DeskFor(f.uid, id)
	p := d.Doc.Tabs[0].Panes[reopenPane]
	p.Host = "removed-machine-0000000000"
	d.Doc.Tabs[0].Panes[reopenPane] = p
	d, err := f.st.UpdateDeskFor(f.uid, d, d.Version)
	if err != nil {
		t.Fatal(err)
	}
	if code, out, raw := f.do(t, f.cookie, "POST", reopenPath(id), map[string]any{"expect": d.Version}); code != 409 || out["code"] != "machine_gone" {
		t.Fatal(code, raw)
	}
	if len(f.sent()) != 0 {
		t.Fatalf("sent: %+v", f.sent())
	}
}
