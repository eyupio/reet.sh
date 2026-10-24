package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTerminalCLIConfiguresNextControllerStart(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "reet-cli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for _, step := range []struct{ action, want string }{{"start", "started"}, {"status", "started"}, {"stop", "stopped"}, {"status", "stopped"}} {
		var out bytes.Buffer
		if err := runTerminal([]string{step.action, "--data-dir", dir}, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Remote terminals: "+step.want) || !strings.Contains(out.String(), "Controller: stopped") {
			t.Fatal(out.String())
		}
	}
	if err := runTerminal([]string{"restart", "--data-dir", dir}, &bytes.Buffer{}); err == nil {
		t.Fatal("offline restart succeeded")
	}
	if err := runTerminal([]string{"explode", "--data-dir", dir}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown action succeeded")
	}
}
