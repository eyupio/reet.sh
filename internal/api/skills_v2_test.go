package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/store"
)

// pageRow mirrors SkillRow in web/src/lib/api.ts.
type pageRow struct {
	ID           string            `json:"id"`
	ShelfID      string            `json:"shelfId"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Status       string            `json:"status"`
	OriginKind   string            `json:"originKind"`
	Origin       string            `json:"origin"`
	Yours        bool              `json:"yours"`
	Published    bool              `json:"published"`
	PublicURL    string            `json:"publicUrl"`
	HasCopy      bool              `json:"hasCopy"`
	Liked        bool              `json:"liked"`
	Notes        string            `json:"notes"`
	Tags         []store.Tag       `json:"tags"`
	Lists        []store.ListRef   `json:"lists"`
	Health       store.SkillHealth `json:"health"`
	UpgradedFrom string            `json:"upgradedFrom"`
	EditedOn     *struct {
		AgentID string `json:"agentId"`
		Host    string `json:"host"`
	} `json:"editedOn"`
	EditedOnProblem string `json:"editedOnProblem"`
}

// pageData mirrors SkillsPageData in web/src/lib/api.ts.
type pageData struct {
	Skills        []pageRow `json:"skills"`
	Counts        struct{ Active, Archived int }
	NewOnMachines []store.NewSkill `json:"newOnMachines"`
	Machines      int              `json:"machines"`
	Notice        *struct {
		Parked  int `json:"parked"`
		Removed int `json:"removed"`
	} `json:"notice"`
}

func (p pageData) row(t *testing.T, name string) pageRow {
	t.Helper()
	for _, r := range p.Skills {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("%s is not on the page: %+v", name, p.Skills)
	return pageRow{}
}

func (p pageData) names() []string {
	out := []string{}
	for _, r := range p.Skills {
		out = append(out, r.Name)
	}
	return out
}

// owner sends a request as the account's owner, as the web app does.
func (e reconcileEnv) owner(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	return doClient(t, e.srv.Client(), e.srv, method, path, body, "X-Reet-Token", "fixture-owner")
}

func (e reconcileEnv) page(t *testing.T, status string) pageData {
	t.Helper()
	code, raw := e.owner(t, "GET", "/api/v2/skills?status="+status, "")
	if code != 200 {
		t.Fatalf("page: %d %s", code, raw)
	}
	var p pageData
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("page: %v %s", err, raw)
	}
	return p
}

func (e reconcileEnv) settle() { e.api.s.skillCopies.Wait() }

// linkedSkill adds a skill by link, as the web app does, and returns its
// library item once reet has tried to fetch it.
func (e reconcileEnv) linkedSkill(t *testing.T, input string) string {
	t.Helper()
	if code, raw := e.owner(t, "POST", "/api/skills", `{"input":"`+input+`"}`); code != 200 {
		t.Fatalf("add %s: %d %s", input, code, raw)
	}
	e.settle()
	r, err := ref.Parse(strings.TrimPrefix(input, "https://www.skills.sh/"))
	if err != nil {
		t.Fatal(err)
	}
	items, err := e.st.LibraryFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Name == r.Slug {
			return it.ID
		}
	}
	t.Fatalf("no library item for %s: %+v", input, items)
	return ""
}

// onProtocol puts elitebook on the skills protocol and has it report entries.
func (e reconcileEnv) report(t *testing.T, agent string, entries ...store.SkillReportEntry) {
	t.Helper()
	if err := e.st.SetAgentSkillsProtocolFor(e.uid, agent, store.CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	if err := e.st.RecordSkillReportFor(e.uid, agent, store.SkillReport{Version: "v", Entries: entries}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func (e reconcileEnv) removals(t *testing.T) map[string]string {
	t.Helper()
	list, err := e.st.SkillRemovalsFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, r := range list {
		out[r.Name] = r.Reason
	}
	return out
}

func (e reconcileEnv) item(t *testing.T, id string) store.SkillItemRef {
	t.Helper()
	items, err := e.st.SkillItemsFor(e.uid, []string{id})
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return items[0]
}

func (e reconcileEnv) live(t *testing.T, id string) bool {
	t.Helper()
	_, err := e.st.SkillItemsFor(e.uid, []string{id})
	if err != nil && !errors.Is(err, store.ErrPrivateSkillMissing) {
		t.Fatal(err)
	}
	return err == nil
}

// db opens the store's database beside it, for fixtures no store call makes.
func (e reconcileEnv) db(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(e.st.Dir(), "reet.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSkillsPageListsByStatus(t *testing.T) {
	e := newReconcileEnv(t)
	written := e.writtenSkill(t, "alpha")
	archived := e.writtenSkill(t, "beta")
	linked := e.linkedSkill(t, findSkills)
	if code, raw := e.owner(t, "PATCH", "/api/skill", `{"id":"vercel-labs/skills/find-skills","liked":true,"notes":"keep me"}`); code != 200 {
		t.Fatalf("like: %d %s", code, raw)
	}
	if code, raw := e.owner(t, "POST", "/api/skill/tags", `{"id":"vercel-labs/skills/find-skills","add":["research"],"remove":[]}`); code != 200 {
		t.Fatalf("tag: %d %s", code, raw)
	}
	if err := e.st.SetSkillStatusFor(e.uid, []string{archived}, store.SkillArchived, time.Now()); err != nil {
		t.Fatal(err)
	}
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now().Add(store.SkillGrace))
	if err != nil {
		t.Fatal(err)
	}
	var entries []store.SkillReportEntry
	for _, a := range d.Active {
		entries = append(entries, store.SkillReportEntry{Folder: "~/.agents/skills", Name: a.Name, State: "installed", Digest: a.Digest})
	}
	entries = append(entries, store.SkillReportEntry{Folder: "~/.agents/skills", Name: "cq", State: "unknown", Digest: "d1"})
	e.report(t, e.agent.ID, entries...)

	active := e.page(t, "active")
	if got := active.names(); !slices.Equal(got, []string{"alpha", "find-skills"}) {
		t.Fatalf("active: %v", got)
	}
	if active.Counts.Active != 2 || active.Counts.Archived != 1 || active.Machines != 1 || active.Notice != nil {
		t.Fatalf("page: %+v", active)
	}
	if len(active.NewOnMachines) != 1 || active.NewOnMachines[0].Name != "cq" || active.NewOnMachines[0].Copies[0].Host != "elitebook" {
		t.Fatalf("new on machines: %+v", active.NewOnMachines)
	}
	a := active.row(t, "alpha")
	if a.ID != written || a.Status != "active" || a.OriginKind != "written" || !a.Yours || !a.HasCopy || a.Origin != "" || a.Description != "Written in reet" {
		t.Fatalf("written row: %+v", a)
	}
	if a.Health.State != "ok" || a.Health.Total != 1 || len(a.Health.Machines) != 1 || a.Health.Machines[0].Host != "elitebook" {
		t.Fatalf("written health: %+v", a.Health)
	}
	f := active.row(t, "find-skills")
	if f.ID != linked || f.ShelfID != "vercel-labs/skills/find-skills" || f.OriginKind != "imported" || f.Yours || f.Origin != findSkillsOrigin || !f.HasCopy {
		t.Fatalf("linked row: %+v", f)
	}
	if !f.Liked || f.Notes != "keep me" || len(f.Tags) != 1 || f.Tags[0].Name != "research" || f.Lists == nil || f.Description != "Finds skills" {
		t.Fatalf("linked row's shelf: %+v", f)
	}
	archivedTab := e.page(t, "archived")
	if got := archivedTab.names(); !slices.Equal(got, []string{"beta"}) || archivedTab.Skills[0].Status != "archived" || archivedTab.Counts != active.Counts {
		t.Fatalf("archived: %+v", archivedTab)
	}
	// The JSON names are the web app's.
	_, raw := e.owner(t, "GET", "/api/v2/skills?status=active", "")
	for _, key := range []string{`"shelfId":`, `"originKind":"imported"`, `"hasCopy":true`, `"newOnMachines":[`, `"counts":{"active":2,"archived":1}`, `"machines":1`, `"notice":null`, `"digestsDiffer":false`} {
		if !strings.Contains(raw, key) {
			t.Fatalf("%s missing from %s", key, raw)
		}
	}
	if code, raw := e.owner(t, "GET", "/api/v2/skills?status=parked", ""); code != 400 {
		t.Fatalf("a status that is not a tab: %d %s", code, raw)
	}
	if code, raw := e.owner(t, "GET", "/api/v2/skills", ""); code != 200 || !strings.Contains(raw, `"name":"alpha"`) {
		t.Fatalf("no status is the Active tab: %d %s", code, raw)
	}
}

func TestSkillsPageBulkStatus(t *testing.T) {
	e := newReconcileEnv(t)
	a, b := e.writtenSkill(t, "alpha"), e.writtenSkill(t, "beta")
	c := e.writtenSkill(t, "gamma")
	code, raw := e.owner(t, "POST", "/api/v2/skills/status", `{"ids":["`+a+`","`+b+`"],"status":"archived"}`)
	if code != 200 {
		t.Fatalf("archive: %d %s", code, raw)
	}
	var p pageData
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	if got := p.names(); !slices.Equal(got, []string{"alpha", "beta"}) || p.Counts.Active != 1 || p.Counts.Archived != 2 {
		t.Fatalf("the reply is the Archived tab: %+v", p)
	}
	if e.item(t, a).Status != "archived" || e.item(t, b).Status != "archived" || e.item(t, c).Status != "active" {
		t.Fatal("archive did not reach the store")
	}
	if r := e.removals(t); r["alpha"] != "archived" || r["beta"] != "archived" || r["gamma"] != "" {
		t.Fatalf("removals: %v", r)
	}
	// With tab, the answer is that tab, so pruning never switches tabs.
	code, raw = e.owner(t, "POST", "/api/v2/skills/status", `{"ids":["`+b+`"],"status":"active","tab":"archived"}`)
	if err := json.Unmarshal([]byte(raw), &p); err != nil || code != 200 || !slices.Equal(p.names(), []string{"alpha"}) || p.Counts.Active != 2 || p.Counts.Archived != 1 {
		t.Fatalf("restore from the Archived tab: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/status", `{"ids":["`+b+`"],"status":"archived","tab":"parked"}`); code != 400 {
		t.Fatalf("a tab that is not one: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/status", `{"ids":["`+b+`"],"status":"archived","tab":"active"}`); code != 200 || !strings.Contains(raw, `"name":"gamma"`) || strings.Contains(raw, `"name":"alpha"`) {
		t.Fatalf("archive from the Active tab: %d %s", code, raw)
	}
	code, raw = e.owner(t, "POST", "/api/v2/skills/status", `{"ids":["`+a+`"],"status":"active"}`)
	if code != 200 || !strings.Contains(raw, `"name":"alpha"`) || strings.Contains(raw, `"name":"beta"`) {
		t.Fatalf("restore: %d %s", code, raw)
	}
	if e.item(t, a).Status != "active" || e.removals(t)["alpha"] != "" {
		t.Fatal("restore did not reach the store")
	}
	// Nothing changes when one id is not a skill of the account.
	if code, raw = e.owner(t, "POST", "/api/v2/skills/status", `{"ids":["`+c+`","0123456789abcdef0123456789abcdef"],"status":"archived"}`); code != 404 {
		t.Fatalf("unknown id: %d %s", code, raw)
	}
	if e.item(t, c).Status != "active" {
		t.Fatal("a refused request changed a skill")
	}
	many := make([]string, 1001)
	for i := range many {
		many[i] = "0123456789abcdef0123456789abcdef"[:31] + string(rune('a'+i%6))
	}
	body, _ := json.Marshal(map[string]any{"ids": many, "status": "archived"})
	for _, bad := range []string{
		`{"ids":["` + a + `"],"status":"parked"}`,
		`{"ids":[],"status":"archived"}`,
		`{"ids":["` + a + `"],"status":"archived","extra":1}`,
		string(body),
	} {
		if code, raw = e.owner(t, "POST", "/api/v2/skills/status", bad); code != 400 {
			t.Fatalf("%.80s: %d %s", bad, code, raw)
		}
	}
}

func TestSkillsPageDeleteNeedsConfirmWithoutCopy(t *testing.T) {
	e := newReconcileEnv(t)
	kept := e.linkedSkill(t, "a/b/alpha")
	e.fetch.set("", errors.New("offline"))
	empty := e.linkedSkill(t, findSkills)
	if e.item(t, empty).HasCopy {
		t.Fatal("fixture: the linked skill has a copy")
	}
	code, raw := e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["`+kept+`","`+empty+`"]}`)
	if code != 409 {
		t.Fatalf("delete without a copy: %d %s", code, raw)
	}
	var refusal struct {
		Code  string   `json:"code"`
		Names []string `json:"names"`
		Error string   `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &refusal); err != nil || refusal.Code != "no-copy" || !slices.Equal(refusal.Names, []string{"find-skills"}) || refusal.Error == "" {
		t.Fatalf("refusal: %s", raw)
	}
	if !e.live(t, kept) || !e.live(t, empty) {
		t.Fatal("a refused delete deleted something")
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["`+kept+`","`+empty+`"],"noCopyConfirmed":true}`); code != 200 {
		t.Fatalf("confirmed delete: %d %s", code, raw)
	}
	if e.live(t, kept) || e.live(t, empty) {
		t.Fatal("confirmed delete left a skill")
	}
	if r := e.removals(t); r["alpha"] != "deleted" || r["find-skills"] != "deleted" {
		t.Fatalf("removals: %v", r)
	}
	e.fetch.set("v1", nil)
	other := e.linkedSkill(t, "a/b/beta")
	if code, raw = e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["`+other+`"]}`); code != 200 || e.live(t, other) {
		t.Fatalf("a skill reet has a copy of needs no confirmation: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["0123456789abcdef0123456789abcdef"]}`); code != 404 {
		t.Fatalf("unknown id: %d %s", code, raw)
	}
}

