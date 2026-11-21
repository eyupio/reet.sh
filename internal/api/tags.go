package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) tags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.st.ListTagsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"tags": tags})
}

func (s *server) skillTags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     string   `json:"id"`
		Add    []string `json:"add"`
		Remove []int64  `json:"remove"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ID == "" {
		writeErr(w, 400, "id required")
		return
	}
	if len(in.Add) > 0 {
		if err := s.st.AddSkillTagsFor(s.uid(r), in.ID, in.Add); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	for _, id := range in.Remove {
		if err := s.st.RemoveSkillTagFor(s.uid(r), in.ID, id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	sk, ok, err := s.st.GetFor(s.uid(r), in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "skill not found")
		return
	}
	s.eventRequest(r, "tag", sk.ID, map[string]any{"name": sk.Name, "added": in.Add, "removed": in.Remove})
	writeJSON(w, 200, sk)
}

func (s *server) tagMany(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Add    []string `json:"add"`
		Remove []int64  `json:"remove"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validIDs(in.IDs) {
		writeErr(w, 400, "choose between 1 and 500 skills")
		return
	}
	n, err := s.st.TagManyFor(s.uid(r), in.IDs, in.Add, in.Remove)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "bulk-tag", "", map[string]any{"count": n, "added": in.Add, "removed": in.Remove})
	writeJSON(w, 200, map[string]int{"tagged": n})
}

func (s *server) renameTag(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	err := s.st.RenameTag(in.ID, in.Name)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "tag not found")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, 409, "that tag already exists")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "tag-rename", "", map[string]any{"name": in.Name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *server) mergeTags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From int64 `json:"from"`
		Into int64 `json:"into"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.MergeTags(in.From, in.Into); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "tag-merge", "", map[string]any{"from": in.From, "into": in.Into})
	writeJSON(w, 200, map[string]any{"ok": true})
}
