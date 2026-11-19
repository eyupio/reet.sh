package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
// the machine's user-level claude folder, two published hosted skills and a draft.
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

type addBody struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Added  bool   `json:"added"`
	Error  string `json:"error"`
	Code   string `json:"code"`
}

func decodeAdd(t *testing.T, raw string) addBody {
	t.Helper()
	var b addBody
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatal(err, raw)
	}
	return b
}

// itemNamed is the person's live library item called name.
func (f *installFx) itemNamed(t *testing.T, name string) store.LibraryItem {
	t.Helper()
	items, err := f.st.LibraryFor(f.uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Name == name {
			return it
		}
	}
	return store.LibraryItem{}
}

// pendingInstall queues an install the way the old session picker did, for the
// tests of what still refuses one while it waits.
func (f *installFx) pendingInstall(t *testing.T, slug string) {
	t.Helper()
	if _, err := f.st.InstallHostedSkillFor(f.uid, store.InstallRequest{Tool: "claude", Host: f.agent.Host, AgentID: f.agent.ID, Slug: slug, Origin: "http://example.com"}); err != nil {
		t.Fatal(err)
	}
}

// The session picker's Add to your skills makes a published reet skill Active
// in the library, with its content, so it goes on every machine. Nothing is
// queued for the one machine.
func TestPickerAddToYourSkillsMakesActive(t *testing.T) {
	f := installFixture(t)
	pdf := f.itemNamed(t, "pdf")
	if pdf.ID == "" || pdf.RevisionID == "" {
		t.Fatalf("pdf: %+v", pdf)
	}
	if err := f.st.SetSkillStatusFor(f.uid, []string{pdf.ID}, store.SkillArchived, time.Now()); err != nil {
		t.Fatal(err)
	}
	code, raw := f.install(`{"skill":"reet:pdf"}`)
	if b := decodeAdd(t, raw); code != 200 || b.Name != "pdf" || b.Status != "active" || !b.Added {
		t.Fatalf("add: %d %s", code, raw)
	}
	if got := f.itemNamed(t, "pdf"); got.Status != store.SkillActive || got.RevisionID == "" {
		t.Fatalf("pdf after add: %+v", got)
	}
	if jobs := f.jobs(t); len(jobs) != 0 {
		t.Fatalf("a sync job was queued: %+v", jobs)
	}
	// Asking again changes nothing: it is already Active.
	if code, raw := f.install(`{"skill":"reet:pdf"}`); code != 200 || decodeAdd(t, raw).Added {
		t.Fatalf("again: %d %s", code, raw)
	}

	// A skill someone else published on this reet is added Active.
	other, err := f.st.CreateUser("publisher@example.test", "Publisher", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.st.ClaimPublishingUsername(other.ID, "publisher"); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"slides", "secret"} {
		if _, err = f.st.SaveHostedSkill(other.ID, store.HostedSkill{Slug: slug, Description: "Shared", Content: "---\nname: " + slug + "\ndescription: Shared\n---\n\nBody", Published: slug == "slides"}); err != nil {
			t.Fatal(err)
		}
	}
	code, raw = f.install(`{"skill":"reet:publisher/slides"}`)
	if b := decodeAdd(t, raw); code != 200 || b.Name != "slides" || b.Status != "active" || !b.Added {
		t.Fatalf("add a published skill: %d %s", code, raw)
	}
	if got := f.itemNamed(t, "slides"); got.ID == "" || got.Status != store.SkillActive {
		t.Fatalf("slides: %+v", got)
	}
	// A draft someone else has not published is not there to add.
	code, raw = f.install(`{"skill":"reet:publisher/secret"}`)
	if b := decodeAdd(t, raw); code != 422 || b.Code != "missing" || b.Error != "This skill is no longer available. Refresh the list." {
		t.Fatalf("an unpublished skill: %d %s", code, raw)
	}
	if code, raw = f.install(`{"skill":"reet:gone"}`); code != 422 || decodeAdd(t, raw).Code != "missing" {
		t.Fatalf("a missing skill: %d %s", code, raw)
	}
	if jobs := f.jobs(t); len(jobs) != 0 {
		t.Fatalf("a sync job was queued: %+v", jobs)
	}
}

