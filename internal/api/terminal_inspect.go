package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (s *server) terminalInspect(w http.ResponseWriter, r *http.Request) {
	q, parseErr := url.ParseQuery(r.URL.RawQuery)
	if parseErr != nil {
		writeErr(w, 400, "invalid session view")
		return
	}
	view := q.Get("view")
	if view == "" {
		view = "status"
	}
	// Reject unknown and repeated fields instead of silently accepting a directory override.
	for key, values := range q {
		if (key != "view" && key != "path") || len(values) != 1 {
			writeErr(w, 400, "invalid session view")
			return
		}
	}
	result, err := s.terminals.InspectSession(r.Context(), s.uid(r), r.PathValue("id"), view, q.Get("path"))
	switch {
	case err == nil:
		writeJSON(w, 200, result)
	case errors.Is(err, terminalproto.ErrInvalid):
		writeErr(w, 400, "invalid session view or path")
	case errors.Is(err, terminalbroker.ErrDenied):
		writeErr(w, 404, "session not found")
	case errors.Is(err, terminalbroker.ErrInspectUnsupported):
		writeErrCode(w, 409, "inspect_unsupported", "", err.Error(), "")
	default:
		writeErr(w, 503, "The machine could not be reached. Try again when it is online.")
	}
}
