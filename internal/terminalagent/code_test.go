//go:build linux || darwin

package terminalagent

import (
	"fmt"
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
