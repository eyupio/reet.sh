package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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
	mux.HandleFunc("GET /api/v2/gaffer/outcomes/{id}/history", access(s.gafferOutcomeHistory))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/stop", access(s.gafferOutcomeStop))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/stop-following", access(s.gafferOutcomeStopFollowing))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/archive", access(s.gafferOutcomeArchive(true)))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/restore", access(s.gafferOutcomeArchive(false)))
	mux.HandleFunc("DELETE /api/v2/gaffer/outcomes/{id}", access(s.gafferOutcomeDelete))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/merge", access(s.gafferOutcomeMerge))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/retry", access(s.gafferOutcomeRetry))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/advance", access(s.gafferOutcomeAdvance))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/automerge", access(s.gafferOutcomeAutoMerge))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/back", access(s.gafferOutcomeBack))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/tell", access(s.gafferOutcomeTell))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions", access(s.gafferOutcomeAdd))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/review", access(s.gafferOutcomeReview))
}

func gafferOutcomeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrGafferOutcomeLimit), errors.Is(err, store.ErrGafferOutcomeBusy), errors.Is(err, store.ErrGafferOutcomeActive):
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

// gafferOutcomeHistory is the activity recorded for one outcome: what the
// person, Gaffer and the controller did to its pipelines, newest first. It
// reads the activity log only.
func (s *server) gafferOutcomeHistory(w http.ResponseWriter, r *http.Request) {
	o, err := s.gafferProviders.Outcome(s.uid(r), r.PathValue("id"))
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	events, err := s.st.OutcomeEventsFor(s.uid(r), o.ID, 200)
	if err != nil {
		writeErr(w, 500, "The history could not be read.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"events": events})
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
	if in.Yolo {
		// Refused here, before anything is saved, with the words a launch would
		// give. The broker checks again, with the machine, at every start.
		if setting, err := s.yoloSetting(); err != nil || !setting.Allowed {
			writeErrCode(w, http.StatusForbidden, "yolo_not_allowed", "", sentenceYoloNotAllowed, "")
			return
		}
	}
	o, err := s.gafferProviders.CreateOutcome(s.uid(r), in)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-outcome-approved", "", map[string]any{"outcome": o.ID, "sessions": len(o.Sessions), "machine": o.AgentID, "projectId": o.ProjectID, "tool": o.Tool, "yolo": o.Yolo})
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

// gafferOutcomeStopFollowing is the person ending an outcome that is stuck or
// no longer wanted: no pipeline is followed any more, so it can be archived or
// deleted. Sessions that still exist keep running.
func (s *server) gafferOutcomeStopFollowing(w http.ResponseWriter, r *http.Request) {
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
	o, err := s.gafferProviders.StopFollowingOutcome(s.uid(r), r.PathValue("id"), in.Revision)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-outcome-following-stopped", "", map[string]any{"outcome": o.ID})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeArchive puts a finished outcome away, or brings it back.
func (s *server) gafferOutcomeArchive(archive bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Revision int64 `json:"revision"`
		}
		if !decodeStrictJSON(w, r, 1024, &in, "That outcome request is invalid.") {
			return
		}
		if in.Revision < 1 {
			writeErr(w, 400, "Reload this outcome before changing it.")
			return
		}
		o, err := s.gafferProviders.ArchiveOutcome(s.uid(r), r.PathValue("id"), in.Revision, archive)
		if err != nil {
			gafferOutcomeError(w, err)
			return
		}
		name := "gaffer-outcome-archived"
		if !archive {
			name = "gaffer-outcome-restored"
		}
		s.eventRequest(r, name, "", map[string]any{"outcome": o.ID})
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, o)
	}
}

// gafferOutcomeDelete removes a finished outcome's record for good. Its
// sessions and pull requests are left alone.
func (s *server) gafferOutcomeDelete(w http.ResponseWriter, r *http.Request) {
	revision, _ := strconv.ParseInt(r.URL.Query().Get("revision"), 10, 64)
	if revision < 1 {
		writeErr(w, 400, "Reload this outcome before deleting it.")
		return
	}
	id := r.PathValue("id")
	if err := s.gafferProviders.DeleteOutcome(s.uid(r), id, revision); err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-outcome-deleted", "", map[string]any{"outcome": id})
	w.WriteHeader(http.StatusNoContent)
}

// gafferOutcomeMerge is the person approving the merge of a pipeline that is
// ready. Without it, or the auto-merge they chose for that pipeline, the merge
// stage never starts.
func (s *server) gafferOutcomeMerge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision  int64 `json:"revision"`
		Confirmed bool  `json:"confirmed"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That merge approval is invalid.") {
		return
	}
	if in.Revision < 1 || !in.Confirmed {
		writeErr(w, 400, "Reload this pipeline and confirm the merge.")
		return
	}
	o, err := s.gafferProviders.ApproveMerge(s.uid(r), r.PathValue("id"), r.PathValue("key"), in.Revision)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pipeline-merge-approved", "", map[string]any{"outcome": o.ID, "session": r.PathValue("key")})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeRetry tries a stopped pipeline again, at the person's request.
// gafferOutcomeAdvance is the person moving a pipeline on from the stage in
// hand. It is their decision, so it does not wait for Gaffer's review.
func (s *server) gafferOutcomeAdvance(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirmed bool `json:"confirmed"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That request is invalid.") {
		return
	}
	if !in.Confirmed {
		writeErr(w, 400, "Confirm moving this pipeline on before it leaves its stage.")
		return
	}
	if !s.terminalEnabled() {
		writeErr(w, 403, "Remote sessions are disabled on this controller, so Gaffer cannot start any.")
		return
	}
	o, err := s.gafferProviders.MovePipelineOn(s.uid(r), r.PathValue("id"), r.PathValue("key"))
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pipeline-moved-on", "", map[string]any{"outcome": o.ID, "pipeline": r.PathValue("key")})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeBack is the person sending a pipeline back to an earlier stage.
func (s *server) gafferOutcomeBack(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Stage     string `json:"stage"`
		Reason    string `json:"reason"`
		Confirmed bool   `json:"confirmed"`
	}
	if !decodeStrictJSON(w, r, 8<<10, &in, "That request is invalid.") {
		return
	}
	if !in.Confirmed {
		writeErr(w, 400, "Confirm sending this pipeline back before its stage is done again.")
		return
	}
	if !s.terminalEnabled() {
		writeErr(w, 403, "Remote sessions are disabled on this controller, so Gaffer cannot start any.")
		return
	}
	o, err := s.gafferProviders.SendPipelineBack(s.uid(r), r.PathValue("id"), r.PathValue("key"), in.Stage, in.Reason)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pipeline-sent-back", "", map[string]any{"outcome": o.ID, "pipeline": r.PathValue("key"), "stage": in.Stage})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeTell passes the person's instruction to the working agent of a
