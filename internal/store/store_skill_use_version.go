package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
)

var (
	// ErrSkillMachineMissing is a machine that is not one of the account's.
	ErrSkillMachineMissing = errors.New("that machine is not one of yours")
	// ErrSkillNoBackup is Use this version of a skill reet knows of no
	// edited copy of on that machine.
	ErrSkillNoBackup = errors.New("reet knows of no edit to that skill on that machine")
	// ErrSkillBackupPath is a backup outside the backups beside the managed
	// skills folders.
	ErrSkillBackupPath = errors.New("a version can only be taken from a backup beside a managed skills folder")
	// ErrSkillMachineOld is a machine whose reet cannot read a backup for
	// Use this version.
	ErrSkillMachineOld = errors.New("reet on that machine is too old to send a version")
	// ErrSkillMachinePaused is a machine whose sync is paused.
	ErrSkillMachinePaused = errors.New("sync is paused on that machine")
)

// UseVersionRequest is the capture Use this version asked a machine for.
type UseVersionRequest struct {
	ID, Host, Backup string
}

// managedBackupPath says whether p, a path a machine reported, names one
// backup beside one of the four managed skills folders: under home, in the
// folder's .reet-backups/{date}/ or, from before, its
// .reet-sync-backups/removed-*/. It returns the agent of that folder. The
// path is the machine's own: the controller never opens it, and hands it back
// only to that machine, which checks it again.
func managedBackupPath(p string) (agent string, ok bool) {
	if p == "" || len(p) > 1024 || !utf8.ValidString(p) || strings.ContainsAny(p, "\x00\r\n\\") {
		return "", false
	}
	rest, ok := strings.CutPrefix(p, "~/")
	if !ok {
		return "", false
	}
	segs := strings.Split(rest, "/")
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return "", false
		}
	}
	for _, m := range managedSkillFolders {
		parent := strings.Split(strings.TrimPrefix(path.Dir(m.folder), "~/"), "/")
		if len(segs) != len(parent)+3 || strings.Join(segs[:len(parent)], "/") != strings.Join(parent, "/") {
			continue
		}
		root, group := segs[len(parent)], segs[len(parent)+1]
		switch root {
		case ".reet-backups":
			date, err := time.Parse("2006-01-02", group)
			if err == nil && date.Format("2006-01-02") == group {
				return m.agent, true
			}
		case ".reet-sync-backups":
			if len(group) > len("removed-") && strings.HasPrefix(group, "removed-") {
				return m.agent, true
			}
		}
	}
	return "", false
}

// RequestSkillUseVersionFor is Use this version: it asks the machine agentID
// for its backup of the copy of item it edited, through the existing machine
// capture, and the files it sends become a new revision of item (origin kind
// use-version) that every machine then gets. The backup is the newest edited
// copy of the skill reet heard of from that machine (SkillBackupsFor at now).
// While a request for that backup is running, it asks nothing more.
func (s *Store) RequestSkillUseVersionFor(uid int64, item, agentID string, now time.Time) (UseVersionRequest, error) {
	var out UseVersionRequest
	var name, bundle string
	err := s.db.QueryRow(`SELECT name,bundle_id FROM library_items WHERE user_id=? AND id=? AND item_type=? AND deleted_at=0 AND `+privateSkillAccount, uid, item, LibrarySkill, uid).Scan(&name, &bundle)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrPrivateSkillMissing
	}
	if err != nil {
		return out, err
	}
	var protocol int
	var paused bool
	err = s.db.QueryRow(`SELECT host,skills_protocol,paused FROM sync_agents WHERE id=? AND user_id=?`, agentID, uid).Scan(&out.Host, &protocol, &paused)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrSkillMachineMissing
	}
	if err != nil {
		return out, err
	}
	backups, err := s.SkillBackupsFor(uid, name, now)
	if err != nil {
		return out, err
	}
	for _, b := range backups { // newest first
		if b.AgentID == agentID {
			out.Backup = b.Backup
			break
		}
	}
	if out.Backup == "" || bundle == "" {
		return out, ErrSkillNoBackup
	}
	kind, ok := managedBackupPath(out.Backup)
	if !ok {
		return out, ErrSkillBackupPath
	}
	switch {
	case protocol != CurrentSkillsProtocol:
		return out, ErrSkillMachineOld
	case paused:
		return out, ErrSkillMachinePaused
	}
	b, err := s.PrivateSkillBundleFor(uid, bundle)
	if err != nil {
		return out, err
	}
	var running, asked int
	if err = s.db.QueryRow(`SELECT COALESCE(SUM(state IN ('queued','previewing','reviewing','approved','uploading')),0),count(*) FROM machine_import_requests WHERE user_id=? AND agent_id=? AND json_extract(data,'$.useVersionItem')=? AND json_extract(data,'$.backupPath')=?`, uid, agentID, item, out.Backup).Scan(&running, &asked); err != nil {
		return out, err
	}
	if running > 0 {
		return out, nil
	}
	if err = s.TouchEnvironmentFor(uid, out.Host, kind, "", ""); err != nil {
		return out, err
	}
	key := "useversion_" + secretHash(item+"\x00"+agentID+"\x00"+out.Backup+"\x00"+string(rune('0'+asked)))
	r, err := s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: key, AgentID: agentID, EnvironmentID: EnvironmentIDFor(uid, out.Host, kind, ""), Name: b.Name, BundleID: b.ID, ExpectedBase: b.Head, Chosen: true, UseVersionItem: item, BackupPath: out.Backup})
	out.ID = r.ID
	return out, err
}

