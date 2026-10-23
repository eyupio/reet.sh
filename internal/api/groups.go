package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/store"
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
	s.eventRequest(r, "group-create", "", map[string]any{"id": g.ID, "kind": g.Kind})
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
	// Read the group and person first: the notice says what actually changed,
	// and the store has already forgotten a removed role by the time it returns.
	group, groupErr := s.st.GroupFor(s.uid(r), s.role(r), id)
	email := store.NormalizeListEmail(in.Email)
	oldRole := ""
	for _, m := range group.Members {
		if strings.EqualFold(m.Email, email) {
			oldRole = m.Role
			break
		}
	}
	person, personOK, _ := s.st.UserByEmail(email)
	if err := s.st.SetGroupMember(s.uid(r), s.role(r), id, in.Email, in.Role, in.Remove); err != nil {
		if err == store.ErrNotFound {
			writeErr(w, 404, "group not found")
		} else {
			writeErr(w, 400, err.Error())
		}
		return
	}
	s.eventRequest(r, "group-membership", "", map[string]any{"group": id, "removed": in.Remove})
	if groupErr == nil && personOK {
		action, role := "member_joined", in.Role
		switch {
		case in.Remove && oldRole != "":
			action, role = "member_left", oldRole
		case in.Remove:
			personOK = false
		case oldRole != "" && oldRole != in.Role:
			action = "member_role"
		case oldRole != "":
			personOK = false
		}
		if personOK {
			s.notifyTeam(r, id, action, fmt.Sprintf("%d:%s:%d", person.ID, action, time.Now().Unix()),
				mail.Data{Name: person.Name, Actor: s.currentUser(r).Name, Role: role, When: time.Now()})
		}
	}
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
	s.eventRequest(r, "group-update", "", map[string]any{"group": id, "deleted": in.Remove})
	s.groups(w, r)
}
