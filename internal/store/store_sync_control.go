package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/eyupio/reet.sh/internal/ref"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
)

func syncID() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func secretHash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
func asSync(sk Skill) SyncSkill {
	return SyncSkill{ID: sk.ID, Name: sk.Name, Slug: sk.Slug, Source: sk.Source, SourceType: sk.SourceType, URL: sk.URL}
}

// SyncCandidatesFor only exposes the current account's shelf or an accessible list.
func (s *Store) SyncCandidatesFor(uid int64, role string, listID int64) ([]SyncSkill, error) {
	out := []SyncSkill{}
	if listID != 0 {
		d, err := s.ListDetailFor(uid, role, listID)
		if err != nil {
			return nil, err
		}
		for _, sk := range d.Skills {
			candidate, e := s.privateSyncCandidate(uid, asSync(sk.Skill))
			if e != nil {
				return nil, e
			}
			out = append(out, candidate)
		}
		return out, nil
	}
	skills, _, err := s.ListFor(uid, Filter{Sort: "name"})
	if err != nil {
		return nil, err
	}
	for _, sk := range skills {
		candidate, e := s.privateSyncCandidate(uid, asSync(sk))
		if e != nil {
			return nil, e
		}
		out = append(out, candidate)
	}
	return out, nil
}

type DesiredSelection struct {
	FollowPending bool     `json:"-"` // Preserve whole-shelf intent while private source files are in flight.
	FollowSource  bool     `json:"followSource"`
	Origin        string   `json:"-"`
	TargetID      string   `json:"targetId"`
	SourceID      string   `json:"sourceId"`
	ListID        int64    `json:"listId"`
	SkillIDs      []string `json:"skillIds"`
	Mode          string   `json:"mode"`
	Revision      string   `json:"revision"`
}

