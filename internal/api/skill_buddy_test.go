package api

import (
	"encoding/json"
	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"strings"
	"testing"
)

func TestSkillBuddySessionOwnershipAndReceipts(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, session, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp", "title": "Review security authentication"}))
	if code != 200 {
		t.Fatalf("create %d %s", code, body)
	}
	id := session["id"].(string)
	code, out, body := f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/recommendations", map[string]any{"sessionId": id})
	if code != 200 || len(out["skills"].([]any)) == 0 {
		t.Fatalf("recommend %d %s", code, body)
	}
	for _, v := range out["skills"].([]any) {
		if v.(map[string]any)["instructions"] != nil {
			t.Fatal("eager instruction disclosure")
		}
	}
	code, out, body = f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", map[string]any{"sessionId": id, "skillId": "builtin:review", "action": "drafted"})
	if code != 200 || !strings.Contains(out["text"].(string), "grants no additional permissions") {
		t.Fatalf("choose %d %s", code, body)
	}
	for range 2 {
		f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/recommendations", map[string]any{"sessionId": id})
	}
	rows, err := f.st.SkillBuddyRecordsFor(f.uid, "session:"+id)
	if err != nil {
		t.Fatal(err)
	}
	chosen := 0
	for _, r := range rows {
		if r.Skill.ID == "builtin:review" && r.Skill.State == "drafted" {
			chosen++
		}
	}
	if chosen != 1 {
		t.Fatalf("refresh lost or duplicated choice: %+v", rows)
	}
	code, _, _ = f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", map[string]any{"sessionId": terminalbroker.ID(), "skillId": "builtin:review", "action": "drafted"})
	if code != 404 {
		t.Fatalf("unknown session %d", code)
	}
	code, _, _ = f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", map[string]any{"sessionId": id, "skillId": "community:any", "action": "drafted"})
	if code != 400 {
		t.Fatalf("community executed %d", code)
	}
	// A saved skill too long for the composer says so when reviewed, and a
	// missing one is reported plainly rather than as a storage error.
	base := ""
	if _, err = f.st.SaveLibraryContentFor(f.uid, "", store.PrivateSkillWrite{RequestID: strings.Repeat("c", 32), ExpectedBase: &base, Name: "long", Files: []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: long\ndescription: Long\n---\n" + strings.Repeat("x", 3000))}}}); err != nil {
		t.Fatal(err)
	}
	items, err := f.st.LibraryFor(f.uid)
	if err != nil || len(items) != 1 {
		t.Fatalf("library %v %+v", err, items)
	}
	code, out, body = f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", map[string]any{"sessionId": id, "skillId": "library:" + items[0].ID, "action": "inspect"})
	if code != 200 || out["tooLong"] != true {
		t.Fatalf("long skill inspect %d %s", code, body)
	}
	code, _, body = f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", map[string]any{"sessionId": id, "skillId": "library:gone", "action": "inspect"})
	if code != 404 || strings.Contains(body, "private skill") || !strings.Contains(body, "no longer in your library") {
		t.Fatalf("missing skill %d %s", code, body)
	}
}

func TestSkillBuddyAdminOverridesAreScopedAndRevisionChecked(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	// The launch fixture's owner can maintain the instance defaults.
	code, out, body := f.do(t, f.cookie, "GET", "/api/v2/admin/system-skills", nil)
	if code != 200 {
		t.Fatalf("admin read %d %s", code, body)
	}
	sk := skillbuddy.Builtins()[0]
	sk.Instructions = "Use the team's planning checklist."
	version := out["version"]
	code, _, body = f.do(t, f.cookie, "PUT", "/api/v2/admin/system-skills", map[string]any{"version": version, "skill": sk})
	if code != 200 {
		t.Fatalf("save %d %s", code, body)
	}
	got, _, err := f.srv.s.buddySystem()
	if err != nil || got[0].Instructions != sk.Instructions || got[0].Revision == "1" {
		t.Fatalf("override %+v %v", got, err)
	}
	if skillbuddy.Builtins()[0].Instructions == sk.Instructions {
		t.Fatal("repo default changed")
	}
	code, _, _ = f.do(t, f.cookie, "PUT", "/api/v2/admin/system-skills", map[string]any{"version": version, "skill": sk})
	if code != 409 {
		t.Fatalf("stale write %d", code)
	}
	if err = f.st.UpdateUserRole(f.uid, "member"); err != nil {
		t.Fatal(err)
	}
	code, _, _ = f.do(t, f.cookie, "GET", "/api/v2/admin/system-skills", nil)
	if code != 403 {
		t.Fatalf("member admin read %d", code)
	}
}

func TestSkillBuddyMCPProgressiveLoadingAndAccountIsolation(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	access := mcpAccess{User: store.User{ID: f.uid, Role: "owner"}}
	out, err := f.srv.s.executeMCPBuddy(access, "reet_recommend_skills", map[string]any{"task": "Review authentication security", "stage": "review"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range out["skills"].([]skillbuddy.Skill) {
		if sk.Instructions != "" {
			t.Fatal("recommendations loaded instructions")
		}
	}
	out, err = f.srv.s.executeMCPBuddy(access, "reet_load_skill", map[string]any{"id": "builtin:review"})
	if err != nil || out["guidance"] == "" {
		t.Fatalf("load %v %v", out, err)
	}
	if _, err = f.srv.s.executeMCPBuddy(access, "reet_load_skill", map[string]any{"id": "library:foreign"}); err == nil {
		t.Fatal("foreign content loaded")
	}
	access.User.Role = "viewer"
	if _, err = f.srv.s.executeMCPBuddy(access, "reet_load_skill", map[string]any{"id": "builtin:review"}); err == nil {
		t.Fatal("viewer loaded skill")
	}
}

func TestSkillBuddyFutureStagePinsReviewedGuidanceAndRefusesRunningStage(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	o, err := f.st.CreateGafferOutcomeFor(f.uid, store.GafferOutcome{ID: strings.Repeat("b", 32), Objective: "Ship the fix", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Fix login", Prompt: "Fix login and verify it", Stages: []store.GafferOutcomeStage{{Kind: "code"}, {Kind: "review"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"outcomeId": o.ID, "pipeline": "a", "stage": 1, "skillId": "builtin:review", "action": "selected", "revision": "1"}
	code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", body)
	if code != 200 {
		t.Fatalf("select %d %s", code, raw)
	}
	o, err = f.st.GafferOutcomeFor(f.uid, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	selected := o.Sessions[0].Stages[1].SkillGuidance
	if len(selected) != 1 || selected[0].Revision != "1" || selected[0].Instructions == "" {
		t.Fatalf("not pinned: %+v", selected)
	}
	// A later system edit cannot rewrite the snapshot already selected.
	changed := reviewBuiltin()
	changed.Instructions = "Different future guidance."
	changed.Revision = "2"
	config, _ := json.Marshal(buddySystemConfig{Version: 1, Overrides: []skillbuddy.Skill{changed}})
	if err = f.st.SetSetting(buddySystemKey, string(config)); err != nil {
		t.Fatal(err)
	}
	code, _, _ = f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", body)
	if code != 409 {
		t.Fatalf("changed preview accepted %d", code)
	}
	o, _ = f.st.GafferOutcomeFor(f.uid, o.ID)
	if o.Sessions[0].Stages[1].SkillGuidance[0].Instructions != selected[0].Instructions {
		t.Fatal("selected guidance changed")
	}
	changed.Disabled = true
	config, _ = json.Marshal(buddySystemConfig{Version: 2, Overrides: []skillbuddy.Skill{changed}})
	if err = f.st.SetSetting(buddySystemKey, string(config)); err != nil {
		t.Fatal(err)
	}
	body["action"] = "dismissed"
	code, _, raw = f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", body)
	if code != 200 {
		t.Fatalf("disabled selection could not be removed: %d %s", code, raw)
	}
	o, _ = f.st.GafferOutcomeFor(f.uid, o.ID)
	if len(o.Sessions[0].Stages[1].SkillGuidance) != 0 {
		t.Fatal("removed guidance retained")
	}
	body["stage"] = 0
	body["skillId"] = "builtin:code"
	body["revision"] = "1"
	code, _, _ = f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", body)
	if code != 409 {
		t.Fatalf("current stage rewritten %d", code)
	}
}

func reviewBuiltin() skillbuddy.Skill {
	for _, sk := range skillbuddy.Builtins() {
		if sk.ID == "builtin:review" {
			return sk
		}
	}
	panic("missing review skill")
}

func TestImpeccableDistillInSessionAndFactory(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, session, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp", "title": "Home page",
	}))
	if code != 200 {
		t.Fatalf("create session %d %s", code, raw)
	}
	o, err := f.st.CreateGafferOutcomeFor(f.uid, store.GafferOutcome{
		ID: strings.Repeat("d", 32), Objective: "Remove duplicate actions from the home page",
		Sessions: []store.GafferOutcomeSession{{
			Key: "a", Title: "Home page", Prompt: "Improve the empty state",
			Stages: []store.GafferOutcomeStage{{Kind: "plan"}, {Kind: "design"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	const skillID = "builtin:impeccable-distill"
	for _, tc := range []struct {
		name, action string
		context      map[string]any
	}{
		{"session", "drafted", map[string]any{"sessionId": session["id"], "task": "Remove duplicate actions"}},
		{"factory", "selected", map[string]any{"outcomeId": o.ID, "pipeline": "a", "stage": 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, result, raw := f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/recommendations", tc.context)
			if code != 200 {
				t.Fatalf("recommend %d %s", code, raw)
			}
			for _, item := range result["skills"].([]any) {
				sk := item.(map[string]any)
				if sk["id"] == skillID {
					tc.context["revision"] = sk["revision"]
					break
				}
			}
			if tc.context["revision"] == nil {
				t.Fatalf("missing %s: %s", skillID, raw)
			}
			tc.context["skillId"] = skillID
			tc.context["action"] = tc.action
			code, result, raw = f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/choose", tc.context)
			if code != 200 || !strings.Contains(result["text"].(string), "Consolidate where possible") {
				t.Fatalf("choose %d %s", code, raw)
			}
		})
	}
	saved, err := f.st.GafferOutcomeFor(f.uid, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	pinned := saved.Sessions[0].Stages[1].SkillGuidance
	if len(pinned) != 1 || pinned[0].ID != skillID || pinned[0].Instructions == "" {
		t.Fatalf("guidance was not pinned: %+v", pinned)
	}
}

func TestSkillBuddyRanksACustomStageByItsPrompt(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	o, err := f.st.CreateGafferOutcomeFor(f.uid, store.GafferOutcome{ID: strings.Repeat("d", 32), Objective: "Ship", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Work", Prompt: "Do the work", PathwayID: "pw1", PathwayName: "Careful", Stages: []store.GafferOutcomeStage{{Kind: "code", State: "running"}, {Kind: "custom", Name: "Audit", Prompt: "Check security and authentication for holes."}}}}})
	if err != nil {
		t.Fatal(err)
	}
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/skill-buddy/recommendations", map[string]any{"outcomeId": o.ID, "pipeline": "a", "stage": 1})
	skills, _ := out["skills"].([]any)
	if code != 200 || len(skills) == 0 || out["stage"] != "" {
		t.Fatalf("recommend %d %s", code, raw)
	}
	at := map[string]int{}
	for i, v := range skills {
		at[v.(map[string]any)["id"].(string)] = i + 1
	}
	if at["builtin:upstream-security-diff"] == 0 || at["builtin:upstream-ponytail"] != 0 && at["builtin:upstream-security-diff"] > at["builtin:upstream-ponytail"] {
		t.Fatalf("a security stage did not rank the security skill above an unrelated one: %s", raw)
	}
}
