package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// RestoreResult names the copy that was restored and where the previous
// database was kept.
type RestoreResult struct {
	ID       string
	Previous string
	Message  string
}

// RestoreInstance downloads an instance copy and replaces the live database
// with it. closeStore is called before any live file changes so SQLite is not
// writing while its files are swapped. The store stays open when the download
// needs a password, so a caller can retry with one.
func RestoreInstance(ctx context.Context, svc *Service, dataDir, id, password string, closeStore func() error) (RestoreResult, error) {
	if closeStore == nil {
		closeStore = func() error { return nil }
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	manifest, payload, err := svc.DownloadWithPassword(ctx, 0, id, password)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := closeStore(); err != nil {
		return RestoreResult{}, err
	}
	if manifest.Kind != "instance" {
		return RestoreResult{}, errors.New("that is not an instance backup")
	}
	temp, err := os.CreateTemp(dataDir, ".reet-restore-*.db")
	if err != nil {
		return RestoreResult{}, err
	}
	tempName := temp.Name()
	temp.Close()
	defer os.Remove(tempName)
	if err := os.WriteFile(tempName, payload, 0o600); err != nil {
		return RestoreResult{}, err
	}
	if err := store.CheckDatabaseFile(ctx, tempName); err != nil {
		return RestoreResult{}, fmt.Errorf("the backup does not pass an integrity check: %w", err)
	}
	result := RestoreResult{ID: id}
	live := filepath.Join(dataDir, "reet.db")
	if _, err := os.Stat(live); err == nil {
		aside := live + ".before-restore-" + time.Now().UTC().Format("20060102T150405Z")
		if err := os.Rename(live, aside); err != nil {
			return RestoreResult{}, err
		}
		result.Previous = aside
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(live + suffix)
	}
	if err := os.Rename(tempName, live); err != nil {
		return RestoreResult{}, err
	}
	if err := os.Chmod(live, 0o600); err != nil {
		return RestoreResult{}, err
	}
	var message strings.Builder
	if result.Previous != "" {
		fmt.Fprintf(&message, "The database that was live is now %s.\n", result.Previous)
	}
	fmt.Fprintf(&message, "Restored %s. Start reet again; it will open the restored database.\n", id)
	result.Message = message.String()
	return result, nil
}
