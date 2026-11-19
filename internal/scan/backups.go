package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/filelock"
)

// backupRetentionDays is how many days a backed-up skill is kept before the
// agent prunes it.
const backupRetentionDays = 30

const backupDateLayout = "2006-01-02"

// backupFolders are the four skills folders the agent manages, relative to
// the home directory, in the order backups are listed within one date.
var backupFolders = []string{
	".agents/skills",
	".claude/skills",
	".cursor/skills",
	".config/opencode/skills",
}

type backupEntry struct {
	Name, Folder, Path, Date string
	At                       int64  // Unix ms the backup was taken; 0 when unknown
	legacy                   bool   // lives in .reet-sync-backups/removed-*
	index                    string // base name in the date folder's index.json
}

const backupIndexFile = "index.json"

type indexRecord struct {
	Name   string `json:"name"`
	Folder string `json:"folder"`
	At     int64  `json:"at"`
}

type backupIndex struct {
	Entries map[string]indexRecord `json:"entries"`
}

// readBackupIndex returns the date folder's index; a missing or corrupt
// index reads as empty so listing falls back to parsing entry names.
func readBackupIndex(dir string) backupIndex {
	var idx backupIndex
	b, err := os.ReadFile(filepath.Join(dir, backupIndexFile))
	if err != nil || json.Unmarshal(b, &idx) != nil || idx.Entries == nil {
		return backupIndex{Entries: map[string]indexRecord{}}
	}
	return idx
}

// backupIndexLock is the lock file for the index.json of the date folder
// dir, in the .reet-backups folder above it, where listing and pruning, which
// read only date folders, never take it for a backup.
func backupIndexLock(dir string) string {
	return filepath.Join(filepath.Dir(dir), ".index.lock")
}

// updateBackupIndex reads the index of the date folder dir, changes it and
// writes it back while holding its lock, so a backup and a restore in two
// processes (the agent and reet agent restore) do not lose each other's
// entries. It waits a few seconds for the lock, then goes ahead without it:
// listing falls back to an entry's own name, so a lost record is never a
// lost backup.
func updateBackupIndex(dir string, change func(*backupIndex) bool) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := filelock.Acquire(backupIndexLock(dir))
		if err == nil {
			defer f.Close()
			break
		}
		if !errors.Is(err, filelock.ErrLocked) || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	idx := readBackupIndex(dir)
	if !change(&idx) {
		return nil
	}
	return writeBackupIndex(dir, idx)
}

