//go:build linux || darwin

package main

import (
	"bytes"
	"github.com/creack/pty"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalStartActuallyPromptsOnATerminal(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "reet-prompt-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	t.Setenv("REET_PUBLIC_URL", "https://reet.example")
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	c, err := terminalcontrol.Open(filepath.Join(dir, "data"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err = c.Serve(func(bool) {}, func() bool { return true }, terminalcontrol.Options{PublicURL: func() string { return "https://reet.example" }, AuthMode: func() string { return "accounts" }}); err != nil {
		t.Fatal(err)
	}
	if _, err = master.Write([]byte("no\n")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = runTerminalInput([]string{"start", "--data-dir", filepath.Join(dir, "data"), "--agent-config", filepath.Join(dir, "missing-agent.json")}, slave, &out); err != nil {
		t.Fatal(err, out.String())
	}
	if !strings.Contains(out.String(), "Also set up this machine for web terminal access?") || !strings.Contains(out.String(), "not enrolled or granted shell access") {
		t.Fatal("start printed advice instead of asking", out.String())
	}
}