func (s *Store) SaveDesiredFor(uid int64, role string, in DesiredSelection) error {
	if role == "viewer" {
		return errors.New("read-only account")
	}
	if !slices.Contains([]string{"additive", "mirror", "bidirectional"}, in.Mode) {
		return errors.New("choose additive, mirror or bidirectional sync")
	}
	var candidates []SyncSkill
	var err error
	name := "Reet.sh selected skills"
	source := in.SourceID
	if source == "hosted" {
		candidates, err = s.SyncHostedFor(uid, in.Origin)
		name = "My published reet.sh skills"
	} else if source == "" || source == "shelf" {
		source = "shelf"
		candidates, err = s.SyncCandidatesFor(uid, role, in.ListID)
		if in.ListID != 0 {
			source = fmt.Sprintf("list:%d", in.ListID)
			name = "Reet.sh list selection"
		}
	} else {
		envs, e := s.SyncEnvironmentsFor(uid)
		if e != nil {
			return e
		}
		found := false
		for _, env := range envs {
			if env.ID == source && env.Complete && env.ScannedAt > time.Now().Add(-14*24*time.Hour).UnixMilli() {
				candidates = env.Skills
				name = env.Name
				found = true
			}
		}
		if !found {
			return ErrSync
		}
	}
	if err != nil {
		return err
	}
	for i := range candidates {
		candidates[i], err = s.privateSyncCandidate(uid, candidates[i])
		if err != nil {
			return err
		}
	}
	if in.FollowSource && source == in.TargetID {
		return errors.New("choose a different source machine")
	}
	if in.FollowSource {
		if source == "hosted" {
			return errors.New("choose individual published skills for syncing")
		}
		in.SkillIDs = nil
		for _, sk := range candidates {
			if sk.Source == "" || sk.SourceType == "local" {
				return errors.New("link all skill sources before syncing the whole source")
			}
			in.SkillIDs = append(in.SkillIDs, sk.ID)
		}
	}
	// An explicit per-skill install may supplement a list or machine source
	// with a skill on this account's shelf. Shared source lists are unchanged.
	if !in.FollowSource {
		seen := map[string]bool{}
		for _, sk := range candidates {
			seen[sk.ID] = true
		}
		add := func(list []SyncSkill) {
			for _, sk := range list {
				if !seen[sk.ID] {
					seen[sk.ID] = true
					candidates = append(candidates, sk)
				}
			}
		}
		if source != "shelf" {
			shelf, e := s.SyncCandidatesFor(uid, role, 0)
			if e != nil {
				return e
			}
			add(shelf)
		}
		// A hosted skill installed from a session's skill picker sits in any
		// saved set, so re-saving the set must accept it. Best effort: without
		// a usable origin nothing is added and the save works as before.
		if source != "hosted" && in.Origin != "" {
			if hosted, e := s.SyncHostedFor(uid, in.Origin); e == nil {
				add(hosted)
			}
		}
	}
	selected := map[string]bool{}
	for _, id := range in.SkillIDs {
		selected[id] = true
	}
	// A local copy and a source-linked copy can share a name. The linked copy is
	// the one that can be installed, so saving deduplicates them automatically.
	// Two different linked sources with the same name remain ambiguous.
	bySlug := map[string][]SyncSkill{}
	order := []string{}
	for _, sk := range candidates {
		if !selected[sk.ID] {
			continue
		}
		if _, ok := bySlug[sk.Slug]; !ok {
			order = append(order, sk.Slug)
		}
		bySlug[sk.Slug] = append(bySlug[sk.Slug], sk)
		delete(selected, sk.ID)
	}
	if len(selected) > 0 {
		return errors.New("selected skills are unavailable; refresh and review again")
	}
	desired := []SyncSkill{}
	for _, slug := range order {
		linked := []SyncSkill{}
		for _, sk := range bySlug[slug] {
			if sk.Source != "" && sk.SourceType != "local" {
				linked = append(linked, sk)
			}
		}
		if len(linked) == 0 {
			return errors.New("link unknown sources before saving; local copies cannot be installed")
		}
		if len(linked) > 1 && (linked[0].Source != linked[1].Source || linked[0].SourceType != linked[1].SourceType) {
			return errors.New("resolve duplicate names before saving; more than one source is selected")
		}
		desired = append(desired, linked[0])
	}
	return s.withTx(func(tx *sql.Tx) error {
		// A pending install owns the set until it ends: a save now would carry
		// its skill past the install's revert.
		if err := refuseWhileInstallPendingTx(tx, uid, in.TargetID); err != nil {
			return err
		}
		var complete bool
		var scanned int64
		var agent, project string
		if err := tx.QueryRow(`SELECT sc.complete,sc.scanned_at,e.agent,e.project FROM environments e JOIN environment_scans sc ON sc.env_id=e.id WHERE e.id=? AND e.user_id=?`, in.TargetID, uid).Scan(&complete, &scanned, &agent, &project); err != nil {
			return ErrSync
		}
		if !complete || scanned < time.Now().Add(-14*24*time.Hour).UnixMilli() || !Syncable(agent) || (project != "" && !strings.HasPrefix(project, "/")) {
			return ErrSync
		}
		var revision string
		err := tx.QueryRow(`SELECT revision FROM sync_targets WHERE user_id=? AND target_id=?`, uid, in.TargetID).Scan(&revision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if revision != in.Revision {
			return errors.New("desired skills changed; refresh and review again")
		}
		actual, err := environmentSkills(tx, uid, in.TargetID)
		if err != nil {
			return err
		}
		data, _ := json.Marshal(desired)
		baseline, _ := json.Marshal(actual)
		_, err = tx.Exec(`INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(user_id,target_id) DO UPDATE SET source_id=excluded.source_id,source_name=excluded.source_name,desired=excluded.desired,revision=excluded.revision,updated_at=excluded.updated_at`, uid, in.TargetID, source, name, string(data), syncID(), now())
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO sync_policies(target_id,follow_source) VALUES(?,?) ON CONFLICT(target_id) DO UPDATE SET follow_source=excluded.follow_source,problem=''`, in.TargetID, in.FollowSource || in.FollowPending); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO sync_options(target_id,mode,baseline) VALUES(?,?,?) ON CONFLICT(target_id) DO UPDATE SET mode=excluded.mode,baseline=excluded.baseline`, in.TargetID, in.Mode, string(baseline))
		return err
	})
}

