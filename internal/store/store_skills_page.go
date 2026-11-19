package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/skillimport"
	"gopkg.in/yaml.v3"
)

// SkillPageRow is one skill on the Skills page: a live library skill and what
// the page shows about it. Likes, notes, tags and lists belong to the shelf
// row ShelfID names, and keep their own endpoints.
type SkillPageRow struct {
	ID          string `json:"id"`
	ShelfID     string `json:"shelfId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	// OriginKind is how the skill arrived: written, imported or kept.
	OriginKind string `json:"originKind"`
	// Origin is where it came from, for display only: an imported skill's
	// link, or the machine a kept one was kept from.
	Origin       string         `json:"origin"`
	Yours        bool           `json:"yours"`
	Published    bool           `json:"published"`
	PublicURL    string         `json:"publicUrl,omitempty"`
	HasCopy      bool           `json:"hasCopy"`
	Liked        bool           `json:"liked"`
	Notes        string         `json:"notes"`
	Tags         []Tag          `json:"tags"`
	Lists        []ListRef      `json:"lists"`
	Health       SkillHealth    `json:"health"`
	UpgradedFrom string         `json:"upgradedFrom,omitempty"`
	EditedOn     *SkillEditedOn `json:"editedOn,omitempty"`
	// EditedOnProblem says, in plain words, why Use this version could not
	// take the edited copy EditedOn names, when it tried and could not.
	EditedOnProblem string `json:"editedOnProblem,omitempty"`
	// HostedSlug is the written skill's name in the owner's hosted skills,
	// from which the API makes PublicURL.
	HostedSlug string `json:"-"`
}

// SkillEditedOn is the machine whose edit of a skill was backed up most
// recently, for Use this version.
type SkillEditedOn struct {
	AgentID string `json:"agentId"`
	Host    string `json:"host"`
}

type SkillCounts struct {
	Active   int `json:"active"`
	Archived int `json:"archived"`
}

// SkillsPage is what the Skills page shows for one tab: its skills, the
// count of each tab, the skills new on a machine, how many machines follow
// the library, and the one-time upgrade notice.
type SkillsPage struct {
	Skills        []SkillPageRow `json:"skills"`
	Counts        SkillCounts    `json:"counts"`
	NewOnMachines []NewSkill     `json:"newOnMachines"`
	Machines      int            `json:"machines"`
	Notice        *UpgradeNotice `json:"notice"`
}

// SkillItemRef is a library skill a page action names. Written says the
// person wrote it (origin written).
type SkillItemRef struct {
	ID, Name, Status string
	HasCopy, Written bool
}

// skillDescriptionRead is how much of a SKILL.md is read for its
// description: the front matter is at the top.
const skillDescriptionRead = 8192

// SkillsPageFor reads the Skills page for the tab status (active or
// archived) at now. Every live library skill is on one of the two tabs,
// whatever made it, so the page shows everything machines are sent.
func (s *Store) SkillsPageFor(uid int64, status string, now time.Time) (SkillsPage, error) {
	page := SkillsPage{Skills: []SkillPageRow{}, NewOnMachines: []NewSkill{}}
	if !validLibraryStatus(status) {
		return page, ErrPrivateSkillInvalid
	}
	rows, err := s.db.Query(`SELECT i.id,i.name,i.status,i.origin,i.origin_ref,i.revision_id<>'',i.upgraded_from,
 COALESCE((SELECT a.reference FROM library_aliases a JOIN shelf sh ON sh.user_id=a.user_id AND sh.skill_id=a.reference JOIN skills k ON k.id=a.reference WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind='shelf' ORDER BY k.hidden,k.source_type='local',a.reference LIMIT 1),''),
 COALESCE((SELECT h.slug FROM library_aliases a JOIN hosted_skills h ON h.user_id=a.user_id AND h.slug=a.reference WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind='hosted' ORDER BY h.slug LIMIT 1),''),
 EXISTS(SELECT 1 FROM library_aliases a JOIN hosted_skills h ON h.user_id=a.user_id AND h.slug=a.reference WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind='hosted' AND h.published=1),
 COALESCE((SELECT h.description FROM library_aliases a JOIN hosted_skills h ON h.user_id=a.user_id AND h.slug=a.reference WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind='hosted' ORDER BY h.slug LIMIT 1),''),
 EXISTS(SELECT 1 FROM library_aliases a JOIN skills k ON k.id=a.reference WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind='shelf' AND k.source<>'' AND k.source_type<>'local'),
 EXISTS(SELECT 1 FROM library_aliases a LEFT JOIN skills k ON k.id=a.reference WHERE a.user_id=i.user_id AND a.item_id=i.id AND (a.kind='machine_capture' OR (a.kind='shelf' AND k.source_type='local')))
 FROM library_items i WHERE i.user_id=? AND i.item_type=? AND i.deleted_at=0 AND `+privateSkillAccount+`
 ORDER BY lower(i.name),i.id`, uid, LibrarySkill, uid)
	if err != nil {
		return page, err
	}
	var all []SkillPageRow
	for rows.Next() {
		var r SkillPageRow
		var origin, originRef, hostedDescription string
		var linked, fromMachine bool
		if err = rows.Scan(&r.ID, &r.Name, &r.Status, &origin, &originRef, &r.HasCopy, &r.UpgradedFrom, &r.ShelfID, &r.HostedSlug, &r.Published, &hostedDescription, &linked, &fromMachine); err != nil {
			break
		}
		r.OriginKind = skillOriginKind(origin, r.HostedSlug != "", linked, fromMachine)
		r.Yours = r.OriginKind == "written"
		r.Origin = skillOriginDisplay(r.OriginKind, originRef)
		r.Description = hostedDescription
		r.Tags, r.Lists = []Tag{}, []ListRef{}
		all = append(all, r)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return page, err
	}
	for _, r := range all {
		switch r.Status {
		case SkillActive:
			page.Counts.Active++
		case SkillArchived:
			page.Counts.Archived++
		}
		if r.Status == status {
			page.Skills = append(page.Skills, r)
		}
	}
	if err = s.attachSkillPageShelf(uid, page.Skills); err != nil {
		return page, err
	}
	if err = s.attachSkillPageDescriptions(uid, page.Skills); err != nil {
		return page, err
	}
	health, err := s.SkillHealthFor(uid, now)
	if err != nil {
		return page, err
	}
	backups, err := s.SkillBackupsFor(uid, "", now)
	if err != nil {
		return page, err
	}
	// Use this version takes a machine's newest backup of the skill and
	// refuses one outside the backups folders, so the page offers the newest
	// of those that the controller would take.
	type offer struct {
		on     SkillEditedOn
		backup string
	}
	edited, newest := map[string]offer{}, map[string]bool{}
	for _, b := range backups { // newest first
		name := strings.ToLower(b.Name)
		if newest[name+"\x00"+b.AgentID] {
			continue
		}
		newest[name+"\x00"+b.AgentID] = true
		if _, ok := managedBackupPath(b.Backup); !ok {
			continue
		}
		if _, ok := edited[name]; !ok {
			edited[name] = offer{SkillEditedOn{AgentID: b.AgentID, Host: b.Host}, b.Backup}
		}
	}
	for i := range page.Skills {
		r := &page.Skills[i]
		h, ok := health[r.ID]
		if !ok {
			h = SkillHealth{State: "off", Hosts: []string{}, Machines: []MachineSkillState{}}
		}
		r.Health = h
		if o, ok := edited[strings.ToLower(r.Name)]; ok {
			on := o.on
			r.EditedOn = &on
			if r.EditedOnProblem, err = s.useVersionProblem(uid, r.ID, on, o.backup); err != nil {
				return page, err
			}
		}
	}
	if page.NewOnMachines, err = s.NewOnMachinesFor(uid); err != nil {
		return page, err
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM sync_agents WHERE user_id=? AND paused=0 AND skills_protocol=?`, uid, CurrentSkillsProtocol).Scan(&page.Machines); err != nil {
		return page, err
	}
	page.Notice, err = s.SkillUpgradeNoticeFor(uid)
	return page, err
}

