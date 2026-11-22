package gaffer

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A delivery review is the person's own decision on finished work, against
// the acceptance criteria they agreed before it started. A pipeline with
// criteria stops for it before it merges or is called done. The decision is
// tied to the criteria revision and the Git commit that were looked at, and
// it is never Gaffer's, a model's or an MCP client's to take. Accepting a
// delivery is not approving a merge: that stays its own action.

const (
	deliveryWaitNote = "Delivery review. The work is ready for your decision against the criteria you agreed. Nothing merges until you accept it."

	maxEvidenceTextBytes  = 1000
	maxDeliveryNotesBytes = 2000
	// maxDeliverySentBackBytes bounds what a sent-back stage is told, so the
	// stage's boundary still fits in its prompt beside a handover.
	maxDeliverySentBackBytes = 2400
	deliveryReadWait         = 20 * time.Second
)

// Statuses of a criterion and states of the reviewed commit.
const (
	CriterionPassed     = "passed"
	CriterionNeedsWork  = "needs_work"
	CriterionUnverified = "unverified"

	commitVerified    = "verified"
	commitNone        = "none"
	commitUnverified  = "unverified"
	commitUnreachable = "unreachable"
)

// DeliveryConflict is a write refused because the review is no longer what
// the person was shown: someone decided first, the criteria or evidence
// changed, or a new commit arrived. Code says which, for the page.
type DeliveryConflict struct{ Code, Message string }

func (e *DeliveryConflict) Error() string { return e.Message }

func deliveryConflict(code, message string) error { return &DeliveryConflict{code, message} }

// DeliveryInspector reads the Git state of the folder a pipeline works in,
// from the machine: the commit it is at and, when github is set, its pull
// request and checks. It is the same bounded, read-only read the session page
// uses, addressed by the controller's own record of the folder.
type DeliveryInspector interface {
	OutcomeWorkState(ctx context.Context, uid int64, agentID, dir string, github bool) (terminalproto.InspectResult, error)
}

// awaitsDelivery reports whether a pipeline stops for the person's delivery
// review before it merges or finishes.
func awaitsDelivery(item store.GafferOutcomeSession) bool {
	return len(item.Criteria) > 0 && !item.DeliveryAccepted
}

// CriterionInput is one criterion as the person wrote it. ID names the
// criterion being kept or reworded; empty makes a new one.
type CriterionInput struct {
	ID   string `json:"id,omitempty"`
	Text string `json:"text"`
}

func validCriterionText(text string) bool {
	return text != "" && len(text) <= store.MaxGafferCriterionText && store.ValidWorkerText(text) && !strings.ContainsAny(text, "\n\r\t")
}

// newCriteria turns what the person wrote into the saved criteria at
// revision. A criterion that names a known ID keeps it, and keeps its own
// revision unless its wording changed; blank rows are dropped.
func newCriteria(old []store.GafferCriterion, items []CriterionInput, revision int) ([]store.GafferCriterion, error) {
	known := map[string]store.GafferCriterion{}
	for _, c := range old {
		known[c.ID] = c
	}
	out, used := []store.GafferCriterion{}, map[string]bool{}
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		if !validCriterionText(text) {
			return nil, invalid("criteria", "Write each acceptance criterion on one line, in up to "+strconv.Itoa(store.MaxGafferCriterionText)+" bytes of plain text.")
		}
		if len(out) >= store.MaxGafferCriteria {
			return nil, invalid("criteria", "A pipeline has at most "+strconv.Itoa(store.MaxGafferCriteria)+" acceptance criteria.")
		}
		c := store.GafferCriterion{Text: text, Revision: revision}
		if item.ID != "" {
			have, ok := known[item.ID]
			if !ok || used[item.ID] {
				return nil, invalid("criteria", "One of those criteria is no longer part of this pipeline. Reload the review and edit it again.")
			}
			c.ID = have.ID
			if have.Text == text {
				c.Revision = have.Revision
			}
		} else {
			id, err := newChatID()
			if err != nil {
				return nil, err
			}
			c.ID = id
		}
		used[c.ID] = true
		out = append(out, c)
	}
	return out, nil
}

// planCriteria are the criteria a pipeline is approved with.
func planCriteria(texts []string) ([]store.GafferCriterion, error) {
	items := make([]CriterionInput, 0, len(texts))
	for _, text := range texts {
		items = append(items, CriterionInput{Text: text})
	}
	return newCriteria(nil, items, 1)
}

// criteriaPrompt tells a stage's agent what the person will review the
// delivery against. Empty for a pipeline with no criteria.
func criteriaPrompt(criteria []store.GafferCriterion) string {
	if len(criteria) == 0 {
		return ""
	}
	text := "\n\nAcceptance criteria the person agreed for this work, and will review the delivery against:"
	for i, c := range criteria {
		text += "\n" + strconv.Itoa(i+1) + ". " + c.Text
	}
	return text
}

