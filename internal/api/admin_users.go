package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	users, err := s.st.ListUsersMeta()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"users": users})
}

func (s *server) adminTarget(w http.ResponseWriter, r *http.Request, id int64) (store.User, bool) {
	target, ok, err := s.st.UserByID(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return store.User{}, false
	}
	if !ok {
		writeErr(w, 404, "user not found")
		return store.User{}, false
	}
	actor := s.currentUser(r)
	if target.ID == actor.ID {
		writeErr(w, 400, "use another admin account to change this one")
		return store.User{}, false
	}
	if target.Role == "owner" {
		writeErr(w, 403, "the owner account cannot be changed here")
		return store.User{}, false
	}
	if actor.Role == "admin" && target.Role == "admin" {
		writeErr(w, 403, "only the owner can change an admin")
		return store.User{}, false
	}
	return target, true
}

func (s *server) adminUserUpdate(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		ID       int64   `json:"id"`
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, ok := s.adminTarget(w, r, in.ID)
	if !ok {
		return
	}
	actor := s.currentUser(r)
	if in.Role != nil {
		role := *in.Role
		if role == "owner" {
			writeErr(w, 400, "use transfer ownership")
			return
		}
		validRole := role == "admin" || role == "member" || role == "viewer"
		if !validRole {
			writeErr(w, 400, "invalid role")
			return
		}
		if role == "admin" && actor.Role != "owner" {
			writeErr(w, 403, "only the owner can make an admin")
			return
		}
		if err := s.st.UpdateUserRole(target.ID, role); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.event(actor.ID, "ui", "user-role", "", map[string]any{"name": target.Name, "email": target.Email, "role": role})
	}
	if in.Disabled != nil {
		if err := s.st.SetUserDisabled(target.ID, *in.Disabled); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if *in.Disabled {
			// Their open terminals are bound to the sessions being ended.
			ids, _ := s.st.UserSessionIDs(target.ID)
			_ = s.st.DeleteUserSessions(target.ID)
			_ = s.st.RevokeUserTokens(target.ID)
			s.endSessions(ids)
		}
		s.event(actor.ID, "ui", "user-disable", "", map[string]any{"name": target.Name, "email": target.Email, "disabled": *in.Disabled})
	}
	s.adminUsers(w, r)
}

func (s *server) adminUserDelete(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id == 0 {
		writeErr(w, 400, "id required")
		return
	}
	target, ok := s.adminTarget(w, r, id)
	if !ok {
		return
	}
	ids, _ := s.st.UserSessionIDs(target.ID)
	if err := s.st.DeleteUser(target.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.endSessions(ids)
	s.eventRequest(r, "user-delete", "", map[string]any{"name": target.Name, "email": target.Email})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) adminUserReset(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		ID int64 `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, ok := s.adminTarget(w, r, in.ID)
	if !ok {
		return
	}
	if s.mail != nil && s.mail.Delivers() {
		if err := s.mail.PasswordReset(r.Context(), target, mail.StaffResetTTL); err != nil {
			writeErr(w, 500, "could not send the reset link: "+err.Error())
			return
		}
		s.eventRequest(r, "user-reset", "", map[string]any{"name": target.Name, "email": target.Email, "emailed": true})
		writeJSON(w, 200, map[string]any{"emailed": true, "email": target.Email})
		return
	}
	raw, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "could not reset the password")
		return
	}
	password := raw[:16]
	hash, err := auth.HashPassword(password)
	if err != nil {
		writeErr(w, 500, "could not reset the password")
		return
	}
	if err := s.st.SetUserPassword(target.ID, hash); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.st.RevokeUserTokens(target.ID)
	s.eventRequest(r, "user-reset", "", map[string]any{"name": target.Name, "email": target.Email, "emailed": false})
	writeJSON(w, 200, map[string]any{"password": password})
}

func (s *server) adminUserRevoke(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		ID int64 `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, ok := s.adminTarget(w, r, in.ID)
	if !ok {
		return
	}
	if err := s.st.RevokeUserTokens(target.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "user-revoke", "", map[string]any{"name": target.Name, "email": target.Email})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) adminUserTransfer(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		ID int64 `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, ok, err := s.st.UserByID(in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "user not found")
		return
	}
	actor := s.currentUser(r)
	if target.ID == actor.ID {
		writeErr(w, 400, "already the owner")
		return
	}
	if err := s.st.TransferOwner(actor.ID, target.ID); err != nil {
		code := 400
		if errors.Is(err, store.ErrNotOwner) {
			code = 403
		}
		writeErr(w, code, err.Error())
		return
	}
	s.event(actor.ID, "ui", "user-transfer", "", map[string]any{"name": target.Name, "email": target.Email})
	s.adminUsers(w, r)
}
