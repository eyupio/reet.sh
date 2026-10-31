package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	sentenceSoftwareUpdate  = "Update reet on that machine to inspect its installed software."
	sentenceSoftwareUnknown = "reet couldn't read how OpenCode is installed on that machine."
)

func (s *server) registerSoftwareRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/machines/{agentId}/software/plan", s.terminal(s.sameOriginRead(s.machineSoftwarePlan)))
	mux.HandleFunc("POST /api/v2/machines/{agentId}/software/apply", s.terminal(s.machineSoftwareApply))
	mux.HandleFunc("GET /api/v2/software/plans", s.terminal(s.sameOriginRead(s.fleetSoftwarePlans)))
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
		writeErrCode(w, http.StatusBadRequest, "software_action_invalid", "", "Choose install, update, rollback or remove.", "")
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
	if errors.Is(err, agentsoftware.ErrNoRollback) {
		writeErrCode(w, http.StatusConflict, "software_no_rollback", "", "No earlier reet-managed version is kept on "+host+".", "")
		return
	}
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

// machineSoftwareApply runs one reviewed install or update. The browser sends
// only the tool and action; the controller rebuilds the plan from stored
// evidence, refuses while a session of that tool runs, and the connector
// re-checks the evidence before it downloads anything.
func (s *server) machineSoftwareApply(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	agentID := r.PathValue("agentId")
	var in struct {
		Tool   string `json:"tool"`
		Action string `json:"action"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeErrCode(w, http.StatusBadRequest, "software_request_invalid", "", "Choose an install or update.", "")
		return
	}
	tool := strings.TrimSpace(in.Tool)
	if tool == "" {
		tool = agentsoftware.OpenCodeTool
	}
	action := agentsoftware.Action(strings.TrimSpace(in.Action))
	if action != agentsoftware.Install && action != agentsoftware.Update && action != agentsoftware.Rollback && action != agentsoftware.Remove {
		writeErrCode(w, http.StatusBadRequest, "software_action_invalid", "", "Choose install, update, rollback or remove.", "")
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
	if !caps.Capabilities.SoftwareApply {
		writeErrCode(w, http.StatusConflict, "software_apply_unsupported", "", "Update reet on that machine to install agents.", "")
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
	if errors.Is(err, agentsoftware.ErrNoRollback) {
		writeErrCode(w, http.StatusConflict, "software_no_rollback", "", "No earlier reet-managed version is kept on "+host+".", "")
		return
	}
	if errors.Is(err, agentsoftware.ErrAction) {
		writeErrCode(w, http.StatusConflict, "software_action_mismatch", "", "That action doesn't match what is installed on "+host+".", "")
		return
	}
	if err != nil {
		writeErrCode(w, http.StatusConflict, "software_unsupported", "", "reet has no reviewed OpenCode plan for that machine.", "")
		return
	}
	if running > 0 {
		writeErrCode(w, http.StatusConflict, "software_sessions_running", "", plan.Effects[0], "")
		return
	}

	request := terminalproto.SoftwareApply{
		Tool: tool, Action: action, ExpectedState: entry.State,
		ExpectedVersion: entry.Version, ExpectedPrevious: entry.Previous, ExpectedManaged: entry.Managed,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Minute)
	defer cancel()
	result, err := s.terminals.SoftwareApply(ctx, uid, agentID, request)
	if errors.Is(err, terminalbroker.ErrSoftwareApplyUnsupported) {
		writeErrCode(w, http.StatusConflict, "software_apply_unsupported", "", "Update reet on that machine to install agents.", "")
		return
	}
	if errors.Is(err, terminalbroker.ErrOffline) {
		writeErrCode(w, http.StatusConflict, "software_offline", "", "That machine isn't connected. Try again when it reconnects.", "")
		return
	}
	if err != nil {
		terminalError(w, err)
		return
	}

	s.event(uid, "ui", "software-apply", "", map[string]any{
		"host": host, "tool": tool, "action": string(action), "state": string(result.State),
		"version": result.Version, "previous": result.Previous, "reason": string(result.Reason),
	})
	if result.State != terminalproto.SoftwareApplyApplied {
		status, code, sentence := softwareApplyRefusal(result)
		writeErrCode(w, status, code, "", sentence, "")
		return
	}
	if result.Entry != nil {
		if latest, err := s.st.RemoteCapabilitiesFor(uid, agentID); err == nil && latest.Generation == caps.Generation {
			merged := latest.Capabilities
			merged.Software = mergeSoftwareEntry(merged.Software, *result.Entry)
			if merged.Validate() == nil {
				_ = s.st.UpdateRemoteCapabilitiesFor(uid, agentID, latest.InstanceID, latest.Generation, merged)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan, "result": result})
}

func mergeSoftwareEntry(entries []terminalproto.SoftwareEntry, entry terminalproto.SoftwareEntry) []terminalproto.SoftwareEntry {
	out := make([]terminalproto.SoftwareEntry, 0, len(entries)+1)
	replaced := false
	for _, existing := range entries {
		if existing.Tool == entry.Tool {
			out = append(out, entry)
			replaced = true
			continue
		}
		out = append(out, existing)
	}
	if !replaced {
		out = append(out, entry)
	}
	return out
}

func softwareApplyRefusal(result terminalproto.SoftwareApplyResult) (int, string, string) {
	switch result.Reason {
	case terminalproto.SoftwareApplyChanged:
		return http.StatusConflict, "software_changed", "The machine's installation changed since you reviewed the plan. Review it again."
	case terminalproto.SoftwareApplyNoRollback:
		return http.StatusConflict, "software_no_rollback", "No earlier reet-managed version is kept on that machine."
	case terminalproto.SoftwareApplyRollback:
		return http.StatusInternalServerError, "software_rollback_failed", "That machine couldn't roll back to the kept version. Nothing was changed."
	case terminalproto.SoftwareApplyRemoveFailed:
		return http.StatusInternalServerError, "software_remove_failed", "That machine couldn't remove its managed copy. Nothing was changed."
	case terminalproto.SoftwareApplyBusy:
		return http.StatusConflict, "software_busy", "That machine is already installing an agent. Try again in a moment."
	case terminalproto.SoftwareApplySessions:
		return http.StatusConflict, "software_sessions_running", "An OpenCode session is still running on that machine. Finish or close it first; reet won't stop it."
	case terminalproto.SoftwareApplyNotManaged:
		return http.StatusConflict, "software_not_managed", "reet didn't install that OpenCode, so it won't change it."
	case terminalproto.SoftwareApplyOff:
		return http.StatusConflict, "software_apply_off", "Software changes are switched off on that machine."
	case terminalproto.SoftwareApplyDownload:
		return http.StatusBadGateway, "software_download_failed", "That machine couldn't download the reviewed artifact. Nothing was changed."
	case terminalproto.SoftwareApplyChecksum:
		return http.StatusBadGateway, "software_checksum_failed", "The downloaded artifact didn't match its recorded checksum. Nothing was installed."
	case terminalproto.SoftwareApplyTimedOut:
		return http.StatusGatewayTimeout, "software_timed_out", "That machine took too long to install the artifact. Nothing was changed."
	case terminalproto.SoftwareApplyUnsupported:
		return http.StatusConflict, "software_apply_unsupported", "Update reet on that machine to install agents."
	default:
		return http.StatusInternalServerError, "software_install_failed", "That machine couldn't install the reviewed artifact. The earlier version, if any, was kept."
	}
}

// fleetSoftwareRow is one machine's summary for the plan-only fleet review. It
// is built from stored evidence; the review never contacts a machine.
type fleetSoftwareRow struct {
	AgentID         string `json:"agentId"`
	Host            string `json:"host"`
	State           string `json:"state"`
	Action          string `json:"action,omitempty"`
	Version         string `json:"version,omitempty"`
	Previous        string `json:"previous,omitempty"`
	Managed         bool   `json:"managed,omitempty"`
	Target          string `json:"target,omitempty"`
	Source          string `json:"source,omitempty"`
	SessionsRunning int    `json:"sessionsRunning"`
	CanApply        bool   `json:"canApply"`
	Sentence        string `json:"sentence"`
}

func (s *server) fleetSoftwarePlans(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	agents, err := s.st.SyncAgentsFor(uid)
	if err != nil {
		terminalError(w, err)
		return
	}
	running := map[string]int{}
	if sessions, err := s.st.TerminalSessionsFor(uid); err == nil {
		for _, session := range sessions {
			if session.Tool == agentsoftware.OpenCodeTool && session.State.ReservesQuota() {
				running[session.AgentID]++
			}
		}
	}
	rows := make([]fleetSoftwareRow, 0, len(agents))
	for _, agent := range agents {
		row := fleetSoftwareRow{AgentID: agent.ID, Host: agent.Host, SessionsRunning: running[agent.ID]}
		caps, err := s.st.RemoteCapabilitiesFor(uid, agent.ID)
		if errors.Is(err, store.ErrRemoteNotFound) {
			row.State, row.Sentence = "unknown", sentenceSoftwareUpdate
			rows = append(rows, row)
			continue
		}
		if err != nil {
			row.State, row.Sentence = "unknown", "reet couldn't read that machine's software."
			rows = append(rows, row)
			continue
		}
		row.CanApply = caps.Capabilities.SoftwareApply
		var entry *terminalproto.SoftwareEntry
		for i := range caps.Capabilities.Software {
			if caps.Capabilities.Software[i].Tool == agentsoftware.OpenCodeTool {
				entry = &caps.Capabilities.Software[i]
				break
			}
		}
		if entry == nil {
			row.State, row.Sentence = "unknown", "reet has no OpenCode evidence for this machine."
			if len(caps.Capabilities.Software) == 0 {
				row.Sentence = sentenceSoftwareUpdate
			}
			rows = append(rows, row)
			continue
		}
		row.Version, row.Previous, row.Managed = entry.Version, entry.Previous, entry.Managed
		action := agentsoftware.Action("")
		switch {
		case entry.State == terminalproto.SoftwareInstalled && entry.Managed && entry.Version != agentsoftware.OpenCodeReviewedVersion:
			action = agentsoftware.Update
		case entry.State == terminalproto.SoftwareInstalled && entry.Managed:
			row.State, row.Sentence = "none", "OpenCode is already the reviewed version."
			if entry.Previous != "" {
				row.Sentence += " A rollback is available in Settings."
			}
		case entry.State == terminalproto.SoftwareInstalled && !entry.Managed:
			action = agentsoftware.Install
		case entry.State == terminalproto.SoftwareMissing:
			action = agentsoftware.Install
		default:
			row.State, row.Sentence = "unknown", "reet couldn't read how OpenCode is installed."
		}
		if row.State == "unknown" && action == "" {
			rows = append(rows, row)
			continue
		}
		if action == "" {
			rows = append(rows, row)
			continue
		}
		plan, planErr := agentsoftware.For(*entry, action, agent.Host, row.SessionsRunning)
		if planErr != nil {
			row.State, row.Sentence = "unknown", "No reviewed plan is available for this machine."
			rows = append(rows, row)
			continue
		}
		row.State, row.Action = string(action), string(action)
		row.Target, row.Source, row.Sentence = plan.Target, plan.Source, plan.Sentence
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"machines": rows})
}
