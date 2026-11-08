package api

import (
	"github.com/eyupio/reet.sh/internal/store"
	"net/http"
	"os"
	"testing"
)

func TestSelectedLibraryExplicitSelection(t *testing.T) {
	h := newSyncContractHarness(t)
	wanted := h.hosted("selected", "wanted", false, true)
	h.hosted("unselected", "must stay off machines", false, true)
	a, b := h.machine("selected-a"), h.machine("selected-b")
	h.rounds(1, a, b) // complete real inventory before using the legacy intent API
	for _, m := range []*syncContractMachine{a, b} {
		targetID := store.EnvironmentIDFor(h.uid, m.name, "agents", "")
		targets, err := h.st.SyncTargetsFor(h.uid)
		if err != nil {
			t.Fatal(err)
		}
		revision := ""
		for _, target := range targets {
			if target.Target.ID == targetID {
				revision = target.Revision
			}
		}
		code, body := h.request(http.MethodPut, "/api/v2/sync/desired", store.DesiredSelection{
			TargetID: targetID, Revision: revision,
			SelectedOnly: true, SourceID: "library", SkillIDs: []string{wanted.ID}, Mode: "additive",
		})
		if code != 200 {
			t.Errorf("%s could not save an individual selection: %d %s", m.name, code, body)
		}
	}
	c := h.machine("unselected-c")
	h.rounds(6, a, b, c)
	if _, err := os.Stat(c.path("selected", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("unselected machine received an item", err)
	}
	for _, m := range []*syncContractMachine{a, b} {
		m.requireFile("selected", "SKILL.md", syncContractContent("selected", "wanted"))
		if _, err := os.Stat(m.path("unselected", "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("%s received an unselected skill: %v", m.name, err)
		}
	}
	h.hosted("later", "must not subscribe to future additions", false, true)
	h.hosted("selected", "central edit propagates", false, true)
	h.rounds(6, a, b, c)
	for _, m := range []*syncContractMachine{a, b} {
		m.requireFile("selected", "SKILL.md", syncContractContent("selected", "central edit propagates"))
		if _, err := os.Stat(m.path("later", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatal("future item was implicitly selected", err)
		}
	}
}
