package gaffer

import (
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"github.com/eyupio/reet.sh/internal/store"
)

func pst(kind string) store.GafferPathwayStage { return store.GafferPathwayStage{Kind: kind} }
func pcustom(name string) store.GafferPathwayStage {
	return store.GafferPathwayStage{Kind: CustomStage, Name: name, Prompt: "Do it."}
}

func pathwayOf(stages ...store.GafferPathwayStage) store.GafferPathway {
	return store.GafferPathway{Name: "Mine", Stages: stages}
}

func TestCleanPathway(t *testing.T) {
	with := func(s store.GafferPathwayStage, f func(*store.GafferPathwayStage)) store.GafferPathwayStage {
		f(&s)
		return s
	}
	many := func(n int) []store.GafferPathwayStage {
		out := make([]store.GafferPathwayStage, n)
		for i := range out {
			out[i] = pcustom("Stage " + strings.Repeat("x", i+1))
		}
		return out
	}
	tests := []struct {
		name  string
		p     store.GafferPathway
		field string // "" means sound
	}{
		{"sound line", pathwayOf(pst("plan"), pst("code"), pcustom("Security audit"), pst("pr"), pst("merge")), ""},
		{"review before code", pathwayOf(pst("review"), pst("code")), ""},
		{"single custom", pathwayOf(pcustom("Audit")), ""},
		{"no stages", pathwayOf(), "stages"},
		{"13 stages", pathwayOf(many(13)...), "stages"},
		{"12 stages", pathwayOf(many(12)...), ""},
		{"repeated kind", pathwayOf(pst("code"), pst("code")), "stages"},
		{"merge not last", pathwayOf(pst("pr"), pst("merge"), pst("code")), "stages"},
		{"merge before code", pathwayOf(pst("merge"), pst("code")), "stages"},
		{"babysit without pr", pathwayOf(pst("code"), pst("babysit")), "stages"},
		{"merge without pr", pathwayOf(pst("code"), pst("merge")), "stages"},
		{"custom without prompt", pathwayOf(with(pcustom("Audit"), func(s *store.GafferPathwayStage) { s.Prompt = "" })), "stages"},
		{"custom named Merge", pathwayOf(pcustom("Merge")), "stages"},
		{"custom named spaced merge", pathwayOf(pcustom(" merge ")), "stages"},
		{"custom named PR", pathwayOf(pcustom("PR")), "stages"},
		{"custom named custom", pathwayOf(pcustom("custom")), "stages"},
		{"custom without name", pathwayOf(pcustom("")), "stages"},
		{"custom name too long", pathwayOf(pcustom(strings.Repeat("a", 41))), "stages"},
		{"duplicate custom names", pathwayOf(pcustom("Audit"), pcustom("audit")), "stages"},
		{"built-in with a name", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Name = "Build" })), "stages"},
		{"unknown kind", pathwayOf(pst("deploy")), "stages"},
		{"prompt too long", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Prompt = strings.Repeat("a", 4001) })), "stages"},
		{"three skills", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Skills = []string{"a", "b", "c"} })), "stages"},
		{"repeated skill", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Skills = []string{"a", "a"} })), "stages"},
		{"whitespace-only skill", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Skills = []string{"  "} })), "stages"},
		{"skill repeated with spaces", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Skills = []string{"a", " a"} })), "stages"},
		{"empty skill", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Skills = []string{""} })), "stages"},
		{"tier and agent", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) {
			s.Tier, s.Agent = "light", &store.GafferStageAgent{Tool: "claude"}
		})), "stages"},
		{"tier huge", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Tier = "huge" })), "stages"},
		{"agent with unknown tool", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Agent = &store.GafferStageAgent{Tool: "nope"} })), "stages"},
		{"agent with provider on a tool that takes none", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) {
			s.Agent = &store.GafferStageAgent{Tool: "claude", Provider: "x"}
		})), "stages"},
		{"agent set", pathwayOf(with(pst("code"), func(s *store.GafferPathwayStage) { s.Agent = &store.GafferStageAgent{Tool: "claude"} })), ""},
		{"empty name", store.GafferPathway{Stages: []store.GafferPathwayStage{pst("code")}}, "name"},
		{"name of 61 runes", store.GafferPathway{Name: strings.Repeat("é", 61), Stages: []store.GafferPathwayStage{pst("code")}}, "name"},
		{"description of 301 bytes", store.GafferPathway{Name: "Mine", Description: strings.Repeat("a", 301), Stages: []store.GafferPathwayStage{pst("code")}}, "description"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cleanPathway(tc.p)
			if tc.field == "" {
				if err != nil {
					t.Fatalf("want sound, got %v", err)
				}
				return
			}
			var inv *Invalid
			if !errors.As(err, &inv) || inv.Field != tc.field {
				t.Fatalf("want Invalid on %q, got %v", tc.field, err)
			}
		})
	}
}

func TestCleanPathwayDefaultsTier(t *testing.T) {
	got, err := cleanPathway(pathwayOf(pst("plan"), pcustom("Audit")))
	if err != nil {
		t.Fatal(err)
	}
	if got.Stages[0].Tier != "heavy" || got.Stages[1].Tier != "standard" {
		t.Fatalf("tiers %q, %q", got.Stages[0].Tier, got.Stages[1].Tier)
	}
}

