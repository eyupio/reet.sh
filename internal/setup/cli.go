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
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"golang.org/x/term"
)

// ProfileRemote is an optional server-side profile store.
type ProfileRemote interface {
	Push(server, token string, profile SetupProfile) error
	Pull(server, token, name string) (SetupProfile, error)
}

// Run implements the standalone setup command. No server credentials are needed
// unless profile push/pull is used with a supplied remote.
func Run(args []string, out io.Writer, remote ...ProfileRemote) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, `Usage: reet setup scan [--json] [--project PATH] [--components] [--probe-versions] [--inspect-installation] [--export-policy]
       reet setup backup --preview [--json] [--project PATH]
       reet setup backup [--password-file FILE] [--dir DIR] [--json] [--project PATH] [--probe-versions] [--inspect-installation] [--include-credentials] [--s3]
       reet setup list [--json] [--dir DIR]
       reet setup inspect [--profile NAME [--revision N] | SNAPSHOT] [--json] [--dir DIR] [--password-file FILE]
       reet setup restore [--profile NAME [--revision N] | SNAPSHOT] [--json] [--dir DIR] [--password-file FILE] [--project PATH] [--probe-versions] [--inspect-installation] [--apply] [--resume JOURNAL] [--on-conflict skip|replace] [--conflict PATH=skip|replace]...
       reet setup verify [--profile NAME [--revision N] | SNAPSHOT] [--json] [--dir DIR] [--password-file FILE] [--project PATH] [--probe-versions] [--inspect-installation]
       reet setup rollback (--journal FILE | --last) [--profile NAME [--revision N] | SNAPSHOT] [--json] [--dir DIR] [--password-file FILE] [--project PATH]
       reet setup journals [--json] [--dir DIR]
       reet setup health [--json] [--project PATH] [--probe-versions] [--inspect-installation]
       reet setup fetch --s3 [--profile NAME [--revision N] | SNAPSHOT] [--json] [--dir DIR]
       reet setup remote (list [--json] | test [--json] | delete --yes [--json] SNAPSHOT | prune --keep N [--apply] [--json])
       reet setup profile (save [--exclude-kind KIND] [--exclude-source PATH] [--json] [--dir DIR] [--password-file FILE] NAME SNAPSHOT | list [--json] [--dir DIR] | show [--json] [--dir DIR] [--revision N] NAME | remove [--yes] [--dir DIR] NAME | rename [--dir DIR] OLD NEW | push [--server URL] [--token TOKEN] NAME | pull [--server URL] [--token TOKEN] NAME)
       reet setup prune --keep N [--apply] [--json] [--dir DIR]
       reet setup support [--json]

Discover known agent setup locations and executables locally.
Use --components to read bounded configuration summaries without returning values.
Use --probe-versions to invoke discovered agents with --version (3-second timeout each).
Use --inspect-installation to read local package evidence without running commands.
Use --export-policy to declare component and field selection for future encrypted snapshots.
--preview shows the policy-driven selection without writing an archive.
Without --preview, write a passphrase-encrypted local setup snapshot.
list and inspect read clear headers; inspect opens the payload only with a passphrase.
restore previews a mapped plan; --apply creates or replaces conflicts when chosen, with optional per-path choices.
verify compares the selected fields in the destination without repairing them.
rollback reverses a completed create/replace journal after checking it against the snapshot; --last picks the newest completed one.
journals lists local apply/rollback records without opening snapshots.
health gives conservative local evidence and authentication guidance.
fetch downloads a snapshot from the configured S3 destination.
remote list reads the configured S3 snapshot prefix; remote test checks put/get/delete; remote delete/prune manage remote objects.
profile saves, lists and shows named local snapshot revisions; restore/verify/inspect/rollback accept --profile.
prune removes old local snapshots, protecting profile and journal references.
support prints the conservative per-agent component and restore support matrix.`)
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
	case "verify":
		return runVerify(args[1:], out)
	case "rollback":
		return runRollback(args[1:], out)
	case "journals":
		return runJournals(args[1:], out)
	case "health":
		return runHealth(args[1:], out)
	case "fetch":
		return runFetch(args[1:], out)
	case "remote":
		return runRemote(args[1:], out)
	case "profile":
		return runProfile(args[1:], out, remote...)
	case "prune":
		return runPrune(args[1:], out)
	case "support":
		return runSupport(args[1:], out)
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
	passwordFile := f.String("password-file", "", "read the passphrase from this file, or - for standard input; omit to prompt in a terminal")
	dir := f.String("dir", "", "write snapshots here (default REET_SETUP_DIR or the user config directory)")
	project := f.String("project", "", "also consider known locations in this project (no recursive search)")
	probeVersions := f.Bool("probe-versions", false, "record application --version reports; timeout 3s each, output bounded")
	inspectInstallation := f.Bool("inspect-installation", false, "record bounded local Node package evidence")
	includeCredentials := f.Bool("include-credentials", false, "include literal credential values inside the encrypted snapshot; explicit opt-in")
	uploadS3 := f.Bool("s3", false, "upload the written snapshot to the configured REET_SETUP_S3_* destination")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() == 1 && f.Arg(0) == "-" {
		*passwordFile = "-"
	} else if f.NArg() != 0 {
		return fmt.Errorf("unexpected setup backup arguments; use --password-file - for standard input")
	}
	passphrase := ""
	var err error
	if *includeCredentials && *preview {
		return fmt.Errorf("--include-credentials only applies when writing a snapshot; preview never contains credential values")
	}
	if !*preview {
		passphrase, err = readSetupPassphrase(*passwordFile, true, os.Stderr)
		if err != nil {
			return fmt.Errorf("%w; omit --password-file to be prompted in a terminal", err)
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
	if *inspectInstallation {
		InspectInstallations(&r)
	}
	if *probeVersions {
		if err := ProbeVersions(&r); err != nil {
			return err
		}
	}
	selection, err := BuildSnapshotSelectionWithOptions(r, SnapshotBuildOptions{IncludeCredentials: *includeCredentials})
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
	if *uploadS3 {
		remote, err := uploadSetupSnapshotFile(result.Path, result.ID)
		if err != nil {
			return fmt.Errorf("snapshot written locally at %q but S3 upload failed: %w", result.Path, err)
		}
		result.S3 = &remote
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
	profile := f.String("profile", "", "use this local profile")
	revision := f.Int("revision", 0, "profile revision; default is the newest")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if (*profile == "") == (f.NArg() != 1) {
		return fmt.Errorf("usage: reet setup inspect [--profile NAME [--revision N] | SNAPSHOT] [--json] [--dir DIR] [--password-file FILE]")
	}
	passphrase := ""
	if *passwordFile != "" {
		var err error
		passphrase, err = readSetupPassphrase(*passwordFile, false, os.Stderr)
		if err != nil {
			return err
		}
	}
	snapshotRef := f.Arg(0)
	if *profile != "" {
		var err error
		_, selected, err := LoadProfile(*profile, *dir, *revision)
		if err != nil {
			return err
		}
		snapshotRef = selected.SnapshotID
	}
	inspection, err := InspectSetupSnapshot(snapshotRef, passphrase, *dir)
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
	machine := f.String("machine", "", "apply this machine's profile overrides")
	profile := f.String("profile", "", "use this local profile")
	revision := f.Int("revision", 0, "profile revision; default is the newest")
	probeVersions := f.Bool("probe-versions", false, "compare application --version reports before planning")
	inspectInstallation := f.Bool("inspect-installation", false, "compare bounded local Node package evidence before planning")
	apply := f.Bool("apply", false, "apply the plan; conflicts need --on-conflict")
	resume := f.String("resume", "", "resume this running restore journal; implies --apply")
	onConflict := f.String("on-conflict", "", "for conflicting destinations: skip or replace; default refuses the run")
	conflicts := conflictChoicesFlag{}
	f.Var(conflicts, "conflict", "per-destination conflict policy PATH=skip|replace; repeat for several")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *profile == "" && f.NArg() == 0 && interactive() && !*asJSON {
		return runRestoreWizard(out, *dir, *project, *machine)
	}
	if (*profile == "") == (f.NArg() != 1) {
		return fmt.Errorf("usage: reet setup restore [--profile NAME [--revision N] | SNAPSHOT] [--json] [--dir DIR] [--password-file FILE] [--project PATH] [--probe-versions] [--inspect-installation] [--apply] [--resume JOURNAL] [--on-conflict skip|replace] [--conflict PATH=skip|replace]...")
	}
	if *resume != "" {
		*apply = true
	}
	if !*apply && (*onConflict != "" || len(conflicts) > 0) {
		return fmt.Errorf("--on-conflict and --conflict are only used with --apply")
	}
	passphrase, err := readSetupPassphrase(*passwordFile, false, os.Stderr)
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
	snapshotRef := f.Arg(0)
	var profileRevision *SetupProfileRevision
	if *profile == "" && *revision != 0 {
		return fmt.Errorf("--revision is only used with --profile")
	}
	if *profile != "" {
		_, selected, err := LoadProfile(*profile, *dir, *revision)
		if err != nil {
			return err
		}
		snapshotRef, profileRevision = selected.SnapshotID, &selected
	}
	snapshot, manifest, err := loadSetupSnapshot(snapshotRef, passphrase, *dir)
	if err != nil {
		return err
	}
	destination, err := Discover(home, *project)
	if err != nil {
		return err
	}
	if profileRevision != nil {
		snapshot = ApplyProfileOverrides(snapshot, profileRevision.Overrides, *machine, *project)
	}
	if *inspectInstallation {
		InspectInstallations(&destination)
	}
	if *probeVersions {
		if err := ProbeVersions(&destination); err != nil {
			return err
		}
	}
	plan := BuildRestorePlan(snapshot, manifest, destination)
	if *apply {
		result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: *dir, Conflict: *onConflict, Conflicts: conflicts, ResumeJournal: *resume})
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

func runVerify(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup verify", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON verification")
	dir := f.String("dir", "", "read snapshots here (default REET_SETUP_DIR or the user config directory)")
	passwordFile := f.String("password-file", "", "read the snapshot passphrase from this file, or - for standard input")
	project := f.String("project", "", "check project-scoped sources in this destination project")
	machine := f.String("machine", "", "apply this machine's profile overrides")
	profile := f.String("profile", "", "use this local profile")
	revision := f.Int("revision", 0, "profile revision; default is the newest")
	probeVersions := f.Bool("probe-versions", false, "compare application --version reports; timeout 3s each")
	inspectInstallation := f.Bool("inspect-installation", false, "compare bounded local Node package evidence")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if (*profile == "") == (f.NArg() != 1) {
		return fmt.Errorf("usage: reet setup verify [--profile NAME | SNAPSHOT] [--json] [--dir DIR] [--password-file FILE] [--project PATH] [--probe-versions] [--inspect-installation]")
	}
	passphrase, err := readSetupPassphrase(*passwordFile, false, os.Stderr)
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
	snapshotRef := f.Arg(0)
	var profileRevision *SetupProfileRevision
	if *profile == "" && *revision != 0 {
		return fmt.Errorf("--revision is only used with --profile")
	}
	if *profile != "" {
		_, selected, err := LoadProfile(*profile, *dir, *revision)
		if err != nil {
			return err
		}
		snapshotRef, profileRevision = selected.SnapshotID, &selected
	}
	snapshot, manifest, err := loadSetupSnapshot(snapshotRef, passphrase, *dir)
	if err != nil {
		return err
	}
	if profileRevision != nil {
		snapshot = ApplyProfileOverrides(snapshot, profileRevision.Overrides, *machine, *project)
	}
	destination, err := Discover(home, *project)
	if err != nil {
		return err
	}
	if *inspectInstallation {
		InspectInstallations(&destination)
	}
	if *probeVersions {
		if err := ProbeVersions(&destination); err != nil {
			return err
		}
	}
	result := VerifySetupSnapshot(snapshot, manifest, destination)
	if *asJSON {
		return writeJSON(out, result)
	}
	return renderSetupVerify(out, result)
}

func runRollback(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup rollback", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON rollback result")
	journal := f.String("journal", "", "the completed restore journal to reverse")
	last := f.Bool("last", false, "use the newest completed journal for the snapshot")
	profile := f.String("profile", "", "use this local profile")
	revision := f.Int("revision", 0, "profile revision; default is the newest")
	dir := f.String("dir", "", "read snapshots here (default REET_SETUP_DIR or the user config directory)")
	passwordFile := f.String("password-file", "", "read the snapshot passphrase from this file, or - for standard input")
	project := f.String("project", "", "map project-scoped sources into this destination project")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if (*profile == "") == (f.NArg() != 1) || (*journal == "" && !*last) || (*journal != "" && *last) {
		return fmt.Errorf("usage: reet setup rollback (--journal FILE | --last) [--profile NAME [--revision N] | SNAPSHOT] [--json] [--dir DIR] [--password-file FILE] [--project PATH]")
	}
	passphrase, err := readSetupPassphrase(*passwordFile, false, os.Stderr)
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
	snapshotRef := f.Arg(0)
	if *profile == "" && *revision != 0 {
		return fmt.Errorf("--revision is only used with --profile")
	}
	if *profile != "" {
		_, selected, err := LoadProfile(*profile, *dir, *revision)
		if err != nil {
			return err
		}
		snapshotRef = selected.SnapshotID
	}
	snapshot, manifest, err := loadSetupSnapshot(snapshotRef, passphrase, *dir)
	if err != nil {
		return err
	}
	if *last {
		journalPath, err := latestCompletedJournal(*dir, manifest.ID)
		if err != nil {
			return err
		}
		*journal = journalPath
	}
	destination, err := Discover(home, *project)
	if err != nil {
		return err
	}
	result, err := RollbackRestore(*journal, snapshot, manifest, destination, RestoreRollbackOptions{})
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, result)
	}
	return renderRestoreRollback(out, result)
}

