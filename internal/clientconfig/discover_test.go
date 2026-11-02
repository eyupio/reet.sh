package clientconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverUsesEnvFileBeforePrompting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("REET_URL", "")
	t.Setenv("REET_TOKEN", "")
	t.Setenv("REET_ENV", "")
	t.Setenv("REET_ENV_FILE", "")
	file := filepath.Join(dir, ".env")
	os.WriteFile(file, []byte("REET_URL=https://reet.example\nREET_TOKEN='abc123'\nREET_ENV=ns1\n"), 0o600)
	t.Chdir(dir)
	c := Discover("", "", "")
	if c.Server != "https://reet.example" || c.Host != "ns1" || !c.HasToken() || c.TokenFile != ".env" {
		t.Fatalf("unexpected %+v", c)
	}
}

func TestDiscoverReportsMissingToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	for _, k := range []string{"REET_URL", "REET_TOKEN", "REET_ENV", "REET_ENV_FILE"} {
		t.Setenv(k, "")
	}
	t.Chdir(dir)
	if c := Discover("", "", ""); c.HasToken() || c.Server != "" {
		t.Fatalf("unexpected %+v", c)
	}
}
