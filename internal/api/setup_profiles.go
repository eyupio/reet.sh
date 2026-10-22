package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/eyupio/reet.sh/internal/setup"
	"github.com/eyupio/reet.sh/internal/store"
)

const maxSetupProfileBytes = 4 << 20

func (s *server) setupProfileList(w http.ResponseWriter, r *http.Request) {
	records, err := s.st.SetupProfilesFor(s.uid(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type summary struct {
		Name           string `json:"name"`
		Revision       int    `json:"revision"`
		SnapshotID     string `json:"snapshotId"`
		SnapshotDigest string `json:"snapshotDigest"`
		UpdatedAt      int64  `json:"updatedAt"`
	}
	out := make([]summary, 0, len(records))
	for _, record := range records {
		out = append(out, summary{Name: record.Name, Revision: record.Revision, SnapshotID: record.SnapshotID, SnapshotDigest: record.SnapshotDigest, UpdatedAt: record.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": out})
}

func (s *server) getSetupProfile(w http.ResponseWriter, r *http.Request) {
	record, ok, err := s.st.SetupProfileFor(s.uid(r), r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "setup profile not found")
		return
	}
	var profile setup.SetupProfile
	if err := json.Unmarshal([]byte(record.Payload), &profile); err != nil {
		writeErr(w, http.StatusInternalServerError, "stored setup profile is invalid")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *server) saveSetupProfile(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSetupProfileBytes+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read setup profile")
		return
	}
	if len(body) > maxSetupProfileBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "setup profile is too large")
		return
	}
	profile, err := setup.ParseSetupProfile(body, r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	latest := profile.Revisions[len(profile.Revisions)-1]
	record := store.SetupProfileRecord{
		UserID: s.uid(r), Name: profile.Name, Revision: latest.Revision,
		SnapshotID: latest.SnapshotID, SnapshotDigest: latest.SnapshotDigest,
		Payload: string(body),
	}
	if err := s.st.SaveSetupProfile(record); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventRequest(r, "setup-profile-save", "", map[string]string{"profile": profile.Name, "snapshot": latest.SnapshotID})
	writeJSON(w, http.StatusOK, profile)
}

func (s *server) deleteSetupProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok, err := s.st.SetupProfileFor(s.uid(r), name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeErr(w, http.StatusNotFound, "setup profile not found")
		return
	}
	if err := s.st.DeleteSetupProfileFor(s.uid(r), name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventRequest(r, "setup-profile-delete", "", map[string]string{"profile": name})
	writeJSON(w, http.StatusOK, map[string]string{"deleted": name})
}
