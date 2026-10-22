package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	setupProfileSchemaVersion = 1
	maxProfileBytes           = 4 << 20
)

type SetupProfileRevision struct {
	Revision       int             `json:"revision"`
	CreatedAt      time.Time       `json:"created_at"`
	SnapshotID     string          `json:"snapshot_id"`
	SnapshotDigest string          `json:"snapshot_digest"`
	TakenAt        time.Time       `json:"snapshot_taken_at"`
	Preview        SnapshotPreview `json:"preview"`
	Software       []SetupSoftware `json:"software,omitempty"`
}

type SetupProfile struct {
	SchemaVersion int                    `json:"schema_version"`
	Name          string                 `json:"name"`
	Revisions     []SetupProfileRevision `json:"revisions"`
}

type SetupProfileSummary struct {
	Name           string    `json:"name"`
	Revision       int       `json:"revision"`
	SnapshotID     string    `json:"snapshot_id"`
	SnapshotDigest string    `json:"snapshot_digest"`
	UpdatedAt      time.Time `json:"updated_at"`
	Revisions      int       `json:"revisions"`
}

type SetupProfileList struct {
	SchemaVersion int                   `json:"schema_version"`
	Dir           string                `json:"dir"`
	Profiles      []SetupProfileSummary `json:"profiles"`
	Limitations   []string              `json:"limitations"`
}

func profileDir(dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = SnapshotDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "profiles"), nil
}

