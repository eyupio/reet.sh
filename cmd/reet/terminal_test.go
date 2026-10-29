package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTerminalCLIConfiguresNextControllerStart(t *testing.T) {
	t.Setenv("REET_PUBLIC_URL", "https://reet.example")
	dir, err := os.MkdirTemp("/tmp", "reet-cli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for _, step := range []struct{ action, want string }{{"start", "configured on"}, {"status", "configured on"}, {"stop", "stopped"}, {"status", "stopped"}} {
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

// reet terminal clipboard is the machine's local clipboard switch, not a
// controller command.
func TestTerminalClipboardIsTheMachineSwitch(t *testing.T) {
	err := runTerminal([]string{"clipboard"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "reet terminal clipboard on|off|status") {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("/tmp", "reet-cli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = runTerminal([]string{"clipboard", "status", "--config", dir + "/missing.json"}, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "unexpected terminal command arguments") {
		t.Fatal("clipboard reached the controller command", err)
	}
	var out bytes.Buffer
	if err := runTerminal([]string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "reet terminal clipboard on|off|status") {
		t.Fatal(out.String(), err)
	}
}

// reet terminal uploads is the machine's local image-upload switch, not a
// controller command.
func TestTerminalUploadsIsTheMachineSwitch(t *testing.T) {
	err := runTerminal([]string{"uploads"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "reet terminal uploads on|off|status") {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("/tmp", "reet-cli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = runTerminal([]string{"uploads", "status", "--config", dir + "/missing.json"}, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "unexpected terminal command arguments") {
		t.Fatal("uploads reached the controller command", err)
	}
	var out bytes.Buffer
	if err := runTerminal([]string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "reet terminal uploads on|off|status") {
		t.Fatal(out.String(), err)
	}
}

// reet terminal browse is the machine's local folder-browse switch, not a
// controller command.
func TestTerminalBrowseIsTheMachineSwitch(t *testing.T) {
	err := runTerminal([]string{"browse"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "reet terminal browse on|off|status") {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("/tmp", "reet-cli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = runTerminal([]string{"browse", "status", "--config", dir + "/missing.json"}, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "unexpected terminal command arguments") {
		t.Fatal("browse reached the controller command", err)
	}
	var out bytes.Buffer
	if err := runTerminal([]string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "reet terminal browse on|off|status") {
		t.Fatal(out.String(), err)
	}
}

// reet terminal models is the machine's local model-listing switch, not a
// controller command.
func TestTerminalModelsIsTheMachineSwitch(t *testing.T) {
	err := runTerminal([]string{"models"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "reet terminal models on|off|status") {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("/tmp", "reet-cli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = runTerminal([]string{"models", "status", "--config", dir + "/missing.json"}, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "unexpected terminal command arguments") {
		t.Fatal("models reached the controller command", err)
	}
	var out bytes.Buffer
	if err := runTerminal([]string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "reet terminal models on|off|status") {
		t.Fatal(out.String(), err)
	}
}
