//go:build linux

package acpagent

import "syscall"

func procAttr() *syscall.SysProcAttr {
	// The agent is the parent; Pdeathsig makes the kernel kill the chat if the
	// connector crashes rather than relying on cleanup running.
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
