package scan

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/store"
)

type syncAction struct {
	BeforeDigest string          `json:"beforeDigest,omitempty"`
	Add          bool            `json:"add"`
	Replace      bool            `json:"replace,omitempty"`
	Skill        store.SyncSkill `json:"skill"`
}
type SyncPlan struct {
	Target    string       `json:"target"`
	Directory string       `json:"directory"`
	Revision  string       `json:"revision"`
	Actions   []syncAction `json:"actions"`
	Backups   []string     `json:"backups,omitempty"`
}

func (p SyncPlan) Hash() string {
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type syncOptions struct {
	verifyEnrollment bool
	approvedHash     string
	cwd              string
	onPlan           func(SyncPlan)
	onProgress       func(string, string)
	onBackup         func(string)
}

var safeSlug = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var githubSource = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*/[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
var domainSource = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*\.[a-zA-Z0-9-]+$`)

func syncSource(sk store.SyncSkill) (string, error) {
	if !safeSlug.MatchString(sk.Slug) || sk.Slug == "." || sk.Slug == ".." {
		return "", errors.New("invalid skill slug")
	}
	if sk.SourceType == "private" {
		if privateSourcePattern.MatchString(sk.Source) {
			return sk.Source, nil
		}
		return "", errors.New("invalid private skill source")
	}
	if sk.SourceType == "github" && githubSource.MatchString(sk.Source) {
		return "https://github.com/" + sk.Source, nil
	}
	if sk.SourceType == "hosted" {
		raw := sk.URL
		if raw == "" {
			raw = "https://" + sk.Source + "/" + sk.Slug
		}
		u, err := url.Parse(raw)
		if err == nil && (u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) && u.Host+u.Path == sk.Source+"/"+sk.Slug && u.User == nil && u.RawQuery == "" && u.Fragment == "" && len(strings.Split(strings.Trim(u.Path, "/"), "/")) == 2 {
			return u.String(), nil
		}
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
		if installed.Source == "" {
			// The local copy has no provenance. Replace it with the reviewed
			// source-linked skill; apply moves the old folder into the backup.
			actions = append(actions, syncAction{Add: true, Replace: true, Skill: sk, BeforeDigest: installed.Digest})
			continue
		}
		if canonicalSource(installed.Source) != canonicalSource(sk.Source) {
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
		if target.Target.ID == id || slices.Contains(target.TargetAliases, id) {
			return target, nil
		}
	}
	return store.SyncTarget{}, errors.New("sync target not found in this account; configure it on Your account")
}

func RunSync(args []string, in io.Reader, out io.Writer) error {
	return RunSyncVersion(args, in, out, "dev")
}

func RunSyncVersion(args []string, in io.Reader, out io.Writer, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	return runSyncWithOptions(args, in, out, func(args []string, cwd string) error {
		cmd := exec.CommandContext(ctx, "npx", args...)
		cmd.Dir = cwd
		cmd.Stdout = out
		cmd.Stderr = out
		cmd.Stdin = in
		return cmd.Run()
	}, version, syncOptions{verifyEnrollment: true})
}

func runSync(args []string, in io.Reader, out io.Writer, execute func([]string, string) error, version string) error {
	return runSyncWithOptions(args, in, out, execute, version, syncOptions{})
}
func runSyncWithOptions(args []string, in io.Reader, out io.Writer, execute func([]string, string) error, version string, options syncOptions) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(out)
	server := fs.String("server", os.Getenv("REET_URL"), "controller URL (env REET_URL)")
	token := fs.String("token", "", "personal token (env REET_TOKEN)")
	tokenFile := fs.String("env-file", "", "read REET_TOKEN from an explicit env file")
	targetID := fs.String("target", "", "target ID from Your account")
	host := fs.String("env", os.Getenv("REET_ENV"), "local environment name (default hostname)")
	apply := fs.Bool("apply", false, "preview, then ask before installing/removing skills")
	config := fs.String("config", agentConfigPath(), "enrolled machine configuration")
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
	if *token == "" && *tokenFile != "" {
		value, err := clientconfig.ResolveToken("", *tokenFile, strings.NewReader(""), os.Stderr)
		if err != nil {
			return err
		}
		*token = value
	}
	if c, e := loadAgentConfig(*config); e == nil {
		if *server == "" {
			*server = c.Server
		}
		if *token == "" && strings.TrimRight(*server, "/") == c.Server {
			*token = c.Token
		}
		if *host == "" {
			*host = c.Host
		}
	}
	if *server == "" {
		value, err := clientconfig.ResolveURL("", in, os.Stderr)
		if err != nil {
			return err
		}
		*server = value
	}
	targetValue, inputErr := clientconfig.Require(*targetID, "--target (target ID from Account)", false, in, os.Stderr)
	if inputErr != nil {
		return inputErr
	}
	*targetID = targetValue
	tokenValue, inputErr := clientconfig.ResolveToken(*token, *tokenFile, in, os.Stderr)
	if inputErr != nil {
		return inputErr
	}
	*token = tokenValue
	if *host == "" {
		*host, _ = os.Hostname()
	}
	target, err := fetchTarget(*server, *token, *targetID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(*host, target.Target.Host) && !slices.ContainsFunc(target.HostAliases, func(alias string) bool { return strings.EqualFold(alias, *host) }) {
		return fmt.Errorf("this command is for %s, not %s; run it on the target machine", target.Target.Host, *host)
	}
	*host = target.Target.Host
	if *apply && options.verifyEnrollment {
		c, err := loadAgentConfig(*config)
		if err != nil {
			return fmt.Errorf("enroll this machine with reet agent enroll before applying: %w", err)
		}
		if strings.TrimRight(*server, "/") != c.Server {
			return errors.New("enrollment belongs to another controller")
		}
		var a store.SyncAgent
		if err := agentRequest(c, "/api/v2/sync/identity", map[string]string{}, &a, version); err != nil {
			return err
		}
		if !strings.EqualFold(a.Host, *host) {
			return errors.New("this installation is enrolled as a different machine")
		}
		applicationLock := *config + ".apply.lock"
		lock, err := os.OpenFile(applicationLock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return errors.New("another apply is running for this installation; inspect the process before recovering a stale .apply.lock")
		}
		lock.Close()
		defer os.Remove(applicationLock)

	}

	agent := syncAgent(target.Target.Agent)
	if agent == "" {
		return errors.New("this environment needs manual sync; supported targets are shared .agents (Codex), Claude Code, Cursor and OpenCode")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cwd := options.cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	resolvedCWD := cwd
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		resolvedCWD = resolved
	}
	rescan := func() error {
		args := []string{"--server", *server, "--token", *token, "--env", *host, "--prune"}
		if options.cwd != "" {
			return runVersionAt(options.cwd, args, out, version)
		}
		return RunVersion(args, out, version)
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
		if err := inventory.Err(); err != nil {
			return nil, fmt.Errorf("scan incomplete; no changes allowed: %w", err)
		}
		actual := []Found{}
		// Agent-specific installs can share a canonical directory. Do not replace
		// another agent's same-name skill with a different or unknown source.
		for _, desired := range target.Desired {
			for _, found := range inventory.Found {
				if found.Project == target.Target.Project && found.Agent != target.Target.Agent && found.Host == *host && found.Name == desired.Slug && canonicalSource(found.Source) != canonicalSource(desired.Source) {
					// An unlinked local copy in another agent scope does not block
					// replacing this scope's copy. Known, different sources still do.
					if found.Source == "" {
						continue
					}
					return nil, fmt.Errorf("%s is shared with %s from a different source; resolve it before syncing", found.Name, found.Agent)
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
		// A missing skills folder is an empty scope, not a reason to stop. The
		// install command creates it, and apply makes it first so the lock has a
		// home. Any other read error still blocks: a folder we cannot read must
		// never look like an empty one.
		if _, err := os.ReadDir(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("cannot read target skills directory: %w", err)
		}
		desired := target.Desired
		// An imported private copy has no public lock-file URL. Compare its
		// actual file digest against the pinned revision instead of reinstalling
		// it on every sync or guessing an upstream from its name.
		for i := range actual {
			for _, want := range desired {
				if want.SourceType == "private" && actual[i].Name == want.Slug {
					digest, e := privateSkillDigest(filepath.Join(targetPath, want.Slug))
					if e != nil {
						return nil, e
					}
					actual[i].Digest = digest
					if parts := privateSourcePattern.FindStringSubmatch(want.Source); parts != nil && digest == parts[3] {
						actual[i].Source = want.Source
					}
				}
			}
		}
		if target.Mode == "bidirectional" {
			// A changed local set must be reconciled and saved in the web UI first.
			base := map[string]string{}
			have := map[string]string{}
			for _, sk := range target.Baseline {
				base[sk.Slug] = canonicalSource(sk.Source)
			}
			for _, sk := range actual {
				have[sk.Name] = canonicalSource(sk.Source)
			}
			// After a successful apply, actual matches desired rather than the old baseline.
			wanted := map[string]string{}
			for _, sk := range desired {
				wanted[sk.Slug] = canonicalSource(sk.Source)
			}
			same := func(a, b map[string]string) bool {
				if len(a) != len(b) {
					return false
				}
				for k, v := range a {
					if w, ok := b[k]; !ok || v != w {
						return false
					}
				}
				return true
			}
			if !same(have, base) && !same(have, wanted) {
				return nil, errors.New("local changes need review in Compare & sync; save the reconciled selection before applying")
			}
		}

		actions, err := planSync(desired, actual)
		if target.Mode == "additive" {
			// Preserve extras, including local-only skills, in additive mode.
			filtered := []Found{}
			for _, f := range actual {
				for _, d := range desired {
					if d.Slug == f.Name {
						filtered = append(filtered, f)
						break
					}
				}
			}
			actions, err = planSync(desired, filtered)
		}
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
	progress := func(stage, detail string) {
		if options.onProgress != nil {
			options.onProgress(stage, detail)
		}
	}
	progress("read", "Reading installed skills on this machine")
	actions, err := collect()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Controller: %s\nTarget: %s\nDirectory: %s\nReference snapshot: %s\n", *server, target.Target.Name, targetPath, target.SourceName)
	for _, action := range actions {
		verb := "REMOVE"
		if action.Add && action.Replace {
			verb = "REPLACE"
		} else if action.Add {
			verb = "INSTALL"
		}
		fmt.Fprintf(out, "  %s %s (%s)\n", verb, action.Skill.Slug, action.Skill.Source)
	}
	progress("plan", fmt.Sprintf("%d changes planned", len(actions)))
	plan := SyncPlan{Target: target.Target.ID, Directory: targetPath, Revision: target.Revision, Actions: actions}
	if options.onPlan != nil {
		options.onPlan(plan)
	}
	if options.approvedHash != "" && options.approvedHash != plan.Hash() {
		return errors.New("approved plan no longer matches local files or desired revision; preview again")
	}
	if len(actions) == 0 {
		fmt.Fprintln(out, "Already in sync.")
		return rescan()
	}
	if !*apply {
		fmt.Fprintln(out, "Preview only. Add --apply to review and confirm these changes.")
		return nil
	}
	if options.approvedHash == "" {
		fmt.Fprintf(out, "Removals move entries into .reet-sync-backups beside the skills folder. This may affect skills available to this agent. Type %q to apply: ", target.Target.Name)
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		if strings.TrimSpace(line) != target.Target.Name {
			return errors.New("cancelled; no skills changed")
		}
	}
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("cannot create target skills directory: %w", err)
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

	for index, action := range actions {
		verb := "Installing"
		if !action.Add {
			verb = "Backing up"
		}
		progress("apply", fmt.Sprintf("%s %s · %d of %d", verb, action.Skill.Slug, index+1, len(actions)))
		var operationErr error
		if action.Add {
			if action.Replace {
				operationErr = backupSkillEntry(targetPath, action.Skill.Slug, out, options.onBackup)
			}
			if operationErr == nil && action.Skill.SourceType == "private" {
				operationErr = installPrivateSkill(*server, *token, action.Skill, targetPath)
			} else if operationErr == nil {
				source, err := syncSource(action.Skill)
				if err != nil {
					return err
				}
				command := []string{"--yes", "skills@1.7.4", "add", source, "--skill", action.Skill.Slug, "--agent", agent, "--yes"}
				if target.Target.Project == "" {
					command = append(command, "--global")
				}
				operationErr = execute(command, cwd)
			}
		} else {
			operationErr = backupSkillEntry(targetPath, action.Skill.Slug, out, options.onBackup)
		}
		if operationErr != nil {
			_ = rescan()
			return fmt.Errorf("sync stopped at %s: %w; rescan and preview again", action.Skill.Slug, operationErr)
		}
	}

	progress("verify", "Checking installed skills and reporting the result")
	if err := rescan(); err != nil {
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

func backupSkillEntry(targetPath, slug string, out io.Writer, onBackup func(string)) error {
	// Move only the selected directory entry. Symlinks are moved as symlinks;
	// their canonical skill content stays intact for other agents.
	root := filepath.Join(filepath.Dir(targetPath), ".reet-sync-backups")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	backup, err := os.MkdirTemp(root, "removed-")
	if err != nil {
		return err
	}
	path := filepath.Join(backup, slug)
	if err = os.Rename(filepath.Join(targetPath, slug), path); err != nil {
		return err
	}
	fmt.Fprintf(out, "Backed up existing entry to %s\n", path)
	if onBackup != nil {
		onBackup(path)
	}
	return nil
}

func canonicalSource(source string) string {
	source = strings.TrimPrefix(strings.TrimPrefix(source, "https://"), "http://")
	source = strings.TrimPrefix(source, "github.com/")
	return strings.TrimSuffix(strings.TrimRight(source, "/"), ".git")
}
