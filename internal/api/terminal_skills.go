package api

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
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
	Published   bool   `json:"published"`
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

// inProject reports whether dir is the project folder or inside it. Both are
// absolute paths as the machine reported them; /work/repo does not contain /work/repo2.
func inProject(dir, project string) bool {
	if dir == "" || project == "" {
		return false
	}
	dir, project = filepath.Clean(dir), filepath.Clean(project)
	if dir == project {
		return true
	}
	if !strings.HasSuffix(project, string(filepath.Separator)) {
		project += string(filepath.Separator)
	}
	return strings.HasPrefix(dir, project)
}

// buildSessionSkills builds the catalogue for a session running tool in dir
// on host: native machine skills, other-agent machine skills, then reet skills.
// An environment scoped to a project counts only when dir is that project or
// below it; a skill that no counting environment holds is not listed at all.
// MachineSeen still follows every environment on the machine, so the freshness
// line does not depend on where the session runs.
func buildSessionSkills(tool, host, dir string, skills []store.Skill, hosted []store.HostedSkill) SessionSkills {
	out := SessionSkills{Skills: []SessionSkill{}, Host: host}
	isNative := func(e store.Environment) bool { return e.Agent == tool || e.Agent == "agents" }

	installed := map[string]bool{}
	var native, other, reet []SessionSkill
	for _, sk := range skills {
		var here []store.Environment
		for _, e := range sk.Environments {
			if !strings.EqualFold(e.Host, host) {
				continue
			}
			if e.LastSeen > out.MachineSeen {
				out.MachineSeen = e.LastSeen
			}
			if e.Project == "" || inProject(dir, e.Project) {
				here = append(here, e)
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
			Source: "machine", Agent: agent, Native: hasNative, OnMachine: true, Published: true,
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
			ID: "reet:" + h.Slug, Name: h.Slug, Description: h.Description, Source: "reet", Published: h.Published,
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

// terminalSessionSkills lists the skills a running session can use: those
// installed on its machine, then the account's reet-hosted ones. It only reads.
func (s *server) terminalSessionSkills(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	session, _, host, err := s.sessionMachine(uid, r.PathValue("id"))
	if err != nil {
		terminalError(w, err)
		return
	}
	// The reet-hosted group does not depend on the machine, so it is read first.
	hosted, err := s.st.HostedSkillsFor(uid)
	if err != nil {
		terminalError(w, err)
		return
	}
	if host == "" {
		// The session's machine is not among the account's agents: no machine skills.
		writeJSON(w, 200, buildSessionSkills(session.Tool, "", session.Dir, nil, hosted))
		return
	}
	skills, _, err := s.st.ListFor(uid, store.Filter{})
	if err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, 200, buildSessionSkills(session.Tool, host, session.Dir, skills, hosted))
}

// sessionMachine resolves a session the account owns and the machine it runs
// on. host is "" when the session's machine is not among the account's agents;
// otherwise it is the alias-resolved machine name. An unknown or foreign
// session is terminalbroker.ErrDenied, the sibling routes' answer.
func (s *server) sessionMachine(uid int64, id string) (session store.TerminalSession, agentID, host string, err error) {
	session, err = s.st.TerminalSessionFor(uid, id)
	if err != nil {
		return session, "", "", terminalbroker.ErrDenied
	}
	agents, err := s.st.SyncAgentsFor(uid)
	if err != nil {
		return session, "", "", err
	}
	for _, a := range agents {
		if a.ID == session.AgentID {
			host = a.Host
			break
		}
	}
	if host == "" {
		return session, session.AgentID, "", nil
	}
	if host, err = s.st.ResolveMachineHostFor(uid, host); err != nil {
		return session, "", "", err
	}
	return session, session.AgentID, host, nil
}