func runJournals(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup journals", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON journal list")
	dir := f.String("dir", "", "read journals here (default REET_SETUP_DIR or the user config directory)")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected setup journals arguments")
	}
	list, err := ListRestoreJournals(*dir)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, list)
	}
	return renderRestoreJournalList(out, list)
}

func runHealth(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup health", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON health report")
	project := f.String("project", "", "also inspect known locations in this project (no recursive search)")
	probeVersions := f.Bool("probe-versions", false, "include bounded application --version reports")
	inspectInstallation := f.Bool("inspect-installation", false, "include bounded local Node package evidence")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected setup health arguments")
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
	if *inspectInstallation {
		InspectInstallations(&r)
	}
	if *probeVersions {
		if err := ProbeVersions(&r); err != nil {
			return err
		}
	}
	report := BuildSetupHealthReport(r)
	if *asJSON {
		return writeJSON(out, report)
	}
	return renderSetupHealth(out, report)
}

func runFetch(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup fetch", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON fetch result")
	dir := f.String("dir", "", "write the downloaded snapshot here (default REET_SETUP_DIR or the user config directory)")
	fromS3 := f.Bool("s3", false, "download from the configured REET_SETUP_S3_* destination")
	profile := f.String("profile", "", "use this local profile")
	revision := f.Int("revision", 0, "profile revision; default is the newest")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if (*profile == "") == (f.NArg() != 1) || !*fromS3 {
		return fmt.Errorf("usage: reet setup fetch --s3 [--profile NAME [--revision N] | SNAPSHOT] [--json] [--dir DIR]")
	}
	snapshotID := f.Arg(0)
	if *profile == "" && *revision != 0 {
		return fmt.Errorf("--revision is only used with --profile")
	}
	if *profile != "" {
		_, selected, err := LoadProfile(*profile, *dir, *revision)
		if err != nil {
			return err
		}
		snapshotID = selected.SnapshotID
	}
	result, err := FetchSetupSnapshot(snapshotID, *dir)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, result)
	}
	return renderSetupFetch(out, result)
}

