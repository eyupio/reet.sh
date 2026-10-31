package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestRestoreInstanceReplacesTheLiveDatabase(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := FromEnv(func(key string) string {
		if key == "REET_DATA_DIR" {
			return dir
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := New(st, cfg).RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RestoreInstance(context.Background(), New(st, cfg), dir, item.ID, "", st.Close)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != item.ID || result.Previous == "" || result.Message == "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "reet.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(result.Previous); err != nil {
		t.Fatal("previous database was not kept")
	}
}
