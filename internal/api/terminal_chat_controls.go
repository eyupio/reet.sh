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

// chatCommands is the slash command list a chat's agent last announced.
// Commands is never null. Code and Error say why a machine cannot read them,
// with the same code style the controls refusal uses.
type chatCommands struct {
	Commands []terminalproto.ACPCommand `json:"commands"`
	Code     string                     `json:"code,omitempty"`
	Error    string                     `json:"error,omitempty"`
}

func (s *server) terminalChatCommands(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.terminalChatTarget(w, r); !ok {
		return
	}
	answer, err := s.terminals.ChatControl(r.Context(), s.uid(r), r.PathValue("id"), terminalproto.ACPControl{Action: "commands"})
	out := chatCommands{Commands: []terminalproto.ACPCommand{}}
	if errors.Is(err, terminalbroker.ErrChatCommandsUnsupported) {
		out.Code, out.Error = "chat_commands_unsupported", err.Error()
		writeJSON(w, 200, out)
		return
	}
	if err != nil {
		s.terminalChatError(w, err)
		return
	}
	out.Commands = append(out.Commands, answer.Commands...)
	writeJSON(w, 200, out)
}
