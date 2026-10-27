package terminalhost

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// browseHome is a fresh home folder with symlinks resolved, as Browse
// reports it.
func browseHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func mkdirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// putFile writes a small file (browse never reads it).
func putFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// makeRepo makes dir a working tree top: a .git folder holding a HEAD file.
func makeRepo(t *testing.T, dir string) {
	t.Helper()
	mkdirs(t, dir, ".git")
	putFile(t, filepath.Join(dir, ".git", "HEAD"), "secret contents")
}

// requireEncodable fails unless r is a valid browse_result that fits a
// control frame.
func requireEncodable(t *testing.T, r terminalproto.BrowseResult) {
	t.Helper()
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: "browse-test-request-0001", Generation: 1, Body: r}); err != nil {
		t.Fatalf("browse result does not encode: %v (%+v)", err, r)
	}
}

func browseWith(t *testing.T, home string, req terminalproto.Browse, fsys FS) terminalproto.BrowseResult {
	t.Helper()
	r := Browse(context.Background(), home, req, fsys, time.Now)
	requireEncodable(t, r)
	return r
}

func browseOS(t *testing.T, home string, req terminalproto.Browse) terminalproto.BrowseResult {
	t.Helper()
	return browseWith(t, home, req, OSFS)
}

func entryNames(r terminalproto.BrowseResult) []string {
	names := []string{}
	for _, e := range r.Entries {
		names = append(names, e.Name)
	}
	return names
}

func requireNames(t *testing.T, r terminalproto.BrowseResult, want ...string) {
	t.Helper()
	if got := entryNames(r); !reflect.DeepEqual(got, append([]string{}, want...)) {
		t.Fatalf("entries = %q, want %q (result %+v)", got, want, r)
	}
}

func requireReason(t *testing.T, r terminalproto.BrowseResult, want terminalproto.BrowseReason) {
	t.Helper()
	if r.Reason != want {
		t.Fatalf("reason = %q, want %q (result %+v)", r.Reason, want, r)
	}
}

// The fakes below wrap another FS. Each passes rootAt through to it, so a
// browse over a fake wrapping OSFS still runs every call beneath os.Root.

// recordingFS records every call the browser makes.
type recordingFS struct {
	base   FS
	parent *recordingFS
	mu     sync.Mutex
	calls  []string
}

func (f *recordingFS) rootAt(dir string) (FS, func(), error) {
	base, done, err := rootBeneath(f.base, dir)
	return &recordingFS{base: base, parent: f}, done, err
}

func (f *recordingFS) record(op, path string) {
	if f.parent != nil {
		f.parent.record(op, path)
		return
	}
	f.mu.Lock()
	f.calls = append(f.calls, op+" "+path)
	f.mu.Unlock()
}
func (f *recordingFS) ReadDir(p string) ([]os.DirEntry, error) {
	f.record("readdir", p)
	return f.base.ReadDir(p)
}
func (f *recordingFS) Lstat(p string) (os.FileInfo, error) {
	f.record("lstat", p)
	return f.base.Lstat(p)
}
func (f *recordingFS) Stat(p string) (os.FileInfo, error) { f.record("stat", p); return f.base.Stat(p) }

// hookFS lets a test change what one call sees.
type hookFS struct {
	base    FS
	readDir func(base FS, p string) ([]os.DirEntry, error)
	lstat   func(base FS, p string) (os.FileInfo, error)
}

func (f hookFS) rootAt(dir string) (FS, func(), error) {
	base, done, err := rootBeneath(f.base, dir)
	f.base = base
	return f, done, err
}
func (f hookFS) ReadDir(p string) ([]os.DirEntry, error) {
	if f.readDir != nil {
		return f.readDir(f.base, p)
	}
	return f.base.ReadDir(p)
}
func (f hookFS) Lstat(p string) (os.FileInfo, error) {
	if f.lstat != nil {
		return f.lstat(f.base, p)
	}
	return f.base.Lstat(p)
}
func (f hookFS) Stat(p string) (os.FileInfo, error) { return f.base.Stat(p) }

// linkInfo reports a real file's info as a symbolic link.
type linkInfo struct{ os.FileInfo }

func (linkInfo) Mode() fs.FileMode { return fs.ModeSymlink | 0o777 }
func (linkInfo) IsDir() bool       { return false }

type fakeDirEntry struct {
	name string
	typ  fs.FileMode
}

func (e fakeDirEntry) Name() string               { return e.name }
func (e fakeDirEntry) IsDir() bool                { return e.typ.IsDir() }
func (e fakeDirEntry) Type() fs.FileMode          { return e.typ }
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrNotExist }

func TestBrowseListsOnlyFolders(t *testing.T) {
	home := browseHome(t)
	mkdirs(t, home, "beta", "alpha", "Gamma")
	putFile(t, filepath.Join(home, "notes.txt"), "secret contents")
	symlink(t, filepath.Join(home, "notes.txt"), filepath.Join(home, "notes-link"))

	r := browseOS(t, home, terminalproto.Browse{})
	requireNames(t, r, "alpha", "beta", "Gamma")
	if r.Path != home || r.Home != home || r.Parent != "" || r.Repo != terminalproto.BrowseRepoNone || r.RepoTop != "" {
		t.Fatalf("home browse = %+v", r)
	}
	if r.Scanned != 5 || r.Omitted != 2 || r.More || r.Partial {
		t.Fatalf("counts: scanned %d omitted %d more %v partial %v; want 5, 2 (the file and the link to it)", r.Scanned, r.Omitted, r.More, r.Partial)
	}
	for _, e := range r.Entries {
		if !e.Dir || e.Hidden || e.RepoRoot {
			t.Fatalf("entry %+v", e)
		}
	}

	sub := browseOS(t, home, terminalproto.Browse{Path: filepath.Join(home, "alpha")})
	if sub.Path != filepath.Join(home, "alpha") || sub.Parent != home || len(sub.Entries) != 0 || sub.Scanned != 0 {
		t.Fatalf("empty folder = %+v", sub)
	}
	requireReason(t, browseOS(t, home, terminalproto.Browse{Path: filepath.Join(home, "notes.txt")}), terminalproto.BrowseNotDir)
	requireReason(t, browseOS(t, home, terminalproto.Browse{Path: filepath.Join(home, "missing")}), terminalproto.BrowseNotFound)
}

func TestBrowseNeverOpensFiles(t *testing.T) {
	home := browseHome(t)
	mkdirs(t, home, "project", "docs")
	secret := filepath.Join(home, "secret.key")
	putFile(t, secret, "secret contents")
	putFile(t, filepath.Join(home, "project", "secret.env"), "secret contents")

	readable := browseOS(t, home, terminalproto.Browse{})
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	rec := &recordingFS{base: OSFS}
	unreadable := browseWith(t, home, terminalproto.Browse{}, rec)
	if !reflect.DeepEqual(readable, unreadable) {
		t.Fatalf("an unreadable file changed the answer:\n%+v\n%+v", readable, unreadable)
	}
	requireNames(t, unreadable, "docs", "project")
	if unreadable.Omitted != 1 {
		t.Fatalf("omitted = %d, want the one file", unreadable.Omitted)
	}
	for _, call := range rec.calls {
		if strings.Contains(call, "secret") {
			t.Fatalf("browse touched a file: %q (all calls %q)", call, rec.calls)
		}
		if strings.HasPrefix(call, "readdir ") && call != "readdir "+home {
			t.Fatalf("browse read a second folder: %q", call)
		}
		if strings.HasPrefix(call, "stat ") {
			t.Fatalf("browse followed a path with stat: %q", call)
		}
	}
}