type setupProfileSaveResult struct {
	Name     string               `json:"name"`
	Revision SetupProfileRevision `json:"revision"`
}

func runProfile(args []string, out io.Writer, remote ...ProfileRemote) error {
	if len(args) == 0 && interactive() {
		return runProfileWizard("", out, remote...)
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, "Usage: reet setup profile (interactive wizard when run without a subcommand)\n       reet setup profile save [--exclude-kind KIND] [--exclude-source PATH] [--machine NAME] [--project PATH] [--json] [--dir DIR] [--password-file FILE] NAME [SNAPSHOT]\n       reet setup profile list [--json] [--dir DIR]\n       reet setup profile show [--json] [--dir DIR] [--revision N] NAME\n       reet setup profile remove [--yes] [--dir DIR] NAME\n       reet setup profile rename [--dir DIR] OLD NEW\n       reet setup profile push [--server URL] [--token TOKEN] [--env-file FILE] NAME\n       reet setup profile pull [--server URL] [--token TOKEN] [--env-file FILE] NAME")
		return err
	}
	switch args[0] {
	case "save":
		f := flag.NewFlagSet("reet setup profile save", flag.ContinueOnError)
		f.SetOutput(out)
		asJSON := f.Bool("json", false, "write a versioned JSON profile revision")
		dir := f.String("dir", "", "save profiles here (default REET_SETUP_DIR or the user config directory)")
		passwordFile := f.String("password-file", "", "optionally open the snapshot for preview/software evidence; otherwise only its clear header is recorded")
		excludeKinds := stringListFlag{}
		excludeSources := stringListFlag{}
		machines := stringListFlag{}
		projects := stringListFlag{}
		f.Var(&excludeKinds, "exclude-kind", "exclude this component kind (settings, mcp, instructions, skills); repeat for several")
		f.Var(&excludeSources, "exclude-source", "exclude this exact snapshot source path; repeat for several")
		f.Var(&machines, "machine", "apply the exclusions only to this machine name; repeat for several")
		f.Var(&projects, "project", "apply the exclusions only to this project path; repeat for several")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() == 0 && interactive() && !*asJSON {
			return runProfileWizard(*dir, out, remote...)
		}
		if f.NArg() < 1 || f.NArg() > 2 {
			return fmt.Errorf("usage: reet setup profile save [--exclude-kind KIND] [--exclude-source PATH] [--machine NAME] [--project PATH] [--json] [--dir DIR] [--password-file FILE] NAME [SNAPSHOT]")
		}
		if strings.HasSuffix(*passwordFile, setupSnapshotExtension) {
			return fmt.Errorf("--password-file is for the passphrase, not the snapshot; pass the snapshot as SNAPSHOT or omit SNAPSHOT to use the newest")
		}
		passphrase := ""
		var err error
		if *passwordFile != "" {
			passphrase, err = readSetupPassphrase(*passwordFile, false, os.Stderr)
			if err != nil {
				return fmt.Errorf("%w; omit --password-file to save a profile that references the snapshot without opening it", err)
			}
		}
		snapshotRef := ""
		if f.NArg() == 2 {
			snapshotRef = f.Arg(1)
			if !validSetupSnapshotID(snapshotRef) && !strings.HasSuffix(snapshotRef, setupSnapshotExtension) && !strings.ContainsAny(snapshotRef, `/\\`) {
				return fmt.Errorf("snapshot %q is not a local ID or path; use reet setup list or omit SNAPSHOT to use the newest", snapshotRef)
			}
		} else {
			snapshots, listErr := ListSetupSnapshots(*dir)
			if listErr != nil {
				return listErr
			}
			if len(snapshots.Snapshots) == 0 {
				return fmt.Errorf("no local setup snapshots found; run reet setup backup first or pass SNAPSHOT")
			}
			snapshotRef = snapshots.Snapshots[0].ID
		}
		profile, revision, err := SaveSetupProfileWithOptions(f.Arg(0), passphrase, snapshotRef, *dir, time.Now().UTC(), SaveSetupProfileOptions{ExcludeKinds: excludeKinds, ExcludeSources: excludeSources, Machines: machines, Projects: projects})
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, setupProfileSaveResult{Name: profile.Name, Revision: revision})
		}
		if _, err := fmt.Fprintf(out, "Saved profile %s revision %d for snapshot %s.\n", profile.Name, revision.Revision, revision.SnapshotID); err != nil {
			return err
		}
		if revision.Preview.SchemaVersion == 0 {
			if _, err := fmt.Fprintln(out, "Header only; pass --password-file to record preview and software evidence."); err != nil {
				return err
			}
		}
		if !*asJSON {
			if err := offerProfileSync(profile, out, remote...); err != nil {
				return err
			}
		}
		return nil
	case "list":
		f := flag.NewFlagSet("reet setup profile list", flag.ContinueOnError)
		f.SetOutput(out)
		asJSON := f.Bool("json", false, "write a versioned JSON profile list")
		dir := f.String("dir", "", "read profiles here (default REET_SETUP_DIR or the user config directory)")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected setup profile list arguments")
		}
		list, err := ListSetupProfiles(*dir)
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, list)
		}
		return renderProfileList(out, list)
	case "show":
		f := flag.NewFlagSet("reet setup profile show", flag.ContinueOnError)
		f.SetOutput(out)
		asJSON := f.Bool("json", false, "write a versioned JSON profile revision")
		dir := f.String("dir", "", "read profiles here (default REET_SETUP_DIR or the user config directory)")
		revision := f.Int("revision", 0, "show this revision; default is the newest")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 1 {
			return fmt.Errorf("usage: reet setup profile show [--json] [--dir DIR] [--revision N] NAME")
		}
		profile, selected, err := LoadProfile(f.Arg(0), *dir, *revision)
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, setupProfileSaveResult{Name: profile.Name, Revision: selected})
		}
		return renderProfile(out, profile, selected)
	case "push":
		f := flag.NewFlagSet("reet setup profile push", flag.ContinueOnError)
		f.SetOutput(out)
		server := f.String("server", os.Getenv("REET_URL"), "reet server URL (env REET_URL)")
		token := f.String("token", "", "personal token (env REET_TOKEN)")
		envFile := f.String("env-file", profileEnvFile(""), "read REET_URL/REET_TOKEN from this env file (default: .env or REET_ENV_FILE)")
		dir := f.String("dir", "", "read profiles here (default REET_SETUP_DIR or the user config directory)")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 1 || len(remote) == 0 {
			return fmt.Errorf("profile push needs a server remote and exactly one NAME")
		}
		profile, revision, err := LoadProfile(f.Arg(0), *dir, 0)
		if err != nil {
			return err
		}
		if revision.Preview.SchemaVersion == 0 && interactive() {
			record, err := wizardYesNo(out, "This profile has no preview. Open its snapshot now to record it?", true)
			if err != nil {
				return err
			}
			if record {
				passphrase, err := readSetupPassphrase("", false, out)
				if err != nil {
					return err
				}
				profile, revision, err = SaveSetupProfileWithOptions(profile.Name, passphrase, revision.SnapshotID, *dir, time.Now().UTC(), SaveSetupProfileOptions{Overrides: revision.Overrides})
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "Recorded preview for profile %s revision %d.\n", profile.Name, revision.Revision)
			}
		}
		if *server == "" && *envFile != "" {
			*server = envFileValue(*envFile, "REET_URL")
		}
		if *server == "" && interactive() {
			prompted, err := wizardPrompt(out, "Server URL", "")
			if err != nil {
				return err
			}
			*server = prompted
		}
		resolved, err := clientconfig.ResolveToken(*token, *envFile, os.Stdin, os.Stderr)
		if err != nil {
			return err
		}
		if err := pushProfile(remote[0], *server, resolved, profile, out); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Pushed profile %s.\n", profile.Name)
		return err
	case "pull":
		f := flag.NewFlagSet("reet setup profile pull", flag.ContinueOnError)
		f.SetOutput(out)
		server := f.String("server", os.Getenv("REET_URL"), "reet server URL (env REET_URL)")
		token := f.String("token", "", "personal token (env REET_TOKEN)")
		envFile := f.String("env-file", profileEnvFile(""), "read REET_URL/REET_TOKEN from this env file (default: .env or REET_ENV_FILE)")
		dir := f.String("dir", "", "write profiles here (default REET_SETUP_DIR or the user config directory)")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 1 || len(remote) == 0 {
			return fmt.Errorf("profile pull needs a server remote and exactly one NAME")
		}
		if *server == "" && *envFile != "" {
			*server = envFileValue(*envFile, "REET_URL")
		}
		if *server == "" && interactive() {
			prompted, err := wizardPrompt(out, "Server URL", "")
			if err != nil {
				return err
			}
			*server = prompted
		}
		resolved, err := clientconfig.ResolveToken(*token, *envFile, os.Stdin, os.Stderr)
		if err != nil {
			return err
		}
		profile, err := remote[0].Pull(*server, resolved, f.Arg(0))
		if err != nil {
			return err
		}
		if err := WriteSetupProfile(profile, *dir); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Pulled profile %s revision %d.\n", profile.Name, profile.Revisions[len(profile.Revisions)-1].Revision)
		return err
	case "remove":
		f := flag.NewFlagSet("reet setup profile remove", flag.ContinueOnError)
		f.SetOutput(out)
		yes := f.Bool("yes", false, "confirm removing the profile file")
		dir := f.String("dir", "", "read profiles here (default REET_SETUP_DIR or the user config directory)")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 1 || !*yes {
			return fmt.Errorf("usage: reet setup profile remove --yes [--dir DIR] NAME")
		}
		if err := RemoveSetupProfile(f.Arg(0), *dir); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Removed profile %s. Snapshots are kept.\n", f.Arg(0))
		return err
	case "rename":
		f := flag.NewFlagSet("reet setup profile rename", flag.ContinueOnError)
		f.SetOutput(out)
		dir := f.String("dir", "", "read profiles here (default REET_SETUP_DIR or the user config directory)")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 2 {
			return fmt.Errorf("usage: reet setup profile rename [--dir DIR] OLD NEW")
		}
		if err := RenameSetupProfile(f.Arg(0), f.Arg(1), *dir); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Renamed profile %s to %s.\n", f.Arg(0), f.Arg(1))
		return err
	default:
		return fmt.Errorf("unknown setup profile command %q; use reet setup profile --help", args[0])
	}
}

