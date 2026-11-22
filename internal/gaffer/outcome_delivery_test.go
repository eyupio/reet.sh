package gaffer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// deliveryInput is one pipeline of code, test and merge with two criteria.
func deliveryInput(r *outcomeRig, auto bool) OutcomeInput {
	in := pipelineInput(r, auto)
	in.Sessions[0].Stages = []OutcomeStageInput{{Kind: "code", Tool: "opencode"}, {Kind: "test", Tool: "opencode"}, {Kind: "merge", Tool: "opencode"}}
	in.Sessions[0].Criteria = []string{"The export downloads as CSV.", "  ", "Amounts keep two decimal places."}
	return in
}

// toReview runs a pipeline's stages until it waits for its delivery review.
func toReview(t *testing.T, r *outcomeRig, id string) store.GafferOutcomeSession {
	t.Helper()
	for {
		p := r.piece(t, id, func(p store.GafferOutcomeSession) bool {
			return p.State == "delivery_review" || p.State == "following" && p.ConversationID != ""
		})
		if p.State == "delivery_review" {
			return p
		}
		kind := p.Stages[p.Stage].Kind
		r.s.stageHandedOver(r.p.UserID, p.ConversationID, "The "+kind+" stage is finished.", "gaffer")
		at := p.Stage
		r.piece(t, id, func(p store.GafferOutcomeSession) bool { return p.State != "following" || p.Stage != at })
	}
}

func request(n byte) string { return strings.Repeat(string(rune('0'+n)), 32) }

func decisionFor(v DeliveryReviewView, kind string, n byte) DeliveryDecisionInput {
	return DeliveryDecisionInput{RequestID: request(n), Decision: kind, CriteriaRevision: v.CriteriaRevision, ReviewRevision: v.ReviewRevision, Commit: v.Commit, Statuses: v.statuses(), Confirmed: true}
}

func conflictCode(err error) string {
	var c *DeliveryConflict
	if errors.As(err, &c) {
		return c.Code
	}
	return ""
}

