package gaffer

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Balancing spreads an outcome's stages across the plans the person pays for.
// A stage runs on its first choice until that agent's plan is running low in
// its tightest window; it then runs on the backup the person chose for it, if
// the backup's plan has more left. Nothing else is ever chosen for a stage.
const (
	// DefaultBalanceBelow is how much of a plan must be left, as a whole
	// percentage, for a stage to stay on its first choice, unless the person
	// chose another figure. MinBalanceBelow and MaxBalanceBelow bound theirs.
	DefaultBalanceBelow = 20
	MinBalanceBelow     = 5
	MaxBalanceBelow     = 90
	// balanceFreshFor is how old a machine's plan figures may be and still
	// move a stage. Windows start again; older figures decide nothing.
	balanceFreshFor = 6 * time.Hour
)

// planFigure is what is known of one agent's plan for one model.
type planFigure struct {
	signedIn bool
	// known is whether there are figures fresh enough to act on; left is then
	// the share left in the tightest window, as a whole percentage.
	known bool
	left  int
}

// planLeft reads an agent's plan from a machine's last answer. A window that
// covers one model counts only for a stage on that model.
func planLeft(accounts []terminalproto.AgentAccount, tool, model string, now time.Time) planFigure {
	for _, account := range accounts {
		if account.Tool != tool || account.State != terminalproto.AccountSignedIn {
			continue
		}
		out := planFigure{signedIn: true}
		if len(account.Limits) == 0 || account.LimitsAt <= 0 || now.Sub(time.UnixMilli(account.LimitsAt)) > balanceFreshFor {
			return out
		}
		name := strings.ToLower(model)
		for _, limit := range account.Limits {
			if label := strings.ToLower(limit.Label); label != "" && (name == "" || !strings.Contains(name, label)) {
				continue
			}
			if left := 100 - limit.UsedPercent; !out.known || left < out.left {
				out.known, out.left = true, left
			}
		}
		return out
	}
	return planFigure{}
}

func agentWords(a store.GafferStageAgent) string {
	name := store.AgentLabel(a.Tool)
	if a.Model != "" {
		name += " (" + a.Model + ")"
	}
	return name
}

// validBalanceBelow reports whether n is a threshold a person may choose.
// Zero is none chosen.
func validBalanceBelow(n int) bool {
	return n == 0 || n >= MinBalanceBelow && n <= MaxBalanceBelow
}

// balanceChoice reports whether a stage should run on its backup, and why.
// below is the share of the first choice's plan under which it does.
func balanceChoice(accounts []terminalproto.AgentAccount, first, backup store.GafferStageAgent, below int, now time.Time) (bool, string) {
	if below == 0 {
		below = DefaultBalanceBelow
	}
	one := planLeft(accounts, first.Tool, first.Model, now)
	if !one.known || one.left >= below {
		return false, ""
	}
	two := planLeft(accounts, backup.Tool, backup.Model, now)
	if !two.signedIn || two.known && two.left <= one.left {
		return false, ""
	}
	why := agentWords(first) + "’s plan had " + strconv.Itoa(one.left) + "% left, so this stage runs on " + agentWords(backup)
	if two.known {
		return true, why + ", which had " + strconv.Itoa(two.left) + "% left."
	}
	// Signed in with a key, or on a plan reet reads no figures for: the
	// person chose it as the backup knowing that.
	return true, why + ", which has no plan figures to run low."
}

