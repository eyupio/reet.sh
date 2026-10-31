// Package softwaremirror caches the one reviewed agent artifact on the
// controller so machines that cannot reach the registry can still apply it.
//
// A machine asks for the digest in its own pinned table, never a URL the
// controller chose. The controller downloads the archive once, verifies it
// against the same pinned digests, and makes it visible only after a full,
// verified write. A partial or corrupt file is never served.
package softwaremirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
)

// DownloadTimeout bounds one ingest of the reviewed archive.
const DownloadTimeout = 10 * time.Minute

// ErrDisabled means the artifact is not cached and the caller did not ask to
// cache it.
var ErrDisabled = errors.New("the artifact mirror is off")

// Cached describes the one reviewed artifact as it is on disk now.
type Cached struct {
	Present bool   `json:"cached"`
	Size    int64  `json:"size,omitempty"`
	Digest  string `json:"digest,omitempty"`
}

// Mirror is one artifact's content-addressed cache. Ensure is safe for
// concurrent callers: one download is shared and the rest wait for it.
type Mirror struct {
	dir      string
	artifact agentsoftware.Artifact
	client   *http.Client
	mu       sync.Mutex
	flight   *flight
}

type flight struct {
	done   chan struct{}
	cached Cached
	err    error
}

// New makes a mirror under dir for artifact.
func New(dir string, artifact agentsoftware.Artifact) *Mirror {
	return &Mirror{dir: dir, artifact: artifact, client: &http.Client{Timeout: DownloadTimeout}}
}

// Digest is the content address machines ask for.
func (m *Mirror) Digest() string { return m.artifact.ArchiveSHA256 }

// Path is the cached archive's path. The name is the archive's sha256.
func (m *Mirror) Path() string { return filepath.Join(m.dir, m.artifact.ArchiveSHA256+".tgz") }

// Status reports whether the archive is cached, without downloading it.
func (m *Mirror) Status() Cached {
	info, err := os.Stat(m.Path())
	if err != nil || !info.Mode().IsRegular() || info.Size() != m.artifact.ArchiveSize {
		return Cached{Digest: m.artifact.ArchiveSHA256}
	}
	return Cached{Present: true, Size: info.Size(), Digest: m.artifact.ArchiveSHA256}
}

// Ensure returns the cached archive, downloading and verifying it once if it
// is absent. A waiting caller can give up with its own context; the shared
// download continues for the others.
func (m *Mirror) Ensure(ctx context.Context) (Cached, error) {
	if cached := m.Status(); cached.Present {
		return cached, nil
	}
	m.mu.Lock()
	if f := m.flight; f != nil {
		m.mu.Unlock()
		select {
		case <-f.done:
			return f.cached, f.err
		case <-ctx.Done():
			return Cached{Digest: m.artifact.ArchiveSHA256}, ctx.Err()
		}
	}
	f := &flight{done: make(chan struct{})}
	m.flight = f
	m.mu.Unlock()

	ingestCtx, cancel := context.WithTimeout(context.Background(), DownloadTimeout)
	f.cached, f.err = m.ingest(ingestCtx)
	cancel()
	close(f.done)
	m.mu.Lock()
	m.flight = nil
	m.mu.Unlock()
	return f.cached, f.err
}

func (m *Mirror) ingest(ctx context.Context) (Cached, error) {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return Cached{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, m.artifact.URL, nil)
	if err != nil {
		return Cached{}, agentsoftware.ErrArtifactDownload
	}
	response, err := m.client.Do(request)
	if err != nil {
		return Cached{}, fmt.Errorf("%w: %v", agentsoftware.ErrArtifactDownload, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Cached{}, fmt.Errorf("%w: HTTP %d", agentsoftware.ErrArtifactDownload, response.StatusCode)
	}
	if response.ContentLength > agentsoftware.MaxArtifactBytes {
		return Cached{}, agentsoftware.ErrArtifactTooLarge
	}
	temp, err := os.CreateTemp(m.dir, ".opencode-*.tgz")
	if err != nil {
		return Cached{}, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	written, err := io.Copy(temp, io.LimitReader(response.Body, agentsoftware.MaxArtifactBytes+1))
	if err != nil {
		temp.Close()
		return Cached{}, fmt.Errorf("%w: %v", agentsoftware.ErrArtifactDownload, err)
	}
	if written > agentsoftware.MaxArtifactBytes {
		temp.Close()
		return Cached{}, agentsoftware.ErrArtifactTooLarge
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return Cached{}, err
	}
	if err := temp.Close(); err != nil {
		return Cached{}, err
	}
	if err := agentsoftware.CheckTarball(tempPath, m.artifact); err != nil {
		return Cached{}, err
	}
	if err := os.Rename(tempPath, m.Path()); err != nil {
		// Windows refuses to replace an existing file; only a corrupt cached
		// file can be there, because Status said it was absent.
		if removeErr := os.Remove(m.Path()); removeErr != nil {
			return Cached{}, err
		}
		if err := os.Rename(tempPath, m.Path()); err != nil {
			return Cached{}, err
		}
	}
	return m.Status(), nil
}

// GC removes every cached file that is not the reviewed digest, including
// staging files a stopped controller left behind. It is for startup.
func (m *Mirror) GC() error {
	entries, err := os.ReadDir(m.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	keep := m.Path()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(m.dir, entry.Name())
		if path == keep {
			continue
		}
		_ = os.Remove(path)
	}
	return nil
}
