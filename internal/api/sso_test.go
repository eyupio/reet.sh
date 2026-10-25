package api

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/oidc"
	"github.com/eyupio/reet.sh/internal/oidc/oidctest"
	"github.com/eyupio/reet.sh/internal/store"
)

type ssoEnv struct {
	t     *testing.T
	idp   *oidctest.Server
	srv   *httptest.Server
	st    *store.Store
	owner store.User
	// ended collects the session ids a sign-out, disable or logout ended, as
	// the terminal broker was told.
	ended []string
}

type ssoOpts struct {
	env      map[string]string
	noOwner  bool
	noSSO    bool
	authMode AuthMode
}

const ownerPassword = "owner password 123"

func newSSOEnv(t *testing.T, o ssoOpts) *ssoEnv {
	t.Helper()
	e := &ssoEnv{t: t, idp: oidctest.New(t)}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e.st = st
	if !o.noOwner {
		hash, _ := auth.HashPassword(ownerPassword)
		e.owner, _, err = st.CreateFirstOwner("owner@example.com", "Owner", "owner", hash)
		if err != nil {
			t.Fatal(err)
		}
	}
	opts := []Option{WithAuthMode(AuthAccounts)}
	if o.authMode != "" {
		opts = []Option{WithAuthMode(o.authMode)}
	}
	if !o.noSSO {
		vars := map[string]string{
			"REET_OIDC_ISSUER": e.idp.Issuer, "REET_OIDC_CLIENT_ID": oidctest.ClientID, "REET_OIDC_CLIENT_SECRET": "s3cret",
			"REET_OIDC_REDIRECT_URL": "http://localhost/api/v2/auth/oidc/callback",
		}
		for k, v := range o.env {
			vars[k] = v
		}
		p, err := oidc.FromEnv(func(k string) string { return vars[k] })
		if err != nil || p == nil {
			t.Fatalf("provider: %v %v", p, err)
		}
		opts = append(opts, WithOIDC(p))
	}
	s := newServer(st, deadFetcher(t), "bootstrap", opts...)
	s.afterSessionsEnded = func(ids []string) { e.ended = append(e.ended, ids...) }
	e.srv = httptest.NewServer(&Server{s: s, h: s.logRequests(s.auth(s.routes()))})
	t.Cleanup(func() { e.srv.Close(); st.Close() })
	return e
}

