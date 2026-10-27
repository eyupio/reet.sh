package api

import (
	"encoding/json"
	"errors"
	"github.com/eyupio/reet.sh/internal/store"
	"io"
	"net/http"
)

func (s *server) machineSkillImport(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	if a.Paused {
		writeErr(w, 409, "Machine is paused.")
		return
	}
	if r.Method == "GET" {
		writeJSON(w, 200, map[string]int{"schemaVersion": 1, "maxBytes": 8 << 20, "maxFiles": 256})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	var in store.MachineSkillImport
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF {
		writeErr(w, 400, "Invalid or oversized machine skill import.")
		return
	}
	result, err := s.st.SaveMachineSkillImportFor(s.uid(r), a.ID, in)
	if errors.Is(err, store.ErrMachineImportChanged) {
		writeErr(w, 409, "Contents differ from the reviewed digest; preview again.")
		return
	}
	if errors.Is(err, store.ErrRemoteNotFound) || errors.Is(err, store.ErrContentObservationStale) {
		writeErr(w, 409, "Enrollment or scope unavailable; scan again before importing.")
		return
	}
	if err != nil {
		privateSkillError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
