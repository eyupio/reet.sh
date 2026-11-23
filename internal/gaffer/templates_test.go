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
	again, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(2), OutcomeID: o.ID, Name: "CSV export", Description: "Billing exports."})
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
	started, _, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, approved)
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
	again, _, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, approved)
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
	if _, _, err = r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, newer); conflictCode(err) != "template_skills_changed" {
		t.Fatalf("launch over a changed skill: %v", err)
	}
	launch.SkippedSkills = []string{"0:3:library:review"}
	started, _, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, newer)
	if err != nil {
		t.Fatal(err)
	}
	if g := started.Sessions[0].Stages[3].SkillGuidance; len(g) != 0 {
		t.Fatalf("a skill at another revision was used: %+v", g)
	}
	// Agreeing to skip a skill that is in fact still approved is refused too.
	launch.RequestID = request(3)
	if _, _, err = r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, skillsAt(map[string]string{"library:review": "rev-1"})); conflictCode(err) != "template_skills_changed" {
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
	started, _, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, TemplateLaunchInput{RequestID: request(2), Version: 1, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true}, skillsAt(map[string]string{"library:review": "rev-1"}))
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

// A repeated save or edit answers the version it made, even after the
// outcome or the template moved on; the same id with other content, or an id
// a launch used, is refused, and a launch may not take a save's id.
func TestARepeatedTemplateRequestAnswersWhatItMade(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	o := finishedOutcome(t, r)
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export", Description: "Billing."})
	if err != nil {
		t.Fatal(err)
	}
	// The pipeline is sent back after the save: the plan is no longer saveable, but the save it made stands.
	o = mustOutcome(t, r, o.ID)
	if _, err = r.st.UpdateGafferOutcomeFor(r.p.UserID, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.Sessions[1].DeliveryAccepted = false
		return true
	}); err != nil {
		t.Fatal(err)
	}
	again, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export", Description: "Billing."})
	if err != nil || again.ID != saved.ID || again.Version != 1 {
		t.Fatalf("repeated save after the outcome changed: %+v %v", again, err)
	}
	for name, in := range map[string]TemplateSaveInput{
		"another name":        {RequestID: request(1), OutcomeID: o.ID, Name: "CSV export, again", Description: "Billing."},
		"another description": {RequestID: request(1), OutcomeID: o.ID, Name: "CSV export", Description: "Something else."},
	} {
		if _, err = r.s.SaveTemplate(r.p.UserID, in); conflictCode(err) != "template_request_reused" {
			t.Fatalf("save under a used id with %s: %v", name, err)
		}
	}
	// Edit on version 1 makes version 2; a later edit removes a criterion. The first edit again is still version 2.
	content := copyContent(t, saved.Content)
	content.Objective = "Ship CSV export for billing and finance."
	second, err := r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(2), BaseVersion: 1, Content: content})
	if err != nil || second.Version != 2 {
		t.Fatalf("edit: %+v %v", second, err)
	}
	third := copyContent(t, second.Content)
	third.Pipelines[0].Criteria = third.Pipelines[0].Criteria[:1]
	if _, err = r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(3), BaseVersion: 2, Content: third}); err != nil {
		t.Fatal(err)
	}
	if again, err := r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(2), BaseVersion: 1, Content: content}); err != nil || again.Version != 2 {
		t.Fatalf("repeated edit after a later one: %+v %v", again, err)
	}
	other := copyContent(t, content)
	other.Objective = "Ship something else."
	if _, err = r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(2), BaseVersion: 1, Content: other}); conflictCode(err) != "template_request_reused" {
		t.Fatalf("edit under a used id with other content: %v", err)
	}
	// A launch may not take a save's or an edit's id, nor a save or edit a launch's.
	approved := skillsAt(map[string]string{"library:review": "rev-1"})
	for _, id := range []string{request(1), request(2)} {
		if _, _, err = r.s.LaunchTemplate(r.p.UserID, saved.ID, TemplateLaunchInput{RequestID: id, Version: 3, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true}, approved); conflictCode(err) != "template_request_reused" {
			t.Fatalf("launch under a template write's id: %v", err)
		}
	}
	started, created, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, TemplateLaunchInput{RequestID: request(4), Version: 3, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true}, approved)
	if err != nil || !created {
		t.Fatalf("launch: %v %v", created, err)
	}
	if again, created, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, TemplateLaunchInput{RequestID: request(4), Version: 3, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true}, approved); err != nil || created || again.ID != started.ID {
		t.Fatalf("repeated launch: %v %v", created, err)
	}
	if _, err = r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(4), OutcomeID: o.ID, Name: "CSV export"}); conflictCode(err) != "template_request_reused" {
		t.Fatalf("save under a launch's id: %v", err)
	}
	if _, err = r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(4), BaseVersion: 3, Content: third}); conflictCode(err) != "template_request_reused" {
		t.Fatalf("edit under a launch's id: %v", err)
	}
}