func writeBackupIndex(dir string, idx backupIndex) error {
	b, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".index-*.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, backupIndexFile))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

func backupRoot(folder string) string {
	return filepath.Join(filepath.Dir(folder), ".reet-backups")
}

func backupPrefix(folder string) string {
	return filepath.Base(filepath.Dir(folder)) + "-"
}

func validBackupName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`) && !strings.ContainsRune(name, 0)
}

func validBackupDate(date string) bool {
	t, err := time.Parse(backupDateLayout, date)
	return err == nil && t.Format(backupDateLayout) == date
}

// backupSkill moves folder/name into a dated backup beside the folder and
// returns where it went. A symlink moves as a symlink. A path comes back with
// an error when the backup is whole but the original is still partly there.
func backupSkill(folder, name string, now time.Time) (string, error) {
	if !validBackupName(name) {
		return "", fmt.Errorf("refusing to back up %q: not a plain skill name", name)
	}
	src := filepath.Join(folder, name)
	if _, err := os.Lstat(src); err != nil {
		return "", err
	}
	dir := filepath.Join(backupRoot(folder), now.Format(backupDateLayout))
	// List and prune ignore a .reet-backups or date folder that is a link, so
	// a backup made through one could never be found again.
	for _, d := range []string{backupRoot(folder), dir} {
		if err := ensureRealDir(d); err != nil {
			return "", err
		}
	}
	prefix := backupPrefix(folder)
	dest := filepath.Join(dir, backupEntryName(prefix, name, 1))
	for n := 2; ; n++ {
		if _, err := os.Lstat(dest); os.IsNotExist(err) {
			break
		} else if err != nil {
			return "", err
		}
		dest = filepath.Join(dir, backupEntryName(prefix, name, n))
	}
	// A move across devices that copied all of the skill but could not remove
	// all of the original still made a whole backup: it is indexed and its
	// path returned with the error, so the record of it is not lost.
	var left *sourceLeftError
	moveErr := moveEntry(src, dest)
	if moveErr != nil && !errors.As(moveErr, &left) {
		return "", moveErr
	}
	err := updateBackupIndex(dir, func(idx *backupIndex) bool {
		idx.Entries[filepath.Base(dest)] = indexRecord{Name: name, Folder: folder, At: now.UnixMilli()}
		return true
	})
	if err != nil {
		// The skill is safely in its backup, and listing falls back to the
		// entry's name, so this is a warning and not a failure.
		log.Printf("backed up to %s but could not record its name: %v", dest, err)
	}
	return dest, moveErr
}

// maxEntryName is the longest name a file system takes for one entry.
const maxEntryName = 255

// backupEntryName names the n-th backup of name in a date folder, n being 1
// for the first: <prefix><name>, then <prefix><name>-2 and so on. When that
// would be too long for a file name, the name is cut short, on a character
// boundary, and ends in a hash of the whole name, so it still names this
// skill alone. index.json keeps the real name for listing and restore.
func backupEntryName(prefix, name string, n int) string {
	suffix := ""
	if n > 1 {
		suffix = "-" + strconv.Itoa(n)
	}
	if len(prefix)+len(name)+len(suffix) <= maxEntryName {
		return prefix + name + suffix
	}
	sum := sha256.Sum256([]byte(name))
	tag := "-" + hex.EncodeToString(sum[:6])
	keep := max(maxEntryName-len(prefix)-len(tag)-len(suffix), 0)
	for keep > 0 && !utf8.RuneStart(name[keep]) {
		keep--
	}
	return prefix + name[:keep] + tag + suffix
}

// ensureRealDir makes dir, one level, when it is missing, and refuses it when
// something else than a real directory is there. A link is not followed.
func ensureRealDir(dir string) error {
	st, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err = os.Mkdir(dir, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		st, err = os.Lstat(dir)
	}
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("refusing to back up into %s: it is not a real folder", dir)
	}
	return nil
}

// moveEntry renames src to dest, falling back to copy-then-remove when the
// two are on different devices.
func moveEntry(src, dest string) error {
	_, err := moveEntryCopying(src, dest)
	return err
}

// moveEntryCopying is moveEntry, also saying whether it had to copy.
func moveEntryCopying(src, dest string) (bool, error) {
	err := renameEntry(src, dest)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return false, err
	}
	if err = copyEntry(src, dest); err != nil {
		return true, err
	}
	if err = removeAllEntry(src); err != nil {
		return true, &sourceLeftError{err}
	}
	return true, nil
}

// removeAllEntry removes what a move across devices copied; tests replace it.
var removeAllEntry = os.RemoveAll

// sourceLeftError is a move across devices that copied the whole entry to
// its destination but could not then remove all of the original.
type sourceLeftError struct{ err error }

func (e *sourceLeftError) Error() string {
	return "copied, but the original could not all be removed: " + e.err.Error()
}
func (e *sourceLeftError) Unwrap() error { return e.err }

// copyEntry copies a file, directory tree or symlink to dest; symlinks stay
// links. The copy is made in a hidden .reet-copy-* folder beside dest and
// moved in only when it is complete and nothing is at dest by then, so dest
// never holds a partial copy and whatever another process puts there is left
// alone. On a failure only that hidden folder is removed. Its name does not
// grow with dest's, so any name dest's folder can hold can be copied to.
func copyEntry(src, dest string) error {
	taken := func() error {
		_, err := os.Lstat(dest)
		if err == nil {
			return &os.PathError{Op: "copy", Path: dest, Err: fs.ErrExist}
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := taken(); err != nil {
		return err
	}
	aside, err := os.MkdirTemp(filepath.Dir(dest), ".reet-copy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(aside)
	copied := filepath.Join(aside, "copy")
	if err := copyTree(src, copied); err != nil {
		return err
	}
	if err := taken(); err != nil {
		return err
	}
	return copyRename(copied, dest)
}

// copyTree copies src to dest, a path nothing else uses.
func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if copyEntryStep != nil && p != src {
			copyEntryStep()
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.IsDir():
			return os.Mkdir(target, info.Mode().Perm()|0700)
		case info.Mode().IsRegular():
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
			if err != nil {
				return err
			}
			if _, err = io.Copy(out, in); err != nil {
				out.Close()
				return err
			}
			return out.Close()
		}
		return fmt.Errorf("cannot back up %s: unsupported file type", p)
	})
}

// renameEntry is a rename that never replaces what is at the destination;
// tests replace it to stand for two devices.
var renameEntry = renameNoReplace

// copyEntryStep, when set, runs before each entry under the top one is
// copied; tests use it to stand for another process at work meanwhile.
var copyEntryStep func()

func isRealDir(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.IsDir()
}

// dateFolders lists the YYYY-MM-DD folders under a .reet-backups root.
// Anything else there is ignored.
func dateFolders(root string) []string {
	if !isRealDir(root) {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dates []string
	for _, e := range entries {
		if e.IsDir() && validBackupDate(e.Name()) {
			dates = append(dates, e.Name())
		}
	}
	return dates
}

// pruneBackups removes dated backup folders older than backupRetention from
// the .reet-backups beside each managed folder under home that a pass works
// in (see managedRoots, given the enrolled projects as unmanaged). Beside a
// folder reet leaves alone, such as one whose parent links into a project,
// .reet-backups lies in someone else's folder, and nothing there is pruned.
// Nor is anything beside a folder handled under another that is the same
// folder: when the link is at the parent, its .reet-backups is the handled
// one's under another name, where keep is not recognised. A date folder
// holding one of keep, a backup the controller has not heard of yet, is
// kept however old it is.
func pruneBackups(home string, now time.Time, unmanaged []string, keep ...string) error {
	// A date folder goes once its whole day is more than 30 days gone, so a
	// backup made late in the day is kept as long as one made early in it.
	y, m, d := now.Date()
	cutoff := time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -backupRetentionDays)
	held := map[string]bool{}
	for _, p := range keep {
		held[filepath.Dir(filepath.Clean(p))] = true
	}
	roots := managedRoots(home, unmanaged)
	var errs []error
	for i, f := range managedFolders {
		if !roots[i].handled() {
			continue
		}
		folder := filepath.Join(home, filepath.FromSlash(f.Dir))
		root := backupRoot(folder)
		for _, date := range dateFolders(root) {
			day, err := time.ParseInLocation(backupDateLayout, date, now.Location())
			if err != nil || !day.Before(cutoff) || held[filepath.Join(root, date)] {
				continue
			}
			// RemoveAll unlinks symlinks inside without following them.
			if err := os.RemoveAll(filepath.Join(root, date)); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// listBackups returns every backup under home, newest date first: the
// dated .reet-backups folders and the older .reet-sync-backups/removed-*
// folders, for the managed skills folders only. It fails if any of them
// cannot be read, so a restore never takes an older backup for want of a
// newer one it could not see.
func listBackups(home string) ([]backupEntry, error) {
	return backupsBeside(homeBackupFolders(home))
}

// listBackupsEach is listBackups that carries on past a backups folder it
// cannot read, as --list does: what could be read is listed, and each
// folder that could not is returned.
func listBackupsEach(home string) ([]backupEntry, []unreadableBackups) {
	return backupsBesideEach(homeBackupFolders(home))
}

func homeBackupFolders(home string) []string {
	var folders []string
	for _, rel := range backupFolders {
		folders = append(folders, filepath.Join(home, rel))
	}
	return folders
}

// readBackupDir reads a backup folder; a variable so a test can make one fail.
var readBackupDir = os.ReadDir

// unreadableBackups is a folder of backups that could not be read: a
// .reet-backups or .reet-sync-backups folder, or one date or removed-*
// folder in it. Folder is that folder.
type unreadableBackups struct {
	Folder string
	Err    error
}

// listJobBackups lists, as listBackups does, the backups beside the skills
// folders a sync job could also have worked in: Codex's under home, and those
// of the enrolled projects. It only reads.
//
// A folder whose backups cannot be read does not stop the rest: it is
// returned in the second result, with what was read from the others.
func listJobBackups(home string, projects []string) ([]backupEntry, []unreadableBackups) {
	folders := []string{filepath.Join(home, ".codex", "skills")}
	for _, p := range projects {
		for _, d := range projectDirs {
			folders = append(folders, filepath.Join(p, filepath.FromSlash(d.Dir)))
		}
	}
	return backupsBesideEach(folders)
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// backupsBeside lists the backups beside each of folders, newest date first,
// then in the order folders are given. It fails if any folder of backups
// cannot be read.
func backupsBeside(folders []string) ([]backupEntry, error) {
	out, bad := backupsBesideEach(folders)
	if len(bad) > 0 {
		return nil, bad[0].Err
	}
	return out, nil
}

// backupsBesideEach is backupsBeside that carries on past a folder of backups
// it cannot read, naming each such folder in the second result. What was read
// from the others is listed.
func backupsBesideEach(folders []string) ([]backupEntry, []unreadableBackups) {
	var out []backupEntry
	var bad []unreadableBackups
	for _, folder := range folders {
		got, unreadable := backupsInFolder(folder)
		out = append(out, got...)
		bad = append(bad, unreadable...)
	}
	order := map[string]int{}
	for i, folder := range folders {
		order[folder] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		if out[i].Folder != out[j].Folder {
			return order[out[i].Folder] < order[out[j].Folder]
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].At != out[j].At {
			return out[i].At > out[j].At
		}
		return out[i].Path > out[j].Path
	})
	return out, bad
}

// backupsInFolder lists the backups beside one skills folder, unsorted. A
// folder of backups it cannot read (the .reet-backups or .reet-sync-backups
// folder, or one date or removed-* folder in it) hides only what it holds:
// it is returned in the second result, and the rest is listed.
func backupsInFolder(folder string) ([]backupEntry, []unreadableBackups) {
	var out []backupEntry
	var bad []unreadableBackups
	root, prefix := backupRoot(folder), backupPrefix(folder)
	var dates []string
	if isRealDir(root) {
		entries, err := readBackupDir(root)
		if err != nil {
			bad = append(bad, unreadableBackups{Folder: root, Err: err})
		}
		for _, e := range entries {
			if e.IsDir() && validBackupDate(e.Name()) {
				dates = append(dates, e.Name())
			}
		}
	}
	for _, date := range dates {
		dir := filepath.Join(root, date)
		entries, err := readBackupDir(dir)
		if err != nil {
			bad = append(bad, unreadableBackups{Folder: dir, Err: err})
			continue
		}
		idx := readBackupIndex(dir)
		present := map[string]bool{}
		for _, e := range entries {
			present[e.Name()] = true
		}
		for _, e := range entries {
			if e.Name() == backupIndexFile {
				continue
			}
			b := backupEntry{Folder: folder, Path: filepath.Join(dir, e.Name()), Date: date, index: e.Name()}
			if rec, ok := idx.Entries[e.Name()]; ok && validBackupName(rec.Name) {
				b.Name, b.At = rec.Name, rec.At
			} else if name := strings.TrimPrefix(e.Name(), prefix); name != e.Name() && validBackupName(name) {
				// Without an index, a trailing -N is a clash suffix only
				// beside the entry it clashed with.
				if stem, ok := clashStem(name); ok && present[prefix+stem] {
					name = stem
				}
				b.Name = name
			} else {
				continue
			}
			out = append(out, b)
		}
	}
	legacy := filepath.Join(filepath.Dir(folder), ".reet-sync-backups")
	if !isRealDir(legacy) {
		return out, bad
	}
	groups, err := readBackupDir(legacy)
	if err != nil {
		return out, append(bad, unreadableBackups{Folder: legacy, Err: err})
	}
	for _, g := range groups {
		dir := filepath.Join(legacy, g.Name())
		st, err := os.Lstat(dir)
		if !strings.HasPrefix(g.Name(), "removed-") || err != nil || !st.IsDir() {
			continue
		}
		entries, err := readBackupDir(dir)
		if err != nil {
			bad = append(bad, unreadableBackups{Folder: dir, Err: err})
			continue
		}
		for _, e := range entries {
			if !validBackupName(e.Name()) {
				continue
			}
			out = append(out, backupEntry{Name: e.Name(), Folder: folder, Path: filepath.Join(dir, e.Name()),
				Date: st.ModTime().Format(backupDateLayout), At: st.ModTime().UnixMilli(), legacy: true})
		}
	}
	return out, bad
}

// leftAloneWhy says why reet leaves the managed folder alone, or "" when it
// does not.
func leftAloneWhy(home string, roots []managedRoot, folder string) string {
	for i, f := range managedFolders {
		if filepath.Join(home, filepath.FromSlash(f.Dir)) == folder {
			return roots[i].why
		}
	}
	return ""
}

// clashStem returns name without a trailing -N (N at least 2), the suffix
// backupSkill adds when a backup of the name is already there.
func clashStem(name string) (string, bool) {
	i := strings.LastIndexByte(name, '-')
	if i <= 0 {
		return "", false
	}
	n, err := strconv.Atoi(name[i+1:])
	if err != nil || n < 2 || name[i+1] == '+' {
		return "", false
	}
	return name[:i], true
}

// restoreBackup moves the named backup back into the folder it came from and
// returns the restored paths. With no date it uses the newest date holding the
// name; within a date, per folder, it takes the newest copy. If that date
// holds the name from several folders, every one is restored, and nothing
// moves if any target exists or is a folder reet leaves alone (see
// managedRoots, given the enrolled projects as unmanaged). If a move then
// fails part-way, the paths already restored are returned with the error.
func restoreBackup(home, name, date string, unmanaged ...string) ([]string, error) {
	if !validBackupName(name) {
		return nil, fmt.Errorf("%q is not a plain skill name", name)
	}
	if date != "" && !validBackupDate(date) {
		return nil, fmt.Errorf("%q is not a date; use YYYY-MM-DD", date)
	}
	all, err := listBackups(home)
	if err != nil {
		return nil, err
	}
	// The name matches without case, as skill names do; a backup with
	// exactly the name typed is taken over one that differs only in case.
	exact := slices.ContainsFunc(all, func(b backupEntry) bool { return b.Name == name && (date == "" || b.Date == date) })
	var found []backupEntry
	for _, b := range all { // newest date first; within a folder, newest copy first
		same := b.Name == name || !exact && strings.EqualFold(b.Name, name)
		if !same || (date != "" && b.Date != date) {
			continue
		}
		if len(found) > 0 && b.Date != found[0].Date {
			break
		}
		dup := false
		for _, f := range found {
			dup = dup || f.Folder == b.Folder
		}
		if !dup {
			found = append(found, b)
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no backup of %q found", name)
	}
	roots := managedRoots(home, unmanaged)
	var dests, taken, kept []string
	for _, b := range found {
		if why := leftAloneWhy(home, roots, b.Folder); why != "" {
			return nil, fmt.Errorf("%s; nothing was restored, and the backup stays in %s", why, b.Path)
		}
		dest := filepath.Join(b.Folder, b.Name)
		if _, err := os.Lstat(dest); !os.IsNotExist(err) {
			taken, kept = append(taken, dest), append(kept, b.Path)
		}
		dests = append(dests, dest)
	}
	if len(taken) > 0 {
		// Usually reet's own copy, put back by the library: the backup is
		// the version that was there before.
		return nil, fmt.Errorf("%s already %s, and restore never overwrites an installed skill, so nothing was restored. The backed-up version stays in %s. To keep it, copy it somewhere else; once reet offers Use this version on the Skills page, that makes it reet's copy",
			strings.Join(taken, " and "), map[bool]string{true: "exists", false: "exist"}[len(taken) == 1], strings.Join(kept, " and "))
	}
	var restored []string
	for i, b := range found {
		if err := os.MkdirAll(b.Folder, 0755); err != nil {
			return restored, err
		}
		if err := moveEntry(b.Path, dests[i]); err != nil {
			return restored, err
		}
		restored = append(restored, dests[i])
		dir := filepath.Dir(b.Path)
		if b.legacy {
			_ = os.Remove(dir) // only if now empty
			continue
		}
		_ = updateBackupIndex(dir, func(idx *backupIndex) bool {
			_, ok := idx.Entries[b.index]
			delete(idx.Entries, b.index)
			return ok
		})
	}
	return restored, nil
}
