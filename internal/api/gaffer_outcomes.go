package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
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
	mux.HandleFunc("POST /api/v2/gaffer/work-orders/check", access(s.gafferWorkOrderCheck))
	mux.HandleFunc("GET /api/v2/gaffer/outcomes/{id}/work-report", access(s.gafferWorkReport))
	mux.HandleFunc("GET /api/v2/gaffer/outcomes/{id}/history", access(s.gafferOutcomeHistory))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/stop", access(s.gafferOutcomeStop))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/stop-following", access(s.gafferOutcomeStopFollowing))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/archive", access(s.gafferOutcomeArchive(true)))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/restore", access(s.gafferOutcomeArchive(false)))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/automation", access(s.gafferOutcomeAutomation))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/yolo", access(s.gafferOutcomeYolo))
	mux.HandleFunc("DELETE /api/v2/gaffer/outcomes/{id}", access(s.gafferOutcomeDelete))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/merge", access(s.gafferOutcomeMerge))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/retry", access(s.gafferOutcomeRetry))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/advance", access(s.gafferOutcomeAdvance))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/automerge", access(s.gafferOutcomeAutoMerge))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/back", access(s.gafferOutcomeBack))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/tell", access(s.gafferOutcomeTell))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions", access(s.gafferOutcomeAdd))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/review", access(s.gafferOutcomeReview))
	mux.HandleFunc("GET /api/v2/gaffer/outcomes/{id}/sessions/{key}/delivery", access(s.gafferDelivery))
	mux.HandleFunc("PUT /api/v2/gaffer/outcomes/{id}/sessions/{key}/delivery/criteria", access(s.gafferDeliveryCriteria))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/delivery/evidence", access(s.gafferDeliveryEvidence))
	mux.HandleFunc("POST /api/v2/gaffer/outcomes/{id}/sessions/{key}/delivery/decision", access(s.gafferDeliveryDecision))
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
	events, err := s.st.OutcomeEventsFor(s.uid(r), o.ID, o.CreatedAt-60_000, 200)
	if err != nil {
		writeErr(w, 500, "The history could not be read.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"events": events})
}

func (s *server) gafferOutcomeCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		gaffer.OutcomeInput
		Pathway string `json:"pathway"`
	}
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
	// A request asked again is answered with the outcome it already made, as
	// CreateOutcome would, before the pathway is read: it may be off since,
	// and the receipts written then may have been changed by the person.
	if in.RequestID != "" {
		if existing, err := s.gafferProviders.Outcome(s.uid(r), in.RequestID); err == nil {
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, 201, existing)
			return
		}
	}
	if err := s.usePathway(s.uid(r), in.Pathway, in.Sessions); err != nil {
		gafferDeliveryError(w, err)
		return
	}
	o, err := s.gafferProviders.CreateOutcome(s.uid(r), in.OutcomeInput)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.recordPathwaySkills(s.uid(r), in.Pathway, o, len(in.Sessions))
	s.eventRequest(r, "gaffer-outcome-approved", "", map[string]any{"outcome": o.ID, "sessions": len(o.Sessions), "machine": o.AgentID, "projectId": o.ProjectID, "tool": o.Tool, "yolo": o.Yolo})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, o)
}

