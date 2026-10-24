//go:build linux || darwin

package scan

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/eyupio/reet.sh/internal/lifecycle"
)

// updateAgentBinary downloads and verifies a controller-requested release,
// keeps the current executable as <binary>.previous, and returns whether the
// running process should re-exec into it. The controller never supplies binary
// bytes; the agent resolves and verifies the published release itself.
func updateAgentBinary(ctx context.Context, requested, current string) (string, string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "latest"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Minute}
	resolved, err := lifecycle.ResolveRelease(ctx, client, requested)
	if err != nil {
		return "", "", err
	}
	stage, err := os.MkdirTemp("", "reet-agent-update-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(stage)
	candidate, err := lifecycle.FetchReleaseBinary(ctx, client, lifecycle.ReleaseDownloads, resolved, stage)
	if err != nil {
		return "", "", err
	}
	build, err := exec.CommandContext(ctx, candidate, "--version").Output()
	if err != nil {
		return "", "", errors.New("verified release binary cannot run on this host")
	}
	fields := strings.Fields(string(build))
	expected := strings.TrimPrefix(resolved, "v")
	if len(fields) != 3 || fields[0] != "reet" || strings.TrimPrefix(fields[1], "v") != expected {
		return "", "", fmt.Errorf("release binary version %q does not match %s", strings.TrimSpace(string(build)), resolved)
	}
	target, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", err
	}
	if lifecycle.SameBinary(candidate, target) {
		return fmt.Sprintf("Already running %s.", fields[1]), "", nil
	}
	if err = lifecycle.ReplaceBinary(target, target+".previous"); err != nil {
		return "", "", fmt.Errorf("could not preserve the current binary: %w", err)
	}
	if err = lifecycle.ReplaceBinary(candidate, target); err != nil {
		return "", "", fmt.Errorf("could not replace the current binary: %w", err)
	}
	current = strings.TrimSpace(current)
	message := fmt.Sprintf("Updated from %s to %s. Restarting the agent.", current, fields[1])
	if current == "" || current == "dev" {
		message = fmt.Sprintf("Updated to %s. Restarting the agent.", fields[1])
	}
	return message, target, nil
}

func restartAgent(path string) error {
	return syscall.Exec(path, os.Args, os.Environ())
}
