package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	sentenceSoftwareUpdate  = "Update reet on that machine to inspect its installed software."
	sentenceSoftwareUnknown = "reet couldn't read how OpenCode is installed on that machine."
)

func (s *server) registerSoftwareRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/machines/{agentId}/software/plan", s.terminal(s.sameOriginRead(s.machineSoftwarePlan)))
}

type softwarePlanView struct {
	Host      string `json:"host"`
	CheckedAt int64  `json:"checkedAt"`
	agentsoftware.Plan
}

// machineSoftwarePlan returns a reviewable plan for one observed OpenCode
// installation. It reads stored evidence and running sessions; it never
// contacts the machine or runs a command.
func (s *server) machineSoftwarePlan(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	agentID := r.PathValue("agentId")
	tool := strings.TrimSpace(r.URL.Query().Get("tool"))
	if tool == "" {
		tool = agentsoftware.OpenCodeTool
	}
	action := agentsoftware.Action(strings.TrimSpace(r.URL.Query().Get("action")))
	if !action.Valid() {
		writeErrCode(w, http.StatusBadRequest, "software_action_invalid", "", "Choose install, update or remove.", "")
		return
	}
	if !terminalproto.ValidTool(tool) {
		writeErrCode(w, http.StatusBadRequest, "software_tool_invalid", "", "reet doesn't know that agent.", "")
		return
	}

	var host string
	found := false
	if agents, err := s.st.SyncAgentsFor(uid); err == nil {
		for _, a := range agents {
			if a.ID == agentID {
				host, found = a.Host, true
				break
			}
		}
	}
	if !found {
		writeErrCode(w, http.StatusNotFound, "machine_not_found", "", "That machine isn't enrolled.", "")
		return
	}

	caps, err := s.st.RemoteCapabilitiesFor(uid, agentID)
	if errors.Is(err, store.ErrRemoteNotFound) {
		writeErrCode(w, http.StatusConflict, "software_unsupported", "", sentenceSoftwareUpdate, "")
		return
	}
	if err != nil {
		terminalError(w, err)
		return
	}
	var entry *terminalproto.SoftwareEntry
	for i := range caps.Capabilities.Software {
		if caps.Capabilities.Software[i].Tool == tool {
			entry = &caps.Capabilities.Software[i]
			break
		}
	}
	if entry == nil {
		if len(caps.Capabilities.Software) == 0 {
			writeErrCode(w, http.StatusConflict, "software_unsupported", "", sentenceSoftwareUpdate, "")
			return
		}
		writeErrCode(w, http.StatusConflict, "software_unsupported", "", "reet has no reviewed plan for that agent.", "")
		return
	}
	switch entry.State {
	case terminalproto.SoftwareInstalled:
	case terminalproto.SoftwareMissing:
		if action != agentsoftware.Install {
			writeErrCode(w, http.StatusConflict, "software_not_installed", "", "OpenCode isn't installed on "+host+".", "")
			return
		}
	case terminalproto.SoftwareUnsupported:
		writeErrCode(w, http.StatusConflict, "software_unsupported", "", "reet has no reviewed OpenCode plan for that machine.", "")
		return
	default:
		writeErrCode(w, http.StatusConflict, "software_unreadable", "", sentenceSoftwareUnknown, "")
		return
	}

	running := 0
	if sessions, err := s.st.TerminalSessionsFor(uid); err == nil {
		for _, session := range sessions {
			if session.AgentID == agentID && session.Tool == tool && session.State.ReservesQuota() {
				running++
			}
		}
	}
	plan, err := agentsoftware.For(*entry, action, host, running)
	if errors.Is(err, agentsoftware.ErrAction) {
		writeErrCode(w, http.StatusConflict, "software_action_mismatch", "", "That action doesn't match what is installed on "+host+".", "")
		return
	}
	if err != nil {
		writeErrCode(w, http.StatusConflict, "software_unsupported", "", "reet has no reviewed OpenCode plan for that machine.", "")
		return
	}
	writeJSON(w, http.StatusOK, softwarePlanView{Host: host, CheckedAt: entry.CheckedAt, Plan: plan})
}
