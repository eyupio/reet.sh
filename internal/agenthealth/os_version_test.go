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
		"pretty":       {"NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\nVERSION_ID=\"24.04\"\n", "Ubuntu 24.04 LTS"},
		"name+version": {"NAME=Debian\nVERSION_ID='12'\n", "Debian 12"},
		"name only":    {"NAME=Arch\n", "Arch"},
		"control":      {"PRETTY_NAME=\"Evil\x1b[2J\"\n", ""},
		"oversized":    {"PRETTY_NAME=\"" + strings.Repeat("a", 200) + "\"\n", ""},
		"empty":        {"ID=x\n", ""},
	} {
		if got := osReleaseVersion(write(tc.body)); got != tc.want {
			t.Errorf("%s: got %q want %q", name, got, tc.want)
		}
	}
	if osReleaseVersion(filepath.Join(dir, "missing")) != "" {
		t.Fatal("missing file did not fall back")
	}
}