func runPrune(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup prune", flag.ContinueOnError)
	f.SetOutput(out)
	keep := f.Int("keep", 0, "keep this many newest local snapshots")
	apply := f.Bool("apply", false, "delete the planned snapshots; default is a dry run")
	asJSON := f.Bool("json", false, "write a versioned JSON prune result")
	dir := f.String("dir", "", "prune snapshots here (default REET_SETUP_DIR or the user config directory)")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 || *keep < 1 {
		return fmt.Errorf("usage: reet setup prune --keep N [--apply] [--json] [--dir DIR]")
	}
	result, err := PruneSetupSnapshots(*dir, *keep, *apply)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, result)
	}
	return renderSetupPrune(out, result)
}

func runRemote(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, "Usage: reet setup remote list [--json]\n       reet setup remote test [--json]\n       reet setup remote delete --yes [--json] SNAPSHOT\n       reet setup remote prune --keep N [--apply] [--json]")
		return err
	}
	switch args[0] {
	case "list":
		f := flag.NewFlagSet("reet setup remote list", flag.ContinueOnError)
		f.SetOutput(out)
		asJSON := f.Bool("json", false, "write a versioned JSON remote list")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected setup remote list arguments")
		}
		list, err := ListRemoteSetupSnapshots()
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, list)
		}
		return renderRemoteSetupList(out, list)
	case "test":
		f := flag.NewFlagSet("reet setup remote test", flag.ContinueOnError)
		f.SetOutput(out)
		asJSON := f.Bool("json", false, "write a versioned JSON connection test")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected setup remote test arguments")
		}
		steps, err := TestSetupS3Destination()
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, map[string]any{"schema_version": 1, "steps": steps})
		}
		return renderSetupS3Test(out, steps)
	case "delete":
		f := flag.NewFlagSet("reet setup remote delete", flag.ContinueOnError)
		f.SetOutput(out)
		asJSON := f.Bool("json", false, "write a versioned JSON delete result")
		yes := f.Bool("yes", false, "confirm remote deletion")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 1 || !*yes {
			return fmt.Errorf("usage: reet setup remote delete --yes [--json] SNAPSHOT")
		}
		result, err := DeleteRemoteSetupSnapshot(f.Arg(0))
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, result)
		}
		return renderRemoteDelete(out, result)
	case "prune":
		f := flag.NewFlagSet("reet setup remote prune", flag.ContinueOnError)
		f.SetOutput(out)
		asJSON := f.Bool("json", false, "write a versioned JSON prune result")
		keep := f.Int("keep", 0, "keep this many newest remote snapshots")
		apply := f.Bool("apply", false, "delete the planned remote snapshots; default is a dry run")
		if err := f.Parse(reorderFlags(f, args[1:])); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 0 || *keep < 1 {
			return fmt.Errorf("usage: reet setup remote prune --keep N [--apply] [--json]")
		}
		result, err := PruneRemoteSetupSnapshots(*keep, *apply)
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, result)
		}
		return renderRemotePrune(out, result)
	default:
		return fmt.Errorf("unknown setup remote command %q; use reet setup remote --help", args[0])
	}
}

