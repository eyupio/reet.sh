package lifecycle

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// App keeps host interaction replaceable so tests never install a real service.
type App struct {
	Version, Commit string
	In              io.Reader
	Out             io.Writer
	client          *http.Client
	run             runner
	executable      func() (string, error)
	api, downloads  string
}

func New(version, commit string, in io.Reader, out io.Writer) *App {
	return &App{Version: version, Commit: commit, In: in, Out: out, client: defaultClient(), run: execute, executable: os.Executable, api: releaseAPI, downloads: "https://github.com/eyupio/reet.sh/releases/download"}
}
func (a *App) Run(command string, args []string) error {
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
	fmt.Fprint(a.Out, "Apply these changes? Type yes: ")
	line, _ := bufio.NewReader(io.LimitReader(a.In, 4096)).ReadString('\n')
	if strings.TrimSpace(line) != "yes" {
		return fmt.Errorf("cancelled; no deployment changes applied (use --yes for unattended operation)")
	}
	return nil
}
func (a *App) install(args []string) error {
	f := flags("install", a.Out)
	mode := f.String("mode", "binary", "binary, native (user service), or compose")
	stateArg := f.String("state-dir", stateDefault(), "deployment configuration directory")
	dirArg := f.String("dir", binaryDefault(), "binary installation directory")
	dataArg := f.String("data-dir", dataDefault(), "server database directory (kept across upgrades)")
	addr := f.String("addr", "127.0.0.1:8080", "server HOST:PORT (loopback by default)")
	tag := f.String("version", "latest", "Compose image release: latest, dev or vX.Y.Z; native installs this binary")
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
	if *mode != "compose" && *tag != "latest" {
		return fmt.Errorf("--version selects a Compose image; use reet upgrade --version for a native binary")
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
	unlock, err := acquire(state)
	if err != nil {
		return err
	}
	defer unlock()
	// Recheck after locking in case another installer completed while confirmation was pending.
	if _, e := os.Lstat(filepath.Join(state, "deployment.json")); e == nil {
		return fmt.Errorf("deployment was created by another operation")
	}
	ctx := context.Background()
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
		build, e := a.run(ctx, candidate, "--version")
		if e != nil {
			return e
		}
		imageBuild, e := a.run(ctx, "docker", "run", "--rm", "--network", "none", "--read-only", d.Image, "--version")
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
			if _, err = a.run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
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
	fmt.Fprintln(a.Out, "Installed. Add the binary directory to PATH, then run reet doctor.")
	if *noStart && d.Mode == "native" {
		fmt.Fprintln(a.Out, "Start manually: systemctl --user daemon-reload && systemctl --user enable --now reet.service (Linux); launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/sh.reet.server.plist (macOS).")
	}
	return nil
}

func (a *App) upgrade(args []string) error {
	f := flags("upgrade", a.Out)
	stateArg := f.String("state-dir", stateDefault(), "managed deployment configuration directory")
	tag := f.String("version", "latest", "latest stable, dev or vX.Y.Z (explicit older tags allow downgrade)")
	check := f.Bool("check", false, "resolve the published tag without downloading or changing the installation")
	dry := f.Bool("dry-run", false, "download and verify, then show the plan without applying it")
	yes := f.Bool("yes", false, "apply without prompting")
	ok, err := parse(f, args)
	if !ok {
		return err
	}
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
	ctx := context.Background()
	resolved, err := resolveRelease(ctx, a.client, a.api, *tag)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Installed: %s (%s)\nRequested: %s\nDeployment: %s\nBinary: %s\n", d.Version, a.Commit, resolved, d.Mode, d.Binary)
	if *check {
		fmt.Fprintln(a.Out, "Tag lookup only. Use --dry-run to verify the binary and detect changes on the rolling dev channel.")
		return nil
	}
	stage, err := os.MkdirTemp("", "reet-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	candidate, err := fetchBinary(ctx, a.client, a.downloads, resolved, stage)
	if err != nil {
		return err
	}
	build, err := a.run(ctx, candidate, "--version")
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
		imageBuild, e := a.run(ctx, "docker", "run", "--rm", "--network", "none", "--read-only", next.Image, "--version")
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
			return err
		}
	}
	backup := ""
	if d.Mode != "binary" {
		backup, err = backupDB(ctx, d.Data, state)
		if err != nil {
			if running {
				if e = a.start(ctx, d, state); e != nil {
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
	if err = replaceBinary(d.Binary, previous); err != nil {
		if running {
			err = errors.Join(err, a.start(ctx, d, state))
		}
		return err
	}
	applied := false
	rollback := func(cause error) error {
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
	if err = replaceBinary(candidate, d.Binary); err != nil {
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
	fmt.Fprintln(a.Out, "Upgraded. Previous binary: "+previous)
	if d.Mode == "binary" {
		fmt.Fprintln(a.Out, "Restart any manually launched reet server to use the new binary.")
	} else if !running {
		fmt.Fprintln(a.Out, "Server was stopped and remains stopped; its first start may migrate the database. Backup retained.")
	}
	return nil
}
