package setup

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Run implements the standalone setup command. No server credentials are needed.
func Run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, "Usage: reet setup scan [--json] [--project PATH] [--components] [--probe-versions] [--inspect-installation] [--export-policy]\n\nDiscover known agent setup locations and executables locally.\nUse --components to read bounded configuration summaries without returning values.\nUse --probe-versions to invoke discovered agents with --version (3-second timeout each).\nUse --inspect-installation to read local package evidence without running commands.\nUse --export-policy to declare component and field selection for future encrypted snapshots.\nSetup backup and restore are not yet available.")
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
	installation := f.Bool("inspect-installation", false, "read bounded local Node package metadata associated with executables")
	versions := f.Bool("probe-versions", false, "invoke discovered executables with --version; timeout 3s each, output bounded")
	exportPolicy := f.Bool("export-policy", false, "declare component and field selection for future encrypted snapshots; reads no configuration values")
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
	if *exportPolicy {
		ApplyExportPolicies(&r)
	}
	if *installation {
		InspectInstallations(&r)
	}
	if *versions {
		if err := ProbeVersions(&r); err != nil {
			return err
		}
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
	if len(r.Agents) > 0 && r.Agents[0].Components != nil {
		mode = "Configuration summaries; values omitted"
	}
	if r.InstallationInspection {
		mode += "; local package inspection enabled"
	}
	if r.VersionProbes {
		mode += "; application version probes enabled"
	}
	if r.ExportPolicies {
		mode += "; export policy declared, no export implemented"
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
		if v := agent.Installation; v != nil {
			if _, err := fmt.Fprintf(out, "  Installation: %s %s %s (kind: %s; manager: %s) %s\n", v.State, v.Package, v.Version, v.Kind, v.Manager, v.Issue); err != nil {
				return err
			}
		}
		if v := agent.ApplicationVersion; v != nil {
			if _, err := fmt.Fprintf(out, "  Application version: %s %s %s (installation method: %s)\n", v.State, v.Version, v.Issue, v.InstallMethod); err != nil {
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
		if agent.ExportPolicy != nil {
			if err := renderExportPolicy(out, *agent.ExportPolicy); err != nil {
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

func renderExportPolicy(out io.Writer, policy ExportPolicy) error {
	if _, err := fmt.Fprintf(out, "  Export policy (version %d; declared only, no export implemented):\n", policy.PolicyVersion); err != nil {
		return err
	}
	for _, source := range policy.Sources {
		if _, err := fmt.Fprintf(out, "    %q (%s, %s, %s)\n", source.Path, source.Scope, source.Observed, source.Format); err != nil {
			return err
		}
		for _, component := range source.Components {
			if _, err := fmt.Fprintf(out, "      %s: %s, %s; unknown fields %s\n", component.Kind, component.Selection, component.Handling, component.UnlistedFields); err != nil {
				return err
			}
			for _, state := range []string{"include", "encrypted_only", "reference_only", "exclude"} {
				if fields := fieldPaths(component.Fields, state); len(fields) > 0 {
					if _, err := fmt.Fprintf(out, "        %s: %s\n", state, strings.Join(fields, ", ")); err != nil {
						return err
					}
				}
			}
			if component.Reason != "" {
				if _, err := fmt.Fprintf(out, "        %s\n", component.Reason); err != nil {
					return err
				}
			}
		}
	}
	for _, class := range policy.Classes {
		if _, err := fmt.Fprintf(out, "    %s: %s, %s — %s\n", class.Kind, class.Selection, class.Handling, class.Reason); err != nil {
			return err
		}
	}
	return nil
}

func fieldPaths(fields []FieldExportPolicy, state string) []string {
	paths := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.State == state {
			paths = append(paths, field.Path)
		}
	}
	return paths
}
