//go:build linux || darwin

package scan

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTerminalClipboardSwitchAndCapability(t *testing.T) {
	config, c := browseEnrollment(t)
	allowed := clipboardAllowed(config, c)
	consent := func() error { return clipboardConsent(config, c) }
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := runAgentTerminal(append([]string{"clipboard"}, append(args, "--config", config)...), &out); err != nil {
			t.Fatal(args, err)
		}
		return out.String()
	}
	// On by default.
	if out := run("status"); out != "Clipboard: on\n" {
		t.Fatalf("status: %q", out)
	}
	if !allowed() || consent() != nil {
		t.Fatal("clipboard is on by default")
	}

	if out := run("off"); !strings.Contains(out, "Clipboard off for fixture.") {
		t.Fatalf("off: %q", out)
	}
	loaded, err := loadAgentConfig(config)
	if err != nil || !loaded.ClipboardOff || !loaded.TerminalEnabled {
		t.Fatalf("not stored: %+v %v", loaded, err)
	}
	data, _ := os.ReadFile(config)
	if !strings.Contains(string(data), `"clipboardOff": true`) {
		t.Fatal("config file", string(data))
	}
	if out := run("status"); out != "Clipboard: off\n" {
		t.Fatalf("status: %q", out)
	}
	// The running connector sees the switch without a restart: the capability
	// goes (ClipboardAllowed is what agenthealth reports it from) and the
	// manager would stop forwarding.
	if allowed() || consent() == nil {
		t.Fatal("capability still reported")
	}
	var status bytes.Buffer
	if err := runAgentTerminal([]string{"status", "--config", config}, &status); err != nil || !strings.Contains(status.String(), "Clipboard: off\n") {
		t.Fatalf("terminal status: %q %v", status.String(), err)
	}

	if out := run("on"); !strings.Contains(out, "Clipboard on for fixture.") {
		t.Fatalf("on: %q", out)
	}
	if !allowed() || consent() != nil {
		t.Fatal("clipboard on again not reported")
	}
	data, _ = os.ReadFile(config)
	if strings.Contains(string(data), "clipboardOff") {
		t.Fatal("clipboardOff kept when on", string(data))
	}

	// Terminal access off removes the capability even with the switch on.
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if allowed() || consent() == nil {
		t.Fatal("clipboard allowed with terminal access off")
	}
	if out := run("status"); out != "Clipboard: on (unavailable while terminal access is off)\n" {
		t.Fatalf("status with terminal off: %q", out)
	}

	for _, bad := range [][]string{{"clipboard"}, {"clipboard", "maybe"}, {"clipboard", "off", "extra"}} {
		if err := runAgentTerminal(append(bad, "--config", config), &bytes.Buffer{}); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}

// The same switch is reachable as reet terminal clipboard.
func TestRunTerminalClipboard(t *testing.T) {
	config, _ := browseEnrollment(t)
	var out bytes.Buffer
	if err := RunTerminalClipboard([]string{"off", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if loaded, err := loadAgentConfig(config); err != nil || !loaded.ClipboardOff {
		t.Fatal(loaded, err)
	}
	out.Reset()
	if err := RunTerminalClipboard([]string{"status", "--config", config}, &out); err != nil || out.String() != "Clipboard: off\n" {
		t.Fatalf("status: %q %v", out.String(), err)
	}
}

// clipboard --help prints its usage like the other subcommands, and turning
// clipboard on while terminal access is off says plainly that nothing can be
// forwarded until terminal access is on.
func TestTerminalClipboardHelpAndTerminalOff(t *testing.T) {
	config, _ := browseEnrollment(t)
	for _, args := range [][]string{{"--help"}, {"-h"}, {"on", "--help"}} {
		var out bytes.Buffer
		if err := RunTerminalClipboard(args, &out); err != nil || !strings.Contains(out.String(), "reet terminal clipboard on|off|status") {
			t.Fatalf("%q: %q %v", args, out.String(), err)
		}
	}
	var help bytes.Buffer
	if err := runAgentTerminal([]string{"clipboard", "--help"}, &help); err != nil || !strings.Contains(help.String(), "on|off|status") {
		t.Fatalf("agent terminal clipboard --help: %q %v", help.String(), err)
	}
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunTerminalClipboard([]string{"on", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "can forward") || !strings.Contains(out.String(), "terminal access is off") || !strings.Contains(out.String(), "reet agent terminal enable") {
		t.Fatalf("on with terminal off: %q", out.String())
	}
	out.Reset()
	if err := RunTerminalClipboard([]string{"status", "--config", config}, &out); err != nil || out.String() != "Clipboard: on (unavailable while terminal access is off)\n" {
		t.Fatalf("status with terminal off: %q %v", out.String(), err)
	}
}
