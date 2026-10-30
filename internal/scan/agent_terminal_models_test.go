//go:build linux || darwin

package scan

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTerminalModelsSwitchAndCapability(t *testing.T) {
	config, c := browseEnrollment(t)
	allowed := modelsAllowed(config, c)
	consent := func() error { return modelsConsent(config, c) }
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := runAgentTerminal(append([]string{"models"}, append(args, "--config", config)...), &out); err != nil {
			t.Fatal(args, err)
		}
		return out.String()
	}
	// On by default.
	if out := run("status"); out != "Models: on\n" {
		t.Fatalf("status: %q", out)
	}
	if !allowed() || consent() != nil {
		t.Fatal("models are on by default")
	}

	if out := run("off"); !strings.Contains(out, "Models off for fixture.") {
		t.Fatalf("off: %q", out)
	}
	loaded, err := loadAgentConfig(config)
	if err != nil || !loaded.ModelsOff || !loaded.TerminalEnabled {
		t.Fatalf("not stored: %+v %v", loaded, err)
	}
	data, _ := os.ReadFile(config)
	if !strings.Contains(string(data), `"modelsOff": true`) {
		t.Fatal("config file", string(data))
	}
	if out := run("status"); out != "Models: off\n" {
		t.Fatalf("status: %q", out)
	}
	// The running connector sees the switch without a restart.
	if allowed() || consent() == nil {
		t.Fatal("capability still reported")
	}
	var status bytes.Buffer
	if err := runAgentTerminal([]string{"status", "--config", config}, &status); err != nil || !strings.Contains(status.String(), "Models: off\n") {
		t.Fatalf("terminal status: %q %v", status.String(), err)
	}

	if out := run("on"); !strings.Contains(out, "Models on for fixture.") {
		t.Fatalf("on: %q", out)
	}
	if !allowed() || consent() != nil {
		t.Fatal("models on again not reported")
	}
	data, _ = os.ReadFile(config)
	if strings.Contains(string(data), "modelsOff") {
		t.Fatal("modelsOff kept when on", string(data))
	}

	// Terminal access off removes the capability even with the switch on.
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if allowed() || consent() == nil {
		t.Fatal("models allowed with terminal access off")
	}
	if out := run("status"); out != "Models: on (unavailable while terminal access is off)\n" {
		t.Fatalf("status with terminal off: %q", out)
	}

	for _, bad := range [][]string{{"models"}, {"models", "maybe"}, {"models", "off", "extra"}} {
		if err := runAgentTerminal(append(bad, "--config", config), &bytes.Buffer{}); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}

func TestModelsConfigRoundTrip(t *testing.T) {
	config, c := browseEnrollment(t)
	c.ModelsOff = true
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadAgentConfig(config)
	if err != nil || !loaded.ModelsOff {
		t.Fatal(loaded, err)
	}
	c.ModelsOff = false
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadAgentConfig(config)
	if err != nil || loaded.ModelsOff {
		t.Fatal(loaded, err)
	}
	data, _ := os.ReadFile(config)
	if strings.Contains(string(data), "modelsOff") {
		t.Fatal("false modelsOff was written", string(data))
	}
}

func TestRunTerminalModels(t *testing.T) {
	config, _ := browseEnrollment(t)
	var out bytes.Buffer
	if err := RunTerminalModels([]string{"off", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if loaded, err := loadAgentConfig(config); err != nil || !loaded.ModelsOff {
		t.Fatal(loaded, err)
	}
	out.Reset()
	if err := RunTerminalModels([]string{"status", "--config", config}, &out); err != nil || out.String() != "Models: off\n" {
		t.Fatalf("status: %q %v", out.String(), err)
	}
}

func TestTerminalModelsHelpAndWiring(t *testing.T) {
	config, _ := browseEnrollment(t)
	for _, args := range [][]string{{"--help"}, {"-h"}, {"on", "--help"}} {
		var out bytes.Buffer
		if err := RunTerminalModels(args, &out); err != nil || !strings.Contains(out.String(), "reet terminal models on|off|status") {
			t.Fatalf("%q: %q %v", args, out.String(), err)
		}
	}
	var help bytes.Buffer
	if err := runAgentTerminal([]string{"models", "--help"}, &help); err != nil || !strings.Contains(help.String(), "on|off|status") {
		t.Fatalf("agent terminal models --help: %q %v", help.String(), err)
	}
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunTerminalModels([]string{"on", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "can read the model list") || !strings.Contains(out.String(), "terminal access is off") || !strings.Contains(out.String(), "reet agent terminal enable") {
		t.Fatalf("on with terminal off: %q", out.String())
	}
	out.Reset()
	if err := RunTerminalModels([]string{"status", "--config", config}, &out); err != nil || out.String() != "Models: on (unavailable while terminal access is off)\n" {
		t.Fatalf("status with terminal off: %q %v", out.String(), err)
	}
}

// The connector built by terminalControl refuses a Models request when the
// local switch is off, proving scan wires ModelsConsent from the config.
func TestModelsSwitchReachesTheConnector(t *testing.T) {
	config, c := browseEnrollment(t)
	c.ModelsOff = true
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	factory := terminalControl(t.Context(), config, c)
	if factory == nil {
		t.Fatal("no terminal control")
	}
	got := make(chan terminalproto.Message, 4)
	headers := http.Header{}
	headers.Set(terminalproto.FeatureHeader, terminalproto.ModelsFeature)
	handle, cleanup := factory(t.Context(), http.DefaultClient, headers, 1, func(_ context.Context, m terminalproto.Message) error { got <- m; return nil })
	defer cleanup()
	msg := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: "models-request-00001", Body: &terminalproto.Models{Tool: "opencode"}}
	if err := handle(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		r, ok := m.Body.(terminalproto.ModelsResult)
		if !ok || r.Reason != terminalproto.ModelsOff {
			t.Fatalf("%+v", m)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no models answer")
	}
}