// outcomeRevision reads the {"revision"} body that stop, stop following, archive
// and report share. It has already answered when it returns false.
func outcomeRevision(w http.ResponseWriter, r *http.Request, invalid, reload string) (int64, bool) {
	var in struct {
		Revision int64 `json:"revision"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, invalid) {
		return 0, false
	}
	if in.Revision < 1 {
		writeErr(w, 400, reload)
		return 0, false
	}
	return in.Revision, true
}

func (s *server) gafferOutcomeStop(w http.ResponseWriter, r *http.Request) {
	revision, ok := outcomeRevision(w, r, "That outcome request is invalid.", "Reload this outcome before stopping it.")
	if !ok {
		return
	}
	o, err := s.gafferProviders.StopOutcome(s.uid(r), r.PathValue("id"), revision)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	if o.State != "stopped" {
		// Nothing was being started, so nothing was stopped: say so rather than record a stop.
		writeErr(w, 409, "This outcome has no sessions left to start. Use Stop following to end the pipelines in hand.")
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
	revision, ok := outcomeRevision(w, r, "That outcome request is invalid.", "Reload this outcome before stopping it.")
	if !ok {
		return
	}
	o, err := s.gafferProviders.StopFollowingOutcome(s.uid(r), r.PathValue("id"), revision)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-outcome-following-stopped", "", map[string]any{"outcome": o.ID})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeAutomation is the person giving Gaffer an outcome's decisions,
// or taking them back. It never covers a merge or an agent's approval.
func (s *server) gafferOutcomeAutomation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
		On       *bool `json:"on"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That outcome request is invalid.") {
		return
	}
	if in.Revision < 1 || in.On == nil {
		writeErr(w, 400, "Reload this outcome before changing it.")
		return
	}
	o, err := s.gafferProviders.SetOutcomeAutomation(s.uid(r), r.PathValue("id"), in.Revision, *in.On)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-outcome-decisions", "", map[string]any{"outcome": o.ID, "gaffer": o.Automation})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeYolo puts a started outcome on YOLO or takes it off. Turning
// it on is the person's own confirmed choice and needs the account's Allow
// YOLO setting, exactly as approving an outcome on YOLO does. The answer says
// which pipelines' sessions in hand could not be switched and still ask.
func (s *server) gafferOutcomeYolo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision  int64 `json:"revision"`
		On        *bool `json:"on"`
		Confirmed bool  `json:"confirmed"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That outcome request is invalid.") {
		return
	}
	if in.Revision < 1 || in.On == nil {
		writeErr(w, 400, "Reload this outcome before changing it.")
		return
	}
	if *in.On {
		if !in.Confirmed {
			writeErr(w, 400, "Confirm that this outcome's agents may act without asking you.")
			return
		}
		if setting, err := s.yoloSetting(); err != nil || !setting.Allowed {
			writeErrCode(w, http.StatusForbidden, "yolo_not_allowed", "", sentenceYoloNotAllowed, "")
			return
		}
	}
	o, left, err := s.gafferProviders.SetOutcomeYolo(r.Context(), s.uid(r), r.PathValue("id"), in.Revision, *in.On)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	if left == nil {
		left = []string{}
	}
	s.eventRequest(r, "gaffer-outcome-yolo", "", map[string]any{"outcome": o.ID, "yolo": o.Yolo, "sessionsNotSwitched": len(left)})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"outcome": o, "notSwitched": left})
}

// gafferOutcomeArchive puts a finished outcome away, or brings it back.
func (s *server) gafferOutcomeArchive(archive bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		revision, ok := outcomeRevision(w, r, "That outcome request is invalid.", "Reload this outcome before changing it.")
		if !ok {
			return
		}
		o, err := s.gafferProviders.ArchiveOutcome(s.uid(r), r.PathValue("id"), revision, archive)
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
	// An approval starts the merge stage, as the other controls that start
	// work do: it is refused where remote sessions are off.
	if !s.terminalEnabled() {
		writeErr(w, 409, "Remote sessions are disabled on this controller, so the merge stage cannot be started.")
		return
	}
	o, err := s.gafferProviders.ApproveMerge(s.uid(r), r.PathValue("id"), r.PathValue("key"), in.Revision)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pipeline-merge-approved", "", map[string]any{"outcome": o.ID, "pipeline": r.PathValue("key"), "session": r.PathValue("key")})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeAdvance is the person moving a pipeline on from the stage in
// hand. It is their decision, so it does not wait for Gaffer's review.
func (s *server) gafferOutcomeAdvance(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirmed bool `json:"confirmed"`
		// Stage is the stage the person was shown as in hand.
		Stage string `json:"stage"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That request is invalid.") {
		return
	}
	if !in.Confirmed {
		writeErr(w, 400, "Confirm moving this pipeline on before it leaves its stage.")
		return
	}
	// Without the stage the person was shown, a pipeline that has moved on since would be moved again.
	if in.Stage == "" {
		writeErr(w, 400, "Reload this pipeline before moving it on.")
		return
	}
	if !s.terminalEnabled() {
		writeErr(w, 403, "Remote sessions are disabled on this controller, so Gaffer cannot start any.")
		return
	}
	o, err := s.gafferProviders.MovePipelineOn(s.uid(r), r.PathValue("id"), r.PathValue("key"), in.Stage)
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