// useVersionProblem says why the last Use this version of item from the
// backup on.AgentID took could not bring it in, or "" when it did not try or
// has not failed: the backup holds a file reet will not copy, or the machine
// could not send it.
func (s *Store) useVersionProblem(uid int64, item string, on SkillEditedOn, backup string) (string, error) {
	var raw string
	err := s.db.QueryRow(`SELECT data FROM machine_import_requests WHERE user_id=? AND agent_id=? AND json_extract(data,'$.useVersionItem')=? AND json_extract(data,'$.backupPath')=? ORDER BY created_at DESC,rowid DESC LIMIT 1`, uid, on.AgentID, item, backup).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var r RemoteImport
	if err = json.Unmarshal([]byte(raw), &r); err != nil {
		return "", err
	}
	if activeImport(r.State) && r.ExpiresAt < now() {
		r.State = "expired"
	}
	switch r.State {
	case "reviewing":
		if r.Preview == nil {
			return "", nil
		}
		for _, f := range r.Preview.Files {
			if f.Blocked != "" {
				return fmt.Sprintf("The edited copy on %s has a file reet won't copy: %s (%s).", on.Host, f.Path, skillimport.BlockDescription(f.Blocked)), nil
			}
		}
		if !r.Preview.SkillValid {
			return fmt.Sprintf("The edited copy on %s has no valid SKILL.md.", on.Host), nil
		}
	case "failed":
		why := map[string]string{
			"capture_failed":         "reet couldn't read it",
			"scope_unavailable":      "it isn't in that machine's skills backups",
			"source_changed":         "it changed while it was being read",
			"transfer_failed":        "sending it failed",
			"invalid_skill_manifest": "its SKILL.md isn't valid",
		}[r.Error]
		if why == "" {
			why = "it could not be read"
		}
		return fmt.Sprintf("%s couldn't send its edited copy: %s.", on.Host, why), nil
	case "expired":
		return fmt.Sprintf("%s didn't send its edited copy in time.", on.Host), nil
	}
	return "", nil
}

