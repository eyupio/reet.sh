package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// terminalPrompt serves the whole opening prompt to the session's owner. The
// list carries only an excerpt because it goes to every open tab; this is the
// one route that returns the full text, so it is never logged or audited, and
// terminalAccess already marks the reply no-store. An archived session answers
// an empty prompt, as its text was cleared when it was archived.
func (s *server) terminalPrompt(w http.ResponseWriter, r *http.Request) {
	prompt, err := s.st.TerminalPromptFor(s.uid(r), r.PathValue("id"))
	switch {
	case err == nil:
		writeJSON(w, 200, map[string]string{"prompt": prompt})
	case errors.Is(err, terminalproto.ErrInvalid):
		writeErr(w, 400, "invalid session")
	case errors.Is(err, store.ErrRemoteNotFound):
		writeErr(w, 404, "session not found")
	default:
		writeErr(w, 500, "the prompt could not be read")
	}
}
