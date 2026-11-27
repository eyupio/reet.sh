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
// no prompt sent. A wrong effort is not worth stopping a pipeline for. The
// machine's last report is read first: a level it says the agent lacks is not
// asked for, and the stage starts on the nearest lower level the agent lists,
// or on its default when it lists none below, and says so. A start refused for
// the effort all the same is asked for once more on the agent's default. A
// wrong model still stops the stage.

// effortLadder orders the thinking levels agents name, lightest first. "none"
// is left out: stepping down never turns thinking off.
var effortLadder = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// nearestEffort is the heaviest of offered that is lighter than effort, or ""
// when there is none or effort is not on the ladder.
func nearestEffort(effort string, offered []string) string {
	at := slices.Index(effortLadder, effort)
	for i := at - 1; i >= 0; i-- {
		if slices.Contains(offered, effortLadder[i]) {
			return effortLadder[i]
		}
	}
	return ""
}

// effortNotReported reports whether the machine's last report of its
// capabilities shows that the agent does not offer effort for a chat: it
// describes the agent's options and lists no thought level, or not this one.
// instead is the nearest lower level it lists, or "" for the agent's default.
// A machine that has not reported, whose reet takes no start options, or that
// does not describe its agents' options, says nothing either way; its
// refusal, if it comes, is handled when the start is answered.
func (s *Service) effortNotReported(uid int64, agentID, tool, effort string) (notOffered bool, instead string) {
	if effort == "" {
		return false, ""
	}
	reported, err := s.store.RemoteCapabilitiesFor(uid, agentID)
	if err != nil {
		return false, ""
	}
	caps := reported.Capabilities
	if !caps.ACP {
		return false, ""
	}
	// A machine whose reet takes no start options is too old to say: its own
	// refusal, which tells the person to update it, stands.
	if !caps.ACPStartOptions || !caps.ACPAgentOptions {
		return false, ""
	}
	chat, ok := caps.ChatAgent(tool)
	if !ok {
		// Not available as a chat at all: the start says why.
		return false, ""
	}
	for _, option := range chat.Options {
		if option.Category == terminalproto.AgentOptionThoughtLevel {
			if slices.Contains(option.Values, effort) {
				return false, ""
			}
			return true, nearestEffort(effort, option.Values)
		}
	}
	return true, ""
}

// dropStageEffort records on the stage in hand that its agent does not offer
// the effort its launch asks for, so the stage starts on instead (the nearest
// lower level the agent lists, or "" for its default) until it is asked for
// afresh (tried again or sent back). EffortNotOffered keeps the level the
// person chose, even when a level put in its place is refused in turn. The launch's
// request is the one the stage is being started under; request, when set, is
// a fresh one to ask again under, because the machine refused the one before.
// A launch with no effort, for a stage whose effort was already dropped, only
// moves to the fresh request: the record stands and is not logged again. how is "reported" (the
// machine's report said so) or "refused" (it refused the start). It reports
// false when the pipeline is no longer the controller's to start, or the
// stage has moved on: nothing is then recorded and nothing should be asked for.
func (s *Service) dropStageEffort(o store.GafferOutcome, key string, at int, kind string, launch OutcomeLaunch, instead, request, how string) bool {
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
				row.Stages[at].Effort = instead
				if row.Stages[at].EffortNotOffered == "" {
					row.Stages[at].EffortNotOffered = effort
				}
			} else if row.Stages[at].EffortNotOffered == "" || row.Stages[at].Effort != "" {
				return false
			}
			if request != "" {
				// The first stage's request is the pipeline's own as well.
				if row.RequestID == from {
					row.RequestID = request
				}
				// Recorded as in flight before it is sent, as every launch is.
				row.Stages[at].RequestID, row.UnsureRequest = request, request
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
		"agent": launch.Tool, "model": launch.Model, "effort": effort, "instead": instead, "how": how,
		"reason": terminalproto.ToolName(launch.Tool) + " does not offer it for this model here.",
	})
	return true
}
