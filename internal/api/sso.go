package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/oidc"
	"github.com/eyupio/reet.sh/internal/store"
)

// Single sign-on: a person signs in at an identity provider and comes back
// with a session of the same kind a password gives. Everything that can go
// wrong sends the browser home with a fixed error code in the address and
// nothing the provider said, so a crafted link cannot put words on the page.

const (
	ssoBindingCookie = "reet_sso"
	ssoCookiePath    = "/api/v2/auth/oidc"
	ssoLogoutPath    = "/api/v2/auth/oidc/backchannel-logout"
	ssoAttemptLife   = 10 * time.Minute
)

// WithOIDC turns on sign-in through an identity provider. It needs accounts
// mode; in any other mode the routes answer that accounts are not enabled.
func WithOIDC(p *oidc.Provider) Option { return func(s *server) { s.sso = p } }

func (s *server) ssoOn() bool { return s.sso != nil && s.authMode == AuthAccounts }

func (s *server) registerSSORoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/auth/oidc/start", s.ssoStart)
	mux.HandleFunc("GET /api/v2/auth/oidc/callback", s.ssoCallback)
	mux.HandleFunc("POST "+ssoLogoutPath, s.ssoBackchannelLogout)
	mux.HandleFunc("GET /api/v2/auth/oidc/identity", s.ssoIdentity)
	mux.HandleFunc("DELETE /api/v2/auth/oidc/identity", s.ssoUnlink)
	mux.HandleFunc("GET /api/v2/admin/sso", s.requireAdmin(s.ssoAdminStatus))
}

// ssoStatus is the part of auth/status that tells the sign-in page what to offer.
func (s *server) ssoStatus() map[string]any {
	if !s.ssoOn() {
		return map[string]any{"enabled": false}
	}
	return map[string]any{"enabled": true, "label": s.sso.Label(), "passwordLogin": s.sso.PasswordLogin()}
}

// safeNext is where a sign-in resumes: a path on this site, and nothing a
// browser could read as another address. Anything else becomes the home page.
func safeNext(raw string) string {
	if raw == "" || len(raw) > 2048 || raw[0] != '/' || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "\\") {
		return "/"
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "/"
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") || strings.Contains(u.Path, "\\") {
		return "/"
	}
	// An API address is data, not a page to land on.
	if u.Path == "/api" || strings.HasPrefix(u.Path, "/api/") {
		return "/"
	}
	return raw
}

