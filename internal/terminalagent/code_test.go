//go:build linux || darwin

package terminalagent

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestLaunchUnavailableIsLaunchFailed(t *testing.T) {
	if code(fmt.Errorf("folder: %w", terminalhost.ErrLaunchUnavailable)) != terminalproto.LaunchFailed {
		t.Fatal("a refused launch must be launch_failed")
	}
	if code(terminalhost.ErrSessionCapacity) != terminalproto.Capacity || code(terminalhost.ErrNotRunning) != terminalproto.Unavailable {
		t.Fatal("other mappings changed")
	}
}

func TestLaunchFieldsRequireNegotiatedFeature(t *testing.T) {
	// The manager cannot start here, so anything that passes the launch guard
	// is answered Unavailable; the guard itself answers launch_failed.
	for _, tc := range []struct {
		features string
		in       terminalproto.Create
		want     terminalproto.ResultCode
	}{
		{"", terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "claude"}, terminalproto.LaunchFailed},
		{terminalproto.CommandHistoryFeature, terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "claude"}, terminalproto.LaunchFailed},
		{terminalproto.LaunchFeature, terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "claude"}, terminalproto.Unavailable},
		{"", terminalproto.Create{Title: "a", Cols: 80, Rows: 24}, terminalproto.Unavailable},
	} {
		var got terminalproto.Message
		send := func(_ context.Context, m terminalproto.Message) error { got = m; return nil }
		headers := http.Header{}
		if tc.features != "" {
			headers.Set(terminalproto.FeatureHeader, tc.features)
		}
		handle, cleanup := Factory(terminalhost.ManagerOptions{})(t.Context(), http.DefaultClient, headers, 1, send)
		in := tc.in
		if err := handle(t.Context(), terminalproto.Message{Version: 1, Generation: 1, RequestID: "r", SessionID: "s", Body: &in}); err != nil {
			t.Fatal(err)
		}
		cleanup()
		if r, ok := got.Body.(terminalproto.Result); !ok || r.Code != tc.want {
			t.Fatal(tc.features, tc.in.Tool, got.Body)
		}
	}
}
