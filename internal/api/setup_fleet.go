package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/setup"
	"github.com/eyupio/reet.sh/internal/setuprun"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A fleet setup copy: read one machine's setup, review a plan for each chosen
// machine, then apply it one machine at a time. Between the read and the
// batch the copied text is held in this controller's memory only, for at most
// setupReadTTL and one review per person; a batch moves it into its own row,
// the one place the controller stores setup text.

const setupReadTTL = 15 * time.Minute

func (s *server) registerSetupFleetRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/setup/fleet/source", s.terminal(s.setupFleetSource))
	mux.HandleFunc("POST /api/v2/setup/fleet/batches", s.terminal(s.setupBatchStart))
	mux.HandleFunc("GET /api/v2/setup/fleet/batches/active", s.terminal(s.sameOriginRead(s.setupBatchActive)))
	mux.HandleFunc("GET /api/v2/setup/fleet/batches/{batchId}", s.terminal(s.sameOriginRead(s.setupBatchGet)))
	mux.HandleFunc("POST /api/v2/setup/fleet/batches/{batchId}/members/{position}/retry", s.terminal(s.setupBatchRetry))
	mux.HandleFunc("POST /api/v2/setup/fleet/batches/{batchId}/members/{position}/skip", s.terminal(s.setupBatchSkip))
	mux.HandleFunc("POST /api/v2/setup/fleet/batches/{batchId}/members/{position}/undo", s.terminal(s.setupBatchUndo))
	mux.HandleFunc("POST /api/v2/setup/fleet/batches/{batchId}/abandon", s.terminal(s.setupBatchAbandon))
}

func (s *server) newSetupRuns() *setuprun.Orchestrator {
	return setuprun.New(setuprun.Config{
		Store:  s.st,
		Broker: s.terminals,
		Paused: s.st.Fenced,
		Settled: func(uid int64, batch store.SetupBatch) {
			s.event(uid, "ui", "setup-batch", "", setupBatchAudit(batch, "finish"))
		},
		Logf: func(format string, args ...any) { log.Printf(format, args...) },
	})
}

// ResumeSetupBatches deletes setup text past its time and continues batches
// left with pending machines by the last stop.
func (s *Server) ResumeSetupBatches(ctx context.Context) {
	if s.s == nil || s.s.setupRuns == nil {
		return
	}
	if s.s.st.Fenced() {
		log.Printf("setup batches: this controller is fenced; pending batches wait until the fence is lifted")
		return
	}
	if err := s.s.setupRuns.Resume(); err != nil {
		log.Printf("setup batches: resume: %v", err)
	}
}

// setupRead is one reviewed read, held in memory until a batch takes it.
type setupRead struct {
	uid      int64
	source   store.SetupBatchSource
	items    []terminalproto.SetupItem
	machines map[string]setupMachine
	expires  time.Time
}

type setupReads struct {
	mu    sync.Mutex
	reads map[string]setupRead
}

// put keeps one read per person: a new read replaces the last.
func (r *setupReads) put(id string, read setupRead) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reads == nil {
		r.reads = map[string]setupRead{}
	}
	for key, old := range r.reads {
		if old.uid == read.uid || time.Now().After(old.expires) {
			delete(r.reads, key)
		}
	}
	r.reads[id] = read
}

func (r *setupReads) get(uid int64, id string) (setupRead, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	read, ok := r.reads[id]
	if !ok || read.uid != uid || time.Now().After(read.expires) {
		delete(r.reads, id)
		return setupRead{}, false
	}
	return read, true
}

func (r *setupReads) drop(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.reads, id)
}

// setupMachine is one target's review. State is ready, unchanged or excluded;
// Code and Reason say why a machine is not ready.
type setupMachine struct {
	AgentID     string                        `json:"agentId"`
	Host        string                        `json:"host"`
	State       string                        `json:"state"`
	Code        string                        `json:"code,omitempty"`
	Reason      string                        `json:"reason,omitempty"`
	Widens      bool                          `json:"widens"`
	Fingerprint string                        `json:"fingerprint,omitempty"`
	Files       []terminalproto.SetupFilePlan `json:"files"`
}

