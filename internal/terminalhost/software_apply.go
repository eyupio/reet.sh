//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// softwareArtifact is the one archive this connector will install. Tests
	// replace it with a local archive; production never accepts another URL.
	softwareArtifact = agentsoftware.OpenCodeLinuxX64
	// softwareApplyTimeout bounds one download plus install, including a slow
	// fall back to the controller's mirror. The controller gives the request a
	// little longer so a timeout can be reported.
	softwareApplyTimeout  = 20 * time.Minute
	softwareRegistryLimit = 5 * time.Minute
	softwareMirrorLimit   = 15 * time.Minute
)

// SoftwareSource is the controller's artifact mirror as this connector may use
// it. Path is derived from the connector's own pinned table, never accepted as
// a URL from the controller; the private client carries the enrollment.
type SoftwareSource struct {
	Client *http.Client
	// BaseURL is the connector's private origin; empty means the built-in one.
	BaseURL string
	// Path is derived from the connector's pinned digest.
	Path string
}

// ApplySoftware downloads the reviewed artifact, verifies it, installs it under
// reet's own folder and reports the fresh evidence. Rollback and remove act
// only on a reet-managed copy. It refuses when the observed installation
// changed since the person approved the plan.
func ApplySoftware(ctx context.Context, home string, req terminalproto.SoftwareApply, lookPath func(string) (string, error), sources ...SoftwareSource) terminalproto.SoftwareApplyResult {
	result := terminalproto.SoftwareApplyResult{Tool: req.Tool, Action: req.Action}
	if req.Tool != agentsoftware.OpenCodeTool || runtime.GOOS != agentsoftware.SupportedPlatform || runtime.GOARCH != agentsoftware.SupportedArch {
		result.State, result.Reason = terminalproto.SoftwareApplyRefused, terminalproto.SoftwareApplyUnsupported
		return result
	}
	switch req.Action {
	case terminalproto.SoftwareActionInstall, terminalproto.SoftwareActionUpdate,
		terminalproto.SoftwareActionRollback, terminalproto.SoftwareActionRemove:
	default:
		result.State, result.Reason = terminalproto.SoftwareApplyRefused, terminalproto.SoftwareApplyUnsupported
		return result
	}
	fresh := InspectSoftware(home, req.Tool, lookPath)
	if fresh.State != req.ExpectedState || fresh.Managed != req.ExpectedManaged {
		result.State, result.Reason = terminalproto.SoftwareApplyRefused, terminalproto.SoftwareApplyChanged
		return result
	}
	switch req.Action {
	case terminalproto.SoftwareActionUpdate, terminalproto.SoftwareActionRemove:
		if fresh.Version != req.ExpectedVersion {
			result.State, result.Reason = terminalproto.SoftwareApplyRefused, terminalproto.SoftwareApplyChanged
			return result
		}
	case terminalproto.SoftwareActionRollback:
		if fresh.Version != req.ExpectedVersion || fresh.Previous != req.ExpectedPrevious {
			result.State, result.Reason = terminalproto.SoftwareApplyRefused, terminalproto.SoftwareApplyChanged
			return result
		}
	}

	switch req.Action {
	case terminalproto.SoftwareActionRollback:
		receipt, err := agentsoftware.RollbackManaged(home, req.Tool)
		if errors.Is(err, agentsoftware.ErrNoRollback) {
			result.State, result.Reason = terminalproto.SoftwareApplyRefused, terminalproto.SoftwareApplyNoRollback
			return result
		}
		if err != nil || receipt.Version != req.ExpectedPrevious {
			result.State, result.Reason = terminalproto.SoftwareApplyFailed, terminalproto.SoftwareApplyRollback
			return result
		}
		entry := InspectSoftware(home, req.Tool, lookPath)
		if entry.State != terminalproto.SoftwareInstalled || !entry.Managed || entry.Version != req.ExpectedPrevious {
			result.State, result.Reason = terminalproto.SoftwareApplyFailed, terminalproto.SoftwareApplyRollback
			return result
		}
		result.State, result.Version, result.Previous, result.Entry = terminalproto.SoftwareApplyApplied, entry.Version, req.ExpectedVersion, &entry
		return result
	case terminalproto.SoftwareActionRemove:
		if err := agentsoftware.RemoveManaged(home, req.Tool); err != nil {
			result.State, result.Reason = terminalproto.SoftwareApplyFailed, terminalproto.SoftwareApplyRemoveFailed
			return result
		}
		entry := InspectSoftware(home, req.Tool, lookPath)
		if entry.State == terminalproto.SoftwareInstalled && entry.Managed {
			result.State, result.Reason = terminalproto.SoftwareApplyFailed, terminalproto.SoftwareApplyRemoveFailed
			return result
		}
		result.State, result.Entry = terminalproto.SoftwareApplyApplied, &entry
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
	staged, source, err := stageSoftware(runCtx, dir, sources)
	if err != nil {
		result.Source = source
		result.State, result.Reason = terminalproto.SoftwareApplyFailed, softwareFailure(err)
		return result
	}
	result.Source = source
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
		_, _ = agentsoftware.RollbackManaged(home, req.Tool)
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

// stageSoftware tries the registry first and the controller's mirror only when
// the registry download fails. Both verify the same pinned artifact; the source
// is reported so the controller can say what was tried.
func stageSoftware(ctx context.Context, dir string, sources []SoftwareSource) (agentsoftware.Staged, string, error) {
	registryCtx, cancel := context.WithTimeout(ctx, softwareRegistryLimit)
	staged, err := agentsoftware.StageTarball(registryCtx, nil, dir, softwareArtifact)
	cancel()
	if err == nil {
		return staged, "registry", nil
	}
	mirrorTried := false
	for _, source := range sources {
		if source.Client == nil || source.Path == "" {
			continue
		}
		mirrorTried = true
		mirrorCtx, cancel := context.WithTimeout(ctx, softwareMirrorLimit)
		base := source.BaseURL
		if base == "" {
			base = privateconn.Origin
		}
		staged, mirrorErr := agentsoftware.StageTarballFrom(mirrorCtx, source.Client, dir, softwareArtifact, strings.TrimRight(base, "/")+source.Path)
		cancel()
		if mirrorErr == nil {
			return staged, "mirror", nil
		}
	}
	if mirrorTried {
		return agentsoftware.Staged{}, "registry+mirror", err
	}
	return agentsoftware.Staged{}, "registry", err
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
