package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
)

const (
	setupSnapshotListSchemaVersion       = 1
	setupSnapshotInspectionSchemaVersion = 1
	setupHeaderProbeBytes                = sealed.MaxHeaderBytes + 64
)

type SetupSnapshotHeader struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	TakenAt     time.Time `json:"taken_at"`
	Bytes       int64     `json:"bytes"`
	Version     string    `json:"version,omitempty"`
	Protection  string    `json:"protection"`
	Compression string    `json:"compression"`
}

type SetupSnapshotSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type SetupSnapshotList struct {
	SchemaVersion int                   `json:"schema_version"`
	Dir           string                `json:"dir"`
	Snapshots     []SetupSnapshotHeader `json:"snapshots"`
	Skipped       []SetupSnapshotSkip   `json:"skipped,omitempty"`
	Limitations   []string              `json:"limitations"`
}

type SetupSnapshotSourceSummary struct {
	Path         string `json:"path"`
	Component    string `json:"component"`
	Fields       int    `json:"fields"`
	Declarations int    `json:"declarations,omitempty"`
	Digest       string `json:"digest"`
}

type SetupSnapshotInspection struct {
	SchemaVersion int                          `json:"schema_version"`
	Header        SetupSnapshotHeader          `json:"header"`
	Opened        bool                         `json:"opened"`
	Preview       *SnapshotPreview             `json:"preview,omitempty"`
	Sources       []SetupSnapshotSourceSummary `json:"sources,omitempty"`
	Software      []SetupSoftware              `json:"software,omitempty"`
	Limitations   []string                     `json:"limitations"`
}

