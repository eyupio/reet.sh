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
type restoreWizardOptions struct {
	PasswordFile                       string
	Revision                           int
	ProbeVersions, InspectInstallation bool
}

func runRestoreWizard(out io.Writer, dir, project, machine string, opts restoreWizardOptions) error {
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
		return errors.New("no local setup profiles found; pull one with reet setup profile pull NAME --server URL, or create one with reet setup profile")
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
	_, revision, err := LoadProfile(chosen.Name, dir, opts.Revision)
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
	passphrase, err := readSetupPassphrase(opts.PasswordFile, false, os.Stderr)
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
	if opts.InspectInstallation {
		InspectInstallations(&destination)
	}
	if opts.ProbeVersions {
		if err := ProbeVersions(&destination); err != nil {
			return err
		}
	}
	plan := BuildRestorePlan(snapshot, manifest, destination)
	if _, err := fmt.Fprintln(out, "Stop the destination agents before restoring settings or memory. Replace writes selected snapshot content and removes destination-only fields in that file; a rollback copy is retained. File verification does not check runtime health."); err != nil {
		return err
	}
	if err := renderRestorePlan(out, plan); err != nil {
		return err
	}
	creates, merges, conflicts, blocked := 0, 0, 0, 0
	for _, action := range plan.Actions {
		switch action.State {
		case "create":
			creates++
		case "merge":
			merges++
		case "conflict":
			conflicts++
		case "blocked":
			blocked++
		}
	}
	if _, err := fmt.Fprintf(out, "Plan: %d create, %d merge, %d conflict, %d blocked; %d unsupported/omitted.\n", creates, merges, conflicts, blocked, len(plan.Unsupported)); err != nil {
		return err
	}
	if blocked > 0 {
		return errors.New("the plan has blocked destinations; run reet setup restore --profile " + chosen.Name + " to inspect them")
	}
	choices := map[string]string{}
	for _, action := range plan.Actions {
		if action.State != "conflict" {
			continue
		}
		answer, err := wizardPrompt(out, "Conflict at "+action.Destination+": skip, replace or cancel", "skip")
		if err != nil {
			return err
		}
		switch answer {
		case "skip", "replace":
			choices[action.Destination] = answer
		case "cancel":
			_, err = fmt.Fprintln(out, "Plan not applied.")
			return err
		default:
			return errors.New("choose skip, replace or cancel; nothing was applied")
		}
	}
	apply, err := wizardYesNo(out, "Apply the reviewed plan and conflict choices now?", false)
	if err != nil {
		return err
	}
	if !apply {
		_, err = fmt.Fprintln(out, "Plan not applied.")
		return err
	}
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: dir, Conflicts: choices})
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
	if err := setupVerificationError(verification); err != nil {
		return fmt.Errorf("files applied; %w. Journal: %s", err, result.JournalPath)
	}
	_, err = fmt.Fprintf(out, "File verification passed %s. Runtime health is not checked.\n", time.Now().UTC().Format(time.RFC3339))
	return err
}

func setupVerificationError(result SetupVerifyResult) error {
	s := result.Summary
	if s.Changed+s.Missing+s.Blocked > 0 {
		return fmt.Errorf("verification incomplete: %d changed, %d missing, %d blocked", s.Changed, s.Missing, s.Blocked)
	}
	return nil
}
