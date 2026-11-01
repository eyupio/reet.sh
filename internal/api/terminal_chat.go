package api

import (
	"bytes"
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
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/chat/model", s.terminal(s.terminalChatModel))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/chat/cancel", s.terminal(s.terminalChatCancel))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/chat/permissions/{permissionId}", s.terminal(s.terminalChatPermission))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/chat/questions/{questionId}", s.terminal(s.terminalChatQuestion))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/chat/approval-mode", s.terminal(s.terminalChatApprovalMode))
	mux.HandleFunc("POST /api/v2/terminal/sessions/{id}/mode", s.terminal(s.terminalSessionMode))
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

// terminalChatQuestion answers a typed agent question. The body is
// {"action":"accept","answers":{...}}, {"action":"decline"} or {"action":"cancel"}.
// The machine validates the answers against the question it holds; a rejected
// answer is a 422 that leaves the question waiting.
func (s *server) terminalChatQuestion(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.terminalChatTarget(w, r); !ok {
		return
	}
	var in struct {
		Action  string          `json:"action"`
		Answers json.RawMessage `json:"answers"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	questionID := r.PathValue("questionId")
	if !terminalproto.ValidQuestionID(questionID) {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	if bytes.Equal(bytes.TrimSpace(in.Answers), []byte("null")) {
		in.Answers = nil
	}
	if err := s.terminals.AnswerQuestionChat(r.Context(), s.uid(r), r.PathValue("id"), questionID, in.Action, in.Answers); err != nil {
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
	case errors.Is(err, terminalbroker.ErrACPModelRejected), errors.Is(err, terminalbroker.ErrACPModelUnsupported):
		writeErr(w, 409, err.Error())
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
	case errors.Is(err, terminalbroker.ErrACPQuestionUnsupported):
		writeErr(w, 409, "that machine or agent cannot answer questions from reet")
	case errors.Is(err, terminalbroker.ErrQuestionGone):
		writeErrCode(w, 409, "question_gone", "", "That question is no longer waiting.", "")
	case errors.As(err, new(*terminalbroker.QuestionInvalidError)):
		var invalid *terminalbroker.QuestionInvalidError
		errors.As(err, &invalid)
		writeErrCode(w, 422, "question_invalid", "", "That answer was not accepted: "+invalid.Detail, "Check the highlighted answers and send them again.")
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

func (s *server) terminalChatModel(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.terminalChatTarget(w, r); !ok {
		return
	}
	var in struct {
		Model string `json:"model"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	if in.Model == "" || !terminalproto.ValidModel(in.Model) {
		writeErr(w, 400, "Choose a valid model.")
		return
	}
	if err := s.terminals.ModelChat(r.Context(), s.uid(r), r.PathValue("id"), in.Model); err != nil {
		s.terminalChatError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"changed": true})
}

// terminalSessionMode switches a running OpenCode session between Gaffer
// ("chat") and the OpenCode TUI ("terminal"). Unlike the chat routes it does
// not require a structured row: a session in the OpenCode TUI must be able to
// switch to chat. The switch can take the machine up to
// terminalproto.SessionModeTimeout; the server sets no response deadline, so
// the handler waits on the request context alone.
func (s *server) terminalSessionMode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !terminalproto.ValidID(id) {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	var in struct {
		Mode  string `json:"mode"`
		Trust bool   `json:"trust"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	if in.Mode != terminalproto.SessionModeTerminal && in.Mode != terminalproto.SessionModeChat {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	if _, err := s.st.TerminalSessionFor(s.uid(r), id); err != nil {
		writeErr(w, 404, "that reet session is not available")
		return
	}
	if err := s.terminals.SwitchSessionMode(r.Context(), s.uid(r), id, in.Mode, in.Trust); err != nil {
		s.terminalSessionModeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) terminalSessionModeError(w http.ResponseWriter, err error) {
	var trust *terminalbroker.ACPTrustError
	switch {
	case errors.As(err, &trust):
		// The structured create's trust refusal, with the plugin files it names.
		writeJSON(w, http.StatusConflict, map[string]string{"error": "This folder has OpenCode plugins. Start only in a folder you trust.", "code": "acp_trust_required", "detail": trust.Detail})
	case errors.Is(err, terminalbroker.ErrSessionModeBusy):
		writeErrCode(w, 409, "session_mode_busy", "", err.Error(), "")
	case errors.Is(err, terminalbroker.ErrSessionModeNoSession):
		writeErrCode(w, 409, "session_mode_no_session", "", err.Error(), "")
	case errors.Is(err, terminalbroker.ErrSessionModeUnsupported):
		writeErrCode(w, 409, "session_mode_unsupported", "", err.Error(), "")
	case errors.Is(err, terminalbroker.ErrSessionModeNoConversation):
		writeErrCode(w, 409, "session_mode_no_conversation", "", err.Error(), "")
	case errors.Is(err, terminalbroker.ErrSessionModeFailed):
		writeErrCode(w, 502, "session_mode_failed", "", err.Error(), "")
	case errors.Is(err, terminalbroker.ErrSessionModeEnded):
		// The switch failed and the machine ended the session: not retryable.
		writeErrCode(w, 409, "session_mode_ended", "", err.Error(), "")
	case errors.Is(err, terminalbroker.ErrYoloNotAllowed):
		// A YOLO session moving to the OpenCode TUI is a YOLO launch; the
		// same refusal and sentence as a launch.
		writeErrCode(w, http.StatusForbidden, "yolo_not_allowed", "", "An owner or admin can allow YOLO for this account in Settings.", "")
	default:
		s.terminalChatError(w, err)
	}
}
