package terminalhost

import (
	"context"
	"os"
	"runtime"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Readiness reports local terminal infrastructure without creating or repairing
// state. It does not prove that a controller can create a session yet.
func Readiness(ctx context.Context, layout Layout, consent bool) terminalproto.Readiness {
	if !consent {
		return terminalproto.DisabledLocally
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return terminalproto.Unsupported
	}
	if _, err := FindTmux(ctx); err != nil {
		return terminalproto.TmuxUnavailable
	}
	if err := Check(layout); err != nil {
		return terminalproto.SupervisorUnavailable
	}
	info, err := os.Lstat(layout.Socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return terminalproto.SupervisorUnavailable
	}
	return terminalproto.Ready
}
