package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func packageFixture(t *testing.T, agent, command, bin string) (Agent, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.FromSlash(bin))
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte("not executable code"), 0o700); err != nil {
		t.Fatal(err)
	}
	writePackage(t, dir, map[string]any{"name": nodePackageNames(agent)[0], "version": "1.2.3", "bin": map[string]any{command: bin}, "private": "PRIVATE_SENTINEL"})
	return Agent{ID: agent, Executable: Executable{Command: command, State: "present", Path: path}}, dir
}

func writePackage(t *testing.T, dir string, object map[string]any) {
	t.Helper()
	data, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInstallationMatchesDeclaredBinAcrossKnownPackages(t *testing.T) {
	for _, tc := range []struct{ agent, command, bin string }{
		{"claude", "claude", "cli.js"}, {"claude", "claude", "bin/claude.exe"},
		{"codex", "codex", "bin/codex.js"}, {"opencode", "opencode", "bin/opencode"},
		{"pi", "pi", "dist/cli.js"}, {"gemini", "gemini", "bundle/gemini.js"},
	} {
		t.Run(tc.agent+tc.bin, func(t *testing.T) {
			a, dir := packageFixture(t, tc.agent, tc.command, tc.bin)
			v := inspectInstallation(a)
			if v.State != "matched" || v.Package != nodePackageNames(a.ID)[0] || v.Version != "1.2.3" || v.Kind != "node_package" || v.Manager != "unknown" || v.MetadataPath != filepath.Join(dir, "package.json") {
				t.Fatalf("incorrect association: %+v", v)
			}
		})
	}
}

func TestInstallationRejectsUnrelatedOrUnsafeMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, issue string
		edit        func(map[string]any)
	}{
		{"wrong package", "unrecognised_package", func(p map[string]any) { p["name"] = "PRIVATE_SENTINEL" }},
		{"missing version", "invalid_package_version", func(p map[string]any) { delete(p, "version") }},
		{"invalid version", "invalid_package_version", func(p map[string]any) { p["version"] = "PRIVATE_SENTINEL" }},
		{"missing bin", "unsupported_package_bin", func(p map[string]any) { delete(p, "bin") }},
		{"string bin", "unsupported_package_bin", func(p map[string]any) { p["bin"] = "cli.js" }},
		{"different command", "unsupported_package_bin", func(p map[string]any) { p["bin"] = map[string]any{"other": "cli.js"} }},
		{"different file", "package_bin_mismatch", func(p map[string]any) { p["bin"] = map[string]any{"codex": "other.js"} }},
		{"escape", "unsupported_package_bin", func(p map[string]any) { p["bin"] = map[string]any{"codex": "../cli.js"} }},
		{"absolute", "unsupported_package_bin", func(p map[string]any) { p["bin"] = map[string]any{"codex": "/cli.js"} }},
		{"backslash", "unsupported_package_bin", func(p map[string]any) { p["bin"] = map[string]any{"codex": `bin\cli.js`} }},
		{"drive", "unsupported_package_bin", func(p map[string]any) { p["bin"] = map[string]any{"codex": "C:cli.js"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, dir := packageFixture(t, "codex", "codex", "cli.js")
			p := map[string]any{"name": nodePackageNames("codex")[0], "version": "1.2.3", "bin": map[string]any{"codex": "cli.js"}}
			tc.edit(p)
			writePackage(t, dir, p)
			v := inspectInstallation(a)
			if v.State == "matched" || v.Issue != tc.issue || v.Package != "" || v.Version != "" {
				t.Fatalf("unsafe metadata accepted: %+v", v)
			}
		})
	}
}

func TestInstallationStopsAtNearestPackageBoundary(t *testing.T) {
	a, dir := packageFixture(t, "codex", "codex", "nested/bin/cli.js")
	writePackage(t, filepath.Join(dir, "nested"), map[string]any{"name": "unrelated"})
	if v := inspectInstallation(a); v.Issue != "unrecognised_package" {
		t.Fatalf("used outer package: %+v", v)
	}
}

func TestInstallationBoundsAncestorSearchAndMetadata(t *testing.T) {
	a, _ := packageFixture(t, "codex", "codex", strings.Repeat("nested/", maxPackageAncestors)+"cli.js")
	if v := inspectInstallation(a); v.Issue != "package_metadata_not_found_within_limit" {
		t.Fatalf("unbounded search: %+v", v)
	}
	for _, tc := range []struct{ data, issue string }{
		{`{"name":"PRIVATE_SENTINEL",`, "invalid_package_metadata"},
		{`{"name":"@openai/codex","name":"@openai/codex"}`, "invalid_package_metadata"},
		{strings.Repeat("x", maxConfigBytes+1), "package_file_too_large"},
	} {
		a, dir := packageFixture(t, "codex", "codex", "cli.js")
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tc.data), 0o600); err != nil {
			t.Fatal(err)
		}
		if v := inspectInstallation(a); v.State != "error" || v.Issue != tc.issue {
			t.Fatalf("invalid metadata accepted: %+v", v)
		}
	}
}

func TestInstallationExecutableAndMetadataSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires platform privileges")
	}
	a, dir := packageFixture(t, "codex", "codex", "bin/cli.js")
	link := filepath.Join(t.TempDir(), "codex")
	if err := os.Symlink(a.Executable.Path, link); err != nil {
		t.Fatal(err)
	}
	a.Executable.Path = link
	if v := inspectInstallation(a); v.State != "matched" {
		t.Fatalf("executable symlink not resolved: %+v", v)
	}
	metadata := filepath.Join(dir, "package.json")
	if err := os.Rename(metadata, metadata+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(metadata+".real", metadata); err != nil {
		t.Fatal(err)
	}
	if v := inspectInstallation(a); v.Issue != "package_metadata_not_regular" {
		t.Fatalf("followed metadata symlink: %+v", v)
	}
}

func TestInstallationRejectsEscapingBinSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires platform privileges")
	}
	a, dir := packageFixture(t, "codex", "codex", "cli.js")
	outside := filepath.Join(t.TempDir(), "other.js")
	if err := os.Link(a.Executable.Path, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escaped.js")); err != nil {
		t.Fatal(err)
	}
	writePackage(t, dir, map[string]any{"name": nodePackageNames(a.ID)[0], "version": "1.2.3", "bin": map[string]any{"codex": "escaped.js"}})
	if v := inspectInstallation(a); v.Issue != "package_bin_mismatch" {
		t.Fatalf("accepted escaping symlink to same inode: %+v", v)
	}
}

func TestInstallationUnavailableAndUnsupportedRemainExplicit(t *testing.T) {
	for _, tc := range []struct{ agent, state, path, want, issue string }{
		{"codex", "missing", "", "not_detected", ""},
		{"codex", "error", "", "unknown", "executable_unavailable"},
		{"codex", "present", "relative", "unknown", "executable_unavailable"},
		{"cursor", "present", filepath.Join(t.TempDir(), "cursor"), "unknown", "installation_adapter_not_supported"},
		{"codex", "present", filepath.Join(t.TempDir(), "missing"), "error", "executable_unavailable"},
	} {
		v := inspectInstallation(Agent{ID: tc.agent, Executable: Executable{State: tc.state, Path: tc.path}})
		if v.State != tc.want || v.Issue != tc.issue {
			t.Fatalf("wrong observation: %+v", v)
		}
	}
}

func TestInstallationCLIOptInDoesNotExecuteOrExposeMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	isolatedCLI(t)
	dir := os.Getenv("PATH")
	marker := filepath.Join(dir, "executed")
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\nprintf yes > '"+marker+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	writePackage(t, dir, map[string]any{"name": nodePackageNames("codex")[0], "version": "1.2.3", "bin": map[string]any{"codex": "codex"}, "private": "PRIVATE_SENTINEL"})
	for _, flags := range [][]string{{}, {"--inspect-installation"}, {"--components", "--inspect-installation"}} {
		var out bytes.Buffer
		if err := Run(append([]string{"scan", "--json"}, flags...), &out); err != nil {
			t.Fatal(err)
		}
		var r Report
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		a := findAgent(t, r, "codex")
		if (a.Installation != nil) != (len(flags) > 0) || r.InstallationInspection != (len(flags) > 0) || a.ApplicationVersion != nil {
			t.Fatal("incorrect opt-in semantics")
		}
		if len(flags) > 0 && (a.Installation.State != "matched" || r.SchemaVersion != 2) {
			t.Fatal("missing installation evidence")
		}
		var rendered bytes.Buffer
		if err := render(&rendered, r); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String()+rendered.String(), "PRIVATE_SENTINEL") {
			t.Fatal("raw metadata leaked")
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("inspection executed a command")
		}
	}
}

func TestInstallationRecognisesCurrentAndLegacyPackageNames(t *testing.T) {
	for _, tc := range []struct{ agent, name, command, bin string }{
		{"opencode", "@opencode/cli", "opencode", "bin/opencode.exe"},
		{"opencode", "opencode-ai", "opencode", "bin/opencode"},
		{"pi", "@earendil-works/pi-coding-agent", "pi", "dist/bundle/cli.js"},
		{"pi", "@mariozechner/pi-coding-agent", "pi", "dist/cli.js"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, dir := packageFixture(t, tc.agent, tc.command, tc.bin)
			writePackage(t, dir, map[string]any{"name": tc.name, "version": "1.2.3", "bin": map[string]any{tc.command: tc.bin}})
			v := inspectInstallation(a)
			if v.State != "matched" || v.Package != tc.name {
				t.Fatalf("incorrect package identity: %+v", v)
			}
		})
	}
}