// ReconcileSkills uses the last acknowledged common set. Absence is a deletion,
// not a reason to resurrect an entry. Conflicting sources require explicit review.
func ReconcileSkills(base, desired, actual []SyncSkill) ([]SyncSkill, []string) {
	index := func(skills []SyncSkill) map[string]SyncSkill {
		m := map[string]SyncSkill{}
		for _, sk := range skills {
			m[sk.Slug] = sk
		}
		return m
	}
	b, d, a := index(base), index(desired), index(actual)
	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range d {
		keys[k] = true
	}
	for k := range a {
		keys[k] = true
	}
	out := []SyncSkill{}
	conflicts := []string{}
	same := func(x, y SyncSkill) bool { return x.ID == y.ID && x.Source == y.Source && x.SourceType == y.SourceType }
	for k := range keys {
		v := d[k]
		if same(d[k], b[k]) {
			v = a[k]
		} else if !same(a[k], b[k]) && !same(a[k], d[k]) {
			conflicts = append(conflicts, k)
			continue
		}
		if v.ID != "" {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	sort.Strings(conflicts)
	return out, conflicts
}

type SyncAgent struct {
	KeepSynced bool   `json:"keepSynced"`
	ID         string `json:"id"`
	Host       string `json:"host"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	LastSeen   int64  `json:"lastSeen"`
	Paused     bool   `json:"paused"`
	AutoUpdate bool   `json:"autoUpdate"`
}
type SyncJob struct {
	ID        string          `json:"id"`
	AgentID   string          `json:"agentId"`
	TargetID  string          `json:"targetId"`
	Kind      string          `json:"kind"`
	Status    string          `json:"status"`
	Revision  string          `json:"revision"`
	Plan      json.RawMessage `json:"plan"`
	Hash      string          `json:"hash"`
	Message   string          `json:"message"`
	Stage     string          `json:"stage,omitempty"`
	Stages    []UpdateStage   `json:"stages,omitempty"`
	CreatedAt int64           `json:"createdAt"`
	UpdatedAt int64           `json:"updatedAt"`
}

func cleanAgentBuild(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 64 || strings.ContainsAny(value, "\x00\r\n") {
		return ""
	}
	return value
}

func (s *Store) migrateSyncAgents() error {
	for _, c := range []struct{ name, ddl string }{
		{"source_commit", "source_commit TEXT NOT NULL DEFAULT ''"},
		{"auto_update", "auto_update INTEGER NOT NULL DEFAULT 1"},
		{"keep_synced", "keep_synced INTEGER NOT NULL DEFAULT 0"},
	} {
		ok, err := hasColumn(s.db, "sync_agents", c.name)
		if err != nil {
			return err
		}
		if ok {
			continue
		}
		if _, err = s.db.Exec(`ALTER TABLE sync_agents ADD COLUMN ` + c.ddl); err != nil {
			return err
		}
	}
	return nil
}

// migrateSyncJobs adds the update timeline to databases made before it. Old
// jobs read as having no stages, which every reader treats as "not an update
// timeline" rather than as an empty progress bar.
func (s *Store) migrateSyncJobs() error {
	for _, c := range []struct{ name, ddl string }{
		{"stage", "stage TEXT NOT NULL DEFAULT ''"},
		{"stages", "stages TEXT NOT NULL DEFAULT '[]'"},
	} {
		ok, err := hasColumn(s.db, "sync_jobs", c.name)
		if err != nil {
			return err
		}
		if ok {
			continue
		}
		if _, err = s.db.Exec(`ALTER TABLE sync_jobs ADD COLUMN ` + c.ddl); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) EnrollSyncAgent(uid int64, host, version string) (SyncAgent, string, error) {
	return s.EnrollSyncAgentBuild(uid, host, version, "")
}

// EnrollSyncAgentBuild also records the source commit so dev builds remain
// distinguishable after the controller restarts.
func (s *Store) EnrollSyncAgentBuild(uid int64, host, version, commit string) (SyncAgent, string, error) {
	host = strings.TrimSpace(host)
	if !validMachineName(host) {
		return SyncAgent{}, "", ErrMachineName
	}
	canonical, err := s.ResolveMachineHostFor(uid, host)
	if err != nil {
		return SyncAgent{}, "", err
	}
	a := SyncAgent{ID: syncID(), Host: canonical, Version: cleanAgentBuild(version), Commit: cleanAgentBuild(commit), LastSeen: now(), AutoUpdate: true}
	secret := syncID()
	_, err = s.db.Exec(`INSERT INTO sync_agents(id,user_id,host,secret_hash,version,source_commit,last_seen,paused,auto_update) VALUES(?,?,?,?,?,?,?,0,1)`, a.ID, uid, a.Host, secretHash(secret), a.Version, a.Commit, a.LastSeen)
	if err != nil {
		return a, "", errors.New("machine already enrolled; revoke its existing enrollment before enrolling again")
	}
	return a, secret, nil
}

const agentCols = `id,host,version,source_commit,last_seen,paused,auto_update,keep_synced`

func scanAgent(row interface{ Scan(...any) error }) (SyncAgent, error) {
	var a SyncAgent
	err := row.Scan(&a.ID, &a.Host, &a.Version, &a.Commit, &a.LastSeen, &a.Paused, &a.AutoUpdate, &a.KeepSynced)
	return a, err
}

func (s *Store) SyncAgentsFor(uid int64) ([]SyncAgent, error) {
	rows, err := s.db.Query(`SELECT `+agentCols+` FROM sync_agents WHERE user_id=? ORDER BY host`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SyncAgent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AuthenticateSyncAgent(uid int64, id, secret, version string) (SyncAgent, error) {
	return s.AuthenticateSyncAgentBuild(uid, id, secret, version, "")
}

// AuthenticateSyncAgentBuild updates the reported build alongside last-seen.
// An empty commit keeps the last known commit, so an older agent cannot erase it.
func (s *Store) AuthenticateSyncAgentBuild(uid int64, id, secret, version, commit string) (SyncAgent, error) {
	a, err := scanAgent(s.db.QueryRow(`SELECT `+agentCols+` FROM sync_agents WHERE user_id=? AND id=? AND secret_hash=?`, uid, id, secretHash(secret)))
	if err != nil {
		return a, errors.New("machine enrollment is missing or revoked")
	}
	version = cleanAgentBuild(version)
	commit = cleanAgentBuild(commit)
	if version == "" {
		version = a.Version
	}
	if commit != "" {
		a.Commit = commit
	}
	_, err = s.db.Exec(`UPDATE sync_agents SET last_seen=?,version=?,source_commit=? WHERE id=?`, now(), version, a.Commit, id)
	a.Version, a.LastSeen = version, now()
	return a, err
}

// AuthenticateRemoteAgent resolves the owning account from enrollment
// credentials alone. It exists only for the private agent listener: browser and
// public HTTP authorization must keep using a session or personal token. A
// revoked enrollment, a disabled account, or an unknown secret all fail the
// same way so callers cannot enumerate enrollments.
func (s *Store) AuthenticateRemoteAgent(id, secret, version string) (int64, SyncAgent, error) {
	var a SyncAgent
	var uid int64
	if id == "" || secret == "" || len(id) > 128 || len(secret) > 256 || len(version) > 64 || strings.ContainsAny(version, "\x00\r\n") {
		return 0, a, errors.New("machine enrollment is missing or revoked")
	}
	err := s.db.QueryRow(`SELECT a.user_id,a.id,a.host,a.version,a.source_commit,a.last_seen,a.paused,a.auto_update,a.keep_synced FROM sync_agents a JOIN users u ON u.id=a.user_id WHERE a.id=? AND a.secret_hash=? AND u.disabled_at=0`, id, secretHash(secret)).Scan(&uid, &a.ID, &a.Host, &a.Version, &a.Commit, &a.LastSeen, &a.Paused, &a.AutoUpdate, &a.KeepSynced)
	if err != nil {
		return 0, a, errors.New("machine enrollment is missing or revoked")
	}
	if version == "" {
		version = a.Version
	}
	_, err = s.db.Exec(`UPDATE sync_agents SET last_seen=?,version=? WHERE id=? AND user_id=?`, now(), version, id, uid)
	return uid, a, err
}

func (s *Store) ControlSyncAgent(uid int64, id, action string) error {
	switch action {
	case "keep-sync-on", "keep-sync-off":
		_, err := s.db.Exec(`UPDATE sync_agents SET keep_synced=? WHERE id=? AND user_id=?`, action == "keep-sync-on", id, uid)
		return err
	case "pause", "resume":
		_, err := s.db.Exec(`UPDATE sync_agents SET paused=? WHERE id=? AND user_id=?`, action == "pause", id, uid)
		return err
	case "auto-update-on", "auto-update-off":
		_, err := s.db.Exec(`UPDATE sync_agents SET auto_update=? WHERE id=? AND user_id=?`, action == "auto-update-on", id, uid)
		return err
	case "revoke":
		// Revoking deletes the machine's jobs; an install waiting among them
		// is cancelled first so its skill leaves the saved set.
		return s.withTx(func(tx *sql.Tx) error {
			if err := endSyncJobsTx(tx, "cancelled", "Machine enrollment revoked.", `j.agent_id=? AND j.user_id=? AND j.status IN ('queued','running','awaiting-approval')`, id, uid); err != nil {
				return err
			}
			_, err := tx.Exec(`DELETE FROM sync_agents WHERE id=? AND user_id=?`, id, uid)
			return err
		})
	}
	return errors.New("unknown machine action")
}

func scanJob(row interface{ Scan(...any) error }) (SyncJob, error) {
	var j SyncJob
	var plan, stages string
	err := row.Scan(&j.ID, &j.AgentID, &j.TargetID, &j.Kind, &j.Status, &j.Revision, &plan, &j.Hash, &j.Message, &j.Stage, &stages, &j.CreatedAt, &j.UpdatedAt)
	j.Plan = json.RawMessage(plan)
	j.Stages = decodeStages(stages)
	return j, err
}

const jobCols = `id,agent_id,target_id,kind,status,revision,plan,hash,message,stage,stages,created_at,updated_at`

func (s *Store) SyncJobsFor(uid int64) ([]SyncJob, error) {
	if err := s.withTx(func(tx *sql.Tx) error { return reapStaleSyncJobsTx(tx, `j.user_id=?`, uid) }); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`SELECT `+jobCols+` FROM sync_jobs WHERE user_id=? ORDER BY created_at DESC LIMIT 100`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SyncJob{}
	for rows.Next() {
		j, e := scanJob(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func (s *Store) QueueSyncJob(uid int64, agentID, targetID, kind string) (SyncJob, error) {
	if kind != "scan" && kind != "preview" && kind != "auto" {
		return SyncJob{}, errors.New("queue a scan, preview or automatic sync")
	}
	var host string
	if err := s.db.QueryRow(`SELECT host FROM sync_agents WHERE id=? AND user_id=?`, agentID, uid).Scan(&host); err != nil {
		return SyncJob{}, ErrNotFound
	}
	revision := ""
	if targetID != "" {
		var targetHost string
		if err := s.db.QueryRow(`SELECT e.host,t.revision FROM environments e JOIN sync_targets t ON t.target_id=e.id WHERE e.id=? AND t.user_id=?`, targetID, uid).Scan(&targetHost, &revision); err != nil {
			return SyncJob{}, ErrSync
		}
		if !strings.EqualFold(targetHost, host) {
			return SyncJob{}, errors.New("target belongs to another enrolled machine")
		}
	} else if kind != "scan" {
		return SyncJob{}, ErrSync
	}
	j := SyncJob{ID: syncID(), AgentID: agentID, TargetID: targetID, Kind: kind, Status: "queued", Revision: revision, Plan: json.RawMessage(`{}`), CreatedAt: now(), UpdatedAt: now()}
	// Distinct scopes may queue read-only previews together. Scans and applies
	// remain exclusive, duplicate previews are rejected, and ClaimSyncJob
	// serialises execution on each machine. Approval still requires an exact hash.
	err := s.withTx(func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE agent_id=? AND status IN ('queued','running','awaiting-approval') AND (?='scan' OR kind!='preview' OR target_id=?)`, agentID, kind, targetID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return errors.New("finish or cancel this machine's current job first")
		}
		_, err := tx.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,target_id,kind,status,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, j.ID, uid, agentID, targetID, kind, j.Status, revision, j.CreatedAt, j.UpdatedAt)
		return err
	})
	return j, err
}
func (s *Store) ControlSyncJob(uid int64, id, action, hash string) error {
	return s.withTx(func(tx *sql.Tx) error {
		j, err := scanJob(tx.QueryRow(`SELECT `+jobCols+` FROM sync_jobs WHERE id=? AND user_id=?`, id, uid))
		if err != nil {
			return ErrNotFound
		}
		if action == "cancel" {
			if j.Status == "running" {
				return errors.New("operation already running; pause the machine to prevent further jobs, then inspect its result")
			}
			if j.Status != "queued" && j.Status != "awaiting-approval" {
				return errors.New("job already finished")
			}
			if _, err = tx.Exec(`UPDATE sync_jobs SET status='cancelled',updated_at=? WHERE id=?`, now(), id); err != nil {
				return err
			}
			_, err = revertInstallTx(tx, id)
			return err
		}
		if action != "approve" || j.Status != "awaiting-approval" || hash == "" || hash != j.Hash {
			return errors.New("plan changed or cannot be approved")
		}
		var revision string
		if err = tx.QueryRow(`SELECT revision FROM sync_targets WHERE user_id=? AND target_id=?`, uid, j.TargetID).Scan(&revision); err != nil || revision != j.Revision {
			return errors.New("desired skills changed; cancel and preview again")
		}
		_, err = tx.Exec(`UPDATE sync_jobs SET kind='apply',status='queued',updated_at=? WHERE id=?`, now(), id)
		return err
	})
}
func (s *Store) ClaimSyncJob(uid int64, a SyncAgent) (*SyncJob, error) {
	if a.Paused {
		return nil, nil
	}
	var job *SyncJob
	err := s.withTx(func(tx *sql.Tx) error {
		// Expired work is never replayed automatically: it may have partially applied.
		err := reapStaleSyncJobsTx(tx, `j.agent_id=?`, a.ID)
		if err != nil {
			return err
		}
		var running int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE agent_id=? AND status='running'`, a.ID).Scan(&running); err != nil || running > 0 {
			return err
		}
		j, err := scanJob(tx.QueryRow(`SELECT `+jobCols+` FROM sync_jobs WHERE user_id=? AND agent_id=? AND status='queued' ORDER BY created_at LIMIT 1`, uid, a.ID))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if j.TargetID != "" {
			var revision, host string
			err = tx.QueryRow(`SELECT t.revision,e.host FROM sync_targets t JOIN environments e ON e.id=t.target_id WHERE t.user_id=? AND t.target_id=?`, uid, j.TargetID).Scan(&revision, &host)
			if err != nil || revision != j.Revision || !strings.EqualFold(host, a.Host) {
				return endSyncJobsTx(tx, "failed", "Target changed; preview again.", `j.id=?`, j.ID)
			}
		}
		_, err = tx.Exec(`UPDATE sync_jobs SET status='running',updated_at=? WHERE id=?`, now(), j.ID)
		j.Status = "running"
		if j.Kind == "update" {
			// The agent is collecting the job now; the download is the first
			// step it can be seen working on.
			stages := j.Stages
			if len(stages) == 0 {
				stages = NewUpdateStages()
			}
			advanceStages(stages, UpdateStageDownload, "The machine is downloading the release.")
			j.Stage, j.Stages = UpdateStageDownload, stages
			if _, err = tx.Exec(`UPDATE sync_jobs SET stage=?,stages=? WHERE id=?`, j.Stage, encodeStages(stages), j.ID); err != nil {
				return err
			}
		}
		job = &j
		return err
	})
	return job, err
}

// SyncJobResultRefused marks a result the controller will never accept for
// this job. The agent may discard its durable copy; any other FinishSyncJob
// error is transient and should be retried with the same result.
type SyncJobResultRefused struct{ Reason string }

func (e SyncJobResultRefused) Error() string { return e.Reason }

func refuseSyncJobResult(reason string) error { return SyncJobResultRefused{Reason: reason} }

func (s *Store) FinishSyncJob(uid int64, agentID, id, status, hash, message string, plan json.RawMessage) error {
	if len(message) > 8000 {
		message = message[:8000]
	}
	if len(plan) == 0 {
		plan = json.RawMessage(`{}`)
	}
	if !json.Valid(plan) || len(plan) > 1<<20 {
		return refuseSyncJobResult("invalid plan")
	}
	return s.withTx(func(tx *sql.Tx) error {
		j, err := scanJob(tx.QueryRow(`SELECT `+jobCols+` FROM sync_jobs WHERE user_id=? AND agent_id=? AND id=?`, uid, agentID, id))
		if errors.Is(err, sql.ErrNoRows) {
			return refuseSyncJobResult("job not found")
		}
		if err != nil {
			return err
		}
		var received int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_receipts WHERE job_id=? AND status=? AND hash=?`, id, status, hash).Scan(&received); err != nil {
			return err
		}
		if received > 0 {
			return nil
		}
		if j.Status != "running" {
			// An update the controller gave up on can still report the truth:
			// the machine did restart, or it rolled itself back. Taking that
			// late account is better than showing a failure that is not one.
			if j.Status == "failed" && j.Kind == "update" && strings.HasPrefix(j.Message, "The machine did not report back") && (status == "succeeded" || status == "rolled_back") {
				j.Status = "running"
			} else if j.Status == "failed" && (strings.HasPrefix(j.Message, "Agent disconnected or timed out") || strings.HasPrefix(j.Message, "The machine did not report back")) {
				// Preserve the unknown/partial outcome; a fresh scan and preview are required.
				_, err = tx.Exec(`INSERT OR IGNORE INTO sync_receipts(job_id,status,hash) VALUES(?,?,?)`, id, status, hash)
				return err
			} else {
				return refuseSyncJobResult("job is no longer running")
			}
		}
		if status != "failed" && status != "succeeded" && !(j.Kind == "update" && status == "rolled_back") && !((j.Kind == "preview" || j.Kind == "auto") && status == "awaiting-approval" && len(hash) == 64) {
			return refuseSyncJobResult("invalid job result")
		}
		if status == "succeeded" && j.Kind == "apply" {
			var revision string
			if err = tx.QueryRow(`SELECT revision FROM sync_targets WHERE user_id=? AND target_id=?`, uid, j.TargetID).Scan(&revision); err != nil || revision != j.Revision {
				status = "failed"
				message = "Desired revision changed during apply; rescan and preview."
			}
			if status == "succeeded" {
				_, err = tx.Exec(`UPDATE sync_options SET baseline=(SELECT desired FROM sync_targets WHERE target_id=? AND user_id=?),applied_revision=? WHERE target_id=?`, j.TargetID, uid, j.Revision, j.TargetID)
				if err != nil {
					return err
				}
			}
		}
		originalStatus := status
		if message == "Desired revision changed during apply; rescan and preview." {
			originalStatus = "succeeded"
		}
		if _, err = tx.Exec(`INSERT INTO sync_receipts(job_id,status,hash) VALUES(?,?,?)`, id, originalStatus, hash); err != nil {
			return err
		}
		finalKind, finalStatus := j.Kind, status
		if j.Kind == "auto" && status == "awaiting-approval" {
			if additivePlan(plan) {
				finalKind = "apply"
				if len(planActions(plan)) == 0 {
					finalStatus = "succeeded"
					message = "Already in sync."
				} else {
					finalStatus = "queued"
					message = "Additive plan approved automatically; applying."
				}
			} else {
				message = "Removals need review before applying."
			}
		}
		stage := j.Stage
		stages := j.Stages
		if len(stages) > 0 && finalStatus != "awaiting-approval" && finalStatus != "queued" {
			stage, stages = finishStages(stages, finalStatus, message)
		}
		if _, err = tx.Exec(`UPDATE sync_jobs SET kind=?,status=?,plan=?,hash=?,message=?,stage=?,stages=?,updated_at=? WHERE id=?`, finalKind, finalStatus, string(plan), hash, message, stage, encodeStages(stages), now(), id); err != nil {
			return err
		}
		// Any end other than a successful apply leaves an install unapplied.
		if finalStatus != "queued" && finalStatus != "awaiting-approval" && (finalStatus != "succeeded" || finalKind != "apply") {
			if _, err = revertInstallTx(tx, id); err != nil {
				return err
			}
		}
		outcome := runNext
		switch {
		case finalStatus == "failed":
			outcome = runFailed
		case finalStatus == "cancelled":
			outcome = runCancelled
		case finalKind == "auto" && finalStatus == "awaiting-approval":
			outcome = runReview
		case finalStatus == "succeeded" && (j.Kind == "auto" || j.Kind == "apply"):
			outcome = runNext
		default:
			return nil
		}
		return s.finishSyncRun(tx, uid, id, outcome)
	})
}

