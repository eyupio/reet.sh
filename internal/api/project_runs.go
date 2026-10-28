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
	// Start in several projects: a run of folder projects chosen in the rail,
	// read and retried under the folder projects' own routes (the routes
	// above go with the saved Projects).
	// A new start counts as a folder-project write; a resend of one already
	// made (a lost reply) does not, so it always gets its run back.
	mux.HandleFunc("POST /api/v2/folder-projects/runs", s.folderAccess(s.selectionRunStart))
	mux.HandleFunc("GET /api/v2/folder-projects/runs/{runId}", s.folderAccess(s.projectRun))
	mux.HandleFunc("POST /api/v2/folder-projects/runs/{runId}/members/{position}/retry", s.folderAccess(s.projectRunRetry))
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
// after a restart, and stops the orchestrator when ctx ends; its watcher also
// ends when the Server is closed. A member that was in flight when the
// controller stopped is sent again with its unchanged request id, which the
// broker and the machine answer with the session they already made.
func (s *Server) ResumeProjectRuns(ctx context.Context) {
	if err := s.s.runs.Resume(); err != nil {
		log.Printf("projects: resume runs: %v", err)
	}
	runs := s.s.runs
	done := make(chan struct{})
	s.s.resumeDone = done
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			runs.Close()
		case <-runs.Done(): // the Server was closed
		}
	}()
}

func terminalsOff(w http.ResponseWriter) {
	writeErrCode(w, http.StatusForbidden, "terminals_disabled", "", sentenceTerminalsOff, "")
}

