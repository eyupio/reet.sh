package store

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// skillOfflineAfter is how long a machine may go unseen before work
	// pending on it reads as offline.
	skillOfflineAfter = 5 * time.Minute
	// skillForgetAfter is how long a deleted name stays on the removal list
	// when machines have not all confirmed it gone.
	skillForgetAfter = 30 * 24 * time.Hour
	// skillActivityKept is how many activity rows each account keeps.
	skillActivityKept = 500
)

type SkillReportEntry struct{ Folder, Name, State, Digest, Detail, Backup string }

// SkillReport is what a machine did on its last reconcile. State is one of
// installed, removed, failed, unknown or edited; Blocked is "" or
// "allow-apply".
type SkillReport struct {
	Version, Blocked string
	Entries          []SkillReportEntry
}

// A report an agent cut to the controller's cap ends with a failed entry of
// this name and detail, with no folder.
const (
	SkillReportCutName   = "(not reported)"
	SkillReportCutDetail = "Too many skills to report; some are not shown."
)

// Cut reports whether the agent left entries out of r: a name missing from
// it may still be on the machine.
func (r SkillReport) Cut() bool {
	for _, e := range r.Entries {
		if e.Folder == "" && e.Name == SkillReportCutName && e.State == "failed" {
			return true
		}
	}
	return false
}

// MachineSkillState is one machine's part in a skill's health. State is ok
// (an Active skill installed there), off (an Archived one not there),
// installing, removing, failed, offline or blocked.
type MachineSkillState struct {
	Host    string `json:"host"`
	AgentID string `json:"agentId"`
	State   string `json:"state"`
	Detail  string `json:"detail"`
}

// SkillHealth is one skill's line on the Skills page. State is ok,
// installing, removing, failed, offline, blocked, copying, copy-failed,
// getting, fetch-failed or off. needs-update is a Sync page state only.
type SkillHealth struct {
	State    string              `json:"state"`
	Pending  int                 `json:"pending"`
	Total    int                 `json:"total"`
	Hosts    []string            `json:"hosts"`
	Reason   string              `json:"reason"`
	Machines []MachineSkillState `json:"machines"`
}

type NewSkillCopy struct {
	AgentID string `json:"agentId"`
	Host    string `json:"host"`
	Folder  string `json:"folder"`
	Digest  string `json:"digest"`
	SeenAt  int64  `json:"seenAt"`
}

type NewSkill struct {
	Name          string         `json:"name"`
	Copies        []NewSkillCopy `json:"copies"`
	DigestsDiffer bool           `json:"digestsDiffer"`
}