// A pipeline whose delivery was accepted and which then failed, was
// unfollowed or was cancelled did not succeed.
func TestAnAcceptedPipelineThatThenFailedIsNotSuccessful(t *testing.T) {
	r := outcomeFixture(t)
	o := finishedOutcome(t, r)
	for n, state := range []string{"failed", "unfollowed", "cancelled"} {
		o = mustOutcome(t, r, o.ID)
		if _, err := r.st.UpdateGafferOutcomeFor(r.p.UserID, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
			o.State, o.Sessions[1].State, o.Sessions[1].DeliveryAccepted = "stopped", state, true
			return true
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(byte(n + 1)), OutcomeID: o.ID, Name: "CSV export"}); conflictCode(err) != "template_not_successful" {
			t.Fatalf("accepted then %s: %v", state, err)
		}
	}
}

// The crew chosen in the launch does the stages, tier by tier, and is checked
// as an approval checks it. The run says when the template was last used and
// keeps the template's name after the template goes.
func TestALaunchTakesItsCrewAndTheRunKeepsTheTemplateName(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	o := finishedOutcome(t, r)
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.st.SetGafferCrewFor(r.p.UserID, store.GafferCrew{Standard: store.GafferCrewTier{Tool: "codex", Model: "standard-saved"}}); err != nil {
		t.Fatal(err)
	}
	approved := skillsAt(map[string]string{"library:review": "rev-1"})
	if rows, _ := r.s.Templates(r.p.UserID); len(rows) != 1 || rows[0].LastUsedAt != 0 {
		t.Fatalf("never used: %+v", rows)
	}
	bad := TemplateLaunchInput{RequestID: request(2), Version: 1, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true, Crew: &TemplateCrew{Heavy: store.GafferCrewTier{Tool: "not-an-agent"}}}
	var invalidCrew *Invalid
	if _, _, err = r.s.LaunchTemplate(r.p.UserID, saved.ID, bad, approved); !errors.As(err, &invalidCrew) {
		t.Fatalf("a crew no approval would take: %v", err)
	}
	launch := TemplateLaunchInput{RequestID: request(3), Version: 1, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true, Crew: &TemplateCrew{
		Heavy: store.GafferCrewTier{Tool: "claude", Model: "heavy-now", Effort: "high", Backup: &store.GafferStageAgent{Tool: "codex", Model: "heavy-backup"}},
		Light: store.GafferCrewTier{Tool: "codex", Model: "light-now"},
	}}
	started, _, err := r.s.LaunchTemplate(r.p.UserID, saved.ID, launch, approved)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range started.Sessions {
		for j, st := range p.Stages {
			switch st.Tier {
			case "heavy":
				if st.Tool != "claude" || st.Model != "heavy-now" || st.Effort != "high" || st.Backup == nil || st.Backup.Tool != "codex" {
					t.Fatalf("heavy stage %d.%d: %+v", i, j, st)
				}
			case "light":
				if st.Tool != "codex" || st.Model != "light-now" {
					t.Fatalf("light stage %d.%d: %+v", i, j, st)
				}
			default:
				// The launch left standard empty: the outcome's agent, not the saved crew's.
				if st.Tool != "opencode" || st.Model != "" {
					t.Fatalf("standard stage %d.%d: %+v", i, j, st)
				}
			}
		}
	}
	if started.TemplateName != "CSV export" || started.TemplateID != saved.ID || started.TemplateVersion != 1 {
		t.Fatalf("template on the run: %+v", started)
	}
	if rows, _ := r.s.Templates(r.p.UserID); len(rows) != 1 || rows[0].LastUsedAt != started.CreatedAt || rows[0].LastUsedVersion != 1 {
		t.Fatalf("summary after a launch: %+v", rows)
	}
	if view, _ := r.s.Template(r.p.UserID, saved.ID, 0, approved); view.LastUsedAt != started.CreatedAt {
		t.Fatalf("view after a launch: %+v", view)
	}
	if err = r.s.DeleteTemplate(r.p.UserID, saved.ID, 1); err != nil {
		t.Fatal(err)
	}
	if after := mustOutcome(t, r, started.ID); after.TemplateName != "CSV export" {
		t.Fatalf("the run forgot its template's name: %+v", after)
	}
}

