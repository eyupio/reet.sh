package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
)

const (
	restoreRollbackSchemaVersion = 1
	maxRestoreJournalBytes       = 4 << 20
)

type RestoreRollbackOptions struct {
	Now func() time.Time
}

type RestoreRollbackResult struct {
	SchemaVersion int       `json:"schema_version"`
	SnapshotID    string    `json:"snapshot_id"`
	JournalPath   string    `json:"journal_path"`
	Removed       []string  `json:"removed"`
	Restored      []string  `json:"restored"`
	Skipped       []string  `json:"skipped"`
	CompletedAt   time.Time `json:"completed_at"`
}

type rollbackTarget struct {
	index       int
	action      restoreJournalAction
	expected    []byte
	backup      []byte
	destination string
}

// RollbackRestore reverses a completed create/replace journal. It verifies the
// journal against the snapshot and current destinations before changing anything.
func RollbackRestore(journalPath string, snapshot SetupSnapshot, manifest sealed.Manifest, destination Report, opts RestoreRollbackOptions) (RestoreRollbackResult, error) {
	journal, _, err := loadRestoreJournal(journalPath)
	if err != nil {
		return RestoreRollbackResult{}, err
	}
	if journal.SnapshotID != manifest.ID {
		return RestoreRollbackResult{}, errors.New("the restore journal belongs to a different snapshot")
	}
	result := RestoreRollbackResult{
		SchemaVersion: restoreRollbackSchemaVersion, SnapshotID: journal.SnapshotID,
		JournalPath: journalPath, Removed: []string{}, Restored: []string{}, Skipped: []string{},
	}
	if journal.State == "rolled_back" {
		result.CompletedAt = time.Now().UTC()
		return result, nil
	}
	if journal.State != "completed" {
		return RestoreRollbackResult{}, fmt.Errorf("the restore journal is %s, not a completed apply", journal.State)
	}
	plan := BuildRestorePlan(snapshot, manifest, destination)
	if plan.files == nil {
		return RestoreRollbackResult{}, errors.New("the snapshot has no selected content to roll back")
	}
	journalDir := filepath.Clean(filepath.Dir(journalPath))
	var targets []rollbackTarget
	for i, action := range journal.Actions {
		switch action.State {
		case "created", "replaced":
		case "skipped":
			result.Skipped = append(result.Skipped, action.Destination)
			continue
		default:
			return RestoreRollbackResult{}, fmt.Errorf("restore journal action %q is %s and cannot be rolled back", action.Destination, action.State)
		}
		file, ok := plan.files[action.Destination]
		if !ok {
			return RestoreRollbackResult{}, fmt.Errorf("restore journal destination %q is not in the snapshot plan", action.Destination)
		}
		expected, err := restoreContent(file)
		if err != nil {
			return RestoreRollbackResult{}, err
		}
		expectedHash := sha256Bytes(expected)
		if action.AppliedSHA256 == "" || action.AppliedSHA256 != expectedHash {
			return RestoreRollbackResult{}, fmt.Errorf("restore journal for %q does not match the snapshot content", action.Destination)
		}
		info, err := os.Lstat(action.Destination)
		if err != nil {
			return RestoreRollbackResult{}, fmt.Errorf("inspect %q before rollback: %w", action.Destination, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return RestoreRollbackResult{}, fmt.Errorf("%q is no longer a regular file", action.Destination)
		}
		if fileSHA256(action.Destination) != expectedHash {
			return RestoreRollbackResult{}, fmt.Errorf("%q changed after the apply; refusing to roll back", action.Destination)
		}
		target := rollbackTarget{index: i, action: action, expected: expected, destination: action.Destination}
		if action.State == "replaced" {
			backup := filepath.Clean(action.Backup)
			if action.Backup == "" || filepath.Dir(backup) != journalDir || !strings.HasPrefix(filepath.Base(backup), strings.TrimSuffix(filepath.Base(journalPath), ".json")+"-") || !strings.HasSuffix(backup, ".bak") {
				return RestoreRollbackResult{}, fmt.Errorf("restore journal rollback copy for %q is not in the journal directory", action.Destination)
			}
			backupInfo, err := os.Lstat(backup)
			if err != nil || backupInfo.Mode()&os.ModeSymlink != 0 || !backupInfo.Mode().IsRegular() {
				return RestoreRollbackResult{}, fmt.Errorf("restore journal rollback copy for %q is missing or unsafe", action.Destination)
			}
			backupData, err := os.ReadFile(backup)
			if err != nil || action.BackupSHA256 == "" || sha256Bytes(backupData) != action.BackupSHA256 {
				return RestoreRollbackResult{}, fmt.Errorf("restore journal rollback copy for %q is damaged", action.Destination)
			}
			target.backup = backupData
		}
		targets = append(targets, target)
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	for i := len(targets) - 1; i >= 0; i-- {
		target := targets[i]
		if target.action.State == "replaced" {
			if err := writeRestoreFile(target.destination, target.backup, true); err != nil {
				return rollbackFailure(journalPath, journal, target, err)
			}
			result.Restored = append(result.Restored, target.destination)
		} else {
			if err := os.Remove(target.destination); err != nil {
				return rollbackFailure(journalPath, journal, target, err)
			}
			result.Removed = append(result.Removed, target.destination)
		}
		journal.Actions[target.index].State = "rolled_back"
		if err := writeRestoreJournal(journalPath, journal); err != nil {
			return rollbackFailure(journalPath, journal, target, err)
		}
	}
	journal.State, journal.FinishedAt = "rolled_back", &now
	if err := writeRestoreJournal(journalPath, journal); err != nil {
		return RestoreRollbackResult{}, fmt.Errorf("finish the rollback journal: %w", err)
	}
	result.CompletedAt = now
	return result, nil
}

func rollbackFailure(journalPath string, journal restoreJournal, target rollbackTarget, cause error) (RestoreRollbackResult, error) {
	journal.State = "rollback_failed"
	journal.Actions[target.index].State = "rollback_failed"
	journal.Actions[target.index].Error = cause.Error()
	finished := time.Now().UTC()
	journal.FinishedAt = &finished
	if err := writeRestoreJournal(journalPath, journal); err != nil {
		return RestoreRollbackResult{}, fmt.Errorf("%w; writing the rollback journal also failed: %v", cause, err)
	}
	return RestoreRollbackResult{}, fmt.Errorf("rollback failed at %q: %w", target.destination, cause)
}

func loadRestoreJournal(path string) (restoreJournal, os.FileInfo, error) {
	var journal restoreJournal
	info, err := os.Lstat(path)
	if err != nil {
		return journal, nil, fmt.Errorf("inspect restore journal: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return journal, nil, errors.New("the restore journal must be a regular file, not a symlink")
	}
	if info.Size() > maxRestoreJournalBytes {
		return journal, nil, errors.New("the restore journal is larger than this build can read")
	}
	f, err := os.Open(path)
	if err != nil {
		return journal, nil, fmt.Errorf("read restore journal: %w", err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return journal, nil, errors.New("the restore journal changed during read")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxRestoreJournalBytes+1))
	if err != nil {
		return journal, nil, fmt.Errorf("read restore journal: %w", err)
	}
	if len(data) > maxRestoreJournalBytes {
		return journal, nil, errors.New("the restore journal is larger than this build can read")
	}
	if err := json.Unmarshal(data, &journal); err != nil {
		return journal, nil, errors.New("the restore journal is invalid")
	}
	if journal.SchemaVersion != restoreJournalSchemaVersion {
		return journal, nil, fmt.Errorf("unsupported restore journal schema version %d", journal.SchemaVersion)
	}
	if journal.SnapshotID == "" || journal.State == "" || len(journal.Actions) == 0 || journal.StartedAt.IsZero() {
		return journal, nil, errors.New("the restore journal is incomplete")
	}
	return journal, info, nil
}

func renderRestoreRollback(out io.Writer, result RestoreRollbackResult) error {
	if _, err := fmt.Fprintf(out, "Rolled back restore journal %q for snapshot %s.\n", result.JournalPath, result.SnapshotID); err != nil {
		return err
	}
	for _, path := range result.Removed {
		if _, err := fmt.Fprintf(out, "  removed %q\n", path); err != nil {
			return err
		}
	}
	for _, path := range result.Restored {
		if _, err := fmt.Fprintf(out, "  restored %q\n", path); err != nil {
			return err
		}
	}
	for _, path := range result.Skipped {
		if _, err := fmt.Fprintf(out, "  skipped %q\n", path); err != nil {
			return err
		}
	}
	if len(result.Removed)+len(result.Restored) == 0 {
		if _, err := fmt.Fprintln(out, "Nothing needed changing."); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "Rollback copies are left in place for inspection.")
	return err
}
