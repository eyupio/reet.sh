package gaffer

import (
	"errors"
	"strings"
	"testing"

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
