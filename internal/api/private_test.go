package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type fakePrivateTransport struct {
	address string
	err     error
	ensures int
}

func (f *fakePrivateTransport) Ensure(context.Context) (string, error) {
	f.ensures++
	return f.address, f.err
}

func (*fakePrivateTransport) Close() error { return nil }

func privateFixture(t *testing.T, mode AuthMode, token string) (*Server, *store.Store, store.SyncAgent, string, string) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	if _, err := st.CreateAPIToken(uid, "owner", auth.HashToken("fixture-owner")); err != nil {
		t.Fatal(err)
	}
	agent, secret, err := st.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(st, deadFetcher(t), token, WithAuthMode(mode))
	t.Cleanup(func() { srv.Close() })
	return srv, st, agent, secret, "fixture-owner"
}

func handshakeBody(t *testing.T, requestID string, caps terminalproto.Capabilities) string {
	t.Helper()
	data, err := terminalproto.EncodeHandshake(terminalproto.Handshake{
		ProtocolVersion: terminalproto.Version,
		RequestID:       requestID,
		AgentInstanceID: strings.Repeat("i", 32),
		Capabilities:    caps,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func healthOnlyCapabilities() terminalproto.Capabilities {
	return terminalproto.Capabilities{
		ProtocolVersion: 1, OS: "linux", OSUser: "tester",
		TerminalEnabled: false, HealthEnabled: true, Readiness: terminalproto.DisabledLocally,
	}
}

func privateRequest(t *testing.T, handler http.Handler, method, path, body, agentID, credential string, marked bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://private.invalid"+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Reet-Agent", agentID)
	req.Header.Set("X-Reet-Agent-Key", credential)
	req.Header.Set("X-Reet-Version", "test")
	if marked {
		req = req.WithContext(privateconn.WithPrivateTransport(req.Context()))
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestPrivateBootstrapReturnsCapabilityOnlyToTheOwningEnrollment(t *testing.T) {
	srv, st, agent, secret, ownerToken := privateFixture(t, AuthAccounts, "")
	fake := &fakePrivateTransport{address: "tc-private-capability-fixture"}
	srv.s.private = fake
	ts := httptest.NewServer(srv)
	defer ts.Close()
	call := func(token string, headers ...string) (int, string) {
		all := append([]string{"X-Reet-Token", token}, headers...)
		return doClient(t, ts.Client(), ts, "POST", "/api/v2/sync/private", `{}`, all...)
	}
	enrollment := []string{"X-Reet-Agent", agent.ID, "X-Reet-Agent-Key", secret}
	code, body := call(ownerToken, enrollment...)
	if code != 200 || !strings.Contains(body, fake.address) || fake.ensures != 1 {
		t.Fatalf("bootstrap: %d %s ensures=%d", code, body, fake.ensures)
	}
	code, _ = doClient(t, ts.Client(), ts, "GET", "/api/v2/sync/private", "", "X-Reet-Token", ownerToken)
	if code != http.StatusMethodNotAllowed {
		t.Fatalf("GET bootstrap status = %d", code)
	}
	if code, _ := call(ownerToken, "X-Reet-Agent", agent.ID, "X-Reet-Agent-Key", "wrong"); code != 403 {
		t.Fatalf("wrong enrollment secret status = %d", code)
	}
	if code, _ := call("", enrollment...); code != 401 {
		t.Fatalf("missing account token status = %d", code)
	}
	other, _ := st.CreateUser("other@remote.invalid", "Other", "member", "")
	if _, err := st.CreateAPIToken(other.ID, "other", auth.HashToken("fixture-other")); err != nil {
		t.Fatal(err)
	}
	if code, _ := call("fixture-other", enrollment...); code != 403 {
		t.Fatalf("cross-account bootstrap status = %d", code)
	}
	code, state := doClient(t, ts.Client(), ts, "GET", "/api/v2/sync", "", "X-Reet-Token", ownerToken)
	if code != 200 || strings.Contains(state, fake.address) {
		t.Fatal("browser machine state exposed the private capability")
	}
}

func TestPrivateBootstrapRefusesUnauthenticatedInstances(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  AuthMode
		token string
	}{
		{"none", AuthNone, ""},
		{"token without a token", AuthToken, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _, agent, secret, _ := privateFixture(t, tc.mode, tc.token)
			fake := &fakePrivateTransport{address: "tc-private-capability-fixture"}
			srv.s.private = fake
			ts := httptest.NewServer(srv)
			defer ts.Close()
			code, _ := doClient(t, ts.Client(), ts, "POST", "/api/v2/sync/private", `{}`,
				"X-Reet-Agent", agent.ID, "X-Reet-Agent-Key", secret)
			if code != 403 {
				t.Fatalf("disabled bootstrap status = %d", code)
			}
			if fake.ensures != 0 {
				t.Fatal("disabled bootstrap started the private listener")
			}
		})
	}
}

func TestPrivateBootstrapTokenModeUsesTheInstanceToken(t *testing.T) {
	srv, _, agent, secret, _ := privateFixture(t, AuthToken, "fixture-instance-token")
	srv.s.private = &fakePrivateTransport{address: "tc-token-mode-capability"}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	code, body := doClient(t, ts.Client(), ts, "POST", "/api/v2/sync/private", `{}`,
		"X-Reet-Token", "fixture-instance-token", "X-Reet-Agent", agent.ID, "X-Reet-Agent-Key", secret)
	if code != 200 || !strings.Contains(body, "tc-token-mode-capability") {
		t.Fatalf("token-mode bootstrap: %d %s", code, body)
	}
	if code, _ := doClient(t, ts.Client(), ts, "POST", "/api/v2/sync/private", `{}`,
		"X-Reet-Token", "wrong", "X-Reet-Agent", agent.ID, "X-Reet-Agent-Key", secret); code != 401 {
		t.Fatalf("wrong instance token status = %d", code)
	}
}

func TestPrivateBootstrapReportsMissingConfiguration(t *testing.T) {
	srv, _, agent, secret, ownerToken := privateFixture(t, AuthAccounts, "")
	ts := httptest.NewServer(srv)
	defer ts.Close()
	code, _ := doClient(t, ts.Client(), ts, "POST", "/api/v2/sync/private", `{}`,
		"X-Reet-Token", ownerToken, "X-Reet-Agent", agent.ID, "X-Reet-Agent-Key", secret)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured bootstrap status = %d", code)
	}
}

func TestPrivateBootstrapReportsRelayFailure(t *testing.T) {
	srv, _, agent, secret, ownerToken := privateFixture(t, AuthAccounts, "")
	srv.s.private = &fakePrivateTransport{err: errors.New("relay unavailable")}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	code, body := doClient(t, ts.Client(), ts, "POST", "/api/v2/sync/private", `{}`,
		"X-Reet-Token", ownerToken, "X-Reet-Agent", agent.ID, "X-Reet-Agent-Key", secret)
	if code != http.StatusServiceUnavailable || strings.Contains(body, "relay unavailable") {
		t.Fatalf("relay failure: %d %s", code, body)
	}
}

func TestPrivateHandlerRequiresListenerTransportAndExactRoute(t *testing.T) {
	srv, _, _, secret, _ := privateFixture(t, AuthAccounts, "")
	handler := srv.s.privateHandler()
	body := handshakeBody(t, strings.Repeat("r", 32), healthOnlyCapabilities())
	for _, tc := range []struct {
		name   string
		method string
		path   string
		marked bool
		code   int
	}{
		{"unmarked", "POST", "/private/v1/hello", false, 404},
		{"unknown route", "POST", "/private/v1/admin", true, 404},
		{"query string", "POST", "/private/v1/hello?token=secret", true, 404},
		{"wrong method", "GET", "/private/v1/hello", true, 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := privateRequest(t, handler, tc.method, tc.path, body, strings.Repeat("a", 32), secret, tc.marked)
			if rec.Code != tc.code {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.code, rec.Body.String())
			}
		})
	}
	spoofed := httptest.NewRequest("POST", "http://private.invalid/private/v1/hello", strings.NewReader(body))
	spoofed.Header.Set("Content-Type", "application/json")
	spoofed.Header.Set("X-Reet-Agent", strings.Repeat("a", 32))
	spoofed.Header.Set("X-Reet-Agent-Key", secret)
	spoofed.Header.Set("X-Reet-Transport", "private")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, spoofed)
	if rec.Code != 404 {
		t.Fatalf("header-marked request status = %d", rec.Code)
	}
}

