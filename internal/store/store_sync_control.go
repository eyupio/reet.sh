package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
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

type SyncAgent struct {
	KeepSynced bool   `json:"keepSynced"`
	ID         string `json:"id"`
	Host       string `json:"host"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	LastSeen   int64  `json:"lastSeen"`
	Paused     bool   `json:"paused"`
	AutoUpdate bool   `json:"autoUpdate"`

	// SkillsProtocol is the skills protocol the agent last announced in
	// X-Reet-Skills-Protocol. Below CurrentSkillsProtocol it gets no skill
	// work, and SkillsState says so.
	SkillsProtocol int    `json:"skillsProtocol"`
	SkillsState    string `json:"skillsState,omitempty"`

	// EnrolledVia is how the machine was enrolled, set at enrolment and by
	// nothing else: "code" (an enrolment code), "session" (a signed-in
	// browser), "token" (a personal API token; every machine enrolled before
	// codes existed) or "open" (an instance without accounts). Only a
	// machine enrolled by code or session authenticates by its secret alone.
	EnrolledVia string `json:"enrolledVia"`
	// LegacyToken says the machine was enrolled with a personal API token,
	// so it keeps sending one and stops when that token is revoked. Only
	// enrolling it again with a code clears it.
	LegacyToken bool `json:"legacyToken"`
	// Locked is the machine saying it is locked there (reet agent lock), as
	// it last reported. Only the sync listing fills it in.
	Locked bool `json:"locked"`
	// LockedAt is when the lock went on, in Unix seconds, when the machine
	// said so.
	LockedAt int64 `json:"lockedAt,omitempty"`
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
		{"skills_protocol", "skills_protocol INTEGER NOT NULL DEFAULT 0"},
		{"enrolled_via", "enrolled_via TEXT NOT NULL DEFAULT 'token'"},
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
	return s.EnrollSyncAgentVia(uid, host, version, commit, EnrolledByToken)
}

// How a machine was enrolled (SyncAgent.EnrolledVia).
const (
	EnrolledByCode    = "code"
	EnrolledBySession = "session"
	EnrolledByToken   = "token"
	EnrolledOpen      = "open"
)

// EnrollSyncAgentVia enrols a new machine and records how, once.
func (s *Store) EnrollSyncAgentVia(uid int64, host, version, commit, via string) (SyncAgent, string, error) {
	host = strings.TrimSpace(host)
	if !validMachineName(host) {
		return SyncAgent{}, "", ErrMachineName
	}
	canonical, err := s.ResolveMachineHostFor(uid, host)
	if err != nil {
		return SyncAgent{}, "", err
	}
	a := SyncAgent{ID: syncID(), Host: canonical, Version: cleanAgentBuild(version), Commit: cleanAgentBuild(commit), LastSeen: now(), AutoUpdate: true, EnrolledVia: via, LegacyToken: via == EnrolledByToken}
	secret := syncID()
	_, err = s.db.Exec(`INSERT INTO sync_agents(id,user_id,host,secret_hash,version,source_commit,last_seen,paused,auto_update,enrolled_via) VALUES(?,?,?,?,?,?,?,0,1,?)`, a.ID, uid, a.Host, secretHash(secret), a.Version, a.Commit, a.LastSeen, via)
	if err != nil {
		return a, "", errors.New("machine already enrolled; revoke its existing enrollment before enrolling again")
	}
	return a, secret, nil
}

const agentCols = `id,host,version,source_commit,last_seen,paused,auto_update,keep_synced,skills_protocol,enrolled_via`

func scanAgent(row interface{ Scan(...any) error }) (SyncAgent, error) {
	var a SyncAgent
	var protocol int
	err := row.Scan(&a.ID, &a.Host, &a.Version, &a.Commit, &a.LastSeen, &a.Paused, &a.AutoUpdate, &a.KeepSynced, &protocol, &a.EnrolledVia)
	a.SetSkillsProtocol(protocol)
	a.LegacyToken = a.EnrolledVia == EnrolledByToken
	return a, err
}

// CurrentSkillsProtocol is the skills protocol of agents that make their
// skills folders match the desired state and report what they did.
const CurrentSkillsProtocol = 2

// SkillsNeedUpdate is the SkillsState of an agent below
// CurrentSkillsProtocol: its skills stay as they are until reet on it is
// updated.
const SkillsNeedUpdate = "needs-update"

// SkillsNeedNewerController is the SkillsState of an agent above
// CurrentSkillsProtocol: this controller is older than reet on the machine.
const SkillsNeedNewerController = "needs-newer-controller"

// SetSkillsProtocol sets the agent's skills protocol and the state it gives.
func (a *SyncAgent) SetSkillsProtocol(protocol int) {
	a.SkillsProtocol, a.SkillsState = protocol, ""
	switch {
	case protocol < CurrentSkillsProtocol:
		a.SkillsState = SkillsNeedUpdate
	case protocol > CurrentSkillsProtocol:
		a.SkillsState = SkillsNeedNewerController
	}
}

// SetAgentSkillsProtocolFor records the skills protocol an agent announced in
// X-Reet-Skills-Protocol. An agent below CurrentSkillsProtocol gets no skill
// work, only its updates and scans.
func (s *Store) SetAgentSkillsProtocolFor(uid int64, agentID string, protocol int) error {
	_, err := s.db.Exec(`UPDATE sync_agents SET skills_protocol=? WHERE id=? AND user_id=?`, protocol, agentID, uid)
	return err
}

// HostOnSkillsProtocolFor reports whether the machine named host, or a machine
// it is an alias of (compared without case), has an agent on
// CurrentSkillsProtocol or a newer one, as last recorded.
func (s *Store) HostOnSkillsProtocolFor(uid int64, host string) (bool, error) {
	host, err := s.ResolveMachineHostFor(uid, host)
	if err != nil {
		return false, err
	}
	var on bool
	err = s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sync_agents WHERE user_id=? AND host=? COLLATE NOCASE AND skills_protocol>=?)`, uid, host, CurrentSkillsProtocol).Scan(&on)
	return on, err
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
	a, err := scanAgent(s.read.QueryRow(`SELECT `+agentCols+` FROM sync_agents WHERE user_id=? AND id=? AND secret_hash=?`, uid, id, secretHash(secret)))
	if err != nil {
		return a, errors.New("machine enrollment is missing or revoked")
	}
	version = cleanAgentBuild(version)
	commit = cleanAgentBuild(commit)
	if version == "" {
		version = a.Version
	}
	changed := version != a.Version
	if commit != "" {
		changed = changed || commit != a.Commit
		a.Commit = commit
	}
	// A changed build is written at once; an unchanged one only stamps
	// last-seen now and then.
	if s.stamps.due("agent:"+id, agentStampEvery) || changed {
		_, err = s.db.Exec(`UPDATE sync_agents SET last_seen=?,version=?,source_commit=? WHERE id=?`, now(), version, a.Commit, id)
	}
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
	validID := id != "" && len(id) <= 128
	validSecret := secret != "" && len(secret) <= 256
	validVersion := len(version) <= 64 && !strings.ContainsAny(version, "\x00\r\n")
	validCredentials := validID && validSecret
	if !validCredentials || !validVersion {
		return 0, a, errors.New("machine enrollment is missing or revoked")
	}
	err := s.read.QueryRow(`SELECT a.user_id,a.id,a.host,a.version,a.source_commit,a.last_seen,a.paused,a.auto_update,a.keep_synced,a.enrolled_via FROM sync_agents a JOIN users u ON u.id=a.user_id WHERE a.id=? AND a.secret_hash=? AND u.disabled_at=0`, id, secretHash(secret)).Scan(&uid, &a.ID, &a.Host, &a.Version, &a.Commit, &a.LastSeen, &a.Paused, &a.AutoUpdate, &a.KeepSynced, &a.EnrolledVia)
	if err != nil {
		return 0, a, errors.New("machine enrollment is missing or revoked")
	}
	if version == "" {
		version = a.Version
	}
	if s.stamps.due("agent:"+id, agentStampEvery) || version != a.Version {
		_, err = s.db.Exec(`UPDATE sync_agents SET last_seen=?,version=? WHERE id=? AND user_id=?`, now(), version, id, uid)
	}
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
			// Its terminal sessions go with it (by cascade), so no desk pane
			// may name one, and what it reported for desk panes goes too.
			if err := clearDeskSessionsForAgentTx(tx, uid, id); err != nil {
				return err
			}
			_, err := tx.Exec(`DELETE FROM sync_agents WHERE id=? AND user_id=?`, id, uid)
			return err
		})
	}
	return errors.New("unknown machine action")
}

// AgentSecretHash is the stored hash of an enrolled machine's secret, read so a
// controller can prove it holds the record (see internal/syncproof). It is
// never returned to a caller outside the server. A machine whose account is
// disabled has no usable record.
func (s *Store) AgentSecretHash(id string) (string, bool, error) {
	var h string
	err := s.db.QueryRow(`SELECT a.secret_hash FROM sync_agents a JOIN users u ON u.id=a.user_id WHERE a.id=? AND u.disabled_at=0`, id).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return h, err == nil, err
}
