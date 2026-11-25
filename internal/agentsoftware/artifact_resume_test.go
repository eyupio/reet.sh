package agentsoftware

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// resumeFixture is an archive big enough to cut in half, its artifact, and the files left in dir.
func resumeFixture(t *testing.T) (archive, binary []byte, artifact Artifact) {
	t.Helper()
	binary = bytes.Repeat([]byte("opencode binary "), 4096)
	archive = makeTarball(t, tarEntry{name: "package/package.json", data: []byte(`{}`)}, tarEntry{name: "package/bin/opencode", data: binary})
	artifact = artifactFor(archive, binary)
	artifact.ArchiveSize = int64(len(archive))
	return
}

func partials(t *testing.T, dir string) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join(dir, ".opencode-*.part"))
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// cutOff serves the first half of body and then drops the connection; later requests are served in full, with ranges.
func cutOff(t *testing.T, body []byte, requests *atomic.Int32, ranges *[]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		*ranges = append(*ranges, r.Header.Get("Range"))
		if n == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			_, _ = w.Write(body[:len(body)/2])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// A download that stops half-way is continued by the next attempt, which asks only for the rest.
func TestStageTarballContinuesAnInterruptedDownload(t *testing.T) {
	archive, binary, artifact := resumeFixture(t)
	var requests atomic.Int32
	var ranges []string
	server := cutOff(t, archive, &requests, &ranges)
	dir := t.TempDir()

	if _, err := StageTarballFrom(context.Background(), server.Client(), dir, artifact, server.URL, nil); !errors.Is(err, ErrArtifactDownload) {
		t.Fatalf("first attempt: %v", err)
	}
	kept := partials(t, dir)
	if len(kept) != 1 {
		t.Fatalf("the half that arrived was not kept: %v", kept)
	}
	if info, _ := os.Stat(kept[0]); info.Size() == 0 || info.Size() >= int64(len(archive)) {
		t.Fatalf("kept %d of %d bytes", info.Size(), len(archive))
	}
	staged, err := StageTarballFrom(context.Background(), server.Client(), dir, artifact, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged.Path)
	if len(ranges) != 2 || ranges[0] != "" || ranges[1] == "" {
		t.Fatalf("the second request did not ask for the rest: %q", ranges)
	}
	if data, _ := os.ReadFile(staged.Path); !bytes.Equal(data, binary) {
		t.Fatal("the staged binary differs")
	}
	if left := partials(t, dir); len(left) != 0 {
		t.Fatalf("a finished download left %v", left)
	}
}

// What one source delivered is finished by another: the registry stops half-way and a mirror supplies the rest.
func TestStageTarballFinishesFromAnotherSource(t *testing.T) {
	archive, binary, artifact := resumeFixture(t)
	var requests atomic.Int32
	var ranges []string
	registry := cutOff(t, archive, &requests, &ranges)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(archive))
	}))
	defer mirror.Close()
	dir := t.TempDir()

	if _, err := StageTarballFrom(context.Background(), registry.Client(), dir, artifact, registry.URL, nil); err == nil {
		t.Fatal("the cut-off download succeeded")
	}
	staged, err := StageTarballFrom(context.Background(), mirror.Client(), dir, artifact, mirror.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged.Path)
	if data, _ := os.ReadFile(staged.Path); !bytes.Equal(data, binary) {
		t.Fatal("the staged binary differs")
	}
}

// A server that ignores the range and sends everything again is not asked twice and does not corrupt the file.
func TestStageTarballRestartsWhenTheServerIgnoresTheRange(t *testing.T) {
	archive, binary, artifact := resumeFixture(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
			_, _ = w.Write(archive[:len(archive)/3])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
		_, _ = w.Write(archive) // no range support: always a whole 200
	}))
	defer server.Close()
	dir := t.TempDir()

	_, _ = StageTarballFrom(context.Background(), server.Client(), dir, artifact, server.URL, nil)
	staged, err := StageTarballFrom(context.Background(), server.Client(), dir, artifact, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged.Path)
	if data, _ := os.ReadFile(staged.Path); !bytes.Equal(data, binary) {
		t.Fatal("the staged binary differs")
	}
}

// A 206 that does not start where asked, or a refused range, starts the file again instead of splicing the wrong bytes.
func TestStageTarballStartsAgainOnAWrongRange(t *testing.T) {
	archive, binary, artifact := resumeFixture(t)
	for name, answer := range map[string]func(w http.ResponseWriter){
		"wrong start": func(w http.ResponseWriter) {
			w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(len(archive)-1)+"/"+strconv.Itoa(len(archive)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(archive)
		},
		"refused": func(w http.ResponseWriter) { w.WriteHeader(http.StatusRequestedRangeNotSatisfiable) },
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Header.Get("Range") == "" && requests.Add(1) == 1:
					w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
					_, _ = w.Write(archive[:len(archive)/2])
					w.(http.Flusher).Flush()
					panic(http.ErrAbortHandler)
				case r.Header.Get("Range") != "":
					answer(w)
				default:
					_, _ = w.Write(archive)
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			_, _ = StageTarballFrom(context.Background(), server.Client(), dir, artifact, server.URL, nil)
			staged, err := StageTarballFrom(context.Background(), server.Client(), dir, artifact, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(staged.Path)
			if data, _ := os.ReadFile(staged.Path); !bytes.Equal(data, binary) {
				t.Fatal("the staged binary differs")
			}
		})
	}
}

// Bytes that do not add up to the archive are deleted, not continued, and an old half is not trusted.
func TestStageTarballNeverContinuesABadOrStaleFile(t *testing.T) {
	archive, binary, artifact := resumeFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(archive))
	}))
	defer server.Close()

	t.Run("bad bytes", func(t *testing.T) {
		dir := t.TempDir()
		bad := bytes.Repeat([]byte{'x'}, len(archive)/2)
		if err := os.WriteFile(partialPath(dir, artifact), bad, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := StageTarballFrom(context.Background(), server.Client(), dir, artifact, server.URL, nil); !errors.Is(err, ErrArtifactChecksum) {
			t.Fatalf("a half made of wrong bytes: %v", err)
		}
		if left := partials(t, dir); len(left) != 0 {
			t.Fatalf("a bad file was kept: %v", left)
		}
		staged, err := StageTarballFrom(context.Background(), server.Client(), dir, artifact, server.URL, nil)
		if err != nil {
			t.Fatalf("the next attempt did not start clean: %v", err)
		}
		os.Remove(staged.Path)
	})
	t.Run("stale", func(t *testing.T) {
		dir := t.TempDir()
		path := partialPath(dir, artifact)
		if err := os.WriteFile(path, archive[:len(archive)/2], 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-48 * time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		staged, err := StageTarballFrom(context.Background(), server.Client(), dir, artifact, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(staged.Path)
		if data, _ := os.ReadFile(staged.Path); !bytes.Equal(data, binary) {
			t.Fatal("the staged binary differs")
		}
	})
}
