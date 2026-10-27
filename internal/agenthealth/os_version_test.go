package agenthealth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOSReleaseVersion(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "os-release")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for name, tc := range map[string]struct{ body, want string }{
		"pretty":             {"NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\nVERSION_ID=\"24.04\"\n", "Ubuntu 24.04 LTS"},
		"name+version":       {"NAME=Debian\nVERSION_ID='12'\n", "Debian 12"},
		"name only":          {"NAME=Arch\n", "Arch"},
		"control":            {"PRETTY_NAME=\"Evil\x1b[2J\"\n", ""},
		"oversized":          {"PRETTY_NAME=\"" + strings.Repeat("a", 200) + "\"\n", ""},
		"left-to-right mark": {"PRETTY_NAME=\"Ubuntu\u200e 24.04\"\n", ""},
		"line separator":     {"PRETTY_NAME=\"Ubuntu\u2028 24.04\"\n", ""},
		"paragraph sep":      {"PRETTY_NAME=\"Ubuntu\u2029 24.04\"\n", ""},
		"empty":              {"ID=x\n", ""},
	} {
		if got := osReleaseVersion(write(tc.body)); got != tc.want {
			t.Errorf("%s: got %q want %q", name, got, tc.want)
		}
	}
	if osReleaseVersion(filepath.Join(dir, "missing")) != "" {
		t.Fatal("missing file did not fall back")
	}
}

func TestReadOSVersionFallsBackToKernelRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	kernel := func() string { return "6.1.0" }
	if got := readOSVersionFrom(path, kernel); got != "6.1.0" {
		t.Fatalf("missing file: %q", got)
	}
	if err := os.WriteFile(path, []byte("PRETTY_NAME=\"Bad\u200e\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readOSVersionFrom(path, kernel); got != "6.1.0" {
		t.Fatalf("rejected os-release: %q", got)
	}
	if err := os.WriteFile(path, []byte("PRETTY_NAME=\"Ubuntu 24.04\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readOSVersionFrom(path, kernel); got != "Ubuntu 24.04" {
		t.Fatalf("good os-release: %q", got)
	}
}

func TestSanitiseOSVersionMatchesProtocolRule(t *testing.T) {
	for _, v := range []string{"a\u200eb", "a\u2028b", "a\u2029b", "a\x00b", "a\xffb"} {
		if sanitiseOSVersion(v) != "" {
			t.Errorf("accepted %q", v)
		}
	}
}
