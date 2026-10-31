package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/softwarerun"
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
	mux.HandleFunc("POST /api/v2/software/batches", s.terminal(s.softwareBatchStart))
	mux.HandleFunc("GET /api/v2/software/batches/{batchId}", s.terminal(s.sameOriginRead(s.softwareBatchGet)))
	mux.HandleFunc("POST /api/v2/software/batches/{batchId}/members/{position}/retry", s.terminal(s.softwareBatchRetry))
	mux.HandleFunc("POST /api/v2/software/batches/{batchId}/members/{position}/skip", s.terminal(s.softwareBatchSkip))
	mux.HandleFunc("POST /api/v2/software/batches/{batchId}/abandon", s.terminal(s.softwareBatchAbandon))
	mux.HandleFunc("GET /api/v2/settings/software-mirror", s.requireWriter(s.softwareMirrorGet))
	mux.HandleFunc("PUT /api/v2/settings/software-mirror", s.requireAdmin(s.softwareMirrorPut))
	mux.HandleFunc("POST /api/v2/settings/software-mirror/cache", s.requireAdmin(s.softwareMirrorCache))
}

// newSoftwareRuns wires the batch driver to the store and the same apply path
// a single machine uses. A finished batch is audited once, in the background.
func (s *server) newSoftwareRuns() *softwarerun.Orchestrator {
	return softwarerun.New(softwarerun.Config{
		Store:  s.st,
		Broker: s.terminals,
		Settled: func(uid int64, batch store.SoftwareBatch) {
			s.event(uid, "ui", "software-batch", "", softwareBatchAudit(batch, "finish"))
		},
		Observed: func(uid int64, agentID string, entry terminalproto.SoftwareEntry) {
			s.recordSoftwareEntry(uid, agentID, 0, entry)
		},
		Logf: func(format string, args ...any) {
			log.Printf("software batch: "+format, args...)
		},
	})
}

// recordSoftwareEntry merges fresh evidence from an apply into the stored
// capabilities and refreshes the launch inventory, so the fleet review and the
// next launch see the change without waiting for a reconnect. expectGeneration
// is the generation the plan was made from; zero accepts the latest.
func (s *server) recordSoftwareEntry(uid int64, agentID string, expectGeneration int64, entry terminalproto.SoftwareEntry) {
	latest, err := s.st.RemoteCapabilitiesFor(uid, agentID)
	if err != nil || expectGeneration != 0 && latest.Generation != expectGeneration {
		return
	}
	merged := latest.Capabilities
	merged.Software = mergeSoftwareEntry(merged.Software, entry)
	if merged.Launch {
		merged.Tools = mergeLaunchableTool(merged.Tools, entry.Tool, entry.State)
	}
	if merged.Validate() == nil {
		_ = s.st.UpdateRemoteCapabilitiesFor(uid, agentID, latest.InstanceID, latest.Generation, merged)
	}
}

// ResumeSoftwareBatches continues batches that still have pending work after a
// controller restart. A member left running is reset to pending and reconciled.
func (s *Server) ResumeSoftwareBatches(ctx context.Context) {
	if s.s == nil || s.s.softwareRuns == nil {
		return
	}
	if err := s.s.softwareRuns.Resume(); err != nil {
		log.Printf("software batches: resume: %v", err)
	}
}

const maxBatchMachines = 100

