package backup

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

// liveController is a controller with one copy of itself taken, ready to be
// restored over itself.
func liveController(t *testing.T) (svc *Service, st *store.Store, dir, id string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir = t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("owner@example.com", "Owner", "owner", "x"); err != nil {
		t.Fatal(err)
	}
	cfg, err := FromEnv(func(k string) string {
		if k == "REET_DATA_DIR" {
			return dir
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	svc = New(st, cfg)
	item, err := svc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return svc, st, dir, item.ID
}

func liveBytes(t *testing.T, dir string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func noAsideCopy(t *testing.T, dir string) {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, "reet.db.before-restore-*"))
	if len(m) != 0 {
		t.Fatalf("a refused restore moved the database aside: %v", m)
	}
}

func TestARestoreForAnOwnerTheCopyLacksIsRefusedBeforeTheSwap(t *testing.T) {
	svc, st, dir, id := liveController(t)
	before := liveBytes(t, dir)
	closed := false
	_, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{OwnerEmail: "nobody@example.com"},
		func() error { closed = true; return st.Close() })
	if err == nil || !strings.Contains(err.Error(), "nobody@example.com") {
		t.Fatalf("want a refusal naming the address, got %v", err)
	}
	if closed {
		t.Fatal("the store was closed for a restore that was refused")
	}
	if !bytes.Equal(before, liveBytes(t, dir)) {
		t.Fatal("a refused restore changed reet.db")
	}
	noAsideCopy(t, dir)
}

func TestARestoreForAnOwnerAnAccountThatIsOnlyAMemberIsRefused(t *testing.T) {
	svc, st, dir, _ := liveController(t)
	if _, err := st.CreateUser("sam@example.com", "Sam", "member", "x"); err != nil {
		t.Fatal(err)
	}
	item, err := svc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreInstance(context.Background(), svc, dir, item.ID, "", RestoreOptions{OwnerEmail: "sam@example.com"}, st.Close); err == nil {
		t.Fatal("a member is not a way in as an owner")
	}
}

func TestARestoreForAnOwnerTheCopyHasGoesAheadCaseInsensitively(t *testing.T) {
	svc, st, dir, id := liveController(t)
	res, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{OwnerEmail: "Owner@Example.com"}, st.Close)
	if err != nil {
		t.Fatal(err)
	}
	if res.Previous == "" {
		t.Fatal("nothing was swapped")
	}
}

