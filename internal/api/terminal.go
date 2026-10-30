package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (s *server) registerTerminalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/terminal/commands", s.terminalAccess(s.terminalCommands, true))
	mux.HandleFunc("POST /api/v2/terminal/commands", s.terminalAccess(s.terminalCommandSave, true))
	mux.HandleFunc("PATCH /api/v2/terminal/commands/{id}", s.terminalAccess(s.terminalCommandSave, true))
	mux.HandleFunc("DELETE /api/v2/terminal/commands/{id}", s.terminalAccess(s.terminalCommandDelete, true))
	mux.HandleFunc("GET /api/v2/terminal/status", s.terminal(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.terminalStatus())
	}))
	mux.HandleFunc("GET /api/v2/terminal/machines", s.terminal(s.terminalMachines))
	mux.HandleFunc("GET /api/v2/machines/{agentId}/yolo", s.terminalAccess(s.terminalMachineYolo, true))
	mux.HandleFunc("POST /api/v2/machines/{agentId}/uploads", s.terminal(s.terminalLaunchUpload))
	mux.HandleFunc("GET /api/v2/terminal/sessions", s.terminal(s.terminalSessions))
	mux.HandleFunc("POST /api/v2/terminal/sessions", s.terminal(s.terminalCreate))
	mux.HandleFunc("PATCH /api/v2/terminal/sessions/{id}", s.terminal(s.terminalRename))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/end", s.terminal(s.terminalEnd))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/archive", s.terminalAccess(s.terminalArchive, true))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/seen", s.terminalAccess(s.terminalSessionSeen, true))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/attachments", s.terminal(s.terminalAttachment))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/uploads", s.terminal(s.terminalUpload))
	mux.HandleFunc("GET /api/v2/terminal/sessions/{id}/history", s.terminal(s.terminalHistory))
	mux.HandleFunc("GET /api/v2/terminal/sessions/{id}/command-history", s.terminal(s.terminalCommandHistory))
	mux.HandleFunc("GET /api/v2/terminal/socket", s.terminal(s.terminalSocket))
	mux.HandleFunc("POST /api/v2/terminal/worktrees/status", s.terminal(s.terminalWorktreeStatus))
	mux.HandleFunc("POST /api/v2/terminal/worktrees/remove", s.terminal(s.terminalWorktreeRemove))
	s.registerTerminalChatRoutes(mux)
}

// Deliberately accounts-only in v1. Single-token, anonymous, API and MCP
// principals cannot inherit interactive shell access from the default owner.
// No auth middleware bypass is required, even for the exact socket handler.
func (s *server) terminal(next http.HandlerFunc) http.HandlerFunc {
	return s.terminalAccess(next, false)
}

// Only controller-local metadata handlers opt out of execution availability. Account,
// role, cookie-only and canonical-origin checks still apply to every route.
func (s *server) terminalAccess(next http.HandlerFunc, allowDisabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if s.authMode != AuthAccounts || requestToken(r) != "" {
			writeErr(w, 403, "terminal access requires an account browser session")
			return
		}
		u, ok := s.sessionUser(r)
		if !ok || u.Role == "viewer" {
			writeErr(w, 403, "terminal access requires an active browser session")
			return
		}
		// Always reject URL credentials, including an empty query on the stream.
		if r.URL.Query().Has("t") || r.URL.Query().Has("ticket") || r.URL.Path == "/api/v2/terminal/socket" && (r.URL.RawQuery != "" || r.URL.ForceQuery) {
			writeErr(w, 400, "terminal credentials must not appear in URLs")
			return
		}
		metadata := r.Method == http.MethodGet && (r.URL.Path == "/api/v2/terminal/status" || r.URL.Path == "/api/v2/terminal/machines" || r.URL.Path == "/api/v2/terminal/sessions")
		if !metadata && !allowDisabled && !s.terminalEnabled() {
			writeErr(w, 403, "remote terminals are disabled by the server; monitoring and local shells remain available")
			return
		}
		if r.Method != http.MethodGet || r.URL.Path == "/api/v2/terminal/socket" {
			if !s.terminalOrigin(r) {
				writeErr(w, 403, "terminal origin is not allowed")
				return
			}
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, u)))
	}
}
func (s *server) terminalConfigured() bool {
	enabled := s.remoteTerminals
	if override := s.terminalOverride.Load(); override != nil {
		enabled = *override
	}
	return enabled
}
func (s *server) terminalEnabled() bool {
	return s.terminalConfigured() && s.authMode == AuthAccounts && s.terminalOriginConfigured()
}
func (s *server) terminalOriginConfigured() bool { return validTerminalOrigin(s.publicOrigin()) }
func validTerminalOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback())
}
func (s *server) terminalUnavailableReason() string {
	return s.terminalStatus().Reason
}