func TestTerminalSkillInstallRefusesAForeignSessionAnAnonymousCallerAndAViewer(t *testing.T) {
	f := installFixture(t)
	pdf := f.itemNamed(t, "pdf")
	if err := f.st.SetSkillStatusFor(f.uid, []string{pdf.ID}, store.SkillArchived, time.Now()); err != nil {
		t.Fatal(err)
	}
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
	if got := f.itemNamed(t, "pdf"); got.Status != store.SkillArchived {
		t.Fatalf("a refused call changed pdf: %+v", got)
	}
}

func TestTerminalSkillInstallRejectsABadBody(t *testing.T) {
	f := installFixture(t)
	for _, body := range []string{
		`not json`, ``, `{}`, `{"skill":""}`, `{"skill":"pdf"}`, `{"skill":"reet:"}`, `{"skill":"reet:a/"}`, `{"skill":"reet:/b"}`,
		`{"skill":"reet:a/b/c"}`, `{"skill":"reet:..\\x"}`, `{"skill":"reet:../pdf"}`, `{"skill":"reet:pdf"} {"x":1}`,
	} {
		if code, raw := f.install(body); code != 400 {
			t.Fatalf("%q got %d %s", body, code, raw)
		}
	}
	if len(f.jobs(t)) != 0 {
		t.Fatal("a bad body queued a job")
	}
}

func TestSaveDesiredAnswersAPendingInstallWithItsSentence(t *testing.T) {
	f := installFixture(t)
	f.pendingInstall(t, "pdf")
	var tg store.SyncTarget
	targets, _ := f.st.SyncTargetsFor(f.uid)
	for _, x := range targets {
		if x.Target.ID == f.env {
			tg = x
		}
	}
	body, _ := json.Marshal(map[string]any{"targetId": f.env, "sourceId": "hosted", "listId": 0, "skillIds": []string{tg.Desired[0].ID}, "mode": "additive", "revision": tg.Revision})
	r := httptest.NewRequest("PUT", "/api/v2/sync/desired", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://reet.example")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	if w.Code != 409 || w.Body.String() != `{"error":"An install is waiting on this folder. Finish or cancel it first."}` {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestEnvironmentMergeRefusesARunningInstallWith409(t *testing.T) {
	f := installFixture(t)
	f.pendingInstall(t, "pdf")
	if j, err := f.st.ClaimSyncJob(f.uid, f.agent); err != nil || j == nil {
		t.Fatal(j, err)
	}
	if err := f.st.TouchEnvironmentFor(f.uid, f.agent.Host, "claude", "/work/app", "/work/app/.claude/skills"); err != nil {
		t.Fatal(err)
	}
	dest := store.EnvironmentIDFor(f.uid, f.agent.Host, "claude", "/work/app")
	body, _ := json.Marshal(map[string]any{"from_id": f.env, "to_id": dest, "confirm": true})
	code, raw := f.post("/api/v2/environments/merge", string(body), f.cookie, "https://reet.example")
	if code != 409 || !strings.Contains(raw, `"code":"environment_job_running"`) {
		t.Fatalf("got %d %s", code, raw)
	}
	// The preview refuses the same way, before anything is confirmed.
	preview, _ := json.Marshal(map[string]any{"from_id": f.env, "to_id": dest, "dry_run": true})
	code, raw = f.post("/api/v2/environments/merge", string(preview), f.cookie, "https://reet.example")
	if code != 409 || !strings.Contains(raw, `"code":"environment_job_running"`) {
		t.Fatalf("preview got %d %s", code, raw)
	}
}

func TestEnvironmentMergePreviewCountsPendingInstalls(t *testing.T) {
	f := installFixture(t)
	f.pendingInstall(t, "pdf")
	if err := f.st.TouchEnvironmentFor(f.uid, f.agent.Host, "claude", "/work/app", "/work/app/.claude/skills"); err != nil {
		t.Fatal(err)
	}
	dest := store.EnvironmentIDFor(f.uid, f.agent.Host, "claude", "/work/app")
	body, _ := json.Marshal(map[string]any{"from_id": f.env, "to_id": dest, "dry_run": true})
	code, raw := f.post("/api/v2/environments/merge", string(body), f.cookie, "https://reet.example")
	if code != 200 || !strings.Contains(raw, `"pending_jobs_cancelled":1`) || !strings.Contains(raw, `"pending_installs_cancelled":1`) {
		t.Fatalf("got %d %s", code, raw)
	}
}

// catalogue reads the session's slash-command catalogue as the browser does.
func (f *installFx) catalogue(t *testing.T) SessionSkills {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/v2/terminal/sessions/"+f.session+"/skills", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out SessionSkills
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err, w.Body.String())
	}
	return out
}

