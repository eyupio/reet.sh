package api

import (
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
	writeJSON(w, 200, map[string]any{"environments": envs, "machines": store.GroupMachines(envs), "targets": targets, "canWrite": s.currentUser(r).Role != "viewer"})
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
	s.event(s.uid(r), "ui", "sync-target", "", map[string]string{"source": in.SourceID, "target": in.TargetID})
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
	s.event(s.uid(r), "ui", "forget-environment", "", map[string]string{"environment": id})
	s.syncState(w, r)
}
