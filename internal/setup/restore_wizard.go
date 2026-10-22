package setup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

// runRestoreWizard guides a restore from a local profile. Execution still
// happens locally after an explicit confirmation.
func runRestoreWizard(out io.Writer, dir, project, machine string) error {
	var err error
	project, err = setupProject(project)
	if err != nil {
		return err
	}
	profiles, err := ListSetupProfiles(dir)
	if err != nil {
		return err
	}
	if len(profiles.Profiles) == 0 {
		return errors.New("no local setup profiles found; run reet setup profile first")
	}
	chosen := profiles.Profiles[0]
	if len(profiles.Profiles) > 1 {
		if _, err := fmt.Fprintln(out, "Choose a profile:"); err != nil {
			return err
		}
		for i, profile := range profiles.Profiles {
			if _, err := fmt.Fprintf(out, "  %d. %s (revision %d)\n", i+1, profile.Name, profile.Revision); err != nil {
				return err
			}
		}
		answer, err := wizardPrompt(out, "Profile number", "1")
		if err != nil {
			return err
		}
		index, err := strconv.Atoi(answer)
		if err != nil || index < 1 || index > len(profiles.Profiles) {
			return errors.New("choose a number from the list")
		}
		chosen = profiles.Profiles[index-1]
	}
	_, revision, err := LoadProfile(chosen.Name, dir, 0)
	if err != nil {
		return err
	}
	snapshotPath, err := resolveSetupSnapshotPath(revision.SnapshotID, dir)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(snapshotPath); os.IsNotExist(err) {
		fetch := false
		if _, _, s3Err := SetupS3FromEnv(); s3Err == nil {
			fetch, err = wizardYesNo(out, "Snapshot not found locally. Fetch it from S3 now?", true)
			if err != nil {
				return err
			}
		}
		if !fetch {
			return fmt.Errorf("snapshot %s is not local; fetch it first with reet setup fetch --s3 --profile %s", revision.SnapshotID, chosen.Name)
		}
		if _, err := FetchSetupSnapshot(revision.SnapshotID, dir); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	passphrase, err := readSetupPassphrase("", false, out)
	if err != nil {
		return err
	}
	snapshot, manifest, err := loadSetupSnapshot(revision.SnapshotID, passphrase, dir)
	if err != nil {
		return err
	}
	snapshot = ApplyProfileOverrides(snapshot, revision.Overrides, machine, project)
	home, err := setupHome()
	if err != nil {
		return err
	}
	destination, err := Discover(home, project)
	if err != nil {
		return err
	}
	plan := BuildRestorePlan(snapshot, manifest, destination)
	creates, conflicts, blocked := 0, 0, 0
	for _, action := range plan.Actions {
		switch action.State {
		case "create":
			creates++
		case "conflict":
			conflicts++
		case "blocked":
			blocked++
		}
	}
	if _, err := fmt.Fprintf(out, "Plan: %d create, %d conflict, %d blocked; %d unsupported/omitted.\n", creates, conflicts, blocked, len(plan.Unsupported)); err != nil {
		return err
	}
	if blocked > 0 {
		return errors.New("the plan has blocked destinations; run reet setup restore --profile " + chosen.Name + " to inspect them")
	}
	apply, err := wizardYesNo(out, "Apply this restore plan now?", false)
	if err != nil || !apply {
		if err == nil {
			_, err = fmt.Fprintln(out, "Plan not applied.")
		}
		return err
	}
	conflictPolicy := ""
	if conflicts > 0 {
		conflictPolicy, err = wizardPrompt(out, "Conflicts: skip, replace or cancel", "cancel")
		if err != nil {
			return err
		}
		switch conflictPolicy {
		case "skip", "replace":
		case "cancel", "":
			_, err = fmt.Fprintln(out, "Plan not applied.")
			return err
		default:
			return errors.New("choose skip, replace or cancel")
		}
	}
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: dir, Conflict: conflictPolicy})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Restored %d files. Journal: %s\n", len(result.Created)+len(result.Replaced), result.JournalPath); err != nil {
		return err
	}
	updated, err := Discover(home, project)
	if err != nil {
		return err
	}
	verification := VerifySetupSnapshot(snapshot, manifest, updated)
	if _, err := fmt.Fprintf(out, "Verified: %d; changed: %d; missing: %d; blocked: %d.\n", verification.Summary.Verified, verification.Summary.Changed, verification.Summary.Missing, verification.Summary.Blocked); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Completed %s. Runtime health is not checked.\n", time.Now().UTC().Format(time.RFC3339))
	return err
}
