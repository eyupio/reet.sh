package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentAllowApplyToggle(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "agent.json")
	enrollment := agentConfig{Server: "https://example.invalid", Token: "token", ID: "agent0123456789abcdef", Secret: "secret0123456789abcdef", Host: "fixture", Hostname: hostname, Home: home}
	if err := writePrivate(config, enrollment); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := RunAgent([]string{"allow-apply", "status", "--config", config}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Remote apply enabled locally: off") {
		t.Fatalf("status: %q", out.String())
	}

	out.Reset()
	if err := RunAgent([]string{"allow-apply", "on", "--config", config}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if loaded, err := loadAgentConfig(config); err != nil || !loaded.AllowApply {
		t.Fatalf("on was not saved: %+v %v", loaded, err)
	}
	if !strings.Contains(out.String(), "Remote apply enabled locally") {
		t.Fatalf("on: %q", out.String())
	}

	out.Reset()
	if err := RunAgent([]string{"allow-apply", "off", "--config", config}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if loaded, err := loadAgentConfig(config); err != nil || loaded.AllowApply {
		t.Fatalf("off was not saved: %+v %v", loaded, err)
	}

	for _, args := range [][]string{{"allow-apply"}, {"allow-apply", "maybe"}, {"allow-apply", "on", "extra"}} {
		if err := RunAgent(append(args, "--config", config), &bytes.Buffer{}, "test"); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
	var help bytes.Buffer
	if err := RunAgent([]string{"allow-apply", "--help"}, &help, "test"); err != nil || !strings.Contains(help.String(), "on|off|status") {
		t.Fatalf("help: %q %v", help.String(), err)
	}
}
