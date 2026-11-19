package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	// 1,024 skills reet can hold in each of the four managed folders, with as
	// much again for skills found there. The agent never sends more.
	skillReportMaxEntries = 8192
	// skillReportBodyLimit gives each of those entries up to 1 KiB.
	skillReportBodyLimit = skillReportMaxEntries << 10
	// skillPollKickEvery is how often polls may kick the copy worker for an
	// account. Reports kick it every time.
	skillPollKickEvery = 30 * time.Second
	// skillsProtocolMax is the largest protocol an agent may announce; a
	// header past it is no protocol at all.
	skillsProtocolMax = 999
)

// scanHomeAgents are the agents whose home skills folders an inventory scan
// reads (homeDirs in internal/scan): the four reet manages, and Codex's,
// which reet leaves alone.
var scanHomeAgents = map[string]bool{"agents": true, "claude": true, "cursor": true, "codex": true, "opencode": true}

// scanMakesItems says, for each sighting in an inventory scan from machine,
// whether it may make a library item of a name the library does not have. A
// machine on the skills protocol reports the skills in its managed folders
// with its skills report, and a name reet does not know is New on a machine
// there until the person keeps it, so the scan makes nothing of it from any
// of its home folders. Codex's folder is left alone, so a skill only there
// appears nowhere. The same goes for a skill one of its lock files names with
// no folder: it is no skill reet has, and the scan reports it from the
// machine's home. Older agents, project folders, a folder the person scans
// with --dir (agent "other") and names typed into the import keep today's
// import.
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
		case item.Host != "" && item.Project == "" && scanHomeAgents[item.Agent]:
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
	mux.HandleFunc("GET /api/v2/sync/skills", s.syncSkillsState)
	mux.HandleFunc("POST /api/v2/sync/skills/retry", s.requireWriter(s.retrySyncSkills))
}

// syncActivityShown is how many activity lines the Sync page shows.
const syncActivityShown = 50

// syncSkillsState is the Sync page: each machine, what it is doing, and what
// the machines did, newest first. work is what the Skills page's header
// counts, for the page's one sentence; active is how many skills go on every
// machine.
func (s *server) syncSkillsState(w http.ResponseWriter, r *http.Request) {
	uid, now := s.uid(r), time.Now()
	machines, err := s.st.SyncMachinesFor(uid, now)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	work, err := s.st.SkillWorkFor(uid, now)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	desired, err := s.st.DesiredSkillsFor(uid, now)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	activity, err := s.st.SkillActivityFor(uid, syncActivityShown)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"machines": machines, "activity": activity, "work": work, "active": len(desired.Active)})
}

// retrySyncSkills asks a machine to try failed skills again on its next poll.
func (s *server) retrySyncSkills(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AgentID string   `json:"agentId"`
		Names   []string `json:"names"`
	}
	if !decode(w, r, &in) {
		return
	}
	names, err := s.st.RequestSkillRetryFor(s.uid(r), in.AgentID, in.Names, time.Now())
	var refused store.SkillRetryRefused
	switch {
	case errors.Is(err, store.ErrNoSuchMachine):
		writeErr(w, 404, err.Error())
		return
	case errors.As(err, &refused) && len(in.Names) == 0:
		writeErr(w, 400, err.Error())
		return
	case errors.As(err, &refused):
		writeErr(w, 409, err.Error())
		return
	case err != nil:
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "sync-skills-retry", "", map[string]any{"agent": in.AgentID, "names": names})
	writeJSON(w, 200, map[string]any{"names": names})
}

// announcedSkillsProtocol is the skills protocol the request's agent speaks:
// what it announced, or 0 for one that says nothing or what it cannot be. A
// protocol newer than this controller's is kept as it is, so the machine can
// be shown as needing a newer controller.
func announcedSkillsProtocol(r *http.Request) int {
	p, err := strconv.Atoi(r.Header.Get(skillsProtocolHeader))
	if err != nil || p < 0 || p > skillsProtocolMax {
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
// protocol, or a newer one. It is sent its update job, if one is queued, and
// never another kind: its skills follow the desired state, which it is sent
// whenever its version differs from the one the machine has. A paused machine
// does no skill work, so it is sent none, and neither is one newer than this
// controller, whose desired state it may not read as this controller means
// it. Polls also make sure, at most every skillPollKickEvery, that reet is
// getting a copy of any skill it has none of.
func (s *server) pollSkillsAgent(w http.ResponseWriter, r *http.Request, a store.SyncAgent) {
	var in struct {
		SkillsVersion string `json:"skillsVersion"`
	}
	if !decodeAgentJSON(w, r, skillPollBodyLimit, &in, "Invalid or oversized poll.") {
		return
	}
	uid := s.uid(r)
	reply := map[string]any{"agent": a}
	if !a.Paused && a.SkillsProtocol == store.CurrentSkillsProtocol {
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
		// Asked once: a retry handed over is gone, whatever the machine
		// then does with it.
		retry, err := s.st.TakeSkillRetriesFor(uid, a.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if len(retry) > 0 {
			reply["retry"] = retry
		}
		if s.skillPollKicks.due(uid) {
			s.kickSkillCopies(uid)
		}
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
	if a.SkillsProtocol > store.CurrentSkillsProtocol {
		writeErr(w, 409, "reet on this machine is newer than this controller. Update the controller to take its skills report.")
		return
	}
	var rep store.SkillReport
	if !decodeAgentJSON(w, r, skillReportBodyLimit, &rep, "Invalid or oversized skills report.") {
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

// pollKicks spaces the copy worker kicks polls make, per account.
type pollKicks struct {
	mu   sync.Mutex
	now  func() time.Time // tests only
	last map[int64]time.Time
}

// due reports whether a poll for uid may kick the copy worker now, and if so
// counts this kick. The first poll may; later ones once skillPollKickEvery has
// passed since the last kick a poll made.
func (k *pollKicks) due(uid int64) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	at := time.Now()
	if k.now != nil {
		at = k.now()
	}
	if last, ok := k.last[uid]; ok && at.Sub(last) < skillPollKickEvery {
		return false
	}
	if k.last == nil {
		k.last = map[int64]time.Time{}
	}
	k.last[uid] = at
	return true
}
