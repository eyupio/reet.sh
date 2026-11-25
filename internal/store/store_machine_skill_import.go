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

// ErrMachineBundleNotOwned is a machine adding to a bundle it did not start.
var ErrMachineBundleNotOwned = errors.New("this machine did not start that skill")

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
		// Only to a bundle this enrolment started: an existing bundle with no revision from it is another
		// machine's, or the browser's, and its next revision would be installed on the account's other machines.
		// A bundle that does not exist falls through to the missing-bundle error below.
		if in.BundleID != "" {
			var exists, own int
			err := tx.QueryRow(`SELECT (SELECT COUNT(*) FROM private_skill_bundles WHERE user_id=? AND id=?),
				(SELECT COUNT(*) FROM private_skill_revisions WHERE user_id=? AND bundle_id=? AND json_extract(origin,'$.kind')='import' AND json_extract(json_extract(origin,'$.reference'),'$.Enrollment')=?)`,
				uid, in.BundleID, uid, in.BundleID, enrollment).Scan(&exists, &own)
			if err != nil {
				return err
			}
			if exists > 0 && own == 0 {
				return ErrMachineBundleNotOwned
			}
		}
		if guard != nil {
			return guard(tx)
		}
		return nil
	}, func(tx *sql.Tx, result PrivateSkillResult) error {
		if finish != nil {
			if err := finish(tx, result); err != nil {
				return err
			}
		}
		// A capture that would make a second item of a name is refused.
		if err := refuseSecondSkillName(tx, uid, result); err != nil {
			return err
		}
		return machineCopyLandedTx(tx, uid, enrollment, in.Agent, result)
	}, func(tx *sql.Tx, result PrivateSkillResult) error {
		return recordLibraryMachineCapture(tx, uid, enrollment, in.Agent, in.Project, in.Name, result)
	})
}
