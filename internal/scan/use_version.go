package scan

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var errBackupCapture = errors.New("a version is only read from one backup beside this agent's managed skills folder")

// skillBackupRoot is the backup a Use this version capture reads instead of
// the skill's live folder: backup, as the controller handed it back, must be
// one entry of the backups beside agent's managed skills folder under home,
// ~/{parent}/.reet-backups/{date}/{entry} or, from before,
// ~/{parent}/.reet-sync-backups/removed-*/{entry}, and a home-folder scope.
// It is never taken as a path as given: it is matched against those
// folders, and found in the managed folder's real parent, where reet makes
// its backups. The backups folder, the date folder and the backup itself
// must be real folders, not links, so nothing outside is read through them;
// skillbundle reads no link inside. unchanged says whether the two folders
// above the backup are still the ones checked, for after the read.
func skillBackupRoot(c agentConfig, agent, project, backup string) (root string, unchanged func() bool, err error) {
	rest, ok := strings.CutPrefix(backup, "~/")
	if project != "" || !ok || len(backup) > 1024 || strings.ContainsAny(backup, "\x00\\\r\n") {
		return "", nil, errBackupCapture
	}
	segs := strings.Split(rest, "/")
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return "", nil, errBackupCapture
		}
	}
	index := -1
	for i, f := range managedFolders {
		if f.Agent == agent {
			index = i
		}
	}
	if index < 0 {
		return "", nil, errBackupCapture
	}
	parent := strings.Split(path.Dir(managedFolders[index].Dir), "/")
	if len(segs) != len(parent)+3 || strings.Join(segs[:len(parent)], "/") != strings.Join(parent, "/") {
		return "", nil, errBackupCapture
	}
	backups, group, entry := segs[len(parent)], segs[len(parent)+1], segs[len(parent)+2]
	switch {
	case backups == ".reet-backups" && validBackupDate(group):
	case backups == ".reet-sync-backups" && len(group) > len("removed-") && strings.HasPrefix(group, "removed-"):
	default:
		return "", nil, errBackupCapture
	}
	if !validBackupName(entry) {
		return "", nil, errBackupCapture
	}
	// A folder reet leaves alone keeps its backups in someone else's folder.
	managed := managedRoots(c.Home, c.Projects)[index]
	if managed.skip {
		return "", nil, errBackupCapture
	}
	dirs := []string{filepath.Join(managed.parent, backups)}
	dirs = append(dirs, filepath.Join(dirs[0], group))
	root = filepath.Join(dirs[1], entry)
	var seen []os.FileInfo
	for _, p := range append(dirs, root) {
		st, err := os.Lstat(p)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return "", nil, errBackupCapture
		}
		seen = append(seen, st)
	}
	unchanged = func() bool {
		for i, p := range dirs {
			st, err := os.Lstat(p)
			if err != nil || st.Mode()&os.ModeSymlink != 0 || !os.SameFile(st, seen[i]) {
				return false
			}
		}
		return true
	}
	return root, unchanged, nil
}
