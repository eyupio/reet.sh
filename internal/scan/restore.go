package scan

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
)

func restoreSkill(c agentConfig, targetID, backup string, in io.Reader, out io.Writer, version string) error {
	var agent store.SyncAgent
	if err := agentRequest(c, "/api/v2/sync/identity", map[string]string{}, &agent, version); err != nil {
		return err
	}
	target, err := fetchTarget(c.Server, c.Token, targetID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(target.Target.Host, agent.Host) {
		return errors.New("target belongs to another enrolled machine")
	}
	base := c.Home
	dirs := homeDirs
	if target.Target.Project != "" {
		base = target.Target.Project
		dirs = projectDirs
		allowed := false
		for _, p := range c.Projects {
			if p == base {
				allowed = true
			}
		}
		if !allowed {
			return errors.New("project is not enrolled")
		}
	}
	directory := ""
	for _, d := range dirs {
		if d.Agent == target.Target.Agent {
			directory = filepath.Join(base, d.Dir)
		}
	}
	if directory == "" {
		return errors.New("unsupported agent scope")
	}
	root := filepath.Join(filepath.Dir(directory), ".reet-sync-backups")
	absolute, err := filepath.Abs(backup)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, absolute)
	if err != nil {
		return err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "removed-") || !safeSlug.MatchString(parts[1]) || parts[1] == ".." {
		return errors.New("choose a removed skill entry from this target's .reet-sync-backups directory")
	}
	for _, p := range []string{root, filepath.Dir(absolute)} {
		st, e := os.Lstat(p)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("backup parent must be a real directory")
		}
	}
	destination := filepath.Join(directory, parts[1])
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		return errors.New("destination exists or is unavailable; restore never overwrites an installed skill")
	}
	if _, err = os.Lstat(absolute); err != nil {
		return err
	}
	fmt.Fprintf(out, "Restore %s\nTo %s\nType %q to restore: ", absolute, destination, target.Target.Name)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	if strings.TrimSpace(line) != target.Target.Name {
		return errors.New("cancelled; no files changed")
	}
	lockPath := filepath.Join(directory, ".reet-sync.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	lock.Close()
	defer os.Remove(lockPath)
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		return errors.New("destination changed; restore cancelled")
	}
	if err = os.Rename(absolute, destination); err != nil {
		return err
	}
	fmt.Fprintln(out, "Restored. Review local & web changes before the next sync.")
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(cwd)
	if err = os.Chdir(base); err != nil {
		return err
	}
	return RunVersion([]string{"--server", c.Server, "--token", c.Token, "--env", agent.Host, "--prune"}, out, version)
}
