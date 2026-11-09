package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestCanonicalSelectionOfflineRenameAndNewMachine(t *testing.T) {
	h := newSyncContractHarness(t)
	h.hosted("canonical", "first content", false, true)
	items, err := h.st.LibraryFor(h.uid)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	item := items[0]
	a, b := h.machine("canonical-a"), h.machine("canonical-b")
	h.rounds(5, a, b)
	if _, err := os.Stat(a.path("canonical", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("new connector installed an item before selection", err)
	}
	b.link.offline.Store(true)
	for _, m := range []*syncContractMachine{a, b} {
		target := store.EnvironmentIDFor(h.uid, m.name, "agents", "")
		if err = h.st.RecordInventoryFor(h.uid, target, false, store.MachineInfo{}); err != nil {
			t.Fatal(err)
		}
		code, body := h.request(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: target, ItemIDs: []string{item.ID}})
		if code != 200 {
			t.Fatalf("offline intent rejected %d %s", code, body)
		}
		var selection store.LibrarySelection
		if err = json.Unmarshal([]byte(body), &selection); err != nil {
			t.Fatal(err)
		}
		if len(selection.ItemIDs) != 1 || selection.ItemIDs[0] != item.ID || selection.Version == "" {
			t.Fatal("canonical identity not retained", selection)
		}
		code, _ = h.request(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: target, ItemIDs: []string{}})
		if code != 409 {
			t.Fatal("stale selection replaced intent", code)
		}
	}
	b.link.offline.Store(false)
	h.rounds(8, a, b)
	for _, m := range []*syncContractMachine{a, b} {
		m.requireFile("canonical", "SKILL.md", syncContractContent("canonical", "first content"))
	}
	username, err := h.st.PublishingUsername(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	oldRef, err := ref.ParseAt(h.controller.URL+"/"+username+"/reet-canonical", h.controller.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	newRef, err := ref.ParseAt(h.controller.URL+"/"+username+"/reet-renamed", h.controller.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.st.RenameHostedSkill(h.uid, "reet-canonical", "reet-renamed", oldRef, newRef); err != nil {
		t.Fatal(err)
	}
	c := h.machine("unselected-canonical-c")
	h.rounds(8, a, b, c)
	for _, m := range []*syncContractMachine{a, b} {
		m.requireFile("renamed", "SKILL.md", syncContractContent("renamed", "first content"))
		selected, err := h.st.LibrarySelectionFor(h.uid, store.EnvironmentIDFor(h.uid, m.name, "agents", ""))
		if err != nil || selected.ItemIDs[0] != item.ID {
			t.Fatal("rename changed selection", selected, err)
		}
	}
	if _, err = os.Stat(c.path("renamed", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("new machine received implicit selection", err)
	}
}

func TestCanonicalSelectedMachineEditsAndOfflineOrdering(t *testing.T) {
	h := newSyncContractHarness(t)
	h.hosted("edits", "original", false, true)
	items, err := h.st.LibraryFor(h.uid)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	item := items[0]
	a, b, c := h.machine("edit-a"), h.machine("edit-b"), h.machine("edit-c")
	h.rounds(1, a, b, c)
	for _, m := range []*syncContractMachine{a, b} {
		h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, m.name, "agents", ""), ItemIDs: []string{item.ID}})
	}
	h.rounds(8, a, b, c)
	edit := func(m *syncContractMachine, text string, at time.Time) {
		t.Helper()
		path := m.path("edits", "SKILL.md")
		if err := os.WriteFile(path, []byte(syncContractContent("edits", text)), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	edit(a, "from A", time.Now())
	if err := os.Chmod(a.path("edits", "reference.txt"), 0755); err != nil {
		t.Fatal(err)
	}
	h.rounds(8, a, b, c)
	b.requireFile("edits", "SKILL.md", syncContractContent("edits", "from A"))
	if info, err := os.Stat(b.path("edits", "reference.txt")); err != nil || info.Mode()&0111 == 0 {
		t.Fatal("machine edit lost executable mode", info, err)
	}
	current, err := h.st.LibraryItemFor(h.uid, item.ID)
	if err != nil || current.RevisionID == item.RevisionID {
		t.Fatal("machine edit did not move canonical head", current, err)
	}
	// Central edits use the same execution path and do not produce upload loops.
	h.hosted("edits", "from central", false, true)
	h.rounds(8, a, b)
	a.requireFile("edits", "SKILL.md", syncContractContent("edits", "from central"))
	b.requireFile("edits", "SKILL.md", syncContractContent("edits", "from central"))
	a.link.offline.Store(true)
	b.link.offline.Store(true)
	older := time.Now().Add(time.Second)
	newer := older.Add(time.Second)
	edit(a, "older offline edit", older)
	edit(b, "newer offline edit", newer)
	for _, m := range []*syncContractMachine{a, b} {
		if _, err := m.tick(); err == nil {
			t.Fatal("offline poll unexpectedly succeeded")
		}
		m.stop()
		m.start()
	}
	// Newer saves arrive first; a controller restart does not forget their order.
	b.link.offline.Store(false)
	h.rounds(8, b)
	h.restartController()
	a.link.offline.Store(false)
	h.rounds(10, a, b, c)
	for _, m := range []*syncContractMachine{a, b} {
		m.requireFile("edits", "SKILL.md", syncContractContent("edits", "newer offline edit"))
	}
	history, err := h.st.LibraryEditHistoryFor(h.uid, item.ID)
	if err != nil || len(history) != 3 {
		t.Fatal("offline edits not retained exactly once", history, err)
	}
	// Verify losing bytes remain readable in private immutable history.
	loser := 0
	for _, event := range history {
		if !event.Won {
			loser++
			head, _ := h.st.LibraryItemFor(h.uid, item.ID)
			rev, err := h.st.PrivateSkillRevisionFor(h.uid, head.BundleID, event.RevisionID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range rev.Files {
				if f.Path == "SKILL.md" && string(f.Data) == syncContractContent("edits", "older offline edit") {
					found = true
				}
			}
			if !found {
				t.Fatal("losing content missing", rev)
			}
		}
	}
	if loser != 1 {
		t.Fatal("wrong conflict outcome", history)
	}
	before := len(history)
	jobsBefore, err := h.st.SyncJobsFor(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	h.rounds(8, a, b)
	jobsAfter, err := h.st.SyncJobsFor(h.uid)
	if err != nil || len(jobsAfter) != len(jobsBefore) {
		t.Fatal("converged content kept generating no-op jobs", len(jobsBefore), len(jobsAfter), err)
	}
	history, err = h.st.LibraryEditHistoryFor(h.uid, item.ID)
	if err != nil || len(history) != before {
		t.Fatal("import/apply loop", history, err)
	}
	if _, err := os.Stat(c.path("edits", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("C received unselected content", err)
	}
}

func TestCanonicalImportedSourceEditsAndBlockedItemIsolation(t *testing.T) {
	h := newSyncContractHarness(t)
	source, destination := h.machine("root-source"), h.machine("root-destination")
	source.localSkill("imported", "original imported bytes")
	// Normal connectors report metadata before the first explicit content
	// capture. That report's identity/status must survive this import.
	h.rounds(1, source, destination)
	reported, err := h.st.LibraryFor(h.uid)
	if err != nil || len(reported) != 1 {
		t.Fatal(reported, err)
	}
	reportedID := reported[0].ID
	custom := filepath.Join(source.home, "custom-skills")
	if err := os.MkdirAll(filepath.Join(custom, "reet-custom"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(custom, "reet-custom", "SKILL.md")
	if err := os.WriteFile(path, []byte(syncContractContent("custom", "custom original")), 0644); err != nil {
		t.Fatal(err)
	}
	source.stop()
	if output, err := source.command("library-import", "--config", source.config, "--folder", custom); err != nil {
		t.Fatalf("import: %v %s", err, output)
	}
	source.start()
	items, err := h.st.LibraryFor(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, item := range items {
		if item.Name == "reet-imported" && (item.ID != reportedID || item.Status != store.LibraryInUse) {
			t.Fatal("first capture lost reported identity/status", item, reportedID)
		}
		if err := h.st.SetLibraryStatusFor(h.uid, item.ID, store.LibraryInUse, item.Version); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
	}
	h.rounds(2, source, destination)
	h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, destination.name, "agents", ""), ItemIDs: ids})
	h.rounds(8, source, destination)
	destination.requireFile("imported", "SKILL.md", syncContractContent("imported", "original imported bytes"))
	destination.requireFile("custom", "SKILL.md", syncContractContent("custom", "custom original"))
	if err := os.WriteFile(path, []byte(syncContractContent("custom", "source custom edit")), 0644); err != nil {
		t.Fatal(err)
	}
	h.rounds(8, source, destination)
	destination.requireFile("custom", "SKILL.md", syncContractContent("custom", "source custom edit"))
	// One unreadable/unsupported managed item must not hold healthy content in
	// the same target folder. Local absence never deletes the canonical revision.
	if err := os.WriteFile(destination.path("imported", "blocked-link"), []byte("temporary"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(destination.path("imported", "blocked-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/missing/stage3-fixture", destination.path("imported", "blocked-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(syncContractContent("custom", "healthy despite blocked neighbor")), 0644); err != nil {
		t.Fatal(err)
	}
	h.rounds(8, source, destination)
	destination.requireFile("custom", "SKILL.md", syncContractContent("custom", "healthy despite blocked neighbor"))
	current, err := h.st.LibraryFor(h.uid)
	if err != nil || len(current) != 2 {
		t.Fatal("blocked capture changed library", current, err)
	}
}

func TestCanonicalEditReceiptsClockFallbackAndEnrollmentFence(t *testing.T) {
	h := newSyncContractHarness(t)
	h.hosted("clock", "original", false, true)
	items, err := h.st.LibraryFor(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]
	a, b, c := h.machine("clock-a"), h.machine("clock-b"), h.machine("clock-unselected")
	h.rounds(1, a, b, c)
	for _, m := range []*syncContractMachine{a, b} {
		h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, m.name, "agents", ""), ItemIDs: []string{item.ID}})
	}
	revision, err := h.st.PrivateSkillRevisionFor(h.uid, item.BundleID, item.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range revision.Files {
		if revision.Files[i].Path == "SKILL.md" {
			revision.Files[i].Data = []byte(strings.Replace(string(revision.Files[i].Data), "description: Stage 0 fixture", "description: Edited on machine", 1))
		}
	}
	event := store.LibraryEdit{EventID: strings.Repeat("a", 32), ItemID: item.ID, Base: item.RevisionID, SavedAt: time.Now().Add(24 * time.Hour).UnixMilli(), Uncertainty: 3600000, Files: revision.Files}
	receipt, err := h.st.AcceptLibraryEditFor(h.uid, a.id, event)
	if err != nil || !receipt.ClockFallback || !receipt.Won {
		t.Fatal("uncertain clock was not handled automatically", receipt, err)
	}
	// An uncertain clock uses its causal base, not the arrival or future timestamp.
	if receipt.OrderAt <= item.CreatedAt || receipt.OrderAt >= event.SavedAt {
		t.Fatal("bad logical fallback", receipt)
	}
	later := event
	later.EventID = strings.Repeat("b", 32)
	later.SavedAt = time.Now().Add(time.Second).UnixMilli()
	later.Uncertainty = 1
	second, err := h.st.AcceptLibraryEditFor(h.uid, b.id, later)
	if err != nil || !second.Won {
		t.Fatal(second, err)
	}
	authored, err := h.st.HostedSkillsFor(h.uid)
	if err != nil || len(authored) != 1 || authored[0].Description != "Edited on machine" {
		t.Fatal("authored metadata did not follow machine edit", authored, err)
	}
	replay, err := h.st.AcceptLibraryEditFor(h.uid, a.id, event)
	if err != nil || replay != receipt {
		t.Fatal("receipt replay changed outcome", replay, receipt, err)
	}
	head, err := h.st.LibraryItemFor(h.uid, item.ID)
	if err != nil || head.RevisionID != second.RevisionID {
		t.Fatal("receipt replay moved head", head, err)
	}
	event.SavedAt--
	if _, err = h.st.AcceptLibraryEditFor(h.uid, a.id, event); !errors.Is(err, store.ErrPrivateSkillConflict) {
		t.Fatal("changed event payload accepted", err)
	}
	if _, err = h.st.AcceptLibraryEditFor(h.uid, c.id, later); err == nil {
		t.Fatal("unselected enrollment wrote content")
	}
	if err = h.st.ControlSyncAgent(h.uid, a.id, "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err = h.st.AcceptLibraryEditFor(h.uid, a.id, event); err == nil {
		t.Fatal("revoked enrollment replayed content")
	}
}

func TestCanonicalSelectionsAcrossSupportedPrivateAdapters(t *testing.T) {
	h := newSyncContractHarness(t)
	h.hosted("adapters", "exact bytes", false, true)
	items, err := h.st.LibraryFor(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	m := h.machine("adapters")
	roots := map[string]string{"agents": ".agents/skills", "claude": ".claude/skills", "opencode": ".config/opencode/skills", "cursor": ".cursor/skills"}
	for _, root := range roots {
		if err := os.MkdirAll(filepath.Join(m.home, root), 0755); err != nil {
			t.Fatal(err)
		}
	}
	h.rounds(1, m)
	for agent := range roots {
		h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, m.name, agent, ""), ItemIDs: []string{items[0].ID}})
	}
	h.rounds(18, m)
	for agent, root := range roots {
		data, err := os.ReadFile(filepath.Join(m.home, root, "reet-adapters", "SKILL.md"))
		if err != nil || string(data) != syncContractContent("adapters", "exact bytes") {
			t.Fatalf("%s: %q %v", agent, data, err)
		}
	}
}

func TestCanonicalForegroundNormalAndReconnectLatency(t *testing.T) {
	h := newSyncContractHarness(t)
	h.hosted("latency", "original", false, true)
	items, err := h.st.LibraryFor(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	a, b := h.machine("latency-a"), h.machine("latency-b")
	h.rounds(1, a, b)
	for _, m := range []*syncContractMachine{a, b} {
		h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, m.name, "agents", ""), ItemIDs: []string{items[0].ID}})
	}
	h.rounds(8, a, b)
	a.startForeground()
	b.startForeground()
	await := func(text string) time.Duration {
		t.Helper()
		start := time.Now()
		timeout := time.NewTimer(45 * time.Second)
		defer timeout.Stop()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout.C:
				t.Fatal("background sync did not converge")
			case <-ticker.C:
				data, err := os.ReadFile(b.path("latency", "SKILL.md"))
				if err == nil && string(data) == syncContractContent("latency", text) {
					return time.Since(start)
				}
			}
		}
	}
	if err := os.WriteFile(a.path("latency", "SKILL.md"), []byte(syncContractContent("latency", "normal edit")), 0644); err != nil {
		t.Fatal(err)
	}
	normal := await("normal edit")
	beforeOffline := b.link.failedSyncRequests.Load()
	b.link.offline.Store(true)
	if err := os.WriteFile(a.path("latency", "SKILL.md"), []byte(syncContractContent("latency", "reconnect edit")), 0644); err != nil {
		t.Fatal(err)
	}
	// Prove A's upload and a failed B cycle before reconnecting B.
	timeout := time.NewTimer(45 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
waiting:
	for {
		select {
		case <-timeout.C:
			t.Fatal("source did not upload during destination outage")
		case <-ticker.C:
			head, err := h.st.LibraryItemFor(h.uid, items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			revision, err := h.st.PrivateSkillRevisionFor(h.uid, head.BundleID, head.RevisionID)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range revision.Files {
				if file.Path == "SKILL.md" && string(file.Data) == syncContractContent("latency", "reconnect edit") && b.link.failedSyncRequests.Load() > beforeOffline {
					break waiting
				}
			}
		}
	}
	b.link.offline.Store(false)
	reconnect := await("reconnect edit")
	t.Logf("production foreground 5-second polling: normal=%s reconnect=%s", normal, reconnect)
}

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
	h.hosted("shelved", "kept on shelf", false, false)
	h.hosted("disabled", "kept disabled", false, false)
	items, err := h.st.LibraryFor(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.ID)
		status := ""
		if item.Name == "reet-shelved" {
			status = store.LibraryOnShelf
		}
		if item.Name == "reet-disabled" {
			status = store.LibraryDisabled
		}
		if status != "" {
			if err := h.st.SetLibraryStatusFor(h.uid, item.ID, status, item.Version); err != nil {
				t.Fatal(err)
			}
		}
	}
	m := h.machine("inactive-machine")
	h.rounds(1, m)
	h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, m.name, "agents", ""), ItemIDs: ids})
	h.rounds(6, m)
	for _, name := range []string{"private-draft", "published-to-test", "shelved", "disabled"} {
		if _, err := os.Stat(m.path(name, "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("inactive authored skill %s was installed: %v", name, err)
		}
	}
}

