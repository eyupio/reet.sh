package api

import (
	"github.com/eyupio/reet.sh/internal/store"
	"net/http"
)

func (s *server) groups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.st.GroupsFor(s.uid(r), s.role(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"groups": groups})
}
func (s *server) createGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		ParentID int64  `json:"parentId"`
	}
	if !decode(w, r, &in) {
		return
	}
	g, err := s.st.CreateGroup(s.uid(r), s.role(r), in.Name, in.Kind, in.ParentID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "group-create", "", map[string]any{"id": g.ID, "kind": g.Kind})
	s.groups(w, r)
}
func (s *server) groupMember(w http.ResponseWriter, r *http.Request) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErr(w, 404, "group not found")
		return
	}
	var in struct {
		Email  string `json:"email"`
		Role   string `json:"role"`
		Remove bool   `json:"remove"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.SetGroupMember(s.uid(r), s.role(r), id, in.Email, in.Role, in.Remove); err != nil {
		if err == store.ErrNotFound {
			writeErr(w, 404, "group not found")
		} else {
			writeErr(w, 400, err.Error())
		}
		return
	}
	s.event(s.uid(r), "ui", "group-membership", "", map[string]any{"group": id, "removed": in.Remove})
	s.groups(w, r)
}
func (s *server) community(w http.ResponseWriter, r *http.Request) {
	lists, skills, err := s.st.Community()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"lists": lists, "skills": skills})
}

func (s *server) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErr(w, 404, "group not found")
		return
	}
	var in struct {
		Name   string `json:"name"`
		Remove bool   `json:"remove"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.UpdateGroup(s.uid(r), s.role(r), id, in.Name, in.Remove); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.event(s.uid(r), "ui", "group-update", "", map[string]any{"group": id, "deleted": in.Remove})
	s.groups(w, r)
}
