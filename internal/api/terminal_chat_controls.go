package api

import (
	"errors"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"net/http"
)

func (s *server) terminalChatControls(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.terminalChatTarget(w, r); !ok {
		return
	}
	in := terminalproto.ACPControl{Action: "get"}
	if r.Method == http.MethodPost {
		if !terminalJSON(w, r, &in) {
			return
		}
		if in.Action != "set" {
			writeErr(w, 400, "invalid chat setting")
			return
		}
		// Encoding applies the same shape bounds as the private decoder.
		if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: terminalbroker.ID(), SessionID: r.PathValue("id"), Generation: 1, Body: &in}); err != nil {
			writeErr(w, 400, "invalid chat setting")
			return
		}
	}
	answer, err := s.terminals.ChatControl(r.Context(), s.uid(r), r.PathValue("id"), in)
	if errors.Is(err, terminalbroker.ErrLiveControlsUnsupported) {
		writeJSON(w, 409, map[string]string{"error": err.Error(), "code": "chat_controls_unsupported"})
		return
	}
	if err != nil {
		s.terminalChatError(w, err)
		return
	}
	writeJSON(w, 200, answer)
}
