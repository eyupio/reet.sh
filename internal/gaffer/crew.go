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
	return out, nil
}
