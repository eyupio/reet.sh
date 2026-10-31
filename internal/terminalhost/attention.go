//go:build linux || darwin

package terminalhost

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ReadAttention reads a tmux session's window_activity and window_bell_flag
// without capturing any terminal output. When window_bell_flag is set on a
// window, the current time is returned as bellAt and the flag is cleared on
// that window so a subsequent bell can be observed independently.
//
// tmux may be either a tmux socket path (when socket is omitted and tmux names
// a unix socket) or the tmux binary path (with socket[0] naming the socket).
func ReadAttention(ctx context.Context, tmux, session string, socket ...string) (activityAt, bellAt int64, err error) {
	return readAttentionWithEnv(ctx, tmux, session, nil, "", socket...)
}

func (m *Manager) readAttention(ctx context.Context, sessionID string) (activityAt, bellAt int64, err error) {
	return readAttentionWithEnv(ctx, m.tmux.Path, m.name(sessionID), m.environment(), m.home, m.layout.Socket)
}

func readAttentionWithEnv(ctx context.Context, tmux, session string, env []string, dir string, socket ...string) (activityAt, bellAt int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tmuxBin := tmux
	sockPath := ""
	if len(socket) > 0 {
		sockPath = socket[0]
	} else if info, statErr := os.Stat(tmux); statErr == nil && info.Mode()&os.ModeSocket != 0 {
		sockPath = tmux
		found, findErr := FindTmux(ctx)
		if findErr != nil {
			return 0, 0, findErr
		}
		tmuxBin = found.Path
	}
	if tmuxBin == "" {
		found, findErr := FindTmux(ctx)
		if findErr != nil {
			return 0, 0, findErr
		}
		tmuxBin = found.Path
	}

	target := session
	if !strings.HasPrefix(target, "=") {
		target = "=" + target
	}

	run := func(args ...string) ([]byte, error) {
		full := args
		if sockPath != "" {
			full = append([]string{"-S", sockPath}, args...)
		}
		cmd := exec.CommandContext(ctx, tmuxBin, full...)
		if len(env) > 0 {
			cmd.Env = env
		}
		if dir != "" {
			cmd.Dir = dir
		}
		var out cappedBuffer
		out.limit = 16 << 10
		cmd.Stdout = &out
		runErr := cmd.Run()
		if out.overflow {
			return nil, ErrOutputLimit
		}
		if runErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrNotRunning
		}
		return out.Bytes(), nil
	}

	out, err := run("list-windows", "-t", target, "-F", "#{window_index}"+fieldSep+"#{window_activity}"+fieldSep+"#{window_bell_flag}")
	if err != nil {
		return 0, 0, err
	}

	var bellWindows []string
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, fieldSep)
		if len(fields) != 3 {
			return 0, 0, ErrNotRunning
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			return 0, 0, ErrNotRunning
		}
		sec, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || sec < 0 {
			return 0, 0, ErrNotRunning
		}
		if actMillis := sec * 1000; actMillis > activityAt {
			activityAt = actMillis
		}
		switch fields[2] {
		case "0":
		case "1":
			if bellAt == 0 {
				bellAt = time.Now().UnixMilli()
			}
			bellWindows = append(bellWindows, fields[0])
		default:
			return 0, 0, ErrNotRunning
		}
	}

	for _, idx := range bellWindows {
		tmp := "999"
		if idx == "999" {
			tmp = "998"
		}
		win := target + ":" + idx
		tmpWin := target + ":" + tmp
		_, _ = run("move-window", "-s", win, "-t", tmpWin, ";", "move-window", "-s", tmpWin, "-t", win)
	}
	return activityAt, bellAt, nil
}
