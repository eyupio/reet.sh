package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (s *server) registerHealthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/machines/health", s.machineHealth)
}

type machineHealthState struct {
	AgentID      string                      `json:"agentId"`
	Host         string                      `json:"host"`
	State        string                      `json:"state"`
	Capabilities *terminalproto.Capabilities `json:"capabilities,omitempty"`
	ObservedAt   int64                       `json:"observedAt,omitempty"`
	Generation   int64                       `json:"generation,omitempty"`
	Connection   *agenthealth.Status         `json:"connection,omitempty"`
}

type machineHealthHistory struct {
	AgentID      string                      `json:"agentId"`
	Minute       int64                       `json:"minute"`
	Generation   int64                       `json:"generation"`
	Revision     int64                       `json:"revision"`
	Summary      terminalproto.HealthSummary `json:"summary"`
	RTTP50Millis *float64                    `json:"rttP50Millis,omitempty"`
	RTTP95Millis *float64                    `json:"rttP95Millis,omitempty"`
}

type machineHealthResponse struct {
	ServerNow int64                  `json:"serverNow"`
	Machines  []machineHealthState   `json:"machines"`
	History   []machineHealthHistory `json:"history,omitempty"`
}

// machineHealth returns controller-derived current state and, for an explicit
// machine, its bounded stored history. It never infers live connectivity from
// retained history and never exposes another account's machines or a private
// transport capability.
func (s *server) machineHealth(w http.ResponseWriter, r *http.Request) {
	if !s.privateCredentials() {
		writeErr(w, http.StatusForbidden, "machine health requires an authenticated instance")
		return
	}
	uid := s.uid(r)
	agents, err := s.st.SyncAgentsFor(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not read machines")
		return
	}
	now := time.Now()
	response := machineHealthResponse{ServerNow: now.UnixMilli(), Machines: []machineHealthState{}}
	wanted := r.URL.Query().Get("agent")
	found := wanted == ""
	for _, agent := range agents {
		machine := machineHealthState{AgentID: agent.ID, Host: agent.Host, State: "never_connected"}
		caps, capsErr := s.st.RemoteCapabilitiesFor(uid, agent.ID)
		if capsErr == nil {
			value := caps.Capabilities
			machine.Capabilities = &value
			machine.ObservedAt = caps.ObservedAt
			machine.Generation = caps.Generation
			machine.State = "unknown"
		} else if !errors.Is(capsErr, store.ErrRemoteNotFound) {
			writeErr(w, http.StatusInternalServerError, "could not read machine capabilities")
			return
		}
		if s.healthHub != nil {
			if status, ok := s.healthHub.Status(uid, agent.ID); ok {
				machine.Connection = &status
				switch {
				case status.Connected:
					machine.State = "connected"
				case status.Unreachable || status.LastKnown:
					machine.State = "unreachable"
				default:
					machine.State = "unknown"
				}
			}
		}
		if wanted == agent.ID {
			found = true
		}
		response.Machines = append(response.Machines, machine)
	}
	if !found {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if wanted != "" {
		minutes, err := s.st.RemoteHealthMinutesFor(uid, wanted)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not read machine history")
			return
		}
		response.History = make([]machineHealthHistory, 0, len(minutes))
		for _, minute := range minutes {
			response.History = append(response.History, machineHealthHistory{
				AgentID: minute.AgentID, Minute: minute.Minute, Generation: minute.Generation, Revision: minute.Revision,
				Summary:      minute.Summary,
				RTTP50Millis: agenthealth.Percentile(minute.Summary.RTTMillis, 0.5),
				RTTP95Millis: agenthealth.Percentile(minute.Summary.RTTMillis, 0.95),
			})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}