// gafferOutcomeRetry tries a stopped pipeline again, at the person's request.
func (s *server) gafferOutcomeRetry(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision       int64                     `json:"revision"`
		StoppedSession string                    `json:"stoppedSession,omitempty"`
		Worker         *gaffer.WorkerReplacement `json:"worker,omitempty"`
		// EndSession is the session in hand, still open, that the person is
		// leaving to run its stage again in a new one.
		EndSession string `json:"endSession,omitempty"`
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
	var o store.GafferOutcome
	var err error
	if in.StoppedSession != "" {
		o, err = s.gafferProviders.RecoverPipelineWorker(s.uid(r), r.PathValue("id"), r.PathValue("key"), in.StoppedSession, in.Revision, in.Worker)
	} else if in.Worker != nil {
		writeErr(w, 400, "Name the stopped session before choosing a replacement model.")
		return
	} else if in.EndSession != "" {
		o, err = s.gafferProviders.RestartPipelineStage(s.uid(r), r.PathValue("id"), r.PathValue("key"), in.EndSession, in.Revision)
	} else {
		o, err = s.gafferProviders.RetryPipeline(s.uid(r), r.PathValue("id"), r.PathValue("key"), in.Revision)
	}
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pipeline-retried", "", map[string]any{"outcome": o.ID, "pipeline": r.PathValue("key"), "session": r.PathValue("key")})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeAdd adds pipelines the person approved to an outcome.
func (s *server) gafferOutcomeAdd(w http.ResponseWriter, r *http.Request) {
	var in struct {
		gaffer.AddPipelinesInput
		Pathway string `json:"pathway"`
	}
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
	if err := s.usePathway(s.uid(r), in.Pathway, in.Sessions); err != nil {
		gafferDeliveryError(w, err)
		return
	}
	o, err := s.gafferProviders.AddPipelines(s.uid(r), r.PathValue("id"), in.AddPipelinesInput)
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	s.recordPathwaySkills(s.uid(r), in.Pathway, o, len(in.Sessions))
	s.eventRequest(r, "gaffer-pipelines-added", "", map[string]any{"outcome": o.ID, "sessions": len(in.Sessions)})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, o)
}

