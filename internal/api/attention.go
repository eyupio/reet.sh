package api

import (
	"net/http"
	"time"
)

// terminalSessionSeen marks a session as seen by the signed-in person so
// Needs you and waiting bell indicators clear across their devices.
func (s *server) terminalSessionSeen(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	id := r.PathValue("id")
	if _, err := s.st.TerminalSessionFor(uid, id); err != nil {
		terminalError(w, err)
		return
	}
	at := time.Now().UnixMilli()
	if err := s.st.MarkSessionSeenFor(uid, id, at); err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seenAt": at})
}
