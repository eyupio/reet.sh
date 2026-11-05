package gaffer

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"time"
)

var tailnet = netip.MustParsePrefix("100.64.0.0/10")

func checkAddresses(ips []netip.Addr, local bool) error {
	if len(ips) == 0 {
		return errors.New("the provider address did not resolve")
	}
	for _, ip := range ips {
		ip = ip.Unmap()
		private := ip.IsLoopback() || ip.IsPrivate() || tailnet.Contains(ip)
		metadata := ip.String() == "100.100.100.200" || ip.String() == "fd00:ec2::254"
		if metadata || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() || (!ip.IsGlobalUnicast() && !ip.IsLoopback()) || (local && !private) || (!local && private) {
			return errors.New("the provider address is outside its allowed network; check the address and Local network only setting")
		}
	}
	return nil
}

func resolved(ctx context.Context, host string, local bool) ([]netip.Addr, error) {
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		var err error
		ips, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, errors.New("the provider address could not be resolved")
		}
	}
	if err := checkAddresses(ips, local); err != nil {
		return nil, err
	}
	return ips, nil
}

type guardedTransport struct {
	transport *http.Transport
	local     bool
}

type proxyAddressKey struct{}

func (t *guardedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if _, err := resolved(r.Context(), r.URL.Hostname(), t.local); err != nil {
		return nil, err
	}
	if t.transport.Proxy != nil {
		proxy, err := t.transport.Proxy(r)
		if err != nil {
			return nil, err
		}
		if proxy != nil {
			port := proxy.Port()
			if port == "" {
				port = "80"
				if proxy.Scheme == "https" {
					port = "443"
				}
			}
			address := net.JoinHostPort(proxy.Hostname(), port)
			r = r.Clone(context.WithValue(r.Context(), proxyAddressKey{}, address))
		}
	}
	return t.transport.RoundTrip(r)
}
func (t *guardedTransport) CloseIdleConnections() { t.transport.CloseIdleConnections() }

func newProviderClient(local bool) *http.Client {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 15 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	if local {
		transport.Proxy = nil
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		// Hosted requests may use the deployment's configured forward proxy.
		// The target itself is checked in RoundTrip; for direct/local requests,
		// resolve and pin the actual destination again at connection time.
		if proxy, _ := ctx.Value(proxyAddressKey{}).(string); !local && proxy != "" && address == proxy {
			return dialer.DialContext(ctx, network, address)
		}
		ips, err := resolved(ctx, host, local)
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return &http.Client{Transport: &guardedTransport{transport, local}}
}
