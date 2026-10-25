package oidc

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func goodEnv() map[string]string {
	return map[string]string{
		"REET_OIDC_ISSUER": "https://idp.example.test/", "REET_OIDC_CLIENT_ID": "client",
		"REET_OIDC_CLIENT_SECRET": "s3cret", "REET_OIDC_REDIRECT_URL": "https://reet.example.test/api/v2/auth/oidc/callback",
	}
}

func TestNothingSetMeansNoSingleSignOn(t *testing.T) {
	p, err := FromEnv(env(nil))
	if p != nil || err != nil {
		t.Fatalf("FromEnv with nothing set = %v, %v; want nil, nil", p, err)
	}
	e := goodEnv()
	e["REET_OIDC"] = "off"
	if p, err := FromEnv(env(e)); p != nil || err != nil {
		t.Fatalf("switched off = %v, %v", p, err)
	}
}

func TestAHalfSetConfigurationIsAnErrorNotSilence(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(map[string]string)
		want   string
	}{
		"no client id":     {func(m map[string]string) { delete(m, "REET_OIDC_CLIENT_ID") }, "REET_OIDC_CLIENT_ID"},
		"no secret":        {func(m map[string]string) { delete(m, "REET_OIDC_CLIENT_SECRET") }, "CLIENT_SECRET"},
		"issuer over http": {func(m map[string]string) { m["REET_OIDC_ISSUER"] = "http://idp.example.test" }, "https"},
		"no redirect":      {func(m map[string]string) { delete(m, "REET_OIDC_REDIRECT_URL") }, "REET_PUBLIC_URL"},
		"bad default role": {func(m map[string]string) { m["REET_OIDC_DEFAULT_ROLE"] = "owner" }, "member or viewer"},
		"bad flag":         {func(m map[string]string) { m["REET_OIDC_AUTO_CREATE"] = "sometimes" }, "on or off"},
	} {
		t.Run(name, func(t *testing.T) {
			e := goodEnv()
			tc.mutate(e)
			p, err := FromEnv(env(e))
			if p != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("FromEnv = %v, %v; want an error naming %q", p, err, tc.want)
			}
		})
	}
}

func TestBothTheValueAndItsFileIsRefusedAndAFileIsRead(t *testing.T) {
	e := goodEnv()
	e["REET_OIDC_CLIENT_SECRET_FILE"] = "/run/secret"
	if _, err := FromEnv(env(e)); err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("both set = %v", err)
	}
	delete(e, "REET_OIDC_CLIENT_SECRET")
	readFile = func(string) ([]byte, error) { return []byte("from-file\n"), nil }
	defer func() { readFile = nil }()
	p, err := FromEnv(env(e))
	if err != nil {
		t.Fatal(err)
	}
	if string(p.cfg.ClientSecret) != "from-file" {
		t.Fatalf("secret = %q", string(p.cfg.ClientSecret))
	}
}

func TestTheRedirectIsWorkedOutFromThePublicAddressAndLoopbackHTTPIsAllowed(t *testing.T) {
	e := goodEnv()
	delete(e, "REET_OIDC_REDIRECT_URL")
	e["REET_PUBLIC_URL"] = "https://reet.example.test/"
	p, err := FromEnv(env(e))
	if err != nil {
		t.Fatal(err)
	}
	if p.RedirectURL() != "https://reet.example.test/api/v2/auth/oidc/callback" {
		t.Fatalf("redirect = %q", p.RedirectURL())
	}
	e["REET_OIDC_ISSUER"] = "http://127.0.0.1:5556"
	if _, err := FromEnv(env(e)); err != nil {
		t.Fatalf("a loopback http issuer was refused: %v", err)
	}
}

func TestTheSecretNeverFormatsIntoALogLine(t *testing.T) {
	p, err := FromEnv(env(goodEnv()))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{fmtV(p.Config()), fmtV(&p.cfg), fmtV(p.cfg.ClientSecret)} {
		if strings.Contains(s, "s3cret") {
			t.Fatalf("the secret reached a formatted value: %s", s)
		}
	}
}

func fmtV(v any) string {
	return strings.Join([]string{sprintf("%v", v), sprintf("%+v", v), sprintf("%#v", v)}, " ")
}