func readSetupPassphrase(file string, confirm bool, promptOut io.Writer) (string, error) {
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	if file == "" && !interactive {
		return "", errors.New("a setup snapshot passphrase is required; give --password-file FILE or --password-file - for standard input")
	}
	if file == "" || file == "-" && interactive {
		passphrase, err := promptSetupPassphrase(confirm, promptOut)
		if err != nil {
			return "", err
		}
		return passphrase, nil
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

func promptSetupPassphrase(confirm bool, out io.Writer) (string, error) {
	fmt.Fprint(out, "Setup passphrase (at least 12 characters): ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(out)
	if err != nil {
		return "", errors.New("could not read the setup passphrase from the terminal")
	}
	passphrase := string(raw)
	for i := range raw {
		raw[i] = 0
	}
	if err := validateSetupPassphrase(passphrase); err != nil {
		return "", err
	}
	if confirm {
		fmt.Fprint(out, "Confirm setup passphrase: ")
		again, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", errors.New("could not read the setup passphrase confirmation")
		}
		confirmed := string(again)
		for i := range again {
			again[i] = 0
		}
		if passphrase != confirmed {
			return "", errors.New("the setup passphrases do not match")
		}
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

func runSupport(args []string, out io.Writer) error {
	f := flag.NewFlagSet("reet setup support", flag.ContinueOnError)
	f.SetOutput(out)
	asJSON := f.Bool("json", false, "write a versioned JSON support matrix")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected setup support arguments")
	}
	matrix := BuildSupportMatrix()
	if *asJSON {
		return writeJSON(out, matrix)
	}
	if _, err := fmt.Fprintf(out, "AI agent setup support (%s/%s)\n", matrix.OS, matrix.Architecture); err != nil {
		return err
	}
	for _, row := range matrix.Agents {
		if _, err := fmt.Fprintf(out, "\n%s (%s)\n  settings=%s mcp=%s instructions=%s skills=%s\n  memory=%s history=%s credentials=%s restore=%s\n  %s\n", row.Name, row.AgentID, row.Settings, row.MCP, row.Instructions, row.Skills, row.Memory, row.History, row.Credentials, row.Restore, row.Notes); err != nil {
			return err
		}
	}
	for _, note := range matrix.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
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
	if _, err := fmt.Fprintf(out, "AI agent setup inventory (%s/%s)\n%s; no values are read by this inventory.\n", r.OS, r.Architecture, mode); err != nil {
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
	if _, err := fmt.Fprintf(out, "Selected %d components, %d skill folders (%d files) and %d memory databases; %d omissions recorded; %d agents with software evidence.\n", result.Components, result.Skills, result.SkillFiles, result.Memories, result.Omissions, len(result.Software)); err != nil {
		return err
	}
	if result.CredentialsIncluded {
		if _, err := fmt.Fprintln(out, "Literal credential values were included inside the encrypted payload."); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "It is passphrase-encrypted on this machine. Preview a restore with reet setup restore; no runtime verification has been run.")
	return err
}

type conflictChoicesFlag map[string]string

func (c conflictChoicesFlag) String() string { return "" }

func (c conflictChoicesFlag) Set(value string) error {
	path, policy, ok := strings.Cut(value, "=")
	path = strings.TrimSpace(path)
	policy = strings.ToLower(strings.TrimSpace(policy))
	if !ok || path == "" {
		return errors.New("conflict choice must be PATH=skip|replace")
	}
	if policy != "skip" && policy != "replace" {
		return fmt.Errorf("conflict choice for %q must be skip or replace", path)
	}
	if _, exists := c[path]; exists {
		return fmt.Errorf("duplicate conflict choice for %q", path)
	}
	c[path] = policy
	return nil
}

type stringListFlag []string

func (s stringListFlag) String() string { return "" }

func (s *stringListFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("value cannot be empty")
	}
	*s = append(*s, value)
	return nil
}

func reorderFlags(f *flag.FlagSet, args []string) []string {
	var options, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if before, _, ok := strings.Cut(name, "="); ok {
			name = before
		}
		option := f.Lookup(name)
		if option == nil {
			options = append(options, arg)
			continue
		}
		options = append(options, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		if boolean, ok := option.Value.(interface{ IsBoolFlag() bool }); ok && boolean.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			options = append(options, args[i+1])
			i++
		}
	}
	return append(options, positional...)
}
