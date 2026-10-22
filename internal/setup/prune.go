package setup

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const setupPruneSchemaVersion = 1

type SetupPruneProtected struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type SetupPruneResult struct {
	SchemaVersion int                   `json:"schema_version"`
	Dir           string                `json:"dir"`
	Keep          int                   `json:"keep"`
	Applied       bool                  `json:"applied"`
	Total         int                   `json:"total"`
	Kept          int                   `json:"kept"`
	Protected     []SetupPruneProtected `json:"protected,omitempty"`
	WouldRemove   []string              `json:"would_remove,omitempty"`
	Removed       []string              `json:"removed,omitempty"`
	Limitations   []string              `json:"limitations"`
}

// PruneSetupSnapshots plans or removes local snapshots beyond keep. Snapshots
// referenced by any profile revision or restore journal are never removed.
func PruneSetupSnapshots(dir string, keep int, apply bool) (SetupPruneResult, error) {
	if keep < 1 {
		return SetupPruneResult{}, errors.New("--keep must be at least 1")
	}
	list, err := ListSetupSnapshots(dir)
	if err != nil {
		return SetupPruneResult{}, err
	}
	result := SetupPruneResult{
		SchemaVersion: setupPruneSchemaVersion, Dir: list.Dir, Keep: keep, Applied: apply,
		Total: len(list.Snapshots), Protected: []SetupPruneProtected{},
		WouldRemove: []string{}, Removed: []string{},
		Limitations: []string{
			"Only local .reetsetup files are pruned; journals, profiles, rollback copies and remote objects are left in place.",
			"Snapshots referenced by any profile revision or restore journal are protected.",
		},
	}
	protected := profileSnapshotReferences(dir)
	journals, err := ListRestoreJournals(dir)
	if err != nil {
		return SetupPruneResult{}, err
	}
	for _, journal := range journals.Journals {
		protected[journal.SnapshotID] = "restore journal " + journal.State
	}
	for i, snapshot := range list.Snapshots {
		if i < keep {
			result.Kept++
			continue
		}
		if reason, ok := protected[snapshot.ID]; ok {
			result.Protected = append(result.Protected, SetupPruneProtected{ID: snapshot.ID, Reason: reason})
			continue
		}
		if !apply {
			result.WouldRemove = append(result.WouldRemove, snapshot.Path)
			continue
		}
		if err := os.Remove(snapshot.Path); err != nil {
			return SetupPruneResult{}, fmt.Errorf("remove %q: %w", snapshot.Path, err)
		}
		result.Removed = append(result.Removed, snapshot.Path)
	}
	return result, nil
}

func renderSetupPrune(out io.Writer, result SetupPruneResult) error {
	mode := "Would remove"
	if result.Applied {
		mode = "Removed"
	}
	if _, err := fmt.Fprintf(out, "Setup snapshot prune (%s)\n  %d total, keeping %d new\n", result.Dir, result.Total, result.Keep); err != nil {
		return err
	}
	for _, path := range result.WouldRemove {
		if _, err := fmt.Fprintf(out, "  %s %q\n", mode, path); err != nil {
			return err
		}
	}
	for _, path := range result.Removed {
		if _, err := fmt.Fprintf(out, "  removed %q\n", path); err != nil {
			return err
		}
	}
	for _, protected := range result.Protected {
		if _, err := fmt.Fprintf(out, "  protected %s: %s\n", protected.ID, protected.Reason); err != nil {
			return err
		}
	}
	if len(result.WouldRemove) == 0 && len(result.Removed) == 0 {
		if _, err := fmt.Fprintln(out, "Nothing to prune."); err != nil {
			return err
		}
	}
	for _, note := range result.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}
