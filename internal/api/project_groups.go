package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
)

// Project groups: named, coloured collections the rail can fold and reorder.
// A project belongs to at most one group and keeps its own place in the rail,
// so a group is an arrangement, not a container that owns the project.

const sentenceGroupName = "Give the group a name of 1 to 80 characters, without line breaks or control characters."

func (s *server) registerProjectGroupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/project-groups", s.folderAccess(s.projectGroupList))
	mux.HandleFunc("POST /api/v2/project-groups", s.folderAccess(s.folderProjectWrite(s.projectGroupCreate)))
	mux.HandleFunc("POST /api/v2/project-groups/reorder", s.folderAccess(s.folderProjectWrite(s.projectGroupReorder)))
	mux.HandleFunc("PATCH /api/v2/project-groups/{id}", s.folderAccess(s.folderProjectWrite(s.projectGroupUpdate)))
	mux.HandleFunc("DELETE /api/v2/project-groups/{id}", s.folderAccess(s.folderProjectWrite(s.projectGroupDelete)))
	mux.HandleFunc("POST /api/v2/folder-projects/{id}/group", s.folderAccess(s.folderProjectWrite(s.folderProjectSetGroup)))
}

type projectGroupInput struct {
	Name   *string `json:"name"`
	Colour *string `json:"colour"`
}

func (in projectGroupInput) apply(g store.ProjectGroup) store.ProjectGroup {
	if in.Name != nil {
		g.Name = *in.Name
	}
	if in.Colour != nil {
		g.Colour = *in.Colour
	}
	return g
}

type projectGroupView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Colour    string `json:"colour"`
	Position  int    `json:"position"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	Version   int64  `json:"version"`
}

func viewProjectGroup(g store.ProjectGroup) projectGroupView {
	return projectGroupView{ID: g.ID, Name: g.Name, Colour: g.Colour, Position: g.Position, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt, Version: g.Version}
}

func (s *server) writeProjectGroups(w http.ResponseWriter, uid int64) {
	groups, err := s.st.ListProjectGroupsFor(uid)
	if err != nil {
		projectGroupRefusal(err).write(w)
		return
	}
	out := make([]projectGroupView, 0, len(groups))
	for _, g := range groups {
		out = append(out, viewProjectGroup(g))
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

func projectGroupRefusal(err error) *projectRefusal {
	switch {
	case errors.Is(err, store.ErrProjectGroupNotFound):
		return &projectRefusal{404, "group_not_found", "", "That group isn't on this account.", ""}
	case errors.Is(err, store.ErrProjectGroupChanged):
		return &projectRefusal{409, "group_changed", "", "Someone changed this group. Reload it, then try again.", ""}
	case errors.Is(err, store.ErrProjectGroupName):
		return &projectRefusal{409, "group_name_taken", "name", "Another group on this account already has that name.", ""}
	case errors.Is(err, store.ErrProjectGroupLimit):
		return &projectRefusal{409, "group_limit", "", "You have the most groups reet keeps. Remove one first.", ""}
	case errors.Is(err, store.ErrProjectGroupInvalid):
		return &projectRefusal{422, "invalid_group", "", "Something in that group can't be saved. Check each field and try again.", ""}
	case errors.Is(err, store.ErrProjectGroupForbidden):
		return &projectRefusal{422, "invalid_group", "groupId", "Choose one of your groups.", ""}
	}
	log.Printf("project groups: store error: %v", err)
	return &projectRefusal{500, "server_error", "", "reet couldn't do that just now. Try again.", ""}
}

// checkGroupShape refuses a name or colour with a precise field code.
func checkGroupShape(g *store.ProjectGroup) *projectRefusal {
	if g.Name == "" || !folderNameShape(g.Name) {
		return &projectRefusal{422, "invalid_name", "name", sentenceGroupName, ""}
	}
	if !validProjectColour(g.Colour) {
		return &projectRefusal{422, "invalid_colour", "colour", "Choose a colour from the palette, or no colour.", ""}
	}
	return nil
}

func (s *server) projectGroupList(w http.ResponseWriter, r *http.Request) {
	s.writeProjectGroups(w, s.uid(r))
}

func (s *server) projectGroupCreate(w http.ResponseWriter, r *http.Request) {
	var in projectGroupInput
	if !folderProjectBody(w, r, &in) {
		return
	}
	g := in.apply(store.ProjectGroup{})
	if refusal := checkGroupShape(&g); refusal != nil {
		refusal.write(w)
		return
	}
	uid := s.uid(r)
	saved, err := s.st.CreateProjectGroupFor(uid, g)
	if err != nil {
		projectGroupRefusal(err).write(w)
		return
	}
	w.Header().Set("ETag", projectETag(saved.Version))
	writeJSON(w, http.StatusCreated, viewProjectGroup(saved))
}

func (s *server) projectGroupUpdate(w http.ResponseWriter, r *http.Request) {
	var in projectGroupInput
	if !folderProjectBody(w, r, &in) {
		return
	}
	expect, ok := ifMatch(r)
	if !ok {
		folderVersionRefused(w)
		return
	}
	uid := s.uid(r)
	current, err := s.st.ProjectGroupFor(uid, r.PathValue("id"))
	if err != nil {
		projectGroupRefusal(err).write(w)
		return
	}
	g := in.apply(current)
	if refusal := checkGroupShape(&g); refusal != nil {
		refusal.write(w)
		return
	}
	saved, err := s.st.UpdateProjectGroupFor(uid, g, expect)
	if err != nil {
		projectGroupRefusal(err).write(w)
		return
	}
	w.Header().Set("ETag", projectETag(saved.Version))
	writeJSON(w, http.StatusOK, viewProjectGroup(saved))
}

func (s *server) projectGroupDelete(w http.ResponseWriter, r *http.Request) {
	expect, ok := ifMatch(r)
	if !ok {
		folderVersionRefused(w)
		return
	}
	uid := s.uid(r)
	if err := s.st.DeleteProjectGroupFor(uid, r.PathValue("id"), expect); err != nil {
		projectGroupRefusal(err).write(w)
		return
	}
	s.writeProjectGroups(w, uid)
}

func (s *server) projectGroupReorder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs      []string `json:"ids"`
		Versions []int64  `json:"versions"`
	}
	if !folderProjectBody(w, r, &in) {
		return
	}
	if in.IDs == nil || len(in.IDs) != len(in.Versions) || len(in.IDs) > store.MaxProjectGroups {
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_order", "ids", "List every group once, each with the version you read.", "")
		return
	}
	uid := s.uid(r)
	if err := s.st.ReorderProjectGroupsFor(uid, in.IDs, in.Versions); err != nil {
		if errors.Is(err, store.ErrProjectGroupInvalid) {
			writeErrCode(w, http.StatusUnprocessableEntity, "invalid_order", "ids", "List every group once, each with the version you read.", "")
			return
		}
		projectGroupRefusal(err).write(w)
		return
	}
	s.writeProjectGroups(w, uid)
}

// folderProjectSetGroup moves one project into a group, or out of every group
// when groupId is "". Its place in the rail does not change.
func (s *server) folderProjectSetGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		GroupID *string `json:"groupId"`
	}
	if !folderProjectBody(w, r, &in) {
		return
	}
	if in.GroupID == nil {
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_group", "groupId", "Name the group, or use \"\" to leave every group.", "")
		return
	}
	uid := s.uid(r)
	if err := s.st.SetFolderProjectGroupFor(uid, r.PathValue("id"), *in.GroupID); err != nil {
		projectGroupRefusal(err).write(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groupId": *in.GroupID})
}
