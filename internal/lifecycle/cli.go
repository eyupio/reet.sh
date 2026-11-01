package lifecycle

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/cliui"
)

// App keeps host interaction replaceable so tests never install a real service.
type App struct {
	Version, Commit string
	In              io.Reader
	Out             io.Writer
	// Progress receives transient progress lines. It defaults to os.Stderr so
	// structured and redirected commands disable presentation on both streams.
	Progress                    io.Writer
	client                      *http.Client
	interactiveRun              func(context.Context, string, ...string) error
	run                         runner
	executable                  func() (string, error)
	api, downloads              string
	ctx                         context.Context
	noAnimation, nonInteractive bool
	structured                  bool
	recovering                  bool
	// Test seam for the presentation boundary, independent of upgrade work.
	taskRunner func(context.Context, string, bool, cliui.Work) error
}

func New(version, commit string, in io.Reader, out io.Writer) *App {
	return &App{Version: version, Commit: commit, In: in, Out: out, Progress: os.Stderr, interactiveRun: interactiveCommand(in, out), client: defaultClient(), run: execute, executable: os.Executable, api: releaseAPI, downloads: "https://github.com/eyupio/reet.sh/releases/download"}
}
func (a *App) Run(command string, args []string) error {
	return a.RunContext(context.Background(), command, args)
}

