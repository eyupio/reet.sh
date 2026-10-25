// Package oidctest is an identity provider for tests: discovery, a key set and
// a token endpoint, on a loopback port, which signs whatever a test asks it
// to. It exists so the code that signs people in can be exercised end to end
// without a real provider.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// ClientID is the audience of every token the server signs.
const ClientID = "client"

// Server is a running fake provider.
type Server struct {
	t      *testing.T
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	codes  map[string]string // authorisation code -> ID token
	Tokens int               // token endpoint calls
	// Issuer is the address the provider is configured at.
	Issuer string
}

// New starts a provider and stops it when the test ends.
func New(t *testing.T) *Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{t: t, key: key, codes: map[string]string{}}
	mux := http.NewServeMux()
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	s.Issuer = s.srv.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"issuer": s.Issuer, "authorization_endpoint": s.Issuer + "/authorize",
			"token_endpoint": s.Issuer + "/token", "jwks_uri": s.Issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		s.mu.Lock()
		s.Tokens++
		tok, ok := s.codes[r.PostForm.Get("code")]
		delete(s.codes, r.PostForm.Get("code")) // a code is worth one token
		s.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id_token": tok, "token_type": "Bearer", "access_token": "x"})
	})
	return s
}

func enc(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Sign signs claims as an RS256 token.
func (s *Server) Sign(claims map[string]any) string {
	s.t.Helper()
	signing := enc(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"}) + "." + enc(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	if err != nil {
		s.t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// Person is who a sign-in is for.
type Person struct {
	Subject, Email, Name, SessionID string
	EmailVerified                   bool
	// Groups is the groups claim; a nil slice leaves the claim out.
	Groups []string
}

// Complete plays the provider's part of a sign-in that began at authURL (the
// address the application redirected to): it reads the state and nonce out of
// it, prepares an ID token for person, and returns the query the provider
// would send the browser back with.
func (s *Server) Complete(authURL string, p Person) url.Values {
	s.t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		s.t.Fatal(err)
	}
	q := u.Query()
	now := time.Now()
	claims := map[string]any{
		"iss": s.Issuer, "aud": ClientID, "sub": p.Subject, "nonce": q.Get("nonce"),
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
		"email": p.Email, "email_verified": p.EmailVerified, "name": p.Name,
	}
	if p.SessionID != "" {
		claims["sid"] = p.SessionID
	}
	if p.Groups != nil {
		claims["groups"] = p.Groups
	}
	code := "code-" + q.Get("state")
	s.mu.Lock()
	s.codes[code] = s.Sign(claims)
	s.mu.Unlock()
	return url.Values{"code": {code}, "state": {q.Get("state")}}
}

// LogoutToken is a back-channel logout token for a subject and, if given, a
// provider session.
func (s *Server) LogoutToken(subject, sid, jti string) string {
	claims := map[string]any{
		"iss": s.Issuer, "aud": ClientID, "sub": subject, "jti": jti, "iat": time.Now().Unix(),
		"events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}},
	}
	if sid != "" {
		claims["sid"] = sid
	}
	if subject == "" {
		delete(claims, "sub")
	}
	return s.Sign(claims)
}
