package setup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

func interactive() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

func wizardPrompt(out io.Writer, label, fallback string) (string, error) {
	if fallback != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, fallback)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	var b strings.Builder
	one := make([]byte, 1)
	for b.Len() <= 4096 {
		n, err := os.Stdin.Read(one)
		if n > 0 {
			if one[0] == '\n' {
				break
			}
			b.WriteByte(one[0])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", errors.New("input ended")
			}
			return "", err
		}
	}
	value := strings.TrimSpace(b.String())
	if value == "" {
		value = fallback
	}
	return value, nil
}

func wizardYesNo(out io.Writer, label string, fallback bool) (bool, error) {
	def := "n"
	if fallback {
		def = "y"
	}
	answer, err := wizardPrompt(out, label+" (y/n)", def)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, errors.New("answer yes or no")
	}
}

func runProfileWizard(dir string, out io.Writer) error {
	snapshots, err := ListSetupSnapshots(dir)
	if err != nil {
		return err
	}
	snapshotID := ""
	passphrase := ""
	if len(snapshots.Snapshots) == 0 {
		create, err := wizardYesNo(out, "No setup snapshot found. Create one now?", true)
		if err != nil {
			return err
		}
		if !create {
			return errors.New("no snapshot to save; run reet setup backup when ready")
		}
		passphrase, err = promptSetupPassphrase(true, out)
		if err != nil {
			return err
		}
		home, err := setupHome()
		if err != nil {
			return err
		}
		report, err := Discover(home, "")
		if err != nil {
			return err
		}
		selection, err := BuildSnapshotSelection(report)
		if err != nil {
			return err
		}
		result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: passphrase})
		if err != nil {
			return err
		}
		snapshotID = result.ID
		if _, err := fmt.Fprintf(out, "Took setup snapshot %s.\n", result.ID); err != nil {
			return err
		}
	} else {
		chosen := snapshots.Snapshots[0]
		if len(snapshots.Snapshots) > 1 {
			if _, err := fmt.Fprintln(out, "Choose a snapshot:"); err != nil {
				return err
			}
			for i, snapshot := range snapshots.Snapshots {
				if _, err := fmt.Fprintf(out, "  %d. %s (%s)\n", i+1, snapshot.ID, snapshot.TakenAt.Format(time.RFC3339)); err != nil {
					return err
				}
			}
			answer, err := wizardPrompt(out, "Snapshot number", "1")
			if err != nil {
				return err
			}
			index, err := strconv.Atoi(answer)
			if err != nil || index < 1 || index > len(snapshots.Snapshots) {
				return errors.New("choose a number from the list")
			}
			chosen = snapshots.Snapshots[index-1]
		}
		snapshotID = chosen.ID
		preview, err := wizardYesNo(out, "Open the snapshot to record preview and software evidence?", false)
		if err != nil {
			return err
		}
		if preview {
			passphrase, err = readSetupPassphrase("", false, out)
			if err != nil {
				return err
			}
		}
	}
	name, err := wizardPrompt(out, "Profile name", "work")
	if err != nil {
		return err
	}
	if !validProfileName(name) {
		return errors.New("profile names use 1-64 letters, digits, '-' or '_'")
	}
	profile, revision, err := SaveSetupProfileWithOptions(name, passphrase, snapshotID, dir, time.Now().UTC(), SaveSetupProfileOptions{})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Saved profile %s revision %d for snapshot %s.\n", profile.Name, revision.Revision, revision.SnapshotID); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Preview: %s. Next: reet setup profile push %s (optional), or reet setup restore --profile %s.\n", map[bool]string{true: "recorded", false: "header only"}[revision.Preview.SchemaVersion != 0], profile.Name, profile.Name)
	return err
}
