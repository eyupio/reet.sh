package gaffer

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"sync"
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
		return dialProviderAddresses(ctx, network, port, ips, local, dialer.DialContext)
	}
	return &http.Client{Transport: &guardedTransport{transport, local}}
}

// dialProviderAddresses preserves address pinning while giving both validated
// families a chance. Every racer is cancelled and joined before returning, and
// a late successful connection is closed rather than leaked.
func dialProviderAddresses(parent context.Context, network, port string, ips []netip.Addr, local bool, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	if err := checkAddresses(ips, local); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	type result struct {
		conn    net.Conn
		err     error
		primary bool
	}
	results := make(chan result, 2)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
		close(results)
		for r := range results {
			if r.conn != nil {
				r.conn.Close()
			}
		}
	}()
	groups := [2][]netip.Addr{}
	first4 := ips[0].Unmap().Is4()
	for _, ip := range ips {
		ip = ip.Unmap()
		if network == "tcp4" && !ip.Is4() || network == "tcp6" && ip.Is4() {
			continue
		}
		group := 0
		if ip.Is4() != first4 {
			group = 1
		}
		groups[group] = append(groups[group], ip)
	}
	if len(groups[0]) == 0 {
		groups[0], groups[1] = groups[1], nil
	}
	if len(groups[0]) == 0 {
		return nil, errors.New("the provider has no address for this network")
	}
	start := func(primary bool, addresses []netip.Addr) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			var failures []error
			for i, ip := range addresses {
				if err := ctx.Err(); err != nil {
					results <- result{err: err, primary: primary}
					return
				}
				deadline, _ := ctx.Deadline()
				remaining := time.Until(deadline)
				perAddress := remaining / time.Duration(len(addresses)-i)
				if perAddress < 2*time.Second {
					perAddress = min(2*time.Second, remaining)
				}
				attempt, stop := context.WithTimeout(ctx, perAddress)
				conn, err := dial(attempt, network, net.JoinHostPort(ip.String(), port))
				stop()
				if err == nil && ctx.Err() == nil {
					results <- result{conn: conn, primary: primary}
					return
				}
				if conn != nil {
					conn.Close()
				}
				if err == nil {
					err = ctx.Err()
				}
				failures = append(failures, err)
			}
			results <- result{err: errors.Join(failures...), primary: primary}
		}()
	}
	start(true, groups[0])
	fallbackStarted := false
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	var failures []error
	for {
		select {
		case r := <-results:
			if r.err == nil {
				if err := ctx.Err(); err != nil {
					r.conn.Close()
					return nil, err
				}
				return r.conn, nil
			}
			failures = append(failures, r.err)
			if len(groups[1]) == 0 || len(failures) == 2 {
				return nil, errors.Join(failures...)
			}
			if r.primary && !fallbackStarted {
				fallbackStarted = true
				timer.Stop()
				start(false, groups[1])
			}
		case <-timer.C:
			if len(groups[1]) > 0 && !fallbackStarted {
				fallbackStarted = true
				start(false, groups[1])
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
