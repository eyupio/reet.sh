package scan

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Shared clients keep transports (and therefore TCP/TLS connections) alive
// across sync, report and import calls. A new http.Client per call makes every
// request pay a fresh handshake and leaves the previous idle connections
// unreachable until they time out.
var (
	// httpClient carries the controller token and agent key, which Go keeps
	// on a redirect to another host, so it only follows redirects that stay
	// on the same scheme, host and port.
	httpClient = &http.Client{Timeout: 30 * time.Second, CheckRedirect: sameOriginRedirect}
	// noRedirectClient is used for requests that must observe a redirect
	// instead of following it.
	noRedirectClient = &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	// longHTTPClient downloads public release assets and sends no
	// credentials. GitHub redirects those to another host, so it follows
	// redirects as usual.
	longHTTPClient = &http.Client{Timeout: 2 * time.Minute}
)

// sameOriginRedirect refuses a redirect that leaves the address the request
// was first sent to.
func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	first := via[0].URL
	if req.URL.Scheme != first.Scheme || req.URL.Host != first.Host {
		return fmt.Errorf("refusing a redirect to another address (%s://%s); credentials are only sent to %s://%s", req.URL.Scheme, req.URL.Host, first.Scheme, first.Host)
	}
	return nil
}
