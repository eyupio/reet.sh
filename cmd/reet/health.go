package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/cliui"
	"github.com/eyupio/reet.sh/internal/health"
)

func runHealthcheck(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet healthcheck", flag.ContinueOnError)
	f.SetOutput(out)
	target := f.String("url", os.Getenv("REET_URL"), "server base URL or /healthz URL (default: REET_URL/saved target)")
	local := f.Bool("local", false, "check local REET_ADDR; ignore saved/remote target")
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
		return healthcheck(healthURL(env("REET_ADDR", ":8080")))
	}
	if *target == "" && os.Getenv("REET_ADDR") == "" {
		value, err := clientconfig.ResolveURL("", os.Stdin, os.Stderr)
		if err != nil {
			return err
		}
		*target = value
	}
	endpoint := healthURL(env("REET_ADDR", ":8080"))
	if *target != "" {
		var err error
		endpoint, err = health.Endpoint(*target)
		if err != nil {
			return err
		}
	}
	stop := cliui.Start(os.Stderr, "Checking server health")
	defer stop()
	return healthcheck(endpoint)
}

// healthURL turns a listen address such as ":8080" into the local /healthz URL.
func healthURL(addr string) string { return health.URLFromAddr(addr) }

// healthcheck returns nil when the server at url answers 200; used by the container HEALTHCHECK.
func healthcheck(url string) error {
	_, err := health.Check(context.Background(), url)
	return err
}
