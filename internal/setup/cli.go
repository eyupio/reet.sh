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
		_, err := fmt.Fprintln(out, "Usage: reet setup scan [--json] [--project PATH] [--components] [--probe-versions] [--inspect-installation] [--export-policy]\n       reet setup backup --preview [--json] [--project PATH]\n       reet setup backup --password-file FILE [--dir DIR] [--json] [--project PATH]\n       reet setup list [--json] [--dir DIR]\n       reet setup inspect [--json] [--dir DIR] [--password-file FILE] SNAPSHOT\n       reet setup restore [--json] [--dir DIR] [--password-file FILE] [--project PATH] [--apply] SNAPSHOT\n\nDiscover known agent setup locations and executables locally.\nUse --components to read bounded configuration summaries without returning values.\nUse --probe-versions to invoke discovered agents with --version (3-second timeout each).\nUse --inspect-installation to read local package evidence without running commands.\nUse --export-policy to declare component and field selection for future encrypted snapshots.\n--preview shows the policy-driven selection without writing an archive.\nWithout --preview, write a passphrase-encrypted local setup snapshot.\nlist and inspect read clear headers; inspect opens the payload only with a passphrase.\nrestore previews a mapped plan; --apply creates only new files and refuses conflicts.")
		return err
	}
	switch args[0] {
	case "scan":
		return runScan(args[1:], out)
	case "backup":
		return runBackup(args[1:], out)
	case "list":
		return runList(args[1:], out)
	case "inspect":
		return runInspect(args[1:], out)
	case "restore":
		return runRestore(args[1:], out)
	default:
		return fmt.Errorf("unknown setup command %q; use reet setup --help", args[0])
	}
}

func runScan(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup scan", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON inventory")
	project := f.String("project", "", "also inspect known locations in this project (no recursive search)")
	components := f.Bool("components", false, "read known configuration files and summarise settings/MCP without values")
	installation := f.Bool("inspect-installation", false, "read bounded local Node package metadata associated with executables")
	versions := f.Bool("probe-versions", false, "invoke discovered executables with --version; timeout 3s each, output bounded")
	exportPolicy := f.Bool("export-policy", false, "declare component and field selection for future encrypted snapshots; reads no configuration values")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected setup scan arguments")
	}
	home, err := setupHome()
	if err != nil {
		return err
	}
	*project, err = setupProject(*project)
	if err != nil {
		return err
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
		return writeJSON(out, r)
	}
	return render(out, r)
}

func runBackup(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup backup", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON preview or result")
	preview := f.Bool("preview", false, "show policy-driven selection without writing an archive")
	passwordFile := f.String("password-file", "", "read the snapshot passphrase from this file, or - for standard input")
	dir := f.String("dir", "", "write snapshots here (default REET_SETUP_DIR or the user config directory)")
	project := f.String("project", "", "also consider known locations in this project (no recursive search)")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected setup backup arguments")
	}
	passphrase := ""
	var err error
	if !*preview {
		passphrase, err = readSetupPassphrase(*passwordFile)
		if err != nil {
			return err
		}
	}
	home, err := setupHome()
	if err != nil {
		return err
	}
	*project, err = setupProject(*project)
	if err != nil {
		return err
	}
	r, err := Discover(home, *project)
	if err != nil {
		return err
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		return err
	}
	if *preview {
		if *asJSON {
			return writeJSON(out, selection.Preview)
		}
		return renderSnapshotPreview(out, selection.Preview)
	}
	result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: *dir, Password: passphrase})
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, result)
	}
	return renderSnapshotWrite(out, result)
}

func runList(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup list", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON list")
	dir := f.String("dir", "", "read snapshots here (default REET_SETUP_DIR or the user config directory)")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected setup list arguments")
	}
	list, err := ListSetupSnapshots(*dir)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, list)
	}
	return renderSnapshotList(out, list)
}

func runInspect(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup inspect", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON inspection")
	dir := f.String("dir", "", "read snapshots here (default REET_SETUP_DIR or the user config directory)")
	passwordFile := f.String("password-file", "", "read the snapshot passphrase from this file, or - for standard input")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 1 {
		return fmt.Errorf("usage: reet setup inspect [--json] [--dir DIR] [--password-file FILE] SNAPSHOT")
	}
	passphrase := ""
	if *passwordFile != "" {
		var err error
		passphrase, err = readSetupPassphrase(*passwordFile)
		if err != nil {
			return err
		}
	}
	inspection, err := InspectSetupSnapshot(f.Arg(0), passphrase, *dir)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, inspection)
	}
	return renderSnapshotInspection(out, inspection)
}

func runRestore(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup restore", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON restore plan or apply result")
	dir := f.String("dir", "", "read snapshots here (default REET_SETUP_DIR or the user config directory)")
	passwordFile := f.String("password-file", "", "read the snapshot passphrase from this file, or - for standard input")
	project := f.String("project", "", "map project-scoped sources into this destination project")
	apply := f.Bool("apply", false, "create only new files from the plan; conflicts are refused")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 1 {
		return fmt.Errorf("usage: reet setup restore [--json] [--dir DIR] [--password-file FILE] [--project PATH] [--apply] SNAPSHOT")
	}
	passphrase, err := readSetupPassphrase(*passwordFile)
	if err != nil {
		return err
	}
	home, err := setupHome()
	if err != nil {
		return err
	}
	*project, err = setupProject(*project)
	if err != nil {
		return err
	}
	snapshot, manifest, err := loadSetupSnapshot(f.Arg(0), passphrase, *dir)
	if err != nil {
		return err
	}
	destination, err := Discover(home, *project)
	if err != nil {
		return err
	}
	plan := BuildRestorePlan(snapshot, manifest, destination)
	if *apply {
		result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: *dir})
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, result)
		}
		return renderRestoreApply(out, result)
	}
	if *asJSON {
		return writeJSON(out, plan)
	}
	return renderRestorePlan(out, plan)
}

func readSetupPassphrase(file string) (string, error) {
	if file == "" {
		return "", errors.New("a setup snapshot passphrase is required; give --password-file FILE or - for standard input")
	}
	var raw []byte
	var err error
	if file == "-" {
		raw, err = io.ReadAll(io.LimitReader(os.Stdin, 4097))
	} else {
		info, statErr := os.Lstat(file)
		if statErr != nil {
			return "", fmt.Errorf("inspect setup passphrase file: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", errors.New("the setup passphrase file must be a regular file, not a symlink")
		}
		if info.Size() > 4096 {
			return "", errors.New("the setup passphrase file is larger than 4 KiB")
		}
		raw, err = os.ReadFile(file)
	}
	if err != nil {
		return "", fmt.Errorf("read setup passphrase: %w", err)
	}
	if len(raw) > 4096 {
		return "", errors.New("the setup passphrase input is larger than 4 KiB")
	}
	passphrase := strings.TrimSpace(string(raw))
	if err := validateSetupPassphrase(passphrase); err != nil {
		return "", err
	}
	return passphrase, nil
}

func setupHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return home, nil
}

func setupProject(project string) (string, error) {
	if project == "" {
		return "", nil
	}
	resolved, err := filepath.Abs(project)
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}
	return resolved, nil
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
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

func renderSnapshotWrite(out io.Writer, result SetupWriteResult) error {
	if _, err := fmt.Fprintf(out, "Took setup snapshot %s (%d bytes) at %q.\n", result.ID, result.Bytes, result.Path); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Selected %d components; %d omissions recorded.\n", result.Components, result.Omissions); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "It is passphrase-encrypted on this machine. Setup restore is not implemented.")
	return err
}