type SkillActivity struct {
	At     int64  `json:"at"`
	Host   string `json:"host"`
	Name   string `json:"name"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

func (s *Store) migrateSkillReports() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS machine_skills(user_id INTEGER NOT NULL,agent_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE,folder TEXT NOT NULL,name TEXT NOT NULL COLLATE NOCASE,state TEXT NOT NULL,digest TEXT NOT NULL DEFAULT '',detail TEXT NOT NULL DEFAULT '',backup TEXT NOT NULL DEFAULT '',at INTEGER NOT NULL,PRIMARY KEY(user_id,agent_id,folder,name))`,
		// What a report says about the machine as a whole, so a report with no
		// entries still records that it was held back.
		`CREATE TABLE IF NOT EXISTS machine_skill_reports(user_id INTEGER NOT NULL,agent_id TEXT NOT NULL PRIMARY KEY REFERENCES sync_agents(id) ON DELETE CASCADE,version TEXT NOT NULL DEFAULT '',blocked TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS skill_activity(user_id INTEGER NOT NULL,at INTEGER NOT NULL,agent_id TEXT NOT NULL,name TEXT NOT NULL,action TEXT NOT NULL,detail TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS skill_activity_user ON skill_activity(user_id,at)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	// Earlier builds kept a report time nothing read, and a copy of the report's
	// version on every row. The version stays on machine_skill_reports, the
	// time of a sighting on machine_skills.
	for _, drop := range [][2]string{{"machine_skills", "version"}, {"machine_skill_reports", "at"}} {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM pragma_table_info(?) WHERE name=?`, drop[0], drop[1]).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE ` + drop[0] + ` DROP COLUMN ` + drop[1]); err != nil {
			return err
		}
	}
	return nil
}

func cleanReportText(v string, max int) string {
	v = strings.TrimSpace(v)
	if len(v) > max {
		// Cut on a rune boundary so the text stays valid UTF-8.
		cut := max
		for cut > 0 && !utf8.RuneStart(v[cut]) {
			cut--
		}
		v = v[:cut]
	}
	return v
}

type reportedRow struct{ state, digest, detail string }

// RecordSkillReportFor replaces the machine's rows with the report (a row's at
// stays the time it was first seen in its state), keeps the backup of each
// edited copy (SkillBackupsFor), adds an activity line for each change from
// what it reported before, and records the removals the report confirms. A
// deleted name is forgotten once every unpaused machine on the new protocol
// has confirmed it or 30 days have passed; an archived name stays on the list.
func (s *Store) RecordSkillReportFor(uid int64, agentID string, rep SkillReport, now time.Time) error {
	at := now.UnixMilli()
	return s.withTx(func(tx *sql.Tx) error {
		var host string
		if err := tx.QueryRow(`SELECT host FROM sync_agents WHERE id=? AND user_id=?`, agentID, uid).Scan(&host); err != nil {
			return errors.New("unknown machine")
		}
		prev := map[string]reportedRow{}
		prevAt := map[string]int64{} // when each row was first seen in its present state
		rows, err := tx.Query(`SELECT folder,lower(name),state,digest,detail,at FROM machine_skills WHERE user_id=? AND agent_id=?`, uid, agentID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var folder, name string
			var r reportedRow
			var seen int64
			if err = rows.Scan(&folder, &name, &r.state, &r.digest, &r.detail, &seen); err != nil {
				break
			}
			prev[folder+"\x00"+name] = r
			prevAt[folder+"\x00"+name] = seen
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		removals := map[string]string{}
		rrows, err := tx.Query(`SELECT lower(name),reason FROM skill_removals WHERE user_id=?`, uid)
		if err != nil {
			return err
		}
		for rrows.Next() {
			var name, reason string
			if err = rrows.Scan(&name, &reason); err != nil {
				break
			}
			removals[name] = reason
		}
		if err == nil {
			err = rrows.Err()
		}
		rrows.Close()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM machine_skills WHERE user_id=? AND agent_id=?`, uid, agentID); err != nil {
			return err
		}
		blocked := ""
		if rep.Blocked == "allow-apply" {
			blocked = rep.Blocked
		}
		if _, err = tx.Exec(`INSERT INTO machine_skill_reports(user_id,agent_id,version,blocked) VALUES(?,?,?,?) ON CONFLICT(agent_id) DO UPDATE SET version=excluded.version,blocked=excluded.blocked`, uid, agentID, cleanReportText(rep.Version, 128), blocked); err != nil {
			return err
		}
		// Before the entries are folded into one per folder and name, so the
		// backup of every edited copy is kept.
		if err = recordSkillBackupsTx(tx, uid, agentID, rep.Entries, at); err != nil {
			return err
		}
		present := map[string]bool{}
		for _, e := range dedupeReportEntries(rep.Entries) {
			name, folder := cleanReportText(e.Name, 256), cleanReportText(e.Folder, 512)
			switch e.State {
			case "installed", "removed", "failed", "unknown", "edited":
			default:
				continue
			}
			if name == "" {
				continue
			}
			cur := reportedRow{e.State, cleanReportText(e.Digest, 128), cleanReportText(e.Detail, 500)}
			key := name
			lname := strings.ToLower(name)
			// at is the first sighting: a row reported again in the same state
			// keeps the time it was first seen, not the time of this report.
			seen := at
			if old, had := prev[folder+"\x00"+lname]; had && old.state == cur.state {
				seen = prevAt[folder+"\x00"+lname]
			}
			if _, err = tx.Exec(`INSERT INTO machine_skills(user_id,agent_id,folder,name,state,digest,detail,backup,at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id,agent_id,folder,name) DO UPDATE SET state=excluded.state,digest=excluded.digest,detail=excluded.detail,backup=excluded.backup,at=excluded.at`,
				uid, agentID, folder, key, cur.state, cur.digest, cur.detail, cleanReportText(e.Backup, 512), seen); err != nil {
				return err
			}
			if cur.state != "removed" {
				present[lname] = true
			}
			old, had := prev[folder+"\x00"+lname]
			action, detail := skillActivityFor(host, cur, old, had, removals[lname] != "")
			if action == "" {
				continue
			}
			if _, err = tx.Exec(`INSERT INTO skill_activity(user_id,at,agent_id,name,action,detail) VALUES(?,?,?,?,?,?)`, uid, at, agentID, name, action, detail); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`DELETE FROM skill_activity WHERE user_id=? AND rowid NOT IN (SELECT rowid FROM skill_activity WHERE user_id=? ORDER BY at DESC,rowid DESC LIMIT ?)`, uid, uid, skillActivityKept); err != nil {
			return err
		}
		cut := rep.Cut()
		for name := range removals {
			if present[name] {
				// It is back on this machine, so an earlier confirmation is void.
				_, err = tx.Exec(`DELETE FROM skill_removal_confirms WHERE user_id=? AND name=? AND agent_id=?`, uid, name, agentID)
			} else if cut {
				// A name a cut report leaves out may be among what it left out.
				continue
			} else {
				_, err = tx.Exec(`INSERT INTO skill_removal_confirms(user_id,name,agent_id,at) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`, uid, name, agentID, at)
			}
			if err != nil {
				return err
			}
		}
		return forgetDeletedSkillsTx(tx, uid, at)
	})
}

