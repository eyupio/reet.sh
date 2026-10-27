//go:build linux || darwin

package scan

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// browseEnrollment writes an enrollment with terminal access on for a home
// of its own (HOME points at it, as loadAgentConfig requires).
func browseEnrollment(t *testing.T) (string, agentConfig) {
	t.Helper()
	home, err := filepath.EvalSymlinks(shortScanDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "code"), 0o700); err != nil {
		t.Fatal(err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(shortScanDir(t), "agent.json")
	c := agentConfig{Server: "https://example.invalid", Token: "token", ID: "agent0123456789abcdef", Secret: "secret0123456789abcdef", Host: "fixture", Hostname: hostname, Home: home, TerminalEnabled: true}
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	return config, c
}

// browseThrough sends one browse to the connector this enrollment makes.
func browseThrough(t *testing.T, config string, c agentConfig) terminalproto.BrowseResult {
	t.Helper()
	factory := terminalControl(config, c)
	if factory == nil {
		t.Fatal("no terminal control")
	}
	got := make(chan terminalproto.Message, 4)
	headers := http.Header{}
	headers.Set(terminalproto.FeatureHeader, terminalproto.BrowseFeature)
	handle, cleanup := factory(t.Context(), http.DefaultClient, headers, 1, func(_ context.Context, m terminalproto.Message) error { got <- m; return nil })
	defer cleanup()
	msg := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: "browse-request-00001", Body: &terminalproto.Browse{}}
	if err := handle(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		r, ok := m.Body.(terminalproto.BrowseResult)
		if !ok || m.RequestID != msg.RequestID {
			t.Fatalf("%+v", m)
		}
		return r
	case <-time.After(10 * time.Second):
		t.Fatal("no browse answer")
	}
	return terminalproto.BrowseResult{}
}

func TestTerminalBrowseOffRemovesCapability(t *testing.T) {
	config, c := browseEnrollment(t)
	allowed := browseAllowed(config, c)
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := runAgentTerminal(append([]string{"browse"}, append(args, "--config", config)...), &out); err != nil {
			t.Fatal(args, err)
		}
		return out.String()
	}
	// On by default.
	if out := run("status"); out != "Folder browse: on\n" {
		t.Fatalf("status: %q", out)
	}
	if !allowed() {
		t.Fatal("browse is on by default")
	}
	if r := browseThrough(t, config, c); r.Reason != "" || r.Path != c.Home || len(r.Entries) != 1 || r.Entries[0].Name != "code" {
		t.Fatalf("browse on: %+v", r)
	}

	if out := run("off"); !strings.Contains(out, "Folder browse off for fixture.") {
		t.Fatalf("off: %q", out)
	}
	loaded, err := loadAgentConfig(config)
	if err != nil || !loaded.BrowseOff || !loaded.TerminalEnabled {
		t.Fatalf("not stored: %+v %v", loaded, err)
	}
	data, _ := os.ReadFile(config)
	if !strings.Contains(string(data), `"browseOff": true`) {
		t.Fatal("config file", string(data))
	}
	if out := run("status"); out != "Folder browse: off\n" {
		t.Fatalf("status: %q", out)
	}
	// The running connector sees the switch without a restart: the
	// capability goes (BrowseAllowed is what agenthealth reports it from)
	// and a browse is answered off.
	if allowed() {
		t.Fatal("capability still reported")
	}
	if r := browseThrough(t, config, c); !reflect.DeepEqual(r, terminalproto.BrowseResult{Reason: terminalproto.BrowseOff}) {
		t.Fatalf("browse off: %+v", r)
	}
	var out bytes.Buffer
	if err := runAgentTerminal([]string{"status", "--config", config}, &out); err != nil || !strings.Contains(out.String(), "Folder browse: off\n") {
		t.Fatalf("terminal status: %q %v", out.String(), err)
	}

	if out := run("on"); !strings.Contains(out, "Folder browse on for fixture.") {
		t.Fatalf("on: %q", out)
	}
	if !allowed() {
		t.Fatal("browse on again not reported")
	}
	data, _ = os.ReadFile(config)
	if strings.Contains(string(data), "browseOff") {
		t.Fatal("browseOff kept when on", string(data))
	}

	// Terminal access off refuses browsing too.
	run("off")
	run("on")
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if allowed() {
		t.Fatal("browse allowed with terminal access off")
	}
	if r := browseThrough(t, config, c); r.Reason != terminalproto.BrowseOff {
		t.Fatalf("terminal off: %+v", r)
	}

	for _, bad := range [][]string{{"browse"}, {"browse", "maybe"}, {"browse", "off", "extra"}} {
		if err := runAgentTerminal(append(bad, "--config", config), &bytes.Buffer{}); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}

// The same switch is reachable as reet terminal browse.
func TestRunTerminalBrowse(t *testing.T) {
	config, _ := browseEnrollment(t)
	var out bytes.Buffer
	if err := RunTerminalBrowse([]string{"off", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if loaded, err := loadAgentConfig(config); err != nil || !loaded.BrowseOff {
		t.Fatal(loaded, err)
	}
}

// browse --help prints its usage like the other subcommands, and turning
// browse on while terminal access is off says plainly that nothing can be
// listed until terminal access is on.
func TestTerminalBrowseHelpAndTerminalOff(t *testing.T) {
	config, _ := browseEnrollment(t)
	for _, args := range [][]string{{"--help"}, {"-h"}, {"on", "--help"}} {
		var out bytes.Buffer
		if err := RunTerminalBrowse(args, &out); err != nil || !strings.Contains(out.String(), "reet terminal browse on|off|status") {
			t.Fatalf("%q: %q %v", args, out.String(), err)
		}
	}
	var help bytes.Buffer
	if err := runAgentTerminal([]string{"browse", "--help"}, &help); err != nil || !strings.Contains(help.String(), "on|off|status") {
		t.Fatalf("agent terminal browse --help: %q %v", help.String(), err)
	}
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunTerminalBrowse([]string{"on", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "can list") || !strings.Contains(out.String(), "terminal access is off") || !strings.Contains(out.String(), "reet agent terminal enable") {
		t.Fatalf("on with terminal off: %q", out.String())
	}
	out.Reset()
	if err := RunTerminalBrowse([]string{"status", "--config", config}, &out); err != nil || out.String() != "Folder browse: on (unavailable while terminal access is off)\n" {
		t.Fatalf("status with terminal off: %q %v", out.String(), err)
	}
}

// terminalAllowed is the cheap terminal opt-in read browse and branches are
// gated on: this enrollment, terminal access on. It probes nothing.
func TestTerminalAllowedReadsOnlyTheEnrollment(t *testing.T) {
	config, c := browseEnrollment(t)
	allowed := terminalAllowed(config, c)
	if allowed == nil || !allowed() {
		t.Fatal("terminal access on is not allowed")
	}
	if err := runAgentTerminal([]string{"browse", "off", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !allowed() {
		t.Fatal("browse off must not turn terminal access off")
	}
	if err := runAgentTerminal([]string{"disable", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if allowed() {
		t.Fatal("terminal access off is allowed")
	}
	other := c
	other.Secret = "another0123456789abcdef"
	other.TerminalEnabled = true
	if err := writePrivate(config, other); err != nil {
		t.Fatal(err)
	}
	if allowed() {
		t.Fatal("a changed enrollment is allowed")
	}
}
