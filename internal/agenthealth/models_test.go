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

func TestModelsCapabilityNeedsNegotiationTerminalAndTheLocalSwitch(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	full := func() AgentConfig {
		return AgentConfig{TerminalSession: factory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready, models: true, modelsAllowed: true}
	}
	for _, tc := range []struct {
		name     string
		edit     func(*AgentConfig)
		models   bool
		modelsOf bool
	}{
		{"everything", func(*AgentConfig) {}, true, false},
		{"models not negotiated", func(c *AgentConfig) { c.models = false }, false, false},
		{"models off locally", func(c *AgentConfig) { c.modelsAllowed = false }, false, true},
		{"terminal access off", func(c *AgentConfig) { c.TerminalEnabled = false }, false, false},
		{"no terminal control", func(c *AgentConfig) { c.TerminalSession = nil }, false, false},
	} {
		cfg := full()
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if err := caps.Validate(); err != nil {
			t.Fatal(tc.name, err)
		}
		data, _ := json.Marshal(caps)
		raw := string(data)
		if caps.Models != tc.models || caps.ModelsOff != tc.modelsOf {
			t.Fatalf("%s: capabilities %+v", tc.name, caps)
		}
		if strings.Contains(raw, `"models":`) != tc.models || strings.Contains(raw, `"modelsOff":`) != tc.modelsOf {
			t.Fatalf("%s: encoded %s", tc.name, raw)
		}
	}
}

func TestModelsFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for _, tc := range []struct {
		advertised string
		want       bool
	}{
		{"", false},
		{terminalproto.ClipboardFeature, false},
		{terminalproto.ModelsFeature, true},
		{terminalproto.LaunchFeature + "," + terminalproto.ModelsFeature, true},
		{"models-v2", false},
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, tc.advertised)
		if cfg.models != tc.want {
			t.Fatal("negotiated", tc.advertised, cfg.models)
		}
		headers := http.Header{}
		setTerminalFeatures(headers, cfg)
		if terminalproto.SupportsModels(headers.Get(terminalproto.FeatureHeader)) != tc.want {
			t.Fatal("terminal features header", tc.advertised, headers.Get(terminalproto.FeatureHeader))
		}
	}
}

func TestModelsSwitchChangeSendsCapabilities(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	if readModelsAllowed(AgentConfig{}) {
		t.Fatal("no switch must mean no models")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	var on atomic.Bool
	on.Store(true)
	cfg := AgentConfig{
		InstanceID: testInstance(), ConsentInterval: 5 * time.Millisecond, TerminalSession: factory, models: true, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready,
		Consent:       func() (bool, terminalproto.Readiness, error) { return true, terminalproto.Ready, nil },
		ModelsAllowed: func() bool { return on.Load() },
	}
	cfg.modelsAllowed = readModelsAllowed(cfg)
	if !capabilities(cfg).Models || capabilities(cfg).ModelsOff {
		t.Fatal("models on is not reported")
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
			if !ok || hello.Capabilities.Models != want || hello.Capabilities.ModelsOff != !want || !hello.Capabilities.TerminalEnabled {
				t.Fatalf("models %t: %+v", want, m)
			}
		case err := <-errs:
			t.Fatal(err)
		case <-time.After(5 * time.Second):
			t.Fatal("no capability update for models", want)
		}
	}
}

func TestModelsSwitchRollsBackAnInvalidCapability(t *testing.T) {
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
	var polls atomic.Int32
	blocked := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseBlocked := func() { releaseOnce.Do(func() { close(release) }) }
	cfg := AgentConfig{
		InstanceID: testInstance(), ConsentInterval: 5 * time.Millisecond, TerminalSession: factory, models: true, agentsV2: true, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready,
		Consent: func() (bool, terminalproto.Readiness, error) {
			if polls.Add(1) == 2 {
				close(blocked)
				<-release
			}
			return true, terminalproto.Ready, nil
		},
		ModelsAllowed: func() bool { return on.Load() },
	}
	cfg.modelsAllowed = false
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
		if !ok || !hello.Capabilities.Models || hello.Capabilities.ModelsOff || !hello.Capabilities.AgentsV2 {
			t.Fatalf("rolled-back switch not reported: %+v", m)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("the switch was not rolled back and retried")
	}
}
