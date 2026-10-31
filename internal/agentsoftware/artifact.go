package agentsoftware

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Artifact is one reviewed package archive. The connector never accepts a URL,
// checksum or member name from the controller: this table is the whole set of
// things it will download.
type Artifact struct {
	URL          string
	SHA512       string // base64, as npm records it without the sha512- prefix
	BinarySHA256 string // hex
	BinarySize   int64
	Member       string // the one regular file extracted from the archive
}

// OpenCodeLinuxX64 is the archive the probe verified for OpenCode 1.19.5. The
// platform package contains only package.json and the binary; the binary is a
// single-file Bun executable.
var OpenCodeLinuxX64 = Artifact{
	URL:          "https://registry.npmjs.org/opencode-linux-x64/-/opencode-linux-x64-1.19.5.tgz",
	SHA512:       "i1qh7VooLGML9xX6ud93y0JA8j/1aPeDYRKjsmYXrJX1TjIWHgBBMkEbEJPZthpWbsJg//XfBgR6Kb7+NPaJiQ==",
	BinarySHA256: "7df4e4136301f16589b0f30bf91a61380fca5f8ef22dd36570f5ebe1d4ec802c",
	BinarySize:   214386864,
	Member:       "package/bin/opencode",
}

// MaxArtifactBytes bounds one downloaded archive. The reviewed binary is about
// 214 MiB and compresses well; 300 MiB leaves room without allowing an
// unbounded response into the connector's disk.
const MaxArtifactBytes = 300 << 20

var (
	ErrArtifactDownload = errors.New("the reviewed archive could not be downloaded")
	ErrArtifactTooLarge = errors.New("the reviewed archive was larger than allowed")
	ErrArtifactChecksum = errors.New("the reviewed archive did not match its recorded checksum")
	ErrArtifactShape    = errors.New("the reviewed archive did not contain the expected binary")
)

// Staged is one verified binary in a temporary file. The caller installs it or
// removes it; nothing here changes the managed store.
type Staged struct {
	Path         string
	SHA512       string
	BinarySHA256 string
	Size         int64
	Artifact     Artifact
}

// StageTarball downloads artifact.URL, verifies the archive's sha512, extracts
// only artifact.Member and verifies the binary's sha256 and size. It never
// executes the binary or an installer. dir holds both temporary files and must
// be on the same filesystem as the eventual install for an atomic rename.
func StageTarball(ctx context.Context, client *http.Client, dir string, artifact Artifact) (Staged, error) {
	if dir == "" || artifact.URL == "" || artifact.Member == "" || artifact.SHA512 == "" || artifact.BinarySHA256 == "" || artifact.BinarySize <= 0 {
		return Staged{}, ErrArtifactShape
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return Staged{}, ErrArtifactDownload
	}
	response, err := client.Do(request)
	if err != nil {
		return Staged{}, fmt.Errorf("%w: %v", ErrArtifactDownload, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Staged{}, fmt.Errorf("%w: HTTP %d", ErrArtifactDownload, response.StatusCode)
	}
	if response.ContentLength > MaxArtifactBytes {
		return Staged{}, ErrArtifactTooLarge
	}

	archive, err := os.CreateTemp(dir, ".opencode-*.tgz")
	if err != nil {
		return Staged{}, err
	}
	archivePath := archive.Name()
	defer func() {
		archive.Close()
		os.Remove(archivePath)
	}()
	hash := sha512.New()
	written, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(response.Body, MaxArtifactBytes+1))
	if err != nil {
		return Staged{}, fmt.Errorf("%w: %v", ErrArtifactDownload, err)
	}
	if written > MaxArtifactBytes {
		return Staged{}, ErrArtifactTooLarge
	}
	if base64.StdEncoding.EncodeToString(hash.Sum(nil)) != artifact.SHA512 {
		return Staged{}, ErrArtifactChecksum
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return Staged{}, err
	}
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		return Staged{}, ErrArtifactShape
	}
	defer compressed.Close()
	entries := tar.NewReader(compressed)
	for {
		header, err := entries.Next()
		if errors.Is(err, io.EOF) {
			return Staged{}, ErrArtifactShape
		}
		if err != nil {
			return Staged{}, ErrArtifactShape
		}
		if header.Name != artifact.Member {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size != artifact.BinarySize {
			return Staged{}, ErrArtifactShape
		}
		return stageBinary(dir, entries, artifact)
	}
}

func stageBinary(dir string, reader io.Reader, artifact Artifact) (Staged, error) {
	binary, err := os.CreateTemp(dir, ".opencode-binary-*")
	if err != nil {
		return Staged{}, err
	}
	path := binary.Name()
	cleanup := func() {
		binary.Close()
		os.Remove(path)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(binary, hash), io.LimitReader(reader, artifact.BinarySize+1))
	if err != nil {
		cleanup()
		return Staged{}, ErrArtifactShape
	}
	if written != artifact.BinarySize {
		cleanup()
		return Staged{}, ErrArtifactShape
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.BinarySHA256 {
		cleanup()
		return Staged{}, ErrArtifactChecksum
	}
	if err := binary.Sync(); err != nil {
		cleanup()
		return Staged{}, err
	}
	if err := binary.Close(); err != nil {
		os.Remove(path)
		return Staged{}, err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		os.Remove(path)
		return Staged{}, err
	}
	return Staged{Path: path, SHA512: artifact.SHA512, BinarySHA256: artifact.BinarySHA256, Size: artifact.BinarySize, Artifact: artifact}, nil
}