// The changes shown before a start reach back to the oldest version kept when
// the one last used has been dropped, and say that older changes are not shown.
func TestTheDefaultDiffFallsBackWhenTheLastUsedVersionIsDropped(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	o := finishedOutcome(t, r)
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.s.LaunchTemplate(r.p.UserID, saved.ID, TemplateLaunchInput{RequestID: request(2), Version: 1, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true}, skillsAt(map[string]string{"library:review": "rev-1"})); err != nil {
		t.Fatal(err)
	}
	if d, err := r.s.TemplateDiff(r.p.UserID, saved.ID, 0, 0); err != nil || d.From != 1 || d.OlderNotShown {
		t.Fatalf("diff with the last used version kept: %+v %v", d, err)
	}
	content := copyContent(t, saved.Content)
	for v := 1; v <= store.MaxGafferTemplateVersions; v++ {
		content.Description = "Change " + strings.Repeat("x", v)
		id := strings.Repeat("a", 30) + string(rune('a'+v/10)) + string(rune('0'+v%10))
		if _, err = r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: id, BaseVersion: v, Content: content}); err != nil {
			t.Fatal(err)
		}
	}
	d, err := r.s.TemplateDiff(r.p.UserID, saved.ID, 0, 0)
	if err != nil || d.From != 2 || d.To != 21 || !d.OlderNotShown || !d.DescriptionChanged {
		t.Fatalf("diff with the last used version dropped: %+v %v", d, err)
	}
}

// Renaming a template whose skill is no longer approved at its revision still
// saves: the gap is the launch's to deal with, not the rename's. The same
// skill twice in one stage is refused, text is stored trimmed, and the diff
// reports pipelines put in another order.
func TestARenameKeepsAGapAndAnEditIsTidied(t *testing.T) {
	r := outcomeFixture(t)
	o := finishedOutcome(t, r)
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"})
	if err != nil {
		t.Fatal(err)
	}
	newer := skillsAt(map[string]string{"library:review": "rev-2"})
	if view, _ := r.s.Template(r.p.UserID, saved.ID, 0, newer); len(view.SkillGaps) != 1 {
		t.Fatalf("gap: %+v", view.SkillGaps)
	}
	renamed := copyContent(t, saved.Content)
	renamed.Name = "CSV export for billing"
	got, err := r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(2), BaseVersion: 1, Content: renamed})
	if err != nil || got.Version != 2 || got.Content.Pipelines[0].Stages[3].Skills[0].Revision != "rev-1" {
		t.Fatalf("rename with a skill gap: %+v %v", got, err)
	}
	twice := copyContent(t, got.Content)
	twice.Pipelines[0].Stages[3].Skills = append(twice.Pipelines[0].Stages[3].Skills, twice.Pipelines[0].Stages[3].Skills[0])
	var bad *Invalid
	if _, err = r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(3), BaseVersion: 2, Content: twice}); !errors.As(err, &bad) || bad.Field != "skills" {
		t.Fatalf("the same skill twice: %v", err)
	}
	spaced := copyContent(t, got.Content)
	p := &spaced.Pipelines[0]
	p.Title, p.Prompt = " "+p.Title+" ", p.Prompt+"\n"
	p.Stages[0].TierWhy = " " + p.Stages[0].TierWhy + " "
	p.Stages[1].Part, p.Stages[1].Brief = p.Stages[1].Part+" ", " "+p.Stages[1].Brief
	spaced.Pipelines[0], spaced.Pipelines[1] = spaced.Pipelines[1], spaced.Pipelines[0]
	tidy, err := r.s.EditTemplate(r.p.UserID, saved.ID, TemplateEditInput{RequestID: request(4), BaseVersion: 2, Content: spaced})
	if err != nil {
		t.Fatal(err)
	}
	api := tidy.Content.Pipelines[1]
	if api.Title != "Export API" || api.Prompt != "Add the CSV export endpoint with tests." || api.Stages[0].TierWhy != "Money must not be lost." || api.Stages[1].Part != "Endpoint" || api.Stages[1].Brief != "Write the endpoint." {
		t.Fatalf("untrimmed: %+v", api)
	}
	d, err := r.s.TemplateDiff(r.p.UserID, saved.ID, 2, 3)
	if err != nil || !d.Changed || !d.OrderChanged || d.Pipelines[0].Change != "same" || d.Pipelines[1].Change != "same" {
		t.Fatalf("reordered pipelines: %+v %v", d, err)
	}
}

