package main

import (
	"strings"
	"testing"
)

// A mistyped proxy entry stops the server at start-up, naming the entry,
// rather than leaving sign-in limits keyed on the proxy's address.
func TestServeRefusesABadTrustedProxyEntry(t *testing.T) {
	t.Setenv("REET_DATA_DIR", t.TempDir())
	t.Setenv("REET_TRUSTED_PROXIES", "10.0.0.0/8, proxy.internal")
	err := serve()
	if err == nil || !strings.Contains(err.Error(), "proxy.internal") {
		t.Fatalf("serve() = %v, want an error naming the entry", err)
	}
}