func (r *outcomeRig) review(t *testing.T, id string) DeliveryReviewView {
	t.Helper()
	v, err := r.s.DeliveryReview(context.Background(), r.p.UserID, id, "a")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (r *outcomeRig) note(t *testing.T, id string, v DeliveryReviewView, criterion int, verdict, text string, n byte) DeliveryReviewView {
	t.Helper()
	out, err := r.s.AddDeliveryEvidence(context.Background(), r.p.UserID, id, "a", DeliveryEvidenceInput{RequestID: request(n), ReviewRevision: v.ReviewRevision, CriterionID: v.Criteria[criterion].ID, Kind: "note", Verdict: verdict, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// The criteria agreed with the plan get stable IDs and a first revision, the
// stages' agents are told them, and the pipeline stops for the person's
// delivery review before anything merges. Accepting records who decided, on
// which commit and criteria, and still leaves the merge to its own approval.
func TestAPipelineWithCriteriaWaitsForItsDeliveryReviewAndTheMergeStaysSeparate(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	plan := created.Sessions[0]
	if len(plan.Criteria) != 2 || plan.CriteriaRevision != 1 || !store.ValidGafferOutcomeID(plan.Criteria[0].ID) || plan.Criteria[0].ID == plan.Criteria[1].ID || plan.Criteria[1].Text != "Amounts keep two decimal places." || plan.Criteria[0].Revision != 1 {
		t.Fatalf("approved criteria: %+v", plan)
	}
	waiting := toReview(t, r, created.ID)
	if waiting.Stage != 2 || waiting.Stages[2].State != "pending" || waiting.DeliveryAccepted || len(r.starter.asked()) != 2 {
		t.Fatalf("waiting for review: %+v launches=%d", waiting, len(r.starter.asked()))
	}
	for _, launch := range r.starter.asked() {
		if !strings.Contains(launch.Prompt, "Acceptance criteria the person agreed for this work") || !strings.Contains(launch.Prompt, "1. The export downloads as CSV.\n2. Amounts keep two decimal places.") || !strings.HasSuffix(launch.Prompt, "Do not merge.") {
			t.Fatalf("stage prompt without the criteria: %q", launch.Prompt)
		}
	}
	if o := mustOutcome(t, r, created.ID); o.Finished() {
		t.Fatal("an outcome waiting for a delivery review reads as finished")
	}
	v := r.review(t, created.ID)
	if !v.Open || v.Commit != fixedHead || v.CommitState != "verified" || v.Next != "ready_to_merge" || v.DefaultStage != "code" || strings.Join(v.Stages, ",") != "code,test,merge" || v.Accepted {
		t.Fatalf("review: %+v", v)
	}
	// Nothing was recorded for either criterion: missing evidence is Unverified, never Passed.
	for _, c := range v.Criteria {
		if c.Status != CriterionUnverified || len(c.Evidence) != 0 {
			t.Fatalf("criterion with no evidence: %+v", c)
		}
	}
	// Gaffer cannot move it on, and the merge cannot be approved from here.
	if _, err := r.s.MovePipelineOn(r.p.UserID, created.ID, "a", ""); err == nil {
		t.Fatal("a pipeline in delivery review was moved on")
	}
	if _, err := r.s.ApproveMerge(r.p.UserID, created.ID, "a", mustOutcome(t, r, created.ID).Revision); err == nil {
		t.Fatal("a merge was approved before the delivery was accepted")
	}
	v = r.note(t, created.ID, v, 0, CriterionPassed, "Downloaded it and opened it in a spreadsheet.", 1)
	if v.Criteria[0].Status != CriterionPassed || v.Criteria[0].Evidence[0].Kind != "note" || v.Criteria[0].Evidence[0].ByName != "Owner" || v.Criteria[0].Evidence[0].Commit != fixedHead || v.Criteria[1].Status != CriterionUnverified {
		t.Fatalf("after a note: %+v", v.Criteria)
	}
	// The page's own read of the outcome, every few seconds, carries the
	// criteria and not the evidence: the review reads that for itself.
	if listed, err := r.s.Outcome(r.p.UserID, created.ID); err != nil || len(listed.Sessions[0].Criteria) != 2 || len(listed.Sessions[0].Evidence) != 0 || len(mustOutcome(t, r, created.ID).Sessions[0].Evidence) != 1 {
		t.Fatalf("outcome as the page reads it: %+v %v", listed.Sessions[0], err)
	}
	_, after, decision, recorded, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(v, "accept", 2))
	if err != nil || !recorded {
		t.Fatalf("accept: %v recorded=%v", err, recorded)
	}
	if decision.Decision != "accept" || decision.Commit != fixedHead || decision.CriteriaRevision != 1 || decision.By != r.p.UserID || decision.ByName != "Owner" || decision.At == 0 || decision.Statuses[v.Criteria[1].ID] != CriterionUnverified || len(decision.Failed) != 0 {
		t.Fatalf("recorded decision: %+v", decision)
	}
	if after.Open || len(after.Decisions) != 1 || !after.Accepted || !after.AcceptedCurrent || after.AcceptedCommit != fixedHead {
		t.Fatalf("review after accepting: %+v", after)
	}
	// Accepting the delivery approved no merge: the pipeline waits for that separately.
	ready := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	time.Sleep(60 * time.Millisecond)
	if ready.MergeApproved || !ready.DeliveryAccepted || len(r.starter.asked()) != 2 {
		t.Fatalf("after acceptance: %+v launches=%d", ready, len(r.starter.asked()))
	}
	if _, err := r.s.ApproveMerge(r.p.UserID, created.ID, "a", mustOutcome(t, r, created.ID).Revision); err != nil {
		t.Fatal(err)
	}
	merge := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })
	if !merge.MergeApproved {
		t.Fatalf("merge stage: %+v", merge)
	}
}

// An outcome approved before delivery review has no criteria: it opens, runs
// and waits for its merge exactly as it did, its review has nothing in it and
// asks no machine, and no decision can be recorded on it.
func TestAPipelineWithoutCriteriaNeverStopsForADeliveryReview(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := deliveryInput(r, false)
	in.Sessions[0].Criteria = nil
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(created)
	for _, key := range []string{"criteria", "criteriaRevision", "reviewRevision", "evidence", "deliveries", "deliveryAccepted"} {
		if strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatalf("an outcome without criteria carries %q: %s", key, raw)
		}
	}
	for _, at := range []int{0, 1} {
		p := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool {
			return p.State == "following" && p.Stage == at && p.ConversationID != ""
		})
		r.s.stageHandedOver(r.p.UserID, p.ConversationID, "Done.", "gaffer")
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	v := r.review(t, created.ID)
	if v.Open || len(v.Criteria) != 0 || r.starter.worked != 0 {
		t.Fatalf("review of a pipeline without criteria: %+v machine reads=%d", v, r.starter.worked)
	}
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(v, "accept", 1)); conflictCode(err) != "delivery_closed" {
		t.Fatalf("a decision on a pipeline with no review: %v", err)
	}
}

