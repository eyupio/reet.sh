package scan

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/eyupio/reet.sh/internal/filelock"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/store"
)

type libraryBinding struct {
	Paths    []string `json:"paths"`
	Path     string   `json:"path"`
	ItemID   string   `json:"itemId"`
	Revision string   `json:"revision"`
	Digest   string   `json:"digest"`
	// Imported source roots and legacy edit bases never imply permission to
	// remove a directory. Deployment is granted only by a verified new install.
	Source     bool               `json:"source,omitempty"`
	Deployment *libraryDeployment `json:"deployment,omitempty"`
}

// errLocalAuthorityChanged means the local consent or enrollment this run was
// authorised by is no longer the one in force. It affects the whole target,
// never a single item, so an apply stops instead of isolating it.
var errLocalAuthorityChanged = errors.New("local sync consent or enrollment changed")

type libraryDeployment struct {
	TargetID string          `json:"targetId"`
	Revision string          `json:"revision"`
	Digest   string          `json:"digest"`
	Verified bool            `json:"verified"`
	Removal  *libraryRemoval `json:"removal,omitempty"`
}
type libraryRemoval struct {
	Generation string `json:"generation"`
	Backup     string `json:"backup"`
	Phase      string `json:"phase"`
	// Digest is the content the entry had when the move was prepared, so recovery
	// can prove the backup is that content before it relies on it.
	Digest string `json:"digest,omitempty"`
}
type libraryLocalState struct {
	Server          string              `json:"server"`
	Enrollment      string              `json:"enrollment"`
	Offset          int64               `json:"offset"`
	Uncertainty     int64               `json:"uncertainty"`
	CalibratedAt    int64               `json:"calibratedAt"`
	Bindings        []libraryBinding    `json:"bindings"`
	Pending         []store.LibraryEdit `json:"pending"`
	ManagedRemovals int                 `json:"managedRemovals,omitempty"`
	// Missing and Restores are the debounced deletion episodes and the verified
	// restores not yet reported; see library_restore.go.
	Missing  []libraryMissing `json:"missing,omitempty"`
	Restores []libraryRestore `json:"restores,omitempty"`
}

func loadLibraryState(config string, c agentConfig) (libraryLocalState, error) {
	state := libraryLocalState{Server: c.Server, Enrollment: c.ID}
	data, err := os.ReadFile(config + ".library")
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	if state.Server != c.Server || state.Enrollment != c.ID {
		return state, errLocalAuthorityChanged
	}
	return state, nil
}

func libraryApplyConsent(config string, c agentConfig) error {
	local, err := loadAgentConfig(config)
	if err != nil {
		return err
	}
	if !local.AllowApply || local.ID != c.ID || local.Server != c.Server || local.Secret != c.Secret {
		return errLocalAuthorityChanged
	}
	return nil
}