func (e *ssoEnv) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (e *ssoEnv) get(c *http.Client, path string) *http.Response {
	e.t.Helper()
	resp, err := c.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func (e *ssoEnv) body(c *http.Client, method, path, body string, hdr ...string) (int, string) {
	e.t.Helper()
	return doClient(e.t, c, e.srv, method, path, body, hdr...)
}

// start begins a sign-in and returns the provider address it redirected to.
func (e *ssoEnv) start(c *http.Client, query string) (string, *http.Response) {
	e.t.Helper()
	resp := e.get(c, "/api/v2/auth/oidc/start"+query)
	return resp.Header.Get("Location"), resp
}

// callback sends the browser back with what the provider would send.
func (e *ssoEnv) callback(c *http.Client, q url.Values) *http.Response {
	e.t.Helper()
	return e.get(c, "/api/v2/auth/oidc/callback?"+q.Encode())
}

// login runs a whole sign-in for person and returns where the browser is sent.
func (e *ssoEnv) login(c *http.Client, p oidctest.Person, query string) string {
	e.t.Helper()
	auth, resp := e.start(c, query)
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(auth, e.idp.Issuer+"/authorize") {
		e.t.Fatalf("start = %d %q", resp.StatusCode, auth)
	}
	return e.callback(c, e.idp.Complete(auth, p)).Header.Get("Location")
}

func (e *ssoEnv) signedInAs(c *http.Client) (string, bool) {
	e.t.Helper()
	code, body := e.body(c, "GET", "/api/v2/auth/me", "")
	if code != 200 {
		return "", false
	}
	var u store.User
	if json.Unmarshal([]byte(body), &u) != nil {
		return "", false
	}
	return u.Email, true
}

func ada(sub string) oidctest.Person {
	return oidctest.Person{Subject: sub, Email: "ada@example.com", Name: "Ada", EmailVerified: true, SessionID: "sid-" + sub}
}

func (e *ssoEnv) member(email, role string) store.User {
	e.t.Helper()
	u, err := e.st.CreateUser(email, "", role, "")
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func errorCode(loc string) string {
	u, _ := url.Parse(loc)
	return u.Query().Get("sso_error")
}

func TestASignInThroughTheProviderStartsASession(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	u := e.member("ada@example.com", "member")
	if err := e.st.LinkSSOIdentity(e.idp.Issuer, "sub-ada", u.ID); err != nil {
		t.Fatal(err)
	}
	c := e.client()
	auth, resp := e.start(c, "")
	for _, h := range []string{"no-store"} {
		if !strings.Contains(resp.Header.Get("Cache-Control"), h) {
			t.Fatalf("start Cache-Control = %q", resp.Header.Get("Cache-Control"))
		}
	}
	q, _ := url.Parse(auth)
	for _, k := range []string{"state", "nonce", "code_challenge"} {
		if q.Query().Get(k) == "" {
			t.Fatalf("authorisation URL lacks %s: %s", k, auth)
		}
	}
	cb := e.callback(c, e.idp.Complete(auth, ada("sub-ada")))
	if cb.StatusCode != http.StatusFound || cb.Header.Get("Location") != "/" {
		t.Fatalf("callback = %d %q", cb.StatusCode, cb.Header.Get("Location"))
	}
	if cb.Header.Get("Cache-Control") != "no-store" || cb.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("callback headers: %v", cb.Header)
	}
	if email, ok := e.signedInAs(c); !ok || email != "ada@example.com" {
		t.Fatalf("signed in as %q %v", email, ok)
	}
	// The binding cookie is spent with the attempt.
	for _, ck := range cb.Cookies() {
		if ck.Name == "reet_sso" && ck.MaxAge >= 0 {
			t.Fatal("the browser binding cookie was not cleared")
		}
	}
}

func TestACallbackCanOnlyBePlayedOnce(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	u := e.member("ada@example.com", "member")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-ada", u.ID)
	c := e.client()
	auth, _ := e.start(c, "")
	q := e.idp.Complete(auth, ada("sub-ada"))
	if loc := e.callback(c, q).Header.Get("Location"); errorCode(loc) != "" {
		t.Fatalf("first callback: %s", loc)
	}
	again := e.client() // a thief replaying the address without the cookie, or the same browser
	for _, cl := range []*http.Client{c, again} {
		if loc := e.callback(cl, q).Header.Get("Location"); errorCode(loc) != "expired" {
			t.Fatalf("replay = %q, want expired", loc)
		}
	}
	if e.idp.Tokens != 1 {
		t.Fatalf("the provider's token endpoint was called %d times", e.idp.Tokens)
	}
}

func TestACallbackFromAnotherBrowserIsRefusedAndBurnsTheAttempt(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	u := e.member("ada@example.com", "member")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-ada", u.ID)
	victim, attacker := e.client(), e.client()
	auth, _ := e.start(victim, "")
	q := e.idp.Complete(auth, ada("sub-ada"))
	if loc := e.callback(attacker, q).Header.Get("Location"); errorCode(loc) != "mismatch" {
		t.Fatalf("another browser = %q, want mismatch", loc)
	}
	if _, ok := e.signedInAs(attacker); ok {
		t.Fatal("the other browser got a session")
	}
	if loc := e.callback(victim, q).Header.Get("Location"); errorCode(loc) != "expired" {
		t.Fatalf("the attempt should be spent: %q", loc)
	}
	// A cookie with the wrong value is no better than none.
	c := e.client()
	auth, _ = e.start(c, "")
	su, _ := url.Parse(e.srv.URL)
	c.Jar.SetCookies(su, []*http.Cookie{{Name: "reet_sso", Value: "forged", Path: "/api/v2/auth/oidc"}})
	if loc := e.callback(c, e.idp.Complete(auth, ada("sub-ada"))).Header.Get("Location"); errorCode(loc) != "mismatch" {
		t.Fatalf("forged binding = %q", loc)
	}
}

func TestUnknownAndCancelledStatesSignNobodyIn(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	c := e.client()
	if loc := e.callback(c, url.Values{"code": {"x"}, "state": {"nonsense"}}).Header.Get("Location"); errorCode(loc) != "expired" {
		t.Fatalf("unknown state = %q", loc)
	}
	if loc := e.callback(c, url.Values{"code": {"x"}}).Header.Get("Location"); errorCode(loc) != "expired" {
		t.Fatalf("no state = %q", loc)
	}
	auth, _ := e.start(c, "")
	st, _ := url.Parse(auth)
	loc := e.callback(c, url.Values{"error": {"access_denied"}, "error_description": {"<script>alert(1)</script> secret words"}, "state": {st.Query().Get("state")}}).Header.Get("Location")
	if errorCode(loc) != "denied" || strings.Contains(loc, "script") || strings.Contains(loc, "secret") {
		t.Fatalf("a refusal at the provider = %q: only a fixed code may travel", loc)
	}
	// A refusal spends the attempt too.
	if loc := e.callback(c, e.idp.Complete(auth, ada("sub-x"))).Header.Get("Location"); errorCode(loc) != "expired" {
		t.Fatalf("after a refusal = %q", loc)
	}
	// A callback with a good state but no code is a failed sign-in, not a session.
	auth, _ = e.start(c, "")
	st, _ = url.Parse(auth)
	if loc := e.callback(c, url.Values{"state": {st.Query().Get("state")}}).Header.Get("Location"); errorCode(loc) != "failed" {
		t.Fatalf("no code = %q", loc)
	}
}

func TestADisabledPersonIsNotSignedInByTheProvider(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	u := e.member("ada@example.com", "member")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-ada", u.ID)
	if err := e.st.SetUserDisabled(u.ID, true); err != nil {
		t.Fatal(err)
	}
	c := e.client()
	if loc := e.login(c, ada("sub-ada"), ""); errorCode(loc) != "disabled" {
		t.Fatalf("disabled = %q", loc)
	}
	if _, ok := e.signedInAs(c); ok {
		t.Fatal("a disabled account got a session")
	}
}

func TestWithAutoCreateOffAStrangerIsRefused(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	c := e.client()
	if loc := e.login(c, ada("sub-new"), ""); errorCode(loc) != "no_account" {
		t.Fatalf("stranger = %q", loc)
	}
	if n, _ := e.st.UserCount(); n != 1 {
		t.Fatalf("an account was created: %d users", n)
	}
}

func TestWithAutoCreateOnAStrangerGetsTheRoleTheirGroupsMapTo(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{env: map[string]string{"REET_OIDC_AUTO_CREATE": "on", "REET_OIDC_ADMIN_GROUPS": "ops", "REET_OIDC_MEMBER_GROUPS": "staff"}})
	c := e.client()
	p := ada("sub-new")
	p.Groups = []string{"staff"}
	if loc := e.login(c, p, ""); loc != "/" {
		t.Fatalf("login = %q", loc)
	}
	u, ok, _ := e.st.UserByEmail("ada@example.com")
	if !ok || u.Role != "member" || u.PasswordHash != "" {
		t.Fatalf("created %+v", u)
	}
	// Having no password, the account cannot sign in with one.
	code, _ := e.body(e.client(), "POST", "/api/v2/auth/sign-in", `{"email":"ada@example.com","password":""}`)
	if code != http.StatusUnauthorized {
		t.Fatalf("an empty password signed in: %d", code)
	}
	// Whatever the groups say, the owner role is never given.
	e2 := newSSOEnv(t, ssoOpts{env: map[string]string{"REET_OIDC_AUTO_CREATE": "on", "REET_OIDC_ADMIN_GROUPS": "owner,ops"}})
	p2 := ada("sub-boss")
	p2.Groups = []string{"owner", "ops"}
	e2.login(e2.client(), p2, "")
	if u, _, _ := e2.st.UserByEmail("ada@example.com"); u.Role != "admin" {
		t.Fatalf("the highest a group can give is admin; got %q", u.Role)
	}
}

