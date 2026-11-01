package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/cliui"
	"github.com/eyupio/reet.sh/internal/health"
)

func runHealthcheck(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet healthcheck", flag.ContinueOnError)
	f.SetOutput(out)
	target := f.String("url", os.Getenv("REET_URL"), "server base URL or /healthz URL (default: REET_URL/saved target)")
	local := f.Bool("local", false, "check local REET_ADDR; ignore saved/remote target")
	noAnimation := f.Bool("no-animation", false, "disable terminal animations")
	nonInteractive := f.Bool("non-interactive", false, "never prompt for a missing target")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected healthcheck arguments")
	}
	if *local {
		explicitURL := false
		f.Visit(func(v *flag.Flag) {
			if v.Name == "url" {
				explicitURL = true
			}
		})
		if explicitURL {
			return fmt.Errorf("choose --local or --url")
		}
	}
	if !*local && *target == "" && os.Getenv("REET_ADDR") == "" {
		var input io.Reader = os.Stdin
		if *nonInteractive {
			input = strings.NewReader("")
		}
		value, err := clientconfig.ResolveURL("", input, os.Stderr)
		if err != nil {
			return err
		}
		*target = value
	}
	endpoint := healthURL(env("REET_ADDR", ":8080"))
	if !*local && *target != "" {
		var err error
		endpoint, err = health.Endpoint(*target)
		if err != nil {
			return err
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cliui.Task(ctx, os.Stdin, os.Stderr, "Checking server health", cliui.TaskOptions{Disabled: *nonInteractive || !cliui.Terminal(out), NoAnimation: *noAnimation}, func(ctx context.Context, observe func(string)) error {
		observe("Waiting for the server's /healthz response")
		_, err := health.Check(ctx, endpoint)
		return err
	})
}

// healthURL turns a listen address such as ":8080" into the local /healthz URL.
func healthURL(addr string) string { return health.URLFromAddr(addr) }

// healthcheck returns nil when the server at url answers 200; used by the container HEALTHCHECK.
func healthcheck(url string) error {
	_, err := health.Check(context.Background(), url)
	return err
}