type setupItemView struct {
	Agent  string `json:"agent"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Format string `json:"format"`
	Label  string `json:"label,omitempty"`
	Bytes  int    `json:"bytes"`
}

func (s *server) enrolledHosts(uid int64) (map[string]string, error) {
	agents, err := s.st.SyncAgentsFor(uid)
	if err != nil {
		return nil, err
	}
	hosts := map[string]string{}
	for _, a := range agents {
		hosts[a.ID] = a.Host
	}
	return hosts, nil
}

// setupRefusal is the plain answer for a machine's closed refusal reason.
func setupRefusal(reason, agent, name string) (int, string, string) {
	switch reason {
	case terminalproto.SetupReasonOff:
		return http.StatusConflict, "setup_terminal_off", "Terminal access is off on that machine."
	case terminalproto.SetupReasonLocked:
		return http.StatusConflict, "setup_locked", "That machine is locked."
	case terminalproto.SetupReasonNotAllowed:
		return http.StatusConflict, "setup_not_allowed", "Setup changes aren't allowed on that machine. Run reet agent allow-apply on there to allow them."
	case terminalproto.SetupReasonTooLarge:
		return http.StatusRequestEntityTooLarge, "setup_too_large", "That machine's setup is larger than reet can send in one message (about 60 KB). Shorten its instruction files and read it again."
	case terminalproto.SetupReasonCredential:
		return http.StatusUnprocessableEntity, "setup_credential", "reet won't copy " + name + " (" + agent + "): it looks like it holds a credential. Remove it on that machine and read it again."
	case terminalproto.SetupReasonNothing:
		return http.StatusConflict, "setup_nothing", "That machine has no global instruction files or settings reet can copy."
	case terminalproto.SetupReasonBusy:
		return http.StatusConflict, "setup_busy", "That machine is already reading or writing its setup. Try again in a moment."
	case terminalproto.SetupReasonChanged:
		return http.StatusConflict, "setup_changed", "A file on that machine changed after the copy, so reet won't undo it. Nothing was changed."
	case terminalproto.SetupReasonNoJournal:
		return http.StatusConflict, "setup_no_journal", "That machine no longer has the copy's journal, so it can't be undone from here."
	case terminalproto.SetupReasonTimedOut:
		return http.StatusGatewayTimeout, "setup_timed_out", "That machine took too long to answer."
	}
	return http.StatusBadGateway, "setup_failed", "That machine couldn't do it. Nothing was changed."
}

func setupCallError(w http.ResponseWriter, err error, unsupported string) {
	switch {
	case errors.Is(err, terminalbroker.ErrNotSent), errors.Is(err, terminalbroker.ErrOffline):
		writeErrCode(w, http.StatusConflict, "setup_offline", "", "That machine isn't connected. Try again when it reconnects.", "")
	case errors.Is(err, terminalbroker.ErrSetupUnsupported):
		writeErrCode(w, http.StatusConflict, "setup_unsupported", "", unsupported, "")
	case errors.Is(err, context.DeadlineExceeded):
		writeErrCode(w, http.StatusGatewayTimeout, "setup_timed_out", "", "That machine took too long to answer.", "")
	default:
		terminalError(w, err)
	}
}

// setupFleetSource reads the source machine and reviews a plan for each
// target. Nothing is written anywhere.
func (s *server) setupFleetSource(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	var in struct {
		AgentID string   `json:"agentId"`
		Targets []string `json:"targets"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeErrCode(w, http.StatusBadRequest, "setup_request_invalid", "", "Choose the machine to copy from.", "")
		return
	}
	hosts, err := s.enrolledHosts(uid)
	if err != nil {
		terminalError(w, err)
		return
	}
	host, ok := hosts[in.AgentID]
	if !ok {
		writeErrCode(w, http.StatusNotFound, "machine_not_found", "agentId", "That machine isn't enrolled.", "")
		return
	}
	var targets []string
	seen := map[string]bool{}
	for _, id := range in.Targets {
		if !seen[id] {
			seen[id] = true
			targets = append(targets, id)
		}
	}
	if len(targets) == 0 {
		for id := range hosts {
			if id != in.AgentID {
				targets = append(targets, id)
			}
		}
	}
	if len(targets) > maxBatchMachines {
		writeErrCode(w, http.StatusBadRequest, "setup_batch_empty", "targets", "Choose at most "+strconv.Itoa(maxBatchMachines)+" machines.", "")
		return
	}
	caps, err := s.st.RemoteCapabilitiesFor(uid, in.AgentID)
	if err != nil || !caps.Capabilities.SetupRead {
		writeErrCode(w, http.StatusConflict, "setup_unsupported", "", "Update reet on that machine, and turn on its terminal access, to copy its setup.", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), setuprun.ApplyTimeout)
	result, err := s.terminals.SetupFleet(ctx, uid, in.AgentID, terminalproto.SetupFleet{Op: terminalproto.SetupOpRead})
	cancel()
	readAt := time.Now().UnixMilli()
	if err != nil {
		setupCallError(w, err, "Update reet on that machine to copy its setup.")
		return
	}
	if result.State != terminalproto.SetupOK {
		status, code, sentence := setupRefusal(result.Reason, result.Agent, result.Name)
		s.event(uid, "ui", "setup-read", "", map[string]any{"host": host, "state": result.State, "reason": result.Reason, "agent": result.Agent, "name": result.Name})
		writeErrCode(w, status, code, "", sentence, "")
		return
	}
	// The controller checks again: a credential anywhere refuses the whole
	// read, naming the file but never the text.
	if err := setup.CheckFleetItems(result.Items); err != nil {
		var refusal *setup.FleetRefusal
		reason, agent, name := terminalproto.SetupReasonInvalid, "", ""
		if errors.As(err, &refusal) {
			reason, agent, name = refusal.Reason, refusal.Agent, refusal.Name
		}
		s.event(uid, "ui", "setup-read", "", map[string]any{"host": host, "state": "refused", "reason": reason, "agent": agent, "name": name})
		status, code, sentence := setupRefusal(reason, agent, name)
		if reason == terminalproto.SetupReasonInvalid {
			status, code, sentence = http.StatusBadGateway, "setup_read_invalid", "That machine sent setup reet can't copy."
		}
		writeErrCode(w, status, code, "", sentence, "")
		return
	}

	read := setupRead{
		uid: uid, source: store.SetupBatchSource{AgentID: in.AgentID, Host: host, ReadAt: readAt},
		items: result.Items, machines: map[string]setupMachine{}, expires: time.Now().Add(setupReadTTL),
	}
	machines := make([]setupMachine, len(targets))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 8)
	for i, target := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			machines[i] = s.reviewSetupTarget(r.Context(), uid, hosts, in.AgentID, target, result.Items)
		}()
	}
	wg.Wait()
	for _, m := range machines {
		read.machines[m.AgentID] = m
	}
	readID := terminalbroker.ID()
	s.setupReads.put(readID, read)
	items := make([]setupItemView, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, setupItemView{Agent: item.Agent, Kind: item.Kind, Name: item.Name, Format: item.Format, Label: item.Label, Bytes: len(item.Content)})
	}
	s.event(uid, "ui", "setup-read", "", map[string]any{"host": host, "state": "ok", "items": len(items), "machines": len(machines)})
	writeJSON(w, http.StatusOK, map[string]any{
		"readId":    readID,
		"source":    map[string]any{"agentId": in.AgentID, "host": host, "readAt": readAt},
		"expiresAt": read.expires.UnixMilli(),
		"items":     items,
		"machines":  machines,
	})
}