func TestAnEmailTheProviderDoesNotVouchForCreatesNoAccount(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{env: map[string]string{"REET_OIDC_AUTO_CREATE": "on"}})
	c := e.client()
	p := ada("sub-new")
	p.EmailVerified = false
	if loc := e.login(c, p, ""); errorCode(loc) != "email_unverified" {
		t.Fatalf("unverified = %q", loc)
	}
	p = ada("sub-none")
	p.Email = ""
	if loc := e.login(c, p, ""); errorCode(loc) != "email_missing" {
		t.Fatalf("no email = %q", loc)
	}
	if n, _ := e.st.UserCount(); n != 1 {
		t.Fatalf("accounts = %d", n)
	}
}

func TestLinkingByEmailIsOffUntilAskedForAndNeverReachesTheOwnerOrAnAdmin(t *testing.T) {
	off := newSSOEnv(t, ssoOpts{})
	off.member("ada@example.com", "member")
	if loc := off.login(off.client(), ada("sub-ada"), ""); errorCode(loc) != "exists" {
		t.Fatalf("link-by-email off = %q", loc)
	}
	if _, ok, _ := off.st.SSOIdentityUser(off.idp.Issuer, "sub-ada"); ok {
		t.Fatal("linked without being asked to")
	}

	on := newSSOEnv(t, ssoOpts{env: map[string]string{"REET_OIDC_LINK_BY_EMAIL": "on"}})
	u := on.member("ada@example.com", "member")
	unverified := ada("sub-ada")
	unverified.EmailVerified = false
	if loc := on.login(on.client(), unverified, ""); errorCode(loc) != "email_unverified" {
		t.Fatalf("unverified email = %q", loc)
	}
	c := on.client()
	if loc := on.login(c, ada("sub-ada"), ""); loc != "/" {
		t.Fatalf("link-by-email on = %q", loc)
	}
	if got, ok, _ := on.st.SSOIdentityUser(on.idp.Issuer, "sub-ada"); !ok || got.ID != u.ID {
		t.Fatal("not linked")
	}
	// Already linked to someone else's subject: a second subject is refused.
	if loc := on.login(on.client(), ada("sub-impostor"), ""); errorCode(loc) != "already_linked" {
		t.Fatalf("second subject for a linked account = %q", loc)
	}
	// The owner and admins cannot be taken over by an email match.
	on.st.CreateUser("admin@example.com", "", "admin", "")
	for _, email := range []string{"owner@example.com", "admin@example.com"} {
		p := ada("sub-" + email)
		p.Email = email
		if loc := on.login(on.client(), p, ""); errorCode(loc) != "not_linkable" {
			t.Fatalf("%s = %q", email, loc)
		}
	}
}

