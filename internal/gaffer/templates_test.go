package gaffer

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"github.com/eyupio/reet.sh/internal/store"
)

func copyContent(t *testing.T, c store.GafferTemplateContent) store.GafferTemplateContent {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var out store.GafferTemplateContent
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// skillsAt is a loader that approves each skill at the revision given.
func skillsAt(revisions map[string]string) SkillLoader {
	return func(id string) (skillbuddy.Skill, error) {
		rev, ok := revisions[id]
		if !ok {
			return skillbuddy.Skill{}, errors.New("not in the library")
		}
		return skillbuddy.Skill{ID: id, Name: "Skill " + id, Source: "personal", Revision: rev, Instructions: "Follow " + id + " at " + rev + "."}, nil
	}
}

// finishedOutcome saves a two-pipeline plan with criteria and skills straight
// to the store and marks it finished: one pipeline done, the other accepted in
// its delivery review. It carries everything a run leaves on a record.
func finishedOutcome(t *testing.T, r *outcomeRig) store.GafferOutcome {
	t.Helper()
	auto := true
	in := []OutcomeSession{
		{Title: "Export API", Prompt: "Add the CSV export endpoint with tests.", AutoMerge: &auto, Criteria: []string{"The export downloads as CSV.", "Amounts keep two decimal places."}, Stages: []OutcomeStageInput{
			{Kind: "plan", Tool: "claude", Model: "secret-model", Tier: "heavy", TierWhy: "Money must not be lost."},
			{Kind: "code", Tool: "codex", Part: "Endpoint", Brief: "Write the endpoint."},
			{Kind: "code", Tool: "codex", Part: "Tests", Brief: "Write its tests.", Tier: "light"},
			{Kind: "review", Tool: "opencode"},
			{Kind: "merge", Tool: "opencode"},
		}},
		{Title: "Export button", Prompt: "Add the export button.", Stages: []OutcomeStageInput{{Kind: "code", Tool: "opencode"}, {Kind: "pr", Tool: "opencode"}}},
	}
	pieces, err := outcomePieces(in, "opencode", "", "", store.GafferCrew{}, false, map[string]bool{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	o, err := r.st.CreateGafferOutcomeFor(r.p.UserID, store.GafferOutcome{ID: strings.Repeat("e", 32), Objective: "Ship CSV export for billing.", AgentID: r.project.Host, ProjectID: r.project.ID, ProjectName: "billing", Dir: "/work/billing", Tool: "opencode", Images: []string{"/uploads/secret-image.png"}, MaxReviews: 8, Hours: 24, Sessions: pieces})
	if err != nil {
		t.Fatal(err)
	}
	o, err = r.st.UpdateGafferOutcomeFor(r.p.UserID, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State = "settled"
		for i := range o.Sessions {
			p := &o.Sessions[i]
			p.State, p.WorkDir, p.SessionID, p.ConversationID, p.Note = "done", "/work/billing-wt-secret", "secret-session", "secret-conversation", "Secret note."
			for j := range p.Stages {
				p.Stages[j].State, p.Stages[j].SessionID, p.Stages[j].Summary = "done", "secret-stage-session", "Secret handover."
			}
		}
		o.Sessions[0].Stages[3].SkillGuidance = []skillbuddy.Skill{{ID: "library:review", Name: "Careful review", Source: "personal", Revision: "rev-1", Instructions: "Secret instructions."}}
		o.Sessions[0].Evidence = []store.GafferEvidence{{ID: "secret-evidence", Text: "Secret evidence."}}
		o.Sessions[0].Deliveries = []store.GafferDeliveryDecision{{RequestID: "secret-decision", Decision: "accept", Notes: "Secret decision."}}
		o.Sessions[1].State, o.Sessions[1].DeliveryAccepted = "ready_to_merge", true
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// Only a plan whose every pipeline succeeded is saved, and only what can be
// used again is copied: never the machine, folder, agents, sessions, images,
// notes, evidence or decisions.
func TestATemplateIsSavedFromASuccessfulPlanWithOnlyWhatIsReusable(t *testing.T) {
	r := outcomeFixture(t)
	o := finishedOutcome(t, r)
	// One pipeline still in hand: the plan has not succeeded.
	unfinished, err := r.st.UpdateGafferOutcomeFor(r.p.UserID, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.Sessions[1].DeliveryAccepted = false
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"}); conflictCode(err) != "template_not_successful" {
		t.Fatalf("unfinished plan: %v", err)
	}
	if _, err = r.st.UpdateGafferOutcomeFor(r.p.UserID, o.ID, unfinished.Revision, func(o *store.GafferOutcome) bool {
		o.Sessions[1].DeliveryAccepted = true
		return true
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(2), OutcomeID: o.ID, Name: " CSV export ", Description: "Billing exports."})
	if err != nil {
		t.Fatal(err)
	}
	c := saved.Content
	if saved.Version != 1 || saved.OutcomeID != o.ID || c.Name != "CSV export" || c.Objective != o.Objective || c.MaxReviews != 8 || c.Hours != 24 || len(c.Pipelines) != 2 {
		t.Fatalf("saved: %+v", saved)
	}
	api := c.Pipelines[0]
	if api.Title != "Export API" || api.Prompt != "Add the CSV export endpoint with tests." || !api.AutoMerge || c.Pipelines[1].AutoMerge || len(api.Stages) != 5 ||
		api.Stages[0].Tier != "heavy" || api.Stages[0].TierWhy != "Money must not be lost." || api.Stages[1].Part != "Endpoint" || api.Stages[1].Brief != "Write the endpoint." || api.Stages[2].Tier != "light" || api.Stages[1].Tier != "standard" {
		t.Fatalf("stage layout: %+v", api)
	}
	if len(api.Criteria) != 2 || api.Criteria[0].ID != o.Sessions[0].Criteria[0].ID || api.Criteria[1].Text != "Amounts keep two decimal places." {
		t.Fatalf("criteria: %+v", api.Criteria)
	}
	if sk := api.Stages[3].Skills; len(sk) != 1 || sk[0] != (store.GafferTemplateSkill{ID: "library:review", Name: "Careful review", Source: "personal", Revision: "rev-1"}) {
		t.Fatalf("skills: %+v", sk)
	}
	raw, _ := json.Marshal(saved)
	for _, never := range []string{"secret", "Secret", r.project.Host, r.project.ID, "/work/billing", "claude", "codex", "opencode", "requestId\":\"" + o.Sessions[0].RequestID} {
		if strings.Contains(string(raw), never) {
			t.Fatalf("the template copied %q: %s", never, raw)
		}
	}
	// Another account's outcome cannot be saved, nor this template read.
	other, err := r.st.CreateUser("templates-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.s.SaveTemplate(other.ID, TemplateSaveInput{RequestID: request(3), OutcomeID: o.ID, Name: "Theirs"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another account's outcome: %v", err)
	}
	if _, err = r.s.Template(other.ID, saved.ID, 0, skillsAt(nil)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another account's template: %v", err)
	}
	viewer, err := r.st.CreateUser("templates-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.s.Templates(viewer.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("read-only account: %v", err)
	}
	// The same request again answers the same version; a reused id for something else is refused.
	again, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(2), OutcomeID: o.ID, Name: "CSV export"})
	if err != nil || again.ID != saved.ID || again.Version != 1 {
		t.Fatalf("repeat: %+v %v", again, err)
	}
	if _, err = r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(2), OutcomeID: o.ID, Name: "CSV export", TemplateID: saved.ID, BaseVersion: 1}); conflictCode(err) != "template_request_reused" {
		t.Fatalf("reused request id: %v", err)
	}
	if rows, _ := r.s.Templates(r.p.UserID); len(rows) != 1 || rows[0].Pipelines != 2 || rows[0].Version != 1 {
		t.Fatalf("list: %+v", rows)
	}
}

