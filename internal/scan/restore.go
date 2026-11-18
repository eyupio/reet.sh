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
// `reet agent restore --target ID --backup PATH`, which puts one exact backup
// of a sync job back where that job's target keeps its skills.
func runAgentRestore(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("agent restore", flag.ContinueOnError)
	fs.SetOutput(out)
	config := fs.String("config", agentConfigPath(), "private machine enrollment file")
	list := fs.Bool("list", false, "list backed-up skills")
	date := fs.String("date", "", "backup date, YYYY-MM-DD (default: newest)")
	target := fs.String("target", "", "the sync job's target, with --backup")
	backup := fs.String("backup", "", "the exact backup a sync job made, with --target")
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
		if *target == "" || *backup == "" || *list || name != "" || *date != "" {
			return errors.New("usage: reet agent restore --target ID --backup PATH")
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
		return errors.New("usage: reet agent restore --list | reet agent restore NAME [--date YYYY-MM-DD] | reet agent restore --target ID --backup PATH")
	}
	// The enrolled projects are left alone, as the agent leaves them. With no
	// enrollment there are none to know of; Codex's folder and a custom
	// config folder are left alone either way.
	home, projects := "", []string(nil)
	if c, err := loadAgentConfig(*config); err == nil {
		home, projects = c.Home, c.Projects
	}
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		home = h
	}
	if *list {
		entries, err := listBackups(home)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Fprintln(out, "No backed-up skills.")
		}
		for _, b := range entries {
			fmt.Fprintf(out, "%s  %s  from %s\n", b.Date, b.Name, b.Folder)
		}
		return nil
	}
	restored, err := restoreBackup(home, name, *date, projects...)
	for _, p := range restored { // what was put back is said even when a later folder failed
		fmt.Fprintf(out, "Restored %s\n", p)
	}
	return err
}

// restoreJobBackup puts backup, one a sync job made, back in the skills
// folder of the job's target: the machine's home or an enrolled project, for
// the target's agent. The backup must be an entry of that folder's
// .reet-sync-backups/removed-* or .reet-backups/<date> folder, named plainly;
// nothing already in the skills folder is replaced. It returns where the
// skill went.
func restoreJobBackup(c agentConfig, targetID, backup string) (string, error) {
	if !filepath.IsAbs(backup) || slices.Contains(strings.Split(filepath.ToSlash(backup), "/"), "..") {
		return "", errors.New("give the backup as the full path the Sync page shows")
	}
	backup = filepath.Clean(backup)
	var agent store.SyncAgent
	if err := agentRequest(c, "/api/v2/sync/identity", map[string]string{}, &agent, ""); err != nil {
		return "", err
	}
	target, err := fetchTarget(c.Server, c.Token, targetID)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(target.Target.Host, agent.Host) {
		return "", errors.New("that target belongs to another machine")
	}
	base, dirs := c.Home, homeDirs
	if target.Target.Project != "" {
		base, dirs = target.Target.Project, projectDirs
		if !slices.Contains(c.Projects, base) {
			return "", errors.New("that target's project is not enrolled on this machine")
		}
	}
	folder := ""
	for _, d := range dirs {
		if d.Agent == target.Target.Agent {
			folder = filepath.Join(base, filepath.FromSlash(d.Dir))
		}
	}
	if folder == "" {
		return "", errors.New("that target's agent has no skills folder reet knows")
	}
	if target.Target.Project == "" {
		if why := leftAloneWhy(c.Home, managedRoots(c.Home, c.Projects), folder); why != "" {
			return "", fmt.Errorf("%s; nothing was restored, and the backup stays in %s", why, backup)
		}
	}
	parent := filepath.Dir(folder)
	rel, err := filepath.Rel(parent, backup)
	if err != nil {
		return "", err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	name := ""
	if len(parts) == 3 && validBackupName(parts[2]) {
		switch {
		case parts[0] == ".reet-sync-backups" && strings.HasPrefix(parts[1], "removed-"):
			name = parts[2]
		case parts[0] == ".reet-backups" && validBackupDate(parts[1]):
			if rec, ok := readBackupIndex(filepath.Join(parent, parts[0], parts[1])).Entries[parts[2]]; ok && rec.Folder == folder {
				name = rec.Name
			} else if rest, ok := strings.CutPrefix(parts[2], backupPrefix(folder)); ok {
				name = rest
			}
		}
	}
	if !validBackupName(name) {
		return "", errors.New("choose a backup of this target's skills folder, from its .reet-sync-backups or .reet-backups folder")
	}
	for _, dir := range []string{filepath.Join(parent, parts[0]), filepath.Join(parent, parts[0], parts[1])} {
		if !isRealDir(dir) {
			return "", errors.New("the backup's folder must be a real folder, not a link")
		}
	}
	if _, err = os.Lstat(backup); err != nil {
		return "", err
	}
	dest := filepath.Join(folder, name)
	if _, err = os.Lstat(dest); !os.IsNotExist(err) {
		return "", fmt.Errorf("%s already exists; restore never overwrites an installed skill", dest)
	}
	if err = os.MkdirAll(folder, 0755); err != nil {
		return "", err
	}
	// The move refuses to replace anything that appears at dest meanwhile.
	if err = moveEntry(backup, dest); err != nil {
		return "", err
	}
	dir := filepath.Dir(backup)
	if parts[0] == ".reet-sync-backups" {
		_ = os.Remove(dir) // only if now empty
	} else if idx := readBackupIndex(dir); idx.Entries[parts[2]] != (indexRecord{}) {
		delete(idx.Entries, parts[2])
		_ = writeBackupIndex(dir, idx)
	}
	return dest, nil
}
