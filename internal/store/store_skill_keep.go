package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	// ErrSkillNotNew is a Keep or Archive of a name no machine reports as new.
	ErrSkillNotNew = errors.New("that skill is not new on any machine")
	// ErrSkillChooseCopy is a Keep of a skill whose machines hold different
	// copies, with no machine chosen.
	ErrSkillChooseCopy = errors.New("the machines have different copies of that skill; choose which one to keep")
	// ErrSkillCopyNotThere is a Keep or Archive from a machine that has no
	// copy of the skill.
	ErrSkillCopyNotThere = errors.New("that machine has no copy of that skill")
)

// KeptNewSkill is the library item Keep or Archive made of a skill new on a
// machine, and the copy reet takes its files from.
type KeptNewSkill struct {
	Item, Name, AgentID, Host, Folder string
}

// NewSkillError is why Keep or Archive refused the skill Name. Err is one of
// ErrSkillNotNew, ErrSkillChooseCopy and ErrSkillCopyNotThere, or another
// failure.
type NewSkillError struct {
	Name string
	Err  error
}

func (e *NewSkillError) Error() string { return e.Name + ": " + e.Err.Error() }
func (e *NewSkillError) Unwrap() error { return e.Err }

type newSkillCopy struct {
	name, agent, host, folder, digest string
}

// KeepNewSkillsFor decides skills that machines report and the library does
// not have. status is active (Keep) or archived (Archive). The item takes its
// files from the first machine that reported the skill, or from the machine
// from names; when the copies differ, Keep needs from. A live item of the
// name, matched without case, is joined rather than doubled, and the decision
// applies to it.
//
// It only makes the item and ties it to the machine's copy (a shelf row for
// the copy, sighted in that machine's folder), so the existing capture can ask
// the machine for the files: see RequestSkillCaptureFor. An Archived item's
// name joins the removal list once its files land.
//
// Several names are decided together: if one is refused, none is decided,
// and the error is a *NewSkillError naming it.
func (s *Store) KeepNewSkillsFor(uid int64, names []string, status, from string, at time.Time) ([]KeptNewSkill, error) {
	if !validLibraryStatus(status) {
		return nil, ErrPrivateSkillInvalid
	}
	var out []KeptNewSkill
	err := s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		out = nil
		for _, name := range names {
			k, err := keepNewSkillTx(tx, uid, name, status, from, at)
			if err != nil {
				return &NewSkillError{Name: name, Err: err}
			}
			out = append(out, k)
		}
		return nil
	})
	return out, err
}

func keepNewSkillTx(tx *sql.Tx, uid int64, name, status, from string, at time.Time) (KeptNewSkill, error) {
	var none KeptNewSkill
	copies, err := newSkillCopiesTx(tx, uid, name)
	if err != nil {
		return none, err
	}
	// New is what NewOnMachinesFor lists: reported unknown, not on the
	// removal list and not a live skill. A live skill that is not Active
	// (one Archived before reet has a copy, which machines still report) is
	// joined; an Active one is not new, however stale a report says so.
	var removal, active int
	if err = tx.QueryRow(`SELECT count(*) FROM skill_removals WHERE user_id=? AND name=?`, uid, name).Scan(&removal); err != nil {
		return none, err
	}
	if err = tx.QueryRow(`SELECT count(*) FROM library_items WHERE user_id=? AND item_type=? AND deleted_at=0 AND status=? AND lower(name)=lower(?)`, uid, LibrarySkill, SkillActive, name).Scan(&active); err != nil {
		return none, err
	}
	if len(copies) == 0 || removal > 0 || active > 0 {
		return none, ErrSkillNotNew
	}
	chosen, err := chooseNewSkillCopy(copies, status, from)
	if err != nil {
		return none, err
	}
	kind, _, ok := managedSkillPath(chosen.folder)
	if !ok {
		return none, ErrSkillCapturePath
	}
	item, joined, err := newSkillItemTx(tx, uid, chosen, kind, at)
	if err != nil {
		return none, err
	}
	it, err := skillStateItemTx(tx, uid, item)
	if err != nil {
		return none, err
	}
	if joined {
		err = setSkillStatusTx(tx, uid, it, status, at)
	} else if it.status != status {
		// A new item has no earlier status for machines to keep for the
		// grace period.
		_, err = tx.Exec(`UPDATE library_items SET status=?,prev_status='',status_at=? WHERE user_id=? AND id=?`, status, at.UnixMilli(), uid, item)
	}
	if err != nil {
		return none, err
	}
	// The shelf flags the old pages read follow the status: In use is
	// Active, Parked is Archived.
	used := 0
	if status == SkillActive {
		used = 1
	}
	if _, err = tx.Exec(`UPDATE shelf SET used=?,bookmarked=CASE WHEN ?=1 THEN bookmarked ELSE 0 END WHERE user_id=? AND skill_id=?`, used, used, uid, "local/"+chosen.name); err != nil {
		return none, err
	}
	// The copy it is kept from is where it came from, before its files
	// land: the copy worker asks that machine first.
	if _, err = tx.Exec(`UPDATE library_items SET origin=CASE WHEN origin='' THEN 'machine' ELSE origin END,origin_ref=CASE WHEN origin_ref='' AND revision_id='' THEN ? ELSE origin_ref END WHERE user_id=? AND id=?`, "machine:"+chosen.host+":"+chosen.folder, uid, item); err != nil {
		return none, err
	}
	return KeptNewSkill{Item: item, Name: chosen.name, AgentID: chosen.agent, Host: chosen.host, Folder: chosen.folder}, nil
}

