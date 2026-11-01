package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
)

// installErrorStatus is the HTTP status for a refusal code of the store.
func installErrorStatus(code string) int {
	switch code {
	case "unpublished", "no-scan", "unsupported-agent", "connector-offline":
		return http.StatusUnprocessableEntity
	case "follows-source", "slug-conflict", "install-in-progress", "already-installed", "cannot-cancel":
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

type installErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeInstallError(w http.ResponseWriter, err error) {
	var ie *store.InstallError
	if errors.As(err, &ie) {
		writeJSON(w, installErrorStatus(ie.Code), installErrorBody{Error: ie.Message, Code: ie.Code})
		return
	}
	// Anything else may carry a database or origin detail the person should not see.
	writeJSON(w, http.StatusInternalServerError, installErrorBody{Error: "The install could not be started. Try again.", Code: "internal"})
}

// terminalSkillInstall puts one published hosted skill on the session's
// machine. The store queues a preview job only; the person approves the plan.
func (s *server) terminalSkillInstall(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Skill string `json:"skill"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	slug, ok := strings.CutPrefix(in.Skill, "reet:")
	if !ok || slug == "" || strings.ContainsAny(slug, `/\`) || strings.Contains(slug, "..") {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	uid := s.uid(r)
	session, agentID, host, err := s.sessionMachine(uid, r.PathValue("id"))
	if err != nil {
		terminalError(w, err)
		return
	}
	if host == "" {
		writeInstallError(w, &store.InstallError{Code: "connector-offline", Message: "This session's machine isn't connected to reet right now, so it can't install this skill."})
		return
	}
	res, err := s.st.InstallHostedSkillFor(uid, store.InstallRequest{Tool: session.Tool, Host: host, AgentID: agentID, Slug: slug, Origin: requestOrigin(r)})
	if err != nil {
		writeInstallError(w, err)
		return
	}
	s.eventRequest(r, "skill-install", "", map[string]any{"session": session.ID, "machine": res.Host, "slug": slug, "job": res.JobID, "reused": res.Reused})
	writeJSON(w, 200, map[string]any{"job": res.JobID, "reused": res.Reused, "directory": res.Directory, "host": res.Host})
}

// terminalSkillInstallCancel cancels a queued install and takes the skill back
// out of the saved set.
func (s *server) terminalSkillInstallCancel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Job string `json:"job"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	if in.Job == "" {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	uid := s.uid(r)
	session, _, host, err := s.sessionMachine(uid, r.PathValue("id"))
	if err != nil {
		terminalError(w, err)
		return
	}
	reverted, err := s.st.CancelInstallFor(uid, in.Job)
	if errors.Is(err, store.ErrNotFound) {
		terminalError(w, terminalbroker.ErrDenied)
		return
	}
	if err != nil {
		var ie *store.InstallError
		if errors.As(err, &ie) {
			writeInstallError(w, err)
		} else {
			// A database detail is not for the person, and the machine is not reconnecting.
			writeJSON(w, http.StatusInternalServerError, installErrorBody{Error: "The install could not be cancelled. Try again.", Code: "internal"})
		}
		return
	}
	s.eventRequest(r, "skill-install-cancel", "", map[string]any{"session": session.ID, "machine": host, "job": in.Job, "reverted": reverted})
	writeJSON(w, 200, map[string]bool{"reverted": reverted})
}
