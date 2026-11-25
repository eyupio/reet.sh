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
	"path/filepath"
	"strings"
	"time"
)

// Artifact is one reviewed package archive. The connector never accepts a URL,
// checksum or member name from the controller: this table is the whole set of
// things it will download.
type Artifact struct {
	URL           string
	ArchiveSHA256 string // hex, the content address a controller mirror serves under
	ArchiveSize   int64
	SHA512        string // base64, as npm records it without the sha512- prefix
	BinarySHA256  string // hex
	BinarySize    int64
	Member        string // the one regular file extracted from the archive
}

// OpenCodeLinuxX64 is the archive the probe verified for OpenCode 1.19.5. The
// platform package contains only package.json and the binary; the binary is a
// single-file Bun executable.
var OpenCodeLinuxX64 = Artifact{
	URL:           "https://registry.npmjs.org/opencode-linux-x64/-/opencode-linux-x64-1.19.5.tgz",
	ArchiveSHA256: "2d7cfc303decc6cbbcdb22ff1be258b03ce2b5eae57583545afc8f12c01dd5e0",
	ArchiveSize:   79136190,
	SHA512:        "i1qh7VooLGML9xX6ud93y0JA8j/1aPeDYRKjsmYXrJX1TjIWHgBBMkEbEJPZthpWbsJg//XfBgR6Kb7+NPaJiQ==",
	BinarySHA256:  "7df4e4136301f16589b0f30bf91a61380fca5f8ef22dd36570f5ebe1d4ec802c",
	BinarySize:    214386864,
	Member:        "package/bin/opencode",
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
	return StageTarballFrom(ctx, client, dir, artifact, artifact.URL, nil)
}