// skillOriginKind names how a skill arrived. An item from before origins were
// recorded is read from what refers to it: a written skill, a link, or a
// machine's copy; one with none of those had its content put into reet
// directly, which is the person's own.
func skillOriginKind(origin string, hosted, linked, fromMachine bool) string {
	switch origin {
	case "written", "imported":
		return origin
	case "machine":
		return "kept"
	}
	switch {
	case hosted:
		return "written"
	case linked:
		return "imported"
	case fromMachine:
		return "kept"
	}
	return "written"
}

// skillOriginDisplay is where a skill came from, as the page shows it: an
// imported skill's link, or the machine a kept one came from. origin_ref
// stores a machine as machine:{host}:{folder}.
func skillOriginDisplay(kind, ref string) string {
	switch kind {
	case "imported":
		return ref
	case "kept":
		rest, ok := strings.CutPrefix(ref, "machine:")
		if !ok {
			return ""
		}
		if i := strings.Index(rest, ":~/"); i >= 0 {
			rest = rest[:i]
		}
		return rest
	}
	return ""
}

// attachSkillPageShelf fills each row's likes, notes, description, tags and
// lists from the shelf row it names.
func (s *Store) attachSkillPageShelf(uid int64, rows []SkillPageRow) error {
	ids := []any{uid}
	for _, r := range rows {
		if r.ShelfID != "" {
			ids = append(ids, r.ShelfID)
		}
	}
	if len(ids) == 1 {
		return nil
	}
	q, err := s.db.Query(`SELECT `+shelfCols+` FROM skills s JOIN shelf sh ON sh.skill_id=s.id WHERE sh.user_id=? AND s.id IN (`+placeholders(len(ids)-1)+`)`, ids...)
	if err != nil {
		return err
	}
	reader := newSkillRowScanner(false)
	var shelf []Skill
	for q.Next() {
		k, e := reader.scan(q)
		if e != nil {
			q.Close()
			return e
		}
		shelf = append(shelf, k)
	}
	err = q.Err()
	q.Close()
	if err != nil {
		return err
	}
	if err = s.attachMetaForMode(uid, shelf, attachMode{compact: true}); err != nil {
		return err
	}
	byID := make(map[string]Skill, len(shelf))
	for _, k := range shelf {
		byID[k.ID] = k
	}
	for i := range rows {
		k, ok := byID[rows[i].ShelfID]
		if !ok {
			continue
		}
		rows[i].Liked, rows[i].Notes = k.Liked, k.Notes
		if rows[i].Description == "" {
			rows[i].Description = k.Description
		}
		if k.Tags != nil {
			rows[i].Tags = k.Tags
		}
		if k.Lists != nil {
			rows[i].Lists = k.Lists
		}
	}
	return nil
}