// gafferOutcomeReview asks Gaffer's provider for one report across an
// outcome's pipelines. It starts nothing; any pipelines it proposes wait for
// the person.
func (s *server) gafferOutcomeReview(w http.ResponseWriter, r *http.Request) {
	revision, ok := outcomeRevision(w, r, "That report request is invalid.", "Reload this outcome before asking for a report.")
	if !ok {
		return
	}
	o, err := s.gafferProviders.ReviewOutcome(r.Context(), s.gafferPrincipal(r), r.PathValue("id"), revision)
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

// SetOutcomeSessionYolo switches a pipeline's live session between asking and
// YOLO, through the same gates as the person's own switch in the chat.
func (o outcomeStarter) SetOutcomeSessionYolo(ctx context.Context, uid int64, sessionID string, on bool) error {
	if o.s.terminals == nil {
		return errors.New("Remote sessions are disabled on this controller.")
	}
	return o.s.terminals.SetChatYolo(ctx, uid, sessionID, on)
}

// EndOutcomeSession ends a session a pipeline has finished with, the way the
// person's own End does with its history kept: the worktree is left alone.
func (o outcomeStarter) EndOutcomeSession(ctx context.Context, uid int64, sessionID string) error {
	s := o.s
	if s.terminals == nil {
		return errors.New("Remote sessions are disabled on this controller.")
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	_, err := s.terminals.End(ctx, uid, sessionID, hex.EncodeToString(raw))
	return err
}

// OutcomeSessionQuestion returns the typed question a session's agent is
// waiting on, if it is waiting on one.
func (o outcomeStarter) OutcomeSessionQuestion(_ context.Context, uid int64, sessionID string) (gaffer.AgentQuestion, bool) {
	s := o.s
	if !s.terminalEnabled() || s.terminals == nil {
		return gaffer.AgentQuestion{}, false
	}
	id, question, alone, ok := s.terminals.PendingQuestionChat(uid, sessionID)
	return gaffer.AgentQuestion{ID: id, Question: question, Alone: alone}, ok
}

// AnswerOutcomeSessionQuestion answers that question by the path the person's
// own answer takes, so the machine validates it against the question it holds.
func (o outcomeStarter) AnswerOutcomeSessionQuestion(ctx context.Context, uid int64, sessionID, questionID string, answers json.RawMessage) error {
	s := o.s
	if !s.terminalEnabled() || s.terminals == nil {
		return errors.New("Remote sessions are disabled on this controller.")
	}
	return s.terminals.AnswerQuestionChat(ctx, uid, sessionID, questionID, "accept", answers)
}

// OutcomeSessionChoice asks a session's machine what the session is running:
// the model and effort its agent reports now, not what its launch asked for.
// The answer is also kept on the session, as any read of its controls is.
func (o outcomeStarter) OutcomeSessionChoice(ctx context.Context, uid int64, sessionID string) (gaffer.SessionChoice, error) {
	s := o.s
	if !s.terminalEnabled() || s.terminals == nil {
		return gaffer.SessionChoice{}, errors.New("Remote sessions are disabled on this controller.")
	}
	row, err := s.st.TerminalSessionFor(uid, sessionID)
	if err != nil {
		return gaffer.SessionChoice{}, errors.New("The session is no longer listed.")
	}
	answer, err := s.terminals.ChatControl(ctx, uid, sessionID, terminalproto.ACPControl{Action: "get"})
	if err != nil {
		return gaffer.SessionChoice{}, err
	}
	if answer.Controls.Configuring {
		return gaffer.SessionChoice{}, errors.New("The agent was changing its settings when it was asked.")
	}
	choice := gaffer.SessionChoice{Tool: row.Tool, Model: answer.Controls.Model}
	for _, option := range answer.Controls.Options {
		if option.Category == terminalproto.AgentOptionThoughtLevel {
			choice.Effort = option.Current
		}
	}
	return choice, nil
}

// OutcomeWorkState reads the Git state of the folder a pipeline works in from
// its machine: the commit it is at and, when github is set, its pull request
// and checks. The folder is the controller's own record, on a machine the
// account owns, and the read is the bounded, read-only one a session page uses.
func (o outcomeStarter) OutcomeWorkState(ctx context.Context, uid int64, agentID, dir string, github bool) (terminalproto.InspectResult, error) {
	s := o.s
	if !s.terminalEnabled() || s.terminals == nil {
		return terminalproto.InspectResult{}, errors.New("Remote sessions are disabled on this controller.")
	}
	view := "status"
	if github {
		view = "github"
	}
	return s.terminals.InspectFolderView(ctx, uid, agentID, dir, view)
}

func (o outcomeStarter) StartOutcomeSession(ctx context.Context, uid int64, in gaffer.OutcomeLaunch) (string, string, error) {
	s := o.s
	if !s.terminalEnabled() || s.terminals == nil {
		return "", "", errors.New("Remote sessions are disabled on this controller.")
	}
	create := terminalproto.Create{Title: in.Title, Cols: 100, Rows: 30, Tool: in.Tool, Model: in.Model, Effort: in.Effort, Provider: in.Provider, Dir: in.Dir, Prompt: gaffer.OutcomePromptImages(in.Prompt, in.Images), ProjectID: in.ProjectID, Structured: true, Trust: in.Trust, Yolo: in.Yolo, PullLatest: in.PullLatest}
	create.Title, create.TitleOrigin = terminalproto.ResolveTitle(in.Title, "", in.Tool, in.Prompt, in.Dir)
	refusal := func(err error) (string, string, error) {
		capture := &replyCapture{header: http.Header{}}
		s.terminalCreateError(capture, err, uid, in.AgentID, in.Tool)
		var reply struct {
			Error string `json:"error"`
		}
		message := "That machine could not start the session."
		if json.Unmarshal(capture.body.Bytes(), &reply) == nil && reply.Error != "" {
			message = reply.Error
		}
		// A locked machine refused before anything was sent: nothing started,
		// and the sentence says what to do.
		if errors.Is(err, terminalbroker.ErrLocked) {
			return "", "", errors.New(message)
		}
		// A machine that could not be reached, or did not answer, has said
		// nothing about a session an earlier request may have started.
		if errors.Is(err, terminalbroker.ErrOffline) || errors.Is(err, terminalbroker.ErrNotSent) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", "", gaffer.LaunchUnknown(message)
		}
		// The agent did not confirm the chosen effort for the model: nothing
		// runs and no prompt was sent, so the Factory may ask again without it.
		// A launch that carries no effort gets this answer when its request is
		// one already refused for an effort, so it is classed the same way.
		var unavailable *terminalbroker.ACPUnavailableError
		if errors.As(err, &unavailable) && unavailable.EffortNotConfirmed() {
			return "", "", gaffer.EffortNotOffered(message)
		}
		return "", "", errors.New(message)
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
		if in.SkillScope != "" {
			if err := s.st.SkillBuddyPreparedFor(uid, in.SkillScope, session.ID); err != nil {
				s.event(uid, "skills", "skill-receipt-failed", "", map[string]any{"scope": in.SkillScope})
			}
		}
	}
	if in.LinkProjectID != "" {
		// Best effort: the session runs either way, it is only listed under its project when this holds.
		if err := s.st.LinkTerminalSessionProjectFor(uid, session.ID, in.LinkProjectID); err != nil {
			log.Printf("gaffer: session %q was not listed under project %q: %v", session.ID, in.LinkProjectID, err)
		}
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
