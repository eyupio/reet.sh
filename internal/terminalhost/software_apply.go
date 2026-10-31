//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// softwareArtifact is the one archive this connector will install. Tests
	// replace it with a local archive; production never accepts another URL.
	softwareArtifact = agentsoftware.OpenCodeLinuxX64
	// softwareApplyTimeout bounds one download plus install. The controller
	// gives the request a little longer so a timeout can be reported.
	softwareApplyTimeout = 5 * time.Minute
)

// ApplySoftware downloads the reviewed artifact, verifies it, installs it under
// reet's own folder and reports the fresh evidence. It refuses when the
// observed installation changed since the person approved the plan, and it
// never touches an installation reet did not make.
func ApplySoftware(ctx context.Context, home string, req terminalproto.SoftwareApply, lookPath func(string) (string, error)) terminalproto.SoftwareApplyResult {
	result := terminalproto.SoftwareApplyResult{Tool: req.Tool, Action: req.Action}
	if req.Tool != agentsoftware.OpenCodeTool || req.Action != terminalproto.SoftwareActionInstall && req.Action != terminalproto.SoftwareActionUpdate {
		result.State, result.Reason = terminalproto.SoftwareApplyRefused, terminalproto.SoftwareApplyUnsupported
		return result
	}
	fresh := InspectSoftware(home, req.Tool, lookPath)
	if fresh.State != req.ExpectedState || fresh.Managed != req.ExpectedManaged ||
		req.Action == terminalproto.SoftwareActionUpdate && fresh.Version != req.ExpectedVersion {
		result.State, result.Reason = terminalproto.SoftwareApplyRefused, terminalproto.SoftwareApplyChanged
		return result
	}

	dir := filepath.Join(home, ".reet", "software", req.Tool, "tmp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		result.State, result.Reason = terminalproto.SoftwareApplyFailed, terminalproto.SoftwareApplyInstall
		return result
	}
	defer os.RemoveAll(dir)
	runCtx, cancel := context.WithTimeout(ctx, softwareApplyTimeout)
	defer cancel()
	staged, err := agentsoftware.StageTarball(runCtx, nil, dir, softwareArtifact)
	if err != nil {
		result.State, result.Reason = terminalproto.SoftwareApplyFailed, softwareFailure(err)
		return result
	}
	defer os.Remove(staged.Path)
	if runCtx.Err() != nil {
		result.State, result.Reason = terminalproto.SoftwareApplyFailed, terminalproto.SoftwareApplyTimedOut
		return result
	}
	receipt, err := agentsoftware.InstallStaged(home, req.Tool, agentsoftware.OpenCodeReviewedVersion, staged)
	if err != nil {
		result.State, result.Reason = terminalproto.SoftwareApplyFailed, terminalproto.SoftwareApplyInstall
		return result
	}
	_, current, found, err := agentsoftware.Current(home, req.Tool)
	if err != nil || !found || current.Version != agentsoftware.OpenCodeReviewedVersion {
		// A failed verification must not leave the failed version current when
		// an earlier managed version can be restored.
		_, _ = agentsoftware.Rollback(home, req.Tool)
		result.State, result.Reason = terminalproto.SoftwareApplyFailed, terminalproto.SoftwareApplyInstall
		return result
	}
	entry := InspectSoftware(home, req.Tool, lookPath)
	if entry.State != terminalproto.SoftwareInstalled || !entry.Managed || entry.Version != receipt.Version {
		result.State, result.Reason = terminalproto.SoftwareApplyFailed, terminalproto.SoftwareApplyInstall
		return result
	}
	result.State, result.Version, result.Previous, result.Entry = terminalproto.SoftwareApplyApplied, receipt.Version, receipt.Previous, &entry
	return result
}

func softwareFailure(err error) terminalproto.SoftwareApplyReason {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return terminalproto.SoftwareApplyTimedOut
	case errors.Is(err, agentsoftware.ErrArtifactDownload):
		return terminalproto.SoftwareApplyDownload
	case errors.Is(err, agentsoftware.ErrArtifactChecksum):
		return terminalproto.SoftwareApplyChecksum
	default:
		return terminalproto.SoftwareApplyInstall
	}
}
