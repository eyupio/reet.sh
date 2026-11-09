package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/eyupio/reet.sh/internal/filelock"
	"github.com/eyupio/reet.sh/internal/store"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// A deletion is only believed once the same files have been missing across
// observations spanning this window. An editor's atomic save, a package
// manager rewriting a folder or a user moving a file back all finish well
// inside it, so none of them can start a restore or a notification.
var libraryRestoreDebounce = 10 * time.Second

// libraryMissing is one observed deletion of files from a managed install.
// It is kept apart from the binding: an install replaces the binding's
// receipt, and the episode must survive that to be reported afterwards.
type libraryMissing struct {
	Path     string   `json:"path"`
	TargetID string   `json:"targetId"`
	ItemID   string   `json:"itemId"`
	Whole    bool     `json:"whole,omitempty"`
	Files    []string `json:"files,omitempty"`
	// FirstSeen and LastSeen are Unix milliseconds of the first and latest
	// observation of exactly this set. Count is the number of observations.
	FirstSeen int64 `json:"firstSeen"`
	LastSeen  int64 `json:"lastSeen"`
	Count     int   `json:"count"`
}

func (m libraryMissing) stable(window time.Duration) bool {
	return m.Count >= 2 && m.LastSeen-m.FirstSeen >= window.Milliseconds()
}

func (m libraryMissing) sameSet(whole bool, files []string) bool {
	if m.Whole != whole || len(m.Files) != len(files) {
		return false
	}
	for i := range files {
		if m.Files[i] != files[i] {
			return false
		}
	}
	return true
}

// episode names one deletion for receipts. It is derived from what was
// deleted and when it was first seen, so a replayed report is the same event.
func (m libraryMissing) episode() string {
	sum := sha256.Sum256([]byte(m.TargetID + "\x00" + m.ItemID + "\x00" + m.Path + "\x00" + strconv.FormatInt(m.FirstSeen, 10)))
	return hex.EncodeToString(sum[:16])
}

// libraryRestore is a verified restore waiting to be reported to the controller.
type libraryRestore struct {
	Episode  string `json:"episode"`
	Path     string `json:"path"`
	TargetID string `json:"targetId"`
	ItemID   string `json:"itemId"`
	Revision string `json:"revision"`
	Whole    bool   `json:"whole,omitempty"`
	Files    int    `json:"files"`
	At       int64  `json:"at"`
}

// missingManagedFiles reports what a managed install has lost. Anything that
// is not simply absent (a link, a file where a folder belongs, an unreadable
// location) is not a deletion: it is left alone and never restored.
func missingManagedFiles(binding libraryBinding) (whole bool, files []string, deletion bool) {
	info, err := os.Lstat(binding.Path)
	if os.IsNotExist(err) {
		return true, nil, true
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, nil, false
	}
	for _, name := range binding.Paths {
		if !filepath.IsLocal(name) {
			return false, nil, false // an unsafe record is never trusted
		}
		if _, err := os.Lstat(filepath.Join(binding.Path, name)); os.IsNotExist(err) {
			files = append(files, name)
		} else if err != nil {
			return false, nil, false
		}
	}
	sort.Strings(files)
	return false, files, len(files) > 0
}

// observeLibraryDeletions advances the debounce for every install this
// machine is proven to own. It only records what it sees: it never touches the
// filesystem, and an incomplete view (anything but plain absence) clears the
// record instead of extending it.
func observeLibraryDeletions(state *libraryLocalState, now time.Time) bool {
	at := now.UnixMilli()
	kept := make([]libraryMissing, 0, len(state.Missing))
	changed := false
	previous := map[string]libraryMissing{}
	for _, record := range state.Missing {
		previous[record.Path] = record
	}
	for _, binding := range state.Bindings {
		if binding.Source || binding.Deployment == nil || !binding.Deployment.Verified || binding.Deployment.Removal != nil {
			continue
		}
		whole, files, deletion := missingManagedFiles(binding)
		if !deletion {
			continue
		}
		record, seen := previous[binding.Path]
		if seen && record.TargetID == binding.Deployment.TargetID && record.ItemID == binding.ItemID && record.sameSet(whole, files) {
			record.LastSeen = at
			record.Count++
		} else {
			record = libraryMissing{Path: binding.Path, TargetID: binding.Deployment.TargetID, ItemID: binding.ItemID, Whole: whole, Files: files, FirstSeen: at, LastSeen: at, Count: 1}
		}
		kept = append(kept, record)
		changed = true
	}
	if len(kept) != len(state.Missing) {
		changed = true
	}
	state.Missing = kept
	return changed
}

// planLibraryRestores records this plan's observation and names the managed
// items whose deletion is not yet stable. The planner holds back their
// reinstall: a first sighting is never enough to rewrite a folder.
func planLibraryRestores(config string, c agentConfig, target store.SyncTarget, path string) (map[string]bool, error) {
	lock, err := filelock.Acquire(config + ".library.lock")
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	state, err := loadLibraryState(config, c)
	if err != nil {
		return nil, err
	}
	if observeLibraryDeletions(&state, time.Now()) {
		if err := writePrivate(config+".library", state); err != nil {
			return nil, err
		}
	}
	pending := map[string]bool{}
	for _, record := range state.Missing {
		if record.TargetID == target.Target.ID && filepath.Dir(record.Path) == filepath.Clean(path) && !record.stable(libraryRestoreDebounce) {
			pending[filepath.Base(record.Path)] = true
		}
	}
	return pending, nil
}

// recordVerifiedRestore turns a stable deletion into a receipt once the
// restored install has been verified byte for byte. An install with no stable
// deletion behind it (an ordinary update) leaves nothing to report, and a
// record that never became stable is simply dropped.
func recordVerifiedRestore(state *libraryLocalState, path, targetID, itemID, revision string, now time.Time) {
	kept := state.Missing[:0]
	for _, record := range state.Missing {
		if record.Path != path {
			kept = append(kept, record)
			continue
		}
		if record.TargetID == targetID && record.ItemID == itemID && record.stable(libraryRestoreDebounce) {
			state.Restores = append(state.Restores, libraryRestore{Episode: record.episode(), Path: path, TargetID: targetID, ItemID: itemID, Revision: revision, Whole: record.Whole, Files: len(record.Files), At: now.UnixMilli()})
		}
	}
	state.Missing = kept
}
