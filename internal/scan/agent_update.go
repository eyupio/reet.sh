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
	"github.com/eyupio/reet.sh/internal/store"
)

// updateAgentBinary downloads and verifies a controller-requested release,
// keeps the current executable as <binary>.previous, and returns whether the
// running process should re-exec into it. The controller never supplies binary
// bytes; the agent resolves and verifies the published release itself.
func updateAgentBinary(ctx context.Context, config, jobID, requested, current string, report func(stage, detail string)) (string, string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "latest"
	}
	report(store.UpdateStageDownload, "Resolving the "+channelName(requested)+" channel")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Minute}
	resolved, err := lifecycle.ResolveRelease(ctx, client, requested)
	if err != nil {
		return "", "", err
	}
	report(store.UpdateStageDownload, "Downloading "+resolved)
	stage, err := os.MkdirTemp("", "reet-agent-update-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(stage)
	candidate, err := lifecycle.FetchReleaseBinary(ctx, client, lifecycle.ReleaseDownloads, resolved, stage)
	if err != nil {
		return "", "", err
	}
	report(store.UpdateStageVerify, "Checking the downloaded build runs on this host")
	build, err := exec.CommandContext(ctx, candidate, "--version").Output()
	if err != nil {
		return "", "", errors.New("verified release binary cannot run on this host")
	}
	version, commit, ok := parseAgentBuild(string(build))
	if !ok {
		return "", "", fmt.Errorf("release binary returned an unreadable version: %q", strings.TrimSpace(string(build)))
	}
	if !strings.EqualFold(strings.TrimSpace(version), strings.TrimSpace(resolved)) {
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
		return fmt.Sprintf("Already running %s.", describeBuild(version, commit)), "", nil
	}
	previous := target + ".previous"
	report(store.UpdateStagePreserve, "Keeping the current binary as "+filepath.Base(previous))
	if err = lifecycle.ReplaceBinary(target, previous); err != nil {
		return "", "", fmt.Errorf("could not preserve the current binary: %w", err)
	}
	pending := pendingUpdate{
		JobID: jobID, From: current, FromCommit: buildCommit,
		To: version, Commit: commit, Target: target, Previous: previous,
		Started: time.Now().UnixMilli(),
	}
	if err = savePendingUpdate(config, pending); err != nil {
		return "", "", fmt.Errorf("could not record the pending update: %w", err)
	}
	report(store.UpdateStageReplace, "Installing "+describeBuild(version, commit))
	if err = lifecycle.ReplaceBinary(candidate, target); err != nil {
		clearPendingUpdate(config)
		return "", "", fmt.Errorf("could not replace the current binary: %w", err)
	}
	report(store.UpdateStageRestart, "Restarting into "+describeBuild(version, commit))
	message := fmt.Sprintf("Updated from %s to %s. Restarting the agent.", describeBuild(current, buildCommit), describeBuild(version, commit))
	if strings.TrimSpace(current) == "" || strings.EqualFold(current, "dev") {
		message = fmt.Sprintf("Updated to %s. Restarting the agent.", describeBuild(version, commit))
	}
	return message, target, nil
}

func channelName(requested string) string {
	if strings.EqualFold(strings.TrimSpace(requested), "dev") {
		return "development"
	}
	return "release"
}

func reportAgentStage(c agentConfig, jobID, stage, detail, version string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = agentRequestContext(ctx, c, "/api/v2/sync/agents/update/stage", map[string]string{"id": jobID, "stage": stage, "detail": detail}, nil, version)
}

// checkPendingUpdate runs on the first cycle after the agent re-execs into a
// new build. It proves the new build can reach the controller and, when it
// cannot, puts the previous binary back and reports the rollback.
func checkPendingUpdate(c agentConfig, config, version string) (bool, error) {
	pending, ok := loadPendingUpdate(config)
	if !ok {
		return false, nil
	}
	if !buildMatches(pending, version, buildCommit) {
		// The replacement did not take, or an old process came back. Leave the
		// binary as it is and tell the controller rather than retrying blindly.
		result := agentResult{ID: pending.JobID, Status: "failed", Message: fmt.Sprintf("The update to %s did not take; still running %s. Try again from the controller.", describeBuild(pending.To, pending.Commit), describeBuild(version, buildCommit))}
		clearPendingUpdate(config)
		return true, writePrivate(config+".result", result)
	}
	reportAgentStage(c, pending.JobID, store.UpdateStageCheck, "Waiting for the controller to confirm "+describeBuild(version, buildCommit), version)
	var agent store.SyncAgent
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := agentRequestContext(ctx, c, "/api/v2/sync/identity", map[string]string{}, &agent, version); err != nil {
		pending.Attempts++
		_ = savePendingUpdate(config, pending)
		if !shouldRollback(err, pending.Started, time.Now()) {
			return true, nil
		}
		reason := fmt.Sprintf("The new %s build could not reach the controller; restored %s.", describeBuild(pending.To, pending.Commit), describeBuild(pending.From, pending.FromCommit))
		if rollbackErr := rollbackPendingUpdate(config, pending, reason); rollbackErr != nil {
			// Do not wedge the agent on a marker it cannot act on: report what
			// happened, forget the marker, and keep the new build running.
			clearPendingUpdate(config)
			_ = writePrivate(config+".result", agentResult{ID: pending.JobID, Status: "failed", Message: fmt.Sprintf("The new build is running but the previous binary could not be restored: %v", rollbackErr)})
			return true, nil
		}
		return true, nil
	}
	result := agentResult{ID: pending.JobID, Status: "succeeded", Message: fmt.Sprintf("Updated from %s to %s. The agent is reporting again.", describeBuild(pending.From, pending.FromCommit), describeBuild(version, buildCommit))}
	if strings.TrimSpace(pending.From) == "" || strings.EqualFold(pending.From, "dev") {
		result.Message = fmt.Sprintf("Updated to %s. The agent is reporting again.", describeBuild(version, buildCommit))
	}
	if err := writePrivate(config+".result", result); err != nil {
		return true, err
	}
	clearPendingUpdate(config)
	return true, nil
}

func rollbackPendingUpdate(config string, pending pendingUpdate, reason string) error {
	if err := lifecycle.ReplaceBinary(pending.Previous, pending.Target); err != nil {
		return fmt.Errorf("could not restore the previous binary: %w", err)
	}
	if err := writePrivate(config+".result", agentResult{ID: pending.JobID, Status: "rolled_back", Message: reason}); err != nil {
		return err
	}
	clearPendingUpdate(config)
	return restartAgent(pending.Target)
}

func restartAgent(path string) error {
	return syscall.Exec(path, os.Args, os.Environ())
}
