package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func pruneFixture(t *testing.T) (string, []SetupWriteResult) {
	t.Helper()
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"PRUNE-MODEL"}`)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var snapshots []SetupWriteResult
	for i := 0; i < 5; i++ {
		result, err := WriteSetupSnapshot(selection, SetupWriteOptions{
			Dir: dir, Password: testSetupPassphrase,
			Now: func() time.Time { return time.Date(2026, 10, 22, 10+i, 0, 0, 0, time.UTC) },
		})
		if err != nil {
			t.Fatal(err)
		}
		snapshots = append(snapshots, result)
	}
	return dir, snapshots
}

func TestPruneSetupSnapshotsProtectsProfilesAndJournals(t *testing.T) {
	dir, snapshots := pruneFixture(t)
	if _, _, err := SaveSetupProfile("old", testSetupPassphrase, snapshots[1].ID, dir, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	journal := restoreJournal{
		SchemaVersion: restoreJournalSchemaVersion, SnapshotID: snapshots[2].ID,
		PlanFingerprint: "test", State: "completed", StartedAt: time.Now().UTC(),
		Actions: []restoreJournalAction{{Destination: "/tmp/protected", State: "created"}},
	}
	if err := writeRestoreJournal(filepath.Join(dir, "restore-protect.json"), journal); err != nil {
		t.Fatal(err)
	}
	dry, err := PruneSetupSnapshots(dir, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.WouldRemove) != 1 || dry.WouldRemove[0] != snapshots[0].Path || len(dry.Removed) != 0 {
		t.Fatalf("dry run wrong: %+v", dry)
	}
	if len(dry.Protected) != 2 {
		t.Fatalf("protected snapshots wrong: %+v", dry.Protected)
	}
	if _, err := os.Stat(snapshots[0].Path); err != nil {
		t.Fatal("dry run deleted a snapshot")
	}
	applied, err := PruneSetupSnapshots(dir, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Removed) != 1 || applied.Removed[0] != snapshots[0].Path {
		t.Fatalf("applied prune wrong: %+v", applied)
	}
	if _, err := os.Stat(snapshots[0].Path); !os.IsNotExist(err) {
		t.Fatal("old unprotected snapshot still exists")
	}
	for _, index := range []int{1, 2, 3, 4} {
		if _, err := os.Stat(snapshots[index].Path); err != nil {
			t.Fatalf("protected/new snapshot %d removed", index)
		}
	}
	if _, err := PruneSetupSnapshots(dir, 0, false); err == nil {
		t.Fatal("zero keep accepted")
	}
}

func TestSetupPruneCLI(t *testing.T) {
	dir, snapshots := pruneFixture(t)
	var out bytes.Buffer
	if err := Run([]string{"prune", "--keep", "2", "--dir", dir, "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var dry SetupPruneResult
	if err := json.Unmarshal(out.Bytes(), &dry); err != nil || len(dry.WouldRemove) != 3 || len(dry.Removed) != 0 {
		t.Fatalf("CLI dry run: %+v %v", dry, err)
	}
	var applied bytes.Buffer
	if err := Run([]string{"prune", "--keep", "2", "--apply", "--dir", dir, "--json"}, &applied); err != nil {
		t.Fatal(err)
	}
	var result SetupPruneResult
	if err := json.Unmarshal(applied.Bytes(), &result); err != nil || len(result.Removed) != 3 {
		t.Fatalf("CLI apply: %+v %v", result, err)
	}
	if _, err := os.Stat(snapshots[0].Path); !os.IsNotExist(err) {
		t.Fatal("CLI prune did not remove oldest")
	}
}
