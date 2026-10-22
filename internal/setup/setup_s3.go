package setup

import (
	"context"
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