// checkTone classes a GitHub check state as good, bad or pending, as the
// session page does.
func checkTone(state string) string {
	switch state {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return "good"
	case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return "bad"
	}
	return "pending"
}

// deliveryHead is what the machine said about the folder a pipeline works in.
type deliveryHead struct {
	state, note, commit string
	branch, remote      string
	pull                *terminalproto.InspectPull
}

// atHead reports whether GitHub's pull request is at the reviewed commit, so
// its checks are checks of that commit.
func (h deliveryHead) atHead() bool {
	return h.state == commitVerified && h.pull != nil && h.pull.HeadOID != "" && h.pull.HeadOID == h.commit
}

// decidable reports whether a decision can name what was looked at.
func (h deliveryHead) decidable() bool { return h.state == commitVerified || h.state == commitNone }

// readDeliveryHead asks the pipeline's machine where its folder stands. Every
// failure is a state the page can show; none is read as "nothing changed".
func (s *Service) readDeliveryHead(ctx context.Context, o store.GafferOutcome, item store.GafferOutcomeSession, github bool) deliveryHead {
	if item.WorkDir == "" {
		return deliveryHead{state: commitUnverified, note: "This pipeline has not started work in a folder yet."}
	}
	s.mu.Lock()
	inspector, _ := s.starter.(DeliveryInspector)
	s.mu.Unlock()
	if inspector == nil {
		return deliveryHead{state: commitUnreachable, note: "Remote sessions are disabled on this controller, so the reviewed commit cannot be read."}
	}
	ctx, cancel := context.WithTimeout(ctx, deliveryReadWait)
	defer cancel()
	got, err := inspector.OutcomeWorkState(ctx, o.OwnerID, o.AgentID, item.WorkDir, github)
	if err != nil {
		return deliveryHead{state: commitUnreachable, note: "The machine could not be asked for the commit this work is at. Try again when it is online."}
	}
	head := deliveryHead{branch: got.Branch, remote: got.Remote, pull: got.Pull}
	switch {
	case got.Reason == "not_repo":
		head.state, head.note = commitNone, "This folder is not a Git repository, so the decision is recorded without a commit."
	case got.Head != "" && terminalproto.ValidCommit(got.Head):
		head.state, head.commit = commitVerified, got.Head
	case got.Repo:
		head.state, head.note = commitUnverified, "The machine did not report the commit this work is at. Update reet on that machine, then look again."
	default:
		head.state, head.note = commitUnreachable, "The machine could not read this folder just now. Try again in a moment."
	}
	return head
}

// DeliveryPull is the pull request of the reviewed work, as GitHub has it.
type DeliveryPull struct {
	Number     int    `json:"number"`
	URL        string `json:"url"`
	State      string `json:"state"`
	HeadCommit string `json:"headCommit,omitempty"`
	// AtHead is whether GitHub's copy is at the reviewed commit. When it is
	// not, its checks say nothing about that commit.
	AtHead bool `json:"atHead"`
}

type DeliveryCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Tone  string `json:"tone"`
	URL   string `json:"url,omitempty"`
}

// DeliveryEvidenceView is one piece of evidence as it stands now. Historical
// marks evidence of an earlier commit or an earlier wording of its criterion,
// which is shown and never counted. Unverified marks evidence that would
// count but could not be checked: a check GitHub no longer reports for the
// reviewed commit, or any evidence while the commit itself cannot be read.
type DeliveryEvidenceView struct {
	store.GafferEvidence
	Historical bool `json:"historical"`
	Unverified bool `json:"unverified"`
}

type DeliveryCriterionView struct {
	ID       string                 `json:"id"`
	Text     string                 `json:"text"`
	Status   string                 `json:"status"`
	Evidence []DeliveryEvidenceView `json:"evidence"`
}

// DeliveryReviewView is one pipeline's delivery review as the person is shown
// it: the criteria with their status and evidence for the commit the machine
// reports now, and the decisions already recorded.
type DeliveryReviewView struct {
	OutcomeID string `json:"outcomeId"`
	Key       string `json:"key"`
	Title     string `json:"title"`
	State     string `json:"state"`
	// Open is whether the pipeline is waiting for a delivery decision.
	Open             bool                           `json:"open"`
	CriteriaRevision int                            `json:"criteriaRevision"`
	ReviewRevision   int                            `json:"reviewRevision"`
	Commit           string                         `json:"commit"`
	CommitState      string                         `json:"commitState"`
	CommitNote       string                         `json:"commitNote"`
	Branch           string                         `json:"branch,omitempty"`
	Remote           string                         `json:"remote,omitempty"`
	Pull             *DeliveryPull                  `json:"pull,omitempty"`
	Checks           []DeliveryCheck                `json:"checks"`
	Criteria         []DeliveryCriterionView        `json:"criteria"`
	Decisions        []store.GafferDeliveryDecision `json:"decisions"`
	// Accepted is whether the delivery in hand was accepted, AcceptedCommit
	// the commit that was, and AcceptedCurrent whether that is still the
	// commit and criteria the pipeline is at.
	Accepted        bool   `json:"accepted"`
	AcceptedCommit  string `json:"acceptedCommit"`
	AcceptedCurrent bool   `json:"acceptedCurrent"`
	// Stages are the stages a delivery can be sent back to, DefaultStage the
	// one offered first, and Next what accepting leads to: "ready_to_merge"
	// (the merge still waits for its own approval), "merge" or "done".
	Stages       []string `json:"stages"`
	DefaultStage string   `json:"defaultStage"`
	Next         string   `json:"next"`
}

