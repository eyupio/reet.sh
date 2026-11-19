package skillfetch

// The address guard and the client that enforces it: skills are public, so
// nothing on this machine or a private network is a legitimate source.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"
)

var errAddress = errors.New("skill source address is not one this server may connect to")

// notPublic are the ranges, besides what net.IP can name, that a skill is never
// fetched from: "this network", carrier-grade NAT, IETF protocol assignments,
// benchmarking, reserved (including the broadcast address) and the local-use
// NAT64 prefix.
var notPublic = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "64:ff9b:1::/48"} {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}()

// publicAddress reports whether ip is one skills may be fetched from: not this
// machine, a private or link-local network, a reserved range or any other
// non-unicast address. An address that carries an IPv4 one (NAT64 64:ff9b::/96,
// 6to4 2002::/16, IPv4-compatible ::/96, SIIT ::ffff:0:0:0/96, Teredo
// 2001::/32) is public only if the IPv4 address inside it is. The dial
// guard and the check made before a request goes to a proxy share it.
func publicAddress(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, n := range notPublic {
		if n.Contains(ip) {
			return false
		}
	}
	if ip16 := ip.To16(); len(ip) == net.IPv6len && ip16 != nil {
		switch {
		case ip16[0] == 0x00 && ip16[1] == 0x64 && ip16[2] == 0xff && ip16[3] == 0x9b && allZero(ip16[4:12]):
			return publicAddress(net.IP(ip16[12:16])) // NAT64
		case ip16[0] == 0x20 && ip16[1] == 0x02:
			return publicAddress(net.IP(ip16[2:6])) // 6to4
		case allZero(ip16[:12]):
			return publicAddress(net.IP(ip16[12:16])) // IPv4-compatible, deprecated
		case allZero(ip16[:8]) && ip16[8] == 0xff && ip16[9] == 0xff && ip16[10] == 0 && ip16[11] == 0:
			return publicAddress(net.IP(ip16[12:16])) // SIIT ::ffff:0:a.b.c.d
		case ip16[0] == 0xfe && ip16[1]&0xc0 == 0xc0:
			return false // deprecated site-local fec0::/10
		case ip16[0] == 0x20 && ip16[1] == 0x01 && ip16[2] == 0 && ip16[3] == 0:
			// Teredo: the server's address, and the client's with every bit flipped.
			client := net.IPv4(^ip16[12], ^ip16[13], ^ip16[14], ^ip16[15])
			return publicAddress(net.IP(ip16[4:8])) && publicAddress(client)
		}
	}
	return true
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// refuseAddress is the dial-time guard. Skills are public, so unlike an
// identity provider nothing internal is a legitimate source.
func refuseAddress(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("skill source address is not an IP address")
	}
	if !publicAddress(ip) {
		return errAddress
	}
	return nil
}

func lookupIPs(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	ips := make([]net.IP, len(addrs))
	for i, a := range addrs {
		ips[i] = a.IP
	}
	return ips, err
}

// defaultClient honours the proxy settings of the environment, as the rest of
// reet does. Without a proxy the dial guard refuses non-public addresses. With
// one, the proxy itself is operator configuration and may be private, but the
// destination is resolved here first and refused unless every address is public;
// that runs for each redirect hop too, since each hop is a request of its own.
func defaultClient(proxy func(*http.Request) (*url.URL, error), lookup func(context.Context, string) ([]net.IP, error)) *http.Client {
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	if lookup == nil {
		lookup = lookupIPs
	}
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &guardedTransport{
			proxy:   proxy,
			lookup:  lookup,
			direct:  newTransport(&net.Dialer{Timeout: 5 * time.Second, Control: refuseAddress}, nil),
			proxied: map[string]*http.Transport{},
		},
	}
}

func newTransport(d *net.Dialer, proxy func(*http.Request) (*url.URL, error)) *http.Transport {
	return &http.Transport{
		Proxy:                 proxy,
		DialContext:           d.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	}
}

// guardedTransport sends each request either directly, through the dial guard,
// or through the proxy the environment names for it. The proxy is dialled by a
// transport that can reach nothing else (its proxy is fixed and its dialer is
// not guarded), so the exemption never applies to a direct connection, even to
// the same address.
type guardedTransport struct {
	proxy  func(*http.Request) (*url.URL, error)
	lookup func(context.Context, string) ([]net.IP, error)
	direct *http.Transport

	mu      sync.Mutex
	proxied map[string]*http.Transport
}

func (g *guardedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p, err := g.proxy(req)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return g.direct.RoundTrip(req)
	}
	if err := g.checkDestination(req); err != nil {
		return nil, err
	}
	return g.viaProxy(p).RoundTrip(req)
}

// checkDestination refuses a request that would go through a proxy to an
// address this server may not connect to, or to a name that has none.
func (g *guardedTransport) checkDestination(req *http.Request) error {
	host := req.URL.Hostname()
	ips := []net.IP{net.ParseIP(host)}
	if ips[0] == nil {
		var err error
		if ips, err = g.lookup(req.Context(), host); err != nil {
			return fmt.Errorf("resolve skill source: %w", err)
		}
	}
	if len(ips) == 0 {
		return errAddress
	}
	for _, ip := range ips {
		if !publicAddress(ip) {
			return errAddress
		}
	}
	return nil
}

func (g *guardedTransport) viaProxy(p *url.URL) *http.Transport {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := p.String()
	t := g.proxied[key]
	if t == nil {
		t = newTransport(&net.Dialer{Timeout: 5 * time.Second}, http.ProxyURL(p))
		g.proxied[key] = t
	}
	return t
}

func (g *guardedTransport) CloseIdleConnections() {
	g.direct.CloseIdleConnections()
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, t := range g.proxied {
		t.CloseIdleConnections()
	}
}
