package gaffer

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestGafferProviderLocalhostReachesASingleFamilyListener(t *testing.T) {
	ips, err := net.DefaultResolver.LookupNetIP(t.Context(), "ip", "localhost")
	if err != nil {
		t.Fatal(err)
	}
	has4, has6 := false, false
	for _, ip := range ips {
		if ip.Unmap().Is4() {
			has4 = true
		} else {
			has6 = true
		}
	}
	if !has4 || !has6 {
		t.Skip("localhost does not resolve to both address families")
	}
	family, address := "tcp4", "127.0.0.1:0"
	if ips[0].Unmap().Is4() {
		family, address = "tcp6", "[::1]:0"
	}
	listener, err := net.Listen(family, address)
	if err != nil {
		t.Skipf("alternate loopback family unavailable: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "reachable") }))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	client := newProviderClient(true)
	defer client.CloseIdleConnections()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	request, err := http.NewRequestWithContext(t.Context(), "GET", "http://localhost:"+port, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("localhost must fall back to the listening address family: %v (resolved %v)", err, ips)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "reachable" {
		t.Fatalf("body %q", body)
	}
}

func TestGafferProviderTriesTheNextValidatedAddress(t *testing.T) {
	calls := []string{}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	conn, err := dialProviderAddresses(t.Context(), "tcp", "1234", []netip.Addr{netip.MustParseAddr("127.0.0.2"), netip.MustParseAddr("127.0.0.1")}, true, func(ctx context.Context, network, address string) (net.Conn, error) {
		calls = append(calls, address)
		if len(calls) == 1 {
			return nil, errors.New("refused")
		}
		return client, nil
	})
	if err != nil || conn != client || len(calls) != 2 {
		t.Fatalf("fallback: %v %v calls=%v", conn, err, calls)
	}
}
func TestGafferProviderFallbackDoesNotWaitForABlackholedFamily(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		firstStopped := make(chan struct{})
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		began := time.Now()
		conn, err := dialProviderAddresses(t.Context(), "tcp", "1234", []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}, true, func(ctx context.Context, network, address string) (net.Conn, error) {
			if address == "[::1]:1234" {
				defer close(firstStopped)
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return client, nil
		})
		if err != nil || conn != client || time.Since(began) != 300*time.Millisecond {
			t.Fatalf("dual stack: %v elapsed=%v", err, time.Since(began))
		}
		select {
		case <-firstStopped:
		default:
			t.Fatal("losing dial was not joined")
		}
	})
}
func TestGafferProviderNeverFallsBackOutsideTheAllowedNetwork(t *testing.T) {
	for _, ips := range [][]netip.Addr{
		{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("1.1.1.1")},
		{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("169.254.169.254")},
	} {
		called := false
		conn, err := dialProviderAddresses(t.Context(), "tcp", "80", ips, true, func(context.Context, string, string) (net.Conn, error) {
			called = true
			return nil, errors.New("must not dial")
		})
		if err == nil || conn != nil || called {
			t.Fatal("an invalid DNS set was dialled")
		}
	}
}

type trackedProviderConn struct {
	net.Conn
	closed atomic.Bool
}

func (c *trackedProviderConn) Close() error { c.closed.Store(true); return c.Conn.Close() }
func TestGafferProviderClosesALateSuccessfulDial(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		connections := []*trackedProviderConn{}
		partners := []net.Conn{}
		both := make(chan struct{})
		conn, err := dialProviderAddresses(t.Context(), "tcp", "1234", []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}, true, func(ctx context.Context, network, address string) (net.Conn, error) {
			a, b := net.Pipe()
			c := &trackedProviderConn{Conn: a}
			mu.Lock()
			connections = append(connections, c)
			partners = append(partners, b)
			if len(connections) == 2 {
				close(both)
			}
			mu.Unlock()
			<-both
			return c, nil
		})
		defer func() {
			for _, c := range connections {
				c.Close()
			}
			for _, p := range partners {
				p.Close()
			}
		}()
		if err != nil || len(connections) != 2 {
			t.Fatalf("dial result %v, attempts %d", err, len(connections))
		}
		for _, c := range connections {
			if c == conn && c.closed.Load() {
				t.Fatal("winning connection was closed")
			}
			if c != conn && !c.closed.Load() {
				t.Fatal("late connection leaked")
			}
		}
	})
}

func TestGafferProviderAllAddressAttemptsShareOneDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		began := time.Now()
		conn, err := dialProviderAddresses(t.Context(), "tcp", "1234", []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}, true, func(ctx context.Context, network, address string) (net.Conn, error) {
			calls.Add(1)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		if conn != nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(began) != 10*time.Second || calls.Load() != 2 {
			t.Fatalf("deadline: conn=%v err=%v elapsed=%v calls=%d", conn, err, time.Since(began), calls.Load())
		}
	})
}
func TestGafferProviderCancelledDialDoesNotAttemptAnotherAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var calls atomic.Int32
	conn, err := dialProviderAddresses(ctx, "tcp", "1234", []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}, true, func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("must not dial")
	})
	if conn != nil || !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("cancel: %v calls=%d", err, calls.Load())
	}
}
