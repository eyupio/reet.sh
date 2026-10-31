//go:build linux || darwin

package terminalhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func writeNodePackage(t *testing.T, root, name, version string) string {
	t.Helper()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(binDir, "opencode")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]any{"name": name, "version": version, "bin": map[string]string{"opencode": "bin/opencode"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), metadata, 0o644); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestInspectSoftwareReportsOpenCodeEvidence(t *testing.T) {
	bin := writeNodePackage(t, t.TempDir(), "opencode-ai", "1.19.5")
	entry := InspectSoftware(t.TempDir(), "opencode", func(string) (string, error) { return bin, nil })
	if entry.State != terminalproto.SoftwareInstalled || entry.Version != "1.19.5" || entry.Package != "opencode-ai" {
		t.Fatalf("unexpected evidence: %+v", entry)
	}
	if entry.Manager != "node_package" || entry.Source != "package.json" || entry.Path != bin {
		t.Fatalf("manager, source or path wrong: %+v", entry)
	}
	if entry.Platform != runtime.GOOS || entry.Arch != runtime.GOARCH || entry.CheckedAt == 0 {
		t.Fatalf("platform evidence wrong: %+v", entry)
	}
	if entry.Managed {
		t.Fatal("a system installation was reported as reet-managed")
	}
	if len(entry.Operations) != 1 || entry.Operations[0] != terminalproto.SoftwarePlanInstall {
		t.Fatalf("system installation should only offer a managed copy: %+v", entry.Operations)
	}
	if !entry.Valid() {
		t.Fatalf("entry does not validate: %+v", entry)
	}
}

func TestInspectSoftwareMissingOffersOnlyInstall(t *testing.T) {
	entry := InspectSoftware(t.TempDir(), "opencode", func(string) (string, error) { return "", exec.ErrNotFound })
	if entry.State != terminalproto.SoftwareMissing || entry.Reason != terminalproto.SoftwareNotInstalled {
		t.Fatalf("unexpected state: %+v", entry)
	}
	if entry.Package != "opencode-ai" || entry.Source != "npm" {
		t.Fatalf("missing install source wrong: %+v", entry)
	}
	if len(entry.Operations) != 1 || entry.Operations[0] != terminalproto.SoftwarePlanInstall {
		t.Fatalf("missing operations wrong: %+v", entry.Operations)
	}
	if !entry.Valid() {
		t.Fatalf("entry does not validate: %+v", entry)
	}
}

func TestInspectSoftwareUnsupportedToolReadsNothing(t *testing.T) {
	called := false
	entry := InspectSoftware(t.TempDir(), "claude", func(string) (string, error) {
		called = true
		return "", errors.New("must not run")
	})
	if called {
		t.Fatal("inspected an unsupported tool")
	}
	if entry.State != terminalproto.SoftwareUnsupported || entry.Reason != terminalproto.SoftwareUnsupportedTool {
		t.Fatalf("unexpected state: %+v", entry)
	}
	if !entry.Valid() {
		t.Fatalf("entry does not validate: %+v", entry)
	}
}

func TestInspectSoftwareUnrecognisedPackageStaysUnknown(t *testing.T) {
	bin := writeNodePackage(t, t.TempDir(), "some-other-package", "9.9.9")
	entry := InspectSoftware(t.TempDir(), "opencode", func(string) (string, error) { return bin, nil })
	if entry.State != terminalproto.SoftwareUnknown || entry.Reason != terminalproto.SoftwarePackageUnknown {
		t.Fatalf("unexpected state: %+v", entry)
	}
	if len(entry.Operations) != 0 {
		t.Fatalf("unknown installation advertises operations: %+v", entry.Operations)
	}
	if !entry.Valid() {
		t.Fatalf("entry does not validate: %+v", entry)
	}
}

func TestInspectSoftwareUsesTheResolvedExecutable(t *testing.T) {
	first := writeNodePackage(t, t.TempDir(), "opencode-ai", "1.19.5")
	second := writeNodePackage(t, t.TempDir(), "opencode-ai", "1.20.0")
	entry := InspectSoftware(t.TempDir(), "opencode", func(string) (string, error) { return first, nil })
	if entry.Version != "1.19.5" || entry.Path != first {
		t.Fatalf("read another executable: %+v", entry)
	}
	if entry.Version == "1.20.0" || entry.Path == second {
		t.Fatal("read the shadowed executable")
	}
}

func installManagedCopy(t *testing.T, home, content string) string {
	t.Helper()
	stagedPath := filepath.Join(t.TempDir(), "staged")
	if err := os.WriteFile(stagedPath, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	staged := agentsoftware.Staged{
		Path: stagedPath, SHA512: "archive-sha", BinarySHA256: hex.EncodeToString(sum[:]),
		Size: int64(len(content)), Artifact: agentsoftware.Artifact{URL: "https://registry.example/opencode.tgz"},
	}
	if _, err := agentsoftware.InstallStaged(home, "opencode", "1.19.5", staged); err != nil {
		t.Fatal(err)
	}
	return stagedPath
}

func TestInspectSoftwarePrefersTheManagedCopy(t *testing.T) {
	home := t.TempDir()
	installManagedCopy(t, home, "managed binary")
	entry := InspectSoftware(home, "opencode", func(string) (string, error) { return "", exec.ErrNotFound })
	if entry.State != terminalproto.SoftwareInstalled || !entry.Managed || entry.Version != "1.19.5" {
		t.Fatalf("managed evidence wrong: %+v", entry)
	}
	if entry.Manager != "reet" || entry.Source != "verified_tarball" || entry.Package != "opencode-ai" {
		t.Fatalf("managed source wrong: %+v", entry)
	}
	if entry.Path != filepath.Join(home, ".reet", "software", "opencode", "versions", "1.19.5", "opencode") {
		t.Fatalf("managed path wrong: %q", entry.Path)
	}
	if len(entry.Operations) != 2 || entry.Operations[0] != terminalproto.SoftwarePlanUpdate || entry.Operations[1] != terminalproto.SoftwarePlanRemove {
		t.Fatalf("managed operations wrong: %+v", entry.Operations)
	}
	if !entry.Valid() {
		t.Fatalf("managed entry does not validate: %+v", entry)
	}
}

func TestInspectSoftwareReportsABrokenManagedCopy(t *testing.T) {
	home := t.TempDir()
	installManagedCopy(t, home, "managed binary")
	path := filepath.Join(home, ".reet", "software", "opencode", "versions", "1.19.5", "opencode")
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	entry := InspectSoftware(home, "opencode", func(string) (string, error) { return "", exec.ErrNotFound })
	if entry.State != terminalproto.SoftwareUnknown || entry.Reason != terminalproto.SoftwareManagedBroken || entry.Managed {
		t.Fatalf("broken managed evidence wrong: %+v", entry)
	}
	if !entry.Valid() {
		t.Fatalf("broken entry does not validate: %+v", entry)
	}
}
