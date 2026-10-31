// Package lifecycle manages explicitly installed reet deployments.
package lifecycle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const releaseAPI = "https://api.github.com/repos/eyupio/reet.sh/releases"

// ReleaseDownloads is the public asset root used by the installed agent.
const ReleaseDownloads = "https://github.com/eyupio/reet.sh/releases/download"

const imageRepo = "ghcr.io/eyupio/reet.sh"

var tagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type release struct {
	Tag string `json:"tag_name"`
}

func normaliseTag(tag string) (string, error) {
	if tag == "latest" || tag == "dev" {
		return tag, nil
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	if !tagPattern.MatchString(tag) {
		return "", fmt.Errorf("version must be latest, dev or a release tag such as v1.2.3")
	}
	return tag, nil
}
func get(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed; check connectivity and retry")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, releaseHTTPError{resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("download exceeds size limit")
	}
	return b, nil
}
func resolveRelease(ctx context.Context, client *http.Client, api, tag string) (string, error) {
	tag, err := normaliseTag(tag)
	if err != nil {
		return "", err
	}
	// The dev channel name is its own tag. Its release lists every build ever
	// published, so the API response grows without bound and must not be needed.
	if tag == "dev" {
		return tag, nil
	}
	endpoint := api + "/tags/" + tag
	if tag == "latest" {
		endpoint = api + "/latest"
	}
	b, err := get(ctx, client, endpoint, 1<<20)
	if err != nil {
		return "", err
	}
	var r release
	if err = json.Unmarshal(b, &r); err != nil {
		return "", fmt.Errorf("invalid release response")
	}
	parsed, err := normaliseTag(r.Tag)
	if err != nil || parsed != r.Tag || r.Tag == "latest" {
		return "", fmt.Errorf("invalid published release tag")
	}
	if tag != "latest" && tag != r.Tag {
		return "", fmt.Errorf("release tag did not match request")
	}
	return r.Tag, nil
}

// fetchBinary verifies the whole archive before inspecting any executable bytes.
func fetchBinary(ctx context.Context, client *http.Client, root, tag, dir string) (string, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return "", fmt.Errorf("unsupported operating system")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return "", fmt.Errorf("unsupported architecture")
	}
	name := fmt.Sprintf("reet_%s_%s_%s.tar.gz", tag, runtime.GOOS, runtime.GOARCH)
	manifestName := "checksums.txt"
	if tag == "dev" {
		pointer, err := get(ctx, client, root+"/dev/dev-build.txt", 128)
		var status releaseHTTPError
		if err != nil && !(errors.As(err, &status) && status.Status == http.StatusNotFound) {
			return "", err
		}
		if err == nil {
			sha := strings.TrimSpace(string(pointer))
			if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(sha) {
				return "", fmt.Errorf("invalid dev build pointer")
			}
			name = fmt.Sprintf("reet_dev_%s_%s_%s.tar.gz", sha, runtime.GOOS, runtime.GOARCH)
			manifestName = "checksums-" + sha + ".txt"
		}
	}
	manifest, err := get(ctx, client, root+"/"+tag+"/"+manifestName, 1<<20)
	if err != nil {
		return "", err
	}
	checksum := ""
	matches := 0
	for _, line := range strings.Split(string(manifest), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			checksum = f[0]
			matches++
		}
	}
	decoded, err := hex.DecodeString(checksum)
	if err != nil || len(decoded) != sha256.Size || matches != 1 {
		return "", fmt.Errorf("release must have exactly one valid checksum for %s", name)
	}
	archive, err := get(ctx, client, root+"/"+tag+"/"+name, 128<<20)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(archive)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), checksum) {
		return "", fmt.Errorf("checksum mismatch; release may be publishing, retry later (installed binary unchanged)")
	}
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", err
	}
	defer reader.Close()
	tr := tar.NewReader(reader)
	var executable []byte
	seen := false
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
		if h.Name != "reet" {
			continue
		}
		if seen || h.Typeflag != tar.TypeReg || h.Size <= 0 || h.Size > 128<<20 {
			return "", fmt.Errorf("invalid executable in release archive")
		}
		seen = true
		executable, e = io.ReadAll(io.LimitReader(tr, 128<<20+1))
		if e != nil {
			return "", e
		}
	}
	if !seen {
		return "", fmt.Errorf("release archive has no reet binary")
	}
	f, err := os.CreateTemp(dir, ".reet-download-*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(path)
		}
	}()
	if _, err = f.Write(executable); err != nil {
		return "", err
	}
	if err = f.Chmod(0700); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	return path, nil
}

// ResolveRelease resolves latest, dev or a pinned tag to the published tag.
func ResolveRelease(ctx context.Context, client *http.Client, tag string) (string, error) {
	return resolveRelease(ctx, client, releaseAPI, tag)
}

// FetchReleaseBinary downloads and verifies a release archive, returning a
// staged executable path inside dir.
func FetchReleaseBinary(ctx context.Context, client *http.Client, root, tag, dir string) (string, error) {
	return fetchBinary(ctx, client, root, tag, dir)
}

// ReplaceBinary atomically copies source over target.
func ReplaceBinary(source, target string) error { return replaceBinary(source, target) }

// SameBinary reports whether two files have identical bytes.
func SameBinary(a, b string) bool { return sameBinary(a, b) }

func sameBinary(a, b string) bool {
	x, e := os.ReadFile(a)
	if e != nil {
		return false
	}
	y, e := os.ReadFile(b)
	return e == nil && sha256.Sum256(x) == sha256.Sum256(y)
}
func replaceBinary(source, target string) error {
	f, err := os.CreateTemp(filepath.Dir(target), ".reet-install-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	input, err := os.Open(source)
	if err != nil {
		f.Close()
		return err
	}
	defer input.Close()
	_, err = io.Copy(f, input)
	if err == nil {
		err = f.Chmod(0755)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, target)
}
func defaultClient() *http.Client { return &http.Client{Timeout: 2 * time.Minute} }

type releaseHTTPError struct{ Status int }

func (e releaseHTTPError) Error() string { return fmt.Sprintf("download returned HTTP %d", e.Status) }
