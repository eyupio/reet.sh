//go:build linux || darwin

package scan

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTerminalUploadsSwitchAndCapability(t *testing.T) {
	config, c := browseEnrollment(t)
	allowed := uploadsAllowed(config, c)
	consent := func() error { return uploadConsent(config, c) }
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := runAgentTerminal(append([]string{"uploads"}, append(args, "--config", config)...), &out); err != nil {
			t.Fatal(args, err)
		}
		return out.String()
	}
	// On by default.
	if out := run("status"); out != "Uploads: on\n" {
		t.Fatalf("status: %q", out)
	}
	if !allowed() || consent() != nil {
		t.Fatal("uploads are on by default")
	}

	if out := run("off"); !strings.Contains(out, "Image uploads off for fixture.") {
		t.Fatalf("off: %q", out)
	}
	loaded, err := loadAgentConfig(config)
	if err != nil || !loaded.UploadsOff || !loaded.TerminalEnabled {
		t.Fatalf("not stored: %+v %v", loaded, err)
	}
	data, _ := os.ReadFile(config)
	if !strings.Contains(string(data), `"uploadsOff": true`) {
		t.Fatal("config file", string(data))
	}
	if out := run("status"); out != "Uploads: off\n" {
		t.Fatalf("status: %q", out)
	}
	// The running connector sees the switch without a restart.
	if allowed() || consent() == nil {
		t.Fatal("capability still reported")
	}
	var status bytes.Buffer
	if err := runAgentTerminal([]string{"status", "--config", config}, &status); err != nil || !strings.Contains(status.String(), "Uploads: off\n") {
		t.Fatalf("terminal status: %q %v", status.String(), err)
	}

	if out := run("on"); !strings.Contains(out, "Image uploads on for fixture.") {
		t.Fatalf("on: %q", out)
	}
	if !allowed() || consent() != nil {
		t.Fatal("uploads on again not reported")
	}
	data, _ = os.ReadFile(config)
	if strings.Contains(string(data), "uploadsOff") {
		t.Fatal("uploadsOff kept when on", string(data))
	}

	// Terminal access off removes the capability even with the switch on.
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if allowed() || consent() == nil {
		t.Fatal("uploads allowed with terminal access off")
	}
	if out := run("status"); out != "Uploads: on (unavailable while terminal access is off)\n" {
		t.Fatalf("status with terminal off: %q", out)
	}

	for _, bad := range [][]string{{"uploads"}, {"uploads", "maybe"}, {"uploads", "off", "extra"}} {
		if err := runAgentTerminal(append(bad, "--config", config), &bytes.Buffer{}); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}

// The config round-trips the switch exactly like browse and clipboard.
func TestUploadsConfigRoundTrip(t *testing.T) {
	config, c := browseEnrollment(t)
	c.UploadsOff = true
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadAgentConfig(config)
	if err != nil || !loaded.UploadsOff {
		t.Fatal(loaded, err)
	}
	c.UploadsOff = false
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadAgentConfig(config)
	if err != nil || loaded.UploadsOff {
		t.Fatal(loaded, err)
	}
	data, _ := os.ReadFile(config)
	if strings.Contains(string(data), "uploadsOff") {
		t.Fatal("false uploadsOff was written", string(data))
	}
}

// The same switch is reachable as reet terminal uploads.
func TestRunTerminalUploads(t *testing.T) {
	config, _ := browseEnrollment(t)
	var out bytes.Buffer
	if err := RunTerminalUploads([]string{"off", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if loaded, err := loadAgentConfig(config); err != nil || !loaded.UploadsOff {
		t.Fatal(loaded, err)
	}
	out.Reset()
	if err := RunTerminalUploads([]string{"status", "--config", config}, &out); err != nil || out.String() != "Uploads: off\n" {
		t.Fatalf("status: %q %v", out.String(), err)
	}
}

// uploads --help prints its usage like the other subcommands, and turning
// uploads on while terminal access is off says plainly that nothing can be
// stored until terminal access is on.
func TestTerminalUploadsHelpAndTerminalOff(t *testing.T) {
	config, _ := browseEnrollment(t)
	for _, args := range [][]string{{"--help"}, {"-h"}, {"on", "--help"}} {
		var out bytes.Buffer
		if err := RunTerminalUploads(args, &out); err != nil || !strings.Contains(out.String(), "reet terminal uploads on|off|status") {
			t.Fatalf("%q: %q %v", args, out.String(), err)
		}
	}
	var help bytes.Buffer
	if err := runAgentTerminal([]string{"uploads", "--help"}, &help); err != nil || !strings.Contains(help.String(), "on|off|status") {
		t.Fatalf("agent terminal uploads --help: %q %v", help.String(), err)
	}
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunTerminalUploads([]string{"on", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "stores images") || !strings.Contains(out.String(), "terminal access is off") || !strings.Contains(out.String(), "reet agent terminal enable") {
		t.Fatalf("on with terminal off: %q", out.String())
	}
	out.Reset()
	if err := RunTerminalUploads([]string{"status", "--config", config}, &out); err != nil || out.String() != "Uploads: on (unavailable while terminal access is off)\n" {
		t.Fatalf("status with terminal off: %q %v", out.String(), err)
	}
}
