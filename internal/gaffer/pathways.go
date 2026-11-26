package gaffer

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A pathway is a person's saved, ordered line of stages. This file decides what
// a sound line is and manages the account's pathways.

const (
	CustomStage           = "custom"
	MaxPathwayStages      = 12
	MaxPathwayNameRunes   = 60
	MaxStageNameRunes     = 40
	MaxPathwayDescBytes   = 300
	MaxStagePromptBytes   = 4000
	MaxStageSkills        = 2
	MaxStageGuidanceBytes = 6000
)

// validPathwayOrder reports the first broken order rule for stage labels in
// order, or "" when the order is sound. Custom stages sit anywhere.
func validPathwayOrder(kinds []string) string {
	seen := map[string]bool{}
	for i, k := range kinds {
		if k == CustomStage {
			continue
		}
		if seen[k] {
			return "A stage can appear only once: " + k + " is there twice."
		}
		seen[k] = true
		if (k == "babysit" || k == "merge") && !slices.Contains(kinds[:i], "pr") {
			return "A " + k + " stage needs a pr stage before it."
		}
		if k == "merge" && i != len(kinds)-1 {
			return "The merge stage has to come last."
		}
	}
	return ""
}

// cleanPathway trims and checks p, returning it ready to save.
func cleanPathway(p store.GafferPathway) (store.GafferPathway, error) {
	p.Name, p.Description = strings.TrimSpace(p.Name), strings.TrimSpace(p.Description)
	if !terminalproto.ValidTitle(p.Name) || utf8.RuneCountInString(p.Name) > MaxPathwayNameRunes {
		return p, invalid("name", "Give the pathway a name of up to 60 plain characters.")
	}
	if p.Description != "" && (len(p.Description) > MaxPathwayDescBytes || !store.ValidWorkerText(p.Description)) {
		return p, invalid("description", "Keep the description to 300 bytes of plain text.")
	}
	if len(p.Stages) == 0 || len(p.Stages) > MaxPathwayStages {
		return p, invalid("stages", "A pathway needs 1 to 12 stages.")
	}
	stages := make([]store.GafferPathwayStage, len(p.Stages))
	kinds := make([]string, len(stages))
	var names []string
	for i, st := range p.Stages {
		st.Name, st.Prompt = strings.TrimSpace(st.Name), strings.TrimSpace(st.Prompt)
		kinds[i] = st.Kind
		if st.Kind == CustomStage {
			if !terminalproto.ValidTitle(st.Name) || utf8.RuneCountInString(st.Name) > MaxStageNameRunes {
				return p, invalid("stages", "Give each custom stage a name of up to 40 plain characters.")
			}
			if st.Name == "" || strings.EqualFold(st.Name, CustomStage) || slices.ContainsFunc(StageKinds, func(k string) bool { return strings.EqualFold(k, st.Name) }) {
				return p, invalid("stages", "A custom stage cannot be named after a built-in stage: "+st.Name+".")
			}
			if slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, st.Name) }) {
				return p, invalid("stages", "Two custom stages are named "+st.Name+". Give each its own name.")
			}
			names = append(names, st.Name)
			if st.Prompt == "" {
				return p, invalid("stages", "The custom stage "+st.Name+" needs a prompt.")
			}
		} else if !slices.Contains(StageKinds, st.Kind) {
			return p, invalid("stages", "Stage "+st.Kind+" is not one reet knows.")
		} else if st.Name != "" {
			return p, invalid("stages", "Only a custom stage has its own name.")
		}
		if len(st.Prompt) > MaxStagePromptBytes || st.Prompt != "" && !terminalproto.ValidPrompt(st.Prompt) {
			return p, invalid("stages", "Keep each stage's prompt to 4,000 bytes of plain text.")
		}
		if st.Agent != nil && st.Tier != "" {
			return p, invalid("stages", "Give a stage a tier or an agent, not both.")
		}
		if st.Agent != nil {
			a := *st.Agent
			a.Model, a.Effort, a.Provider = strings.TrimSpace(a.Model), strings.TrimSpace(a.Effort), strings.TrimSpace(a.Provider)
			if !terminalproto.ReviewedChatTool(a.Tool) || a.Model != "" && !terminalproto.ValidModel(a.Model) || a.Effort != "" && !terminalproto.ValidModel(a.Effort) {
				return p, invalid("stages", "Choose an agent, model and effort reet offers for each stage.")
			}
			if a.Provider != "" && (!terminalproto.ToolTakesProvider(a.Tool) || !terminalproto.ValidProvider(a.Provider)) {
				return p, invalid("stages", "That agent does not take a provider.")
			}
			st.Agent = &a
		} else if st.Tier == "" {
			st.Tier = DefaultStageTier(st.Kind)
		} else if !validStageTier(st.Tier) {
			return p, invalid("stages", "A stage's tier is light, standard or heavy.")
		}
		if len(st.Skills) > MaxStageSkills {
			return p, invalid("stages", "A stage can use up to 2 skills.")
		}
		skills := make([]string, len(st.Skills))
		for j, id := range st.Skills {
			if skills[j] = strings.TrimSpace(id); skills[j] == "" || slices.Contains(skills[:j], skills[j]) {
				return p, invalid("stages", "A stage's skills must each be named once.")
			}
		}
		if len(skills) > 0 {
			st.Skills = skills
		}
		stages[i] = st
	}
	if msg := validPathwayOrder(kinds); msg != "" {
		return p, invalid("stages", msg)
	}
	p.Stages = stages
	return p, nil
}

