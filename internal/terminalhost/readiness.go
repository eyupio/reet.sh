package terminalhost

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// responsive asks the running server to read an option. A leftover socket from
// an unclean exit fails this probe; Lstat alone would wrongly report Ready.
func responsive(ctx context.Context, tmux Tmux, layout Layout) bool {
	if !SocketActive(layout) {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	command := exec.CommandContext(probeCtx, tmux.Path, "-S", layout.Socket, "show-options", "-gv", "exit-empty")
	command.Env = append(os.Environ(), "LC_ALL=C")
	return command.Run() == nil
}

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
	tmux, err := FindTmux(ctx)
	if err != nil || !responsive(ctx, tmux, layout) {
		return terminalproto.SupervisorUnavailable
	}
	return terminalproto.Ready
}