// counts reports whether evidence is of the commit and the wording under
// review. unknown is set when that cannot be told because the commit could
// not be read.
func evidenceCounts(ev store.GafferEvidence, c store.GafferCriterion, head deliveryHead) (counts, unknown bool) {
	if ev.CriteriaRevision < c.Revision {
		return false, false
	}
	switch head.state {
	case commitVerified:
		return ev.Commit == head.commit, false
	case commitNone:
		return ev.Commit == "", false
	}
	return false, true
}

// criterionView works out one criterion's status from the evidence that
// counts. A reviewer's latest note or artefact is their present verdict; a
// check is read from GitHub as it is now, never from what was saved.
func criterionView(c store.GafferCriterion, evidence []store.GafferEvidence, head deliveryHead) DeliveryCriterionView {
	view := DeliveryCriterionView{ID: c.ID, Text: c.Text, Status: CriterionUnverified, Evidence: []DeliveryEvidenceView{}}
	live := map[string]terminalproto.InspectCheck{}
	if head.atHead() {
		for _, check := range head.pull.Checks {
			live[check.Name] = check
		}
	}
	manual, good, bad, unsure := "", false, false, false
	for _, ev := range evidence {
		if ev.CriterionID != c.ID {
			continue
		}
		item := DeliveryEvidenceView{GafferEvidence: ev}
		counts, unknown := evidenceCounts(ev, c, head)
		switch {
		case unknown:
			item.Unverified = true
		case !counts:
			item.Historical = true
		case ev.Kind == "check":
			check, ok := live[ev.Name]
			if !ok {
				item.Unverified, unsure = true, true
				break
			}
			item.State, item.URL, item.Verdict = check.State, check.URL, ""
			switch checkTone(check.State) {
			case "good":
				item.Verdict, good = CriterionPassed, true
			case "bad":
				item.Verdict, bad = CriterionNeedsWork, true
			default:
				unsure = true
			}
		default:
			manual = ev.Verdict
		}
		view.Evidence = append(view.Evidence, item)
	}
	switch {
	case manual == CriterionNeedsWork || bad:
		view.Status = CriterionNeedsWork
	case unsure:
		view.Status = CriterionUnverified
	case manual == CriterionPassed || good:
		view.Status = CriterionPassed
	}
	return view
}

func deliveryView(o store.GafferOutcome, item store.GafferOutcomeSession, head deliveryHead) DeliveryReviewView {
	view := DeliveryReviewView{OutcomeID: o.ID, Key: item.Key, Title: item.Title, State: item.State, Open: item.State == "delivery_review" && o.ArchivedAt == 0,
		CriteriaRevision: item.CriteriaRevision, ReviewRevision: item.ReviewRevision, Commit: head.commit, CommitState: head.state, CommitNote: head.note,
		Branch: head.branch, Remote: head.remote, Checks: []DeliveryCheck{}, Criteria: []DeliveryCriterionView{}, Decisions: []store.GafferDeliveryDecision{}, Stages: []string{}}
	if head.pull != nil {
		view.Pull = &DeliveryPull{Number: head.pull.Number, URL: head.pull.URL, State: head.pull.State, HeadCommit: head.pull.HeadOID, AtHead: head.atHead()}
		if head.atHead() {
			for _, check := range head.pull.Checks {
				view.Checks = append(view.Checks, DeliveryCheck{Name: check.Name, State: check.State, Tone: checkTone(check.State), URL: check.URL})
			}
		}
	}
	for _, c := range item.Criteria {
		view.Criteria = append(view.Criteria, criterionView(c, item.Evidence, head))
	}
	view.Decisions = append(view.Decisions, item.Deliveries...)
	if item.DeliveryAccepted {
		for i := len(item.Deliveries) - 1; i >= 0; i-- {
			if d := item.Deliveries[i]; d.Decision == "accept" {
				view.Accepted, view.AcceptedCommit = true, d.Commit
				view.AcceptedCurrent = head.decidable() && d.Commit == head.commit && d.CriteriaRevision == item.CriteriaRevision
				break
			}
		}
	}
	at := item.Stage
	if at >= len(item.Stages) {
		at = len(item.Stages) - 1
	}
	for i := 0; i <= at; i++ {
		kind := item.Stages[i].Kind
		if len(view.Stages) == 0 || view.Stages[len(view.Stages)-1] != kind {
			view.Stages = append(view.Stages, kind)
		}
		if kind == "code" {
			view.DefaultStage = kind
		}
	}
	if view.DefaultStage == "" && len(view.Stages) > 0 {
		view.DefaultStage = view.Stages[len(view.Stages)-1]
	}
	view.Next = "done"
	if item.Stage < len(item.Stages) && item.Stages[item.Stage].Kind == "merge" && item.Stages[item.Stage].State != "done" {
		view.Next = "ready_to_merge"
		if item.MergeApproved || MergesByItself(o, item) {
			view.Next = "merge"
		}
	}
	return view
}

