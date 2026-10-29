package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Model lists read from the machine and cached per account, machine and tool.
// A read is never allowed to block a launch: there is no launch path here.

const (
	modelsSource        = "opencode models"
	modelsRefreshLimit  = 6
	modelsRefreshWindow = time.Minute
)

const (
	sentenceModelsUpdate      = "Update reet on that machine to read its models."
	sentenceModelsOff         = "Models are switched off on that machine."
	sentenceModelsBusy        = "That machine is already reading its models. Try again in a moment."
	sentenceModelsUnsupported = "reet can't read this agent's models; it uses its own list."
	sentenceModelsFailed      = "That machine couldn't read its models."
)

// errModelsUpdate means the capabilities we read were stale and the machine is
// an older connector after all; the answer is the update sentence, not a
// cached failure.
var errModelsUpdate = errors.New("machine needs an update to list models")

func (s *server) registerModelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/machines/{agentId}/models", s.terminal(s.sameOriginRead(s.machineModels)))
	mux.HandleFunc("POST /api/v2/machines/{agentId}/models/refresh", s.terminal(s.machineModelsRefresh))
	mux.HandleFunc("GET /api/v2/models", s.terminal(s.modelsList))
}

// modelLimits bounds person-pressed refreshes per account and machine. Allow
// and Hit run under one lock, so concurrent requests cannot both take the
// last slot.
type modelLimits struct {
	mu      sync.Mutex
	refresh *auth.Limiter
}

func (l *modelLimits) take(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.refresh.Allow(key) {
		return false
	}
	l.refresh.Hit(key)
	return true
}

func (s *server) modelLimitState() *modelLimits {
	s.modelsOnce.Do(func() {
		s.modelsLimits = &modelLimits{refresh: auth.NewLimiter(modelsRefreshLimit, modelsRefreshWindow)}
	})
	return s.modelsLimits
}

type modelView struct {
	Tool      string                     `json:"tool"`
	State     string                     `json:"state"`
	Models    []terminalproto.ModelEntry `json:"models"`
	More      bool                       `json:"more"`
	Source    string                     `json:"source"`
	Version   string                     `json:"version"`
	FetchedAt *int64                     `json:"fetchedAt"`
	CheckedAt *int64                     `json:"checkedAt"`
	Reason    string                     `json:"reason"`
	Sentence  string                     `json:"sentence"`
}

type modelsListEntry struct {
	AgentID   string                     `json:"agentId"`
	Host      string                     `json:"host"`
	State     string                     `json:"state"`
	Models    []terminalproto.ModelEntry `json:"models"`
	More      bool                       `json:"more"`
	Source    string                     `json:"source"`
	Version   string                     `json:"version"`
	FetchedAt *int64                     `json:"fetchedAt"`
	CheckedAt *int64                     `json:"checkedAt"`
	Reason    string                     `json:"reason"`
	Sentence  string                     `json:"sentence"`
}

func modelsSentence(state string, reason terminalproto.ModelsReason, host string) string {
	switch state {
	case "update":
		return sentenceModelsUpdate
	case "offline":
		return sentenceMachineOffline
	case "busy":
		return sentenceModelsBusy
	case store.MachineModelsOff:
		return sentenceModelsOff
	case store.MachineModelsUnsupported:
		return sentenceModelsUnsupported
	case store.MachineModelsListed:
		if host != "" {
			return "Models read from " + host + "."
		}
		return "Models read from that machine."
	}
	switch reason {
	case terminalproto.ModelsTimedOut:
		return sentenceMachineTimeout
	case terminalproto.ModelsConflict:
		return sentenceModelsBusy
	}
	return sentenceModelsFailed
}

func machineModelsView(row store.MachineModels, host string) modelView {
	models := row.Models
	if models == nil {
		models = []terminalproto.ModelEntry{}
	}
	v := modelView{
		Tool: row.Tool, State: row.State, Models: models, More: row.More,
		Source: row.Source, Version: row.AgentVersion, Reason: string(row.Reason),
		Sentence: modelsSentence(row.State, row.Reason, host),
	}
	if row.FetchedAt != 0 {
		at := row.FetchedAt
		v.FetchedAt = &at
	}
	if row.CheckedAt != 0 {
		at := row.CheckedAt
		v.CheckedAt = &at
	}
	return v
}

func machineModelsStatus(tool, state string, reason terminalproto.ModelsReason, host string) modelView {
	return machineModelsView(store.MachineModels{Tool: tool, State: state, Reason: reason}, host)
}

