package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) registerSkillContentTargets(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/skill-content-targets", s.privateSkillAccess(s.skillContentTargets))
	mux.HandleFunc("POST /api/v2/skill-content-targets", s.privateSkillAccess(s.skillContentTargetSave))
	mux.HandleFunc("GET /api/v2/skill-content-targets/{id}", s.privateSkillAccess(s.skillContentTargetGet))
	mux.HandleFunc("DELETE /api/v2/skill-content-targets/{id}", s.privateSkillAccess(s.skillContentTargetDelete))
}
func skillContentTargetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrSkillContentTargetInvalid):
		writeErr(w, 400, "Supply an owned revision, reported scope, additive/managed mode and explicit expected version.")
	case errors.Is(err, store.ErrSkillContentTargetMissing):
		writeErr(w, 404, "Skill content target unavailable.")
	case errors.Is(err, store.ErrSkillContentTargetConflict):
		writeErr(w, 409, "The content target changed. Refresh and review before saving again.")
	case errors.Is(err, store.ErrSkillContentTargetScope):
		writeErr(w, 409, "Choose a complete, recently reported scope on an owned, unpaused connector.")
	case errors.Is(err, store.ErrSkillContentTargetCapacity):
		writeErr(w, 409, "Skill content target limit reached (1024 per account).")
	default:
		privateSkillError(w, err)
	}
}
func decodeContentTarget(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		writeErr(w, 400, "Invalid or oversized skill content target request.")
		return false
	}
	return true
}
func (s *server) skillContentTargets(w http.ResponseWriter, r *http.Request) {
	out, e := s.st.SkillContentTargetsFor(s.uid(r))
	if e != nil {
		skillContentTargetError(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"targets": out})
}
func (s *server) skillContentTargetGet(w http.ResponseWriter, r *http.Request) {
	out, e := s.st.SkillContentTargetFor(s.uid(r), r.PathValue("id"))
	if e != nil {
		skillContentTargetError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (s *server) skillContentTargetSave(w http.ResponseWriter, r *http.Request) {
	var in store.SkillContentTargetWrite
	if !decodeContentTarget(w, r, &in) {
		return
	}
	out, e := s.st.SaveSkillContentTargetFor(s.uid(r), in)
	if e != nil {
		skillContentTargetError(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (s *server) skillContentTargetDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ExpectedVersion *string `json:"expectedVersion"`
	}
	if !decodeContentTarget(w, r, &in) {
		return
	}
	if in.ExpectedVersion == nil {
		skillContentTargetError(w, store.ErrSkillContentTargetInvalid)
		return
	}
	if e := s.st.DeleteSkillContentTargetFor(s.uid(r), r.PathValue("id"), *in.ExpectedVersion); e != nil {
		skillContentTargetError(w, e)
		return
	}
	writeJSON(w, 200, map[string]bool{"removed": true})
}
