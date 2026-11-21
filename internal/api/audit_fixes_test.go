package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// auditServer is a bare server over a fresh store, with an owner whose
// password is ownerPassword.
func auditServer(t *testing.T, token string, opts ...Option) (*server, *store.Store, *httptest.Server, store.User) {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword(ownerPassword)
	owner, _, err := st.CreateFirstOwner("owner@example.com", "Owner", "owner", hash)
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(st, deadFetcher(t), token, opts...)
	srv := httptest.NewServer(&Server{s: s, h: s.logRequests(s.auth(s.routes()))})
	t.Cleanup(func() { srv.Close(); st.Close() })
	return s, st, srv, owner
}

func signInBody(email, password string) string {
	b, _ := json.Marshal(map[string]string{"email": email, "password": password})
	return string(b)
}

// A right password does not use up the allowance of the address it came
// from, so people behind one proxy do not lock each other out.
func TestSuccessfulSignInsDoNotUseUpTheAddressAllowance(t *testing.T) {
	s, st, srv, _ := auditServer(t, "", WithAuthMode(AuthAccounts))
	hash, _ := auth.HashPassword("member password 1")
	for i := 0; i < 8; i++ {
		email := fmt.Sprintf("m%d@example.com", i)
		if _, err := st.CreateUser(email, "", "member", hash); err != nil {
			t.Fatal(err)
		}
		if code, body := do(t, srv, "POST", "/api/v2/auth/sign-in", signInBody(email, "member password 1")); code != 200 {
			t.Fatalf("sign-in %d from one address: %d %s", i, code, body)
		}
	}
	if n := s.addrLimiter.Len(); n != 0 {
		t.Fatalf("%d address keys hold attempts after only successful sign-ins", n)
	}
	// A wrong password is counted, under a namespaced key.
	if code, _ := do(t, srv, "POST", "/api/v2/auth/sign-in", signInBody("m0@example.com", "wrong")); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	if n := s.addrLimiter.Len(); n != 1 {
		t.Fatalf("the failed attempt left %d address keys, want 1", n)
	}
	// The address allowance is the one single sign-on uses, and it does run out.
	for s.addrLimiter.Take("ip:127.0.0.1") {
	}
	code, body := do(t, srv, "POST", "/api/v2/auth/sign-in", signInBody("m1@example.com", "member password 1"))
	if code != http.StatusTooManyRequests {
		t.Fatalf("sign-in past the address allowance: %d %s", code, body)
	}
}