func (v DeliveryReviewView) statuses() map[string]string {
	out := map[string]string{}
	for _, c := range v.Criteria {
		out[c.ID] = c.Status
	}
	return out
}

// withoutEvidence is an outcome as the Factory page reads it every few
// seconds: the criteria and the decisions, which Home and the list show, and
// not the evidence, which only the review itself shows and reads for itself.
func withoutEvidence(o store.GafferOutcome) store.GafferOutcome {
	sessions := append([]store.GafferOutcomeSession(nil), o.Sessions...)
	for i := range sessions {
		sessions[i].Evidence = nil
	}
	o.Sessions = sessions
	return o
}

// deliveryPipeline reads one pipeline of an outcome the account owns. An
// outcome of another account, or a key the outcome does not have, is not
// found: nothing says it exists.
func (s *Service) deliveryPipeline(uid int64, id, key string) (store.User, store.GafferOutcome, store.GafferOutcomeSession, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.User{}, store.GafferOutcome{}, store.GafferOutcomeSession{}, err
	}
	user, ok, err := s.store.UserByID(uid)
	if err != nil || !ok {
		return store.User{}, store.GafferOutcome{}, store.GafferOutcomeSession{}, store.ErrNotFound
	}
	o, err := s.store.GafferOutcomeFor(uid, id)
	if err != nil {
		return user, o, store.GafferOutcomeSession{}, err
	}
	o = outcomeView(o)
	for _, piece := range o.Sessions {
		if piece.Key == key {
			return user, o, piece, nil
		}
	}
	return user, o, store.GafferOutcomeSession{}, store.ErrNotFound
}

// DeliveryReview reads one pipeline's delivery review. A pipeline with
// criteria has its machine asked for the commit and pull request; one without
// has nothing to review and no machine is asked.
func (s *Service) DeliveryReview(ctx context.Context, uid int64, id, key string) (DeliveryReviewView, error) {
	_, o, item, err := s.deliveryPipeline(uid, id, key)
	if err != nil {
		return DeliveryReviewView{}, err
	}
	return s.deliveryRead(ctx, o, item), nil
}

func (s *Service) deliveryRead(ctx context.Context, o store.GafferOutcome, item store.GafferOutcomeSession) DeliveryReviewView {
	head := deliveryHead{state: commitUnverified}
	if len(item.Criteria) > 0 {
		head = s.readDeliveryHead(ctx, o, item, true)
	}
	return deliveryView(o, item, head)
}

// deliveryAfter is the review after a write, against the machine's answer
// that write was made on: one write asks the machine once.
func (s *Service) deliveryAfter(uid int64, id, key string, head deliveryHead) (store.GafferOutcome, DeliveryReviewView, error) {
	_, o, item, err := s.deliveryPipeline(uid, id, key)
	if err != nil {
		return o, DeliveryReviewView{}, err
	}
	return o, deliveryView(o, item, head), nil
}

func actorName(u store.User) string {
	if name := strings.TrimSpace(u.Name); name != "" {
		return name
	}
	return u.Email
}

// DeliveryCriteriaInput is the person editing a pipeline's criteria, on the
// criteria revision they were shown.
type DeliveryCriteriaInput struct {
	CriteriaRevision int              `json:"criteriaRevision"`
	Criteria         []CriterionInput `json:"criteria"`
}

