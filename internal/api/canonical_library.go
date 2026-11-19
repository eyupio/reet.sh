package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
)

// The canonical library's own routes: the account's items, one item, and an
// item's status set behind its version.
func (s *server) registerCanonicalLibrary(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/library", s.privateSkillAccess(s.canonicalLibrary))
	mux.HandleFunc("GET /api/v2/library/{item}", s.privateSkillAccess(s.canonicalLibraryItem))
	mux.HandleFunc("PATCH /api/v2/library/{item}", s.privateSkillAccess(s.canonicalLibraryStatus))
}

func libraryError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrLibraryConflict) {
		writeErr(w, 409, "The library item or location changed. Refresh before retrying.")
		return
	}
	privateSkillError(w, err)
}

func (s *server) canonicalLibrary(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.LibraryViewFor(s.uid(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schemaVersion": 1, "items": items})
}

func (s *server) canonicalLibraryItem(w http.ResponseWriter, r *http.Request) {
	item, err := s.st.LibraryItemFor(s.uid(r), r.PathValue("item"))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, item)
}

func (s *server) canonicalLibraryStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status          string `json:"status"`
		ExpectedVersion int64  `json:"expectedVersion"`
	}
	if !importDecode(w, r, 4096, &in) {
		return
	}
	if err := s.st.SetLibraryStatusFor(s.uid(r), r.PathValue("item"), in.Status, in.ExpectedVersion); err != nil {
		libraryError(w, err)
		return
	}
	s.canonicalLibraryItem(w, r)
}
