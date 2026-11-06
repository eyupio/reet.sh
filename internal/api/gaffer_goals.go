package api

import (
	"github.com/eyupio/reet.sh/internal/gaffer"
	"net/http"
)

func (s *server) registerGoalRoutes(mux *http.ServeMux) {
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{accounts: "Offline goals require a browser account.", signedOut: "Sign in to manage Gaffer goals.", readOnly: "A read-only account cannot delegate Gaffer follow-up."}, h)
	}
	mux.HandleFunc("GET /api/v2/gaffer/goals", access(s.goalList))
	mux.HandleFunc("GET /api/v2/gaffer/conversations/{id}/goal", access(s.goalGet))
	mux.HandleFunc("PUT /api/v2/gaffer/conversations/{id}/goal", access(s.goalSave))
	mux.HandleFunc("POST /api/v2/gaffer/conversations/{id}/goal/setup", access(s.goalSetup))
	mux.HandleFunc("POST /api/v2/gaffer/conversations/{id}/goal/control", access(s.goalControl))
}
func (s *server) goalSetup(w http.ResponseWriter, r *http.Request) {
	var in gaffer.GoalSetupInput
	if !decodeStrictJSON(w, r, 12<<10, &in, "Review this session's access and follow-up limits before continuing.") {
		return
	}
	v, err := s.gafferProviders.SetupGoal(s.uid(r), r.PathValue("id"), in)
	if err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-goal-access-reviewed", "", map[string]any{"conversation": r.PathValue("id"), "goal": v.Goal.ID, "revision": v.Goal.Revision, "action": in.Action, "maxReviews": v.Goal.MaxReviews, "expiresAt": v.Goal.ExpiresAt})
	writeJSON(w, 200, v)
}
func (s *server) goalList(w http.ResponseWriter, r *http.Request) {
	v, err := s.gafferProviders.Goals(s.uid(r))
	if err != nil {
		gafferError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *server) goalGet(w http.ResponseWriter, r *http.Request) {
	v, err := s.gafferProviders.Goal(s.uid(r), r.PathValue("id"))
	if err != nil {
		gafferError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *server) goalSave(w http.ResponseWriter, r *http.Request) {
	var in gaffer.GoalInput
	if !decodeStrictJSON(w, r, 12<<10, &in, "Review the goal and its review budget before starting.") {
		return
	}
	v, err := s.gafferProviders.SaveGoal(s.uid(r), r.PathValue("id"), in)
	if err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-goal-started", "", map[string]any{"conversation": r.PathValue("id"), "goal": v.Goal.ID, "revision": v.Goal.Revision, "maxReviews": v.Goal.MaxReviews, "expiresAt": v.Goal.ExpiresAt})
	writeJSON(w, 200, v)
}
func (s *server) goalControl(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision  int64           `json:"revision"`
		Action    string          `json:"action"`
		Pins      gaffer.GoalPins `json:"pins"`
		Confirmed bool            `json:"confirmed"`
	}
	if !decodeStrictJSON(w, r, 4096, &in, "Reload the goal before changing follow-up.") {
		return
	}
	v, err := s.gafferProviders.ControlGoal(s.uid(r), r.PathValue("id"), in.Revision, in.Action, in.Pins, in.Confirmed)
	if err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-goal-controlled", "", map[string]any{"conversation": r.PathValue("id"), "goal": v.Goal.ID, "revision": v.Goal.Revision, "action": in.Action})
	writeJSON(w, 200, v)
}
