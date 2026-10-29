package agenthealth

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestClipboardCapabilityNeedsNegotiationTerminalAndTheLocalSwitch(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	full := func() AgentConfig {
		return AgentConfig{TerminalSession: factory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready, clipboard: true, clipboardAllowed: true}
	}
	for _, tc := range []struct {
		name string
		edit func(*AgentConfig)
		want bool
	}{
		{"everything", func(*AgentConfig) {}, true},
		{"clipboard not negotiated", func(c *AgentConfig) { c.clipboard = false }, false},
		{"clipboard off locally", func(c *AgentConfig) { c.clipboardAllowed = false }, false},
		{"terminal access off", func(c *AgentConfig) { c.TerminalEnabled = false }, false},
		{"no terminal control", func(c *AgentConfig) { c.TerminalSession = nil }, false},
	} {
		cfg := full()
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if err := caps.Validate(); err != nil {
			t.Fatal(tc.name, err)
		}
		data, _ := json.Marshal(caps)
		if caps.Clipboard != tc.want || strings.Contains(string(data), `"clipboard"`) != tc.want {
			t.Fatal(tc.name, "clipboard", string(data))
		}
	}
}

func TestClipboardFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for _, tc := range []struct {
		advertised string
		want       bool
	}{
		{"", false},
		{terminalproto.BrowseFeature, false},
		{terminalproto.ClipboardFeature, true},
		{terminalproto.LaunchFeature + "," + terminalproto.ClipboardFeature, true},
		{"clipboard-v2", false},
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, tc.advertised)
		if cfg.clipboard != tc.want {
			t.Fatal("negotiated", tc.advertised, cfg.clipboard)
		}
		headers := http.Header{}
		setTerminalFeatures(headers, cfg)
		if terminalproto.SupportsClipboard(headers.Get(terminalproto.FeatureHeader)) != tc.want {
			t.Fatal("terminal features header", tc.advertised, headers.Get(terminalproto.FeatureHeader))
		}
	}
}

// Turning the clipboard switch off (or on) reaches the controller without a
// reconnect, as terminal consent and folder browse do. A missing switch is
// off.
func TestClipboardSwitchChangeSendsCapabilities(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	if readClipboardAllowed(AgentConfig{}) {
		t.Fatal("no switch must mean no clipboard")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	var on atomic.Bool
	on.Store(true)
	cfg := AgentConfig{
		InstanceID: testInstance(), ConsentInterval: 5 * time.Millisecond, TerminalSession: factory, clipboard: true, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready,
		Consent:          func() (bool, terminalproto.Readiness, error) { return true, terminalproto.Ready, nil },
		ClipboardAllowed: func() bool { return on.Load() },
	}
	cfg.clipboardAllowed = readClipboardAllowed(cfg)
	if !capabilities(cfg).Clipboard {
		t.Fatal("clipboard on is not reported")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan terminalproto.Message, 4)
	errs := make(chan error, 2)
	go consentControl(ctx, cfg, 3, true, terminalproto.Ready, func(_ context.Context, m terminalproto.Message) error { sent <- m; return nil }, errs)
	select {
	case m := <-sent:
		t.Fatal("sent with nothing changed", m)
	case <-time.After(50 * time.Millisecond):
	}
	for _, want := range []bool{false, true} {
		on.Store(want)
		select {
		case m := <-sent:
			hello, ok := m.Body.(terminalproto.Hello)
			if !ok || hello.Capabilities.Clipboard != want || !hello.Capabilities.TerminalEnabled {
				t.Fatalf("clipboard %t: %+v", want, m)
			}
		case err := <-errs:
			t.Fatal(err)
		case <-time.After(5 * time.Second):
			t.Fatal("no capability update for clipboard", want)
		}
	}
}

// A switch change whose new capabilities would be invalid is not sent, and the
// switch state is rolled back so a later valid tick still reports it. An
// invalid OS version makes agents-v2 capabilities fail validation.
func TestClipboardSwitchRollsBackAnInvalidCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	cachedOSVersion = "\xff"
	osVersionOnce = sync.Once{}
	osVersionOnce.Do(func() {})
	t.Cleanup(func() {
		cachedOSVersion = ""
		osVersionOnce = sync.Once{}
	})
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	var on atomic.Bool
	on.Store(true)
	// Park the second poll inside Consent, before it reads the OS version, so
	// the test can repair the cache without racing the poll.
	var polls atomic.Int32
	blocked := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseBlocked := func() { releaseOnce.Do(func() { close(release) }) }
	cfg := AgentConfig{
		InstanceID: testInstance(), ConsentInterval: 5 * time.Millisecond, TerminalSession: factory, clipboard: true, agentsV2: true, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready,
		Consent: func() (bool, terminalproto.Readiness, error) {
			if polls.Add(1) == 2 {
				close(blocked)
				<-release
			}
			return true, terminalproto.Ready, nil
		},
		ClipboardAllowed: func() bool { return on.Load() },
	}
	cfg.clipboardAllowed = false
	ctx, cancel := context.WithCancel(context.Background())
	sent := make(chan terminalproto.Message, 4)
	errs := make(chan error, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		consentControl(ctx, cfg, 3, true, terminalproto.Ready, func(_ context.Context, m terminalproto.Message) error { sent <- m; return nil }, errs)
	}()
	defer func() {
		releaseBlocked()
		cancel()
		<-done
	}()
	select {
	case m := <-sent:
		t.Fatalf("sent invalid capabilities: %+v", m)
	case err := <-errs:
		t.Fatal(err)
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("the second poll never started")
	}
	select {
	case m := <-sent:
		t.Fatalf("sent invalid capabilities: %+v", m)
	default:
	}
	cachedOSVersion = "test"
	releaseBlocked()
	select {
	case m := <-sent:
		hello, ok := m.Body.(terminalproto.Hello)
		if !ok || !hello.Capabilities.Clipboard || !hello.Capabilities.AgentsV2 {
			t.Fatalf("rolled-back switch not reported: %+v", m)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("the switch was not rolled back and retried")
	}
}
