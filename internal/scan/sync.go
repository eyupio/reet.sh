package scan

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

type syncAction struct {
	Add   bool
	Skill store.SyncSkill
}

var safeSlug = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var githubSource = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*/[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
var domainSource = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*\.[a-zA-Z0-9-]+$`)

func syncSource(sk store.SyncSkill) (string, error) {
	if !safeSlug.MatchString(sk.Slug) || sk.Slug == "." || sk.Slug == ".." {
		return "", errors.New("invalid skill slug")
	}
	if sk.SourceType == "github" && githubSource.MatchString(sk.Source) {
		return "https://github.com/" + sk.Source, nil
	}
	if sk.SourceType == "well-known" && domainSource.MatchString(sk.Source) {
		return "https://" + sk.Source, nil
	}
	return "", fmt.Errorf("%s has no supported install source; link it before syncing", sk.Name)
}

func planSync(desired []store.SyncSkill, actual []Found) ([]syncAction, error) {
	want, have := map[string]store.SyncSkill{}, map[string]Found{}
	for _, sk := range desired {
		if _, err := syncSource(sk); err != nil {
			return nil, err
		}
		if _, ok := want[sk.Slug]; ok {
			return nil, errors.New("reference contains duplicate skill names")
		}
		want[sk.Slug] = sk
	}
	for _, sk := range actual {
		if !safeSlug.MatchString(sk.Name) {
			return nil, errors.New("invalid installed skill name")
		}
		have[sk.Name] = sk
	}
	actions := []syncAction{}
	for name, sk := range want {
		installed, ok := have[name]
		if !ok {
			actions = append(actions, syncAction{Add: true, Skill: sk})
			continue
		}
		if installed.Source == "" || strings.TrimSuffix(strings.TrimPrefix(installed.Source, "https://github.com/"), ".git") != sk.Source {
			return nil, fmt.Errorf("source conflict for %s; inspect its local lock file before syncing", name)
		}
	}
	for name, sk := range have {
		if _, ok := want[name]; !ok {
			if sk.Source == "" {
				return nil, fmt.Errorf("%s is a local-only skill; back it up and remove it manually before mirroring", name)
			}
			actions = append(actions, syncAction{Skill: store.SyncSkill{Slug: name, Name: name, Source: sk.Source}})
		}
	}
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Add != actions[j].Add {
			return actions[i].Add
		}
		return actions[i].Skill.Slug < actions[j].Skill.Slug
	})
	return actions, nil
}

func syncAgent(agent string) string {
	switch agent {
	case "claude":
		return "claude-code"
	case "agents":
		return "codex"
	case "cursor", "opencode":
		return agent
	}
	return ""
}

func fetchTarget(server, token, id string) (store.SyncTarget, error) {
	req, err := http.NewRequest("GET", strings.TrimRight(server, "/")+"/api/v2/sync", nil)
	if err != nil {
		return store.SyncTarget{}, err
	}
	if token != "" {
		req.Header.Set("X-Reet-Token", token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return store.SyncTarget{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return store.SyncTarget{}, fmt.Errorf("controller replied %s", resp.Status)
	}
	var state struct {
		Targets  []store.SyncTarget `json:"targets"`
		CanWrite bool               `json:"canWrite"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&state); err != nil {
		return store.SyncTarget{}, err
	}
	if !state.CanWrite {
		return store.SyncTarget{}, errors.New("this account cannot apply sync targets")
	}
	for _, target := range state.Targets {
		if target.Target.ID == id {
			return target, nil
		}
	}
	return store.SyncTarget{}, errors.New("sync target not found in this account; configure it on Your account")
}

func RunSync(args []string, in io.Reader, out io.Writer) error {
	return RunSyncVersion(args, in, out, "dev")
}

func RunSyncVersion(args []string, in io.Reader, out io.Writer, version string) error {
	return runSync(args, in, out, func(args []string, cwd string) error {
		cmd := exec.Command("npx", args...)
		cmd.Dir = cwd
		cmd.Stdout = out
		cmd.Stderr = out
		cmd.Stdin = in
		return cmd.Run()
	}, version)
}

