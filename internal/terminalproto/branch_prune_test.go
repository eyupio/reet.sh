package terminalproto

import (
	"bytes"
	"strings"
	"testing"
)

func TestBranchPruneFeatureToken(t *testing.T) {
	for _, value := range []string{"", "branches-v1", "branch-prune-v10", "prefix-branch-prune-v1"} {
		if SupportsBranchPrune(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsBranchPrune("branches-v1, branch-prune-v1") || BranchPruneFeature != "branch-prune-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestBranchPruneRoundTripAndLimits(t *testing.T) {
	oid := strings.Repeat("a", 40)
	for _, body := range []Body{
		BranchPrunePlan{Dir: "/home/u/api", Base: "main"},
		BranchPrunePlanResult{Current: "main", Default: "main", Base: "main", Branches: []BranchPruneBranch{{Name: "old", OID: oid}, {Name: "wip", OID: oid, Skip: BranchPruneNotMerged}}, More: true},
		BranchPrunePlanResult{Branches: []BranchPruneBranch{}},
		BranchPrunePlanResult{Reason: BranchesNotRepoRoot},
		BranchPrune{Dir: "/home/u/api", Base: "main", Branches: []BranchPruneTarget{{Name: "old", OID: oid}}},
		BranchPruneResult{Deleted: []string{"old"}, Skipped: []BranchPruneSkipped{{Name: "wip", Skip: BranchPruneUnpushed}}},
		BranchPruneResult{Reason: BranchesGitFailed},
	} {
		encoded, err := Encode(testMessage(body))
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		if _, err = Encode(decoded); err != nil {
			t.Fatal(err)
		}
	}
	encoded, _ := Encode(testMessage(BranchPrunePlan{Dir: "/home/u/api"}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"dir":`), []byte(`"remote":"o","dir":`), 1)); err == nil {
		t.Fatal("unknown field accepted")
	}

	bad := map[string]BranchPrunePlan{
		"no dir":          {},
		"relative dir":    {Dir: "api"},
		"control in dir":  {Dir: "/a\n"},
		"control in base": {Dir: "/a", Base: "ma\nin"},
	}
	for name, p := range bad {
		if p.valid() {
			t.Error("plan accepted", name)
		}
	}
	if !(BranchPrunePlan{Dir: "/a", Base: "release/1.2"}).valid() {
		t.Error("plan with a valid base refused")
	}

	badTargets := map[string]BranchPrune{
		"no branches":     {Dir: "/a", Base: "main"},
		"no base":         {Dir: "/a", Branches: []BranchPruneTarget{{Name: "x", OID: oid}}},
		"short oid":       {Dir: "/a", Base: "main", Branches: []BranchPruneTarget{{Name: "x", OID: "abc"}}},
		"upper oid":       {Dir: "/a", Base: "main", Branches: []BranchPruneTarget{{Name: "x", OID: strings.Repeat("A", 40)}}},
		"option name":     {Dir: "/a", Base: "main", Branches: []BranchPruneTarget{{Name: "-f", OID: oid}}},
		"repeated branch": {Dir: "/a", Base: "main", Branches: []BranchPruneTarget{{Name: "x", OID: oid}, {Name: "x", OID: oid}}},
	}
	for name, p := range badTargets {
		if p.valid() {
			t.Error("prune accepted", name)
		}
	}

	for name, r := range map[string]BranchPrunePlanResult{
		"option current": {Current: "-f"},
		"short oid":      {Branches: []BranchPruneBranch{{Name: "x", OID: "abc"}}},
		"unknown skip":   {Branches: []BranchPruneBranch{{Name: "x", OID: oid, Skip: "later"}}},
		"repeat name":    {Branches: []BranchPruneBranch{{Name: "x", OID: oid}, {Name: "x", OID: oid}}},
	} {
		if r.valid() {
			t.Error("plan result accepted", name)
		}
	}
	if !(BranchPrunePlanResult{Base: "main", Branches: []BranchPruneBranch{{Name: "x", OID: oid, Skip: BranchPruneCheckedOut}}}).valid() {
		t.Error("valid plan result refused")
	}
}