func TestDiscoveryMustNameTheConfiguredIssuer(t *testing.T) {
	f := newFakeIDP(t)
	f.discoveryIssuer = "https://elsewhere.example.test"
	p, _ := f.provider(nil)
	_, err := p.AuthURL(context.Background(), "s", "n", "v")
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("AuthURL with a mismatched issuer = %v, want ErrUnreachable", err)
	}
}

func TestDiscoveryRefusesInsecureAndOversizedAndRedirectedAnswers(t *testing.T) {
	f := newFakeIDP(t)
	p, _ := f.provider(nil)
	// An endpoint on plain http that is not loopback.
	if err := checkURL("http://idp.example.test/token", "token_endpoint"); err == nil {
		t.Fatal("a non-loopback http endpoint was accepted")
	}
	// A redirect is not followed.
	mux := newRedirector(t)
	p2 := newProvider(Config{Issuer: mux.URL, ClientID: "client", ClientSecret: "x", RedirectURL: "http://127.0.0.1/cb", Scopes: []string{"openid"}, DefaultRole: RoleViewer})
	if _, err := p2.AuthURL(context.Background(), "s", "n", "v"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a redirecting issuer = %v", err)
	}
	// An answer over the cap is refused.
	big := newBigAnswer(t)
	p3 := newProvider(Config{Issuer: big.URL, ClientID: "client", ClientSecret: "x", RedirectURL: "http://127.0.0.1/cb", Scopes: []string{"openid"}, DefaultRole: RoleViewer})
	if _, err := p3.AuthURL(context.Background(), "s", "n", "v"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("an oversized answer = %v", err)
	}
	_ = p
}

func TestTheAuthorisationAddressCarriesStateNonceAndAPKCEChallenge(t *testing.T) {
	f := newFakeIDP(t)
	p, _ := f.provider(nil)
	verifier := Random(32)
	raw, err := p.AuthURL(context.Background(), "the-state", "the-nonce", verifier)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	for k, want := range map[string]string{
		"response_type": "code", "client_id": "client", "state": "the-state", "nonce": "the-nonce",
		"code_challenge": Challenge(verifier), "code_challenge_method": "S256",
		"redirect_uri": "http://127.0.0.1:9/api/v2/auth/oidc/callback", "scope": "openid profile email",
	} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	if Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk") != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatal("the PKCE challenge does not match RFC 7636's example")
	}
}

func exchange(t *testing.T, f *fakeIDP, p *Provider, tokenClaims map[string]any, alg, kid string) (Claims, error) {
	t.Helper()
	f.idToken = f.token(alg, kid, tokenClaims)
	return p.Exchange(context.Background(), "code-1", "verifier-1", "nonce-1")
}

