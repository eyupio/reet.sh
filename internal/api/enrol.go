package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

const (
	enrolPath       = "/api/v2/sync/agents"
	enrolCodePrefix = "reet_enrol_"
	enrolCodeLife   = 15 * time.Minute
	// maxEnrolCodes is how many unused, unexpired codes one account may hold.
	maxEnrolCodes = 20
	badEnrolCode  = "this enrolment code is not valid; make a new one under Machines, Add a machine"
)

// enrolCodeKey carries an enrolment code from auth to the enrolment handler.
type enrolCodeKey struct{}

// apiTokenKey marks a request a personal API token authenticated.
type apiTokenKey struct{}

func usedAPIToken(r *http.Request) bool { v, _ := r.Context().Value(apiTokenKey{}).(bool); return v }

func (s *server) tokenManagement(next http.HandlerFunc) http.HandlerFunc {
	return s.browserOnly("manage API tokens", "accounts are not enabled", next)
}

// createEnrolCode makes a single-use code that enrols one machine for the
// signed-in account. Only its hash is kept.
func (s *server) createEnrolCode(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	if !s.enrolLimiter.Take("enrol-code:" + strconv.FormatInt(uid, 10)) {
		writeErr(w, http.StatusTooManyRequests, "too many enrolment codes made; try again later")
		return
	}
	raw, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "could not make an enrolment code")
		return
	}
	code := enrolCodePrefix + raw
	expires := time.Now().Add(enrolCodeLife)
	err = s.st.CreateEnrolCode(uid, auth.HashToken(code), expires.UnixMilli(), maxEnrolCodes)
	if errors.Is(err, store.ErrEnrolCodeLimit) {
		writeErr(w, http.StatusTooManyRequests, "you have "+strconv.Itoa(maxEnrolCodes)+" unused enrolment codes; use one or wait for them to expire")
		return
	}
	if err != nil {
		writeErr(w, 500, "could not make an enrolment code")
		return
	}
	s.eventRequest(r, "enrol-code-create", "", map[string]any{"expiresAt": expires.Unix()})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"code": code, "expiresAt": expires.Unix()})
}