// SetDeliveryCriteria replaces a pipeline's acceptance criteria. Evidence
// recorded against a wording that changed stops counting. A pipeline that
// never had criteria can be given some while it is still at work, and then
// stops for a delivery review like any other.
func (s *Service) SetDeliveryCriteria(ctx context.Context, uid int64, id, key string, in DeliveryCriteriaInput) (DeliveryReviewView, error) {
	_, o, item, err := s.deliveryPipeline(uid, id, key)
	if err != nil {
		return DeliveryReviewView{}, err
	}
	if o.ArchivedAt > 0 {
		return DeliveryReviewView{}, errOutcomeArchived
	}
	if len(item.Stages) == 0 {
		return DeliveryReviewView{}, invalid("criteria", "Acceptance criteria belong to a pipeline with stages.")
	}
	if len(in.Criteria) > 2*store.MaxGafferCriteria {
		return DeliveryReviewView{}, invalid("criteria", "A pipeline has at most "+strconv.Itoa(store.MaxGafferCriteria)+" acceptance criteria.")
	}
	var refused error
	_, err = s.outcomeStep(uid, id, func(o *store.GafferOutcome) bool {
		refused = nil
		for i := range o.Sessions {
			row := &o.Sessions[i]
			if row.Key != key {
				continue
			}
			if o.ArchivedAt > 0 {
				refused = errOutcomeArchived
				return false
			}
			if row.CriteriaRevision != in.CriteriaRevision {
				refused = deliveryConflict("delivery_criteria_changed", "The criteria changed since you looked. Look at them again before editing.")
				return false
			}
			switch row.State {
			case "waiting", "starting", "following", "advancing", "delivery_review", "unfollowed":
			case "ready_to_merge":
				// Past its delivery review: criteria can be reworded, not begun.
				if len(row.Criteria) == 0 {
					refused = invalid("criteria", "This pipeline is already waiting for its merge. Send it back to give it acceptance criteria.")
					return false
				}
			default:
				refused = invalid("criteria", "This pipeline has finished or was not started, so its criteria can no longer change.")
				return false
			}
			next, err := newCriteria(row.Criteria, in.Criteria, row.CriteriaRevision+1)
			if err != nil {
				refused = err
				return false
			}
			if len(next) == 0 && row.State == "delivery_review" {
				refused = invalid("criteria", "This delivery is waiting for your decision. Accept it or send it back before removing every criterion.")
				return false
			}
			same := len(next) == len(row.Criteria)
			for j := 0; same && j < len(next); j++ {
				same = next[j].ID == row.Criteria[j].ID && next[j].Text == row.Criteria[j].Text
			}
			if same {
				return false
			}
			row.Criteria, row.CriteriaRevision, row.ReviewRevision, row.UpdatedAt = next, row.CriteriaRevision+1, row.ReviewRevision+1, time.Now().UnixMilli()
			return true
		}
		refused = store.ErrNotFound
		return false
	})
	if refused != nil {
		return DeliveryReviewView{}, refused
	}
	if err != nil {
		return DeliveryReviewView{}, err
	}
	return s.DeliveryReview(ctx, uid, id, key)
}

// DeliveryEvidenceInput is the reviewer recording evidence against one
// criterion, on the review they were shown.
type DeliveryEvidenceInput struct {
	RequestID      string `json:"requestId"`
	ReviewRevision int    `json:"reviewRevision"`
	CriterionID    string `json:"criterionId"`
	Kind           string `json:"kind"`
	Verdict        string `json:"verdict,omitempty"`
	Text           string `json:"text,omitempty"`
	URL            string `json:"url,omitempty"`
	Name           string `json:"name,omitempty"`
}

// artefactLink accepts only a link inside the pipeline's own GitHub
// repository, as the machine named it: a run, an artefact, a commit, a file.
func artefactLink(raw, remote string) bool {
	if remote == "" || len(raw) > 2048 || strings.ContainsAny(raw, " \t\r\n") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.Opaque != "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(u.EscapedPath()), "/"+strings.ToLower(remote)+"/") && !strings.Contains(u.EscapedPath(), "..")
}

