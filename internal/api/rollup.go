package api

import (
	"net/http"
)

// Per-person usage is on by default; the owner can turn it down to counts.
func (s *server) rollupIndividual() bool {
	v, _ := s.st.GetSetting("rollup_individual")
	return v != "0"
}

func (s *server) rollup(w http.ResponseWriter, r *http.Request) {
	rollup, err := s.st.RollupFor(s.rollupIndividual())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rollup)
}
