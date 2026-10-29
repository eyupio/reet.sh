//go:build darwin

package opencodeacp

import "syscall"

func procAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
