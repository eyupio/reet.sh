package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// skillsProtocolHeader is how an agent says which skills protocol it speaks.
// At store.CurrentSkillsProtocol it makes its skills folders match the
// desired state on every poll and reports what it did; below it, it gets no
// skill work.
const skillsProtocolHeader = "X-Reet-Skills-Protocol"

const (
	// skillPollBodyLimit bounds a poll's body: the version the machine has.
	skillPollBodyLimit = 4 << 10
	// skillReportMaxEntries bounds one report: one entry for each of the
	// 1,024 skills reet can hold in each of the four managed folders.
	skillReportMaxEntries = 4096
	// skillReportBodyLimit gives each of those entries up to 1 KiB.
	skillReportBodyLimit = skillReportMaxEntries << 10
)

// skillsManagedAgents are the agents whose home skills folders follow reet.
var skillsManagedAgents = map[string]bool{"agents": true, "claude": true, "cursor": true, "opencode": true}

// scanMakesItems says, for each sighting in an inventory scan from machine,
// whether it may make a library item of a name the library does not have. A
// machine on the skills protocol reports the skills in its home skills
// folders with its skills report, and a name reet does not know is New on a
// machine there until the person keeps it, so the scan makes nothing of it.
// The same goes for a skill one of its lock files names with no folder: it
// is no skill reet has, and the scan reports it from the machine's home.
// Older agents, project folders, Codex's folder and names typed into the
// import keep today's import.
func (s *server) scanMakesItems(uid int64, machine string) func(importItem) (bool, error) {
	known := map[string]bool{}
	onProtocol := func(host string) (bool, error) {
		key := strings.ToLower(strings.TrimSpace(host))
		if on, ok := known[key]; ok {
			return on, nil
		}
		on, err := s.st.HostOnSkillsProtocolFor(uid, host)
		if err == nil {
			known[key] = on
		}
		return on, err
	}
	return func(item importItem) (bool, error) {
		host := ""
		switch {
		case item.Host != "" && item.Project == "" && skillsManagedAgents[item.Agent]:
			host = item.Host
		case item.Host == "" && item.Agent == "" && strings.TrimSpace(item.Source) != "":
			host = machine
		}
		if strings.TrimSpace(host) == "" {
			return true, nil
		}
		on, err := onProtocol(host)
		return !on, err
	}
}

func (s *server) registerSkillReconcile(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/sync/skills/report", s.requireWriter(s.reportSkills))
}

// announcedSkillsProtocol is the skills protocol the request's agent speaks:
// what it announced, or 0 for one that says nothing or what it cannot be.
func announcedSkillsProtocol(r *http.Request) int {
	p, err := strconv.Atoi(r.Header.Get(skillsProtocolHeader))
	if err != nil || p < 0 || p > store.CurrentSkillsProtocol {
		return 0
	}
	return p
}

// skillsAgentIdentity is agentIdentity for a poll or a skills report, which
// also records the skills protocol the machine's running build announced: it
// changes when reet on the machine is updated or put back. Only these two
// read it; an agent's other requests do not send it.
func (s *server) skillsAgentIdentity(w http.ResponseWriter, r *http.Request) (store.SyncAgent, bool) {
	a, ok := s.agentIdentity(w, r)
	if !ok {
		return a, false
	}
	if protocol := announcedSkillsProtocol(r); protocol != a.SkillsProtocol {
		if err := s.st.SetAgentSkillsProtocolFor(s.uid(r), a.ID, protocol); err != nil {
			writeErr(w, 500, err.Error())
			return a, false
		}
		a.SetSkillsProtocol(protocol)
	}
	return a, true
}

// pollSkillsAgent answers the poll of an agent on the current skills
// protocol. It is sent its update job, if one is queued, and never another
// kind: its skills follow the desired state, which it is sent whenever its
// version differs from the one the machine has. A paused machine does no
// skill work, so it is sent none. Each poll also makes sure reet is getting a
// copy of any skill it has none of.
func (s *server) pollSkillsAgent(w http.ResponseWriter, r *http.Request, a store.SyncAgent) {
	var in struct {
		SkillsVersion string `json:"skillsVersion"`
	}
	if !decodeStrictJSON(w, r, skillPollBodyLimit, &in, "Invalid or oversized poll.") {
		return
	}
	uid := s.uid(r)
	reply := map[string]any{"agent": a}
	if !a.Paused {
		// Read before a job is claimed, so a failure here leaves no job
		// claimed that the machine was never told about.
		desired, err := s.st.DesiredSkillsFor(uid, time.Now())
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if desired.Version != in.SkillsVersion {
			reply["desired"] = desired
		}
		s.kickSkillCopies(uid)
	}
	s.maybeAutoUpdateAgent(r, a)
	job, err := s.st.ClaimUpdateJob(uid, a)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	reply["job"] = job
	writeJSON(w, 200, reply)
}

// reportSkills takes what a machine did on its last reconcile, one entry per
// skill and folder. The report is the authenticated machine's own: the body
// names no machine. Its folders and backups are the machine's words, kept
// and shown, never used as paths here.
func (s *server) reportSkills(w http.ResponseWriter, r *http.Request) {
	a, ok := s.skillsAgentIdentity(w, r)
	if !ok {
		return
	}
	if a.SkillsProtocol < store.CurrentSkillsProtocol {
		writeErr(w, 400, fmt.Sprintf("Send %s: %d with a skills report.", skillsProtocolHeader, store.CurrentSkillsProtocol))
		return
	}
	var rep store.SkillReport
	if !decodeStrictJSON(w, r, skillReportBodyLimit, &rep, "Invalid or oversized skills report.") {
		return
	}
	if len(rep.Entries) > skillReportMaxEntries {
		writeErr(w, 400, fmt.Sprintf("A skills report holds at most %d entries.", skillReportMaxEntries))
		return
	}
	uid := s.uid(r)
	if err := s.st.RecordSkillReportFor(uid, a.ID, rep, time.Now()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// A report can name a skill only this machine has a copy of; its
	// capture starts now rather than when someone presses Retry.
	s.kickSkillCopies(uid)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