func validProfileName(name string) bool {
	if name == "" || len(name) > 64 || name[0] == '-' {
		return false
	}
	for _, c := range name {
		if !(c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func SaveSetupProfile(name, password, snapshotPath, dir string, now time.Time) (SetupProfile, SetupProfileRevision, error) {
	if !validProfileName(name) {
		return SetupProfile{}, SetupProfileRevision{}, errors.New("profile names use 1-64 letters, digits, '-' or '_'")
	}
	snapshot, manifest, err := loadSetupSnapshot(snapshotPath, password, dir)
	if err != nil {
		return SetupProfile{}, SetupProfileRevision{}, err
	}
	profile, err := loadSetupProfile(name, dir)
	if err != nil && !os.IsNotExist(err) {
		return SetupProfile{}, SetupProfileRevision{}, err
	}
	if profile.SchemaVersion == 0 {
		profile = SetupProfile{SchemaVersion: setupProfileSchemaVersion, Name: name, Revisions: []SetupProfileRevision{}}
	}
	revision := SetupProfileRevision{
		Revision: len(profile.Revisions) + 1, CreatedAt: now.UTC(),
		SnapshotID: manifest.ID, SnapshotDigest: manifest.SHA256,
		TakenAt: time.UnixMilli(manifest.TakenAt).UTC(),
		Preview: snapshot.Preview, Software: snapshot.Software,
	}
	profile.Revisions = append(profile.Revisions, revision)
	path, err := profilePath(name, dir)
	if err != nil {
		return SetupProfile{}, SetupProfileRevision{}, err
	}
	if err := prepareSnapshotDir(filepath.Dir(path)); err != nil {
		return SetupProfile{}, SetupProfileRevision{}, err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return SetupProfile{}, SetupProfileRevision{}, fmt.Errorf("encode profile: %w", err)
	}
	if err := writeSnapshotFile(path, append(data, '\n'), 0o600); err != nil {
		return SetupProfile{}, SetupProfileRevision{}, err
	}
	return profile, revision, nil
}

func loadSetupProfile(name, dir string) (SetupProfile, error) {
	if !validProfileName(name) {
		return SetupProfile{}, errors.New("profile names use 1-64 letters, digits, '-' or '_'")
	}
	path, err := profilePath(name, dir)
	if err != nil {
		return SetupProfile{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return SetupProfile{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return SetupProfile{}, errors.New("the profile must be a regular file, not a symlink")
	}
	if info.Size() > maxProfileBytes {
		return SetupProfile{}, errors.New("the profile is too large")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SetupProfile{}, fmt.Errorf("read profile: %w", err)
	}
	var profile SetupProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return SetupProfile{}, errors.New("the profile is invalid")
	}
	if profile.SchemaVersion != setupProfileSchemaVersion || profile.Name != name || len(profile.Revisions) == 0 {
		return SetupProfile{}, errors.New("the profile is incomplete or from another version")
	}
	for i, revision := range profile.Revisions {
		if revision.Revision != i+1 || revision.SnapshotID == "" || revision.SnapshotDigest == "" {
			return SetupProfile{}, errors.New("the profile revision history is inconsistent")
		}
	}
	return profile, nil
}

func profilePath(name, dir string) (string, error) {
	dir, err := profileDir(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

func LoadProfile(name, dir string, revision int) (SetupProfile, SetupProfileRevision, error) {
	profile, err := loadSetupProfile(name, dir)
	if err != nil {
		return SetupProfile{}, SetupProfileRevision{}, err
	}
	if revision == 0 {
		revision = len(profile.Revisions)
	}
	if revision < 1 || revision > len(profile.Revisions) {
		return SetupProfile{}, SetupProfileRevision{}, fmt.Errorf("profile revision %d does not exist", revision)
	}
	return profile, profile.Revisions[revision-1], nil
}

func ResolveProfileSnapshot(name, dir string) (string, error) {
	_, revision, err := LoadProfile(name, dir, 0)
	if err != nil {
		return "", err
	}
	return revision.SnapshotID, nil
}

func ListSetupProfiles(dir string) (SetupProfileList, error) {
	profilesDir, err := profileDir(dir)
	if err != nil {
		return SetupProfileList{}, err
	}
	list := SetupProfileList{
		SchemaVersion: setupProfileSchemaVersion, Dir: profilesDir,
		Profiles: []SetupProfileSummary{},
		Limitations: []string{
			"Profiles are local named snapshot revisions; they are not synced to an account or another machine.",
			"A profile records a snapshot revision; taking a new backup does not change it.",
		},
	}
	info, err := os.Lstat(profilesDir)
	if os.IsNotExist(err) {
		return list, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return SetupProfileList{}, errors.New("the profiles directory is not a real directory")
	}
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return SetupProfileList{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		if !validProfileName(name) {
			continue
		}
		profile, err := loadSetupProfile(name, dir)
		if err != nil {
			continue
		}
		latest := profile.Revisions[len(profile.Revisions)-1]
		list.Profiles = append(list.Profiles, SetupProfileSummary{
			Name: name, Revision: latest.Revision, SnapshotID: latest.SnapshotID,
			SnapshotDigest: latest.SnapshotDigest, UpdatedAt: latest.CreatedAt,
			Revisions: len(profile.Revisions),
		})
	}
	sort.Slice(list.Profiles, func(i, j int) bool { return list.Profiles[i].Name < list.Profiles[j].Name })
	return list, nil
}

func renderProfileList(out io.Writer, list SetupProfileList) error {
	if _, err := fmt.Fprintf(out, "Setup profiles in %q\n", list.Dir); err != nil {
		return err
	}
	if len(list.Profiles) == 0 {
		if _, err := fmt.Fprintln(out, "No setup profiles found."); err != nil {
			return err
		}
	}
	for _, profile := range list.Profiles {
		if _, err := fmt.Fprintf(out, "  %s  revision %d  snapshot %s  revisions=%d\n", profile.Name, profile.Revision, profile.SnapshotID, profile.Revisions); err != nil {
			return err
		}
	}
	for _, note := range list.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}

func renderProfile(out io.Writer, profile SetupProfile, revision SetupProfileRevision) error {
	if _, err := fmt.Fprintf(out, "Profile %s revision %d\n  snapshot %s\n  taken %s; recorded %s\n", profile.Name, revision.Revision, revision.SnapshotID, revision.TakenAt.Format(time.RFC3339), revision.CreatedAt.Format(time.RFC3339)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  selected components: %d; omitted records: %d; software evidence: %d\n", snapshotComponentCount(revision.Preview), omissionCount(revision.Preview), len(revision.Software)); err != nil {
		return err
	}
	return nil
}