// ListSetupSnapshots reads clear headers only. It never opens an encrypted
// payload and never prints source content.
func ListSetupSnapshots(dir string) (SetupSnapshotList, error) {
	if dir == "" {
		var err error
		dir, err = SnapshotDir()
		if err != nil {
			return SetupSnapshotList{}, err
		}
	}
	list := SetupSnapshotList{
		SchemaVersion: setupSnapshotListSchemaVersion, Dir: dir,
		Snapshots: []SetupSnapshotHeader{}, Skipped: []SetupSnapshotSkip{},
		Limitations: []string{
			"Listing reads clear headers only; encrypted payloads are not opened and selected values are not shown.",
			"Only regular .reetsetup files in this directory are listed; symlinks, temporary files and other kinds are reported or ignored.",
			"This listing does not prove that a snapshot is complete, restorable or compatible with another OS.",
		},
	}
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return list, nil
	}
	if err != nil {
		return SetupSnapshotList{}, fmt.Errorf("inspect setup snapshot directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return SetupSnapshotList{}, fmt.Errorf("the setup snapshot directory must be a real directory, not a symlink or file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return SetupSnapshotList{}, fmt.Errorf("read setup snapshot directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		fileInfo, err := os.Lstat(path)
		if err != nil {
			list.Skipped = append(list.Skipped, SetupSnapshotSkip{Path: path, Reason: "metadata_unavailable"})
			continue
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			list.Skipped = append(list.Skipped, SetupSnapshotSkip{Path: path, Reason: "symlink"})
			continue
		}
		if !fileInfo.Mode().IsRegular() {
			list.Skipped = append(list.Skipped, SetupSnapshotSkip{Path: path, Reason: "not_regular"})
			continue
		}
		if strings.HasPrefix(entry.Name(), ".reet-setup-") {
			list.Skipped = append(list.Skipped, SetupSnapshotSkip{Path: path, Reason: "incomplete_temporary_file"})
			continue
		}
		if filepath.Ext(entry.Name()) != setupSnapshotExtension {
			continue
		}
		if fileInfo.Size() > sealed.MaxObjectBytes {
			list.Skipped = append(list.Skipped, SetupSnapshotSkip{Path: path, Reason: "too_large"})
			continue
		}
		manifest, err := readSetupHeader(path, fileInfo)
		if err != nil {
			list.Skipped = append(list.Skipped, SetupSnapshotSkip{Path: path, Reason: "invalid_header"})
			continue
		}
		if manifest.Kind != setupSnapshotKind {
			list.Skipped = append(list.Skipped, SetupSnapshotSkip{Path: path, Reason: "not_setup_snapshot"})
			continue
		}
		list.Snapshots = append(list.Snapshots, SetupSnapshotHeader{
			ID: manifest.ID, Path: path, TakenAt: time.UnixMilli(manifest.TakenAt).UTC(),
			Bytes: fileInfo.Size(), Version: manifest.Version,
			Protection: manifest.Protection, Compression: manifest.Compression,
		})
	}
	sort.Slice(list.Snapshots, func(i, j int) bool {
		if list.Snapshots[i].TakenAt.Equal(list.Snapshots[j].TakenAt) {
			return list.Snapshots[i].ID < list.Snapshots[j].ID
		}
		return list.Snapshots[i].TakenAt.After(list.Snapshots[j].TakenAt)
	})
	return list, nil
}

// InspectSetupSnapshot shows the clear header, and the encrypted preview and
// source summaries when a passphrase is supplied. It never returns values.
func InspectSetupSnapshot(path, password, dir string) (SetupSnapshotInspection, error) {
	path, err := resolveSetupSnapshotPath(path, dir)
	if err != nil {
		return SetupSnapshotInspection{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return SetupSnapshotInspection{}, fmt.Errorf("inspect setup snapshot: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return SetupSnapshotInspection{}, fmt.Errorf("the setup snapshot must be a regular file, not a symlink")
	}
	if info.Size() > sealed.MaxObjectBytes {
		return SetupSnapshotInspection{}, fmt.Errorf("the setup snapshot is larger than this build can open")
	}
	manifest, err := readSetupHeader(path, info)
	if err != nil || manifest.Kind != setupSnapshotKind {
		return SetupSnapshotInspection{}, fmt.Errorf("this is not a readable reet setup snapshot")
	}
	inspection := SetupSnapshotInspection{
		SchemaVersion: setupSnapshotInspectionSchemaVersion,
		Header: SetupSnapshotHeader{
			ID: manifest.ID, Path: path, TakenAt: time.UnixMilli(manifest.TakenAt).UTC(),
			Bytes: info.Size(), Version: manifest.Version,
			Protection: manifest.Protection, Compression: manifest.Compression,
		},
		Limitations: []string{
			"Inspection never prints selected values, source file contents or credential material.",
			"Opening the payload verifies its archive digest; it does not prove restore compatibility or completeness.",
		},
	}
	if password == "" {
		inspection.Limitations = append(inspection.Limitations, "Give --password-file FILE or - to open the encrypted preview and source summaries.")
		return inspection, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return SetupSnapshotInspection{}, fmt.Errorf("read setup snapshot: %w", err)
	}
	snapshot, _, err := openSetupSnapshot(raw, password)
	if err != nil {
		return SetupSnapshotInspection{}, err
	}
	preview := snapshot.Preview
	inspection.Opened = true
	inspection.Preview = &preview
	inspection.Software = snapshot.Software
	for _, source := range snapshot.Sources {
		summary := SetupSnapshotSourceSummary{
			Path: source.Path, Component: source.Component, Fields: len(source.Content),
		}
		if source.Component == "mcp" {
			summary.Declarations = len(source.Content)
			summary.Fields = 0
		}
		for _, agent := range preview.Agents {
			for _, component := range agent.Components {
				if component.Source == source.Path && component.Kind == source.Component {
					summary.Digest = component.Digest
				}
			}
		}
		inspection.Sources = append(inspection.Sources, summary)
	}
	return inspection, nil
}

func readSetupHeader(path string, before os.FileInfo) (sealed.Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return sealed.Manifest{}, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return sealed.Manifest{}, fmt.Errorf("setup snapshot changed during read")
	}
	buf := make([]byte, setupHeaderProbeBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return sealed.Manifest{}, err
	}
	return sealed.ManifestOf(buf[:n])
}

func resolveSetupSnapshotPath(path, dir string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("name a setup snapshot or its .reetsetup file")
	}
	if filepath.Base(path) == path && !strings.ContainsAny(path, `/\`) && !strings.HasSuffix(path, setupSnapshotExtension) {
		if !validSetupSnapshotID(path) {
			return "", fmt.Errorf("invalid setup snapshot identifier")
		}
		if dir == "" {
			var err error
			dir, err = SnapshotDir()
			if err != nil {
				return "", err
			}
		}
		return filepath.Join(dir, path+setupSnapshotExtension), nil
	}
	return path, nil
}

func validSetupSnapshotID(id string) bool {
	if !strings.HasPrefix(id, "setup-") || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c == '-' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func renderSnapshotList(out io.Writer, list SetupSnapshotList) error {
	if _, err := fmt.Fprintf(out, "Setup snapshots in %q\n", list.Dir); err != nil {
		return err
	}
	if len(list.Snapshots) == 0 {
		if _, err := fmt.Fprintln(out, "No setup snapshots found."); err != nil {
			return err
		}
	}
	for _, snapshot := range list.Snapshots {
		if _, err := fmt.Fprintf(out, "  %s  %s  %d bytes  %s/%s\n", snapshot.ID, snapshot.TakenAt.Format(time.RFC3339), snapshot.Bytes, snapshot.Protection, snapshot.Compression); err != nil {
			return err
		}
	}
	for _, skipped := range list.Skipped {
		if _, err := fmt.Fprintf(out, "  skipped %q: %s\n", skipped.Path, skipped.Reason); err != nil {
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

func renderSnapshotInspection(out io.Writer, inspection SetupSnapshotInspection) error {
	header := inspection.Header
	if _, err := fmt.Fprintf(out, "Setup snapshot %s\n  %q\n  taken %s; %d bytes; %s/%s\n", header.ID, header.Path, header.TakenAt.Format(time.RFC3339), header.Bytes, header.Protection, header.Compression); err != nil {
		return err
	}
	if !inspection.Opened {
		for _, note := range inspection.Limitations {
			if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
				return err
			}
		}
		return nil
	}
	if inspection.Preview != nil {
		if _, err := fmt.Fprintf(out, "  selected components: %d; omitted records: %d\n", snapshotComponentCount(*inspection.Preview), omissionCount(*inspection.Preview)); err != nil {
			return err
		}
	}
	for _, source := range inspection.Sources {
		if _, err := fmt.Fprintf(out, "  %s %q: fields=%d declarations=%d digest=%s\n", source.Component, source.Path, source.Fields, source.Declarations, source.Digest); err != nil {
			return err
		}
	}
	for _, software := range inspection.Software {
		if _, err := fmt.Fprintf(out, "  software %s: version=%s state=%s package=%s install_version=%s manager=%s\n", software.Name, software.Version, software.VersionState, software.Package, software.InstallVersion, software.Manager); err != nil {
			return err
		}
	}
	if inspection.Preview != nil {
		for _, class := range inspection.Preview.Classes {
			if _, err := fmt.Fprintf(out, "  class %s: %s, %s\n", class.Kind, class.Selection, class.Handling); err != nil {
				return err
			}
		}
	}
	for _, note := range inspection.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}

func snapshotComponentCount(preview SnapshotPreview) int {
	total := 0
	for _, agent := range preview.Agents {
		total += len(agent.Components)
	}
	return total
}
