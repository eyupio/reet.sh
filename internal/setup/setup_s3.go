package setup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/objectstore"
	"github.com/eyupio/reet.sh/internal/sealed"
)

const (
	EnvSetupS3Endpoint  = "REET_SETUP_S3_ENDPOINT"
	EnvSetupS3Region    = "REET_SETUP_S3_REGION"
	EnvSetupS3Bucket    = "REET_SETUP_S3_BUCKET"
	EnvSetupS3Prefix    = "REET_SETUP_S3_PREFIX"
	EnvSetupS3AccessKey = "REET_SETUP_S3_ACCESS_KEY"
	EnvSetupS3SecretKey = "REET_SETUP_S3_SECRET_KEY"
	EnvSetupS3PathStyle = "REET_SETUP_S3_PATH_STYLE"
)

type SetupS3Result struct {
	Key      string `json:"key"`
	Bytes    int64  `json:"bytes"`
	Verified bool   `json:"verified"`
}

type SetupFetchResult struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Key   string `json:"key"`
}

type SetupRemoteSnapshot struct {
	ID       string    `json:"id"`
	Key      string    `json:"key"`
	Bytes    int64     `json:"bytes"`
	Modified time.Time `json:"modified,omitempty"`
}

type SetupRemoteList struct {
	SchemaVersion int                   `json:"schema_version"`
	Prefix        string                `json:"prefix"`
	Snapshots     []SetupRemoteSnapshot `json:"snapshots"`
	Limitations   []string              `json:"limitations"`
}

func SetupS3FromEnv() (objectstore.Config, string, error) {
	config := objectstore.Config{
		Endpoint:  strings.TrimSpace(os.Getenv(EnvSetupS3Endpoint)),
		Region:    strings.TrimSpace(os.Getenv(EnvSetupS3Region)),
		Bucket:    strings.TrimSpace(os.Getenv(EnvSetupS3Bucket)),
		AccessKey: strings.TrimSpace(os.Getenv(EnvSetupS3AccessKey)),
		SecretKey: strings.TrimSpace(os.Getenv(EnvSetupS3SecretKey)),
	}
	if config.Endpoint == "" {
		return objectstore.Config{}, "", fmt.Errorf("no setup S3 destination is configured; set %s and the other %s* variables", EnvSetupS3Endpoint, "REET_SETUP_S3_")
	}
	prefix := strings.TrimSpace(os.Getenv(EnvSetupS3Prefix))
	if prefix == "" {
		prefix = "reet/setup/"
	}
	prefix = strings.Trim(prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	if value := strings.TrimSpace(os.Getenv(EnvSetupS3PathStyle)); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return objectstore.Config{}, "", fmt.Errorf("%s must be true or false", EnvSetupS3PathStyle)
		}
		config.PathStyle = &parsed
	}
	return config, prefix, nil
}

func setupS3Key(prefix, id string) string {
	return prefix + id + setupSnapshotExtension
}

func uploadSetupSnapshotFile(path, id string) (SetupS3Result, error) {
	if !validSetupSnapshotID(id) {
		return SetupS3Result{}, errors.New("invalid setup snapshot identifier")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SetupS3Result{}, fmt.Errorf("read setup snapshot for upload: %w", err)
	}
	config, prefix, err := SetupS3FromEnv()
	if err != nil {
		return SetupS3Result{}, err
	}
	client, err := objectstore.New(config)
	if err != nil {
		return SetupS3Result{}, err
	}
	key := setupS3Key(prefix, id)
	if err := client.Put(context.Background(), key, data); err != nil {
		return SetupS3Result{}, fmt.Errorf("upload setup snapshot: %w", err)
	}
	got, err := client.Get(context.Background(), key)
	if err != nil {
		return SetupS3Result{}, fmt.Errorf("verify uploaded setup snapshot: %w", err)
	}
	if string(got) != string(data) {
		return SetupS3Result{}, errors.New("the uploaded setup snapshot did not read back identically")
	}
	return SetupS3Result{Key: key, Bytes: int64(len(data)), Verified: true}, nil
}

func FetchSetupSnapshot(id, dir string) (SetupFetchResult, error) {
	if !validSetupSnapshotID(id) {
		return SetupFetchResult{}, errors.New("invalid setup snapshot identifier")
	}
	config, prefix, err := SetupS3FromEnv()
	if err != nil {
		return SetupFetchResult{}, err
	}
	client, err := objectstore.New(config)
	if err != nil {
		return SetupFetchResult{}, err
	}
	key := setupS3Key(prefix, id)
	data, err := client.Get(context.Background(), key)
	if err != nil {
		return SetupFetchResult{}, fmt.Errorf("download setup snapshot: %w", err)
	}
	manifest, err := sealed.ManifestOf(data)
	if err != nil || manifest.Kind != setupSnapshotKind || manifest.ID != id {
		return SetupFetchResult{}, errors.New("the downloaded object is not the requested setup snapshot")
	}
	if dir == "" {
		dir, err = SnapshotDir()
		if err != nil {
			return SetupFetchResult{}, err
		}
	}
	if err := prepareSnapshotDir(dir); err != nil {
		return SetupFetchResult{}, err
	}
	path := filepath.Join(dir, id+setupSnapshotExtension)
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		if err == nil {
			return SetupFetchResult{}, errors.New("a local setup snapshot with that identifier already exists")
		}
		return SetupFetchResult{}, err
	}
	if err := writeSnapshotFile(path, data, 0o600); err != nil {
		return SetupFetchResult{}, err
	}
	return SetupFetchResult{ID: id, Path: path, Bytes: int64(len(data)), Key: key}, nil
}