// Starting a template gives the plan it holds, with each stage's agent from
// the crew saved at that moment, the version recorded and the skills it names
// at their revisions. A repeated request starts nothing more.
func TestALaunchedTemplateIsItsPlanWithTheCrewOfTheMoment(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	o := finishedOutcome(t, r)
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.st.SetGafferCrewFor(r.p.UserID, store.GafferCrew{Standard: store.GafferCrewTier{Tool: "codex", Model: "standard-now"}}); err != nil {
		t.Fatal(err)
	}
	launch := TemplateLaunchInput{RequestID: request(2), Version: 1, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true}
	approved := skillsAt(map[string]string{"library:review": "rev-1"})
	started, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, approved)
	if err != nil {
		t.Fatal(err)
	}
	if started.TemplateID != saved.ID || started.TemplateVersion != 1 || started.AgentID != r.project.Host || started.Objective != saved.Content.Objective || started.MaxReviews != 8 || len(started.Images) != 0 {
		t.Fatalf("launched outcome: %+v", started)
	}
	for i, p := range started.Sessions {
		want := saved.Content.Pipelines[i]
		if p.Title != want.Title || p.Prompt != want.Prompt || p.AutoMerge != want.AutoMerge || len(p.Stages) != len(want.Stages) || len(p.Criteria) != len(want.Criteria) {
			t.Fatalf("pipeline %d: %+v", i, p)
		}
		for j, st := range p.Stages {
			w := want.Stages[j]
			if st.Kind != w.Kind || st.Part != w.Part || st.Brief != w.Brief || st.Tier != w.Tier || st.TierWhy != w.TierWhy {
				t.Fatalf("stage %d.%d: %+v", i, j, st)
			}
			// The crew of the moment: standard is codex now, other tiers are unset and take the outcome's agent.
			if w.Tier == "standard" && (st.Tool != "codex" || st.Model != "standard-now") || w.Tier != "standard" && st.Tool != "opencode" {
				t.Fatalf("stage %d.%d agent: %+v", i, j, st)
			}
		}
		for k, cr := range p.Criteria {
			if cr.ID != want.Criteria[k].ID || cr.Text != want.Criteria[k].Text {
				t.Fatalf("criterion %d.%d: %+v", i, k, cr)
			}
		}
	}
	if g := started.Sessions[0].Stages[3].SkillGuidance; len(g) != 1 || g[0].Revision != "rev-1" || g[0].Instructions == "" {
		t.Fatalf("skill guidance: %+v", g)
	}
	again, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, approved)
	if err != nil || again.ID != started.ID {
		t.Fatalf("repeated launch: %+v %v", again, err)
	}
	if rows, _ := r.st.GafferOutcomesFor(r.p.UserID); len(rows) != 2 {
		t.Fatalf("a repeated launch started another outcome: %d", len(rows))
	}
	if view, err := r.s.Template(r.p.UserID, saved.ID, 0, approved); err != nil || view.LastUsedVersion != 1 || len(view.SkillGaps) != 0 {
		t.Fatalf("view: %+v %v", view, err)
	}
	// An outcome approved the ordinary way records no template.
	plain, err := r.s.CreateOutcome(r.p.UserID, r.in)
	if err != nil || plain.TemplateID != "" || plain.TemplateVersion != 0 {
		t.Fatalf("ordinary outcome: %+v %v", plain, err)
	}
}