func TestBrowseContainment(t *testing.T) {
	root := browseHome(t)
	home := filepath.Join(root, "home")
	outside := filepath.Join(root, "outside")
	mkdirs(t, home, "work/site")
	mkdirs(t, outside, "private-folder")
	mkdirs(t, root, "home-other")
	symlink(t, "/", filepath.Join(home, "to-root"))
	symlink(t, outside, filepath.Join(home, "to-sibling"))
	symlink(t, "..", filepath.Join(home, "to-parent"))
	symlink(t, filepath.Join(home, "work"), filepath.Join(home, "to-work"))
	symlink(t, "work/site", filepath.Join(home, "to-site"))
	symlink(t, ".", filepath.Join(home, "to-self"))

	t.Run("listing", func(t *testing.T) {
		r := browseOS(t, home, terminalproto.Browse{})
		requireNames(t, r, "to-self", "to-site", "to-work", "work")
		if r.Omitted != 3 {
			t.Fatalf("omitted = %d, want the three links leaving home", r.Omitted)
		}
	})
	t.Run("paths outside home", func(t *testing.T) {
		for _, path := range []string{
			home + "/..",
			home + "/work/../..",
			"/",
			root,
			outside,
			filepath.Join(outside, "private-folder"),
			filepath.Join(outside, "missing"),
			filepath.Join(root, "home-other"),
			filepath.Join(home, "to-root"),
			filepath.Join(home, "to-sibling"),
			filepath.Join(home, "to-sibling", "private-folder"),
			filepath.Join(home, "to-parent"),
			"relative/work",
		} {
			r := browseOS(t, home, terminalproto.Browse{Path: path})
			if r.Reason != terminalproto.BrowseOutsideHome {
				t.Fatalf("Browse(%q) = %+v, want outside_home", path, r)
			}
		}
	})
	t.Run("a link inside home is entered at its target", func(t *testing.T) {
		r := browseOS(t, home, terminalproto.Browse{Path: filepath.Join(home, "to-work")})
		if r.Path != filepath.Join(home, "work") || r.Parent != home {
			t.Fatalf("Browse(to-work) = %+v", r)
		}
		requireNames(t, r, "site")
		r = browseOS(t, home, terminalproto.Browse{Path: home + "/work/site/../.."})
		if r.Path != home || r.Reason != "" {
			t.Fatalf("Browse(work/site/../..) = %+v", r)
		}
	})
	t.Run("a link target swapped for a link after resolving is omitted", func(t *testing.T) {
		site := filepath.Join(home, "work", "site")
		fsys := hookFS{base: OSFS, lstat: func(base FS, p string) (os.FileInfo, error) {
			info, err := base.Lstat(p)
			if p == site && err == nil {
				return linkInfo{info}, nil
			}
			return info, err
		}}
		r := browseWith(t, home, terminalproto.Browse{}, fsys)
		requireNames(t, r, "to-self", "to-work", "work")
	})
	t.Run("the folder swapped for a link while reading", func(t *testing.T) {
		victim := filepath.Join(home, "victim")
		mkdirs(t, home, "victim")
		swapped := false
		fsys := hookFS{base: OSFS, readDir: func(_ FS, p string) ([]os.DirEntry, error) {
			if p == victim && !swapped {
				swapped = true
				if err := os.Rename(victim, victim+"-old"); err != nil {
					t.Fatal(err)
				}
				symlink(t, outside, victim)
			}
			// A filesystem that follows the swapped-in link.
			return os.ReadDir(p)
		}}
		r := browseWith(t, home, terminalproto.Browse{Path: victim}, fsys)
		if !swapped || r.Reason != terminalproto.BrowseOutsideHome || len(r.Entries) != 0 {
			t.Fatalf("a folder swapped for a link mid-browse leaked: %+v", r)
		}
	})
	t.Run("nothing outside home is told apart", func(t *testing.T) {
		mkdirs(t, outside, "exists/deeper")
		putFile(t, filepath.Join(outside, "file.txt"), "x")
		symlink(t, outside, filepath.Join(home, "out"))
		symlink(t, "out", filepath.Join(home, "hop"))
		symlink(t, "work/../out/exists", filepath.Join(home, "hop2"))
		symlink(t, filepath.Join(root, "home", "out"), filepath.Join(home, "abs-hop"))
		for _, path := range []string{
			filepath.Join(home, "to-root", "etc"),
			filepath.Join(home, "to-root", "etc", "passwd"),
			filepath.Join(home, "to-root", "etc", "definitely-not-here"),
			filepath.Join(home, "to-root", "root", ".ssh"),
			filepath.Join(home, "to-root", "root", ".nope"),
			filepath.Join(home, "out", "exists"),
			filepath.Join(home, "out", "exists", "deeper"),
			filepath.Join(home, "out", "nope"),
			filepath.Join(home, "out", "file.txt"),
			filepath.Join(home, "out", "file.txt", "x"),
			filepath.Join(home, "hop", "nope"),
			filepath.Join(home, "hop", "exists"),
			filepath.Join(home, "hop2"),
			filepath.Join(home, "hop2", "nope"),
			filepath.Join(home, "abs-hop", "nope"),
			filepath.Join(home, "to-parent", "home-other"),
			filepath.Join(home, "to-parent", "nope"),
			filepath.Join(home, "work", "..", "to-parent", "nope"),
		} {
			r := browseOS(t, home, terminalproto.Browse{Path: path})
			if r.Reason != terminalproto.BrowseOutsideHome {
				t.Errorf("Browse(%q) = %q, want outside_home whether or not it exists", path, r.Reason)
			}
		}
		putFile(t, filepath.Join(home, "work", "f.txt"), "x")
		for path, want := range map[string]terminalproto.BrowseReason{
			filepath.Join(home, "work", "missing"):          terminalproto.BrowseNotFound,
			filepath.Join(home, "to-work", "missing"):       terminalproto.BrowseNotFound,
			filepath.Join(home, "to-work", "missing", "x"):  terminalproto.BrowseNotFound,
			filepath.Join(home, "to-site", "..", "missing"): terminalproto.BrowseNotFound,
			filepath.Join(home, "work", "f.txt"):            terminalproto.BrowseNotDir,
			filepath.Join(home, "to-work", "f.txt"):         terminalproto.BrowseNotDir,
			filepath.Join(home, "work", "f.txt", "x"):       terminalproto.BrowseNotFound,
		} {
			if r := browseOS(t, home, terminalproto.Browse{Path: path}); r.Reason != want {
				t.Errorf("Browse(%q) = %q, want %q", path, r.Reason, want)
			}
		}
	})
	t.Run("an intermediate folder swapped for an outside link while reading", func(t *testing.T) {
		mkdirs(t, home, "mid/b")
		mkdirs(t, outside, "b/SECRET-NAME")
		mid := filepath.Join(home, "mid")
		fsys := hookFS{base: OSFS, readDir: func(base FS, p string) ([]os.DirEntry, error) {
			// Swap mid for a link out, read, and swap it back before any
			// re-check can see it.
			if err := os.Rename(mid, mid+".real"); err != nil {
				t.Fatal(err)
			}
			symlink(t, outside, mid)
			defer func() {
				if err := os.Remove(mid); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(mid+".real", mid); err != nil {
					t.Fatal(err)
				}
			}()
			return base.ReadDir(p)
		}}
		r := browseWith(t, home, terminalproto.Browse{Path: filepath.Join(mid, "b")}, fsys)
		for _, e := range r.Entries {
			if e.Name == "SECRET-NAME" {
				t.Fatalf("listed a folder outside home through a swapped component: %+v", r)
			}
		}
	})
	t.Run("an entry swapped for an outside repository", func(t *testing.T) {
		mkdirs(t, home, "swapme")
		makeRepo(t, filepath.Join(outside, "outrepo"))
		entry := filepath.Join(home, "swapme")
		fsys := hookFS{base: OSFS, readDir: func(base FS, p string) ([]os.DirEntry, error) {
			list, err := base.ReadDir(p)
			if p == home {
				if err := os.Remove(entry); err != nil {
					t.Fatal(err)
				}
				symlink(t, filepath.Join(outside, "outrepo"), entry)
			}
			return list, err
		}}
		r := browseWith(t, home, terminalproto.Browse{}, fsys)
		for _, e := range r.Entries {
			if e.Name == "swapme" && e.RepoRoot {
				t.Fatalf("read a repository bit outside home through a swapped entry: %+v", r.Entries)
			}
		}
		if err := os.Remove(entry); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a path out through a link and back with .. tells nothing", func(t *testing.T) {
		putFile(t, filepath.Join(outside, "file2"), "x")
		fromRoot := strings.TrimPrefix(home, "/")
		back, err := filepath.Rel(outside, home)
		if err != nil {
			t.Fatal(err)
		}
		for _, group := range [][]string{
			{
				home + "/to-root/etc/../" + fromRoot,
				home + "/to-root/definitely-not-here/../" + fromRoot,
				home + "/to-root/etc/passwd/../../" + fromRoot,
			},
			{
				home + "/to-sibling/private-folder/../" + back,
				home + "/to-sibling/nope/../" + back,
				home + "/to-sibling/file2/../" + back,
			},
			{
				home + "/to-sibling/private-folder/../" + back + "/work",
				home + "/to-sibling/nope/../" + back + "/work",
				home + "/to-sibling/file2/../" + back + "/work",
			},
		} {
			first := browseOS(t, home, terminalproto.Browse{Path: group[0]})
			for _, path := range group[1:] {
				if r := browseOS(t, home, terminalproto.Browse{Path: path}); !reflect.DeepEqual(r, first) {
					t.Errorf("Browse(%q) = %+v\nbut Browse(%q) = %+v: whether a path outside home exists is told", path, r, group[0], first)
				}
			}
		}
	})
}