func renderSetupFetch(out io.Writer, result SetupFetchResult) error {
	_, err := fmt.Fprintf(out, "Fetched setup snapshot %s (%d bytes) to %q from %q.\n", result.ID, result.Bytes, result.Path, result.Key)
	return err
}

// ListRemoteSetupSnapshots reads the configured S3 prefix without downloading
// snapshot payloads.
func ListRemoteSetupSnapshots() (SetupRemoteList, error) {
	config, prefix, err := SetupS3FromEnv()
	if err != nil {
		return SetupRemoteList{}, err
	}
	client, err := objectstore.New(config)
	if err != nil {
		return SetupRemoteList{}, err
	}
	objects, err := client.List(context.Background(), prefix)
	if err != nil {
		return SetupRemoteList{}, err
	}
	list := SetupRemoteList{
		SchemaVersion: 1, Prefix: prefix, Snapshots: []SetupRemoteSnapshot{},
		Limitations: []string{
			"Remote listing reads object metadata only; snapshots are not downloaded or opened.",
			"Remote deletion, retention and destination testing are not implemented.",
		},
	}
	for _, object := range objects {
		if !strings.HasSuffix(object.Key, setupSnapshotExtension) {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(object.Key), setupSnapshotExtension)
		if !validSetupSnapshotID(id) {
			continue
		}
		list.Snapshots = append(list.Snapshots, SetupRemoteSnapshot{ID: id, Key: object.Key, Bytes: object.Size, Modified: object.LastModified})
	}
	sort.Slice(list.Snapshots, func(i, j int) bool {
		if list.Snapshots[i].Modified.Equal(list.Snapshots[j].Modified) {
			return list.Snapshots[i].ID > list.Snapshots[j].ID
		}
		return list.Snapshots[i].Modified.After(list.Snapshots[j].Modified)
	})
	return list, nil
}

