package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnreadableEntryIsIncompleteNotEmpty(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permissions")
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".agents", "skills")
	mkSkill(t, dir, "blocked", true)
	blocked := filepath.Join(dir, "blocked")
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(blocked, 0700)
	scan := CollectScan(home, t.TempDir(), nil, "fixture")
	if scan.Err() == nil || len(scan.Envs) != 1 || len(scan.Envs[0].Errors) == 0 {
		t.Fatalf("unreadable entry reported complete: %+v", scan)
	}
	os.Chmod(blocked, 0700)
	os.WriteFile(filepath.Join(dir, ".reet-sync.lock"), nil, 0600)
	scan = CollectScan(home, t.TempDir(), nil, "fixture")
	if scan.Err() != nil || len(scan.Found) != 1 {
		t.Fatalf("ordinary files broke scan: %+v", scan)
	}
}
func TestHostedLockRetainsPublisherSource(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".agents", "skills")
	mkSkill(t, dir, "example", true)
	os.WriteFile(filepath.Join(home, ".agents", ".skill-lock.json"), []byte(`{"skills":{"example":{"source":"reet.sh","sourceType":"well-known","sourceBaseUrl":"https://reet.sh/publisher/example"}}}`), 0600)
	scan := CollectScan(home, t.TempDir(), nil, "fixture")
	if len(scan.Found) != 1 || canonicalSource(scan.Found[0].Source) != "reet.sh/publisher" {
		t.Fatalf("publisher lost: %+v", scan.Found)
	}
}