func (a *App) RunContext(ctx context.Context, command string, args []string) error {
	previous := a.ctx
	a.ctx = ctx
	defer func() { a.ctx = previous }()
	switch command {
	case "install", "init":
		return a.install(args)
	case "upgrade", "update":
		return a.upgrade(args)
	case "doctor":
		return a.doctor(args)
	}
	return fmt.Errorf("unknown lifecycle command")
}
func flags(name string, out io.Writer) *flag.FlagSet {
	f := flag.NewFlagSet("reet "+name, flag.ContinueOnError)
	f.SetOutput(out)
	return f
}
func parse(f *flag.FlagSet, args []string) (bool, error) {
	err := f.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if f.NArg() != 0 {
		return false, fmt.Errorf("unexpected positional arguments")
	}
	return true, nil
}
func (a *App) confirm(yes bool) error {
	if yes {
		return nil
	}
	if a.nonInteractive {
		return fmt.Errorf("cancelled; no deployment changes applied (use --yes for unattended operation)")
	}
	line, err := clientconfig.Ask(a.In, a.Out, "Apply these changes? Type yes", "")
	if err != nil || strings.TrimSpace(line) != "yes" {
		return fmt.Errorf("cancelled; no deployment changes applied (use --yes for unattended operation)")
	}
	return nil
}
func (a *App) install(args []string) error {
	f := flags("install", a.Out)
	a.progressFlags(f)
	mode := f.String("mode", "binary", "binary, native (user service), or compose")
	stateArg := f.String("state-dir", stateDefault(), "deployment configuration directory")
	dirArg := f.String("dir", binaryDefault(), "binary installation directory")
	dataArg := f.String("data-dir", dataDefault(), "server database directory (kept across upgrades)")
	addr := f.String("addr", "127.0.0.1:8080", "server HOST:PORT (loopback by default)")
	tag := f.String("version", "", "Compose release (default: REET_VERSION or latest); native installs this binary")
	skipTerminalSetup := f.Bool("skip-terminal-setup", false, "skip the interactive terminal connection wizard")
	noStart := f.Bool("no-start", false, "write configuration without starting/enabling the server")
	yes := f.Bool("yes", false, "apply without prompting")
	dry := f.Bool("dry-run", false, "show the plan without changing the deployment")
	ok, err := parse(f, args)
	if !ok {
		return err
	}
	if *mode != "binary" && *mode != "native" && *mode != "compose" {
		return fmt.Errorf("mode must be binary, native or compose")
	}
	if *mode != "compose" && *tag != "" && *tag != "latest" {
		return fmt.Errorf("--version selects a Compose image; use reet upgrade --version for a native binary")
	}
	if *mode == "compose" {
		if *tag == "" {
			*tag = os.Getenv("REET_VERSION")
			if *tag == "" {
				*tag = "latest"
			}
		}
		normalised, e := normaliseTag(*tag)
		if e != nil {
			return e
		}
		*tag = normalised
	}
	state, err := absolute(*stateArg)
	if err != nil {
		return err
	}
	dir, err := absolute(*dirArg)
	if err != nil {
		return err
	}
	data, err := absolute(*dataArg)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(filepath.Join(state, "deployment.json")); err == nil {
		return fmt.Errorf("deployment already exists; use reet upgrade --state-dir %s", strconv.Quote(state))
	} else if !os.IsNotExist(err) {
		return err
	}
	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("--addr must be HOST:PORT")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid server port")
	}
	if host != "" && host != "localhost" && net.ParseIP(host) == nil {
		return fmt.Errorf("--addr host must be localhost or an IP address")
	}
	source, err := a.executable()
	if err != nil {
		return err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "reet")
	if info, e := os.Lstat(target); e == nil && (info.Mode()&os.ModeSymlink != 0 || !sameBinary(source, target)) {
		return fmt.Errorf("%s already contains another binary or symlink; upgrade it or choose --dir", target)
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	var unit string
	if *mode == "native" {
		unit, err = unitPath()
		if err != nil {
			return err
		}
		if _, e := os.Lstat(unit); e == nil {
			return fmt.Errorf("existing service at %s; refusing to replace it", unit)
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	d := &deployment{Mode: *mode, Binary: target, Data: data, Addr: *addr, Version: a.Version, Environment: map[string]string{}}
	if d.Mode == "binary" {
		d.Data = ""
		d.Addr = ""
	} else {
		for _, k := range serverEnv {
			if v, ok := os.LookupEnv(k); ok {
				d.Environment[k] = v
			}
		}
	}
	fmt.Fprintf(a.Out, "Install %s deployment\nBinary: %s\nConfiguration: %s\n", d.Mode, target, state)
	if d.Mode != "binary" {
		fmt.Fprintf(a.Out, "Data: %s (preserved)\nListen: %s\nStart now: %t\nServer settings from REET_* / SKILLS_SH_TOKEN are saved privately; secret values are not printed.\n", data, *addr, !*noStart)
	}
	if d.Mode == "compose" {
		fmt.Fprintf(a.Out, "Image channel: %s (resolved and pinned before starting)\n", *tag)
	}
	if *dry {
		return nil
	}
	if err = a.confirm(*yes); err != nil {
		return err
	}
	ctx, stopSignals := signal.NotifyContext(a.context(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	unlock, err := acquire(state)
	if err != nil {
		return err
	}
	defer unlock()
	// Recheck after locking in case another installer completed while confirmation was pending.
	if _, e := os.Lstat(filepath.Join(state, "deployment.json")); e == nil {
		return fmt.Errorf("deployment was created by another operation")
	}
	if d.Mode == "compose" {
		resolved, e := resolveRelease(ctx, a.client, a.api, *tag)
		if e != nil {
			return e
		}
		d.Version = resolved
		d.Image, e = a.pull(ctx, resolved)
		if e != nil {
			return e
		}

		stage, e := os.MkdirTemp("", "reet-install-*")
		if e != nil {
			return e
		}
		defer os.RemoveAll(stage)
		candidate, e := fetchBinary(ctx, a.client, a.downloads, resolved, stage)
		if e != nil {
			return e
		}
		build, e := a.command(ctx, candidate, "--version")
		if e != nil {
			return e
		}
		imageBuild, e := a.command(ctx, "docker", "run", "--rm", "--network", "none", "--read-only", d.Image, "--version")
		if e != nil {
			return e
		}
		if strings.TrimSpace(build) != strings.TrimSpace(imageBuild) {
			return fmt.Errorf("image and binary builds differ; retry after publication completes")
		}
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if !sameBinary(source, target) {
		if err = replaceBinary(source, target); err != nil {
			return err
		}
	}
	if d.Mode != "binary" {
		if err = os.MkdirAll(data, 0700); err != nil {
			return err
		}
	}
	if err = writeJSON(filepath.Join(state, "deployment.json"), d); err != nil {
		return err
	}
	if d.Mode == "native" {
		if err = os.MkdirAll(filepath.Dir(unit), 0700); err != nil {
			return err
		}
		uf, e := os.OpenFile(unit, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = uf.WriteString(nativeUnit(d, state))
		ce := uf.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		if !*noStart && strings.HasSuffix(unit, ".service") {
			if _, err = a.command(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
				return err
			}
			if _, err = a.native(ctx, "enable"); err != nil {
				return err
			}
		}
	}
	if d.Mode == "compose" {
		if err = a.writeCompose(d, state); err != nil {
			return err
		}
	}
	if d.Mode != "binary" && !*noStart {
		if err = a.start(ctx, d, state); err != nil {
			return fmt.Errorf("configuration saved; server start failed: %w (run reet doctor)", err)
		}
		if err = a.waitHealthy(ctx, d); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.Out, "Reet, installed. Binary: %s\n", target)
	fmt.Fprintf(a.Out, "Check this deployment: %s doctor --state-dir %s\n", shellQuote(target), shellQuote(state))
	if d.Mode == "binary" {
		fmt.Fprintln(a.Out, "Connect to an existing site: open Account on that site, then Connect a machine.")
	} else {
		fmt.Fprintf(a.Out, "Open %s after the server starts. Create the owner account on a new instance.\nData: %s (keep this directory when updating)\n", strings.TrimSuffix(localHealth(d.Addr), "/healthz"), data)
		if d.Mode == "native" && runtime.GOOS == "linux" {
			fmt.Fprintln(a.Out, "Status: systemctl --user status reet\nLogs: journalctl --user -u reet")
		} else if d.Mode == "native" {
			fmt.Fprintf(a.Out, "Logs: %s\n", filepath.Join(state, "server.log"))
		} else {
			fmt.Fprintf(a.Out, "Compose configuration: %s\n", filepath.Join(state, "compose.json"))
		}
	}
	if *noStart && d.Mode == "native" {
		fmt.Fprintln(a.Out, "Start manually: systemctl --user daemon-reload && systemctl --user enable --now reet.service (Linux); launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/sh.reet.server.plist (macOS).")
	}
	stopSignals()
	a.offerTerminalSetup(d, state, !*noStart, *yes || *skipTerminalSetup || a.nonInteractive)
	return nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func (a *App) upgrade(args []string) error {
	f := flags("upgrade", a.Out)
	a.progressFlags(f)
	stateArg := f.String("state-dir", stateDefault(), "managed deployment configuration directory")
	tag := f.String("version", os.Getenv("REET_VERSION"), "latest, dev or vX.Y.Z; default: REET_VERSION, then installed channel (explicit tags allow downgrade)")
	skipTerminalSetup := f.Bool("skip-terminal-setup", false, "skip the interactive terminal connection wizard")
	check := f.Bool("check", false, "resolve the published tag without downloading or changing the installation")
	dry := f.Bool("dry-run", false, "download and verify, then show the plan without applying it")
	yes := f.Bool("yes", false, "apply without prompting")
	ok, err := parse(f, args)
	if !ok {
		return err
	}
	explicitVersion := false
	f.Visit(func(v *flag.Flag) {
		if v.Name == "version" {
			explicitVersion = true
		}
	})
	if *check && *dry {
		return fmt.Errorf("choose --check or --dry-run")
	}
	state, err := absolute(*stateArg)
	if err != nil {
		return err
	}
	d, err := load(state)
	if os.IsNotExist(err) {
		target, e := a.executable()
		if e != nil {
			return e
		}
		target, e = filepath.EvalSymlinks(target)
		if e != nil {
			return e
		}
		d = &deployment{Mode: "binary", Binary: target, Version: a.Version}
	} else if err != nil {
		return err
	}
	if *tag == "" {
		// A standalone installer can replace the executable without updating the
		// deployment record. Prefer this binary's build when it is the target;
		// Compose instead follows its recorded image, not the managing CLI.
		installed := d.Version
		if d.Mode != "compose" {
			path, err := a.executable()
			if err != nil {
				return fmt.Errorf("cannot identify running binary; select --version explicitly")
			}
			running, err := os.Stat(path)
			target, targetErr := os.Stat(d.Binary)
			if err != nil || targetErr != nil {
				return fmt.Errorf("cannot inspect upgrade target; select --version explicitly")
			}
			if os.SameFile(running, target) {
				installed = a.Version
			}
		}
		if installed == "" || installed == "unknown" {
			if a.nonInteractive || !clientconfig.Interactive(a.In) {
				return fmt.Errorf("release channel is missing; set REET_VERSION, pass --version or run reet configure")
			}
			choice, e := clientconfig.Prompt(a.In, a.Out, "REET_VERSION", "")
			if e != nil {
				return e
			}
			if _, e = clientconfig.Save(map[string]string{"REET_VERSION": choice}); e != nil {
				return e
			}
			*tag = choice
		} else {
			*tag = "latest"
		}
		if installed == "dev" {
			*tag = "dev"
		}
	}
	ctx := a.context()
	var resolved string
	err = a.task(ctx, "Checking release channel", false, func(ctx context.Context, observe func(string)) error {
		observe("Resolving the requested release")
		var e error
		resolved, e = resolveRelease(ctx, a.client, a.api, *tag)
		return e
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Running CLI: %s (%s)\nRecorded deployment version: %s\nRequested: %s\nDeployment: %s\nBinary: %s\n", a.Version, a.Commit, d.Version, resolved, d.Mode, d.Binary)
	if *check {
		fmt.Fprintln(a.Out, "Tag lookup only. Use --dry-run to verify the binary and detect changes on the rolling dev channel.")
		return nil
	}
	// REET_VERSION can have been inherited from an old saved preference. A
	// generic "yes" must not silently replace the running dev CLI with stable
	// and remove the very commands needed to recover it. Inspection stays usable;
	// explicit --version still permits an intentional channel switch/downgrade.
	if a.Version == "dev" && resolved != "dev" && d.Mode != "compose" && !explicitVersion {
		path, e := a.executable()
		if e != nil {
			return fmt.Errorf("cannot identify running binary; select --version explicitly")
		}
		running, e := os.Stat(path)
		target, targetErr := os.Stat(d.Binary)
		if e != nil || targetErr != nil {
			return fmt.Errorf("cannot inspect upgrade target; select --version explicitly")
		}
		if os.SameFile(running, target) {
			warning := fmt.Sprintf("Configured release selects %s, but this executable is dev. Use --version dev to stay on dev, or --version %s to explicitly switch channels.", resolved, resolved)
			if !*dry {
				return fmt.Errorf("%s No installation files were changed", warning)
			}
			fmt.Fprintln(a.Out, "Channel change preview:", warning)
		}
	}
	stage, err := os.MkdirTemp("", "reet-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	var candidate string
	err = a.task(ctx, "Downloading and verifying update", false, func(ctx context.Context, observe func(string)) error {
		observe("Verifying the release archive against its SHA-256 checksum")
		var e error
		candidate, e = fetchBinary(ctx, a.client, a.downloads, resolved, stage)
		return e
	})
	if err != nil {
		return err
	}
	build, err := a.command(ctx, candidate, "--version")
	if err != nil {
		return fmt.Errorf("verified release binary cannot run on this host")
	}
	fields := strings.Fields(build)
	if len(fields) != 3 || fields[0] != "reet" || strings.TrimPrefix(fields[1], "v") != strings.TrimPrefix(resolved, "v") {
		return fmt.Errorf("release binary version does not match the requested tag")
	}
	fmt.Fprintf(a.Out, "Verified: %s\n", strings.TrimSpace(build))
	if d.Mode == "binary" && sameBinary(candidate, d.Binary) {
		fmt.Fprintln(a.Out, "Already up to date.")
		if !*dry {
			a.offerTerminalSetup(d, state, false, *yes || *skipTerminalSetup || a.nonInteractive)
		}
		return nil
	}
	fmt.Fprintln(a.Out, "Keep the previous binary; for managed servers, stop, back up SQLite, update, restart if previously running, and verify health. Restore the previous deployment on failure.")
	if *dry {
		if d.Mode == "compose" {
			fmt.Fprintln(a.Out, "Compose image will be pulled and checked against this binary before stopping the server.")
		}
		return nil
	}
	if err = a.confirm(*yes); err != nil {
		return err
	}
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err = ctx.Err(); err != nil {
		return err
	}
	unlock, err := acquire(state)
	if err != nil {
		return err
	}
	defer unlock()
	// Do not apply a stale plan when a concurrent operation changed the manifest.
	current, e := load(state)
	if e == nil {
		old, _ := jsonBytes(d)
		now, _ := jsonBytes(current)
		if string(old) != string(now) {
			return fmt.Errorf("deployment changed; retry upgrade")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	next := *d
	next.Version = resolved
	if d.Mode == "compose" {
		next.Image, err = a.pull(ctx, resolved)
		if err != nil {
			return err
		}
		imageBuild, e := a.command(ctx, "docker", "run", "--rm", "--network", "none", "--read-only", next.Image, "--version")
		if e != nil {
			return e
		}
		if strings.TrimSpace(imageBuild) != strings.TrimSpace(build) {
			return fmt.Errorf("image and binary builds differ; publication may still be running, retry later (server unchanged)")
		}
	}
	running := false
	if d.Mode != "binary" {
		running, err = a.active(ctx, d, state)
		if err != nil {
			return err
		}
		if err = a.stop(ctx, d, state); err != nil {
			if running {
				return errors.Join(err, a.recoverServer(ctx, d, state))
			}
			return err
		}
	}
	backup := ""
	if d.Mode != "binary" {
		err = a.task(ctx, "Backing up and verifying SQLite", false, func(ctx context.Context, observe func(string)) error {
			observe("Taking a consistent copy, including committed WAL data")
			var e error
			backup, e = backupDB(ctx, d.Data, state)
			return e
		})
		if err != nil {
			if running {
				if e = a.recoverServer(ctx, d, state); e != nil {
					return errors.Join(err, fmt.Errorf("old server could not restart: %w", e))
				}
			}
			return err
		}
		if backup != "" {
			fmt.Fprintf(a.Out, "Database backup: %s\n", backup)
		}
	}
	previous := d.Binary + ".previous"
	if err = a.step(ctx, "Keeping the previous binary", func(context.Context) error { return replaceBinary(d.Binary, previous) }); err != nil {
		if running {
			err = errors.Join(err, a.recoverServer(ctx, d, state))
		}
		return err
	}
	applied := false
	rollback := func(cause error) error {
		// A cancelled operation still needs a live context for recovery.
		ctx, cancel := recoveryContext(ctx)
		defer cancel()
		previousPresentation := a.recovering
		a.recovering = true
		defer func() { a.recovering = previousPresentation }()
		// Never restore database bytes while a new server might still be writing them.
		if d.Mode != "binary" {
			if e := a.stop(ctx, &next, state); e != nil {
				return errors.Join(cause, fmt.Errorf("rollback could not stop server; database backup retained at %s; recover manually: %w", backup, e))
			}
		}
		var recovery []error
		recovery = append(recovery, replaceBinary(previous, d.Binary))
		if d.Mode != "binary" {
			recovery = append(recovery, restoreDB(backup, d.Data))
			if d.Mode == "compose" {
				recovery = append(recovery, a.writeCompose(d, state))
			}
		}
		if applied {
			recovery = append(recovery, writeJSON(filepath.Join(state, "deployment.json"), d))
		}
		if e := errors.Join(recovery...); e != nil {
			return errors.Join(cause, fmt.Errorf("rollback needs manual recovery: %w", e))
		}
		if running {
			if e := a.start(ctx, d, state); e != nil {
				return errors.Join(cause, fmt.Errorf("previous server restart failed: %w", e))
			}
			if e := a.waitHealthy(ctx, d); e != nil {
				return errors.Join(cause, e)
			}
		}
		return fmt.Errorf("upgrade failed; previous deployment restored: %w", cause)
	}
	if err = a.step(ctx, "Replacing the verified binary", func(context.Context) error { return replaceBinary(candidate, d.Binary) }); err != nil {
		return rollback(err)
	}
	if d.Mode == "compose" {
		if err = a.writeCompose(&next, state); err != nil {
			return rollback(err)
		}
	}
	if err = writeJSON(filepath.Join(state, "deployment.json"), &next); err != nil {
		return rollback(err)
	}
	applied = true
	if running {
		if err = a.start(ctx, &next, state); err != nil {
			return rollback(err)
		}
		if err = a.waitHealthy(ctx, &next); err != nil {
			return rollback(err)
		}
	}
	if err = ctx.Err(); err != nil {
		return rollback(err)
	}
	fmt.Fprintln(a.Out, "Upgraded. Previous binary: "+previous)
	if d.Mode == "binary" {
		fmt.Fprintln(a.Out, "Restart any manually launched reet server to use the new binary.")
	} else if !running {
		fmt.Fprintln(a.Out, "Server was stopped and remains stopped; its first start may migrate the database. Backup retained.")
	}
	stopSignals()
	a.offerTerminalSetup(&next, state, running, *yes || *skipTerminalSetup || a.nonInteractive)
	return nil
}
