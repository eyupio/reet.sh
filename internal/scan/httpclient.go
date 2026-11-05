package scan

import (
	"net/http"
	"time"
)

// Shared clients keep transports (and therefore TCP/TLS connections) alive
// across sync, report and import calls. A new http.Client per call makes every
// request pay a fresh handshake and leaves the previous idle connections
// unreachable until they time out.
var (
	httpClient = &http.Client{Timeout: 30 * time.Second}
	// noRedirectClient is used for requests that must observe a redirect
	// instead of following it.
	noRedirectClient = &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	longHTTPClient = &http.Client{Timeout: 2 * time.Minute}
)
