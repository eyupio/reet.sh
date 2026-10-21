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

	"gopkg.in/yaml.v3"
)

var ErrNamespaceTaken = errors.New("that username is already taken")
var ErrNamespaceSet = errors.New("your publishing username is already set")
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

func (s *Store) PublishedHostedSkill(username, slug string) (HostedSkill, error) {
	sk, err := scanHosted(s.db.QueryRow(`SELECT h.slug,h.description,h.content,h.files,h.published,h.created_at,h.updated_at FROM hosted_skills h JOIN publishers p ON p.user_id=h.user_id JOIN users u ON u.id=p.user_id WHERE p.username=? AND h.slug=? AND h.published=1 AND u.disabled_at=0`, username, slug))
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return sk, err
}
