package store

import (
	"database/sql"
	"errors"
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
			var taking int
			if err = s.db.QueryRow(`SELECT count(*) FROM machine_import_requests WHERE user_id=? AND state IN ('queued','previewing','reviewing','approved','uploading') AND json_extract(data,'$.bundleId')=? AND json_extract(data,'$.environmentId')=?`, uid, item.BundleID, f.id).Scan(&taking); err != nil {
				return nil, err
			}
			out[item.ID] = append(out[item.ID], LibraryDifference{EnvironmentID: f.id, Host: f.host, Agent: f.agent, Digest: observed.Digest, Taking: taking > 0})
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
	switch action {
	case "keep":
		_, err = s.db.Exec(`INSERT INTO library_difference_choices(user_id,item_id,env_id,digest,chosen_at) VALUES(?,?,?,?,?) ON CONFLICT(user_id,item_id,env_id) DO UPDATE SET digest=excluded.digest,chosen_at=excluded.chosen_at`, uid, item.ID, environment, found.Digest, now())
		return err
	case "take":
		if found.Taking {
			return nil
		}
		var agent string
		err = s.db.QueryRow(`SELECT a.id FROM sync_agents a JOIN environments e ON e.user_id=a.user_id AND e.host=a.host COLLATE NOCASE WHERE e.id=? AND e.user_id=?`, environment, uid).Scan(&agent)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
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
