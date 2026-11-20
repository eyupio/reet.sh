package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
)

// The canonical library's own routes: the account's items, one item, an
// item's status set behind its version, and a skill's history with Put back.
func (s *server) registerCanonicalLibrary(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/library", s.privateSkillAccess(s.canonicalLibrary))
	mux.HandleFunc("GET /api/v2/library/{item}", s.privateSkillAccess(s.canonicalLibraryItem))
	mux.HandleFunc("PATCH /api/v2/library/{item}", s.privateSkillAccess(s.canonicalLibraryStatus))
	mux.HandleFunc("GET /api/v2/library-history/{item}", s.privateSkillAccess(s.libraryHistory))
	mux.HandleFunc("POST /api/v2/library-history/{item}/restore", s.privateSkillAccess(s.libraryRestore))
}

// libraryHistory is GET /api/v2/library-history/{item}: the skill's saved
// versions, newest first, the current one marked.
func (s *server) libraryHistory(w http.ResponseWriter, r *http.Request) {
	out, err := s.st.SkillHistoryFor(s.uid(r), r.PathValue("item"))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"revisions": out})
}

// libraryRestore is POST /api/v2/library-history/{item}/restore, body
// {"revision":"<the version to put back>","head":"<the current version the
// person saw>"}: Put back. The old version's files become a new current
// version, of kind restore, which every machine gets; the one before stays in
// the history. It answers 409 when the skill has moved on from head or the
// version is already the current one.
func (s *server) libraryRestore(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision string  `json:"revision"`
		Head     *string `json:"head"`
	}
	if !decodeStrictJSON(w, r, 4096, &in, "Invalid or oversized request.") {
		return
	}
	if in.Revision == "" || in.Head == nil {
		writeErr(w, 400, "Say which version to put back and which one is current now. Open the skill again.")
		return
	}
	uid, item := s.uid(r), r.PathValue("item")
	res, err := s.st.RestoreSkillRevisionFor(uid, item, in.Revision, *in.Head)
	switch {
	case errors.Is(err, store.ErrPrivateSkillConflict):
		writeErr(w, 409, "The skill changed since you opened it. Open it again to see its history now.")
		return
	case errors.Is(err, store.ErrSkillRevisionCurrent):
		writeErr(w, 409, "That version is already the current one.")
		return
	case errors.Is(err, store.ErrPrivateSkillMissing):
		writeErr(w, 404, "That version is not in this skill's history.")
		return
	case err != nil:
		privateSkillError(w, err)
		return
	}
	out := map[string]any{"revision": res.Revision.ID}
	if err := s.refreshHosted(uid, item); err != nil {
		var problem store.SkillPublishProblem
		if errors.As(err, &problem) {
			out["publicProblem"] = "The public page still shows the version before: " + problem.Reason
		} else {
			log.Printf("skill put back: public page of %s: %v", item, err)
		}
	}
	s.eventRequest(r, "skill-restore", item, map[string]any{"revision": res.Revision.ID, "from": in.Revision})
	writeJSON(w, 200, out)
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
