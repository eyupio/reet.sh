package api

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/store"
)

// maxSkillInvocationName is the longest skill name, in runes, that is
// turned into text typed into a terminal.
const maxSkillInvocationName = 80

// SessionSkill is one entry of the per-session skill picker.
type SessionSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Agent       string `json:"agent"`
	Native      bool   `json:"native"`
	OnMachine   bool   `json:"onMachine"`
	Invocation  string `json:"invocation"`
}

// SessionSkills is the catalogue offered for one session.
type SessionSkills struct {
	Skills      []SessionSkill `json:"skills"`
	MachineSeen int64          `json:"machineSeen"`
	Host        string         `json:"host"`
}

// skillInvocation returns the single line of text that asks an agent to
// use a skill, or "" when the name cannot be typed safely.
func skillInvocation(agent, name string) string {
	if name == "" || utf8.RuneCountInString(name) > maxSkillInvocationName {
		return ""
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return ""
		}
	}
	return "Use the " + name + " skill."
}

// buildSessionSkills builds the catalogue for a session running tool on
// host: native machine skills, other-agent machine skills, then reet skills.
func buildSessionSkills(tool, host string, skills []store.Skill, hosted []store.HostedSkill) SessionSkills {
	out := SessionSkills{Skills: []SessionSkill{}, Host: host}
	isNative := func(e store.Environment) bool { return e.Agent == tool || e.Agent == "agents" }

	installed := map[string]bool{}
	var native, other, reet []SessionSkill
	for _, sk := range skills {
		var here []store.Environment
		for _, e := range sk.Environments {
			if strings.EqualFold(e.Host, host) {
				here = append(here, e)
				if e.LastSeen > out.MachineSeen {
					out.MachineSeen = e.LastSeen
				}
			}
		}
		if len(here) == 0 {
			continue
		}
		installed[sk.Slug] = true
		agent := here[0].Agent
		hasNative := false
		for _, e := range here {
			if isNative(e) {
				agent, hasNative = e.Agent, true
				break
			}
		}
		entry := SessionSkill{
			ID: sk.ID, Name: sk.Name, Description: sk.Description,
			Source: "machine", Agent: agent, Native: hasNative, OnMachine: true,
		}
		if hasNative {
			entry.Invocation = skillInvocation(tool, sk.Name)
			native = append(native, entry)
		} else {
			other = append(other, entry)
		}
	}
	for _, h := range hosted {
		if installed[h.Slug] {
			continue
		}
		reet = append(reet, SessionSkill{
			ID: "reet:" + h.Slug, Name: h.Slug, Description: h.Description, Source: "reet",
		})
	}
	for _, group := range [][]SessionSkill{native, other, reet} {
		sort.SliceStable(group, func(i, j int) bool {
			a, b := strings.ToLower(group[i].Name), strings.ToLower(group[j].Name)
			if a != b {
				return a < b
			}
			return group[i].ID < group[j].ID
		})
		out.Skills = append(out.Skills, group...)
	}
	return out
}