// A skill that is no longer approved at the template's revision is never
// replaced: the launch is refused until the person agrees to start without
// it, and then the stage simply does not have it.
func TestASkillNoLongerApprovedIsShownAndNeverSubstituted(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	o := finishedOutcome(t, r)
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"})
	if err != nil {
		t.Fatal(err)
	}
	newer := skillsAt(map[string]string{"library:review": "rev-2"})
	view, err := r.s.Template(r.p.UserID, saved.ID, 1, newer)
	if err != nil || len(view.SkillGaps) != 1 || view.SkillGaps[0].Status != "changed" || view.SkillGaps[0].Key != "0:3:library:review" {
		t.Fatalf("gaps: %+v %v", view.SkillGaps, err)
	}
	if gone, _ := r.s.Template(r.p.UserID, saved.ID, 1, skillsAt(nil)); len(gone.SkillGaps) != 1 || gone.SkillGaps[0].Status != "unavailable" {
		t.Fatalf("unavailable: %+v", gone.SkillGaps)
	}
	launch := TemplateLaunchInput{RequestID: request(2), Version: 1, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true}
	if _, err = r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, newer); conflictCode(err) != "template_skills_changed" {
		t.Fatalf("launch over a changed skill: %v", err)
	}
	launch.SkippedSkills = []string{"0:3:library:review"}
	started, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, newer)
	if err != nil {
		t.Fatal(err)
	}
	if g := started.Sessions[0].Stages[3].SkillGuidance; len(g) != 0 {
		t.Fatalf("a skill at another revision was used: %+v", g)
	}
	// Agreeing to skip a skill that is in fact still approved is refused too.
	launch.RequestID = request(3)
	if _, err = r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, skillsAt(map[string]string{"library:review": "rev-1"})); conflictCode(err) != "template_skills_changed" {
		t.Fatalf("skip of an approved skill: %v", err)
	}
}

