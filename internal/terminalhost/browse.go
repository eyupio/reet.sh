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
	"syscall"
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
	// maxBrowseHops bounds the links whyNot follows, as the kernel does.
	maxBrowseHops = 255
)

// FS is everything a browse may do to the filesystem: list a folder it has
// already contained, and lstat or stat a path for its type. There is no way
// to open or read a file through it, by design. Paths are absolute.
type FS interface {
	ReadDir(string) ([]os.DirEntry, error)
	Lstat(string) (os.FileInfo, error)
	Stat(string) (os.FileInfo, error)
}

// rooter is an FS that can confine every later call beneath one folder.
// Browse roots its FS at the real home for the whole browse; a test fake that
// wraps another FS passes rootAt through so it is confined too.
type rooter interface {
	rootAt(dir string) (FS, func(), error)
}

// errNotRootable refuses a browse over an FS that cannot be confined.
var errNotRootable = errors.New("this filesystem cannot be confined beneath home")

// rootBeneath confines fsys beneath dir. It fails closed: an FS that cannot
// be rooted is an error, never used unconfined. done releases what was
// opened.
func rootBeneath(fsys FS, dir string) (FS, func(), error) {
	if r, ok := fsys.(rooter); ok {
		return r.rootAt(dir)
	}
	return nil, nil, errNotRootable
}

type osFS struct{}

// rootAt opens dir as an os.Root: every call through the returned FS is
// resolved component by component beneath it, and a link (or "..") that
// would leave it, even one swapped in mid-browse, is refused there.
func (osFS) rootAt(dir string) (FS, func(), error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	return rootFS{root: root, dir: dir}, func() { root.Close() }, nil
}

