package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (s *server) registerTerminalChatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/terminal/sessions/{id}/chat", s.terminal(s.terminalChat))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/chat/prompt", s.terminal(s.terminalChatPrompt))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/chat/cancel", s.terminal(s.terminalChatCancel))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/chat/permissions/{permissionId}", s.terminal(s.terminalChatPermission))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/chat/approval-mode", s.terminal(s.terminalChatApprovalMode))
}

func (s *server) terminalChatSession(r *http.Request) (store.TerminalSession, bool) {
	id := r.PathValue("id")
	if !terminalproto.ValidID(id) {
		writeErr(w(r), 400, "invalid terminal request")
		return store.TerminalSession{}, false
	}
	session, err := s.st.TerminalSessionFor(s.uid(r), id)
	if err != nil || !session.Structured {
		writeErr(w(r), 404, "that reet session is not available")
		return store.TerminalSession{}, false
	}
	return session, true
}

func w(r *http.Request) http.ResponseWriter {
	return r.Context().Value(responseWriterKey{}).(http.ResponseWriter)
}

type responseWriterKey struct{}

func (s *server) terminalChat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !terminalproto.ValidID(id) {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	session, err := s.st.TerminalSessionFor(s.uid(r), id)
	if err != nil || !session.Structured {
		writeErr(w, 404, "that reet session is not available")
		return
	}
	snapshot, truncated, events, cancel := s.terminals.SubscribeChat(s.uid(r), id)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming is unavailable")
		return
	}
	reset, _ := json.Marshal(map[string]bool{"truncated": truncated})
	_, _ = fmt.Fprintf(w, "event: reset\ndata: %s\n\n", reset)
	for _, event := range snapshot {
		writeChatSSE(w, event)
	}
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			u, ok := s.sessionUser(r)
			if !ok || u.Role == "viewer" || !s.terminalEnabled() {
				return
			}
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case event, ok := <-events:
			if !ok {
				return
			}
			writeChatSSE(w, event)
			flusher.Flush()
		}
	}
}

func writeChatSSE(w http.ResponseWriter, event terminalbroker.ChatEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: chat\nid: %d\ndata: %s\n\n", event.Seq, data)
}

func (s *server) terminalChatPrompt(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.terminalChatTarget(w, r); !ok {
		return
	}
	var in struct {
		Text   string   `json:"text"`
		Images []string `json:"images"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	if !terminalproto.ValidACPPrompt(in.Text, in.Images) {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	if err := s.terminals.PromptChat(r.Context(), s.uid(r), r.PathValue("id"), in.Text, in.Images); err != nil {
		s.terminalChatError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"sent": true})
}

func (s *server) terminalChatCancel(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.terminalChatTarget(w, r); !ok {
		return
	}
	if err := s.terminals.CancelChat(r.Context(), s.uid(r), r.PathValue("id")); err != nil {
		s.terminalChatError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"cancelled": true})
}

func (s *server) terminalChatPermission(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.terminalChatTarget(w, r); !ok {
		return
	}
	var in struct {
		Allow      bool `json:"allow"`
		ForSession bool `json:"forSession"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	permissionID := r.PathValue("permissionId")
	if !terminalproto.ValidPermissionID(permissionID) {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	if err := s.terminals.PermissionChat(r.Context(), s.uid(r), r.PathValue("id"), permissionID, in.Allow, in.ForSession); err != nil {
		s.terminalChatError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"answered": true})
}

func (s *server) terminalChatApprovalMode(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.terminalChatTarget(w, r); !ok {
		return
	}
	var in struct {
		Mode string `json:"mode"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	if in.Mode != "ask" && in.Mode != "yolo" {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	if err := s.terminals.SetChatYolo(r.Context(), s.uid(r), r.PathValue("id"), in.Mode == "yolo"); err != nil {
		s.terminalChatError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"mode": in.Mode})
}

func (s *server) terminalChatTarget(w http.ResponseWriter, r *http.Request) (store.TerminalSession, bool) {
	id := r.PathValue("id")
	if !terminalproto.ValidID(id) {
		writeErr(w, 400, "invalid terminal request")
		return store.TerminalSession{}, false
	}
	session, err := s.st.TerminalSessionFor(s.uid(r), id)
	if err != nil || !session.Structured {
		writeErr(w, 404, "that reet session is not available")
		return store.TerminalSession{}, false
	}
	return session, true
}

func (s *server) terminalChatError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrRemoteNotFound):
		writeErr(w, 404, "that reet session is not available")
	case errors.Is(err, terminalbroker.ErrACPUnsupported), errors.Is(err, terminalbroker.ErrACPClosed):
		writeErr(w, 409, "that reet session is not running")
	case errors.Is(err, terminalbroker.ErrACPImagesUnsupported):
		writeErr(w, 409, "update reet on that machine to send images with a chat message")
	case errors.Is(err, terminalbroker.ErrACPImageUnavailable):
		writeErr(w, 409, "that image is no longer available on the machine; attach it again")
	case errors.Is(err, terminalbroker.ErrACPApprovalUnsupported):
		writeErr(w, 409, "update reet on that machine to switch approval mode")
	case errors.Is(err, terminalbroker.ErrYoloNotAllowed):
		writeErr(w, 403, "an owner or admin can allow YOLO for this account in Settings")
	case errors.Is(err, terminalbroker.ErrYoloRefused):
		writeErr(w, 403, "that machine or this agent does not allow YOLO")
	case errors.Is(err, terminalbroker.ErrACPBusy):
		writeErr(w, 409, "that reet session is already working")
	case errors.Is(err, terminalbroker.ErrOffline):
		writeErr(w, 503, "terminal machine is reconnecting; retry after it reconnects")
	case errors.Is(err, terminalproto.ErrInvalid):
		writeErr(w, 400, "invalid terminal request")
	default:
		writeErr(w, 502, "the reet session could not complete that request")
	}
}
