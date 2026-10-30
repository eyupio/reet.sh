//go:build darwin

package acpagent

import "syscall"

func procAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
