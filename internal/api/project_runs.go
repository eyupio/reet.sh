package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/eyupio/reet.sh/internal/projectrun"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Starting a run and retrying one member. The orchestrator (internal/projectrun)
// does the starting in the background; these handlers create the run, move a
// failed member back to pending, and return at once.
func (s *server) registerProjectRunRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/projects/{id}/runs", s.projectAccess(s.projectRunStart))
	mux.HandleFunc("POST /api/v2/project-runs/{runId}/members/{position}/retry", s.projectAccess(s.projectRunRetry))
}

const sentenceTerminalsOff = "Remote terminals are switched off on this server, so agents can't be started."

// newProjectRuns wires the orchestrator to the broker, the terminal switch
// and the launch audit (which never records the prompt).
func (s *server) newProjectRuns() *projectrun.Orchestrator {
	return projectrun.New(projectrun.Config{
		Store:    s.st,
		Broker:   s.terminals,
		Enabled:  s.terminalEnabled,
		Launched: s.auditTerminalLaunch,
		Logf: func(format string, args ...any) {
			log.Printf("projects: "+format, args...)
		},
	})
}

// ResumeProjectRuns continues every run that still has pending members, as
// after a restart, and stops the orchestrator when ctx ends. A member that was
// in flight when the controller stopped is sent again with its unchanged
// request id, which the broker and the machine answer with the session they
// already made.
func (s *Server) ResumeProjectRuns(ctx context.Context) {
	if err := s.s.runs.Resume(); err != nil {
		log.Printf("projects: resume runs: %v", err)
	}
	go func() {
		<-ctx.Done()
		s.s.runs.Close()
	}()
}

func terminalsOff(w http.ResponseWriter) {
	writeErrCode(w, http.StatusForbidden, "terminals_disabled", "", sentenceTerminalsOff, "")
}

// projectRunStart creates a run of the chosen members and returns it at once
// (201, every member pending); the members are started in the background.
// Body: {"requestId":"…","positions":[0,2]}; the request id makes it
// idempotent (a replay answers 200 with the same run). A member whose machine is not enrolled on this
// account is never sent anything: the orchestrator marks it unknown_machine.
func (s *server) projectRunStart(w http.ResponseWriter, r *http.Request) {
	if !s.terminalEnabled() {
		terminalsOff(w)
		return
	}
	var in struct {
		RequestID string `json:"requestId"`
		Positions *[]int `json:"positions"`
	}
	if !projectBody(w, r, &in) {
		return
	}
	uid := s.uid(r)
	p, err := s.st.ProjectFor(uid, r.PathValue("id"))
	if err != nil {
		storeRefusal(err).write(w)
		return
	}
	if !terminalproto.ValidID(in.RequestID) {
		writeErrCode(w, http.StatusUnprocessableEntity, "request_id_required", "requestId", "Starting a run needs a request id: 16 to 128 letters, digits, hyphens or underscores.", "")
		return
	}
	if in.Positions == nil || len(*in.Positions) == 0 {
		storeRefusal(store.ErrRunNoMembers).write(w)
		return
	}
	known := map[int]bool{}
	for _, m := range p.Members {
		known[m.Position] = true
	}
	chosen := map[int]bool{}
	for _, pos := range *in.Positions {
		if !known[pos] || chosen[pos] {
			unknownPosition(w)
			return
		}
		chosen[pos] = true
	}
	run, created, err := s.st.StartRunFor(uid, p.ID, in.RequestID, *in.Positions)
	switch {
	case errors.Is(err, store.ErrRunRequestChanged):
		writeErrCode(w, http.StatusConflict, "request_changed", "", "That request changed. Start it again.", "")
		return
	case errors.Is(err, store.ErrRunInProgress):
		writeErrCode(w, http.StatusConflict, "run_in_progress", "", "This Project is still starting. Wait for it to finish, then start again.", "")
		return
	case err != nil:
		storeRefusal(err).write(w)
		return
	}
	// A replay of the same request answers with the run as it is now and
	// starts nothing new (the driver only picks up pending members).
	s.runs.Start(uid, run.ID)
	if !created {
		writeJSON(w, http.StatusOK, viewRun(run))
		return
	}
	s.event(uid, "ui", "project-run", "", map[string]any{"project": p.ID, "run": run.ID, "members": len(run.Members)})
	writeJSON(w, http.StatusCreated, viewRun(run))
}

func unknownPosition(w http.ResponseWriter) {
	writeErrCode(w, http.StatusUnprocessableEntity, "unknown_position", "positions", "Choose repositories that are in this Project, each once.", "")
}

// projectRunRetry tries one failed member again: with the same request id when
// the earlier outcome is unknown, with the next attempt after a definite
// refusal. Body: {}.
func (s *server) projectRunRetry(w http.ResponseWriter, r *http.Request) {
	if !s.terminalEnabled() {
		terminalsOff(w)
		return
	}
	var in struct{}
	if !projectBody(w, r, &in) {
		return
	}
	uid := s.uid(r)
	runID, raw := r.PathValue("runId"), r.PathValue("position")
	position, err := strconv.Atoi(raw)
	if err != nil || position < 0 || strconv.Itoa(position) != raw {
		unknownPosition(w)
		return
	}
	m, err := s.runs.Retry(uid, runID, position)
	switch {
	case errors.Is(err, projectrun.ErrNoSuchMember):
		unknownPosition(w)
		return
	case errors.Is(err, projectrun.ErrNotFailed):
		writeErrCode(w, http.StatusConflict, "not_failed", "", "Only a repository that failed can be tried again.", "")
		return
	case err != nil:
		storeRefusal(err).write(w)
		return
	}
	run, err := s.st.RunFor(uid, runID)
	if err != nil {
		storeRefusal(err).write(w)
		return
	}
	s.event(uid, "ui", "project-run-retry", "", map[string]any{"project": run.ProjectID, "run": run.ID, "position": position, "attempt": m.Attempt})
	writeJSON(w, http.StatusOK, viewRun(run))
}
