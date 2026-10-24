package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/ref"
	"gopkg.in/yaml.v3"
)

var ErrNamespaceTaken = errors.New("that username is already taken")
var ErrNamespaceSet = errors.New("your publishing username is already set")
var ErrHostedNameTaken = errors.New("you already have a skill with that name")

// DefaultHostedPrefix keeps reet-hosted names recognisable next to skills
// imported from elsewhere. An empty prefix turns the behaviour off.
const DefaultHostedPrefix = "reet-"

var hostedName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// HostedSkill is an author's skill bundle. Published bundles are public.
type HostedSkill struct {
	Slug        string            `json:"slug"`
	Description string            `json:"description"`
	Content     string            `json:"content"`
	Files       map[string][]byte `json:"files"`
	Published   bool              `json:"published"`
	CreatedAt   int64             `json:"createdAt"`
	UpdatedAt   int64             `json:"updatedAt"`
}

// ValidHostedName follows the Agent Skills name format.
func ValidHostedName(name string) bool {
	return len(name) > 0 && len(name) <= 64 && hostedName.MatchString(name)
}

// NormalizeHostedPrefix accepts a profile prefix such as "acme" or "acme-"
// and stores it with a single trailing hyphen. An empty result disables the
// prefix entirely.
func NormalizeHostedPrefix(raw string) (string, error) {
	prefix := strings.TrimRight(strings.ToLower(strings.TrimSpace(raw)), "-")
	if prefix == "" {
		return "", nil
	}
	if len(prefix) > 32 {
		return "", errors.New("the skill name prefix must be 32 characters or fewer")
	}
	if !hostedName.MatchString(prefix) {
		return "", errors.New("the skill name prefix needs lowercase letters, numbers and single hyphens")
	}
	return prefix + "-", nil
}

func ReservedUsername(name string) bool {
	switch name {
	case "api", "admin", "account", "hosting", "assets", "lists", "l", "shared", "spaces", "community", "join", "quick", "healthz", "skills", "www", "support":
		return true
	}
	return false
}

