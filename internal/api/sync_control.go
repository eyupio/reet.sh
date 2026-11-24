package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/eyupio/reet.sh/internal/agentupdate"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/syncproof"
)

func (s *server) registerSyncControl(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/sync/skill-import", s.privateSkillAccess(s.machineSkillImport))
	mux.HandleFunc("POST /api/v2/sync/skill-import", s.privateSkillAccess(s.machineSkillImport))
	mux.HandleFunc("POST /api/v2/sync/content-observations/prepare", s.requireWriter(s.contentObservation))
	mux.HandleFunc("POST /api/v2/sync/content-observations", s.requireWriter(s.contentObservation))
	mux.HandleFunc("POST /api/v2/sync/agents/update", s.requireWriter(s.updateSyncAgent))
	mux.HandleFunc("POST /api/v2/sync/agents/update/stage", s.requireWriter(s.reportUpdateStage))
	mux.HandleFunc("POST "+enrolPath, s.requireWriter(s.enrollSyncAgent))
	mux.HandleFunc("POST /api/v2/sync/enrol-codes", s.browserOnly("create enrolment codes", "enrolment codes require accounts mode", s.requireWriter(s.createEnrolCode)))
	mux.HandleFunc("PATCH /api/v2/sync/agents", s.requireWriter(s.controlSyncAgent))
	mux.HandleFunc("POST /api/v2/sync/jobs", s.requireWriter(s.queueSyncJob))
	mux.HandleFunc("PATCH /api/v2/sync/jobs", s.requireWriter(s.controlSyncJob))
	mux.HandleFunc("POST /api/v2/sync/poll", s.requireWriter(s.pollSyncAgent))
	mux.HandleFunc("POST /api/v2/sync/result", s.requireWriter(s.finishSyncJob))
	mux.HandleFunc("POST /api/v2/sync/identity", s.requireWriter(s.syncIdentity))
	mux.HandleFunc("POST "+syncproof.Path, s.syncAgentProof)
	mux.HandleFunc("POST /api/v2/sync/private", s.requireWriter(s.privateBootstrap))
	s.registerRemoteImports(mux)
	s.registerSkillReconcile(mux)
	// The plan, preview and approve sync is gone. A page or an older agent
	// that still asks for it is told so plainly, rather than with a bare 404.
	for _, route := range []string{
		"GET /api/v2/sync/candidates", "POST /api/v2/sync/library", "POST /api/v2/sync/library/dedupe",
		"POST /api/v2/sync/source-conflicts/resolve", "PUT /api/v2/sync/desired", "POST /api/v2/sync/reconcile",
		"POST /api/v2/sync/adopt", "POST /api/v2/sync/run", "POST /api/v2/sync/jobs/stage", "POST /api/v2/sync/from-machine",
		"PUT /api/v2/sync", "DELETE /api/v2/sync",
		"PUT /api/v2/library/selections", "GET /api/v2/library/selections/{target}", "POST /api/v2/library/{item}/difference",
		"POST /api/v2/library/deployment-intent", "POST /api/v2/library/install-evidence", "POST /api/v2/library/deletion-observed",
		"POST /api/v2/library/restore-receipt", "POST /api/v2/library/bases", "POST /api/v2/library/protocol",
		"POST /api/v2/library/edits", "POST /api/v2/library/locations", "POST /api/v2/library/locations/content",
		"/api/v2/skill-content-targets", "/api/v2/skill-content-targets/{id}",
		"/api/v2/skill-content-previews", "/api/v2/skill-content-previews/{id}", "/api/v2/skill-content-previews/{id}/{action...}",
		"/api/v2/skill-content-receipts", "/api/v2/skill-content-receipts/{id}", "/api/v2/sync/skill-content-preview/{action}",
		"GET /api/skill/ignore", "DELETE /api/skill/ignore", "GET /api/ignored",
	} {
		mux.HandleFunc(route, syncEngineRemoved)
	}
}

