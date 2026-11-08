//go:build sync_contract

package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// Desired-behaviour tests are deliberately opt-in while the staged migration
// is incomplete. They must fail for the documented product gaps, not because
// of a broken harness. Run TestSimpleSyncHarness separately as its control.
func TestSimpleSyncContractSelectedItemsOnly(t *testing.T) {
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
			SourceID: "library", SkillIDs: []string{wanted.ID}, Mode: "additive",
		})
		if code != 200 {
			t.Errorf("%s could not save an individual selection: %d %s", m.name, code, body)
		}
	}
	h.rounds(6, a, b)
	for _, m := range []*syncContractMachine{a, b} {
		m.requireFile("selected", "SKILL.md", syncContractContent("selected", "wanted"))
		if _, err := os.Stat(m.path("unselected", "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("%s received an unselected skill: %v", m.name, err)
		}
	}
}

func TestSimpleSyncContractInactiveAuthoredSkillsStayOffMachines(t *testing.T) {
	h := newSyncContractHarness(t)
	h.hosted("private-draft", "not activated", false, false)
	toTest := h.hosted("published-to-test", "not activated", true, false)
	skill, found, err := h.st.GetFor(h.uid, toTest.ID)
	if err != nil || !found || skill.Used || !skill.Bookmarked {
		t.Fatalf("fixture is not an authored To test skill: %+v, found=%t: %v", skill, found, err)
	}
	m := h.machine("inactive-machine")
	h.rounds(6, m)
	for _, name := range []string{"private-draft", "published-to-test"} {
		if _, err := os.Stat(m.path(name, "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("inactive authored skill %s was installed: %v", name, err)
		}
	}
}

func TestSimpleSyncContractLocalDeletionRestoresAndNotifies(t *testing.T) {
	h := newSyncContractHarness(t)
	h.hosted("restore-me", "canonical instructions", false, true)
	m := h.machine("restore-machine")
	h.rounds(5, m)
	m.requireFile("restore-me", "SKILL.md", syncContractContent("restore-me", "canonical instructions"))
	if err := os.Remove(m.path("restore-me", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	h.rounds(8, m)
	if got, err := os.ReadFile(m.path("restore-me", "SKILL.md")); err != nil ||
		string(got) != syncContractContent("restore-me", "canonical instructions") {
		t.Errorf("desired local file was not restored correctly: %q: %v", got, err)
	}
	raw := h.mustRequest(http.MethodGet, "/api/v2/notifications", nil)
	var response struct{ Notifications []store.Notification }
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, notice := range response.Notifications {
		message := strings.ToLower(notice.Title + " " + notice.Body)
		if strings.Contains(message, "restore-me") && strings.Contains(message, m.name) &&
			strings.Contains(message, "restor") {
			matches++
		}
	}
	if matches != 1 {
		t.Errorf("want one restore notification naming skill and machine, got %d: %s", matches, raw)
	}
}

func TestSimpleSyncContractLeavingInUseRemovesManagedCopy(t *testing.T) {
	h := newSyncContractHarness(t)
	skill := h.hosted("deactivate-me", "kept in history", true, true)
	m := h.machine("deactivate-machine")
	h.rounds(5, m)
	m.requireFile("deactivate-me", "SKILL.md", syncContractContent("deactivate-me", "kept in history"))
	h.used(skill.ID, false)
	h.rounds(6, m)
	if _, err := os.Stat(m.path("deactivate-me", "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("leaving In use did not remove the managed deployment: %v", err)
	}
	saved, found, err := h.st.GetFor(h.uid, skill.ID)
	if err != nil || !found || saved.Used {
		t.Errorf("deactivation lost the library entry or status: found=%t used=%t: %v", found, saved.Used, err)
	}
}
