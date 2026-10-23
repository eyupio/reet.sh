package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/eyupio/reet.sh/internal/ref"
)

// SharedList is one named set of skills a team keeps together.
type SharedList struct {
	ID           int64  `json:"id"`
	OwnerID      int64  `json:"ownerId"`
	OwnerName    string `json:"ownerName,omitempty"`
	SharedWithMe bool   `json:"sharedWithMe"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Visibility   string `json:"visibility"`
	Slug         string `json:"slug,omitempty"`
	Role         string `json:"role,omitempty"`
	SkillCount   int    `json:"skillCount"`
	MemberCount  int    `json:"memberCount"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

// ListSkill is a catalogue skill in a shared list, with the list's note.
type ListSkill struct {
	Skill
	Note        string `json:"note,omitempty"`
	AddedBy     int64  `json:"addedBy,omitempty"`
	AddedByName string `json:"addedByName,omitempty"`
	UseCount    int    `json:"useCount"`
	MemberCount int    `json:"memberCount"`
}

// ListMember is a person with access to a shared list. The owner is included.
type ListMember struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// ListDetail is one list with its skills and people.
type ListDetail struct {
	SharedList
	Skills      []ListSkill  `json:"skills"`
	Members     []ListMember `json:"members,omitempty"`
	LinkEnabled bool         `json:"linkEnabled"`
	CanEdit     bool         `json:"canEdit"`
	CanManage   bool         `json:"canManage"`
	CanTransfer bool         `json:"canTransfer"`
	Audience    *Audience    `json:"audience,omitempty"`
}

// ListAccess is the caller's standing on one list.
type ListAccess struct {
	List        SharedList
	CanEdit     bool
	CanManage   bool
	CanTransfer bool
}

func normalVisibility(v string) (string, error) {
	switch v {
	case "", "members":
		return "members", nil
	case "private", "link", "personal", "team", "public":
		return v, nil
	}
	return "", errors.New("choose personal, private, team, public, members (legacy) or link")
}

func normalMemberRole(r string) (string, error) {
	switch r {
	case "", "viewer":
		return "viewer", nil
	case "editor":
		return "editor", nil
	}
	return "", errors.New("role must be editor or viewer")
}

func listAccessFor(userID int64, userRole string, ownerID int64, memberRole, visibility string) (role string, read, edit, manage, transfer bool) {
	switch {
	case userID == ownerID:
		role, read, edit, manage, transfer = "owner", true, true, true, true
	case userRole == "owner":
		role, read, edit, manage, transfer = "admin", true, true, true, true
	case userRole == "admin":
		role, read, edit, manage, transfer = "admin", true, true, true, false
	case memberRole == "editor":
		role, read, edit = "editor", true, true
	case memberRole == "viewer":
		role, read = "viewer", true
	case visibility == "members" || visibility == "public":
		role, read = "reader", true
	}
	// An instance viewer can be given a list role, but the account itself stays read-only.
	if read && userRole == "viewer" {
		edit, manage, transfer = false, false, false
		if role == "editor" {
			role = "viewer"
		}
	}
	return
}

func newListSlug() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// CreateList makes a list owned by userID.
func (s *Store) CreateList(userID int64, name, description, visibility string) (SharedList, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || len(name) > 80 {
		return SharedList{}, errors.New("list name must be 1 to 80 characters")
	}
	if len(description) > 240 {
		return SharedList{}, errors.New("description is too long")
	}
	vis, err := normalVisibility(visibility)
	if err != nil {
		return SharedList{}, err
	}
	t := now()
	for attempt := 0; attempt < 3; attempt++ {
		slug := newListSlug()
		res, err := s.db.Exec(`INSERT INTO lists(owner_id,name,description,visibility,slug,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?)`, userID, name, description, vis, slug, t, t)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue
			}
			return SharedList{}, err
		}
		id, _ := res.LastInsertId()
		return s.SharedListByID(id)
	}
	return SharedList{}, errors.New("could not make a unique list link")
}

// SharedListByID reads a list without the caller's access check.
func (s *Store) SharedListByID(id int64) (SharedList, error) {
	return s.scanSharedList(s.db.QueryRow(listSummaryQuery+` WHERE l.id=?`, id))
}

