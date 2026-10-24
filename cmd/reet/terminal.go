package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
	"io"
	"os"
	"time"
)

func runTerminal(args []string, out io.Writer) error {
	return runTerminalInput(args, os.Stdin, out)
}
func runTerminalInput(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "Usage: reet terminal start|stop|restart|status [--data-dir PATH]\nManage controller web terminal availability locally. Running host shells are preserved.\nUse the controller's data directory and OS user; for containers run this command inside the controller container.")
		return nil
	}
	action := args[0]
	fs := flag.NewFlagSet("terminal "+action, flag.ContinueOnError)
	fs.SetOutput(out)
	dir := fs.String("data-dir", env("REET_DATA_DIR", "./data"), "controller data directory")
	envFile := fs.String("env-file", "", "configuration file (default REET_ENV_FILE or .env)")
	skipHostSetup := fs.Bool("skip-host-setup", false, "configure controller availability without offering local host enrollment")
	agentConfig := fs.String("agent-config", "", "local machine enrollment file for guided host setup")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected terminal command arguments")
	}
	values, err := readLocalEnv(*envFile)
	if err != nil {
		return err
	}
	lookup := func(key string) string {
		if value, exists := os.LookupEnv(key); exists {
			return value
		}
		return values[key]
	}
	explicitDir := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "data-dir" {
			explicitDir = true
		}
	})
	if !explicitDir && lookup("REET_DATA_DIR") != "" {
		*dir = lookup("REET_DATA_DIR")
	}
	publicURL := lookup("REET_PUBLIC_URL")
	if publicURL == "" {
		publicURL = lookup("REET_URL")
	}
	call := func(operation, origin, auth string) (terminalcontrol.Status, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return terminalcontrol.Command(ctx, *dir, operation, lookup("REET_REMOTE_TERMINALS") == "on", origin, auth)
	}
	var status terminalcontrol.Status
	if action == "start" || action == "restart" {
		status, err = call("status", "", "")
		if err != nil {
			return err
		}
		if action == "restart" && !status.Running {
			return errors.New("controller is not running; start reet serve first")
		}
		if publicURL == "" && status.PublicURL == "" {
			if err := clientconfig.Apply(); err != nil {
				return err
			}
			publicURL = os.Getenv("REET_URL")
		}
		var ask func(string, string) (string, error)
		if clientconfig.Interactive(in) {
			ask = func(key, current string) (string, error) { return clientconfig.Ask(in, out, key, current) }
		}
		var auth string
		publicURL, auth, err = prepareTerminalSetup(status, publicURL, lookup("REET_AUTH"), ask, out)
		if err != nil {
			return err
		}
		status, err = call(action, publicURL, auth)
	} else {
		status, err = call(action, "", "")
	}
	if err != nil {
		return err
	}
	enabled := "stopped"
	if status.Enabled {
		enabled = "active"
		if !status.Available {
			enabled = "configured on"
		}
	}
	fmt.Fprintf(out, "Remote terminals: %s\n", enabled)
	if !status.Running {
		fmt.Fprintln(out, "Controller: stopped. Saved setting will apply when reet serve starts.")
		return nil
	}
	fmt.Fprintln(out, "Controller: running")
	if status.Enabled && !status.Available {
		if status.RestartRequired {
			fmt.Fprintln(out, "Setup saved. Restart the controller once to activate accounts authentication; future commands reuse this setting.")
			if action != "status" {
				return errors.New("setup saved; controller restart required")
			}
			return nil
		}
		if status.Reason == "accounts_required" {
			fmt.Fprintln(out, "Availability: blocked by the running controller's authentication mode. Restart reet serve with the accounts configuration from your .env.")
		} else {
			fmt.Fprintln(out, "Availability: blocked; no usable public URL was found. Set REET_PUBLIC_URL or REET_URL in .env, or use --env-file PATH.")
		}
		if action != "status" {
			return errors.New("terminal access is configured on but prerequisites are not met")
		}
	} else if status.Available {
		fmt.Fprintln(out, "Availability: enabled.")
		if (action == "start" || action == "restart") && !*skipHostSetup && clientconfig.Interactive(in) {
			return scan.SetupTerminalHost(scan.TerminalSetupOptions{Config: *agentConfig, Server: publicURL, Token: lookup("REET_TOKEN"), Version: version, Ask: func(label, current string) (string, error) { return clientconfig.Ask(in, out, label, current) }}, in, out)
		}
	} else {
		fmt.Fprintln(out, "Availability: disabled. Running host shells are preserved.")
	}
	return nil
}
