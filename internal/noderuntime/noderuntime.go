// Package noderuntime provisions the Node.js runtime that the skills CLI needs,
// without asking the person to install an operating-system package.
package noderuntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Version is the reviewed Node.js build reet installs. Update this table and
// the checksums together from https://nodejs.org/dist/<version>/SHASUMS256.txt.
const Version = "v26.13.0"

// MaxTarballBytes bounds the downloaded archive. The reviewed build is about
// 60 MiB; the ceiling leaves room without accepting an unbounded response.
const MaxTarballBytes = 256 << 20

var (
	ErrUnsupported = errors.New("reet has no reviewed Node.js build for this platform")
	ErrChecksum    = errors.New("the Node.js archive did not match its recorded checksum")
	ErrShape       = errors.New("the Node.js archive did not contain a usable runtime")
)

type artifact struct {
	URL    string
	SHA256 string
}

// Platform builds are keyed by GOOS/GOARCH. Node names its platforms x64 and
// arm64; reet only ships binaries for the platforms below.
var artifacts = map[string]artifact{
	"linux/amd64": {
		URL:    "https://nodejs.org/dist/" + Version + "/node-" + Version + "-linux-x64.tar.gz",
		SHA256: "5848553868b402d3fabb5b09270649977780a390f832155721025a08b5104455",
	},
	"linux/arm64": {
		URL:    "https://nodejs.org/dist/" + Version + "/node-" + Version + "-linux-arm64.tar.gz",
		SHA256: "439e9c438acdf6a6ffe47418da709153d66c4bed812e30a473faacb107dd2791",
	},
	"darwin/amd64": {
		URL:    "https://nodejs.org/dist/" + Version + "/node-" + Version + "-darwin-x64.tar.gz",
		SHA256: "525c9b29bb0263d687de60a4cec0e2a6db876a8eb032369e5795eca71e7c358e",
	},
	"darwin/arm64": {
		URL:    "https://nodejs.org/dist/" + Version + "/node-" + Version + "-darwin-arm64.tar.gz",
		SHA256: "d38bd5e5a2ddce6bbb5ce45e46e208337da4d51fffe611e567fd32073d6439f6",
	},
}

// ArtifactFor returns the reviewed build for a platform.
func ArtifactFor(goos, goarch string) (artifact, bool) {
	a, ok := artifacts[goos+"/"+goarch]
	return a, ok
}

// Dir is where reet keeps the managed runtime for this version.
func Dir(home string) string {
	return filepath.Join(home, ".reet", "tools", "node", Version)
}

// MinimumMajor is the oldest Node.js the skills CLI is expected to run on.
const MinimumMajor = 18

// Major reports the major version of the node on PATH.
func Major() (int, bool) {
	out, err := exec.Command("node", "--version").Output()
	if err != nil {
		return 0, false
	}
	value := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	major := strings.SplitN(value, ".", 2)[0]
	number, err := strconv.Atoi(major)
	if err != nil {
		return 0, false
	}
	return number, true
}

// Managed returns the managed npx for this version when it is complete.
func Managed(home string) (string, bool) {
	npx := filepath.Join(Dir(home), "bin", "npx")
	npxInfo, err := os.Stat(npx)
	if err != nil || npxInfo.IsDir() {
		return "", false
	}
	nodeInfo, err := os.Stat(filepath.Join(Dir(home), "bin", "node"))
	if err != nil || nodeInfo.IsDir() {
		return "", false
	}
	return npx, true
}

// Lookup finds an npx to use: one already on PATH first, then reet's managed
// runtime. The returned bool says whether it came from the managed runtime.
func Lookup(home string) (string, bool) {
	if path, err := exec.LookPath("npx"); err == nil {
		if _, nodeErr := exec.LookPath("node"); nodeErr == nil {
			if major, ok := Major(); !ok || major >= MinimumMajor {
				return path, false
			}
		}
	}
	return Managed(home)
}

// Use makes an npx path reachable by later commands in this process.
func Use(npxPath string) {
	dir := filepath.Dir(npxPath)
	path := os.Getenv("PATH")
	if path == "" {
		os.Setenv("PATH", dir)
		return
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+path)
}

// Install downloads the reviewed runtime, verifies its checksum, extracts it
// under the managed directory and returns its npx. progress, when set, receives
// short status lines. A complete installation is reused without downloading.
func Install(ctx context.Context, client *http.Client, home string, progress io.Writer) (string, error) {
	if npx, ok := Managed(home); ok {
		return npx, nil
	}
	build, ok := ArtifactFor(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return "", fmt.Errorf("%w (%s/%s)", ErrUnsupported, runtime.GOOS, runtime.GOARCH)
	}
	dir := Dir(home)
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	if _, err := os.Lstat(dir); err == nil {
		return "", fmt.Errorf("managed Node.js directory %s exists but is incomplete; remove it and sync again", dir)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if progress != nil {
		fmt.Fprintf(progress, "Downloading Node.js %s from nodejs.org...\n", Version)
	}
	staging, err := os.MkdirTemp(parent, ".node-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	archive := filepath.Join(staging, "node.tar.gz")
	if err := download(ctx, client, build, archive); err != nil {
		return "", err
	}
	if progress != nil {
		fmt.Fprintln(progress, "Verifying and extracting the runtime...")
	}
	tree := filepath.Join(staging, "tree")
	if err := extract(archive, tree); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(tree, "bin", "npx")); err != nil {
		return "", fmt.Errorf("%w: no bin/npx", ErrShape)
	}
	if err := os.Rename(tree, dir); err != nil {
		return "", err
	}
	npx := filepath.Join(dir, "bin", "npx")
	if _, err := os.Stat(npx); err != nil {
		return "", fmt.Errorf("%w: no bin/npx", ErrShape)
	}
	return npx, nil
}

func download(ctx context.Context, client *http.Client, build artifact, destination string) error {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, build.URL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Node.js download: nodejs.org replied %s", response.Status)
	}
	if response.ContentLength > MaxTarballBytes {
		return errors.New("the Node.js archive was larger than allowed")
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, MaxTarballBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > MaxTarballBytes {
		return errors.New("the Node.js archive was larger than allowed")
	}
	if hex.EncodeToString(hash.Sum(nil)) != build.SHA256 {
		return ErrChecksum
	}
	return nil
}
