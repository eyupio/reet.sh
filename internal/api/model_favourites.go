package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// maxModelFavouritesBodyBytes bounds one favourites request: 50 references
// with a tool and a 100-byte model each fit well inside it.
const maxModelFavouritesBodyBytes = 16 << 10

func (s *server) registerModelFavouriteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/profile/model-favourites", s.modelFavouritesList)
	mux.HandleFunc("POST /api/v2/profile/model-favourites", s.requireWriter(s.modelFavouriteAdd))
	mux.HandleFunc("DELETE /api/v2/profile/model-favourites", s.requireWriter(s.modelFavouriteRemove))
	mux.HandleFunc("PUT /api/v2/profile/model-favourites/order", s.requireWriter(s.modelFavouritesOrder))
	mux.HandleFunc("PUT /api/v2/profile/model-favourites/default", s.requireWriter(s.modelFavouriteDefault))
}

func (s *server) modelFavouritesList(w http.ResponseWriter, r *http.Request) {
	out, err := s.st.ModelFavouritesFor(s.uid(r))
	if err != nil {
		modelFavouriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) modelFavouriteAdd(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Tool  string `json:"tool"`
		Model string `json:"model"`
	}
	if !decodeStrictJSON(w, r, maxModelFavouritesBodyBytes, &in, "Invalid model favourite request.") {
		return
	}
	if !validFavouritePair(w, in.Tool, in.Model) {
		return
	}
	out, created, err := s.st.AddModelFavouriteFor(s.uid(r), in.Tool, in.Model)
	if err != nil {
		modelFavouriteError(w, err)
		return
	}
	if created {
		s.event(s.uid(r), "ui", "model-favourite-added", "", map[string]any{"tool": in.Tool, "model": in.Model})
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, out)
}

func (s *server) modelFavouriteRemove(w http.ResponseWriter, r *http.Request) {
	q, ok := machineQuery(r, "tool", "model")
	if !ok {
		writeErrCode(w, http.StatusBadRequest, "invalid_request", "", "That request isn't a model favourite reet can read.", "")
		return
	}
	tool, model := q.Get("tool"), q.Get("model")
	if !validFavouritePair(w, tool, model) {
		return
	}
	_, removed, err := s.st.RemoveModelFavouriteFor(s.uid(r), tool, model)
	if err != nil {
		modelFavouriteError(w, err)
		return
	}
	if removed {
		s.event(s.uid(r), "ui", "model-favourite-removed", "", map[string]any{"tool": tool, "model": model})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) modelFavouritesOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version int64                     `json:"version"`
		Items   []store.ModelFavouriteRef `json:"items"`
	}
	if !decodeStrictJSON(w, r, maxModelFavouritesBodyBytes, &in, "Invalid model favourite order request.") {
		return
	}
	for _, item := range in.Items {
		if !validFavouritePair(w, item.Tool, item.Model) {
			return
		}
	}
	out, err := s.st.ReorderModelFavouritesFor(s.uid(r), in.Version, in.Items)
	if err != nil {
		modelFavouriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) modelFavouriteDefault(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Tool  string `json:"tool"`
		Model string `json:"model"`
	}
	if !decodeStrictJSON(w, r, maxModelFavouritesBodyBytes, &in, "Invalid model favourite default request.") {
		return
	}
	if !terminalproto.ValidToolV2(in.Tool) {
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_tool", "tool", "Choose one of the agents reet can start.", "")
		return
	}
	if in.Model != "" && !terminalproto.ValidModel(in.Model) {
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_model", "model", sentenceModel, "")
		return
	}
	out, changed, err := s.st.SetModelFavouriteDefaultFor(s.uid(r), in.Tool, in.Model)
	if err != nil {
		modelFavouriteError(w, err)
		return
	}
	if changed {
		s.event(s.uid(r), "ui", "model-favourite-default", "", map[string]any{"tool": in.Tool, "model": in.Model})
	}
	writeJSON(w, http.StatusOK, out)
}

// validFavouritePair refuses a tool or model the store would reject, naming
// the field so a picker can point at it.
func validFavouritePair(w http.ResponseWriter, tool, model string) bool {
	if !terminalproto.ValidToolV2(tool) {
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_tool", "tool", "Choose one of the agents reet can start.", "")
		return false
	}
	if !terminalproto.ValidModel(model) {
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_model", "model", sentenceModel, "")
		return false
	}
	return true
}

func modelFavouriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrModelFavouritesFull):
		writeErrCode(w, http.StatusConflict, "favourites_full", "", "You have starred the most models reet keeps. Unstar one first.", "")
	case errors.Is(err, store.ErrModelFavouriteNotFound):
		writeErrCode(w, http.StatusNotFound, "favourite_not_found", "", "That model isn't one of your favourites.", "")
	case errors.Is(err, store.ErrModelFavouritesStale):
		writeErrCode(w, http.StatusConflict, "favourites_stale", "", "Your favourites changed. Reload them, then try again.", "")
	case errors.Is(err, terminalproto.ErrInvalid):
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_model", "", "That model favourite can't be saved.", "")
	default:
		log.Printf("model favourites: store error: %v", err)
		writeErrCode(w, http.StatusInternalServerError, "server_error", "", "reet couldn't do that just now. Try again.", "")
	}
}
