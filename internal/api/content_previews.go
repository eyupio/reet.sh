package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/eyupio/reet.sh/internal/skillpreview"
	"github.com/eyupio/reet.sh/internal/store"
)

// previewProtocol is the connector channel version. It is negotiated before a
// connector reads any file, and is independent of import and observation channels.
const previewProtocol = 1

// Browser routes ask for a metadata-only look at one machine for one owned pin;
// connector routes let an enrolled machine claim that work and answer it. Neither
// can carry file bytes, choose a filesystem path or authorise a write.
func (s *server) registerSkillContentPreviews(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/skill-content-previews", s.privateSkillAccess(s.skillContentPreviews))
	mux.HandleFunc("POST /api/v2/skill-content-previews", s.privateSkillAccess(s.skillContentPreviewCreate))
	mux.HandleFunc("GET /api/v2/skill-content-previews/{id}", s.privateSkillAccess(s.skillContentPreviewGet))
	mux.HandleFunc("POST /api/v2/skill-content-previews/{id}/cancel", s.privateSkillAccess(s.skillContentPreviewCancel))
	mux.HandleFunc("DELETE /api/v2/skill-content-previews/{id}", s.privateSkillAccess(s.skillContentPreviewDismiss))
	mux.HandleFunc("POST /api/v2/sync/skill-content-preview/work", s.privateSkillAccess(s.connectorPreviewWork))
	mux.HandleFunc("POST /api/v2/sync/skill-content-preview/report", s.privateSkillAccess(s.connectorPreviewReport))
	mux.HandleFunc("POST /api/v2/sync/skill-content-preview/failure", s.privateSkillAccess(s.connectorPreviewFailure))
}

func skillContentPreviewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrSkillContentPreviewInvalid):
		writeErr(w, 400, "Supply a request key, an owned pin and the pin's current version.")
	case errors.Is(err, store.ErrSkillContentPreviewMissing), errors.Is(err, store.ErrSkillContentTargetMissing):
		writeErr(w, 404, "Content preview unavailable.")
	case errors.Is(err, store.ErrSkillContentPreviewCapacity):
		writeErr(w, 409, "Content preview history is full (64 requests). Dismiss finished previews, then try again.")
	case errors.Is(err, store.ErrSkillContentPreview), errors.Is(err, store.ErrSkillContentTargetConflict), errors.Is(err, store.ErrSkillContentTargetScope),
		errors.Is(err, store.ErrContentObservationStale), errors.Is(err, store.ErrRemoteNotFound):
		writeErr(w, 409, "This preview is unavailable, blocked, changed or expired. Refresh the machine, check the pin, and request a new preview.")
	default:
		privateSkillError(w, err)
	}
}

func decodePreview(w http.ResponseWriter, r *http.Request, limit int64, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		writeErr(w, 400, "Invalid or oversized content preview request.")
		return false
	}
	return true
}

func (s *server) skillContentPreviews(w http.ResponseWriter, r *http.Request) {
	previews, capabilities, err := s.st.SkillContentPreviewsFor(s.uid(r))
	if err != nil {
		skillContentPreviewError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"previews": previews, "capabilities": capabilities})
}

func (s *server) skillContentPreviewGet(w http.ResponseWriter, r *http.Request) {
	out, err := s.st.SkillContentPreviewFor(s.uid(r), r.PathValue("id"))
	if err != nil {
		skillContentPreviewError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *server) skillContentPreviewCreate(w http.ResponseWriter, r *http.Request) {
	var in store.SkillContentPreviewCreate
	if !decodePreview(w, r, 16<<10, &in) {
		return
	}
	out, err := s.st.CreateSkillContentPreviewFor(s.uid(r), in)
	if err != nil {
		skillContentPreviewError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *server) skillContentPreviewCancel(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !decodePreview(w, r, 4096, &in) {
		return
	}
	uid, id := s.uid(r), r.PathValue("id")
	if err := s.st.StopSkillContentPreviewFor(uid, id); err != nil {
		skillContentPreviewError(w, err)
		return
	}
	out, err := s.st.SkillContentPreviewFor(uid, id)
	if err != nil {
		skillContentPreviewError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *server) skillContentPreviewDismiss(w http.ResponseWriter, r *http.Request) {
	if _, err := s.st.DismissSkillContentPreviewFor(s.uid(r), r.PathValue("id")); err != nil {
		skillContentPreviewError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) connectorPreviewWork(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in struct {
		SchemaVersion int    `json:"schemaVersion"`
		Instance      string `json:"instanceId"`
	}
	if !decodePreview(w, r, 4096, &in) {
		return
	}
	if in.SchemaVersion != previewProtocol {
		writeErr(w, 400, "Unsupported content preview protocol.")
		return
	}
	work, err := s.st.ClaimSkillContentPreviewFor(s.uid(r), a.ID, in.Instance)
	if err != nil {
		skillContentPreviewError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schemaVersion": previewProtocol, "work": work})
}

func (s *server) connectorPreviewReport(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in struct {
		ID       string               `json:"id"`
		Instance string               `json:"instanceId"`
		Capture  skillpreview.Capture `json:"capture"`
	}
	if !decodePreview(w, r, 1<<20, &in) {
		return
	}
	if err := s.st.RecordSkillContentCaptureFor(s.uid(r), a.ID, in.ID, in.Instance, in.Capture); err != nil {
		skillContentPreviewError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"recorded": true})
}

func (s *server) connectorPreviewFailure(w http.ResponseWriter, r *http.Request) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	var in struct {
		ID       string `json:"id"`
		Instance string `json:"instanceId"`
		Code     string `json:"code"`
	}
	if !decodePreview(w, r, 4096, &in) {
		return
	}
	if err := s.st.FailSkillContentPreviewFor(s.uid(r), a.ID, in.ID, in.Instance, in.Code); err != nil {
		skillContentPreviewError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"recorded": true})
}