// attachSkillPageDescriptions reads the description of each row that has
// none yet from its SKILL.md in reet's copy.
func (s *Store) attachSkillPageDescriptions(uid int64, rows []SkillPageRow) error {
	args := []any{uid, LibrarySkill}
	index := map[string]int{}
	for i, r := range rows {
		if r.Description == "" && r.HasCopy {
			index[r.ID] = i
			args = append(args, r.ID)
		}
	}
	if len(index) == 0 {
		return nil
	}
	q, err := s.db.Query(`SELECT i.id,substr(b.data,1,?) FROM library_items i
 JOIN private_skill_revision_files f ON f.user_id=i.user_id AND f.bundle_id=i.bundle_id AND f.revision_id=i.revision_id AND f.path='SKILL.md'
 JOIN private_skill_blobs b ON b.user_id=f.user_id AND b.digest=f.digest
 WHERE i.user_id=? AND i.item_type=? AND i.id IN (`+placeholders(len(index))+`)`, append([]any{skillDescriptionRead}, args...)...)
	if err != nil {
		return err
	}
	defer q.Close()
	for q.Next() {
		var id string
		var md []byte
		if err = q.Scan(&id, &md); err != nil {
			return err
		}
		if i, ok := index[id]; ok {
			rows[i].Description = frontMatterDescription(md)
		}
	}
	return q.Err()
}

// frontMatterDescription is the description in a SKILL.md's front matter, or
// "" when there is none to read.
func frontMatterDescription(md []byte) string {
	text := strings.ReplaceAll(string(md), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return ""
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return ""
	}
	var header struct {
		Description string `yaml:"description"`
	}
	if yaml.Unmarshal([]byte(text[4:4+end]), &header) != nil || !utf8.ValidString(header.Description) {
		return ""
	}
	return strings.TrimSpace(header.Description)
}

// SkillItemsFor reads the library skills ids name, in the order given. It
// returns ErrPrivateSkillMissing when one is not a live skill of the account.
func (s *Store) SkillItemsFor(uid int64, ids []string) ([]SkillItemRef, error) {
	out := make([]SkillItemRef, 0, len(ids))
	for _, id := range ids {
		var r SkillItemRef
		err := s.db.QueryRow(`SELECT id,name,status,revision_id<>'',origin='written' FROM library_items WHERE user_id=? AND id=? AND item_type=? AND deleted_at=0 AND `+privateSkillAccount, uid, id, LibrarySkill, uid).Scan(&r.ID, &r.Name, &r.Status, &r.HasCopy, &r.Written)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPrivateSkillMissing
		}
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// DeletedSkillItemsFor checks that every one of ids is a skill of the
// account, deleted or not. It returns ErrPrivateSkillMissing otherwise.
func (s *Store) DeletedSkillItemsFor(uid int64, ids []string) error {
	for _, id := range ids {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM library_items WHERE user_id=? AND id=? AND item_type=?`, uid, id, LibrarySkill).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return ErrPrivateSkillMissing
		}
	}
	return nil
}
