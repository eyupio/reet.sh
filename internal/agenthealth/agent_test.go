package agenthealth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