const listSummaryQuery = `SELECT l.id,l.owner_id,u.name,l.name,l.description,l.visibility,l.slug,
	(SELECT COUNT(*) FROM list_skills ls WHERE ls.list_id=l.id),
	1+(SELECT COUNT(*) FROM list_members m WHERE m.list_id=l.id),l.created_at,l.updated_at
	FROM lists l JOIN users u ON u.id=l.owner_id`

func (s *Store) scanSharedList(row interface{ Scan(...any) error }) (SharedList, error) {
	var l SharedList
	err := row.Scan(&l.ID, &l.OwnerID, &l.OwnerName, &l.Name, &l.Description, &l.Visibility,
		&l.Slug, &l.SkillCount, &l.MemberCount, &l.CreatedAt, &l.UpdatedAt)
	return l, err
}

// ListListsFor returns the lists a person can see in the toolbar: their own, lists
// they are on, and every members-visible list. Admins see every list.
func (s *Store) ListListsFor(userID int64, userRole string) ([]SharedList, error) {
	rows, err := s.db.Query(`SELECT id FROM lists ORDER BY updated_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []SharedList{}
	for _, id := range ids {
		access, e := s.ListAccessFor(userID, userRole, id)
		if errors.Is(e, ErrNotFound) {
			continue
		}
		if e != nil {
			return nil, e
		}
		out = append(out, access.List)
	}
	return out, nil
}

// ListAccessFor checks what a person may do with one list.
func (s *Store) ListAccessFor(userID int64, userRole string, id int64) (ListAccess, error) {
	var l SharedList
	var memberRole string
	err := s.db.QueryRow(`SELECT l.id,l.owner_id,u.name,l.name,l.description,l.visibility,l.slug,
		(SELECT COUNT(*) FROM list_skills ls WHERE ls.list_id=l.id),
		1+(SELECT COUNT(*) FROM list_members m WHERE m.list_id=l.id),
		COALESCE((SELECT m.role FROM list_members m WHERE m.list_id=l.id AND m.user_id=?),''),
		l.created_at,l.updated_at
		FROM lists l JOIN users u ON u.id=l.owner_id WHERE l.id=?`, userID, id).
		Scan(&l.ID, &l.OwnerID, &l.OwnerName, &l.Name, &l.Description, &l.Visibility,
			&l.Slug, &l.SkillCount, &l.MemberCount, &memberRole, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ListAccess{}, ErrNotFound
	}
	if err != nil {
		return ListAccess{}, err
	}
	administrative := userID == l.OwnerID || userRole == "owner" || userRole == "admin"
	var excluded bool
	if err = s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM list_exclusions WHERE list_id=? AND user_id=?)`, id, userID).Scan(&excluded); err != nil {
		return ListAccess{}, err
	}
	if !administrative && (excluded || l.Visibility == "personal") {
		return ListAccess{}, ErrNotFound
	}
	inherited := false
	if l.Visibility == "team" {
		var groupRole string
		e := s.db.QueryRow(`SELECT a.role FROM list_group_access a JOIN audience_groups g ON g.id=a.group_id WHERE a.list_id=? AND (g.owner_id=? OR EXISTS(SELECT 1 FROM audience_group_members m WHERE m.group_id=g.id AND m.user_id=?))`, id, userID, userID).Scan(&groupRole)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return ListAccess{}, e
		}
		if e == nil && memberRole != "editor" {
			if memberRole == "" || groupRole == "editor" {
				memberRole = groupRole
				inherited = true
			}
		}
	}
	role, read, edit, manage, transfer := listAccessFor(userID, userRole, l.OwnerID, memberRole, l.Visibility)
	if inherited && role == "viewer" {
		role = "reader"
	}
	audienceIDs, e := s.listAudienceIDs(id, l.OwnerID, l.Visibility)
	if e != nil {
		return ListAccess{}, e
	}
	l.MemberCount = len(audienceIDs)
	if !read {
		return ListAccess{}, ErrNotFound
	}
	l.SharedWithMe = userID != l.OwnerID && memberRole != "" && !excluded
	l.Role = role
	return ListAccess{List: l, CanEdit: edit, CanManage: manage, CanTransfer: transfer}, nil
}