// syncEngineRemoved answers a route of the removed job-based sync.
func syncEngineRemoved(w http.ResponseWriter, _ *http.Request) {
	writeErr(w, http.StatusGone, "This was part of the old sync, which is gone: every machine follows your library. Make a skill Active or Archived on the Skills page, and see each machine on the Sync page.")
}
func (s *server) updateSyncAgent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	if _, err := s.st.QueueAgentUpdate(s.uid(r), in.ID, in.Version); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	s.eventRequest(r, "sync-agent-update", "", map[string]string{"agent": in.ID, "version": in.Version})
	s.syncState(w, r)
}
func (s *server) enrollSyncAgent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Host    string `json:"host"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if !decode(w, r, &in) {
		return
	}
	uid, via := s.uid(r), "session"
	var a store.SyncAgent
	var secret string
	var err error
	if code, ok := r.Context().Value(enrolCodeKey{}).(string); ok {
		via = "code"
		uid, a, secret, err = s.st.EnrolWithCode(auth.HashToken(code), in.Host, in.Version, in.Commit)
		if errors.Is(err, store.ErrEnrolCodeInvalid) {
			writeErr(w, 401, badEnrolCode)
			return
		}
	} else {
		if usedAPIToken(r) {
			via = "token"
		}
		a, secret, err = s.st.EnrollSyncAgentBuild(uid, in.Host, in.Version, in.Commit)
		if err == nil && via == "token" {
			a.LegacyToken = true
			err = s.st.SetAgentLegacyTokenFor(uid, a.ID, true)
		}
	}
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	if err = s.st.ControlSyncAgent(uid, a.ID, "keep-sync-on"); err != nil {
		_ = s.st.ControlSyncAgent(uid, a.ID, "revoke")
		writeErr(w, 500, err.Error())
		return
	}
	a.KeepSynced = true
	s.event(uid, "ui", "machine-enrol", "", map[string]string{"agent": a.ID, "host": a.Host, "via": via})
	writeJSON(w, 201, map[string]any{"agent": a, "secret": secret})
}
func (s *server) controlSyncAgent(w http.ResponseWriter, r *http.Request) {
	var in struct{ ID, Action string }
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.ControlSyncAgent(s.uid(r), in.ID, in.Action); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Action == "revoke" {
		s.terminals.Revoke(s.uid(r), in.ID)
	}
	s.syncState(w, r)
}

// queueSyncJob asks a machine for a scan, the one job a person queues here.
// Binary updates are queued by updateSyncAgent.
func (s *server) queueSyncJob(w http.ResponseWriter, r *http.Request) {
	var in struct{ AgentID, TargetID, Kind string }
	if !decode(w, r, &in) {
		return
	}
	if _, err := s.st.QueueSyncJob(s.uid(r), in.AgentID, in.Kind); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	s.syncState(w, r)
}

// controlSyncJob cancels a job that has not started.
func (s *server) controlSyncJob(w http.ResponseWriter, r *http.Request) {
	var in struct{ ID, Action string }
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.ControlSyncJob(s.uid(r), in.ID, in.Action); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	s.syncState(w, r)
}
func (s *server) agentIdentity(w http.ResponseWriter, r *http.Request) (store.SyncAgent, bool) {
	a, err := s.st.AuthenticateSyncAgentBuild(s.uid(r), r.Header.Get("X-Reet-Agent"), r.Header.Get("X-Reet-Agent-Key"), r.Header.Get("X-Reet-Version"), r.Header.Get("X-Reet-Commit"))
	if err != nil {
		writeErr(w, 403, err.Error())
		return a, false
	}
	if legacy := usedAPIToken(r); legacy != a.LegacyToken {
		if err = s.st.SetAgentLegacyTokenFor(s.uid(r), a.ID, legacy); err != nil {
			writeErr(w, 500, err.Error())
			return a, false
		}
		a.LegacyToken = legacy
	}
	return a, true
}
func (s *server) syncIdentity(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if ok {
		writeJSON(w, 200, a)
	}
}

// autoUpdateCooldown keeps a failed, rolled-back or ineffective update from
// being queued on every poll. A successful update usually makes the agent
// current, so the wait costs nothing there; it only matters when the machine
// still reports the old build afterwards.
const autoUpdateCooldown = time.Hour

func (s *server) maybeAutoUpdateAgent(r *http.Request, a store.SyncAgent) {
	unavailable := !a.AutoUpdate || a.Paused
	if s.releases == nil || unavailable {
		return
	}
	target, update := agentupdate.UpdateTarget(a.Version, a.Commit, s.releases.Info(r.Context()))
	if !update {
		return
	}
	recent, err := s.st.AgentUpdateSettledSince(s.uid(r), a.ID, time.Now().Add(-autoUpdateCooldown).UnixMilli())
	if err != nil || recent {
		return
	}
	if _, err = s.st.QueueAgentUpdate(s.uid(r), a.ID, target); err == nil {
		s.eventRequest(r, "sync-agent-auto-update", "", map[string]string{"agent": a.ID, "version": target, "commit": a.Commit})
	}
}

// reportUpdateStage lets the enrolled agent narrate its own update. The
// controller keeps the timeline the person watches; it never supplies the
// binary or its stages' outcome.
func (s *server) reportUpdateStage(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in struct{ ID, Stage, Detail string }
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.SetAgentUpdateStage(s.uid(r), a.ID, in.ID, in.Stage, in.Detail); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// pollSyncAgent answers a machine's poll. A machine below the skills
// protocol is told it needs an update (its agent's skillsState) and gets no
// skill work: only its update jobs and scans.
func (s *server) pollSyncAgent(w http.ResponseWriter, r *http.Request) {
	a, ok := s.skillsAgentIdentity(w, r)
	if !ok {
		return
	}
	s.pollSkillsAgent(w, r, a)
}

// finishJobBodyLimit is the largest job result a machine may send.
const finishJobBodyLimit = 16 << 20

func (s *server) finishSyncJob(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	// An older agent finishing an engine job it ran before the upgrade sends
	// that job's whole plan beside its status. The cap is the one the old
	// route had, so such a result is read and refused (409, which the agent
	// drops) rather than called malformed; only the id, status and message
	// are kept.
	var in struct{ ID, Status, Message string }
	if !decodeAgentJSON(w, r, finishJobBodyLimit, &in, "Invalid or oversized job result.") {
		return
	}
	if err := s.st.FinishSyncJob(s.uid(r), a.ID, in.ID, in.Status, in.Message); err != nil {
		var refused store.SyncJobResultRefused
		if errors.As(err, &refused) {
			writeErr(w, 409, err.Error())
			return
		}
		// A storage or transaction failure may clear up; keep the agent's
		// durable result for another attempt instead of calling it a refusal.
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// syncAgentProof answers a machine that is deciding whether to trust this
// controller with its credentials, for `reet agent set-server`. It needs no
// credential, because it is asked before any is sent. The machine gives its id
// and a fresh nonce; the answer is an HMAC keyed by the stored hash of the
// machine's secret, which only a controller holding the machine's record can
// make. The id is not a secret (it is shown in the Machines page), the nonce is
// signed under a fixed label so it cannot be turned into another kind of
// signature, and the answer reveals neither the hash nor the secret.
func (s *server) syncAgentProof(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID    string `json:"id"`
		Nonce string `json:"nonce"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.ID == "" || len(in.ID) > 128 || !syncproof.ValidNonce(in.Nonce) {
		writeErrCode(w, http.StatusBadRequest, "bad_proof_request", "", "Send the machine id and a nonce of 32 to 128 hex digits.", "")
		return
	}
	hash, ok, err := s.st.AgentSecretHash(in.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read the machine record")
		return
	}
	if !ok {
		writeErrCode(w, http.StatusNotFound, "unknown_machine", "", "This controller has no record of that machine.", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"proof": syncproof.Compute(hash, in.ID, in.Nonce)})
}