type terminalControllerStatus struct {
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	Reason     string `json:"reason"`
}

func (s *server) terminalStatus() terminalControllerStatus {
	status := terminalControllerStatus{Configured: s.terminalConfigured()}
	switch {
	case !status.Configured:
		status.Reason = "disabled"
	case s.authMode != AuthAccounts:
		status.Reason = "accounts_required"
	case !s.terminalOriginConfigured():
		status.Reason = "https_required"
	default:
		status.Available = true
	}
	return status
}
func (s *server) terminalOrigin(r *http.Request) bool {
	u, err := url.Parse(s.publicOrigin())
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "https" && u.Scheme != "http" || u.Path != "" && u.Path != "/" {
		return false
	}
	return r.Header.Get("Origin") == u.Scheme+"://"+u.Host
}
func terminalJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, terminalproto.MaxControlBytes)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		writeErr(w, 400, "invalid terminal request")
		return false
	}
	return true
}
func terminalError(w http.ResponseWriter, err error) {
	status := 503
	message := "terminal machine is reconnecting; retry after it reconnects"
	switch {
	case errors.Is(err, store.ErrRemoteNotFound), errors.Is(err, terminalbroker.ErrDenied):
		status = 403
		message = "terminal access denied"
	case errors.Is(err, store.ErrTerminalCapacity):
		status = 409
		message = "terminal session limit reached"
	case errors.Is(err, terminalbroker.ErrConflict), errors.Is(err, store.ErrTerminalConflict):
		status = 409
		message = "terminal operation conflicts with current state"
	case errors.Is(err, terminalproto.ErrInvalid):
		status = 400
		message = "invalid terminal request"
	}
	writeErr(w, status, message)
}
func (s *server) terminalMachines(w http.ResponseWriter, r *http.Request) {
	agents, err := s.st.SyncAgentsFor(s.uid(r))
	if err != nil {
		terminalError(w, err)
		return
	}
	out := []map[string]any{}
	for _, a := range agents {
		item := map[string]any{"agentId": a.ID, "host": a.Host, "ready": s.terminalEnabled() && s.terminals.Ready(s.uid(r), a.ID), "serverEnabled": s.terminalEnabled()}
		if c, e := s.st.RemoteCapabilitiesFor(s.uid(r), a.ID); e == nil {
			item["capabilities"] = c.Capabilities
		}
		out = append(out, item)
	}
	writeJSON(w, 200, map[string]any{"machines": out})
}

// terminalMachineYolo is the confirmation dialog's gate: whether the machine
// currently reports that it accepts YOLO, and when that was observed.
func (s *server) terminalMachineYolo(w http.ResponseWriter, r *http.Request) {
	c, err := s.st.RemoteCapabilitiesFor(s.uid(r), r.PathValue("agentId"))
	if errors.Is(err, store.ErrRemoteNotFound) {
		writeJSON(w, 200, map[string]any{"accepts": false, "checkedAt": int64(0)})
		return
	}
	if err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"accepts": c.Capabilities.Yolo, "checkedAt": c.ObservedAt})
}

