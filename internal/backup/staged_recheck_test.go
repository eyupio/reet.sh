package backup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// sealedAtVersion is a whole-instance copy of an empty controller whose data
// format is stamped as version, sealed under svc's key.
func sealedAtVersion(t *testing.T, svc *Service, id string, version int) []byte {
	t.Helper()
	src := t.TempDir()
	st, err := store.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("owner@example.com", "Owner", "owner", "x"); err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", filepath.Join(src, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, version)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	payload, err := os.ReadFile(filepath.Join(src, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := svc.cfg.Sealer.Seal(Manifest{ID: id, Kind: "instance", Target: "instance"}, payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Copies taken before data formats were recorded (0) and by the release before
// this one (one below the current) must keep restoring.
func TestACopyFromAnOlderDataFormatStillStages(t *testing.T) {
	for _, version := range []int{0, store.SchemaVersion - 1} {
		t.Run(fmt.Sprintf("format %d", version), func(t *testing.T) {
			svc, dir := destination(t)
			raw := sealedAtVersion(t, svc, "instance-older", version)
			rep, err := svc.StageImport(context.Background(), dir, raw, "", RestoreOptions{AllowLiveSource: true})
			if err != nil {
				t.Fatalf("an older copy was refused: %v", err)
			}
			if rep.SchemaVersion != version {
				t.Fatalf("report says data format %d, want %d", rep.SchemaVersion, version)
			}
			if p, _ := PendingImport(dir); p == nil {
				t.Fatal("the older copy was not staged")
			}
		})
	}
}

func stagedController(t *testing.T) (dir string) {
	t.Helper()
	svc, dir := destination(t)
	raw := sealedAtVersion(t, svc, "instance-staged", store.SchemaVersion)
	if _, err := svc.StageImport(context.Background(), dir, raw, "", RestoreOptions{AllowLiveSource: true}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A staged import can wait for days. If the file changed in that time it is not
// the copy that was checked, and the live database must stay as it is.
func TestAStagedImportThatChangedBeforeItIsAppliedIsRefusedAndLeavesTheLiveDatabaseAlone(t *testing.T) {
	dir := stagedController(t)
	live := filepath.Join(dir, "reet.db")
	before, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, pendingDir, pendingDB), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("tampered")
	f.Close()

	res, err := ApplyPendingImport(dir)
	if err == nil || res != nil {
		t.Fatalf("a changed stage was applied: %v %v", res, err)
	}
	if !strings.Contains(err.Error(), "changed after it was checked") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
	after, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("a refused stage changed reet.db")
	}
	if p, _ := PendingImport(dir); p != nil {
		t.Fatal("the refused stage is still pending and would be tried again")
	}
	aside, _ := filepath.Glob(filepath.Join(dir, pendingDir+".refused-*", pendingState))
	if len(aside) != 1 {
		t.Fatalf("the refused stage was not kept with its report: %v", aside)
	}
	raw, _ := os.ReadFile(aside[0])
	if !strings.Contains(string(raw), `"refusal"`) || !strings.Contains(string(raw), "changed after it was checked") {
		t.Fatalf("the report beside the refused stage does not carry the reason: %s", raw)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "reet.db.before-restore-*")); len(m) != 0 {
		t.Fatal("a refused stage moved the live database aside")
	}
}

// The binary may have been downgraded between staging and the next start.
func TestAStagedImportNewerThanTheBinaryIsRefusedAtApply(t *testing.T) {
	dir := stagedController(t)
	staged := filepath.Join(dir, pendingDir, pendingDB)
	db, err := sql.Open("sqlite", staged)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version=999`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	// Keep the digest honest, so it is the data format that refuses.
	rep, _ := PendingImport(dir)
	rep.StagedSHA256, _ = fileDigest(staged)
	state, _ := jsonMarshal(rep)
	if err := os.WriteFile(filepath.Join(dir, pendingDir, pendingState), state, 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "reet.db"))
	if _, err := ApplyPendingImport(dir); err == nil || !strings.Contains(err.Error(), "data format 999") {
		t.Fatalf("want a refusal naming data format 999, got %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "reet.db"))
	if !bytes.Equal(before, after) {
		t.Fatal("a refused stage changed reet.db")
	}
}

func TestAnUntouchedStagedImportStillApplies(t *testing.T) {
	dir := stagedController(t)
	res, err := ApplyPendingImport(dir)
	if err != nil || res == nil {
		t.Fatalf("an untouched stage was refused: %v %v", res, err)
	}
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