func TestSignInLimitsEachEmailAndCapsItsLength(t *testing.T) {
	s, _, srv, _ := auditServer(t, "", WithAuthMode(AuthAccounts))
	for i := 0; i < 5; i++ {
		if code, body := do(t, srv, "POST", "/api/v2/auth/sign-in", signInBody("owner@example.com", "wrong")); code != 401 {
			t.Fatalf("attempt %d: %d %s", i, code, body)
		}
	}
	if code, _ := do(t, srv, "POST", "/api/v2/auth/sign-in", signInBody("owner@example.com", ownerPassword)); code != http.StatusTooManyRequests {
		t.Fatalf("sixth attempt on one email: %d", code)
	}
	// Another email from the same address is not held back by it.
	if code, _ := do(t, srv, "POST", "/api/v2/auth/sign-in", signInBody("other@example.com", "wrong")); code != 401 {
		t.Fatalf("another email: %d", code)
	}
	keys, addrs := s.limiter.Len(), s.addrLimiter.Len()
	long := strings.Repeat("a", 300) + "@example.com"
	if code, _ := do(t, srv, "POST", "/api/v2/auth/sign-in", signInBody(long, "wrong")); code != 400 {
		t.Fatalf("an over-long email: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v2/auth/forgot", `{"email":"`+long+`"}`); code != 400 {
		t.Fatalf("an over-long email at forgot: %d", code)
	}
	if s.limiter.Len() != keys || s.addrLimiter.Len() != addrs {
		t.Fatal("an over-long email became a limiter key")
	}
}

// A sign-in whose email is spelled like another limiter key does not spend
// that key.
func TestSignInEmailCannotSpendTheSignupAllowance(t *testing.T) {
	_, _, srv, _ := auditServer(t, "", WithAuthMode(AuthAccounts), WithSignup(true))
	for _, email := range []string{"signup", "signup:all", "signup-ip:127.0.0.1", "ip:127.0.0.1"} {
		for i := 0; i < 5; i++ {
			do(t, srv, "POST", "/api/v2/auth/sign-in", signInBody(email, "wrong"))
		}
	}
	body := `{"name":"New","email":"new@example.com","password":"a long password"}`
	if code, out := do(t, srv, "POST", "/api/v2/auth/signup", body); code != 200 {
		t.Fatalf("sign-up after sign-ins with key-shaped emails: %d %s", code, out)
	}
}

func TestPasswordChangeRevokesTokensAndMCPConnections(t *testing.T) {
	_, st, srv, owner := auditServer(t, "", WithAuthMode(AuthAccounts))
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if code, body := doClient(t, c, srv, "POST", "/api/v2/auth/sign-in", signInBody(owner.Email, ownerPassword)); code != 200 {
		t.Fatalf("sign-in: %d %s", code, body)
	}
	if _, err := st.CreateAPIToken(owner.ID, "cli", auth.HashToken("reet_token")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateMCPConnection(owner.ID, "desk", "read", "mcp-hash", time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if code, _ := do(t, srv, "GET", "/api/v2/auth/me", "", "Authorization", "Bearer reet_token"); code != 200 {
		t.Fatalf("fixture: the token does not work: %d", code)
	}
	body := `{"current":"` + ownerPassword + `","password":"a brand new password"}`
	if code, out := doClient(t, c, srv, "POST", "/api/v2/profile/password", body, "Origin", srv.URL); code != 200 {
		t.Fatalf("change: %d %s", code, out)
	}
	if code, _ := do(t, srv, "GET", "/api/v2/auth/me", "", "Authorization", "Bearer reet_token"); code != 401 {
		t.Fatalf("an API token outlived the password change: %d", code)
	}
	if conns, err := st.MCPConnections(owner.ID); err != nil || len(conns) != 0 {
		t.Fatalf("MCP connections left: %+v %v", conns, err)
	}
	// The browser that made the change stays signed in.
	if code, _ := doClient(t, c, srv, "GET", "/api/v2/auth/me", ""); code != 200 {
		t.Fatalf("the changing browser was signed out: %d", code)
	}
}

func TestANewInviteEndsTheEarlierOneAndJoiningEndsThemAll(t *testing.T) {
	_, st, srv, owner := auditServer(t, "", WithAuthMode(AuthAccounts))
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if code, body := doClient(t, c, srv, "POST", "/api/v2/auth/sign-in", signInBody(owner.Email, ownerPassword)); code != 200 {
		t.Fatalf("sign-in: %d %s", code, body)
	}
	invite := func(role string) string {
		t.Helper()
		code, body := doClient(t, c, srv, "POST", "/api/v2/admin/invites", `{"email":"new@example.com","role":"`+role+`"}`, "Origin", srv.URL)
		var out struct{ Token string }
		if code != 200 || json.Unmarshal([]byte(body), &out) != nil || out.Token == "" {
			t.Fatalf("invite: %d %s", code, body)
		}
		return out.Token
	}
	first, second := invite("admin"), invite("viewer")
	if code, _ := do(t, srv, "GET", "/api/v2/auth/invite?token="+first, ""); code != 404 {
		t.Fatalf("the earlier invite still answers: %d", code)
	}
	if code, body := do(t, srv, "POST", "/api/v2/auth/join", `{"token":"`+first+`","name":"N","password":"a long password"}`); code != 404 {
		t.Fatalf("joining with the earlier, admin invite: %d %s", code, body)
	}
	// A stale invite for the address, as one made before this fix would be.
	if err := st.CreateInvite("unrelated", "someone@example.com", "member", owner.ID, time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if code, body := do(t, srv, "POST", "/api/v2/auth/join", `{"token":"`+second+`","name":"N","password":"a long password"}`); code != 200 {
		t.Fatalf("join: %d %s", code, body)
	}
	u, ok, _ := st.UserByEmail("new@example.com")
	if !ok || u.Role != "viewer" {
		t.Fatalf("joined as %+v", u)
	}
	if _, ok, _ := st.InviteByHash(auth.HashToken(second), time.Now().UnixMilli()); ok {
		t.Fatal("the used invite still works")
	}
	if _, ok, _ := st.InviteByHash("unrelated", time.Now().UnixMilli()); !ok {
		t.Fatal("joining removed another address's invite")
	}
}

func hostStatus(t *testing.T, s *server, host, path string, hdr ...string) int {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	r.Host = host
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	s.auth(s.routes()).ServeHTTP(w, r)
	return w.Code
}

// With no sign-in, a request under a name this instance does not know is
// refused: the name may be a stranger's domain pointed at this machine.
func TestTokenlessInstancesRefuseUnknownHosts(t *testing.T) {
	for name, mode := range map[string]AuthMode{"none": AuthNone, "token mode without a token": AuthToken} {
		t.Run(name, func(t *testing.T) {
			s, _, _, _ := auditServer(t, "", WithAuthMode(mode), WithPublicURL("https://reet.example.com"))
			for _, host := range []string{"localhost", "localhost:8080", "LOCALHOST", "127.0.0.1:8080", "192.168.1.20", "[::1]:8080", "[::1]", "reet.example.com", "reet.example.com:443", "Reet.Example.com.", "homeserver", "homeserver:8080", "reet", "nas.local", "box.lan:8080", "reet.internal", "pi.home.arpa", "app.localhost"} {
				if code := hostStatus(t, s, host, "/api/skills"); code != 200 {
					t.Errorf("host %q: %d", host, code)
				}
			}
			for _, host := range []string{"evil.example", "evil.example:8080", "localhost.evil.example", "reet.example.com.evil.example", "127.0.0.1.evil.example", "local.evil.example", "evil.example.local.evil.example", "box.tail1234.ts.net", ""} {
				if code := hostStatus(t, s, host, "/api/skills"); code != 403 {
					t.Errorf("host %q: %d", host, code)
				}
				if code := hostStatus(t, s, host, "/quick?action=save&url=x"); code != 403 {
					t.Errorf("host %q at /quick: %d", host, code)
				}
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/api/skills", nil)
			r.Host = "evil.example"
			s.auth(s.routes()).ServeHTTP(w, r)
			if !strings.Contains(w.Body.String(), "REET_PUBLIC_URL") {
				t.Fatalf("the refusal does not name the setting: %s", w.Body.String())
			}
		})
	}
	// Where a credential is needed the name is not what keeps strangers out.
	s, _, _, _ := auditServer(t, "s3cret", WithAuthMode(AuthToken))
	if code := hostStatus(t, s, "any.example", "/api/skills", "X-Reet-Token", "s3cret"); code != 200 {
		t.Fatalf("token mode with a token: %d", code)
	}
	s, _, _, _ = auditServer(t, "", WithAuthMode(AuthAccounts))
	if code := hostStatus(t, s, "any.example", "/api/v2/auth/status"); code != 200 {
		t.Fatalf("accounts mode: %d", code)
	}
}

// The token is read from the address only at /quick, which a bookmarklet
// opens as a plain link.
func TestQueryTokenIsReadOnlyAtQuick(t *testing.T) {
	s, _, _, _ := auditServer(t, "s3cret", WithAuthMode(AuthToken))
	if code := hostStatus(t, s, "localhost", "/api/skills?t=s3cret"); code != 401 {
		t.Fatalf("a token in an API address: %d", code)
	}
	if code := hostStatus(t, s, "localhost", "/api/skills", "X-Reet-Token", "s3cret"); code != 200 {
		t.Fatalf("a token in a header: %d", code)
	}
	if code := hostStatus(t, s, "localhost", "/quick?action=save&url="+findSkills); code != 401 {
		t.Fatalf("/quick without a token: %d", code)
	}
	if code := hostStatus(t, s, "localhost", "/quick?t=s3cret&action=save&url="+findSkills); code != 200 {
		t.Fatalf("/quick with its token: %d", code)
	}

	s, st, _, owner := auditServer(t, "", WithAuthMode(AuthAccounts))
	if _, err := st.CreateAPIToken(owner.ID, "cli", auth.HashToken("reet_token")); err != nil {
		t.Fatal(err)
	}
	if code := hostStatus(t, s, "localhost", "/api/v2/auth/me?t=reet_token"); code != 401 {
		t.Fatalf("an account token in an API address: %d", code)
	}
	if code := hostStatus(t, s, "localhost", "/api/v2/auth/me", "Authorization", "Bearer reet_token"); code != 200 {
		t.Fatalf("an account token in a header: %d", code)
	}
	if code := hostStatus(t, s, "localhost", "/quick?t=reet_token&action=save&url="+findSkills); code != 200 {
		t.Fatalf("/quick with an account token: %d", code)
	}
}

// The limit itself is tested in the store; this is the ordinary path.
func TestOAuthRegistrationStillRegisters(t *testing.T) {
	_, _, srv, _ := auditServer(t, "", WithAuthMode(AuthAccounts))
	code, body := do(t, srv, "POST", "/oauth/register", `{"client_name":"c","redirect_uris":["https://client.example/cb"]}`)
	if code != 201 {
		t.Fatalf("register: %d %s", code, body)
	}
}
