package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/gaffer"
)

// A pipeline's delivery review: the person's decision on finished work,
// against the criteria they agreed. Every route is the owning browser
// account's, like the outcome routes it sits under: the outcome is read by
// its owner, so another account's outcome is not found, and the pipeline,
// criteria and evidence are resolved inside that outcome only. No API token
// or MCP connection can read or decide a delivery.

func gafferDeliveryError(w http.ResponseWriter, err error) {
	var conflict *gaffer.DeliveryConflict
	if errors.As(err, &conflict) {
		writeErrCode(w, http.StatusConflict, conflict.Code, "", conflict.Message, "")
		return
	}
	gafferOutcomeError(w, err)
}

func (s *server) gafferDelivery(w http.ResponseWriter, r *http.Request) {
	review, err := s.gafferProviders.DeliveryReview(r.Context(), s.uid(r), r.PathValue("id"), r.PathValue("key"))
	if err != nil {
		gafferDeliveryError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, review)
}

func (s *server) gafferDeliveryCriteria(w http.ResponseWriter, r *http.Request) {
	var in gaffer.DeliveryCriteriaInput
	if !decodeStrictJSON(w, r, 16<<10, &in, "Those criteria are invalid or too large.") {
		return
	}
	review, err := s.gafferProviders.SetDeliveryCriteria(r.Context(), s.uid(r), r.PathValue("id"), r.PathValue("key"), in)
	if err != nil {
		gafferDeliveryError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-delivery-criteria", "", map[string]any{"outcome": review.OutcomeID, "pipeline": review.Key, "criteria": len(review.Criteria), "criteriaRevision": review.CriteriaRevision})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, review)
}

func (s *server) gafferDeliveryEvidence(w http.ResponseWriter, r *http.Request) {
	var in gaffer.DeliveryEvidenceInput
	if !decodeStrictJSON(w, r, 16<<10, &in, "That evidence is invalid or too large.") {
		return
	}
	review, err := s.gafferProviders.AddDeliveryEvidence(r.Context(), s.uid(r), r.PathValue("id"), r.PathValue("key"), in)
	if err != nil {
		gafferDeliveryError(w, err)
		return
	}
	s.eventRequest(r, "gaffer-delivery-evidence", "", map[string]any{"outcome": review.OutcomeID, "pipeline": review.Key, "kind": in.Kind, "commit": review.Commit})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, review)
}

// gafferDeliveryDecision is the person accepting a delivery or sending it
// back. Either can start a session (the next stage, or the stage done again),
// so it is refused where remote sessions are off, as a merge approval is.
func (s *server) gafferDeliveryDecision(w http.ResponseWriter, r *http.Request) {
	var in gaffer.DeliveryDecisionInput
	if !decodeStrictJSON(w, r, 16<<10, &in, "That decision is invalid or too large.") {
		return
	}
	if !s.terminalEnabled() {
		writeErr(w, 409, "Remote sessions are disabled on this controller, so a delivery cannot be decided here.")
		return
	}
	o, review, decision, recorded, err := s.gafferProviders.DecideDelivery(r.Context(), s.uid(r), r.PathValue("id"), r.PathValue("key"), in)
	if err != nil {
		gafferDeliveryError(w, err)
		return
	}
	// A request answered from its record is not a second decision, so it is
	// not logged as one.
	if recorded {
		name := "gaffer-delivery-accepted"
		if decision.Decision == "send_back" {
			name = "gaffer-delivery-sent-back"
		}
		s.eventRequest(r, name, "", map[string]any{"outcome": o.ID, "pipeline": review.Key, "commit": decision.Commit, "criteriaRevision": decision.CriteriaRevision, "stage": decision.Stage, "needsWork": len(decision.Failed)})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"outcome": o, "review": review, "decision": decision})
}
