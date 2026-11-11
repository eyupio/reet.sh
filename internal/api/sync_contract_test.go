//go:build sync_contract

package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// Desired-behaviour tests are deliberately opt-in while the staged migration
// is incomplete. They must fail for the documented product gaps, not because
// of a broken harness. Run TestSimpleSyncHarness separately as its control.
func TestSimpleSyncContractLocalDeletionRestoresAndNotifies(t *testing.T) {
	h := newSyncContractHarness(t)
	h.enableManagedRemovals()
	h.hosted("restore-me", "canonical instructions", false, true)
	m := h.machine("restore-machine")
	h.rounds(1, m)
	items, libraryErr := h.st.LibraryFor(h.uid)
	if libraryErr != nil {
		t.Fatal(libraryErr)
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, m.name, "agents", ""), ItemIDs: ids})
	h.rounds(5, m)
	m.requireFile("restore-me", "SKILL.md", syncContractContent("restore-me", "canonical instructions"))
	if err := os.Remove(m.path("restore-me", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	// A deletion is believed only after it persists past the debounce window.
	h.rounds(2, m)
	time.Sleep(450 * time.Millisecond)
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
	h.enableManagedRemovals()
	skill := h.hosted("deactivate-me", "kept in history", true, true)
	m := h.machine("deactivate-machine")
	h.rounds(1, m)
	items, libraryErr := h.st.LibraryFor(h.uid)
	if libraryErr != nil {
		t.Fatal(libraryErr)
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, m.name, "agents", ""), ItemIDs: ids})
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
