//go:build linux || darwin

package skillpreview

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func unknown(c Capture, path, code string) Capture {
	c.Presence, c.Complete, c.Manifest = "unknown", false, nil
	c.Issues = []Issue{{Path: path, Code: code}}
	return c
}

// physicalID hashes device and inode, so it is stable for one directory, changes
// when the directory is replaced or moved to another filesystem, and exposes no path.
func physicalID(info os.FileInfo) (string, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("reet-root-v1\x00%d\x00%d", uint64(st.Dev), uint64(st.Ino))))
	return hex.EncodeToString(sum[:]), true
}

func missing(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// Observe captures one skill slot beneath a configured scope root.
//
// Nothing is read until the scope root has been shown to be a real directory that
// no other configured scope resolves to: a symlinked, aliased or unidentifiable
// root yields unknown without looking inside. The skill folder itself is never
// followed if it is a link, and its tree is read with S1's no-follow traversal.
// Nothing is created, changed or removed.
func Observe(roots Roots, name string) Capture { return observe(roots, name, nil) }

// afterCapture lets tests replace the root at the exact point a race would.
func observe(roots Roots, name string, afterCapture func()) Capture {
	c := Capture{SchemaVersion: Version, Adapter: Adapter, RootID: Unavailable, RootOwnership: "unknown", Presence: "unknown", Issues: []Issue{}}
	if !ValidName(name) {
		return unknown(c, ".", "invalid_skill_name")
	}
	before, err := os.Lstat(roots.Target)
	switch {
	case missing(err):
		return unknown(c, ".", "scope_root_missing")
	case err != nil:
		return unknown(c, ".", "scope_root_unreadable")
	case before.Mode()&os.ModeSymlink != 0:
		// A link means the same directory has more than one name; who else uses it
		// cannot be shown from here, so this is shared rather than a safe target.
		if real, e := os.Stat(roots.Target); e == nil && real.IsDir() {
			if id, ok := physicalID(real); ok {
				c.RootID, c.RootOwnership = id, "shared"
			}
		}
		return unknown(c, ".", "scope_root_symlink")
	case !before.IsDir():
		return unknown(c, ".", "scope_root_not_directory")
	}
	id, ok := physicalID(before)
	if !ok {
		return unknown(c, ".", "scope_root_unreadable")
	}
	c.RootID = id
	aliased, incomplete := false, !roots.OthersComplete
	for _, other := range roots.Others {
		info, e := os.Stat(other)
		switch {
		case missing(e):
		case e != nil:
			// A root we may not inspect could still be the same directory.
			incomplete = true
		case os.SameFile(before, info):
			aliased = true
		}
	}
	switch {
	case aliased:
		c.RootOwnership = "shared"
		return unknown(c, ".", "root_aliased")
	case incomplete:
		return unknown(c, ".", "alias_check_incomplete")
	}
	c.RootOwnership = "exclusive"

	root, err := os.OpenRoot(roots.Target)
	if err != nil {
		return unknown(c, ".", "scope_root_unreadable")
	}
	defer root.Close()
	if opened, e := root.Stat("."); e != nil || !os.SameFile(before, opened) {
		return unknown(c, ".", "scope_root_changed")
	}
	entry, err := root.Lstat(name)
	switch {
	case missing(err):
		c.Presence, c.Complete = "absent", true
	case err != nil:
		return unknown(c, name, "skill_unavailable")
	case entry.Mode()&os.ModeSymlink != 0:
		return unknown(c, name, "skill_symlink")
	case !entry.IsDir():
		return unknown(c, name, "skill_not_directory")
	default:
		m, scanErr := skillbundle.Scan(filepath.Join(roots.Target, name), skillbundle.Limits{})
		if scanErr != nil {
			c = unknown(c, ".", "capture_failed")
			c.Issues = issuesFrom(m, c.Issues)
		} else {
			m.Issues = nil
			c.Presence, c.Complete, c.Manifest = "present", true, &m
		}
	}
	if afterCapture != nil {
		afterCapture()
	}
	// The scope root must still be the directory that was checked, or the answer
	// describes something else.
	if after, e := os.Lstat(roots.Target); e != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after) {
		return unknown(c, ".", "scope_root_changed")
	}
	return c
}

// issuesFrom keeps S1's own explanation (relative paths and codes only), bounded.
func issuesFrom(m skillbundle.Manifest, fallback []Issue) []Issue {
	out := []Issue{}
	for _, issue := range m.Issues {
		if len(out) == MaxIssues {
			break
		}
		path, code := issue.Path, issue.Code
		if !validIssuePath(path) {
			path = "."
		}
		if !issueCode.MatchString(code) {
			code = "capture_failed"
		}
		out = append(out, Issue{Path: path, Code: code})
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
