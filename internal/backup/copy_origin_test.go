package backup

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func manifestOfCopy(t *testing.T, svc *Service, id string) Manifest {
	t.Helper()
	raw, err := svc.Archive(context.Background(), 0, id)
	if err != nil {
		t.Fatal(err)
	}
	m, err := ManifestOf(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func fencedCopy(t *testing.T, svc *Service, st *store.Store) string {
	t.Helper()
	if err := st.SetSetting(store.SettingFenceSince, "1760000000000"); err != nil {
		t.Fatal(err)
	}
	item, err := svc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return item.ID
}

func TestACopyRecordsItsSourceAndWhetherItWasFenced(t *testing.T) {
	svc, st, _, id := liveController(t)
	live := manifestOfCopy(t, svc, id)
	want, err := st.ControllerID()
	if err != nil || want == "" {
		t.Fatalf("controller id %q: %v", want, err)
	}
	if live.Source != want || live.Fenced == nil || *live.Fenced {
		t.Fatalf("an unfenced controller's copy says source %q fenced %v", live.Source, live.Fenced)
	}
	fenced := manifestOfCopy(t, svc, fencedCopy(t, svc, st))
	if fenced.Source != want || fenced.Fenced == nil || !*fenced.Fenced {
		t.Fatalf("a fenced controller's copy says source %q fenced %v", fenced.Source, fenced.Fenced)
	}
}

func TestTheSourceIdentifierIsStableAndSurvivesAChangeOfSettings(t *testing.T) {
	_, st, _, _ := liveController(t)
	a, _ := st.ControllerID()
	b, _ := st.ControllerID()
	if a == "" || a != b || !strings.HasPrefix(a, "ctl_") {
		t.Fatalf("identifier changed between calls: %q %q", a, b)
	}
}

func TestACopyWithNoMarkersReadsAsUnknown(t *testing.T) {
	svc, _ := destination(t)
	raw := sealedAtVersion(t, svc, "instance-old", store.SchemaVersion)
	m, err := ManifestOf(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "" || m.Fenced != nil || sourceState(m) != "unknown" {
		t.Fatalf("a copy with no markers reads as %q (source %q)", sourceState(m), m.Source)
	}
}

func TestARestoreOfAnUnfencedCopyIsRefusedWithoutTheFlagAndChangesNothing(t *testing.T) {
	svc, st, dir, id := liveController(t)
	before := liveBytes(t, dir)
	_, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{}, st.Close)
	if !errors.Is(err, ErrLiveSource) {
		t.Fatalf("want ErrLiveSource, got %v", err)
	}
	for _, want := range []string{"two controllers", "out of date", "--allow-live-source"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal %q does not say %q", err, want)
		}
	}
	if !bytes.Equal(before, liveBytes(t, dir)) {
		t.Fatal("a refused restore changed reet.db")
	}
	noAsideCopy(t, dir)
}

func TestARestoreOfAnUnfencedCopyGoesAheadWithTheFlagAndIsLogged(t *testing.T) {
	svc, st, dir, id := liveController(t)
	if _, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{AllowLiveSource: true}, st.Close); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	evs, _ := restored.ListEvents(20, "")
	for _, e := range evs {
		if e.Action == "backup-restored" {
			if !strings.Contains(e.Detail, `"allowLiveSource":true`) || !strings.Contains(e.Detail, `"sourceState":"live"`) {
				t.Fatalf("the activity log does not record the choice: %s", e.Detail)
			}
			return
		}
	}
	t.Fatal("no activity line")
}

func TestARestoreOfAFencedCopyNeedsNoFlagAndTheRestoredControllerIsNotFenced(t *testing.T) {
	svc, st, dir, _ := liveController(t)
	id := fencedCopy(t, svc, st)
	if _, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{}, st.Close); err != nil {
		t.Fatalf("a fenced copy was refused: %v", err)
	}
	restored, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if since, _ := restored.FenceSince(); since != 0 {
		t.Fatal("the restored controller came up fenced; the copy's fence must not come across")
	}
}

