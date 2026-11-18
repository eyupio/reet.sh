package scan

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// backupRetention is how long a backed-up skill is kept before the agent
// prunes it.
const backupRetention = 30 * 24 * time.Hour

const backupDateLayout = "2006-01-02"

// backupFolders are the four skills folders the agent manages, relative to
// the home directory, in the order backups are listed within one date.
var backupFolders = []string{
	".agents/skills",
	".claude/skills",
	".cursor/skills",
	".config/opencode/skills",
}

type backupEntry struct{ Name, Folder, Path, Date string }

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
// returns where it went. A symlink moves as a symlink.
func backupSkill(folder, name string, now time.Time) (string, error) {
	if !validBackupName(name) {
		return "", fmt.Errorf("refusing to back up %q: not a plain skill name", name)
	}
	src := filepath.Join(folder, name)
	if _, err := os.Lstat(src); err != nil {
		return "", err
	}
	dir := filepath.Join(backupRoot(folder), now.Format(backupDateLayout))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	base := backupPrefix(folder) + name
	dest := filepath.Join(dir, base)
	for n := 2; ; n++ {
		if _, err := os.Lstat(dest); os.IsNotExist(err) {
			break
		} else if err != nil {
			return "", err
		}
		dest = filepath.Join(dir, fmt.Sprintf("%s-%d", base, n))
	}
	if err := moveEntry(src, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// moveEntry renames src to dest, falling back to copy-then-remove when the
// two are on different devices.
func moveEntry(src, dest string) error {
	err := os.Rename(src, dest)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err = copyEntry(src, dest); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	return os.RemoveAll(src)
}

// copyEntry copies a file, directory tree or symlink; symlinks stay links.
func copyEntry(src, dest string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
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
// the managed folders' .reet-backups under home.
func pruneBackups(home string, now time.Time) error {
	cutoff := now.Add(-backupRetention)
	var errs []error
	for _, rel := range backupFolders {
		root := backupRoot(filepath.Join(home, rel))
		for _, date := range dateFolders(root) {
			day, err := time.ParseInLocation(backupDateLayout, date, now.Location())
			if err != nil || !day.Before(cutoff) {
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

// listBackups returns every backup under home, newest date first.
func listBackups(home string) ([]backupEntry, error) {
	var out []backupEntry
	for _, rel := range backupFolders {
		folder := filepath.Join(home, rel)
		root, prefix := backupRoot(folder), backupPrefix(folder)
		for _, date := range dateFolders(root) {
			entries, err := os.ReadDir(filepath.Join(root, date))
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				name := strings.TrimPrefix(e.Name(), prefix)
				if name == e.Name() || !validBackupName(name) {
					continue
				}
				out = append(out, backupEntry{Name: name, Folder: folder, Path: filepath.Join(root, date, e.Name()), Date: date})
			}
		}
	}
	order := map[string]int{}
	for i, rel := range backupFolders {
		order[filepath.Join(home, rel)] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		if out[i].Folder != out[j].Folder {
			return order[out[i].Folder] < order[out[j].Folder]
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// restoreBackup moves the named backup back into the folder it came from and
// returns the restored path. With no date it uses the newest date holding the
// name. If that date holds the name from several folders, every one is
// restored (one path per line), and nothing moves if any target exists.
func restoreBackup(home, name, date string) (string, error) {
	if !validBackupName(name) {
		return "", fmt.Errorf("%q is not a plain skill name", name)
	}
	if date != "" && !validBackupDate(date) {
		return "", fmt.Errorf("%q is not a date; use YYYY-MM-DD", date)
	}
	all, err := listBackups(home)
	if err != nil {
		return "", err
	}
	var found []backupEntry
	for _, b := range all { // newest date first
		if b.Name != name || (date != "" && b.Date != date) {
			continue
		}
		if len(found) > 0 && b.Date != found[0].Date {
			break
		}
		found = append(found, b)
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no backup of %q found", name)
	}
	var dests []string
	for _, b := range found {
		dest := filepath.Join(b.Folder, name)
		if _, err := os.Lstat(dest); !os.IsNotExist(err) {
			return "", fmt.Errorf("%s already exists; restore never overwrites an installed skill", dest)
		}
		dests = append(dests, dest)
	}
	for i, b := range found {
		if err := os.MkdirAll(b.Folder, 0755); err != nil {
			return "", err
		}
		if err := moveEntry(b.Path, dests[i]); err != nil {
			return "", err
		}
	}
	return strings.Join(dests, "\n"), nil
}
