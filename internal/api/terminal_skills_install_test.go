package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type installFx struct {
	srv     *Server
	st      *store.Store
	uid     int64
	agent   store.SyncAgent
	session string
	cookie  string
	env     string
	token   string
}

// installFixture is the Phase 1 session fixture plus a complete recent scan of
// the machine's user-level claude folder and two published hosted skills.
func installFixture(t *testing.T) *installFx {
	t.Helper()
	srv, st, agent, _, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	caps, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, Readiness: terminalproto.Ready})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := st.ReserveTerminalCreateFor(uid, agent.ID, terminalbroker.ID(), caps.Generation, terminalproto.Create{Title: "Skills", Tool: "claude", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ClaimPublishingUsername(uid, "install-author"); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"pdf", "docx", "draft"} {
		if _, err := st.SaveHostedSkill(uid, store.HostedSkill{Slug: slug, Description: "Fixture", Content: "---\nname: " + slug + "\ndescription: Fixture\n---\n\nBody", Published: slug != "draft"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.TouchEnvironmentFor(uid, agent.Host, "claude", "", "/home/u/.claude/skills"); err != nil {
		t.Fatal(err)
	}
	env := store.EnvironmentIDFor(uid, agent.Host, "claude", "")
	if err := st.RecordInventoryFor(uid, env, true, store.MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	return &installFx{srv: srv, st: st, uid: uid, agent: agent, session: created.ID, cookie: cookie, env: env, token: token}
}

func (f *installFx) post(path, body, cookie, origin string) (int, string) {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

func (f *installFx) install(body string) (int, string) {
	return f.post("/api/v2/terminal/sessions/"+f.session+"/skills/install", body, f.cookie, "https://reet.example")
}

func (f *installFx) cancel(session, body, cookie string) (int, string) {
	return f.post("/api/v2/terminal/sessions/"+session+"/skills/install/cancel", body, cookie, "https://reet.example")
}

func (f *installFx) raw(t *testing.T, query string, args ...any) {
	t.Helper()
	raw, err := sql.Open("sqlite", filepath.Join(f.st.Dir(), "reet.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *installFx) jobs(t *testing.T) []store.SyncJob {
	t.Helper()
	jobs, err := f.st.SyncJobsFor(f.uid)
	if err != nil {
		t.Fatal(err)
	}
	return jobs
}

type installBody struct {
	Job       string `json:"job"`
	Reused    bool   `json:"reused"`
	Directory string `json:"directory"`
	Host      string `json:"host"`
	Error     string `json:"error"`
	Code      string `json:"code"`
	Reverted  bool   `json:"reverted"`
}

func decodeInstall(t *testing.T, raw string) installBody {
	t.Helper()
	var b installBody
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatal(err, raw)
	}
	return b
}

func TestTerminalSkillInstallQueuesOnePreviewJob(t *testing.T) {
	f := installFixture(t)
	code, raw := f.install(`{"skill":"reet:pdf"}`)
	if code != 200 {
		t.Fatal(code, raw)
	}
	b := decodeInstall(t, raw)
	if b.Job == "" || b.Reused || b.Directory != "/home/u/.claude/skills" || b.Host != f.agent.Host {
		t.Fatalf("body %s", raw)
	}
	jobs := f.jobs(t)
	if len(jobs) != 1 || jobs[0].ID != b.Job || jobs[0].Kind != "preview" || jobs[0].TargetID != f.env || jobs[0].AgentID != f.agent.ID {
		t.Fatalf("jobs %+v", jobs)
	}
	events, err := f.st.ListEvents(20, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.Action == "skill-install" {
			found = true
			if !strings.Contains(e.Detail, f.session) || !strings.Contains(e.Detail, "pdf") || !strings.Contains(e.Detail, b.Job) || strings.Contains(e.Detail, "Body") {
				t.Fatalf("event detail %s", e.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("no skill-install event in %+v", events)
	}
	// Asking again reuses the same job.
	code, raw = f.install(`{"skill":"reet:pdf"}`)
	if again := decodeInstall(t, raw); code != 200 || !again.Reused || again.Job != b.Job {
		t.Fatal(code, raw)
	}
}

func TestTerminalSkillInstallErrorCodesAndStatuses(t *testing.T) {
	cases := []struct {
		name   string
		code   string
		status int
		setup  func(t *testing.T, f *installFx)
		skill  string
		text   string
	}{
		{"unpublished", "unpublished", 422, nil, "reet:draft", "Publish this skill to install it."},
		{"no-scan", "no-scan", 422, func(t *testing.T, f *installFx) {
			f.raw(t, `DELETE FROM environment_scans`)
		}, "reet:pdf", "reet hasn't read " + "laptop" + "'s claude skills folder recently. Run a scan on laptop, then try again."},
		{"connector-offline", "connector-offline", 422, func(t *testing.T, f *installFx) {
			f.raw(t, `UPDATE sync_agents SET last_seen=1 WHERE id=?`, f.agent.ID)
		}, "reet:pdf", "laptop isn't connected to reet right now, so it can't install this skill."},
		{"follows-source", "follows-source", 409, func(t *testing.T, f *installFx) {
			f.raw(t, `INSERT INTO sync_policies(target_id,follow_source) VALUES(?,1) ON CONFLICT(target_id) DO UPDATE SET follow_source=1`, f.env)
		}, "reet:pdf", "laptop's skills follow another machine, so this skill would be removed the next time it syncs."},
		{"slug-conflict", "slug-conflict", 409, func(t *testing.T, f *installFx) {
			f.raw(t, `INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,'shelf','Shelf',?,'rev1',1)`, f.uid, f.env, `[{"id":"x/y/pdf","name":"pdf","slug":"pdf","source":"x/y","sourceType":"github"}]`)
			f.raw(t, `INSERT INTO sync_policies(target_id,follow_source) VALUES(?,0)`, f.env)
			f.raw(t, `INSERT INTO sync_options(target_id,mode) VALUES(?,'mirror')`, f.env)
		}, "reet:pdf", "A different skill named pdf is already in laptop's saved list. Remove it on the Sync page first."},
		{"install-in-progress", "install-in-progress", 409, func(t *testing.T, f *installFx) {
			if code, raw := f.install(`{"skill":"reet:docx"}`); code != 200 {
				t.Fatal(code, raw)
			}
		}, "reet:pdf", "Another install is waiting on laptop. Finish or cancel it first."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := installFixture(t)
			if c.setup != nil {
				c.setup(t, f)
			}
			code, raw := f.install(`{"skill":"` + c.skill + `"}`)
			b := decodeInstall(t, raw)
			if code != c.status || code != installErrorStatus(c.code) || b.Code != c.code || b.Error != c.text {
				t.Fatalf("got %d %s", code, raw)
			}
		})
	}
	t.Run("already-installed", func(t *testing.T) {
		f := installFixture(t)
		body, _ := json.Marshal(map[string]any{"items": []map[string]string{{"name": "pdf", "source": "a/b", "host": f.agent.Host, "agent": "claude"}}})
		r := httptest.NewRequest("POST", "/api/import", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+f.token)
		w := httptest.NewRecorder()
		f.srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if err := f.st.RecordInventoryFor(f.uid, f.env, true, store.MachineInfo{}); err != nil {
			t.Fatal(err)
		}
		code, raw := f.install(`{"skill":"reet:pdf"}`)
		if b := decodeInstall(t, raw); code != 409 || b.Code != "already-installed" || b.Error != "This skill is already on laptop for claude." {
			t.Fatal(code, raw)
		}
	})
	t.Run("unsupported-agent", func(t *testing.T) {
		f := installFixture(t)
		caps, err := f.st.BeginRemoteConnectionFor(f.uid, f.agent.ID, terminalbroker.ID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, Readiness: terminalproto.Ready})
		if err != nil {
			t.Fatal(err)
		}
		created, _, err := f.st.ReserveTerminalCreateFor(f.uid, f.agent.ID, terminalbroker.ID(), caps.Generation, terminalproto.Create{Title: "Codex", Tool: "codex", Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		f.session = created.ID
		code, raw := f.install(`{"skill":"reet:pdf"}`)
		if b := decodeInstall(t, raw); code != 422 || b.Code != "unsupported-agent" || b.Error != "Skills can't be installed for codex from reet yet." {
			t.Fatal(code, raw)
		}
	})
	t.Run("other errors never reach the person", func(t *testing.T) {
		f := installFixture(t)
		f.raw(t, `ALTER TABLE skill_installs RENAME TO skill_installs_gone`)
		code, raw := f.install(`{"skill":"reet:pdf"}`)
		if code != 500 || raw != `{"error":"The install could not be started. Try again.","code":"internal"}` {
			t.Fatalf("got %d %q", code, raw)
		}
	})
}

func TestTerminalSkillInstallRefusesAForeignSessionAnAnonymousCallerAndAViewer(t *testing.T) {
	f := installFixture(t)
	other, err := f.st.CreateUser("install-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := f.st.CreateUser("install-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v2/terminal/sessions/" + f.session + "/skills/install"
	const body = `{"skill":"reet:pdf"}`
	for _, c := range []struct {
		name, cookie string
		want         int
	}{
		{"foreign", terminalLogin(t, f.st, other.ID), 403},
		{"anonymous", "", 401},
		{"viewer", terminalLogin(t, f.st, viewer.ID), 403},
	} {
		if code, raw := f.post(path, body, c.cookie, "https://reet.example"); code != c.want {
			t.Fatalf("%s got %d %s", c.name, code, raw)
		}
	}
	if code, raw := f.post(path, body, f.cookie, "https://evil.example"); code != 403 {
		t.Fatalf("foreign origin got %d %s", code, raw)
	}
	if len(f.jobs(t)) != 0 {
		t.Fatal("a refused call queued a job")
	}
}

func TestTerminalSkillInstallRejectsABadBody(t *testing.T) {
	f := installFixture(t)
	for _, body := range []string{
		`not json`, ``, `{}`, `{"skill":""}`, `{"skill":"pdf"}`, `{"skill":"reet:"}`,
		`{"skill":"reet:a/b"}`, `{"skill":"reet:..\\x"}`, `{"skill":"reet:../pdf"}`, `{"skill":"reet:pdf"} {"x":1}`,
	} {
		if code, raw := f.install(body); code != 400 {
			t.Fatalf("%q got %d %s", body, code, raw)
		}
	}
	if len(f.jobs(t)) != 0 {
		t.Fatal("a bad body queued a job")
	}
	if code, _ := f.cancel(f.session, `{"job":""}`, f.cookie); code != 400 {
		t.Fatalf("empty job got %d", code)
	}
}

func TestTerminalSkillInstallCancel(t *testing.T) {
	f := installFixture(t)
	code, raw := f.install(`{"skill":"reet:pdf"}`)
	if code != 200 {
		t.Fatal(code, raw)
	}
	job := decodeInstall(t, raw).Job
	// Another account cannot cancel it.
	other, err := f.st.CreateUser("cancel-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, raw := f.cancel(f.session, `{"job":"`+job+`"}`, terminalLogin(t, f.st, other.ID)); code != 403 {
		t.Fatalf("other account got %d %s", code, raw)
	}
	if code, raw := f.cancel(f.session, `{"job":"nope"}`, f.cookie); code != 403 {
		t.Fatalf("unknown job got %d %s", code, raw)
	}
	code, raw = f.cancel(f.session, `{"job":"`+job+`"}`, f.cookie)
	if b := decodeInstall(t, raw); code != 200 || !b.Reverted || raw != `{"reverted":true}` {
		t.Fatalf("cancel got %d %s", code, raw)
	}
	targets, err := f.st.SyncTargetsFor(f.uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range targets {
		if x.Target.ID == f.env {
			t.Fatalf("saved set still there: %+v", x)
		}
	}
	code, raw = f.cancel(f.session, `{"job":"`+job+`"}`, f.cookie)
	if b := decodeInstall(t, raw); code != 409 || b.Code != "cannot-cancel" {
		t.Fatalf("second cancel got %d %s", code, raw)
	}
	events, _ := f.st.ListEvents(20, "")
	found := false
	for _, e := range events {
		found = found || e.Action == "skill-install-cancel"
	}
	if !found {
		t.Fatal("no skill-install-cancel event")
	}
}