// reviewSetupTarget asks one target for its plan, or says plainly why it is
// left out.
func (s *server) reviewSetupTarget(ctx context.Context, uid int64, hosts map[string]string, sourceID, agentID string, items []terminalproto.SetupItem) setupMachine {
	m := setupMachine{AgentID: agentID, Host: hosts[agentID], Files: []terminalproto.SetupFilePlan{}}
	exclude := func(code, reason string) setupMachine {
		m.State, m.Code, m.Reason = "excluded", code, reason
		return m
	}
	if agentID == sourceID {
		return exclude("same_machine", "That is the machine the setup was read from.")
	}
	if m.Host == "" {
		return exclude("not_enrolled", "That machine isn't enrolled.")
	}
	caps, err := s.st.RemoteCapabilitiesFor(uid, agentID)
	switch {
	case err != nil:
		return exclude("unsupported", "Update reet on that machine to copy setup to it.")
	case caps.Capabilities.Locked:
		return exclude("locked", "That machine is locked.")
	case !caps.Capabilities.TerminalEnabled:
		return exclude("terminal_off", "Terminal access is off on that machine.")
	case !caps.Capabilities.SetupApply:
		return exclude("unsupported", "Update reet on that machine to copy setup to it.")
	}
	callCtx, cancel := context.WithTimeout(ctx, terminalbroker.SetupCallTimeout)
	defer cancel()
	result, err := s.terminals.SetupFleet(callCtx, uid, agentID, terminalproto.SetupFleet{Op: terminalproto.SetupOpPlan, Items: items})
	switch {
	case errors.Is(err, terminalbroker.ErrNotSent), errors.Is(err, terminalbroker.ErrOffline):
		return exclude("offline", "That machine isn't connected.")
	case errors.Is(err, terminalbroker.ErrSetupUnsupported):
		return exclude("unsupported", "Update reet on that machine to copy setup to it.")
	case errors.Is(err, terminalbroker.ErrLocked):
		return exclude("locked", "That machine is locked.")
	case err != nil:
		return exclude("plan_failed", "reet couldn't get a plan from that machine.")
	case result.State != terminalproto.SetupOK:
		_, code, sentence := setupRefusal(result.Reason, result.Agent, result.Name)
		switch result.Reason {
		case terminalproto.SetupReasonOff, terminalproto.SetupReasonLocked, terminalproto.SetupReasonNotAllowed, terminalproto.SetupReasonBusy:
			return exclude(code[len("setup_"):], sentence)
		}
		return exclude("plan_failed", "reet couldn't get a plan from that machine.")
	}
	m.Files, m.Fingerprint, m.State = result.Files, result.Fingerprint, "unchanged"
	for _, f := range result.Files {
		m.Widens = m.Widens || len(f.Widens) > 0
		if f.State == "create" || f.State == "merge" {
			m.State = "ready"
		}
	}
	if m.State == "unchanged" {
		m.Code, m.Reason = "unchanged", "Nothing to change on that machine."
	}
	return m
}

