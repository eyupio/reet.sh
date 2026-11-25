package gaffer

import (
	"context"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Whether a stage's agent, model and effort were applied.
//
// What the person chooses for a stage goes to the machine as a request. The
// machine refuses a launch whose model or effort the agent does not confirm,
// but the Factory kept only the choice, so the page could repeat what was
// asked for and never say what the session was on. A stage that carries on in
// the session of the stage before is not launched at all, so nothing
// confirmed its choice. After a stage's session is in Gaffer's care the
// controller therefore asks the machine what the session is running, and
// keeps the answer with the stage.

const stageChoiceWait = 20 * time.Second

// stageChoiceCheck compares what was chosen for a stage with what its session
// reports. Only what was chosen is compared: a model or effort left to the
// agent's default cannot be wrong. The note names each difference.
func stageChoiceCheck(stage store.GafferOutcomeStage, got SessionChoice) (check, note string) {
	var differs []string
	say := func(what, chose, on string) {
		if on == "" {
			differs = append(differs, "You chose the "+what+" "+chose+", and the session did not say which "+what+" it is on.")
			return
		}
		differs = append(differs, "You chose the "+what+" "+chose+"; the session is on "+on+".")
	}
	if got.Tool != stage.Tool {
		say("agent", stage.Tool, got.Tool)
	}
	if stage.Model != "" && got.Model != stage.Model {
		say("model", stage.Model, got.Model)
	}
	if stage.Effort != "" && got.Effort != stage.Effort {
		say("effort", stage.Effort, got.Effort)
	}
	// An effort the agent does not offer was dropped before the start: the
	// session is on the agent's default, which is what was asked of it then.
	dropped := ""
	if stage.Effort == "" && stage.EffortNotOffered != "" {
		dropped = effortNotOfferedNote(stage.Tool, stage.EffortNotOffered)
	}
	if len(differs) == 0 {
		return "applied", dropped
	}
	if dropped != "" {
		differs = append(differs, dropped)
	}
	return "mismatch", strings.Join(differs, " ")
}

// effortNotOfferedNote says that the effort chosen for a stage was not
// offered by its agent, so the session started on the agent's own default.
func effortNotOfferedNote(tool, effort string) string {
	return "You chose the effort " + effort + "; " + terminalproto.ToolName(tool) + " does not offer it for this model here, so the session was started on the agent's default effort."
}

// checkStageChoice asks the machine what a stage's session is running and
// records it on the stage. A difference is recorded and logged, and stops
// nothing: whether it matters is the person's call. A machine that cannot be
// asked leaves the stage marked as not checked, never as applied.
func (s *Service) checkStageChoice(ctx context.Context, o store.GafferOutcome, key string, at int, sessionID string) {
	s.mu.Lock()
	reader, _ := s.starter.(SessionChoiceReader)
	s.mu.Unlock()
	if reader == nil || sessionID == "" {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, stageChoiceWait)
	got, err := reader.OutcomeSessionChoice(readCtx, o.OwnerID, sessionID)
	cancel()
	if ctx.Err() != nil {
		return
	}
	check, note := "unchecked", ""
	if err != nil {
		got = SessionChoice{}
		note = "reet could not read back what this session is running: " + clipBytes(outcomeReason(err), 300)
	}
	unread := note
	var stage store.GafferOutcomeStage
	wrote := false
	_, stepErr := s.outcomeStep(o.OwnerID, o.ID, func(o *store.GafferOutcome) bool {
		wrote = false
		for i := range o.Sessions {
			row := &o.Sessions[i]
			if row.Key != key || row.SessionID != sessionID {
				continue
			}
			if len(row.Stages) == 0 {
				// A pipeline with no stages runs on the outcome's own agent and
				// model, so that is what its one session is compared with.
				chosen := store.GafferOutcomeStage{Tool: o.Tool, Model: o.Model}
				if err == nil {
					check, note = stageChoiceCheck(chosen, got)
				}
				row.Applied = &store.GafferAppliedChoice{Tool: got.Tool, Model: got.Model, Effort: got.Effort, At: time.Now().UnixMilli(), Check: check, Note: note, ChosenTool: o.Tool, ChosenModel: o.Model}
				stage, wrote = chosen, true
				return true
			}
			if at >= len(row.Stages) || row.Stage != at {
				continue
			}
			if err == nil {
				check, note = stageChoiceCheck(row.Stages[at], got)
			} else if dropped := row.Stages[at]; dropped.Effort == "" && dropped.EffortNotOffered != "" {
				note = unread + " " + effortNotOfferedNote(dropped.Tool, dropped.EffortNotOffered)
			}
			row.Stages[at].AppliedTool, row.Stages[at].AppliedModel, row.Stages[at].AppliedEffort = got.Tool, got.Model, got.Effort
			row.Stages[at].AppliedAt, row.Stages[at].ChoiceCheck, row.Stages[at].ChoiceNote = time.Now().UnixMilli(), check, note
			stage, wrote = row.Stages[at], true
			return true
		}
		return false
	})
	if stepErr != nil || !wrote || check != "mismatch" {
		return
	}
	s.audit(o.OwnerID, "gaffer-stage-choice-not-applied", map[string]any{
		"outcome": o.ID, "pipeline": key, "stage": stage.Label(), "session": sessionID,
		"chosen":  map[string]any{"agent": stage.Tool, "model": stage.Model, "effort": stage.Effort},
		"applied": map[string]any{"agent": got.Tool, "model": got.Model, "effort": got.Effort},
		"reason":  note,
	})
}
