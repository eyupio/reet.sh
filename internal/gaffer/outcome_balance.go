package gaffer

import (
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
	// balanceLowPercent is how much of a plan must be left for a stage to
	// stay on its first choice.
	balanceLowPercent = 20
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

// balanceChoice reports whether a stage should run on its backup, and why.
func balanceChoice(accounts []terminalproto.AgentAccount, first, backup store.GafferStageAgent, now time.Time) (bool, string) {
	one := planLeft(accounts, first.Tool, first.Model, now)
	if !one.known || one.left >= balanceLowPercent {
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
	if stage.Backup == nil {
		return stage, false
	}
	first := store.GafferStageAgent{Tool: stage.Tool, Model: stage.Model, Effort: stage.Effort, Provider: stage.Provider}
	if stage.First != nil {
		first = *stage.First
	}
	use, why := false, ""
	if kept, ok, err := s.store.MachineAccountsFor(o.OwnerID, o.AgentID); err == nil && ok {
		use, why = balanceChoice(kept.Accounts, first, *stage.Backup, time.Now())
	}
	out := stage
	if use {
		out.Tool, out.Model, out.Effort, out.Provider = stage.Backup.Tool, stage.Backup.Model, stage.Backup.Effort, stage.Backup.Provider
		out.First, out.Balanced = &first, why
	} else {
		out.Tool, out.Model, out.Effort, out.Provider = first.Tool, first.Model, first.Effort, first.Provider
		out.First, out.Balanced = nil, ""
	}
	changed := out.Tool != stage.Tool || out.Model != stage.Model || out.Effort != stage.Effort || out.Provider != stage.Provider || out.Balanced != stage.Balanced || (out.First == nil) != (stage.First == nil)
	return out, changed
}
