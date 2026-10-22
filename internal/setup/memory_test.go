package setup

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func memoryFixture(t *testing.T) (environment, string) {
	t.Helper()
	e := fixture(t)
	path := filepath.Join(e.home, ".codex", "memories_1.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE memories (body TEXT); INSERT INTO memories(body) VALUES ('reusable memory')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return e, path
}

func TestCodexMemoryCaptureRestoreAndVerify(t *testing.T) {
	e, sourcePath := memoryFixture(t)
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Preview.Memories) != 1 {
		t.Fatalf("memory not selected: %+v", selection.Preview.Memories)
	}
	memory := selection.Preview.Memories[0]
	if memory.Name != "memories_1.sqlite" || memory.AgentID != "codex" || memory.Bytes == 0 || memory.Digest == "" {
		t.Fatalf("wrong memory preview: %+v", memory)
	}
	if data, err := os.ReadFile(sourcePath); err != nil || len(data) == 0 {
		t.Fatalf("source memory missing: %v", err)
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	dest := filepath.Join(destination.home, ".codex", "memories_1.sqlite")
	action := findRestoreAction(t, plan, dest)
	if action.State != "create" || action.Format != "sqlite" {
		t.Fatalf("wrong memory action: %+v", action)
	}
	if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	updated, err := discover(fixtureWithHome(t, destination.home))
	if err != nil {
		t.Fatal(err)
	}
	verified := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("setup-memory"), updated)
	found := false
	for _, component := range verified.Components {
		if component.Kind == "memory" && component.State == "verified" {
			found = true
		}
	}
	if !found {
		t.Fatalf("memory not verified: %+v", verified)
	}
	db, err := sql.Open("sqlite", dest)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var body string
	if err := db.QueryRow(`SELECT body FROM memories`).Scan(&body); err != nil || body != "reusable memory" {
		t.Fatalf("restored memory content wrong: %q %v", body, err)
	}
}

func TestCodexMemoryRestoreBlocksWithSidecars(t *testing.T) {
	e, _ := memoryFixture(t)
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	dest := filepath.Join(destination.home, ".codex", "memories_1.sqlite")
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	action := findRestoreAction(t, plan, dest)
	if action.State != "blocked" || action.Reason != "destination_sqlite_sidecars_present" {
		t.Fatalf("sidecars not blocked: %+v", action)
	}
	if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()}); err == nil {
		t.Fatal("blocked memory restore was applied")
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "db" {
		t.Fatal("blocked memory restore changed the destination")
	}
}

func TestMemoryProfileOverride(t *testing.T) {
	e, _ := memoryFixture(t)
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := BuildSetupSnapshot(selection)
	filtered := ApplyProfileOverrides(snapshot, &SetupProfileOverrides{ExcludeKinds: []string{"memory"}}, "", "")
	if len(filtered.Memories) != 0 || len(filtered.Preview.Memories) != 0 {
		t.Fatalf("memory override did not remove content: %+v", filtered.Preview.Memories)
	}
	found := false
	for _, agent := range filtered.Preview.Agents {
		for _, omitted := range agent.Omitted {
			if omitted.Kind == "memory" && strings.Contains(omitted.Reason, "profile_excluded") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("memory omission missing: %+v", filtered.Preview.Agents)
	}
}
