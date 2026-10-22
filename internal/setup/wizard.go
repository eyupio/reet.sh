package setup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"golang.org/x/term"
)

var ErrProfileUnauthorized = errors.New("profile server rejected the token")

func interactive() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

func wizardPrompt(out io.Writer, label, fallback string) (string, error) {
	if fallback != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, fallback)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	var b strings.Builder
	one := make([]byte, 1)
	for b.Len() <= 4096 {
		n, err := os.Stdin.Read(one)
		if n > 0 {
			if one[0] == '\n' {
				break
			}
			b.WriteByte(one[0])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", errors.New("input ended")
			}
			return "", err
		}
	}
	value := strings.TrimSpace(b.String())
	if value == "" {
		value = fallback
	}
	return value, nil
}

func wizardYesNo(out io.Writer, label string, fallback bool) (bool, error) {
	def := "n"
	if fallback {
		def = "y"
	}
	answer, err := wizardPrompt(out, label+" (y/n)", def)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, errors.New("answer yes or no")
	}
}

func runProfileWizard(dir string, out io.Writer, remote ...ProfileRemote) error {
	snapshots, err := ListSetupSnapshots(dir)
	if err != nil {
		return err
	}
	snapshotID := ""
	passphrase := ""
	if len(snapshots.Snapshots) == 0 {
		create, err := wizardYesNo(out, "No setup snapshot found. Create one now?", true)
		if err != nil {
			return err
		}
		if !create {
			return errors.New("no snapshot to save; run reet setup backup when ready")
		}
		passphrase, err = promptSetupPassphrase(true, out)
		if err != nil {
			return err
		}
		home, err := setupHome()
		if err != nil {
			return err
		}
		report, err := Discover(home, "")
		if err != nil {
			return err
		}
		selection, err := BuildSnapshotSelection(report)
		if err != nil {
			return err
		}
		result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: passphrase})
		if err != nil {
			return err
		}
		snapshotID = result.ID
		if _, err := fmt.Fprintf(out, "Took setup snapshot %s.\n", result.ID); err != nil {
			return err
		}
	} else {
		chosen := snapshots.Snapshots[0]
		if len(snapshots.Snapshots) > 1 {
			if _, err := fmt.Fprintln(out, "Choose a snapshot:"); err != nil {
				return err
			}
			for i, snapshot := range snapshots.Snapshots {
				if _, err := fmt.Fprintf(out, "  %d. %s (%s)\n", i+1, snapshot.ID, snapshot.TakenAt.Format(time.RFC3339)); err != nil {
					return err
				}
			}
			answer, err := wizardPrompt(out, "Snapshot number", "1")
			if err != nil {
				return err
			}
			index, err := strconv.Atoi(answer)
			if err != nil || index < 1 || index > len(snapshots.Snapshots) {
				return errors.New("choose a number from the list")
			}
			chosen = snapshots.Snapshots[index-1]
		}
		snapshotID = chosen.ID
		preview, err := wizardYesNo(out, "Open the snapshot to record preview and software evidence?", false)
		if err != nil {
			return err
		}
		if preview {
			passphrase, err = readSetupPassphrase("", false, out)
			if err != nil {
				return err
			}
		}
	}
	name, err := wizardPrompt(out, "Profile name", "work")
	if err != nil {
		return err
	}
	if !validProfileName(name) {
		return errors.New("profile names use 1-64 letters, digits, '-' or '_'")
	}
	profile, revision, err := SaveSetupProfileWithOptions(name, passphrase, snapshotID, dir, time.Now().UTC(), SaveSetupProfileOptions{})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Saved profile %s revision %d for snapshot %s.\n", profile.Name, revision.Revision, revision.SnapshotID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Preview: %s. Next: reet setup restore --profile %s.\n", map[bool]string{true: "recorded", false: "header only"}[revision.Preview.SchemaVersion != 0], profile.Name); err != nil {
		return err
	}
	return offerProfileSync(profile, out, remote...)
}

func offerProfileSync(profile SetupProfile, out io.Writer, remote ...ProfileRemote) error {
	if len(remote) == 0 || !interactive() {
		return nil
	}
	syncNow, err := wizardYesNo(out, "Push this profile to your reet server now?", false)
	if err != nil || !syncNow {
		return err
	}
	envFile := profileEnvFile("")
	server := os.Getenv("REET_URL")
	if server == "" && envFile != "" {
		server = envFileValue(envFile, "REET_URL")
	}
	if server == "" {
		server, err = wizardPrompt(out, "Server URL", "")
		if err != nil {
			return err
		}
	}
	token, err := clientconfig.ResolveToken("", envFile, os.Stdin, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Pushing profile %s to %s...\n", profile.Name, server)
	if err := pushProfile(remote[0], server, token, profile, out); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Pushed profile %s to %s.\n", profile.Name, server)
	return err
}

func pushProfile(remote ProfileRemote, server, token string, profile SetupProfile, out io.Writer) error {
	err := remote.Push(server, token, profile)
	if err == nil || !errors.Is(err, ErrProfileUnauthorized) || !interactive() {
		return err
	}
	fmt.Fprintf(out, "The server rejected the token. Enter a personal API token from Account on %s (hidden, Enter to cancel): ", server)
	secret, readErr := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(out)
	if readErr != nil {
		return errors.New("could not read the personal API token")
	}
	replacement := string(secret)
	for i := range secret {
		secret[i] = 0
	}
	if replacement == "" {
		return err
	}
	if _, saveErr := clientconfig.SaveToken(replacement); saveErr != nil {
		return saveErr
	}
	return remote.Push(server, replacement, profile)
}

// profileEnvFile selects an explicit env file, REET_ENV_FILE, or a .env in
// the current directory. Only REET_URL and REET_TOKEN are read.
func profileEnvFile(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if value := strings.TrimSpace(os.Getenv("REET_ENV_FILE")); value != "" {
		return value
	}
	if info, err := os.Stat(".env"); err == nil && info.Mode().IsRegular() {
		return ".env"
	}
	return ""
}

func envFileValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 1<<20 {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '\'' && value[len(value)-1] == '\'' || value[0] == '"' && value[len(value)-1] == '"') {
			value = value[1 : len(value)-1]
		}
		return value
	}
	return ""
}
