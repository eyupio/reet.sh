package gaffer

import "github.com/eyupio/reet.sh/internal/store"

// stageAgentUsage is what a stage's agent used: the session's reported total
// less what earlier stages that shared the session already account for. A
// session keeps one running total, and one agent may do several stages in it.
func (s *Service) stageAgentUsage(uid int64, piece store.GafferOutcomeSession, at int) (store.TerminalSessionUsage, bool) {
	if at < 0 || at >= len(piece.Stages) || piece.Stages[at].SessionID == "" {
		return store.TerminalSessionUsage{}, false
	}
	total, err := s.store.TerminalSessionUsageFor(uid, piece.Stages[at].SessionID)
	if err != nil || !total.Reported {
		return total, false
	}
	for i, stage := range piece.Stages {
		if i == at || stage.SessionID != piece.Stages[at].SessionID || stage.State != "done" {
			continue
		}
		total.InputTokens -= stage.AgentInputTokens
		total.OutputTokens -= stage.AgentOutputTokens
		total.CachedTokens -= stage.AgentCachedTokens
	}
	if total.InputTokens < 0 {
		total.InputTokens = 0
	}
	if total.OutputTokens < 0 {
		total.OutputTokens = 0
	}
	if total.CachedTokens < 0 {
		total.CachedTokens = 0
	}
	return total, true
}

// withAgentUsage fills in, for reading, what the stage in hand of each
// pipeline has used so far and how full its agent's context is. Finished
// stages already carry the figures kept at their hand-over.
func (s *Service) withAgentUsage(uid int64, o store.GafferOutcome) store.GafferOutcome {
	for i := range o.Sessions {
		piece := &o.Sessions[i]
		if piece.SessionID != "" {
			switch {
			case s.sessionGone(uid, piece.SessionID):
				piece.SessionState = "gone"
			case s.sessionRunning(uid, piece.SessionID):
				piece.SessionState = "running"
			default:
				piece.SessionState = "ended"
			}
		}
		at := piece.Stage
		if at >= len(piece.Stages) || piece.Stages[at].State == "done" {
			continue
		}
		if piece.SessionID != "" && (piece.State == "following" || piece.State == "ready_to_merge") {
			// What the session last reported doing, so the page can tell a
			// busy agent from one that has gone quiet.
			if row, err := s.store.TerminalSessionFor(uid, piece.SessionID); err == nil {
				piece.Stages[at].AgentState, piece.Stages[at].AgentActiveAt = row.ChatStatus, row.ActivityAt
			}
		}
		if used, ok := s.stageAgentUsage(uid, *piece, at); ok {
			stage := &piece.Stages[at]
			stage.AgentInputTokens, stage.AgentOutputTokens, stage.AgentCachedTokens, stage.AgentUsageReported = used.InputTokens, used.OutputTokens, used.CachedTokens, true
			stage.ContextUsed, stage.ContextSize = used.ContextUsed, used.ContextSize
		}
	}
	return o
}