// The unrooted methods only serve callers that never root (none in Browse).
func (osFS) ReadDir(name string) ([]os.DirEntry, error) {
	f, err := os.OpenFile(name, os.O_RDONLY|dirOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	return readDirFile(f)
}
func (osFS) Lstat(name string) (os.FileInfo, error) { return os.Lstat(name) }
func (osFS) Stat(name string) (os.FileInfo, error)  { return os.Stat(name) }

// readDirFile reads at most maxBrowseScan+1 entries, unsorted, and closes f.
func readDirFile(f *os.File) ([]os.DirEntry, error) {
	defer f.Close()
	entries, err := f.ReadDir(maxBrowseScan + 1)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return entries, err
}

// errOutsideRoot refuses a path that is not beneath the browse root.
var errOutsideRoot = errors.New("path is outside the browse root")

// rootFS is the machine's filesystem confined beneath dir.
type rootFS struct {
	root *os.Root
	dir  string
}

func (r rootFS) rel(p string) (string, error) {
	rel, err := filepath.Rel(r.dir, p)
	if err != nil || !within(r.dir, p) {
		return "", errOutsideRoot
	}
	return rel, nil
}

// ReadDir opens name only as a directory (dirOpenFlags), so neither a file
// nor a FIFO is ever opened.
func (r rootFS) ReadDir(name string) ([]os.DirEntry, error) {
	rel, err := r.rel(name)
	if err != nil {
		return nil, err
	}
	f, err := r.root.OpenFile(rel, os.O_RDONLY|dirOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	return readDirFile(f)
}
func (r rootFS) Lstat(name string) (os.FileInfo, error) {
	rel, err := r.rel(name)
	if err != nil {
		return nil, err
	}
	return r.root.Lstat(rel)
}
func (r rootFS) Stat(name string) (os.FileInfo, error) {
	rel, err := r.rel(name)
	if err != nil {
		return nil, err
	}
	return r.root.Stat(rel)
}

// OSFS is the machine's filesystem, for Browse.
var OSFS FS = osFS{}

// Browse lists the folders directly inside req.Path (the home when empty),
// for a remote controller that must be treated as hostile:
//
//   - The folder is resolved by ResolveDir, symlinks followed, and must be
//     home or inside it. When it is refused, the reason is found by walking
//     from home one component at a time, and a path that leaves home at any
//     point is outside_home whether or not anything exists there.
//   - Every filesystem call is made through an os.Root opened at the real
//     home, so a component swapped for a link that leads out, even between
//     two calls, is refused rather than followed. The folder is also
//     resolved again after it is read.
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
//   - At most 5,000 entries are read (Scanned shows the cap) and 200
//     returned, and the answer always fits terminalproto.MaxBrowseReplyBytes.
//     More is set only when a page ends before an entry it could still list,
//     so a page with More always has a last name to continue from; an entry
//     too wide to fit any page is omitted.
//   - After the deadline (3 s, or ctx's if sooner) no repository check is
//     made (folders are listed with RepoRoot false, and Partial is set), and
//     the page ends with More at the next link, which needs a filesystem call
//     to resolve. A page that has nothing yet resolves that one link, so
//     paging always makes progress.
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
	rooted, done, err := rootBeneath(b.fsys, realHome)
	if err != nil {
		return terminalproto.BrowseResult{}, terminalproto.BrowseUnreadable
	}
	defer done()
	b.fsys = rooted

	path := req.Path
	if path == "" {
		path = realHome
	}
	// ".." is taken lexically, before anything is resolved. Resolving first
	// would follow a link out of home and lstat the name before a "..",
	// so whether "~/link-out/<probe>/../.." succeeds would tell whether
	// <probe> exists outside home.
	path = filepath.Clean(path)
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
	// What was read must still resolve to dir. The root already refuses a
	// component swapped for a link that leads out; this also catches an FS
	// that cannot be rooted.
	if again, err := ResolveDir(realHome, dir); err != nil || again != dir {
		return terminalproto.BrowseResult{}, terminalproto.BrowseOutsideHome
	}
	if len(list) > maxBrowseScan {
		list = list[:maxBrowseScan]
	}
	r.Scanned = len(list)
	sort.Slice(list, func(i, j int) bool { return terminalproto.BrowseNameLess(list[i].Name(), list[j].Name()) })

	filter := strings.ToLower(req.Filter)
	size := browseBaseSize(r)
	for _, entry := range list {
		name := entry.Name()
		if req.After != "" && !terminalproto.BrowseNameLess(req.After, name) {
			continue
		}
		if !browseName(name) {
			r.Omitted++
			continue
		}
		hidden := strings.HasPrefix(name, ".")
		if hidden && !req.Hidden || !strings.Contains(strings.ToLower(name), filter) {
			continue
		}
		typ := entry.Type()
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
			if b.late() && len(r.Entries) > 0 {
				r.More = true
				break
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
			if len(r.Entries) == 0 {
				r.Omitted++ // too wide for any page here
				continue
			}
			r.More = true
			break
		}
		size += n
		r.Entries = append(r.Entries, e)
	}
	r.Partial = b.partial
	return r, ""
}

// whyNot names why ResolveDir refused path. It resolves path itself, one
// component at a time from the real home, following links by reading them,
// and answers outside_home as soon as the walk would leave home: nothing
// beyond that point is looked at, so whether a path outside home exists is
// never told. Every lstat is of an entry in a folder already inside home.
func (b *browser) whyNot(home, realHome, path string) terminalproto.BrowseReason {
	if !filepath.IsAbs(path) {
		return terminalproto.BrowseOutsideHome
	}
	cleanHome := filepath.Clean(home)
	// beneath splits an absolute path inside home (by either spelling) into
	// its components below the real home.
	beneath := func(p string) ([]string, bool) {
		p = filepath.Clean(p)
		for _, h := range []string{realHome, cleanHome} {
			if within(h, p) {
				rel, err := filepath.Rel(h, p)
				return splitPath(rel), err == nil
			}
		}
		return nil, false
	}
	rest, ok := beneath(path)
	if !ok {
		return terminalproto.BrowseOutsideHome
	}
	cur := realHome
	for hops := 0; len(rest) > 0; {
		c := rest[0]
		rest = rest[1:]
		switch c {
		case ".":
			continue
		case "..":
			if cur == realHome {
				return terminalproto.BrowseOutsideHome
			}
			cur = filepath.Dir(cur)
			continue
		}
		next := filepath.Join(cur, c)
		info, err := b.fsys.Lstat(next)
		if err != nil {
			return statReason(err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			if hops++; hops > maxBrowseHops {
				return terminalproto.BrowseUnreadable
			}
			target, err := os.Readlink(next)
			if err != nil {
				return terminalproto.BrowseUnreadable
			}
			if filepath.IsAbs(target) {
				parts, ok := beneath(target)
				if !ok {
					return terminalproto.BrowseOutsideHome
				}
				cur, rest = realHome, append(parts, rest...)
			} else {
				rest = append(splitPath(target), rest...)
			}
			continue
		}
		if !info.IsDir() {
			if len(rest) > 0 {
				return terminalproto.BrowseNotFound // a file used as a folder
			}
			return terminalproto.BrowseNotDir
		}
		cur = next
	}
	// The walk found a folder inside home that ResolveDir refused: it changed
	// in between, or could not be read.
	return terminalproto.BrowseUnreadable
}

// splitPath splits a relative path into its components, dropping empty ones.
func splitPath(p string) []string {
	var parts []string
	for _, part := range strings.Split(filepath.ToSlash(p), "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func statReason(err error) terminalproto.BrowseReason {
	if errors.Is(err, syscall.ENOTDIR) {
		return terminalproto.BrowseNotFound
	}
	return readReason(err)
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
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || c == '\u2028' || c == '\u2029' {
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