func TestBrowseRepoDetection(t *testing.T) {
	root := browseHome(t)
	makeRepo(t, root) // the folder above home is a repository; it must never be seen
	home := filepath.Join(root, "home")
	mkdirs(t, home, "plain/deeper", "repo/src/deep", "worktree", "nohead/.git", "headdir/.git/HEAD", "linkgit")
	makeRepo(t, filepath.Join(home, "repo"))
	putFile(t, filepath.Join(home, "worktree", ".git"), "secret contents") // a linked worktree's .git file
	symlink(t, filepath.Join(home, "repo", ".git"), filepath.Join(home, "linkgit", ".git"))
	symlink(t, filepath.Join(home, "repo"), filepath.Join(home, "repo-link"))

	rec := &recordingFS{base: OSFS}
	r := browseWith(t, home, terminalproto.Browse{}, rec)
	requireNames(t, r, "headdir", "linkgit", "nohead", "plain", "repo", "repo-link", "worktree")
	roots := map[string]bool{}
	for _, e := range r.Entries {
		roots[e.Name] = e.RepoRoot
	}
	want := map[string]bool{"headdir": false, "linkgit": false, "nohead": false, "plain": false, "repo": true, "repo-link": true, "worktree": false}
	if !reflect.DeepEqual(roots, want) {
		t.Fatalf("repoRoot = %v, want %v", roots, want)
	}
	if r.Repo != terminalproto.BrowseRepoNone || r.RepoTop != "" {
		t.Fatalf("home under a repository must read none: %+v", r)
	}

	for path, want := range map[string]struct {
		repo terminalproto.BrowseRepo
		top  string
	}{
		filepath.Join(home, "repo"):                {terminalproto.BrowseRepoRoot, filepath.Join(home, "repo")},
		filepath.Join(home, "repo-link"):           {terminalproto.BrowseRepoRoot, filepath.Join(home, "repo")},
		filepath.Join(home, "repo", "src"):         {terminalproto.BrowseRepoInside, filepath.Join(home, "repo")},
		filepath.Join(home, "repo", "src", "deep"): {terminalproto.BrowseRepoInside, filepath.Join(home, "repo")},
		filepath.Join(home, "plain", "deeper"):     {terminalproto.BrowseRepoNone, ""},
		filepath.Join(home, "worktree"):            {terminalproto.BrowseRepoNone, ""},
		filepath.Join(home, "linkgit"):             {terminalproto.BrowseRepoNone, ""},
		home:                                       {terminalproto.BrowseRepoNone, ""},
	} {
		got := browseWith(t, home, terminalproto.Browse{Path: path}, rec)
		if got.Repo != want.repo || got.RepoTop != want.top {
			t.Fatalf("Browse(%q) repo = %q %q, want %q %q", path, got.Repo, got.RepoTop, want.repo, want.top)
		}
	}
	for _, call := range rec.calls {
		p := call[strings.IndexByte(call, ' ')+1:]
		if p != home && !strings.HasPrefix(p, home+"/") {
			t.Fatalf("browse looked above home: %q", call)
		}
		if strings.HasPrefix(call, "stat ") {
			t.Fatalf("repository detection followed a link: %q", call)
		}
	}

	t.Run("home itself a repository", func(t *testing.T) {
		makeRepo(t, home)
		got := browseOS(t, home, terminalproto.Browse{Path: filepath.Join(home, "plain", "deeper")})
		if got.Repo != terminalproto.BrowseRepoInside || got.RepoTop != home {
			t.Fatalf("inside a repository at home = %+v", got)
		}
		got = browseOS(t, home, terminalproto.Browse{})
		if got.Repo != terminalproto.BrowseRepoRoot || got.RepoTop != home {
			t.Fatalf("home a repository = %+v", got)
		}
	})
}