func TestCanonicalExistingCopiesAdoptWithoutLosingEdits(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		t.Run(fmt.Sprintf("dirty-%v", dirty), func(t *testing.T) {
			h := newSyncContractHarness(t)
			h.hosted("existing", "original", false, true)
			items, err := h.st.LibraryFor(h.uid)
			if err != nil || len(items) != 1 {
				t.Fatal(items, err)
			}
			item := items[0]
			a, b := h.machine("existing-a"), h.machine("existing-b")
			h.rounds(1, a, b)
			for _, m := range []*syncContractMachine{a, b} {
				h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, m.name, "agents", ""), ItemIDs: []string{item.ID}})
			}
			h.rounds(8, a, b)
			a.requireFile("existing", "SKILL.md", syncContractContent("existing", "original"))
			a.stop()
			// A pre-upgrade install has a controller apply baseline but no local edit journal.
			if err := os.Remove(a.config + ".library"); err != nil {
				t.Fatal(err)
			}
			targetID := store.EnvironmentIDFor(h.uid, a.name, "agents", "")
			if dirty {
				h.hosted("existing", "central changed while offline", false, true)
				if err := os.WriteFile(a.path("existing", "SKILL.md"), []byte(syncContractContent("existing", "edit before upgrade")), 0644); err != nil {
					t.Fatal(err)
				}
				saved := time.Now().Add(2 * time.Second)
				if err := os.Chtimes(a.path("existing", "SKILL.md"), saved, saved); err != nil {
					t.Fatal(err)
				}
			}
			// Selecting existing copies during upgrade must establish tracking,
			// including when there is no content difference to trigger an install.
			{
				selected, err := h.st.LibrarySelectionFor(h.uid, targetID)
				if err != nil {
					t.Fatal(err)
				}
				h.mustRequest(http.MethodPut, "/api/v2/library/selections", selected)
			}
			a.start()
			h.rounds(10, a, b)
			if dirty {
				b.requireFile("existing", "SKILL.md", syncContractContent("existing", "edit before upgrade"))
			}
			history, err := h.st.LibraryEditHistoryFor(h.uid, item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if dirty && (len(history) != 1 || history[0].Base != item.RevisionID) {
				t.Fatal("upgrade manufactured a new causal base", history, item.RevisionID)
			}

			if err := os.WriteFile(a.path("existing", "SKILL.md"), []byte(syncContractContent("existing", "edit after upgrade")), 0644); err != nil {
				t.Fatal(err)
			}
			h.rounds(10, a, b)
			b.requireFile("existing", "SKILL.md", syncContractContent("existing", "edit after upgrade"))
		})
	}
}

func TestCanonicalUnknownLocalCopyIsPreservedWhileHealthyItemsSync(t *testing.T) {
	h := newSyncContractHarness(t)
	h.hosted("unknown", "central bytes", false, true)
	h.hosted("healthy", "healthy bytes", false, true)
	a := h.machine("unknown-copy")
	a.localSkill("unknown", "unimported local work")
	h.rounds(1, a)
	items, err := h.st.LibraryFor(h.uid)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, item := range items {
		if item.Name == "reet-unknown" || item.Name == "reet-healthy" {
			ids = append(ids, item.ID)
		}
	}
	h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: store.EnvironmentIDFor(h.uid, a.name, "agents", ""), ItemIDs: ids})
	h.rounds(8, a)
	a.requireFile("unknown", "SKILL.md", syncContractContent("unknown", "unimported local work"))
	a.requireFile("healthy", "SKILL.md", syncContractContent("healthy", "healthy bytes"))
}
