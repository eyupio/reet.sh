package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkSharedInstallExposesSkillInAgentFolder(t *testing.T) {
	home := t.TempDir()
	canonical := filepath.Join(home, ".agents", "skills", "demo")
	if err := os.MkdirAll(canonical, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("# demo"), 0644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".config", "opencode", "skills")
	if err := linkSharedInstall(home, target, "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "demo", "SKILL.md")); err != nil {
		t.Fatalf("skill not visible through the link: %v", err)
	}
	if err := linkSharedInstall(home, target, "demo"); err != nil {
		t.Fatalf("second call should be a no-op: %v", err)
	}
	if err := linkSharedInstall(home, target, "missing"); err == nil {
		t.Fatal("expected an error when the shared install is absent")
	}
}
