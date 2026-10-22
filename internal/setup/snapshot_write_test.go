package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSetupPassphrase = "a setup passphrase long enough"

func writePassphrase(t *testing.T, dir, value string) string {
	t.Helper()
	path := filepath.Join(dir, "passphrase")
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtureSelection(t *testing.T) SnapshotSelection {
	t.Helper()
	e := fixture(t)
	const secret = "SYNTHETIC-WRITE-SECRET-DO-NOT-RETURN"
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{
  "model":"MODEL",
  "permissions":{"allow":["Read"]},
  "env":{"TOKEN":"`+secret+`"},
  "hooks":{"x":"`+secret+`"},
  "mcpServers":{"local":{"command":"server","args":["--x"],"env":{"TOKEN":"`+secret+`"},"headers":{"Authorization":"`+secret+`"}}}
}`)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func TestWriteSetupSnapshotEncryptsAndVerifies(t *testing.T) {
	selection := fixtureSelection(t)
	dir := filepath.Join(t.TempDir(), "snapshots")
	result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.ID, "setup-") || filepath.Ext(result.Path) != setupSnapshotExtension || result.Bytes == 0 ||
		result.Components < 1 || result.Omissions == 0 || result.TakenAt.IsZero() || result.SHA256 == "" ||
		result.Protection != "password" || result.Compression != "gzip" {
		t.Fatalf("incomplete result: %+v", result)
	}
	info, err := os.Stat(result.Path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot permissions: %+v %v", info, err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil || dirInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("snapshot directory permissions: %+v %v", dirInfo, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(result.Path) {
		t.Fatalf("unexpected snapshot directory contents: %+v %v", entries, err)
	}
	raw, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SYNTHETIC-WRITE-SECRET-DO-NOT-RETURN", "MODEL", "https://"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("plaintext %q found in sealed snapshot", secret)
		}
	}
	opened, err := OpenSetupSnapshot(raw, testSetupPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if len(opened.Sources) != len(selection.sources) {
		t.Fatalf("wrong source count: %d", len(opened.Sources))
	}
	var content map[string]any
	for _, source := range opened.Sources {
		if source.Component == "settings" {
			content = source.Content
		}
	}
	if content["model"] != "MODEL" {
		t.Fatalf("selected values missing after opening: %#v", content)
	}
	if _, exists := content["env"]; exists {
		t.Fatal("credential-bearing field entered the snapshot")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "SYNTHETIC-WRITE-SECRET-DO-NOT-RETURN") || strings.Contains(string(encoded), testSetupPassphrase) {
		t.Fatal("write result exposed a value")
	}
	if _, err := OpenSetupSnapshot(raw, "the wrong passphrase"); err == nil {
		t.Fatal("a wrong passphrase opened the snapshot")
	}
	tampered := append([]byte(nil), raw...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := OpenSetupSnapshot(tampered, testSetupPassphrase); err == nil {
		t.Fatal("a tampered snapshot opened")
	}
}

func TestWriteSetupSnapshotCLI(t *testing.T) {
	home := isolatedCLI(t)
	const secret = "SYNTHETIC-WRITE-CLI-SECRET"
	writeConfig(t, filepath.Join(home, ".claude.json"), `{"model":"`+secret+`"}`)
	dir := t.TempDir()
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var out bytes.Buffer
	if err := Run([]string{"backup", "--password-file", passFile, "--dir", dir, "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var result SetupWriteResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Path == "" || result.Bytes == 0 || strings.Contains(out.String(), secret) || strings.Contains(out.String(), testSetupPassphrase) {
		t.Fatalf("bad CLI result: %+v", result)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvSetupDir, filepath.Join(t.TempDir(), "from-env"))
	var text bytes.Buffer
	if err := Run([]string{"backup", "--password-file", passFile}, &text); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Took setup snapshot") || !strings.Contains(text.String(), "passphrase-encrypted") || strings.Contains(text.String(), secret) {
		t.Fatalf("bad text output: %s", text.String())
	}
}

func TestWriteSetupSnapshotRefusesUnsafeInputs(t *testing.T) {
	selection := fixtureSelection(t)
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "snapshots")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, tc := range []struct {
		name string
		opts SetupWriteOptions
	}{
		{"short passphrase", SetupWriteOptions{Dir: t.TempDir(), Password: "short"}},
		{"symlink directory", SetupWriteOptions{Dir: link, Password: testSetupPassphrase}},
		{"file as directory", SetupWriteOptions{Dir: filepath.Join(t.TempDir(), "file"), Password: testSetupPassphrase}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "file as directory" {
				if err := os.WriteFile(tc.opts.Dir, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := WriteSetupSnapshot(selection, tc.opts); err == nil {
				t.Fatal("unsafe destination accepted")
			}
		})
	}
	var out bytes.Buffer
	if err := Run([]string{"backup", "--password-file", filepath.Join(t.TempDir(), "missing")}, &out); err == nil {
		t.Fatal("missing passphrase file accepted")
	}
}

func TestVerifySetupSnapshotRejectsContentMismatch(t *testing.T) {
	selection := fixtureSelection(t)
	snapshot := BuildSetupSnapshot(selection)
	if err := verifySetupSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Sources[0].Content = map[string]any{"model": "changed"}
	if err := verifySetupSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("content mismatch accepted: %v", err)
	}
	snapshot.SchemaVersion = 99
	if err := verifySetupSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("unknown schema accepted: %v", err)
	}
}
