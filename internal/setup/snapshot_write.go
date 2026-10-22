package setup

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
)

const (
	setupSnapshotSchemaVersion = 1
	setupSnapshotKind          = "setup"
	setupSnapshotVersion       = "setup-v1"
	setupSnapshotExtension     = ".reetsetup"

	// EnvSetupDir overrides the default local setup snapshot directory.
	EnvSetupDir = "REET_SETUP_DIR"
)

// SetupSnapshot is the encrypted payload of a local setup snapshot. Source
// paths and selected content stay inside the sealed payload.
type SetupSnapshot struct {
	SchemaVersion int                   `json:"schema_version"`
	CreatedAt     time.Time             `json:"created_at"`
	Preview       SnapshotPreview       `json:"preview"`
	Sources       []SetupSnapshotSource `json:"sources"`
	Software      []SetupSoftware       `json:"software,omitempty"`
}

// SetupSoftware records application evidence without executable or metadata paths.
type SetupSoftware struct {
	AgentID        string `json:"agent_id"`
	Name           string `json:"name"`
	Version        string `json:"application_version,omitempty"`
	VersionState   string `json:"application_version_state,omitempty"`
	InstallState   string `json:"installation_state,omitempty"`
	Package        string `json:"installation_package,omitempty"`
	InstallVersion string `json:"installation_version,omitempty"`
	Manager        string `json:"installation_manager,omitempty"`
}

type SetupSnapshotSource struct {
	Path      string         `json:"path"`
	Component string         `json:"component"`
	Content   map[string]any `json:"content,omitempty"`
	Text      string         `json:"text,omitempty"`
	Digest    string         `json:"digest,omitempty"`
}

type SetupWriteOptions struct {
	Dir      string
	Password string
	Now      func() time.Time
}

type SetupWriteResult struct {
	ID                  string          `json:"id"`
	Path                string          `json:"path"`
	Bytes               int64           `json:"bytes"`
	TakenAt             time.Time       `json:"taken_at"`
	SHA256              string          `json:"sha256"`
	Protection          string          `json:"protection"`
	Compression         string          `json:"compression"`
	Components          int             `json:"components"`
	Omissions           int             `json:"omissions"`
	CredentialsIncluded bool            `json:"credentials_included,omitempty"`
	Preview             SnapshotPreview `json:"preview"`
	Software            []SetupSoftware `json:"software,omitempty"`
	S3                  *SetupS3Result  `json:"s3,omitempty"`
}

func validateSetupPassphrase(passphrase string) error {
	if len(passphrase) < sealed.MinPassword {
		return fmt.Errorf("the setup passphrase needs at least %d characters", sealed.MinPassword)
	}
	return nil
}

// SnapshotDir returns the local directory for setup snapshots.
func SnapshotDir() (string, error) {
	if value := strings.TrimSpace(os.Getenv(EnvSetupDir)); value != "" {
		return filepath.Abs(value)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "reet", "setup"), nil
}

// BuildSetupSnapshot turns a policy-driven selection into the payload that is
// sealed. Selected values must never be printed or written outside the payload.
func BuildSetupSnapshot(selection SnapshotSelection) SetupSnapshot {
	snapshot := SetupSnapshot{
		SchemaVersion: setupSnapshotSchemaVersion,
		CreatedAt:     selection.Preview.CapturedAt,
		Preview:       selection.Preview,
		Sources:       []SetupSnapshotSource{},
		Software:      selection.software,
	}
	for _, source := range selection.sources {
		digest := contentDigest(source.Content)
		if source.Component == "instructions" {
			digest = textDigest(source.Text)
		}
		snapshot.Sources = append(snapshot.Sources, SetupSnapshotSource{
			Path: source.Path, Component: source.Component, Content: source.Content, Text: source.Text, Digest: digest,
		})
	}
	if snapshot.Preview.CredentialsIncluded {
		redactPreviewDigests(&snapshot.Preview)
	}
	return snapshot
}

