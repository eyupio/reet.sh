package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseTrustedProxies(t *testing.T) {
	got, err := ParseTrustedProxies(" 10.0.0.0/8, 192.168.1.7 ,::1, fd00::/8, ::ffff:172.16.0.1,")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.String())
	}
	if want := "10.0.0.0/8 192.168.1.7/32 ::1/128 fd00::/8 172.16.0.1/32"; strings.Join(names, " ") != want {
		t.Fatalf("parsed %v, want %s", names, want)
	}
	if got, err := ParseTrustedProxies("  "); err != nil || len(got) != 0 {
		t.Fatalf("empty setting: %v %v", got, err)
	}
	for _, bad := range []string{"10.0.0.0/33", "proxy.example.com", "10.0.0.1:80"} {
		if _, err := ParseTrustedProxies("10.0.0.1," + bad); err == nil || !strings.Contains(err.Error(), bad) {
			t.Fatalf("%q: error %v does not name the entry", bad, err)
		}
	}
}

func TestClientIPTrustsForwardedForOnlyFromListedProxies(t *testing.T) {
	trusted, err := ParseTrustedProxies("10.0.0.0/8, 2001:db8:ffff::1")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, peer string
		xff        []string
		list       bool
		want       string
	}{
		{"no list: the header is ignored", "10.0.0.5:4000", []string{"203.0.113.9"}, false, "10.0.0.5"},
		{"untrusted peer: the header is ignored", "198.51.100.4:4000", []string{"203.0.113.9"}, true, "198.51.100.4"},
		{"trusted peer: the right-most address", "10.0.0.5:4000", []string{"203.0.113.9"}, true, "203.0.113.9"},
		{"a spoofed left-most entry is not used", "10.0.0.5:4000", []string{"1.1.1.1, 203.0.113.9"}, true, "203.0.113.9"},
		{"several proxies are skipped", "10.0.0.5:4000", []string{"1.1.1.1, 203.0.113.9, 10.1.1.1", "10.2.2.2"}, true, "203.0.113.9"},
		{"only proxies: the left-most proxy", "10.0.0.5:4000", []string{"10.3.3.3, 10.4.4.4"}, true, "10.3.3.3"},
		{"no header: the peer", "10.0.0.5:4000", nil, true, "10.0.0.5"},
		{"garbage stops the walk at the last valid address", "10.0.0.5:4000", []string{"203.0.113.9, nonsense, 10.1.1.1"}, true, "10.1.1.1"},
		{"garbage right after the peer", "10.0.0.5:4000", []string{"203.0.113.9, "}, true, "10.0.0.5"},
		{"IPv6 with brackets and port", "10.0.0.5:4000", []string{"[2001:db8::7]:5555"}, true, "2001:db8::7"},
		{"IPv6 with brackets", "10.0.0.5:4000", []string{"[2001:db8::7]"}, true, "2001:db8::7"},
		{"IPv4 with port", "10.0.0.5:4000", []string{"203.0.113.9:5555"}, true, "203.0.113.9"},
		{"IPv6 zone", "10.0.0.5:4000", []string{"fe80::1%eth0"}, true, "fe80::1"},
		{"IPv6 proxy peer", "[2001:db8:ffff::1]:443", []string{"2001:db8::7"}, true, "2001:db8::7"},
		{"IPv4-mapped peer is trusted", "[::ffff:10.0.0.5]:4000", []string{"203.0.113.9"}, true, "203.0.113.9"},
		{"IPv4-mapped entry", "10.0.0.5:4000", []string{"::ffff:203.0.113.9"}, true, "203.0.113.9"},
		{"IPv4-mapped peer without a list", "[::ffff:198.51.100.4]:4000", nil, false, "198.51.100.4"},
	}
	for _, c := range cases {
		s := &server{}
		if c.list {
			s.trustedProxies = trusted
		}
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.peer
		for _, v := range c.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		if got := s.clientIP(r); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAddrKeyGroupsIPv6By64(t *testing.T) {
	cases := map[string]string{
		"203.0.113.9":          "203.0.113.9",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1": "2001:db8:1:2::/64",
		"2001:db8:1:3::1":      "2001:db8:1:3::/64",
		"not an address":       "not an address",
	}
	for in, want := range cases {
		if got := addrKey(in); got != want {
			t.Errorf("addrKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// Two people behind one reverse proxy: the server sees only the proxy's
// address, and the proxy names each person in X-Forwarded-For.
func TestVisitorsBehindOneProxyKeepTheirOwnAllowance(t *testing.T) {
	for _, listed := range []bool{true, false} {
		e := newSSOEnv(t, ssoOpts{})
		if listed {
			e.srvr.trustedProxies, _ = ParseTrustedProxies("127.0.0.1, ::1")
		}
		a := []string{"X-Forwarded-For", "203.0.113.1"}
		b := []string{"X-Forwarded-For", "198.51.100.2"}
		shared := !listed

		// Single sign-on: A starts until refused; B can still start.
		for i := 0; ; i++ {
			if errorCode(ssoStartFrom(t, e, a)) == "rate_limited" {
				break
			}
			if i > 100 {
				t.Fatal("A was never limited starting single sign-on")
			}
		}
		if limited := errorCode(ssoStartFrom(t, e, b)) == "rate_limited"; limited != shared {
			t.Errorf("listed=%v: B limited starting single sign-on = %v", listed, limited)
		}

		// Password reset: A sends incomplete links until refused.
		for i := 0; ; i++ {
			if code, _ := e.body(e.client(), "POST", "/api/v2/auth/reset", `{"token":"","password":"x"}`, a...); code == http.StatusTooManyRequests {
				break
			}
			if i > 100 {
				t.Fatal("A was never limited at reset")
			}
		}
		code, _ := e.body(e.client(), "POST", "/api/v2/auth/reset", `{"token":"","password":"x"}`, b...)
		if limited := code == http.StatusTooManyRequests; limited != shared {
			t.Errorf("listed=%v: B limited at reset = %v (%d)", listed, limited, code)
		}

		// Password sign-in: A's address has used its failures; B signs in.
		for e.srvr.addrLimiter.Take("ip:" + map[bool]string{true: "203.0.113.1", false: "127.0.0.1"}[listed]) {
		}
		code, body := e.body(e.client(), "POST", "/api/v2/auth/sign-in", signInBody("owner@example.com", ownerPassword), b...)
		if limited := code == http.StatusTooManyRequests; limited != shared || (!shared && code != 200) {
			t.Errorf("listed=%v: B signing in after A's address was spent: %d %s", listed, code, body)
		}
	}
}

func ssoStartFrom(t *testing.T, e *ssoEnv, hdr []string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v2/auth/oidc/start", nil)
	req.Header.Set(hdr[0], hdr[1])
	resp, err := e.client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.Header.Get("Location")
}
