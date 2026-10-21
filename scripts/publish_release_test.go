package scripts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishRelease(t *testing.T) {
	script, err := filepath.Abs("publish-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, tag, existing, problem string
		missing                      bool
	}{
		{name: "new stable", tag: "v1.2.3"},
		{name: "new prerelease", tag: "v1.2.3-rc.1"},
		{name: "build metadata is not prerelease", tag: "v1.2.3+build-with-hyphens"},
		{name: "existing empty release", tag: "v1.0.0", existing: "empty"},
		{name: "partial upload", tag: "v1.0.0", existing: "partial"},
		{name: "already uploaded", tag: "v1.0.0", existing: "complete"},
		{name: "published archive differs", tag: "v1.0.0", existing: "conflict", problem: "Published asset differs"},
		{name: "published checksum differs", tag: "v1.0.0", existing: "checksum-conflict", problem: "Published asset differs"},
		{name: "missing matrix artifact", tag: "v1.2.3", missing: true, problem: "Missing release archive"},
		{name: "invalid tag", tag: "../../bad", problem: "Expected a version tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			artifacts := filepath.Join(root, "artifacts")
			remote := filepath.Join(root, "remote")
			mock := filepath.Join(root, "bin")
			log := filepath.Join(root, "calls")
			for _, dir := range []string{artifacts, remote, mock} {
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path string, b []byte) {
				t.Helper()
				if err := os.WriteFile(path, b, 0644); err != nil {
					t.Fatal(err)
				}
			}
			var names []string
			var manifest strings.Builder
			if tc.problem != "Expected a version tag" {
				for _, system := range []string{"darwin", "linux"} {
					for _, arch := range []string{"amd64", "arm64"} {
						name := fmt.Sprintf("reet_%s_%s_%s.tar.gz", tc.tag, system, arch)
						data := []byte(system + "/" + arch)
						names = append(names, name)
						fmt.Fprintf(&manifest, "%x  %s\n", sha256.Sum256(data), name)
						if !tc.missing || len(names) != 4 {
							write(filepath.Join(artifacts, name), data)
						}
					}
				}
			}
			if tc.existing != "" {
				write(filepath.Join(remote, ".exists"), nil)
				write(filepath.Join(remote, "notes.txt"), []byte("keep release notes"))
			}
			switch tc.existing {
			case "partial":
				write(filepath.Join(remote, names[0]), []byte("darwin/amd64"))
			case "complete":
				for _, name := range names {
					data, err := os.ReadFile(filepath.Join(artifacts, name))
					if err != nil {
						t.Fatal(err)
					}
					write(filepath.Join(remote, name), data)
				}
				write(filepath.Join(remote, "checksums.txt"), []byte(manifest.String()))
			case "conflict":
				write(filepath.Join(remote, names[3]), []byte("different published bytes"))
			case "checksum-conflict":
				write(filepath.Join(remote, "checksums.txt"), []byte("old checksum manifest"))
			}
			gh := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$MOCK_LOG"
case "$1 $2" in
 'release view')
  test -f "$MOCK_REMOTE/.exists" || exit 1
  for file in "$MOCK_REMOTE"/*; do test -f "$file" && basename "$file"; done ;;
 'release download') cp "$MOCK_REMOTE/$5" "$7/$5" ;;
 'release create'|'release upload')
  if [ "$2" = create ]; then test ! -f "$MOCK_REMOTE/.exists"; touch "$MOCK_REMOTE/.exists"; fi
  shift 3
  for file in "$@"; do
   if test -f "$file"; then
    test ! -f "$MOCK_REMOTE/$file" || { echo 'would overwrite published asset' >&2; exit 1; }
    cp "$file" "$MOCK_REMOTE/$file"
   fi
  done ;;
 *) echo 'unexpected gh call' >&2; exit 1 ;;
esac
`
			if err := os.WriteFile(filepath.Join(mock, "gh"), []byte(gh), 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", script, artifacts)
			cmd.Env = append(os.Environ(), "PATH="+mock+string(os.PathListSeparator)+os.Getenv("PATH"), "REET_TAG="+tc.tag, "MOCK_REMOTE="+remote, "MOCK_LOG="+log)
			out, err := cmd.CombinedOutput()
			calls, _ := os.ReadFile(log)
			if tc.problem != "" {
				if err == nil || !strings.Contains(string(out), tc.problem) {
					t.Fatalf("expected %q, got %v: %s", tc.problem, err, out)
				}
				if strings.Contains(string(calls), "release upload") || strings.Contains(string(calls), "release create") {
					t.Fatalf("must not publish on validation failure: %s", calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("publish: %v: %s", err, out)
			}
			for _, name := range names {
				a, err := os.ReadFile(filepath.Join(artifacts, name))
				if err != nil {
					t.Fatal(err)
				}
				b, err := os.ReadFile(filepath.Join(remote, name))
				if err != nil || string(a) != string(b) {
					t.Fatalf("asset %s differs: %v", name, err)
				}
			}
			sum, err := os.ReadFile(filepath.Join(remote, "checksums.txt"))
			if err != nil || string(sum) != manifest.String() {
				t.Fatalf("checksums differ: %q, %v", sum, err)
			}
			if strings.Contains(string(calls), "--clobber") {
				t.Fatal("must never overwrite published files")
			}
			if tc.existing == "complete" && strings.Contains(string(calls), "release upload") {
				t.Fatal("complete releases must be a no-op")
			}
			if tc.existing == "" {
				prerelease := strings.Contains(strings.SplitN(tc.tag, "+", 2)[0], "-")
				if strings.Contains(string(calls), "--prerelease") != prerelease {
					t.Fatalf("incorrect prerelease handling: %s", calls)
				}
				if !strings.Contains(string(calls), "--verify-tag") {
					t.Fatal("tag must be verified")
				}
			} else if strings.Contains(string(calls), "release create") {
				t.Fatal("must reuse the existing release")
			}
		})
	}
}
