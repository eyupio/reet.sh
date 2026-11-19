package scan

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
)

// runAgentRestore implements `reet agent restore --list` and
// `reet agent restore <name> [--date YYYY-MM-DD]`, which work on this
// machine's dated backups and need no sync target, and
// `reet agent restore [--target ID] --backup PATH`, which puts one exact
// backup of a sync job back in the skills folder it came from.
func runAgentRestore(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("agent restore", flag.ContinueOnError)
	fs.SetOutput(out)
	config := fs.String("config", agentConfigPath(), "private machine enrollment file")
	list := fs.Bool("list", false, "list backed-up skills")
	date := fs.String("date", "", "backup date, YYYY-MM-DD (default: newest)")
	target := fs.String("target", "", "the sync job's target, checked when it is still saved")
	backup := fs.String("backup", "", "the exact backup a sync job made")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	name := ""
	if fs.NArg() > 0 { // allow flags after the name
		name = fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return err
		}
	}
	exact := *target != "" || *backup != ""
	if exact {
		if *backup == "" || *list || name != "" || *date != "" {
			return errors.New("usage: reet agent restore [--target ID] --backup PATH")
		}
		c, err := loadAgentConfig(*config)
		if err != nil {
			return err
		}
		restored, err := restoreJobBackup(c, *target, *backup)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Restored %s\n", restored)
		return nil
	}
	if fs.NArg() != 0 || (*list && name != "") || (!*list && name == "") {
		return errors.New("usage: reet agent restore --list | reet agent restore NAME [--date YYYY-MM-DD] | reet agent restore [--target ID] --backup PATH")
	}
	// The enrolled projects are left alone, as the agent leaves them. With no
	// enrollment there are none to know of; Codex's folder and a custom
	// config folder are left alone either way.
	home, projects := "", []string(nil)
	var desired *store.DesiredState
	if c, err := loadAgentConfig(*config); err == nil {
		home, projects = c.Home, c.Projects
		desired = readDesiredSkills(*config, c)
	}
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		home = h
	}
	if *list {
		// A folder of backups that cannot be read is said, one line each,
		// and does not hide what the others hold.
		entries, unreadable := listBackupsEach(home)
		// A sync job's backups beside a project's or Codex's skills folder
		// are listed too, with the command that restores each exactly.
		jobs, unreadableJobs := listJobBackups(home, projects)
		unreadable = append(unreadable, unreadableJobs...)
		for _, u := range unreadable {
			fmt.Fprintf(out, "Couldn't read the backups in %s: %v\n", u.Folder, u.Err)
		}
		if len(entries)+len(jobs) == 0 {
			if len(unreadable) > 0 {
				// "No backed-up skills" would be untrue, and nothing was listed.
				return fmt.Errorf("couldn't read the backups in %d folder(s), so none are listed", len(unreadable))
			}
			fmt.Fprintln(out, "No backed-up skills.")
		}
		for _, b := range entries {
			fmt.Fprintf(out, "%s  %s  from %s\n", b.Date, b.Name, b.Folder)
		}
		for _, b := range jobs {
			fmt.Fprintf(out, "%s  %s  from %s  (reet agent restore --backup %s)\n", b.Date, b.Name, b.Folder, shellQuote(b.Path))
		}
		return nil
	}
	restored, err := restoreBackup(home, name, *date, projects...)
	for _, p := range restored { // what was put back is said even when a later folder failed
		fmt.Fprintf(out, "Restored %s\n", p)
	}
	// The library decides what this machine keeps: a name it has Archived or
	// Deleted is taken off again on the agent's next pass.
	if len(restored) > 0 && desired != nil && slices.ContainsFunc(desired.Remove, func(r store.DesiredRemoval) bool { return strings.EqualFold(r.Name, name) }) {
		fmt.Fprintf(out, "%s is Archived or Deleted in your reet library, so reet takes it off this machine again on its next pass. To keep it, restore it on the Skills page, or copy it somewhere outside the skills folders.\n", name)
	}
	return err
}

// jobBackupFolders are the skills folders a sync job may have backed up
// from: every home skills folder the agent reads, Codex's included, and the
// skills folders of the enrolled projects.
func jobBackupFolders(c agentConfig) []string {
	var out []string
	for _, d := range homeDirs {
		out = append(out, filepath.Join(c.Home, filepath.FromSlash(d.Dir)))
	}
	for _, p := range c.Projects {
		for _, d := range projectDirs {
			out = append(out, filepath.Join(p, filepath.FromSlash(d.Dir)))
		}
	}
	return out
}

