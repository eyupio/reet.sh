package gaffer

import (
	"github.com/eyupio/reet.sh/internal/store"
)

// AuditFunc records one thing Gaffer or the controller did for an account in
// the activity log. It must not call back into the Service.
type AuditFunc func(uid int64, action string, detail map[string]any)

// SetAudit installs the activity log. Without one nothing is recorded.
func (s *Service) SetAudit(record AuditFunc) {
	if record == nil {
		s.auditor.Store(nil)
		return
	}
	s.auditor.Store(&record)
}

func (s *Service) audit(uid int64, action string, detail map[string]any) {
	if record := s.auditor.Load(); record != nil {
		(*record)(uid, action, detail)
	}
}

// pipelineOf names the pipeline and the stage in hand that a conversation's
// goal follows. It reads the store only.
func (s *Service) pipelineOf(uid int64, conversation string) (outcome, pipeline, stage string, ok bool) {
	rows, err := s.store.GafferOutcomesFor(uid)
	if err != nil || conversation == "" {
		return "", "", "", false
	}
	for _, o := range rows {
		for _, piece := range o.Sessions {
			if piece.ConversationID != conversation || len(piece.Stages) == 0 {
				continue
			}
			if piece.Stage < len(piece.Stages) {
				stage = piece.Stages[piece.Stage].Kind
			}
			return o.ID, piece.Key, stage, true
		}
	}
	return "", "", "", false
}

// auditGoal records why a goal stopped being followed or was picked up again:
// it needs the person, is held while the working model waits for them, was
// released, or ran out of budget. The reason is the controller's own note,
// never model or session text.
func (s *Service) auditGoal(g store.GafferGoal, action, reason string) {
	detail := map[string]any{"conversation": g.ConversationID, "session": g.SessionID, "goal": g.ID, "reason": reason}
	if outcome, pipeline, stage, ok := s.pipelineOf(g.OwnerID, g.ConversationID); ok {
		detail["outcome"], detail["pipeline"], detail["stage"] = outcome, pipeline, stage
	}
	s.audit(g.OwnerID, action, detail)
}

// goalStateAction is the activity a goal reaching this state is recorded as.
func goalStateAction(state string) string {
	switch state {
	case "needs_user":
		return "gaffer-goal-needs-you"
	case "limit":
		return "gaffer-goal-limit"
	}
	return ""
}

// auditPipeline records where the controller left a pipeline after a step:
// a stage under way, waiting for a merge decision, done, or stopped. The
// reason is the pipeline's own note.
func (s *Service) auditPipeline(o store.GafferOutcome, key string) {
	for _, row := range o.Sessions {
		if row.Key != key {
			continue
		}
		action := ""
		switch row.State {
		case "following":
			action = "gaffer-stage-started"
		case "ready_to_merge":
			action = "gaffer-pipeline-waiting-merge"
		case "done":
			action = "gaffer-pipeline-done"
		case "unfollowed", "failed":
			action = "gaffer-pipeline-stopped"
		default:
			return
		}
		detail := map[string]any{"outcome": o.ID, "pipeline": key, "state": row.State, "reason": row.Note}
		if row.Stage < len(row.Stages) {
			detail["stage"] = row.Stages[row.Stage].Kind
		}
		if row.SessionID != "" && row.State != "done" {
			detail["session"] = row.SessionID
		}
		s.audit(o.OwnerID, action, detail)
		return
	}
}