func (s *Store) PublishingUsername(userID int64) (string, error) {
	var name string
	err := s.db.QueryRow(`SELECT username FROM publishers WHERE user_id=?`, userID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// ClaimPublishingUsername is immutable so another account cannot take over old install URLs.
func (s *Store) ClaimPublishingUsername(userID int64, name string) error {
	if !ValidHostedName(name) || ReservedUsername(name) {
		return errors.New("choose a username of lowercase letters, numbers and single hyphens (up to 64 characters)")
	}
	return s.withTx(func(tx *sql.Tx) error {
		var existing string
		err := tx.QueryRow(`SELECT username FROM publishers WHERE user_id=?`, userID).Scan(&existing)
		if err == nil {
			if existing == name {
				return nil
			}
			return ErrNamespaceSet
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var taken int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM publishers WHERE username=?`, name).Scan(&taken); err != nil {
			return err
		}
		if taken > 0 {
			return ErrNamespaceTaken
		}
		_, err = tx.Exec(`INSERT INTO publishers(username,user_id) VALUES(?,?)`, name, userID)
		return err
	})
}

// ValidateHostedSkill parses real YAML frontmatter and bounds the entire bundle.
func ValidateHostedSkill(sk *HostedSkill) error {
	if !ValidHostedName(sk.Slug) {
		return errors.New("skill names must use lowercase letters, numbers and single hyphens (up to 64 characters)")
	}
	md := strings.ReplaceAll(sk.Content, "\r\n", "\n")
	if !utf8.ValidString(md) || !strings.HasPrefix(md, "---\n") {
		return errors.New("SKILL.md must start with YAML frontmatter containing name and description")
	}
	end := strings.Index(md[4:], "\n---\n")
	if end < 0 {
		return errors.New("SKILL.md needs a closing frontmatter delimiter on its own line")
	}
	var header struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(md[4:4+end]), &header); err != nil {
		return errors.New("invalid SKILL.md YAML frontmatter")
	}
	if header.Name != sk.Slug {
		return errors.New("the SKILL.md name must match the skill name in the URL")
	}
	header.Description = strings.TrimSpace(header.Description)
	if header.Description == "" || len(header.Description) > 1024 {
		return errors.New("the SKILL.md description must contain 1–1024 bytes")
	}
	if strings.TrimSpace(md[4+end+5:]) == "" {
		return errors.New("add instructions below the SKILL.md frontmatter")
	}
	if len(sk.Files) > 100 {
		return errors.New("a skill can have at most 100 supporting files")
	}
	size := len(md)
	for name, content := range sk.Files {
		if !utf8.ValidString(name) || len(name) > 240 || path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\\x00\r\n?#%") || strings.Contains(name, "..") || strings.EqualFold(name, "SKILL.md") || name == "." || strings.HasPrefix(name, ".well-known/") {
			return fmt.Errorf("invalid supporting file path %q", name)
		}
		for _, part := range strings.Split(name, "/") {
			if part == "" || strings.HasPrefix(part, ".") {
				return fmt.Errorf("invalid supporting file path %q", name)
			}
		}
		size += len(content)
	}
	if size > 2<<20 {
		return errors.New("a skill bundle must be 2 MiB or smaller")
	}
	sk.Content, sk.Description = md, header.Description
	if sk.Files == nil {
		sk.Files = map[string][]byte{}
	}
	return nil
}

func scanHosted(row interface{ Scan(...any) error }) (HostedSkill, error) {
	var sk HostedSkill
	var files string
	err := row.Scan(&sk.Slug, &sk.Description, &sk.Content, &files, &sk.Published, &sk.CreatedAt, &sk.UpdatedAt)
	if err != nil {
		return sk, err
	}
	err = json.Unmarshal([]byte(files), &sk.Files)
	return sk, err
}

const hostedCols = `slug,description,content,files,published,created_at,updated_at`

func (s *Store) HostedSkillsFor(userID int64) ([]HostedSkill, error) {
	rows, err := s.db.Query(`SELECT `+hostedCols+` FROM hosted_skills WHERE user_id=? ORDER BY slug`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HostedSkill{}
	for rows.Next() {
		sk, err := scanHosted(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (s *Store) SaveHostedSkill(userID int64, sk HostedSkill) (HostedSkill, error) {
	if err := ValidateHostedSkill(&sk); err != nil {
		return sk, err
	}
	username, err := s.PublishingUsername(userID)
	if err != nil {
		return sk, err
	}
	if username == "" {
		return sk, errors.New("claim a publishing username first")
	}
	files, err := json.Marshal(sk.Files)
	if err != nil {
		return sk, err
	}
	at := now()
	_, err = s.db.Exec(`INSERT INTO hosted_skills(user_id,slug,description,content,files,published,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)
 ON CONFLICT(user_id,slug) DO UPDATE SET description=excluded.description,content=excluded.content,files=excluded.files,published=excluded.published,updated_at=excluded.updated_at`, userID, sk.Slug, sk.Description, sk.Content, string(files), sk.Published, at, at)
	if err != nil {
		return sk, err
	}
	return scanHosted(s.db.QueryRow(`SELECT `+hostedCols+` FROM hosted_skills WHERE user_id=? AND slug=?`, userID, sk.Slug))
}

func (s *Store) DeleteHostedSkill(userID int64, slug string) error {
	result, err := s.db.Exec(`DELETE FROM hosted_skills WHERE user_id=? AND slug=?`, userID, slug)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) HostedSkillExists(userID int64, slug string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM hosted_skills WHERE user_id=? AND slug=?`, userID, slug).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

var hostedNameLine = regexp.MustCompile(`(?m)^name\s*:.*$`)

func renameHostedContent(content, name string) string {
	md := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(md, "---\n") {
		return md
	}
	end := strings.Index(md[4:], "\n---\n")
	if end < 0 {
		return md
	}
	header := md[4 : 4+end]
	if hostedNameLine.MatchString(header) {
		header = hostedNameLine.ReplaceAllString(header, "name: "+name)
	} else {
		header = "name: " + name + "\n" + header
	}
	return "---\n" + header + "\n---\n" + md[4+end+5:]
}

// RenameHostedSkill changes a hosted skill's slug and keeps the frontmatter
// name in sync, so the bundle remains valid after the URL changes.
func (s *Store) RenameHostedSkill(userID int64, slug, newSlug string, oldRef, newRef ref.Ref) (HostedSkill, error) {
	if !ValidHostedName(newSlug) {
		return HostedSkill{}, errors.New("skill names must use lowercase letters, numbers and single hyphens (up to 64 characters)")
	}
	var renamed HostedSkill
	err := s.withTx(func(tx *sql.Tx) error {
		old, err := scanHosted(tx.QueryRow(`SELECT `+hostedCols+` FROM hosted_skills WHERE user_id=? AND slug=?`, userID, slug))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if slug == newSlug {
			renamed = old
			return nil
		}
		var one int
		err = tx.QueryRow(`SELECT 1 FROM hosted_skills WHERE user_id=? AND slug=?`, userID, newSlug).Scan(&one)
		if err == nil {
			return ErrHostedNameTaken
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		files, err := json.Marshal(old.Files)
		if err != nil {
			return err
		}
		content := renameHostedContent(old.Content, newSlug)
		at := now()
		if _, err := tx.Exec(`INSERT INTO hosted_skills(user_id,slug,description,content,files,published,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
			userID, newSlug, old.Description, content, string(files), old.Published, old.CreatedAt, at); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM hosted_skills WHERE user_id=? AND slug=?`, userID, slug); err != nil {
			return err
		}
		if err := moveHostedReferencesTx(tx, oldRef, newRef); err != nil {
			return err
		}
		renamed = HostedSkill{Slug: newSlug, Description: old.Description, Content: content, Files: old.Files, Published: old.Published, CreatedAt: old.CreatedAt, UpdatedAt: at}
		return nil
	})
	return renamed, err
}

func moveHostedReferencesTx(tx *sql.Tx, oldRef, newRef ref.Ref) error {
	if oldRef.SourceType != "hosted" || newRef.SourceType != "hosted" || oldRef.ID == newRef.ID {
		return nil
	}
	var one int
	err := tx.QueryRow(`SELECT 1 FROM skills WHERE id=?`, oldRef.ID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO skills(`+cols+`)
			SELECT ?,?,?,?,?,?,description,notes,installs,installs_week,trend,liked,bookmarked,used,created_at,?
			FROM skills WHERE id=?
			ON CONFLICT(id) DO UPDATE SET source=excluded.source, slug=excluded.slug, source_type=excluded.source_type, url=excluded.url`,
		newRef.ID, newRef.Source, newRef.Slug, newRef.SourceType, newRef.URL, newRef.Slug, now(), oldRef.ID); err != nil {
		return err
	}
	for _, stmt := range []string{
		`INSERT OR IGNORE INTO shelf(user_id,skill_id,liked,bookmarked,used,notes,created_at,updated_at)
			SELECT user_id,?,liked,bookmarked,used,notes,created_at,updated_at FROM shelf WHERE skill_id=?`,
		`INSERT OR IGNORE INTO skill_tags(user_id,skill_id,tag_id,source,hidden)
			SELECT user_id,?,tag_id,source,hidden FROM skill_tags WHERE skill_id=?`,
		`INSERT OR IGNORE INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path)
			SELECT ?,env_id,installed_at,last_seen,source_path FROM skill_envs WHERE skill_id=?`,
		`INSERT OR IGNORE INTO list_skills(list_id,skill_id,note,added_by,position,added_at)
			SELECT list_id,?,note,added_by,position,added_at FROM list_skills WHERE skill_id=?`,
		`INSERT OR IGNORE INTO topic_skills(tag_id,skill_id)
			SELECT tag_id,? FROM topic_skills WHERE skill_id=?`,
		`INSERT OR IGNORE INTO ignored(user_id,id,name,reason,at)
			SELECT user_id,?,name,reason,at FROM ignored WHERE id=?`,
	} {
		if _, err := tx.Exec(stmt, newRef.ID, oldRef.ID); err != nil {
			return err
		}
	}
	for _, table := range []string{`shelf`, `skill_tags`, `skill_envs`, `list_skills`, `topic_skills`} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE skill_id=?`, oldRef.ID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM ignored WHERE id=?`, oldRef.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE events SET skill_id=? WHERE skill_id=?`, newRef.ID, oldRef.ID); err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM skills WHERE id=?`, oldRef.ID)
	return err
}

func (s *Store) PublishedHostedSkill(username, slug string) (HostedSkill, error) {
	sk, err := scanHosted(s.db.QueryRow(`SELECT h.slug,h.description,h.content,h.files,h.published,h.created_at,h.updated_at FROM hosted_skills h JOIN publishers p ON p.user_id=h.user_id JOIN users u ON u.id=p.user_id WHERE p.username=? AND h.slug=? AND h.published=1 AND u.disabled_at=0`, username, slug))
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return sk, err
}