// restoreJobBackup puts backup, one a sync job made, back in the skills
// folder it was taken from, which its path names: an entry of
// <parent>/.reet-sync-backups/removed-* or <parent>/.reet-backups/<date> came
// from <parent>/skills, which must be one of jobBackupFolders. targetID, when
// given and still saved, must be that folder's target. Nothing already in the
// skills folder is replaced. It returns where the skill went.
func restoreJobBackup(c agentConfig, targetID, backup string) (string, error) {
	if !filepath.IsAbs(backup) || slices.Contains(strings.Split(filepath.ToSlash(backup), "/"), "..") {
		return "", errors.New("give the backup as the full path the Sync page shows")
	}
	backup = filepath.Clean(backup)
	group := filepath.Dir(backup)
	root := filepath.Dir(group)
	parent := filepath.Dir(root)
	entry, kind := filepath.Base(backup), filepath.Base(root)
	if kind != ".reet-sync-backups" && kind != ".reet-backups" {
		return "", errors.New("choose a backup from a .reet-sync-backups or .reet-backups folder")
	}
	folder := ""
	for _, f := range jobBackupFolders(c) {
		if candidate := filepath.Join(parent, "skills"); f == candidate || samePath(resolvePath(f), resolvePath(candidate)) {
			folder = f
			break
		}
	}
	if folder == "" {
		return "", fmt.Errorf("reet can't tell which skills folder %s came from: it is not beside one of this machine's skills folders", backup)
	}
	name := ""
	switch {
	case !validBackupName(entry):
	case kind == ".reet-sync-backups" && strings.HasPrefix(filepath.Base(group), "removed-"):
		name = entry
	case kind == ".reet-backups" && validBackupDate(filepath.Base(group)):
		if rec, ok := readBackupIndex(group).Entries[entry]; ok && rec.Folder == folder {
			name = rec.Name
		} else if rest, ok := strings.CutPrefix(entry, backupPrefix(folder)); ok {
			name = rest
		}
	}
	if !validBackupName(name) {
		return "", errors.New("choose a backup of one skill, from a .reet-sync-backups/removed-* or .reet-backups/YYYY-MM-DD folder")
	}
	if err := checkJobTarget(c, targetID, folder); err != nil {
		return "", err
	}
	if why := leftAloneWhy(c.Home, managedRoots(c.Home, c.Projects), folder); why != "" {
		return "", fmt.Errorf("%s; nothing was restored, and the backup stays in %s", why, backup)
	}
	for _, dir := range []string{root, group} {
		if !isRealDir(dir) {
			return "", errors.New("the backup's folder must be a real folder, not a link")
		}
	}
	if _, err := os.Lstat(backup); err != nil {
		return "", err
	}
	dest := filepath.Join(folder, name)
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		return "", fmt.Errorf("%s already exists; restore never overwrites an installed skill", dest)
	}
	if err := os.MkdirAll(folder, 0755); err != nil {
		return "", err
	}
	// The move refuses to replace anything that appears at dest meanwhile.
	if err := moveEntry(backup, dest); err != nil {
		return "", err
	}
	if kind == ".reet-sync-backups" {
		_ = os.Remove(group) // only if now empty
	} else if idx := readBackupIndex(group); idx.Entries[entry] != (indexRecord{}) {
		delete(idx.Entries, entry)
		_ = writeBackupIndex(group, idx)
	}
	return dest, nil
}

// checkJobTarget checks, when targetID names a target the controller still
// keeps for this machine, that its skills folder is folder. A target that is
// no longer saved checks nothing: the backup's own path says where it goes.
func checkJobTarget(c agentConfig, targetID, folder string) error {
	if targetID == "" {
		return nil
	}
	targets, err := listTargets(c.Server, c.Token)
	if err != nil {
		return err
	}
	var target *store.SyncTarget
	for i := range targets {
		if targets[i].Target.ID == targetID || slices.Contains(targets[i].TargetAliases, targetID) {
			target = &targets[i]
		}
	}
	if target == nil {
		return nil
	}
	var agent store.SyncAgent
	if err = agentRequest(c, "/api/v2/sync/identity", map[string]string{}, &agent, ""); err != nil {
		return err
	}
	if !strings.EqualFold(target.Target.Host, agent.Host) {
		return errors.New("that target belongs to another machine")
	}
	base, dirs := c.Home, homeDirs
	if target.Target.Project != "" {
		base, dirs = target.Target.Project, projectDirs
	}
	for _, d := range dirs {
		if d.Agent == target.Target.Agent && filepath.Join(base, filepath.FromSlash(d.Dir)) == folder {
			return nil
		}
	}
	return errors.New("that backup is not from that target's skills folder; nothing was restored")
}