func modelsListEntryFor(row store.MachineModels, host string) modelsListEntry {
	v := machineModelsView(row, host)
	return modelsListEntry{
		AgentID: row.AgentID, Host: host, State: v.State, Models: v.Models, More: v.More,
		Source: v.Source, Version: v.Version, FetchedAt: v.FetchedAt, CheckedAt: v.CheckedAt,
		Reason: v.Reason, Sentence: v.Sentence,
	}
}

func modelsQuery(r *http.Request) (string, bool) {
	q, ok := machineQuery(r, "tool")
	if !ok {
		return "", false
	}
	tool := q.Get("tool")
	return tool, terminalproto.ValidToolV2(tool)
}

func machineModelsRow(tool string, result terminalproto.ModelsResult, now int64) store.MachineModels {
	row := store.MachineModels{Tool: tool, Reason: result.Reason, CheckedAt: now}
	switch result.Reason {
	case "":
		row.State = store.MachineModelsListed
		row.Models = result.Models
		row.More = result.More
		row.Source = modelsSource
		row.AgentVersion = result.Version
		row.FetchedAt = now
	case terminalproto.ModelsOff:
		row.State = store.MachineModelsOff
	case terminalproto.ModelsUnsupported:
		row.State = store.MachineModelsUnsupported
	default:
		row.State = store.MachineModelsFailed
	}
	return row
}

// readMachineModels makes one bounded read and records its outcome. A listed
// answer replaces the cache; any other answer keeps the last good list. The
// row is read back so the caller reports what was actually stored.
func (s *server) readMachineModels(ctx context.Context, uid int64, agent, tool string) (store.MachineModels, error) {
	callCtx, cancel := context.WithTimeout(ctx, terminalbroker.ModelsCallTimeout)
	defer cancel()
	result, err := s.terminals.Models(callCtx, uid, agent, tool)
	now := time.Now().UnixMilli()
	var row store.MachineModels
	if err != nil {
		switch {
		case errors.Is(err, terminalbroker.ErrModelsUnsupported):
			return store.MachineModels{}, errModelsUpdate
		case errors.Is(err, terminalbroker.ErrOffline):
			return store.MachineModels{}, err
		case errors.Is(err, terminalbroker.ErrModelsOff):
			row = store.MachineModels{Tool: tool, State: store.MachineModelsOff, Reason: terminalproto.ModelsOff, CheckedAt: now}
		case errors.Is(err, terminalbroker.ErrMachineTimeout), errors.Is(err, context.DeadlineExceeded):
			row = store.MachineModels{Tool: tool, State: store.MachineModelsFailed, Reason: terminalproto.ModelsTimedOut, CheckedAt: now}
		case errors.Is(err, terminalbroker.ErrConflict):
			row = store.MachineModels{Tool: tool, State: store.MachineModelsFailed, Reason: terminalproto.ModelsConflict, CheckedAt: now}
		default:
			row = store.MachineModels{Tool: tool, State: store.MachineModelsFailed, CheckedAt: now}
		}
	} else {
		row = machineModelsRow(tool, result, now)
	}
	row.AgentID = agent
	if err := s.st.UpsertMachineModelsFor(uid, row); err != nil {
		return store.MachineModels{}, err
	}
	stored, ok, err := s.st.MachineModelsFor(uid, agent, tool)
	if err != nil {
		return store.MachineModels{}, err
	}
	if !ok {
		return row, nil
	}
	return stored, nil
}

