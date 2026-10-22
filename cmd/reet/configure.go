package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/eyupio/reet.sh/internal/clientconfig"
)

func runConfigure(args []string, in io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("reet configure", flag.ContinueOnError)
	f.SetOutput(out)
	channel := f.String("version", "", "save release channel/version (REET_VERSION)")
	server := f.String("url", "", "save target server URL (REET_URL)")
	forgetToken := f.Bool("forget-token", false, "delete the saved personal API token")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected configure arguments")
	}
	set := map[string]bool{}
	f.Visit(func(v *flag.Flag) { set[v.Name] = true })
	if *forgetToken {
		if set["version"] || set["url"] {
			return errors.New("--forget-token cannot be combined with other settings")
		}
		path, err := clientconfig.ForgetToken()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Forgot any saved personal API token at %s\n", path)
		return nil
	}
	values := map[string]string{"REET_VERSION": *channel, "REET_URL": *server}
	for _, item := range []struct{ key, flag string }{{"REET_VERSION", "version"}, {"REET_URL", "url"}} {
		if !set[item.flag] {
			values[item.key] = os.Getenv(item.key)
			if item.key == "REET_VERSION" && values[item.key] == "" {
				values[item.key] = "latest"
				if version == "dev" {
					values[item.key] = "dev"
				}
			}
			if clientconfig.Interactive(in) {
				value, err := clientconfig.Prompt(in, out, item.key, values[item.key])
				if err != nil {
					return err
				}
				values[item.key] = value
			}
		}
		if err := clientconfig.Validate(item.key, values[item.key]); err != nil {
			return fmt.Errorf("%w; provide --%s or run reet configure in a terminal", err, item.flag)
		}
	}
	path, err := clientconfig.Save(values)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Saved REET_VERSION=%s and REET_URL=%s\nDefaults: %s\nFuture reet commands load these automatically; exported environment values and explicit flags override them.\n", values["REET_VERSION"], values["REET_URL"], path)
	return nil
}

func usesClientDefaults(command string, args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return false
		}
	}
	switch command {
	case "configure", "install", "init", "upgrade", "update", "doctor", "scan", "sync", "agent":
		return true
	case "healthcheck":
		for _, arg := range args {
			if arg == "--local" || arg == "--local=true" {
				return false
			}
		}
		return true
	}
	return false
}
