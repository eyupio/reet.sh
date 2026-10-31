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

	rolled, err := RollbackManaged(home, "opencode")
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
	if _, err := RollbackManaged(home, "opencode"); !errors.Is(err, ErrNoRollback) {
		t.Fatalf("rollback error = %v", err)
	}
	if _, err := RollbackManaged(t.TempDir(), "opencode"); !errors.Is(err, ErrNotManaged) {
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

func TestRecoverManagedCleansLeftoversAndKeepsCurrentAndPrevious(t *testing.T) {
	home := t.TempDir()
	if _, err := InstallStaged(home, "opencode", "1.18.0", stagedFixture(t, "older")); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallStaged(home, "opencode", "1.19.5", stagedFixture(t, "newer")); err != nil {
		t.Fatal(err)
	}
	root, _ := Root(home, "opencode")
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tmp", "stale.tgz"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".receipt-stale"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(root, "versions", "1.19.9", "bin")
	if err := os.MkdirAll(partial, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partial, "opencode"), []byte("partial"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "versions", "1.19.5", ".opencode-stale"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RecoverManaged(home, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "versions", "1.19.9")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial version survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".receipt-stale")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("receipt temp file survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "versions", "1.19.5", ".opencode-stale")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary temp file survived: %v", err)
	}
	if _, receipt, found, err := Current(home, "opencode"); err != nil || !found || receipt.Version != "1.19.5" || receipt.Previous != "1.18.0" {
		t.Fatalf("current after cleanup: %+v %v %v", receipt, found, err)
	}
	if _, err := os.Stat(filepath.Join(root, "versions", "1.18.0", "opencode")); err != nil {
		t.Fatalf("previous version removed: %v", err)
	}
}

func TestRecoverManagedRestoresPreviousWhenCurrentIsBroken(t *testing.T) {
	home := t.TempDir()
	if _, err := InstallStaged(home, "opencode", "1.18.0", stagedFixture(t, "older")); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallStaged(home, "opencode", "1.19.5", stagedFixture(t, "newer")); err != nil {
		t.Fatal(err)
	}
	path, _, _, err := Current(home, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	if err := RecoverManaged(home, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, receipt, found, err := Current(home, "opencode"); err != nil || !found || receipt.Version != "1.18.0" || receipt.Previous != "" {
		t.Fatalf("restored current: %+v %v %v", receipt, found, err)
	}
}

func TestRecoverManagedReportsABrokenCurrentWithoutAPrevious(t *testing.T) {
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
	if err := RecoverManaged(home, "opencode"); !errors.Is(err, ErrManagedBroken) {
		t.Fatalf("recovery error = %v", err)
	}
	if _, _, found, err := Current(home, "opencode"); !errors.Is(err, ErrManagedBroken) || found {
		t.Fatalf("broken current was hidden: found=%v err=%v", found, err)
	}
}

func TestRecoverManagedClearsAnInvalidPrevious(t *testing.T) {
	home := t.TempDir()
	if _, err := InstallStaged(home, "opencode", "1.18.0", stagedFixture(t, "older")); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallStaged(home, "opencode", "1.19.5", stagedFixture(t, "newer")); err != nil {
		t.Fatal(err)
	}
	root, _ := Root(home, "opencode")
	previous := filepath.Join(root, "versions", "1.18.0", "opencode")
	if err := os.Truncate(previous, 0); err != nil {
		t.Fatal(err)
	}
	if err := RecoverManaged(home, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, receipt, _, err := Current(home, "opencode"); err != nil || receipt.Previous != "" {
		t.Fatalf("invalid previous not cleared: %+v %v", receipt, err)
	}
	if _, err := os.Stat(filepath.Join(root, "versions", "1.18.0")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid previous survived: %v", err)
	}
}

func TestRecoverManagedRemovesVersionsWithoutACurrentReceipt(t *testing.T) {
	home := t.TempDir()
	if _, err := InstallStaged(home, "opencode", "1.19.5", stagedFixture(t, "binary")); err != nil {
		t.Fatal(err)
	}
	root, _ := Root(home, "opencode")
	if err := os.Remove(filepath.Join(root, "receipt.json")); err != nil {
		t.Fatal(err)
	}
	if err := RecoverManaged(home, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := Current(home, "opencode"); err != nil || found {
		t.Fatalf("orphan version became current: found=%v err=%v", found, err)
	}
	if _, err := os.Stat(filepath.Join(root, "versions")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("versions survived without a current receipt: %v", err)
	}
}
