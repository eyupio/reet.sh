package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A controller-owned outcome: the person approves a plan in their browser
// and the controller starts and follows its sessions. The routes need the
// same browser account as Gaffer settings, and the terminal feature, since
// approving a plan starts agents on a machine.
func (s *server) registerGafferOutcomeRoutes(mux *http.ServeMux) {
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{
			accounts: "Gaffer outcomes need a browser account.", signedOut: "Sign in to give Gaffer an outcome.", readOnly: "This account is read-only, so it cannot give Gaffer an outcome.",
		}, h)
	}
	mux.HandleFunc("GET /api/v2/gaffer/outcomes", access(s.gafferOutcomes))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes", access(s.gafferOutcomeCreate))
	mux.HandleFunc("GET /api/v2/gaffer/outcomes/{id}", access(s.gafferOutcome))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/stop", access(s.gafferOutcomeStop))
}

func gafferOutcomeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrGafferOutcomeLimit), errors.Is(err, store.ErrGafferOutcomeBusy):
		writeErr(w, 409, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "That outcome is not available.")
	default:
		gafferError(w, err)
	}
}

func (s *server) gafferOutcomes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.gafferProviders.Outcomes(s.uid(r))
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"outcomes": rows})
}

func (s *server) gafferOutcome(w http.ResponseWriter, r *http.Request) {
	o, err := s.gafferProviders.Outcome(s.uid(r), r.PathValue("id"))
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

func (s *server) gafferOutcomeCreate(w http.ResponseWriter, r *http.Request) {
	var in gaffer.OutcomeInput
	if !decodeStrictJSON(w, r, 96<<10, &in, "That outcome is invalid or too large.") {
		return
	}
	if !s.terminalEnabled() {
		writeErr(w, 403, "Remote sessions are disabled on this controller, so Gaffer cannot start any.")
		return
	}
	o, err := s.gafferProviders.CreateOutcome(s.uid(r), in)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-outcome-approved", "", map[string]any{"outcome": o.ID, "sessions": len(o.Sessions), "machine": o.AgentID, "projectId": o.ProjectID, "tool": o.Tool})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, o)
}

func (s *server) gafferOutcomeStop(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That outcome request is invalid.") {
		return
	}
	if in.Revision < 1 {
		writeErr(w, 400, "Reload this outcome before stopping it.")
		return
	}
	o, err := s.gafferProviders.StopOutcome(s.uid(r), r.PathValue("id"), in.Revision)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-outcome-stopped", "", map[string]any{"outcome": o.ID})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// outcomeStarter starts a session of an approved outcome through the launch
// path a browser uses: the project is checked, a git project gets a worktree
// of its own, the broker applies its capability and trust rules, and the
// launch is audited. It never asks for YOLO, and takes its machine, folder
// and agent from the outcome the person approved.
type outcomeStarter struct{ s *server }

// replyCapture keeps the launch handler's own refusal so an outcome reports
// the words a browser launch would have shown.
type replyCapture struct {
	header http.Header
	body   bytes.Buffer
}

func (c *replyCapture) Header() http.Header         { return c.header }
func (c *replyCapture) Write(b []byte) (int, error) { return c.body.Write(b) }
func (c *replyCapture) WriteHeader(int)             {}

func (o outcomeStarter) StartOutcomeSession(ctx context.Context, uid int64, in gaffer.OutcomeLaunch) (string, error) {
	s := o.s
	if !s.terminalEnabled() || s.terminals == nil {
		return "", errors.New("Remote sessions are disabled on this controller.")
	}
	create := terminalproto.Create{Title: in.Title, Cols: 100, Rows: 30, Tool: in.Tool, Dir: in.Dir, Prompt: in.Prompt, ProjectID: in.ProjectID, Structured: true}
	create.Title, create.TitleOrigin = terminalproto.ResolveTitle(in.Title, "", in.Tool, in.Prompt, in.Dir)
	refusal := func(err error) (string, error) {
		capture := &replyCapture{header: http.Header{}}
		s.terminalCreateError(capture, err, uid, in.AgentID, in.Tool)
		var reply struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(capture.body.Bytes(), &reply) != nil || reply.Error == "" {
			return "", errors.New("That machine could not start the session.")
		}
		return "", errors.New(reply.Error)
	}
	if !s.terminalCreateRecorded(uid, in.AgentID, in.RequestID) {
		if err := s.st.TerminalProjectCheckFor(uid, in.AgentID, in.ProjectID, in.Dir); err != nil {
			return refusal(err)
		}
	}
	s.isolateWorktree(uid, in.AgentID, in.RequestID, &create)
	session, started, err := s.terminals.CreateSession(ctx, uid, in.AgentID, in.RequestID, create)
	if err != nil {
		return refusal(err)
	}
	if started {
		s.auditTerminalLaunch(uid, in.AgentID, create, session)
	}
	// A create can be recorded and still not have started anything: the
	// machine refused the launch. Nothing is running, so it was not started.
	if session.State == terminalproto.Lost || session.State == terminalproto.Ended {
		return "", errors.New("That machine didn't start the agent. Check its terminal settings on that machine.")
	}
	return session.ID, nil
}
