package store

import (
	"database/sql"
	"errors"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"sort"
)

// LibraryDifference is a connected machine holding files under a library
// item's name that are not any version reet has saved for it. reet keeps its
// own copy until a person says which one is right.
type LibraryDifference struct {
	EnvironmentID string `json:"environmentId"`
	Host          string `json:"host"`
	Agent         string `json:"agent"`
	Digest        string `json:"digest"`
	// Taking is set while the machine's version is being saved as the item's copy.
	Taking bool `json:"taking,omitempty"`
	// Comparing is set while the machine is being asked which files it has.
	Comparing bool `json:"comparing,omitempty"`
	// Compared is set once the machine has answered; Files then lists what
	// differs. Only names, sizes and fingerprints were read: no file contents
	// leave the machine for a comparison.
	Compared bool             `json:"compared,omitempty"`
	Files    []DifferenceFile `json:"files,omitempty"`
}

// DifferenceFile is one file that is not the same in reet's copy and on the
// machine. Change is "changed" (in both, different), "machine" (only on the
// machine) or "reet" (only in reet's copy).
type DifferenceFile struct {
	Path         string `json:"path"`
	Change       string `json:"change"`
	ReetBytes    int64  `json:"reetBytes,omitempty"`
	MachineBytes int64  `json:"machineBytes,omitempty"`
}

// compareKey names one comparison: this item's current version against this
// folder's current files. A new version or a new edit is a new comparison.
func compareKey(item LibraryItem, environment, digest string) string {
	return "compare_" + secretHash(item.ID+"\x00"+environment+"\x00"+item.RevisionID+"\x00"+digest)
}