// One request is one decision: asked again it answers what it recorded, also
// after the pipeline has moved on, as a browser that never heard back would
// ask. The same request with other content is refused.
func TestARepeatedDeliveryDecisionIsRecordedOnce(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	v := r.review(t, created.ID)
	in := decisionFor(v, "accept", 1)
	in.Notes = "Checked on staging."
	_, _, first, recorded, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", in)
	if err != nil || !recorded {
		t.Fatalf("first: %v", err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" })
	_, again, second, recorded, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", in)
	if err != nil || recorded || second.At != first.At || second.RequestID != first.RequestID || len(again.Decisions) != 1 {
		t.Fatalf("the interrupted request asked again: %v recorded=%v %+v", err, recorded, again.Decisions)
	}
	other := in
	other.Decision, other.Notes = "send_back", "Changed my mind."
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", other); conflictCode(err) != "delivery_request_reused" {
		t.Fatalf("the same request with another decision: %v", err)
	}
	// A new request finds the delivery decided.
	late := decisionFor(v, "send_back", 3)
	late.Notes = "Too late."
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", late); conflictCode(err) != "delivery_closed" {
		t.Fatalf("a decision after the delivery was decided: %v", err)
	}
	if p := mustOutcome(t, r, created.ID).Sessions[0]; len(p.Deliveries) != 1 || p.Deliveries[0].Decision != "accept" {
		t.Fatalf("decisions kept: %+v", p.Deliveries)
	}
}

// Two people looking at the same review and deciding at the same moment leave
// one decision; the other is told it was decided.
func TestTwoReviewersDecidingAtOnceLeaveOneDecision(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	v := r.review(t, created.ID)
	inputs := []DeliveryDecisionInput{decisionFor(v, "accept", 1), decisionFor(v, "send_back", 2)}
	inputs[1].Notes = "The totals are wrong."
	errs := make([]error, len(inputs))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range inputs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, _, _, errs[i] = r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", inputs[i])
		}()
	}
	close(start)
	wg.Wait()
	won, lost := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case conflictCode(err) == "delivery_closed" || conflictCode(err) == "delivery_changed":
			lost++
		default:
			t.Fatalf("unexpected refusal: %v", err)
		}
	}
	p := mustOutcome(t, r, created.ID).Sessions[0]
	if won != 1 || lost != 1 || len(p.Deliveries) != 1 {
		t.Fatalf("won=%d lost=%d decisions=%+v", won, lost, p.Deliveries)
	}
}