// dedupeReportEntries keeps the last entry for each folder and name compared
// without case: the table has one row for them, so a second would write an
// activity line for a change that is not recorded.
func dedupeReportEntries(entries []SkillReportEntry) []SkillReportEntry {
	seen := map[string]bool{}
	var kept []SkillReportEntry
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		key := cleanReportText(e.Folder, 512) + "\x00" + strings.ToLower(cleanReportText(e.Name, 256))
		if seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, e)
	}
	slices.Reverse(kept)
	return kept
}

// skillActivityFor names what a row's change from the machine's previous row
// means, or "" when nothing changed. onRemovalList says the name is on the
// removal list.
func skillActivityFor(host string, cur, old reportedRow, had, onRemovalList bool) (action, detail string) {
	if had && cur == old {
		return "", ""
	}
	switch cur.state {
	case "installed":
		switch {
		case had && old.state == "removed" && onRemovalList:
			return "removed again", "it came back on " + host
		case had && old.state == "edited" && old.digest == cur.digest:
			// The edited report put reet's copy back and said so; this is
			// the same copy, not a second install.
			return "", ""
		case had && (old.state == "installed" || old.state == "edited"):
			return "updated", cur.digest
		}
		return "installed", cur.digest
	case "edited":
		return "edited", "edited on " + host + "; reet's copy put back"
	case "removed":
		if had && old.state == "removed" {
			return "", ""
		}
		return "removed", ""
	case "failed":
		return "failed", cur.detail
	case "unknown":
		return "found", cur.digest
	}
	return "", ""
}

// forgetDeletedSkillsTx drops the deleted names every counted machine has
// confirmed, and those older than skillForgetAfter. Their confirmations go
// with them (by cascade, and here for clarity).
func forgetDeletedSkillsTx(tx *sql.Tx, uid, at int64) error {
	var counted int
	if err := tx.QueryRow(`SELECT count(*) FROM sync_agents WHERE user_id=? AND paused=0 AND skills_protocol=2`, uid).Scan(&counted); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT r.name,r.created_at,(SELECT count(*) FROM skill_removal_confirms c JOIN sync_agents a ON a.id=c.agent_id WHERE c.user_id=r.user_id AND c.name=r.name AND a.paused=0 AND a.skills_protocol=2) FROM skill_removals r WHERE r.user_id=? AND r.reason='deleted'`, uid)
	if err != nil {
		return err
	}
	var forget []string
	for rows.Next() {
		var name string
		var created int64
		var confirmed int
		if err = rows.Scan(&name, &created, &confirmed); err != nil {
			break
		}
		if at-created >= skillForgetAfter.Milliseconds() || (counted > 0 && confirmed >= counted) {
			forget = append(forget, name)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range forget {
		if _, err = tx.Exec(`DELETE FROM skill_removal_confirms WHERE user_id=? AND name=?`, uid, name); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM skill_removals WHERE user_id=? AND name=? AND reason='deleted'`, uid, name); err != nil {
			return err
		}
	}
	return nil
}