func TestAGoodIDTokenSignsAPersonInAndTheSecretTravelsInTheBasicHeader(t *testing.T) {
	for _, alg := range []string{"RS256", "ES256"} {
		t.Run(alg, func(t *testing.T) {
			f := newFakeIDP(t)
			p, now := f.provider(nil)
			kid := f.kid
			if alg == "ES256" {
				kid = "ec1"
			}
			c, err := exchange(t, f, p, f.idClaims(*now, "nonce-1"), alg, kid)
			if err != nil {
				t.Fatal(err)
			}
			if c.Subject != "subject-1" || c.Email != "ada@example.com" || !c.EmailVerified || c.Name != "Ada Lovelace" || c.SessionID != "sess-1" ||
				!c.HasGroups || len(c.Groups) != 1 || c.Groups[0] != "reet-admin" {
				t.Fatalf("claims = %+v", c)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if !strings.HasPrefix(f.lastAuth, "Basic ") || f.lastForm["code_verifier"][0] != "verifier-1" || f.lastForm["grant_type"][0] != "authorization_code" {
				t.Fatalf("token request: auth %q form %v", f.lastAuth, f.lastForm)
			}
			if _, ok := f.lastForm["client_secret"]; ok {
				t.Fatal("the secret was also sent in the body")
			}
		})
	}
}

func TestEveryWayAnIDTokenCanBeWrongIsRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		alg, kid string
		mutate   func(map[string]any, time.Time)
		want     string
	}{
		"alg none":            {"none", "k1", nil, "algorithm"},
		"hs256 forgery":       {"HS256", "k1", nil, "algorithm"},
		"unknown kid":         {"RS256", "nope", nil, "does not publish"},
		"wrong issuer":        {"RS256", "k1", func(c map[string]any, _ time.Time) { c["iss"] = "https://evil.example.test" }, "issuer"},
		"wrong audience":      {"RS256", "k1", func(c map[string]any, _ time.Time) { c["aud"] = "someone-else" }, "different client"},
		"several aud, no azp": {"RS256", "k1", func(c map[string]any, _ time.Time) { c["aud"] = []string{"client", "other"} }, "authorised party"},
		"wrong azp":           {"RS256", "k1", func(c map[string]any, _ time.Time) { c["aud"] = []string{"client", "other"}; c["azp"] = "other" }, "authorised party"},
		"expired":             {"RS256", "k1", func(c map[string]any, n time.Time) { c["exp"] = n.Add(-10 * time.Minute).Unix() }, "expired"},
		"not yet valid":       {"RS256", "k1", func(c map[string]any, n time.Time) { c["nbf"] = n.Add(10 * time.Minute).Unix() }, "not valid yet"},
		"issued in future":    {"RS256", "k1", func(c map[string]any, n time.Time) { c["iat"] = n.Add(10 * time.Minute).Unix() }, "future"},
		"no expiry":           {"RS256", "k1", func(c map[string]any, _ time.Time) { delete(c, "exp") }, "no expiry"},
		"nonce mismatch":      {"RS256", "k1", func(c map[string]any, _ time.Time) { c["nonce"] = "another" }, "nonce"},
		"no nonce":            {"RS256", "k1", func(c map[string]any, _ time.Time) { delete(c, "nonce") }, "nonce"},
		"no subject":          {"RS256", "k1", func(c map[string]any, _ time.Time) { delete(c, "sub") }, "subject"},
		"huge subject":        {"RS256", "k1", func(c map[string]any, _ time.Time) { c["sub"] = strings.Repeat("s", 256) }, "subject"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeIDP(t)
			p, now := f.provider(nil)
			claims := f.idClaims(*now, "nonce-1")
			if tc.mutate != nil {
				tc.mutate(claims, *now)
			}
			_, err := exchange(t, f, p, claims, tc.alg, tc.kid)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Exchange = %v; want an error containing %q", err, tc.want)
			}
		})
	}
}

func TestATokenSignedByAnotherKeyIsRefused(t *testing.T) {
	f := newFakeIDP(t)
	p, now := f.provider(nil)
	other := newFakeIDP(t) // a different key pair, same key id
	f.idToken = other.token("RS256", "k1", f.idClaims(*now, "nonce-1"))
	if _, err := p.Exchange(context.Background(), "c", "v", "nonce-1"); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("a token signed by a stranger's key = %v", err)
	}
}

func TestTheTokenEndpointsErrorTextIsNotPassedOn(t *testing.T) {
	f := newFakeIDP(t)
	f.status = 400
	p, _ := f.provider(nil)
	_, err := p.Exchange(context.Background(), "c", "v", "n")
	if err == nil || strings.Contains(err.Error(), "secret-echo") {
		t.Fatalf("Exchange = %v; want a refusal that does not repeat the provider's text", err)
	}
}

func TestARotatedKeyIsFoundOnceAndTheRefetchIsThrottled(t *testing.T) {
	f := newFakeIDP(t)
	p, now := f.provider(nil)
	if _, err := exchange(t, f, p, f.idClaims(*now, "nonce-1"), "RS256", "k1"); err != nil {
		t.Fatal(err)
	}
	if f.count("jwks") != 1 {
		t.Fatalf("jwks fetched %d times", f.count("jwks"))
	}
	// The provider rotates: a new key signs and is published.
	newKey, _ := rsaKey(t)
	f.extra["k2"] = newKey
	*now = now.Add(2 * time.Minute) // past the refetch throttle
	if _, err := exchange(t, f, p, f.idClaims(*now, "nonce-1"), "RS256", "k2"); err != nil {
		t.Fatalf("a rotated key was not picked up: %v", err)
	}
	if f.count("jwks") != 2 {
		t.Fatalf("jwks fetched %d times after a rotation", f.count("jwks"))
	}
	// A stranger inventing key ids cannot make this server call out each time.
	for i := 0; i < 5; i++ {
		exchange(t, f, p, f.idClaims(*now, "nonce-1"), "RS256", "invented")
	}
	if got := f.count("jwks"); got > 3 {
		t.Fatalf("unknown key ids caused %d fetches; want the refetch throttled", got)
	}
}

