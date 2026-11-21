package api

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) authStatus(w http.ResponseWriter, r *http.Request) {
	mode := string(s.authMode)
	setupRequired, bootstrapRequired := false, false
	var user *store.User
	switch s.authMode {
	case AuthToken:
		if s.token == "" {
			mode = string(AuthNone)
		}
	case AuthAccounts:
		n, err := s.st.UserCount()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not check accounts")
			return
		}
		setupRequired = n == 0
		bootstrapRequired = setupRequired && s.token != ""
		if u, ok := s.sessionUser(r); ok {
			user = &u
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":              mode,
		"setupRequired":     setupRequired,
		"bootstrapRequired": bootstrapRequired,
		"signupEnabled":     s.authMode == AuthAccounts && s.signup && !setupRequired,
		"supportDesk":       s.supportDesk,
		"contactEmail":      s.contactEmail,
		"passwordReset":     s.mail != nil && s.mail.Delivers(),
		"user":              user,
		"sso":               s.ssoStatus(),
	})
}

func (s *server) setup(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	n, err := s.st.UserCount()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not check accounts")
		return
	}
	if n > 0 {
		writeErr(w, http.StatusConflict, "this instance is already set up")
		return
	}
	if s.token != "" && !s.hasBootstrap(r) {
		writeErr(w, http.StatusUnauthorized, "the bootstrap token is required")
		return
	}
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") || len(email) > 254 {
		writeErr(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	if len(name) > 100 {
		writeErr(w, http.StatusBadRequest, "name is too long")
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(in.Password) > 1024 {
		writeErr(w, http.StatusBadRequest, "password is too long")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create the account")
		return
	}
	user, created, err := s.st.CreateFirstOwner(email, name, "owner", hash)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !created {
		writeErr(w, http.StatusConflict, "this instance is already set up")
		return
	}
	if err := s.st.MigrateToOwner(user.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not migrate the library")
		return
	}
	if _, err := s.newSession(w, r, user); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	s.event(user.ID, "ui", "setup", "", map[string]any{"name": user.Name, "email": user.Email})
	writeJSON(w, http.StatusOK, user)
}

// signUp creates an ordinary member account when the operator opened signups.
// The first account must still go through setup so every instance has an owner.
func (s *server) signUp(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	if !s.signup {
		writeErr(w, http.StatusForbidden, "sign-ups are closed on this instance")
		return
	}
	n, err := s.st.UserCount()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not check accounts")
		return
	}
	if n == 0 {
		writeErr(w, http.StatusConflict, "this instance is not set up yet")
		return
	}
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") || len(email) > 254 {
		writeErr(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	name := strings.TrimSpace(in.Name)
	if len(name) > 100 {
		writeErr(w, http.StatusBadRequest, "name is too long")
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(in.Password) > 1024 {
		writeErr(w, http.StatusBadRequest, "password is too long")
		return
	}
	if !s.limiter.Take("signup:all", "signup-ip:"+clientIP(r), "email:"+email) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create the account")
		return
	}
	user, err := s.st.CreateUser(email, name, "member", hash)
	if errors.Is(err, store.ErrUserExists) {
		writeErr(w, http.StatusConflict, "that email is already registered")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.newSession(w, r, user); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	s.event(user.ID, "ui", "signup", "", map[string]any{"name": user.Name, "email": user.Email})
	if s.mail != nil {
		if err := s.mail.Welcome(r.Context(), user); err != nil {
			log.Printf("email: queueing a welcome: %v", err)
		}
	}
	s.emitNewAccount(user)
	writeJSON(w, http.StatusOK, user)
}

func (s *server) signIn(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	// No address is longer, and the address is a limiter key below.
	if len(email) > 254 {
		writeErr(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	// The address the server sees is charged for failed attempts only: the
	// attempt is recorded now and taken back when the password is right, so
	// people behind one proxy do not use up each other's allowance.
	ipKey := "ip:" + clientIP(r)
	allowed := s.addrLimiter.Take(ipKey) && s.limiter.Take("email:"+email)
	if !allowed {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	user, ok, err := s.st.UserByEmail(email)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		_ = auth.VerifyPassword(s.dummyHash, in.Password)
		writeErr(w, http.StatusUnauthorized, "email or password is not correct")
		return
	}
	good := auth.VerifyPassword(user.PasswordHash, in.Password)
	// Where password sign-in is off for everyone but the owner, a right
	// password is answered exactly as a wrong one: a different answer would
	// tell anybody guessing whether a leaked password is still good. The sign-in
	// page already points people at single sign-on.
	passwordsOff := s.ssoOn() && !s.sso.PasswordLogin() && user.Role != "owner"
	unusable := user.DisabledAt != 0 || !good
	if unusable || passwordsOff {
		writeErr(w, http.StatusUnauthorized, "email or password is not correct")
		return
	}
	s.addrLimiter.Forgive(ipKey)
	if _, err := s.newSession(w, r, user); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	s.event(user.ID, "ui", "sign-in", "", map[string]any{"name": user.Name, "email": user.Email})
	writeJSON(w, http.StatusOK, user)
}

func (s *server) signOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		hash := auth.HashToken(c.Value)
		if user, ok, _ := s.st.SessionUser(hash, time.Now().UnixMilli()); ok {
			s.event(user.ID, "ui", "sign-out", "", map[string]any{"name": user.Name, "email": user.Email})
		}
		_ = s.st.DeleteSession(hash)
		s.endSessions([]string{hash})
	}
	s.clearSession(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	if user, ok := userFrom(r); ok {
		writeJSON(w, http.StatusOK, user)
		return
	}
	if s.authMode == AuthNone {
		writeJSON(w, http.StatusOK, map[string]any{"name": "", "role": "owner", "implicit": true})
		return
	}
	writeErr(w, http.StatusUnauthorized, "sign in required")
}
