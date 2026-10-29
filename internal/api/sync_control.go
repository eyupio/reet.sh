package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/eyupio/reet.sh/internal/agentupdate"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) registerSyncControl(mux *http.ServeMux) {
	s.registerRemoteImports(mux)
	mux.HandleFunc("GET /api/v2/sync/skill-import", s.privateSkillAccess(s.machineSkillImport))
	mux.HandleFunc("POST /api/v2/sync/skill-import", s.privateSkillAccess(s.machineSkillImport))
	mux.HandleFunc("POST /api/v2/sync/content-observations/prepare", s.requireWriter(s.contentObservation))
	mux.HandleFunc("POST /api/v2/sync/content-observations", s.requireWriter(s.contentObservation))
	mux.HandleFunc("GET /api/v2/sync/candidates", s.syncCandidates)
	mux.HandleFunc("PUT /api/v2/sync/desired", s.requireWriter(s.saveDesired))
	mux.HandleFunc("POST /api/v2/sync/reconcile", s.requireWriter(s.reconcileDesired))
	mux.HandleFunc("POST /api/v2/sync/adopt", s.requireWriter(s.adoptSyncSkills))
	mux.HandleFunc("POST /api/v2/sync/run", s.requireWriter(s.startSyncRun))
	mux.HandleFunc("POST /api/v2/sync/agents/update", s.requireWriter(s.updateSyncAgent))
	mux.HandleFunc("POST /api/v2/sync/agents/update/stage", s.requireWriter(s.reportUpdateStage))
	mux.HandleFunc("POST /api/v2/sync/agents", s.requireWriter(s.enrollSyncAgent))
	mux.HandleFunc("PATCH /api/v2/sync/agents", s.requireWriter(s.controlSyncAgent))
	mux.HandleFunc("POST /api/v2/sync/jobs/stage", s.requireWriter(s.reportSyncStage))
	mux.HandleFunc("POST /api/v2/sync/jobs", s.requireWriter(s.queueSyncJob))
	mux.HandleFunc("PATCH /api/v2/sync/jobs", s.requireWriter(s.controlSyncJob))
	mux.HandleFunc("POST /api/v2/sync/poll", s.requireWriter(s.pollSyncAgent))
	mux.HandleFunc("POST /api/v2/sync/result", s.requireWriter(s.finishSyncJob))
	mux.HandleFunc("POST /api/v2/sync/identity", s.requireWriter(s.syncIdentity))
	mux.HandleFunc("POST /api/v2/sync/private", s.requireWriter(s.privateBootstrap))
}
func (s *server) syncCandidates(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("list"), 10, 64)
	skills, err := s.st.SyncCandidatesFor(s.uid(r), s.role(r), id)
	if r.URL.Query().Get("collection") == "hosted" {
		skills, err = s.st.SyncHostedFor(s.uid(r), requestOrigin(r))
	}
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"skills": skills})
}
func (s *server) saveDesired(w http.ResponseWriter, r *http.Request) {
	var in store.DesiredSelection
	if !decode(w, r, &in) {
		return
	}
	s.importMu.Lock()
	defer s.importMu.Unlock()
	in.Origin = requestOrigin(r)
	if err := s.st.SaveDesiredFor(s.uid(r), s.role(r), in); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	s.eventRequest(r, "sync-desired", "", map[string]string{"target": in.TargetID, "mode": in.Mode})
	s.syncState(w, r)
}
func (s *server) reconcileDesired(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TargetID string `json:"targetId"`
	}
	if !decode(w, r, &in) {
		return
	}
	targets, err := s.st.SyncTargetsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	envs, err := s.st.SyncEnvironmentsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, target := range targets {
		if target.Target.ID != in.TargetID {
			continue
		}
		for _, env := range envs {
			if env.ID == in.TargetID && env.Complete && env.ScannedAt > time.Now().Add(-14*24*time.Hour).UnixMilli() {
				skills, conflicts := store.ReconcileSkills(target.Baseline, target.Desired, env.Skills)
				writeJSON(w, 200, map[string]any{"skills": skills, "conflicts": conflicts, "revision": target.Revision})
				return
			}
		}
	}
	writeErr(w, 409, "target needs a complete recent scan")
}
func (s *server) startSyncRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AgentID   string   `json:"agentId"`
		TargetIDs []string `json:"targetIds"`
	}
	if !decode(w, r, &in) {
		return
	}
	if _, err := s.st.StartSyncRun(s.uid(r), in.AgentID, in.TargetIDs); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	s.eventRequest(r, "sync-run", "", map[string]string{"agent": in.AgentID})
	s.syncState(w, r)
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
func (s *server) adoptSyncSkills(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TargetID string   `json:"targetId"`
		SkillIDs []string `json:"skillIds"`
		ListID   int64    `json:"listId"`
	}
	if !decode(w, r, &in) {
		return
	}
	s.importMu.Lock()
	defer s.importMu.Unlock()
	if err := s.st.AdoptSyncSkillsFor(s.uid(r), s.role(r), in.TargetID, in.SkillIDs, in.ListID); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
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
	a, secret, err := s.st.EnrollSyncAgentBuild(s.uid(r), in.Host, in.Version, in.Commit)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
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
func (s *server) queueSyncJob(w http.ResponseWriter, r *http.Request) {
	var in struct{ AgentID, TargetID, Kind string }
	if !decode(w, r, &in) {
		return
	}
	if _, err := s.st.QueueSyncJob(s.uid(r), in.AgentID, in.TargetID, in.Kind); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	s.syncState(w, r)
}
func (s *server) controlSyncJob(w http.ResponseWriter, r *http.Request) {
	var in struct{ ID, Action, Hash string }
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.ControlSyncJob(s.uid(r), in.ID, in.Action, in.Hash); err != nil {
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
	if s.releases == nil || !a.AutoUpdate || a.Paused {
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

func (s *server) pollSyncAgent(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	s.importMu.Lock()
	err := s.st.AdvanceMachineSkillSyncs(s.uid(r), s.role(r))
	if err == nil {
		err = s.st.MaintainSync(s.uid(r), s.role(r), a)
	}
	s.importMu.Unlock()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.maybeAutoUpdateAgent(r, a)
	job, err := s.st.ClaimSyncJob(s.uid(r), a)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"agent": a, "job": job})
}
func (s *server) finishSyncJob(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in struct {
		ID, Status, Hash, Message string
		Plan                      json.RawMessage
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.FinishSyncJob(s.uid(r), a.ID, in.ID, in.Status, in.Hash, in.Message, in.Plan); err != nil {
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

func (s *server) reportSyncStage(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in struct{ ID, Stage, Detail string }
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.SetSyncStage(s.uid(r), a.ID, in.ID, in.Stage, in.Detail); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