func (s *server) terminalSessions(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	sessions, err := s.st.TerminalSessionsFor(uid)
	if err != nil {
		terminalError(w, err)
		return
	}
	for i := range sessions {
		s.terminals.ApplyChatAttention(uid, &sessions[i])
	}
	writeJSON(w, 200, map[string]any{"sessions": sessions})
}
func (s *server) terminalCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AgentID   string `json:"agentId"`
		RequestID string `json:"requestId"`
		Title     string `json:"title"`
		Cols      int    `json:"cols"`
		Rows      int    `json:"rows"`
		Tool      string `json:"tool"`
		Base      string `json:"base"`
		Effort    string `json:"effort"`
		Yolo      bool   `json:"yolo"`
		Resume    bool   `json:"resume"`
		Model     string `json:"model"`
		Dir       string `json:"dir"`
		Prompt    string `json:"prompt"`
		// Worktree is a run token chosen by the caller (the Projects run
		// derives it), never generated here: a retried request must carry the
		// same token so the machine adopts the worktree it already made.
		Worktree string `json:"worktree"`
		Project  string `json:"project"`
		// ProjectID is the folder project the session belongs to, checked
		// here and recorded by the controller only; the machine never sees it.
		ProjectID string `json:"projectId"`
		// Structured starts an OpenCode chat; trust confirms the repository
		// plugin warning. Both are controller-side choices and are never
		// inferred by the machine.
		Structured bool   `json:"structured"`
		Trust      bool   `json:"trust"`
		ResumeACP  string `json:"resumeAcp"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	create := terminalproto.Create{Title: in.Title, Cols: in.Cols, Rows: in.Rows, Tool: in.Tool, Model: in.Model, Dir: in.Dir, Prompt: in.Prompt, Worktree: in.Worktree, Project: in.Project, ProjectID: in.ProjectID, Base: in.Base, Effort: in.Effort, Yolo: in.Yolo, Resume: in.Resume, Structured: in.Structured, Trust: in.Trust, ResumeACP: in.ResumeACP}
	uid := s.uid(r)
	// The project is checked before any machine is asked, so an unknown
	// project is refused the same way whether the machine is online or not.
	// The store checks it again when it reserves the session. A resend of a
	// create already recorded (its reply was lost) skips this check: the
	// store compares its project with the recorded one and answers the
	// session that started, even if the project was removed since.
	if in.ProjectID != "" && !s.terminalCreateRecorded(uid, in.AgentID, in.RequestID) {
		if err := s.st.TerminalProjectCheckFor(uid, in.AgentID, in.ProjectID, in.Dir); err != nil {
			s.terminalCreateError(w, err, uid, in.AgentID, in.Tool)
			return
		}
	}
	session, started, err := s.terminals.CreateSession(r.Context(), uid, in.AgentID, in.RequestID, create)
	if err != nil {
		s.terminalCreateError(w, err, uid, in.AgentID, in.Tool)
		return
	}
	if started && create.Tool != "" {
		s.auditTerminalLaunch(uid, in.AgentID, create, session)
	}
	writeJSON(w, 200, session)
}

// terminalCreateRecorded reports whether uid already has a create with this
// request id on agent.
func (s *server) terminalCreateRecorded(uid int64, agent, requestID string) bool {
	if requestID == "" {
		return false
	}
	_, err := s.st.TerminalOperationFor(uid, agent, "create", requestID)
	return err == nil
}

// terminalCreateError maps launch refusals to stable codes and plain
// sentences; every other error keeps its existing terminal mapping.
func (s *server) terminalCreateError(w http.ResponseWriter, err error, uid int64, agent, tool string) {
	var trust *terminalbroker.ACPTrustError
	if errors.As(err, &trust) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "This folder has OpenCode plugins. Start only in a folder you trust.", "code": "acp_trust_required", "detail": trust.Detail})
		return
	}
	var acpUnavailable *terminalbroker.ACPUnavailableError
	if errors.As(err, &acpUnavailable) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": acpUnavailable.Detail, "code": "acp_unavailable", "detail": acpUnavailable.Detail})
		return
	}
	switch {
	case errors.Is(err, store.ErrFolderProjectNotFound):
		// The same reply for an unknown id and another account's id.
		writeErrCode(w, http.StatusNotFound, "project_not_found", "projectId", "That project isn't on this account.", "")
	case errors.Is(err, store.ErrProjectMachineMismatch):
		writeErrCode(w, http.StatusUnprocessableEntity, "project_machine_mismatch", "projectId", "That project is on another machine. Start the session on its machine.", "")
	case errors.Is(err, store.ErrProjectFolderMismatch):
		writeErrCode(w, http.StatusUnprocessableEntity, "project_folder_mismatch", "dir", "That isn't the project's folder. Start the session in the project's folder.", "")
	case errors.Is(err, terminalbroker.ErrResumeUnsupported):
		writeErrCode(w, http.StatusConflict, "resume_unsupported", "", "Update reet on that machine to continue sessions.", "")
	case errors.Is(err, terminalbroker.ErrLaunchOptionUnsupported) && s.terminalAgentsV2(uid, agent):
		// Updating a machine that already has agents-v2 would not help.
		writeErrCode(w, http.StatusConflict, "launch_option_unsupported", "", "That agent doesn't take that option.", "")
	case errors.Is(err, terminalbroker.ErrLaunchOptionUnsupported):
		writeErrCode(w, http.StatusConflict, "launch_option_unsupported", "", "Update reet on that machine to use that option.", "")
	case errors.Is(err, terminalbroker.ErrToolUnavailable):
		writeErrCode(w, http.StatusUnprocessableEntity, "tool_unavailable", "tool", "reet doesn't start this agent yet.", "")
	case errors.Is(err, terminalbroker.ErrToolNotInstalled):
		writeErrCode(w, http.StatusUnprocessableEntity, "tool_not_installed", "tool", "That agent isn't installed on that machine.", "")
	case errors.Is(err, terminalbroker.ErrYoloNotAllowed):
		writeErrCode(w, http.StatusForbidden, "yolo_not_allowed", "", "An owner or admin can allow YOLO for this account in Settings.", "")
	case errors.Is(err, terminalbroker.ErrYoloRefused):
		writeErrCode(w, http.StatusConflict, "yolo_refused", "", "That machine doesn't accept YOLO sessions.", "")
	case errors.Is(err, terminalbroker.ErrPromptUnsupported):
		writeErrCode(w, http.StatusUnprocessableEntity, "prompt_unsupported", "prompt", "Write the opening prompt as at least two words.", "")
	case errors.Is(err, terminalbroker.ErrWorktreeUnsupported):
		writeErrCode(w, http.StatusConflict, "worktree_unsupported", "", "Update reet on that machine to start agents with worktrees.", "")
	case errors.Is(err, terminalbroker.ErrWorktreeRefused):
		var refusal *terminalbroker.WorktreeError
		if errors.As(err, &refusal) && refusal.Reason == terminalproto.WorktreeBaseMissing {
			writeErrCode(w, http.StatusUnprocessableEntity, "base_missing", "base", "That branch isn't in the repository on "+s.machineHost(uid, agent)+".", "")
			return
		}
		writeWorktreeRefusal(w, err, worktreeCreating)
	case errors.Is(err, terminalbroker.ErrWorktreeMismatch):
		writeErrCode(w, http.StatusServiceUnavailable, "worktree_mismatch", "", "That machine's answer did not match the request. Check again.", "")
	case errors.Is(err, terminalbroker.ErrLaunchUnsupported):
		writeErrCode(w, http.StatusConflict, "launch_unsupported", "", "Update reet on that machine to start agents.", "")
	case errors.Is(err, terminalbroker.ErrLaunchFailed) && !s.terminalToolInstalled(uid, agent, tool):
		writeErrCode(w, http.StatusUnprocessableEntity, "tool_not_installed", "tool", "That agent isn't installed on that machine.", "")
	case errors.Is(err, terminalbroker.ErrLaunchFailed):
		writeErrCode(w, http.StatusUnprocessableEntity, "folder_unavailable", "dir", "That folder isn't available on that machine.", "")
	case errors.Is(err, terminalbroker.ErrShellDirUnsupported):
		writeErrCode(w, http.StatusConflict, "shell_dir_unsupported", "", "Update reet on that machine to open a terminal in that folder.", "")
	case errors.Is(err, terminalbroker.ErrACPHistoryUnsupported):
		writeErrCode(w, http.StatusConflict, "acp_history_unsupported", "", "Update reet on that machine to reopen reet sessions with their history.", "")
	case errors.Is(err, terminalbroker.ErrACPResumeNotFound):
		writeErrCode(w, http.StatusNotFound, "acp_resume_not_found", "", "That reet session's history is no longer available on that machine.", "")
	case errors.Is(err, terminalbroker.ErrACPUnsupported):
		writeErrCode(w, http.StatusConflict, "acp_unavailable", "", "Update reet on that machine to use OpenCode chats.", "")
	case errors.Is(err, terminalbroker.ErrACPVersion):
		writeErrCode(w, http.StatusConflict, "acp_version", "", "The reet TUI needs OpenCode 1.19.5 or newer on that machine.", "")
	default:
		terminalError(w, err)
	}
}

// terminalAgentsV2 reports whether the machine's stored capabilities
// negotiated agents-v2, so a refused option is not blamed on its version.
func (s *server) terminalAgentsV2(uid int64, agent string) bool {
	caps, err := s.st.RemoteCapabilitiesFor(uid, agent)
	return err == nil && caps.Capabilities.AgentsV2
}

// terminalToolInstalled reads the machine's reported tools: the connector's
// launch_failed does not say whether the tool or the folder was refused.
func (s *server) terminalToolInstalled(uid int64, agent, tool string) bool {
	caps, err := s.st.RemoteCapabilitiesFor(uid, agent)
	return err == nil && slices.Contains(caps.Capabilities.Tools, tool)
}

// auditTerminalLaunch records who started which agent where. The opening
// prompt is deliberately absent: it is user text kept only with the session.
func (s *server) auditTerminalLaunch(uid int64, agent string, in terminalproto.Create, session store.TerminalSession) {
	host := s.machineHost(uid, agent)
	detail := map[string]string{"machine": agent, "host": host, "tool": in.Tool, "model": in.Model, "dir": in.Dir, "effort": in.Effort, "base": in.Base, "yolo": strconv.FormatBool(in.Yolo), "resume": strconv.FormatBool(in.Resume), "structured": strconv.FormatBool(in.Structured)}
	if in.ProjectID != "" {
		detail["projectId"] = in.ProjectID
	}
	// For a worktree launch: the run token, project and branch, and the
	// folder the machine reported making (absent only if that report was lost).
	if in.Worktree != "" {
		detail["worktreeRun"] = in.Worktree
		detail["project"] = in.Project
		detail["branch"] = worktreeBranch(in.Project, in.Worktree)
		if session.WorktreePath != "" {
			detail["worktree"] = session.WorktreePath
		}
	}
	s.event(uid, "ui", "terminal-launch", "", detail)
}
func (s *server) machineHost(uid int64, agent string) string {
	if agents, err := s.st.SyncAgentsFor(uid); err == nil {
		for _, a := range agents {
			if a.ID == agent {
				return a.Host
			}
		}
	}
	return ""
}
func (s *server) terminalRename(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title string `json:"title"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	if err := s.st.RenameTerminalFor(s.uid(r), r.PathValue("id"), in.Title); err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *server) terminalEnd(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RequestID     string `json:"requestId"`
		RemoveHistory bool   `json:"removeHistory"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	session, err := s.terminals.End(r.Context(), s.uid(r), r.PathValue("id"), in.RequestID, in.RemoveHistory)
	if err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, 200, session)
}
func (s *server) terminalArchive(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !terminalJSON(w, r, &in) {
		return
	}
	if err := s.st.ArchiveTerminalFor(s.uid(r), r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrTerminalConflict) {
			writeErr(w, 409, "Only ended, exited or lost sessions can be removed from history.")
			return
		}
		terminalError(w, err)
		return
	}
	s.terminals.CloseChat(s.uid(r), r.PathValue("id"))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) terminalAttachment(w http.ResponseWriter, r *http.Request) {
	var in terminalbroker.Options
	if !terminalJSON(w, r, &in) {
		return
	}
	cookie, _ := r.Cookie(sessionCookie)
	ticket, err := s.terminals.Ticket(s.uid(r), auth.HashToken(cookie.Value), r.PathValue("id"), in)
	if err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ticket": ticket, "expiresIn": 30})
}
func (s *server) terminalHistory(w http.ResponseWriter, r *http.Request) {
	lines := 200
	if raw := r.URL.Query().Get("lines"); raw != "" {
		var err error
		lines, err = strconv.Atoi(raw)
		if err != nil {
			terminalError(w, terminalproto.ErrInvalid)
			return
		}
	}
	data, err := s.terminals.History(r.Context(), s.uid(r), r.PathValue("id"), lines)
	if err != nil {
		terminalError(w, err)
		return
	}
	if u, ok := s.sessionUser(r); !ok || u.ID != s.uid(r) || u.Role == "viewer" {
		terminalError(w, terminalbroker.ErrDenied)
		return
	}
	if _, err := s.st.TerminalSessionFor(s.uid(r), r.PathValue("id")); err != nil {
		terminalError(w, terminalbroker.ErrDenied)
		return
	}
	serveTerminalHistory(w, r, data)
}

// A snapshot is inert downloadable text even when a shell prints HTML. The
// standard file-serving path also avoids arbitrary response-writer wrappers
// treating a network-derived terminal buffer as an HTML response.
func serveTerminalHistory(w http.ResponseWriter, r *http.Request, data []byte) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="terminal-history.txt"`)
	w.Header().Set("X-Reet-History-Snapshot", "true")
	http.ServeContent(w, r, "terminal-history.txt", time.Time{}, bytes.NewReader(data))
}
func (s *server) terminalSocket(w http.ResponseWriter, r *http.Request) {
	// The wrapper already checked the exact scheme and origin; the library
	// also checks the explicit canonical host when running behind a proxy.
	origin, _ := url.Parse(s.publicOrigin())
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{origin.Host}})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(terminalproto.MaxBinaryBytes)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	kind, data, err := c.Read(ctx)
	cancel()
	var hello struct {
		Ticket string `json:"ticket"`
	}
	if err != nil || kind != websocket.MessageText || len(data) > 1024 || json.Unmarshal(data, &hello) != nil || !terminalproto.ValidID(hello.Ticket) {
		_ = c.Close(websocket.StatusPolicyViolation, "fresh attachment authorization required")
		return
	}
	cookie, _ := r.Cookie(sessionCookie)
	hash := auth.HashToken(cookie.Value)
	uid := s.uid(r)
	authorize := func() bool {
		if !s.terminalEnabled() {
			return false
		}
		u, ok, err := s.st.SessionUser(hash, time.Now().UnixMilli())
		return err == nil && ok && u.ID == uid && u.Role != "viewer"
	}
	if err := s.terminals.Serve(r.Context(), uid, hash, hello.Ticket, c, authorize); err != nil {
		_ = c.Close(websocket.StatusPolicyViolation, "attachment closed; obtain fresh authorization and resynchronise")
	}
}
func (s *server) privateTerminalData(w http.ResponseWriter, r *http.Request) {
	if !s.privateCredentials() || !s.terminalEnabled() {
		http.NotFound(w, r)
		return
	}
	uid, agent, err := s.st.AuthenticateRemoteAgent(r.Header.Get("X-Reet-Agent"), r.Header.Get("X-Reet-Agent-Key"), r.Header.Get("X-Reet-Version"))
	if err != nil {
		writeErr(w, 403, "machine enrollment is missing or revoked")
		return
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(terminalproto.MaxBinaryBytes)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	kind, data, err := c.Read(ctx)
	cancel()
	if err != nil || kind != websocket.MessageText {
		return
	}
	h, err := terminalproto.DecodeStreamHello(data)
	if err != nil {
		return
	}
	_ = s.terminals.Bind(r.Context(), uid, agent.ID, h, c)
}