// A run keeps what it was started on: changing the template and deleting it
// leave the outcome's plan and recorded version as they were.
func TestARunKeepsItsInputsWhenItsTemplateChangesOrGoes(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	o := finishedOutcome(t, r)
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"})
	if err != nil {
		t.Fatal(err)
	}
	started, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, TemplateLaunchInput{RequestID: request(2), Version: 1, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true}, skillsAt(map[string]string{"library:review": "rev-1"}))
	if err != nil {
		t.Fatal(err)
	}
	content := copyContent(t, saved.Content)
	content.Pipelines[0].Prompt = "Add the CSV and JSON export endpoints."
	content.Pipelines[0].Criteria[0].Text = "The export downloads as CSV or JSON."
	content.Pipelines[0].Criteria = append(content.Pipelines[0].Criteria[:1], store.GafferTemplateCriterion{Text: "Large exports stream."})
	content.Pipelines[0].Stages[3].Skills[0].Revision = "rev-2"
	content.Pipelines[1].Stages = append(content.Pipelines[1].Stages, store.GafferTemplateStage{Kind: "babysit"})
	content.Pipelines = append(content.Pipelines, store.GafferTemplatePipeline{Title: "Export docs", Prompt: "Document the export.", Stages: []store.GafferTemplateStage{{Kind: "code"}}})
	edited, err := r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(3), BaseVersion: 1, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Version != 2 || edited.Content.Pipelines[0].Criteria[0].ID != saved.Content.Pipelines[0].Criteria[0].ID || edited.Content.Pipelines[0].Criteria[1].ID == "" || edited.Content.Pipelines[2].Stages[0].Tier != "standard" {
		t.Fatalf("edited: %+v", edited)
	}
	// An edit on the version it replaced is refused, and the first version still reads as it was.
	if _, err = r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(4), BaseVersion: 1, Content: content}); !errors.Is(err, store.ErrGafferTemplateChanged) {
		t.Fatalf("stale edit: %v", err)
	}
	if first, _ := r.s.Template(r.p.UserID, saved.ID, 1, skillsAt(nil)); first.Content.Pipelines[0].Prompt != saved.Content.Pipelines[0].Prompt || first.Latest != 2 {
		t.Fatalf("first version changed: %+v", first)
	}
	d, err := r.s.TemplateDiff(r.p.UserID, saved.ID, 0, 0)
	if err != nil || d.From != 1 || d.To != 2 || !d.Changed {
		t.Fatalf("diff from the version last used: %+v %v", d, err)
	}
	api, button, docs := d.Pipelines[0], d.Pipelines[1], d.Pipelines[2]
	if api.Change != "changed" || !api.PromptChanged || api.StagesChanged || len(api.Criteria.Reworded) != 1 || api.Criteria.Reworded[0].To != "The export downloads as CSV or JSON." ||
		len(api.Criteria.Added) != 1 || len(api.Criteria.Removed) != 1 || api.Criteria.Removed[0] != "Amounts keep two decimal places." ||
		len(api.Skills) != 1 || api.Skills[0].From != "rev-1" || api.Skills[0].To != "rev-2" || api.Skills[0].Stage != "review" {
		t.Fatalf("Export API: %+v", api)
	}
	if button.Change != "changed" || !button.StagesChanged || strings.Join(button.StagesAfter, ",") != "code,pr,babysit" || docs.Change != "added" {
		t.Fatalf("other pipelines: %+v %+v", button, docs)
	}
	if err = r.s.DeleteTemplate(r.p.UserID, saved.ID, 1); !errors.Is(err, store.ErrGafferTemplateChanged) {
		t.Fatalf("delete on an older version: %v", err)
	}
	if err = r.s.DeleteTemplate(r.p.UserID, saved.ID, 2); err != nil {
		t.Fatal(err)
	}
	after := mustOutcome(t, r, started.ID)
	if after.TemplateID != saved.ID || after.TemplateVersion != 1 || after.Sessions[0].Prompt != saved.Content.Pipelines[0].Prompt || len(after.Sessions[0].Criteria) != 2 || after.Sessions[0].Criteria[1].Text != "Amounts keep two decimal places." || len(after.Sessions) != 2 {
		t.Fatalf("the run changed with its template: %+v", after)
	}
	if _, err = r.s.Template(r.p.UserID, saved.ID, 1, skillsAt(nil)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted template: %v", err)
	}
}

