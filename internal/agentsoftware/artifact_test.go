package agentsoftware

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type tarEntry struct {
	name     string
	data     []byte
	typeflag byte
}

func makeTarball(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Mode: 0o755, Size: int64(len(entry.data)), Typeflag: typeflag}
		if typeflag == tar.TypeSymlink {
			header.Linkname = "somewhere"
			header.Size = 0
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if typeflag == tar.TypeReg {
			if _, err := writer.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func artifactFor(archive, binary []byte) Artifact {
	sum := sha512.Sum512(archive)
	binarySum := sha256.Sum256(binary)
	return Artifact{
		URL: "http://example.test/opencode.tgz", SHA512: base64.StdEncoding.EncodeToString(sum[:]),
		BinarySHA256: hex.EncodeToString(binarySum[:]), BinarySize: int64(len(binary)),
		Member: "package/bin/opencode",
	}
}

func serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(server.Close)
	return server
}

func TestStageTarballExtractsAndVerifies(t *testing.T) {
	binary := []byte("#!/bin/sh\necho opencode\n")
	archive := makeTarball(t, tarEntry{name: "package/package.json", data: []byte(`{"name":"opencode-linux-x64"}`)}, tarEntry{name: "package/bin/opencode", data: binary})
	artifact := artifactFor(archive, binary)
	server := serve(t, archive)
	artifact.URL = server.URL + "/opencode.tgz"
	dir := t.TempDir()

	staged, err := StageTarball(context.Background(), server.Client(), dir, artifact)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged.Path)
	if staged.Size != int64(len(binary)) || staged.BinarySHA256 != artifact.BinarySHA256 || staged.SHA512 != artifact.SHA512 {
		t.Fatalf("unexpected staged metadata: %+v", staged)
	}
	data, err := os.ReadFile(staged.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, binary) {
		t.Fatal("staged binary differs")
	}
}

func TestStageTarballRejectsArchiveChecksum(t *testing.T) {
	binary := []byte("binary")
	archive := makeTarball(t, tarEntry{name: "package/bin/opencode", data: binary})
	artifact := artifactFor(archive, binary)
	artifact.SHA512 = "wrong"
	server := serve(t, archive)
	artifact.URL = server.URL + "/opencode.tgz"
	if _, err := StageTarball(context.Background(), server.Client(), t.TempDir(), artifact); !errors.Is(err, ErrArtifactChecksum) {
		t.Fatalf("error = %v", err)
	}
}

func TestStageTarballRejectsBinaryChecksum(t *testing.T) {
	binary := []byte("binary")
	archive := makeTarball(t, tarEntry{name: "package/bin/opencode", data: binary})
	artifact := artifactFor(archive, binary)
	artifact.BinarySHA256 = "00"
	server := serve(t, archive)
	artifact.URL = server.URL + "/opencode.tgz"
	if _, err := StageTarball(context.Background(), server.Client(), t.TempDir(), artifact); !errors.Is(err, ErrArtifactChecksum) {
		t.Fatalf("error = %v", err)
	}
}

func TestStageTarballRejectsWrongShape(t *testing.T) {
	binary := []byte("binary")
	archive := makeTarball(t, tarEntry{name: "package/bin/opencode", data: binary})
	artifact := artifactFor(archive, binary)
	server := serve(t, archive)
	artifact.URL = server.URL + "/opencode.tgz"

	missing := artifact
	missing.Member = "package/bin/other"
	if _, err := StageTarball(context.Background(), server.Client(), t.TempDir(), missing); !errors.Is(err, ErrArtifactShape) {
		t.Fatalf("missing member error = %v", err)
	}
	symlinkArchive := makeTarball(t, tarEntry{name: "package/bin/opencode", typeflag: tar.TypeSymlink})
	symlink := artifactFor(symlinkArchive, []byte("x"))
	symlinkServer := serve(t, symlinkArchive)
	symlink.URL = symlinkServer.URL + "/opencode.tgz"
	if _, err := StageTarball(context.Background(), server.Client(), t.TempDir(), symlink); !errors.Is(err, ErrArtifactShape) {
		t.Fatalf("symlink member error = %v", err)
	}
}

func TestStageTarballIgnoresTraversalEntries(t *testing.T) {
	binary := []byte("binary")
	archive := makeTarball(t,
		tarEntry{name: "../escaped", data: []byte("bad")},
		tarEntry{name: "package/bin/opencode", data: binary},
	)
	artifact := artifactFor(archive, binary)
	server := serve(t, archive)
	artifact.URL = server.URL + "/opencode.tgz"
	parent := t.TempDir()
	dir := filepath.Join(parent, "staging")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	staged, err := StageTarball(context.Background(), server.Client(), dir, artifact)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged.Path)
	if _, err := os.Stat(filepath.Join(parent, "escaped")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("traversal entry was written: %v", err)
	}
}

func TestStageTarballReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	t.Cleanup(server.Close)
	artifact := Artifact{URL: server.URL + "/missing.tgz", SHA512: "x", BinarySHA256: "y", BinarySize: 1, Member: "package/bin/opencode"}
	if _, err := StageTarball(context.Background(), server.Client(), t.TempDir(), artifact); !errors.Is(err, ErrArtifactDownload) {
		t.Fatalf("error = %v", err)
	}
}