// WriteSetupSnapshot seals selected content with the user's passphrase, verifies
// it before and after the private atomic write, and returns no secret values.
func WriteSetupSnapshot(selection SnapshotSelection, opts SetupWriteOptions) (SetupWriteResult, error) {
	if err := validateSetupPassphrase(opts.Password); err != nil {
		return SetupWriteResult{}, err
	}
	snapshot := BuildSetupSnapshot(selection)
	if err := verifySetupSnapshot(snapshot); err != nil {
		return SetupWriteResult{}, fmt.Errorf("checking the selected content: %w", err)
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return SetupWriteResult{}, fmt.Errorf("encoding the setup snapshot: %w", err)
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	id, err := newSetupSnapshotID(now)
	if err != nil {
		return SetupWriteResult{}, err
	}
	dir := opts.Dir
	if dir == "" {
		dir, err = SnapshotDir()
		if err != nil {
			return SetupWriteResult{}, err
		}
	}
	path := filepath.Join(dir, id+setupSnapshotExtension)
	if err := prepareSnapshotDir(dir); err != nil {
		return SetupWriteResult{}, err
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		if err == nil {
			return SetupWriteResult{}, errors.New("a setup snapshot with that identifier already exists")
		}
		return SetupWriteResult{}, err
	}

	manifest := sealed.Manifest{
		ID: id, Kind: setupSnapshotKind, Target: setupSnapshotKind,
		TakenAt: now.UnixMilli(), Version: setupSnapshotVersion,
	}
	sealer, err := sealed.NewSealer(strings.Repeat("0", 32), "setup")
	if err != nil {
		return SetupWriteResult{}, err
	}
	sealedData, err := sealer.SealArchive(manifest, payload, sealed.SealOptions{Compress: true, Password: opts.Password})
	if err != nil {
		return SetupWriteResult{}, fmt.Errorf("sealing the setup snapshot: %w", err)
	}
	_, written, err := openSetupSnapshot(sealedData, opts.Password)
	if err != nil {
		return SetupWriteResult{}, fmt.Errorf("checking the sealed snapshot: %w", err)
	}
	if err := writeSnapshotFile(path, sealedData, 0o600); err != nil {
		return SetupWriteResult{}, err
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		_ = os.Remove(path)
		return SetupWriteResult{}, fmt.Errorf("reading back the setup snapshot: %w", err)
	}
	if _, _, err := openSetupSnapshot(onDisk, opts.Password); err != nil {
		_ = os.Remove(path)
		return SetupWriteResult{}, fmt.Errorf("checking the written setup snapshot: %w", err)
	}
	return SetupWriteResult{
		ID: id, Path: path, Bytes: int64(len(onDisk)),
		TakenAt: time.UnixMilli(written.TakenAt).UTC(),
		SHA256:  written.SHA256, Protection: written.Protection, Compression: written.Compression,
		Components: len(snapshot.Sources), Omissions: omissionCount(snapshot.Preview),
		CredentialsIncluded: snapshot.Preview.CredentialsIncluded,
		Preview:             snapshot.Preview, Software: snapshot.Software,
	}, nil
}

// OpenSetupSnapshot opens and verifies a sealed setup snapshot payload.
func OpenSetupSnapshot(data []byte, password string) (SetupSnapshot, error) {
	snapshot, _, err := openSetupSnapshot(data, password)
	return snapshot, err
}

func openSetupSnapshot(data []byte, password string) (SetupSnapshot, sealed.Manifest, error) {
	manifest, payload, err := sealed.OpenArchive(data, password, "")
	if err != nil {
		return SetupSnapshot{}, sealed.Manifest{}, err
	}
	if manifest.Kind != setupSnapshotKind {
		return SetupSnapshot{}, sealed.Manifest{}, errors.New("this is not a reet setup snapshot")
	}
	var snapshot SetupSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return SetupSnapshot{}, sealed.Manifest{}, errors.New("the setup snapshot payload is invalid")
	}
	if err := verifySetupSnapshot(snapshot); err != nil {
		return SetupSnapshot{}, sealed.Manifest{}, err
	}
	return snapshot, manifest, nil
}

func verifySetupSnapshot(snapshot SetupSnapshot) error {
	if snapshot.SchemaVersion != setupSnapshotSchemaVersion {
		return fmt.Errorf("unsupported setup snapshot schema version %d", snapshot.SchemaVersion)
	}
	components := map[string]SnapshotComponent{}
	for _, agent := range snapshot.Preview.Agents {
		for _, component := range agent.Components {
			components[component.Source+"\x00"+component.Kind] = component
		}
	}
	seen := map[string]bool{}
	for _, source := range snapshot.Sources {
		key := source.Path + "\x00" + source.Component
		if seen[key] {
			return errors.New("duplicate setup snapshot source")
		}
		seen[key] = true
		component, ok := components[key]
		if !ok || component.Selection != "include" {
			return errors.New("setup snapshot content is not in the selected preview")
		}
		computed := contentDigest(source.Content)
		if source.Component == "instructions" {
			computed = textDigest(source.Text)
		}
		recorded := source.Digest
		if recorded == "" {
			recorded = component.Digest
		}
		if recorded == "" || computed != recorded {
			return errors.New("setup snapshot content does not match its recorded digest")
		}
	}
	return nil
}

func omissionCount(preview SnapshotPreview) int {
	total := 0
	for _, agent := range preview.Agents {
		total += len(agent.Omitted)
	}
	return total
}

func newSetupSnapshotID(now time.Time) (string, error) {
	nonce := make([]byte, 4)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate setup snapshot identifier: %w", err)
	}
	return "setup-" + now.Format("20060102T150405Z") + "-" + hex.EncodeToString(nonce), nil
}

func prepareSnapshotDir(dir string) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return fmt.Errorf("inspect setup snapshot directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("the setup snapshot directory must be a real directory, not a symlink or file")
	}
	return nil
}

func writeSnapshotFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".reet-setup-*")
	if err != nil {
		return fmt.Errorf("create setup snapshot: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("write setup snapshot: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("write setup snapshot: %w", closeErr)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("finish setup snapshot: %w", err)
	}
	return nil
}

func snapshotSoftware(agents []Agent) []SetupSoftware {
	var out []SetupSoftware
	for _, agent := range agents {
		software := SetupSoftware{AgentID: agent.ID, Name: agent.Name}
		if agent.ApplicationVersion != nil {
			software.Version, software.VersionState = agent.ApplicationVersion.Version, agent.ApplicationVersion.State
		}
		if agent.Installation != nil {
			software.InstallState, software.Package = agent.Installation.State, agent.Installation.Package
			software.InstallVersion, software.Manager = agent.Installation.Version, agent.Installation.Manager
		}
		if software.Version != "" || software.InstallState != "" {
			out = append(out, software)
		}
	}
	return out
}

func redactPreviewDigests(preview *SnapshotPreview) {
	for i := range preview.Agents {
		for j := range preview.Agents[i].Components {
			preview.Agents[i].Components[j].Digest = ""
		}
	}
}