func TestOnlyALocalPathIsFollowedAfterSignIn(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	u := e.member("ada@example.com", "member")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-ada", u.ID)
	for _, next := range []string{
		"//evil.example", "https://evil.example/", "/\\evil.example", "/%5Cevil.example", "javascript:alert(1)", "", "evil", "/ok\nSet-Cookie: x=1",
		"/api/v2/tokens", "http:/evil", "/" + strings.Repeat("a", 3000),
	} {
		loc := e.login(e.client(), ada("sub-ada"), "?next="+url.QueryEscape(next))
		if loc != "/" {
			t.Errorf("next=%q went to %q, want /", next, loc)
		}
	}
	for _, next := range []string{"/lists?tab=liked", "/account/mcp/authorize?client_id=reet_client_x&state=a%20b&code_challenge=c"} {
		if loc := e.login(e.client(), ada("sub-ada"), "?next="+url.QueryEscape(next)); loc != next {
			t.Errorf("next=%q went to %q: a sign-in must resume where it began (the MCP consent page among them)", next, loc)
		}
	}
}

func TestNobodyIsSignedInBeforeSetupAndNeverBecomesTheOwner(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{noOwner: true, env: map[string]string{"REET_OIDC_AUTO_CREATE": "on"}})
	c := e.client()
	if loc, resp := e.start(c, ""); resp.StatusCode != http.StatusFound || errorCode(loc) != "not_ready" {
		t.Fatalf("start before setup = %d %q", resp.StatusCode, loc)
	}
	if n, _ := e.st.UserCount(); n != 0 {
		t.Fatalf("users = %d", n)
	}
	// A callback that was somehow begun earlier is refused too.
	if err := e.st.CreateSSOLogin(store.SSOLogin{StateHash: auth.HashToken("s"), BindingHash: auth.HashToken("b"), Nonce: "n", Verifier: "v", ExpiresAt: time.Now().Add(time.Minute).UnixMilli()}, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	su, _ := url.Parse(e.srv.URL)
	c.Jar.SetCookies(su, []*http.Cookie{{Name: "reet_sso", Value: "b", Path: "/api/v2/auth/oidc"}})
	if loc := e.callback(c, url.Values{"code": {"x"}, "state": {"s"}}).Header.Get("Location"); errorCode(loc) != "not_ready" {
		t.Fatalf("callback before setup = %q", loc)
	}
	if n, _ := e.st.UserCount(); n != 0 {
		t.Fatalf("a single sign-on login created an account before setup: %d", n)
	}
}

func TestStatusStaysASupersetOfWhatItWasAndSaysWhatSingleSignOnOffers(t *testing.T) {
	off := newSSOEnv(t, ssoOpts{noSSO: true})
	_, body := off.body(off.client(), "GET", "/api/v2/auth/status", "")
	var before map[string]any
	if err := json.Unmarshal([]byte(body), &before); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"mode", "setupRequired", "bootstrapRequired", "signupEnabled", "contactEmail", "passwordReset", "user"} {
		if _, ok := before[k]; !ok {
			t.Errorf("status lost %q", k)
		}
	}
	if sso, _ := before["sso"].(map[string]any); sso["enabled"] != false || len(sso) != 1 {
		t.Fatalf("without single sign-on, sso = %v", before["sso"])
	}
	on := newSSOEnv(t, ssoOpts{env: map[string]string{"REET_OIDC_BUTTON_LABEL": "Sign in with Acme", "REET_OIDC_PASSWORD_LOGIN": "off"}})
	_, body = on.body(on.client(), "GET", "/api/v2/auth/status", "")
	var after map[string]any
	_ = json.Unmarshal([]byte(body), &after)
	for k := range before {
		if k != "sso" {
			if _, ok := after[k]; !ok {
				t.Errorf("status with single sign-on lost %q", k)
			}
		}
	}
	sso, _ := after["sso"].(map[string]any)
	if sso["enabled"] != true || sso["label"] != "Sign in with Acme" || sso["passwordLogin"] != false {
		t.Fatalf("sso = %v", sso)
	}
	if strings.Contains(body, "s3cret") || strings.Contains(body, on.idp.Issuer) {
		t.Fatalf("status leaks configuration: %s", body)
	}
}

