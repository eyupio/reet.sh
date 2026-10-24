package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/eyupio/reet.sh/internal/store"
)

// Private bundles are account-owned content, independent of publication and
// terminal execution. Personal API tokens are supported for future CLI clients;
// MCP credentials retain their existing, separate endpoint allowlist.
func (s *server) privateSkillAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if s.authMode != AuthAccounts || s.uid(r) <= 0 || s.role(r) == "viewer" {
			writeErr(w, 403, "Private skill content requires an active account with a write role.")
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			writeErr(w, 400, "Private skill requests do not accept query parameters.")
			return
		}
		if r.Method != http.MethodGet && requestToken(r) == "" && (r.Header.Get("Origin") == "" || !privateSkillSameOrigin(r)) {
			writeErr(w, 403, "Private skill writes require a same-origin browser request or personal API token.")
			return
		}
		next(w, r)
	}
}
func privateSkillSameOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && mcpSameOrigin(r) && origin.Path == "" && origin.RawQuery == "" && !origin.ForceQuery && origin.Fragment == ""
}
func (s *server) registerPrivateSkillRoutes(mux *http.ServeMux) {
	s.registerSkillContentTargets(mux)
	s.registerSkillContentPreviews(mux)
	mux.HandleFunc("GET /api/v2/skill-bundles", s.privateSkillAccess(s.privateSkills))
	mux.HandleFunc("POST /api/v2/skill-bundles", s.privateSkillAccess(s.privateSkillSave))
	mux.HandleFunc("GET /api/v2/skill-bundles/{bundle}", s.privateSkillAccess(s.privateSkillGet))
	mux.HandleFunc("GET /api/v2/skill-bundles/{bundle}/revisions", s.privateSkillAccess(s.privateSkillRevisions))
	mux.HandleFunc("POST /api/v2/skill-bundles/{bundle}/revisions", s.privateSkillAccess(s.privateSkillSave))
	mux.HandleFunc("GET /api/v2/skill-bundles/{bundle}/revisions/{revision}", s.privateSkillAccess(s.privateSkillRevision))
}
func privateSkillError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrPrivateSkillInvalid):
		writeErr(w, 400, "Invalid private skill revision. Supply a request ID, explicit expected base, matching skill name, valid SKILL.md and bounded portable file paths.")
	case errors.Is(err, store.ErrPrivateSkillMissing):
		writeErr(w, 404, "Private skill unavailable.")
	case errors.Is(err, store.ErrPrivateSkillConflict):
		writeErr(w, 409, "The skill head changed or this request ID was used for a different write. Read the current head before submitting a new revision.")
	case errors.Is(err, store.ErrPrivateSkillCapacity):
		writeErr(w, 409, "Private skill storage limit reached (128 bundles, 256 revisions or 64 MiB of unique file bytes per account).")
	default:
		writeErr(w, 500, "Could not access private skill storage.")
	}
}
func (s *server) privateSkills(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.PrivateSkillBundlesFor(s.uid(r))
	if err != nil {
		privateSkillError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"bundles": items})
}
func (s *server) privateSkillGet(w http.ResponseWriter, r *http.Request) {
	item, err := s.st.PrivateSkillBundleFor(s.uid(r), r.PathValue("bundle"))
	if err != nil {
		privateSkillError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (s *server) privateSkillRevisions(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.PrivateSkillRevisionsFor(s.uid(r), r.PathValue("bundle"))
	if err != nil {
		privateSkillError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"revisions": items})
}
func (s *server) privateSkillRevision(w http.ResponseWriter, r *http.Request) {
	item, err := s.st.PrivateSkillRevisionFor(s.uid(r), r.PathValue("bundle"), r.PathValue("revision"))
	if err != nil {
		privateSkillError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (s *server) privateSkillSave(w http.ResponseWriter, r *http.Request) {
	var in store.PrivateSkillWrite
	// Base64 expands an 8 MiB payload; the remaining space bounds metadata/escaping.
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF {
		writeErr(w, 400, "Invalid or oversized private skill request.")
		return
	}
	id := r.PathValue("bundle")
	if id != "" && !store.PrivateSkillIDValid(id) {
		privateSkillError(w, store.ErrPrivateSkillMissing)
		return
	}
	item, err := s.st.SavePrivateSkillRevisionFor(s.uid(r), id, in)
	if err != nil {
		privateSkillError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