func TestAnImportOfAnUnfencedOrUnmarkedCopyIsRefusedWithoutTheFlag(t *testing.T) {
	// movable's copy is taken from a controller that was not fenced.
	unfenced, _ := movable(t)
	for name, tc := range map[string]struct {
		raw      []byte
		password string
	}{"unfenced": {unfenced, copyPassword}} {
		t.Run(name, func(t *testing.T) {
			svc, dir := destination(t)
			if _, err := svc.StageImport(context.Background(), dir, tc.raw, tc.password, RestoreOptions{}); !errors.Is(err, ErrLiveSource) {
				t.Fatalf("want ErrLiveSource, got %v", err)
			}
			if p, _ := PendingImport(dir); p != nil {
				t.Fatal("a refused copy was staged")
			}
			if _, err := svc.StageImport(context.Background(), dir, tc.raw, tc.password, RestoreOptions{AllowLiveSource: true}); err != nil {
				t.Fatalf("with the flag: %v", err)
			}
		})
	}
	t.Run("no markers", func(t *testing.T) {
		svc, dir := destination(t)
		raw := sealedAtVersion(t, svc, "instance-nomarkers", store.SchemaVersion)
		if _, err := svc.StageImport(context.Background(), dir, raw, "", RestoreOptions{}); !errors.Is(err, ErrLiveSource) || !strings.Contains(err.Error(), "does not say") {
			t.Fatalf("want an unknown-source refusal, got %v", err)
		}
		if _, err := svc.StageImport(context.Background(), dir, raw, "", RestoreOptions{AllowLiveSource: true}); err != nil {
			t.Fatalf("with the flag: %v", err)
		}
	})
}

func TestAnImportOfAFencedCopyNeedsNoFlag(t *testing.T) {
	svc, st, _, _ := liveController(t)
	id := fencedCopy(t, svc, st)
	raw, err := svc.Archive(context.Background(), 0, id)
	if err != nil {
		t.Fatal(err)
	}
	// Same backup key on the other side, so the copy opens without a password.
	other := New(mustStore(t), svc.cfg)
	dir := other.st.Dir()
	if _, err := other.StageImport(context.Background(), dir, raw, "", RestoreOptions{}); err != nil {
		t.Fatalf("a fenced copy was refused: %v", err)
	}
}

func mustStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// A fenced controller takes no scheduled copy; the owner's own run still works
// and is marked fenced, and the schedule goes on after the lift.
func TestAFencedControllerTakesNoScheduledCopyButAManualOneWorks(t *testing.T) {
	svc, st, _, _ := liveController(t)
	if _, err := svc.SaveSettings(0, SettingsInput{EverySeconds: 60, Keep: 10, Destination: "local", Compression: CompressionNone}); err != nil {
		t.Fatal(err)
	}
	// Make the next scheduled copy due now.
	due, err := st.BackupSettingsFor(0)
	if err != nil {
		t.Fatal(err)
	}
	due.NextAt = 1
	if err := st.SaveBackupSettingsFor(due); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		items, err := svc.LocalItems(0)
		if err != nil {
			t.Fatal(err)
		}
		return len(items)
	}
	before := count()
	if _, err := st.SetFence(""); err != nil {
		t.Fatal(err)
	}
	svc.runDue(context.Background())
	if count() != before {
		t.Fatal("a scheduled copy was taken while fenced")
	}
	if _, err := st.LiftFence(); err != nil {
		t.Fatal(err)
	}
	after := count()
	svc.runDue(context.Background())
	if count() != after+1 {
		t.Fatal("the schedule did not run after the lift")
	}
	if _, err := st.SetFence(""); err != nil {
		t.Fatal(err)
	}
	item, err := svc.RunInstance(context.Background(), "cli")
	if err != nil {
		t.Fatalf("the owner's own run was refused while fenced: %v", err)
	}
	if m := manifestOfCopy(t, svc, item.ID); m.Fenced == nil || !*m.Fenced {
		t.Fatal("a copy taken while fenced is not marked fenced")
	}
}
