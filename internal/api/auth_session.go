package api

import (
	"net"
	"net/http"
	"net/url"
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

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
