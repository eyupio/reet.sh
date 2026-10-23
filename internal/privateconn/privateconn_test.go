package privateconn

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/sealed"
	"tailscale.com/derp/derpserver"
	"tailscale.com/envknob"
	"tailscale.com/net/netcheck"
	"tailscale.com/syncs"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

func TestMain(m *testing.M) {
	// Keep upstream netcheck from probing the gateway or captive portals.
	// Force the actual encrypted data path through the local DERP fixture.
	envknob.Setenv("IN_TS_TEST", "true")
	// Keep real UDP sockets so upstream's socket-rebind shutdown works;
	// suppress direct discovery/data instead of installing dummy sockets.
	envknob.Setenv("TS_DEBUG_ALWAYS_USE_DERP", "false")
	envknob.Setenv("TS_DEBUG_NEVER_DIRECT_UDP", "true")
	netcheck.HookStartCaptivePortalDetection.SetForTest(func(context.Context, *netcheck.Client, *tailcfg.DERPMap, tailcfg.DERPRegionID, func(bool)) (<-chan struct{}, func()) {
		return syncs.ClosedChan(), func() {}
	})
	os.Exit(m.Run())
}

func testRelay(t *testing.T) *tailcfg.DERPRegion {
	t.Helper()
	d := derpserver.New(key.NewNode(), func(string, ...any) {})
	srv := httptest.NewUnstartedServer(derpserver.Handler(d))
	srv.Config.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	srv.StartTLS()
	t.Cleanup(func() {
		_ = d.Close()
		srv.Close()
	})
	return &tailcfg.DERPRegion{
		RegionID: 1, RegionCode: "local-test",
		Nodes: []*tailcfg.DERPNode{{
			Name: "local-test", RegionID: 1, HostName: "127.0.0.1",
			IPv4: "127.0.0.1", IPv6: "none", STUNPort: -1,
			DERPPort:         srv.Listener.Addr().(*net.TCPAddr).Port,
			InsecureForTests: true,
		}},
	}
}

