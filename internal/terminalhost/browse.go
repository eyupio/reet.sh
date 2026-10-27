package terminalhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	// browseDeadline bounds one browse; a context deadline that comes sooner
	// wins.
	browseDeadline = 3 * time.Second
	// maxBrowseScan is the most directory entries one browse reads.
	maxBrowseScan = 5000
)

// FS is everything a browse may do to the filesystem: list a folder it has
// already contained, and lstat or stat a path for its type. There is no way
// to open or read a file through it, by design.
type FS interface {
	ReadDir(string) ([]os.DirEntry, error)
	Lstat(string) (os.FileInfo, error)
	Stat(string) (os.FileInfo, error)
}

type osFS struct{}

// ReadDir opens name only as a directory and never through a final symbolic
// link (where the system allows; see dirOpenFlags), so neither a file nor a
// FIFO is ever opened, and reads at most maxBrowseScan+1 entries, unsorted.
func (osFS) ReadDir(name string) ([]os.DirEntry, error) {
	f, err := os.OpenFile(name, os.O_RDONLY|dirOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(maxBrowseScan + 1)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return entries, err
}
func (osFS) Lstat(name string) (os.FileInfo, error) { return os.Lstat(name) }
func (osFS) Stat(name string) (os.FileInfo, error)  { return os.Stat(name) }

// OSFS is the machine's filesystem, for Browse.
var OSFS FS = osFS{}

// Browse lists the folders directly inside req.Path (the home when empty),
// for a remote controller that must be treated as hostile:
//
//   - The folder is resolved by ResolveDir, symlinks followed, and must be
//     home or inside it; it is checked again after it is read, so a folder
//     swapped for a link mid-browse is refused rather than listed.
//   - Only folders are listed. Files are known from the directory entry's
//     type alone: no file is opened, read or even stat'ed. A symlink is
//     listed, under its own name, only when ResolveDir puts its target in a
//     folder inside home, and an lstat of that target still finds a real
//     folder.
//   - Repository detection is two lstat calls (<dir>/.git a real directory,
//     <dir>/.git/HEAD a regular file); a .git link is never followed and
//     HEAD is never read. The folder's own repository is found by walking up,
//     stopping at home.
//   - Names that cannot be shown or sent back safely (invalid UTF-8, control
//     or format characters, over 255 bytes) are omitted and counted.
//   - At most 5,000 entries are read and 200 returned, and the answer always
//     fits terminalproto.MaxBrowseReplyBytes: a page ends early with More set.
//   - After the deadline (3 s, or ctx's if sooner) no filesystem call is
//     made: folders still known from their entry are listed with RepoRoot
//     false, links are omitted, and Partial is set.
//
// Omitted counts the entries examined for this page and left out (hidden
// folders and filter misses are not counted). The answer is always a valid
// browse_result.
func Browse(ctx context.Context, home string, req terminalproto.Browse, fsys FS, now func() time.Time) terminalproto.BrowseResult {
	deadline := now().Add(browseDeadline)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	b := &browser{fsys: fsys, expired: func() bool { return ctx.Err() != nil || !now().Before(deadline) }}
	r, reason := b.browse(home, req)
	if reason == "" && !browseResultValid(r) {
		reason = terminalproto.BrowseUnreadable
	}
	if reason != "" {
		return terminalproto.BrowseResult{Reason: reason}
	}
	return r
}

type browser struct {
	fsys    FS
	expired func() bool
	partial bool
}

// late reports whether the deadline has passed, and marks the answer partial
// when it has.
func (b *browser) late() bool {
	if b.expired() {
		b.partial = true
	}
	return b.partial
}

func (b *browser) browse(home string, req terminalproto.Browse) (terminalproto.BrowseResult, terminalproto.BrowseReason) {
	realHome, err := ResolveDir(home, home)
	if err != nil {
		return terminalproto.BrowseResult{}, terminalproto.BrowseUnreadable
	}
	path := req.Path
	if path == "" {
		path = realHome
	}
	dir, err := ResolveDir(realHome, path)
	if err != nil {
		return terminalproto.BrowseResult{}, b.whyNot(home, realHome, path)
	}
	r := terminalproto.BrowseResult{Path: dir, Home: realHome, Repo: terminalproto.BrowseRepoNone}
	if dir != realHome {
		r.Parent = filepath.Dir(dir)
	}
	r.Repo, r.RepoTop = b.repoOf(realHome, dir)

	list, err := b.fsys.ReadDir(dir)
	if err != nil {
		return terminalproto.BrowseResult{}, readReason(err)
	}
	// What was read must still resolve to dir: a component swapped for a link
	// between resolving and reading could have led anywhere. (The machine's
	// ReadDir also refuses to open a final link.)
	if again, err := ResolveDir(realHome, dir); err != nil || again != dir {
		return terminalproto.BrowseResult{}, terminalproto.BrowseOutsideHome
	}
	if len(list) > maxBrowseScan {
		list, r.More = list[:maxBrowseScan], true
	}
	r.Scanned = len(list)
	keys := make([]string, len(list))
	order := make([]int, len(list))
	for i, e := range list {
		keys[i], order[i] = strings.ToLower(e.Name()), i
	}
	sort.Slice(order, func(i, j int) bool {
		a, c := order[i], order[j]
		if keys[a] != keys[c] {
			return keys[a] < keys[c]
		}
		return list[a].Name() < list[c].Name()
	})

	afterKey := strings.ToLower(req.After)
	filter := strings.ToLower(req.Filter)
	size := browseBaseSize(r)
	for _, i := range order {
		name := list[i].Name()
		if req.After != "" && (keys[i] < afterKey || keys[i] == afterKey && name <= req.After) {
			continue
		}
		if !browseName(name) {
			r.Omitted++
			continue
		}
		hidden := strings.HasPrefix(name, ".")
		if hidden && !req.Hidden || !strings.Contains(keys[i], filter) {
			continue
		}
		typ := list[i].Type()
		if !typ.IsDir() && typ&fs.ModeSymlink == 0 {
			r.Omitted++ // a file, device, FIFO or socket
			continue
		}
		if len(r.Entries) == terminalproto.MaxBrowseEntries {
			r.More = true
			break
		}
		target := filepath.Join(dir, name)
		if typ&fs.ModeSymlink != 0 {
			if b.late() {
				r.Omitted++
				continue
			}
			resolved, ok := b.linkedDir(realHome, target)
			if !ok {
				r.Omitted++
				continue
			}
			target = resolved
		}
		e := terminalproto.BrowseEntry{Name: name, Dir: true, Hidden: hidden}
		if !b.late() {
			e.RepoRoot = b.repoRoot(target)
		}
		n := browseEntrySize(e)
		if size+n > terminalproto.MaxBrowseReplyBytes {
			r.More = true
			break
		}
		size += n
		r.Entries = append(r.Entries, e)
	}
	r.Partial = b.partial
	return r, ""
}

// whyNot names why ResolveDir refused path. A path outside home is answered
// outside_home before anything there is looked at, so a browse never tells
// whether something outside home exists.
func (b *browser) whyNot(home, realHome, path string) terminalproto.BrowseReason {
	if !filepath.IsAbs(path) {
		return terminalproto.BrowseOutsideHome
	}
	clean := filepath.Clean(path)
	if !within(realHome, clean) && !within(filepath.Clean(home), clean) {
		return terminalproto.BrowseOutsideHome
	}
	resolved, err := filepath.EvalSymlinks(clean)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return terminalproto.BrowseNotFound
	case errors.Is(err, fs.ErrPermission):
		return terminalproto.BrowseDenied
	case err != nil:
		return terminalproto.BrowseUnreadable
	case !within(realHome, resolved):
		return terminalproto.BrowseOutsideHome
	}
	info, err := b.fsys.Lstat(resolved)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return terminalproto.BrowseNotFound
	case err != nil:
		return terminalproto.BrowseUnreadable
	case !info.IsDir():
		return terminalproto.BrowseNotDir
	}
	return terminalproto.BrowseUnreadable
}

