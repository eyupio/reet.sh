package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicReportsAFailedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "copy.enc")
	if err := os.WriteFile(path, []byte("the copy before"), 0o600); err != nil {
		t.Fatal(err)
	}
	full := errors.New("no space left on device")
	old := fileWrite
	fileWrite = func(f *os.File, data []byte) (int, error) {
		n, _ := f.Write(data[:len(data)/2])
		return n, full
	}
	t.Cleanup(func() { fileWrite = old })

	if err := writeAtomic(path, []byte("a whole new copy"), 0o600); !errors.Is(err, full) {
		t.Fatalf("writeAtomic = %v, want the write's error", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "the copy before" {
		t.Fatalf("the file in place is %q (%v); a half-written one replaced it", got, err)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".reet-backup-*"))
	if len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
}

func TestSwapInKeepsTheWriteAheadLogWithThePreviousDatabase(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "reet.db")
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(live, "old database")
	write(live+"-wal", "commits not yet in the file")
	write(live+"-shm", "index")
	src := filepath.Join(dir, "restored.tmp")
	write(src, "restored database")

	previous, err := swapIn(dir, src)
	if err != nil || previous == "" {
		t.Fatalf("swapIn = %q %v", previous, err)
	}
	for path, want := range map[string]string{
		live:              "restored database",
		previous:          "old database",
		previous + "-wal": "commits not yet in the file",
		previous + "-shm": "index",
	} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("%s = %q (%v), want %q", filepath.Base(path), got, err, want)
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(live + suffix); !os.IsNotExist(err) {
			t.Fatalf("reet.db%s was left beside the restored database", suffix)
		}
	}
}

func TestSwapInRemovesAStrayLogWhenNothingWasLive(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "reet.db")
	if err := os.WriteFile(live+"-wal", []byte("stray"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "restored.tmp")
	if err := os.WriteFile(src, []byte("restored database"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := swapIn(dir, src)
	if err != nil || previous != "" {
		t.Fatalf("swapIn = %q %v", previous, err)
	}
	if _, err := os.Stat(live + "-wal"); !os.IsNotExist(err) {
		t.Fatal("a stray write-ahead log was left beside the restored database")
	}
}