func TestSkillsPageUndelete(t *testing.T) {
	e := newReconcileEnv(t)
	a := e.writtenSkill(t, "alpha")
	if code, raw := e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["`+a+`"],"writtenConfirmed":true}`); code != 200 {
		t.Fatalf("delete: %d %s", code, raw)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/undelete", `{"ids":["`+a+`"]}`); code != 200 {
		t.Fatalf("undelete: %d %s", code, raw)
	}
	if it := e.item(t, a); it.Status != "active" || e.removals(t)["alpha"] != "" {
		t.Fatalf("undelete did not bring it back: %+v %v", it, e.removals(t))
	}
	if p := e.page(t, "active"); !slices.Equal(p.names(), []string{"alpha"}) {
		t.Fatalf("page: %v", p.names())
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/undelete", `{"ids":["0123456789abcdef0123456789abcdef"]}`); code != 404 {
		t.Fatalf("unknown id: %d %s", code, raw)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/undelete", `{"ids":[]}`); code != 400 {
		t.Fatalf("no ids: %d %s", code, raw)
	}
}

func TestSkillsPageNewKeepAndArchive(t *testing.T) {
	e := newReconcileEnv(t)
	studio, _, err := e.st.EnrollSyncAgent(e.uid, "studio", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.ClaimRemoteImportFor(e.uid, e.agent.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	e.report(t, e.agent.ID,
		store.SkillReportEntry{Folder: "~/.agents/skills", Name: "cq", State: "unknown", Digest: "d1"},
		store.SkillReportEntry{Folder: "~/.agents/skills", Name: "paseo-help", State: "unknown", Digest: "d2"})
	e.report(t, studio.ID, store.SkillReportEntry{Folder: "~/.claude/skills", Name: "cq", State: "unknown", Digest: "d3"})
	if p := e.page(t, "active"); len(p.NewOnMachines) != 2 || !p.NewOnMachines[0].DigestsDiffer {
		t.Fatalf("new on machines: %+v", p.NewOnMachines)
	}

	// Keep of copies that differ asks which machine's copy to keep, and Keep
	// all decides none of them until it can decide all.
	code, raw := e.owner(t, "POST", "/api/v2/skills/new", `{"names":["paseo-help","cq"],"action":"keep"}`)
	if code != 409 || !strings.Contains(raw, `"code":"choose-copy"`) || !strings.Contains(raw, `"names":["cq"]`) {
		t.Fatalf("keep without a choice: %d %s", code, raw)
	}
	if p := e.page(t, "active"); len(p.NewOnMachines) != 2 || p.Counts.Active != 0 {
		t.Fatalf("a refused Keep all kept some: %+v", p)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/new", `{"names":["cq"],"action":"keep","from":"`+e.agent.ID+`"}`); code != 200 {
		t.Fatalf("keep: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/new", `{"names":["paseo-help"],"action":"archive"}`); code != 200 {
		t.Fatalf("archive: %d %s", code, raw)
	}
	e.settle()
	lib := map[string]store.LibraryItem{}
	items, err := e.st.LibraryFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if _, dup := lib[strings.ToLower(it.Name)]; dup {
			t.Fatalf("two items named %s", it.Name)
		}
		lib[strings.ToLower(it.Name)] = it
	}
	if lib["cq"].Status != "active" || lib["paseo-help"].Status != "archived" {
		t.Fatalf("library: %+v", lib)
	}
	if origin, reference, err := e.st.SkillOriginFor(e.uid, lib["cq"].ID); err != nil || origin != "machine" || reference != "machine:elitebook:~/.agents/skills" {
		t.Fatalf("kept from: %s %s %v", origin, reference, err)
	}
	// No copy yet, so the archived name is not on the removal list.
	if r := e.removals(t); r["paseo-help"] != "" {
		t.Fatalf("removals before the copy landed: %v", r)
	}
	requests, _, err := e.st.RemoteImportsFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	asked := map[string]string{}
	for _, r := range requests {
		asked[r.SyncSkillID] = r.AgentID
	}
	if asked["local/cq"] != e.agent.ID || asked["local/paseo-help"] != e.agent.ID {
		t.Fatalf("captures: %+v", requests)
	}
	active := e.page(t, "active")
	if len(active.NewOnMachines) != 0 || active.Counts.Active != 1 || active.Counts.Archived != 1 {
		t.Fatalf("page after: %+v", active)
	}
	if r := active.row(t, "cq"); r.OriginKind != "kept" || r.Origin != "elitebook" || r.HasCopy || r.Yours {
		t.Fatalf("kept row: %+v", r)
	}
	// cq is an Active skill now, though elitebook still reports it unknown
	// until its next report: it is not new, and neither Keep nor Archive
	// touches it.
	for _, action := range []string{"keep", "archive"} {
		code, raw = e.owner(t, "POST", "/api/v2/skills/new", `{"names":["CQ"],"action":"`+action+`","from":"`+e.agent.ID+`"}`)
		if code != 409 || !strings.Contains(raw, `"code":"not-new"`) {
			t.Fatalf("%s of an Active skill: %d %s", action, code, raw)
		}
	}
	if p := e.page(t, "active"); p.Counts.Active != 1 || p.Counts.Archived != 1 || p.row(t, "cq").Status != "active" {
		t.Fatalf("a refused decision changed the library: %+v", p)
	}
	// paseo-help is Archived with no copy yet, so machines still report it:
	// Keep joins it rather than making a second item.
	if code, raw = e.owner(t, "POST", "/api/v2/skills/new", `{"names":["paseo-help"],"action":"keep","from":"no-such-machine"}`); code != 400 {
		t.Fatalf("keep from a machine without a copy: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/new", `{"names":["paseo-help"],"action":"keep"}`); code != 200 {
		t.Fatalf("keep of an archived skill without a copy: %d %s", code, raw)
	}
	if p := e.page(t, "active"); p.Counts.Active != 2 || p.Counts.Archived != 0 || p.row(t, "paseo-help").ID != lib["paseo-help"].ID {
		t.Fatalf("keep did not join paseo-help: %+v", p)
	}
	for body, want := range map[string]int{
		`{"names":["never-seen"],"action":"keep"}`:      409,
		`{"names":["cq"],"action":"park"}`:              400,
		`{"names":[],"action":"keep"}`:                  400,
		`{"names":["cq"],"action":"keep","extra":true}`: 400,
	} {
		if code, raw = e.owner(t, "POST", "/api/v2/skills/new", body); code != want {
			t.Fatalf("%s: %d %s", body, code, raw)
		}
	}
}

func TestSkillsPageNoticeDismissAndRestoreParked(t *testing.T) {
	e := newReconcileEnv(t)
	parked := e.writtenSkill(t, "parked-one")
	db := e.db(t)
	if _, err := db.Exec(`UPDATE library_items SET status='archived',upgraded_from='parked' WHERE user_id=? AND id=?`, e.uid, parked); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO skill_notices(user_id,parked,removed) VALUES(?,1,2)`, e.uid); err != nil {
		t.Fatal(err)
	}
	p := e.page(t, "archived")
	if p.Notice == nil || p.Notice.Parked != 1 || p.Notice.Removed != 2 || p.row(t, "parked-one").UpgradedFrom != "parked" {
		t.Fatalf("notice: %+v %+v", p.Notice, p.Skills)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/notice", `{"action":"restore-parked"}`); code != 200 {
		t.Fatalf("restore parked: %d %s", code, raw)
	}
	if e.item(t, parked).Status != "active" {
		t.Fatal("the parked skill is not Active")
	}
	if p = e.page(t, "active"); p.Notice != nil {
		t.Fatalf("restoring the parked ones dismisses the notice: %+v", p.Notice)
	}
	if _, err := db.Exec(`UPDATE skill_notices SET dismissed=0 WHERE user_id=?`, e.uid); err != nil {
		t.Fatal(err)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/notice", `{"action":"dismiss"}`); code != 200 {
		t.Fatalf("dismiss: %d %s", code, raw)
	}
	if p = e.page(t, "archived"); p.Notice != nil {
		t.Fatalf("dismissed notice: %+v", p.Notice)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/notice", `{"action":"later"}`); code != 400 {
		t.Fatalf("unknown action: %d %s", code, raw)
	}
}

