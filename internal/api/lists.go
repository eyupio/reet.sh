package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
)

func listIDFrom(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// Link sharing is a public data surface, so it is off until the owner turns it on.
func (s *server) linkListsEnabled() bool {
	v, _ := s.st.GetSetting("link_lists_enabled")
	return v == "1"
}

func (s *server) listAccess(w http.ResponseWriter, r *http.Request) (store.ListAccess, bool) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "list not found")
		return store.ListAccess{}, false
	}
	access, err := s.st.ListAccessFor(s.uid(r), s.role(r), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "list not found")
		return store.ListAccess{}, false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return store.ListAccess{}, false
	}
	return access, true
}

func (s *server) writeListDetail(w http.ResponseWriter, r *http.Request, id int64) {
	d, err := s.st.ListDetailFor(s.uid(r), s.role(r), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "list not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.LinkEnabled = s.linkListsEnabled()
	writeJSON(w, http.StatusOK, d)
}

func (s *server) listLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.st.ListListsFor(s.uid(r), s.role(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lists":       lists,
		"userId":      s.uid(r),
		"linkEnabled": s.linkListsEnabled(),
	})
}

func (s *server) createList(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Visibility  string         `json:"visibility"`
		Audience    store.Audience `json:"audience"`
	}
	if !decode(w, r, &in) {
		return
	}
	visibility := strings.TrimSpace(in.Visibility)
	if visibility == "" {
		visibility = "personal"
	}
	if visibility == "link" && !s.linkListsEnabled() {
		writeErr(w, http.StatusForbidden, "link sharing is turned off by the owner")
		return
	}
	list, err := s.st.SaveAudienceList(s.uid(r), s.role(r), 0, in.Name, in.Description, visibility, in.Audience)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eventRequest(r, "list-create", "", map[string]any{"list": list.Name, "id": list.ID, "visibility": list.Visibility})
	if visibility == "team" && in.Audience.GroupID != 0 {
		s.notifyListShared(r, in.Audience.GroupID, list.ID, list.Name, list.UpdatedAt)
	}
	s.writeListDetail(w, r, list.ID)
}

func (s *server) listDetail(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	s.writeListDetail(w, r, access.List.ID)
}