// A launch refused because the skills changed carries the gaps as they are now.
func TestASkillsChangedRefusalCarriesTheCurrentGaps(t *testing.T) {
	r := outcomeFixture(t)
	o := finishedOutcome(t, r)
	saved, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.s.LaunchTemplate(r.p.UserID, saved.ID, TemplateLaunchInput{RequestID: request(2), Version: 1, ProjectID: r.project.ID, Tool: "opencode", Confirmed: true}, skillsAt(nil))
	var changed *TemplateSkillsChanged
	if !errors.As(err, &changed) || conflictCode(err) != "template_skills_changed" || len(changed.Gaps) != 1 || changed.Gaps[0].Key != "0:3:library:review" || changed.Gaps[0].Status != "unavailable" {
		t.Fatalf("skills changed: %#v", err)
	}
}

// A stage whose tier, brief or part changed is named with what it was and is,
// so a pipeline marked changed always has something to say.
func TestTheDiffNamesAStageWhoseTierBriefOrPartChanged(t *testing.T) {
	stage := func(kind, part, brief, tier string) store.GafferTemplateStage {
		return store.GafferTemplateStage{Kind: kind, Part: part, Brief: brief, Tier: tier}
	}
	a := store.GafferTemplateContent{Pipelines: []store.GafferTemplatePipeline{{Title: "Export API",
		Stages: []store.GafferTemplateStage{stage("code", "Endpoint", "Write the endpoint.", "standard"), stage("review", "", "", "standard")}}}}
	b := copyContent(t, a)
	b.Pipelines[0].Stages[0] = stage("code", "API", "", "standard")
	b.Pipelines[0].Stages[1].Tier = "heavy"
	p := diffTemplates(a, b).Pipelines[0]
	want := []TemplateStageEdit{
		{Stage: "code", Field: "part", From: "Endpoint", To: "API"},
		{Stage: "code: API", Field: "brief", From: "Write the endpoint.", To: ""},
		{Stage: "review", Field: "tier", From: "standard", To: "heavy"},
	}
	if p.Change != "changed" || !p.StagesChanged || len(p.StageEdits) != len(want) {
		t.Fatalf("diff: %+v", p)
	}
	for n := range want {
		if p.StageEdits[n] != want[n] {
			t.Fatalf("edit %d: %+v, want %+v", n, p.StageEdits[n], want[n])
		}
	}
	// A stage of another kind in its place is a new layout, said by the stage lists, not an edit.
	c := copyContent(t, a)
	c.Pipelines[0].Stages[1] = stage("test", "", "", "light")
	if p := diffTemplates(a, c).Pipelines[0]; len(p.StageEdits) != 0 || !p.StagesChanged {
		t.Fatalf("another kind: %+v", p)
	}
	if p := diffTemplates(a, copyContent(t, a)).Pipelines[0]; p.Change != "same" || p.StageEdits == nil || len(p.StageEdits) != 0 {
		t.Fatalf("same: %+v", p)
	}
}

// A refusal names the pipeline that did not finish well, so the person knows which.
func TestANotSuccessfulRefusalNamesThePipeline(t *testing.T) {
	r := outcomeFixture(t)
	o := finishedOutcome(t, r)
	if _, err := r.st.UpdateGafferOutcomeFor(r.p.UserID, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.Sessions[1].DeliveryAccepted = false
		return true
	}); err != nil {
		t.Fatal(err)
	}
	_, err := r.s.SaveTemplate(r.p.UserID, TemplateSaveInput{RequestID: request(1), OutcomeID: o.ID, Name: "CSV export"})
	if conflictCode(err) != "template_not_successful" || !strings.Contains(err.Error(), "“"+o.Sessions[1].Title+"” did not finish well") {
		t.Fatalf("refusal: %v", err)
	}
}
