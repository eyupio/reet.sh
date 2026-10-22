package setup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const restoreJournalListSchemaVersion = 1

type RestoreJournalSummary struct {
	Path        string     `json:"path"`
	SnapshotID  string     `json:"snapshot_id"`
	State       string     `json:"state"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Actions     int        `json:"actions"`
	Created     int        `json:"created,omitempty"`
	Replaced    int        `json:"replaced,omitempty"`
	Skipped     int        `json:"skipped,omitempty"`
	Fingerprint string     `json:"fingerprint"`
}

type RestoreJournalSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type RestoreJournalList struct {
	SchemaVersion int                     `json:"schema_version"`
	Dir           string                  `json:"dir"`
	Journals      []RestoreJournalSummary `json:"journals"`
	Skipped       []RestoreJournalSkip    `json:"skipped,omitempty"`
	Limitations   []string                `json:"limitations"`
}

// ListRestoreJournals lists local apply and rollback journals without opening
// snapshots or destinations.
func ListRestoreJournals(dir string) (RestoreJournalList, error) {
	if dir == "" {
		var err error
		dir, err = SnapshotDir()
		if err != nil {
			return RestoreJournalList{}, err
		}
	}
	list := RestoreJournalList{
		SchemaVersion: restoreJournalListSchemaVersion, Dir: dir,
		Journals: []RestoreJournalSummary{}, Skipped: []RestoreJournalSkip{},
		Limitations: []string{
			"Listing reads local journals only; it does not verify destinations or snapshots.",
			"A completed journal is not proof that the restored setup works.",
		},
	}
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return list, nil
	}
	if err != nil {
		return RestoreJournalList{}, fmt.Errorf("inspect journal directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return RestoreJournalList{}, errors.New("the journal directory must be a real directory, not a symlink or file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return RestoreJournalList{}, fmt.Errorf("read journal directory: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "restore-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(dir, name)
		journal, _, err := loadRestoreJournal(path)
		if err != nil {
			list.Skipped = append(list.Skipped, RestoreJournalSkip{Path: path, Reason: "invalid_journal"})
			continue
		}
		summary := RestoreJournalSummary{
			Path: path, SnapshotID: journal.SnapshotID, State: journal.State,
			StartedAt: journal.StartedAt, FinishedAt: journal.FinishedAt,
			Actions: len(journal.Actions), Fingerprint: journal.PlanFingerprint,
		}
		for _, action := range journal.Actions {
			switch action.State {
			case "created":
				summary.Created++
			case "replaced":
				summary.Replaced++
			case "skipped":
				summary.Skipped++
			}
		}
		list.Journals = append(list.Journals, summary)
	}
	sort.Slice(list.Journals, func(i, j int) bool {
		if list.Journals[i].StartedAt.Equal(list.Journals[j].StartedAt) {
			return list.Journals[i].Path > list.Journals[j].Path
		}
		return list.Journals[i].StartedAt.After(list.Journals[j].StartedAt)
	})
	return list, nil
}

func latestCompletedJournal(dir, snapshotID string) (string, error) {
	list, err := ListRestoreJournals(dir)
	if err != nil {
		return "", err
	}
	for _, journal := range list.Journals {
		if journal.SnapshotID == snapshotID && journal.State == "completed" {
			return journal.Path, nil
		}
	}
	return "", errors.New("no completed restore journal was found for this snapshot")
}

func renderRestoreJournalList(out io.Writer, list RestoreJournalList) error {
	if _, err := fmt.Fprintf(out, "Setup restore journals in %q\n", list.Dir); err != nil {
		return err
	}
	if len(list.Journals) == 0 {
		if _, err := fmt.Fprintln(out, "No restore journals found."); err != nil {
			return err
		}
	}
	for _, journal := range list.Journals {
		if _, err := fmt.Fprintf(out, "  %s  %s  %s  actions=%d created=%d replaced=%d skipped=%d\n",
			journal.State, journal.StartedAt.Format(time.RFC3339), journal.SnapshotID,
			journal.Actions, journal.Created, journal.Replaced, journal.Skipped); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "    %q\n", journal.Path); err != nil {
			return err
		}
	}
	for _, skipped := range list.Skipped {
		if _, err := fmt.Fprintf(out, "  skipped %q: %s\n", skipped.Path, skipped.Reason); err != nil {
			return err
		}
	}
	for _, note := range list.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}