func readReason(err error) terminalproto.BrowseReason {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return terminalproto.BrowseNotFound
	case errors.Is(err, fs.ErrPermission):
		return terminalproto.BrowseDenied
	}
	return terminalproto.BrowseUnreadable
}

// within reports whether p is root or inside it (both clean and absolute).
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// realDir reports whether an lstat finds a directory (not a link) at p.
func (b *browser) realDir(p string) bool {
	info, err := b.fsys.Lstat(p)
	return err == nil && info.IsDir()
}

// linkedDir resolves a symlink entry. The target must be a folder inside
// home (ResolveDir), and an lstat after resolving must still find a real
// folder there, not a link swapped in since.
func (b *browser) linkedDir(realHome, link string) (string, bool) {
	resolved, err := ResolveDir(realHome, link)
	if err != nil || !b.realDir(resolved) {
		return "", false
	}
	return resolved, true
}

// repoRoot reports whether dir holds a real .git directory with a HEAD
// file. Neither is followed if it is a link, and HEAD is not read.
func (b *browser) repoRoot(dir string) bool {
	git, err := b.fsys.Lstat(filepath.Join(dir, ".git"))
	if err != nil || !git.IsDir() {
		return false
	}
	head, err := b.fsys.Lstat(filepath.Join(dir, ".git", "HEAD"))
	return err == nil && head.Mode().IsRegular()
}

// repoOf walks up from dir, never above home, to the nearest working tree
// top. dir and home are resolved and dir is inside home.
func (b *browser) repoOf(home, dir string) (terminalproto.BrowseRepo, string) {
	for d := dir; within(home, d); d = filepath.Dir(d) {
		if b.late() {
			return terminalproto.BrowseRepoNone, ""
		}
		if b.repoRoot(d) {
			if d == dir {
				return terminalproto.BrowseRepoRoot, d
			}
			return terminalproto.BrowseRepoInside, d
		}
		if filepath.Dir(d) == d { // home is /
			break
		}
	}
	return terminalproto.BrowseRepoNone, ""
}

// browseName reports whether a folder name can be shown and sent back: the
// protocol's entry rule (valid UTF-8, no control or format characters, at
// most 255 bytes).
func browseName(name string) bool {
	if name == "" || name == "." || name == ".." || len(name) > terminalproto.MaxBrowseNameBytes || !utf8.ValidString(name) || strings.ContainsRune(name, '/') {
		return false
	}
	for _, c := range name {
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || c == ' ' || c == ' ' {
			return false
		}
	}
	return true
}

// browseBaseSize is an upper bound on the encoded answer without entries:
// More and Partial set, Omitted at its largest, and the entries brackets.
func browseBaseSize(r terminalproto.BrowseResult) int {
	r.More, r.Partial, r.Omitted, r.Entries = true, true, r.Scanned, nil
	data, _ := json.Marshal(r)
	return len(data) + len(`,"entries":[]`)
}

// browseEntrySize is what one entry adds to the encoded answer, with its
// separating comma.
func browseEntrySize(e terminalproto.BrowseEntry) int {
	data, _ := json.Marshal(e)
	return len(data) + 1
}

// browseResultValid reports whether r encodes as a browse_result.
func browseResultValid(r terminalproto.BrowseResult) bool {
	_, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: "browse-result-check", Generation: 1, Body: r})
	return err == nil
}