// newSkillCopiesTx lists the copies of name machines report as unknown, in
// the order NewOnMachinesFor lists them, so the first machine listed on the
// page is the one Keep takes the files from.
func newSkillCopiesTx(tx *sql.Tx, uid int64, name string) ([]newSkillCopy, error) {
	rows, err := tx.Query(`SELECT m.name,m.agent_id,a.host,m.folder,m.digest FROM machine_skills m JOIN sync_agents a ON a.id=m.agent_id
 WHERE m.user_id=? AND m.state='unknown' AND lower(m.name)=lower(?) ORDER BY `+newSkillCopyOrder, uid, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []newSkillCopy
	for rows.Next() {
		var c newSkillCopy
		if err = rows.Scan(&c.name, &c.agent, &c.host, &c.folder, &c.digest); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// chooseNewSkillCopy picks the copy a decision takes its files from: the
// first one listed on from when it is given, else the first one listed. Keep
// of copies that differ needs from.
func chooseNewSkillCopy(copies []newSkillCopy, status, from string) (newSkillCopy, error) {
	if from != "" {
		for _, c := range copies {
			if c.agent == from {
				return c, nil
			}
		}
		return newSkillCopy{}, ErrSkillCopyNotThere
	}
	if status == SkillActive {
		for _, c := range copies[1:] {
			if c.digest != copies[0].digest {
				return newSkillCopy{}, ErrSkillChooseCopy
			}
		}
	}
	return copies[0], nil
}

// newSkillItemTx makes the shelf row of the copy and the library item it
// stands for, or joins the live item of its name, and records the copy as
// seen in the machine's folder. joined says the item was already there.
func newSkillItemTx(tx *sql.Tx, uid int64, c newSkillCopy, kind string, at time.Time) (item string, joined bool, err error) {
	shelfID, t := "local/"+c.name, at.UnixMilli()
	var live string
	err = tx.QueryRow(`SELECT id FROM library_items WHERE user_id=? AND item_type=? AND deleted_at=0 AND lower(name)=lower(?) ORDER BY id LIMIT 1`, uid, LibrarySkill, c.name).Scan(&live)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	joined = live != ""
	if _, err = tx.Exec(`INSERT INTO skills(`+cols+`) VALUES(?,?,?,?,?,?,'','',0,0,'',0,0,0,?,?) ON CONFLICT(id) DO NOTHING`, shelfID, "local", c.name, "local", "", c.name, t, t); err != nil {
		return "", false, err
	}
	// The shelf trigger makes the item, or joins the live one of its name.
	if _, err = tx.Exec(`INSERT INTO shelf(user_id,skill_id,liked,bookmarked,used,notes,created_at,updated_at) VALUES(?,?,0,0,1,'',?,?) ON CONFLICT(user_id,skill_id) DO NOTHING`, uid, shelfID, t, t); err != nil {
		return "", false, err
	}
	bound, err := libraryAliasID(tx, uid, "shelf", shelfID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	boundLive := false
	if bound != "" {
		if err = tx.QueryRow(`SELECT deleted_at=0 FROM library_items WHERE user_id=? AND id=?`, uid, bound).Scan(&boundLive); errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return "", false, err
		}
	}
	switch {
	case joined && !boundLive:
		// A shelf row left from an earlier, deleted item of the name now
		// stands for the live one.
		_, err = tx.Exec(`INSERT INTO library_aliases(user_id,kind,reference,item_id) VALUES(?,'shelf',?,?) ON CONFLICT(user_id,kind,reference) DO UPDATE SET item_id=excluded.item_id`, uid, shelfID, live)
		item = live
	case joined:
		item = live
	case boundLive:
		item = bound
	default:
		// The row stood for a deleted item, or for none: the kept skill is
		// a new item.
		if item, err = privateSkillRandomID(); err != nil {
			return "", false, err
		}
		if _, err = tx.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,origin,created_at,updated_at) VALUES(?,?,?,?,?,'machine',?,?)`, uid, item, LibrarySkill, c.name, SkillActive, t, t); err != nil {
			return "", false, err
		}
		_, err = tx.Exec(`INSERT INTO library_aliases(user_id,kind,reference,item_id) VALUES(?,'shelf',?,?) ON CONFLICT(user_id,kind,reference) DO UPDATE SET item_id=excluded.item_id`, uid, shelfID, item)
	}
	if err != nil {
		return "", false, err
	}
	env := EnvironmentIDFor(uid, c.host, kind, "")
	if _, err = tx.Exec(`INSERT INTO environments(id,user_id,name,kind,agent,host,path,project,first_seen,last_seen) VALUES(?,?,?,'machine',?,?,?,'',?,?)
 ON CONFLICT(id) DO UPDATE SET last_seen=excluded.last_seen`, env, uid, EnvironmentName(c.host, kind, ""), kind, c.host, strings.TrimPrefix(c.folder, "~/"), t, t); err != nil {
		return "", false, err
	}
	_, err = tx.Exec(`INSERT INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path) VALUES(?,?,?,?,?) ON CONFLICT(skill_id,env_id) DO UPDATE SET last_seen=excluded.last_seen`, shelfID, env, t, t, c.folder+"/"+c.name)
	return item, joined, err
}
