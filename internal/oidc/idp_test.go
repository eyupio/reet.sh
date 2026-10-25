package oidc

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeIDP is an identity provider on a loopback port: discovery, a key set
// and a token endpoint that hands back whatever token the test asked for.
type fakeIDP struct {
	t       *testing.T
	srv     *httptest.Server
	rsaKey  *rsa.PrivateKey
	ecKey   *ecdsa.PrivateKey
	kid     string
	extra   map[string]*rsa.PrivateKey // published but not the signing key
	issuer  string
	mu      sync.Mutex
	hits    map[string]int
	idToken string // what the token endpoint returns next
	status  int    // token endpoint status; 0 is 200
	// discoveryIssuer overrides the issuer the document claims.
	discoveryIssuer string
	// jwksOverride replaces the key set document.
	jwksOverride func() any
	lastAuth     string
	lastForm     map[string][]string
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ek, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	f := &fakeIDP{t: t, rsaKey: rk, ecKey: ek, kid: "k1", extra: map[string]*rsa.PrivateKey{}, hits: map[string]int{}}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	f.issuer = f.srv.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		f.hit("discovery")
		iss := f.issuer
		if f.discoveryIssuer != "" {
			iss = f.discoveryIssuer
		}
		json.NewEncoder(w).Encode(map[string]string{
			"issuer": iss, "authorization_endpoint": f.srv.URL + "/authorize",
			"token_endpoint": f.srv.URL + "/token", "jwks_uri": f.srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		f.hit("jwks")
		if f.jwksOverride != nil {
			json.NewEncoder(w).Encode(f.jwksOverride())
			return
		}
		keys := []map[string]string{rsaJWK(f.kid, &f.rsaKey.PublicKey), ecJWK("ec1", &f.ecKey.PublicKey)}
		for kid, k := range f.extra {
			keys = append(keys, rsaJWK(kid, &k.PublicKey))
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.hit("token")
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastAuth = r.Header.Get("Authorization")
		r.ParseForm()
		f.lastForm = r.PostForm
		if f.status != 0 {
			w.WriteHeader(f.status)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "secret-echo"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id_token": f.idToken, "token_type": "Bearer", "access_token": "x"})
	})
	return f
}

func (f *fakeIDP) hit(name string) { f.mu.Lock(); f.hits[name]++; f.mu.Unlock() }
func (f *fakeIDP) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[name]
}

func rsaJWK(kid string, k *rsa.PublicKey) map[string]string {
	return map[string]string{"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes())}
}

func ecJWK(kid string, k *ecdsa.PublicKey) map[string]string {
	pad := func(b *big.Int) []byte { out := make([]byte, 32); b.FillBytes(out); return out }
	return map[string]string{"kty": "EC", "kid": kid, "use": "sig", "alg": "ES256", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(pad(k.X)), "y": base64.RawURLEncoding.EncodeToString(pad(k.Y))}
}

func enc(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

// token signs claims with the named algorithm and key id.
func (f *fakeIDP) token(alg, kid string, claims map[string]any) string {
	f.t.Helper()
	head := enc(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"})
	signing := head + "." + enc(claims)
	sum := sha256.Sum256([]byte(signing))
	var sig []byte
	switch alg {
	case "RS256":
		key := f.rsaKey
		if k, ok := f.extra[kid]; ok {
			key = k
		}
		s, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
		if err != nil {
			f.t.Fatal(err)
		}
		sig = s
	case "ES256":
		r, s, err := ecdsa.Sign(rand.Reader, f.ecKey, sum[:])
		if err != nil {
			f.t.Fatal(err)
		}
		sig = make([]byte, 64)
		r.FillBytes(sig[:32])
		s.FillBytes(sig[32:])
	case "HS256":
		// Signed with the public key's modulus as the secret: the forgery an
		// RS256-only verifier must refuse.
		m := hmac.New(sha256.New, f.rsaKey.PublicKey.N.Bytes())
		m.Write([]byte(signing))
		sig = m.Sum(nil)
	case "none":
		return signing + "."
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// idClaims are the claims of a good ID token for a provider whose client id is
// "client", before a test spoils one.
func (f *fakeIDP) idClaims(now time.Time, nonce string) map[string]any {
	return map[string]any{
		"iss": f.issuer, "aud": "client", "sub": "subject-1", "nonce": nonce,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
		"email": "ada@example.com", "email_verified": true, "name": "Ada Lovelace",
		"groups": []string{"reet-admin"}, "sid": "sess-1",
	}
}

// provider builds a Provider talking to the fake, with a clock the test owns.
func (f *fakeIDP) provider(mutate func(*Config)) (*Provider, *time.Time) {
	f.t.Helper()
	cfg := Config{
		Issuer: f.issuer, ClientID: "client", ClientSecret: "s3cret", RedirectURL: "http://127.0.0.1:9/api/v2/auth/oidc/callback",
		Scopes: []string{"openid", "profile", "email"}, DefaultRole: RoleViewer, PasswordLogin: true,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	if err := cfg.validate(); err != nil {
		f.t.Fatal(err)
	}
	p := newProvider(cfg)
	now := time.Now()
	p.now = func() time.Time { return now }
	return p, &now
}
