package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// olderAgents serves h as a controller sees agents from before the skills
// protocol. Tests of the job-based sync use it: only such agents are given
// that sync, until it is removed.
func olderAgents(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(skillsProtocolHeader)
		h.ServeHTTP(w, r)
	})
}

// reconcileEnv is a controller with one enrolled machine, elitebook, and the
// owner's token, as an agent talks to it.
type reconcileEnv struct {
	srv    *httptest.Server
	api    *Server
	st     *store.Store
	uid    int64
	agent  store.SyncAgent
	secret string
	fetch  *stubFetcher
}

func newReconcileEnv(t *testing.T) reconcileEnv {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "owner", auth.HashToken("fixture-owner")); err != nil {
		t.Fatal(err)
	}
	f := &stubFetcher{body: "v1"}
	h := New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts), WithSkillFetcher(f))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	a, secret, err := st.EnrollSyncAgent(uid, "elitebook", "test")
	if err != nil {
		t.Fatal(err)
	}
	return reconcileEnv{srv, h, st, uid, a, secret, f}
}

// call sends a request as the machine: the owner's token, its enrollment and,
// with protocol, the skills protocol header.
func (e reconcileEnv) call(t *testing.T, path, body string, protocol bool, extra ...string) (int, string) {
	t.Helper()
	headers := []string{"X-Reet-Token", "fixture-owner", "X-Reet-Agent", e.agent.ID, "X-Reet-Agent-Key", e.secret, "X-Reet-Version", "test"}
	if protocol {
		headers = append(headers, "X-Reet-Skills-Protocol", "2")
	}
	return doClient(t, e.srv.Client(), e.srv, "POST", path, body, append(headers, extra...)...)
}

type skillPollReply struct {
	Agent   store.SyncAgent
	Job     *store.SyncJob
	Desired *store.DesiredState
}

func (e reconcileEnv) poll(t *testing.T, version string, protocol bool) (skillPollReply, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"skillsVersion": version})
	code, raw := e.call(t, "/api/v2/sync/poll", string(body), protocol)
	if code != 200 {
		t.Fatalf("poll: %d %s", code, raw)
	}
	var reply skillPollReply
	if err := json.Unmarshal([]byte(raw), &reply); err != nil {
		t.Fatal(err)
	}
	return reply, raw
}

