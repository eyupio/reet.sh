package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Prompt text is prose and can carry a long body after JSON escaping.
func terminalPromptJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF {
		writeErr(w, 400, "invalid saved prompt request")
		return false
	}
	return true
}

func terminalPromptError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, terminalproto.ErrInvalid):
		writeErr(w, 400, "Use a title of 1–80 characters, a body up to 8,000 bytes without terminal control codes, and notes up to 500 characters.")
	case errors.Is(err, store.ErrTerminalConflict):
		writeErr(w, 409, "This prompt changed in another browser. Refresh the library to review the latest version.")
	case errors.Is(err, store.ErrTerminalPromptLimit):
		writeErr(w, 409, "Your prompt library is full (200 prompts). Remove an unused prompt before adding another.")
	default:
		terminalError(w, err)
	}
}

func (s *server) terminalPrompts(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.TerminalPromptsFor(s.uid(r))
	if err != nil {
		terminalPromptError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"prompts": items})
}

func (s *server) terminalPromptSave(w http.ResponseWriter, r *http.Request) {
	var in store.TerminalPromptWrite
	if !terminalPromptJSON(w, r, &in) {
		return
	}
	item, err := s.st.SaveTerminalPromptFor(s.uid(r), r.PathValue("id"), in)
	if err != nil {
		terminalPromptError(w, err)
		return
	}
	writeJSON(w, 200, item)
}

func (s *server) terminalPromptDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
	}
	if !terminalPromptJSON(w, r, &in) {
		return
	}
	if err := s.st.DeleteTerminalPromptFor(s.uid(r), r.PathValue("id"), in.Revision); err != nil {
		terminalPromptError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
