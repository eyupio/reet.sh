package gaffer

import (
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// The crew: which agent, model and effort does a light, a standard and a
// heavy stage. Gaffer advises how demanding a stage is; the crew is the
// person's answer to what that means, saved once for the account and
// changeable for any one outcome when they approve it.

// Crew returns the account's saved crew, or an empty one.
func (s *Service) Crew(uid int64) (store.GafferCrew, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferCrew{}, err
	}
	return s.store.GafferCrewFor(uid)
}

// SaveCrew replaces the account's crew. A tier may be left unset.
func (s *Service) SaveCrew(uid int64, crew store.GafferCrew) (store.GafferCrew, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferCrew{}, err
	}
	if !validBalanceBelow(crew.BalanceBelow) {
		return store.GafferCrew{}, invalid("crew", "Choose between 5 and 90 percent for when a backup takes over.")
	}
	var err error
	for _, tier := range []*store.GafferCrewTier{&crew.Light, &crew.Standard, &crew.Heavy} {
		if *tier, err = crewTier(*tier); err != nil {
			return store.GafferCrew{}, err
		}
	}
	return s.store.SetGafferCrewFor(uid, crew)
}

func crewTier(in store.GafferCrewTier) (store.GafferCrewTier, error) {
	out := store.GafferCrewTier{Tool: in.Tool, Model: strings.TrimSpace(in.Model), Effort: strings.TrimSpace(in.Effort), Provider: strings.TrimSpace(in.Provider)}
	bad := invalid("crew", "Choose an agent that can run a Gaffer chat for each tier you set, with a model, effort and source it accepts.")
	if out.Tool == "" {
		if out.Model != "" || out.Effort != "" || out.Provider != "" {
			return out, bad
		}
		return out, nil
	}
	if !terminalproto.ReviewedChatTool(out.Tool) || out.Model != "" && !terminalproto.ValidModel(out.Model) || out.Effort != "" && !terminalproto.ValidModel(out.Effort) {
		return out, bad
	}
	if out.Provider != "" && (!terminalproto.ToolTakesProvider(out.Tool) || !terminalproto.ValidProvider(out.Provider)) {
		return out, bad
	}
	backup, err := stageBackup(in.Backup, out.Tool, out.Model, out.Effort, out.Provider)
	if err != nil {
		return out, bad
	}
	out.Backup = backup
	return out, nil
}

// stageBackup accepts a second choice for a stage or tier whose first choice
// is tool, model, effort and provider. One with no agent, or the same as the
// first choice, is no backup at all.
func stageBackup(in *store.GafferStageAgent, tool, model, effort, provider string) (*store.GafferStageAgent, error) {
	if in == nil || in.Tool == "" {
		if in != nil && (strings.TrimSpace(in.Model) != "" || strings.TrimSpace(in.Effort) != "" || strings.TrimSpace(in.Provider) != "") {
			return nil, invalid("sessions", "Choose an agent for a backup, or leave the backup out.")
		}
		return nil, nil
	}
	out := store.GafferStageAgent{Tool: in.Tool, Model: strings.TrimSpace(in.Model), Effort: strings.TrimSpace(in.Effort), Provider: strings.TrimSpace(in.Provider)}
	if !terminalproto.ReviewedChatTool(out.Tool) || out.Model != "" && !terminalproto.ValidModel(out.Model) || out.Effort != "" && !terminalproto.ValidModel(out.Effort) {
		return nil, invalid("sessions", "Choose a backup agent that can run a Gaffer chat, with a model and effort it accepts.")
	}
	if out.Provider != "" && (!terminalproto.ToolTakesProvider(out.Tool) || !terminalproto.ValidProvider(out.Provider)) {
		return nil, invalid("provider", "Choose a billing source for a Cline backup, or leave it to Cline's own.")
	}
	if out.Tool == tool && out.Model == model && out.Effort == effort && out.Provider == provider {
		return nil, nil
	}
	return &out, nil
}