func TestAKeySetWithOneBadKeyStillServesTheGoodOnes(t *testing.T) {
	f := newFakeIDP(t)
	f.jwksOverride = func() any {
		return map[string]any{"keys": []map[string]string{
			{"kty": "RSA", "kid": "short", "use": "sig", "n": "AQAB", "e": "AQAB"},
			rsaJWK("k1", &f.rsaKey.PublicKey),
		}}
	}
	p, now := f.provider(nil)
	if _, err := exchange(t, f, p, f.idClaims(*now, "nonce-1"), "RS256", "k1"); err != nil {
		t.Fatalf("one malformed key hid the good one: %v", err)
	}
}

func TestClaimsAreMappedWithTheirFallbacks(t *testing.T) {
	f := newFakeIDP(t)
	p, now := f.provider(func(c *Config) { c.GroupsClaim = "roles" })
	for name, tc := range map[string]struct {
		mutate func(map[string]any)
		check  func(Claims) bool
	}{
		"groups as a string":  {func(c map[string]any) { c["roles"] = "reet-admin, other" }, func(c Claims) bool { return c.HasGroups && len(c.Groups) == 2 }},
		"groups as a list":    {func(c map[string]any) { c["roles"] = []string{"a", "b"} }, func(c Claims) bool { return c.HasGroups && len(c.Groups) == 2 }},
		"no groups claim":     {func(c map[string]any) { delete(c, "roles") }, func(c Claims) bool { return !c.HasGroups }},
		"empty groups list":   {func(c map[string]any) { c["roles"] = []string{} }, func(c Claims) bool { return c.HasGroups && len(c.Groups) == 0 }},
		"verified as string":  {func(c map[string]any) { c["email_verified"] = "true" }, func(c Claims) bool { return c.EmailVerified }},
		"unverified":          {func(c map[string]any) { c["email_verified"] = false }, func(c Claims) bool { return !c.EmailVerified }},
		"name from username":  {func(c map[string]any) { delete(c, "name"); c["preferred_username"] = "ada" }, func(c Claims) bool { return c.Name == "ada" }},
		"name from the email": {func(c map[string]any) { delete(c, "name") }, func(c Claims) bool { return c.Name == "ada" }},
	} {
		t.Run(name, func(t *testing.T) {
			claims := f.idClaims(*now, "nonce-1")
			delete(claims, "groups")
			tc.mutate(claims)
			c, err := exchange(t, f, p, claims, "RS256", f.kid)
			if err != nil || !tc.check(c) {
				t.Fatalf("claims = %+v, %v", c, err)
			}
		})
	}
}

func TestGroupsMapToTheHighestRoleAndTheOwnerIsNeverGiven(t *testing.T) {
	f := newFakeIDP(t)
	p, _ := f.provider(func(c *Config) {
		c.AdminGroups, c.MemberGroups, c.ViewerGroups = []string{"Admins"}, []string{"staff"}, []string{"guests"}
	})
	for _, tc := range []struct {
		groups []string
		role   string
		mapped bool
	}{
		{[]string{"guests", "staff", "admins"}, RoleAdmin, true}, // case-insensitive, highest wins
		{[]string{"staff"}, RoleMember, true},
		{[]string{"guests"}, RoleViewer, true},
		{[]string{"nothing-we-know"}, RoleViewer, true}, // a typo gives read-only
		{nil, RoleViewer, true},
		{[]string{"owner", "root"}, RoleViewer, true}, // there is no way to ask for the owner
	} {
		if role, mapped := p.RoleFor(tc.groups); role != tc.role || mapped != tc.mapped {
			t.Errorf("RoleFor(%v) = %q, %v; want %q, %v", tc.groups, role, mapped, tc.role, tc.mapped)
		}
	}
	none, _ := f.provider(nil)
	if role, mapped := none.RoleFor([]string{"admins"}); mapped || role != RoleViewer {
		t.Fatalf("with no groups mapped RoleFor = %q, %v; want the default and unmapped", role, mapped)
	}
}