func runSync(args []string, in io.Reader, out io.Writer, execute func([]string, string) error, version string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(out)
	server := fs.String("server", os.Getenv("REET_URL"), "controller URL (env REET_URL)")
	token := fs.String("token", "", "personal token (env REET_TOKEN)")
	targetID := fs.String("target", "", "target ID from Your account")
	host := fs.String("env", os.Getenv("REET_ENV"), "local environment name (default hostname)")
	apply := fs.Bool("apply", false, "preview, then ask before installing/removing skills")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments; use --help")
	}
	if *token == "" {
		*token = os.Getenv("REET_TOKEN")
	}
	if *server == "" || *targetID == "" {
		return errors.New("--server and --target are required; copy the command from Your account")
	}
	if *host == "" {
		*host, _ = os.Hostname()
	}
	target, err := fetchTarget(*server, *token, *targetID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(*host, target.Target.Host) {
		return fmt.Errorf("this command is for %s, not %s; run it on the target machine", target.Target.Host, *host)
	}
	agent := syncAgent(target.Target.Agent)
	if agent == "" {
		return errors.New("this environment needs manual sync; supported targets are shared .agents (Codex), Claude Code, Cursor and OpenCode")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	resolvedCWD := cwd
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		resolvedCWD = resolved
	}
	if target.Target.Project != "" && (!filepath.IsAbs(target.Target.Project) || filepath.Clean(resolvedCWD) != filepath.Clean(target.Target.Project)) {
		return fmt.Errorf("run this command from project %s", target.Target.Project)
	}
	if target.Target.Project == "" && target.Target.Agent == "claude" && os.Getenv("CLAUDE_CONFIG_DIR") != "" && filepath.Clean(os.Getenv("CLAUDE_CONFIG_DIR")) != filepath.Join(home, ".claude") {
		return errors.New("custom CLAUDE_CONFIG_DIR requires manual sync")
	}
	if target.Target.Project == "" && target.Target.Agent == "opencode" && os.Getenv("XDG_CONFIG_HOME") != "" && filepath.Clean(os.Getenv("XDG_CONFIG_HOME")) != filepath.Join(home, ".config") {
		return errors.New("custom XDG_CONFIG_HOME requires manual sync")
	}
	targetPath := ""
	collect := func() ([]syncAction, error) {
		inventory := CollectScan(home, cwd, nil, *host)
		actual := []Found{}
		// Agent-specific installs can share a canonical directory. Do not replace
		// another agent's same-name skill with a different or unknown source.
		for _, desired := range target.Desired {
			for _, found := range inventory.Found {
				if found.Project == target.Target.Project && found.Agent != target.Target.Agent && found.Host == *host && found.Name == desired.Slug && found.Source != desired.Source {
					return nil, fmt.Errorf("%s is shared with %s from a different or unknown source; resolve it before syncing", found.Name, found.Agent)
				}
			}
		}
		for _, found := range inventory.Found {
			if found.Agent == target.Target.Agent && found.Project == target.Target.Project && found.Host == *host {
				actual = append(actual, found)
			}
		}
		// Do not interpret an unreadable directory as an empty target.
		path := ""
		dirs := homeDirs
		base := home
		if target.Target.Project != "" {
			dirs = projectDirs
			base = cwd
		}
		for _, dir := range dirs {
			if dir.Agent == target.Target.Agent {
				path = filepath.Join(base, dir.Dir)
			}
		}
		if path == "" {
			return nil, errors.New("unsupported target directory")
		}
		targetPath = path
		if _, err := os.ReadDir(path); err != nil {
			return nil, fmt.Errorf("cannot read target skills directory: %w", err)
		}
		actions, err := planSync(target.Desired, actual)
		if err != nil {
			return nil, err
		}

		canonicalBase := home
		if target.Target.Project != "" {
			canonicalBase = cwd
		}
		for _, action := range actions {
			if action.Add {
				for _, candidate := range []string{filepath.Join(targetPath, action.Skill.Slug), filepath.Join(canonicalBase, ".agents", "skills", action.Skill.Slug)} {
					if _, err := os.Lstat(candidate); err == nil {
						if skill, err := os.Stat(filepath.Join(candidate, "SKILL.md")); err != nil || skill.IsDir() {
							return nil, fmt.Errorf("unrecognised existing entry %s; inspect or move it aside before installing", candidate)
						}
					} else if !os.IsNotExist(err) {
						return nil, err
					}
				}
			}
		}
		if target.Target.Agent == "agents" {
			for _, action := range actions {
				if !action.Add {
					for _, found := range inventory.Found {
						if found.Name == action.Skill.Slug && found.Host == *host && found.Project == target.Target.Project && found.Agent != "agents" {
							return nil, fmt.Errorf("%s is shared with %s; unlink that agent's copy before removing it from the shared folder", found.Name, found.Agent)
						}
					}
				}
			}
		}
		return actions, nil
	}
	actions, err := collect()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Controller: %s\nTarget: %s\nDirectory: %s\nReference snapshot: %s\n", *server, target.Target.Name, targetPath, target.SourceName)
	for _, action := range actions {
		verb := "REMOVE"
		if action.Add {
			verb = "INSTALL"
		}
		fmt.Fprintf(out, "  %s %s (%s)\n", verb, action.Skill.Slug, action.Skill.Source)
	}
	if len(actions) == 0 {
		fmt.Fprintln(out, "Already in sync.")
		return RunVersion([]string{"--server", *server, "--token", *token, "--env", *host, "--prune"}, out, version)
	}
	if !*apply {
		fmt.Fprintln(out, "Preview only. Add --apply to review and confirm these changes.")
		return nil
	}
	fmt.Fprintf(out, "Removals move entries into .reet-sync-backups beside the skills folder. This may affect skills available to this agent. Type %q to apply: ", target.Target.Name)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	if strings.TrimSpace(line) != target.Target.Name {
		return errors.New("cancelled; no skills changed")
	}
	lockPath := filepath.Join(targetPath, ".reet-sync.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("cannot lock target: %w; another sync may be running", err)
	}
	lock.Close()
	defer os.Remove(lockPath)
	latest, err := fetchTarget(*server, *token, *targetID)
	if err != nil {
		return err
	}
	if latest.Revision != target.Revision {
		return errors.New("controller target changed; preview again")
	}
	current, err := collect()
	if err != nil {
		return err
	}
	old, _ := json.Marshal(actions)
	fresh, _ := json.Marshal(current)
	if string(old) != string(fresh) {
		return errors.New("local skills changed; preview again")
	}

	for _, action := range actions {
		var operationErr error
		if action.Add {
			source, err := syncSource(action.Skill)
			if err != nil {
				return err
			}
			command := []string{"--yes", "skills@1.7.4", "add", source, "--skill", action.Skill.Slug, "--agent", agent, "--yes"}
			if target.Target.Project == "" {
				command = append(command, "--global")
			}
			operationErr = execute(command, cwd)
		} else {
			// Move only the selected directory entry. Symlinks are moved as symlinks;
			// their canonical skill content stays intact for other agents.
			root := filepath.Join(filepath.Dir(targetPath), ".reet-sync-backups")
			if err := os.MkdirAll(root, 0700); err != nil {
				operationErr = err
			} else if backup, err := os.MkdirTemp(root, "removed-"); err != nil {
				operationErr = err
			} else {
				operationErr = os.Rename(filepath.Join(targetPath, action.Skill.Slug), filepath.Join(backup, action.Skill.Slug))
				if operationErr == nil {
					fmt.Fprintf(out, "Backed up removed entry to %s\n", filepath.Join(backup, action.Skill.Slug))
				}
			}
		}
		if operationErr != nil {
			_ = RunVersion([]string{"--server", *server, "--token", *token, "--env", *host, "--prune"}, out, version)
			return fmt.Errorf("sync stopped at %s: %w; rescan and preview again", action.Skill.Slug, operationErr)
		}
	}

	if err := RunVersion([]string{"--server", *server, "--token", *token, "--env", *host, "--prune"}, out, version); err != nil {
		return err
	}
	remaining, err := collect()
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		return errors.New("commands completed but differences remain; inspect the agent installation and preview again")
	}
	fmt.Fprintln(out, "Sync complete. Updated inventory persisted on the controller.")
	return nil
}