// AddDeliveryEvidence records evidence against a criterion at the commit the
// machine reports now. A note is the reviewer's own and is kept as such; an
// artefact is a link inside the pipeline's repository; a check is one GitHub
// reports for the reviewed commit, whose state is read from the machine and
// never taken from the request.
func (s *Service) AddDeliveryEvidence(ctx context.Context, uid int64, id, key string, in DeliveryEvidenceInput) (DeliveryReviewView, error) {
	user, o, item, err := s.deliveryPipeline(uid, id, key)
	if err != nil {
		return DeliveryReviewView{}, err
	}
	if !store.ValidGafferOutcomeID(in.RequestID) {
		return DeliveryReviewView{}, invalid("requestId", "A request ID is 32 lowercase hexadecimal characters.")
	}
	for _, ev := range item.Evidence {
		if ev.ID == in.RequestID {
			// Asked for before under this request: it is already recorded.
			return s.deliveryRead(ctx, o, item), nil
		}
	}
	if o.ArchivedAt > 0 {
		return DeliveryReviewView{}, errOutcomeArchived
	}
	if item.State != "delivery_review" {
		return DeliveryReviewView{}, deliveryConflict("delivery_closed", "This delivery is not waiting for a decision any more. Look at it again.")
	}
	if item.ReviewRevision != in.ReviewRevision {
		return DeliveryReviewView{}, deliveryConflict("delivery_changed", "This review changed since you looked. Look at it again before adding to it.")
	}
	found := false
	for _, c := range item.Criteria {
		found = found || c.ID == in.CriterionID
	}
	if !found {
		return DeliveryReviewView{}, invalid("criterionId", "That criterion is not part of this pipeline.")
	}
	text := strings.TrimSpace(in.Text)
	if text != "" && (len(text) > maxEvidenceTextBytes || !store.ValidWorkerText(text)) {
		return DeliveryReviewView{}, invalid("text", "Write the note in up to 1,000 UTF-8 bytes of plain text.")
	}
	verdict := in.Verdict
	if in.Kind != "check" && verdict != CriterionPassed && verdict != CriterionNeedsWork {
		return DeliveryReviewView{}, invalid("verdict", "Say whether this shows the criterion passed or needs work.")
	}
	switch in.Kind {
	case "note":
		if text == "" {
			return DeliveryReviewView{}, invalid("text", "Write what you checked.")
		}
	case "artefact", "check":
	default:
		return DeliveryReviewView{}, invalid("kind", "Evidence is a reviewer note, an artefact link or a GitHub check.")
	}
	head := s.readDeliveryHead(ctx, o, item, true)
	if !head.decidable() {
		return DeliveryReviewView{}, deliveryConflict("delivery_head_unverified", head.note)
	}
	ev := store.GafferEvidence{ID: in.RequestID, CriterionID: in.CriterionID, Kind: in.Kind, Verdict: verdict, Text: text, Commit: head.commit, CriteriaRevision: item.CriteriaRevision, By: uid, ByName: actorName(user), At: time.Now().UnixMilli()}
	switch in.Kind {
	case "artefact":
		if !artefactLink(in.URL, head.remote) {
			return DeliveryReviewView{}, invalid("url", "Link an artefact inside this pipeline's own repository on github.com, by https.")
		}
		ev.URL = in.URL
	case "check":
		if !head.atHead() {
			return DeliveryReviewView{}, invalid("name", "GitHub has no pull request at the reviewed commit, so none of its checks can be evidence for it yet. Push the work, then look again.")
		}
		ok := false
		for _, check := range head.pull.Checks {
			if check.Name == in.Name && in.Name != "" {
				ev.Name, ev.State, ev.URL, ev.Verdict, ok = check.Name, check.State, check.URL, "", true
			}
		}
		if !ok {
			return DeliveryReviewView{}, invalid("name", "GitHub reports no check of that name for the reviewed commit.")
		}
	}
	var refused error
	_, err = s.outcomeStep(uid, id, func(o *store.GafferOutcome) bool {
		refused = nil
		for i := range o.Sessions {
			row := &o.Sessions[i]
			if row.Key != key {
				continue
			}
			for _, have := range row.Evidence {
				if have.ID == ev.ID {
					return false
				}
			}
			if o.ArchivedAt > 0 || row.State != "delivery_review" {
				refused = deliveryConflict("delivery_closed", "This delivery is not waiting for a decision any more. Look at it again.")
				return false
			}
			if row.ReviewRevision != in.ReviewRevision || row.CriteriaRevision != ev.CriteriaRevision {
				refused = deliveryConflict("delivery_changed", "This review changed since you looked. Look at it again before adding to it.")
				return false
			}
			row.Evidence = append(row.Evidence, ev)
			// The oldest evidence of other commits goes first; evidence of the
			// commit under review is kept while there is room for it.
			for j := 0; len(row.Evidence) > store.MaxGafferEvidence && j < len(row.Evidence); {
				if row.Evidence[j].Commit != ev.Commit {
					row.Evidence = append(row.Evidence[:j], row.Evidence[j+1:]...)
					continue
				}
				j++
			}
			if extra := len(row.Evidence) - store.MaxGafferEvidence; extra > 0 {
				row.Evidence = append([]store.GafferEvidence(nil), row.Evidence[extra:]...)
			}
			row.ReviewRevision, row.UpdatedAt = row.ReviewRevision+1, time.Now().UnixMilli()
			return true
		}
		refused = store.ErrNotFound
		return false
	})
	if refused != nil {
		return DeliveryReviewView{}, refused
	}
	if err != nil {
		return DeliveryReviewView{}, err
	}
	_, view, err := s.deliveryAfter(uid, id, key, head)
	return view, err
}

// DeliveryDecisionInput is the person's decision on a delivery, with what
// they were shown when they made it: the criteria revision, the review
// revision, the commit and each criterion's status.
type DeliveryDecisionInput struct {
	RequestID        string            `json:"requestId"`
	Decision         string            `json:"decision"`
	CriteriaRevision int               `json:"criteriaRevision"`
	ReviewRevision   int               `json:"reviewRevision"`
	Commit           string            `json:"commit"`
	Statuses         map[string]string `json:"statuses"`
	Notes            string            `json:"notes,omitempty"`
	Stage            string            `json:"stage,omitempty"`
	Confirmed        bool              `json:"confirmed"`
}

// errDeliveryReplay marks a write that found its own request already recorded.
var errDeliveryReplay = errors.New("delivery decision already recorded")

func sameDecision(d store.GafferDeliveryDecision, in DeliveryDecisionInput, notes string) bool {
	return d.Decision == in.Decision && d.Commit == in.Commit && d.CriteriaRevision == in.CriteriaRevision && d.Notes == notes && (in.Stage == "" || d.Stage == in.Stage)
}