// balanceStage returns the stage as it should start: on its backup when the
// first choice's plan is running low, on its first choice otherwise, which
// puts back a stage that was balanced on an earlier start. changed is whether
// that differs from the stage as given.
func (s *Service) balanceStage(o store.GafferOutcome, stage store.GafferOutcomeStage) (store.GafferOutcomeStage, bool) {
	spares := store.Spares(stage.Backup, stage.More)
	if len(spares) == 0 {
		return stage, false
	}
	// An effort dropped because the agent does not offer it is still the one
	// the person asked for: it is what the first choice goes back to, and what
	// tells a stage that stays where it is from one that moves.
	asked := stage.Effort
	if asked == "" {
		asked = stage.EffortNotOffered
	}
	first := store.GafferStageAgent{Tool: stage.Tool, Model: stage.Model, Effort: asked, Provider: stage.Provider}
	if stage.First != nil {
		first = *stage.First
	}
	out := stage
	out.Tool, out.Model, out.Effort, out.Provider = first.Tool, first.Model, first.Effort, first.Provider
	out.First, out.Balanced = nil, ""
	if kept, ok, err := s.store.MachineAccountsFor(o.OwnerID, o.AgentID); err == nil && ok {
		// The backups are tried in the order the person gave them: the first
		// one with more of its plan left than the first choice does the stage.
		for _, spare := range spares {
			if use, why := balanceChoice(kept.Accounts, first, spare, o.BalanceBelow, time.Now()); use {
				out.Tool, out.Model, out.Effort, out.Provider = spare.Tool, spare.Model, spare.Effort, spare.Provider
				out.First, out.Balanced = &first, why
				break
			}
		}
	}
	changed := out.Tool != stage.Tool || out.Model != stage.Model || out.Effort != asked || out.Provider != stage.Provider || out.Balanced != stage.Balanced || (out.First == nil) != (stage.First == nil)
	if changed {
		// The record of a dropped effort was made for the agent the stage is
		// leaving; the one it moves to is asked for its own effort afresh.
		out.EffortNotOffered = ""
	}
	return out, changed
}

// rebalanceStopped runs a stage again on its backup when its agent has
// stopped and its plan is now below the person's threshold. A working agent
// is never interrupted: only a session that has ended, or whose agent has
// stopped or failed, is replaced, and only once for a stage.
func (s *Service) rebalanceStopped(ctx context.Context) {
	rows, err := s.store.BackedGafferOutcomes()
	if err != nil {
		return
	}
	for _, o := range rows {
		for _, piece := range o.Sessions {
			if ctx.Err() != nil {
				return
			}
			if piece.State != "following" || piece.Stage >= len(piece.Stages) || piece.SessionID == "" {
				continue
			}
			stage := piece.Stages[piece.Stage]
			if stage.Rebalanced || stage.Backup == nil {
				continue
			}
			running := s.sessionRunning(o.OwnerID, piece.SessionID)
			if running {
				// Still there: only an agent that has itself stopped or failed counts.
				g, err := s.store.GafferGoalFor(o.OwnerID, piece.ConversationID)
				halted, _ := s.store.GafferWorkerHalted(o.OwnerID, piece.SessionID)
				if err != nil || g.State != "needs_user" || g.ActiveTurnID != "" || g.WaitingOnWorker || !halted {
					continue
				}
			}
			next, changed := s.balanceStage(o, stage)
			if !changed || next.Balanced == "" {
				continue
			}
			if _, err := s.chatPrincipal(o.OwnerID); err != nil {
				continue
			}
			key, at, marked := piece.Key, piece.Stage, false
			after, err := s.outcomeStep(o.OwnerID, o.ID, func(o *store.GafferOutcome) bool {
				marked = false
				for i := range o.Sessions {
					if row := &o.Sessions[i]; row.Key == key && row.Stage == at && row.State == "following" && at < len(row.Stages) && !row.Stages[at].Rebalanced {
						row.Stages[at].Rebalanced, marked = true, true
						return true
					}
				}
				return false
			})
			if err != nil || !marked {
				continue
			}
			if running {
				// The session is the pipeline's own in hand, so it is ended here
				// and not by the helper that only ends sessions nothing uses.
				s.mu.Lock()
				ender, _ := s.starter.(SessionEnder)
				s.mu.Unlock()
				if ender == nil {
					continue
				}
				endCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				err := ender.EndOutcomeSession(endCtx, o.OwnerID, piece.SessionID)
				cancel()
				if err != nil {
					continue
				}
				s.leftBehind(o.OwnerID, piece.SessionID)
				s.audit(o.OwnerID, "gaffer-session-ended", map[string]any{"outcome": o.ID, "pipeline": key, "session": piece.SessionID, "reason": "Its agent stopped with its plan running low, so the stage is run again on its backup."})
			}
			if _, err := s.RetryPipeline(o.OwnerID, o.ID, key, after.Revision); err != nil {
				continue
			}
			s.audit(o.OwnerID, "gaffer-stage-rebalanced", map[string]any{"outcome": o.ID, "pipeline": key, "stage": stage.Kind, "from": stage.Tool, "to": next.Tool, "reason": next.Balanced})
		}
	}
}
