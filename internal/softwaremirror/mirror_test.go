package softwaremirror

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
)

func fakeMirrorArtifact(t *testing.T, binary []byte, handler func(http.ResponseWriter, *http.Request)) (agentsoftware.Artifact, *httptest.Server) {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gzipWriter)
	if err := writer.WriteHeader(&tar.Header{Name: "package/bin/opencode", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	archive := buffer.Bytes()
	sum := sha512.Sum512(archive)
	archiveSum := sha256.Sum256(archive)
	binarySum := sha256.Sum256(binary)
	if handler == nil {
		handler = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	return agentsoftware.Artifact{
		URL:           server.URL + "/opencode.tgz",
		ArchiveSHA256: hex.EncodeToString(archiveSum[:]),
		ArchiveSize:   int64(len(archive)),
		SHA512:        base64.StdEncoding.EncodeToString(sum[:]),
		BinarySHA256:  hex.EncodeToString(binarySum[:]),
		BinarySize:    int64(len(binary)),
		Member:        "package/bin/opencode",
	}, server
}

func TestEnsureCachesAndVerifies(t *testing.T) {
	artifact, _ := fakeMirrorArtifact(t, []byte("reviewed binary"), nil)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(mustArchive(t, []byte("reviewed binary")))
	}))
	t.Cleanup(server.Close)
	artifact.URL = server.URL + "/opencode.tgz"

	dir := t.TempDir()
	mirror := New(dir, artifact)
	if status := mirror.Status(); status.Present {
		t.Fatalf("fresh mirror reports cached: %+v", status)
	}
	cached, err := mirror.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !cached.Present || cached.Size != artifact.ArchiveSize || cached.Digest != artifact.ArchiveSHA256 {
		t.Fatalf("cached: %+v", cached)
	}
	if _, err := os.Stat(mirror.Path()); err != nil {
		t.Fatal(err)
	}
	if _, err := mirror.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("downloads = %d, want 1", got)
	}
}

func mustArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gzipWriter)
	if err := writer.WriteHeader(&tar.Header{Name: "package/bin/opencode", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestEnsureSharesOneDownload(t *testing.T) {
	artifact, _ := fakeMirrorArtifact(t, []byte("reviewed binary"), nil)
	var requests atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		close(started)
		<-release
		_, _ = w.Write(mustArchive(t, []byte("reviewed binary")))
	}))
	t.Cleanup(server.Close)
	artifact.URL = server.URL + "/opencode.tgz"

	mirror := New(t.TempDir(), artifact)
	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := mirror.Ensure(context.Background())
			errs <- err
		}()
	}
	<-started
	if got := requests.Load(); got != 1 {
		t.Fatalf("concurrent downloads = %d, want 1", got)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("downloads = %d, want 1", got)
	}
}

func TestEnsureRejectsACorruptArchive(t *testing.T) {
	artifact, _ := fakeMirrorArtifact(t, []byte("reviewed binary"), nil)
	artifact.SHA512 = "wrong"
	mirror := New(t.TempDir(), artifact)
	if _, err := mirror.Ensure(context.Background()); err == nil {
		t.Fatal("a corrupt archive was accepted")
	}
	if _, err := os.Stat(mirror.Path()); !os.IsNotExist(err) {
		t.Fatalf("a corrupt archive was left in place: %v", err)
	}
}

func TestStatusRejectsATruncatedFile(t *testing.T) {
	artifact, _ := fakeMirrorArtifact(t, []byte("reviewed binary"), nil)
	dir := t.TempDir()
	mirror := New(dir, artifact)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mirror.Path(), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if status := mirror.Status(); status.Present {
		t.Fatalf("a truncated file looked cached: %+v", status)
	}
}

func TestGCKeepsOnlyTheReviewedDigest(t *testing.T) {
	artifact, _ := fakeMirrorArtifact(t, []byte("reviewed binary"), nil)
	dir := t.TempDir()
	mirror := New(dir, artifact)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Base(mirror.Path())
	if err := os.WriteFile(mirror.Path(), make([]byte, artifact.ArchiveSize), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old-digest.tgz", ".opencode-stale.tgz"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := mirror.GC(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != keep {
		t.Fatalf("GC left %v, want only %s", entries, keep)
	}
}
