package privateconn

import (
	"context"
	"net"
	"sync"

	"github.com/tailscale/tailcat"
)

// Listener admits only TCP port 80 inside the userspace Tailcat network.
// Its caller must supply an authenticated, allowlisted agent-only HTTP handler.
type Listener struct {
	listener net.Listener
	node     *tailcat.Server
	address  string
	mu       sync.Mutex
	closed   bool
	conns    map[*ownedConn]struct{}
	once     sync.Once
	err      error
}

// Listen restores the supplied identity. Closing ctx after startup does not
// close the listener: the owner must call Close during shutdown.
func Listen(ctx context.Context, identity *Identity) (*Listener, error) {
	if !identity.valid() {
		return nil, ErrIdentity
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	node := &tailcat.Server{
		Key: identity.key.Private, PresharedKey: identity.key.Public.PresharedKey,
		Region: identity.key.Public.Region[0],
		// Upstream diagnostics can contain connection capabilities.
		Logf: func(string, ...any) {},
	}
	ctx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()
	ln, err := node.Listen(ctx, "tcp", ":80")
	if err != nil {
		_ = node.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	return &Listener{
		listener: ln, node: node, address: string(node.TailcatAddr()),
		conns: make(map[*ownedConn]struct{}),
	}, nil
}

// Address is a secret capability for enrolled agents, never a display label.
func (l *Listener) Address() string { return l.address }
func (l *Listener) Addr() net.Addr  { return l.listener.Addr() }
func (*Listener) String() string    { return "[private connection listener]" }
func (*Listener) GoString() string  { return "[private connection listener]" }

func (l *Listener) Accept() (net.Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	c := &ownedConn{Conn: conn, owner: l}
	l.conns[c] = struct{}{}
	return c, nil
}

// Close releases pending accepts, accepted streams, and the network stack.
func (l *Listener) Close() error {
	l.once.Do(func() {
		l.mu.Lock()
		l.closed = true
		conns := make([]*ownedConn, 0, len(l.conns))
		for c := range l.conns {
			conns = append(conns, c)
		}
		l.mu.Unlock()
		_ = l.listener.Close()
		for _, c := range conns {
			_ = c.Close()
		}
		l.err = l.node.Close()
	})
	return l.err
}

type ownedConn struct {
	net.Conn
	owner *Listener
	once  sync.Once
	err   error
}

func (c *ownedConn) Close() error {
	c.once.Do(func() {
		c.err = c.Conn.Close()
		c.owner.mu.Lock()
		delete(c.owner.conns, c)
		c.owner.mu.Unlock()
	})
	return c.err
}
