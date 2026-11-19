package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

// skillContentLimit bounds a content save: base64 expands an 8 MiB skill, and
// the rest bounds the paths and the JSON around them.
const skillContentLimit = 16 << 20

// registerSkillDetail serves the opened skill on the Skills page: its content
// for Write a skill, and Publish.
func (s *server) registerSkillDetail(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/skills/{item}/content", s.requireWriter(s.skillContent))
	mux.HandleFunc("PUT /api/v2/skills/{item}/content", s.requireWriter(s.skillContentSave))
	mux.HandleFunc("POST /api/v2/skills/{item}/publish", s.requireWriter(s.skillPublish))
}

// skillItem is the {item} of the path, or false once it has answered 400.
func skillItem(w http.ResponseWriter, r *http.Request) (string, bool) {
	item := r.PathValue("item")
	if item == "" || len(item) > 64 {
		writeErrCode(w, 400, errInvalidArgument, "item", "That is not a skill id.", "")
		return "", false
	}
	return item, true
}

// skillContent is GET /api/v2/skills/{item}/content: the files of the skill's
// head revision, and that revision, which a save names as the one it edits.
// A skill reet has no copy of yet has revision "" and no files.
func (s *server) skillContent(w http.ResponseWriter, r *http.Request) {
	item, ok := skillItem(w, r)
	if !ok {
		return
	}
	uid := s.uid(r)
	li, err := s.st.LibraryItemFor(uid, item)
	if err == nil && li.ItemType != store.LibrarySkill {
		err = store.ErrPrivateSkillMissing
	}
	if err != nil {
		skillsPageError(w, err)
		return
	}
	files := []skillbundle.ContentFile{}
	if li.BundleID != "" && li.RevisionID != "" {
		rev, err := s.st.PrivateSkillRevisionFor(uid, li.BundleID, li.RevisionID)
		if err != nil {
			skillsPageError(w, err)
			return
		}
		files = rev.Files
	}
	writeJSON(w, 200, map[string]any{"id": li.ID, "name": li.Name, "revision": li.RevisionID, "files": files})
}

// skillContentSave is PUT /api/v2/skills/{item}/content, body
// {"revision":"<the revision GET answered>","files":[{path,data,executable}]}.
// The files become the skill's next revision, of kind edit, which every
// machine gets; the one before stays in its history. It refuses with 409 when
// the skill has moved on since that revision. A published skill's public
// page follows.
func (s *server) skillContentSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision *string                   `json:"revision"`
		Files    []skillbundle.ContentFile `json:"files"`
	}
	if !decodeStrictJSON(w, r, skillContentLimit, &in, "Invalid or oversized skill content.") {
		return
	}
	item, ok := skillItem(w, r)
	if !ok {
		return
	}
	if in.Revision == nil {
		writeErrCode(w, 400, errInvalidArgument, "revision", "Say which revision you edited. Open the skill again.", "")
		return
	}
	uid := s.uid(r)
	res, err := s.st.SetSkillContentFromFor(uid, item, in.Files, store.PrivateSkillOrigin{Kind: "edit"}, *in.Revision)
	switch {
	case errors.Is(err, store.ErrPrivateSkillConflict):
		writeErr(w, 409, "The skill changed since you opened it. Open it again to edit what is there now.")
		return
	case errors.Is(err, store.ErrPrivateSkillInvalid):
		writeErr(w, 400, "That isn't a skill reet can save: it needs a SKILL.md with a name and description, and plain relative file paths.")
		return
	case err != nil:
		privateSkillError(w, err)
		return
	}
	out := map[string]any{"revision": res.Revision.ID}
	// The content is saved whatever happens here; a public page that cannot
	// follow keeps what it showed, and the person is told why.
	if err := s.st.RefreshHostedSkillFor(uid, item); err != nil {
		var problem store.SkillPublishProblem
		if !errors.As(err, &problem) {
			writeErr(w, 500, "Saved, but the public page could not be updated.")
			return
		}
		out["publicProblem"] = "Saved. The public page still shows the version before: " + problem.Reason
	}
	s.eventRequest(r, "skill-edit", item, map[string]any{"revision": res.Revision.ID})
	writeJSON(w, 200, out)
}

// skillPublish is POST /api/v2/skills/{item}/publish, body {"public":bool}:
// Publish and Unpublish on the opened skill. It answers whether the skill is
// public and its public address, or 409 with why it cannot be published.
func (s *server) skillPublish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Public *bool `json:"public"`
	}
	if !decodeStrictJSON(w, r, skillsPageSmallLimit, &in, "Invalid or oversized request.") {
		return
	}
	item, ok := skillItem(w, r)
	if !ok {
		return
	}
	if in.Public == nil {
		writeErrCode(w, 400, errInvalidArgument, "public", "Say whether to publish the skill or make it private.", "")
		return
	}
	uid := s.uid(r)
	out, err := s.st.PublishSkillFor(uid, item, *in.Public)
	if err != nil {
		var problem store.SkillPublishProblem
		if errors.As(err, &problem) {
			writeErrCode(w, 409, "not-publishable", "public", problem.Reason, "")
			return
		}
		skillsPageError(w, err)
		return
	}
	username, err := s.st.PublishingUsername(uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// A skill the person wrote follows Write a skill's switch: its public
	// entry joins the shelf when published and is hidden when not.
	if out.Written && username != "" {
		if !out.Published {
			s.setHostedSkillVisible(r, username, out.Slug, false)
		} else if hosted, err := s.st.HostedSkillsFor(uid); err == nil {
			for _, sk := range hosted {
				if sk.Slug == out.Slug {
					s.addHostedSkillToShelf(r, username, sk)
				}
			}
		}
	}
	url := ""
	if out.Published {
		url = requestOrigin(r) + "/" + username + "/" + out.Slug
	}
	s.eventRequest(r, "skill-publish", item, map[string]any{"public": out.Published})
	writeJSON(w, 200, map[string]any{"published": out.Published, "publicUrl": url})
}
