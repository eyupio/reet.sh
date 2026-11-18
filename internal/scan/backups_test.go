package scan

import (
	"bytes"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	mkdirs(t, home, ".cursor/skills")
	if err := pruneBackups(home, backupNow, nil); err != nil {
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
	mkdirs(t, home, ".claude/skills")
	if err := pruneBackups(home, backupNow, nil); err != nil {
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

func TestBackupClashListsAndRestoresNewestUnderRealName(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "skills")
	for _, body := range []string{"older", "newer"} {
		if err := os.MkdirAll(filepath.Join(folder, "foo"), 0755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(folder, "foo", "SKILL.md"), []byte(body), 0644)
		if _, err := backupSkill(folder, "foo", backupNow.Add(time.Duration(len(body))*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := listBackups(home)
	if err != nil || len(entries) != 2 || entries[0].Name != "foo" || entries[1].Name != "foo" {
		t.Fatalf("list = %+v, %v", entries, err)
	}
	if _, err := restoreBackup(home, "foo", ""); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(folder, "foo", "SKILL.md")); string(b) != "newer" {
		t.Fatalf("restored %q, want the newer copy", b)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", ".reet-backups", "2026-11-18", ".claude-foo-2")); !os.IsNotExist(err) {
		t.Fatal("restored entry still present")
	}
	// The older copy is left; with foo installed again a second restore refuses.
	if _, err := restoreBackup(home, "foo", "2026-11-18"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected refusal, got %v", err)
	}
	if left, _ := listBackups(home); len(left) != 1 || left[0].Name != "foo" {
		t.Fatalf("left = %+v", left)
	}
}

func TestBackupCorruptIndexStillLists(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".cursor", "skills")
	writeSkill(t, filepath.Join(folder, "alpha"))
	got, err := backupSkill(folder, "alpha", backupNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(got), "index.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := listBackups(home)
	if err != nil || len(entries) != 1 || entries[0].Name != "alpha" || entries[0].Folder != folder {
		t.Fatalf("list = %+v, %v", entries, err)
	}
}

func TestListBackupsLegacyRemovedEntries(t *testing.T) {
	home := t.TempDir()
	removed := filepath.Join(home, ".claude", ".reet-sync-backups", "removed-abc")
	writeSkill(t, filepath.Join(removed, "foo"))
	when := backupNow.AddDate(0, 0, -60)
	if err := os.Chtimes(removed, when, when); err != nil {
		t.Fatal(err)
	}
	entries, err := listBackups(home)
	if err != nil || len(entries) != 1 {
		t.Fatalf("list = %+v, %v", entries, err)
	}
	if want := when.Local().Format("2006-01-02"); entries[0].Date != want || entries[0].Name != "foo" {
		t.Fatalf("entry = %+v, want date %s", entries[0], want)
	}
	mkdirs(t, home, ".claude/skills")
	if err := pruneBackups(home, backupNow, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(removed, "foo", "SKILL.md")); err != nil {
		t.Fatal("prune touched a legacy backup")
	}
	got, err := restoreBackup(home, "foo", "")
	if err != nil || got != filepath.Join(home, ".claude", "skills", "foo") {
		t.Fatalf("restore = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(got, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(removed); !os.IsNotExist(err) {
		t.Fatal("empty removed-* folder kept")
	}
}

// A move between devices copies, and never removes what it finds where it
// copies to: a copy that appeared there stays, and a copy that fails part-way
// removes only what it made.
func TestBackupMoveAcrossDevicesLeavesWhatIsAtItsDestination(t *testing.T) {
	was := renameEntry
	t.Cleanup(func() { renameEntry = was })
	renameEntry = func(src, dest string) error {
		return &os.LinkError{Op: "rename", Old: src, New: dest, Err: syscall.EXDEV}
	}
	root := t.TempDir()
	folder := filepath.Join(root, "folder")
	writeSkill(t, folder)
	link := filepath.Join(root, "link")
	if err := os.Symlink(folder, link); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{folder, link, file} {
		dest := filepath.Join(root, "taken-"+filepath.Base(src))
		writeSkill(t, dest)
		if err := moveEntry(src, dest); err == nil {
			t.Fatalf("%s: moved onto a copy that was there", src)
		}
		if b, err := os.ReadFile(filepath.Join(dest, "SKILL.md")); err != nil || string(b) != "body" {
			t.Fatalf("%s: the copy at the destination was touched: %q %v", src, b, err)
		}
		if _, err := os.Lstat(src); err != nil {
			t.Fatalf("%s: the source went: %v", src, err)
		}
	}
	partial := filepath.Join(root, "with-pipe")
	writeSkill(t, partial)
	if err := syscall.Mkfifo(filepath.Join(partial, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "fresh")
	if err := copyEntry(partial, dest); err == nil {
		t.Fatal("copied a named pipe")
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("a failed copy left what it made: %v", err)
	}
}

// Another process writing where a copy is going while it is made keeps what
// it wrote when the copy then fails: the copy is made aside and moved in only
// when complete, and cleaning up removes only what the copy made.
func TestBackupCopyLeavesWhatAnotherProcessWritesAlone(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	writeSkill(t, src)
	if err := syscall.Mkfifo(filepath.Join(src, "zz-pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "dest")
	t.Cleanup(func() { copyEntryStep = nil })
	copyEntryStep = func() {
		if err := os.MkdirAll(dest, 0755); err != nil {
			t.Error(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "theirs.txt"), []byte("theirs"), 0644); err != nil {
			t.Error(err)
		}
	}
	if err := copyEntry(src, dest); err == nil {
		t.Fatal("copied a named pipe")
	}
	if b, err := os.ReadFile(filepath.Join(dest, "theirs.txt")); err != nil || string(b) != "theirs" {
		t.Fatalf("another process's file went: %q %v", b, err)
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".reet-copy-") {
			t.Fatalf("left %s", e.Name())
		}
	}
}

// Across devices, a backup and its restore copy through a hidden folder
// named apart from the skill, so a long name still fits.
func TestBackupAndRestoreALongNameAcrossDevices(t *testing.T) {
	was := renameEntry
	t.Cleanup(func() { renameEntry = was })
	renameEntry = func(src, dest string) error {
		return &os.LinkError{Op: "rename", Old: src, New: dest, Err: syscall.EXDEV}
	}
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "skills")
	name := strings.Repeat("b", 240)
	writeSkill(t, filepath.Join(folder, name))
	if _, err := backupSkill(folder, name, backupNow); err != nil {
		t.Fatal(err)
	}
	got, err := restoreBackup(home, name, "")
	if err != nil || got != filepath.Join(folder, name) {
		t.Fatalf("restore = %q, %v", got, err)
	}
	if b, err := os.ReadFile(filepath.Join(got, "SKILL.md")); err != nil || string(b) != "body" {
		t.Fatalf("restored %q %v", b, err)
	}
}

// A date folder goes only when its whole day is more than 30 days gone: a
// backup made late on a day is still kept 30 days after it was made.
func TestPruneBackupsKeepsADateFolderUntilItsWholeDayIs30DaysOld(t *testing.T) {
	for _, tc := range []struct {
		name string
		now  time.Time
	}{
		{"utc noon", time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)},
		{"just after midnight", time.Date(2026, 11, 18, 0, 0, 1, 0, time.UTC)},
		{"just before midnight", time.Date(2026, 11, 18, 23, 59, 59, 0, time.UTC)},
		{"far west, local date ahead of nowhere", time.Date(2026, 11, 18, 23, 30, 0, 0, time.FixedZone("west", -10*3600))},
		{"far east, local date ahead of utc", time.Date(2026, 11, 18, 0, 30, 0, 0, time.FixedZone("east", 14*3600))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".cursor", ".reet-backups")
			day := func(n int) string { return tc.now.AddDate(0, 0, -n).Format(backupDateLayout) }
			for _, n := range []int{29, 30, 31} {
				writeSkill(t, filepath.Join(root, day(n), ".cursor-a"))
			}
			mkdirs(t, home, ".cursor/skills")
			if err := pruneBackups(home, tc.now, nil); err != nil {
				t.Fatal(err)
			}
			for n, kept := range map[int]bool{29: true, 30: true, 31: false} {
				_, err := os.Lstat(filepath.Join(root, day(n)))
				if kept && err != nil {
					t.Errorf("the folder %d days back was pruned: %v", n, err)
				}
				if !kept && !os.IsNotExist(err) {
					t.Errorf("the folder %d days back was kept", n)
				}
			}
		})
	}
}

// A .reet-backups that is a link, or a date folder that is, is not followed:
// list and prune ignore it, so a backup made there could not be found again.
func TestBackupRefusesALinkedBackupsRootOrDateFolder(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "skills")
	writeSkill(t, filepath.Join(folder, "alpha"))
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(home, ".claude", ".reet-backups")); err != nil {
		t.Fatal(err)
	}
	if _, err := backupSkill(folder, "alpha", backupNow); err == nil {
		t.Fatal("backed up through a linked .reet-backups")
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("wrote through the link: %v", entries)
	}
	if _, err := os.Lstat(filepath.Join(folder, "alpha")); err != nil {
		t.Fatalf("the skill moved anyway: %v", err)
	}

	home = t.TempDir()
	folder = filepath.Join(home, ".claude", "skills")
	writeSkill(t, filepath.Join(folder, "alpha"))
	elsewhere = t.TempDir()
	root := filepath.Join(home, ".claude", ".reet-backups")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(root, "2026-11-18")); err != nil {
		t.Fatal(err)
	}
	if _, err := backupSkill(folder, "alpha", backupNow); err == nil {
		t.Fatal("backed up through a linked date folder")
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("wrote through the link: %v", entries)
	}
}

