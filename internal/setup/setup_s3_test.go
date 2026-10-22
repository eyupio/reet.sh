package setup

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeSetupS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	server  *httptest.Server
}

func newFakeSetupS3(t *testing.T) *fakeSetupS3 {
	t.Helper()
	f := &fakeSetupS3{objects: map[string][]byte{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unsigned", http.StatusForbidden)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/bucket/")
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			f.objects[key] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := f.objects[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(f.server.Close)
	t.Setenv(EnvSetupS3Endpoint, f.server.URL)
	t.Setenv(EnvSetupS3Bucket, "bucket")
	t.Setenv(EnvSetupS3AccessKey, "key")
	t.Setenv(EnvSetupS3SecretKey, "secret")
	t.Setenv(EnvSetupS3PathStyle, "true")
	t.Setenv(EnvSetupS3Prefix, "prefix/")
	return f
}

func writeLocalSnapshot(t *testing.T) SetupWriteResult {
	t.Helper()
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"S3-MODEL"}`)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: t.TempDir(), Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSetupSnapshotUploadAndFetch(t *testing.T) {
	fake := newFakeSetupS3(t)
	local := writeLocalSnapshot(t)
	remote, err := uploadSetupSnapshotFile(local.Path, local.ID)
	if err != nil {
		t.Fatal(err)
	}
	key := "prefix/" + local.ID + setupSnapshotExtension
	if remote.Key != key || remote.Bytes != local.Bytes || !remote.Verified {
		t.Fatalf("wrong upload result: %+v", remote)
	}
	fake.mu.Lock()
	uploaded := append([]byte(nil), fake.objects[key]...)
	fake.mu.Unlock()
	if !bytes.Equal(uploaded, mustRead(t, local.Path)) {
		t.Fatal("uploaded bytes differ from the local snapshot")
	}
	dir := t.TempDir()
	fetched, err := FetchSetupSnapshot(local.ID, dir)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.ID != local.ID || fetched.Key != key || fetched.Bytes != local.Bytes {
		t.Fatalf("wrong fetch result: %+v", fetched)
	}
	opened, err := OpenSetupSnapshot(mustRead(t, fetched.Path), testSetupPassphrase)
	if err != nil || len(opened.Sources) != 1 {
		t.Fatalf("fetched snapshot did not open: %+v %v", opened, err)
	}
	if _, err := FetchSetupSnapshot(local.ID, dir); err == nil {
		t.Fatal("fetch overwrote an existing local snapshot")
	}
}

func TestSetupFetchRejectsWrongObjectAndMissingConfig(t *testing.T) {
	fake := newFakeSetupS3(t)
	local := writeLocalSnapshot(t)
	otherID := "setup-20200101T000000Z-deadbeef"
	fake.mu.Lock()
	fake.objects["prefix/"+otherID+setupSnapshotExtension] = mustRead(t, local.Path)
	fake.mu.Unlock()
	if _, err := FetchSetupSnapshot(otherID, t.TempDir()); err == nil {
		t.Fatal("wrong snapshot identity accepted")
	}
	t.Setenv(EnvSetupS3Endpoint, "")
	if _, _, err := SetupS3FromEnv(); err == nil {
		t.Fatal("missing S3 configuration accepted")
	}
}

func TestSetupS3CLI(t *testing.T) {
	newFakeSetupS3(t)
	sourceHome := isolatedCLI(t)
	writeConfig(t, filepath.Join(sourceHome, ".claude.json"), `{"model":"S3-CLI-MODEL"}`)
	dir := t.TempDir()
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var written bytes.Buffer
	if err := Run([]string{"backup", "--s3", "--password-file", passFile, "--dir", dir, "--json"}, &written); err != nil {
		t.Fatal(err)
	}
	var result SetupWriteResult
	if err := json.Unmarshal(written.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.S3 == nil || !result.S3.Verified {
		t.Fatalf("CLI upload result missing: %+v", result)
	}
	fetchDir := t.TempDir()
	var fetchedOut bytes.Buffer
	if err := Run([]string{"fetch", "--s3", "--dir", fetchDir, "--json", result.ID}, &fetchedOut); err != nil {
		t.Fatal(err)
	}
	var fetched SetupFetchResult
	if err := json.Unmarshal(fetchedOut.Bytes(), &fetched); err != nil {
		t.Fatal(err)
	}
	if fetched.ID != result.ID || fetched.Path == "" {
		t.Fatalf("CLI fetch result wrong: %+v", fetched)
	}
	if err := Run([]string{"fetch", "--dir", fetchDir, result.ID}, &bytes.Buffer{}); err == nil {
		t.Fatal("fetch without --s3 accepted")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
