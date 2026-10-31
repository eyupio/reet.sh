package store

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var (
	testTemplateOnce sync.Once
	testTemplateDir  string
	testTemplateErr  error
)

func openTemplateStore(t *testing.T) (*Store, error) {
	t.Helper()
	testTemplateOnce.Do(func() { testTemplateDir, testTemplateErr = buildTestTemplate() })
	if testTemplateErr != nil {
		return nil, fmt.Errorf("build template: %w", testTemplateErr)
	}
	dir := t.TempDir()
	if err := copyTestFile(filepath.Join(testTemplateDir, "reet.db"), filepath.Join(dir, "reet.db")); err != nil {
		return nil, fmt.Errorf("copy template: %w", err)
	}
	return OpenWithoutMigrations(dir)
}

func buildTestTemplate() (string, error) {
	dir, err := os.MkdirTemp("", "reet-store-template-")
	if err != nil {
		return "", err
	}
	s, err := Open(dir)
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	if err := s.Close(); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	if err := checkpointTestTemplate(dir); err != nil {
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

func checkpointTestTemplate(dir string) error {
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

func copyTestFile(src, dst string) error {
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