// The move is what matters: when the name cannot be recorded the backup
// stands, the caller is told so, and a clash copy still lists as its skill.
func TestBackupSucceedsWhenItsIndexCannotBeWritten(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "skills")
	dateDir := filepath.Join(home, ".claude", ".reet-backups", "2026-11-18")
	// A folder where index.json goes makes every write of the index fail.
	if err := os.MkdirAll(filepath.Join(dateDir, backupIndexFile), 0700); err != nil {
		t.Fatal(err)
	}
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	var got []string
	for i := 0; i < 2; i++ {
		writeSkill(t, filepath.Join(folder, "foo"))
		dest, err := backupSkill(folder, "foo", backupNow)
		if err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
		if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
			t.Fatalf("backup %d is not at %s: %v", i, dest, err)
		}
		got = append(got, filepath.Base(dest))
	}
	if got[0] != ".claude-foo" || got[1] != ".claude-foo-2" {
		t.Fatalf("backups at %v", got)
	}
	if !strings.Contains(logged.String(), "could not record") {
		t.Fatalf("no warning logged: %q", logged.String())
	}
	entries, err := listBackups(home)
	if err != nil || len(entries) != 2 || entries[0].Name != "foo" || entries[1].Name != "foo" {
		t.Fatalf("list = %+v, %v", entries, err)
	}
}

