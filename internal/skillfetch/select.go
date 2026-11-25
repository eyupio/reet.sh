package skillfetch

import (
	"path"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type held struct {
	data []byte
	exec bool
}

// normalize compares names loosely: case, spaces and hyphens do not matter.
func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(s)), "-"))
}

// squash keeps only the letters and digits of s, in lower case. A catalogue
// slug drops punctuation that a written name keeps: "UI/UX Design Review" is
// uiux-design-review.
func squash(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// sameSquashed reports whether a and b have the same letters and digits, and
// have some.
func sameSquashed(a, b string) bool {
	a = squash(a)
	return a != "" && a == squash(b)
}

// SameName reports whether two skill names are the same name loosely, as a
// fetch matches a folder to a skill: case, spaces and punctuation aside.
func SameName(a, b string) bool { return normalize(a) == normalize(b) || sameSquashed(a, b) }

// SkillName is the name in the YAML front matter of a SKILL.md, or "" when it
// has none or the front matter cannot be read. A fetch picks a skill's folder
// by it and the copy checks the files against it, so both read it this way.
func SkillName(data []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for end, l := range lines[1:] {
		if strings.TrimSpace(l) != "---" {
			continue
		}
		var header struct {
			Name string `yaml:"name"`
		}
		if yaml.Unmarshal([]byte(strings.Join(lines[1:1+end], "\n")), &header) != nil {
			return ""
		}
		return header.Name
	}
	return ""
}

// pickFolder chooses which of the folders holding a SKILL.md is the skill slug.
// A folder matches by, in order of preference: skills/<slug>; its name being
// exactly the slug; the name in its SKILL.md front matter; its name loosely;
// the front-matter name with its punctuation dropped.
// Among folders of the same preference the shallowest wins, then the first by
// name. all holds the retained files by path, for the front matter.
func pickFolder(skillDirs map[string]bool, all map[string]held, slug string) (string, bool) {
	norm := normalize(slug)
	const none = 5
	best, bestRank := "", none
	for dir := range skillDirs {
		rank := none
		switch {
		case dir == "skills/"+slug:
			rank = 0
		case dir != "" && path.Base(dir) == slug:
			rank = 1
		default:
			// Only a folder the cheap matches left open is worth reading.
			name := SkillName(all[join(dir, "SKILL.md")].data)
			switch {
			case normalize(name) == norm:
				rank = 2
			case dir != "" && normalize(path.Base(dir)) == norm:
				rank = 3
			case sameSquashed(name, slug):
				rank = 4
			}
		}
		if rank == none {
			continue
		}
		shallower := strings.Count(dir, "/") < strings.Count(best, "/")
		if rank < bestRank || (rank == bestRank && (shallower || (strings.Count(dir, "/") == strings.Count(best, "/") && dir < best))) {
			best, bestRank = dir, rank
		}
	}
	return best, bestRank != none
}

func parent(d string) string {
	if p := path.Dir(d); p != "." {
		return p
	}
	return ""
}

func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}