func usePathwayRig(t *testing.T, enabled bool) (*outcomeRig, store.GafferPathway) {
	t.Helper()
	r := outcomeFixture(t)
	saved, err := r.s.SavePathway(r.p.UserID, store.GafferPathway{Name: "Careful", Enabled: enabled, Stages: []store.GafferPathwayStage{
		{Kind: "review", Tier: "heavy", Skills: []string{"a", "b"}},
		{Kind: "code", Agent: &store.GafferStageAgent{Tool: "claude", Model: "opus", Effort: "high"}},
		pcustom("Audit"), pst("pr"), pst("merge"),
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return r, saved
}

func TestUsePathwayFillsStages(t *testing.T) {
	r, p := usePathwayRig(t, true)
	sessions := []OutcomeSession{{Title: "One", Prompt: "Do one."}, {Title: "Two", Prompt: "Do two."}}
	if err := r.s.UsePathway(r.p.UserID, p.ID, sessions, skillsAt(map[string]string{"a": "1", "b": "2"})); err != nil {
		t.Fatal(err)
	}
	for _, se := range sessions {
		if se.Pathway == nil || se.Pathway.ID != p.ID || se.Pathway.Name != "Careful" || len(se.Stages) != 5 {
			t.Fatalf("session %+v", se)
		}
		review, code, audit := se.Stages[0], se.Stages[1], se.Stages[2]
		if review.Tool != "" || review.Tier != "heavy" || len(review.Skills) != 2 || review.Skills[0].ID != "a" || review.Skills[1].ID != "b" || review.Skills[0].State != "selected" {
			t.Fatalf("tier stage %+v", review)
		}
		if code.Tool != "claude" || code.Model != "opus" || code.Effort != "high" || code.Backup != nil || len(code.More) != 0 || code.Tier != "" {
			t.Fatalf("agent stage %+v", code)
		}
		if audit.Name != "Audit" || audit.Prompt != "Do it." {
			t.Fatalf("custom stage %+v", audit)
		}
	}
}

func TestUsePathwayRefusals(t *testing.T) {
	r, p := usePathwayRig(t, true)
	good := skillsAt(map[string]string{"a": "1", "b": "2"})
	big := func(id string) (skillbuddy.Skill, error) {
		return skillbuddy.Skill{ID: id, Name: id, Source: "personal", Revision: "1", Instructions: strings.Repeat("x", 1200)}, nil
	}
	lint, err := r.s.SavePathway(r.p.UserID, store.GafferPathway{Name: "Linty", Enabled: true, Stages: []store.GafferPathwayStage{{Kind: "code", Skills: []string{"lint-rules"}}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	off, _ := usePathwayRig(t, false)
	offs, err := off.s.Pathways(off.p.UserID)
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		rig      *outcomeRig
		id       string
		sessions []OutcomeSession
		load     SkillLoader
		want     []string
	}{
		"has stages": {r, p.ID, []OutcomeSession{{Title: "One", Stages: []OutcomeStageInput{{Kind: "code"}}}}, good, []string{"not both"}},
		"disabled":   {off, offs[0].ID, []OutcomeSession{{Title: "One"}}, good, []string{"Careful", "switched off"}},
		"unknown":    {r, strings.Repeat("0", 32), []OutcomeSession{{Title: "One"}}, good, []string{"not available"}},
		"loader":     {r, lint.ID, []OutcomeSession{{Title: "One"}}, good, []string{"Linty", "code", "lint-rules"}},
		"too big":    {r, p.ID, []OutcomeSession{{Title: "One"}}, big, []string{"Careful", "review", "2,000 bytes"}},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.rig.s.UsePathway(tc.rig.p.UserID, tc.id, tc.sessions, tc.load)
			var inv *Invalid
			if !errors.As(err, &inv) {
				t.Fatalf("want Invalid, got %v", err)
			}
			for _, w := range tc.want {
				if !strings.Contains(inv.Message, w) {
					t.Fatalf("message %q lacks %q", inv.Message, w)
				}
			}
			if len(tc.sessions[0].Stages) > 1 || name != "has stages" && len(tc.sessions[0].Stages) != 0 {
				t.Fatalf("a refusal changed the sessions: %+v", tc.sessions[0])
			}
		})
	}
	if got := r.s.PathwayProblems(p, good); len(got) != 0 {
		t.Fatalf("sound pathway has problems: %v", got)
	}
	if got := r.s.PathwayProblems(lint, good); len(got) != 1 {
		t.Fatalf("problems = %v", got)
	}
}

// Review Focus 3: work under way keeps the stages it was started with.
func TestRunningWorkKeepsItsStages(t *testing.T) {
	r, p := usePathwayRig(t, true)
	r.s.StartGoalSupervisor()
	in := r.in
	in.Sessions = []OutcomeSession{{Title: "Logs", Prompt: "Tidy the logs."}}
	if err := r.s.UsePathway(r.p.UserID, p.ID, in.Sessions, skillsAt(map[string]string{"a": "1", "b": "2"})); err != nil {
		t.Fatal(err)
	}
	// The agent stage names claude, which this machine's fake does not run; the order is what is under test.
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	before := func() store.GafferOutcomeSession {
		o, err := r.st.GafferOutcomeFor(r.p.UserID, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		return o.Sessions[0]
	}
	want := before()
	if want.PathwayName != "Careful" || len(want.Stages) != 5 {
		t.Fatalf("stored %+v", want)
	}
	p.Stages = p.Stages[:4]
	if _, err := r.s.SavePathway(r.p.UserID, p, p.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if err := r.s.DeletePathway(r.p.UserID, p.ID); err != nil {
		t.Fatal(err)
	}
	got := before()
	if got.PathwayName != "Careful" || len(got.Stages) != 5 || got.Stages[2].Label() != "Audit" || got.Stages[0].Label() != "review" {
		t.Fatalf("after edit and delete %+v", got)
	}
}