// pipeline's stage in hand. The text is theirs; nothing is added to it.
func (s *server) gafferOutcomeTell(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID string `json:"requestId"`
		Text      string `json:"text"`
	}
	if !decodeStrictJSON(w, r, 16<<10, &in, "That instruction is invalid or too large.") {
		return
	}
	m, err := s.gafferProviders.TellPipelineAgent(r.Context(), s.uid(r), r.PathValue("id"), r.PathValue("key"), in.RequestID, in.Text)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pipeline-agent-told", "", map[string]any{"outcome": r.PathValue("id"), "pipeline": r.PathValue("key"), "message": m.ID, "state": m.State})
	m.Text = ""
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"message": m})
}

// gafferOutcomeAutoMerge is the person changing whether one pipeline merges
// by itself at its merge stage.
func (s *server) gafferOutcomeAutoMerge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		On *bool `json:"on"`
		// YoloConfirmed is the person's separate yes for a YOLO pipeline.
		YoloConfirmed bool `json:"yoloConfirmed"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That request is invalid.") {
		return
	}
	if in.On == nil {
		writeErr(w, 400, "Say whether this pipeline merges by itself.")
		return
	}
	o, err := s.gafferProviders.SetPipelineAutoMerge(s.uid(r), r.PathValue("id"), r.PathValue("key"), *in.On, in.YoloConfirmed)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pipeline-merge-setting", "", map[string]any{"outcome": o.ID, "pipeline": r.PathValue("key"), "autoMerge": *in.On, "yoloConfirmed": *in.On && in.YoloConfirmed})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

func (s *server) gafferOutcomeRetry(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That retry is invalid.") {
		return
	}
	if in.Revision < 1 {
		writeErr(w, 400, "Reload this pipeline before trying it again.")
		return
	}
	if !s.terminalEnabled() {
		writeErr(w, 403, "Remote sessions are disabled on this controller, so Gaffer cannot start any.")
		return
	}
	o, err := s.gafferProviders.RetryPipeline(s.uid(r), r.PathValue("id"), r.PathValue("key"), in.Revision)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pipeline-retried", "", map[string]any{"outcome": o.ID, "session": r.PathValue("key")})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeAdd adds pipelines the person approved to an outcome.
func (s *server) gafferOutcomeAdd(w http.ResponseWriter, r *http.Request) {
	var in gaffer.AddPipelinesInput
	if !decodeStrictJSON(w, r, 96<<10, &in, "Those pipelines are invalid or too large.") {
		return
	}
	if in.Revision < 1 {
		writeErr(w, 400, "Reload this outcome before adding to it.")
		return
	}
	if !s.terminalEnabled() {
		writeErr(w, 403, "Remote sessions are disabled on this controller, so Gaffer cannot start any.")
		return
	}
	o, err := s.gafferProviders.AddPipelines(s.uid(r), r.PathValue("id"), in)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pipelines-added", "", map[string]any{"outcome": o.ID, "sessions": len(in.Sessions)})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeReview asks Gaffer's provider for one report across an
// outcome's pipelines. It starts nothing; any pipelines it proposes wait for
// the person.
func (s *server) gafferOutcomeReview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That report request is invalid.") {
		return
	}
	if in.Revision < 1 {
		writeErr(w, 400, "Reload this outcome before asking for a report.")
		return
	}
	o, err := s.gafferProviders.ReviewOutcome(r.Context(), s.gafferPrincipal(r), r.PathValue("id"), in.Revision)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-outcome-reviewed", "", map[string]any{"outcome": o.ID})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// outcomeStarter starts a session of an approved outcome through the launch
// path a browser uses: the project is checked, a git project gets a worktree
// of its own, the broker applies its capability and trust rules, and the
// launch is audited. YOLO is asked for only when the person chose it for the
// outcome, and the broker checks the account setting, agent and machine each
// time. Machine, folder and agent come from the outcome the person approved.
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

func (o outcomeStarter) StartOutcomeSession(ctx context.Context, uid int64, in gaffer.OutcomeLaunch) (string, string, error) {
	s := o.s
	if !s.terminalEnabled() || s.terminals == nil {
		return "", "", errors.New("Remote sessions are disabled on this controller.")
	}
	create := terminalproto.Create{Title: in.Title, Cols: 100, Rows: 30, Tool: in.Tool, Model: in.Model, Effort: in.Effort, Dir: in.Dir, Prompt: in.Prompt, ProjectID: in.ProjectID, Structured: true, Trust: in.Trust, Yolo: in.Yolo}
	create.Title, create.TitleOrigin = terminalproto.ResolveTitle(in.Title, "", in.Tool, in.Prompt, in.Dir)
	refusal := func(err error) (string, string, error) {
		capture := &replyCapture{header: http.Header{}}
		s.terminalCreateError(capture, err, uid, in.AgentID, in.Tool)
		var reply struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(capture.body.Bytes(), &reply) != nil || reply.Error == "" {
			return "", "", errors.New("That machine could not start the session.")
		}
		return "", "", errors.New(reply.Error)
	}
	// A later pipeline stage works in the folder its first stage was given and
	// names no project: it is not a new project launch.
	if in.ProjectID != "" && !s.terminalCreateRecorded(uid, in.AgentID, in.RequestID) {
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
	if in.LinkProjectID != "" {
		// Best effort: the session runs either way, it is only listed under its project when this holds.
		_ = s.st.LinkTerminalSessionProjectFor(uid, session.ID, in.LinkProjectID)
	}
	// A create can be recorded and still not have started anything: the
	// machine refused the launch. Nothing is running, so it was not started.
	if session.State == terminalproto.Lost || session.State == terminalproto.Ended {
		return "", "", errors.New("That machine didn't start the agent. Check its terminal settings on that machine.")
	}
	// Where the session works: its own worktree in a git project, else the folder asked for.
	workDir := session.WorktreePath
	if workDir == "" {
		workDir = in.Dir
	}
	return session.ID, workDir, nil
}