// rescanInstall reports one installed skill the way the connector's post-apply
// rescan does for a private install: the skill is in its folder, with no
// skills-CLI lock source to name it.
func (f *installFx) rescanInstall(t *testing.T, name string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"items":        []map[string]string{{"name": name, "host": f.agent.Host, "agent": "claude"}},
		"environments": []map[string]any{{"host": f.agent.Host, "agent": "claude", "project": "", "path": ".claude/skills", "skills": []string{name}}},
	})
	r := httptest.NewRequest("POST", "/api/import", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func sessionSkillByID(skills []SessionSkill, id string) *SessionSkill {
	for i := range skills {
		if skills[i].ID == id {
			return &skills[i]
		}
	}
	return nil
}

func sessionSkillByName(skills []SessionSkill, name string) *SessionSkill {
	for i := range skills {
		if skills[i].Name == name && skills[i].Source == "machine" {
			return &skills[i]
		}
	}
	return nil
}

// The picker lists what the session's agent can use on its machine: skills reet
// knows there, then those New on this machine, each with a way to Keep it. The
// person's written skills that are not on the machine are not listed: every
// Active one is on every machine already, and Archived ones are not offered.
func TestTerminalSessionSkillsListsNewOnThisMachine(t *testing.T) {
	f := installFixture(t)
	if err := f.st.SetAgentSkillsProtocolFor(f.uid, f.agent.ID, store.CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	other, _, err := f.st.EnrollSyncAgent(f.uid, "elsewhere", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.st.SetAgentSkillsProtocolFor(f.uid, other.ID, store.CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	report := func(agent string, entries ...store.SkillReportEntry) {
		t.Helper()
		if err := f.st.RecordSkillReportFor(f.uid, agent, store.SkillReport{Version: "v1", Entries: entries}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	digest := strings.Repeat("c", 64)
	report(f.agent.ID,
		store.SkillReportEntry{Folder: "~/.claude/skills", Name: "cq", State: "unknown", Digest: digest},
		store.SkillReportEntry{Folder: "~/.cursor/skills", Name: "cursor-only", State: "unknown", Digest: digest})
	report(other.ID, store.SkillReportEntry{Folder: "~/.claude/skills", Name: "not-here", State: "unknown", Digest: digest})
	f.rescanInstall(t, "pdf")

	got := f.catalogue(t)
	for _, s := range got.Skills {
		if strings.HasPrefix(s.ID, "reet:") {
			t.Fatalf("a written skill is still offered to install: %+v", got.Skills)
		}
	}
	if s := sessionSkillByName(got.Skills, "pdf"); s == nil || !s.Native || s.Invocation != "Use the pdf skill." {
		t.Fatalf("pdf on the machine: %+v (all %+v)", s, got.Skills)
	}
	cq := sessionSkillByID(got.Skills, "new:cq")
	if cq == nil || cq.Source != "new" || cq.Name != "cq" || !cq.Native || !cq.OnMachine || cq.Agent != "claude" || cq.Invocation != "Use the cq skill." {
		t.Fatalf("cq: %+v (all %+v)", cq, got.Skills)
	}
	cursor := sessionSkillByID(got.Skills, "new:cursor-only")
	if cursor == nil || cursor.Native || cursor.Agent != "cursor" || cursor.Invocation != "" {
		t.Fatalf("cursor-only: %+v", cursor)
	}
	if s := sessionSkillByID(got.Skills, "new:not-here"); s != nil {
		t.Fatalf("a skill new on another machine is listed: %+v", s)
	}
	// An Archived skill is coming off every machine and is not offered.
	if err = f.st.SetSkillStatusFor(f.uid, []string{f.itemNamed(t, "pdf").ID}, store.SkillArchived, time.Now()); err != nil {
		t.Fatal(err)
	}
	if s := sessionSkillByName(f.catalogue(t).Skills, "pdf"); s != nil {
		t.Fatalf("an Archived skill is offered: %+v", s)
	}
}
