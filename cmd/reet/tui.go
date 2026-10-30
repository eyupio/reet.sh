package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/lifecycle"
	"github.com/eyupio/reet.sh/internal/tui"
	"golang.org/x/term"
)

// tuiAvailable reports whether this process has the terminal Bubble Tea needs.
// REET_NO_TUI is handled by the caller so an explicit --tui still works.
func tuiAvailable() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// runTUI opens the read-only interface. screen is the command name already
// stripped of its leading "reet", or "" for the menu.
func runTUI(screen string, args []string, version, commit string) error {
	if err := clientconfig.Apply(); err != nil {
		return err
	}
	opts := tui.Options{
		Version: version,
		Commit:  commit,
		In:      os.Stdin,
		Out:     os.Stdout,
		NoColor: os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb",
	}
	switch screen {
	case "":
		if len(args) != 0 {
			return errors.New("unexpected arguments for the reet menu")
		}
		opts.Start = tui.ScreenHome
	case "doctor":
		fs := flag.NewFlagSet("reet doctor --tui", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		state := fs.String("state-dir", lifecycle.StateDefault(), "managed deployment configuration directory")
		server := fs.String("server", "", "controller URL (defaults to REET_URL or REET_CONTROLLER_URL)")
		envFile := fs.String("env-file", "", "read REET_TOKEN from an explicit env file")
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("unexpected doctor arguments")
		}
		opts.Start = tui.ScreenDoctor
		opts.DoctorStateDir = *state
		opts.DoctorServer = *server
		opts.DoctorTokenFile = *envFile
	case "healthcheck":
		fs := flag.NewFlagSet("reet healthcheck --tui", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		target := fs.String("url", os.Getenv("REET_URL"), "server base URL or /healthz URL (default: REET_URL/local)")
		local := fs.Bool("local", false, "check local REET_ADDR; ignore saved/remote target")
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("unexpected healthcheck arguments")
		}
		if *local {
			explicitURL := false
			fs.Visit(func(v *flag.Flag) {
				if v.Name == "url" {
					explicitURL = true
				}
			})
			if explicitURL {
				return errors.New("choose --local or --url")
			}
		}
		opts.Start = tui.ScreenHealth
		opts.HealthURL = *target
		opts.HealthLocal = *local
	case "version":
		if len(args) != 0 {
			return errors.New("unexpected version arguments")
		}
		opts.Start = tui.ScreenVersion
	case "terminal":
		action, rest, err := tuiAction("terminal", args)
		if err != nil {
			return err
		}
		if action != "status" {
			return fmt.Errorf("reet terminal %s --tui is not available yet; use reet terminal status --tui", action)
		}
		fs := flag.NewFlagSet("reet terminal status --tui", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("data-dir", "", "controller data directory (default REET_DATA_DIR or ./data)")
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("unexpected terminal arguments")
		}
		opts.Start = tui.ScreenTerminal
		opts.TerminalDataDir = *dir
	case "agent":
		action, rest, err := tuiAction("agent", args)
		if err != nil {
			return err
		}
		if action != "status" {
			return fmt.Errorf("reet agent %s --tui is not available yet; use reet agent status --tui", action)
		}
		fs := flag.NewFlagSet("reet agent status --tui", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		config := fs.String("config", "", "private machine enrollment file (default: user config directory)")
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("unexpected agent arguments")
		}
		opts.Start = tui.ScreenAgent
		opts.AgentConfig = *config
	default:
		return fmt.Errorf("reet %s --tui is not available yet", screen)
	}
	return tui.Run(context.Background(), opts)
}

// tuiAction takes the optional status word before a screen's flags.
func tuiAction(name string, args []string) (string, []string, error) {
	action := "status"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
		args = args[1:]
	}
	if action == "" {
		return "", nil, fmt.Errorf("usage: reet %s status --tui", name)
	}
	return action, args, nil
}
