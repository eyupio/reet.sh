package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
)

// Private Factory pathways: the signed-in browser account's saved, ordered
// stages. Another account's pathway is not found, as with templates.
func (s *server) registerGafferPathwayRoutes(mux *http.ServeMux) {
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{
			accounts: "Factory pathways need a browser account.", signedOut: "Sign in to use Factory pathways.", readOnly: "This account is read-only, so it cannot use Factory pathways.",
		}, h)
	}
	mux.HandleFunc("GET /api/v2/gaffer/pathways", access(s.gafferPathways))
	mux.HandleFunc("POST /api/v2/gaffer/pathways", access(s.gafferPathwayCreate))
	mux.HandleFunc("GET /api/v2/gaffer/pathways/{id}", access(s.gafferPathway))
	mux.HandleFunc("PUT /api/v2/gaffer/pathways/{id}", access(s.gafferPathwayEdit))
	mux.HandleFunc("DELETE /api/v2/gaffer/pathways/{id}", access(s.gafferPathwayDelete))
}

func gafferPathwayError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrGafferPathwayChanged):
		writeErrCode(w, http.StatusConflict, "pathway_changed", "", err.Error(), "")
	case errors.Is(err, store.ErrGafferPathwayLimit):
		writeErrCode(w, http.StatusConflict, "pathway_limit", "", err.Error(), "")
	case errors.Is(err, store.ErrGafferPathwayName):
		writeErrCode(w, http.StatusConflict, "pathway_name", "", err.Error(), "")
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "That pathway is not available.")
	default:
		gafferDeliveryError(w, err)
	}
}

// pathwayView is a pathway with what would refuse a start on it now.
type pathwayView struct {
	store.GafferPathway
	Problems []string `json:"problems"`
}

func (s *server) pathwayView(uid int64, p store.GafferPathway) pathwayView {
	v := pathwayView{p, s.gafferProviders.PathwayProblems(p, s.templateSkills(uid))}
	if v.Problems == nil {
		v.Problems = []string{}
	}
	return v
}

func (s *server) gafferPathways(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	rows, err := s.gafferProviders.Pathways(uid)
	if err != nil {
		gafferPathwayError(w, err)
		return
	}
	out := make([]pathwayView, len(rows))
	for i, p := range rows {
		out[i] = s.pathwayView(uid, p)
	}
	writeJSON(w, 200, map[string]any{"pathways": out})
}

func (s *server) gafferPathway(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	p, err := s.gafferProviders.Pathway(uid, r.PathValue("id"))
	if err != nil {
		gafferPathwayError(w, err)
		return
	}
	writeJSON(w, 200, s.pathwayView(uid, p))
}

func (s *server) gafferPathwayCreate(w http.ResponseWriter, r *http.Request) {
	var in store.GafferPathway
	if !decodeStrictJSON(w, r, 64<<10, &in, "That pathway is invalid or too large.") {
		return
	}
	uid := s.uid(r)
	in.ID, in.CreatedAt, in.UpdatedAt = "", 0, 0
	saved, err := s.gafferProviders.SavePathway(uid, in, 0)
	if err != nil {
		gafferPathwayError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pathway-saved", "", map[string]any{"pathway": saved.ID})
	writeJSON(w, 201, s.pathwayView(uid, saved))
}

func (s *server) gafferPathwayEdit(w http.ResponseWriter, r *http.Request) {
	var in store.GafferPathway
	if !decodeStrictJSON(w, r, 64<<10, &in, "That pathway is invalid or too large.") {
		return
	}
	uid := s.uid(r)
	readAt := in.UpdatedAt
	if readAt < 1 { // a save with no read time would make a pathway under the client's id
		gafferPathwayError(w, store.ErrGafferPathwayChanged)
		return
	}
	in.ID = r.PathValue("id")
	saved, err := s.gafferProviders.SavePathway(uid, in, readAt)
	if err != nil {
		gafferPathwayError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pathway-saved", "", map[string]any{"pathway": saved.ID})
	writeJSON(w, 200, s.pathwayView(uid, saved))
}

func (s *server) gafferPathwayDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.gafferProviders.DeletePathway(s.uid(r), id); err != nil {
		gafferPathwayError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-pathway-deleted", "", map[string]any{"pathway": id})
	w.WriteHeader(http.StatusNoContent)
}