// Criteria can be reworded after approval. Evidence of a wording that changed
// stops counting, evidence of one that did not still counts, and a decision
// made on the earlier criteria is refused.
func TestChangedCriteriaMakeEarlierEvidenceHistoricalAndRefuseAnOlderDecision(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	v := r.review(t, created.ID)
	v = r.note(t, created.ID, v, 0, CriterionPassed, "Downloaded it.", 1)
	v = r.note(t, created.ID, v, 1, CriterionPassed, "Checked three invoices.", 2)
	seen := decisionFor(v, "accept", 3)
	edit := DeliveryCriteriaInput{CriteriaRevision: 1, Criteria: []CriterionInput{{ID: v.Criteria[0].ID, Text: v.Criteria[0].Text}, {ID: v.Criteria[1].ID, Text: "Amounts keep two decimal places, also for refunds."}, {Text: "The file name carries the month."}}}
	after, err := r.s.SetDeliveryCriteria(context.Background(), r.p.UserID, created.ID, "a", edit)
	if err != nil {
		t.Fatal(err)
	}
	if after.CriteriaRevision != 2 || len(after.Criteria) != 3 || after.Criteria[0].ID != v.Criteria[0].ID || after.Criteria[1].ID != v.Criteria[1].ID {
		t.Fatalf("edited criteria: %+v", after)
	}
	if c := after.Criteria[0]; c.Status != CriterionPassed || c.Evidence[0].Historical {
		t.Fatalf("an unchanged criterion lost its evidence: %+v", c)
	}
	if c := after.Criteria[1]; c.Status != CriterionUnverified || !c.Evidence[0].Historical {
		t.Fatalf("a reworded criterion kept its old evidence: %+v", c)
	}
	if after.Criteria[2].Status != CriterionUnverified {
		t.Fatalf("a new criterion: %+v", after.Criteria[2])
	}
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", seen); conflictCode(err) != "delivery_criteria_changed" {
		t.Fatalf("a decision on the earlier criteria: %v", err)
	}
	// An edit made on the earlier revision is refused too, and so are rows that are not criteria.
	if _, err := r.s.SetDeliveryCriteria(context.Background(), r.p.UserID, created.ID, "a", edit); conflictCode(err) != "delivery_criteria_changed" {
		t.Fatalf("a second edit on the earlier revision: %v", err)
	}
	for name, bad := range map[string][]CriterionInput{
		"two lines":      {{Text: "One\nTwo"}},
		"too long":       {{Text: strings.Repeat("x", store.MaxGafferCriterionText+1)}},
		"unknown id":     {{ID: request(9), Text: "Whose is this?"}},
		"none while due": {},
	} {
		if _, err := r.s.SetDeliveryCriteria(context.Background(), r.p.UserID, created.ID, "a", DeliveryCriteriaInput{CriteriaRevision: 2, Criteria: bad}); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	if p := mustOutcome(t, r, created.ID).Sessions[0]; p.State != "delivery_review" || len(p.Deliveries) != 0 || p.CriteriaRevision != 2 {
		t.Fatalf("pipeline after the refusals: %+v", p)
	}
}

// A new commit makes what was recorded about the last one history: nothing of
// it counts, and a decision made on the earlier commit is refused. A machine
// that cannot say which commit the work is at lets nothing be decided.
func TestANewCommitMakesEvidenceHistoricalAndRefusesTheOlderDecision(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	v := r.review(t, created.ID)
	v = r.note(t, created.ID, v, 0, CriterionPassed, "Downloaded it.", 1)
	seen := decisionFor(v, "accept", 2)
	next := strings.Repeat("b", 40)
	head, reachable := next, true
	r.starter.mu.Lock()
	r.starter.work = func(string, bool) (terminalproto.InspectResult, error) {
		if !reachable {
			return terminalproto.InspectResult{}, errors.New("offline")
		}
		return terminalproto.InspectResult{Repo: true, Head: head}, nil
	}
	r.starter.mu.Unlock()
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", seen); conflictCode(err) != "delivery_head_changed" {
		t.Fatalf("a decision on the earlier commit: %v", err)
	}
	now := r.review(t, created.ID)
	if now.Commit != next || now.Criteria[0].Status != CriterionUnverified || !now.Criteria[0].Evidence[0].Historical || now.Criteria[0].Evidence[0].Commit != fixedHead {
		t.Fatalf("review at the new commit: %+v", now.Criteria[0])
	}
	// An older machine reports no commit, and an offline one nothing at all.
	head = ""
	if got := r.review(t, created.ID); got.CommitState != "unverified" || got.CommitNote == "" || got.Criteria[0].Status != CriterionUnverified || !got.Criteria[0].Evidence[0].Unverified {
		t.Fatalf("a machine that reports no commit: %+v", got)
	}
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(now, "accept", 3)); conflictCode(err) != "delivery_head_unverified" {
		t.Fatalf("a decision with no commit reported: %v", err)
	}
	reachable = false
	if got := r.review(t, created.ID); got.CommitState != "unreachable" {
		t.Fatalf("an offline machine: %+v", got)
	}
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(now, "accept", 4)); conflictCode(err) != "delivery_head_unverified" {
		t.Fatalf("a decision with the machine offline: %v", err)
	}
	if p := mustOutcome(t, r, created.ID).Sessions[0]; len(p.Deliveries) != 0 || p.State != "delivery_review" {
		t.Fatalf("nothing should have been decided: %+v", p)
	}
}

