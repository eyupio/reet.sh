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

func tsSkill(id, name, slug string, envs ...store.Environment) store.Skill {
	return store.Skill{ID: id, Name: name, Slug: slug, Description: name + " desc", Environments: envs}
}

func tsEnv(host, agent string, seen int64) store.Environment {
	return store.Environment{Host: host, Agent: agent, LastSeen: seen}
}

func TestSkillInvocationIsASingleSafeLine(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "pdf", "Use the pdf skill."},
		{"empty", "", ""},
		{"newline", "a\nb", ""},
		{"tab", "a\tb", ""},
		{"escape", "a\x1b[2Jb", ""},
		{"81 runes", strings.Repeat("é", 81), ""},
		{"80 runes", strings.Repeat("é", 80), "Use the " + strings.Repeat("é", 80) + " skill."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := skillInvocation("claude", c.in); got != c.want {
				t.Fatalf("skillInvocation(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestBuildSessionSkillsSplitsNativeOtherAndReet(t *testing.T) {
	skills := []store.Skill{
		tsSkill("s1", "Zeta", "zeta", tsEnv("box", "claude", 10)),
		tsSkill("s2", "alpha", "alpha", tsEnv("box", "agents", 20)),
		tsSkill("s3", "Codex Only", "codex-only", tsEnv("box", "codex", 5)),
	}
	hosted := []store.HostedSkill{{Slug: "mine", Description: "hosted desc"}}
	got := buildSessionSkills("claude", "box", "", skills, hosted)
	if len(got.Skills) != 4 {
		t.Fatalf("got %d skills: %+v", len(got.Skills), got.Skills)
	}
	order := []string{"s2", "s1", "s3", "reet:mine"}
	for i, id := range order {
		if got.Skills[i].ID != id {
			t.Fatalf("position %d = %q, want %q", i, got.Skills[i].ID, id)
		}
	}
	if s := got.Skills[0]; !s.Native || !s.OnMachine || s.Source != "machine" || s.Agent != "agents" || s.Invocation != "Use the alpha skill." {
		t.Fatalf("agents skill wrong: %+v", s)
	}
	if s := got.Skills[1]; !s.Native || s.Agent != "claude" || s.Invocation != "Use the Zeta skill." {
		t.Fatalf("claude skill wrong: %+v", s)
	}
	if s := got.Skills[2]; s.Native || !s.OnMachine || s.Agent != "codex" || s.Invocation != "" {
		t.Fatalf("codex skill wrong: %+v", s)
	}
	if s := got.Skills[3]; s.Source != "reet" || s.OnMachine || s.Native || s.Invocation != "" || s.Name != "mine" || s.Description != "hosted desc" {
		t.Fatalf("reet skill wrong: %+v", s)
	}
}

func TestBuildSessionSkillsIgnoresOtherMachinesAndMatchesHostCaseInsensitively(t *testing.T) {
	skills := []store.Skill{
		tsSkill("a", "here", "here", tsEnv("BOX", "claude", 1)),
		tsSkill("b", "elsewhere", "elsewhere", tsEnv("other", "claude", 99)),
	}
	got := buildSessionSkills("claude", "box", "", skills, nil)
	if len(got.Skills) != 1 || got.Skills[0].ID != "a" {
		t.Fatalf("got %+v", got.Skills)
	}
	if got.MachineSeen != 1 {
		t.Fatalf("MachineSeen = %d, want 1", got.MachineSeen)
	}
}

func TestBuildSessionSkillsDoesNotRepeatAHostedSkillThatIsInstalled(t *testing.T) {
	skills := []store.Skill{tsSkill("a", "Mine", "mine", tsEnv("box", "claude", 1))}
	hosted := []store.HostedSkill{{Slug: "mine"}, {Slug: "other"}}
	got := buildSessionSkills("claude", "box", "", skills, hosted)
	if len(got.Skills) != 2 || got.Skills[0].ID != "a" || got.Skills[1].ID != "reet:other" {
		t.Fatalf("got %+v", got.Skills)
	}
}

func TestBuildSessionSkillsMachineSeenAndEmpty(t *testing.T) {
	skills := []store.Skill{
		tsSkill("a", "a", "a", tsEnv("box", "claude", 5), tsEnv("box", "codex", 40)),
		tsSkill("b", "b", "b", tsEnv("box", "claude", 12), tsEnv("other", "claude", 500)),
	}
	if got := buildSessionSkills("claude", "box", "", skills, nil); got.MachineSeen != 40 {
		t.Fatalf("MachineSeen = %d, want 40", got.MachineSeen)
	}
	empty := buildSessionSkills("claude", "unscanned", "", nil, nil)
	if empty.MachineSeen != 0 || len(empty.Skills) != 0 || empty.Skills == nil {
		t.Fatalf("empty = %+v", empty)
	}
	b, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"skills":[]`) {
		t.Fatalf("json = %s", b)
	}
}

func TestBuildSessionSkillsListsAnUnsafeNameWithoutAnInvocation(t *testing.T) {
	skills := []store.Skill{tsSkill("a", "bad\nname", "bad", tsEnv("box", "claude", 1))}
	got := buildSessionSkills("claude", "box", "", skills, nil)
	if len(got.Skills) != 1 {
		t.Fatalf("got %+v", got.Skills)
	}
	if s := got.Skills[0]; !s.Native || s.Invocation != "" {
		t.Fatalf("got %+v", s)
	}
}

// skillsEndpointFixture starts a Claude session on the fixture's machine and
// returns a caller for the skills route plus the pieces tests need.
func skillsEndpointFixture(t *testing.T) (call func(login, id string) (int, SessionSkills), importSkill func(host, agent, name string), st *store.Store, uid int64, agent store.SyncAgent, session, cookie string) {
	t.Helper()
	srv, st, agent, _, token := privateFixture(t, AuthAccounts, "")
	uid, _ = st.DefaultOwner()
	cookie = terminalLogin(t, st, uid)
	caps, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, Readiness: terminalproto.Ready})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := st.ReserveTerminalCreateFor(uid, agent.ID, terminalbroker.ID(), caps.Generation, terminalproto.Create{Title: "Skills", Tool: "claude", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	session = created.ID
	call = func(login, id string) (int, SessionSkills) {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v2/terminal/sessions/"+id+"/skills", nil)
		r.Header.Set("Origin", "https://reet.example")
		if login != "" {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: login})
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		var out SessionSkills
		if w.Code == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err, w.Body.String())
			}
		}
		return w.Code, out
	}
	importSkill = func(host, agentName, name string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"items": []map[string]string{{"name": name, "source": "a/b", "host": host, "agent": agentName}}})
		r := httptest.NewRequest("POST", "/api/import", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal("import", w.Code, w.Body.String())
		}
	}
	return
}

func TestTerminalSessionSkillsListsTheSessionsMachine(t *testing.T) {
	call, importSkill, _, _, agent, session, cookie := skillsEndpointFixture(t)
	importSkill(agent.Host, "claude", "pdf")
	importSkill("elsewhere", "claude", "not-here")
	code, got := call(cookie, session)
	if code != 200 || got.Host != agent.Host || len(got.Skills) != 1 {
		t.Fatalf("got %d %+v", code, got)
	}
	sk := got.Skills[0]
	if sk.Name != "pdf" || !sk.Native || !sk.OnMachine || sk.Invocation != "Use the pdf skill." {
		t.Fatalf("entry %+v", sk)
	}
}

func TestTerminalSessionSkillsFollowsAMachineAlias(t *testing.T) {
	call, importSkill, st, uid, agent, session, cookie := skillsEndpointFixture(t)
	// A skill recorded under an old name that was merged into the agent's host.
	importSkill("old-box", "claude", "legacy")
	if err := st.MergeMachinesFor(uid, "old-box", agent.Host); err != nil {
		t.Fatal(err)
	}
	// The agent's own host name is itself renamed: reports now live under the preferred name.
	importSkill(agent.Host, "claude", "current")
	if err := st.MergeMachinesFor(uid, agent.Host, "preferred"); err != nil {
		t.Fatal(err)
	}
	code, got := call(cookie, session)
	if code != 200 || got.Host != "preferred" {
		t.Fatalf("got %d %+v", code, got)
	}
	names := map[string]bool{}
	for _, sk := range got.Skills {
		names[sk.Name] = true
	}
	if !names["legacy"] || !names["current"] {
		t.Fatalf("skills %+v", got.Skills)
	}
}

func TestTerminalSessionSkillsRefusesAnotherAccountAndAnonymous(t *testing.T) {
	call, _, st, _, _, session, cookie := skillsEndpointFixture(t)
	other, err := st.CreateUser("skills-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	// Sibling routes answer a session the caller does not own with 403.
	if code, _ := call(terminalLogin(t, st, other.ID), session); code != 403 {
		t.Fatalf("other account got %d", code)
	}
	if code, _ := call(cookie, strings.Repeat("0", len(session))); code != 403 {
		t.Fatalf("unknown session got %d", code)
	}
	if code, _ := call("", session); code != 401 {
		t.Fatalf("anonymous got %d", code)
	}
}

func TestTerminalSessionSkillsStillListsHostedSkillsWhenTheMachineIsUnknown(t *testing.T) {
	call, importSkill, st, uid, agent, session, cookie := skillsEndpointFixture(t)
	importSkill(agent.Host, "claude", "pdf")
	if err := st.ClaimPublishingUsername(uid, "skills-author"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveHostedSkill(uid, store.HostedSkill{Slug: "mine", Description: "hosted desc", Content: "---\nname: mine\ndescription: hosted desc\n---\n\nSteps\n"}); err != nil {
		t.Fatal(err)
	}
	// Revoking an agent deletes its sessions with it, so the machine is made
	// unknown by blanking its host on a second connection to the same database.
	raw, err := sql.Open("sqlite", filepath.Join(st.Dir(), "reet.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`UPDATE sync_agents SET host='' WHERE id=?`, agent.ID); err != nil {
		t.Fatal(err)
	}
	code, got := call(cookie, session)
	if code != 200 || got.Host != "" || got.MachineSeen != 0 || len(got.Skills) != 1 {
		t.Fatalf("got %d %+v", code, got)
	}
	if s := got.Skills[0]; s.Source != "reet" || s.Name != "mine" || s.Invocation != "" || s.OnMachine || s.Native {
		t.Fatalf("hosted entry %+v", s)
	}
}

func tsProjectEnv(host, agent, project string, seen int64) store.Environment {
	return store.Environment{Host: host, Agent: agent, Project: project, LastSeen: seen}
}

func TestBuildSessionSkillsOnlyCountsProjectEnvironmentsForTheSessionsFolder(t *testing.T) {
	cases := []struct {
		name     string
		dir      string
		skills   []store.Skill
		wantIDs  []string
		wantSeen int64
	}{
		{"project skill inside the dir is listed and native", "/work/repoA/pkg",
			[]store.Skill{tsSkill("p", "proj", "proj", tsProjectEnv("box", "claude", "/work/repoA", 7))}, []string{"p"}, 7},
		{"session in the project root itself", "/work/repoA",
			[]store.Skill{tsSkill("p", "proj", "proj", tsProjectEnv("box", "claude", "/work/repoA", 7))}, []string{"p"}, 7},
		{"project skill for another folder is absent but still counts as scanned", "/work/repoA",
			[]store.Skill{tsSkill("p", "proj", "proj", tsProjectEnv("box", "claude", "/work/repoB", 9))}, nil, 9},
		{"prefix trap: /work/repo is not /work/repo2", "/work/repo2",
			[]store.Skill{tsSkill("p", "proj", "proj", tsProjectEnv("box", "claude", "/work/repo", 3))}, nil, 3},
		{"prefix trap the other way", "/work/repo",
			[]store.Skill{tsSkill("p", "proj", "proj", tsProjectEnv("box", "claude", "/work/repo2", 3))}, nil, 3},
		{"trailing slash on the session folder", "/work/repoA/",
			[]store.Skill{tsSkill("p", "proj", "proj", tsProjectEnv("box", "claude", "/work/repoA", 1))}, []string{"p"}, 1},
		{"empty dir hides project skills", "",
			[]store.Skill{tsSkill("p", "proj", "proj", tsProjectEnv("box", "claude", "/work/repoA", 4))}, nil, 4},
		{"global skill is unaffected by the dir", "",
			[]store.Skill{tsSkill("g", "glob", "glob", tsEnv("box", "claude", 2))}, []string{"g"}, 2},
		{"global and unrelated project environments list the skill once, from the global one", "/work/repoA",
			[]store.Skill{tsSkill("g", "both", "both", tsEnv("box", "codex", 2), tsProjectEnv("box", "claude", "/work/repoB", 50))}, []string{"g"}, 50},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildSessionSkills("claude", "box", c.dir, c.skills, nil)
			var ids []string
			for _, s := range got.Skills {
				ids = append(ids, s.ID)
			}
			if strings.Join(ids, ",") != strings.Join(c.wantIDs, ",") {
				t.Fatalf("ids = %v, want %v", ids, c.wantIDs)
			}
			if got.MachineSeen != c.wantSeen {
				t.Fatalf("MachineSeen = %d, want %d", got.MachineSeen, c.wantSeen)
			}
		})
	}
}

func TestBuildSessionSkillsListsAProjectSkillAsNativeAndAGlobalOneFromTheGlobalAgent(t *testing.T) {
	skills := []store.Skill{
		tsSkill("p", "proj", "proj", tsProjectEnv("box", "claude", "/work/repoA", 1)),
		tsSkill("both", "both", "both", tsEnv("box", "codex", 1), tsProjectEnv("box", "claude", "/work/repoA", 1)),
	}
	got := buildSessionSkills("claude", "box", "/work/repoA", skills, nil)
	if len(got.Skills) != 2 {
		t.Fatalf("got %+v", got.Skills)
	}
	for _, s := range got.Skills {
		if !s.Native || s.Agent != "claude" || s.Invocation == "" {
			t.Fatalf("entry %+v", s)
		}
	}
}
