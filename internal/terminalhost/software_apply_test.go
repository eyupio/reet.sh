//go:build linux

package terminalhost

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func fakeArtifact(t *testing.T, binary []byte) (*httptest.Server, agentsoftware.Artifact) {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gzipWriter)
	if err := writer.WriteHeader(&tar.Header{Name: "package/bin/opencode", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(buffer.Bytes())
	binarySum := sha256.Sum256(binary)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(buffer.Bytes()) }))
	t.Cleanup(server.Close)
	return server, agentsoftware.Artifact{
		URL: server.URL + "/opencode.tgz", SHA512: base64.StdEncoding.EncodeToString(sum[:]),
		BinarySHA256: hex.EncodeToString(binarySum[:]), BinarySize: int64(len(binary)), Member: "package/bin/opencode",
	}
}

func installManagedVersion(t *testing.T, home, version, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "staged")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	staged := agentsoftware.Staged{
		Path: path, SHA512: "archive-sha", BinarySHA256: hex.EncodeToString(sum[:]),
		Size: int64(len(content)), Artifact: agentsoftware.Artifact{URL: "https://registry.example/opencode.tgz"},
	}
	if _, err := agentsoftware.InstallStaged(home, "opencode", version, staged); err != nil {
		t.Fatal(err)
	}
}

func missingLookPath(string) (string, error) { return "", exec.ErrNotFound }

func TestApplySoftwareInstallsAndUpdates(t *testing.T) {
	_, artifact := fakeArtifact(t, []byte("reviewed binary"))
	original := softwareArtifact
	softwareArtifact = artifact
	t.Cleanup(func() { softwareArtifact = original })

	home := t.TempDir()
	result := ApplySoftware(context.Background(), home, terminalproto.SoftwareApply{
		Tool: "opencode", Action: terminalproto.SoftwareActionInstall, ExpectedState: terminalproto.SoftwareMissing,
	}, missingLookPath)
	if result.State != terminalproto.SoftwareApplyApplied || result.Version != agentsoftware.OpenCodeReviewedVersion || result.Entry == nil || !result.Entry.Managed {
		t.Fatalf("install result: %+v", result)
	}
	if _, _, found, err := agentsoftware.Current(home, "opencode"); err != nil || !found {
		t.Fatalf("managed install missing: found=%v err=%v", found, err)
	}

	updateHome := t.TempDir()
	installManagedVersion(t, updateHome, "1.18.0", "older managed binary")
	result = ApplySoftware(context.Background(), updateHome, terminalproto.SoftwareApply{
		Tool: "opencode", Action: terminalproto.SoftwareActionUpdate, ExpectedState: terminalproto.SoftwareInstalled,
		ExpectedVersion: "1.18.0", ExpectedManaged: true,
	}, missingLookPath)
	if result.State != terminalproto.SoftwareApplyApplied || result.Version != agentsoftware.OpenCodeReviewedVersion || result.Previous != "1.18.0" {
		t.Fatalf("update result: %+v", result)
	}
	if _, receipt, found, err := agentsoftware.Current(updateHome, "opencode"); err != nil || !found || receipt.Version != agentsoftware.OpenCodeReviewedVersion {
		t.Fatalf("current after update: %+v %v %v", receipt, found, err)
	}
	if _, err := agentsoftware.RollbackManaged(updateHome, "opencode"); err != nil {
		t.Fatalf("rollback after update: %v", err)
	}
}

func TestApplySoftwareRefusesAChangedInstallation(t *testing.T) {
	home := t.TempDir()
	result := ApplySoftware(context.Background(), home, terminalproto.SoftwareApply{
		Tool: "opencode", Action: terminalproto.SoftwareActionUpdate, ExpectedState: terminalproto.SoftwareInstalled,
		ExpectedVersion: "1.18.0", ExpectedManaged: true,
	}, missingLookPath)
	if result.State != terminalproto.SoftwareApplyRefused || result.Reason != terminalproto.SoftwareApplyChanged {
		t.Fatalf("changed refusal: %+v", result)
	}
}

func TestApplySoftwareRejectsACorruptArtifact(t *testing.T) {
	_, artifact := fakeArtifact(t, []byte("reviewed binary"))
	artifact.SHA512 = "wrong"
	original := softwareArtifact
	softwareArtifact = artifact
	t.Cleanup(func() { softwareArtifact = original })
	home := t.TempDir()
	result := ApplySoftware(context.Background(), home, terminalproto.SoftwareApply{
		Tool: "opencode", Action: terminalproto.SoftwareActionInstall, ExpectedState: terminalproto.SoftwareMissing,
	}, missingLookPath)
	if result.State != terminalproto.SoftwareApplyFailed || result.Reason != terminalproto.SoftwareApplyChecksum {
		t.Fatalf("corrupt result: %+v", result)
	}
	if _, _, found, _ := agentsoftware.Current(home, "opencode"); found {
		t.Fatal("a corrupt artifact left a managed install")
	}
}

