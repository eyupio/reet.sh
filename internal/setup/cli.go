package setup

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Run implements the standalone setup command. No server credentials are needed.
func Run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, "Usage: reet setup scan [--json] [--project PATH]\n\nDiscover known agent setup locations and executables locally.\nThis is metadata-only discovery; setup backup and restore are not yet available.")
		return err
	}
	if args[0] != "scan" {
		return fmt.Errorf("unknown setup command %q; use reet setup --help", args[0])
	}
	f := flag.NewFlagSet("reet setup scan", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON inventory")
	project := f.String("project", "", "also inspect known locations in this project (no recursive search)")
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected setup scan arguments")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	if *project != "" {
		*project, err = filepath.Abs(*project)
		if err != nil {
			return fmt.Errorf("resolve project directory: %w", err)
		}
	}
	r, err := Discover(home, *project)
	if err != nil {
		return err
	}
	if *asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(r)
	}
	return render(out, r)
}

func render(out io.Writer, r Report) error {
	if _, err := fmt.Fprintf(out, "AI agent setup inventory (%s/%s)\nMetadata only; setup backup and restore are not implemented.\n", r.OS, r.Architecture); err != nil {
		return err
	}
	for _, agent := range r.Agents {
		if _, err := fmt.Fprintf(out, "\n%s: %s\n  Executable %s: %s", agent.Name, agent.State, agent.Executable.Command, agent.Executable.State); err != nil {
			return err
		}
		if agent.Executable.Path != "" {
			if _, err := fmt.Fprintf(out, " (%q)", agent.Executable.Path); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if agent.Executable.Issue != "" {
			if _, err := fmt.Fprintf(out, "  %s\n", agent.Executable.Issue); err != nil {
				return err
			}
		}
		for _, loc := range agent.Locations {
			if _, err := fmt.Fprintf(out, "  %s %s: %s %q [%s]", loc.Scope, loc.Kind, loc.State, loc.Path, loc.Source); err != nil {
				return err
			}
			if loc.Issue != "" {
				if _, err := fmt.Fprintf(out, " (%s)", loc.Issue); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
	}
	for _, note := range r.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}