// StageTarballFrom is StageTarball from url instead of artifact.URL. A
// controller mirror serves the same archive under a different address; the
// archive and member are still verified against artifact. header, when set,
// is sent with the request; the mirror needs the machine's enrollment.
func StageTarballFrom(ctx context.Context, client *http.Client, dir string, artifact Artifact, url string, header http.Header) (Staged, error) {
	valid := dir != "" && url != "" && artifact.Member != "" && artifact.SHA512 != "" &&
		artifact.BinarySHA256 != "" && artifact.BinarySize > 0
	if !valid {
		return Staged{}, ErrArtifactShape
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	archive, err := downloadArchive(ctx, client, dir, artifact, url, header)
	if err != nil {
		return Staged{}, err
	}
	archivePath := archive.Name()
	// The archive is finished with, good or bad, once it has been read here; only a download that stopped part-way keeps its file.
	defer func() {
		archive.Close()
		os.Remove(archivePath)
	}()
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

// partialMaxAge is how long a half-downloaded archive is kept for a later attempt to continue.
const partialMaxAge = 24 * time.Hour

// partialPath is where a download of artifact waits between attempts. It is named for the archive's checksum, so a
// different artifact never continues it, and every source of the same archive (the registry, a mirror) continues the same
// file: the whole archive is checked against its checksum at the end whichever source supplied which part.
func partialPath(dir string, artifact Artifact) string {
	sum := sha256.Sum256([]byte(artifact.SHA512))
	return filepath.Join(dir, ".opencode-"+hex.EncodeToString(sum[:8])+".part")
}

// downloadArchive returns artifact's whole archive as an open file positioned at its start, verified against
// artifact.SHA512. A download that fails part-way leaves what it has in partialPath, and the next call asks the server
// for the rest (a Range request); a server that ignores the range sends the whole archive and the file starts again. An
// archive that fails its checksum is deleted, so a bad file is never continued.
//
// shortcut: one download of an artifact at a time (the software lock makes it so), and no If-Range: the end checksum is the
// guarantee that the pieces belong together.
func downloadArchive(ctx context.Context, client *http.Client, dir string, artifact Artifact, url string, header http.Header) (*os.File, error) {
	path := partialPath(dir, artifact)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*os.File, error) { file.Close(); return nil, err }
	restart := func() error {
		if err := file.Truncate(0); err != nil {
			return err
		}
		_, err := file.Seek(0, io.SeekStart)
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if size := info.Size(); size > MaxArtifactBytes || (artifact.ArchiveSize > 0 && size > artifact.ArchiveSize) || time.Since(info.ModTime()) > partialMaxAge {
		if err := restart(); err != nil {
			return fail(err)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		have, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			return fail(err)
		}
		complete := artifact.ArchiveSize > 0 && have == artifact.ArchiveSize
		if !complete {
			status, err := fetchInto(ctx, client, file, have, url, header)
			if err != nil {
				file.Close() // what arrived stays for the next attempt, unless it can never be used
				if errors.Is(err, ErrArtifactTooLarge) {
					os.Remove(path)
				}
				return nil, err
			}
			if status == fetchAgain {
				if err := restart(); err != nil {
					return fail(err)
				}
				continue
			}
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return fail(err)
		}
		hash := sha512.New()
		written, err := io.Copy(hash, file)
		if err != nil {
			return fail(err)
		}
		if written > MaxArtifactBytes {
			file.Close()
			os.Remove(path)
			return nil, ErrArtifactTooLarge
		}
		if base64.StdEncoding.EncodeToString(hash.Sum(nil)) != artifact.SHA512 {
			file.Close()
			os.Remove(path)
			return nil, ErrArtifactChecksum
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return fail(err)
		}
		return file, nil
	}
	return fail(fmt.Errorf("%w: the server would not continue the download", ErrArtifactDownload))
}

// PrunePartials deletes the half-downloaded archives in dir that are older than a download may be continued, so one that is never
// tried again does not sit on disk for ever.
func PrunePartials(dir string) {
	found, _ := filepath.Glob(filepath.Join(dir, ".opencode-*.part"))
	for _, path := range found {
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > partialMaxAge {
			os.Remove(path)
		}
	}
}

type fetchStatus int

const (
	fetched fetchStatus = iota
	fetchAgain
)

// fetchInto appends the rest of the archive, from byte have, to file (positioned at its end).
func fetchInto(ctx context.Context, client *http.Client, file *os.File, have int64, url string, header http.Header) (fetchStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fetched, ErrArtifactDownload
	}
	for name, values := range header {
		request.Header[name] = append([]string(nil), values...)
	}
	if have > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	response, err := client.Do(request)
	if err != nil {
		return fetched, fmt.Errorf("%w: %v", ErrArtifactDownload, err)
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusPartialContent && have > 0:
		var start, end, total int64
		if n, _ := fmt.Sscanf(response.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &total); n < 2 || start != have {
			return fetchAgain, nil
		}
	case response.StatusCode == http.StatusOK:
		// The server sent the whole archive (it ignored the range, or none was asked for): start the file again.
		if have > 0 {
			if err := file.Truncate(0); err != nil {
				return fetched, err
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return fetched, err
			}
			have = 0
		}
	case response.StatusCode == http.StatusRequestedRangeNotSatisfiable && have > 0:
		return fetchAgain, nil
	default:
		return fetched, fmt.Errorf("%w: HTTP %d", ErrArtifactDownload, response.StatusCode)
	}
	if response.ContentLength > MaxArtifactBytes-have {
		return fetched, ErrArtifactTooLarge
	}
	written, err := io.Copy(file, io.LimitReader(response.Body, MaxArtifactBytes-have+1))
	if err != nil {
		return fetched, fmt.Errorf("%w: %v", ErrArtifactDownload, err)
	}
	if have+written > MaxArtifactBytes {
		return fetched, ErrArtifactTooLarge
	}
	return fetched, nil
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

// CheckTarball verifies a downloaded archive at path against artifact: its
// size, sha256 and sha512, and the one member's type, size and sha256. It
// writes nothing.
func CheckTarball(path string, artifact Artifact) error {
	valid := artifact.ArchiveSHA256 != "" && artifact.ArchiveSize > 0 && artifact.Member != "" &&
		artifact.SHA512 != "" && artifact.BinarySHA256 != "" && artifact.BinarySize > 0
	if !valid {
		return ErrArtifactShape
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h256, h512 := sha256.New(), sha512.New()
	written, err := io.Copy(io.MultiWriter(h256, h512), io.LimitReader(f, artifact.ArchiveSize+1))
	if err != nil {
		return ErrArtifactDownload
	}
	if written != artifact.ArchiveSize {
		return ErrArtifactShape
	}
	if !strings.EqualFold(hex.EncodeToString(h256.Sum(nil)), artifact.ArchiveSHA256) {
		return ErrArtifactChecksum
	}
	if base64.StdEncoding.EncodeToString(h512.Sum(nil)) != artifact.SHA512 {
		return ErrArtifactChecksum
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	compressed, err := gzip.NewReader(f)
	if err != nil {
		return ErrArtifactShape
	}
	defer compressed.Close()
	entries := tar.NewReader(compressed)
	for {
		header, err := entries.Next()
		if errors.Is(err, io.EOF) {
			return ErrArtifactShape
		}
		if err != nil {
			return ErrArtifactShape
		}
		if header.Name != artifact.Member {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size != artifact.BinarySize {
			return ErrArtifactShape
		}
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(entries, artifact.BinarySize+1))
		if err != nil {
			return ErrArtifactShape
		}
		if n != artifact.BinarySize || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), artifact.BinarySHA256) {
			return ErrArtifactChecksum
		}
		return nil
	}
}