// ListDetailFor loads one list with its skills and members for a signed-in person.
func (s *Store) ListDetailFor(userID int64, userRole string, id int64) (ListDetail, error) {
	access, err := s.ListAccessFor(userID, userRole, id)
	if err != nil {
		return ListDetail{}, err
	}
	d := ListDetail{SharedList: access.List, CanEdit: access.CanEdit, CanManage: access.CanManage, CanTransfer: access.CanTransfer}
	if access.CanManage {
		a, e := s.AudienceFor(id)
		if e != nil {
			return ListDetail{}, e
		}
		d.Audience = &a
	}
	d.Skills, err = s.listSkills(id)
	if err != nil {
		return ListDetail{}, err
	}
	d.Members, err = s.listMembers(id)
	if err != nil {
		return ListDetail{}, err
	}
	if d.Role == "reader" {
		d.Members = nil
	}
	if err := s.applyListAdoption(id, access.List.OwnerID, access.List.MemberCount, d.Skills); err != nil {
		return ListDetail{}, err
	}
	return d, nil
}

func (s *Store) applyListAdoption(listID, ownerID int64, memberCount int, skills []ListSkill) error {
	list, err := s.SharedListByID(listID)
	if err != nil {
		return err
	}
	ids, err := s.listAudienceIDs(listID, ownerID, list.Visibility)
	if err != nil {
		return err
	}
	for i := range skills {
		args := []any{skills[i].ID}
		for _, id := range ids {
			args = append(args, id)
		}
		var count int
		if len(ids) > 0 {
			if err = s.db.QueryRow(`SELECT COUNT(*) FROM shelf WHERE skill_id=? AND used=1 AND user_id IN (`+placeholders(len(ids))+`)`, args...).Scan(&count); err != nil {
				return err
			}
		}
		skills[i].UseCount = count
		skills[i].MemberCount = len(ids)
	}
	return nil
}

