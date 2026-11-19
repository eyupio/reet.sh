package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

type installErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// cleanHostedPart is one part of a reet-hosted skill's name: a username or a
// slug, never a path.
func cleanHostedPart(part string) bool {
	return part != "" && !strings.ContainsAny(part, `/\`) && !strings.Contains(part, "..")
}

// terminalSkillInstall is the session picker's Add to your skills. It makes a
// reet-hosted skill Active in the library, which puts it on every machine, the
// session's included; nothing is queued for one machine. "reet:SLUG" is one of
// the person's own written skills, and an Archived one is restored.
// "reet:USERNAME/SLUG" is a skill someone published on this reet: it is added
// Active, and reet copies its files in.
func (s *server) terminalSkillInstall(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Skill string `json:"skill"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	name, ok := strings.CutPrefix(in.Skill, "reet:")
	username, slug, foreign := strings.Cut(name, "/")
	if !foreign {
		slug = username
	}
	if !ok || !cleanHostedPart(slug) || foreign && !cleanHostedPart(username) {
		writeErr(w, 400, "invalid terminal request")
		return
	}
	uid := s.uid(r)
	session, _, host, err := s.sessionMachine(uid, r.PathValue("id"))
	if err != nil {
		terminalError(w, err)
		return
	}
	missing := installErrorBody{Error: "This skill is no longer available. Refresh the list.", Code: "missing"}
	if foreign {
		mine, err := s.st.PublishingUsername(uid)
		if err != nil {
			terminalError(w, err)
			return
		}
		foreign = !strings.EqualFold(mine, username)
	}
	added := false
	if foreign {
		if _, err := s.st.PublishedHostedSkill(username, slug); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, missing)
			return
		}
		rf, err := ref.ParseAt(requestOrigin(r)+"/"+username+"/"+slug, r.Host)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, missing)
			return
		}
		live, err := s.st.LiveLibrarySkillFor(uid, "", rf.ID)
		if err != nil {
			terminalError(w, err)
			return
		}
		// An item the scan was told to ignore would be dropped from what
		// machines report: take it off that list first, or add nothing.
		if err = s.st.UnignoreFor(uid, rf.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, installErrorBody{Error: "The skill could not be added. Try again.", Code: "internal"})
			return
		}
		sk, err := s.st.AddFor(uid, rf, store.Patch{})
		if errors.Is(err, store.ErrSkillNameTaken) {
			writeJSON(w, http.StatusConflict, installErrorBody{Error: "A skill called " + slug + " is already in your library.", Code: "slug-conflict"})
			return
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, installErrorBody{Error: "The skill could not be added. Try again.", Code: "internal"})
			return
		}
		added = !live
		// AddFor keeps an Archived item Archived: bring it back, as for a
		// written skill, so it goes on every machine again.
		if live && sk.Status == store.SkillArchived && sk.LibraryItemID != "" {
			if err = s.st.SetSkillStatusFor(uid, []string{sk.LibraryItemID}, store.SkillActive, s.skillNow()); err != nil {
				writeJSON(w, http.StatusInternalServerError, installErrorBody{Error: "The skill could not be added. Try again.", Code: "internal"})
				return
			}
			added = true
		}
		s.enqueue(rf)
		s.kickSkillCopies(uid)
	} else {
		id, status, err := s.writtenSkillItem(uid, slug)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, installErrorBody{Error: "The skill could not be added. Try again.", Code: "internal"})
			return
		}
		if id == "" {
			writeJSON(w, http.StatusUnprocessableEntity, missing)
			return
		}
		if status == store.SkillArchived {
			if err = s.st.SetSkillStatusFor(uid, []string{id}, store.SkillActive, s.skillNow()); err != nil {
				writeJSON(w, http.StatusInternalServerError, installErrorBody{Error: "The skill could not be added. Try again.", Code: "internal"})
				return
			}
			added = true
		}
	}
	s.eventRequest(r, "skill-add", "", map[string]any{"session": session.ID, "machine": host, "slug": slug, "added": added})
	writeJSON(w, 200, map[string]any{"name": slug, "status": store.SkillActive, "added": added})
}

// writtenSkillItem finds the live library item of one of the person's own
// written skills, with its status; id is "" when there is none.
func (s *server) writtenSkillItem(uid int64, slug string) (id, status string, err error) {
	written, err := s.st.HostedSkillsFor(uid)
	if err != nil {
		return "", "", err
	}
	known := false
	for _, h := range written {
		known = known || h.Slug == slug
	}
	if !known {
		return "", "", nil
	}
	items, err := s.st.LibraryFor(uid)
	if err != nil {
		return "", "", err
	}
	for _, it := range items {
		for _, a := range it.Aliases {
			if a.Kind == "hosted" && a.Reference == slug {
				return it.ID, it.Status, nil
			}
		}
	}
	return "", "", nil
}
