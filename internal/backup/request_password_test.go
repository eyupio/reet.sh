package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const requestPassword = "a password given for one copy"

func archiveOf(t *testing.T, svc *Service, id string) []byte {
	t.Helper()
	raw, err := svc.Archive(context.Background(), 0, id)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// A copy taken with a password on the request opens with that password, and
// not with the controller's key or any other password.
func TestACopyTakenWithARequestPasswordOpensWithItAndNotWithTheKey(t *testing.T) {
	svc, _, _, _ := liveController(t)
	item, err := svc.RunInstanceWithPassword(context.Background(), "manual", requestPassword)
	if err != nil {
		t.Fatal(err)
	}
	raw := archiveOf(t, svc, item.ID)
	m, err := ManifestOf(raw)
	if err != nil || m.Protection != ProtectionPassword || !m.Encrypted {
		t.Fatalf("manifest: %+v %v", m, err)
	}
	if _, payload, err := OpenArchive(raw, requestPassword, ""); err != nil || len(payload) == 0 {
		t.Fatalf("the request password did not open the copy: %v", err)
	}
	for name, try := range map[string]func() error{
		"the controller's key": func() error { _, _, err := OpenArchive(raw, "", svc.cfg.Key); return err },
		"no password":          func() error { _, _, err := OpenArchive(raw, "", ""); return err },
		"another password":     func() error { _, _, err := OpenArchive(raw, "some other long password", ""); return err },
		"the sealer's key":     func() error { _, _, err := svc.cfg.Sealer.Open(raw); return err },
	} {
		if try() == nil {
			t.Errorf("the copy opened with %s", name)
		}
	}
}

// With no request password a copy is sealed as it was: under the key when no
// copy password is stored, and under the stored password when one is.
func TestACopyWithNoRequestPasswordIsSealedAsBefore(t *testing.T) {
	svc, _, _, id := liveController(t)
	if m, _ := ManifestOf(archiveOf(t, svc, id)); m.Protection == ProtectionPassword {
		t.Fatal("a copy with no password anywhere was password protected")
	}
	stored := "the stored copy password"
	if _, err := svc.SaveSettings(0, SettingsInput{Password: &stored, Destination: "local", Compression: CompressionNone}); err != nil {
		t.Fatal(err)
	}
	item, err := svc.RunInstance(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	raw := archiveOf(t, svc, item.ID)
	if _, _, err := OpenArchive(raw, stored, ""); err != nil {
		t.Fatalf("the stored password no longer seals a plain run: %v", err)
	}
}

// The stored setting is not read, changed or cleared by a request password.
func TestARequestPasswordNeitherReadsNorChangesTheStoredSetting(t *testing.T) {
	svc, st, _, _ := liveController(t)
	stored := "the stored copy password"
	if _, err := svc.SaveSettings(0, SettingsInput{Password: &stored, Destination: "local", Compression: CompressionNone}); err != nil {
		t.Fatal(err)
	}
	before, err := st.BackupSettingsFor(0)
	if err != nil || before.PasswordSealed == "" {
		t.Fatalf("setup: %+v %v", before, err)
	}
	item, err := svc.RunInstanceWithPassword(context.Background(), "manual", requestPassword)
	if err != nil {
		t.Fatal(err)
	}
	raw := archiveOf(t, svc, item.ID)
	if _, _, err := OpenArchive(raw, requestPassword, ""); err != nil {
		t.Fatalf("the request password did not open its copy: %v", err)
	}
	if _, _, err := OpenArchive(raw, stored, ""); err == nil {
		t.Fatal("the stored password opened a copy sealed with the request password")
	}
	after, _ := st.BackupSettingsFor(0)
	if after.PasswordSealed != before.PasswordSealed || after.EverySeconds != before.EverySeconds || after.Keep != before.Keep || after.Destination != before.Destination {
		t.Fatalf("the stored setting changed:\nbefore %+v\nafter  %+v", before, after)
	}
	// The next plain copy is sealed with the stored password, as it was.
	next, err := svc.RunInstance(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenArchive(archiveOf(t, svc, next.ID), stored, ""); err != nil {
		t.Fatalf("the stored password no longer seals later copies: %v", err)
	}
}

// Not read: a stored value nothing can open does not stop a request-password copy.
func TestARequestPasswordDoesNotNeedTheStoredOneToBeReadable(t *testing.T) {
	svc, st, _, _ := liveController(t)
	broken, err := st.BackupSettingsFor(0)
	if err != nil {
		t.Fatal(err)
	}
	broken.PasswordSealed = "not-a-sealed-secret"
	if err := st.SaveBackupSettingsFor(broken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunInstance(context.Background(), "manual"); err == nil {
		t.Fatal("setup: a plain run read the unreadable stored password without complaint")
	}
	if _, err := svc.RunInstanceWithPassword(context.Background(), "manual", requestPassword); err != nil {
		t.Fatalf("the request password run read the stored setting: %v", err)
	}
}

func TestAShortOrBlankRequestPasswordIsRefusedAndNothingIsTaken(t *testing.T) {
	svc, st, _, _ := liveController(t)
	copies := func() int { items, _ := svc.LocalItems(0); return len(items) }
	runs := func() int { r, _ := st.ListBackupRuns(0, 50); return len(r) }
	c, r := copies(), runs()
	for _, bad := range []string{"", "   ", "\t\n", "short", "11 chars!!!"} {
		_, err := svc.RunInstanceWithPassword(context.Background(), "manual", bad)
		if err == nil || !strings.Contains(err.Error(), "a copy password needs at least 12 characters") {
			t.Fatalf("%q: %v", bad, err)
		}
		if bad != "" && strings.Contains(err.Error(), bad) {
			t.Fatalf("the refusal repeats the password: %v", err)
		}
	}
	if copies() != c || runs() != r {
		t.Fatalf("something was taken: %d copies (was %d), %d runs (was %d)", copies(), c, runs(), r)
	}
	// Exactly twelve is allowed, as it is for the setting.
	if _, err := svc.RunInstanceWithPassword(context.Background(), "manual", "twelve chars"); err != nil {
		t.Fatalf("a twelve character password was refused: %v", err)
	}
}

// The password is in no row of the database and no byte of a copy or the
// response: the copy holds only what it seals.
func TestTheRequestPasswordIsNeverStored(t *testing.T) {
	svc, st, dir, _ := liveController(t)
	secret := "never-written-anywhere-1234"
	item, err := svc.RunInstanceWithPassword(context.Background(), "manual", secret)
	if err != nil {
		t.Fatal(err)
	}
	answer, _ := json.Marshal(item)
	if bytes.Contains(answer, []byte(secret)) {
		t.Fatal("the response carries the password")
	}
	if bytes.Contains(archiveOf(t, svc, item.ID), []byte(secret)) {
		t.Fatal("the copy carries the password in the clear")
	}
	// Every row of every table, from a snapshot of the live database.
	snap := dir + "/snapshot.db"
	if err := st.VacuumInto(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(snap)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) {
		t.Fatal("the database holds the password")
	}
	runs, _ := st.ListBackupRuns(0, 50)
	if enc, _ := json.Marshal(runs); bytes.Contains(enc, []byte(secret)) {
		t.Fatal("a run record holds the password")
	}
}