// A GitHub check is evidence only as GitHub reports it now for the reviewed
// commit: passing is Passed, failing is Needs work, running or gone is
// Unverified, and a pull request at another commit proves nothing. A status
// that moved since the person looked refuses their decision.
func TestACheckIsReadFromGitHubForTheReviewedCommit(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	pullHead, state := fixedHead, "SUCCESS"
	r.starter.mu.Lock()
	r.starter.work = func(_ string, github bool) (terminalproto.InspectResult, error) {
		got := terminalproto.InspectResult{Repo: true, Head: fixedHead, Remote: "acme/billing"}
		if github {
			got.Pull = &terminalproto.InspectPull{Number: 12, URL: "https://github.com/acme/billing/pull/12", State: "OPEN", HeadOID: pullHead, Checks: []terminalproto.InspectCheck{{Name: "test", State: state, URL: "https://github.com/acme/billing/actions/runs/1"}}}
		}
		return got, nil
	}
	r.starter.mu.Unlock()
	v := r.review(t, created.ID)
	if v.Pull == nil || !v.Pull.AtHead || len(v.Checks) != 1 || v.Checks[0].Tone != "good" {
		t.Fatalf("pull request and checks: %+v %+v", v.Pull, v.Checks)
	}
	add := func(v DeliveryReviewView, in DeliveryEvidenceInput, n byte) (DeliveryReviewView, error) {
		in.RequestID, in.ReviewRevision, in.CriterionID = request(n), v.ReviewRevision, v.Criteria[0].ID
		return r.s.AddDeliveryEvidence(context.Background(), r.p.UserID, created.ID, "a", in)
	}
	if _, err := add(v, DeliveryEvidenceInput{Kind: "check", Name: "no-such-check"}, 1); err == nil {
		t.Fatal("a check GitHub does not report was accepted as evidence")
	}
	// The state is the machine's, whatever the request claims.
	v, err = add(v, DeliveryEvidenceInput{Kind: "check", Name: "test", Verdict: CriterionNeedsWork}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if ev := v.Criteria[0].Evidence[0]; v.Criteria[0].Status != CriterionPassed || ev.State != "SUCCESS" || ev.Verdict != CriterionPassed || ev.URL == "" {
		t.Fatalf("a passing check: %+v", v.Criteria[0])
	}
	seen := decisionFor(v, "accept", 3)
	state = "FAILURE"
	if got := r.review(t, created.ID); got.Criteria[0].Status != CriterionNeedsWork {
		t.Fatalf("a failed check: %+v", got.Criteria[0])
	}
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", seen); conflictCode(err) != "delivery_changed" {
		t.Fatalf("a decision made while the check still passed: %v", err)
	}
	state = "IN_PROGRESS"
	if got := r.review(t, created.ID); got.Criteria[0].Status != CriterionUnverified {
		t.Fatalf("a running check: %+v", got.Criteria[0])
	}
	state, pullHead = "SUCCESS", strings.Repeat("c", 40)
	got := r.review(t, created.ID)
	if got.Criteria[0].Status != CriterionUnverified || !got.Criteria[0].Evidence[0].Unverified || got.Pull.AtHead || len(got.Checks) != 0 {
		t.Fatalf("a pull request at another commit: %+v %+v", got.Criteria[0], got.Pull)
	}
	if _, err := add(got, DeliveryEvidenceInput{Kind: "check", Name: "test"}, 4); err == nil {
		t.Fatal("a check of another commit was accepted as evidence")
	}
	// An artefact is a link inside the pipeline's own repository, nothing else.
	for _, bad := range []string{"http://github.com/acme/billing/actions/runs/1", "https://github.com/other/repo/actions/runs/1", "https://evil.example/acme/billing/x", "https://github.com/acme/billing/../../other/x", "https://user@github.com/acme/billing/x", "javascript:alert(1)"} {
		if _, err := add(got, DeliveryEvidenceInput{Kind: "artefact", Verdict: CriterionPassed, URL: bad}, 5); err == nil {
			t.Fatalf("artefact link %q was accepted", bad)
		}
	}
	linked, err := add(got, DeliveryEvidenceInput{Kind: "artefact", Verdict: CriterionPassed, URL: "https://github.com/acme/billing/actions/runs/7/artifacts/9"}, 6)
	if err != nil {
		t.Fatal(err)
	}
	if last := linked.Criteria[0].Evidence[len(linked.Criteria[0].Evidence)-1]; last.Kind != "artefact" || last.URL == "" {
		t.Fatalf("artefact evidence: %+v", last)
	}
	// The check is still unverifiable, so the criterion is not passed on the link alone.
	if linked.Criteria[0].Status != CriterionUnverified {
		t.Fatalf("status with an unverifiable check: %+v", linked.Criteria[0])
	}
}

