package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/eyupio/reet.sh/internal/gaffer"
)

func (s *server) registerGafferChatRoutes(mux *http.ServeMux) {
	s.registerContextRoutes(mux)
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{accounts: "Gaffer conversations need a browser account.", signedOut: "Sign in to talk to Gaffer.", readOnly: "This account is read-only, so it cannot use Gaffer conversations."}, h)
	}
	mux.HandleFunc("GET /api/v2/gaffer/conversations", access(s.gafferConversationScope))
	mux.HandleFunc("POST /api/v2/gaffer/conversations", access(s.gafferConversationCreate))
	mux.HandleFunc("GET /api/v2/gaffer/conversations/{id}", access(s.gafferConversationGet))
	mux.HandleFunc("PATCH /api/v2/gaffer/conversations/{id}", access(s.gafferConversationConfigure))
	mux.HandleFunc("DELETE /api/v2/gaffer/conversations/{id}", access(s.gafferConversationDelete))
	mux.HandleFunc("POST /api/v2/gaffer/conversations/{id}/turns", access(s.gafferTurnStart))
	mux.HandleFunc("POST /api/v2/gaffer/conversations/{id}/turns/{turnId}/cancel", access(s.gafferTurnCancel))
	mux.HandleFunc("GET /api/v2/gaffer/conversations/{id}/turns/{turnId}/events", access(s.gafferTurnEvents))
}
func (s *server) gafferConversationScope(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("all") == "true" {
		if r.URL.Query().Get("sessionId") != "" {
			writeErr(w, 400, "Choose either all Gaffer conversations or one session.")
			return
		}
		rows, err := s.gafferProviders.Conversations(s.uid(r))
		if err != nil {
			gafferError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"conversations": rows})
		return
	}
	v, err := s.gafferProviders.ConversationScope(s.uid(r), r.URL.Query().Get("sessionId"))
	if err != nil {
		gafferError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *server) gafferConversationGet(w http.ResponseWriter, r *http.Request) {
	v, err := s.gafferProviders.Conversation(s.uid(r), r.PathValue("id"))
	if err != nil {
		gafferError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *server) gafferConversationCreate(w http.ResponseWriter, r *http.Request) {
	var in gaffer.ConversationInput
	if !decodeStrictJSON(w, r, 4096, &in, "That Gaffer conversation request is invalid.") {
		return
	}
	v, err := s.gafferProviders.CreateConversation(s.uid(r), in)
	if err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-conversation-opened", "", map[string]any{"conversation": v.Conversation.ID})
	writeJSON(w, 200, v)
}
func (s *server) gafferConversationConfigure(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
		gaffer.ConversationInput
	}
	if !decodeStrictJSON(w, r, 4096, &in, "That Gaffer conversation choice is invalid.") {
		return
	}
	v, err := s.gafferProviders.ConfigureConversation(s.uid(r), r.PathValue("id"), in.Revision, in.ConversationInput)
	if err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-conversation-configured", "", map[string]any{"conversation": v.Conversation.ID})
	writeJSON(w, 200, v)
}
func (s *server) gafferConversationDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "Reload this Gaffer conversation before removing it.") {
		return
	}
	if err := s.gafferProviders.DeleteConversation(s.uid(r), r.PathValue("id"), in.Revision); err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-conversation-removed", "", map[string]any{"conversation": r.PathValue("id")})
	writeJSON(w, 200, map[string]bool{"removed": true})
}
func (s *server) gafferTurnStart(w http.ResponseWriter, r *http.Request) {
	var in gaffer.TurnInput
	if !decodeStrictJSON(w, r, 24<<10, &in, "That Gaffer message is invalid or too large.") {
		return
	}
	t, err := s.gafferProviders.StartTurn(s.uid(r), r.PathValue("id"), in)
	if err != nil {
		gafferError(w, err)
		return
	}
	// Text, model output and credentials never belong in Activity.
	s.eventRequest(r, "gaffer-turn-accepted", "", map[string]any{"conversation": t.ConversationID, "turn": t.ID})
	writeJSON(w, http.StatusAccepted, t)
}
func (s *server) gafferTurnCancel(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !decodeStrictJSON(w, r, 1024, &in, "That Gaffer cancellation is invalid.") {
		return
	}
	if err := s.gafferProviders.CancelTurn(s.uid(r), r.PathValue("id"), r.PathValue("turnId")); err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-turn-cancel-requested", "", map[string]any{"conversation": r.PathValue("id"), "turn": r.PathValue("turnId")})
	writeJSON(w, 200, map[string]bool{"requested": true})
}

// Streaming observes a durable turn; disconnecting this read never replays or
// cancels a provider call. Reconnecting starts with the latest persisted state.
func (s *server) gafferTurnEvents(w http.ResponseWriter, r *http.Request) {
	uid, id, turnID := s.uid(r), r.PathValue("id"), r.PathValue("turnId")
	turn, err := s.gafferProviders.Turn(uid, id, turnID)
	if err != nil {
		gafferError(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "Streaming is unavailable.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	defer controller.SetWriteDeadline(time.Time{})
	write := func(data []byte) error {
		_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
		_, err := fmt.Fprintf(w, "event: turn\ndata: %s\n\n", data)
		if err == nil {
			flusher.Flush()
		}
		return err
	}
	last, _ := json.Marshal(turn)
	if write(last) != nil || turn.State != "running" {
		return
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	heartbeat := time.Now()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			u, ok := s.sessionUser(r)
			if !ok || u.ID != uid || u.Role == "viewer" {
				return
			}
			turn, err = s.gafferProviders.Turn(uid, id, turnID)
			if err != nil {
				return
			}
			data, _ := json.Marshal(turn)
			if string(data) != string(last) {
				if write(data) != nil {
					return
				}
				last = data
				heartbeat = time.Now()
			} else if time.Since(heartbeat) > 10*time.Second {
				_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
				if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
					return
				}
				flusher.Flush()
				heartbeat = time.Now()
			}
			if turn.State != "running" {
				return
			}
		}
	}
}
