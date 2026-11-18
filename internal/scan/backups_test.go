package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var backupNow = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

func writeSkill(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("body"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestBackupMovesFolderAndSymlink(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "skills")
	writeSkill(t, filepath.Join(folder, "alpha"))
	shared := filepath.Join(home, ".agents", "skills", "beta")
	writeSkill(t, shared)
	if err := os.Symlink(shared, filepath.Join(folder, "beta")); err != nil {
		t.Fatal(err)
	}

	got, err := backupSkill(folder, "alpha", backupNow)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".claude", ".reet-backups", "2026-11-18", ".claude-alpha")
	if got != want {
		t.Fatalf("path = %s, want %s", got, want)
	}
	if _, err := os.Stat(filepath.Join(got, "SKILL.md")); err != nil {
		t.Fatalf("backup content missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(folder, "alpha")); !os.IsNotExist(err) {
		t.Fatal("original still in place")
	}
	if st, _ := os.Stat(filepath.Dir(got)); st.Mode().Perm() != 0700 {
		t.Fatalf("date folder mode = %v", st.Mode().Perm())
	}

	link, err := backupSkill(folder, "beta", backupNow)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(link)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink not kept as a link: %v %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(shared, "SKILL.md")); err != nil {
		t.Fatal("symlink target was touched")
	}
}

func TestBackupNameClash(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".agents", "skills")
	writeSkill(t, filepath.Join(folder, "alpha"))
	first, err := backupSkill(folder, "alpha", backupNow)
	if err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(folder, "alpha"))
	second, err := backupSkill(folder, "alpha", backupNow)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != ".agents-alpha" || filepath.Base(second) != ".agents-alpha-2" {
		t.Fatalf("got %s and %s", first, second)
	}
}

func TestCopyEntryKeepsSymlinks(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeSkill(t, src)
	if err := os.Symlink("SKILL.md", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "dest")
	if err := copyEntry(src, dest); err != nil {
		t.Fatal(err)
	}
	if l, err := os.Readlink(filepath.Join(dest, "link")); err != nil || l != "SKILL.md" {
		t.Fatalf("link = %q, %v", l, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "SKILL.md")); string(b) != "body" {
		t.Fatal("file not copied")
	}
}

func TestPruneBackupsKeeps30Days(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cursor", ".reet-backups")
	old := backupNow.AddDate(0, 0, -31).Format("2006-01-02")
	recent := backupNow.AddDate(0, 0, -29).Format("2006-01-02")
	writeSkill(t, filepath.Join(root, old, ".cursor-a"))
	writeSkill(t, filepath.Join(root, recent, ".cursor-b"))
	writeSkill(t, filepath.Join(root, "notes", "x")) // not a date: left alone
	if err := pruneBackups(home, backupNow); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, old)); !os.IsNotExist(err) {
		t.Fatal("31-day-old backup kept")
	}
	for _, keep := range []string{recent, "notes"} {
		if _, err := os.Lstat(filepath.Join(root, keep)); err != nil {
			t.Fatalf("%s removed: %v", keep, err)
		}
	}
}

func TestPruneBackupsRemovesSymlinkWithoutFollowing(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "keep")
	writeSkill(t, outside)
	day := filepath.Join(home, ".claude", ".reet-backups", backupNow.AddDate(0, 0, -40).Format("2006-01-02"))
	if err := os.MkdirAll(day, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(day, ".claude-x")); err != nil {
		t.Fatal(err)
	}
	if err := pruneBackups(home, backupNow); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(day); !os.IsNotExist(err) {
		t.Fatal("old date folder kept")
	}
	if _, err := os.Stat(filepath.Join(outside, "SKILL.md")); err != nil {
		t.Fatal("prune followed the symlink")
	}
}

func TestRestoreBackupByNameAndDate(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "skills")
	writeSkill(t, filepath.Join(folder, "alpha"))
	if _, err := backupSkill(folder, "alpha", backupNow.AddDate(0, 0, -2)); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(folder, "alpha"))
	if _, err := backupSkill(folder, "alpha", backupNow); err != nil {
		t.Fatal(err)
	}
	entries, err := listBackups(home)
	if err != nil || len(entries) != 2 || entries[0].Date != "2026-11-18" || entries[0].Folder != folder {
		t.Fatalf("list = %+v, %v", entries, err)
	}

	got, err := restoreBackup(home, "alpha", "2026-11-16")
	if err != nil || got != filepath.Join(folder, "alpha") {
		t.Fatalf("restore = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(got, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreBackup(home, "alpha", ""); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected refusal when the skill exists, got %v", err)
	}
	if left, _ := listBackups(home); len(left) != 1 {
		t.Fatalf("refused restore must leave the backup: %+v", left)
	}
	if _, err := restoreBackup(home, "alpha", "2026-01-01"); err == nil {
		t.Fatal("expected no backup for that date")
	}
}

func TestRestoreBackupSymlinkAndNewestDefault(t *testing.T) {
	home := t.TempDir()
	shared := filepath.Join(home, ".agents", "skills", "beta")
	writeSkill(t, shared)
	folder := filepath.Join(home, ".cursor", "skills")
	if err := os.MkdirAll(folder, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(folder, "beta")); err != nil {
		t.Fatal(err)
	}
	if _, err := backupSkill(folder, "beta", backupNow); err != nil {
		t.Fatal(err)
	}
	got, err := restoreBackup(home, "beta", "")
	if err != nil {
		t.Fatal(err)
	}
	if l, err := os.Readlink(got); err != nil || l != shared {
		t.Fatalf("restored link = %q, %v", l, err)
	}
}

func TestRestoreBackupRefusesUnsafeInput(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "skills")
	writeSkill(t, filepath.Join(folder, "alpha"))
	if _, err := backupSkill(folder, "alpha", backupNow); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "..", "../x", "a/b", `a\b`, "."} {
		if _, err := restoreBackup(home, name, ""); err == nil {
			t.Errorf("name %q accepted", name)
		}
		if _, err := backupSkill(folder, name, backupNow); err == nil {
			t.Errorf("backup of %q accepted", name)
		}
	}
	for _, date := range []string{"yesterday", "2026-1-1", "../2026-11-18", "2026-13-01"} {
		if _, err := restoreBackup(home, "alpha", date); err == nil {
			t.Errorf("date %q accepted", date)
		}
	}
}

func TestAgentRestoreCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	folder := filepath.Join(home, ".agents", "skills")
	writeSkill(t, filepath.Join(folder, "alpha"))
	if _, err := backupSkill(folder, "alpha", backupNow); err != nil {
		t.Fatal(err)
	}
	cfg := "--config=" + filepath.Join(t.TempDir(), "none.json")
	var out bytes.Buffer
	if err := runAgentRestore([]string{cfg, "--list"}, &out); err != nil {
		t.Fatal(err)
	}
	if want := "2026-11-18  alpha  from " + folder + "\n"; out.String() != want {
		t.Fatalf("list = %q, want %q", out.String(), want)
	}
	out.Reset()
	if err := runAgentRestore([]string{cfg, "alpha", "--date", "2026-11-18"}, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, "alpha", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := runAgentRestore([]string{cfg}, &out); err == nil {
		t.Fatal("expected usage error")
	}
}
