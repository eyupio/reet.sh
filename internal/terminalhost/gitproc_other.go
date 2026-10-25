//go:build !linux && !darwin

package terminalhost

import "os/exec"

// Worktrees are supported on Linux and macOS only; elsewhere git keeps the
// default cancellation (the git process alone is killed).
func killGroupOnCancel(*exec.Cmd) {}
