package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/eyupio/reet.sh/internal/filelock"
	"github.com/eyupio/reet.sh/internal/store"
)

// Version 1 is deliberately not advertised by the controller until Stage 4's
// receipts, recovery and stale-install fences are complete. An old/unknown
// capability always preserves the existing additive behavior.
const managedRemovalVersion = 1

func libraryRemovalCheck(binding libraryBinding, generation string) store.LibraryDeploymentCheck {
	key := sha256.Sum256([]byte(binding.Path))
	return store.LibraryDeploymentCheck{TargetID: binding.Deployment.TargetID, LibraryBase: store.LibraryBase{BindingID: hex.EncodeToString(key[:]), ItemID: binding.ItemID, RevisionID: binding.Deployment.Revision}, Generation: generation}
}

func readLibraryRemovalIntent(ctx context.Context, c agentConfig, check store.LibraryDeploymentCheck, version string) (store.LibraryDeploymentIntent, error) {
	var intent store.LibraryDeploymentIntent
	if err := agentRequestContext(ctx, c, "/api/v2/library/deployment-intent", check, &intent, version); err != nil {
		return intent, err
	}
	if len(intent.Generation) != 64 || (check.Generation != "" && intent.Generation != check.Generation) {
		return intent, errors.New("invalid deployment generation")
	}
	if _, err := hex.DecodeString(intent.Generation); err != nil {
		return intent, errors.New("invalid deployment generation")
	}
	if intent.Action != "maintain" && intent.Action != "blocked" && intent.Action != "remove" {
		return intent, errors.New("unknown deployment intent")
	}
	if intent.Action == "remove" && intent.Reason != "inactive" && intent.Reason != "deselected" && intent.Reason != "library_deleted" {
		return intent, errors.New("unknown removal reason")
	}
	return intent, nil
}

func managedLibraryPath(binding libraryBinding, targetID, path string) bool {
	return !binding.Source && binding.Deployment != nil && binding.Deployment.Verified && binding.Deployment.TargetID == targetID &&
		filepath.IsAbs(binding.Path) && filepath.Dir(binding.Path) == filepath.Clean(path) && safeSlug.MatchString(filepath.Base(binding.Path))
}

// Only journal-proven entries with explicit authoritative removal intent join
// the existing executor's additive plan. Unknown extras are never candidates.
func planLibraryRemovals(ctx context.Context, config string, c agentConfig, target store.SyncTarget, path, version string) ([]SyncAction, map[string]string, error) {
	problems := map[string]string{}
	if !target.Canonical {
		return nil, problems, nil
	}
	lock, err := filelock.Acquire(config + ".library.lock")
	if err != nil {
		return nil, problems, err
	}
	defer lock.Close()
	state, err := loadLibraryState(config, c)
	if err != nil {
		return nil, problems, err
	}
	if state.ManagedRemovals != managedRemovalVersion {
		return nil, problems, nil
	}
	if err := libraryApplyConsent(config, c); err != nil {
		return nil, problems, err
	}
	var actions []SyncAction
	for _, binding := range state.Bindings {
		if !managedLibraryPath(binding, target.Target.ID, path) {
			continue
		}
		slug := filepath.Base(binding.Path)
		if err := safeLibraryParent(binding.Path); err != nil {
			problems[slug] = "The managed directory cannot be verified; its local files are retained."
			continue
		}
		digest, err := privateSkillDigest(binding.Path)
		if os.IsNotExist(err) {
			continue // absence is never central deletion intent
		}
		if err != nil {
			problems[slug] = "The managed content cannot be read completely; its local files are retained."
			continue
		}
		if binding.Deployment.Removal != nil {
			// Do not repeat or overwrite an operation whose outcome needs
			// recovery. Its journal names the exact recoverable backup.
			problems[slug] = "A managed removal is awaiting recovery; its files are retained."
			continue
		}
		check := libraryRemovalCheck(binding, "")
		intent, err := readLibraryRemovalIntent(ctx, c, check, version)
		if err != nil || intent.Action == "blocked" {
			problems[slug] = "Current library intent could not be verified; its local files are retained."
			continue
		}
		if intent.Action != "remove" {
			continue
		}
		for _, desired := range target.Desired {
			if desired.ItemID == binding.ItemID {
				return nil, problems, errTargetChanged
			}
		}
		check.Generation = intent.Generation
		actions = append(actions, SyncAction{BeforeDigest: digest, Skill: store.SyncSkill{ItemID: binding.ItemID, Slug: slug}, RemovalIntent: &check})
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].Skill.Slug < actions[j].Skill.Slug })
	return actions, problems, nil
}