func TestBrowseHiddenAndFilter(t *testing.T) {
	home := browseHome(t)
	mkdirs(t, home, ".config", "reet", "Other", ".reet-cache")

	r := browseOS(t, home, terminalproto.Browse{})
	requireNames(t, r, "Other", "reet")
	if r.Omitted != 0 {
		t.Fatalf("hidden folders are not omitted entries: %d", r.Omitted)
	}
	r = browseOS(t, home, terminalproto.Browse{Hidden: true})
	requireNames(t, r, ".config", ".reet-cache", "Other", "reet")
	if !r.Entries[0].Hidden || !r.Entries[1].Hidden || r.Entries[2].Hidden {
		t.Fatalf("hidden flags %+v", r.Entries)
	}
	requireNames(t, browseOS(t, home, terminalproto.Browse{Filter: "REE"}), "reet")
	requireNames(t, browseOS(t, home, terminalproto.Browse{Filter: "REE", Hidden: true}), ".reet-cache", "reet")
	requireNames(t, browseOS(t, home, terminalproto.Browse{Filter: "nothing"}))
}

func TestBrowsePaging(t *testing.T) {
	home := browseHome(t)
	var all []string
	for i := 0; i < 450; i++ {
		name := "folder-" + strings.Repeat("0", 3-len(strconv.Itoa(i))) + strconv.Itoa(i)
		all = append(all, name)
		mkdirs(t, home, name)
	}
	var got []string
	after := ""
	for page, want := range []struct {
		n    int
		more bool
	}{{200, true}, {200, true}, {50, false}} {
		r := browseOS(t, home, terminalproto.Browse{After: after})
		if len(r.Entries) != want.n || r.More != want.more || r.Scanned != 450 {
			t.Fatalf("page %d: %d entries, more %v, scanned %d; want %d, %v, 450", page, len(r.Entries), r.More, r.Scanned, want.n, want.more)
		}
		got = append(got, entryNames(r)...)
		after = r.Entries[len(r.Entries)-1].Name
	}
	if !reflect.DeepEqual(got, all) {
		t.Fatal("pages did not list every folder once, in order")
	}
}