// projectRunStart creates a run of the chosen members and returns it at once
// (201, every member pending); the members are started in the background.
// Body: {"requestId":"…","positions":[0,2]}; the request id makes it
// idempotent (a replay answers 200 with the same run). With If-Match the run
// starts only from that Project version (409 project_changed otherwise,
// nothing created); a replay still answers with its run. A member whose machine is not enrolled on this
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
	// If-Match names the version the confirmation sheet was made from, as for
	// an update: the run starts only from that version (absent: no check).
	expect, ok := ifMatch(r)
	if !ok {
		writeErrCode(w, http.StatusBadRequest, "invalid_version", "", "The If-Match header must be the Project's version number.", "")
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
	// Against a changed Project the positions are checked by the store: a
	// replay still answers with its run, anything else is project_changed.
	stale := expect != 0 && p.Version != expect
	known := map[int]bool{}
	for _, m := range p.Members {
		known[m.Position] = true
	}
	chosen := map[int]bool{}
	for _, pos := range *in.Positions {
		if (!known[pos] && !stale) || chosen[pos] {
			unknownPosition(w)
			return
		}
		chosen[pos] = true
	}
	run, created, err := s.st.StartRunAtFor(uid, p.ID, in.RequestID, expect, *in.Positions)
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

const (
	sentenceSelection     = "Choose 2 to 12 different projects."
	sentenceRunPrompt     = "The prompt must be valid text of at most 2,000 bytes, without control characters other than line breaks, and must not start with a dash."
	sentenceNoOption      = "That agent doesn't take that option."
	sentenceYoloNotYet    = "reet doesn't start YOLO sessions yet."
	sentenceSelectionBusy = "One of those projects is still starting. Wait for it to finish, then start again."
)

// selectionRunStart starts agents in several folder projects at once: one run
// with a member per project, started in the background by the orchestrator
// (201, every member pending). Body: {"requestId", "projectIds" (2 to 12),
// "tool", "model", "effort", "prompt", "worktree", "yolo"}. The request id
// makes it idempotent: the same request again answers 200 with its run and
// starts nothing new. Each field is checked here with its own sentence, which
// never repeats the prompt; YOLO is refused until reet starts YOLO sessions.
func (s *server) selectionRunStart(w http.ResponseWriter, r *http.Request) {
	if !s.terminalEnabled() {
		terminalsOff(w)
		return
	}
	var in struct {
		RequestID  string   `json:"requestId"`
		ProjectIDs []string `json:"projectIds"`
		Tool       string   `json:"tool"`
		Model      string   `json:"model"`
		Effort     string   `json:"effort"`
		Prompt     string   `json:"prompt"`
		Worktree   bool     `json:"worktree"`
		Yolo       bool     `json:"yolo"`
	}
	if !folderProjectBody(w, r, &in) {
		return
	}
	uid := s.uid(r)
	replay := false
	if terminalproto.ValidID(in.RequestID) {
		var err error
		if replay, err = s.st.SelectionRunForRequest(uid, in.RequestID); err != nil {
			storeRefusal(err).write(w)
			return
		}
	}
	if !replay && !s.chargeFolderWrite(w, uid) {
		return
	}
	refuse := func(status int, code, field, sentence string) { writeErrCode(w, status, code, field, sentence, "") }
	distinct := map[string]bool{}
	for _, id := range in.ProjectIDs {
		distinct[id] = true
	}
	switch {
	case !terminalproto.ValidID(in.RequestID):
		refuse(http.StatusUnprocessableEntity, "request_id_required", "requestId", "Starting needs a request id: 16 to 128 letters, digits, hyphens or underscores.")
	case len(in.ProjectIDs) < store.MinSelectionProjects || len(in.ProjectIDs) > store.MaxSelectionProjects || len(distinct) != len(in.ProjectIDs):
		refuse(http.StatusUnprocessableEntity, "invalid_selection", "projectIds", sentenceSelection)
	case !terminalproto.ValidToolV2(in.Tool):
		refuse(http.StatusUnprocessableEntity, "tool_unavailable", "tool", "reet doesn't start this agent yet.")
	case in.Model != "" && !terminalproto.ValidModel(in.Model):
		refuse(http.StatusUnprocessableEntity, "invalid_model", "model", sentenceModel)
	case in.Effort != "" && in.Effort != "low" && in.Effort != "medium" && in.Effort != "high":
		refuse(http.StatusUnprocessableEntity, "invalid_effort", "effort", "Choose low, medium or high thinking, or the agent's default.")
	case in.Effort != "" && !terminalproto.ToolTakesEffort(in.Tool), in.Model != "" && !terminalproto.ToolTakesModel(in.Tool):
		refuse(http.StatusConflict, "launch_option_unsupported", "", sentenceNoOption)
	case in.Prompt != "" && !terminalproto.ValidPrompt(in.Prompt):
		refuse(http.StatusUnprocessableEntity, "invalid_prompt", "prompt", sentenceRunPrompt)
	case terminalproto.PromptNeedsWords(in.Tool, in.Prompt):
		refuse(http.StatusUnprocessableEntity, "prompt_unsupported", "prompt", sentenceTwoWords)
	case in.Yolo:
		refuse(http.StatusConflict, "yolo_refused", "", sentenceYoloNotYet)
	default:
		run, created, err := s.st.StartSelectionRunFor(uid, in.RequestID, in.ProjectIDs, store.RunLaunch{Tool: in.Tool, Model: in.Model, Effort: in.Effort, Prompt: in.Prompt, Worktree: in.Worktree, Yolo: in.Yolo})
		switch {
		case errors.Is(err, store.ErrRunRequestChanged):
			refuse(http.StatusConflict, "request_changed", "", "That request changed. Start it again.")
			return
		case errors.Is(err, store.ErrRunInProgress):
			refuse(http.StatusConflict, "run_in_progress", "", sentenceSelectionBusy)
			return
		case errors.Is(err, store.ErrSelectionSize):
			refuse(http.StatusUnprocessableEntity, "invalid_selection", "projectIds", sentenceSelection)
			return
		case errors.Is(err, store.ErrFolderProjectNotFound):
			folderRefusal(err, "").write(w)
			return
		case err != nil:
			storeRefusal(err).write(w)
			return
		}
		// A replay answers with the run as it is now; the driver only picks
		// up members still pending.
		s.runs.Start(uid, run.ID)
		if !created {
			writeJSON(w, http.StatusOK, viewRun(run))
			return
		}
		s.event(uid, "ui", "project-run", "", map[string]any{"origin": store.RunOriginSelection, "run": run.ID, "members": len(run.Members), "projects": in.ProjectIDs})
		writeJSON(w, http.StatusCreated, viewRun(run))
	}
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