func (s *Service) Pathways(uid int64) ([]store.GafferPathway, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return nil, err
	}
	return s.store.GafferPathwaysFor(uid)
}

func (s *Service) Pathway(uid int64, id string) (store.GafferPathway, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferPathway{}, err
	}
	return s.store.GafferPathwayFor(uid, id)
}

// SavePathway saves p after checking it. An empty ID makes a new pathway; an
// existing one is replaced only if it is still as it was at readAt.
func (s *Service) SavePathway(uid int64, p store.GafferPathway, readAt int64) (store.GafferPathway, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferPathway{}, err
	}
	p, err := cleanPathway(p)
	if err != nil {
		return p, err
	}
	if p.ID == "" {
		if p.ID, err = newChatID(); err != nil {
			return p, err
		}
	}
	return s.store.SaveGafferPathwayFor(uid, p, readAt)
}

func (s *Service) DeletePathway(uid int64, id string) error {
	if _, err := s.chatPrincipal(uid); err != nil {
		return err
	}
	return s.store.DeleteGafferPathwayFor(uid, id)
}

// pathwayStages turns p into stage inputs with each stage's skills loaded as
// they are approved now. Every reason it cannot is returned, one sentence each.
func pathwayStages(p store.GafferPathway, load SkillLoader) ([]OutcomeStageInput, []string) {
	var problems []string
	out := make([]OutcomeStageInput, len(p.Stages))
	for i, st := range p.Stages {
		in := OutcomeStageInput{Kind: st.Kind, Name: st.Name, Prompt: st.Prompt}
		if a := st.Agent; a != nil {
			in.Tool, in.Model, in.Effort, in.Provider = a.Tool, a.Model, a.Effort, a.Provider
		} else {
			in.Tier = st.Tier
		}
		label := st.Kind
		if st.Name != "" {
			label = st.Name
		}
		size := 0
		for _, id := range st.Skills {
			sk, err := load(id)
			if err != nil {
				problems = append(problems, fmt.Sprintf("The %s pathway cannot use the skill %s in its %s stage: %s", p.Name, id, label, err.Error()))
				continue
			}
			sk.State = "selected"
			size += len(skillbuddy.Prompt(sk))
			in.Skills = append(in.Skills, sk)
			if size > MaxStageGuidanceBytes {
				problems = append(problems, fmt.Sprintf("The %s pathway's %s stage has more guidance than fits in 6,000 bytes, with the skill %s. Use shorter skills.", p.Name, label, id))
				break
			}
		}
		out[i] = in
	}
	return out, problems
}

// PathwayProblems lists what would refuse a start on p, for the "!" mark in
// the pathway list: one sentence each, none when it is sound.
func (s *Service) PathwayProblems(p store.GafferPathway, load SkillLoader) []string {
	_, problems := pathwayStages(p, load)
	return problems
}

// UsePathway gives every session the pathway's stages as it is saved now,
// with each stage's skills loaded. A session that already has stages, a
// missing or disabled pathway and a stage whose skills cannot be had or do not
// fit are refused, and nothing is changed.
func (s *Service) UsePathway(uid int64, id string, sessions []OutcomeSession, load SkillLoader) error {
	if _, err := s.chatPrincipal(uid); err != nil {
		return err
	}
	for _, item := range sessions {
		if len(item.Stages) > 0 {
			return invalid("pathway", "A pipeline takes a pathway or its own stages, not both.")
		}
	}
	p, err := s.store.GafferPathwayFor(uid, id)
	if errors.Is(err, store.ErrNotFound) {
		return invalid("pathway", "That pathway is not available. Choose another.")
	}
	if err != nil {
		return err
	}
	if !p.Enabled {
		return invalid("pathway", "The "+p.Name+" pathway is switched off. Switch it on to use it.")
	}
	stages, problems := pathwayStages(p, load)
	if len(problems) > 0 {
		return invalid("pathway", problems[0])
	}
	for i := range sessions {
		sessions[i].Stages = slices.Clone(stages)
		sessions[i].Pathway = &store.GafferPathwayRef{ID: p.ID, Name: p.Name}
	}
	return nil
}
