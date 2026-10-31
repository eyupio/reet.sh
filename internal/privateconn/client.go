package privateconn

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tailscale/tailcat"
)

// Origin is synthetic: the transport never resolves or dials this hostname.
const Origin = "http://reet-private.invalid"

var ErrAddress = errors.New("invalid private connection capability")

// Client opens private HTTP and WebSocket connections. Application credentials
// are still required on every request; knowledge of an address grants no role.
// Create one Client per connection generation. After transport loss, Close it
// and call NewClient again: an old WireGuard session can outlive the server's
// peer state, and upstream Ping may return a cached introduction acknowledgment.
type Client struct {
	node      *tailcat.Client
	transport *http.Transport
	http      *http.Client
	closed    atomic.Bool
	once      sync.Once
	err       error
}

func (*Client) String() string   { return "[private connection client]" }
func (*Client) GoString() string { return "[private connection client]" }

func NewClient(address string) (*Client, error) {
	if len(address) > maxIdentity || !strings.HasPrefix(address, "tc") {
		return nil, ErrAddress
	}
	info, err := tailcat.ParseAddr(tailcat.Addr(address))
	if err != nil || info.PresharedKey.IsZero() {
		return nil, ErrAddress
	}
	node := tailcat.NewClient(tailcat.Addr(address))
	node.Logf = func(string, ...any) {}
	c := &Client{node: node}
	c.transport = &http.Transport{
		// No ProxyFromEnvironment: this socket is always inside Tailcat.
		DialContext: c.dial,
		// Do not reuse an apparently live virtual TCP connection after the
		// controller has forgotten it. Connection-generation recovery must
		// also replace the native client, as documented on Client.
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxConnsPerHost:       16,
	}
	c.http = &http.Client{
		Transport: c.transport,
		// No overall timeout: WebSockets outlive an ordinary HTTP request.
		// Callers own request cancellation and stream heartbeats.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return c, nil
}

// HTTPClient is for requests to Origin and WebSocket DialOptions. Its transport
// must not be replaced; the client never falls back to ordinary networking.
func (c *Client) HTTPClient() *http.Client { return c.http }

// WithHeaderTimeout returns a copy of base whose transport waits up to
// timeout for response headers instead of the control-plane default. It is for
// a request the controller may legitimately take a long time to start
// answering, such as a cold artifact cache. The copy dials through the same
// private connection and, like base, never falls back to ordinary networking;
// a base that is not a private client is returned unchanged.
func WithHeaderTimeout(base *http.Client, timeout time.Duration) *http.Client {
	if base == nil {
		return nil
	}
	transport, ok := base.Transport.(*http.Transport)
	if !ok {
		return base
	}
	clone := transport.Clone()
	clone.ResponseHeaderTimeout = timeout
	copied := *base
	copied.Transport = clone
	return &copied
}

func (c *Client) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	if c.closed.Load() {
		return nil, net.ErrClosed
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Introduce this generation before dialing. A later Ping is not evidence
	// that an old server generation is still reachable, nor is it the UI's RTT.
	if _, err := c.node.Ping(ctx); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	conn, err := c.node.DialTCPPort(ctx, 80)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	if c.closed.Load() {
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (c *Client) Close() error {
	c.once.Do(func() {
		c.closed.Store(true)
		c.transport.CloseIdleConnections()
		c.err = c.node.Close()
	})
	return c.err
}
