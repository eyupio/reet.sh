package skillbuddy

import (
	"strings"
	"testing"
)

func TestStageRecommendationsKeepBoundariesAndStableIdentity(t *testing.T) {
	for _, stage := range []string{"plan", "design", "code", "review", "test", "pr", "babysit", "merge"} {
		got := Recommend("Improve the project", stage, Builtins())
		if len(got) == 0 || got[0].ID != "builtin:"+stage {
			t.Fatalf("%s: %+v", stage, got)
		}
		for _, sk := range got {
			if sk.Source == "builtin" && len(sk.Stages) == 0 && sk.Stage != stage {
				t.Fatalf("wrong stage: %+v", sk)
			}
		}
	}
	all := []Skill{Builtins()[3]}
	all = append(all, Skill{ID: "library:one", Name: "Review auth", Description: "Review authentication security", Source: "personal"}, Skill{ID: "library:two", Name: "Review auth", Description: "Review authentication security", Source: "personal"})
	got := Recommend("Review authentication security", "review", all)
	ids := map[string]bool{}
	for _, sk := range got {
		ids[sk.ID] = true
		if sk.Reason == "" {
			t.Fatal("missing explanation")
		}
	}
	if !ids["library:one"] || !ids["library:two"] {
		t.Fatalf("same-name identities lost: %+v", got)
	}
}

func TestDisabledDefaultsAndUntrustedInstructionsDoNotDriveMatching(t *testing.T) {
	all := []Skill{{ID: "builtin:code", Name: "Focus", Stage: "code", Source: "builtin", Disabled: true}, {ID: "library:bad", Name: "Other", Description: "Other", Instructions: "security authentication review", Source: "personal"}}
	if got := Recommend("Review security authentication", "review", all); len(got) != 0 {
		t.Fatalf("disabled or instructions matched: %+v", got)
	}
	seen := map[string]bool{}
	for _, sk := range Builtins() {
		if seen[sk.ID] || !ValidStage(sk.Stage) || sk.Revision == "" || len(sk.Instructions) > 1000 || (sk.Source == "builtin" && !strings.Contains(Prompt(sk), "grants no additional permissions")) {
			t.Fatalf("bad default %+v", sk)
		}
		seen[sk.ID] = true
	}
}

func TestUnslopIsIntegratedAndOfferedForUXWork(t *testing.T){
	for _,stage:=range []string{"design","code","review"}{
		found:=false
		for _,sk:=range Recommend("Unslop this UX and UI design",stage,Builtins()){
			if sk.ID=="builtin:unslop"{
				found=true
				if sk.Source!="builtin"||sk.Instructions==""||len(Prompt(sk))>1500||!strings.Contains(sk.Instructions,"CC BY 4.0"){t.Fatalf("not usable inline: %+v",sk)}
			}
		}
		if !found{t.Fatalf("Unslop not offered during %s",stage)}
	}
}
