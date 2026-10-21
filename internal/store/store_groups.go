package store

import (
	"database/sql"
	"errors"
	"strings"
)

type Group struct {
	ID       int64         `json:"id"`
	Name     string        `json:"name"`
	Kind     string        `json:"kind"`
	ParentID int64         `json:"parentId,omitempty"`
	OwnerID  int64         `json:"ownerId"`
	Role     string        `json:"role"`
	Members  []GroupMember `json:"members,omitempty"`
}
type GroupMember struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Store) GroupsFor(userID int64, userRole string) ([]Group, error) {
	admin := userRole == "owner" || userRole == "admin"
	rows, err := s.db.Query(`SELECT g.id,g.name,g.kind,COALESCE(g.parent_id,0),g.owner_id,CASE WHEN g.owner_id=? OR ? THEN 'owner' ELSE COALESCE(m.role,'') END FROM audience_groups g LEFT JOIN audience_group_members m ON m.group_id=g.id AND m.user_id=? WHERE g.owner_id=? OR m.user_id=? OR ? ORDER BY g.kind,g.name`, userID, admin, userID, userID, userID, admin)
	if err != nil {
		return nil, err
	}
	out := []Group{}
	for rows.Next() {
		var g Group
		if err = rows.Scan(&g.ID, &g.Name, &g.Kind, &g.ParentID, &g.OwnerID, &g.Role); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Role != "owner" && out[i].Role != "manager" {
			continue
		}
		out[i].Members, err = s.groupMembers(out[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (s *Store) GroupFor(userID int64, role string, id int64) (Group, error) {
	groups, err := s.GroupsFor(userID, role)
	if err != nil {
		return Group{}, err
	}
	for _, g := range groups {
		if g.ID == id {
			return g, nil
		}
	}
	return Group{}, ErrNotFound
}
func (s *Store) CreateGroup(userID int64, userRole, name, kind string, parentID int64) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return Group{}, errors.New("name must be 1 to 80 characters")
	}
	if kind != "team" && kind != "organisation" {
		return Group{}, errors.New("choose team or organisation")
	}
	if parentID != 0 {
		parent, err := s.GroupFor(userID, userRole, parentID)
		if err != nil || parent.Kind != "organisation" || (parent.Role != "owner" && parent.Role != "manager") || kind != "team" {
			return Group{}, errors.New("only an organisation manager can create a team inside it")
		}
	}
	var parent any
	if parentID != 0 {
		parent = parentID
	}
	res, err := s.db.Exec(`INSERT INTO audience_groups(name,kind,parent_id,owner_id) VALUES(?,?,?,?)`, name, kind, parent, userID)
	if err != nil {
		return Group{}, err
	}
	id, _ := res.LastInsertId()
	return s.GroupFor(userID, userRole, id)
}
func (s *Store) groupMembers(id int64) ([]GroupMember, error) {
	rows, err := s.db.Query(`SELECT u.id,u.name,u.email,CASE WHEN u.id=g.owner_id THEN 'owner' ELSE m.role END FROM audience_groups g JOIN users u ON u.id=g.owner_id LEFT JOIN audience_group_members m ON m.group_id=g.id AND m.user_id=u.id WHERE g.id=? UNION ALL SELECT u.id,u.name,u.email,m.role FROM audience_group_members m JOIN users u ON u.id=m.user_id JOIN audience_groups g ON g.id=m.group_id WHERE g.id=? AND u.id<>g.owner_id ORDER BY 2`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GroupMember{}
	for rows.Next() {
		var m GroupMember
		if err = rows.Scan(&m.ID, &m.Name, &m.Email, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) SetGroupMember(userID int64, userRole string, id int64, email, role string, remove bool) error {
	g, err := s.GroupFor(userID, userRole, id)
	if err != nil {
		return err
	}
	if g.Role != "owner" && g.Role != "manager" {
		return errors.New("only group managers can change membership")
	}
	u, ok, err := s.UserByEmail(normalizeEmail(email))
	if err != nil {
		return err
	}
	if !ok || u.DisabledAt != 0 {
		return errors.New("that person needs an active account first")
	}
	if u.ID == g.OwnerID {
		return errors.New("the group owner cannot be removed or demoted")
	}
	// Managers can manage membership, but only owners/admins can appoint managers.
	if role != "member" && role != "manager" {
		return errors.New("choose member or manager")
	}
	if g.Role != "owner" {
		var old string
		e := s.db.QueryRow(`SELECT role FROM audience_group_members WHERE group_id=? AND user_id=?`, id, u.ID).Scan(&old)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if old == "manager" || role == "manager" {
			return errors.New("only the group owner can change managers")
		}
	}
	if remove {
		_, err = s.db.Exec(`DELETE FROM audience_group_members WHERE group_id=? AND user_id=?`, id, u.ID)
	} else {
		_, err = s.db.Exec(`INSERT INTO audience_group_members(group_id,user_id,role) VALUES(?,?,?) ON CONFLICT(group_id,user_id) DO UPDATE SET role=excluded.role`, id, u.ID, role)
	}
	return err
}

// Audience is explicit: a group grants dynamic membership, people grant individual
// access, and exclusions override both. Owners/admins retain administrative access.
type AudiencePerson struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}
type Audience struct {
	GroupID   int64            `json:"groupId,omitempty"`
	GroupRole string           `json:"groupRole,omitempty"`
	People    []AudiencePerson `json:"people"`
	Excluded  []string         `json:"excluded"`
}
type audienceUser struct {
	id   int64
	role string
}

func (s *Store) validateAudience(userID int64, userRole, visibility string, a Audience) ([]audienceUser, []int64, error) {
	if visibility == "team" {
		if a.GroupID == 0 {
			return nil, nil, errors.New("choose a team or organisation")
		}
		if _, err := s.GroupFor(userID, userRole, a.GroupID); err != nil {
			return nil, nil, errors.New("choose a group you belong to")
		}
		if _, err := normalMemberRole(a.GroupRole); err != nil {
			return nil, nil, err
		}
	} else if a.GroupID != 0 {
		return nil, nil, errors.New("group access requires the team/organisation audience")
	}
	if (visibility == "public" || visibility == "link" || visibility == "members" || visibility == "personal") && len(a.Excluded) > 0 {
		return nil, nil, errors.New("exclusions require a private audience; public readers cannot be excluded")
	}
	if visibility == "personal" && len(a.People) > 0 {
		return nil, nil, errors.New("personal lists cannot grant access to other people")
	}
	if len(a.People) > 200 || len(a.Excluded) > 200 {
		return nil, nil, errors.New("choose at most 200 named people/exclusions")
	}
	people := []audienceUser{}
	exclusions := []int64{}
	seen := map[int64]bool{}
	for _, p := range a.People {
		u, ok, err := s.UserByEmail(normalizeEmail(p.Email))
		if err != nil {
			return nil, nil, err
		}
		if !ok || u.DisabledAt != 0 {
			return nil, nil, errors.New("each selected person needs an active account")
		}
		r, err := normalMemberRole(p.Role)
		if err != nil {
			return nil, nil, err
		}
		if u.Role == "viewer" && r == "editor" {
			return nil, nil, errors.New("a read-only account cannot be made an editor")
		}
		if seen[u.ID] {
			return nil, nil, errors.New("a person was selected more than once")
		}
		seen[u.ID] = true
		people = append(people, audienceUser{u.ID, r})
	}
	for _, email := range a.Excluded {
		u, ok, err := s.UserByEmail(normalizeEmail(email))
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, errors.New("each excluded person needs an account")
		}
		if u.ID == userID {
			return nil, nil, errors.New("you cannot exclude yourself")
		}
		exclusions = append(exclusions, u.ID)
	}
	return people, exclusions, nil
}
func (s *Store) AudienceFor(id int64) (Audience, error) {
	a := Audience{People: []AudiencePerson{}, Excluded: []string{}}
	err := s.db.QueryRow(`SELECT group_id,role FROM list_group_access WHERE list_id=?`, id).Scan(&a.GroupID, &a.GroupRole)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return a, err
	}
	rows, err := s.db.Query(`SELECT u.email,m.role FROM list_members m JOIN users u ON u.id=m.user_id WHERE m.list_id=? ORDER BY u.email`, id)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var p AudiencePerson
		if err = rows.Scan(&p.Email, &p.Role); err != nil {
			rows.Close()
			return a, err
		}
		a.People = append(a.People, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return a, err
	}
	rows, err = s.db.Query(`SELECT u.email FROM list_exclusions x JOIN users u ON u.id=x.user_id WHERE x.list_id=? ORDER BY u.email`, id)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var email string
		if err = rows.Scan(&email); err != nil {
			return a, err
		}
		a.Excluded = append(a.Excluded, email)
	}
	return a, rows.Err()
}
func writeAudience(tx *sql.Tx, id, ownerID int64, visibility string, a Audience, people []audienceUser, excluded []int64) error {
	for _, table := range []string{"list_members", "list_group_access", "list_exclusions"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE list_id=?`, id); err != nil {
			return err
		}
	}
	if visibility == "team" {
		role, err := normalMemberRole(a.GroupRole)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO list_group_access(list_id,group_id,role) VALUES(?,?,?)`, id, a.GroupID, role); err != nil {
			return err
		}
	}
	for _, p := range people {
		if p.id == ownerID {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO list_members(list_id,user_id,role) VALUES(?,?,?)`, id, p.id, p.role); err != nil {
			return err
		}
	}
	for _, userID := range excluded {
		if userID == ownerID {
			return errors.New("the list owner cannot be excluded")
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO list_exclusions(list_id,user_id) VALUES(?,?)`, id, userID); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) SaveAudienceList(userID int64, userRole string, id int64, name, description, visibility string, a Audience) (SharedList, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || len(name) > 80 || len(description) > 240 {
		return SharedList{}, errors.New("use a name up to 80 characters and description up to 240")
	}
	visibility, err := normalVisibility(visibility)
	if err != nil {
		return SharedList{}, err
	}
	ownerID := userID
	if id != 0 {
		access, e := s.ListAccessFor(userID, userRole, id)
		if e != nil {
			return SharedList{}, e
		}
		if !access.CanManage {
			return SharedList{}, errors.New("only the list owner or an admin can change its audience")
		}
		ownerID = access.List.OwnerID
	}
	people, excluded, err := s.validateAudience(userID, userRole, visibility, a)
	if err != nil {
		return SharedList{}, err
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if id == 0 {
			slug := newListSlug()
			if slug == "" {
				return errors.New("cannot generate list link")
			}
			res, e := tx.Exec(`INSERT INTO lists(owner_id,name,description,visibility,slug,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, userID, name, description, visibility, slug, now(), now())
			if e != nil {
				return e
			}
			id, e = res.LastInsertId()
			if e != nil {
				return e
			}
		} else {
			if _, e := tx.Exec(`UPDATE lists SET name=?,description=?,visibility=?,updated_at=? WHERE id=?`, name, description, visibility, now(), id); e != nil {
				return e
			}
		}
		return writeAudience(tx, id, ownerID, visibility, a, people, excluded)
	})
	if err != nil {
		return SharedList{}, err
	}
	return s.SharedListByID(id)
}

func (s *Store) listAudienceIDs(id, ownerID int64, visibility string) ([]int64, error) {
	if visibility == "personal" {
		return []int64{ownerID}, nil
	}
	rows, err := s.db.Query(`SELECT user_id FROM (
 SELECT ? AS user_id UNION SELECT user_id FROM list_members WHERE list_id=?
 UNION SELECT g.owner_id FROM list_group_access a JOIN audience_groups g ON g.id=a.group_id WHERE a.list_id=? AND ?='team'
 UNION SELECT m.user_id FROM list_group_access a JOIN audience_group_members m ON m.group_id=a.group_id WHERE a.list_id=? AND ?='team'
 ) AS recipients WHERE user_id=? OR NOT EXISTS(SELECT 1 FROM list_exclusions x WHERE x.list_id=? AND x.user_id=recipients.user_id)`, ownerID, id, id, visibility, id, visibility, ownerID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var userID int64
		if err = rows.Scan(&userID); err != nil {
			return nil, err
		}
		out = append(out, userID)
	}
	return out, rows.Err()
}
func (s *Store) Community() ([]SharedList, []ListSkill, error) {
	rows, err := s.db.Query(`SELECT slug FROM lists WHERE visibility='public' ORDER BY updated_at DESC,id DESC`)
	if err != nil {
		return nil, nil, err
	}
	slugs := []string{}
	for rows.Next() {
		var slug string
		if err = rows.Scan(&slug); err != nil {
			rows.Close()
			return nil, nil, err
		}
		slugs = append(slugs, slug)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	lists := []SharedList{}
	skills := []ListSkill{}
	seen := map[string]bool{}
	for _, slug := range slugs {
		d, e := s.PublicListFor(slug)
		if errors.Is(e, ErrNotFound) {
			continue
		}
		if e != nil {
			return nil, nil, e
		}
		lists = append(lists, d.SharedList)
		for _, skill := range d.Skills {
			if seen[skill.ID] {
				continue
			}
			seen[skill.ID] = true
			skill.Note = ""
			skills = append(skills, skill)
		}
	}
	return lists, skills, nil
}

func (s *Store) UpdateGroup(userID int64, userRole string, id int64, name string, remove bool) error {
	g, err := s.GroupFor(userID, userRole, id)
	if err != nil {
		return err
	}
	if g.Role != "owner" && (remove || g.Role != "manager") {
		return errors.New("only the group owner can delete it; managers may rename it")
	}
	if remove {
		var children int
		if err = s.db.QueryRow(`SELECT COUNT(*) FROM audience_groups WHERE parent_id=?`, id).Scan(&children); err != nil {
			return err
		}
		if children != 0 {
			return errors.New("delete the organisation's teams first; their lists will be retained")
		}
		_, err = s.db.Exec(`DELETE FROM audience_groups WHERE id=?`, id)
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return errors.New("name must be 1 to 80 characters")
	}
	_, err = s.db.Exec(`UPDATE audience_groups SET name=? WHERE id=?`, name, id)
	return err
}