// Only exact private revisions installed by this enrollment are watched. A
// missing file is not an edit/deletion; Stage 4 restores it from central content.
func captureLibraryEdits(state *libraryLocalState) error {
	for i := range state.Bindings {
		binding := &state.Bindings[i]
		var queuedBytes int64
		for _, event := range state.Pending {
			for _, file := range event.Files {
				queuedBytes += int64(len(file.Data))
			}
		}
		if queuedBytes >= 64<<20 || len(state.Pending) >= 256 {
			break
		}
		if err := safeLibraryParent(binding.Path); err != nil {
			continue
		}
		missing := false
		for _, path := range binding.Paths {
			if _, err := os.Lstat(filepath.Join(binding.Path, filepath.FromSlash(path))); err != nil {
				missing = true
				break
			}
		}
		if missing {
			continue
		}
		manifest, err := skillbundle.Scan(binding.Path, skillbundle.Limits{})
		if err != nil || !manifest.Complete || manifest.Digest == binding.Digest || queuedBytes+manifest.TotalBytes > 64<<20 {
			continue
		}
		// Never turn local absence of a managed file into a central deletion.
		info, err := os.Lstat(filepath.Join(binding.Path, "SKILL.md"))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		_, files, err := skillbundle.ReadContents(binding.Path, manifest.Digest)
		if err != nil {
			continue
		}
		if _, _, err = skillimport.Select(files, nil); err != nil {
			continue
		}
		saved := info.ModTime().UnixMilli()
		for _, file := range files {
			info, err := os.Lstat(filepath.Join(binding.Path, filepath.FromSlash(file.Path)))
			if err != nil {
				continue
			}
			if info.ModTime().UnixMilli() > saved {
				saved = info.ModTime().UnixMilli()
			}
		}
		random := make([]byte, 16)
		if _, err = rand.Read(random); err != nil {
			return err
		}
		// A later local edit queues after the prior one; upload binds its causal
		// base to that prior receipt before sending it. All bytes are durable first.
		state.Pending = append(state.Pending, store.LibraryEdit{EventID: hex.EncodeToString(random), ItemID: binding.ItemID, Base: binding.Revision, SavedAt: saved + state.Offset, Uncertainty: state.Uncertainty + max(0, time.Now().UnixMilli()-state.CalibratedAt)/10000, Files: files})
		binding.Digest = manifest.Digest
	}
	return nil
}
func syncLibraryEdits(ctx context.Context, c agentConfig, config, version string, out io.Writer) error {
	local, err := loadAgentConfig(config)
	if err != nil {
		return err
	}
	if local.ID != c.ID || local.Server != c.Server || local.Secret != c.Secret {
		return errLocalAuthorityChanged
	}
	c.AllowApply = local.AllowApply
	lock, err := filelock.Acquire(config + ".library.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	state, err := loadLibraryState(config, c)
	if err != nil {
		return err
	}
	// Capture precedes all network operations: offline edits keep their original
	// base and calibrated source time through process/controller restarts.
	if state.CalibratedAt != 0 && c.AllowApply {
		if err = captureLibraryEdits(&state); err != nil {
			return err
		}
		if err = writePrivate(config+".library", state); err != nil {
			return err
		}
	}
	// Observing a deletion is read-only; it keeps the debounce moving between jobs.
	if observeLibraryDeletions(&state, time.Now()) {
		if err = writePrivate(config+".library", state); err != nil {
			return err
		}
	}
	start := time.Now()
	var protocol struct {
		SchemaVersion   int   `json:"schemaVersion"`
		ServerTime      int64 `json:"serverTime"`
		ManagedRemovals int   `json:"managedRemovals"`
	}
	if err = agentRequestContext(ctx, c, "/api/v2/library/protocol", struct{}{}, &protocol, version); err != nil {
		return err
	}
	if protocol.SchemaVersion != 1 {
		return errors.New("unsupported library protocol")
	}
	end := time.Now()
	state.Offset = protocol.ServerTime - (start.UnixMilli()+end.UnixMilli())/2
	state.Uncertainty = (end.Sub(start).Milliseconds()+1)/2 + 1
	state.CalibratedAt = end.UnixMilli()
	// Older controllers omit the capability. Never infer it from protocol 1
	// or retain an earlier controller's advertised version after a downgrade.
	state.ManagedRemovals = protocol.ManagedRemovals
	if err = writePrivate(config+".library", state); err != nil {
		return err
	}
	if !c.AllowApply {
		return nil
	}
	// A bad item's upload cannot prevent other items' edits from being sent.
	failed := map[string]bool{}
	for i := 0; i < len(state.Pending); {
		event := state.Pending[i]
		if failed[event.ItemID] {
			i++
			continue
		}
		var receipt store.LibraryEditReceipt
		if err = agentRequestContext(ctx, c, "/api/v2/library/edits", event, &receipt, version); err != nil {
			failed[event.ItemID] = true
			fmt.Fprintln(out, "A library edit is retained locally and will retry:", err)
			i++
			continue
		}
		if receipt.EventID != event.EventID || !store.PrivateSkillIDValid(receipt.RevisionID) {
			return errors.New("invalid library edit receipt")
		}
		// Persist the receipt before dropping bytes. An uncertain HTTP result is
		// retried with exactly the same ID/payload and cannot move the head twice.
		state.Pending = append(state.Pending[:i], state.Pending[i+1:]...)
		for j := range state.Bindings {
			if state.Bindings[j].ItemID == event.ItemID {
				state.Bindings[j].Revision = receipt.RevisionID
			}
		}
		for j := i; j < len(state.Pending); j++ {
			if state.Pending[j].ItemID == event.ItemID {
				state.Pending[j].Base = receipt.RevisionID
				break
			}
		}
		if err = writePrivate(config+".library", state); err != nil {
			return err
		}
	}
	return nil
}
func recordLibraryInstall(config string, c agentConfig, skill store.SyncSkill, path string, files []skillbundle.ContentFile, targetID string) error {
	if skill.ItemID == "" {
		return nil
	}
	parts := privateSourcePattern.FindStringSubmatch(skill.Source)
	if parts == nil || !safeSlug.MatchString(skill.Slug) || !filepath.IsAbs(path) {
		return errors.New("invalid library source")
	}
	state, err := loadLibraryState(config, c)
	if err != nil {
		return err
	}
	binding := libraryBinding{Path: filepath.Join(path, skill.Slug), ItemID: skill.ItemID, Revision: parts[2], Digest: parts[3]}
	for _, file := range files {
		binding.Paths = append(binding.Paths, file.Path)
	}
	found := false
	for i := range state.Bindings {
		if state.Bindings[i].Path == binding.Path {
			previous := state.Bindings[i]
			if previous.ItemID != binding.ItemID {
				return errors.New("library directory belongs to a different item")
			}
			if targetID != "" && previous.Deployment != nil && previous.Deployment.TargetID != targetID {
				return errors.New("library directory belongs to a different target")
			}
			binding.Source = previous.Source
			// Unknown old receipts remain edit bases, even when their bytes
			// match. A prior source import must never become removable by sync.
			if !previous.Source && previous.Deployment != nil && previous.Deployment.TargetID == targetID && targetID != "" {
				binding.Deployment = &libraryDeployment{TargetID: targetID, Revision: binding.Revision, Digest: binding.Digest}
			}
			state.Bindings[i] = binding
			found = true
			break
		}
	}
	if !found {
		if targetID != "" {
			binding.Deployment = &libraryDeployment{TargetID: targetID, Revision: binding.Revision, Digest: binding.Digest}
		}
		state.Bindings = append(state.Bindings, binding)
	}
	return writePrivate(config+".library", state)
}

// finishLibraryInstall runs only after the existing executor has renamed the
// staged bundle into place. The pre-write base pin alone never grants ownership:
// failed/interrupted writes retain an unverified journal entry. Legacy/source
// bindings deliberately stay unowned until a separate migration proves them.
func finishLibraryInstall(config string, c agentConfig, skill store.SyncSkill, path, targetID string) error {
	if skill.ItemID == "" {
		return nil
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
	parts := privateSourcePattern.FindStringSubmatch(skill.Source)
	if parts == nil || targetID == "" || !safeSlug.MatchString(skill.Slug) || !filepath.IsAbs(path) {
		return errors.New("invalid library deployment")
	}
	candidate := filepath.Join(path, skill.Slug)
	for i := range state.Bindings {
		binding := &state.Bindings[i]
		if binding.Path != candidate {
			continue
		}
		if binding.Source || binding.Deployment == nil {
			return nil
		}
		deployment := binding.Deployment
		if binding.ItemID != skill.ItemID || binding.Revision != parts[2] || binding.Digest != parts[3] || deployment.TargetID != targetID || deployment.Revision != parts[2] || deployment.Digest != parts[3] {
			return errors.New("library deployment changed before verification")
		}
		if err := libraryApplyConsent(config, c); err != nil {
			return err
		}
		if err := safeLibraryParent(candidate); err != nil {
			return err
		}
		digest, err := privateSkillDigest(candidate)
		if err != nil {
			return err
		}
		if digest != deployment.Digest {
			return errors.New("installed library content changed before verification")
		}
		deployment.Verified = true
		recordVerifiedRestore(&state, candidate, targetID, skill.ItemID, parts[2], time.Now())
		return writePrivate(config+".library", state)
	}
	return errors.New("library deployment was not prepared")
}

// A failed or unsent local capture must survive a remote apply. Exclude just
// that item from additive reconciliation, and let healthy items proceed.
func filterLibraryPending(config string, c agentConfig, desired []store.SyncSkill) []store.SyncSkill {
	state, err := loadLibraryState(config, c)
	if err != nil {
		return nil
	}
	pending := map[string]bool{}
	for _, event := range state.Pending {
		pending[event.ItemID] = true
	}
	out := make([]store.SyncSkill, 0, len(desired))
	for _, skill := range desired {
		blocked := pending[skill.ItemID]
		for _, binding := range state.Bindings {
			if skill.ItemID == "" || binding.ItemID != skill.ItemID {
				continue
			}
			digest, err := privateSkillDigest(binding.Path)
			// Unknown/unreadable and partial deletions stay local until restoration is
			// implemented. A changed digest needs capture before any overwrite.
			if (err != nil && !os.IsNotExist(err)) || (err == nil && digest != binding.Digest) {
				blocked = true
			}
		}
		if !blocked {
			out = append(out, skill)
		}
	}
	return out
}

func safeLibraryParent(path string) error {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return errors.New("library location contains an unsupported directory link")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

func recordLibrarySource(config string, c agentConfig, path, item, revision string, manifest skillbundle.Manifest) error {
	lock, err := filelock.Acquire(config + ".library.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	state, err := loadLibraryState(config, c)
	if err != nil {
		return err
	}
	// Re-running an import must not replace the base of an unsent offline edit.
	for _, event := range state.Pending {
		if event.ItemID == item {
			return errors.New("source has an unsent edit")
		}
	}
	binding := libraryBinding{Path: path, ItemID: item, Revision: revision, Digest: manifest.Digest, Source: true}
	for _, file := range manifest.Files {
		binding.Paths = append(binding.Paths, file.Path)
	}
	found := false
	for i := range state.Bindings {
		if state.Bindings[i].Path == path {
			state.Bindings[i] = binding
			found = true
			break
		}
	}
	if !found {
		state.Bindings = append(state.Bindings, binding)
	}
	return writePrivate(config+".library", state)
}

func prepareLibraryInstall(ctx context.Context, config string, c agentConfig, skill store.SyncSkill, path string, files []skillbundle.ContentFile, version, targetID string) error {
	if skill.ItemID == "" {
		return nil
	}
	if err := libraryApplyConsent(config, c); err != nil {
		return err
	}
	lock, err := filelock.Acquire(config + ".library.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	return prepareLibraryInstallLocked(ctx, config, c, skill, path, files, version, targetID)
}

func prepareLibraryInstallLocked(ctx context.Context, config string, c agentConfig, skill store.SyncSkill, path string, files []skillbundle.ContentFile, version, targetID string) error {
	state, err := loadLibraryState(config, c)
	if err != nil {
		return err
	}
	for _, event := range state.Pending {
		if event.ItemID == skill.ItemID {
			return errors.New("local library edit is still waiting to upload")
		}
	}

	parts := privateSourcePattern.FindStringSubmatch(skill.Source)
	if parts == nil {
		return errors.New("invalid library source")
	}
	key := sha256.Sum256([]byte(filepath.Join(path, skill.Slug)))
	in := store.LibraryBase{BindingID: hex.EncodeToString(key[:]), ItemID: skill.ItemID, RevisionID: parts[2]}
	var receipt store.LibraryBase
	if err := agentRequestContext(ctx, c, "/api/v2/library/bases", in, &receipt, version); err != nil {
		return err
	}
	if receipt != in {
		return errors.New("invalid library base receipt")
	}
	return recordLibraryInstall(config, c, skill, path, files, targetID)
}

// Adopt an existing directory only from exact selected bytes or a verified
// previous apply baseline. An unrelated same-name directory never supplies a base.
func adoptLibraryInstall(ctx context.Context, config string, c agentConfig, want store.SyncSkill, target store.SyncTarget, path, version string) error {
	if want.ItemID == "" {
		return errors.New("The selected item has no canonical identity.")
	}
	lock, err := filelock.Acquire(config + ".library.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	local, err := loadAgentConfig(config)
	if err != nil {
		return err
	}
	if !local.AllowApply {
		return errors.New("Enable sync locally to track this existing copy.")
	}
	if local.ID != c.ID || local.Server != c.Server || local.Secret != c.Secret {
		return errLocalAuthorityChanged
	}
	state, err := loadLibraryState(config, c)
	if err != nil {
		return err
	}
	candidate := filepath.Join(path, want.Slug)
	for _, binding := range state.Bindings {
		if binding.Path == candidate {
			if binding.ItemID != want.ItemID {
				return errors.New("This directory belongs to a different library item.")
			}
			return nil
		}
	}
	digest, err := privateSkillDigest(candidate)
	if err != nil {
		return err
	}
	wanted := privateSourcePattern.FindStringSubmatch(want.Source)
	if wanted == nil {
		return errors.New("The selected item has no private revision.")
	}
	base := want
	dirty := digest != wanted[3]
	if dirty {
		proven := false
		for _, old := range target.Baseline {
			parts := privateSourcePattern.FindStringSubmatch(old.Source)
			if parts != nil && old.Slug == want.Slug && (old.ItemID == want.ItemID || old.ID == want.ID || parts[1] == wanted[1]) {
				base = old
				base.ItemID = want.ItemID
				proven = true
				break
			}
		}
		if !proven {
			return errors.New("Local content differs; import this folder to preserve it before syncing this item.")
		}
	}
	// Original file paths come from the immutable baseline, so a missing local
	// support file is never captured as an intentional central deletion.
	files, err := privateSkillFiles(c.Server, c.Token, base)
	if err != nil {
		return err
	}
	if err := prepareLibraryInstallLocked(ctx, config, c, base, path, files, version, ""); err != nil {
		return err
	}
	if dirty {
		return errors.New("Local edits retained; they will upload on the next sync.")
	}
	return nil
}