func TestApplySoftwareInstallsAlongsideASystemCopy(t *testing.T) {
	_, artifact := fakeArtifact(t, []byte("reviewed binary"))
	original := softwareArtifact
	softwareArtifact = artifact
	t.Cleanup(func() { softwareArtifact = original })

	home := t.TempDir()
	systemBinary := writeNodePackage(t, t.TempDir(), "opencode-ai", "1.18.0")
	result := ApplySoftware(context.Background(), home, terminalproto.SoftwareApply{
		Tool: "opencode", Action: terminalproto.SoftwareActionInstall, ExpectedState: terminalproto.SoftwareInstalled,
	}, func(string) (string, error) { return systemBinary, nil })
	if result.State != terminalproto.SoftwareApplyApplied || result.Entry == nil || !result.Entry.Managed {
		t.Fatalf("alongside result: %+v", result)
	}
	if _, err := os.Stat(systemBinary); err != nil {
		t.Fatalf("the system copy was changed: %v", err)
	}
	if _, receipt, found, err := agentsoftware.Current(home, "opencode"); err != nil || !found || receipt.Version != agentsoftware.OpenCodeReviewedVersion {
		t.Fatalf("managed copy after alongside install: %+v %v %v", receipt, found, err)
	}
}

func TestApplySoftwareRollsBackAndRemovesAReetManagedCopy(t *testing.T) {
	home := t.TempDir()
	installManagedVersion(t, home, "1.18.0", "older managed binary")
	installManagedVersion(t, home, "1.19.5", "newer managed binary")

	result := ApplySoftware(context.Background(), home, terminalproto.SoftwareApply{
		Tool: "opencode", Action: terminalproto.SoftwareActionRollback, ExpectedState: terminalproto.SoftwareInstalled,
		ExpectedVersion: "1.19.5", ExpectedPrevious: "1.18.0", ExpectedManaged: true,
	}, missingLookPath)
	if result.State != terminalproto.SoftwareApplyApplied || result.Version != "1.18.0" || result.Previous != "1.19.5" {
		t.Fatalf("rollback result: %+v", result)
	}
	if _, receipt, found, err := agentsoftware.Current(home, "opencode"); err != nil || !found || receipt.Version != "1.18.0" {
		t.Fatalf("current after rollback: %+v %v %v", receipt, found, err)
	}

	result = ApplySoftware(context.Background(), home, terminalproto.SoftwareApply{
		Tool: "opencode", Action: terminalproto.SoftwareActionRemove, ExpectedState: terminalproto.SoftwareInstalled,
		ExpectedVersion: "1.18.0", ExpectedManaged: true,
	}, missingLookPath)
	if result.State != terminalproto.SoftwareApplyApplied || result.Version != "" || result.Entry == nil || result.Entry.State != terminalproto.SoftwareMissing {
		t.Fatalf("remove result: %+v", result)
	}
	if _, _, found, _ := agentsoftware.Current(home, "opencode"); found {
		t.Fatal("managed copy stayed after remove")
	}
}

func TestApplySoftwareRemoveLeavesASystemCopy(t *testing.T) {
	home := t.TempDir()
	systemBinary := writeNodePackage(t, t.TempDir(), "opencode-ai", "1.18.0")
	installManagedVersion(t, home, "1.19.5", "managed binary")
	lookPath := func(string) (string, error) { return systemBinary, nil }

	result := ApplySoftware(context.Background(), home, terminalproto.SoftwareApply{
		Tool: "opencode", Action: terminalproto.SoftwareActionRemove, ExpectedState: terminalproto.SoftwareInstalled,
		ExpectedVersion: "1.19.5", ExpectedManaged: true,
	}, lookPath)
	if result.State != terminalproto.SoftwareApplyApplied || result.Entry == nil || result.Entry.Managed || result.Entry.State != terminalproto.SoftwareInstalled {
		t.Fatalf("remove beside a system copy: %+v", result)
	}
	if _, err := os.Stat(systemBinary); err != nil {
		t.Fatalf("the system copy was changed: %v", err)
	}
}

func TestApplySoftwareRefusesAChangedRollback(t *testing.T) {
	home := t.TempDir()
	installManagedVersion(t, home, "1.19.5", "managed binary")
	result := ApplySoftware(context.Background(), home, terminalproto.SoftwareApply{
		Tool: "opencode", Action: terminalproto.SoftwareActionRollback, ExpectedState: terminalproto.SoftwareInstalled,
		ExpectedVersion: "1.19.5", ExpectedPrevious: "1.18.0", ExpectedManaged: true,
	}, missingLookPath)
	if result.State != terminalproto.SoftwareApplyRefused || result.Reason != terminalproto.SoftwareApplyChanged {
		t.Fatalf("changed rollback: %+v", result)
	}
}
