package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// An outcome saved before delivery review, as a backup restore brings back,
// has none of its fields. It still opens, is still written back, and gains
// nothing it did not have: no criteria, so nothing to stop it for a review.
func TestAnOutcomeSavedBeforeDeliveryReviewStillOpensAndIsWrittenBack(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	const old = `{"id":"0123456789abcdef0123456789abcdef","revision":4,"objective":"Ship CSV export.","state":"settled","note":"","agentId":"AGENT","projectId":"p1","projectName":"billing","dir":"/work/billing","tool":"opencode","providerId":"prov","maxReviews":8,"hours":24,"createdAt":1700000000000,"updatedAt":1700000000000,
"sessions":[{"key":"a","title":"Export API","prompt":"Add the endpoint.","requestId":"r1","state":"ready_to_merge","note":"Ready to merge. Nothing merges until you approve it.","sessionId":"s1","updatedAt":1700000000000,"stage":1,"workDir":"/work/tree",
"stages":[{"kind":"code","tool":"opencode","state":"done","requestId":"r1","summary":"Done."},{"kind":"merge","tool":"opencode","state":"pending","requestId":"r2"}]}]}`
	raw := strings.Replace(old, "AGENT", a.ID, 1)
	if _, err := s.db.Exec(`INSERT INTO gaffer_outcomes(id,user_id,created_at,data) VALUES(?,?,?,?)`, "0123456789abcdef0123456789abcdef", uid, 1700000000000, raw); err != nil {
		t.Fatal(err)
	}
	o, err := s.GafferOutcomeFor(uid, "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	piece := o.Sessions[0]
	if piece.State != "ready_to_merge" || len(piece.Criteria) != 0 || piece.CriteriaRevision != 0 || piece.ReviewRevision != 0 || piece.DeliveryAccepted || len(piece.Evidence) != 0 || len(piece.Deliveries) != 0 || o.Finished() {
		t.Fatalf("restored outcome: %+v", piece)
	}
	after, err := s.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *GafferOutcome) bool {
		o.Sessions[0].MergeApproved, o.Sessions[0].State = true, "advancing"
		return true
	})
	if err != nil || after.Revision != 5 {
		t.Fatalf("write back: %+v %v", after, err)
	}
	var saved string
	if err := s.db.QueryRow(`SELECT data FROM gaffer_outcomes WHERE id=?`, o.ID).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"criteria", "criteriaRevision", "reviewRevision", "evidence", "deliveries", "deliveryAccepted"} {
		if strings.Contains(saved, `"`+key+`"`) {
			t.Fatalf("an outcome without criteria was saved with %q: %s", key, saved)
		}
	}
	// A pipeline waiting for its delivery review is still moving: it is never
	// finished, so it cannot be archived, deleted or dropped as old.
	waiting := GafferOutcome{State: "settled", Sessions: []GafferOutcomeSession{{State: "delivery_review"}}}
	if waiting.Finished() {
		t.Fatal("an outcome waiting for a delivery review reads as finished")
	}
	// The new fields survive a round trip.
	in := GafferOutcomeSession{Key: "a", Criteria: []GafferCriterion{{ID: "c1", Text: "It exports.", Revision: 1}}, CriteriaRevision: 1, ReviewRevision: 2, DeliveryAccepted: true,
		Evidence:   []GafferEvidence{{ID: "e1", CriterionID: "c1", Kind: "note", Verdict: "passed", Text: "Tried it.", Commit: strings.Repeat("a", 40), CriteriaRevision: 1, By: uid, ByName: "Owner", At: 5}},
		Deliveries: []GafferDeliveryDecision{{RequestID: "q1", Decision: "accept", Commit: strings.Repeat("a", 40), CriteriaRevision: 1, By: uid, ByName: "Owner", At: 6, Statuses: map[string]string{"c1": "passed"}}}}
	b, _ := json.Marshal(in)
	var out GafferOutcomeSession
	if err := json.Unmarshal(b, &out); err != nil || out.Deliveries[0].Statuses["c1"] != "passed" || out.Evidence[0].Commit != in.Evidence[0].Commit || !out.DeliveryAccepted {
		t.Fatalf("round trip: %+v %v", out, err)
	}
}
