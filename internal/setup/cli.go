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
		_, err := fmt.Fprintln(out, "Usage: reet setup scan [--json] [--project PATH] [--components]\n\nDiscover known agent setup locations and executables locally.\nUse --components to read bounded configuration summaries without returning values.\nSetup backup and restore are not yet available.")
		return err
	}
	if args[0] != "scan" {
		return fmt.Errorf("unknown setup command %q; use reet setup --help", args[0])
	}
	f := flag.NewFlagSet("reet setup scan", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON inventory")
	project := f.String("project", "", "also inspect known locations in this project (no recursive search)")
	components := f.Bool("components", false, "read known configuration files and summarise settings/MCP without values")
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
	if *components {
		InspectComponents(&r)
	}
	if *asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(r)
	}
	return render(out, r)
}

func render(out io.Writer, r Report) error {
	mode := "Metadata only"
	if r.SchemaVersion == 2 {
		mode = "Configuration summaries; values omitted"
	}
	if _, err := fmt.Fprintf(out, "AI agent setup inventory (%s/%s)\n%s; setup backup and restore are not implemented.\n", r.OS, r.Architecture, mode); err != nil {
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
		if agent.Components != nil {
			for _, source := range agent.Components.Sources {
				if _, err := fmt.Fprintf(out, "  Config %q: %s (MCP: %s) %s\n", source.Path, source.State, source.MCPState, source.Issue); err != nil {
					return err
				}
				if s := source.Settings; s != nil {
					if _, err := fmt.Fprintf(out, "    Settings fields: model=%t hooks=%t permissions=%t packages=%t environment=%d references=%d literals=%d\n", s.ModelConfigured, s.HooksConfigured, s.PermissionsConfigured, s.PackagesConfigured, s.EnvironmentFields, s.ReferenceFields, s.LiteralFields); err != nil {
						return err
					}
				}
				for _, mcp := range source.MCP {
					if _, err := fmt.Fprintf(out, "    MCP %q: %s, %s; environment=%d headers=%d references=%d literals=%d\n", mcp.Name, mcp.Transport, mcp.State, mcp.EnvironmentFields, mcp.HeaderFields, mcp.ReferenceFields, mcp.LiteralFields); err != nil {
						return err
					}
				}
			}
			for _, note := range agent.Components.Limitations {
				if _, err := fmt.Fprintf(out, "  %s\n", note); err != nil {
					return err
				}
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
