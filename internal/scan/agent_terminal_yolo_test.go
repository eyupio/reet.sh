//go:build linux || darwin

package scan

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTerminalYoloSwitchAndCapability(t *testing.T) {
	config, c := browseEnrollment(t)
	allowed := yoloAllowed(config, c)
	consent := func() error { return yoloConsent(config, c) }
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := runAgentTerminal(append([]string{"yolo"}, append(args, "--config", config)...), &out); err != nil {
			t.Fatal(args, err)
		}
		return out.String()
	}
	// A machine accepts YOLO by default; the account and the session still gate it.
	if out := run("status"); out != "YOLO: on\n" {
		t.Fatalf("status: %q", out)
	}
	if !allowed() || consent() != nil {
		t.Fatal("YOLO is on by default")
	}

	if out := run("off"); !strings.Contains(out, "YOLO off for fixture.") {
		t.Fatalf("off: %q", out)
	}
	loaded, err := loadAgentConfig(config)
	if err != nil || !loaded.YoloOff || !loaded.TerminalEnabled {
		t.Fatalf("not stored: %+v %v", loaded, err)
	}
	data, _ := os.ReadFile(config)
	if !strings.Contains(string(data), `"yoloOff": true`) {
		t.Fatal("config file", string(data))
	}
	if out := run("status"); out != "YOLO: off\n" {
		t.Fatalf("status: %q", out)
	}
	// The running connector sees the switch without a restart.
	if allowed() || consent() == nil {
		t.Fatal("capability still reported")
	}
	var status bytes.Buffer
	if err := runAgentTerminal([]string{"status", "--config", config}, &status); err != nil || !strings.Contains(status.String(), "YOLO: off\n") {
		t.Fatalf("terminal status: %q %v", status.String(), err)
	}

	if out := run("on"); !strings.Contains(out, "YOLO on for fixture.") {
		t.Fatalf("on: %q", out)
	}
	if !allowed() || consent() != nil {
		t.Fatal("YOLO on again not reported")
	}
	data, _ = os.ReadFile(config)
	if strings.Contains(string(data), "yoloOff") {
		t.Fatal("yoloOff kept when on", string(data))
	}

	// Terminal access off removes the capability even with the switch on.
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if allowed() || consent() == nil {
		t.Fatal("YOLO allowed with terminal access off")
	}
	if out := run("status"); out != "YOLO: on (unavailable while terminal access is off)\n" {
		t.Fatalf("status with terminal off: %q", out)
	}

	for _, bad := range [][]string{{"yolo"}, {"yolo", "maybe"}, {"yolo", "off", "extra"}} {
		if err := runAgentTerminal(append(bad, "--config", config), &bytes.Buffer{}); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}

func TestRunTerminalYolo(t *testing.T) {
	config, _ := browseEnrollment(t)
	var out bytes.Buffer
	if err := RunTerminalYolo([]string{"off", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if loaded, err := loadAgentConfig(config); err != nil || !loaded.YoloOff {
		t.Fatal(loaded, err)
	}
	out.Reset()
	if err := RunTerminalYolo([]string{"status", "--config", config}, &out); err != nil || out.String() != "YOLO: off\n" {
		t.Fatalf("status: %q %v", out.String(), err)
	}
}

func TestTerminalYoloHelpAndWiring(t *testing.T) {
	config, _ := browseEnrollment(t)
	for _, args := range [][]string{{"--help"}, {"-h"}, {"on", "--help"}} {
		var out bytes.Buffer
		if err := RunTerminalYolo(args, &out); err != nil || !strings.Contains(out.String(), "reet terminal yolo on|off|status") {
			t.Fatalf("%q: %q %v", args, out.String(), err)
		}
	}
	var help bytes.Buffer
	if err := runAgentTerminal([]string{"yolo", "--help"}, &help); err != nil || !strings.Contains(help.String(), "on|off|status") {
		t.Fatalf("agent terminal yolo --help: %q %v", help.String(), err)
	}
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}
