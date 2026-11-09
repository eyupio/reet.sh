package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/backup"
)

func runEnv(t *testing.T) (dir, key string) {
	t.Helper()
	dir = t.TempDir()
	key = strings.Repeat("k", 32)
	t.Setenv("REET_DATA_DIR", dir)
	t.Setenv("REET_BACKUP_KEY", key)
	t.Setenv("REET_BACKUP_DIR", filepath.Join(dir, "backups"))
	t.Setenv("HOME", t.TempDir())
	return dir, key
}

func copiesOnDisk(dir string) []string {
	m, _ := filepath.Glob(filepath.Join(dir, "backups", "instance-*"))
	return m
}

func TestBackupRunWithAPasswordFileSealsThatCopyAlone(t *testing.T) {
	dir, key := runEnv(t)
	pw := "a password for the team's copy"
	file := filepath.Join(t.TempDir(), "pw")
	os.WriteFile(file, []byte(pw+"\n"), 0o600)
	var out bytes.Buffer
	if err := backupRun([]string{"--password-file", file}, &out); err != nil {
		t.Fatal(err)
	}
	copies := copiesOnDisk(dir)
	if len(copies) != 1 {
		t.Fatalf("copies: %v", copies)
	}
	raw, _ := os.ReadFile(copies[0])
	if m, _ := backup.ManifestOf(raw); m.Protection != backup.ProtectionPassword {
		t.Fatalf("protection %q", m.Protection)
	}
	if _, _, err := backup.OpenArchive(raw, pw, ""); err != nil {
		t.Fatalf("the password did not open it: %v", err)
	}
	if _, _, err := backup.OpenArchive(raw, "", key); err == nil {
		t.Fatal("the controller's key opened a password copy")
	}
	if strings.Contains(out.String(), pw) {
		t.Fatal("the output repeats the password")
	}
}

func TestBackupRunWithNoPasswordFileIsAsBefore(t *testing.T) {
	dir, _ := runEnv(t)
	if err := backupRun(nil, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(copiesOnDisk(dir)[0])
	if m, _ := backup.ManifestOf(raw); m.Protection == backup.ProtectionPassword {
		t.Fatal("a plain run was password protected")
	}
}

func TestBackupRunRefusesABadPasswordFileBeforeTakingAnything(t *testing.T) {
	dir, _ := runEnv(t)
	short, blank := filepath.Join(t.TempDir(), "short"), filepath.Join(t.TempDir(), "blank")
	os.WriteFile(short, []byte("short"), 0o600)
	os.WriteFile(blank, []byte("  \n"), 0o600)
	for _, args := range [][]string{{"--password-file", short}, {"--password-file", blank}, {"--password-file", filepath.Join(t.TempDir(), "absent")}} {
		if err := backupRun(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("%v was accepted", args)
		}
	}
	if n := len(copiesOnDisk(dir)); n != 0 {
		t.Fatalf("a refused run took %d copies", n)
	}
	// A password is a file or -, never an argument.
	if err := backupRun([]string{"--password", "a long enough password"}, &bytes.Buffer{}); err == nil {
		t.Fatal("run accepted a password as an argument")
	}
	if err := backupRun([]string{"a long enough password"}, &bytes.Buffer{}); err == nil {
		t.Fatal("run accepted a password as a positional argument")
	}
}
