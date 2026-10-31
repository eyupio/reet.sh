package agentsoftware

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func stagedFixture(t *testing.T, content string) Staged {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "binary-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	return Staged{
		Path: file.Name(), SHA512: "recorded-archive-sha512", BinarySHA256: hex.EncodeToString(sum[:]),
		Size: int64(len(content)), Artifact: Artifact{URL: "https://registry.example/opencode-linux-x64-1.19.5.tgz"},
	}
}

func TestInstallCurrentRollbackAndRemove(t *testing.T) {
	home := t.TempDir()
	first, err := InstallStaged(home, "opencode", "1.19.5", stagedFixture(t, "version one"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != "1.19.5" || first.Previous != "" || first.InstalledAt == 0 {
		t.Fatalf("unexpected first receipt: %+v", first)
	}
	path, receipt, found, err := Current(home, "opencode")
	if err != nil || !found || receipt.Version != "1.19.5" {
		t.Fatalf("current after first install: %q %+v %v %v", path, receipt, found, err)
	}
	if data, _ := os.ReadFile(path); string(data) != "version one" {
		t.Fatalf("binary content = %q", string(data))
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("binary mode: %v %v", info, err)
	}

	second, err := InstallStaged(home, "opencode", "1.20.0", stagedFixture(t, "version two"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Previous != "1.19.5" {
		t.Fatalf("second receipt did not keep the previous version: %+v", second)
	}
	if path, receipt, _, _ := Current(home, "opencode"); receipt.Version != "1.20.0" {
		t.Fatalf("current after second install: %q %+v", path, receipt)
	}

	rolled, err := Rollback(home, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Version != "1.19.5" || rolled.Previous != "1.20.0" {
		t.Fatalf("unexpected rollback receipt: %+v", rolled)
	}
	path, receipt, _, err = Current(home, "opencode")
	if err != nil || receipt.Version != "1.19.5" {
		t.Fatalf("current after rollback: %q %+v %v", path, receipt, err)
	}
	if data, _ := os.ReadFile(path); string(data) != "version one" {
		t.Fatalf("rolled back binary content = %q", string(data))
	}

	if err := RemoveManaged(home, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := Current(home, "opencode"); err != nil || found {
		t.Fatalf("current after remove: found=%v err=%v", found, err)
	}
	if err := RemoveManaged(home, "opencode"); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("second remove error = %v", err)
	}
}

func TestSystemInstallIsInspectOnly(t *testing.T) {
	home := t.TempDir()
	if _, _, found, err := Current(home, "opencode"); err != nil || found {
		t.Fatalf("system install treated as managed: found=%v err=%v", found, err)
	}
	if _, found, err := ManagedBinary(home, "opencode"); err != nil || found {
		t.Fatalf("managed binary for a system install: found=%v err=%v", found, err)
	}
	if err := RemoveManaged(home, "opencode"); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("remove of a system install error = %v", err)
	}
}

func TestBrokenManagedBinaryIsReported(t *testing.T) {
	home := t.TempDir()
	if _, err := InstallStaged(home, "opencode", "1.19.5", stagedFixture(t, "binary")); err != nil {
		t.Fatal(err)
	}
	path, _, _, err := Current(home, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := Current(home, "opencode"); !errors.Is(err, ErrManagedBroken) || found {
		t.Fatalf("broken current: found=%v err=%v", found, err)
	}
	if _, found, err := ManagedBinary(home, "opencode"); !errors.Is(err, ErrManagedBroken) || found {
		t.Fatalf("broken managed binary: found=%v err=%v", found, err)
	}
}

func TestRollbackWithoutPrevious(t *testing.T) {
	home := t.TempDir()
	if _, err := InstallStaged(home, "opencode", "1.19.5", stagedFixture(t, "binary")); err != nil {
		t.Fatal(err)
	}
	if _, err := Rollback(home, "opencode"); !errors.Is(err, ErrNoRollback) {
		t.Fatalf("rollback error = %v", err)
	}
	if _, err := Rollback(t.TempDir(), "opencode"); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("rollback without a managed install error = %v", err)
	}
}

func TestRemoveKeepsUserConfiguration(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallStaged(home, "opencode", "1.19.5", stagedFixture(t, "binary")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveManaged(home, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config); err != nil {
		t.Fatalf("configuration was removed with the binary: %v", err)
	}
}

func TestInstallRejectsUnsafeNames(t *testing.T) {
	home := t.TempDir()
	if _, err := InstallStaged(home, "opencode", "../escape", stagedFixture(t, "binary")); err == nil {
		t.Fatal("accepted an unsafe version")
	}
	if _, err := InstallStaged(home, "../../etc", "1.19.5", stagedFixture(t, "binary")); err == nil {
		t.Fatal("accepted an unsafe tool")
	}
	if _, err := Root("relative/home", "opencode"); err == nil {
		t.Fatal("accepted a relative home")
	}
}