func renderRemoteSetupList(out io.Writer, list SetupRemoteList) error {
	if _, err := fmt.Fprintf(out, "Remote setup snapshots under %q\n", list.Prefix); err != nil {
		return err
	}
	if len(list.Snapshots) == 0 {
		if _, err := fmt.Fprintln(out, "No remote setup snapshots found."); err != nil {
			return err
		}
	}
	for _, snapshot := range list.Snapshots {
		if _, err := fmt.Fprintf(out, "  %s  %d bytes  %s\n", snapshot.ID, snapshot.Bytes, snapshot.Modified.Format(time.RFC3339)); err != nil {
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

type SetupRemoteDeleteResult struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

type SetupRemotePruneResult struct {
	SchemaVersion int                   `json:"schema_version"`
	Prefix        string                `json:"prefix"`
	Keep          int                   `json:"keep"`
	Applied       bool                  `json:"applied"`
	Total         int                   `json:"total"`
	Kept          int                   `json:"kept"`
	Protected     []SetupPruneProtected `json:"protected,omitempty"`
	WouldRemove   []string              `json:"would_remove,omitempty"`
	Removed       []string              `json:"removed,omitempty"`
	Limitations   []string              `json:"limitations"`
}

func DeleteRemoteSetupSnapshot(id string) (SetupRemoteDeleteResult, error) {
	if !validSetupSnapshotID(id) {
		return SetupRemoteDeleteResult{}, errors.New("invalid setup snapshot identifier")
	}
	if reason, ok := profileSnapshotReferences("")[id]; ok {
		return SetupRemoteDeleteResult{}, fmt.Errorf("snapshot %s is referenced by %s; remove or revise that profile first", id, reason)
	}
	config, prefix, err := SetupS3FromEnv()
	if err != nil {
		return SetupRemoteDeleteResult{}, err
	}
	client, err := objectstore.New(config)
	if err != nil {
		return SetupRemoteDeleteResult{}, err
	}
	key := setupS3Key(prefix, id)
	if err := client.Delete(context.Background(), key); err != nil {
		return SetupRemoteDeleteResult{}, err
	}
	return SetupRemoteDeleteResult{ID: id, Key: key}, nil
}

func PruneRemoteSetupSnapshots(keep int, apply bool) (SetupRemotePruneResult, error) {
	if keep < 1 {
		return SetupRemotePruneResult{}, errors.New("--keep must be at least 1")
	}
	config, prefix, err := SetupS3FromEnv()
	if err != nil {
		return SetupRemotePruneResult{}, err
	}
	client, err := objectstore.New(config)
	if err != nil {
		return SetupRemotePruneResult{}, err
	}
	objects, err := client.List(context.Background(), prefix)
	if err != nil {
		return SetupRemotePruneResult{}, err
	}
	result := SetupRemotePruneResult{
		SchemaVersion: 1, Prefix: prefix, Keep: keep, Applied: apply,
		Protected: []SetupPruneProtected{}, WouldRemove: []string{}, Removed: []string{},
		Limitations: []string{
			"Only objects ending in .reetsetup under the configured prefix are pruned.",
			"Snapshots referenced by any local profile revision are protected.",
		},
	}
	protected := profileSnapshotReferences("")
	type remote struct {
		id, key  string
		modified time.Time
	}
	var snapshots []remote
	for _, object := range objects {
		if !strings.HasSuffix(object.Key, setupSnapshotExtension) {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(object.Key), setupSnapshotExtension)
		if !validSetupSnapshotID(id) {
			continue
		}
		snapshots = append(snapshots, remote{id: id, key: object.Key, modified: object.LastModified})
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].modified.Equal(snapshots[j].modified) {
			return snapshots[i].id > snapshots[j].id
		}
		return snapshots[i].modified.After(snapshots[j].modified)
	})
	result.Total = len(snapshots)
	for i, snapshot := range snapshots {
		if i < keep {
			result.Kept++
			continue
		}
		if reason, ok := protected[snapshot.id]; ok {
			result.Protected = append(result.Protected, SetupPruneProtected{ID: snapshot.id, Reason: reason})
			continue
		}
		if !apply {
			result.WouldRemove = append(result.WouldRemove, snapshot.key)
			continue
		}
		if err := client.Delete(context.Background(), snapshot.key); err != nil {
			return SetupRemotePruneResult{}, fmt.Errorf("delete %q: %w", snapshot.key, err)
		}
		result.Removed = append(result.Removed, snapshot.key)
	}
	return result, nil
}

func renderRemoteDelete(out io.Writer, result SetupRemoteDeleteResult) error {
	_, err := fmt.Fprintf(out, "Deleted remote setup snapshot %s (%s).\n", result.ID, result.Key)
	return err
}

func renderRemotePrune(out io.Writer, result SetupRemotePruneResult) error {
	mode := "Would remove"
	if result.Applied {
		mode = "Removed"
	}
	if _, err := fmt.Fprintf(out, "Remote setup snapshot prune under %q\n  %d total, keeping %d new\n", result.Prefix, result.Total, result.Keep); err != nil {
		return err
	}
	for _, key := range result.WouldRemove {
		if _, err := fmt.Fprintf(out, "  %s %q\n", mode, key); err != nil {
			return err
		}
	}
	for _, key := range result.Removed {
		if _, err := fmt.Fprintf(out, "  removed %q\n", key); err != nil {
			return err
		}
	}
	for _, protected := range result.Protected {
		if _, err := fmt.Fprintf(out, "  protected %s: %s\n", protected.ID, protected.Reason); err != nil {
			return err
		}
	}
	if len(result.WouldRemove) == 0 && len(result.Removed) == 0 {
		if _, err := fmt.Fprintln(out, "Nothing to prune."); err != nil {
			return err
		}
	}
	for _, note := range result.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}

type SetupS3Check struct {
	Step   string `json:"step"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// TestSetupS3Destination writes, reads and deletes a small object under the
// configured prefix. It does not list or touch snapshots.
func TestSetupS3Destination() ([]SetupS3Check, error) {
	config, prefix, err := SetupS3FromEnv()
	if err != nil {
		return nil, err
	}
	client, err := objectstore.New(config)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	key := prefix + ".connection-test/" + hex.EncodeToString(nonce) + ".txt"
	body := []byte("A setup connection test from reet.sh\n")
	var steps []SetupS3Check
	step := func(name string, err error, detail string) error {
		if err != nil {
			steps = append(steps, SetupS3Check{Step: name, Detail: err.Error()})
			return err
		}
		steps = append(steps, SetupS3Check{Step: name, OK: true, Detail: detail})
		return nil
	}
	if err := step("put", client.Put(context.Background(), key, body), "Wrote "+key+"."); err != nil {
		return steps, err
	}
	got, err := client.Get(context.Background(), key)
	if err == nil && string(got) != string(body) {
		err = errors.New("the object read back is not the one written")
	}
	if err := step("get", err, "Read it back byte for byte."); err != nil {
		_ = client.Delete(context.Background(), key)
		return steps, err
	}
	if err := step("delete", client.Delete(context.Background(), key), "Deleted it again."); err != nil {
		return steps, err
	}
	return steps, nil
}

func renderSetupS3Test(out io.Writer, steps []SetupS3Check) error {
	for _, step := range steps {
		if _, err := fmt.Fprintf(out, "  [%t] %s: %s\n", step.OK, step.Step, step.Detail); err != nil {
			return err
		}
	}
	return nil
}