// writtenSkill saves a skill written in reet, which has content from the
// start, and returns its library item id.
func (e reconcileEnv) writtenSkill(t *testing.T, name string) string {
	t.Helper()
	if username, err := e.st.PublishingUsername(e.uid); err != nil {
		t.Fatal(err)
	} else if username == "" {
		if err = e.st.ClaimPublishingUsername(e.uid, "writer"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.st.SaveHostedSkill(e.uid, store.HostedSkill{Slug: name, Content: "---\nname: " + name + "\ndescription: Written in reet\n---\nBody.\n"}); err != nil {
		t.Fatal(err)
	}
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now().Add(store.SkillGrace))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range d.Active {
		if a.Name == name {
			return a.Item
		}
	}
	t.Fatalf("%s has no content in reet: %+v", name, d)
	return ""
}

func TestPollSendsDesiredToProtocol2(t *testing.T) {
	e := newReconcileEnv(t)
	e.writtenSkill(t, "demo")
	reply, raw := e.poll(t, "", true)
	if reply.Desired == nil || reply.Desired.Version == "" || len(reply.Desired.Active) != 1 || reply.Desired.Active[0].Name != "demo" || reply.Desired.Active[0].Digest == "" {
		t.Fatalf("desired: %s", raw)
	}
	if reply.Agent.ID != e.agent.ID || reply.Agent.SkillsProtocol != 2 || reply.Agent.SkillsState != "" {
		t.Fatalf("agent: %+v", reply.Agent)
	}
	again, raw := e.poll(t, reply.Desired.Version, true)
	if again.Desired != nil || strings.Contains(raw, `"desired"`) {
		t.Fatalf("a machine at the current version is sent it again: %s", raw)
	}
	if other, raw := e.poll(t, "an-older-version", true); other.Desired == nil || other.Desired.Version != reply.Desired.Version {
		t.Fatalf("a machine at another version: %s", raw)
	}
	agents, err := e.st.SyncAgentsFor(e.uid)
	if err != nil || len(agents) != 1 || agents[0].SkillsProtocol != 2 {
		t.Fatalf("the protocol is recorded: %+v %v", agents, err)
	}
	// A paused machine does no skill work, so it is sent none.
	if err = e.st.ControlSyncAgent(e.uid, e.agent.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	if paused, raw := e.poll(t, "", true); paused.Desired != nil || !paused.Agent.Paused {
		t.Fatalf("paused: %s", raw)
	}
}

func TestPollBodyIsSmallAndStrict(t *testing.T) {
	e := newReconcileEnv(t)
	for _, bad := range []string{`nope`, `{"skillsVersion":"v","extra":1}`, `{"skillsVersion":"` + strings.Repeat("v", 8<<10) + `"}`, `{} {}`} {
		if code, body := e.call(t, "/api/v2/sync/poll", bad, true); code != 400 {
			t.Fatalf("poll with %.40q: %d %s", bad, code, body)
		}
	}
	if code, body := e.call(t, "/api/v2/sync/poll", `{}`, true); code != 200 {
		t.Fatalf("a poll with no version: %d %s", code, body)
	}
}

func TestPollGivesProtocol2OnlyUpdateJobs(t *testing.T) {
	e := newReconcileEnv(t)
	scan, err := e.st.QueueSyncJob(e.uid, e.agent.ID, "", "scan")
	if err != nil {
		t.Fatal(err)
	}
	if reply, raw := e.poll(t, "", true); reply.Job != nil {
		t.Fatalf("an old-engine job was handed out: %s", raw)
	}
	update, err := e.st.QueueAgentUpdate(e.uid, e.agent.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	reply, raw := e.poll(t, "", true)
	if reply.Job == nil || reply.Job.ID != update.ID || reply.Job.Kind != "update" {
		t.Fatalf("the update job: %s", raw)
	}
	jobs, err := e.st.SyncJobsFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	// The machine's first poll on the protocol closed its job-based work,
	// saying why, and no more can be queued for it.
	for _, j := range jobs {
		if j.ID == scan.ID && (j.Status != "cancelled" || j.Message != "This machine now follows your library; this job is no longer needed.") {
			t.Fatalf("the scan job: %+v", j)
		}
	}
	if code, raw := doClient(t, e.srv.Client(), e.srv, "POST", "/api/v2/sync/jobs", `{"agentId":"`+e.agent.ID+`","kind":"scan"}`, "X-Reet-Token", "fixture-owner"); code != 409 || !strings.Contains(raw, "follows your library") {
		t.Fatalf("queueing a scan: %d %s", code, raw)
	}
}

func TestPollWithoutProtocolGetsNoSkillWork(t *testing.T) {
	e := newReconcileEnv(t)
	e.writtenSkill(t, "demo")
	reply, raw := e.poll(t, "", false)
	if reply.Desired != nil || strings.Contains(raw, `"desired"`) {
		t.Fatalf("an older agent was sent skills: %s", raw)
	}
	machineState := func() store.SyncAgent {
		t.Helper()
		code, body := doClient(t, e.srv.Client(), e.srv, "GET", "/api/v2/sync", "", "X-Reet-Token", "fixture-owner")
		var state struct{ Agents []store.SyncAgent }
		if code != 200 || json.Unmarshal([]byte(body), &state) != nil || len(state.Agents) != 1 {
			t.Fatalf("sync state: %d %s", code, body)
		}
		return state.Agents[0]
	}
	if a := machineState(); a.SkillsState != "needs-update" || a.SkillsProtocol != 0 {
		t.Fatalf("an older agent: %+v", a)
	}
	e.poll(t, "", true)
	if a := machineState(); a.SkillsState != "" || a.SkillsProtocol != 2 {
		t.Fatalf("after updating: %+v", a)
	}
	// The same enrollment run by an older build again gets no skill work.
	if reply, raw := e.poll(t, "", false); reply.Desired != nil {
		t.Fatalf("an older build was sent skills: %s", raw)
	}
	if a := machineState(); a.SkillsState != "needs-update" {
		t.Fatalf("after going back to an older build: %+v", a)
	}
}

func TestReportEndpointRecords(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.writtenSkill(t, "demo")
	e.poll(t, "", true)
	health := func() store.SkillHealth {
		t.Helper()
		m, err := e.st.SkillHealthFor(e.uid, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return m[item]
	}
	if h := health(); h.State != "installing" || h.Pending != 1 {
		t.Fatalf("before the report: %+v", h)
	}
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rep := store.SkillReport{Version: d.Version, Entries: []store.SkillReportEntry{
		{Folder: "~/.agents/skills", Name: "demo", State: "installed", Digest: d.Active[0].Digest},
		// An agent sends a name it refused with no folder, and backups under ~/.
		{Name: "bad/name", State: "failed", Detail: "refused: the name holds a path separator"},
		{Folder: "~/.claude/skills", Name: "gone", State: "removed", Backup: "~/.claude/.reet-backups/2027-01-15/claude-gone"},
	}}
	body, _ := json.Marshal(rep)
	if code, raw := e.call(t, "/api/v2/sync/skills/report", string(body), true); code != 200 {
		t.Fatalf("report: %d %s", code, raw)
	}
	if h := health(); h.State != "ok" || h.Pending != 0 || h.Total != 1 {
		t.Fatalf("after the report: %+v", h)
	}
	activity, err := e.st.SkillActivityFor(e.uid, 10)
	if err != nil || len(activity) != 3 {
		t.Fatalf("activity: %+v %v", activity, err)
	}

	// Another account's token with this machine's enrollment is refused, and
	// records nothing.
	other, err := e.st.CreateUser("other@reconcile.invalid", "Other", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.CreateAPIToken(other.ID, "other", auth.HashToken("fixture-other")); err != nil {
		t.Fatal(err)
	}
	failed := store.SkillReport{Version: d.Version, Entries: []store.SkillReportEntry{{Folder: "~/.agents/skills", Name: "demo", State: "failed", Detail: "forged"}}}
	body, _ = json.Marshal(failed)
	if code, raw := e.call(t, "/api/v2/sync/skills/report", string(body), true, "X-Reet-Token", "fixture-other"); code != 403 {
		t.Fatalf("another account's report: %d %s", code, raw)
	}
	if code, raw := e.call(t, "/api/v2/sync/skills/report", string(body), true, "X-Reet-Agent-Key", "wrong"); code != 403 {
		t.Fatalf("a wrong enrollment secret: %d %s", code, raw)
	}
	if h := health(); h.State != "ok" {
		t.Fatalf("a refused report changed health: %+v", h)
	}
}

func TestReportBodyIsBoundedAndStrict(t *testing.T) {
	e := newReconcileEnv(t)
	many := store.SkillReport{Version: "v1"}
	for i := range 8193 {
		many.Entries = append(many.Entries, store.SkillReportEntry{Folder: "~/.agents/skills", Name: fmt.Sprintf("s%d", i), State: "unknown"})
	}
	tooMany, _ := json.Marshal(many)
	huge, _ := json.Marshal(store.SkillReport{Version: "v1", Entries: []store.SkillReportEntry{{Folder: "~/.agents/skills", Name: "x", State: "failed", Detail: strings.Repeat("x", 9<<20)}}})
	for name, bad := range map[string]string{
		"not json":     `nope`,
		"unknown":      `{"Version":"v1","Entries":[],"Extra":true}`,
		"trailing":     `{"Version":"v1"} {}`,
		"8193 entries": string(tooMany),
		"over 8 MiB":   string(huge),
	} {
		if code, raw := e.call(t, "/api/v2/sync/skills/report", bad, true); code != 400 {
			t.Fatalf("%s: %d %.200s", name, code, raw)
		}
	}
	if code, raw := e.call(t, "/api/v2/sync/skills/report", `{"Version":"v1","Entries":[]}`, false); code != 400 {
		t.Fatalf("a report without the skills protocol: %d %s", code, raw)
	}
	if code, raw := e.call(t, "/api/v2/sync/skills/report", `{"Version":"v1","Entries":[]}`, true); code != 200 {
		t.Fatalf("an empty report: %d %s", code, raw)
	}
	// As many entries as four folders of the most skills reet holds, and more.
	many.Entries = many.Entries[:8192]
	full, _ := json.Marshal(many)
	if code, raw := e.call(t, "/api/v2/sync/skills/report", string(full), true); code != 200 {
		t.Fatalf("8192 entries: %d %.200s", code, raw)
	}
}

// A report keeps the machine's last-seen time current and records the skills
// protocol it announced, as a poll does.
func TestReportRecordsProtocolAndLastSeen(t *testing.T) {
	e := newReconcileEnv(t)
	agents, err := e.st.SyncAgentsFor(e.uid)
	if err != nil || len(agents) != 1 || agents[0].SkillsProtocol != 0 {
		t.Fatalf("fixture: %+v %v", agents, err)
	}
	before := agents[0].LastSeen
	time.Sleep(5 * time.Millisecond)
	if code, raw := e.call(t, "/api/v2/sync/skills/report", `{"Version":"v1","Entries":[]}`, true); code != 200 {
		t.Fatalf("report: %d %s", code, raw)
	}
	agents, err = e.st.SyncAgentsFor(e.uid)
	if err != nil || agents[0].SkillsProtocol != 2 || agents[0].LastSeen <= before {
		t.Fatalf("after the report: %+v (last seen before %d) %v", agents, before, err)
	}
}

func TestPollKicksCopies(t *testing.T) {
	e := newReconcileEnv(t)
	r, err := ref.Parse("vercel-labs/skills/find-skills")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.UpsertFor(e.uid, r, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now()); len(d.Active) != 0 {
		t.Fatalf("fixture: an Active skill without content: %+v", d)
	}
	e.poll(t, "", false)
	e.api.s.skillCopies.Wait()
	if calls := e.fetch.callCount(); calls != 0 {
		t.Fatalf("an older agent's poll fetched %d times", calls)
	}
	e.poll(t, "", true)
	e.api.s.skillCopies.Wait()
	if calls := e.fetch.callCount(); calls != 1 {
		t.Fatalf("one poll fetched %d times", calls)
	}
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now())
	if err != nil || len(d.Active) != 1 || d.Active[0].Name != "find-skills" {
		t.Fatalf("after the poll: %+v %v", d, err)
	}
	// The machine's next poll carries the skill.
	if reply, raw := e.poll(t, "", true); reply.Desired == nil || len(reply.Desired.Active) != 1 {
		t.Fatalf("next poll: %s", raw)
	}
}

// A machine reporting a skill reet has no copy of starts its capture, without
// anyone pressing Retry.
func TestReportKicksCopies(t *testing.T) {
	e := newReconcileEnv(t)
	if _, err := e.st.UpsertLocalFor(e.uid, "kept", store.Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := e.st.RecordEnvironmentFor(e.uid, "local/kept", "elitebook", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.ClaimRemoteImportFor(e.uid, e.agent.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	e.poll(t, "", true)
	e.api.s.skillCopies.Wait()
	if requests, _, err := e.st.RemoteImportsFor(e.uid); err != nil || len(requests) != 0 {
		t.Fatalf("no machine has reported it yet: %+v %v", requests, err)
	}
	body, _ := json.Marshal(store.SkillReport{Version: "v1", Entries: []store.SkillReportEntry{{Folder: "~/.agents/skills", Name: "kept", State: "installed", Digest: strings.Repeat("d", 64)}}})
	if code, raw := e.call(t, "/api/v2/sync/skills/report", string(body), true); code != 200 {
		t.Fatalf("report: %d %s", code, raw)
	}
	e.api.s.skillCopies.Wait()
	requests, _, err := e.st.RemoteImportsFor(e.uid)
	if err != nil || len(requests) != 1 || requests[0].SyncSkillID != "local/kept" || requests[0].AgentID != e.agent.ID {
		t.Fatalf("the capture was not asked for: %+v %v", requests, err)
	}
}

// An edited copy's backup is still known after the next report says the
// skill is installed, so Use this version can find it.
func TestReportKeepsEditedBackups(t *testing.T) {
	e := newReconcileEnv(t)
	e.writtenSkill(t, "demo")
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	digest := d.Active[0].Digest
	send := func(entries ...store.SkillReportEntry) {
		t.Helper()
		body, _ := json.Marshal(store.SkillReport{Version: d.Version, Entries: entries})
		if code, raw := e.call(t, "/api/v2/sync/skills/report", string(body), true); code != 200 {
			t.Fatalf("report: %d %s", code, raw)
		}
	}
	backup := "~/.agents/.reet-backups/2027-01-15/agents-demo"
	send(store.SkillReportEntry{Folder: "~/.agents/skills", Name: "demo", State: "edited", Digest: digest, Backup: backup})
	send(store.SkillReportEntry{Folder: "~/.agents/skills", Name: "demo", State: "installed", Digest: digest})
	got, err := e.st.SkillBackupsFor(e.uid, "demo", time.Now())
	if err != nil || len(got) != 1 || got[0].Backup != backup || got[0].AgentID != e.agent.ID || got[0].Host != "elitebook" {
		t.Fatalf("backups: %+v %v", got, err)
	}
	activity, err := e.st.SkillActivityFor(e.uid, 10)
	if err != nil || len(activity) != 1 || activity[0].Action != "edited" {
		t.Fatalf("an edit followed by reet's copy is one line: %+v %v", activity, err)
	}
}

// scanItem is one sighting in an inventory scan, as `reet scan` posts it.
type scanItem struct {
	Name    string `json:"name"`
	Source  string `json:"source,omitempty"`
	Host    string `json:"host,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Project string `json:"project,omitempty"`
	Path    string `json:"path,omitempty"`
}

// scanImport posts an inventory scan from elitebook's home folders, as
// `reet scan` does: one sighting in ~/.agents/skills for each name in home, and
// one entry for each name in locked, a skill a lock file names with no folder.
func (e reconcileEnv) scanImport(t *testing.T, home []string, locked ...string) {
	t.Helper()
	items := []scanItem{}
	for _, name := range home {
		items = append(items, scanItem{Name: name, Host: "elitebook", Agent: "agents", Path: ".agents/skills"})
	}
	for _, name := range locked {
		items = append(items, scanItem{Name: name, Source: "owner/repo"})
	}
	e.scanItems(t, "elitebook", items...)
}

// scanItems posts an inventory scan from machine with exactly items.
func (e reconcileEnv) scanItems(t *testing.T, machine string, items ...scanItem) {
	t.Helper()
	envs := map[string]map[string]any{}
	for _, it := range items {
		if it.Host == "" {
			continue
		}
		key := it.Agent + "\x00" + it.Project
		if envs[key] == nil {
			envs[key] = map[string]any{"host": it.Host, "agent": it.Agent, "project": it.Project, "path": it.Path, "skills": []string{}}
		}
		envs[key]["skills"] = append(envs[key]["skills"].([]string), it.Name)
	}
	environments := []map[string]any{}
	for _, env := range envs {
		environments = append(environments, env)
	}
	body, _ := json.Marshal(map[string]any{"machine": map[string]string{"host": machine}, "prune": true, "environments": environments, "items": items})
	if code, raw := doClient(t, e.srv.Client(), e.srv, "POST", "/api/import", string(body), "X-Reet-Token", "fixture-owner"); code != 200 {
		t.Fatalf("scan import: %d %s", code, raw)
	}
}

func (e reconcileEnv) libraryNames(t *testing.T) map[string]store.LibraryItem {
	t.Helper()
	items, err := e.st.LibraryFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]store.LibraryItem{}
	for _, it := range items {
		out[it.Name] = it
	}
	return out
}

// A skill a machine on the skills protocol has and reet does not know is New
// on a machine, from the machine's skills report: the inventory scan makes no
// library item of it, so nothing captures or spreads it before Keep.
func TestScanOfAProtocol2MachineMakesNoLibraryItems(t *testing.T) {
	e := newReconcileEnv(t)
	r, err := ref.Parse("vercel-labs/skills/find-skills")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.UpsertFor(e.uid, r, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	e.poll(t, "", true)
	e.scanImport(t, []string{"cq", "find-skills"}, "locked")
	// Codex's folder is never synced, but a skill there is still the
	// machine's own: installed with a source or left after a delete, it must
	// not become a library item that is fetched or spread.
	e.scanItems(t, "elitebook",
		scanItem{Name: "codex-linked", Source: "owner/repo", Host: "elitebook", Agent: "codex", Path: ".codex/skills"},
		scanItem{Name: "codex-local", Host: "elitebook", Agent: "codex", Path: ".codex/skills"})
	names := e.libraryNames(t)
	if _, ok := names["cq"]; ok || len(names) != 1 {
		t.Fatalf("the scan made library items: %+v", names)
	}
	// A sighting of a name the library has still joins it.
	joined := false
	for _, a := range names["find-skills"].Aliases {
		joined = joined || a.Kind == "shelf" && a.Reference == "local/find-skills"
	}
	if !joined {
		t.Fatalf("the sighting of find-skills did not join it: %+v", names["find-skills"].Aliases)
	}
	body, _ := json.Marshal(store.SkillReport{Version: "v1", Entries: []store.SkillReportEntry{{Folder: "~/.agents/skills", Name: "cq", State: "unknown", Digest: strings.Repeat("c", 64)}}})
	if code, raw := e.call(t, "/api/v2/sync/skills/report", string(body), true); code != 200 {
		t.Fatalf("report: %d %s", code, raw)
	}
	found, err := e.st.NewOnMachinesFor(e.uid)
	if err != nil || len(found) != 1 || found[0].Name != "cq" {
		t.Fatalf("new on a machine: %+v %v", found, err)
	}
	e.api.s.skillCopies.Wait()
	needs, err := e.st.SkillsNeedingCopyFor(e.uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range needs {
		if n.Name == "cq" || n.Name == "locked" || strings.HasPrefix(n.Name, "codex-") {
			t.Fatalf("the copy worker plans a copy of %s: %+v", n.Name, needs)
		}
	}
	requests, _, err := e.st.RemoteImportsFor(e.uid)
	if err != nil || len(requests) != 0 {
		t.Fatalf("a capture was asked for: %+v %v", requests, err)
	}
	if names := e.libraryNames(t); len(names) != 1 {
		t.Fatalf("library after the report: %+v", names)
	}
}

// An agent from before the skills protocol keeps today's scan: what it finds
// becomes a library item.
func TestScanOfAnOlderMachineStillMakesLibraryItems(t *testing.T) {
	e := newReconcileEnv(t)
	e.poll(t, "", false)
	e.scanImport(t, []string{"cq"}, "locked")
	names := e.libraryNames(t)
	if _, ok := names["cq"]; !ok {
		t.Fatalf("an older machine's scan made no item of cq: %+v", names)
	}
	if _, ok := names["locked"]; !ok {
		t.Fatalf("an older machine's scan made no item of its lock entry: %+v", names)
	}
}

// A project folder is not the machine's home: until Task 14 its skills keep
// today's import, on any machine.
func TestScanOfAProjectFolderStillMakesLibraryItems(t *testing.T) {
	e := newReconcileEnv(t)
	e.poll(t, "", true)
	e.scanItems(t, "elitebook", scanItem{Name: "in-project", Host: "elitebook", Agent: "claude", Project: "/home/u/code/app", Path: "/home/u/code/app/.claude/skills"})
	if _, ok := e.libraryNames(t)["in-project"]; !ok {
		t.Fatal("a project sighting made no item")
	}
}

// Names a person types into the import are their choice: they are never held
// back, whichever machine is named beside them.
func TestTypedImportIsNeverGated(t *testing.T) {
	e := newReconcileEnv(t)
	e.poll(t, "", true)
	body := `{"machine":{"host":"elitebook"},"text":"typed-skill\n"}`
	if code, raw := doClient(t, e.srv.Client(), e.srv, "POST", "/api/import", body, "X-Reet-Token", "fixture-owner"); code != 200 {
		t.Fatalf("import: %d %s", code, raw)
	}
	if _, ok := e.libraryNames(t)["typed-skill"]; !ok {
		t.Fatal("a typed name made no item")
	}
}

// Polls kick the copy worker at most once every 30 seconds per account; the
// first poll always does, and every report does.
func TestPollKicksCopiesAtMostEvery30Seconds(t *testing.T) {
	e := newReconcileEnv(t)
	var clock atomic.Int64
	kicks := &e.api.s.skillPollKicks
	kicks.mu.Lock()
	kicks.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	kicks.mu.Unlock()
	// A fetch that finds nothing to keep and records no problem, so each kick
	// tries again and counts.
	e.fetch.set("", errNoSkillFetcher)
	r, err := ref.Parse("vercel-labs/skills/find-skills")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.UpsertFor(e.uid, r, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	pollAt := func(at time.Time, want int) {
		t.Helper()
		clock.Store(at.UnixMilli())
		e.poll(t, "", true)
		e.api.s.skillCopies.Wait()
		if got := e.fetch.callCount(); got != want {
			t.Fatalf("after a poll at +%s: %d kicks, want %d", at.Sub(time.Unix(1_800_000_000, 0)), got, want)
		}
	}
	start := time.Unix(1_800_000_000, 0)
	pollAt(start, 1)
	pollAt(start.Add(5*time.Second), 1)
	pollAt(start.Add(29*time.Second), 1)
	pollAt(start.Add(30*time.Second), 2)
	pollAt(start.Add(35*time.Second), 2)
	if code, raw := e.call(t, "/api/v2/sync/skills/report", `{"Version":"v1","Entries":[]}`, true); code != 200 {
		t.Fatalf("report: %d %s", code, raw)
	}
	e.api.s.skillCopies.Wait()
	if got := e.fetch.callCount(); got != 3 {
		t.Fatalf("a report kicks every time: %d kicks", got)
	}
}

// An agent on a newer skills protocol than this controller's gets no skill
// work and no job-based sync, only updates, and shows that the controller
// needs updating.
func TestPollFromANewerAgentGetsOnlyUpdates(t *testing.T) {
	e := newReconcileEnv(t)
	e.writtenSkill(t, "demo")
	update, err := e.st.QueueAgentUpdate(e.uid, e.agent.ID, "latest")
	if err != nil {
		t.Fatal(err)
	}
	code, raw := e.call(t, "/api/v2/sync/poll", `{"skillsVersion":""}`, false, "X-Reet-Skills-Protocol", "3")
	if code != 200 {
		t.Fatalf("poll: %d %s", code, raw)
	}
	var reply skillPollReply
	if err = json.Unmarshal([]byte(raw), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Desired != nil || strings.Contains(raw, `"desired"`) {
		t.Fatalf("a newer agent was sent skills: %s", raw)
	}
	if reply.Job == nil || reply.Job.ID != update.ID {
		t.Fatalf("the update job: %s", raw)
	}
	if reply.Agent.SkillsProtocol != 3 || reply.Agent.SkillsState != store.SkillsNeedNewerController {
		t.Fatalf("agent: %+v", reply.Agent)
	}
	agents, err := e.st.SyncAgentsFor(e.uid)
	if err != nil || agents[0].SkillsProtocol != 3 || agents[0].SkillsState != store.SkillsNeedNewerController {
		t.Fatalf("recorded: %+v %v", agents, err)
	}
	if _, err = e.st.QueueSyncJob(e.uid, e.agent.ID, "", "scan"); err == nil {
		t.Fatal("a scan was queued for a newer agent")
	}
	if code, raw := e.call(t, "/api/v2/sync/skills/report", `{"Version":"v1","Entries":[]}`, false, "X-Reet-Skills-Protocol", "3"); code != 409 {
		t.Fatalf("a newer agent's report: %d %s", code, raw)
	}
	if m, _ := e.st.SkillHealthFor(e.uid, time.Now()); len(m) != 1 {
		t.Fatalf("health: %+v", m)
	} else {
		for _, h := range m {
			if h.Total != 0 {
				t.Fatalf("a newer agent was counted: %+v", h)
			}
		}
	}
}
