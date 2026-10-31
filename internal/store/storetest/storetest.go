// Package storetest opens a fresh store for tests from a database template
// migrated once per test binary, instead of running the full schema and
// migration path for every test.
package storetest

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

var (
	templateOnce sync.Once
	templateDir  string
	templateErr  error
)

// Open returns a fresh store with the current schema and migrations applied.
func Open(t testing.TB) (*store.Store, error) {
	t.Helper()
	templateOnce.Do(func() { templateDir, templateErr = buildTemplate() })
	if templateErr != nil {
		return nil, fmt.Errorf("build template: %w", templateErr)
	}
	dir := t.TempDir()
	if err := copyFile(filepath.Join(templateDir, "reet.db"), filepath.Join(dir, "reet.db")); err != nil {
		return nil, fmt.Errorf("copy template: %w", err)
	}
	s, err := store.OpenWithoutMigrations(dir)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, nil
}

func buildTemplate() (string, error) {
	dir, err := os.MkdirTemp("", "reet-store-template-")
	if err != nil {
		return "", err
	}
	s, err := store.Open(dir)
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	if err := s.Close(); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	if err := checkpoint(dir); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(filepath.Join(dir, "reet.db"+suffix)); err != nil && !os.IsNotExist(err) {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

func checkpoint(dir string) error {
	db, err := sql.Open("sqlite", filepath.Join(dir, "reet.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint template: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
