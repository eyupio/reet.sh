package api

import (
	"errors"
	"github.com/eyupio/reet.sh/internal/store"
	"net/http"
)

func (s *server) syncState(w http.ResponseWriter, r *http.Request) {
	envs, err := s.st.SyncEnvironmentsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	targets, err := s.st.SyncTargetsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	machines, err := s.st.GroupMachinesFor(s.uid(r), envs)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	agents, err := s.st.SyncAgentsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	jobs, err := s.st.SyncJobsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	runs, err := s.st.SyncRunsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"agents": agents, "jobs": jobs, "runs": runs, "environments": envs, "machines": machines, "targets": targets, "canWrite": s.currentUser(r).Role != "viewer"})
}
func (s *server) saveSync(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SourceID string `json:"sourceId"`
		TargetID string `json:"targetId"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.SaveSyncTargetFor(s.uid(r), in.SourceID, in.TargetID); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.eventRequest(r, "sync-target", "", map[string]string{"source": in.SourceID, "target": in.TargetID})
	s.syncState(w, r)
}
func (s *server) deleteSync(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteSyncTargetFor(s.uid(r), r.URL.Query().Get("target")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.syncState(w, r)
}

func (s *server) forgetEnvironment(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := s.st.ForgetEnvironmentFor(s.uid(r), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "forget-environment", "", map[string]string{"environment": id})
	s.syncState(w, r)
}

func (s *server) mergeMachines(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if !decode(w, r, &in) {
		return
	}
	s.importMu.Lock()
	defer s.importMu.Unlock()
	if err := s.st.MergeMachinesFor(s.uid(r), in.From, in.To); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, store.ErrMachineName) {
			code = http.StatusBadRequest
		}
		if errors.Is(err, store.ErrMachineConflict) {
			code = http.StatusConflict
		}
		writeErr(w, code, err.Error())
		return
	}
	s.eventRequest(r, "merge-machines", "", map[string]string{"from": in.From, "to": in.To})
	s.syncState(w, r)
}
func (s *server) forgetMachine(w http.ResponseWriter, r *http.Request) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	host := r.URL.Query().Get("host")
	if err := s.st.ForgetMachineFor(s.uid(r), host); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.eventRequest(r, "forget-machine", "", map[string]string{"host": host})
	s.syncState(w, r)
}
