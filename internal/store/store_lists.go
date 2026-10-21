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
	ID          int64  `json:"id"`
	OwnerID     int64  `json:"ownerId"`
	OwnerName   string `json:"ownerName,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	Slug        string `json:"slug,omitempty"`
	Role        string `json:"role,omitempty"`
	SkillCount  int    `json:"skillCount"`
	MemberCount int    `json:"memberCount"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
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
	case "private", "link":
		return v, nil
	}
	return "", errors.New("visibility must be private, members or link")
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
	case visibility == "members":
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
	isAdmin := 0
	if userRole == "owner" || userRole == "admin" {
		isAdmin = 1
	}
	rows, err := s.db.Query(`SELECT l.id,l.owner_id,u.name,l.name,l.description,l.visibility,l.slug,
		(SELECT COUNT(*) FROM list_skills ls WHERE ls.list_id=l.id),
		1+(SELECT COUNT(*) FROM list_members m WHERE m.list_id=l.id),
		COALESCE((SELECT m.role FROM list_members m WHERE m.list_id=l.id AND m.user_id=?),''),
		l.created_at,l.updated_at
		FROM lists l JOIN users u ON u.id=l.owner_id
		WHERE l.owner_id=? OR EXISTS (SELECT 1 FROM list_members m WHERE m.list_id=l.id AND m.user_id=?)
			OR l.visibility='members' OR ?
		ORDER BY l.updated_at DESC, l.id DESC`, userID, userID, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SharedList{}
	for rows.Next() {
		var l SharedList
		var memberRole string
		if err := rows.Scan(&l.ID, &l.OwnerID, &l.OwnerName, &l.Name, &l.Description, &l.Visibility,
			&l.Slug, &l.SkillCount, &l.MemberCount, &memberRole, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		l.Role, _, _, _, _ = listAccessFor(userID, userRole, l.OwnerID, memberRole, l.Visibility)
		out = append(out, l)
	}
	return out, rows.Err()
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
	role, read, edit, manage, transfer := listAccessFor(userID, userRole, l.OwnerID, memberRole, l.Visibility)
	if !read {
		return ListAccess{}, ErrNotFound
	}
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
	for i := range skills {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(DISTINCT sh.user_id) FROM shelf sh
			WHERE sh.skill_id=? AND sh.used=1 AND sh.user_id IN (
				SELECT ? UNION SELECT user_id FROM list_members WHERE list_id=?)`,
			skills[i].ID, ownerID, listID).Scan(&n); err != nil {
			return err
		}
		skills[i].UseCount = n
		skills[i].MemberCount = memberCount
	}
	return nil
}

func (s *Store) listSkills(listID int64) ([]ListSkill, error) {
	rows, err := s.db.Query(`SELECT s.id,s.source,s.slug,s.source_type,s.url,s.name,s.description,'',
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
	var id int64
	var visibility string
	err := s.db.QueryRow(`SELECT id,visibility FROM lists WHERE slug=?`, slug).Scan(&id, &visibility)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && visibility != "link") {
		return ListDetail{}, ErrNotFound
	}
	if err != nil {
		return ListDetail{}, err
	}
	list, err := s.SharedListByID(id)
	if err != nil {
		return ListDetail{}, err
	}
	list.Role = "reader"
	list.OwnerName = ""
	d := ListDetail{SharedList: list}
	d.Skills, err = s.listSkills(id)
	if err != nil {
		return ListDetail{}, err
	}
	for i := range d.Skills {
		d.Skills[i].AddedBy = 0
		d.Skills[i].AddedByName = ""
	}
	if err := s.applyListAdoption(id, list.OwnerID, list.MemberCount, d.Skills); err != nil {
		return ListDetail{}, err
	}
	return d, nil
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
	isAdmin := 0
	if role == "owner" || role == "admin" {
		isAdmin = 1
	}
	args := []any{userID, userID, isAdmin}
	ids := make([]string, 0, len(skills))
	index := make(map[string][]int, len(skills))
	for i, k := range skills {
		ids = append(ids, k.ID)
		index[k.ID] = append(index[k.ID], i)
	}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(`SELECT ls.skill_id,l.id,l.name FROM list_skills ls JOIN lists l ON l.id=ls.list_id
		WHERE (l.owner_id=? OR EXISTS (SELECT 1 FROM list_members m WHERE m.list_id=l.id AND m.user_id=?)
			OR l.visibility='members' OR ?)
		AND ls.skill_id IN (`+placeholders(len(ids))+`) ORDER BY lower(l.name)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var skillID string
		var item ListRef
		if err := rows.Scan(&skillID, &item.ID, &item.Name); err != nil {
			return err
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
