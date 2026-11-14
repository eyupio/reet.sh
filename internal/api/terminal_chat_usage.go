package api

import "net/http"

// terminalChatUsage answers what a chat's agent has reported using: the tokens
// of its finished turns added up and how full its context window last was. It
// reads the controller's own record and asks no machine, so it answers for an
// ended chat and for one whose machine is away.
func (s *server) terminalChatUsage(w http.ResponseWriter, r *http.Request) {
	session, ok := s.terminalChatTarget(w, r)
	if !ok {
		return
	}
	usage, err := s.st.TerminalSessionUsageFor(s.uid(r), session.ID)
	if err != nil {
		writeErr(w, 500, "the chat's usage could not be read")
		return
	}
	writeJSON(w, 200, usage)
}
