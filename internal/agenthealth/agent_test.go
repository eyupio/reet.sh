package agenthealth

import (
	"context"
	"errors"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestBootstrapSendsEnrollmentHeadersAndUsesNoRedirects(t *testing.T) {
	var seen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/sync/private" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Reet-Token") != "token" || r.Header.Get("X-Reet-Agent") != "agent0123456789abcdef" || r.Header.Get("X-Reet-Agent-Key") != "secret" || r.Header.Get("X-Reet-Version") != "test" {
			http.Error(w, "missing headers", http.StatusUnauthorized)
			return
		}
		seen = true
		_, _ = w.Write([]byte(`{"protocolVersion":1,"address":"tc-capability"}`))
	}))
	defer server.Close()
	cfg := AgentConfig{
		ServerURL: server.URL, Token: "token", AgentID: "agent0123456789abcdef", Secret: "secret", Version: "test",
		HTTPClient: server.Client(),
	}
	address, err := bootstrap(context.Background(), cfg)
	if err != nil || address != "tc-capability" || !seen {
		t.Fatalf("bootstrap: %q %v seen=%t", address, err, seen)
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/api/v2/sync/private", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	cfg.ServerURL = redirect.URL
	cfg.HTTPClient = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	if _, err := bootstrap(context.Background(), cfg); err == nil {
		t.Fatal("bootstrap followed a redirect")
	}
}

func TestConsentReadFailureDisablesPreviouslyEnabledTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan terminalproto.Message, 1)
	cfg := AgentConfig{InstanceID: testInstance(), ConsentInterval: time.Millisecond, Consent: func() (bool, terminalproto.Readiness, error) {
		return true, terminalproto.Ready, errors.New("unreadable consent")
	}}
	go consentControl(ctx, cfg, 2, true, terminalproto.Ready, func(_ context.Context, m terminalproto.Message) error { sent <- m; return nil }, make(chan error, 3))
	select {
	case m := <-sent:
		hello := m.Body.(terminalproto.Hello)
		if hello.Capabilities.TerminalEnabled || hello.Capabilities.Readiness != terminalproto.DisabledLocally {
			t.Fatal("unreadable consent preserved access")
		}
	case <-time.After(time.Second):
		t.Fatal("local disable was not reported")
	}
}

type collectorFunc func(context.Context, string) Sample

func (f collectorFunc) Collect(ctx context.Context, home string) Sample { return f(ctx, home) }

func TestBoundedCollectorDoesNotAccumulateCallsOrReuseLateSample(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	c := &boundedCollector{busy: make(chan struct{}, 1), inner: collectorFunc(func(context.Context, string) Sample {
		close(started)
		<-release
		value := 99.0
		return Sample{CPUPercent: terminalproto.Reading{Value: &value}}
	})}
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	sample := c.Collect(ctx, "/")
	<-started
	if sample.CPUPercent.Value != nil {
		t.Fatal("expired sample was presented as current")
	}
	for i := 0; i < 100; i++ {
		if got := c.Collect(context.Background(), "/"); got.CPUPercent.Value != nil {
			t.Fatal("late sample reused")
		}
	}
}

func TestDisconnectedSessionJoinsWorkersWhileCollectorIsStalled(t *testing.T) {
	peers := make(chan *websocket.Conn, 1)
	finish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer peer.CloseNow()
		peers <- peer
		<-finish
	}))
	t.Cleanup(func() { close(finish); server.Close() })
	client, _, err := websocket.Dial(context.Background(), strings.Replace(server.URL, "http:", "ws:", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseNow()
	peer := <-peers
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	collector := &boundedCollector{busy: make(chan struct{}, 1), inner: collectorFunc(func(context.Context, string) Sample { close(started); <-release; return Sample{} })}
	cfg := AgentConfig{InstanceID: testInstance(), Collector: collector, CollectTimeout: time.Hour, ReportInterval: time.Hour}
	sequence := int64(0)
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		done <- runControlWorkers(ctx, cfg, client, 1, &sequence, func(ctx context.Context, _ terminalproto.Message) error { return ctx.Err() })
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}
	peer.CloseNow()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("old generation did not release its workers")
	}
	// Returning from runControlWorkers guarantees no worker can mutate this
	// counter while the next connection uses it (verified under -race).
	sequence++
}

func TestCapabilitiesAreHealthOnlyWithoutTerminalOptIn(t *testing.T) {
	caps := capabilities(AgentConfig{})
	if caps.TerminalEnabled || !caps.HealthEnabled || caps.Readiness != terminalproto.DisabledLocally {
		t.Fatalf("capabilities: %+v", caps)
	}
	if caps.ProtocolVersion != terminalproto.Version || caps.OSUser == "" {
		t.Fatalf("capabilities identity: %+v", caps)
	}
	if err := caps.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSampleHealthReportNormalisesEmptyReadings(t *testing.T) {
	report := (Sample{}).HealthReport("instance0123456789abcdef", 1)
	if report.Validate() != nil {
		t.Fatal(report.Validate())
	}
	if report.CPUPercent.Value != nil || report.CPUPercent.Reason != terminalproto.CollectionFailed {
		t.Fatalf("cpu: %+v", report.CPUPercent)
	}
	if report.Memory.Total.Value != nil || report.Memory.Total.Reason != terminalproto.CollectionFailed {
		t.Fatalf("memory: %+v", report.Memory)
	}
	if report.Scope != "host" || report.SampledAt == 0 {
		t.Fatalf("scope/time: %+v", report)
	}
}

func TestRunAgentRejectsUnsafeConfiguration(t *testing.T) {
	for _, cfg := range []AgentConfig{
		{ServerURL: "http://example.com", Token: "token", AgentID: "agent0123456789abcdef", Secret: "secret"},
		{ServerURL: "https://user:pass@example.com", Token: "token", AgentID: "agent0123456789abcdef", Secret: "secret"},
		{ServerURL: "https://example.com/path", Token: "token", AgentID: "agent0123456789abcdef", Secret: "secret"},
		{ServerURL: "https://example.com", AgentID: "agent0123456789abcdef", Secret: "secret"},
		{ServerURL: "https://example.com", Token: "token", AgentID: "short", Secret: "secret"},
	} {
		if err := RunAgent(context.Background(), cfg); err == nil {
			t.Fatalf("unsafe config accepted: %+v", cfg)
		}
	}
	if !strings.Contains(RunAgent(context.Background(), AgentConfig{}).Error(), "controller origin") {
		t.Fatal("missing configuration error was not descriptive")
	}
}

func TestConsentControlSendsUpdatedCapabilities(t *testing.T) {
	cfg := AgentConfig{
		InstanceID: testInstance(), ConsentInterval: 10 * time.Millisecond,
		Consent: func() (bool, terminalproto.Readiness, error) {
			return true, terminalproto.Ready, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan terminalproto.Message, 4)
	errs := make(chan error, 2)
	go consentControl(ctx, cfg, 3, false, terminalproto.DisabledLocally, func(ctx context.Context, message terminalproto.Message) error {
		sent <- message
		return nil
	}, errs)
	select {
	case message := <-sent:
		hello, ok := message.Body.(terminalproto.Hello)
		if !ok || !hello.Capabilities.TerminalEnabled || hello.Capabilities.Readiness != terminalproto.Ready || message.Generation != 3 {
			t.Fatalf("capability update: %+v", message)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("no capability update was sent")
	}
}
