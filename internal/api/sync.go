package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/agentupdate"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) syncState(w http.ResponseWriter, r *http.Request) {
	envs, err := s.st.SyncEnvironmentsFor(s.uid(r))
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
	for i := range agents {
		// What the machine last reported over its private connection.
		if c, e := s.st.RemoteCapabilitiesRawFor(s.uid(r), agents[i].ID); e == nil {
			agents[i].Locked, agents[i].LockedAt = c.Capabilities.Locked, c.Capabilities.LockedAt
		}
	}
	if s.authMode != AuthAccounts {
		// Without accounts there is no personal token to warn about.
		for i := range agents {
			agents[i].LegacyToken = false
		}
	}
	jobs, err := s.st.SyncJobsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	updateJobs, err := s.st.LatestUpdateJobsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	releases := agentupdate.Info{}
	if s.releases != nil {
		releases = s.releases.Info(r.Context())
	}
	// targets and runs belonged to the job-based sync, which is gone: every
	// machine follows the library. They stay, empty, for a page that still
	// reads them.
	writeJSON(w, 200, map[string]any{"agents": agents, "jobs": jobs, "updateJobs": updateJobs, "runs": []any{}, "environments": envs, "machines": machines, "targets": []any{}, "releases": releases, "canWrite": s.currentUser(r).Role != "viewer"})
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
