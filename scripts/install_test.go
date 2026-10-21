package scripts

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the installer against local release fixtures. curl and uname are
// replaced so tests cannot download a release or touch the real installation.
func TestInstallRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell installer targets Linux and macOS")
	}
	for _, dependency := range []string{"sh", "tar", "install"} {
		if _, err := exec.LookPath(dependency); err != nil {
			t.Skipf("%s unavailable", dependency)
		}
	}
	script, err := filepath.Abs("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, system, arch, version, problem string
		tamper, duplicate, omitBinary        bool
	}{
		{name: "linux latest", system: "Linux", arch: "x86_64", version: "latest"},
		{name: "linux arm64", system: "Linux", arch: "aarch64", version: "9.8.7"},
		{name: "macOS arm64", system: "Darwin", arch: "arm64", version: "v9.8.7"},
		{name: "macOS amd64", system: "Darwin", arch: "x86_64", version: "v9.8.7"},
		{name: "checksum failure preserves old binary", system: "Linux", arch: "x86_64", version: "v9.8.7", tamper: true, problem: "checksum mismatch"},
		{name: "ambiguous checksum preserves old binary", system: "Linux", arch: "x86_64", version: "v9.8.7", duplicate: true, problem: "exactly one checksum"},
		{name: "missing binary preserves old binary", system: "Linux", arch: "x86_64", version: "v9.8.7", omitBinary: true, problem: "could not extract"},
		{name: "unsupported OS", system: "FreeBSD", arch: "x86_64", version: "v9.8.7", problem: "supported systems"},
		{name: "unsupported CPU", system: "Linux", arch: "riscv64", version: "v9.8.7", problem: "supported architectures"},
		{name: "reject version path traversal", system: "Linux", arch: "x86_64", version: "../../other", problem: "version must be a release tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			mockBin := filepath.Join(root, "mock-bin")
			installDir := filepath.Join(root, "bin with spaces")
			for _, directory := range []string{mockBin, installDir} {
				if err := os.Mkdir(directory, 0755); err != nil {
					t.Fatal(err)
				}
			}
			write := func(name, content string, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(name, []byte(content), mode); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(installDir, "reet"), "old installation", 0755)
			write(filepath.Join(mockBin, "uname"), fmt.Sprintf("#!/bin/sh\ncase \"$1\" in -s) echo '%s';; -m) echo '%s';; esac\n", tc.system, tc.arch), 0755)
			write(filepath.Join(mockBin, "curl"), `#!/bin/sh
set -eu
out=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) out=$2; shift 2 ;;
    --proto|--proto-redir|--retry|--connect-timeout|--max-time|--write-out) shift 2 ;;
    --fail|--silent|--show-error|--location|--head) shift ;;
    https://*) url=$1; shift ;;
    *) exit 90 ;;
  esac
done
printf '%s\n' "$url" >> "$REET_FIXTURE/requests"
case "$url" in
  */releases/latest) printf 'https://github.com/eyupio/reet.sh/releases/tag/v9.8.7' ;;
  */checksums.txt) cp "$REET_FIXTURE/checksums.txt" "$out" ;;
  */*.tar.gz) cp "$REET_FIXTURE/archive.tar.gz" "$out" ;;
  *) exit 91 ;;
esac
`, 0755)
			binary := []byte("#!/bin/sh\nprintf 'reet v9.8.7 (fixture)\\n'\n")
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			name := "reet"
			if tc.omitBinary {
				name = "other-file"
			}
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(binary))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(binary); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			osName, arch := "linux", "amd64"
			if tc.system == "Darwin" {
				osName = "darwin"
			}
			if tc.arch == "arm64" || tc.arch == "aarch64" {
				arch = "arm64"
			}
			asset := fmt.Sprintf("reet_v9.8.7_%s_%s.tar.gz", osName, arch)
			digest := fmt.Sprintf("%x", sha256.Sum256(archive.Bytes()))
			manifest := digest + "  " + asset + "\n"
			if tc.duplicate {
				manifest += manifest
			}
			if tc.tamper {
				archive.WriteString("corrupted download")
			}
			write(filepath.Join(root, "archive.tar.gz"), archive.String(), 0600)
			write(filepath.Join(root, "checksums.txt"), manifest, 0600)
			cmd := exec.Command("sh", script, "--version", tc.version, "--dir", installDir)
			cmd.Env = append(os.Environ(), "PATH="+mockBin+string(os.PathListSeparator)+os.Getenv("PATH"), "REET_FIXTURE="+root)
			output, installErr := cmd.CombinedOutput()
			installed, err := os.ReadFile(filepath.Join(installDir, "reet"))
			if err != nil {
				t.Fatal(err)
			}
			if tc.problem != "" {
				if installErr == nil || !strings.Contains(string(output), tc.problem) {
					t.Fatalf("expected %q, got %v: %s", tc.problem, installErr, output)
				}
				if string(installed) != "old installation" {
					t.Fatal("failed install changed the existing binary")
				}
				return
			}
			if installErr != nil {
				t.Fatalf("install: %v: %s", installErr, output)
			}
			if !bytes.Equal(installed, binary) {
				t.Fatal("installed file differs from verified release")
			}
			info, err := os.Stat(filepath.Join(installDir, "reet"))
			if err != nil || info.Mode()&0111 == 0 {
				t.Fatal("installed binary is not executable")
			}
			requests, _ := os.ReadFile(filepath.Join(root, "requests"))
			if tc.version != "latest" && strings.Contains(string(requests), "/releases/latest") {
				t.Fatal("a pinned version requested the latest release")
			}
		})
	}
}