// Sending a delivery back records the decision and starts the chosen stage
// again, telling its agent the criteria that need work with the reviewer's
// note on each, the ones nobody verified, and the reviewer's notes. The work
// then comes back for review, and its earlier acceptance does not carry over.
func TestSendingADeliveryBackCarriesTheFailedCriteriaAndNotesIntoRework(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	created, err := r.s.CreateOutcome(r.p.UserID, deliveryInput(r, false))
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	v := r.review(t, created.ID)
	// Nothing to go on: no notes and no criterion that needs work.
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(v, "send_back", 1)); err == nil {
		t.Fatal("a delivery was sent back with no reason")
	}
	v = r.note(t, created.ID, v, 1, CriterionNeedsWork, "Refunds show three decimal places.", 2)
	in := decisionFor(v, "send_back", 3)
	in.Notes = "Check the refund path before sending it back to me."
	in.Stage = "plan"
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", in); err == nil {
		t.Fatal("a delivery was sent back to a stage the pipeline does not have")
	}
	in.Stage = ""
	launches := len(r.starter.asked())
	_, after, decision, recorded, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", in)
	if err != nil || !recorded {
		t.Fatalf("send back: %v", err)
	}
	if decision.Decision != "send_back" || decision.Stage != "code" || len(decision.Failed) != 1 || decision.Failed[0] != v.Criteria[1].ID || decision.Commit != fixedHead || decision.Notes != in.Notes || after.Open {
		t.Fatalf("recorded: %+v", decision)
	}
	again := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool {
		return p.State == "following" && p.Stage == 0 && p.ConversationID != ""
	})
	asked := r.starter.asked()
	if len(asked) != launches+1 || again.DeliveryAccepted {
		t.Fatalf("rework: %+v launches=%d", again, len(asked))
	}
	prompt := asked[len(asked)-1].Prompt
	for _, want := range []string{"This stage is being done again", "Sent back from the person's delivery review of commit " + fixedHead[:12], "These acceptance criteria need work: 2. Amounts keep two decimal places. (reviewer: Refunds show three decimal places.)", "These were not verified, so show how each is met: 1. The export downloads as CSV.", "The reviewer's notes: Check the refund path before sending it back to me."} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("the rework prompt lacks %q:\n%s", want, prompt)
		}
	}
	// The rework is delivered at a new commit: the earlier note is history and the review is open again.
	r.starter.mu.Lock()
	r.starter.work = func(string, bool) (terminalproto.InspectResult, error) {
		return terminalproto.InspectResult{Repo: true, Head: strings.Repeat("d", 40)}, nil
	}
	r.starter.mu.Unlock()
	back := toReview(t, r, created.ID)
	if back.DeliveryAccepted || len(back.Deliveries) != 1 {
		t.Fatalf("back for review: %+v", back)
	}
	second := r.review(t, created.ID)
	if !second.Open || second.Criteria[1].Status != CriterionUnverified || !second.Criteria[1].Evidence[0].Historical || len(second.Decisions) != 1 {
		t.Fatalf("second review: %+v", second)
	}
	// The person's plain send-back also clears an acceptance.
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(second, "accept", 4)); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "ready_to_merge" && p.DeliveryAccepted })
	if _, err := r.s.SendPipelineBack(r.p.UserID, created.ID, "a", "test", "Run the slow suite too."); err != nil {
		t.Fatal(err)
	}
	if p := r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 1 }); p.DeliveryAccepted {
		t.Fatalf("an acceptance survived a send-back: %+v", p)
	}
}