func (s *server) updateList(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanManage {
		writeErr(w, http.StatusForbidden, "the list owner or an admin can change these settings")
		return
	}
	var in struct {
		Name        *string         `json:"name"`
		Description *string         `json:"description"`
		Visibility  *string         `json:"visibility"`
		Audience    *store.Audience `json:"audience"`
	}
	if !decode(w, r, &in) {
		return
	}
	name, description, visibility := access.List.Name, access.List.Description, access.List.Visibility
	if in.Name != nil {
		name = *in.Name
	}
	if in.Description != nil {
		description = *in.Description
	}
	if in.Visibility != nil {
		visibility = *in.Visibility
	}
	if visibility == "link" && access.List.Visibility != "link" && !s.linkListsEnabled() {
		writeErr(w, http.StatusForbidden, "link sharing is turned off by the owner")
		return
	}
	oldVisibility := access.List.Visibility
	audience, err := s.st.AudienceFor(access.List.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	beforeAudience := audience
	if in.Audience != nil {
		audience = *in.Audience
	} else if visibility != access.List.Visibility {
		if visibility != "team" {
			audience.GroupID = 0
		}
		if visibility != "private" && visibility != "team" {
			audience.Excluded = nil
		}
		if visibility == "personal" {
			audience.People = nil
		}
	}
	if _, err := s.st.SaveAudienceList(s.uid(r), s.role(r), access.List.ID, name, description, visibility, audience); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eventRequest(r, "list-update", "", map[string]any{"list": name, "id": access.List.ID})
	if visibility == "team" && audience.GroupID != 0 && (oldVisibility != "team" || beforeAudience.GroupID != audience.GroupID) {
		if updated, err := s.st.SharedListByID(access.List.ID); err == nil {
			s.notifyListShared(r, audience.GroupID, updated.ID, updated.Name, updated.UpdatedAt)
		}
	}
	s.writeListDetail(w, r, access.List.ID)
}

func (s *server) deleteList(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanManage {
		writeErr(w, http.StatusForbidden, "the list owner or an admin can delete it")
		return
	}
	if err := s.st.DeleteList(access.List.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventRequest(r, "list-delete", "", map[string]any{"list": access.List.Name, "id": access.List.ID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) addListSkill(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanEdit {
		writeErr(w, http.StatusForbidden, "you can read this list but not change it")
		return
	}
	var in listSkillInput
	if !decode(w, r, &in) {
		return
	}
	sk, resolveErr := s.resolveListSkill(r, access.List, in)
	if resolveErr != nil {
		code := http.StatusBadRequest
		if resolveErr.code == errSkillNotFound || resolveErr.code == errEnvironmentNotFound {
			code = http.StatusNotFound
		}
		writeErrCode(w, code, resolveErr.code, resolveErr.field, resolveErr.message, resolveErr.hint)
		return
	}
	if err := s.st.AddListSkill(access.List.ID, sk.ID, in.Note, s.uid(r)); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eventRequest(r, "list-skill-add", sk.ID, map[string]any{"list": access.List.Name, "id": access.List.ID})
	s.writeListDetail(w, r, access.List.ID)
}

func (s *server) updateListSkill(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanEdit {
		writeErr(w, http.StatusForbidden, "you can read this list but not change it")
		return
	}
	var in struct {
		ID   string `json:"id"`
		Note string `json:"note"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.UpdateListSkillNote(access.List.ID, strings.TrimSpace(in.ID), in.Note); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "that skill is not on this list")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eventRequest(r, "list-note", in.ID, map[string]any{"list": access.List.Name, "id": access.List.ID})
	s.writeListDetail(w, r, access.List.ID)
}

func (s *server) removeListSkill(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanEdit {
		writeErr(w, http.StatusForbidden, "you can read this list but not change it")
		return
	}
	skillID := strings.TrimSpace(r.URL.Query().Get("id"))
	if skillID == "" {
		writeErr(w, http.StatusBadRequest, "choose a skill")
		return
	}
	if err := s.st.RemoveListSkill(access.List.ID, skillID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "that skill is not on this list")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventRequest(r, "list-skill-remove", skillID, map[string]any{"list": access.List.Name, "id": access.List.ID})
	s.writeListDetail(w, r, access.List.ID)
}

func (s *server) addListMember(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanManage {
		writeErr(w, http.StatusForbidden, "the list owner or an admin can add people")
		return
	}
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	user, ok, err := s.st.UserByEmail(store.NormalizeListEmail(in.Email))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "that person does not have an account here yet")
		return
	}
	if user.DisabledAt != 0 {
		writeErr(w, http.StatusBadRequest, "that account is disabled")
		return
	}
	if user.Role == "viewer" && strings.TrimSpace(in.Role) == "editor" {
		writeErr(w, http.StatusBadRequest, "viewers are read-only; add them as a viewer")
		return
	}
	if err := s.st.AddListMember(access.List.ID, user.ID, in.Role); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eventRequest(r, "list-member-add", "", map[string]any{"list": access.List.Name, "id": access.List.ID, "member": user.Name})
	s.writeListDetail(w, r, access.List.ID)
}

func (s *server) updateListMember(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanManage {
		writeErr(w, http.StatusForbidden, "the list owner or an admin can change roles")
		return
	}
	var in struct {
		UserID int64  `json:"userId"`
		Role   string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	user, ok, err := s.st.UserByID(in.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "that person does not have an account here yet")
		return
	}
	if user.Role == "viewer" && strings.TrimSpace(in.Role) == "editor" {
		writeErr(w, http.StatusBadRequest, "viewers are read-only; keep them as a viewer")
		return
	}
	if err := s.st.AddListMember(access.List.ID, in.UserID, in.Role); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eventRequest(r, "list-member-role", "", map[string]any{"list": access.List.Name, "id": access.List.ID, "userId": in.UserID, "role": in.Role})
	s.writeListDetail(w, r, access.List.ID)
}

func (s *server) removeListMember(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanManage {
		writeErr(w, http.StatusForbidden, "the list owner or an admin can remove people")
		return
	}
	userID, err := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
	if err != nil || userID <= 0 {
		writeErr(w, http.StatusBadRequest, "choose a person")
		return
	}
	if err := s.st.RemoveListMember(access.List.ID, userID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventRequest(r, "list-member-remove", "", map[string]any{"list": access.List.Name, "id": access.List.ID, "userId": userID})
	s.writeListDetail(w, r, access.List.ID)
}

func (s *server) transferList(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanTransfer {
		writeErr(w, http.StatusForbidden, "the list owner can hand it over")
		return
	}
	var in struct {
		UserID int64 `json:"userId"`
	}
	if !decode(w, r, &in) {
		return
	}
	user, ok, err := s.st.UserByID(in.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "that person does not have an account here yet")
		return
	}
	if user.Role == "viewer" {
		writeErr(w, http.StatusBadRequest, "a viewer cannot own a list")
		return
	}
	if err := s.st.TransferList(access.List.ID, in.UserID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eventRequest(r, "list-transfer", "", map[string]any{"list": access.List.Name, "id": access.List.ID, "userId": in.UserID})
	s.writeListDetail(w, r, access.List.ID)
}

func (s *server) addListToShelf(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	n, err := s.st.AddListToShelf(s.uid(r), access.List.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventRequest(r, "list-add-shelf", "", map[string]any{"list": access.List.Name, "id": access.List.ID, "count": n})
	writeJSON(w, http.StatusOK, map[string]any{"added": n})
}

func (s *server) publicList(w http.ResponseWriter, r *http.Request) {

	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		writeErr(w, http.StatusNotFound, "list not found")
		return
	}
	d, err := s.st.PublicListFor(slug)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "list not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d.Visibility == "link" && !s.linkListsEnabled() {
		writeErr(w, 404, "list not found")
		return
	}
	d.LinkEnabled = true
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, d)
}
