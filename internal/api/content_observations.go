package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/store"
)

// Only enrolled connectors may report observations; neither account credentials
// alone nor a browser can select a different host or filesystem root.
func (s *server) contentObservation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeErr(w, 400, "Observation requests do not accept query parameters.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, contentobs.MaxBody)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	var err error
	if r.URL.Path == "/api/v2/sync/content-observations/prepare" {
		var in struct{}
		if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF {
			writeErr(w, 400, "Invalid observation request.")
			return
		}
		var grant contentobs.Grant
		grant, err = s.st.PrepareContentObservationFor(s.uid(r), a.ID)
		if err == nil {
			writeJSON(w, 200, grant)
			return
		}
	} else {
		var in contentobs.Report
		if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || in.Validate() != nil {
			writeErr(w, 400, "Invalid or oversized content observation.")
			return
		}
		err = s.st.RecordContentObservationFor(s.uid(r), a.ID, in)
		if err == nil {
			writeJSON(w, 200, map[string]bool{"recorded": true})
			return
		}
	}
	if errors.Is(err, store.ErrContentObservationStale) {
		writeErr(w, 409, "Observation superseded, expired or machine paused; scan again.")
		return
	}
	if errors.Is(err, store.ErrRemoteNotFound) {
		writeErr(w, 403, "Observation unavailable for this enrollment.")
		return
	}
	writeErr(w, 500, "Could not store content observation.")
}