// Another account, a disabled or read-only one, a pipeline the outcome does
// not have and a criterion of another pipeline are all refused, on every
// route, and say nothing about what exists.
func TestADeliveryReviewIsItsOwnersAndItsPipelinesOnly(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	in := deliveryInput(r, false)
	in.Sessions = append(in.Sessions, OutcomeSession{Title: "Export button", Prompt: "Add the export button.", Stages: []OutcomeStageInput{{Kind: "code", Tool: "opencode"}}, Criteria: []string{"The button is on the billing page."}})
	created, err := r.s.CreateOutcome(r.p.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	v := r.review(t, created.ID)
	other, err := r.st.CreateUser("reviewer-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := r.st.CreateUser("reviewer-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for name, uid := range map[string]int64{"another account": other.ID, "a read-only account": viewer.ID} {
		if _, err := r.s.DeliveryReview(ctx, uid, created.ID, "a"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("%s read the review: %v", name, err)
		}
		if _, err := r.s.SetDeliveryCriteria(ctx, uid, created.ID, "a", DeliveryCriteriaInput{CriteriaRevision: 1, Criteria: []CriterionInput{{Text: "Mine now."}}}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("%s edited the criteria: %v", name, err)
		}
		if _, err := r.s.AddDeliveryEvidence(ctx, uid, created.ID, "a", DeliveryEvidenceInput{RequestID: request(1), ReviewRevision: v.ReviewRevision, CriterionID: v.Criteria[0].ID, Kind: "note", Verdict: CriterionPassed, Text: "Looks fine."}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("%s added evidence: %v", name, err)
		}
		if _, _, _, _, err := r.s.DecideDelivery(ctx, uid, created.ID, "a", decisionFor(v, "accept", 2)); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("%s decided the delivery: %v", name, err)
		}
	}
	// A key the outcome does not have, and an outcome that does not exist.
	if _, err := r.s.DeliveryReview(ctx, r.p.UserID, created.ID, "z"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a pipeline of another outcome: %v", err)
	}
	if _, _, _, _, err := r.s.DecideDelivery(ctx, r.p.UserID, request(7), "a", decisionFor(v, "accept", 2)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("an outcome that does not exist: %v", err)
	}
	// A criterion of the other pipeline is not this pipeline's.
	theirs := mustOutcome(t, r, created.ID).Sessions[1].Criteria[0].ID
	if _, err := r.s.AddDeliveryEvidence(ctx, r.p.UserID, created.ID, "a", DeliveryEvidenceInput{RequestID: request(3), ReviewRevision: v.ReviewRevision, CriterionID: theirs, Kind: "note", Verdict: CriterionPassed, Text: "Wrong pipeline."}); err == nil {
		t.Fatal("evidence was recorded against another pipeline's criterion")
	}
	// The owner disabled after looking can decide nothing.
	if err := r.st.SetUserDisabled(r.p.UserID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := r.s.DecideDelivery(ctx, r.p.UserID, created.ID, "a", decisionFor(v, "accept", 4)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a disabled account decided the delivery: %v", err)
	}
	if p := mustOutcome(t, r, created.ID).Sessions[0]; len(p.Deliveries) != 0 || len(p.Evidence) != 0 || p.CriteriaRevision != 1 {
		t.Fatalf("something was written: %+v", p)
	}
}

// A pipeline set to merge by itself still stops for the delivery review, and
// merges once the delivery is accepted. One with no merge stage stops before
// it is called done. Gaffer, given the outcome's decisions, accepts nothing.
func TestTheDeliveryReviewHoldsWhateverDecidesTheMerge(t *testing.T) {
	r := outcomeFixture(t)
	r.s.StartGoalSupervisor()
	auto := deliveryInput(r, true)
	auto.Automation = true
	created, err := r.s.CreateOutcome(r.p.UserID, auto)
	if err != nil {
		t.Fatal(err)
	}
	toReview(t, r, created.ID)
	time.Sleep(80 * time.Millisecond)
	if p := mustOutcome(t, r, created.ID).Sessions[0]; p.State != "delivery_review" || len(r.starter.asked()) != 2 {
		t.Fatalf("an auto-merge pipeline did not wait for its review: %+v", p)
	}
	v := r.review(t, created.ID)
	if v.Next != "merge" {
		t.Fatalf("what accepting leads to: %q", v.Next)
	}
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, created.ID, "a", decisionFor(v, "accept", 1)); err != nil {
		t.Fatal(err)
	}
	r.piece(t, created.ID, func(p store.GafferOutcomeSession) bool { return p.State == "following" && p.Stage == 2 })

	plain := deliveryInput(r, false)
	plain.Sessions[0].Stages = plain.Sessions[0].Stages[:2]
	second, err := r.s.CreateOutcome(r.p.UserID, plain)
	if err != nil {
		t.Fatal(err)
	}
	waiting := toReview(t, r, second.ID)
	if waiting.Stage != 1 || waiting.Stages[1].State != "done" {
		t.Fatalf("waiting at the end: %+v", waiting)
	}
	end := r.review(t, second.ID)
	if end.Next != "done" || strings.Join(end.Stages, ",") != "code,test" {
		t.Fatalf("review at the end: %+v", end)
	}
	if _, _, _, _, err := r.s.DecideDelivery(context.Background(), r.p.UserID, second.ID, "a", decisionFor(end, "accept", 2)); err != nil {
		t.Fatal(err)
	}
	r.piece(t, second.ID, func(p store.GafferOutcomeSession) bool { return p.State == "done" })
}

// Criteria belong to a pipeline with stages, within their limits.
func TestCriteriaAreCheckedWhenThePlanIsApproved(t *testing.T) {
	r := outcomeFixture(t)
	many := make([]string, store.MaxGafferCriteria+1)
	for i := range many {
		many[i] = "Criterion " + string(rune('a'+i))
	}
	for name, criteria := range map[string][]string{"too many": many, "too long": {strings.Repeat("x", store.MaxGafferCriterionText+1)}, "two lines": {"One\nTwo"}} {
		in := deliveryInput(r, false)
		in.Sessions[0].Criteria = criteria
		if _, err := r.s.CreateOutcome(r.p.UserID, in); err == nil {
			t.Fatalf("%s: approved", name)
		}
	}
	in := r.in
	in.Sessions[0].Criteria = []string{"It works."}
	if _, err := r.s.CreateOutcome(r.p.UserID, in); err == nil {
		t.Fatal("criteria were accepted for a session with no stages")
	}
	if r.starter.sessions() != 0 {
		t.Fatal("a refused plan started a session")
	}
}