func TestPrivateHelloAuthenticatesCapabilitiesAndFencesGenerations(t *testing.T) {
	srv, st, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	handler := srv.s.privateHandler()
	request := func(body, credential string) *httptest.ResponseRecorder {
		return privateRequest(t, handler, "POST", "/private/v1/hello", body, agent.ID, credential, true)
	}
	first := handshakeBody(t, strings.Repeat("r", 32), healthOnlyCapabilities())
	rec := request(first, secret)
	if rec.Code != 200 {
		t.Fatalf("health-only handshake: %d %s", rec.Code, rec.Body.String())
	}
	var reply terminalproto.HandshakeReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Generation != 1 || reply.RequestID != strings.Repeat("r", 32) {
		t.Fatalf("reply: %+v", reply)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("handshake response is cacheable")
	}
	uid, _ := st.DefaultOwner()
	caps, err := st.RemoteCapabilitiesFor(uid, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if caps.Generation != 1 || caps.Capabilities.TerminalEnabled || !caps.Capabilities.HealthEnabled || caps.InstanceID != strings.Repeat("i", 32) {
		t.Fatalf("stored capabilities: %+v", caps)
	}
	second := handshakeBody(t, strings.Repeat("s", 32), healthOnlyCapabilities())
	rec = request(second, secret)
	if rec.Code != 200 {
		t.Fatalf("second handshake: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || reply.Generation != 2 {
		t.Fatalf("second reply: %+v %v", reply, err)
	}
	if rec = request(handshakeBody(t, strings.Repeat("t", 32), healthOnlyCapabilities()), "wrong"); rec.Code != 403 {
		t.Fatalf("wrong secret status = %d", rec.Code)
	}
	if rec = request("{not json", secret); rec.Code != 400 {
		t.Fatalf("malformed body status = %d", rec.Code)
	}
	req := httptest.NewRequest("POST", "http://private.invalid/private/v1/hello", strings.NewReader(handshakeBody(t, strings.Repeat("v", 32), healthOnlyCapabilities())))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Reet-Agent", agent.ID)
	req.Header.Set("X-Reet-Agent-Key", secret)
	req = req.WithContext(privateconn.WithPrivateTransport(req.Context()))
	direct := httptest.NewRecorder()
	handler.ServeHTTP(direct, req)
	if direct.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content type status = %d", direct.Code)
	}
	if err := st.ControlSyncAgent(uid, agent.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if rec = request(handshakeBody(t, strings.Repeat("w", 32), healthOnlyCapabilities()), secret); rec.Code != 403 {
		t.Fatalf("revoked enrollment status = %d", rec.Code)
	}
}

func TestPrivateHelloRejectsDisabledInstancesAndOversizedBodies(t *testing.T) {
	srv, _, agent, secret, _ := privateFixture(t, AuthNone, "")
	handler := srv.s.privateHandler()
	body := handshakeBody(t, strings.Repeat("r", 32), healthOnlyCapabilities())
	rec := privateRequest(t, handler, "POST", "/private/v1/hello", body, agent.ID, secret, true)
	if rec.Code != 403 {
		t.Fatalf("disabled instance status = %d", rec.Code)
	}

	accounts, _, accountsAgent, accountsSecret, _ := privateFixture(t, AuthAccounts, "")
	accountsHandler := accounts.s.privateHandler()
	oversized := strings.Repeat("x", terminalproto.MaxControlBytes+1)
	rec = privateRequest(t, accountsHandler, "POST", "/private/v1/hello", oversized, accountsAgent.ID, accountsSecret, true)
	if rec.Code != 400 {
		t.Fatalf("oversized body status = %d", rec.Code)
	}
}
