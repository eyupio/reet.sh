package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
	"io"
	"os"
	"time"
)

func runTerminal(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "Usage: reet terminal start|stop|restart|status [--data-dir PATH]\nManage controller web terminal availability locally. Running host shells are preserved.\nUse the controller's data directory and OS user; for containers run this command inside the controller container.")
		return nil
	}
	action := args[0]
	fs := flag.NewFlagSet("terminal "+action, flag.ContinueOnError)
	fs.SetOutput(out)
	dir := fs.String("data-dir", env("REET_DATA_DIR", "./data"), "controller data directory")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected terminal command arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	status, err := terminalcontrol.Command(ctx, *dir, action, os.Getenv("REET_REMOTE_TERMINALS") == "on")
	if err != nil {
		return err
	}
	enabled := "stopped"
	if status.Enabled {
		enabled = "started"
	}
	fmt.Fprintf(out, "Remote terminals: %s\n", enabled)
	if !status.Running {
		fmt.Fprintln(out, "Controller: stopped. Saved setting will apply when reet serve starts.")
		return nil
	}
	fmt.Fprintln(out, "Controller: running")
	if status.Enabled && !status.Available {
		fmt.Fprintln(out, "Availability: blocked; configure accounts mode and an HTTPS REET_PUBLIC_URL (loopback HTTP is allowed for development).")
		if action != "status" {
			return errors.New("terminal access is configured on but prerequisites are not met")
		}
	} else if status.Available {
		fmt.Fprintln(out, "Availability: enabled. Host-local terminal consent is still required.")
	} else {
		fmt.Fprintln(out, "Availability: disabled. Running host shells are preserved.")
	}
	return nil
}
