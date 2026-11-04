package noderuntime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactForOnlyReviewedPlatforms(t *testing.T) {
	for _, platform := range []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"} {
		if _, ok := ArtifactFor(filepath.Dir(platform), filepath.Base(platform)); !ok {
			t.Fatalf("missing reviewed build for %s", platform)
		}
	}
	if _, ok := ArtifactFor("windows", "amd64"); ok {
		t.Fatal("windows has no reviewed build")
	}
}

func TestManagedRequiresBothBinaries(t *testing.T) {
	home := t.TempDir()
	if _, ok := Managed(home); ok {
		t.Fatal("empty home has no managed runtime")
	}
	bin := filepath.Join(Dir(home), "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "npx"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, ok := Managed(home); ok {
		t.Fatal("npx without node is incomplete")
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, ok := Managed(home); !ok {
		t.Fatal("complete runtime was not found")
	}
}

func TestExtractStripsRootAndRejectsEscapes(t *testing.T) {
	archive := writeArchive(t, []entry{
		{name: "node-v1/bin/node", mode: 0755, body: "binary"},
		{name: "node-v1/lib/node_modules/npm/bin/npx-cli.js", mode: 0644, body: "js"},
		{name: "node-v1/bin/npx", link: "../lib/node_modules/npm/bin/npx-cli.js"},
	})
	destination := t.TempDir()
	if err := extract(archive, destination); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(destination, "bin", "node"))
	if err != nil || string(body) != "binary" {
		t.Fatalf("stripped binary = %q, %v", body, err)
	}
	link, err := os.Readlink(filepath.Join(destination, "bin", "npx"))
	if err != nil || link != "../lib/node_modules/npm/bin/npx-cli.js" {
		t.Fatalf("symlink = %q, %v", link, err)
	}
	escaping := writeArchive(t, []entry{{name: "node-v1/../evil", mode: 0644, body: "x"}})
	if err := extract(escaping, t.TempDir()); !errors.Is(err, ErrShape) {
		t.Fatalf("escaping entry = %v", err)
	}
}

func TestDownloadChecksTheRecordedChecksum(t *testing.T) {
	body := []byte("reviewed archive")
	sum := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer server.Close()
	build := artifact{URL: server.URL, SHA256: hex.EncodeToString(sum[:])}
	destination := filepath.Join(t.TempDir(), "node.tar.gz")
	if err := download(t.Context(), server.Client(), build, destination); err != nil {
		t.Fatal(err)
	}
	build.SHA256 = hex.EncodeToString(bytes.Repeat([]byte{0}, 32))
	if err := download(t.Context(), server.Client(), build, filepath.Join(t.TempDir(), "bad")); !errors.Is(err, ErrChecksum) {
		t.Fatalf("wrong checksum = %v", err)
	}
}

type entry struct {
	name string
	mode int64
	body string
	link string
}

func writeArchive(t *testing.T, entries []entry) string {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressed)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.body))}
		if e.link != "" {
			header.Typeflag = tar.TypeSymlink
			header.Linkname = e.link
			header.Size = 0
		} else if header.Mode == 0 {
			header.Mode = 0644
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := writer.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "node.tar.gz")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMajorReadsTheNodeOnPath(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\nprintf 'v20.11.1\\n'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	major, ok := Major()
	if !ok || major != 20 {
		t.Fatalf("major = %d, %t", major, ok)
	}
	t.Setenv("PATH", t.TempDir())
	if _, ok := Major(); ok {
		t.Fatal("missing node reported a version")
	}
}