func recordedDecision(item store.GafferOutcomeSession, request string) (store.GafferDeliveryDecision, bool) {
	for _, d := range item.Deliveries {
		if d.RequestID == request {
			return d, true
		}
	}
	return store.GafferDeliveryDecision{}, false
}

// sentBackWords is what the agent of a sent-back stage is told: the criteria
// that need work with the reviewer's note on each, the criteria nobody
// verified, and the reviewer's own notes.
func sentBackWords(view DeliveryReviewView, notes string) string {
	short := view.Commit
	if len(short) > 12 {
		short = short[:12]
	}
	text := "Sent back from the person's delivery review"
	if short != "" {
		text += " of commit " + short
	}
	text += "."
	var failed, unsure []string
	for i, c := range view.Criteria {
		line := strconv.Itoa(i+1) + ". " + c.Text
		switch c.Status {
		case CriterionNeedsWork:
			for j := len(c.Evidence) - 1; j >= 0; j-- {
				ev := c.Evidence[j]
				if ev.Historical || ev.Unverified || ev.Verdict != CriterionNeedsWork {
					continue
				}
				if ev.Kind == "check" {
					line += " (GitHub check “" + ev.Name + "” failed)"
				} else if ev.Text != "" {
					line += " (reviewer: " + clipBytes(ev.Text, 300) + ")"
				}
				break
			}
			failed = append(failed, line)
		case CriterionUnverified:
			unsure = append(unsure, line)
		}
	}
	if len(failed) > 0 {
		text += " These acceptance criteria need work: " + strings.Join(failed, " ")
	}
	if len(unsure) > 0 {
		text += " These were not verified, so show how each is met: " + strings.Join(unsure, " ")
	}
	if notes != "" {
		text += " The reviewer's notes: " + clipBytes(notes, 900)
	}
	return clipTo(text, maxDeliverySentBackBytes)
}

