package terminalhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// TerminalNoNewPrivileges inspects only this enrollment's private tmux server.
// A unit refresh cannot clear this flag in a running server. Errors mean unknown.
func TerminalNoNewPrivileges(ctx context.Context, plan ServicePlan) (bool, error) {
	if runtime.GOOS != "linux" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, plan.TmuxPath, "-S", plan.Layout.Socket, "display-message", "-p", "#{pid}")
	cmd.Env = append([]string{}, plan.Env...)
	output, err := cmd.Output()
	if err != nil {
		return false, errors.New("terminal server privilege state unavailable")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || pid <= 0 {
		return false, errors.New("invalid terminal server process")
	}
	return processNoNewPrivileges(pid)
}
func processNoNewPrivileges(pid int) (bool, error) {
	if pid <= 0 {
		return false, errors.New("invalid process")
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false, errors.New("process privilege state unavailable")
	}
	return parseNoNewPrivileges(string(raw))
}
func parseNoNewPrivileges(status string) (bool, error) {
	for _, line := range strings.Split(status, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || key != "NoNewPrivs" {
			continue
		}
		switch strings.TrimSpace(value) {
		case "0":
			return false, nil
		case "1":
			return true, nil
		default:
			return false, errors.New("invalid process privilege state")
		}
	}
	return false, errors.New("process privilege state unavailable")
}
