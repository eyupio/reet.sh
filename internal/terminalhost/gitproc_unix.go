//go:build linux || darwin

package terminalhost

import (
	"os/exec"
	"syscall"
)

// killGroupOnCancel starts git in its own process group and, when its
// context ends, kills the whole group: a hook, filter or helper git started
// must not outlive a timed-out call.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