// DecideDelivery records the person's decision on a delivery: accept it, or
// send it back. The decision is of the review they were shown, so the commit
// is read from the machine again and the statuses worked out again first, and
// the write itself re-checks the pipeline's state and both revisions. The
// same request asked again answers the decision it already recorded; two
// people deciding at once leave one decision.
//
// Accepting lets the pipeline carry on to what comes next. It approves no
// merge: a pipeline whose merge waits for the person still waits for it.
// The bool reports whether this call recorded the decision, rather than
// answering one an earlier call recorded.
func (s *Service) DecideDelivery(ctx context.Context, uid int64, id, key string, in DeliveryDecisionInput) (store.GafferOutcome, DeliveryReviewView, store.GafferDeliveryDecision, bool, error) {
	none := store.GafferDeliveryDecision{}
	user, o, item, err := s.deliveryPipeline(uid, id, key)
	if err != nil {
		return o, DeliveryReviewView{}, none, false, err
	}
	if !store.ValidGafferOutcomeID(in.RequestID) {
		return o, DeliveryReviewView{}, none, false, invalid("requestId", "A request ID is 32 lowercase hexadecimal characters.")
	}
	if in.Decision != "accept" && in.Decision != "send_back" {
		return o, DeliveryReviewView{}, none, false, invalid("decision", "Accept the delivery or send it back.")
	}
	notes := strings.TrimSpace(in.Notes)
	if notes != "" && (len(notes) > maxDeliveryNotesBytes || !store.ValidWorkerText(notes)) {
		return o, DeliveryReviewView{}, none, false, invalid("notes", "Write the notes in up to 2,000 UTF-8 bytes of plain text.")
	}
	if in.Commit != "" && !terminalproto.ValidCommit(in.Commit) || in.Stage != "" && stageIndex(in.Stage) < 0 || len(in.Statuses) > store.MaxGafferCriteria {
		return o, DeliveryReviewView{}, none, false, invalid("decision", "Reload this delivery review and decide again.")
	}
	replay := func() (store.GafferOutcome, DeliveryReviewView, store.GafferDeliveryDecision, bool, error) {
		_, o, item, err := s.deliveryPipeline(uid, id, key)
		if err != nil {
			return o, DeliveryReviewView{}, none, false, err
		}
		d, ok := recordedDecision(item, in.RequestID)
		if !ok {
			return o, DeliveryReviewView{}, none, false, store.ErrGafferOutcomeBusy
		}
		if !sameDecision(d, in, notes) {
			return o, DeliveryReviewView{}, none, false, deliveryConflict("delivery_request_reused", "That request already recorded a different decision. Reload the review and decide again.")
		}
		return withoutEvidence(o), s.deliveryRead(ctx, o, item), d, false, nil
	}
	if _, ok := recordedDecision(item, in.RequestID); ok {
		return replay()
	}
	if !in.Confirmed {
		return o, DeliveryReviewView{}, none, false, invalid("decision", "Confirm the decision before it is recorded.")
	}
	if o.ArchivedAt > 0 {
		return o, DeliveryReviewView{}, none, false, errOutcomeArchived
	}
	closed := deliveryConflict("delivery_closed", "This delivery has already been decided, or is no longer waiting for a decision. Look at it again.")
	changed := deliveryConflict("delivery_changed", "The evidence or a check changed since you looked. Look at the review again before deciding.")
	criteriaChanged := deliveryConflict("delivery_criteria_changed", "The acceptance criteria changed since you looked. Look at the review again before deciding.")
	if item.State != "delivery_review" {
		return o, DeliveryReviewView{}, none, false, closed
	}
	if item.CriteriaRevision != in.CriteriaRevision {
		return o, DeliveryReviewView{}, none, false, criteriaChanged
	}
	if item.ReviewRevision != in.ReviewRevision {
		return o, DeliveryReviewView{}, none, false, changed
	}
	if err := s.outcomesReady(); err != nil {
		return o, DeliveryReviewView{}, none, false, err
	}
	// What the machine says now, not what the page was told: the head, and
	// GitHub's checks, which a check given as evidence is read from.
	head := s.readDeliveryHead(ctx, o, item, true)
	if !head.decidable() {
		return o, DeliveryReviewView{}, none, false, deliveryConflict("delivery_head_unverified", head.note)
	}
	if head.commit != in.Commit {
		return o, DeliveryReviewView{}, none, false, deliveryConflict("delivery_head_changed", "The work is at a different commit from the one you reviewed. Look at the review again before deciding.")
	}
	view := deliveryView(o, item, head)
	statuses := view.statuses()
	if len(statuses) != len(in.Statuses) {
		return o, DeliveryReviewView{}, none, false, changed
	}
	for id, status := range statuses {
		if in.Statuses[id] != status {
			return o, DeliveryReviewView{}, none, false, changed
		}
	}
	decision := store.GafferDeliveryDecision{RequestID: in.RequestID, Decision: in.Decision, Commit: head.commit, CriteriaRevision: item.CriteriaRevision, By: uid, ByName: actorName(user), At: time.Now().UnixMilli(), Notes: notes, Statuses: statuses}
	for _, c := range view.Criteria {
		if c.Status == CriterionNeedsWork {
			decision.Failed = append(decision.Failed, c.ID)
		}
	}
	// guard is the conditional part of the write: the pipeline is still
	// waiting, on the review and criteria the decision was made on, and this
	// request has not been recorded. It runs on the record being written.
	guard := func(row *store.GafferOutcomeSession) error {
		if _, ok := recordedDecision(*row, in.RequestID); ok {
			return errDeliveryReplay
		}
		if row.State != "delivery_review" {
			return closed
		}
		if row.CriteriaRevision != in.CriteriaRevision {
			return criteriaChanged
		}
		if row.ReviewRevision != in.ReviewRevision {
			return changed
		}
		row.Deliveries = append(row.Deliveries, decision)
		if extra := len(row.Deliveries) - store.MaxGafferDeliveries; extra > 0 {
			row.Deliveries = append([]store.GafferDeliveryDecision(nil), row.Deliveries[extra:]...)
		}
		row.ReviewRevision++
		return nil
	}
	if in.Decision == "send_back" {
		if notes == "" && len(decision.Failed) == 0 {
			return o, DeliveryReviewView{}, none, false, invalid("notes", "Say what needs to change, or record which criterion needs work, before sending this back.")
		}
		decision.Stage = in.Stage
		if decision.Stage == "" {
			decision.Stage = view.DefaultStage
		}
		words := sentBackWords(view, notes)
		_, err = s.sendPipelineBack(uid, id, key, decision.Stage, func(string) string { return words }, guard)
	} else {
		var refused error
		_, err = s.outcomeStep(uid, id, func(o *store.GafferOutcome) bool {
			refused = nil
			for i := range o.Sessions {
				row := &o.Sessions[i]
				if row.Key != key {
					continue
				}
				if o.ArchivedAt > 0 {
					refused = errOutcomeArchived
					return false
				}
				if refused = guard(row); refused != nil {
					return false
				}
				// Accepting is the person asking for the work to carry on, as
				// approving a merge is; it approves no merge itself.
				settleStoppedOutcome(o)
				row.DeliveryAccepted, row.State, row.Note, row.UpdatedAt = true, "advancing", "You accepted the delivery. Moving on.", time.Now().UnixMilli()
				o.State = "starting"
				return true
			}
			refused = store.ErrNotFound
			return false
		})
		if refused != nil {
			err = refused
		}
	}
	if errors.Is(err, errDeliveryReplay) {
		return replay()
	}
	if err != nil {
		return o, DeliveryReviewView{}, none, false, err
	}
	s.wakeOutcomes()
	o, after, err := s.deliveryAfter(uid, id, key, head)
	return withoutEvidence(o), after, decision, err == nil, err
}
