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
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/noderuntime"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/systeminstall"
)

// SyncAction is one install, replace or remove in a sync plan.
type SyncAction struct {
	BeforeDigest  string                        `json:"beforeDigest,omitempty"`
	Add           bool                          `json:"add"`
	Replace       bool                          `json:"replace,omitempty"`
	Skill         store.SyncSkill               `json:"skill"`
	RemovalIntent *store.LibraryDeploymentCheck `json:"removalIntent,omitempty"`
}
type SyncPlan struct {
	Problems  map[string]string `json:"problems,omitempty"`
	Target    string            `json:"target"`
	Directory string            `json:"directory"`
	Revision  string            `json:"revision"`
	Actions   []SyncAction      `json:"actions"`
	Backups   []string          `json:"backups,omitempty"`
}

func (p SyncPlan) Hash() string {
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type syncOptions struct {
	filterDesired func([]store.SyncSkill) []store.SyncSkill
	onExisting    func(store.SyncSkill, store.SyncTarget, string) error
	onInstalled   func(store.SyncSkill, string, []skillbundle.ContentFile) error
	// onInstalled pins the base before writing; onInstallVerified records
	// ownership only after the private installer actually completed its rename.
	onInstallVerified func(store.SyncSkill, string) error
	managedRemovals   func(store.SyncTarget, string) ([]SyncAction, map[string]string, error)
	removeManaged     func(SyncAction, string, io.Writer, func(string)) error
	verifyEnrollment  bool
	approvedHash      string
	cwd               string
	onTarget          func(store.SyncTarget)
	onPlan            func(SyncPlan)
	onProgress        func(string, string)
	onBackup          func(string)
	preflight         func([]SyncAction) error
}

var errMissingNpx = errors.New("npx was not found, or Node.js is older than 18; install a current Node.js (which provides npx and npm) on this machine, then sync again")

// needsNpx reports whether the plan installs a skill through the skills CLI.
// Removals and private reet-hosted installs never invoke it.
func needsNpx(actions []SyncAction) bool {
	for _, action := range actions {
		if action.Add && action.Skill.SourceType != "private" {
			return true
		}
	}
	return false
}

// requireNpx is the non-interactive guard. It accepts an npx on PATH or a
// runtime a previous interactive run provisioned into the managed directory.
func requireNpx(actions []SyncAction) error {
	return resolveNpx(actions, nil, nil, nil)
}

// requireDependencies is the non-interactive guard for everything an apply
// would invoke, so the enrolled agent fails before touching files.
func requireDependencies(actions []SyncAction) error {
	if err := requireNpx(actions); err != nil {
		return err
	}
	if needsGit(actions) {
		if _, err := exec.LookPath("git"); err != nil {
			return errMissingGit
		}
	}
	return nil
}

// resolveNpx finds the skills CLI runtime and, when a person is present, offers
// the system package manager or a verified runtime in reet's managed directory.
// It runs before any file in the plan is touched.
func resolveNpx(actions []SyncAction, ctx context.Context, in io.Reader, out io.Writer) error {
	if !needsNpx(actions) {
		return nil
	}
	home, _ := os.UserHomeDir()
	if path, managed := noderuntime.Lookup(home); path != "" {
		if managed {
			noderuntime.Use(path)
		}
		return nil
	}
	if major, ok := noderuntime.Major(); ok && major < noderuntime.MinimumMajor && out != nil {
		fmt.Fprintf(out, "The Node.js on PATH (v%d) is older than %d.\n", major, noderuntime.MinimumMajor)
	}
	if ctx == nil || !clientconfig.Interactive(in) {
		return errMissingNpx
	}
	if plan, ok := systeminstall.For(systeminstall.Request{Linux: []string{"nodejs", "npm"}, Brew: []string{"node"}}); ok {
		installed, err := offerInstall(ctx, in, out, "Node.js is required to install skills.", plan)
		if err != nil {
			return err
		}
		if installed {
			if path, managed := noderuntime.Lookup(home); path != "" {
				if managed {
					noderuntime.Use(path)
				}
				return nil
			}
			if major, ok := noderuntime.Major(); ok && major < noderuntime.MinimumMajor && out != nil {
				fmt.Fprintf(out, "The packaged Node.js is still older than %d; using a current runtime instead.\n", noderuntime.MinimumMajor)
			} else if out != nil {
				fmt.Fprintln(out, "The package manager finished, but npx is not on PATH yet; using a self-contained runtime instead.")
			}
		}
	}
	answer, err := clientconfig.Ask(in, out, "Download a verified Node.js runtime into ~/.reet instead? About 250 MB installed, no sudo needed (yes/no)", "no")
	if err != nil {
		return err
	}
	if !isYes(answer) {
		return errors.New("cancelled; no skills changed")
	}
	path, err := noderuntime.Install(ctx, nil, home, out)
	if err != nil {
		return fmt.Errorf("could not install Node.js: %w", err)
	}
	noderuntime.Use(path)
	return nil
}

var errMissingGit = errors.New("git was not found in PATH; install git to install skills from GitHub, then sync again")

func needsGit(actions []SyncAction) bool {
	for _, action := range actions {
		if action.Add && action.Skill.SourceType == "github" {
			return true
		}
	}
	return false
}

// resolveGit offers the system package manager for GitHub-sourced skills,
// which the skills CLI fetches with git.
func resolveGit(actions []SyncAction, ctx context.Context, in io.Reader, out io.Writer) error {
	if !needsGit(actions) {
		return nil
	}
	if _, err := exec.LookPath("git"); err == nil {
		return nil
	}
	if ctx == nil || !clientconfig.Interactive(in) {
		return errMissingGit
	}
	plan, ok := systeminstall.For(systeminstall.Request{Linux: []string{"git"}, Brew: []string{"git"}})
	if !ok {
		return errMissingGit
	}
	installed, err := offerInstall(ctx, in, out, "Git is required to install skills from GitHub.", plan)
	if err != nil {
		return err
	}
	if !installed {
		return errors.New("cancelled; no skills changed")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git was installed, but it is still not on PATH; open a new shell and sync again")
	}
	return nil
}

// offerInstall shows the exact command, asks first, then runs it with the real
// terminal so sudo can prompt. It reports whether the command ran and succeeded.
func offerInstall(ctx context.Context, in io.Reader, out io.Writer, why string, plan systeminstall.Plan) (bool, error) {
	command := strings.Join(plan.Command, " ")
	question := fmt.Sprintf("%s Install it with %s now? %s (may ask for your sudo password) (yes/no)", why, plan.Label, command)
	answer, err := clientconfig.Ask(in, out, question, "no")
	if err != nil {
		return false, err
	}
	if !isYes(answer) {
		return false, nil
	}
	fmt.Fprintf(out, "Running %s\n", command)
	if err := systeminstall.Run(ctx, plan, in, out); err != nil {
		return false, fmt.Errorf("%s installation failed: %w", plan.Label, err)
	}
	return true, nil
}

// commandOutputError keeps the tail of an external command's output so a
// failure can be classified after stdout and stderr have already been shown.
type commandOutputError struct {
	err    error
	output string
}

func (e *commandOutputError) Error() string { return e.err.Error() }
func (e *commandOutputError) Unwrap() error { return e.err }

type tailBuffer struct {
	data []byte
	max  int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	if len(b.data) > b.max {
		b.data = b.data[len(b.data)-b.max:]
	}
	return len(p), nil
}

func runNpxCommand(ctx context.Context, args []string, cwd string, in io.Reader, out io.Writer) error {
	tail := &tailBuffer{max: 8 << 10}
	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Dir = cwd
	cmd.Stdout = io.MultiWriter(out, tail)
	cmd.Stderr = io.MultiWriter(out, tail)
	cmd.Stdin = in
	if err := cmd.Run(); err != nil {
		return &commandOutputError{err: err, output: string(tail.data)}
	}
	return nil
}

// networkFailure spots the common offline, proxy and registry errors from the
// skills CLI so the retry advice can mention connectivity.
func networkFailure(err error) bool {
	text := err.Error()
	var withOutput *commandOutputError
	if errors.As(err, &withOutput) {
		text += " " + withOutput.output
	}
	text = strings.ToLower(text)
	for _, marker := range []string{"eai_again", "enotfound", "etimedout", "econnrefused", "econnreset", "network", "proxy", "registry", "certificate"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isYes(answer string) bool {
	switch strings.ToLower(answer) {
	case "yes", "y":
		return true
	}
	return false
}

var safeSlug = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var githubSource = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*/[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// Recoverable refusals: the machine did not change anything, and asking for a
// fresh preview would clear them. The enrolled agent reports these to the
// controller as "replan", which lets an automatic run repair itself instead of
// leaving the person with a failed card to press.
var (
	errApprovedPlanChanged = errors.New("approved plan no longer matches local files or desired revision; preview again")
	errTargetChanged       = errors.New("controller target changed; preview again")
	errLocalSkillsChanged  = errors.New("local skills changed; preview again")
	errSyncTargetChanged   = errors.New("target identity or revision changed")
)

// ErrReplanWorthAttempting reports whether err is a refusal a fresh preview
// would clear without the person changing anything.
func ErrReplanWorthAttempting(err error) bool {
	return errors.Is(err, errApprovedPlanChanged) ||
		errors.Is(err, errTargetChanged) ||
		errors.Is(err, errLocalSkillsChanged) ||
		errors.Is(err, errSyncTargetChanged)
}

var domainSource = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*\.[a-zA-Z0-9-]+$`)

func syncSource(sk store.SyncSkill) (string, error) {
	valid := safeSlug.MatchString(sk.Slug) && sk.Slug != "." && sk.Slug != ".."
	if !valid {
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
		loopback := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
		validScheme := u.Scheme == "https" || (u.Scheme == "http" && loopback)
		validHost := u.Host+u.Path == sk.Source+"/"+sk.Slug
		validQuery := u.User == nil && u.RawQuery == "" && u.Fragment == ""
		validShape := len(strings.Split(strings.Trim(u.Path, "/"), "/")) == 2
		valid := err == nil && validScheme && validHost && validQuery && validShape
		if valid {
			return u.String(), nil
		}
	}
	if sk.SourceType == "well-known" && domainSource.MatchString(sk.Source) {
		return "https://" + sk.Source, nil
	}
	return "", fmt.Errorf("%s has no supported install source; link it before syncing", sk.Name)
}

func planSync(desired []store.SyncSkill, actual []Found) ([]SyncAction, error) {
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
	actions := []SyncAction{}
	for name, sk := range want {
		installed, ok := have[name]
		if !ok {
			actions = append(actions, SyncAction{Add: true, Skill: sk})
			continue
		}
		if installed.Source == "" {
			// The local copy has no provenance. Replace it with the reviewed
			// source-linked skill; apply moves the old folder into the backup.
			actions = append(actions, SyncAction{Add: true, Replace: true, Skill: sk, BeforeDigest: installed.Digest})
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
			actions = append(actions, SyncAction{Skill: store.SyncSkill{Slug: name, Name: name, Source: sk.Source}})
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

func listTargets(server, token string) ([]store.SyncTarget, error) {
	req, err := http.NewRequest("GET", strings.TrimRight(server, "/")+"/api/v2/sync", nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("X-Reet-Token", token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&failure)
		if authErr := clientconfig.NewAuthError(resp.StatusCode, failure.Error); authErr != nil {
			return nil, authErr
		}
		return nil, fmt.Errorf("controller replied %s", resp.Status)
	}

	var state struct {
		Targets  []store.SyncTarget `json:"targets"`
		CanWrite bool               `json:"canWrite"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&state); err != nil {
		return nil, err
	}
	if !state.CanWrite {
		return nil, errors.New("this account cannot apply sync targets")
	}
	return state.Targets, nil
}

// TargetChoice is one saved sync target, labelled for a picker.
type TargetChoice struct {
	ID, Label string
	Here      bool
}

// ListTargetChoices returns the account's saved targets, those on host first, so
// a person can pick one instead of typing an ID.
func ListTargetChoices(server, token, host string) ([]TargetChoice, error) {
	targets, err := listTargets(server, token)
	if err != nil {
		return nil, err
	}
	var hereChoices, other []TargetChoice
	for _, t := range targets {
		label := t.Target.Name
		if label == "" {
			label = t.Target.ID
		}
		onThisMachine := host != "" && t.Target.Host == host
		detail := []string{t.Target.Host}
		if onThisMachine {
			detail = append(detail, "this machine")
		}
		if t.Target.Project != "" {
			detail = append(detail, t.Target.Project)
		}
		if t.Mode != "" {
			detail = append(detail, t.Mode)
		}
		choice := TargetChoice{ID: t.Target.ID, Label: label + " (" + strings.Join(detail, ", ") + ")", Here: onThisMachine}
		if onThisMachine {
			hereChoices = append(hereChoices, choice)
		} else {
			other = append(other, choice)
		}
	}
	return append(hereChoices, other...), nil
}

func fetchTarget(server, token, id string) (store.SyncTarget, error) {
	targets, err := listTargets(server, token)
	if err != nil {
		return store.SyncTarget{}, err
	}
	for _, target := range targets {
		if target.Target.ID == id || slices.Contains(target.TargetAliases, id) {
			return target, nil
		}
	}
	return store.SyncTarget{}, errors.New("sync target not found in this account; configure it on Your account")
}

// chooseSyncMode asks whether to stop at the preview or continue to apply.
// Preview is the default so an empty answer never changes local files.
func chooseSyncMode(in io.Reader, out io.Writer) (bool, error) {
	for {
		answer, err := clientconfig.Ask(in, out, "Preview or apply (preview/apply)", "preview")
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "preview", "p", "1":
			return false, nil
		case "apply", "a", "2":
			return true, nil
		}
		fmt.Fprintln(out, "Enter preview or apply.")
	}
}

func RunSync(args []string, in io.Reader, out io.Writer) error {
	return RunSyncVersion(args, in, out, "dev")
}

func RunSyncVersion(args []string, in io.Reader, out io.Writer, version string) error {
	return runSyncWithOptions(args, in, out, func(args []string, cwd string) error {
		// Each install gets its own budget. The interactive session itself is
		// unbounded, so a person can read the plan and answer prompts without
		// racing a clock that started before they did.
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		defer cancel()
		return runNpxCommand(ctx, args, cwd, in, out)
	}, version, syncOptions{verifyEnrollment: true, preflight: func(actions []SyncAction) error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		if err := resolveNpx(actions, ctx, in, os.Stderr); err != nil {
			return err
		}
		return resolveGit(actions, ctx, in, os.Stderr)
	}})
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
	host := fs.String("env", clientconfig.EnvName(), "local environment name (default hostname)")
	apply := fs.Bool("apply", false, "apply the plan; without it you are offered preview or apply")
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
	tokenSource := clientconfig.SourceFlag
	if *token == "" {
		*token = os.Getenv("REET_TOKEN")
		tokenSource = clientconfig.SourceEnv
	}
	if *token == "" && *tokenFile != "" {
		value, source, err := clientconfig.ResolveTokenSource("", *tokenFile, strings.NewReader(""), os.Stderr)
		if err != nil {
			return err
		}
		*token, tokenSource = value, source
	}
	if c, e := loadAgentConfig(*config); e == nil {
		if *server == "" {
			*server = c.Server
		}
		if *token == "" && strings.TrimRight(*server, "/") == c.Server {
			*token = c.Token
			tokenSource = clientconfig.Source("this machine's enrollment in " + *config)
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
	cred, inputErr := clientconfig.NewCredential(*token, *tokenFile, *server, in, os.Stderr)
	if inputErr != nil {
		return inputErr
	}
	if *token != "" {
		cred.Source = tokenSource // NewCredential only sees the value, not where it was read
	}
	*token = cred.Token
	if *targetID == "" && clientconfig.Interactive(in) {
		// Offer the account's saved targets instead of asking for an ID by heart.
		var choices []TargetChoice
		listErr := clientconfig.Authorize(cred, func(token string) (err error) {
			choices, err = ListTargetChoices(*server, token, *host)
			return err
		})
		*token = cred.Token
		switch {
		case listErr != nil:
			return listErr
		case len(choices) == 0:
			fmt.Fprintf(os.Stderr, "No saved sync targets yet. Create one at %s/machines?view=compare (Machines -> Sync), then run reet sync again.\n", strings.TrimRight(*server, "/"))
		default:
			fmt.Fprintln(os.Stderr, "Saved targets (a target runs only on its own machine):")
			for i, c := range choices {
				fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, c.Label)
			}
			choice := ""
			if choices[0].Here {
				choice = "1"
			} else {
				fmt.Fprintln(os.Stderr, "No target is saved for this machine; another machine's target will be refused here.")
			}
			for {
				answer, err := clientconfig.Ask(in, os.Stderr, "Pick a number, or paste a target ID", choice)
				if err != nil {
					return err
				}
				n, convErr := strconv.Atoi(answer)
				if convErr != nil {
					if answer != "" {
						*targetID = answer
						break
					}
					continue
				}
				if n >= 1 && n <= len(choices) {
					*targetID = choices[n-1].ID
					break
				}
				fmt.Fprintf(os.Stderr, "Enter a number from 1 to %d, or paste a target ID.\n", len(choices))
			}
		}
	}

	targetValue, inputErr := clientconfig.Require(*targetID, "--target (target ID from Account)", false, in, os.Stderr)
	if inputErr != nil {
		return inputErr
	}
	*targetID = targetValue
	if *host == "" {
		*host, _ = os.Hostname()
	}
	var target store.SyncTarget
	err := clientconfig.Authorize(cred, func(token string) (err error) {
		target, err = fetchTarget(*server, token, *targetID)
		return err
	})
	*token = cred.Token
	if err != nil {
		return err
	}
	if options.onTarget != nil {
		options.onTarget(target)
	}
	if !strings.EqualFold(*host, target.Target.Host) && !slices.ContainsFunc(target.HostAliases, func(alias string) bool { return strings.EqualFold(alias, *host) }) {
		return fmt.Errorf("this command is for %s, not %s; run it on the target machine", target.Target.Host, *host)
	}
	*host = target.Target.Host
	if !*apply && clientconfig.Interactive(in) {
		choice, err := chooseSyncMode(in, os.Stderr)
		if err != nil {
			return err
		}
		*apply = choice
	}
	if *apply && options.verifyEnrollment {
		c, err := loadAgentConfig(*config)
		if err != nil {
			return fmt.Errorf("this machine is not enrolled, so apply is refused (preview still works). Enroll it from Machines -> Add a machine at %s, then run the printed reet agent enroll command: %w", strings.TrimRight(*server, "/"), err)
		}
		if strings.TrimRight(*server, "/") != c.Server {
			return fmt.Errorf("this machine is enrolled with another controller than %s; re-enroll it for this one before applying", strings.TrimRight(*server, "/"))
		}
		var a store.SyncAgent
		if err := agentRequest(c, "/api/v2/sync/identity", map[string]string{}, &a, version); err != nil {
			return err
		}
		if !strings.EqualFold(a.Host, *host) {
			return fmt.Errorf("this installation is enrolled as a different machine than %s; check reet agent enroll --env", target.Target.Host)
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
	projectMismatch := !filepath.IsAbs(target.Target.Project) ||
		filepath.Clean(resolvedCWD) != filepath.Clean(target.Target.Project)
	if target.Target.Project != "" && projectMismatch {
		return fmt.Errorf("run this command from project %s (currently %s)", target.Target.Project, resolvedCWD)
	}
	customClaude := target.Target.Project == "" && target.Target.Agent == "claude" &&
		os.Getenv("CLAUDE_CONFIG_DIR") != ""
	if customClaude && filepath.Clean(os.Getenv("CLAUDE_CONFIG_DIR")) != filepath.Join(home, ".claude") {
		return errors.New("custom CLAUDE_CONFIG_DIR requires manual sync")
	}
	customXDG := target.Target.Project == "" && target.Target.Agent == "opencode" &&
		os.Getenv("XDG_CONFIG_HOME") != ""
	if customXDG && filepath.Clean(os.Getenv("XDG_CONFIG_HOME")) != filepath.Join(home, ".config") {
		return errors.New("custom XDG_CONFIG_HOME requires manual sync")
	}
	targetPath := ""
	problems := map[string]string{}
	collect := func() ([]SyncAction, error) {
		problems = map[string]string{}
		desired := target.Desired
		if options.filterDesired != nil {
			desired = options.filterDesired(desired)
			if target.Canonical {
				included := map[string]bool{}
				for _, skill := range desired {
					included[skill.ItemID] = true
				}
				for _, skill := range target.Desired {
					if !included[skill.ItemID] {
						problems[skill.Slug] = "Local edits are waiting to upload; this item is retained."
					}
				}
			}
		}
		blocked := map[string]bool{}
		inventory := CollectScan(home, cwd, nil, *host)
		if err := inventory.Err(); err != nil && !target.Canonical {
			return nil, fmt.Errorf("scan incomplete; no changes allowed: %w", err)
		}
		actual := []Found{}
		// Agent-specific installs can share a canonical directory. Do not replace
		// another agent's same-name skill with a different or unknown source.
		for _, desired := range desired {
			for _, found := range inventory.Found {
				sameScope := found.Project == target.Target.Project && found.Agent != target.Target.Agent &&
					found.Host == *host
				sameName := found.Name == desired.Slug
				differentSource := canonicalSource(found.Source) != canonicalSource(desired.Source)
				conflict := sameScope && sameName
				if conflict && differentSource {
					// An unlinked local copy in another agent scope does not block
					// replacing this scope's copy. Known, different sources still do.
					if found.Source == "" {
						continue
					}
					if target.Canonical {
						blocked[desired.Slug] = true
						problems[desired.Slug] = "A different source uses this name in another app."
						continue
					}
					return nil, fmt.Errorf("%s is shared with %s from a different source; resolve it before syncing", found.Name, found.Agent)
				}
			}
		}
		for _, found := range inventory.Found {
			matches := found.Agent == target.Target.Agent &&
				found.Project == target.Target.Project
			if matches && found.Host == *host {
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
		if target.Canonical {
			// Only additive private items use this partial-plan rule. Parent
			// links and unknown entries are never interpreted as empty.
			if target.Mode != "additive" {
				return nil, errors.New("canonical library requires additive sync")
			}
			for _, want := range desired {
				candidate := filepath.Join(targetPath, want.Slug)
				if err := safeLibraryParent(targetPath); err != nil {
					return nil, err
				}
				info, err := os.Lstat(candidate)
				if err == nil {
					if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
						blocked[want.Slug] = true
						problems[want.Slug] = "The local entry is not a managed directory."
						continue
					}
					if _, err := privateSkillDigest(candidate); err != nil {
						blocked[want.Slug] = true
						problems[want.Slug] = "The local content cannot be read completely."
					} else if options.onExisting != nil {
						if err := options.onExisting(want, target, targetPath); err != nil {
							blocked[want.Slug] = true
							problems[want.Slug] = err.Error()
						}
					}
				} else if !os.IsNotExist(err) {
					blocked[want.Slug] = true
					problems[want.Slug] = "The local entry is unavailable."
				}
			}
			filtered := []store.SyncSkill{}
			for _, want := range desired {
				if !blocked[want.Slug] {
					filtered = append(filtered, want)
				}
			}
			desired = filtered
		}
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

		if target.Canonical && options.managedRemovals != nil {
			removals, blocked, err := options.managedRemovals(target, targetPath)
			if err != nil {
				return nil, err
			}
			for slug, message := range blocked {
				problems[slug] = message
			}
			actions = append(actions, removals...)
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
						matches := found.Name == action.Skill.Slug && found.Host == *host &&
							found.Project == target.Target.Project
						if matches && found.Agent != "agents" {
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
	plan := SyncPlan{Target: target.Target.ID, Directory: targetPath, Revision: target.Revision, Actions: actions, Problems: problems}
	if options.onPlan != nil {
		options.onPlan(plan)
	}
	if options.approvedHash != "" && options.approvedHash != plan.Hash() {
		return errApprovedPlanChanged
	}
	if len(actions) == 0 {
		fmt.Fprintln(out, "Already in sync.")
		return rescan()
	}
	if !*apply {
		fmt.Fprintln(out, "Preview only. Run reet sync again and choose apply to make these changes.")
		return nil
	}
	if options.preflight != nil {
		if err := options.preflight(actions); err != nil {
			return err
		}
	}
	if options.approvedHash == "" {
		fmt.Fprint(os.Stderr, "Removals move entries into .reet-sync-backups beside the skills folder. This may affect skills available to this agent. Type yes to apply: ")
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		answer := strings.TrimSpace(line)
		if !isYes(answer) && !strings.EqualFold(answer, target.Target.Name) {
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
		return errTargetChanged
	}
	current, err := collect()
	if err != nil {
		return err
	}
	old, _ := json.Marshal(actions)
	fresh, _ := json.Marshal(current)
	if string(old) != string(fresh) {
		return errLocalSkillsChanged
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
				operationErr = installPrivateSkill(*server, *token, action.Skill, targetPath, func(files []skillbundle.ContentFile) error {
					if options.onInstalled != nil {
						return options.onInstalled(action.Skill, targetPath, files)
					}
					return nil
				})
				if operationErr == nil && options.onInstallVerified != nil {
					operationErr = options.onInstallVerified(action.Skill, targetPath)
				}
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
				global := operationErr == nil && target.Target.Project == ""
				supported := agent == "cursor" || agent == "opencode"
				if global && supported {
					operationErr = linkSharedInstall(home, targetPath, action.Skill.Slug)
				}
			}
		} else {
			if action.RemovalIntent != nil {
				if options.removeManaged == nil {
					operationErr = errors.New("managed removal is not supported by this executor")
				} else {
					operationErr = options.removeManaged(action, targetPath, out, options.onBackup)
				}
			} else {
				operationErr = backupSkillEntry(targetPath, action.Skill.Slug, out, options.onBackup)
			}
		}
		if operationErr != nil {
			_ = rescan()
			hint := "rescan and preview again"
			if networkFailure(operationErr) {
				hint += "; check network access to registry.npmjs.org and github.com (or your proxy)"
			}
			return fmt.Errorf("sync stopped at %s: %w; %s", action.Skill.Slug, operationErr, hint)
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
		return errors.New("commands completed but the installed skills still differ from the plan; inspect the agent installation, then sync again")
	}
	fmt.Fprintln(out, "Sync complete. Updated inventory persisted on the controller.")
	return nil
}

// linkSharedInstall exposes a skill in an agent's own folder. The skills CLI
// treats Cursor and OpenCode as shared-folder agents and writes only to
// ~/.agents/skills, so without this link their scope never shows the install.
func linkSharedInstall(home, targetPath, slug string) error {
	canonical := filepath.Join(home, ".agents", "skills", slug)
	link := filepath.Join(targetPath, slug)
	_, err := os.Lstat(link)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Join(canonical, "SKILL.md")); err != nil {
		return fmt.Errorf("installed %s not found in the shared skills folder: %w", slug, err)
	}
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return err
	}
	rel, err := filepath.Rel(targetPath, canonical)
	if err != nil {
		rel = canonical
	}
	return os.Symlink(rel, link)
}

func backupSkillEntry(targetPath, slug string, out io.Writer, onBackup func(string), beforeMove ...func(string) error) error {
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
	for _, prepare := range beforeMove {
		if err := prepare(path); err != nil {
			_ = os.Remove(backup) // still empty; no content has moved
			return err
		}
	}
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