func TestBrowseEntryCap(t *testing.T) {
	home := browseHome(t)
	for i := 0; i < 6000; i++ {
		mkdirs(t, home, "d"+strconv.Itoa(i))
	}
	r := browseOS(t, home, terminalproto.Browse{})
	if r.Scanned != 5000 || !r.More || len(r.Entries) != 200 {
		t.Fatalf("scanned %d more %v entries %d; want 5000, true, 200", r.Scanned, r.More, len(r.Entries))
	}
	// A client that follows More and After finishes: the cap is shown by
	// Scanned, never by a More with no name to continue from.
	seen, after := 0, ""
	for pages := 1; ; pages++ {
		r := browseOS(t, home, terminalproto.Browse{After: after})
		if r.Reason != "" || r.More && len(r.Entries) == 0 {
			t.Fatalf("page %d after %d folders: %+v", pages, seen, r)
		}
		seen += len(r.Entries)
		if !r.More {
			break
		}
		after = r.Entries[len(r.Entries)-1].Name
		if pages > 40 {
			t.Fatal("paging past the cap does not end")
		}
	}
	if seen != 5000 {
		t.Fatalf("paged %d folders, want the 5000 scanned", seen)
	}
	// A filesystem that hands back more than the cap is cut to it too.
	many := make([]os.DirEntry, 6000)
	for i := range many {
		many[i] = fakeDirEntry{name: "f" + strconv.Itoa(i)}
	}
	r = browseWith(t, home, terminalproto.Browse{}, hookFS{base: OSFS, readDir: func(FS, string) ([]os.DirEntry, error) { return many, nil }})
	if r.Scanned != 5000 || r.More || r.Omitted != 5000 || len(r.Entries) != 0 {
		t.Fatalf("over-long listing: scanned %d more %v omitted %d", r.Scanned, r.More, r.Omitted)
	}
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// slowFS makes every lstat take step on the fake clock and counts the calls
// made after the clock passed limit.
type slowFS struct {
	base  FS
	clock *fakeClock
	step  time.Duration
	limit time.Time
	late  int
}

func (f *slowFS) rootAt(dir string) (FS, func(), error) {
	base, done, err := rootBeneath(f.base, dir)
	f.base = base
	return f, done, err
}
func (f *slowFS) ReadDir(p string) ([]os.DirEntry, error) { return f.base.ReadDir(p) }
func (f *slowFS) Stat(p string) (os.FileInfo, error)      { return f.base.Stat(p) }
func (f *slowFS) Lstat(p string) (os.FileInfo, error) {
	if !f.clock.now().Before(f.limit) {
		f.late++
	}
	f.clock.advance(f.step)
	return f.base.Lstat(p)
}

func TestBrowseDeadline(t *testing.T) {
	home := browseHome(t)
	var names []string
	for i := 0; i < 10; i++ {
		name := "repo" + strconv.Itoa(i)
		names = append(names, name)
		makeRepo(t, filepath.Join(home, name))
	}
	symlink(t, filepath.Join(home, "repo0"), filepath.Join(home, "zz-link"))

	start := time.Unix(1_800_000_000, 0)
	clock := &fakeClock{t: start}
	fsys := &slowFS{base: OSFS, clock: clock, step: 400 * time.Millisecond, limit: start.Add(3 * time.Second)}
	r := Browse(context.Background(), home, terminalproto.Browse{}, fsys, clock.now)
	requireEncodable(t, r)
	requireNames(t, r, names...)
	if !r.Partial || !r.More || r.Omitted != 0 {
		t.Fatalf("partial %v more %v omitted %d; want a partial page that stops at the unchecked link", r.Partial, r.More, r.Omitted)
	}
	if !r.Entries[0].RepoRoot || r.Entries[len(r.Entries)-1].RepoRoot {
		t.Fatalf("repoRoot checks before the deadline report true, after it false: %+v", r.Entries)
	}
	if fsys.late > 1 {
		t.Fatalf("%d filesystem calls after the deadline", fsys.late)
	}
	next := browseOS(t, home, terminalproto.Browse{After: r.Entries[len(r.Entries)-1].Name})
	requireNames(t, next, "zz-link")
	if next.More {
		t.Fatalf("last page %+v", next)
	}

	t.Run("an earlier context deadline wins", func(t *testing.T) {
		begin := time.Now()
		clock := &fakeClock{t: begin}
		fsys := &slowFS{base: OSFS, clock: clock, step: 400 * time.Millisecond, limit: begin.Add(time.Second)}
		ctx, cancel := context.WithDeadline(context.Background(), begin.Add(time.Second))
		defer cancel()
		r := Browse(ctx, home, terminalproto.Browse{}, fsys, clock.now)
		requireEncodable(t, r)
		checked := 0
		for _, e := range r.Entries {
			if e.RepoRoot {
				checked++
			}
		}
		// Checking home's own repository ends at 0.4 s, repo0 at 1.2 s; then
		// the 1 s context deadline has passed.
		if checked != 1 || !r.Partial {
			t.Fatalf("%d repoRoot checks with a 1 s context deadline, want 1 (%+v)", checked, r)
		}
	})
	t.Run("a cancelled context answers at once", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		rec := &recordingFS{base: OSFS}
		r := Browse(ctx, filepath.Join(home), terminalproto.Browse{Path: filepath.Join(home, "repo1")}, rec, time.Now)
		requireEncodable(t, r)
		if !r.Partial || r.Repo != terminalproto.BrowseRepoNone {
			t.Fatalf("cancelled browse = %+v", r)
		}
		for _, call := range rec.calls {
			if strings.Contains(call, ".git") {
				t.Fatalf("checked a repository after the context ended: %q", call)
			}
		}
	})
	t.Run("a late page still makes progress", func(t *testing.T) {
		home := browseHome(t)
		mkdirs(t, home, "target")
		symlink(t, filepath.Join(home, "target"), filepath.Join(home, "a-link"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := Browse(ctx, home, terminalproto.Browse{}, OSFS, time.Now)
		requireEncodable(t, r)
		requireNames(t, r, "a-link", "target")
		if !r.Partial || r.More {
			t.Fatalf("late first link = %+v", r)
		}
	})
}

func TestBrowseUnusualNames(t *testing.T) {
	home := browseHome(t)
	long := strings.Repeat("n", 255)
	listed := []string{" leading space", "#tag", "-rf", "it's", "my folder", long, `say "hi"`, "ünïcødé"}
	mkdirs(t, home, listed...)
	bad := 0
	for _, name := range []string{"new\nline", "\xff\xfe", "tab\there", "\u202eevil", "zero\u200bwidth"} {
		err := os.Mkdir(filepath.Join(home, name), 0o700)
		if err != nil && !utf8.ValidString(name) && (errors.Is(err, syscall.EILSEQ) || errors.Is(err, syscall.EINVAL)) {
			// APFS refuses names that are not UTF-8; Linux keeps the case.
			t.Logf("this filesystem refuses %q (%v); that case is skipped", name, err)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		bad++
	}
	invalidUTF8 := bad == 5
	r := browseOS(t, home, terminalproto.Browse{})
	requireNames(t, r, listed...)
	if r.Omitted != bad || r.Scanned != len(listed)+bad {
		t.Fatalf("omitted %d scanned %d; want %d and %d", r.Omitted, r.Scanned, bad, len(listed)+bad)
	}

	// Names longer than 255 bytes (possible on some filesystems) are omitted.
	fsys := hookFS{base: OSFS, readDir: func(FS, string) ([]os.DirEntry, error) {
		return []os.DirEntry{fakeDirEntry{name: strings.Repeat("x", 256), typ: fs.ModeDir}, fakeDirEntry{name: "ok", typ: fs.ModeDir}, fakeDirEntry{name: "a/b", typ: fs.ModeDir}}, nil
	}}
	r = browseWith(t, home, terminalproto.Browse{}, fsys)
	requireNames(t, r, "ok")
	if r.Omitted != 2 {
		t.Fatalf("omitted %d; want the over-long and slashed names", r.Omitted)
	}

	r = browseOS(t, home, terminalproto.Browse{Path: filepath.Join(home, `say "hi"`)})
	if r.Path != filepath.Join(home, `say "hi"`) {
		t.Fatalf("Browse into an unusual name = %+v", r)
	}
	// A folder whose own path cannot be sent is refused, not mangled.
	if invalidUTF8 {
		requireReason(t, browseOS(t, home, terminalproto.Browse{Path: filepath.Join(home, "\xff\xfe")}), terminalproto.BrowseUnreadable)
	}
}

func TestBrowseSymlinkLoop(t *testing.T) {
	home := browseHome(t)
	mkdirs(t, home, "real")
	symlink(t, filepath.Join(home, "b"), filepath.Join(home, "a"))
	symlink(t, filepath.Join(home, "a"), filepath.Join(home, "b"))
	symlink(t, "self", filepath.Join(home, "self"))

	began := time.Now()
	r := browseOS(t, home, terminalproto.Browse{})
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("a symlink loop took %v", took)
	}
	requireNames(t, r, "real")
	if r.Omitted != 3 {
		t.Fatalf("omitted %d; want the three looping links", r.Omitted)
	}
	got := browseOS(t, home, terminalproto.Browse{Path: filepath.Join(home, "a")})
	if got.Reason == "" {
		t.Fatalf("Browse into a loop = %+v", got)
	}
}

func TestBrowseReplyFitsFrame(t *testing.T) {
	home := browseHome(t)
	var all []string
	for i := 0; i < 250; i++ {
		name := strings.Repeat("0", 3-len(strconv.Itoa(i))) + strconv.Itoa(i) + strings.Repeat("x", 252)
		all = append(all, name)
		makeRepo(t, filepath.Join(home, name))
	}
	// json escapes <, > and & as six bytes each: the widest names there are.
	for i := 0; i < 30; i++ {
		name := "~" + strings.Repeat("0", 2-len(strconv.Itoa(i))) + strconv.Itoa(i) + strings.Repeat("<", 252)
		all = append(all, name)
		mkdirs(t, home, name)
	}
	var got []string
	after, pages := "", 0
	for {
		r := browseOS(t, home, terminalproto.Browse{After: after})
		pages++
		if len(r.Entries) == 0 || len(r.Entries) > terminalproto.MaxBrowseEntries {
			t.Fatalf("page %d has %d entries", pages, len(r.Entries))
		}
		got = append(got, entryNames(r)...)
		after = r.Entries[len(r.Entries)-1].Name
		if !r.More {
			break
		}
		if pages > 100 {
			t.Fatal("paging does not end")
		}
	}
	if !reflect.DeepEqual(got, all) {
		t.Fatalf("pages listed %d names, want all %d once in order", len(got), len(all))
	}
	if pages < 8 {
		t.Fatalf("only %d pages: long names were not split", pages)
	}

	t.Run("a deep folder whose own path fills the reply", func(t *testing.T) {
		home := browseHome(t)
		d, reached, short := home, 0, false
		for depth := 1; depth <= 14 && !short; depth++ {
			d = filepath.Join(d, strings.Repeat("<", 250)+strconv.Itoa(depth%10))
			if err := os.Mkdir(d, 0o700); errors.Is(err, syscall.ENAMETOOLONG) {
				// macOS has a 1,024-byte PATH_MAX; Linux keeps the full case.
				t.Logf("paths stop at depth %d here: %v", depth, err)
				short = true
				break
			} else if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(d, strings.Repeat("&", 255)), 0o700); errors.Is(err, syscall.ENAMETOOLONG) {
				t.Logf("no child fits at depth %d here: %v", depth, err)
				short = true // browse this one without its child, then stop
			} else if err != nil {
				t.Fatal(err)
			}
			reached = depth
			r := browseOS(t, home, terminalproto.Browse{Path: d})
			t.Logf("depth %d: %d bytes reason %q entries %d omitted %d more %v", depth, len(d), r.Reason, len(r.Entries), r.Omitted, r.More)
			if r.More && len(r.Entries) == 0 {
				t.Fatalf("depth %d (%d-byte path): More with no entries", depth, len(d))
			}
			// While the folder itself can be answered, it is: a child too wide
			// for any page is omitted, not left as an endless More.
			bare := terminalproto.BrowseResult{Path: d, Home: home, Parent: filepath.Dir(d), Repo: terminalproto.BrowseRepoNone, Scanned: 1, Omitted: 1}
			if browseResultValid(bare) && r.Reason != "" {
				t.Fatalf("depth %d (%d-byte path): %q, though the folder fits a reply", depth, len(d), r.Reason)
			}
		}
		if reached < 3 || runtime.GOOS == "linux" && (short || reached != 14) {
			t.Fatalf("built only %d levels (short %v); want 3 anywhere and all 14 with children on Linux", reached, short)
		}
	})
}

// passFS hides rootAt: a wrapper that cannot be rooted.
type passFS struct{ FS }

// A filesystem that cannot be confined beneath home is refused, never used
// unconfined.
func TestBrowseFailsClosedWithoutRoot(t *testing.T) {
	home := browseHome(t)
	mkdirs(t, home, "folder")
	requireReason(t, browseWith(t, home, terminalproto.Browse{}, passFS{OSFS}), terminalproto.BrowseUnreadable)
	requireNames(t, browseWith(t, home, terminalproto.Browse{}, hookFS{base: OSFS}), "folder")
}

// Source files hold no literal control, format or bidi characters (U+202E,
// U+200B, U+2028 and the like): they are written as escapes, so what a
// reviewer reads is what the compiler sees.
func TestPackageSourceHasNoInvisibleCharacters(t *testing.T) {
	for _, dir := range []string{".", "../terminalproto"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatal(dir, err)
		}
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			line := 1
			for _, c := range string(data) {
				if c == '\n' {
					line++
					continue
				}
				if c != '\t' && unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || c == '\u2028' || c == '\u2029' {
					t.Errorf("%s:%d holds a literal %U; write it as an escape", file, line, c)
				}
			}
		}
	}
}
