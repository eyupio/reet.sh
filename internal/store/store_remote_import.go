package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
)

var ErrRemoteImport = errors.New("import changed, expired, unavailable or not approved")
var ErrRemoteImportCapacity = errors.New("import request history full")

type RemoteImport struct {
	SyncError     string               `json:"syncError,omitempty"`
	FollowShelf   bool                 `json:"followShelf,omitempty"`
	SyncSkillID   string               `json:"syncSkillId,omitempty"`
	DestinationID string               `json:"destinationId,omitempty"`
	SyncRunID     string               `json:"syncRunId,omitempty"`
	ID            string               `json:"id"`
	RequestKey    string               `json:"requestKey"`
	AgentID       string               `json:"agentId"`
	EnvironmentID string               `json:"environmentId"`
	Host          string               `json:"host"`
	Agent         string               `json:"agent"`
	Project       string               `json:"project"`
	Name          string               `json:"name"`
	Instance      string               `json:"instanceId"`
	State         string               `json:"state"`
	BundleID      string               `json:"bundleId"`
	ExpectedBase  string               `json:"expectedBase"`
	CreatedAt     int64                `json:"createdAt"`
	UpdatedAt     int64                `json:"updatedAt"`
	ExpiresAt     int64                `json:"expiresAt"`
	Preview       *skillimport.Preview `json:"preview,omitempty"`
	PreviewHash   string               `json:"previewHash,omitempty"`
	Excluded      []string             `json:"excluded"`
	Digest        string               `json:"digest,omitempty"`
	Receipt       *PrivateSkillResult  `json:"receipt,omitempty"`
	Error         string               `json:"error,omitempty"`
}
type ImportCapability struct {
	AgentID    string `json:"agentId"`
	Ready      bool   `json:"ready"`
	ObservedAt int64  `json:"observedAt"`
}
type RemoteImportCreate struct {
	FollowShelf   bool   `json:"followShelf,omitempty"`
	SyncSkillID   string `json:"syncSkillId,omitempty"`
	DestinationID string `json:"destinationId,omitempty"`
	RequestKey    string `json:"requestKey"`
	AgentID       string `json:"agentId"`
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	BundleID      string `json:"bundleId"`
	ExpectedBase  string `json:"expectedBase"`
}