func TestWithPasswordSignInOffTheOwnerCanStillComeInWithAPassword(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{env: map[string]string{"REET_OIDC_PASSWORD_LOGIN": "off"}})
	hash, _ := auth.HashPassword("member password 123")
	m, _ := e.st.CreateUser("member@example.com", "", "member", hash)
	_ = m
	if code, body := e.body(e.client(), "POST", "/api/v2/auth/sign-in", `{"email":"member@example.com","password":"member password 123"}`); code != http.StatusForbidden || !strings.Contains(body, "single sign-on") {
		t.Fatalf("member with the right password = %d %s", code, body)
	}
	if code, body := e.body(e.client(), "POST", "/api/v2/auth/sign-in", `{"email":"member@example.com","password":"wrong"}`); code != http.StatusUnauthorized || strings.Contains(body, "single sign-on") {
		t.Fatalf("a wrong password must not reveal the policy: %d %s", code, body)
	}
	if code, body := e.body(e.client(), "POST", "/api/v2/auth/sign-in", `{"email":"owner@example.com","password":"`+ownerPassword+`"}`); code != 200 {
		t.Fatalf("owner = %d %s", code, body)
	}
}

func TestSigningOutOfASingleSignOnSessionClosesItsTerminals(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	u := e.member("ada@example.com", "member")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-ada", u.ID)
	c := e.client()
	e.login(c, ada("sub-ada"), "")
	code, _ := e.body(c, "POST", "/api/v2/auth/sign-out", "{}")
	if code != http.StatusNoContent {
		t.Fatalf("sign-out = %d", code)
	}
	if len(e.ended) != 1 {
		t.Fatalf("terminals closed for %v", e.ended)
	}
	if _, ok := e.signedInAs(c); ok {
		t.Fatal("still signed in")
	}
}