// This runs inside the existing target executor and target lock. The private
// journal lock also excludes imports/capture while proving ownership and moving
// the entry. The shared backup path performs the only filesystem mutation.
func removeLibraryDeployment(ctx context.Context, config string, c agentConfig, action SyncAction, path, version string, out io.Writer, onBackup func(string)) error {
	if action.Add || action.RemovalIntent == nil || len(action.RemovalIntent.Generation) != 64 || !safeSlug.MatchString(action.Skill.Slug) {
		return errors.New("invalid managed removal")
	}
	lock, err := filelock.Acquire(config + ".library.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	state, err := loadLibraryState(config, c)
	if err != nil {
		return err
	}
	if state.ManagedRemovals != managedRemovalVersion {
		return errors.New("managed removal was not negotiated")
	}
	if err := libraryApplyConsent(config, c); err != nil {
		return err
	}
	for i := range state.Bindings {
		binding := &state.Bindings[i]
		if binding.Path != filepath.Join(path, action.Skill.Slug) {
			continue
		}
		if !managedLibraryPath(*binding, action.RemovalIntent.TargetID, path) || binding.Deployment.Removal != nil || libraryRemovalCheck(*binding, action.RemovalIntent.Generation) != *action.RemovalIntent || binding.ItemID != action.Skill.ItemID {
			return errors.New("managed removal ownership changed")
		}
		if err := safeLibraryParent(binding.Path); err != nil {
			return err
		}
		digest, err := privateSkillDigest(binding.Path)
		if err != nil {
			return err
		}
		if digest != action.BeforeDigest {
			return errLocalSkillsChanged
		}
		intent, err := readLibraryRemovalIntent(ctx, c, *action.RemovalIntent, version)
		if err != nil {
			return err
		}
		if intent.Action != "remove" {
			return errTargetChanged
		}
		// Capture complete valid edits with their original causal base. Partial
		// bundles are not uploaded as deletions; every surviving byte still moves
		// intact into the recoverable backup below.
		capture := state
		capture.Bindings = []libraryBinding{*binding}
		if state.CalibratedAt != 0 {
			if err := captureLibraryEdits(&capture); err != nil {
				return err
			}
		}
		state.Pending = capture.Pending
		*binding = capture.Bindings[0]
		err = backupSkillEntry(path, action.Skill.Slug, out, onBackup, func(backup string) error {
			if err := safeLibraryParent(filepath.Dir(backup)); err != nil {
				return err
			}
			binding.Deployment.Removal = &libraryRemoval{Generation: intent.Generation, Backup: backup, Phase: "prepared"}
			// The destination and pending edit bytes survive a crash even if it
			// happens between rename and the final receipt. A failed write refuses
			// the move; an unreadable/changed source never becomes an empty copy.
			if err := writePrivate(config+".library", state); err != nil {
				return err
			}
			if err := libraryApplyConsent(config, c); err != nil {
				return err
			}
			if err := safeLibraryParent(binding.Path); err != nil {
				return err
			}
			current, err := privateSkillDigest(binding.Path)
			if err != nil || current != action.BeforeDigest {
				return errLocalSkillsChanged
			}
			latest, err := readLibraryRemovalIntent(ctx, c, *action.RemovalIntent, version)
			if err != nil {
				return err
			}
			if latest.Action != "remove" {
				return errTargetChanged
			}
			return nil
		})
		if err != nil {
			return err
		}
		backupDigest, err := privateSkillDigest(binding.Deployment.Removal.Backup)
		if err != nil || backupDigest != action.BeforeDigest {
			return errors.New("managed backup needs recovery; removal is not acknowledged")
		}
		if _, err := os.Lstat(binding.Path); !os.IsNotExist(err) {
			return errors.New("local entry reappeared; removal is not acknowledged")
		}
		latest, err := readLibraryRemovalIntent(ctx, c, *action.RemovalIntent, version)
		if err != nil {
			return err // prepared receipt retains the backup for recovery
		}
		if latest.Action != "remove" {
			return errTargetChanged
		}
		binding.Deployment.Removal.Phase = "removed"
		binding.Deployment.Verified = false
		return writePrivate(config+".library", state)
	}
	return errors.New("managed removal binding was not found")
}
