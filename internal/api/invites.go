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

func (s *server) adminInvite(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") || len(email) > 254 {
		writeErr(w, 400, "enter a valid email address")
		return
	}
	if _, exists, _ := s.st.UserByEmail(email); exists {
		writeErr(w, http.StatusConflict, "that email is already registered")
		return
	}
	role := in.Role
	if role == "" {
		role = "member"
	}
	validRole := role == "admin" || role == "member" || role == "viewer"
	if !validRole {
		writeErr(w, 400, "invalid role")
		return
	}
	actor := s.currentUser(r)
	if role == "admin" && actor.Role != "owner" {
		writeErr(w, 403, "only the owner can invite an admin")
		return
	}
	raw, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "could not create an invite")
		return
	}
	expires := time.Now().Add(7 * 24 * time.Hour)
	if err := s.st.CreateInvite(auth.HashToken(raw), email, role, actor.ID, expires.UnixMilli()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(actor.ID, "ui", "invite", "", map[string]any{"email": email, "role": role})
	emailed := false
	if s.mail != nil && s.mail.Delivers() {
		if err := s.mail.Invite(r.Context(), email, role, s.mailBase(r)+"/join?token="+raw, expires, actor.Name); err != nil {
			log.Printf("email: queueing an invitation: %v", err)
		} else {
			emailed = true
		}
	}
	writeJSON(w, 200, map[string]any{
		"url":       "/join?token=" + raw,
		"token":     raw,
		"email":     email,
		"role":      role,
		"expiresAt": expires.UnixMilli(),
		"emailed":   emailed,
	})
}

func (s *server) inviteInfo(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		writeErr(w, 400, "token required")
		return
	}
	invite, ok, err := s.st.InviteByHash(auth.HashToken(token), time.Now().UnixMilli())
	if err != nil || !ok {
		writeErr(w, 404, "invite not found or expired")
		return
	}
	writeJSON(w, 200, invite)
}

func (s *server) join(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	invite, ok, err := s.st.InviteByHash(auth.HashToken(in.Token), time.Now().UnixMilli())
	if err != nil || !ok {
		writeErr(w, 404, "invite not found or expired")
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, 400, "password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, 500, "could not create the account")
		return
	}
	user, err := s.st.CreateUser(invite.Email, strings.TrimSpace(in.Name), invite.Role, hash)
	if errors.Is(err, store.ErrUserExists) {
		writeErr(w, 409, "that email is already registered")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Every invite for the address goes, not only the one used: an older one
	// may carry another role.
	_ = s.st.DeleteInvitesFor(invite.Email)
	if _, err := s.newSession(w, r, user); err != nil {
		writeErr(w, 500, "could not start a session")
		return
	}
	s.event(user.ID, "ui", "join", "", map[string]any{"name": user.Name, "email": user.Email})
	if s.mail != nil {
		if err := s.mail.Welcome(r.Context(), user); err != nil {
			log.Printf("email: queueing a welcome: %v", err)
		}
	}
	s.emitNewAccount(user)
	writeJSON(w, 200, user)
}