func logoutPost(e *ssoEnv, token string) (int, string) {
	e.t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v2/auth/oidc/backchannel-logout", strings.NewReader(url.Values{"logout_token": {token}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var b strings.Builder
	buf := make([]byte, 512)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if resp.StatusCode == 200 && resp.Header.Get("Cache-Control") != "no-store" {
		e.t.Fatalf("logout answer Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
	return resp.StatusCode, b.String()
}

func TestAProvidersLogoutEndsTheSessionsItNamesAndTheirTerminals(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	u := e.member("ada@example.com", "member")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-ada", u.ID)
	laptop, phone, other := e.client(), e.client(), e.client()
	e.login(laptop, oidctest.Person{Subject: "sub-ada", Email: "ada@example.com", EmailVerified: true, SessionID: "sid-laptop"}, "")
	e.login(phone, oidctest.Person{Subject: "sub-ada", Email: "ada@example.com", EmailVerified: true, SessionID: "sid-phone"}, "")
	// A password session of the owner is not the provider's to end.
	if code, _ := e.body(other, "POST", "/api/v2/auth/sign-in", `{"email":"owner@example.com","password":"`+ownerPassword+`"}`); code != 200 {
		t.Fatal("owner sign-in")
	}

	if code, _ := logoutPost(e, e.idp.LogoutToken("sub-ada", "sid-laptop", "j1")); code != 200 {
		t.Fatalf("logout = %d", code)
	}
	if _, ok := e.signedInAs(laptop); ok {
		t.Fatal("the named session survived")
	}
	if _, ok := e.signedInAs(phone); !ok {
		t.Fatal("another session of the person was ended by a logout that named one")
	}
	if len(e.ended) != 1 {
		t.Fatalf("terminals closed for %v", e.ended)
	}
	// The same token twice is refused.
	if code, _ := logoutPost(e, e.idp.LogoutToken("sub-ada", "sid-phone", "j1")); code != http.StatusBadRequest {
		t.Fatalf("replayed jti = %d", code)
	}
	if _, ok := e.signedInAs(phone); !ok {
		t.Fatal("a refused logout ended a session")
	}
	// A subject alone ends all of that person's sessions at the provider.
	if code, _ := logoutPost(e, e.idp.LogoutToken("sub-ada", "", "j2")); code != 200 {
		t.Fatalf("logout by subject = %d", code)
	}
	if _, ok := e.signedInAs(phone); ok {
		t.Fatal("the person's other session survived a logout by subject")
	}
	if _, ok := e.signedInAs(other); !ok {
		t.Fatal("the owner's password session was ended")
	}
	for _, bad := range []string{"", "not.a.token", e.idp.Sign(map[string]any{"iss": e.idp.Issuer, "aud": oidctest.ClientID, "sub": "sub-ada", "iat": time.Now().Unix()})} {
		if code, _ := logoutPost(e, bad); code != http.StatusBadRequest {
			t.Fatalf("bad logout token %q = %d", bad, code)
		}
	}
}

func TestOnlyTheLogoutRouteAcceptsAFormBody(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	for _, path := range []string{"/api/v2/auth/sign-in", "/api/v2/auth/oidc/backchannel-logout/extra", "/api/v2/auth/oidc/start"} {
		req, _ := http.NewRequest("POST", e.srv.URL+path, strings.NewReader("a=b"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnsupportedMediaType && resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s accepted a form body: %d", path, resp.StatusCode)
		}
	}
}

func TestSingleSignOnRoutesAnswerNothingWithoutSingleSignOnOrAccounts(t *testing.T) {
	for name, o := range map[string]ssoOpts{
		"not configured": {noSSO: true},
	} {
		e := newSSOEnv(t, o)
		for _, path := range []string{"/api/v2/auth/oidc/start", "/api/v2/auth/oidc/callback?state=x&code=y"} {
			if resp := e.get(e.client(), path); resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s: %s = %d", name, path, resp.StatusCode)
			}
		}
		if code, _ := logoutPost(e, "x.y.z"); code != http.StatusNotFound {
			t.Errorf("%s: logout = %d", name, code)
		}
	}
}

func TestStartingSignInIsRateLimitedPerAddress(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	c := e.client()
	var last string
	for i := 0; i < 40; i++ {
		loc, resp := e.start(c, "")
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("start %d = %d", i, resp.StatusCode)
		}
		last = loc
		if errorCode(loc) == "rate_limited" {
			break
		}
	}
	if errorCode(last) != "rate_limited" {
		t.Fatal("forty starts from one address were all allowed")
	}
}

func TestADisabledAccountsSessionsTokensAndTerminalsAreEnded(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	u := e.member("ada@example.com", "member")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-ada", u.ID)
	c := e.client()
	e.login(c, ada("sub-ada"), "")
	tok, _ := auth.NewToken()
	if _, err := e.st.CreateAPIToken(u.ID, "laptop", auth.HashToken(tok)); err != nil {
		t.Fatal(err)
	}
	// How a team operator removes somebody: an owner's session disables the account.
	owner := e.client()
	if code, _ := e.body(owner, "POST", "/api/v2/auth/sign-in", `{"email":"owner@example.com","password":"`+ownerPassword+`"}`); code != 200 {
		t.Fatal("owner sign-in")
	}
	code, body := e.body(owner, "PATCH", "/api/v2/admin/users", `{"id":`+itoa64(u.ID)+`,"disabled":true}`)
	if code != 200 {
		t.Fatalf("disable = %d %s", code, body)
	}
	if _, ok := e.signedInAs(c); ok {
		t.Fatal("a disabled account's session survived")
	}
	if code, _ := e.body(e.client(), "GET", "/api/v2/auth/me", "", "Authorization", "Bearer "+tok); code == 200 {
		t.Fatal("a disabled account's token survived")
	}
	if len(e.ended) != 1 {
		t.Fatalf("terminals closed for %v: disabling must close the open terminals of the sessions it ends", e.ended)
	}
	// And the provider cannot bring them back while they are disabled.
	if loc := e.login(e.client(), ada("sub-ada"), ""); errorCode(loc) != "disabled" {
		t.Fatalf("sign-in after disable = %q", loc)
	}
}

func itoa64(i int64) string { return strconv.FormatInt(i, 10) }

// signIn gives a client a password session.
func (e *ssoEnv) signIn(c *http.Client, email, pw string) {
	e.t.Helper()
	if code, body := e.body(c, "POST", "/api/v2/auth/sign-in", `{"email":"`+email+`","password":"`+pw+`"}`); code != 200 {
		e.t.Fatalf("sign-in as %s = %d %s", email, code, body)
	}
}

func (e *ssoEnv) memberWithPassword(email, role, pw string) store.User {
	e.t.Helper()
	hash, _ := auth.HashPassword(pw)
	u, err := e.st.CreateUser(email, "", role, hash)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func TestSomebodySignedInCanLinkTheirProviderIdentity(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	u := e.memberWithPassword("ada@example.com", "member", "ada password 123")
	c := e.client()
	if loc, resp := e.start(c, "?intent=link"); resp.StatusCode != http.StatusFound || errorCode(loc) != "signin_required" {
		t.Fatalf("link while signed out = %d %q", resp.StatusCode, loc)
	}
	e.signIn(c, "ada@example.com", "ada password 123")
	if loc := e.login(c, oidctest.Person{Subject: "sub-ada", Email: "someone-else@example.com", EmailVerified: true}, "?intent=link&next=%2Faccount"); loc != "/account" {
		t.Fatalf("link = %q", loc)
	}
	if got, ok, _ := e.st.SSOIdentityUser(e.idp.Issuer, "sub-ada"); !ok || got.ID != u.ID {
		t.Fatal("not linked: an identity's email need not match the account's, because the person proved both")
	}
	_, body := e.body(c, "GET", "/api/v2/auth/oidc/identity", "")
	if !strings.Contains(body, `"linked":true`) || !strings.Contains(body, `"canUnlink":true`) || strings.Contains(body, "sub-ada") {
		t.Fatalf("identity = %s", body)
	}
	// Someone else's identity cannot be taken.
	other := e.memberWithPassword("bob@example.com", "member", "bob password 1234")
	_ = other
	oc := e.client()
	e.signIn(oc, "bob@example.com", "bob password 1234")
	if loc := e.login(oc, ada("sub-ada"), "?intent=link"); errorCode(loc) != "linked_elsewhere" {
		t.Fatalf("a subject linked to another account = %q", loc)
	}
	// A different subject for an account that has one is refused too.
	if loc := e.login(c, ada("sub-second"), "?intent=link"); errorCode(loc) != "already_linked" {
		t.Fatalf("second identity = %q", loc)
	}
}

func TestALinkOnlyCompletesForTheSessionThatBeganIt(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	e.memberWithPassword("ada@example.com", "member", "ada password 123")
	e.memberWithPassword("bob@example.com", "member", "bob password 1234")
	c := e.client()
	e.signIn(c, "ada@example.com", "ada password 123")
	auth, _ := e.start(c, "?intent=link")
	// The browser is signed in as somebody else before it comes back.
	e.body(c, "POST", "/api/v2/auth/sign-out", "{}")
	e.signIn(c, "bob@example.com", "bob password 1234")
	if loc := e.callback(c, e.idp.Complete(auth, ada("sub-ada"))).Header.Get("Location"); errorCode(loc) != "signin_required" {
		t.Fatalf("link after the session changed = %q", loc)
	}
	if _, ok, _ := e.st.SSOIdentityUser(e.idp.Issuer, "sub-ada"); ok {
		t.Fatal("an identity was linked to the wrong account")
	}
}

func TestUnlinkingIsRefusedWhenItWouldLeaveNoWayIn(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	// No password: single sign-on is the only way in.
	u, err := e.st.CreateSSOUser(e.idp.Issuer, "sub-nopw", "nopw@example.com", "", "member")
	if err != nil {
		t.Fatal(err)
	}
	_ = u
	c := e.client()
	e.login(c, oidctest.Person{Subject: "sub-nopw", Email: "nopw@example.com", EmailVerified: true}, "")
	if code, body := e.body(c, "DELETE", "/api/v2/auth/oidc/identity", ""); code != http.StatusConflict {
		t.Fatalf("unlinking the only way in = %d %s", code, body)
	}
	if _, ok, _ := e.st.SSOIdentityUser(e.idp.Issuer, "sub-nopw"); !ok {
		t.Fatal("the link was removed anyway")
	}
	// A password, and password sign-in on: fine.
	e.memberWithPassword("pw@example.com", "member", "pw password 12345")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-pw", mustUser(t, e, "pw@example.com").ID)
	pc := e.client()
	e.login(pc, oidctest.Person{Subject: "sub-pw", Email: "pw@example.com", EmailVerified: true}, "")
	if code, body := e.body(pc, "DELETE", "/api/v2/auth/oidc/identity", ""); code != http.StatusNoContent {
		t.Fatalf("unlink with a password = %d %s", code, body)
	}
	// Password sign-in off: a member's password is no way in.
	off := newSSOEnv(t, ssoOpts{env: map[string]string{"REET_OIDC_PASSWORD_LOGIN": "off"}})
	off.memberWithPassword("pw@example.com", "member", "pw password 12345")
	_ = off.st.LinkSSOIdentity(off.idp.Issuer, "sub-pw", mustUser(t, off, "pw@example.com").ID)
	oc := off.client()
	off.login(oc, oidctest.Person{Subject: "sub-pw", Email: "pw@example.com", EmailVerified: true}, "")
	if code, _ := off.body(oc, "DELETE", "/api/v2/auth/oidc/identity", ""); code != http.StatusConflict {
		t.Fatalf("unlink with password sign-in off = %d", code)
	}
	// The owner always has the password.
	_ = off.st.LinkSSOIdentity(off.idp.Issuer, "sub-owner", off.owner.ID)
	ownerC := off.client()
	off.signIn(ownerC, "owner@example.com", ownerPassword)
	if code, _ := off.body(ownerC, "DELETE", "/api/v2/auth/oidc/identity", ""); code != http.StatusNoContent {
		t.Fatalf("owner unlink = %d", code)
	}
}

func mustUser(t *testing.T, e *ssoEnv, email string) store.User {
	t.Helper()
	u, ok, err := e.st.UserByEmail(email)
	if err != nil || !ok {
		t.Fatalf("no user %s", email)
	}
	return u
}

func TestAnAdminSeesHowSingleSignOnIsSetUpAndNeverItsSecret(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{env: map[string]string{"REET_OIDC_ADMIN_GROUPS": "ops", "REET_OIDC_AUTO_CREATE": "on"}})
	m := e.memberWithPassword("member@example.com", "member", "member password 123")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-m", m.ID)
	mc := e.client()
	e.signIn(mc, "member@example.com", "member password 123")
	if code, _ := e.body(mc, "GET", "/api/v2/admin/sso", ""); code != http.StatusForbidden {
		t.Fatalf("a member reading the status = %d", code)
	}
	oc := e.client()
	e.signIn(oc, "owner@example.com", ownerPassword)
	code, body := e.body(oc, "GET", "/api/v2/admin/sso", "")
	if code != 200 {
		t.Fatalf("status = %d %s", code, body)
	}
	var st map[string]any
	_ = json.Unmarshal([]byte(body), &st)
	if st["enabled"] != true || st["issuer"] != e.idp.Issuer || st["autoCreate"] != true || st["linked"] != float64(1) || st["reachable"] != true {
		t.Fatalf("status = %v", st)
	}
	if st["redirectUrl"] != "http://localhost/api/v2/auth/oidc/callback" || st["defaultRole"] != "viewer" {
		t.Fatalf("status = %v", st)
	}
	if strings.Contains(body, "s3cret") || strings.Contains(strings.ToLower(body), "secret") {
		t.Fatalf("the status shows the client secret: %s", body)
	}
	off := newSSOEnv(t, ssoOpts{noSSO: true})
	ooc := off.client()
	off.signIn(ooc, "owner@example.com", ownerPassword)
	if _, body := off.body(ooc, "GET", "/api/v2/admin/sso", ""); !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("status without single sign-on = %s", body)
	}
}

