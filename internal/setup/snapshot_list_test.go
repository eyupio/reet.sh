package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
)

func TestListSetupSnapshotsReadsHeadersAndSkipsUnsafeFiles(t *testing.T) {
	selection := fixtureSelection(t)
	dir := t.TempDir()
	first, err := WriteSetupSnapshot(selection, SetupWriteOptions{
		Dir: dir, Password: testSetupPassphrase,
		Now: func() time.Time { return time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := WriteSetupSnapshot(selection, SetupWriteOptions{
		Dir: dir, Password: testSetupPassphrase,
		Now: func() time.Time { return time.Date(2026, 10, 22, 11, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "invalid.reetsetup"), []byte("not an archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".reet-setup-temp"), []byte("temporary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	sealer, _ := sealed.NewSealer(strings.Repeat("0", 32), "test")
	other, err := sealer.Seal(sealed.Manifest{ID: "instance-1", Kind: "instance", Target: "instance"}, []byte("other"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.reetsetup"), other, 0o600); err != nil {
		t.Fatal(err)
	}
	symlinked := false
	if err := os.Symlink(second.Path, filepath.Join(dir, "linked.reetsetup")); err == nil {
		symlinked = true
	}

	list, err := ListSetupSnapshots(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Snapshots) != 2 || list.Snapshots[0].ID != second.ID || list.Snapshots[1].ID != first.ID {
		t.Fatalf("wrong snapshots or order: %+v", list.Snapshots)
	}
	if list.Snapshots[0].Protection != "password" || list.Snapshots[0].Compression != "gzip" || list.Snapshots[0].Bytes == 0 {
		t.Fatalf("wrong header: %+v", list.Snapshots[0])
	}
	reasons := map[string]bool{}
	for _, skipped := range list.Skipped {
		reasons[skipped.Reason] = true
	}
	for _, want := range []string{"invalid_header", "incomplete_temporary_file", "not_setup_snapshot"} {
		if !reasons[want] {
			t.Fatalf("missing skip reason %q: %+v", want, list.Skipped)
		}
	}
	if symlinked && !reasons["symlink"] {
		t.Fatalf("symlink was not skipped: %+v", list.Skipped)
	}
	encoded, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "SYNTHETIC-WRITE-SECRET-DO-NOT-RETURN") {
		t.Fatal("listing exposed a value")
	}
}

func TestInspectSetupSnapshotRequiresPasswordAndHidesValues(t *testing.T) {
	selection := fixtureSelection(t)
	dir := t.TempDir()
	result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	locked, err := InspectSetupSnapshot(result.ID, "", dir)
	if err != nil {
		t.Fatal(err)
	}
	if locked.Opened || locked.Preview != nil || locked.Header.ID != result.ID || locked.Header.Bytes == 0 {
		t.Fatalf("locked inspection wrong: %+v", locked)
	}
	if _, err := InspectSetupSnapshot(result.ID, "the wrong passphrase", dir); err == nil {
		t.Fatal("a wrong passphrase opened the snapshot")
	}
	opened, err := InspectSetupSnapshot(result.ID, testSetupPassphrase, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !opened.Opened || opened.Preview == nil || len(opened.Sources) == 0 {
		t.Fatalf("opened inspection incomplete: %+v", opened)
	}
	for _, source := range opened.Sources {
		if source.Digest == "" || (source.Fields == 0 && source.Declarations == 0) {
			t.Fatalf("missing source summary: %+v", source)
		}
	}
	if opened.Preview.Agents[0].Omitted == nil {
		t.Fatal("omissions missing from opened inspection")
	}
	for _, value := range []any{locked, opened} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"SYNTHETIC-WRITE-SECRET-DO-NOT-RETURN", "MODEL", testSetupPassphrase} {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("inspection exposed %q", secret)
			}
		}
	}
	var text bytes.Buffer
	if err := renderSnapshotInspection(&text, opened); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Setup snapshot") || strings.Contains(text.String(), "SYNTHETIC-WRITE-SECRET-DO-NOT-RETURN") {
		t.Fatalf("bad inspection text: %s", text.String())
	}
}

func TestSetupListAndInspectCLI(t *testing.T) {
	home := isolatedCLI(t)
	const secret = "SYNTHETIC-LIST-CLI-SECRET"
	writeConfig(t, filepath.Join(home, ".claude.json"), `{"model":"`+secret+`"}`)
	dir := t.TempDir()
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var written bytes.Buffer
	if err := Run([]string{"backup", "--password-file", passFile, "--dir", dir, "--json"}, &written); err != nil {
		t.Fatal(err)
	}
	var result SetupWriteResult
	if err := json.Unmarshal(written.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var listOut bytes.Buffer
	if err := Run([]string{"list", "--dir", dir, "--json"}, &listOut); err != nil {
		t.Fatal(err)
	}
	var list SetupSnapshotList
	if err := json.Unmarshal(listOut.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Snapshots) != 1 || list.Snapshots[0].ID != result.ID {
		t.Fatalf("wrong CLI list: %+v", list)
	}
	var lockedOut bytes.Buffer
	if err := Run([]string{"inspect", "--dir", dir, "--json", result.ID}, &lockedOut); err != nil {
		t.Fatal(err)
	}
	var locked SetupSnapshotInspection
	if err := json.Unmarshal(lockedOut.Bytes(), &locked); err != nil {
		t.Fatal(err)
	}
	if locked.Opened || strings.Contains(lockedOut.String(), secret) {
		t.Fatal("locked CLI inspection wrong")
	}
	var openedOut bytes.Buffer
	if err := Run([]string{"inspect", "--dir", dir, "--password-file", passFile, "--json", result.ID}, &openedOut); err != nil {
		t.Fatal(err)
	}
	var opened SetupSnapshotInspection
	if err := json.Unmarshal(openedOut.Bytes(), &opened); err != nil {
		t.Fatal(err)
	}
	if !opened.Opened || strings.Contains(openedOut.String(), secret) || strings.Contains(openedOut.String(), testSetupPassphrase) {
		t.Fatal("opened CLI inspection wrong")
	}
	var text bytes.Buffer
	if err := Run([]string{"list", "--dir", dir}, &text); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"inspect", "--dir", dir, result.ID}, &text); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Setup snapshots in") || !strings.Contains(text.String(), "Give --password-file") || strings.Contains(text.String(), secret) {
		t.Fatalf("bad CLI text: %s", text.String())
	}
}

func TestListSetupSnapshotsMissingDirectoryIsEmpty(t *testing.T) {
	list, err := ListSetupSnapshots(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(list.Snapshots) != 0 || len(list.Skipped) != 0 {
		t.Fatalf("missing directory not empty: %+v %v", list, err)
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ListSetupSnapshots(link); err == nil {
		t.Fatal("symlink directory accepted")
	}
}