func TestACheckOnlyRestoreReportsAndChangesNothing(t *testing.T) {
	svc, st, dir, id := liveController(t)
	before := liveBytes(t, dir)
	var seen RestoreReport
	res, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{CheckOnly: true, Before: func(r RestoreReport) { seen = r }}, st.Close)
	if err != nil {
		t.Fatal(err)
	}
	if seen.UsersByRole["owner"] != 1 || res.Report.Users != 1 {
		t.Fatalf("report: %+v", seen)
	}
	if !bytes.Equal(before, liveBytes(t, dir)) || res.Previous != "" {
		t.Fatal("a check changed the live database")
	}
	noAsideCopy(t, dir)
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".reet-restore-*")); len(leftovers) != 0 {
		t.Fatalf("plaintext left behind: %v", leftovers)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestARestoreFromANewerCopyIsRefusedNamingBothVersionsAndChangesNothing(t *testing.T) {
	svc, st, dir, _ := liveController(t)
	src := t.TempDir()
	other, err := store.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	other.Close()
	db, err := sql.Open("sqlite", filepath.Join(src, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version=999`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	payload, err := os.ReadFile(filepath.Join(src, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := svc.cfg.Sealer.Seal(Manifest{ID: "instance-20990101T000000Z-deadbeef", Kind: "instance", Target: "instance", Version: "9.9.9"}, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(svc.cfg.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc.cfg.Dir, "instance-20990101T000000Z-deadbeef.reetbackup"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	before := liveBytes(t, dir)
	_, err = RestoreInstance(context.Background(), svc, dir, "instance-20990101T000000Z-deadbeef", "", RestoreOptions{}, st.Close)
	if !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("want ErrNewerSchema, got %v", err)
	}
	for _, want := range []string{"999", "9.9.9", fmt.Sprintf("data format %d", store.SchemaVersion)} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal %q does not name %s", err, want)
		}
	}
	if !bytes.Equal(before, liveBytes(t, dir)) {
		t.Fatal("a refused restore changed reet.db")
	}
	noAsideCopy(t, dir)
}

func TestARestoreWritesItsChoicesToTheActivityLog(t *testing.T) {
	svc, st, dir, id := liveController(t)
	if _, err := RestoreInstance(context.Background(), svc, dir, id, "", RestoreOptions{DropSessions: true}, st.Close); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	evs, err := restored.ListEvents(20, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Action == "backup-restored" {
			if !strings.Contains(e.Detail, `"sessionsDropped":true`) || !strings.Contains(e.Detail, `"tokensDropped":false`) || e.Source != "cli" {
				t.Fatalf("event: %+v", e)
			}
			return
		}
	}
	t.Fatal("the restore left no line in the activity log")
}

// A controller that has just started already holds the topic tags the daily
// sync added and the report the server makes of its own machine. They are not
// data anyone made, so they must not make a new controller refuse an import.
func TestANewControllersOwnStartupDefaultsDoNotCountAsData(t *testing.T) {
	raw, _ := movable(t)
	svc, dir := destination(t)
	owner, err := svc.st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.st.ReplaceTopics([]store.Topic{{Slug: "frontend", Name: "Frontend & React", Skills: []string{"acme/skills/button"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.st.RegisterMachineFor(owner.ID, store.MachineInfo{Host: "vm", Server: true}); err != nil {
		t.Fatal(err)
	}
	h := svc.st.Holdings(context.Background())
	if h.Tags != 1 || h.Environments != 1 {
		t.Fatalf("the fixture should hold the defaults: %+v", h)
	}
	if _, err := svc.StageImport(context.Background(), dir, raw, copyPassword, RestoreOptions{}); err != nil {
		t.Fatalf("a new controller was refused: %v", err)
	}
}

func TestATagOrMachineAPersonMadeStillCountsAsData(t *testing.T) {
	raw, _ := movable(t)
	for name, add := range map[string]func(*store.Store, store.User) error{
		"a tag": func(st *store.Store, _ store.User) error { _, err := st.CreateTag("mine"); return err },
		"a machine": func(st *store.Store, u store.User) error {
			return st.RegisterMachineFor(u.ID, store.MachineInfo{Host: "laptop"})
		},
	} {
		svc, dir := destination(t)
		owner, err := svc.st.EnsureLocalOwner()
		if err != nil {
			t.Fatal(err)
		}
		if err := add(svc.st, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.StageImport(context.Background(), dir, raw, copyPassword, RestoreOptions{}); !errors.Is(err, ErrNotEmpty) {
			t.Fatalf("%s must count as data, got %v", name, err)
		}
	}
}

func TestLibraryOnlyDataCannotBeReplacedAsAnEmptyController(t *testing.T) {
	raw, _ := movable(t)
	svc, dir := destination(t)
	owner, err := svc.st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	base := ""
	_, err = svc.st.SaveLibraryContentFor(owner.ID, "", store.PrivateSkillWrite{Name: "review", RequestID: "library_backup_0001", ExpectedBase: &base, ItemType: store.LibraryAgent, Application: "claude", Format: "markdown", Files: []skillbundle.ContentFile{{Path: "review.md", Data: []byte("---\nname: review\ndescription: Review\n---\nInspect the diff.\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	holdings := svc.st.Holdings(context.Background())
	if holdings.Skills != 0 || holdings.Shelves != 0 || holdings.Machines != 0 || holdings.LibraryItems != 1 || holdings.PrivateBundles != 1 {
		t.Fatal("fixture is not a library-only controller", holdings)
	}
	before := liveBytes(t, dir)
	if _, err = svc.StageImport(context.Background(), dir, raw, copyPassword, RestoreOptions{}); !errors.Is(err, ErrNotEmpty) {
		t.Fatal("library-only user data was treated as an empty controller", err)
	}
	if !bytes.Equal(before, liveBytes(t, dir)) {
		t.Fatal("refused import changed library data")
	}
	noAsideCopy(t, dir)
}