// setupBatchStart turns a review into a batch: the machines the person chose,
// in order, each with the plan it reviewed. A batch that would let agents do
// more without asking on any machine needs confirm "copy".
func (s *server) setupBatchStart(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	var in struct {
		RequestID string   `json:"requestId"`
		ReadID    string   `json:"readId"`
		Agents    []string `json:"agents"`
		Confirm   string   `json:"confirm"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeErrCode(w, http.StatusBadRequest, "setup_request_invalid", "", "Choose the machines to copy to.", "")
		return
	}
	if !terminalproto.ValidID(in.RequestID) {
		writeErrCode(w, http.StatusUnprocessableEntity, "request_id_required", "requestId", "Starting a batch needs a request id: 16 to 128 letters, digits, hyphens or underscores.", "")
		return
	}
	if existing, ok, err := s.st.SetupBatchForRequest(uid, in.RequestID); err != nil {
		terminalError(w, err)
		return
	} else if ok {
		s.setupRuns.Start(uid, existing.ID)
		writeJSON(w, http.StatusOK, existing)
		return
	}
	read, ok := s.setupReads.get(uid, in.ReadID)
	if !ok {
		writeErrCode(w, http.StatusGone, "setup_read_expired", "readId", "That review has expired. Read the source machine again.", "")
		return
	}
	if len(in.Agents) == 0 || len(in.Agents) > maxBatchMachines {
		writeErrCode(w, http.StatusBadRequest, "setup_batch_empty", "agents", "Choose between one and "+strconv.Itoa(maxBatchMachines)+" machines.", "")
		return
	}
	members := make([]store.SetupBatchMemberInput, 0, len(in.Agents))
	seen := map[string]bool{}
	onlySource, widens := true, false
	for _, agentID := range in.Agents {
		if seen[agentID] {
			writeErrCode(w, http.StatusBadRequest, "setup_batch_duplicate", "agents", "A machine appears more than once in that batch.", "")
			return
		}
		seen[agentID] = true
		onlySource = onlySource && agentID == read.source.AgentID
		member := store.SetupBatchMemberInput{Position: len(members), AgentID: agentID}
		reviewed, ok := read.machines[agentID]
		if agentID == read.source.AgentID {
			reviewed, ok = setupMachine{AgentID: agentID, Host: read.source.Host, State: "excluded", Code: "same_machine", Reason: "That is the machine the setup was read from."}, true
		}
		if !ok {
			member.Excluded, member.Code, member.Reason = true, "not_reviewed", "That machine wasn't in the review. Read the source machine again to include it."
			members = append(members, member)
			continue
		}
		member.Host, member.Fingerprint, member.Widens = reviewed.Host, reviewed.Fingerprint, reviewed.Widens
		for _, f := range reviewed.Files {
			member.Files = append(member.Files, store.SetupBatchFile{Agent: f.Agent, Kind: f.Kind, Name: f.Name, State: f.State, Reason: f.Reason, Fields: f.Fields, Widens: f.Widens})
		}
		if reviewed.State != "ready" {
			member.Excluded, member.Code, member.Reason = true, reviewed.Code, reviewed.Reason
		} else {
			widens = widens || reviewed.Widens
		}
		members = append(members, member)
	}
	if onlySource {
		writeErrCode(w, http.StatusConflict, "setup_batch_source_only", "agents", "Choose a machine other than the one the setup was read from.", "")
		return
	}
	if widens && in.Confirm != "copy" {
		writeErrCode(w, http.StatusUnprocessableEntity, "setup_confirm_required", "confirm", "This copy lets agents do more without asking on at least one machine. Type copy to confirm.", "")
		return
	}
	items, err := json.Marshal(read.items)
	if err != nil {
		terminalError(w, err)
		return
	}
	batch, created, err := s.st.CreateSetupBatchFor(uid, in.RequestID, read.source, items, members)
	switch {
	case errors.Is(err, store.ErrSetupBatchInProgress):
		writeErrCode(w, http.StatusConflict, "setup_batch_in_progress", "", "Another setup copy is still waiting or running. Finish or abandon it first.", "")
		return
	case errors.Is(err, store.ErrSetupBatchNoMembers):
		writeErrCode(w, http.StatusConflict, "setup_batch_empty", "", "None of those machines has anything to copy right now.", "")
		return
	case err != nil:
		terminalError(w, err)
		return
	}
	if created {
		// The text now lives in the batch row only.
		s.setupReads.drop(in.ReadID)
		s.event(uid, "ui", "setup-batch", "", setupBatchAudit(batch, "start"))
	}
	s.setupRuns.Start(uid, batch.ID)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, batch)
}

func (s *server) setupBatchActive(w http.ResponseWriter, r *http.Request) {
	batch, ok, err := s.st.ActiveSetupBatchFor(s.uid(r))
	if err != nil {
		terminalError(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

func (s *server) setupBatchGet(w http.ResponseWriter, r *http.Request) {
	batch, err := s.st.SetupBatchFor(s.uid(r), r.PathValue("batchId"))
	if err != nil {
		setupBatchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

func (s *server) setupBatchRetry(w http.ResponseWriter, r *http.Request) {
	s.setupBatchMove(w, r, "setup-batch-retry", s.setupRuns.Retry)
}

func (s *server) setupBatchSkip(w http.ResponseWriter, r *http.Request) {
	s.setupBatchMove(w, r, "setup-batch-skip", s.setupRuns.Skip)
}

func (s *server) setupBatchMove(w http.ResponseWriter, r *http.Request, event string, move func(int64, string, int) error) {
	uid, batchID := s.uid(r), r.PathValue("batchId")
	position, ok := batchPosition(w, r)
	if !ok {
		return
	}
	if err := move(uid, batchID, position); err != nil {
		setupBatchError(w, err)
		return
	}
	s.setupBatchAnswer(w, uid, batchID, event, map[string]any{"batchId": batchID, "position": position})
}

func (s *server) setupBatchAbandon(w http.ResponseWriter, r *http.Request) {
	uid, batchID := s.uid(r), r.PathValue("batchId")
	if _, err := s.setupRuns.Abandon(uid, batchID); err != nil {
		setupBatchError(w, err)
		return
	}
	s.setupBatchAnswer(w, uid, batchID, "setup-batch-abandon", map[string]any{"batchId": batchID})
}

// setupBatchUndo asks one machine to undo its applied copy from its journal.
// The machine refuses when a file changed after the copy.
func (s *server) setupBatchUndo(w http.ResponseWriter, r *http.Request) {
	uid, batchID := s.uid(r), r.PathValue("batchId")
	position, ok := batchPosition(w, r)
	if !ok {
		return
	}
	batch, err := s.st.SetupBatchFor(uid, batchID)
	if err != nil {
		setupBatchError(w, err)
		return
	}
	var member *store.SetupBatchMember
	for i := range batch.Members {
		if batch.Members[i].Position == position {
			member = &batch.Members[i]
		}
	}
	if member == nil {
		setupBatchError(w, store.ErrSetupBatchMemberNotFound)
		return
	}
	if member.UndoneAt != 0 {
		writeErrCode(w, http.StatusConflict, "setup_undo_done", "", "That copy was already undone.", "")
		return
	}
	if member.State != store.SoftwareMemberApplied || member.Journal == "" {
		writeErrCode(w, http.StatusConflict, "setup_undo_unavailable", "", "That machine has no applied copy to undo.", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), setuprun.ApplyTimeout)
	defer cancel()
	result, err := s.terminals.SetupFleet(ctx, uid, member.AgentID, terminalproto.SetupFleet{Op: terminalproto.SetupOpRollback, Journal: member.Journal})
	detail := map[string]any{"batchId": batchID, "position": position, "host": member.Host, "journal": member.Journal}
	if err != nil {
		detail["state"] = "error"
		s.event(uid, "ui", "setup-batch-undo", "", detail)
		setupCallError(w, err, "Update reet on that machine to undo a copy.")
		return
	}
	detail["state"], detail["reason"] = result.State, result.Reason
	s.event(uid, "ui", "setup-batch-undo", "", detail)
	if result.State != terminalproto.SetupOK {
		status, code, sentence := setupRefusal(result.Reason, result.Agent, result.Name)
		writeErrCode(w, status, code, "", sentence, "")
		return
	}
	if err := s.st.SetSetupBatchMemberUndone(uid, batchID, position); err != nil && !errors.Is(err, store.ErrSetupBatchNotUndoable) {
		terminalError(w, err)
		return
	}
	batch, err = s.st.SetupBatchFor(uid, batchID)
	if err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

func (s *server) setupBatchAnswer(w http.ResponseWriter, uid int64, batchID, event string, detail map[string]any) {
	s.event(uid, "ui", event, "", detail)
	batch, err := s.st.SetupBatchFor(uid, batchID)
	if err != nil {
		terminalError(w, err)
		return
	}
	if batch.State == store.SoftwareBatchDone || batch.State == store.SoftwareBatchAbandoned {
		s.event(uid, "ui", "setup-batch", "", setupBatchAudit(batch, "finish"))
	}
	writeJSON(w, http.StatusOK, batch)
}

func setupBatchError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrSetupBatchNotFound), errors.Is(err, store.ErrSetupBatchMemberNotFound):
		writeErrCode(w, http.StatusNotFound, "setup_batch_not_found", "", "That setup copy doesn't exist.", "")
	case errors.Is(err, store.ErrSetupBatchMemberNotFailed):
		writeErrCode(w, http.StatusConflict, "setup_batch_member_not_failed", "", "That machine isn't waiting on a failure.", "")
	case errors.Is(err, store.ErrSetupBatchFinished):
		writeErrCode(w, http.StatusConflict, "setup_batch_finished", "", "That setup copy has already finished.", "")
	case errors.Is(err, store.ErrSetupBatchRunning):
		writeErrCode(w, http.StatusConflict, "setup_batch_running", "", "A machine in that copy is still applying. Try again when it finishes.", "")
	default:
		terminalError(w, err)
	}
}

// setupBatchAudit names machines, files and fields, never setup text.
func setupBatchAudit(batch store.SetupBatch, phase string) map[string]any {
	members := make([]map[string]any, 0, len(batch.Members))
	for _, m := range batch.Members {
		files := make([]string, 0, len(m.Files))
		for _, f := range m.Files {
			files = append(files, f.Agent+"/"+f.Name+":"+f.State)
		}
		members = append(members, map[string]any{
			"position": m.Position, "host": m.Host, "state": m.State, "code": m.Code,
			"widens": m.Widens, "files": files, "journal": m.Journal,
		})
	}
	return map[string]any{
		"batchId": batch.ID, "phase": phase, "state": batch.State, "source": batch.SourceHost,
		"readAt": batch.ReadAt, "widens": batch.Widens, "members": members,
	}
}
