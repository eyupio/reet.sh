package api

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

const (
	sessionCookie = "reet_session"
	sessionTTL    = 30 * 24 * time.Hour
)

type userContextKey struct{}

func (s *server) sessionUser(r *http.Request) (store.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return store.User{}, false
	}
	u, ok, err := s.st.SessionUser(auth.HashToken(c.Value), time.Now().UnixMilli())
	if err != nil {
		return store.User{}, false
	}
	return u, ok
}

// SignedIn reports whether the request already carries a valid account
// session. The SPA's static landing fallback is only for visitors who might
// still sign in, so a signed-in request must not be served it (that is the
// logged-out page a refresh would otherwise flash before the app mounts).
func (s *server) SignedIn(r *http.Request) bool {
	if s.authMode != AuthAccounts {
		return false
	}
	_, ok := s.sessionUser(r)
	return ok
}

func userFrom(r *http.Request) (store.User, bool) {
	u, ok := r.Context().Value(userContextKey{}).(store.User)
	return u, ok
}

func (s *server) currentUser(r *http.Request) store.User {
	if u, ok := userFrom(r); ok {
		return u
	}
	return s.defaultUser
}

func (s *server) uid(r *http.Request) int64 { return s.currentUser(r).ID }

func (s *server) role(r *http.Request) string {
	u := s.currentUser(r)
	if u.Role == "" {
		return "owner"
	}
	return u.Role
}

func (s *server) requireWriter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.role(r) == "viewer" {
			writeErr(w, http.StatusForbidden, "this account is read-only")
			return
		}
		next(w, r)
	}
}

func (s *server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if role := s.role(r); role != "owner" && role != "admin" {
			writeErr(w, http.StatusForbidden, "admins only")
			return
		}
		next(w, r)
	}
}

func (s *server) requireOwner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.role(r) != "owner" {
			writeErr(w, http.StatusForbidden, "the owner only")
			return
		}
		next(w, r)
	}
}

func (s *server) setSession(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure:   s.sessionSecure(r),
		SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
	})
}

func (s *server) clearSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure:   s.sessionSecure(r),
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func (s *server) sessionSecure(r *http.Request) bool {
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	// A TLS-terminating proxy may omit forwarded headers. The configured HTTPS
	// origin must still prevent this privileged browser cookie travelling over HTTP.
	u, err := url.Parse(s.publicOrigin())
	return err == nil && u.Scheme == "https" && u.Host != ""
}

func (s *server) newSession(w http.ResponseWriter, r *http.Request, user store.User) (store.User, error) {
	token, err := auth.NewToken()
	if err != nil {
		return user, err
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.st.CreateSession(auth.HashToken(token), user.ID, expires.UnixMilli(), r.UserAgent()); err != nil {
		return user, err
	}
	s.setSession(w, r, token, expires)
	return user, nil
}

// clientIP is the address a request came from. X-Forwarded-For is read only
// when the peer is a trusted proxy: walking it from the right, entries that
// are trusted proxies are skipped and the first other address is the client.
// An entry that is not an address stops the walk at the last one reached, as
// anything to its left was written by someone the proxies did not vouch for.
func (s *server) clientIP(r *http.Request) string {
	addr, ok := parseIP(r.RemoteAddr)
	if !ok {
		return r.RemoteAddr
	}
	if !s.trustedProxy(addr) {
		return addr.String()
	}
	lines := r.Header.Values("X-Forwarded-For")
	for i := len(lines) - 1; i >= 0; i-- {
		entries := strings.Split(lines[i], ",")
		for j := len(entries) - 1; j >= 0; j-- {
			next, ok := parseIP(entries[j])
			if !ok {
				return addr.String()
			}
			addr = next
			if !s.trustedProxy(addr) {
				return addr.String()
			}
		}
	}
	return addr.String()
}

func (s *server) trustedProxy(addr netip.Addr) bool {
	for _, p := range s.trustedProxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// parseIP reads an address with or without brackets, port and zone, with an
// IPv4-mapped IPv6 address read as the IPv4 address it carries.
func parseIP(v string) (netip.Addr, bool) {
	v = strings.TrimSpace(v)
	if host, _, err := net.SplitHostPort(v); err == nil {
		v = host
	} else if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		v = v[1 : len(v)-1]
	}
	addr, err := netip.ParseAddr(v)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.WithZone("").Unmap(), true
}

// addrKey is the part of an address a limiter counts: an IPv4 address whole,
// an IPv6 address by its /64, as one subscriber is usually given a whole /64.
func addrKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil || addr.Is4() {
		return ip
	}
	return netip.PrefixFrom(addr, 64).Masked().String()
}

// clientKey is the limiter key for the address a request came from.
func (s *server) clientKey(r *http.Request) string {
	return addrKey(s.clientIP(r))
}

// ParseTrustedProxies reads REET_TRUSTED_PROXIES: addresses and CIDR ranges,
// separated by commas.
func ParseTrustedProxies(v string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, entry := range strings.Split(v, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if p, err := netip.ParsePrefix(entry); err == nil {
			// Peers are compared as plain IPv4, so a mapped range is too.
			if p.Addr().Is4In6() {
				if p.Bits() < 96 {
					return nil, fmt.Errorf("REET_TRUSTED_PROXIES: %q reaches past the IPv4-mapped addresses; write the IPv4 range instead", entry)
				}
				p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
			}
			if p.Bits() == 0 {
				return nil, fmt.Errorf("REET_TRUSTED_PROXIES: %q trusts every address, so every visitor could choose their own; list only your proxy's address", entry)
			}
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil || addr.Zone() != "" {
			return nil, fmt.Errorf("REET_TRUSTED_PROXIES: %q is not an IP address or CIDR range", entry)
		}
		addr = addr.Unmap()
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}
