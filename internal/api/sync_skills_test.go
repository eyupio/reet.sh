package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

type syncSkillsState struct {
	Machines []store.SyncMachine   `json:"machines"`
	Activity []store.SkillActivity `json:"activity"`
	Work     *store.SkillWork      `json:"work"`
	Active   *int                  `json:"active"`
}

func (e reconcileEnv) syncSkills(t *testing.T) (syncSkillsState, string) {
	t.Helper()
	code, raw := e.owner(t, "GET", "/api/v2/sync/skills", "")
	if code != 200 {
		t.Fatalf("sync skills: %d %s", code, raw)
	}
	var s syncSkillsState
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("sync skills: %v %s", err, raw)
	}
	return s, raw
}

func (e reconcileEnv) reportSkills(t *testing.T, rep store.SkillReport) {
	t.Helper()
	body, _ := json.Marshal(rep)
	if code, raw := e.call(t, "/api/v2/sync/skills/report", string(body), true); code != 200 {
		t.Fatalf("report: %d %s", code, raw)
	}
}

// The Sync page reads one machine per row, what each is doing, and what the
// machines did, newest first.
func TestSyncSkillsState(t *testing.T) {
	e := newReconcileEnv(t)
	e.writtenSkill(t, "seo-audit")
	e.writtenSkill(t, "seo-cluster")
	e.poll(t, "", true)
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	digest := map[string]string{}
	for _, a := range d.Active {
		digest[a.Name] = a.Digest
	}
	e.reportSkills(t, store.SkillReport{Version: d.Version, Entries: []store.SkillReportEntry{
		{Folder: "~/.agents/skills", Name: "seo-audit", State: "installed", Digest: digest["seo-audit"]},
	}})
	e.reportSkills(t, store.SkillReport{Version: d.Version, Entries: []store.SkillReportEntry{
		{Folder: "~/.agents/skills", Name: "seo-audit", State: "installed", Digest: digest["seo-audit"]},
		{Folder: "~/.claude/skills", Name: "seo-cluster", State: "failed", Detail: "no write access"},
	}})
	paused, _, err := e.st.EnrollSyncAgent(e.uid, "build-box", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.st.SetAgentSkillsProtocolFor(e.uid, paused.ID, 2); err != nil {
		t.Fatal(err)
	}
	if code, raw := e.owner(t, "PATCH", "/api/v2/sync/agents", `{"id":"`+paused.ID+`","action":"pause"}`); code != 200 {
		t.Fatalf("pause: %d %s", code, raw)
	}
	older, _, err := e.st.EnrollSyncAgent(e.uid, "old-mac", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.st.SetAgentSkillsProtocolFor(e.uid, older.ID, 1); err != nil {
		t.Fatal(err)
	}

	s, raw := e.syncSkills(t)
	for _, key := range []string{`"agentId"`, `"host"`, `"paused"`, `"online"`, `"protocol"`, `"state"`, `"failed"`, `"waiting"`, `"lastSeen"`, `"activity"`} {
		if !strings.Contains(raw, key) {
			t.Fatalf("no %s in %s", key, raw)
		}
	}
	if len(s.Machines) != 3 || s.Machines[0].Host != "build-box" || s.Machines[1].Host != "elitebook" || s.Machines[2].Host != "old-mac" {
		t.Fatalf("machines: %+v", s.Machines)
	}
	if m := s.Machines[0]; m.State != "paused" || !m.Paused || m.AgentID != paused.ID || len(m.Failed) != 0 {
		t.Fatalf("paused machine: %+v", m)
	}
	if m := s.Machines[1]; m.State != "failed" || !m.Online || m.Protocol != 2 || m.LastSeen == 0 ||
		len(m.Failed) != 1 || m.Failed[0] != (store.SyncFailure{Name: "seo-cluster", Detail: "no write access"}) || len(m.Waiting) != 0 {
		t.Fatalf("failed machine: %+v", m)
	}
	if m := s.Machines[2]; m.State != "needs-update" || m.Protocol != 1 || m.Paused {
		t.Fatalf("protocol-1 machine: %+v", m)
	}
	if s.Work == nil || s.Work.Failed != 1 || s.Work.NeedsUpdate != 1 || s.Work.Paused != 1 || s.Active == nil || *s.Active != 2 {
		t.Fatalf("work %+v, active %v", s.Work, s.Active)
	}
	// Newest first: the failure came after the install.
	if len(s.Activity) != 2 || s.Activity[0].Name != "seo-cluster" || s.Activity[0].Action != "failed" || s.Activity[1].Name != "seo-audit" || s.Activity[1].Action != "installed" {
		t.Fatalf("activity: %+v", s.Activity)
	}
	if s.Activity[0].At < s.Activity[1].At {
		t.Fatalf("activity is not newest first: %+v", s.Activity)
	}
}

// Retry clears a machine's failure, and its next poll carries the names once.
func TestSyncSkillsRetry(t *testing.T) {
	e := newReconcileEnv(t)
	item := e.writtenSkill(t, "seo-cluster")
	reply, _ := e.poll(t, "", true)
	e.reportSkills(t, store.SkillReport{Version: reply.Desired.Version, Entries: []store.SkillReportEntry{
		{Folder: "~/.claude/skills", Name: "seo-cluster", State: "failed", Detail: "no write access"},
	}})
	retry := `{"agentId":"` + e.agent.ID + `","names":["seo-cluster"]}`

	// A viewer cannot ask, nor can another account, which does not see the machine.
	viewer, err := e.st.CreateUser("viewer@retry.invalid", "Viewer", "viewer", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.CreateAPIToken(viewer.ID, "viewer", auth.HashToken("fixture-viewer")); err != nil {
		t.Fatal(err)
	}
	if code, raw := doClient(t, e.srv.Client(), e.srv, "POST", "/api/v2/sync/skills/retry", retry, "X-Reet-Token", "fixture-viewer"); code != 403 {
		t.Fatalf("viewer: %d %s", code, raw)
	}
	other, err := e.st.CreateUser("other@retry.invalid", "Other", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.CreateAPIToken(other.ID, "other", auth.HashToken("fixture-other")); err != nil {
		t.Fatal(err)
	}
	if code, raw := doClient(t, e.srv.Client(), e.srv, "POST", "/api/v2/sync/skills/retry", retry, "X-Reet-Token", "fixture-other"); code != 404 {
		t.Fatalf("another account: %d %s", code, raw)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/sync/skills/retry", `{"agentId":"`+e.agent.ID+`","names":[]}`); code != 400 {
		t.Fatalf("no names: %d %s", code, raw)
	}
	if again, raw := e.poll(t, reply.Desired.Version, true); strings.Contains(raw, `"retry"`) || again.Desired != nil {
		t.Fatalf("a poll before any retry: %s", raw)
	}

	code, raw := e.owner(t, "POST", "/api/v2/sync/skills/retry", retry)
	if code != 200 || !strings.Contains(raw, `"names":["seo-cluster"]`) {
		t.Fatalf("retry: %d %s", code, raw)
	}
	if m, _ := e.st.SkillHealthFor(e.uid, time.Now()); m[item].State != "installing" {
		t.Fatalf("health after the retry: %+v", m[item])
	}
	_, raw = e.poll(t, reply.Desired.Version, true)
	var withRetry struct{ Retry []string }
	if err = json.Unmarshal([]byte(raw), &withRetry); err != nil || len(withRetry.Retry) != 1 || withRetry.Retry[0] != "seo-cluster" {
		t.Fatalf("the poll after the retry: %s", raw)
	}
	if _, raw = e.poll(t, reply.Desired.Version, true); strings.Contains(raw, `"retry"`) {
		t.Fatalf("the retry was sent twice: %s", raw)
	}

	// A paused machine is not asked, and is not handed a retry while paused.
	if code, raw = e.owner(t, "POST", "/api/v2/sync/skills/retry", retry); code != 200 {
		t.Fatalf("retry: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "PATCH", "/api/v2/sync/agents", `{"id":"`+e.agent.ID+`","action":"pause"}`); code != 200 {
		t.Fatalf("pause: %d %s", code, raw)
	}
	if code, raw = e.owner(t, "POST", "/api/v2/sync/skills/retry", retry); code != 409 {
		t.Fatalf("paused machine: %d %s", code, raw)
	}
	if _, raw = e.poll(t, reply.Desired.Version, true); strings.Contains(raw, `"retry"`) {
		t.Fatalf("a paused machine was handed a retry: %s", raw)
	}
	if code, raw = e.owner(t, "PATCH", "/api/v2/sync/agents", `{"id":"`+e.agent.ID+`","action":"resume"}`); code != 200 {
		t.Fatalf("resume: %d %s", code, raw)
	}
	if _, raw = e.poll(t, reply.Desired.Version, true); !strings.Contains(raw, `"retry":["seo-cluster"]`) {
		t.Fatalf("the retry kept while paused: %s", raw)
	}
}
