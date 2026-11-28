package api

import (
	"embed"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
)

// builtinOwner is the namespace the reet-* skills are published under on skills.sh.
const builtinOwner = "eyupio"

//go:embed builtin_skills
var builtinSkills embed.FS

// builtinSkill returns a skill shipped in the binary, so /eyupio/<slug> resolves without a stored publication.
func builtinSkill(username, slug string) (store.HostedSkill, bool) {
	if username != builtinOwner || !store.ValidHostedName(slug) {
		return store.HostedSkill{}, false
	}
	b, err := builtinSkills.ReadFile("builtin_skills/" + slug + "/SKILL.md")
	if err != nil {
		return store.HostedSkill{}, false
	}
	content := string(b)
	desc := ""
	if front, _, ok := strings.Cut(strings.TrimPrefix(content, "---\n"), "\n---"); ok {
		for _, line := range strings.Split(front, "\n") {
			if v, found := strings.CutPrefix(line, "description:"); found {
				desc = strings.TrimSpace(v)
			}
		}
	}
	var files map[string][]byte
	if entries, err := builtinSkills.ReadDir("builtin_skills/" + slug); err == nil {
		for _, e := range entries {
			if e.IsDir() || e.Name() == "SKILL.md" {
				continue
			}
			if b, err := builtinSkills.ReadFile("builtin_skills/" + slug + "/" + e.Name()); err == nil {
				if files == nil {
					files = map[string][]byte{}
				}
				files[e.Name()] = b
			}
		}
	}
	return store.HostedSkill{Slug: slug, Description: desc, Content: content, Files: files, Published: true}, true
}