func TestSkillsPageReadOnlyRefused(t *testing.T) {
	e := newReconcileEnv(t)
	a := e.writtenSkill(t, "alpha")
	hash, _ := auth.HashPassword("long-enough-password")
	viewer, err := e.st.CreateUser("viewer@example.com", "Viewer", "viewer", hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.CreateAPIToken(viewer.ID, "viewer", auth.HashToken("viewer-token")); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		"/api/v2/skills/status":                `{"ids":["` + a + `"],"status":"archived"}`,
		"/api/v2/skills/delete":                `{"ids":["` + a + `"],"noCopyConfirmed":true}`,
		"/api/v2/skills/undelete":              `{"ids":["` + a + `"]}`,
		"/api/v2/skills/retry":                 `{"items":["` + a + `"]}`,
		"/api/v2/skills/new":                   `{"names":["cq"],"action":"keep"}`,
		"/api/v2/skills/notice":                `{"action":"dismiss"}`,
		"/api/v2/skills/" + a + "/use-version": `{"agentId":"` + e.agent.ID + `"}`,
		"/api/v2/skills/" + a + "/original":    `{"digest":"x","revision":"y"}`,
	} {
		code, raw := doClient(t, e.srv.Client(), e.srv, "POST", path, body, "X-Reet-Token", "viewer-token")
		if code != 403 {
			t.Fatalf("%s as a viewer: %d %s", path, code, raw)
		}
	}
	if !e.live(t, a) || e.item(t, a).Status != "active" {
		t.Fatal("a viewer changed a skill")
	}
}

