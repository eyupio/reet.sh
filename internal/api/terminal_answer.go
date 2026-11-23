package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// terminalAnswer chooses one option of the question a terminal agent is
// waiting on. The browser names the question by its fingerprint and the
// option by its number; the machine reads its screen again and works out the
// keys. It is terminal input, so it has the terminal routes' rules: a signed-in
// browser, never a viewer or an API token, and the caller's own session.
func (s *server) terminalAnswer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !terminalproto.ValidID(id) {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	session, err := s.st.TerminalSessionFor(s.uid(r), id)
	if err != nil || session.Structured {
		writeErr(w, 404, "that reet session is not available")
		return
	}
	var in struct {
		Fingerprint string `json:"fingerprint"`
		Option      int    `json:"option"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	err = s.terminals.AnswerTerminalQuestion(r.Context(), s.uid(r), id, in.Fingerprint, in.Option)
	switch {
	case err == nil:
		writeJSON(w, 200, map[string]bool{"answered": true})
	case errors.Is(err, terminalbroker.ErrAlreadyAnswered):
		writeJSON(w, 200, map[string]bool{"answered": true, "alreadyAnswered": true})
	case errors.Is(err, terminalbroker.ErrQuestionChanged):
		writeErrCode(w, 409, "question_changed", "", err.Error(), "")
	case errors.Is(err, terminalbroker.ErrTerminalQuestionUnsupported):
		writeErrCode(w, 409, "update_required", "", err.Error(), "")
	case errors.Is(err, terminalbroker.ErrAnswerBusy):
		writeErrCode(w, 409, "answer_busy", "", err.Error(), "")
	case errors.Is(err, store.ErrRemoteNotFound):
		writeErr(w, 404, "that reet session is not available")
	default:
		terminalError(w, err)
	}
}
