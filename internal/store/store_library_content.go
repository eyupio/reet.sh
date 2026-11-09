package store

import (
	"bytes"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/setup"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

func validateTypedLibraryContent(in PrivateSkillWrite) (skillbundle.Manifest, error) {
	m, err := skillbundle.FromFiles(in.Files)
	if err != nil || len(in.Files) != 1 {
		return m, ErrPrivateSkillInvalid
	}
	file := in.Files[0]
	if skillimport.BlockedReason(skillbundle.ContentFile{Path: "portable.txt", Data: file.Data}) != "" {
		return m, ErrPrivateSkillInvalid
	}
	if file.Executable || !utf8.Valid(file.Data) || len(file.Data) > 1<<20 {
		return m, ErrPrivateSkillInvalid
	}
	switch in.ItemType {
	case LibraryAgent:
		if in.Application != "claude" && in.Application != "codex" && in.Application != "opencode" && in.Application != "agy" {
			return m, ErrPrivateSkillInvalid
		}
		switch in.Format {
		case "markdown":
			if path.Ext(file.Path) != ".md" || in.Application == "codex" {
				return m, ErrPrivateSkillInvalid
			}
			text := strings.ReplaceAll(string(file.Data), "\r\n", "\n")
			if strings.HasPrefix(text, "---\n") {
				end := strings.Index(text[4:], "\n---\n")
				if end < 0 {
					return m, ErrPrivateSkillInvalid
				}
				var header struct{ Name, Description string }
				if yaml.Unmarshal([]byte(text[4:4+end]), &header) != nil || (header.Name != "" && header.Name != in.Name) {
					return m, ErrPrivateSkillInvalid
				}
				if in.Application != "agy" && strings.TrimSpace(header.Description) == "" {
					return m, ErrPrivateSkillInvalid
				}
				text = text[4+end+5:]
			} else if in.Application != "agy" {
				return m, ErrPrivateSkillInvalid
			}
			if strings.TrimSpace(text) == "" || bytes.IndexByte(file.Data, 0) >= 0 {
				return m, ErrPrivateSkillInvalid
			}
		case "toml":
			if in.Application != "codex" || path.Ext(file.Path) != ".toml" {
				return m, ErrPrivateSkillInvalid
			}
			var config struct {
				Instructions string `toml:"developer_instructions"`
			}
			if toml.Unmarshal(file.Data, &config) != nil || strings.TrimSpace(config.Instructions) == "" {
				return m, ErrPrivateSkillInvalid
			}
		default:
			return m, ErrPrivateSkillInvalid
		}
	case LibrarySettings:
		if strings.TrimPrefix(path.Ext(file.Path), ".") != in.Format || setup.ValidatePortableSettings(in.Application, in.Format, path.Base(file.Path), file.Data) != nil {
			return m, ErrPrivateSkillInvalid
		}
	default:
		return m, ErrPrivateSkillInvalid
	}
	return m, nil
}
