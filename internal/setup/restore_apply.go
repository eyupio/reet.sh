package setup

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/pelletier/go-toml/v2"
)

const (
	restoreApplySchemaVersion   = 1
	restoreJournalSchemaVersion = 2
)

type RestoreApplyOptions struct {
	JournalDir  string
	Now         func() time.Time
	Conflict    string // "", "skip" or "replace"
	beforeWrite func(string) error
}

type RestoreApplyResult struct {
	SchemaVersion   int       `json:"schema_version"`
	SnapshotID      string    `json:"snapshot_id"`
	PlanFingerprint string    `json:"plan_fingerprint"`
	Created         []string  `json:"created"`
	Replaced        []string  `json:"replaced"`
	Skipped         []string  `json:"skipped"`
	Backups         []string  `json:"backups"`
	JournalPath     string    `json:"journal_path"`
	CompletedAt     time.Time `json:"completed_at"`
}

type restoreJournal struct {
	SchemaVersion   int                    `json:"schema_version"`
	SnapshotID      string                 `json:"snapshot_id"`
	PlanFingerprint string                 `json:"plan_fingerprint"`
	State           string                 `json:"state"`
	StartedAt       time.Time              `json:"started_at"`
	FinishedAt      *time.Time             `json:"finished_at,omitempty"`
	Actions         []restoreJournalAction `json:"actions"`
}

