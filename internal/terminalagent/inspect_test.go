//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestInspectNegotiationAndLocalConsent(t *testing.T) {
	for _, tc := range []struct {
		name, features string
		denied         bool
	}{{"old", "", false}, {"denied", terminalproto.InspectFeature, true}, {"allowed", terminalproto.InspectFeature, false}} {
		t.Run(tc.name, func(t *testing.T) {
			handle, got, home := browseConnector(t, tc.features, func(opts *terminalhost.ManagerOptions) {
				if tc.denied {
					opts.BrowseConsent = func() error { return errors.New("off") }
				}
			})
			req := &terminalproto.Inspect{Dir: filepath.Join(home, "site"), View: "files"}
			err := handle(t.Context(), message("inspect-request-0001", req))
			if tc.features == "" {
				if !errors.Is(err, terminalproto.ErrInvalid) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			result := next(t, got, "inspect-request-0001").Body.(terminalproto.InspectResult)
			if tc.denied && result.Reason != "denied" {
				t.Fatal(result)
			}
			if !tc.denied && result.Reason != "" {
				t.Fatal(result)
			}
		})
	}
}

func TestGitHubInspectionDoesNotBlockLocalViews(t *testing.T) {
	original := inspectWorkspace
	started, release := make(chan struct{}), make(chan struct{})
	inspectWorkspace = func(ctx context.Context, home string, req terminalproto.Inspect) terminalproto.InspectResult {
		if req.View == "github" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		return terminalproto.InspectResult{CheckedAt: 1}
	}
	defer func() { inspectWorkspace = original }()
	handle, got, home := browseConnector(t, terminalproto.InspectFeature, nil)
	if err := handle(t.Context(), message("inspect-github-0001", &terminalproto.Inspect{Dir: home, View: "github"})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("GitHub lookup did not start")
	}
	if err := handle(t.Context(), message("inspect-local-00001", &terminalproto.Inspect{Dir: home, View: "status"})); err != nil {
		t.Fatal(err)
	}
	local := next(t, got, "inspect-local-00001").Body.(terminalproto.InspectResult)
	close(release)
	if local.Reason != "" {
		t.Fatalf("GitHub blocked local view: %+v", local)
	}
	next(t, got, "inspect-github-0001")
}
