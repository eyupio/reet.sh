package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
)

var ErrMachineImportChanged = errors.New("skill contents do not match the reviewed digest")

// MachineSkillImport has no caller-selected owner, origin or filesystem root.
type MachineSkillImport struct {
	SchemaVersion int                       `json:"schemaVersion"`
	RequestID     string                    `json:"requestId"`
	BundleID      string                    `json:"bundleId,omitempty"`
	ExpectedBase  *string                   `json:"expectedBase"`
	Agent         string                    `json:"agent"`
	Project       string                    `json:"project"`
	Name          string                    `json:"name"`
	Digest        string                    `json:"digest"`
	Files         []skillbundle.ContentFile `json:"files"`
}

func (s *Store) SaveMachineSkillImportFor(uid int64, enrollment string, in MachineSkillImport) (PrivateSkillResult, error) {
	return s.saveMachineSkillImportFor(uid, enrollment, in, nil, nil)
}
func (s *Store) saveMachineSkillImportFor(uid int64, enrollment string, in MachineSkillImport, guard func(*sql.Tx) error, finish func(*sql.Tx, PrivateSkillResult) error) (PrivateSkillResult, error) {
	var empty PrivateSkillResult
	if in.SchemaVersion != 1 || !contentobs.ValidScope(in.Agent, in.Project) {
		return empty, ErrPrivateSkillInvalid
	}
	m, _, err := skillimport.Select(in.Files, nil)
	if err != nil {
		return empty, ErrPrivateSkillInvalid
	}
	if m.Digest != in.Digest {
		return empty, ErrMachineImportChanged
	}
	reference, _ := json.Marshal(struct{ Enrollment, Agent, Project, Name string }{enrollment, in.Agent, in.Project, in.Name})
	write := PrivateSkillWrite{RequestID: in.RequestID, ExpectedBase: in.ExpectedBase, Name: in.Name, Files: in.Files, Origin: PrivateSkillOrigin{Kind: "import", Reference: string(reference)}}
	return s.savePrivateSkillRevisionWithReceipt(uid, in.BundleID, write, func(tx *sql.Tx) error {
		if err := contentAgent(tx, uid, enrollment); err != nil {
			return err
		}
		var found int
		err := tx.QueryRow(`SELECT count(*) FROM environments e JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE a.id=? AND a.user_id=? AND e.agent=? AND e.project=?`, enrollment, uid, in.Agent, in.Project).Scan(&found)
		if err != nil {
			return err
		}
		if found != 1 {
			return ErrRemoteNotFound
		}
		if guard != nil {
			return guard(tx)
		}
		return nil
	}, finish)
}
