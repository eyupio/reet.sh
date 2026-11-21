package objectstore

// The address guard for a destination a member described. The operator's own
// destination may be MinIO on the same machine; a member's may not be a way
// to reach this machine or the network behind it.
//
// publicAddress is the check internal/skillfetch makes, copied because that
// one is not exported.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// ErrAddress is a destination at an address a member's copy may not go to.
var ErrAddress = errors.New("the destination is on this machine or a private network; only the instance's own destinations may be")

// notPublic are the ranges, besides what net.IP can name, that are not the
// public internet: "this network", carrier-grade NAT, IETF protocol
// assignments, benchmarking, reserved and the local-use NAT64 prefix.
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

// PublicAddress reports whether ip is on the public internet: not this
// machine, a private, unique-local or link-local network, a reserved range or
// a non-unicast address. An address that carries an IPv4 one is public only
// if the IPv4 address inside it is.
func PublicAddress(ip net.IP) bool {
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
			return PublicAddress(net.IP(ip16[12:16])) // NAT64
		case ip16[0] == 0x20 && ip16[1] == 0x02:
			return PublicAddress(net.IP(ip16[2:6])) // 6to4
		case allZero(ip16[:12]):
			return PublicAddress(net.IP(ip16[12:16])) // IPv4-compatible, deprecated
		case allZero(ip16[:8]) && ip16[8] == 0xff && ip16[9] == 0xff && ip16[10] == 0 && ip16[11] == 0:
			return PublicAddress(net.IP(ip16[12:16])) // SIIT ::ffff:0:a.b.c.d
		case ip16[0] == 0xfe && ip16[1]&0xc0 == 0xc0:
			return false // deprecated site-local fec0::/10
		case ip16[0] == 0x20 && ip16[1] == 0x01 && ip16[2] == 0 && ip16[3] == 0:
			// Teredo: the server's address, and the client's with every bit flipped.
			client := net.IPv4(^ip16[12], ^ip16[13], ^ip16[14], ^ip16[15])
			return PublicAddress(net.IP(ip16[4:8])) && PublicAddress(client)
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

// refuseAddress runs as a connection is made, on the address the name
// resolved to, so a name that points inward is refused as an address would be.
func refuseAddress(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !PublicAddress(ip) {
		return ErrAddress
	}
	return nil
}

// publicTransport dials through the guard and uses no proxy.
var publicTransport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   refuseAddress,
	}).DialContext,
	TLSHandshakeTimeout:   15 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second,
	MaxIdleConnsPerHost:   2,
}

// guarded sends a request straight out through the dial guard, or, when the
// environment names a proxy for it, through that proxy once every address the
// destination resolves to is public. The proxy is the operator's and may be
// private; the destination may not.
type guarded struct {
	proxy  func(*http.Request) (*url.URL, error)
	lookup func(context.Context, string) ([]net.IPAddr, error)
}

func (g guarded) RoundTrip(req *http.Request) (*http.Response, error) {
	p, err := g.proxy(req)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return publicTransport.RoundTrip(req)
	}
	host := req.URL.Hostname()
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		addrs, err := g.lookup(req.Context(), host)
		if err != nil {
			return nil, fmt.Errorf("resolve the destination: %w", err)
		}
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
	}
	if len(ips) == 0 {
		return nil, ErrAddress
	}
	for _, ip := range ips {
		if !PublicAddress(ip) {
			return nil, ErrAddress
		}
	}
	return s3Transport.RoundTrip(req)
}

func httpClient(publicOnly bool) *http.Client {
	if !publicOnly {
		return &http.Client{Transport: s3Transport}
	}
	return &http.Client{
		Transport: guarded{proxy: http.ProxyFromEnvironment, lookup: net.DefaultResolver.LookupIPAddr},
		// A redirect could name an address the guard was never asked about
		// when a proxy is in the way, and S3 has no use for one.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
