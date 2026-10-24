package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Command metadata is larger than a wire-control frame after JSON escaping.
func terminalCommandJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF {
		writeErr(w, 400, "invalid saved command request")
		return false
	}
	return true
}
func terminalCommandError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, terminalproto.ErrInvalid):
		writeErr(w, 400, "Use a name of 1–80 characters, a command up to 16 KiB without terminal control codes, and notes up to 500 characters.")
	case errors.Is(err, store.ErrTerminalConflict):
		writeErr(w, 409, "This command changed in another browser. Refresh the library to review the latest version.")
	case errors.Is(err, store.ErrTerminalCommandLimit):
		writeErr(w, 409, "Your command library is full (200 commands). Remove an unused command before adding another.")
	default:
		terminalError(w, err)
	}
}
func (s *server) terminalCommands(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.TerminalCommandsFor(s.uid(r))
	if err != nil {
		terminalCommandError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"commands": items})
}
func (s *server) terminalCommandSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Command     string `json:"command"`
		Description string `json:"description"`
		Revision    int64  `json:"revision"`
	}
	if !terminalCommandJSON(w, r, &in) {
		return
	}
	item, err := s.st.SaveTerminalCommandFor(s.uid(r), r.PathValue("id"), in.Name, in.Command, in.Description, in.Revision)
	if err != nil {
		terminalCommandError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (s *server) terminalCommandDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
	}
	if !terminalCommandJSON(w, r, &in) {
		return
	}
	if err := s.st.DeleteTerminalCommandFor(s.uid(r), r.PathValue("id"), in.Revision); err != nil {
		terminalCommandError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