// Adopt adds selected observed skills without changing personal workflow flags.
func (s *Store) AdoptSyncSkillsFor(uid int64, role, targetID string, ids []string, listID int64) error {
	if role == "viewer" {
		return errors.New("read-only account")
	}
	if listID != 0 {
		access, err := s.ListAccessFor(uid, role, listID)
		if err != nil {
			return err
		}
		if !access.CanEdit {
			return errors.New("list is read-only")
		}
	}
	envs, err := s.SyncEnvironmentsFor(uid)
	if err != nil {
		return err
	}
	available := map[string]SyncSkill{}
	for _, env := range envs {
		if env.ID == targetID && env.Complete && env.ScannedAt > time.Now().Add(-14*24*time.Hour).UnixMilli() {
			for _, sk := range env.Skills {
				available[sk.ID] = sk
			}
		}
	}
	for _, id := range ids {
		if _, ok := available[id]; !ok {
			return errors.New("skill is not in a complete recent report")
		}
	}
	return s.withTx(func(tx *sql.Tx) error {
		for _, id := range ids {
			_, err := tx.Exec(`INSERT INTO shelf(user_id,skill_id,liked,bookmarked,used,notes,created_at,updated_at) VALUES(?,?,0,0,0,'',?,?) ON CONFLICT(user_id,skill_id) DO NOTHING`, uid, id, now(), now())
			if err != nil {
				return err
			}
			if listID != 0 {
				_, err = tx.Exec(`INSERT INTO list_skills(list_id,skill_id,note,added_by,added_at) VALUES(?,?,'',?,?) ON CONFLICT(list_id,skill_id) DO NOTHING`, listID, id, uid, now())
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *Store) SyncHostedFor(uid int64, origin string) ([]SyncSkill, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return nil, errors.New("controller origin unavailable")
	}
	username, err := s.PublishingUsername(uid)
	if err != nil {
		return nil, err
	}
	skills, err := s.HostedSkillsFor(uid)
	if err != nil {
		return nil, err
	}
	out := []SyncSkill{}
	for _, sk := range skills {
		if !sk.Published {
			continue
		}
		rf, err := ref.ParseAt(strings.TrimRight(origin, "/")+"/"+username+"/"+sk.Slug, u.Host)
		if err != nil {
			return nil, err
		}
		out = append(out, SyncSkill{ID: rf.ID, Name: sk.Slug, Slug: sk.Slug, Source: rf.Source, SourceType: rf.SourceType, URL: rf.URL})
	}
	return out, nil
}
