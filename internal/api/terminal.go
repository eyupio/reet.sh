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
	mux.HandleFunc("GET /api/v2/terminal/sessions", s.terminal(s.terminalSessions))
	mux.HandleFunc("POST /api/v2/terminal/sessions", s.terminal(s.terminalCreate))
	mux.HandleFunc("PATCH /api/v2/terminal/sessions/{id}", s.terminal(s.terminalRename))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/end", s.terminal(s.terminalEnd))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/archive", s.terminalAccess(s.terminalArchive, true))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/attachments", s.terminal(s.terminalAttachment))
	mux.HandleFunc("GET /api/v2/terminal/sessions/{id}/history", s.terminal(s.terminalHistory))
	mux.HandleFunc("GET /api/v2/terminal/sessions/{id}/command-history", s.terminal(s.terminalCommandHistory))
	mux.HandleFunc("GET /api/v2/terminal/socket", s.terminal(s.terminalSocket))
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
func (s *server) terminalSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.st.TerminalSessionsFor(s.uid(r))
	if err != nil {
		terminalError(w, err)
		return
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
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	session, err := s.terminals.Create(r.Context(), s.uid(r), in.AgentID, in.RequestID, terminalproto.Create{Title: in.Title, Cols: in.Cols, Rows: in.Rows})
	if err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, 200, session)
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