func logoutClaims(f *fakeIDP, now time.Time) map[string]any {
	return map[string]any{
		"iss": f.issuer, "aud": "client", "sub": "subject-1", "sid": "sess-1", "jti": "jti-1",
		"iat": now.Unix(), "events": map[string]any{logoutEvent: map[string]any{}},
	}
}

func TestALogoutTokenEndsASubjectsOrASessionsAccessOnceOnly(t *testing.T) {
	f := newFakeIDP(t)
	p, now := f.provider(nil)
	got, err := p.VerifyLogoutToken(context.Background(), f.token("RS256", "k1", logoutClaims(f, *now)))
	if err != nil || got.Subject != "subject-1" || got.SessionID != "sess-1" {
		t.Fatalf("VerifyLogoutToken = %+v, %v", got, err)
	}
	if _, err := p.VerifyLogoutToken(context.Background(), f.token("RS256", "k1", logoutClaims(f, *now))); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("a replayed logout token = %v", err)
	}
	*now = now.Add(11 * time.Minute) // the memory of the jti ends, but so does the token's freshness
	c := logoutClaims(f, *now)
	if _, err := p.VerifyLogoutToken(context.Background(), f.token("RS256", "k1", c)); err != nil {
		t.Fatalf("a fresh token with an old jti was refused: %v", err)
	}
}

func TestEveryWayALogoutTokenCanBeWrongIsRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(map[string]any, time.Time)
		want   string
		alg    string
	}{
		"wrong event":    {func(c map[string]any, _ time.Time) { c["events"] = map[string]any{"other": map[string]any{}} }, "not a logout token", "RS256"},
		"no events":      {func(c map[string]any, _ time.Time) { delete(c, "events") }, "not a logout token", "RS256"},
		"has a nonce":    {func(c map[string]any, _ time.Time) { c["nonce"] = "n" }, "nonce", "RS256"},
		"stale iat":      {func(c map[string]any, n time.Time) { c["iat"] = n.Add(-10 * time.Minute).Unix() }, "too old", "RS256"},
		"no iat":         {func(c map[string]any, _ time.Time) { delete(c, "iat") }, "issue time", "RS256"},
		"no jti":         {func(c map[string]any, _ time.Time) { delete(c, "jti") }, "no jti", "RS256"},
		"no sub nor sid": {func(c map[string]any, _ time.Time) { delete(c, "sub"); delete(c, "sid") }, "neither", "RS256"},
		"wrong audience": {func(c map[string]any, _ time.Time) { c["aud"] = "other" }, "different client", "RS256"},
		"wrong issuer":   {func(c map[string]any, _ time.Time) { c["iss"] = "https://x.example.test" }, "issuer", "RS256"},
		"unsigned":       {nil, "algorithm", "none"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeIDP(t)
			p, now := f.provider(nil)
			c := logoutClaims(f, *now)
			if tc.mutate != nil {
				tc.mutate(c, *now)
			}
			if _, err := p.VerifyLogoutToken(context.Background(), f.token(tc.alg, "k1", c)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("VerifyLogoutToken = %v; want an error containing %q", err, tc.want)
			}
		})
	}
}

func TestTheAddressGuardRefusesLinkLocalAndUnspecifiedButNotLoopbackOrPrivate(t *testing.T) {
	for addr, refused := range map[string]bool{
		"169.254.169.254:80": true, "0.0.0.0:80": true, "[fe80::1]:80": true, "[::]:80": true, "224.0.0.1:80": true,
		"127.0.0.1:80": false, "10.0.0.5:443": false, "192.168.1.1:443": false, "93.184.216.34:443": false,
	} {
		err := refuseAddress("tcp", addr, nil)
		if (err != nil) != refused {
			t.Errorf("refuseAddress(%s) = %v, refused should be %v", addr, err, refused)
		}
	}
}