func testIdentity(t *testing.T) *Identity {
	t.Helper()
	i, err := identityForRegion(testRelay(t))
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func testSealer(t *testing.T, material string) *sealed.Sealer {
	t.Helper()
	s, err := sealed.NewSealer(strings.Repeat(material, 32), "test")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func serve(t *testing.T, identity *Identity, handler http.Handler) *Listener {
	t.Helper()
	l, err := Listen(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(l)
	}()
	t.Cleanup(func() {
		_ = l.Close()
		_ = srv.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("private HTTP server did not stop")
		}
	})
	return l
}

const fixtureCredential = "Bearer isolated-agent-fixture"

func agentFixture(redirect string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fixtureCredential {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/agent/ping":
			_, _ = io.WriteString(w, "ready")
		case "/agent/redirect":
			http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
		case "/agent/stream":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()
			conn.SetReadLimit(32 << 10)
			for {
				kind, b, err := conn.Read(r.Context())
				if err != nil {
					return
				}
				if conn.Write(r.Context(), kind, b) != nil {
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	})
}

func request(t *testing.T, c *Client, path, credential string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Origin+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", credential)
	resp, err := c.HTTPClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestEncryptedRelayHTTPAndWebSocket(t *testing.T) {
	var escaped atomic.Int32
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escaped.Add(1)
		http.Error(w, "traffic escaped private transport", 500)
	}))
	defer trap.Close()
	t.Setenv("HTTP_PROXY", trap.URL)
	t.Setenv("HTTPS_PROXY", trap.URL)
	t.Setenv("NO_PROXY", "")
	l := serve(t, testIdentity(t), agentFixture(trap.URL))
	client, err := NewClient(l.Address())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if strings.Contains(fmt.Sprintf("%+v %#v %+v %#v", l, l, client, client), l.Address()) {
		t.Fatal("transport formatting exposed the capability")
	}

	if got := request(t, client, "/agent/ping", "").StatusCode; got != 401 {
		t.Fatalf("unauthenticated request status = %d", got)
	}
	if got := request(t, client, "/admin", fixtureCredential).StatusCode; got != 404 {
		t.Fatalf("unlisted fixture route status = %d", got)
	}
	resp := request(t, client, "/agent/ping", fixtureCredential)
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != 200 || string(body) != "ready" {
		t.Fatalf("authenticated response: status=%d body=%q err=%v", resp.StatusCode, body, err)
	}
	if got := request(t, client, "/agent/redirect", fixtureCredential).StatusCode; got != 307 {
		t.Fatalf("redirect was followed: status=%d", got)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, Origin+"/agent/stream", &websocket.DialOptions{HTTPClient: client.HTTPClient()})
	if conn != nil {
		conn.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != 401 {
		t.Fatal("unauthenticated WebSocket was not rejected")
	}
	conn, _, err = websocket.Dial(ctx, Origin+"/agent/stream", &websocket.DialOptions{
		HTTPClient: client.HTTPClient(),
		HTTPHeader: http.Header{"Authorization": {fixtureCredential}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	payload := []byte("\x00\x1b[31mprivate UTF-8: \xe7\x8c\xab\xff")
	if err := conn.Write(ctx, websocket.MessageBinary, payload); err != nil {
		t.Fatal(err)
	}
	kind, echoed, err := conn.Read(ctx)
	if err != nil || kind != websocket.MessageBinary || !bytes.Equal(echoed, payload) {
		t.Fatalf("binary relay echo: kind=%d err=%v equal=%t", kind, err, bytes.Equal(echoed, payload))
	}
	// Confirm that forced-relay testing actually selected a relay path.
	state := l.node.Status()
	if len(state.Peer) == 0 {
		t.Fatal("no authenticated transport peer")
	}
	for _, peer := range state.Peer {
		if peer.CurAddr != "" || peer.Relay == "" {
			t.Fatal("fixture did not force the relayed data path")
		}
	}
	if escaped.Load() != 0 {
		t.Fatal("a proxy or redirect received private request traffic")
	}
}

func TestNewConnectionRecoversAfterSealedIdentityRestart(t *testing.T) {
	identity := testIdentity(t)
	sealer := testSealer(t, "a")
	ciphertext, err := identity.Seal(sealer)
	if err != nil {
		t.Fatal(err)
	}
	first := serve(t, identity, agentFixture(""))
	address := first.Address()
	client, err := NewClient(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if request(t, client, "/agent/ping", fixtureCredential).StatusCode != 200 {
		t.Fatal("initial request failed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	control, _, err := websocket.Dial(ctx, Origin+"/agent/stream", &websocket.DialOptions{
		HTTPClient: client.HTTPClient(), HTTPHeader: http.Header{"Authorization": {fixtureCredential}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer control.CloseNow()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := control.Read(ctx); err == nil {
		t.Fatal("controller shutdown did not interrupt the old control connection")
	}
	// Transport loss ends a connection generation, not the enrollment or a
	// future tmux session. Upstream's cached peer state must be discarded.
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenIdentity(sealer, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	second := serve(t, restored, agentFixture(""))
	if second.Address() != address {
		t.Fatal("restoring the identity changed its capability")
	}
	reconnected, err := NewClient(address)
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	if request(t, reconnected, "/agent/ping", fixtureCredential).StatusCode != 200 {
		t.Fatal("fresh authenticated connection did not recover")
	}
}

func TestIdentityFailsClosed(t *testing.T) {
	identity := testIdentity(t)
	sealer := testSealer(t, "a")
	ciphertext, err := identity.Seal(sealer)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		s    *sealed.Sealer
		b    string
	}{
		{"missing key", nil, ciphertext},
		{"wrong key", testSealer(t, "b"), ciphertext},
		{"empty", sealer, ""},
		{"damaged", sealer, ciphertext[:len(ciphertext)/2]},
		{"oversized", sealer, strings.Repeat("x", maxIdentity+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := OpenIdentity(tc.s, tc.b); !errors.Is(err, ErrIdentity) {
				t.Fatalf("got %v, want identity error", err)
			}
		})
	}
	wrongPurpose, err := sealer.SealSecret(`{"format":"another-purpose","key":null}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenIdentity(sealer, wrongPurpose); !errors.Is(err, ErrIdentity) {
		t.Fatal("wrong-purpose identity accepted")
	}
	if got := fmt.Sprintf("%+v %#v", identity, identity); strings.Contains(got, "tc") || strings.Contains(got, "Private:") {
		t.Fatal("identity formatting exposed key material")
	}
	if _, err := NewClient("https://untrusted.example/private-capability"); !errors.Is(err, ErrAddress) {
		t.Fatal("non-Tailcat address accepted")
	}
	withoutPSK := identity.key.Public
	withoutPSK.PresharedKey = [32]byte{}
	if _, err := NewClient(string(withoutPSK.Addr())); !errors.Is(err, ErrAddress) {
		t.Fatal("address without pre-shared key accepted")
	}
}

func TestCancelledSetupDoesNotStartListener(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Listen(ctx, testIdentity(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Listen: %v", err)
	}
	if _, err := NewIdentity(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("NewIdentity: %v", err)
	}
}

func TestListenerCloseUnblocksAcceptAndStream(t *testing.T) {
	stopped := make(chan struct{})
	handler := agentFixture("")
	l := serve(t, testIdentity(t), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(stopped)
		handler.ServeHTTP(w, r)
	}))
	client, err := NewClient(l.Address())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, Origin+"/agent/stream", &websocket.DialOptions{
		HTTPClient: client.HTTPClient(), HTTPHeader: http.Header{"Authorization": {fixtureCredential}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed listener accepted: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("closing listener did not release its local stream handler")
	}
	l.mu.Lock()
	remaining := len(l.conns)
	l.mu.Unlock()
	if remaining != 0 {
		t.Fatal("accepted streams remain registered after shutdown")
	}
	// A vanished WireGuard peer need not deliver TCP FIN to its remote end.
	// The agent's application heartbeat/lease must detect that side's loss.
	if err := l.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestUnavailableRelayHonoursRequestDeadline(t *testing.T) {
	identity := testIdentity(t)
	unused, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	identity.key.Public.Region[0].Nodes[0].DERPPort = unused.Addr().(*net.TCPAddr).Port
	_ = unused.Close()
	l := serve(t, identity, agentFixture(""))
	address := l.Address()
	client, err := NewClient(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, Origin+"/agent/ping", nil)
	start := time.Now()
	resp, err := client.HTTPClient().Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unavailable relay: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("private dial exceeded the request deadline")
	}
	if strings.Contains(err.Error(), address) {
		t.Fatal("dial error exposed the capability")
	}
}