// differenceFiles lists what differs between a saved version and what a
// machine reported, by path.
func differenceFiles(reet, machine []skillbundle.File) []DifferenceFile {
	saved := map[string]skillbundle.File{}
	for _, f := range reet {
		saved[f.Path] = f
	}
	out := []DifferenceFile{}
	seen := map[string]bool{}
	for _, f := range machine {
		seen[f.Path] = true
		have, ok := saved[f.Path]
		switch {
		case !ok:
			out = append(out, DifferenceFile{Path: f.Path, Change: "machine", MachineBytes: f.Bytes})
		case have.Digest != f.Digest || have.Executable != f.Executable:
			out = append(out, DifferenceFile{Path: f.Path, Change: "changed", ReetBytes: have.Bytes, MachineBytes: f.Bytes})
		}
	}
	for _, f := range reet {
		if !seen[f.Path] {
			out = append(out, DifferenceFile{Path: f.Path, Change: "reet", ReetBytes: f.Bytes})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// differenceRequests reads the capture requests between one item and one
// folder: whether the machine's version is being taken, and the comparison
// asked for under key, if any.
func (s *Store) differenceRequests(uid int64, item LibraryItem, environment, key string) (taking bool, compare *RemoteImport, err error) {
	rows, err := s.db.Query(`SELECT id FROM machine_import_requests WHERE user_id=? AND json_extract(data,'$.bundleId')=? AND json_extract(data,'$.environmentId')=? ORDER BY created_at`, uid, item.BundleID, environment)
	if err != nil {
		return false, nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return false, nil, err
	}
	for _, id := range ids {
		r, err := s.RemoteImportFor(uid, id)
		if err != nil {
			return false, nil, err
		}
		if r.RequestKey == key {
			found := r
			compare = &found
		}
		if !activeImport(r.State) {
			continue
		}
		// A comparison waits at review; anything approved, uploading or asked
		// for by name is the machine's version on its way in.
		if r.Chosen || r.State == "approved" || r.State == "uploading" {
			taking = true
		}
	}
	return taking, compare, nil
}

func (s *Store) migrateLibraryDifferences() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS library_difference_choices(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,item_id TEXT NOT NULL,env_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,digest TEXT NOT NULL,chosen_at INTEGER NOT NULL,PRIMARY KEY(user_id,item_id,env_id))`)
	return err
}

// LibraryDifferencesFor compares what each connected machine last reported
// with the library, by name. A machine copy counts as different only when its
// digest is none of the item's saved versions: a machine still holding an
// older version is merely behind, and sync brings it up to date.
func (s *Store) LibraryDifferencesFor(uid int64, items []LibraryItem) (map[string][]LibraryDifference, error) {
	byName := map[string][]LibraryItem{}
	for _, item := range items {
		if item.ItemType == LibrarySkill && item.RevisionID != "" && item.Digest != "" && item.BundleID != "" {
			byName[item.Name] = append(byName[item.Name], item)
		}
	}
	if len(byName) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT e.id,e.host,e.agent FROM environments e JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE e.user_id=? AND e.project='' AND e.agent IN ('agents','claude','cursor','opencode') AND a.paused=0 ORDER BY e.host,e.agent`, uid)
	if err != nil {
		return nil, err
	}
	type folder struct{ id, host, agent string }
	var folders []folder
	for rows.Next() {
		var f folder
		if err = rows.Scan(&f.id, &f.host, &f.agent); err != nil {
			break
		}
		folders = append(folders, f)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := map[string][]LibraryDifference{}
	for _, f := range folders {
		content, err := s.ContentObservationFor(uid, f.id)
		if err != nil {
			return nil, err
		}
		// A stale or paused report is not evidence of what is there now.
		if content.State != "observed" && content.State != "incomplete" {
			continue
		}
		for _, observed := range content.Skills {
			named := byName[observed.Name]
			// Two items sharing a name cannot say which one the folder is a copy of.
			if !observed.Complete || observed.Digest == "" || len(named) != 1 {
				continue
			}
			item := named[0]
			if observed.Digest == item.Digest {
				continue
			}
			var known, chosen int
			if err = s.db.QueryRow(`SELECT (SELECT count(*) FROM private_skill_revisions WHERE user_id=? AND bundle_id=? AND manifest_digest=?),(SELECT count(*) FROM library_difference_choices WHERE user_id=? AND item_id=? AND env_id=? AND digest=?)`, uid, item.BundleID, observed.Digest, uid, item.ID, f.id, observed.Digest).Scan(&known, &chosen); err != nil {
				return nil, err
			}
			if known > 0 || chosen > 0 {
				continue
			}
			taking, compare, err := s.differenceRequests(uid, item, f.id, compareKey(item, f.id, observed.Digest))
			if err != nil {
				return nil, err
			}
			difference := LibraryDifference{EnvironmentID: f.id, Host: f.host, Agent: f.agent, Digest: observed.Digest, Taking: taking}
			if compare != nil && !taking {
				switch {
				case compare.Preview != nil:
					revision, err := s.PrivateSkillRevisionFor(uid, item.BundleID, item.RevisionID)
					if err != nil {
						return nil, err
					}
					machine := make([]skillbundle.File, len(compare.Preview.Files))
					for i, file := range compare.Preview.Files {
						machine[i] = file.File
					}
					var saved []skillbundle.File
					if revision.Manifest != nil {
						saved = revision.Manifest.Files
					}
					difference.Compared = true
					difference.Files = differenceFiles(saved, machine)
				case activeImport(compare.State):
					difference.Comparing = true
				}
			}
			out[item.ID] = append(out[item.ID], difference)
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool {
			if out[id][i].Host != out[id][j].Host {
				return out[id][i].Host < out[id][j].Host
			}
			return out[id][i].Agent < out[id][j].Agent
		})
	}
	return out, nil
}

// ResolveLibraryDifferenceFor records a person's answer. "keep" leaves reet's
// copy as it is and stops flagging this machine's current files; if the
// machine's files change again, the question comes back. "take" asks the
// machine for its files and saves them as the item's next version, which
// every machine the item is selected for then receives.
func (s *Store) ResolveLibraryDifferenceFor(uid int64, itemID, environment, action string) error {
	item, err := s.LibraryItemFor(uid, itemID)
	if err != nil {
		return err
	}
	differences, err := s.LibraryDifferencesFor(uid, []LibraryItem{item})
	if err != nil {
		return err
	}
	var found *LibraryDifference
	for i := range differences[item.ID] {
		if differences[item.ID][i].EnvironmentID == environment {
			found = &differences[item.ID][i]
		}
	}
	if found == nil {
		return ErrLibraryConflict
	}
	key := compareKey(item, environment, found.Digest)
	_, compare, err := s.differenceRequests(uid, item, environment, key)
	if err != nil {
		return err
	}
	agentFor := func() (string, error) {
		var agent string
		err := s.db.QueryRow(`SELECT a.id FROM sync_agents a JOIN environments e ON e.user_id=a.user_id AND e.host=a.host COLLATE NOCASE WHERE e.id=? AND e.user_id=?`, environment, uid).Scan(&agent)
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrRemoteNotFound
		}
		return agent, err
	}
	switch action {
	case "compare":
		if found.Taking || found.Compared || found.Comparing {
			return nil
		}
		// A comparison that ended without an answer (the machine went away, or
		// it timed out) is asked again under the same name.
		if compare != nil {
			if _, err = s.db.Exec(`DELETE FROM machine_import_requests WHERE id=? AND user_id=?`, compare.ID, uid); err != nil {
				return err
			}
		}
		agent, err := agentFor()
		if err != nil {
			return err
		}
		_, err = s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: key, AgentID: agent, EnvironmentID: environment, Name: item.Name, BundleID: item.BundleID, ExpectedBase: item.RevisionID})
		return err
	case "keep":
		if compare != nil {
			if err = s.CancelRemoteImportFor(uid, compare.ID); err != nil {
				return err
			}
		}
		_, err = s.db.Exec(`INSERT INTO library_difference_choices(user_id,item_id,env_id,digest,chosen_at) VALUES(?,?,?,?,?) ON CONFLICT(user_id,item_id,env_id) DO UPDATE SET digest=excluded.digest,chosen_at=excluded.chosen_at`, uid, item.ID, environment, found.Digest, now())
		return err
	case "take":
		if found.Taking {
			return nil
		}
		// The machine has already listed these exact files for a comparison:
		// approving that request takes them without asking it twice.
		if compare != nil && compare.State == "reviewing" && compare.Preview != nil && compare.Preview.Allowed(nil) && compare.ExpiresAt >= now() {
			if _, err = s.ApproveRemoteImportFor(uid, compare.ID, compare.PreviewHash, nil); err == nil {
				return nil
			}
		}
		if compare != nil {
			if err = s.CancelRemoteImportFor(uid, compare.ID); err != nil {
				return err
			}
		}
		agent, err := agentFor()
		if err != nil {
			return err
		}
		// The key names the exact pair of versions, so pressing twice asks once
		// and a later, different edit on the machine can be taken again.
		key := "take_" + secretHash(item.ID+"\x00"+environment+"\x00"+item.RevisionID+"\x00"+found.Digest)
		_, err = s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: key, AgentID: agent, EnvironmentID: environment, Name: item.Name, BundleID: item.BundleID, ExpectedBase: item.RevisionID, Chosen: true})
		return err
	}
	return ErrPrivateSkillInvalid
}