// commitUseVersion saves the files a machine sent from its backup for Use
// this version as a new revision of the request's library skill, of origin
// kind use-version, while the skill still has the head the request was made
// against. The backup is then the skill's content, so it is no longer
// offered.
func (s *Store) commitUseVersion(uid int64, agent, instance string, r RemoteImport, files []skillbundle.ContentFile) (PrivateSkillResult, error) {
	m, _, err := skillimport.Select(files, nil)
	if err != nil {
		return PrivateSkillResult{}, ErrPrivateSkillInvalid
	}
	if m.Digest != r.Digest {
		return PrivateSkillResult{}, ErrMachineImportChanged
	}
	reference, _ := json.Marshal(struct{ Enrollment, Host, Backup string }{agent, r.Host, r.BackupPath})
	write := PrivateSkillWrite{RequestID: "remote_" + r.ID, ExpectedBase: &r.ExpectedBase, Name: r.Name, Files: files, Origin: PrivateSkillOrigin{Kind: "use-version", Reference: string(reference)}}
	return s.savePrivateSkillRevisionWithReceipt(uid, r.BundleID, write, func(tx *sql.Tx) error {
		if err := contentAgent(tx, uid, agent); err != nil {
			return err
		}
		current, err := readImport(tx, uid, r.ID)
		if err != nil {
			return err
		}
		sameAgent := current.AgentID == agent && current.Instance == instance
		sameContent := current.Digest == r.Digest && current.PreviewHash == r.PreviewHash && current.UseVersionItem == r.UseVersionItem
		if !sameAgent || !sameContent {
			return ErrRemoteImport
		}
		if current.State == "completed" {
			return nil
		}
		if current.State != "uploading" {
			return ErrRemoteImport
		}
		if err = checkRemoteImport(tx, uid, current, agent, instance); err != nil {
			return err
		}
		var live int
		if err = tx.QueryRow(`SELECT count(*) FROM library_items WHERE user_id=? AND id=? AND item_type=? AND deleted_at=0 AND bundle_id=?`, uid, r.UseVersionItem, LibrarySkill, r.BundleID).Scan(&live); err != nil {
			return err
		}
		if live != 1 {
			return ErrPrivateSkillMissing
		}
		return nil
	}, func(tx *sql.Tx, result PrivateSkillResult) error {
		if err := attachLibraryBundle(tx, uid, r.UseVersionItem, result); err != nil {
			return err
		}
		if err := skillCopyLandedTx(tx, uid, r.UseVersionItem, "", false); err != nil {
			return err
		}
		current, err := readImport(tx, uid, r.ID)
		if err != nil {
			return err
		}
		current.State = "completed"
		current.Receipt = &result
		if err = writeImport(tx, uid, current); err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM skill_edit_backups WHERE user_id=? AND agent_id=? AND backup=?`, uid, agent, r.BackupPath)
		return err
	})
}
