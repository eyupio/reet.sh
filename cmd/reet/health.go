package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/cliui"
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
		u, err := url.Parse(*target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("--url must be an HTTP(S) server URL without credentials, query or fragment")
		}
		u.Path = strings.TrimRight(u.Path, "/")
		u.RawPath = strings.TrimRight(u.RawPath, "/")
		if strings.HasSuffix(u.Path, "/healthz") {
			endpoint = u.String()
		} else {
			endpoint, err = url.JoinPath(u.String(), "healthz")
			if err != nil {
				return fmt.Errorf("invalid healthcheck URL")
			}
		}
	}
	stop := cliui.Start(os.Stderr, "Checking server health")
	defer stop()
	return healthcheck(endpoint)
}

// healthURL turns a listen address such as ":8080" into the local /healthz URL.
func healthURL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = "8080"
	}
	return "http://127.0.0.1:" + port + "/healthz"
}

// healthcheck returns nil when the server at url answers 200; used by the container HEALTHCHECK.
func healthcheck(url string) error {
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %s", resp.Status)
	}
	return nil
}
