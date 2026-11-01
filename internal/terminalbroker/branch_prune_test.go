package terminalbroker

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func pruneCapabilities() terminalproto.Capabilities {
	caps := browseCapabilities()
	caps.BranchPrune = true
	return caps
}

func TestBranchPrunePlanAndPruneAreForwardedExactly(t *testing.T) {
	oid := strings.Repeat("a", 40)
	f := browseFixture(t, pruneCapabilities())
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		switch body.(type) {
		case terminalproto.BranchPrunePlan:
			return terminalproto.BranchPrunePlanResult{
				Current: "main", Default: "main", Base: "main", More: true,
				Branches: []terminalproto.BranchPruneBranch{{Name: "old", OID: oid}, {Name: "wip", OID: oid, Skip: terminalproto.BranchPruneNotMerged}},
			}
		case terminalproto.BranchPrune:
			return terminalproto.BranchPruneResult{
				Deleted: []string{"old"},
				Skipped: []terminalproto.BranchPruneSkipped{{Name: "wip", Skip: terminalproto.BranchPruneNotMerged}},
			}
		}
		return nil
	})
	plan := terminalproto.BranchPrunePlan{Dir: "/home/test/site", Base: "main"}
	got, err := f.b.BranchPrunePlan(t.Context(), f.uid, f.agent, plan)
	if err != nil || got.Base != "main" || !got.More || len(got.Branches) != 2 || got.Branches[0].Name != "old" || got.Branches[1].Skip != terminalproto.BranchPruneNotMerged {
		t.Fatal(got, err)
	}
	prune := terminalproto.BranchPrune{Dir: "/home/test/site", Base: "main", Branches: []terminalproto.BranchPruneTarget{{Name: "old", OID: oid}}}
	res, err := f.b.PruneBranches(t.Context(), f.uid, f.agent, prune)
	if err != nil || len(res.Deleted) != 1 || res.Deleted[0] != "old" || len(res.Skipped) != 1 {
		t.Fatal(res, err)
	}
	sent := f.sentRequests()
	if len(sent) != 2 || !reflect.DeepEqual(sent[0], plan) || !reflect.DeepEqual(sent[1], prune) {
		t.Fatalf("requests not forwarded exactly: %+v", sent)
	}
}

func TestBranchPruneNeedsTheCapability(t *testing.T) {
	f := browseFixture(t, browseCapabilities()) // branches, but no branch-prune-v1
	if _, err := f.b.BranchPrunePlan(t.Context(), f.uid, f.agent, terminalproto.BranchPrunePlan{Dir: "/home/test/site"}); !errors.Is(err, ErrBranchPruneUnsupported) {
		t.Fatal("plan", err)
	}
	oid := strings.Repeat("a", 40)
	if _, err := f.b.PruneBranches(t.Context(), f.uid, f.agent, terminalproto.BranchPrune{Dir: "/home/test/site", Base: "main", Branches: []terminalproto.BranchPruneTarget{{Name: "old", OID: oid}}}); !errors.Is(err, ErrBranchPruneUnsupported) {
		t.Fatal("prune", err)
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatal("an unsupported request was sent", sent)
	}
}

func TestBranchPruneReasonsMapToErrors(t *testing.T) {
	f := browseFixture(t, pruneCapabilities())
	for _, reason := range []terminalproto.BranchesReason{terminalproto.BranchesNotRepoRoot, terminalproto.BranchesOutsideHome, terminalproto.BranchesGitFailed} {
		f.answering(func(terminalproto.Body) terminalproto.Body {
			return terminalproto.BranchPrunePlanResult{Reason: reason}
		})
		_, err := f.b.BranchPrunePlan(t.Context(), f.uid, f.agent, terminalproto.BranchPrunePlan{Dir: "/home/test/site"})
		var refusal *BranchPruneError
		if !errors.As(err, &refusal) || refusal.Reason != reason {
			t.Fatal("plan", reason, err)
		}
	}
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BranchPruneResult{Reason: terminalproto.BranchesGitFailed}
	})
	oid := strings.Repeat("a", 40)
	_, err := f.b.PruneBranches(t.Context(), f.uid, f.agent, terminalproto.BranchPrune{Dir: "/home/test/site", Base: "main", Branches: []terminalproto.BranchPruneTarget{{Name: "old", OID: oid}}})
	var refusal *BranchPruneError
	if !errors.As(err, &refusal) || refusal.Reason != terminalproto.BranchesGitFailed {
		t.Fatal("prune", err)
	}
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BranchPrunePlanResult{Reason: terminalproto.BranchesConflict}
	})
	if _, err := f.b.BranchPrunePlan(t.Context(), f.uid, f.agent, terminalproto.BranchPrunePlan{Dir: "/home/test/site"}); !errors.Is(err, ErrBranchPruneBusy) {
		t.Fatal("conflict", err)
	}
}
