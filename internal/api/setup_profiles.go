package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"fmt"

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

func (s *server) renameSetupProfile(w http.ResponseWriter, r *http.Request) {
	record, ok, err := s.st.SetupProfileFor(s.uid(r), r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "setup profile not found")
		return
	}
	var request struct {
		Name      *string                      `json:"name"`
		Overrides *setup.SetupProfileOverrides `json:"overrides"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSetupProfileBytes))
	if err != nil || json.Unmarshal(body, &request) != nil {
		writeErr(w, http.StatusBadRequest, "the profile edit request is invalid")
		return
	}
	if request.Name == nil && request.Overrides == nil {
		writeErr(w, http.StatusBadRequest, "name a new profile name or overrides")
		return
	}
	if request.Name != nil && request.Overrides != nil {
		writeErr(w, http.StatusBadRequest, "edit the name or overrides, not both")
		return
	}
	var profile setup.SetupProfile
	if err := json.Unmarshal([]byte(record.Payload), &profile); err != nil {
		writeErr(w, http.StatusInternalServerError, "stored setup profile is invalid")
		return
	}
	if request.Overrides != nil {
		if err := setup.ValidateSetupProfileOverrides(request.Overrides); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if overridesEmpty(request.Overrides) {
			request.Overrides = nil
		}
		latest := profile.Revisions[len(profile.Revisions)-1]
		next := latest
		next.Revision = len(profile.Revisions) + 1
		next.CreatedAt = time.Now().UTC()
		next.Overrides = request.Overrides
		profile.Revisions = append(profile.Revisions, next)
		payload, err := json.Marshal(profile)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := setup.ParseSetupProfile(payload, profile.Name); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.st.SaveSetupProfile(store.SetupProfileRecord{
			UserID: s.uid(r), Name: profile.Name, Revision: next.Revision,
			SnapshotID: next.SnapshotID, SnapshotDigest: next.SnapshotDigest, Payload: string(payload),
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.eventRequest(r, "setup-profile-override", "", map[string]string{"profile": profile.Name, "revision": fmt.Sprint(next.Revision)})
		writeJSON(w, http.StatusOK, profile)
		return
	}
	name := *request.Name
	if name == record.Name {
		writeJSON(w, http.StatusOK, profile)
		return
	}
	profile.Name = name
	payload, err := json.Marshal(profile)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := setup.ParseSetupProfile(payload, name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	renamed, err := s.st.RenameSetupProfileFor(s.uid(r), record.Name, name, string(payload))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !renamed {
		writeErr(w, http.StatusConflict, "a setup profile with that name already exists")
		return
	}
	s.eventRequest(r, "setup-profile-rename", "", map[string]string{"from": record.Name, "to": name})
	writeJSON(w, http.StatusOK, profile)
}

func overridesEmpty(overrides *setup.SetupProfileOverrides) bool {
	return len(overrides.ExcludeKinds) == 0 && len(overrides.ExcludeSources) == 0 && len(overrides.Machines) == 0 && len(overrides.Projects) == 0
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