func TestWhereGroupsAreMappedTheProviderDecidesTheRoleAtEachSignIn(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{env: map[string]string{"REET_OIDC_ADMIN_GROUPS": "ops", "REET_OIDC_MEMBER_GROUPS": "staff"}})
	u := e.member("ada@example.com", "member")
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-ada", u.ID)
	role := func() string { got, _, _ := e.st.UserByID(u.ID); return got.Role }
	with := func(groups []string) oidctest.Person { p := ada("sub-ada"); p.Groups = groups; return p }

	e.login(e.client(), with([]string{"ops"}), "")
	if role() != "admin" {
		t.Fatalf("promoted by group: %q", role())
	}
	// A token with no groups claim at all must change nothing.
	e.login(e.client(), with(nil), "")
	if role() != "admin" {
		t.Fatalf("a token without a groups claim demoted the person to %q", role())
	}
	// A claim that is there but matches nothing falls to the default.
	e.login(e.client(), with([]string{"somebody-else"}), "")
	if role() != "viewer" {
		t.Fatalf("a groups claim that matches nothing: %q", role())
	}
	e.login(e.client(), with([]string{"staff"}), "")
	if role() != "member" {
		t.Fatalf("by group: %q", role())
	}
	// The owner's role is never the provider's to change.
	_ = e.st.LinkSSOIdentity(e.idp.Issuer, "sub-owner", e.owner.ID)
	p := oidctest.Person{Subject: "sub-owner", Email: "owner@example.com", EmailVerified: true, Groups: []string{"somebody-else"}}
	e.login(e.client(), p, "")
	if got, _, _ := e.st.UserByID(e.owner.ID); got.Role != "owner" {
		t.Fatalf("the owner became %q", got.Role)
	}
	// With no groups mapped, an administrator's choice stands.
	none := newSSOEnv(t, ssoOpts{})
	v := none.member("ada@example.com", "admin")
	_ = none.st.LinkSSOIdentity(none.idp.Issuer, "sub-ada", v.ID)
	p = ada("sub-ada")
	p.Groups = []string{"anything"}
	none.login(none.client(), p, "")
	if got, _, _ := none.st.UserByID(v.ID); got.Role != "admin" {
		t.Fatalf("with no groups mapped the role changed to %q", got.Role)
	}
}
