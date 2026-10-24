package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (s *server) terminalCommandHistory(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	data, err := s.terminals.CommandHistory(r.Context(), s.uid(r), r.PathValue("id"))
	if errors.Is(err, terminalbroker.ErrCommandHistoryUnsupported) {
		writeErr(w, 409, "Update this machine's reet agent to use shell command history. Terminal output history is still available.")
		return
	}
	if err != nil {
		terminalError(w, err)
		return
	}
	history, err := terminalproto.DecodeCommandHistory(data)
	if err != nil {
		writeErr(w, 502, "The machine returned an invalid command history snapshot.")
		return
	}
	if user, ok := s.sessionUser(r); !ok || user.ID != s.uid(r) || user.Role == "viewer" {
		terminalError(w, terminalbroker.ErrDenied)
		return
	}
	if _, err := s.st.TerminalSessionFor(s.uid(r), r.PathValue("id")); err != nil {
		terminalError(w, terminalbroker.ErrDenied)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, 200, history)
}