// An edit is checked as a plan an approval would take, and a criterion id it
// does not know is refused.
func TestATemplateEditIsCheckedLikeAPlan(t *testing.T) {
	r := outcomeFixture(t)
	o := finishedOutcome(t, r)
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"})
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*store.GafferTemplateContent){
		"stages out of order": func(c *store.GafferTemplateContent) {
			c.Pipelines[1].Stages = []store.GafferTemplateStage{{Kind: "pr"}, {Kind: "code"}}
		},
		"unknown criterion": func(c *store.GafferTemplateContent) {
			c.Pipelines[0].Criteria[0].ID = strings.Repeat("f", 32)
		},
		"no stages": func(c *store.GafferTemplateContent) { c.Pipelines[1].Stages = nil },
		"three skills": func(c *store.GafferTemplateContent) {
			c.Pipelines[0].Stages[3].Skills = append(c.Pipelines[0].Stages[3].Skills, c.Pipelines[0].Stages[3].Skills[0], c.Pipelines[0].Stages[3].Skills[0])
		},
		"bad tier":   func(c *store.GafferTemplateContent) { c.Pipelines[0].Stages[0].Tier = "extreme" },
		"empty name": func(c *store.GafferTemplateContent) { c.Name = " " },
		"six pipelines": func(c *store.GafferTemplateContent) {
			c.Pipelines = append(c.Pipelines, c.Pipelines[1], c.Pipelines[1], c.Pipelines[1], c.Pipelines[1])
		},
	} {
		content := copyContent(t, saved.Content)
		change(&content)
		var bad *Invalid
		if _, err := r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(2), BaseVersion: 1, Content: content}); !errors.As(err, &bad) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if versions, _ := r.st.GafferTemplateVersionsFor(r.p.UserID, saved.ID); len(versions) != 1 {
		t.Fatalf("a refused edit saved a version: %v", versions)
	}
}