func withParam(path, key, value string) string {
	u, err := url.Parse(path)
	if err != nil {
		return "/?" + url.QueryEscape(key) + "=" + url.QueryEscape(value)
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}

// ssoNoStore keeps what carries a code or a state out of caches and
// referrers.
func ssoNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func (s *server) ssoGone(w http.ResponseWriter) bool {
	if !s.ssoOn() {
		writeErr(w, http.StatusNotFound, "single sign-on is not enabled")
		return true
	}
	return false
}

func (s *server) ssoFail(w http.ResponseWriter, r *http.Request, back, code string) {
	ssoNoStore(w)
	http.Redirect(w, r, withParam(back, "sso_error", code), http.StatusFound)
}

func (s *server) ssoClearBinding(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: ssoBindingCookie, Value: "", Path: ssoCookiePath, HttpOnly: true,
		Secure: s.sessionSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func (s *server) ssoStart(w http.ResponseWriter, r *http.Request) {
	if s.ssoGone(w) {
		return
	}
	ssoNoStore(w)
	key := "sso-start:" + clientIP(r)
	if !s.ssoLimiter.Allow(key) {
		s.ssoFail(w, r, "/", "rate_limited")
		return
	}
	s.ssoLimiter.Hit(key)
	if n, err := s.st.UserCount(); err != nil || n == 0 {
		s.ssoFail(w, r, "/", "not_ready")
		return
	}
	q := r.URL.Query()
	next := safeNext(q.Get("next"))
	intent, userID := "signin", int64(0)
	if q.Get("intent") == "link" {
		u, ok := s.sessionUser(r)
		if !ok {
			s.ssoFail(w, r, "/", "signin_required")
			return
		}
		intent, userID = "link", u.ID
	}
	state, nonce, verifier, binding := oidc.Random(32), oidc.Random(32), oidc.Random(48), oidc.Random(32)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	authURL, err := s.sso.AuthURL(ctx, state, nonce, verifier)
	if err != nil {
		log.Printf("single sign-on: start: %v", err)
		s.ssoFail(w, r, next, "unavailable")
		return
	}
	at := time.Now()
	err = s.st.CreateSSOLogin(store.SSOLogin{
		StateHash: auth.HashToken(state), BindingHash: auth.HashToken(binding), Nonce: nonce, Verifier: verifier,
		Next: next, Intent: intent, UserID: userID, ExpiresAt: at.Add(ssoAttemptLife).UnixMilli(),
	}, at.UnixMilli())
	switch {
	case errors.Is(err, store.ErrSSOBusy):
		s.ssoFail(w, r, "/", "busy")
		return
	case err != nil:
		log.Printf("single sign-on: start: %v", err)
		s.ssoFail(w, r, "/", "failed")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: ssoBindingCookie, Value: binding, Path: ssoCookiePath, HttpOnly: true,
		Secure: s.sessionSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: int(ssoAttemptLife.Seconds())})
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *server) ssoCallback(w http.ResponseWriter, r *http.Request) {
	if s.ssoGone(w) {
		return
	}
	ssoNoStore(w)
	q := r.URL.Query()
	binding, _ := r.Cookie(ssoBindingCookie)
	s.ssoClearBinding(w, r)
	state := q.Get("state")
	if state == "" || len(state) > 512 {
		s.ssoFail(w, r, "/", "expired")
		return
	}
	// Spending the attempt comes first: whatever follows, this state is done.
	row, ok, err := s.st.ConsumeSSOLogin(auth.HashToken(state), time.Now().UnixMilli())
	if err != nil {
		log.Printf("single sign-on: callback: %v", err)
		s.ssoFail(w, r, "/", "failed")
		return
	}
	if !ok {
		s.ssoFail(w, r, "/", "expired")
		return
	}
	back := "/"
	if row.Intent == "link" {
		back = row.Next
	}
	if binding == nil || subtle.ConstantTimeCompare([]byte(auth.HashToken(binding.Value)), []byte(row.BindingHash)) != 1 {
		s.ssoFail(w, r, back, "mismatch")
		return
	}
	if n, err := s.st.UserCount(); err != nil || n == 0 {
		s.ssoFail(w, r, back, "not_ready")
		return
	}
	if q.Get("error") != "" {
		s.ssoFail(w, r, back, "denied")
		return
	}
	code := q.Get("code")
	if code == "" || len(code) > 4096 {
		s.ssoFail(w, r, back, "failed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	claims, err := s.sso.Exchange(ctx, code, row.Verifier, row.Nonce)
	if err != nil {
		log.Printf("single sign-on: sign-in refused: %v", err)
		if errors.Is(err, oidc.ErrUnreachable) {
			s.ssoFail(w, r, back, "unavailable")
		} else {
			s.ssoFail(w, r, back, "failed")
		}
		return
	}
	if row.Intent == "link" {
		s.ssoLinkCallback(w, r, row, claims)
		return
	}
	s.ssoSignIn(w, r, row, claims)
}

func (s *server) ssoLinkCallback(w http.ResponseWriter, r *http.Request, row store.SSOLogin, c oidc.Claims) {
	cur, ok := s.sessionUser(r)
	if !ok || cur.ID != row.UserID {
		s.ssoFail(w, r, row.Next, "signin_required")
		return
	}
	switch err := s.st.LinkSSOIdentity(s.sso.Issuer(), c.Subject, cur.ID); {
	case errors.Is(err, store.ErrSSOLinkedElsewhere):
		s.ssoFail(w, r, row.Next, "linked_elsewhere")
		return
	case errors.Is(err, store.ErrSSOAlreadyLinked):
		s.ssoFail(w, r, row.Next, "already_linked")
		return
	case err != nil:
		log.Printf("single sign-on: link: %v", err)
		s.ssoFail(w, r, row.Next, "failed")
		return
	}
	s.event(cur.ID, "ui", "sso-link", "", map[string]any{"name": cur.Name, "email": cur.Email})
	http.Redirect(w, r, row.Next, http.StatusFound)
}

func (s *server) ssoSignIn(w http.ResponseWriter, r *http.Request, row store.SSOLogin, c oidc.Claims) {
	issuer := s.sso.Issuer()
	email := strings.ToLower(strings.TrimSpace(c.Email))
	user, found, err := s.st.SSOIdentityUser(issuer, c.Subject)
	if err != nil {
		log.Printf("single sign-on: lookup: %v", err)
		s.ssoFail(w, r, "/", "failed")
		return
	}
	created := false
	if !found {
		// A person we have not met. Their email is only worth anything if the
		// provider vouches for it: without that, anybody who can register one
		// there could claim another person's address here.
		switch {
		case email == "" || !strings.Contains(email, "@") || len(email) > 254:
			s.ssoFail(w, r, "/", "email_missing")
			return
		case !c.EmailVerified:
			s.ssoFail(w, r, "/", "email_unverified")
			return
		}
		existing, exists, err := s.st.UserByEmail(email)
		if err != nil {
			log.Printf("single sign-on: lookup: %v", err)
			s.ssoFail(w, r, "/", "failed")
			return
		}
		switch {
		case exists && !s.sso.LinkByEmail():
			s.ssoFail(w, r, "/", "exists")
			return
		case exists && (existing.Role == "owner" || existing.Role == "admin"):
			// Matching an address must never hand over the keys.
			s.ssoFail(w, r, "/", "not_linkable")
			return
		case exists:
			switch err := s.st.LinkSSOIdentity(issuer, c.Subject, existing.ID); {
			case errors.Is(err, store.ErrSSOAlreadyLinked):
				s.ssoFail(w, r, "/", "already_linked")
				return
			case errors.Is(err, store.ErrSSOLinkedElsewhere):
				s.ssoFail(w, r, "/", "linked_elsewhere")
				return
			case err != nil:
				log.Printf("single sign-on: link: %v", err)
				s.ssoFail(w, r, "/", "failed")
				return
			}
			user = existing
			s.event(user.ID, "ui", "sso-link", "", map[string]any{"name": user.Name, "email": user.Email, "by": "email"})
		case !s.sso.AutoCreate():
			s.ssoFail(w, r, "/", "no_account")
			return
		default:
			role, _ := s.sso.RoleFor(c.Groups)
			user, err = s.st.CreateSSOUser(issuer, c.Subject, email, c.Name, role)
			if err != nil {
				log.Printf("single sign-on: create account: %v", err)
				s.ssoFail(w, r, "/", "failed")
				return
			}
			created = true
		}
	}
	if user.DisabledAt != 0 {
		s.ssoFail(w, r, "/", "disabled")
		return
	}
	if !created && user.Role != "owner" && c.HasGroups {
		// Where groups are mapped the provider decides the role at each
		// sign-in. A token with no groups claim at all changes nothing.
		if role, mapped := s.sso.RoleFor(c.Groups); mapped && role != user.Role {
			if err := s.st.UpdateUserRole(user.ID, role); err == nil {
				s.event(user.ID, "ui", "user-role", "", map[string]any{"name": user.Name, "email": user.Email, "role": role, "by": "single sign-on"})
				user.Role = role
			}
		}
	}
	token, err := auth.NewToken()
	if err != nil {
		s.ssoFail(w, r, "/", "failed")
		return
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.st.CreateSSOSession(auth.HashToken(token), user.ID, expires.UnixMilli(), r.UserAgent(), issuer, c.SessionID, c.Subject); err != nil {
		log.Printf("single sign-on: session: %v", err)
		s.ssoFail(w, r, "/", "failed")
		return
	}
	s.setSession(w, r, token, expires)
	s.st.TouchSSOIdentity(issuer, c.Subject)
	if created {
		s.event(user.ID, "ui", "sso-create", "", map[string]any{"name": user.Name, "email": user.Email, "role": user.Role})
		s.emitNewAccount(user)
	}
	s.event(user.ID, "ui", "sso-sign-in", "", map[string]any{"name": user.Name, "email": user.Email})
	http.Redirect(w, r, row.Next, http.StatusFound)
}

// ssoBackchannelLogout ends the sessions an identity provider names when
// somebody signs out there (OpenID Connect Back-Channel Logout 1.0).
func (s *server) ssoBackchannelLogout(w http.ResponseWriter, r *http.Request) {
	if s.ssoGone(w) {
		return
	}
	ssoNoStore(w)
	key := "sso-logout:" + clientIP(r)
	if !s.logoutLimiter.Allow(key) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many logout requests")
		return
	}
	s.logoutLimiter.Hit(key)
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	raw := r.PostForm.Get("logout_token")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	claims, err := s.sso.VerifyLogoutToken(ctx, raw)
	if err != nil {
		log.Printf("single sign-on: logout token refused: %v", err)
		writeErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	ids, err := s.st.DeleteSSOSessions(s.sso.Issuer(), claims.Subject, claims.SessionID)
	if err != nil {
		log.Printf("single sign-on: logout: %v", err)
		writeErr(w, http.StatusInternalServerError, "could not end the sessions")
		return
	}
	s.endSessions(ids)
	if len(ids) > 0 && claims.Subject != "" {
		if u, ok, _ := s.st.SSOIdentityUser(s.sso.Issuer(), claims.Subject); ok {
			s.event(u.ID, "ui", "sso-logout", "", map[string]any{"name": u.Name, "email": u.Email, "sessions": len(ids)})
		}
	}
	w.WriteHeader(http.StatusOK)
}

// endSessions closes what hangs off sessions that have just been ended: an
// open terminal is bound to the browser login it began under.
func (s *server) endSessions(ids []string) {
	for _, id := range ids {
		s.terminals.RevokeSource(id)
	}
	if s.afterSessionsEnded != nil {
		s.afterSessionsEnded(ids)
	}
}

// canUnlinkSSO is whether somebody would still have a way in without their
// link: a password, and the instance letting them use one (the owner always
// may).
func (s *server) canUnlinkSSO(u store.User) bool {
	return u.PasswordHash != "" && (u.Role == "owner" || s.sso.PasswordLogin())
}

// ssoIdentity says whether the signed-in person has linked an identity.
func (s *server) ssoIdentity(w http.ResponseWriter, r *http.Request) {
	if !s.ssoOn() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	u := s.currentUser(r)
	id, linked, err := s.st.SSOIdentityFor(u.ID, s.sso.Issuer())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read the link")
		return
	}
	out := map[string]any{"enabled": true, "label": s.sso.Label(), "linked": linked, "hasPassword": u.PasswordHash != ""}
	if linked {
		out["linkedAt"] = id.CreatedAt
		if id.LastLoginAt != 0 {
			out["lastLoginAt"] = id.LastLoginAt
		}
		out["canUnlink"] = s.canUnlinkSSO(u)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) ssoUnlink(w http.ResponseWriter, r *http.Request) {
	if s.ssoGone(w) {
		return
	}
	u := s.currentUser(r)
	if _, linked, err := s.st.SSOIdentityFor(u.ID, s.sso.Issuer()); err != nil || !linked {
		writeErr(w, http.StatusNotFound, "no identity is linked to this account")
		return
	}
	if !s.canUnlinkSSO(u) {
		writeErr(w, http.StatusConflict, "this would leave no way to sign in: set a password first, or ask an admin")
		return
	}
	if err := s.st.UnlinkSSOIdentity(u.ID, s.sso.Issuer()); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not unlink")
		return
	}
	s.event(u.ID, "ui", "sso-unlink", "", map[string]any{"name": u.Name, "email": u.Email})
	w.WriteHeader(http.StatusNoContent)
}

// ssoAdminStatus shows an admin how single sign-on is set up. It is read-only
// and never carries the client secret.
func (s *server) ssoAdminStatus(w http.ResponseWriter, r *http.Request) {
	if !s.ssoOn() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	cfg := s.sso.Config()
	linked, _ := s.st.SSOIdentityCount(cfg.Issuer)
	out := map[string]any{
		"enabled": true, "issuer": cfg.Issuer, "redirectUrl": cfg.RedirectURL, "label": s.sso.Label(),
		"passwordLogin": cfg.PasswordLogin, "autoCreate": cfg.AutoCreate, "linkByEmail": cfg.LinkByEmail,
		"defaultRole": cfg.DefaultRole, "groupsClaim": cfg.GroupsClaim,
		"adminGroups": nonNil(cfg.AdminGroups), "memberGroups": nonNil(cfg.MemberGroups), "viewerGroups": nonNil(cfg.ViewerGroups),
		"linked": linked,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.sso.Check(ctx); err != nil {
		out["reachable"] = false
		out["problem"] = err.Error()
	} else {
		out["reachable"] = true
	}
	writeJSON(w, http.StatusOK, out)
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
