package setup

import (
	"crypto/rand"
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

	"github.com/pelletier/go-toml/v2"
)

const (
	restoreApplySchemaVersion   = 1
	restoreJournalSchemaVersion = 1
)

type RestoreApplyOptions struct {
	JournalDir  string
	Now         func() time.Time
	beforeWrite func(string) error
}

type RestoreApplyResult struct {
	SchemaVersion   int       `json:"schema_version"`
	SnapshotID      string    `json:"snapshot_id"`
	PlanFingerprint string    `json:"plan_fingerprint"`
	Created         []string  `json:"created"`
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
	Destination string `json:"destination"`
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
}

// ApplyRestorePlan creates only the files a plan marked create. It refuses
// conflicts, blocked actions and changed destinations before writing. A
// failure removes files created by this run.
func ApplyRestorePlan(plan RestorePlan, opts RestoreApplyOptions) (RestoreApplyResult, error) {
	if len(plan.Actions) == 0 {
		return RestoreApplyResult{}, errors.New("the restore plan has no supported files to create")
	}
	if plan.files == nil {
		return RestoreApplyResult{}, errors.New("the restore plan has no selected content; rebuild it from the snapshot")
	}
	for _, action := range plan.Actions {
		if action.State != "create" {
			return RestoreApplyResult{}, fmt.Errorf("the restore plan has a %s action for %q; conflict choices are not implemented", action.State, action.Destination)
		}
	}
	for _, action := range plan.Actions {
		if _, err := os.Lstat(action.Destination); err == nil {
			return RestoreApplyResult{}, fmt.Errorf("destination %q changed after the plan; preview again", action.Destination)
		} else if !os.IsNotExist(err) {
			return RestoreApplyResult{}, fmt.Errorf("inspect destination %q: %w", action.Destination, err)
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
	fail := func(index int, cause error) (RestoreApplyResult, error) {
		if index >= 0 {
			journal.Actions[index].State = "failed"
			journal.Actions[index].Error = cause.Error()
		}
		for i := range journal.Actions {
			if journal.Actions[i].State == "created" {
				journal.Actions[i].State = "rolled_back"
			}
		}
		for i := len(created) - 1; i >= 0; i-- {
			_ = os.Remove(created[i])
		}
		finished := time.Now().UTC()
		journal.State, journal.FinishedAt = "failed", &finished
		journalErr := writeRestoreJournal(journalPath, journal)
		if journalErr != nil {
			return RestoreApplyResult{}, fmt.Errorf("%w; writing the restore journal also failed: %v", cause, journalErr)
		}
		return RestoreApplyResult{}, cause
	}
	for i, action := range plan.Actions {
		file, ok := plan.files[action.Destination]
		if !ok {
			return fail(i, fmt.Errorf("the plan has no selected content for %q", action.Destination))
		}
		data, err := restoreContent(file)
		if err != nil {
			return fail(i, err)
		}
		if opts.beforeWrite != nil {
			if err := opts.beforeWrite(action.Destination); err != nil {
				return fail(i, err)
			}
		}
		if err := writeRestoreFile(action.Destination, data); err != nil {
			return fail(i, err)
		}
		if err := verifyRestoreFile(action.Destination, file); err != nil {
			_ = os.Remove(action.Destination)
			return fail(i, err)
		}
		created = append(created, action.Destination)
		journal.Actions[i].State = "created"
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
		PlanFingerprint: plan.Fingerprint, Created: created, JournalPath: journalPath, CompletedAt: finished,
	}, nil
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

func writeRestoreFile(path string, data []byte) error {
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
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("destination %q changed after the plan; preview again", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect destination %q: %w", path, err)
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
