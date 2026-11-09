package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) registerCanonicalLibrary(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/library/deployment-intent", s.privateSkillAccess(s.libraryDeploymentIntent))
	mux.HandleFunc("POST /api/v2/library/bases", s.privateSkillAccess(s.libraryBase))
	mux.HandleFunc("POST /api/v2/library/protocol", s.privateSkillAccess(s.libraryProtocol))
	mux.HandleFunc("POST /api/v2/library/edits", s.privateSkillAccess(s.libraryEdit))
	mux.HandleFunc("GET /api/v2/library-history/{item}", s.privateSkillAccess(s.libraryHistory))
	mux.HandleFunc("PUT /api/v2/library/selections", s.privateSkillAccess(s.saveCanonicalSelection))
	mux.HandleFunc("GET /api/v2/library/selections/{target}", s.privateSkillAccess(s.getCanonicalSelection))
	mux.HandleFunc("GET /api/v2/library", s.privateSkillAccess(s.canonicalLibrary))
	mux.HandleFunc("GET /api/v2/library/{item}", s.privateSkillAccess(s.canonicalLibraryItem))
	mux.HandleFunc("PATCH /api/v2/library/{item}", s.privateSkillAccess(s.canonicalLibraryStatus))
	mux.HandleFunc("POST /api/v2/library/locations", s.privateSkillAccess(s.canonicalLibraryLocation))
	mux.HandleFunc("POST /api/v2/library/locations/content", s.privateSkillAccess(s.canonicalLibraryImport))
}

func (s *server) libraryDeploymentIntent(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in store.LibraryDeploymentCheck
	if !importDecode(w, r, 4096, &in) {
		return
	}
	out, err := s.st.LibraryDeploymentIntentFor(s.uid(r), agent.ID, in)
	if err != nil {
		if errors.Is(err, store.ErrContentObservationStale) {
			writeErr(w, http.StatusConflict, "The machine is paused or its deployment observation changed; retry from current intent.")
			return
		}
		libraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getCanonicalSelection(w http.ResponseWriter, r *http.Request) {
	out, err := s.st.LibrarySelectionFor(s.uid(r), r.PathValue("target"))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *server) saveCanonicalSelection(w http.ResponseWriter, r *http.Request) {
	var in store.LibrarySelection
	if !importDecode(w, r, 65536, &in) {
		return
	}
	if err := s.st.SetSyncLibraryOrigin(s.uid(r), requestOrigin(r)); err != nil {
		libraryError(w, err)
		return
	}
	out, err := s.st.SaveLibrarySelectionFor(s.uid(r), in)
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func libraryError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrLibraryConflict) {
		writeErr(w, 409, "The library item or location changed. Refresh before retrying.")
		return
	}
	if errors.Is(err, store.ErrRemoteNotFound) {
		writeErr(w, 403, "This connector cannot access that location.")
		return
	}
	privateSkillError(w, err)
}

func (s *server) canonicalLibrary(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.LibraryFor(s.uid(r))
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
func (s *server) canonicalLibraryLocation(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in store.LibraryLocation
	if !importDecode(w, r, 16384, &in) {
		return
	}
	items, err := s.st.RegisterLibraryLocationFor(s.uid(r), agent.ID, in)
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schemaVersion": 1, "items": items})
}
func (s *server) canonicalLibraryImport(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in store.LibraryLocationImport
	if !importDecode(w, r, 16<<20, &in) {
		return
	}
	item, err := s.st.ImportLibraryLocationFor(s.uid(r), agent.ID, in)
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, item)
}

func (s *server) libraryProtocol(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	if err := s.st.NegotiateLibraryFor(s.uid(r), agent.ID); err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schemaVersion": 1, "serverTime": time.Now().UnixMilli()})
}
func (s *server) libraryEdit(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in store.LibraryEdit
	if !importDecode(w, r, 16<<20, &in) {
		return
	}
	out, err := s.st.AcceptLibraryEditFor(s.uid(r), agent.ID, in)
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *server) libraryHistory(w http.ResponseWriter, r *http.Request) {
	out, err := s.st.LibraryEditHistoryFor(s.uid(r), r.PathValue("item"))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *server) libraryBase(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in store.LibraryBase
	if !importDecode(w, r, 4096, &in) {
		return
	}
	if err := s.st.RecordLibraryBaseFor(s.uid(r), agent.ID, in); err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, in)
}