// Without an index, a trailing -N is a clash suffix only when the entry it
// would be a clash of is there too.
func TestListBackupsReadsAClashSuffixOnlyBesideItsOriginal(t *testing.T) {
	home := t.TempDir()
	dateDir := filepath.Join(home, ".cursor", ".reet-backups", "2026-11-18")
	for _, n := range []string{".cursor-foo", ".cursor-foo-2", ".cursor-bar-2", ".cursor-baz-x"} {
		writeSkill(t, filepath.Join(dateDir, n))
	}
	entries, err := listBackups(home)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	if want := "bar-2 baz-x foo foo"; strings.Join(names, " ") != want {
		t.Fatalf("names %q, want %q", names, want)
	}
}

// A backup moved across devices is copied and then removed, through the
// same call that moves it on one device: a folder, a link and a file.
func TestMoveEntryAcrossDevicesCopiesThenRemoves(t *testing.T) {
	was := renameEntry
	t.Cleanup(func() { renameEntry = was })
	var tried int
	renameEntry = func(src, dest string) error {
		tried++
		return &os.LinkError{Op: "rename", Old: src, New: dest, Err: syscall.EXDEV}
	}
	root := t.TempDir()
	folder := filepath.Join(root, "folder")
	writeSkill(t, folder)
	if err := os.Symlink("SKILL.md", filepath.Join(folder, "inner")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(folder, link); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{link, file, folder} {
		dest := filepath.Join(root, "moved-"+filepath.Base(src))
		copied, err := moveEntryCopying(src, dest)
		if err != nil || !copied {
			t.Fatalf("%s: copied %v, err %v", src, copied, err)
		}
		if _, err := os.Lstat(src); !os.IsNotExist(err) {
			t.Fatalf("%s: the source stayed: %v", src, err)
		}
		st, err := os.Lstat(dest)
		if err != nil {
			t.Fatal(err)
		}
		switch src {
		case link:
			if l, err := os.Readlink(dest); st.Mode()&os.ModeSymlink == 0 || err != nil || l != folder {
				t.Fatalf("link moved as %v %q %v", st.Mode(), l, err)
			}
		case file:
			if b, _ := os.ReadFile(dest); string(b) != "x" {
				t.Fatalf("file moved as %q", b)
			}
		case folder:
			if l, err := os.Readlink(filepath.Join(dest, "inner")); err != nil || l != "SKILL.md" {
				t.Fatalf("inner link = %q %v", l, err)
			}
			if b, _ := os.ReadFile(filepath.Join(dest, "SKILL.md")); string(b) != "body" {
				t.Fatal("folder not copied")
			}
		}
	}
	if tried != 3 {
		t.Fatalf("rename tried %d times, want 3", tried)
	}
}

func TestRenameNoReplaceRefusesAnythingAtTheDestination(t *testing.T) {
	root := t.TempDir()
	mkFile := func(name, body string) string {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	mkEmptyDir := func(name string) string {
		p := filepath.Join(root, name)
		if err := os.Mkdir(p, 0755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	check := func(name string, src, dest string) {
		t.Helper()
		err := renameNoReplace(src, dest)
		if !errors.Is(err, fs.ErrExist) {
			t.Fatalf("%s: err = %v, want one that is fs.ErrExist", name, err)
		}
		if _, err := os.Lstat(src); err != nil {
			t.Fatalf("%s: the source went: %v", name, err)
		}
	}
	// A file onto a file, a folder onto an empty folder, a link onto a file:
	// os.Rename replaces the target in each of these.
	check("file", mkFile("f1", "mine"), mkFile("f2", "theirs"))
	if b, _ := os.ReadFile(filepath.Join(root, "f2")); string(b) != "theirs" {
		t.Fatalf("the file at the destination is now %q", b)
	}
	d1 := mkEmptyDir("d1")
	mkFile("d1/inner", "mine")
	check("folder", d1, mkEmptyDir("d2"))
	if entries, _ := os.ReadDir(filepath.Join(root, "d2")); len(entries) != 0 {
		t.Fatalf("the empty folder at the destination now holds %v", entries)
	}
	link := filepath.Join(root, "l1")
	if err := os.Symlink("f1", link); err != nil {
		t.Fatal(err)
	}
	check("link", link, mkFile("f3", "theirs"))
	// Where nothing is, it moves.
	if err := renameNoReplace(d1, filepath.Join(root, "free")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "free", "inner")); err != nil {
		t.Fatal(err)
	}
}

// Where the kernel or file system has no no-replace rename, the check and
// the plain rename stand in for it.
func TestRenameNoReplaceFallsBackWhenTheSystemCannotRefuse(t *testing.T) {
	for _, errno := range []error{syscall.ENOSYS, syscall.EINVAL, syscall.ENOTSUP} {
		t.Run(errno.Error(), func(t *testing.T) {
			was := renameExcl
			t.Cleanup(func() { renameExcl = was })
			var calls int
			renameExcl = func(src, dest string) error { calls++; return errno }
			root := t.TempDir()
			src, taken, free := filepath.Join(root, "src"), filepath.Join(root, "taken"), filepath.Join(root, "free")
			for _, p := range []string{src, taken} {
				if err := os.WriteFile(p, []byte(filepath.Base(p)), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := renameNoReplace(src, taken); !errors.Is(err, fs.ErrExist) {
				t.Fatalf("err = %v, want fs.ErrExist", err)
			}
			if b, _ := os.ReadFile(taken); string(b) != "taken" {
				t.Fatalf("destination is now %q", b)
			}
			if err := renameNoReplace(src, free); err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(free); string(b) != "src" || calls != 2 {
				t.Fatalf("moved %q after %d tries", b, calls)
			}
		})
	}
}

// An entry that appears where a skill is restored to after the check, before
// the move, is left as it is, and the backup stays where it was.
func TestRestoreBackupNeverReplacesWhatAppearsBeforeTheMove(t *testing.T) {
	for _, kind := range []string{"file", "empty folder"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			folder := filepath.Join(home, ".claude", "skills")
			// A file or link is what a plain rename replaces; a folder is
			// what it replaces where the system lets it.
			if kind == "file" {
				writeSkill(t, filepath.Join(home, "shared", "alpha"))
				if err := os.MkdirAll(folder, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(home, "shared", "alpha"), filepath.Join(folder, "alpha")); err != nil {
					t.Fatal(err)
				}
			} else {
				writeSkill(t, filepath.Join(folder, "alpha"))
			}
			backup, err := backupSkill(folder, "alpha", backupNow)
			if err != nil {
				t.Fatal(err)
			}
			was := renameEntry
			t.Cleanup(func() { renameEntry = was })
			target := filepath.Join(folder, "alpha")
			renameEntry = func(src, dest string) error {
				if dest == target {
					if kind == "file" {
						if err := os.WriteFile(dest, []byte("theirs"), 0644); err != nil {
							t.Error(err)
						}
					} else if err := os.Mkdir(dest, 0755); err != nil {
						t.Error(err)
					}
				}
				return was(src, dest)
			}
			if _, err := restoreBackup(home, "alpha", ""); err == nil {
				t.Fatal("restored over what appeared")
			}
			st, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "file" {
				if b, _ := os.ReadFile(target); string(b) != "theirs" {
					t.Fatalf("target is now %q", b)
				}
			} else if entries, _ := os.ReadDir(target); !st.IsDir() || len(entries) != 0 {
				t.Fatalf("the empty folder was replaced: %v", entries)
			}
			if _, err := os.Stat(filepath.Join(backup, "SKILL.md")); err != nil {
				t.Fatalf("the backup is gone: %v", err)
			}
			if _, err := os.Lstat(backup); err != nil {
				t.Fatalf("the backup is gone: %v", err)
			}
			if left, _ := listBackups(home); len(left) != 1 {
				t.Fatalf("backup no longer listed: %+v", left)
			}
		})
	}
}

// A copy across devices is moved in with the same refusal to replace.
func TestCopyEntryNeverReplacesWhatAppearsBeforeItIsMovedIn(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.WriteFile(src, []byte("mine"), 0644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "dest")
	was := copyRename
	t.Cleanup(func() { copyRename = was })
	copyRename = func(from, to string) error {
		if err := os.WriteFile(to, []byte("theirs"), 0644); err != nil {
			t.Error(err)
		}
		return was(from, to)
	}
	if err := copyEntry(src, dest); err == nil {
		t.Fatal("moved a copy over a file that appeared")
	}
	if b, _ := os.ReadFile(dest); string(b) != "theirs" {
		t.Fatalf("the file that appeared is now %q", b)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 2 {
		t.Fatalf("left %v", entries)
	}
}