func (s *Store) listSkills(listID int64) ([]ListSkill, error) { return listSkillsFrom(s.db, listID) }
func listSkillsFrom(query environmentQuerier, listID int64) ([]ListSkill, error) {
	rows, err := query.Query(`SELECT s.id,s.source,s.slug,s.source_type,s.url,s.name,s.description,'',
		s.installs,s.installs_week,s.trend,0,0,0,s.created_at,s.updated_at,
		ls.note,COALESCE(ls.added_by,0),COALESCE(u.name,'')
		FROM list_skills ls JOIN skills s ON s.id=ls.skill_id
		LEFT JOIN users u ON u.id=ls.added_by
		WHERE ls.list_id=? ORDER BY ls.position ASC, ls.added_at ASC, lower(s.name)`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ListSkill{}
	for rows.Next() {
		var k ListSkill
		var liked, bookmarked, used int
		var trend string
		if err := rows.Scan(&k.ID, &k.Source, &k.Slug, &k.SourceType, &k.URL, &k.Name, &k.Description, &k.Notes,
			&k.Installs, &k.InstallsWeek, &trend, &liked, &bookmarked, &used, &k.CreatedAt, &k.UpdatedAt,
			&k.Note, &k.AddedBy, &k.AddedByName); err != nil {
			return nil, err
		}
		if trend != "" {
			_ = json.Unmarshal([]byte(trend), &k.Trend)
		}
		k.Liked, k.Bookmarked, k.Used = liked != 0, bookmarked != 0, used != 0
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) listMembers(listID int64) ([]ListMember, error) {
	rows, err := s.db.Query(`SELECT u.id,u.name,u.email,CASE WHEN l.owner_id=u.id THEN 'owner' ELSE m.role END
		FROM lists l JOIN users u ON u.id=l.owner_id
		LEFT JOIN list_members m ON m.list_id=l.id AND m.user_id=u.id
		WHERE l.id=?
		UNION ALL
		SELECT u.id,u.name,u.email,m.role FROM list_members m JOIN users u ON u.id=m.user_id
		WHERE m.list_id=? AND u.id NOT IN (SELECT owner_id FROM lists WHERE id=?)`,
		listID, listID, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ListMember{}
	for rows.Next() {
		var m ListMember
		if err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rank := map[string]int{"owner": 0, "editor": 1, "viewer": 2}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Role] != rank[out[j].Role] {
			return rank[out[i].Role] < rank[out[j].Role]
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// UpdateList changes a list's name, description and visibility.
func (s *Store) UpdateList(id int64, name, description, visibility string) (SharedList, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || len(name) > 80 {
		return SharedList{}, errors.New("list name must be 1 to 80 characters")
	}
	if len(description) > 240 {
		return SharedList{}, errors.New("description is too long")
	}
	vis, err := normalVisibility(visibility)
	if err != nil {
		return SharedList{}, err
	}
	res, err := s.db.Exec(`UPDATE lists SET name=?,description=?,visibility=?,updated_at=? WHERE id=?`,
		name, description, vis, now(), id)
	if err != nil {
		return SharedList{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return SharedList{}, ErrNotFound
	}
	return s.SharedListByID(id)
}

func (s *Store) DeleteList(id int64) error {
	res, err := s.db.Exec(`DELETE FROM lists WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// EnsureSkill adds a skill to the shared catalogue without putting it on anyone's shelf.
func (s *Store) EnsureSkill(r ref.Ref) error {
	_, err := s.db.Exec(`INSERT INTO skills(`+cols+`) VALUES(?,?,?,?,?,?,'','',0,0,'',0,0,0,?,?)
		ON CONFLICT(id) DO NOTHING`, r.ID, r.Source, r.Slug, r.SourceType, r.URL, r.Slug, now(), now())
	return err
}

func (s *Store) AddListSkill(listID int64, skillID, note string, addedBy int64) error {
	note = strings.TrimSpace(note)
	if len(note) > 500 {
		return errors.New("list note is too long")
	}
	res, err := s.db.Exec(`INSERT INTO list_skills(list_id,skill_id,note,added_by,position,added_at)
		VALUES(?,?,?,?,COALESCE((SELECT MAX(position)+1 FROM list_skills WHERE list_id=?),0),?)
		ON CONFLICT(list_id,skill_id) DO UPDATE SET
			note=CASE WHEN excluded.note<>'' THEN excluded.note ELSE list_skills.note END,
			added_by=excluded.added_by`,
		listID, skillID, note, addedBy, listID, now())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.db.Exec(`UPDATE lists SET updated_at=? WHERE id=?`, now(), listID)
	return err
}

func (s *Store) UpdateListSkillNote(listID int64, skillID, note string) error {
	note = strings.TrimSpace(note)
	if len(note) > 500 {
		return errors.New("list note is too long")
	}
	res, err := s.db.Exec(`UPDATE list_skills SET note=? WHERE list_id=? AND skill_id=?`, note, listID, skillID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.db.Exec(`UPDATE lists SET updated_at=? WHERE id=?`, now(), listID)
	return err
}

func (s *Store) RemoveListSkill(listID int64, skillID string) error {
	res, err := s.db.Exec(`DELETE FROM list_skills WHERE list_id=? AND skill_id=?`, listID, skillID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.db.Exec(`UPDATE lists SET updated_at=? WHERE id=?`, now(), listID)
	return err
}

// AddListMember adds a person to a list, refusing a duplicate owner row.
func (s *Store) AddListMember(listID, userID int64, role string) error {
	list, e := s.SharedListByID(listID)
	if e != nil {
		return e
	}
	if list.Visibility == "personal" {
		return errors.New("change the audience to selected people before adding collaborators")
	}
	role, err := normalMemberRole(role)
	if err != nil {
		return err
	}
	var ownerID int64
	if err := s.db.QueryRow(`SELECT owner_id FROM lists WHERE id=?`, listID).Scan(&ownerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if ownerID == userID {
		return errors.New("the owner is already on the list")
	}
	if _, err := s.db.Exec(`INSERT INTO list_members(list_id,user_id,role) VALUES(?,?,?)
		ON CONFLICT(list_id,user_id) DO UPDATE SET role=excluded.role`, listID, userID, role); err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE lists SET updated_at=? WHERE id=?`, now(), listID)
	return err
}

func (s *Store) RemoveListMember(listID, userID int64) error {
	if _, err := s.db.Exec(`DELETE FROM list_members WHERE list_id=? AND user_id=?`, listID, userID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE lists SET updated_at=? WHERE id=?`, now(), listID)
	return err
}

// TransferList hands ownership to a current member and keeps the old owner as an editor.
func (s *Store) TransferList(listID, targetID int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		var ownerID int64
		if err := tx.QueryRow(`SELECT owner_id FROM lists WHERE id=?`, listID).Scan(&ownerID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if targetID == ownerID {
			return errors.New("that person already owns the list")
		}
		var one int
		if err := tx.QueryRow(`SELECT 1 FROM list_members WHERE list_id=? AND user_id=?`, listID, targetID).Scan(&one); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("pick someone who is already on the list")
			}
			return err
		}
		if _, err := tx.Exec(`DELETE FROM list_members WHERE list_id=? AND user_id=?`, listID, targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO list_members(list_id,user_id,role) VALUES(?,?,'editor')
			ON CONFLICT(list_id,user_id) DO UPDATE SET role='editor'`, listID, ownerID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE lists SET owner_id=?,updated_at=? WHERE id=?`, targetID, now(), listID)
		return err
	})
}

// AddListToShelf adds missing list skills to the shelf as To test and reports how many were added.
func (s *Store) AddListToShelf(userID, listID int64) (int, error) {
	rows, err := s.db.Query(`SELECT s.id,s.source,s.slug,s.source_type,s.url
		FROM list_skills ls JOIN skills s ON s.id=ls.skill_id WHERE ls.list_id=?`, listID)
	if err != nil {
		return 0, err
	}
	items := []ref.Ref{}
	for rows.Next() {
		var r ref.Ref
		if err := rows.Scan(&r.ID, &r.Source, &r.Slug, &r.SourceType, &r.URL); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	yes := true
	n := 0
	for _, it := range items {
		var exists int
		err := s.db.QueryRow(`SELECT 1 FROM shelf WHERE user_id=? AND skill_id=?`, userID, it.ID).Scan(&exists)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return n, err
		}
		if _, err := s.UpsertFor(userID, it, Patch{Bookmarked: &yes}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// PublicListFor reads a link-visible list without an account. It shows no members
// or emails, and no personal shelf state.
func (s *Store) PublicListFor(slug string) (ListDetail, error) {
	var d ListDetail
	err := s.withTx(func(tx *sql.Tx) error {
		list, err := s.scanSharedList(tx.QueryRow(listSummaryQuery+` WHERE l.slug=? AND l.visibility IN ('link','public')`, slug))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		list.Role = "reader"
		list.OwnerID = 0
		list.OwnerName = ""
		list.MemberCount = 0
		d = ListDetail{SharedList: list}
		d.Skills, err = listSkillsFrom(tx, list.ID)
		if err != nil {
			return err
		}
		for i := range d.Skills {
			d.Skills[i].AddedBy = 0
			d.Skills[i].AddedByName = ""
			d.Skills[i].UseCount = 0
			d.Skills[i].MemberCount = 0
		}
		return nil
	})
	return d, err
}

// attachListsFor labels catalogue cards with the shared lists the person can see.
func (s *Store) attachListsFor(userID int64, skills []Skill) error {
	if len(skills) == 0 {
		return nil
	}
	var role string
	if err := s.db.QueryRow(`SELECT role FROM users WHERE id=?`, userID).Scan(&role); err != nil {
		return err
	}
	lists, err := s.ListListsFor(userID, role)
	if err != nil {
		return err
	}
	allowed := map[int64]bool{}
	for _, l := range lists {
		allowed[l.ID] = true
	}
	args := []any{}
	index := map[string][]int{}
	for i, k := range skills {
		args = append(args, k.ID)
		index[k.ID] = append(index[k.ID], i)
	}
	rows, err := s.db.Query(`SELECT ls.skill_id,l.id,l.name FROM list_skills ls JOIN lists l ON l.id=ls.list_id WHERE ls.skill_id IN (`+placeholders(len(skills))+`) ORDER BY lower(l.name)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var skillID string
		var item ListRef
		if err = rows.Scan(&skillID, &item.ID, &item.Name); err != nil {
			return err
		}
		if !allowed[item.ID] {
			continue
		}
		for _, i := range index[skillID] {
			skills[i].Lists = append(skills[i].Lists, item)
		}
	}
	return rows.Err()
}

// NormalizeListEmail is used by handlers resolving an invitee or member.
func NormalizeListEmail(email string) string {
	return normalizeEmail(email)
}
