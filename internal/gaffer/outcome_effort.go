package gaffer

import (
	"slices"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// An effort the agent does not offer.
//
// A stage's effort is one of its agent's own thinking levels, and the machine
// refuses a start whose effort the agent does not confirm for the model, with
// no prompt sent. A wrong effort is not worth stopping a pipeline for: the
// stage starts on the agent's own default effort instead, and says so. The
// machine's last report is read first, so a level it says the agent lacks is
// not asked for at all; a start refused for the effort all the same is asked
// for once more without it. A wrong model still stops the stage.

// effortNotReported reports whether the machine's last report of its
// capabilities shows that the agent does not offer effort for a chat: it
// describes the agent's options and lists no thought level, or not this one.
// A machine that has not reported, whose reet takes no start options, or that
// does not describe its agents' options, says nothing either way; its
// refusal, if it comes, is handled when the start is answered.
func (s *Service) effortNotReported(uid int64, agentID, tool, effort string) bool {
	if effort == "" {
		return false
	}
	reported, err := s.store.RemoteCapabilitiesFor(uid, agentID)
	if err != nil {
		return false
	}
	caps := reported.Capabilities
	if !caps.ACP {
		return false
	}
	// A machine whose reet takes no start options is too old to say: its own
	// refusal, which tells the person to update it, stands.
	if !caps.ACPStartOptions || !caps.ACPAgentOptions {
		return false
	}
	chat, ok := caps.ChatAgent(tool)
	if !ok {
		// Not available as a chat at all: the start says why.
		return false
	}
	for _, option := range chat.Options {
		if option.Category == terminalproto.AgentOptionThoughtLevel {
			return !slices.Contains(option.Values, effort)
		}
	}
	return true
}

// dropStageEffort records on the stage in hand that its agent does not offer
// the effort its launch asks for, so the stage starts on the agent's default
// effort until it is asked for afresh (tried again or sent back). The launch's
// request is the one the stage is being started under; request, when set, is
// a fresh one to ask again under, because the machine refused the one before.
// A launch with no effort, for a stage whose effort was already dropped, only
// moves to the fresh request: the record stands and is not logged again. how is "reported" (the
// machine's report said so) or "refused" (it refused the start). It reports
// false when the pipeline is no longer the controller's to start, or the
// stage has moved on: nothing is then recorded and nothing should be asked for.
func (s *Service) dropStageEffort(o store.GafferOutcome, key string, at int, kind string, launch OutcomeLaunch, request, how string) bool {
	effort, from, dropped := launch.Effort, launch.RequestID, false
	if effort == "" && request == "" {
		return false
	}
	_, err := s.outcomeStep(o.OwnerID, o.ID, func(o *store.GafferOutcome) bool {
		dropped = false
		for i := range o.Sessions {
			row := &o.Sessions[i]
			if row.Key != key || row.Stage != at || at >= len(row.Stages) || row.SessionID != "" || !controllerHolds(row.State) || row.Stages[at].RequestID != from {
				continue
			}
			if effort != "" {
				row.Stages[at].Effort, row.Stages[at].EffortNotOffered = "", effort
			} else if row.Stages[at].EffortNotOffered == "" || row.Stages[at].Effort != "" {
				return false
			}
			if request != "" {
				// The first stage's request is the pipeline's own as well.
				if row.RequestID == from {
					row.RequestID = request
				}
				row.Stages[at].RequestID = request
			}
			row.UpdatedAt = time.Now().UnixMilli()
			dropped = true
			return true
		}
		return false
	})
	if err != nil || !dropped {
		return false
	}
	if effort == "" {
		return true
	}
	// Written before the start is answered, so it says what is asked for,
	// not that anything started.
	s.audit(o.OwnerID, "gaffer-stage-effort-not-offered", map[string]any{
		"outcome": o.ID, "pipeline": key, "stage": kind,
		"agent": launch.Tool, "model": launch.Model, "effort": effort, "how": how,
		"reason": terminalproto.ToolName(launch.Tool) + " does not offer it for this model here.",
	})
	return true
}
