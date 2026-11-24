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

// machineAuthKey marks a request a machine's own id and secret authenticated.
type machineAuthKey struct{}

func machineAuthenticated(r *http.Request) bool {
	v, _ := r.Context().Value(machineAuthKey{}).(bool)
	return v
}

// machineRoutes is every request an enrolled machine makes on the public
// address after enrolment, traced from its agent. Its id and secret alone
// (X-Reet-Agent and X-Reet-Agent-Key) open these and nothing else. The
// private listener's routes check the same secret themselves, and the
// agent-proof and moved routes need no credential.
var machineRoutes = []string{
	"POST /api/v2/sync/identity",            // status, update check, set-server
	"POST /api/v2/sync/poll",                // work and the desired skills
	"POST /api/v2/sync/result",              // a job's outcome
	"POST /api/v2/sync/agents/update/stage", // binary update stages
	"POST /api/v2/sync/private",             // private control connection address
	"POST /api/v2/sync/skills/report",       // what its skills folders hold
	"POST /api/v2/sync/content-observations/prepare",
	"POST /api/v2/sync/content-observations",
	"GET /api/v2/sync/skill-import", // reet agent skill
	"POST /api/v2/sync/skill-import",
	"POST /api/v2/sync/skill-import/work", // imports asked for in the browser
	"POST /api/v2/sync/skill-import/preview",
	"POST /api/v2/sync/skill-import/transfer",
	"POST /api/v2/sync/skill-import/failure",
	"GET /api/v2/skill-bundles/{bundle}/revisions/{revision}", // a private skill's files to install
	"POST /api/import",                                        // its inventory scan
}

var machineMux = func() *http.ServeMux {
	mux := http.NewServeMux()
	for _, pattern := range machineRoutes {
		mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	}
	return mux
}()

func machineRoute(r *http.Request) bool {
	_, pattern := machineMux.Handler(r)
	return pattern != ""
}

// machineUser is the account of the enrolled machine the request names, when
// its secret matches and neither it nor the account has been removed.
func (s *server) machineUser(r *http.Request) (store.User, bool) {
	uid, _, err := s.st.AuthenticateRemoteAgent(r.Header.Get("X-Reet-Agent"), r.Header.Get("X-Reet-Agent-Key"), r.Header.Get("X-Reet-Version"))
	if err != nil {
		return store.User{}, false
	}
	u, ok, err := s.st.UserByID(uid)
	return u, ok && err == nil && u.DisabledAt == 0
}
