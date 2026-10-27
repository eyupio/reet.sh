package oidc

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

const maxBody = 1 << 20

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// refuseAddress stops a connection to an address a discovery document could
// be steering the server at: the link-local range holds the cloud metadata
// service, and the unspecified address is this machine under another name.
// Private ranges are allowed, since an identity provider on a LAN is normal.
func refuseAddress(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return errors.New("the identity provider's address is not one this server may connect to")
	}
	return nil
}

// newHTTPClient is the one client every request to the provider uses: short
// deadline, no redirects followed, and the address guard above.
func newHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: refuseAddress}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 8 * time.Second,
			MaxIdleConns:          4,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// readLimited reads a response body of at most maxBody bytes.
func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBody {
		return nil, errors.New("the identity provider's answer is too large")
	}
	return b, nil
}

func (p *Provider) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errors.New("the identity provider could not be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("the identity provider answered with an error")
	}
	return readLimited(resp.Body)
}