// softwareBatchStart creates a batch from the ordered machines the person
// chose. Each machine's plan is rebuilt from stored evidence and frozen into
// the batch; no machine is contacted by this handler.
func (s *server) softwareBatchStart(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	var in struct {
		RequestID string   `json:"requestId"`
		Tool      string   `json:"tool"`
		Agents    []string `json:"agents"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeErrCode(w, http.StatusBadRequest, "software_request_invalid", "", "Choose the machines to update.", "")
		return
	}
	if !terminalproto.ValidID(in.RequestID) {
		writeErrCode(w, http.StatusUnprocessableEntity, "request_id_required", "requestId", "Starting a batch needs a request id: 16 to 128 letters, digits, hyphens or underscores.", "")
		return
	}
	tool := strings.TrimSpace(in.Tool)
	if tool == "" {
		tool = agentsoftware.OpenCodeTool
	}
	if !terminalproto.ValidTool(tool) {
		writeErrCode(w, http.StatusBadRequest, "software_tool_invalid", "", "reet doesn't know that agent.", "")
		return
	}
	if len(in.Agents) == 0 || len(in.Agents) > maxBatchMachines {
		writeErrCode(w, http.StatusBadRequest, "software_batch_empty", "", "Choose between one and "+strconv.Itoa(maxBatchMachines)+" machines.", "")
		return
	}

	agents, err := s.st.SyncAgentsFor(uid)
	if err != nil {
		terminalError(w, err)
		return
	}
	hostByID := map[string]string{}
	for _, a := range agents {
		hostByID[a.ID] = a.Host
	}
	running := map[string]int{}
	if sessions, err := s.st.TerminalSessionsFor(uid); err == nil {
		for _, session := range sessions {
			if session.Tool == tool && session.State.ReservesQuota() {
				running[session.AgentID]++
			}
		}
	}

	members := make([]store.SoftwareBatchMemberInput, 0, len(in.Agents))
	seen := map[string]bool{}
	for _, agentID := range in.Agents {
		if seen[agentID] {
			writeErrCode(w, http.StatusBadRequest, "software_batch_duplicate", "agents", "A machine appears more than once in that batch.", "")
			return
		}
		seen[agentID] = true
		position := len(members)
		member := store.SoftwareBatchMemberInput{Position: position, AgentID: agentID}
		host, enrolled := hostByID[agentID]
		if !enrolled {
			member.Excluded, member.Reason = true, "That machine isn't enrolled."
			members = append(members, member)
			continue
		}
		member.Host = host
		caps, err := s.st.RemoteCapabilitiesFor(uid, agentID)
		if errors.Is(err, store.ErrRemoteNotFound) {
			member.Excluded, member.Reason = true, sentenceSoftwareUpdate
			members = append(members, member)
			continue
		}
		if err != nil {
			member.Excluded, member.Reason = true, "reet couldn't read that machine's software."
			members = append(members, member)
			continue
		}
		if !caps.Capabilities.SoftwareApply {
			member.Excluded, member.Reason = true, "Update reet on that machine to install agents."
			members = append(members, member)
			continue
		}
		var entry *terminalproto.SoftwareEntry
		for i := range caps.Capabilities.Software {
			if caps.Capabilities.Software[i].Tool == tool {
				entry = &caps.Capabilities.Software[i]
				break
			}
		}
		if entry == nil {
			member.Excluded, member.Reason = true, "reet has no OpenCode evidence for that machine."
			if len(caps.Capabilities.Software) == 0 {
				member.Reason = sentenceSoftwareUpdate
			}
			members = append(members, member)
			continue
		}
		action, why := reviewedSoftwareAction(*entry)
		if action == "" {
			member.Excluded, member.Reason = true, why
			members = append(members, member)
			continue
		}
		if running[agentID] > 0 {
			member.Excluded = true
			member.Reason = runningSentence(running[agentID], "apply")
			members = append(members, member)
			continue
		}
		plan, planErr := agentsoftware.For(*entry, action, host, 0)
		if planErr != nil {
			member.Excluded, member.Reason = true, "No reviewed plan is available for that machine."
			members = append(members, member)
			continue
		}
		member.Action = string(action)
		member.Target = plan.Target
		member.ExpectedState = string(entry.State)
		member.ExpectedVersion = entry.Version
		member.ExpectedPrevious = entry.Previous
		member.ExpectedManaged = entry.Managed
		members = append(members, member)
	}

	batch, created, err := s.st.CreateSoftwareBatchFor(uid, in.RequestID, tool, members)
	switch {
	case errors.Is(err, store.ErrSoftwareBatchInProgress):
		writeErrCode(w, http.StatusConflict, "software_batch_in_progress", "", "Another software batch is still waiting or running. Finish or abandon it first.", "")
		return
	case errors.Is(err, store.ErrSoftwareBatchNoMembers):
		writeErrCode(w, http.StatusConflict, "software_batch_empty", "", "None of those machines can apply the reviewed plan right now.", "")
		return
	case err != nil:
		terminalError(w, err)
		return
	}
	if created {
		s.event(uid, "ui", "software-batch", "", softwareBatchAudit(batch, "start"))
	}
	// A replay starts too: pass refuses to drive a stopped, done or abandoned
	// batch, so this only covers a create whose driver never began.
	s.softwareRuns.Start(uid, batch.ID)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, batch)
}

func (s *server) softwareBatchGet(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	batch, err := s.st.SoftwareBatchFor(uid, r.PathValue("batchId"))
	if errors.Is(err, store.ErrSoftwareBatchNotFound) {
		writeErrCode(w, http.StatusNotFound, "software_batch_not_found", "", "That software batch doesn't exist.", "")
		return
	}
	if err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

func (s *server) softwareBatchRetry(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	batchID := r.PathValue("batchId")
	position, ok := batchPosition(w, r)
	if !ok {
		return
	}
	if _, err := s.softwareRuns.Retry(uid, batchID, position); err != nil {
		softwareBatchError(w, err)
		return
	}
	s.event(uid, "ui", "software-batch-retry", "", map[string]any{"batchId": batchID, "position": position})
	s.softwareBatchAnswer(w, uid, batchID)
}

func (s *server) softwareBatchSkip(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	batchID := r.PathValue("batchId")
	position, ok := batchPosition(w, r)
	if !ok {
		return
	}
	if err := s.softwareRuns.Skip(uid, batchID, position); err != nil {
		softwareBatchError(w, err)
		return
	}
	s.event(uid, "ui", "software-batch-skip", "", map[string]any{"batchId": batchID, "position": position})
	s.softwareBatchAnswer(w, uid, batchID)
}

func (s *server) softwareBatchAbandon(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	batchID := r.PathValue("batchId")
	if _, err := s.softwareRuns.Abandon(uid, batchID); err != nil {
		softwareBatchError(w, err)
		return
	}
	s.event(uid, "ui", "software-batch-abandon", "", map[string]any{"batchId": batchID})
	s.softwareBatchAnswer(w, uid, batchID)
}

func (s *server) softwareBatchAnswer(w http.ResponseWriter, uid int64, batchID string) {
	batch, err := s.st.SoftwareBatchFor(uid, batchID)
	if err != nil {
		terminalError(w, err)
		return
	}
	// Retry, skip and abandon move a batch without the driver (skip can finish
	// it outright), so a terminal state is audited here too. The driver audits
	// the terminal states it produces itself.
	if batch.State == store.SoftwareBatchDone || batch.State == store.SoftwareBatchAbandoned {
		s.event(uid, "ui", "software-batch", "", softwareBatchAudit(batch, "finish"))
	}
	writeJSON(w, http.StatusOK, batch)
}

func batchPosition(w http.ResponseWriter, r *http.Request) (int, bool) {
	position, err := strconv.Atoi(r.PathValue("position"))
	if err != nil || position < 0 {
		writeErrCode(w, http.StatusBadRequest, "software_batch_position_invalid", "position", "That isn't a machine position in the batch.", "")
		return 0, false
	}
	return position, true
}

func softwareBatchError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrSoftwareBatchNotFound), errors.Is(err, store.ErrSoftwareBatchMemberNotFound):
		writeErrCode(w, http.StatusNotFound, "software_batch_not_found", "", "That software batch doesn't exist.", "")
	case errors.Is(err, store.ErrSoftwareBatchMemberNotFailed):
		writeErrCode(w, http.StatusConflict, "software_batch_member_not_failed", "", "That machine isn't waiting on a failure.", "")
	case errors.Is(err, store.ErrSoftwareBatchFinished):
		writeErrCode(w, http.StatusConflict, "software_batch_finished", "", "That software batch has already finished.", "")
	case errors.Is(err, store.ErrSoftwareBatchRunning):
		writeErrCode(w, http.StatusConflict, "software_batch_running", "", "A machine in that batch is still applying. Try again when it finishes.", "")
	default:
		terminalError(w, err)
	}
}

func softwareBatchAudit(batch store.SoftwareBatch, phase string) map[string]any {
	hosts := make([]string, 0, len(batch.Members))
	members := make([]map[string]any, 0, len(batch.Members))
	for _, m := range batch.Members {
		hosts = append(hosts, m.Host)
		members = append(members, map[string]any{
			"position": m.Position, "host": m.Host, "action": m.Action,
			"state": m.State, "code": m.Code, "version": m.Version,
		})
	}
	return map[string]any{
		"batchId": batch.ID, "phase": phase, "state": batch.State,
		"tool": batch.Tool, "hosts": hosts, "members": members,
	}
}

// reviewedSoftwareAction is the plan a machine's stored evidence calls for, or
// a sentence saying why there is none.
func reviewedSoftwareAction(entry terminalproto.SoftwareEntry) (agentsoftware.Action, string) {
	switch {
	case entry.State == terminalproto.SoftwareInstalled && entry.Managed && entry.Version != agentsoftware.OpenCodeReviewedVersion:
		return agentsoftware.Update, ""
	case entry.State == terminalproto.SoftwareInstalled && entry.Managed:
		return "", "OpenCode is already the reviewed version."
	case entry.State == terminalproto.SoftwareInstalled && !entry.Managed:
		return agentsoftware.Install, ""
	case entry.State == terminalproto.SoftwareMissing:
		return agentsoftware.Install, ""
	default:
		return "", "reet couldn't read how OpenCode is installed."
	}
}

func runningSentence(n int, action string) string {
	if n == 1 {
		return "1 OpenCode session is running; reet refuses to " + action + " while it runs."
	}
	return strconv.Itoa(n) + " OpenCode sessions are running; reet refuses to " + action + " while they run."
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
	ctx, cancel := context.WithTimeout(r.Context(), 22*time.Minute)
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
		"source": result.Source,
	})
	if result.State != terminalproto.SoftwareApplyApplied {
		status, code, sentence := softwareApplyRefusal(result)
		writeErrCode(w, status, code, "", sentence, "")
		return
	}
	if result.Entry != nil {
		s.recordSoftwareEntry(uid, agentID, caps.Generation, *result.Entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan, "result": result})
}

func mergeLaunchableTool(tools []string, tool string, state terminalproto.SoftwareState) []string {
	switch state {
	case terminalproto.SoftwareInstalled:
		for _, existing := range tools {
			if existing == tool {
				return tools
			}
		}
		out := append(append([]string{}, tools...), tool)
		sort.Strings(out)
		return out
	case terminalproto.SoftwareMissing:
		out := make([]string, 0, len(tools))
		for _, existing := range tools {
			if existing != tool {
				out = append(out, existing)
			}
		}
		return out
	}
	return tools
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
		if result.Source == "registry+mirror" {
			return http.StatusBadGateway, "software_download_failed", "That machine couldn't download the reviewed artifact from the registry or the controller's mirror. Nothing was changed."
		}
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
