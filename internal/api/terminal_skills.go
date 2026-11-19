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

// SessionSkill is one entry of the per-session skill picker. Source is
// "machine" for a skill reet knows on the session's machine (an Active skill,
// or one in a project folder) and "new" for one New on this machine, which
// waits for the person to Keep or Archive it.
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
	// AgentID is the session's machine, whose copy Keep takes.
	AgentID string `json:"agentId,omitempty"`
}

// skillInvocation returns the single line of text that asks an agent to
// use a skill, or "" when the name cannot be typed safely.
func skillInvocation(agent, name, description string) string {
	if name == "" || utf8.RuneCountInString(name) > maxSkillInvocationName {
		return ""
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return ""
		}
	}
	desc := skillBlurb(description)
	if desc == "" {
		return "Use the " + name + " skill."
	}
	return "Use the " + name + " skill: " + desc
}

// maxSkillBlurb is the longest description, in runes, added to an invocation.
const maxSkillBlurb = 280

// skillBlurb makes a skill's own description safe to type on one line: runs of
// whitespace become one space, unprintable runes are dropped, and a long one is
// cut at a word with an ellipsis. It ends in punctuation.
func skillBlurb(description string) string {
	var b strings.Builder
	space := false
	for _, r := range description {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsPrint(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	out := b.String()
	if r := []rune(out); len(r) > maxSkillBlurb {
		out = string(r[:maxSkillBlurb])
		if i := strings.LastIndexByte(out, ' '); i > maxSkillBlurb/2 {
			out = out[:i]
		}
		return strings.TrimRight(out, " ,;:.") + "…"
	}
	if out != "" && !strings.ContainsAny(out[len(out)-1:], ".!?…") {
		out += "."
	}
	return out
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

// newSkillFolderAgents names the agent each home skills folder belongs to, as
// a machine's skills report names the folder.
var newSkillFolderAgents = map[string]string{
	"~/.agents/skills":          "agents",
	"~/.claude/skills":          "claude",
	"~/.cursor/skills":          "cursor",
	"~/.config/opencode/skills": "opencode",
}

// buildSessionSkills builds the catalogue for a session running tool in dir
// on host: native machine skills, other-agent machine skills, then the skills
// New on this machine. An environment scoped to a project counts only when dir
// is that project or below it; a skill that no counting environment holds is
// not listed at all. MachineSeen still follows every environment on the
// machine, so the freshness line does not depend on where the session runs.
// A new skill is listed when agentID, the session's machine, reported it; it
// is in that machine's home folder, so the agent can use it there already.
func buildSessionSkills(tool, host, dir, agentID string, skills []store.Skill, news []store.NewSkill) SessionSkills {
	out := SessionSkills{Skills: []SessionSkill{}, Host: host, AgentID: agentID}
	isNative := func(agent string) bool { return agent == tool || agent == "agents" }

	installed := map[string]bool{}
	var native, other, found []SessionSkill
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
		installed[strings.ToLower(sk.Name)] = true
		agent := here[0].Agent
		hasNative := false
		for _, e := range here {
			if isNative(e.Agent) {
				agent, hasNative = e.Agent, true
				break
			}
		}
		entry := SessionSkill{
			ID: sk.ID, Name: sk.Name, Description: sk.Description,
			Source: "machine", Agent: agent, Native: hasNative, OnMachine: true,
		}
		if hasNative {
			entry.Invocation = skillInvocation(tool, sk.Name, sk.Description)
			native = append(native, entry)
		} else {
			other = append(other, entry)
		}
	}
	for _, n := range news {
		if installed[strings.ToLower(n.Name)] || agentID == "" {
			continue
		}
		agent, hasNative, here := "", false, false
		for _, c := range n.Copies {
			if c.AgentID != agentID {
				continue
			}
			here = true
			a := newSkillFolderAgents[c.Folder]
			if agent == "" {
				agent = a
			}
			if a != "" && isNative(a) {
				agent, hasNative = a, true
				break
			}
		}
		if !here {
			continue
		}
		entry := SessionSkill{ID: "new:" + n.Name, Name: n.Name, Source: "new", Agent: agent, Native: hasNative, OnMachine: true}
		if hasNative {
			entry.Invocation = skillInvocation(tool, n.Name, "")
		}
		found = append(found, entry)
	}
	for _, group := range [][]SessionSkill{native, other, found} {
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
// reet knows on its machine, then those New on it. Archived skills are off
// every machine and are not offered. It only reads.
func (s *server) terminalSessionSkills(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	session, agentID, host, err := s.sessionMachine(uid, r.PathValue("id"))
	if err != nil {
		terminalError(w, err)
		return
	}
	if host == "" {
		// The session's machine is not among the account's agents: no machine skills.
		writeJSON(w, 200, buildSessionSkills(session.Tool, "", session.Dir, "", nil, nil))
		return
	}
	// The machine inventory is the one source of truth for what is installed.
	// ListFor would also hide a private draft's installed copy (shelf visibility),
	// so the picker would keep reporting it as not on the machine.
	skills, err := s.st.InstalledSkillsFor(uid, host)
	if err != nil {
		terminalError(w, err)
		return
	}
	news, err := s.st.NewOnMachinesFor(uid)
	if err != nil {
		terminalError(w, err)
		return
	}
	writeJSON(w, 200, buildSessionSkills(session.Tool, host, session.Dir, agentID, activeOnly(skills), news))
}

// activeOnly drops the skills whose library item is Archived: they are coming
// off every machine and are not offered.
func activeOnly(skills []store.Skill) []store.Skill {
	out := skills[:0:0]
	for _, sk := range skills {
		if sk.Status != store.SkillArchived {
			out = append(out, sk)
		}
	}
	return out
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
