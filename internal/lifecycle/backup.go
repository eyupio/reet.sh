package lifecycle

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func openReadOnly(path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{"mode": {"ro"}, "_pragma": {"busy_timeout(3000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err == nil {
		db.SetMaxOpenConns(1)
	}
	return db, err
}
func checkDB(ctx context.Context, path string) error {
	db, err := openReadOnly(path)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var result string
	if err = db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("SQLite integrity check failed")
	}
	return nil
}

// VACUUM INTO includes committed WAL data without copying a partially written database.
func backupDB(ctx context.Context, data, state string) (string, error) {
	path := filepath.Join(data, "reet.db")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	dir := filepath.Join(state, "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "reet-*.db")
	if err != nil {
		return "", err
	}
	dest := f.Name()
	f.Close()
	os.Remove(dest)
	db, err := openReadOnly(path)
	if err != nil {
		return "", err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err = db.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		os.Remove(dest)
		return "", fmt.Errorf("database backup failed: %w", err)
	}
	if err = os.Chmod(dest, 0600); err == nil {
		err = checkDB(ctx, dest)
	}
	if err != nil {
		os.Remove(dest)
		return "", err
	}
	return dest, nil
}

// Call only after a managed service has been confirmed stopped.
func restoreDB(backup, data string) error {
	if backup == "" {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(filepath.Join(data, "reet.db") + suffix); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	b, err := os.ReadFile(backup)
	if err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err = os.Remove(filepath.Join(data, "reet.db") + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return atomicFile(filepath.Join(data, "reet.db"), b, 0600)
}
