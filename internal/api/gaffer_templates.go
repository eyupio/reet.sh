package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"github.com/eyupio/reet.sh/internal/store"
)

// Private Factory templates. Every route is the signed-in browser account's,
// like the outcome routes: another account's template is not found, and no
// read-only account, API token or MCP connection reaches them.
func (s *server) registerGafferTemplateRoutes(mux *http.ServeMux) {
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{
			accounts: "Factory templates need a browser account.", signedOut: "Sign in to use Factory templates.", readOnly: "This account is read-only, so it cannot use Factory templates.",
		}, h)
	}
	mux.HandleFunc("GET /api/v2/gaffer/templates", access(s.gafferTemplates))
	mux.HandleFunc("POST /api/v2/gaffer/templates", access(s.gafferTemplateSave))
	mux.HandleFunc("GET /api/v2/gaffer/templates/{id}", access(s.gafferTemplate))
	mux.HandleFunc("GET /api/v2/gaffer/templates/{id}/versions/{version}", access(s.gafferTemplate))
	mux.HandleFunc("PUT /api/v2/gaffer/templates/{id}", access(s.gafferTemplateEdit))
	mux.HandleFunc("GET /api/v2/gaffer/templates/{id}/diff", access(s.gafferTemplateDiff))
	mux.HandleFunc("POST /api/v2/gaffer/templates/{id}/launch", access(s.gafferTemplateLaunch))
	mux.HandleFunc("DELETE /api/v2/gaffer/templates/{id}", access(s.gafferTemplateDelete))
}

func gafferTemplateError(w http.ResponseWriter, err error) {
	var changed *gaffer.TemplateSkillsChanged
	switch {
	case errors.As(err, &changed):
		writeJSON(w, http.StatusConflict, map[string]any{"error": changed.Error(), "code": "template_skills_changed", "message": changed.Error(), "gaps": changed.Gaps})
	case errors.Is(err, store.ErrGafferTemplateChanged):
		writeErrCode(w, http.StatusConflict, "template_changed", "", err.Error(), "")
	case errors.Is(err, store.ErrGafferTemplateLimit):
		writeErrCode(w, http.StatusConflict, "template_limit", "", err.Error(), "")
	case errors.Is(err, store.ErrGafferTemplateTooLarge):
		writeErrCode(w, http.StatusUnprocessableEntity, "template_too_large", "", err.Error(), "")
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "That template or outcome is not available.")
	default:
		gafferDeliveryError(w, err)
	}
}

// templateSkills loads a skill for the account the way stage guidance is
// chosen: a built-in that is on, or an Active library skill.
func (s *server) templateSkills(uid int64) gaffer.SkillLoader {
	return func(id string) (skillbuddy.Skill, error) { return s.buddyLoad(uid, id) }
}

// positive reads a whole number from the query; empty is 0 and anything else
// that is not a positive number is -1.
func positive(text string) int {
	if text == "" {
		return 0
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 {
		return -1
	}
	return n
}

func (s *server) gafferTemplates(w http.ResponseWriter, r *http.Request) {
	rows, err := s.gafferProviders.Templates(s.uid(r))
	if err != nil {
		gafferTemplateError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"templates": rows})
}

func (s *server) gafferTemplate(w http.ResponseWriter, r *http.Request) {
	version := positive(r.PathValue("version"))
	if version < 0 {
		writeErr(w, 404, "That template or outcome is not available.")
		return
	}
	view, err := s.gafferProviders.Template(s.uid(r), r.PathValue("id"), version, s.templateSkills(s.uid(r)))
	if err != nil {
		gafferTemplateError(w, err)
		return
	}
	writeJSON(w, 200, view)
}