// machineModels serves the cache first. Only a machine that a first read finds
// online with the models capability is asked once, synchronously.
func (s *server) machineModels(w http.ResponseWriter, r *http.Request) {
	tool, ok := modelsQuery(r)
	agent := r.PathValue("agentId")
	if !ok || !terminalproto.ValidID(agent) {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	row, found, err := s.st.MachineModelsFor(uid, agent, tool)
	if err != nil {
		terminalError(w, err)
		return
	}
	if found {
		writeJSON(w, http.StatusOK, machineModelsView(row, s.machineHost(uid, agent)))
		return
	}
	host := s.machineHost(uid, agent)
	if host == "" {
		// Another account's id is answered as an offline machine.
		writeJSON(w, http.StatusOK, machineModelsStatus(tool, "offline", "", host))
		return
	}
	caps, err := s.st.RemoteCapabilitiesFor(uid, agent)
	if errors.Is(err, store.ErrRemoteNotFound) {
		writeJSON(w, http.StatusOK, machineModelsStatus(tool, "update", "", host))
		return
	}
	if err != nil {
		terminalError(w, err)
		return
	}
	switch {
	case caps.Capabilities.ModelsOff:
		writeJSON(w, http.StatusOK, machineModelsStatus(tool, "off", terminalproto.ModelsOff, host))
		return
	case !caps.Capabilities.Models:
		writeJSON(w, http.StatusOK, machineModelsStatus(tool, "update", "", host))
		return
	case !s.terminals.Ready(uid, agent):
		writeJSON(w, http.StatusOK, machineModelsStatus(tool, "offline", "", host))
		return
	}
	row, err = s.readMachineModels(r.Context(), uid, agent, tool)
	switch {
	case errors.Is(err, errModelsUpdate):
		writeJSON(w, http.StatusOK, machineModelsStatus(tool, "update", "", host))
	case errors.Is(err, terminalbroker.ErrOffline):
		writeJSON(w, http.StatusOK, machineModelsStatus(tool, "offline", "", host))
	case err != nil:
		terminalError(w, err)
	default:
		writeJSON(w, http.StatusOK, machineModelsView(row, host))
	}
}

// machineModelsRefresh is the person-pressed read. It is rate limited per
// account and machine and is the only path that audits.
func (s *server) machineModelsRefresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Tool string `json:"tool"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	agent := r.PathValue("agentId")
	if !terminalproto.ValidID(agent) || !terminalproto.ValidToolV2(in.Tool) {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	host := s.machineHost(uid, agent)
	if host == "" {
		writeJSON(w, http.StatusOK, machineModelsStatus(in.Tool, "offline", "", host))
		return
	}
	caps, err := s.st.RemoteCapabilitiesFor(uid, agent)
	if errors.Is(err, store.ErrRemoteNotFound) {
		writeJSON(w, http.StatusOK, machineModelsStatus(in.Tool, "update", "", host))
		return
	}
	if err != nil {
		terminalError(w, err)
		return
	}
	switch {
	case caps.Capabilities.ModelsOff:
		writeJSON(w, http.StatusOK, machineModelsStatus(in.Tool, "off", terminalproto.ModelsOff, host))
		return
	case !caps.Capabilities.Models:
		writeJSON(w, http.StatusOK, machineModelsStatus(in.Tool, "update", "", host))
		return
	case !s.terminals.Ready(uid, agent):
		writeJSON(w, http.StatusOK, machineModelsStatus(in.Tool, "offline", "", host))
		return
	}
	if !s.modelLimitState().take(strconv.FormatInt(uid, 10) + "/" + agent) {
		writeErrCode(w, http.StatusTooManyRequests, "models_rate_limited", "", sentenceSlowDown, "")
		return
	}
	row, err := s.readMachineModels(r.Context(), uid, agent, in.Tool)
	switch {
	case errors.Is(err, errModelsUpdate):
		writeJSON(w, http.StatusOK, machineModelsStatus(in.Tool, "update", "", host))
		return
	case errors.Is(err, terminalbroker.ErrOffline):
		writeJSON(w, http.StatusOK, machineModelsStatus(in.Tool, "offline", "", host))
		return
	case err != nil:
		terminalError(w, err)
		return
	}
	s.event(uid, "ui", "machine-models-read", "", map[string]any{
		"machine": agent, "tool": in.Tool, "state": row.State, "count": len(row.Models),
	})
	if row.State == store.MachineModelsFailed && row.Reason == terminalproto.ModelsConflict {
		writeErrCode(w, http.StatusConflict, "models_busy", "", sentenceModelsBusy, "")
		return
	}
	writeJSON(w, http.StatusOK, machineModelsView(row, host))
}

// modelsList reports cached rows only; it never reads a machine. Unknown
// hosts are omitted, so no other account's row can appear.
func (s *server) modelsList(w http.ResponseWriter, r *http.Request) {
	tool, ok := modelsQuery(r)
	if !ok {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	rows, err := s.st.MachineModelsAllFor(uid, tool)
	if err != nil {
		terminalError(w, err)
		return
	}
	agents, err := s.st.SyncAgentsFor(uid)
	if err != nil {
		terminalError(w, err)
		return
	}
	hosts := make(map[string]string, len(agents))
	for _, a := range agents {
		hosts[a.ID] = a.Host
	}
	out := []modelsListEntry{}
	for _, row := range rows {
		host, ok := hosts[row.AgentID]
		if !ok {
			continue
		}
		out = append(out, modelsListEntryFor(row, host))
	}
	writeJSON(w, http.StatusOK, map[string]any{"machines": out})
}
