package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) listTokens(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	tokens, err := s.st.ListAPITokens(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"tokens": tokens})
}

func (s *server) createToken(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	raw, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "could not create a token")
		return
	}
	token := "reet_" + raw
	tok, err := s.st.CreateAPIToken(s.uid(r), in.Name, auth.HashToken(token))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "token-create", "", map[string]any{"name": tok.Name})
	writeJSON(w, 200, map[string]any{"id": tok.ID, "name": tok.Name, "token": token, "createdAt": tok.CreatedAt})
}

func (s *server) revokeToken(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id == 0 {
		writeErr(w, 400, "id required")
		return
	}
	err = s.st.RevokeAPIToken(s.uid(r), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "token not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "token-revoke", "", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}