func (s *server) gafferTemplateSave(w http.ResponseWriter, r *http.Request) {
	var in gaffer.TemplateSaveInput
	if !decodeStrictJSON(w, r, 4<<10, &in, "That template request is invalid.") {
		return
	}
	saved, err := s.gafferProviders.SaveTemplate(s.uid(r), in)
	if err != nil {
		gafferTemplateError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-template-saved", "", map[string]any{"template": saved.ID, "version": saved.Version, "outcome": saved.OutcomeID})
	s.writeTemplate(w, r, saved.ID, saved.Version, 201)
}

func (s *server) gafferTemplateEdit(w http.ResponseWriter, r *http.Request) {
	var in gaffer.TemplateEditInput
	if !decodeStrictJSON(w, r, store.MaxGafferTemplateBytes+(4<<10), &in, "That template is invalid or too large.") {
		return
	}
	saved, err := s.gafferProviders.EditTemplate(s.uid(r), r.PathValue("id"), in)
	if err != nil {
		gafferTemplateError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-template-edited", "", map[string]any{"template": saved.ID, "version": saved.Version})
	s.writeTemplate(w, r, saved.ID, saved.Version, 200)
}

func (s *server) writeTemplate(w http.ResponseWriter, r *http.Request, id string, version, code int) {
	view, err := s.gafferProviders.Template(s.uid(r), id, version, s.templateSkills(s.uid(r)))
	if err != nil {
		gafferTemplateError(w, err)
		return
	}
	writeJSON(w, code, view)
}

func (s *server) gafferTemplateDiff(w http.ResponseWriter, r *http.Request) {
	from, to := positive(r.URL.Query().Get("from")), positive(r.URL.Query().Get("to"))
	if from < 0 || to < 0 {
		gafferTemplateError(w, &gaffer.Invalid{Field: "version", Message: "Name the versions to compare by number."})
		return
	}
	d, err := s.gafferProviders.TemplateDiff(s.uid(r), r.PathValue("id"), from, to)
	if err != nil {
		gafferTemplateError(w, err)
		return
	}
	writeJSON(w, 200, d)
}

// recordStageSkills leaves the skill receipts a choice on the page would have
// left for the pipelines' stages, so a stage's Skills panel shows them and its
// launch marks them prepared. A receipt that is not saved changes nothing.
func (s *server) recordStageSkills(uid int64, outcome string, pipelines []store.GafferOutcomeSession) {
	for _, p := range pipelines {
		for i, st := range p.Stages {
			for _, sk := range st.SkillGuidance {
				sk.State = "selected"
				_ = s.st.RecordSkillBuddyFor(uid, "factory:"+outcome+":"+p.Key+":"+strconv.Itoa(i), "", st.Kind, sk)
			}
		}
	}
}

// gafferTemplateLaunch starts an outcome from a template version. It is an
// approval like any other: refused where remote sessions are off, and YOLO
// needs the account's setting.
func (s *server) gafferTemplateLaunch(w http.ResponseWriter, r *http.Request) {
	var in gaffer.TemplateLaunchInput
	// Room for a skipped key for every skill of the largest plan, and the crew.
	if !decodeStrictJSON(w, r, 64<<10, &in, "That launch is invalid or too large.") {
		return
	}
	if !s.terminalEnabled() {
		writeErr(w, 403, "Remote sessions are disabled on this controller, so Gaffer cannot start any.")
		return
	}
	if in.Yolo {
		if setting, err := s.yoloSetting(); err != nil || !setting.Allowed {
			writeErrCode(w, http.StatusForbidden, "yolo_not_allowed", "", sentenceYoloNotAllowed, "")
			return
		}
	}
	uid := s.uid(r)
	o, created, err := s.gafferProviders.LaunchTemplate(uid, r.PathValue("id"), in, s.templateSkills(uid))
	if err != nil {
		gafferTemplateError(w, err)
		return
	}
	// A repeated request leaves the receipts of the outcome it already made.
	if created {
		s.recordStageSkills(uid, o.ID, o.Sessions)
	}
	s.eventRequest(r, "gaffer-template-launched", "", map[string]any{"template": o.TemplateID, "version": o.TemplateVersion, "outcome": o.ID, "machine": o.AgentID, "projectId": o.ProjectID, "yolo": o.Yolo})
	writeJSON(w, 201, o)
}

func (s *server) gafferTemplateDelete(w http.ResponseWriter, r *http.Request) {
	version := positive(r.URL.Query().Get("version"))
	if version < 1 {
		gafferTemplateError(w, &gaffer.Invalid{Field: "version", Message: "Reload this template before deleting it."})
		return
	}
	id := r.PathValue("id")
	if err := s.gafferProviders.DeleteTemplate(s.uid(r), id, version); err != nil {
		gafferTemplateError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-template-deleted", "", map[string]any{"template": id})
	w.WriteHeader(http.StatusNoContent)
}
