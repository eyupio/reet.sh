package api

import "net/http"

func (s *server) terminalNativeView(w http.ResponseWriter, r *http.Request) {
	info, err := s.terminals.NativeViewInfo(r.Context(), s.uid(r), r.PathValue("id"))
	if err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}