type healthMachine struct {
	id, host string
	lastSeen int64
	blocked  bool
}

// SkillHealthFor reads each skill's health from what machines reported,
// compared with DesiredSkillsFor at now, so a change inside the grace window
// shows no pending work. The key is the library item id.
//
// Everything is read in one transaction, so a report or a status change that
// lands meanwhile cannot show half of itself.
func (s *Store) SkillHealthFor(uid int64, now time.Time) (map[string]SkillHealth, error) {
	health, _, err := s.skillHealthAndWork(uid, now)
	return health, err
}

// SkillWorkFor is what the machines that follow the library are doing at
// now, as the Skills page's header counts it.
func (s *Store) SkillWorkFor(uid int64, now time.Time) (SkillWork, error) {
	_, work, err := s.skillHealthAndWork(uid, now)
	return work, err
}

// SkillWork is what the machines that follow the library are doing, over
// every skill they have to install or take off, deleted ones still on the
// removal list included. Each machine is counted once: in Failed when it
// reported a failure, or cannot write skills, with work left; otherwise in
// Installing, Removing or both, and then once in Updating. A machine below
// the skills protocol gets no skill work at all: NeedsUpdate counts the
// unpaused ones, and NeedsUpdateHost names it when there is just one. Nor
// does one above it, newer than this controller: NeedsNewerController and
// NeedsNewerControllerHost count and name those the same way. A paused
// machine is left alone: Paused counts them, whatever their protocol, and
// PausedHost names the one when there is just one.
type SkillWork struct {
	Failed                   int    `json:"failed"`
	Installing               int    `json:"installing"`
	Removing                 int    `json:"removing"`
	Updating                 int    `json:"updating"`
	NeedsUpdate              int    `json:"needsUpdate"`
	NeedsUpdateHost          string `json:"needsUpdateHost,omitempty"`
	NeedsNewerController     int    `json:"needsNewerController"`
	NeedsNewerControllerHost string `json:"needsNewerControllerHost,omitempty"`
	Paused                   int    `json:"paused"`
	PausedHost               string `json:"pausedHost,omitempty"`
}

// skillHealthAndWork reads each skill's health and the machines' work in
// one transaction.
func (s *Store) skillHealthAndWork(uid int64, now time.Time) (map[string]SkillHealth, SkillWork, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, SkillWork{}, err
	}
	defer tx.Rollback()
	return skillHealth(tx, uid, now)
}