type restoreJournalAction struct {
	Destination   string `json:"destination"`
	State         string `json:"state"`
	Backup        string `json:"backup,omitempty"`
	AppliedSHA256 string `json:"applied_sha256,omitempty"`
	BackupSHA256  string `json:"backup_sha256,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ApplyRestorePlan creates new files or replaces conflicts according to the
// chosen conflict policy. It refuses blocked actions and changed destinations.
// A failure removes files created by this run and restores replaced files from
// their rollback copies.
func ApplyRestorePlan(plan RestorePlan, opts RestoreApplyOptions) (RestoreApplyResult, error) {
	if len(plan.Actions) == 0 {
		return RestoreApplyResult{}, errors.New("the restore plan has no supported files to create")
	}
	if plan.files == nil {
		return RestoreApplyResult{}, errors.New("the restore plan has no selected content; rebuild it from the snapshot")
	}
	opts.Conflict = strings.ToLower(strings.TrimSpace(opts.Conflict))
	if opts.Conflict != "" && opts.Conflict != "skip" && opts.Conflict != "replace" {
		return RestoreApplyResult{}, fmt.Errorf("unknown conflict policy %q; use skip or replace", opts.Conflict)
	}
	for _, action := range plan.Actions {
		switch action.State {
		case "blocked":
			return RestoreApplyResult{}, fmt.Errorf("the restore plan has a blocked action for %q; resolve it before applying", action.Destination)
		case "conflict":
			if opts.Conflict == "" {
				return RestoreApplyResult{}, fmt.Errorf("the restore plan has a conflict for %q; give --on-conflict skip or replace", action.Destination)
			}
			if opts.Conflict == "replace" {
				info, err := os.Lstat(action.Destination)
				if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
					return RestoreApplyResult{}, fmt.Errorf("conflicting destination %q must be a regular file before it can be replaced", action.Destination)
				}
			} else if opts.Conflict == "skip" {
				if _, err := os.Lstat(action.Destination); err != nil {
					return RestoreApplyResult{}, fmt.Errorf("conflicting destination %q changed after the plan; preview again", action.Destination)
				}
			}
		case "create":
			if _, err := os.Lstat(action.Destination); err == nil {
				return RestoreApplyResult{}, fmt.Errorf("destination %q changed after the plan; preview again", action.Destination)
			} else if !os.IsNotExist(err) {
				return RestoreApplyResult{}, fmt.Errorf("inspect destination %q: %w", action.Destination, err)
			}
		default:
			return RestoreApplyResult{}, fmt.Errorf("the restore plan has an unknown %s action for %q", action.State, action.Destination)
		}
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	journalDir := opts.JournalDir
	if journalDir == "" {
		var err error
		journalDir, err = SnapshotDir()
		if err != nil {
			return RestoreApplyResult{}, err
		}
	}
	if err := prepareSnapshotDir(journalDir); err != nil {
		return RestoreApplyResult{}, err
	}
	journalPath, err := newRestoreJournalPath(journalDir, plan.SnapshotID, now)
	if err != nil {
		return RestoreApplyResult{}, err
	}
	journal := restoreJournal{
		SchemaVersion: restoreJournalSchemaVersion, SnapshotID: plan.SnapshotID,
		PlanFingerprint: plan.Fingerprint, State: "running", StartedAt: now,
		Actions: make([]restoreJournalAction, len(plan.Actions)),
	}
	for i, action := range plan.Actions {
		journal.Actions[i] = restoreJournalAction{Destination: action.Destination, State: "pending"}
	}
	if err := writeRestoreJournal(journalPath, journal); err != nil {
		return RestoreApplyResult{}, err
	}
	created := []string{}
	replaced := []string{}
	skipped := []string{}
	backups := []string{}
	fail := func(index int, cause error) (RestoreApplyResult, error) {
		if index >= 0 {
			journal.Actions[index].State = "failed"
			journal.Actions[index].Error = cause.Error()
		}
		var rollbackErr error
		for i := len(journal.Actions) - 1; i >= 0; i-- {
			switch journal.Actions[i].State {
			case "created":
				if err := os.Remove(journal.Actions[i].Destination); err != nil && !os.IsNotExist(err) {
					rollbackErr = fmt.Errorf("remove %q: %w", journal.Actions[i].Destination, err)
				}
				journal.Actions[i].State = "rolled_back"
			case "replaced":
				backup := journal.Actions[i].Backup
				if backup == "" {
					rollbackErr = fmt.Errorf("no rollback copy for %q", journal.Actions[i].Destination)
					journal.Actions[i].State = "rollback_failed"
					continue
				}
				data, err := os.ReadFile(backup)
				if err == nil {
					err = writeRestoreFile(journal.Actions[i].Destination, data, true)
				}
				if err != nil {
					rollbackErr = fmt.Errorf("restore %q from its rollback copy: %w", journal.Actions[i].Destination, err)
					journal.Actions[i].State = "rollback_failed"
				} else {
					journal.Actions[i].State = "rolled_back"
				}
			}
		}
		finished := time.Now().UTC()
		journal.State, journal.FinishedAt = "failed", &finished
		journalErr := writeRestoreJournal(journalPath, journal)
		if journalErr != nil {
			return RestoreApplyResult{}, fmt.Errorf("%w; writing the restore journal also failed: %v", cause, journalErr)
		}
		if rollbackErr != nil {
			return RestoreApplyResult{}, fmt.Errorf("%w; rollback also failed: %v", cause, rollbackErr)
		}
		return RestoreApplyResult{}, cause
	}
	for i, action := range plan.Actions {
		if action.State == "conflict" && opts.Conflict == "skip" {
			journal.Actions[i].State = "skipped"
			skipped = append(skipped, action.Destination)
			if err := writeRestoreJournal(journalPath, journal); err != nil {
				return fail(i, fmt.Errorf("update the restore journal: %w", err))
			}
			continue
		}
		file, ok := plan.files[action.Destination]
		if !ok {
			return fail(i, fmt.Errorf("the plan has no selected content for %q", action.Destination))
		}
		data, err := restoreContent(file)
		if err != nil {
			return fail(i, err)
		}
		if action.State == "conflict" && opts.Conflict == "replace" {
			backup := restoreBackupPath(journalPath, i)
			if err := copyRestoreBackup(action.Destination, backup); err != nil {
				return fail(i, err)
			}
			journal.Actions[i].Backup = backup
			journal.Actions[i].BackupSHA256 = fileSHA256(backup)
			if journal.Actions[i].BackupSHA256 == "" {
				return fail(i, fmt.Errorf("read the rollback copy for %q", action.Destination))
			}
			if err := writeRestoreJournal(journalPath, journal); err != nil {
				return fail(i, fmt.Errorf("record the rollback copy: %w", err))
			}
		}
		if opts.beforeWrite != nil {
			if err := opts.beforeWrite(action.Destination); err != nil {
				return fail(i, err)
			}
		}
		if err := writeRestoreFile(action.Destination, data, action.State == "conflict"); err != nil {
			return fail(i, err)
		}
		if err := verifyRestoreFile(action.Destination, file); err != nil {
			_ = os.Remove(action.Destination)
			return fail(i, err)
		}
		journal.Actions[i].AppliedSHA256 = sha256Bytes(data)
		switch action.State {
		case "create":
			created = append(created, action.Destination)
			journal.Actions[i].State = "created"
		case "conflict":
			replaced = append(replaced, action.Destination)
			backups = append(backups, journal.Actions[i].Backup)
			journal.Actions[i].State = "replaced"
		}
		if err := writeRestoreJournal(journalPath, journal); err != nil {
			return fail(i, fmt.Errorf("update the restore journal: %w", err))
		}
	}
	finished := time.Now().UTC()
	journal.State, journal.FinishedAt = "completed", &finished
	if err := writeRestoreJournal(journalPath, journal); err != nil {
		return fail(-1, fmt.Errorf("finish the restore journal: %w", err))
	}
	return RestoreApplyResult{
		SchemaVersion: restoreApplySchemaVersion, SnapshotID: plan.SnapshotID,
		PlanFingerprint: plan.Fingerprint, Created: created, Replaced: replaced,
		Skipped: skipped, Backups: backups, JournalPath: journalPath, CompletedAt: finished,
	}, nil
}

func restoreBackupPath(journalPath string, index int) string {
	return filepath.Join(filepath.Dir(journalPath), strings.TrimSuffix(filepath.Base(journalPath), ".json")+"-"+fmt.Sprint(index)+".bak")
}

func copyRestoreBackup(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect conflicting destination: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("conflicting destination %q must be a regular file", source)
	}
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open conflicting destination: %w", err)
	}
	defer src.Close()
	opened, err := src.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return fmt.Errorf("conflicting destination changed during backup")
	}
	f, err := os.CreateTemp(filepath.Dir(destination), ".reet-restore-backup-*")
	if err != nil {
		return fmt.Errorf("create rollback copy: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	written, copyErr := io.Copy(f, io.LimitReader(src, sealed.MaxObjectBytes+1))
	if copyErr == nil && written > sealed.MaxObjectBytes {
		copyErr = errors.New("the conflicting file is larger than this build can back up")
	}
	if copyErr == nil {
		copyErr = f.Chmod(0o600)
	}
	if copyErr == nil {
		copyErr = f.Sync()
	}
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("write rollback copy: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("write rollback copy: %w", closeErr)
	}
	if err := os.Rename(name, destination); err != nil {
		return fmt.Errorf("finish rollback copy: %w", err)
	}
	return nil
}

func restoreContent(file restoreFile) ([]byte, error) {
	content := map[string]any{}
	for key, value := range file.settings {
		content[key] = value
	}
	if len(file.mcp) > 0 {
		content[file.container] = file.mcp
	}
	if len(content) == 0 {
		return nil, errors.New("a restore action has no selected content")
	}
	switch file.format {
	case "toml":
		data, err := toml.Marshal(content)
		if err != nil {
			return nil, fmt.Errorf("encode TOML restore content: %w", err)
		}
		return append(data, '\n'), nil
	case "json", "jsonc":
		data, err := json.MarshalIndent(content, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode JSON restore content: %w", err)
		}
		return append(data, '\n'), nil
	default:
		return nil, fmt.Errorf("restore writing for %q is not implemented", file.format)
	}
}

func writeRestoreFile(path string, data []byte, overwrite bool) error {
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create restore directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect restore directory: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("the restore directory %q must be a real directory, not a symlink or file", dir)
	}
	f, err := os.CreateTemp(dir, ".reet-restore-*")
	if err != nil {
		return fmt.Errorf("create restored file: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err == nil {
		err = f.Chmod(0o600)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("write restored file: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("write restored file: %w", closeErr)
	}
	if !overwrite {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("destination %q changed after the plan; preview again", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect destination %q: %w", path, err)
		}
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("finish restored file: %w", err)
	}
	return nil
}

func verifyRestoreFile(path string, file restoreFile) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("verify restored file %q: not a regular file", path)
	}
	data, issue := readConfig(path, info)
	if issue != "" {
		return fmt.Errorf("verify restored file %q: %s", path, issue)
	}
	object, err := parseSource(data, file.format)
	if err != nil {
		return fmt.Errorf("verify restored file %q: invalid %s", path, file.format)
	}
	for key, value := range file.settings {
		if !reflect.DeepEqual(object[key], value) {
			return fmt.Errorf("verify restored file %q: setting %q does not match", path, key)
		}
	}
	if len(file.mcp) > 0 {
		container, ok := object[file.container].(map[string]any)
		if !ok {
			return fmt.Errorf("verify restored file %q: MCP container missing", path)
		}
		for name, declaration := range file.mcp {
			if !reflect.DeepEqual(container[name], declaration) {
				return fmt.Errorf("verify restored file %q: MCP declaration %q does not match", path, name)
			}
		}
	}
	return nil
}

func newRestoreJournalPath(dir, snapshotID string, now time.Time) (string, error) {
	safe := strings.Map(func(r rune) rune {
		if r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, snapshotID)
	if safe == "" {
		safe = "snapshot"
	}
	nonce := make([]byte, 4)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate restore journal name: %w", err)
	}
	return filepath.Join(dir, "restore-"+safe+"-"+now.Format("20060102T150405Z")+"-"+hex.EncodeToString(nonce)+".json"), nil
}

func writeRestoreJournal(path string, journal restoreJournal) error {
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("encode restore journal: %w", err)
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".reet-restore-journal-*")
	if err != nil {
		return fmt.Errorf("create restore journal: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err == nil {
		err = f.Chmod(0o600)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("write restore journal: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("write restore journal: %w", closeErr)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("finish restore journal: %w", err)
	}
	return nil
}

func renderRestoreApply(out io.Writer, result RestoreApplyResult) error {
	if _, err := fmt.Fprintf(out, "Restored %d files from setup snapshot %s.\n", len(result.Created), result.SnapshotID); err != nil {
		return err
	}
	for _, path := range result.Created {
		if _, err := fmt.Fprintf(out, "  %q\n", path); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "Journal: %q\nNo runtime verification has been run.\n", result.JournalPath); err != nil {
		return err
	}
	return nil
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fileSHA256(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return sha256Bytes(data)
}
