package scan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillpreview"
	"github.com/eyupio/reet.sh/internal/store"
)

const (
	contentPreviewVersion = 1
	// More enrolled projects than this cannot be checked for aliases, so a root
	// is then reported unknown rather than exclusive.
	maxPreviewProjects = 256
)

var contentPreviewOnce sync.Once
var contentPreviewNonce string

// contentPreviewInstance identifies this process for the controller's fences. It
// is a generation marker, not a credential.
func contentPreviewInstance() string {
	contentPreviewOnce.Do(func() {
		var raw [24]byte
		if _, err := rand.Read(raw[:]); err == nil {
			contentPreviewNonce = hex.EncodeToString(raw[:])
		}
	})
	return contentPreviewNonce
}

// previewRoots maps a reported scope onto this installation's own configured
// roots, and lists every other configured scope root so that aliases of the same
// directory can be found. The controller supplies a scope name, never a path.
// Scan-only agents (Codex) are not writable scopes and are refused.
func previewRoots(c agentConfig, agent, project string) (skillpreview.Roots, error) {
	var roots skillpreview.Roots
	if agent == "codex" {
		return roots, errors.New("this agent scope is scan-only")
	}
	root, canonical, err := skillScopeRoot(c, agent, project)
	if err != nil {
		return roots, err
	}
	roots = skillpreview.Roots{Target: root, OthersComplete: len(c.Projects) <= maxPreviewProjects}
	for _, d := range homeDirs {
		if canonical == "" && d.Agent == agent {
			continue
		}
		roots.Others = append(roots.Others, filepath.Join(c.Home, d.Dir))
	}
	for i, p := range c.Projects {
		if i >= maxPreviewProjects {
			break
		}
		for _, d := range projectDirs {
			if canonical != "" && d.Agent == agent && sameDirectory(p, canonical) {
				continue
			}
			roots.Others = append(roots.Others, filepath.Join(p, d.Dir))
		}
	}
	return roots, nil
}

// processContentPreview serves at most one metadata-only preview request. Asking
// for work is the negotiation: nothing on disk is read until the controller has
// answered with a request for this very process. An older controller (404/405),
// one that forbids the channel (403) or a newer schema leaves files untouched.
func processContentPreview(ctx context.Context, c agentConfig, version string) (bool, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return false, nil
	}
	return processContentPreviewWith(ctx, c, version, contentPreviewInstance())
}

func processContentPreviewWith(ctx context.Context, c agentConfig, version, instance string) (bool, error) {
	return processContentPreviewUsing(ctx, c, version, instance, skillpreview.Observe)
}

func processContentPreviewUsing(ctx context.Context, c agentConfig, version, instance string, observe func(skillpreview.Roots, string) skillpreview.Capture) (bool, error) {
	if !contentobs.ValidID(instance) {
		return false, errors.New("cannot create content preview instance")
	}
	const route = "/api/v2/sync/skill-content-preview/"
	var response struct {
		SchemaVersion int                            `json:"schemaVersion"`
		Work          *store.SkillContentPreviewWork `json:"work"`
	}
	err := controllerPost(ctx, c, route+"work", map[string]any{"schemaVersion": contentPreviewVersion, "instanceId": instance}, &response, version, 4096)
	// 404/405: an older controller. 403: the channel is not offered here. 409: not
	// ready (paused, or a machine the controller cannot match yet). Each is a normal
	// state, not an error to repeat every cycle, and none reads any file.
	if err == controllerHTTPError(404) || err == controllerHTTPError(405) || err == controllerHTTPError(403) || err == controllerHTTPError(409) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if response.SchemaVersion != contentPreviewVersion || response.Work == nil {
		return false, nil
	}
	work := response.Work
	if work.Instance != instance {
		return true, nil
	}
	failure := func(code string) (bool, error) {
		var ack any
		err := controllerPost(ctx, c, route+"failure", map[string]string{"id": work.ID, "instanceId": instance, "code": code}, &ack, version, 4096)
		if err == controllerHTTPError(409) || err == controllerHTTPError(404) {
			err = nil
		}
		return true, err
	}
	if !skillpreview.ValidName(work.Name) {
		return failure("scope_unavailable")
	}
	roots, err := previewRoots(c, work.Agent, work.Project)
	if err != nil {
		return failure("scope_unavailable")
	}
	capture := observe(roots, work.Name)
	// A stopping connector must not report something it was told to abandon.
	if err = ctx.Err(); err != nil {
		return true, err
	}
	if capture.Validate() != nil {
		return failure("capture_failed")
	}
	var ack any
	err = controllerPost(ctx, c, route+"report", map[string]any{"id": work.ID, "instanceId": instance, "capture": capture}, &ack, version, 1<<20)
	// 409/404: cancelled, superseded or already answered. Nothing to retry; the
	// controller decides whether the observation still describes the present.
	if err == controllerHTTPError(409) || err == controllerHTTPError(404) {
		err = nil
	}
	return true, err
}
