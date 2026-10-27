package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }

func rsaKey(t *testing.T) (*rsa.PrivateKey, error) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k, nil
}

// newRedirector sends discovery to a second address that holds a perfectly
// valid document for it, so only refusing the redirect makes the provider
// unreachable: a client that followed would be satisfied.
func newRedirector(t *testing.T) *httptest.Server {
	t.Helper()
	var s *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/real", http.StatusFound)
	})
	mux.HandleFunc("/real", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q}`,
			s.URL, s.URL+"/a", s.URL+"/t", s.URL+"/j")
	})
	s = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// newBigAnswer serves a valid document padded past the answer cap, so only
// the cap makes the provider unreachable.
func newBigAnswer(t *testing.T) *httptest.Server {
	t.Helper()
	var s *httptest.Server
	s = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q,"pad":%q}`,
			s.URL, s.URL+"/a", s.URL+"/t", s.URL+"/j", strings.Repeat("x", maxBody+10))
	}))
	t.Cleanup(s.Close)
	return s
}