func skillHealth(q privateSkillQuery, uid int64, now time.Time) (map[string]SkillHealth, SkillWork, error) {
	var work SkillWork
	at := now.UnixMilli()
	desired, err := desiredSkills(q, uid, now)
	if err != nil {
		return nil, work, err
	}
	install := map[string]DesiredSkill{}
	for _, d := range desired.Active {
		install[d.Item] = d
	}
	remove := map[string]bool{}
	for _, r := range desired.Remove {
		remove[strings.ToLower(r.Name)] = true
	}
	var machines []healthMachine
	rows, err := q.Query(`SELECT a.id,a.host,a.last_seen,COALESCE(r.blocked,'') FROM sync_agents a LEFT JOIN machine_skill_reports r ON r.agent_id=a.id WHERE a.user_id=? AND a.paused=0 AND a.skills_protocol=2 ORDER BY a.host,a.id`, uid)
	if err != nil {
		return nil, work, err
	}
	for rows.Next() {
		var m healthMachine
		var blocked string
		if err = rows.Scan(&m.id, &m.host, &m.lastSeen, &blocked); err != nil {
			break
		}
		m.blocked = blocked == "allow-apply"
		machines = append(machines, m)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, work, err
	}
	reported := map[string]map[string][]reportedRow{}
	hostsOf := map[string][]string{} // name -> hosts of the counted machines that have it
	rows, err = q.Query(`SELECT a.id,a.host,lower(m.name),m.state,m.digest,m.detail FROM machine_skills m JOIN sync_agents a ON a.id=m.agent_id WHERE m.user_id=? AND a.paused=0 AND a.skills_protocol=2 ORDER BY a.host`, uid)
	if err != nil {
		return nil, work, err
	}
	for rows.Next() {
		var id, host, name string
		var r reportedRow
		if err = rows.Scan(&id, &host, &name, &r.state, &r.digest, &r.detail); err != nil {
			break
		}
		if reported[id] == nil {
			reported[id] = map[string][]reportedRow{}
		}
		reported[id][name] = append(reported[id][name], r)
		if r.state != "removed" && r.state != "failed" && r.state != skillRetrying && !slices.Contains(hostsOf[name], host) {
			hostsOf[name] = append(hostsOf[name], host)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, work, err
	}
	work = machinesWork(machines, reported, desired)
	if work.NeedsUpdate, work.NeedsUpdateHost, err = machinesOffProtocol(q, uid, false); err != nil {
		return nil, work, err
	}
	if work.NeedsNewerController, work.NeedsNewerControllerHost, err = machinesOffProtocol(q, uid, true); err != nil {
		return nil, work, err
	}
	if work.Paused, work.PausedHost, err = countedHosts(q, `SELECT host FROM sync_agents WHERE user_id=? AND paused=1 ORDER BY host,id`, uid); err != nil {
		return nil, work, err
	}
	removalNames := map[string]bool{}
	removals, err := skillRemovalsOf(q, uid)
	if err != nil {
		return nil, work, err
	}
	for _, r := range removals {
		removalNames[strings.ToLower(r.Name)] = true
	}
	items, err := q.Query(`SELECT id,name,status,prev_status,status_at,revision_id,origin,copy_problem FROM library_items WHERE user_id=? AND item_type=? AND deleted_at=0`, uid, LibrarySkill)
	if err != nil {
		return nil, work, err
	}
	defer items.Close()
	out := map[string]SkillHealth{}
	for items.Next() {
		var id, name, status, prev, revision, origin, problem string
		var statusAt int64
		if err = items.Scan(&id, &name, &status, &prev, &statusAt, &revision, &origin, &problem); err != nil {
			return nil, work, err
		}
		lname := strings.ToLower(name)
		if at-statusAt < SkillGrace.Milliseconds() && prev != "" {
			status = prev
		}
		h := SkillHealth{Hosts: []string{}, Machines: []MachineSkillState{}}
		// An item without content is held. An Active one is waiting for a
		// fetch or a copy; an Archived one that is not on the removal list,
		// and so still on the machines that have it, is still being fetched
		// or copied in, or could not be.
		held := revision == "" && (status == SkillActive || !removalNames[lname] && (problem != "" || len(hostsOf[lname]) > 0))
		if held {
			fromMachine := origin == "machine"
			switch {
			case problem != "" && origin != "imported":
				h.State, h.Reason = "copy-failed", problem
			case problem != "":
				h.State, h.Reason = "fetch-failed", problem
			case fromMachine:
				h.State = "copying"
				h.Hosts = append(h.Hosts, hostsOf[lname]...)
			default:
				h.State = "getting"
			}
			h.Total = len(machines)
			out[id] = h
			continue
		}
		h.Total = len(machines)
		if len(machines) == 0 {
			h.State = "off"
			out[id] = h
			continue
		}
		want, installing := install[id]
		removing := !installing && remove[lname]
		rank, reason := 0, ""
		hosts := map[int][]string{}
		for _, m := range machines {
			state := machineSkillState(reported[m.id][lname], want, installing, removing)
			detail := ""
			if state == "failed" {
				for _, r := range reported[m.id][lname] {
					if r.state == "failed" {
						detail = r.detail
						break
					}
				}
			}
			if state != "ok" {
				h.Pending++
				switch {
				case state == "failed":
				case m.blocked:
					state = "blocked"
				case at-m.lastSeen > skillOfflineAfter.Milliseconds():
					state = "offline"
				case removing:
					state = "removing"
				default:
					state = "installing"
				}
				r := map[string]int{"failed": 4, "blocked": 3, "offline": 2, "removing": 1, "installing": 1}[state]
				if r > rank {
					rank = r
				}
				hosts[r] = append(hosts[r], m.host)
				if state == "failed" && reason == "" {
					reason = detail
				}
				if r == rank {
					h.State = state
				}
			}
			if state == "ok" && status == SkillArchived {
				// Nothing left to do for an Archived skill: it is off there.
				state = "off"
			}
			h.Machines = append(h.Machines, MachineSkillState{Host: m.host, AgentID: m.id, State: state, Detail: detail})
		}
		switch {
		case h.Pending == 0 && status == SkillArchived:
			// Off every machine, as an Archived skill should be.
			h.State = "off"
		case h.Pending == 0:
			h.State = "ok"
		default:
			h.Hosts = hosts[rank]
			h.Reason = reason
		}
		out[id] = h
	}
	return out, work, items.Err()
}

// machinesOffProtocol counts the unpaused machines below the skills protocol,
// where reet needs updating, or with newer above it, where this controller
// does; and names the one when there is just one.
func machinesOffProtocol(q privateSkillQuery, uid int64, newer bool) (int, string, error) {
	query := `SELECT host FROM sync_agents WHERE user_id=? AND paused=0 AND skills_protocol<? ORDER BY host,id`
	if newer {
		query = `SELECT host FROM sync_agents WHERE user_id=? AND paused=0 AND skills_protocol>? ORDER BY host,id`
	}
	return countedHosts(q, query, uid, CurrentSkillsProtocol)
}

// countedHosts counts the machines query lists, by host, and names the one
// when there is just one.
func countedHosts(q privateSkillQuery, query string, args ...any) (int, string, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	var hosts []string
	for rows.Next() {
		var host string
		if err = rows.Scan(&host); err != nil {
			return 0, "", err
		}
		hosts = append(hosts, host)
	}
	if err = rows.Err(); err != nil || len(hosts) != 1 {
		return len(hosts), "", err
	}
	return 1, hosts[0], nil
}

// machinesWork counts what each machine has left to do: every Active skill
// with content to install at its revision, every name on the removal list to
// take off.
func machinesWork(machines []healthMachine, reported map[string]map[string][]reportedRow, desired DesiredState) SkillWork {
	var w SkillWork
	for _, m := range machines {
		installing, removing, failed := false, false, false
		for _, d := range desired.Active {
			switch machineSkillState(reported[m.id][strings.ToLower(d.Name)], d, true, false) {
			case "failed":
				failed = true
			case "pending":
				installing = true
			}
		}
		for _, r := range desired.Remove {
			switch machineSkillState(reported[m.id][strings.ToLower(r.Name)], DesiredSkill{}, false, true) {
			case "failed":
				failed = true
			case "pending":
				removing = true
			}
		}
		switch {
		case failed || m.blocked && (installing || removing):
			w.Failed++
		case installing || removing:
			if installing {
				w.Installing++
			}
			if removing {
				w.Removing++
			}
			w.Updating++
		}
	}
	return w
}

// machineSkillState is "ok" when the machine has nothing left to do for the
// skill, "failed" when it reported a failure, and "pending" otherwise.
func machineSkillState(rows []reportedRow, want DesiredSkill, installing, removing bool) string {
	for _, r := range rows {
		if r.state == "failed" && (installing || removing) {
			return "failed"
		}
	}
	switch {
	case installing:
		if len(rows) == 0 {
			return "pending"
		}
		for _, r := range rows {
			switch {
			case r.state == "edited":
			case r.state == "installed" && (want.Digest == "" || r.digest == want.Digest):
			default:
				return "pending"
			}
		}
	case removing:
		for _, r := range rows {
			if r.state != "removed" {
				return "pending"
			}
		}
	}
	return "ok"
}

// newSkillCopyOrder orders a new skill's copies: oldest sighting first, then
// by host, folder and machine. NewOnMachinesFor lists them in it, and Keep
// takes the first.
const newSkillCopyOrder = `m.at,a.host,m.folder,m.agent_id`

// followingAgent keeps machines that follow the library now (a is
// sync_agents): unpaused, on this controller's skills protocol (one
// argument). A paused or off-protocol machine's last report is stale.
const followingAgent = `a.paused=0 AND a.skills_protocol=?`

// NewOnMachinesFor lists the unknown skills machines reported whose names are
// neither in the library nor on the removal list, oldest sighting first
// within a name (then by host and folder). It only reads.
func (s *Store) NewOnMachinesFor(uid int64) ([]NewSkill, error) {
	rows, err := s.db.Query(`SELECT m.name,m.agent_id,a.host,m.folder,m.digest,m.at FROM machine_skills m JOIN sync_agents a ON a.id=m.agent_id
 WHERE m.user_id=? AND m.state='unknown' AND `+followingAgent+`
 AND NOT EXISTS(SELECT 1 FROM library_items i WHERE i.user_id=m.user_id AND i.item_type=? AND i.deleted_at=0 AND lower(i.name)=lower(m.name))
 AND NOT EXISTS(SELECT 1 FROM skill_removals r WHERE r.user_id=m.user_id AND r.name=m.name)
 ORDER BY lower(m.name),`+newSkillCopyOrder, uid, CurrentSkillsProtocol, LibrarySkill)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NewSkill{}
	for rows.Next() {
		var name string
		var c NewSkillCopy
		if err = rows.Scan(&name, &c.AgentID, &c.Host, &c.Folder, &c.Digest, &c.SeenAt); err != nil {
			return nil, err
		}
		if n := len(out); n == 0 || !strings.EqualFold(out[n-1].Name, name) {
			out = append(out, NewSkill{Name: name, Copies: []NewSkillCopy{}})
		}
		last := &out[len(out)-1]
		if len(last.Copies) > 0 && last.Copies[0].Digest != c.Digest {
			last.DigestsDiffer = true
		}
		last.Copies = append(last.Copies, c)
	}
	return out, rows.Err()
}

// LiveLibrarySkillFor reports whether a sighting of a skill called name, or
// of the shelf skill shelfID, is of a live library skill: one not deleted
// whose name is name without case, or the one shelfID's shelf row stands for.
func (s *Store) LiveLibrarySkillFor(uid int64, name, shelfID string) (bool, error) {
	var live bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM library_items WHERE user_id=? AND item_type=? AND deleted_at=0 AND lower(name)=lower(?))
 OR EXISTS(SELECT 1 FROM library_aliases a JOIN library_items i ON i.user_id=a.user_id AND i.id=a.item_id WHERE a.user_id=? AND a.kind='shelf' AND a.reference=? AND i.item_type=? AND i.deleted_at=0)`,
		uid, LibrarySkill, name, uid, shelfID, LibrarySkill).Scan(&live)
	return live, err
}

// SkillActivityFor lists what machines did with skills, newest first.
func (s *Store) SkillActivityFor(uid int64, limit int) ([]SkillActivity, error) {
	if limit <= 0 || limit > skillActivityKept {
		limit = skillActivityKept
	}
	rows, err := s.db.Query(`SELECT v.at,COALESCE(a.host,''),v.name,v.action,v.detail FROM skill_activity v LEFT JOIN sync_agents a ON a.id=v.agent_id WHERE v.user_id=? ORDER BY v.at DESC,v.rowid DESC LIMIT ?`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillActivity{}
	for rows.Next() {
		var a SkillActivity
		if err = rows.Scan(&a.At, &a.Host, &a.Name, &a.Action, &a.Detail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