func (s *Store) migrateRemoteImports() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS machine_import_capabilities(agent_id TEXT PRIMARY KEY,user_id INTEGER NOT NULL,instance TEXT NOT NULL,observed_at INTEGER NOT NULL,FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS machine_import_requests(id TEXT PRIMARY KEY,user_id INTEGER NOT NULL,agent_id TEXT NOT NULL,request_key TEXT NOT NULL,state TEXT NOT NULL,data TEXT NOT NULL,created_at INTEGER NOT NULL,UNIQUE(user_id,request_key),FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
func activeImport(state string) bool {
	switch state {
	case "queued", "previewing", "reviewing", "approved", "uploading":
		return true
	}
	return false
}
func readImport(q interface{ QueryRow(string, ...any) *sql.Row }, uid int64, id string) (r RemoteImport, err error) {
	var raw string
	err = q.QueryRow(`SELECT data FROM machine_import_requests WHERE id=? AND user_id=? AND `+privateSkillAccount, id, uid, uid).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrPrivateSkillMissing
	}
	if err == nil {
		err = json.Unmarshal([]byte(raw), &r)
	}
	return
}
func writeImport(tx *sql.Tx, uid int64, r RemoteImport) error {
	r.UpdatedAt = now()
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE machine_import_requests SET state=?,data=? WHERE id=? AND user_id=?`, r.State, string(raw), r.ID, uid)
	return err
}
func (s *Store) RemoteImportsFor(uid int64) ([]RemoteImport, []ImportCapability, error) {
	rows, err := s.db.Query(`SELECT data FROM machine_import_requests WHERE user_id=? AND `+privateSkillAccount+` ORDER BY created_at DESC`, uid, uid)
	if err != nil {
		return nil, nil, err
	}
	requests := []RemoteImport{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			break
		}
		var r RemoteImport
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			break
		}
		if activeImport(r.State) && r.ExpiresAt < now() {
			r.State = "expired"
		}
		r.Preview = nil
		if r.Receipt != nil {
			receipt := *r.Receipt
			receipt.Revision.Manifest = nil
			receipt.Revision.Files = nil
			r.Receipt = &receipt
		}
		requests = append(requests, r)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	rows, err = s.db.Query(`SELECT c.agent_id,c.observed_at,a.paused FROM machine_import_capabilities c JOIN sync_agents a ON a.id=c.agent_id AND a.user_id=c.user_id WHERE c.user_id=? AND `+privateSkillAccount, uid, uid)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	caps := []ImportCapability{}
	for rows.Next() {
		var c ImportCapability
		var paused bool
		if err = rows.Scan(&c.AgentID, &c.ObservedAt, &paused); err != nil {
			return nil, nil, err
		}
		c.Ready = !paused && c.ObservedAt > now()-90000
		caps = append(caps, c)
	}
	return requests, caps, rows.Err()
}
func (s *Store) CreateRemoteImportFor(uid int64, in RemoteImportCreate) (out RemoteImport, err error) {
	if !privateSkillRequestID.MatchString(in.RequestKey) || in.Name == "" || !contentobs.ValidText(in.Name, 255) || strings.ContainsAny(in.Name, "/\\") || in.Name == "." || in.Name == ".." {
		return out, ErrPrivateSkillInvalid
	}
	if (in.BundleID == "") != (in.ExpectedBase == "") || in.BundleID != "" && (!privateSkillID.MatchString(in.BundleID) || !privateSkillID.MatchString(in.ExpectedBase)) {
		return out, ErrPrivateSkillInvalid
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		var existing string
		err := tx.QueryRow(`SELECT id FROM machine_import_requests WHERE user_id=? AND request_key=?`, uid, in.RequestKey).Scan(&existing)
		if err == nil {
			out, err = readImport(tx, uid, existing)
			if err != nil {
				return err
			}
			if out.FollowShelf != in.FollowShelf || out.SyncSkillID != in.SyncSkillID || out.DestinationID != in.DestinationID || out.AgentID != in.AgentID || out.EnvironmentID != in.EnvironmentID || out.Name != in.Name || out.BundleID != in.BundleID || out.ExpectedBase != in.ExpectedBase {
				return ErrRemoteImport
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := s.validateSyncImport(tx, uid, in); err != nil {
			return err
		}
		if err = contentAgent(tx, uid, in.AgentID); err != nil {
			return err
		}
		var instance string
		var observed int64
		if err = tx.QueryRow(`SELECT instance,observed_at FROM machine_import_capabilities WHERE agent_id=? AND user_id=?`, in.AgentID, uid).Scan(&instance, &observed); err != nil || observed < now()-90000 {
			return ErrRemoteImport
		}
		var total, busy int
		if err = tx.QueryRow(`SELECT count(*) FROM machine_import_requests WHERE user_id=?`, uid).Scan(&total); err != nil {
			return err
		}
		if total >= 128 {
			return ErrRemoteImportCapacity
		}
		if err = tx.QueryRow(`SELECT count(*) FROM machine_import_requests WHERE agent_id=? AND state IN ('queued','previewing','reviewing','approved','uploading')`, in.AgentID).Scan(&busy); err != nil {
			return err
		}
		if busy > 0 && in.SyncSkillID == "" {
			return ErrRemoteImport
		}
		out = RemoteImport{FollowShelf: in.FollowShelf, SyncSkillID: in.SyncSkillID, DestinationID: in.DestinationID, ID: syncID(), RequestKey: in.RequestKey, AgentID: in.AgentID, EnvironmentID: in.EnvironmentID, Name: in.Name, Instance: instance, State: "queued", BundleID: in.BundleID, ExpectedBase: in.ExpectedBase, CreatedAt: now(), UpdatedAt: now(), ExpiresAt: now() + 10*60*1000, Excluded: []string{}}
		err = tx.QueryRow(`SELECT e.host,e.agent,e.project FROM environments e JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE e.id=? AND e.user_id=? AND a.id=?`, in.EnvironmentID, uid, in.AgentID).Scan(&out.Host, &out.Agent, &out.Project)
		if err != nil || !contentobs.ValidScope(out.Agent, out.Project) {
			return ErrRemoteImport
		}
		if in.BundleID != "" {
			b, e := privateSkillBundle(tx, uid, in.BundleID)
			if e != nil {
				return e
			}
			if b.Name != in.Name || b.Head != in.ExpectedBase {
				return ErrPrivateSkillConflict
			}
		}
		raw, _ := json.Marshal(out)
		_, err = tx.Exec(`INSERT INTO machine_import_requests(id,user_id,agent_id,request_key,state,data,created_at) VALUES(?,?,?,?,?,?,?)`, out.ID, uid, out.AgentID, out.RequestKey, out.State, string(raw), out.CreatedAt)
		return err
	})
	return
}
func (s *Store) ClaimRemoteImportFor(uid int64, agent, instance string) (work *RemoteImport, err error) {
	if !contentobs.ValidID(instance) {
		return nil, ErrPrivateSkillInvalid
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if err := contentAgent(tx, uid, agent); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO machine_import_capabilities(agent_id,user_id,instance,observed_at) VALUES(?,?,?,?) ON CONFLICT(agent_id) DO UPDATE SET instance=excluded.instance,observed_at=excluded.observed_at`, agent, uid, instance, now())
		if err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT data FROM machine_import_requests WHERE agent_id=? AND user_id=? AND state IN ('queued','previewing','reviewing','approved','uploading') ORDER BY created_at`, agent, uid)
		if err != nil {
			return err
		}
		pending := []RemoteImport{}
		for rows.Next() {
			var raw string
			if err = rows.Scan(&raw); err != nil {
				break
			}
			var r RemoteImport
			if err = json.Unmarshal([]byte(raw), &r); err != nil {
				break
			}
			pending = append(pending, r)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		var running int
		if err = tx.QueryRow(`SELECT count(*) FROM sync_jobs WHERE agent_id=? AND user_id=? AND status='running'`, agent, uid).Scan(&running); err != nil {
			return err
		}
		for _, r := range pending {
			if r.Instance != instance || r.ExpiresAt < now() {
				r.State = "expired"
				r.Error = "preview_expired"
				if err = writeImport(tx, uid, r); err != nil {
					return err
				}
				continue
			}
			if running > 0 || r.State == "reviewing" {
				continue
			}
			switch r.State {
			case "queued", "previewing":
				r.State = "previewing"
			case "approved", "uploading":
				r.State = "uploading"
			default:
				continue
			}
			if err = writeImport(tx, uid, r); err != nil {
				return err
			}
			work = &r
			return nil
		}
		return nil
	})
	return
}
func checkRemoteImport(tx *sql.Tx, uid int64, r RemoteImport, agent, instance string) error {
	if r.AgentID != agent || r.Instance != instance || r.ExpiresAt < now() {
		return ErrRemoteImport
	}
	if err := contentAgent(tx, uid, agent); err != nil {
		return err
	}
	var current string
	var seen int64
	if err := tx.QueryRow(`SELECT instance,observed_at FROM machine_import_capabilities WHERE agent_id=? AND user_id=?`, agent, uid).Scan(&current, &seen); err != nil || current != instance || seen < now()-90000 {
		return ErrRemoteImport
	}
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM environments e JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE e.id=? AND e.user_id=? AND a.id=? AND e.agent=? AND e.project=?`, r.EnvironmentID, uid, agent, r.Agent, r.Project).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return ErrRemoteImport
	}
	return nil
}
func (s *Store) RecordRemotePreviewFor(uid int64, agent, id, instance string, p skillimport.Preview) error {
	hash, err := p.Hash()
	if err != nil {
		return ErrPrivateSkillInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		r, err := readImport(tx, uid, id)
		if err != nil {
			return err
		}
		if err = checkRemoteImport(tx, uid, r, agent, instance); err != nil {
			return err
		}
		if r.PreviewHash != "" {
			if r.PreviewHash == hash {
				return nil
			}
			return ErrRemoteImport
		}
		if r.State != "previewing" {
			return ErrRemoteImport
		}
		r.Preview = &p
		r.PreviewHash = hash
		r.State = "reviewing"
		if r.DestinationID != "" && p.Allowed(nil) {
			m, e := p.Manifest(nil)
			if e != nil {
				return e
			}
			r.Digest = m.Digest
			r.State = "approved"
		}
		return writeImport(tx, uid, r)
	})
}
func (s *Store) ApproveRemoteImportFor(uid int64, id, hash string, exclude []string) (out RemoteImport, err error) {
	sort.Strings(exclude)
	err = s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		r, err := readImport(tx, uid, id)
		if err != nil {
			return err
		}
		if err = checkRemoteImport(tx, uid, r, r.AgentID, r.Instance); err != nil {
			return err
		}
		if r.Preview == nil || r.PreviewHash != hash || !r.Preview.Allowed(exclude) {
			return ErrRemoteImport
		}
		m, err := r.Preview.Manifest(exclude)
		if err != nil {
			return err
		}
		if r.State != "reviewing" {
			if (r.State == "approved" || r.State == "uploading" || r.State == "completed") && r.Digest == m.Digest {
				out = r
				return nil
			}
			return ErrRemoteImport
		}
		r.Excluded = append([]string{}, exclude...)
		r.Digest = m.Digest
		r.State = "approved"
		out = r
		return writeImport(tx, uid, r)
	})
	return
}
func (s *Store) FailRemoteImportFor(uid int64, agent, id, instance, code string) error {
	switch code {
	case "capture_failed", "scope_unavailable", "source_changed", "transfer_failed", "invalid_skill_manifest":
	default:
		return ErrPrivateSkillInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		r, err := readImport(tx, uid, id)
		if err != nil {
			return err
		}
		if r.AgentID != agent || r.Instance != instance {
			return ErrRemoteImport
		}
		if !activeImport(r.State) {
			return nil
		}
		r.State = "failed"
		r.Error = code
		return writeImport(tx, uid, r)
	})
}
func (s *Store) CancelRemoteImportFor(uid int64, id string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		r, err := readImport(tx, uid, id)
		if err != nil {
			return err
		}
		if activeImport(r.State) && r.ExpiresAt >= now() {
			r.State = "cancelled"
			return writeImport(tx, uid, r)
		}
		_, err = tx.Exec(`DELETE FROM machine_import_requests WHERE id=? AND user_id=?`, id, uid)
		return err
	})
}
func (s *Store) CommitRemoteImportFor(uid int64, agent, id, instance, sourceDigest string, files []skillbundle.ContentFile) (PrivateSkillResult, error) {
	r, err := readImport(s.db, uid, id)
	if err != nil {
		return PrivateSkillResult{}, err
	}
	if r.AgentID != agent || r.Preview == nil || r.Preview.SourceDigest != sourceDigest {
		return PrivateSkillResult{}, ErrRemoteImport
	}
	in := MachineSkillImport{SchemaVersion: 1, RequestID: "remote_" + r.ID, BundleID: r.BundleID, ExpectedBase: &r.ExpectedBase, Agent: r.Agent, Project: r.Project, Name: r.Name, Digest: r.Digest, Files: files}
	return s.saveMachineSkillImportFor(uid, agent, in, func(tx *sql.Tx) error {
		current, err := readImport(tx, uid, id)
		if err != nil {
			return err
		}
		if current.AgentID != agent || current.Instance != instance || current.Digest != r.Digest || current.PreviewHash != r.PreviewHash {
			return ErrRemoteImport
		}
		if current.State == "completed" {
			return nil
		}
		if current.State != "uploading" {
			return ErrRemoteImport
		}
		return checkRemoteImport(tx, uid, current, agent, instance)
	}, func(tx *sql.Tx, result PrivateSkillResult) error {
		current, err := readImport(tx, uid, id)
		if err != nil {
			return err
		}
		current.State = "completed"
		current.Receipt = &result
		if err := bindPrivateSync(tx, uid, current, result); err != nil {
			return err
		}
		return writeImport(tx, uid, current)
	})
}

func (s *Store) RemoteImportFor(uid int64, id string) (RemoteImport, error) {
	r, err := readImport(s.db, uid, id)
	if err == nil && activeImport(r.State) && r.ExpiresAt < now() {
		r.State = "expired"
	}
	return r, err
}

// StopRemoteImportFor revokes outstanding approval, but preserves an already
// committed receipt when completion wins the race. Dismissing history is separate.
func (s *Store) StopRemoteImportFor(uid int64, id string) (out RemoteImport, err error) {
	err = s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		r, err := readImport(tx, uid, id)
		if err != nil {
			return err
		}
		if activeImport(r.State) {
			r.State = "cancelled"
			if err = writeImport(tx, uid, r); err != nil {
				return err
			}
		}
		out = r
		return nil
	})
	return
}