// editedOn has elitebook report that it backed up its edit of name and put
// reet's copy back, and returns the backup's path.
func (e reconcileEnv) editedOn(t *testing.T, name, backup string) {
	t.Helper()
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now().Add(store.SkillGrace))
	if err != nil {
		t.Fatal(err)
	}
	digest := ""
	for _, a := range d.Active {
		if a.Name == name {
			digest = a.Digest
		}
	}
	e.report(t, e.agent.ID, store.SkillReportEntry{Folder: "~/.agents/skills", Name: name, State: "edited", Digest: digest, Backup: backup})
}

func TestUseVersionAsksForTheBackup(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.writtenSkill(t, "demo")
	backup := "~/.agents/.reet-backups/2026-11-18/agents-demo"
	e.editedOn(t, "demo", backup)
	if r := e.page(t, "active").row(t, "demo"); r.EditedOn == nil || r.EditedOn.AgentID != e.agent.ID || r.EditedOn.Host != "elitebook" {
		t.Fatalf("edited on: %+v", r.EditedOn)
	}
	if _, err := e.st.ClaimRemoteImportFor(e.uid, e.agent.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	code, raw := e.owner(t, "POST", "/api/v2/skills/"+item+"/use-version", `{"agentId":"`+e.agent.ID+`"}`)
	if code != 202 {
		t.Fatalf("use version: %d %s", code, raw)
	}
	requests, _, err := e.st.RemoteImportsFor(e.uid)
	if err != nil || len(requests) != 1 {
		t.Fatalf("requests: %+v %v", requests, err)
	}
	r := requests[0]
	if r.BackupPath != backup || r.UseVersionItem != item || r.AgentID != e.agent.ID || r.Agent != "agents" || r.Project != "" || r.BundleID == "" || r.ExpectedBase == "" || r.SyncSkillID != "" {
		t.Fatalf("the request must carry the backup: %+v", r)
	}
	// Pressing again while it runs asks nothing more.
	if code, raw = e.owner(t, "POST", "/api/v2/skills/"+item+"/use-version", `{"agentId":"`+e.agent.ID+`"}`); code != 202 {
		t.Fatalf("again: %d %s", code, raw)
	}
	if again, _, _ := e.st.RemoteImportsFor(e.uid); len(again) != 1 {
		t.Fatalf("asked twice: %+v", again)
	}
}

func TestUseVersionRefusesABackupOutsideTheBackupFolders(t *testing.T) {
	for _, backup := range []string{
		"~/.agents/skills/demo",
		"~/.agents/.reet-backups/2026-11-18/../../skills/demo",
		"~/.agents/.reet-backups/not-a-date/agents-demo",
		"~/.codex/.reet-backups/2026-11-18/codex-demo",
		"~/.agents/.reet-backups/2026-11-18",
		"/etc/.reet-backups/2026-11-18/demo",
		"~/projects/app/.agents/.reet-backups/2026-11-18/agents-demo",
		"~/.agents/.reet-sync-backups/kept-1/demo",
	} {
		t.Run(backup, func(t *testing.T) {
			e := newReconcileEnv(t)
			item := e.writtenSkill(t, "demo")
			e.editedOn(t, "demo", backup)
			if _, err := e.st.ClaimRemoteImportFor(e.uid, e.agent.ID, strings.Repeat("a", 48)); err != nil {
				t.Fatal(err)
			}
			code, raw := e.owner(t, "POST", "/api/v2/skills/"+item+"/use-version", `{"agentId":"`+e.agent.ID+`"}`)
			if code != 409 {
				t.Fatalf("%d %s", code, raw)
			}
			if requests, _, _ := e.st.RemoteImportsFor(e.uid); len(requests) != 0 {
				t.Fatalf("asked for %+v", requests)
			}
		})
	}
}

func TestUseVersionWithoutABackupSaysWhy(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.writtenSkill(t, "demo")
	code, raw := e.owner(t, "POST", "/api/v2/skills/"+item+"/use-version", `{"agentId":"`+e.agent.ID+`"}`)
	if code != 404 || !strings.Contains(raw, "elitebook") || !strings.Contains(raw, "demo") {
		t.Fatalf("no backup: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/"+item+"/use-version", `{"agentId":"someone-elses"}`); code != 404 {
		t.Fatalf("unknown machine: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/0123456789abcdef0123456789abcdef/use-version", `{"agentId":"`+e.agent.ID+`"}`); code != 404 {
		t.Fatalf("unknown skill: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/"+item+"/use-version", `{"agentId":""}`); code != 400 {
		t.Fatalf("no machine: %d %s", code, raw)
	}
}

// The files a machine sends from its backup become a revision of kind
// use-version, the skill's content, which the desired state then carries.
func TestUseVersionLandsAsARevision(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.writtenSkill(t, "demo")
	backup := "~/.agents/.reet-backups/2026-11-18/agents-demo"
	e.editedOn(t, "demo", backup)
	instance := strings.Repeat("a", 48)
	if _, err := e.st.ClaimRemoteImportFor(e.uid, e.agent.ID, instance); err != nil {
		t.Fatal(err)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/"+item+"/use-version", `{"agentId":"`+e.agent.ID+`"}`); code != 202 {
		t.Fatalf("use version: %d %s", code, raw)
	}
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Edited on elitebook\n---\nBetter body.\n")}}
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	// As the machine: preview, then send the files.
	work, err := e.st.ClaimRemoteImportFor(e.uid, e.agent.ID, instance)
	if err != nil || work == nil || work.State != "previewing" || work.BackupPath != backup {
		t.Fatalf("work: %+v %v", work, err)
	}
	preview := skillimport.Preview{SchemaVersion: skillimport.RemoteVersion, SourceDigest: m.Digest, SkillValid: true, Files: []skillimport.PreviewFile{}}
	for _, f := range m.Files {
		preview.Files = append(preview.Files, skillimport.PreviewFile{File: f})
	}
	if err = e.st.RecordRemotePreviewFor(e.uid, e.agent.ID, work.ID, instance, preview); err != nil {
		t.Fatal(err)
	}
	if work, err = e.st.ClaimRemoteImportFor(e.uid, e.agent.ID, instance); err != nil || work == nil || work.State != "uploading" {
		t.Fatalf("upload: %+v %v", work, err)
	}
	result, err := e.st.CommitRemoteImportFor(e.uid, e.agent.ID, work.ID, instance, m.Digest, files)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := e.st.PrivateSkillRevisionFor(e.uid, result.Bundle.ID, result.Revision.ID)
	if err != nil || rev.Origin.Kind != "use-version" || !strings.Contains(rev.Origin.Reference, backup) {
		t.Fatalf("revision: %+v %v", rev.Origin, err)
	}
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now())
	if err != nil || len(d.Active) != 1 || d.Active[0].Item != item || d.Active[0].Digest != m.Digest {
		t.Fatalf("desired: %+v %v", d, err)
	}
	if r := e.page(t, "active").row(t, "demo"); r.EditedOn != nil {
		t.Fatalf("the edit is reet's copy now, and no longer offered: %+v", r.EditedOn)
	}
}

// The old shelf Remove deletes the library skill: it leaves every machine and
// its name is free again.
func TestOldRemoveDeletesTheLibrarySkill(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.linkedSkill(t, findSkills)
	if code, raw := e.owner(t, "DELETE", "/api/skill?id=vercel-labs%2Fskills%2Ffind-skills", ""); code != 204 {
		t.Fatalf("remove: %d %s", code, raw)
	}
	if e.live(t, item) || e.removals(t)["find-skills"] != "deleted" {
		t.Fatalf("live=%t removals=%v", e.live(t, item), e.removals(t))
	}
	if l := e.page(t, "active"); len(l.Skills) != 0 {
		t.Fatalf("page: %+v", l.Skills)
	}
	if _, raw := e.owner(t, "GET", "/api/skills", ""); strings.Contains(raw, "find-skills") {
		t.Fatalf("the old page still lists it: %s", raw)
	}
	// The name is free: another source of it can be added.
	if code, raw := e.owner(t, "POST", "/api/skills", `{"input":"other-owner/skills/find-skills"}`); code != 200 {
		t.Fatalf("add the name again: %d %s", code, raw)
	}
}

func TestOldRemoveWithoutCopyNeedsConfirm(t *testing.T) {
	e := newReconcileEnv(t)
	e.fetch.set("", errors.New("offline"))
	one := e.linkedSkill(t, "a/b/one")
	two := e.linkedSkill(t, "a/b/two")
	code, raw := e.owner(t, "DELETE", "/api/skill?id=a%2Fb%2Fone", "")
	if code != 409 || !strings.Contains(raw, `"code":"no-copy"`) || !strings.Contains(raw, `"names":["one"]`) || !e.live(t, one) {
		t.Fatalf("remove without a copy: %d %s", code, raw)
	}
	// It says plainly that nothing was removed, and how to confirm.
	var refusal struct{ Error, Hint string }
	if err := json.Unmarshal([]byte(raw), &refusal); err != nil || refusal.Error != "reet has no copy of one, so it was not removed. Removing it deletes it from every machine, which keeps a backup for 30 days." || !strings.Contains(refusal.Hint, "noCopyConfirmed") {
		t.Fatalf("refusal: %s", raw)
	}
	if code, raw = e.owner(t, "DELETE", "/api/skill?id=a%2Fb%2Fone&noCopyConfirmed=1", ""); code != 204 || e.live(t, one) {
		t.Fatalf("confirmed remove: %d %s", code, raw)
	}
	code, raw = e.owner(t, "POST", "/api/skills/remove", `{"ids":["a/b/two"],"ignore":true}`)
	if code != 409 || !strings.Contains(raw, `"names":["two"]`) || !e.live(t, two) {
		t.Fatalf("bulk remove without a copy: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/skills/remove", `{"ids":["a/b/two"],"ignore":true,"noCopyConfirmed":true}`); code != 200 || !strings.Contains(raw, `"removed":1`) || e.live(t, two) {
		t.Fatalf("confirmed bulk remove: %d %s", code, raw)
	}
	if _, raw = e.owner(t, "GET", "/api/ignored", ""); !strings.Contains(raw, "a/b/two") {
		t.Fatalf("ignored: %s", raw)
	}
}

// The old page's Undo of Remove puts the shelf row back, and with it the
// library skill.
func TestOldRemoveUndoBringsTheSkillBack(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.linkedSkill(t, findSkills)
	_, raw := e.owner(t, "GET", "/api/skills", "")
	var listed struct {
		Skills []store.Skill `json:"skills"`
	}
	if err := json.Unmarshal([]byte(raw), &listed); err != nil || len(listed.Skills) != 1 {
		t.Fatalf("list: %v %s", err, raw)
	}
	if code, raw := e.owner(t, "DELETE", "/api/skill?id=vercel-labs%2Fskills%2Ffind-skills", ""); code != 204 {
		t.Fatalf("remove: %d %s", code, raw)
	}
	snapshot, _ := json.Marshal(map[string]any{"version": 2, "skills": listed.Skills})
	if code, raw := e.owner(t, "POST", "/api/restore", string(snapshot)); code != 200 {
		t.Fatalf("undo: %d %s", code, raw)
	}
	if !e.live(t, item) || e.removals(t)["find-skills"] != "" {
		t.Fatalf("undo left it deleted: live=%t removals=%v", e.live(t, item), e.removals(t))
	}
}

// Putting a shelf row back, as the old page's Undo and a restore of a backup
// do, leaves the library skill it stands for as it was: its content and
// status are kept.
func TestRestoreKeepsTheLibrarySkillsContent(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.linkedSkill(t, findSkills)
	if err := e.st.SetSkillStatusFor(e.uid, []string{item}, store.SkillArchived, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, raw := e.owner(t, "GET", "/api/skills", "")
	var listed struct {
		Skills []store.Skill `json:"skills"`
	}
	if err := json.Unmarshal([]byte(raw), &listed); err != nil || len(listed.Skills) != 1 {
		t.Fatalf("list: %v %s", err, raw)
	}
	snapshot, _ := json.Marshal(map[string]any{"version": 2, "skills": listed.Skills})
	if code, raw := e.owner(t, "POST", "/api/restore", string(snapshot)); code != 200 {
		t.Fatalf("restore: %d %s", code, raw)
	}
	if it := e.item(t, item); !it.HasCopy || it.Status != "archived" {
		t.Fatalf("restore reset the library skill: %+v", it)
	}
}

// A skill the person wrote shows its public link once published. The old
// Remove of its shelf row refuses it and changes nothing, nor does the
// bulk Remove: the writing is deleted from Write a skill.
func TestOldRemoveRefusesAWrittenSkill(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.writtenSkill(t, "demo")
	body := `{"content":"---\nname: demo\ndescription: Written in reet\n---\nBody.\n","published":true}`
	if code, raw := e.owner(t, "PUT", "/api/v2/hosting/skills/demo", body); code != 200 {
		t.Fatalf("publish: %d %s", code, raw)
	}
	row := e.page(t, "active").row(t, "demo")
	if row.ID != item || !row.Published || row.PublicURL != e.srv.URL+"/writer/demo" || row.ShelfID == "" || !row.Yours {
		t.Fatalf("published row: %+v", row)
	}
	if code, raw := e.owner(t, "PATCH", "/api/skill", `{"id":"`+row.ShelfID+`","liked":true,"notes":"mine"}`); code != 200 {
		t.Fatalf("like: %d %s", code, raw)
	}
	for _, try := range []struct{ method, path, body string }{
		{"DELETE", "/api/skill?id=" + url.QueryEscape(row.ShelfID) + "&noCopyConfirmed=1", ""},
		{"POST", "/api/skills/remove", `{"ids":["` + row.ShelfID + `"],"noCopyConfirmed":true}`},
	} {
		code, raw := e.owner(t, try.method, try.path, try.body)
		var refusal struct {
			Code, Error string
			Names       []string
		}
		if err := json.Unmarshal([]byte(raw), &refusal); err != nil || code != 409 || refusal.Code != "written" || !slices.Equal(refusal.Names, []string{"demo"}) || refusal.Error != "You wrote demo. Delete it from Write a skill." {
			t.Fatalf("%s %s: %d %s", try.method, try.path, code, raw)
		}
	}
	if !e.live(t, item) || e.removals(t)["demo"] != "" {
		t.Fatal("a refused remove changed the library skill")
	}
	if r := e.page(t, "active").row(t, "demo"); r.ShelfID != row.ShelfID || !r.Liked || r.Notes != "mine" {
		t.Fatalf("a refused remove changed the shelf row: %+v", r)
	}
}

// Delete asks once for a skill the person wrote, on the server too.
func TestSkillsPageDeleteAsksOnceForWrittenSkills(t *testing.T) {
	e := newReconcileEnv(t)
	written := e.writtenSkill(t, "alpha")
	linked := e.linkedSkill(t, findSkills)
	code, raw := e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["`+written+`","`+linked+`"]}`)
	if code != 409 || !strings.Contains(raw, `"code":"written"`) || !strings.Contains(raw, `"names":["alpha"]`) || !strings.Contains(raw, "You wrote alpha.") {
		t.Fatalf("delete of a written skill: %d %s", code, raw)
	}
	if !e.live(t, written) || !e.live(t, linked) {
		t.Fatal("a refused delete deleted something")
	}
	if code, raw = e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["`+written+`","`+linked+`"],"writtenConfirmed":true}`); code != 200 {
		t.Fatalf("confirmed delete: %d %s", code, raw)
	}
	if e.live(t, written) || e.live(t, linked) {
		t.Fatal("confirmed delete left a skill")
	}
}

// Ignore of an ID that is not on the shelf still puts it on the ignore
// list, as the old Remove did.
func TestOldRemoveIgnoresAnIDNotOnTheShelf(t *testing.T) {
	e := newReconcileEnv(t)
	if code, raw := e.owner(t, "DELETE", "/api/skill?id=a%2Fb%2Fnever-added&ignore=1", ""); code != 204 {
		t.Fatalf("ignore: %d %s", code, raw)
	}
	if _, raw := e.owner(t, "GET", "/api/ignored", ""); !strings.Contains(raw, "a/b/never-added") {
		t.Fatalf("ignored: %s", raw)
	}
}

// Use this version is offered only for a backup the controller would take,
// and says why when the machine's edited copy cannot be copied in.
func TestSkillsPageEditedOnOffersOnlyUsableBackups(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.writtenSkill(t, "demo")
	e.editedOn(t, "demo", "~/.agents/skills/demo")
	if r := e.page(t, "active").row(t, "demo"); r.EditedOn != nil {
		t.Fatalf("a backup the controller refuses is offered: %+v", r.EditedOn)
	}
	backup := "~/.agents/.reet-backups/2026-11-18/agents-demo"
	e.editedOn(t, "demo", backup)
	if r := e.page(t, "active").row(t, "demo"); r.EditedOn == nil || r.EditedOn.AgentID != e.agent.ID || r.EditedOnProblem != "" {
		t.Fatalf("a usable backup is not offered: %+v %q", r.EditedOn, r.EditedOnProblem)
	}
	instance := strings.Repeat("a", 48)
	if _, err := e.st.ClaimRemoteImportFor(e.uid, e.agent.ID, instance); err != nil {
		t.Fatal(err)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/skills/"+item+"/use-version", `{"agentId":"`+e.agent.ID+`"}`); code != 202 {
		t.Fatalf("use version: %d %s", code, raw)
	}
	// The machine's preview names a file reet won't copy, so it waits.
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Edited\n---\nBody.\n")}, {Path: ".env", Data: []byte("TOKEN=x")}}
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	work, err := e.st.ClaimRemoteImportFor(e.uid, e.agent.ID, instance)
	if err != nil || work == nil {
		t.Fatalf("work: %+v %v", work, err)
	}
	preview := skillimport.Preview{SchemaVersion: skillimport.RemoteVersion, SourceDigest: m.Digest, SkillValid: true}
	for _, f := range m.Files {
		for _, c := range files {
			if c.Path == f.Path {
				preview.Files = append(preview.Files, skillimport.PreviewFile{File: f, Blocked: skillimport.BlockedReason(c)})
			}
		}
	}
	if err = e.st.RecordRemotePreviewFor(e.uid, e.agent.ID, work.ID, instance, preview); err != nil {
		t.Fatal(err)
	}
	r := e.page(t, "active").row(t, "demo")
	if r.EditedOn == nil || r.EditedOnProblem != "The edited copy on elitebook has a file reet won't copy: .env (credential or configuration path)." {
		t.Fatalf("problem: %+v %q", r.EditedOn, r.EditedOnProblem)
	}
}

func TestUseVersionRefusesALongItem(t *testing.T) {
	e := newReconcileEnv(t)
	code, raw := e.owner(t, "POST", "/api/v2/skills/"+strings.Repeat("a", 65)+"/use-version", `{"agentId":"`+e.agent.ID+`"}`)
	if code != 400 {
		t.Fatalf("a 65-byte item: %d %s", code, raw)
	}
}

// A refusal of many skills names three of them and counts the rest; the
// full list is in names.
func TestOldRemoveRefusalNamesAFewAndCountsTheRest(t *testing.T) {
	e := newReconcileEnv(t)
	e.fetch.set("", errors.New("offline"))
	ids := []string{}
	for _, name := range []string{"one", "two", "three", "four", "five"} {
		e.linkedSkill(t, "a/b/"+name)
		ids = append(ids, `"a/b/`+name+`"`)
	}
	code, raw := e.owner(t, "POST", "/api/skills/remove", `{"ids":[`+strings.Join(ids, ",")+`]}`)
	var refusal struct {
		Code, Error string
		Names       []string
	}
	if err := json.Unmarshal([]byte(raw), &refusal); err != nil || code != 409 || len(refusal.Names) != 5 ||
		refusal.Error != "reet has no copy of one, two, three and 2 more, so they were not removed. Removing them deletes them from every machine, which keeps a backup for 30 days." {
		t.Fatalf("%d %s", code, raw)
	}
}

// Delete's refusal of a skill the person wrote mentions its public page
// only when it has one.
func TestSkillsPageDeleteOfAWrittenSkillMentionsItsPageOnlyWhenPublished(t *testing.T) {
	e := newReconcileEnv(t)
	draft := e.writtenSkill(t, "draft")
	code, raw := e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["`+draft+`"]}`)
	if code != 409 || !strings.Contains(raw, `"error":"You wrote draft. Deleting it deletes your writing too."`) {
		t.Fatalf("unpublished: %d %s", code, raw)
	}
	published := e.writtenSkill(t, "shared")
	body := `{"content":"---\nname: shared\ndescription: Written in reet\n---\nBody.\n","published":true}`
	if code, raw = e.owner(t, "PUT", "/api/v2/hosting/skills/shared", body); code != 200 {
		t.Fatalf("publish: %d %s", code, raw)
	}
	code, raw = e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["`+published+`"]}`)
	if code != 409 || !strings.Contains(raw, `"error":"You wrote shared. Deleting it deletes your writing and its public page too."`) {
		t.Fatalf("published: %d %s", code, raw)
	}
	code, raw = e.owner(t, "POST", "/api/v2/skills/delete", `{"ids":["`+draft+`","`+published+`"]}`)
	if code != 409 || !strings.Contains(raw, `"error":"You wrote draft and shared. Deleting them deletes your writing and a public page too."`) {
		t.Fatalf("both: %d %s", code, raw)
	}
}

// A stored capture request reet cannot read leaves that skill's reason
// empty; the rest of the page still loads.
func TestSkillsPageLoadsPastAnUnreadableUseVersionRequest(t *testing.T) {
	for name, data := range map[string]string{
		"not JSON":  `{not json`,
		"bad shape": `{"useVersionItem":"%s","backupPath":"~/.agents/.reet-backups/2026-11-18/agents-demo","state":5}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newReconcileEnv(t)
			item := e.writtenSkill(t, "demo")
			e.editedOn(t, "demo", "~/.agents/.reet-backups/2026-11-18/agents-demo")
			if strings.Contains(data, "%s") {
				data = fmt.Sprintf(data, item)
			}
			if _, err := e.db(t).Exec(`INSERT INTO machine_import_requests(id,user_id,agent_id,request_key,state,data,created_at) VALUES('broken',?,?,'broken-request-key','reviewing',?,1)`, e.uid, e.agent.ID, data); err != nil {
				t.Fatal(err)
			}
			r := e.page(t, "active").row(t, "demo")
			if r.EditedOn == nil || r.EditedOnProblem != "" {
				t.Fatalf("row: %+v %q", r.EditedOn, r.EditedOnProblem)
			}
		})
	}
}
