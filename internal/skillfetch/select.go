package skillfetch

import (
	"path"
	"strings"
)

type held struct {
	data []byte
	exec bool
}

// normalize compares names loosely: case, spaces and hyphens do not matter.
func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(s)), "-"))
}

func frontMatterName(data []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if v, ok := strings.CutPrefix(l, "name:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// matchesSlug reports whether the folder dir is plausibly the skill slug, by
// the rules that need only its name; the early stop for an oversized skill uses it.
func matchesSlug(dir, slug string) bool {
	return dir == "skills/"+slug || normalize(path.Base(dir)) == normalize(slug)
}

// pickFolder chooses which of the folders holding a SKILL.md is the skill slug.
// A folder matches by, in order of preference: skills/<slug>; its name being
// exactly the slug; the name in its SKILL.md front matter; its name loosely.
// Among folders of the same preference the shallowest wins, then the first by
// name. all holds the retained files by path, for the front matter.
func pickFolder(skillDirs map[string]bool, all map[string]held, slug string) (string, bool) {
	norm := normalize(slug)
	best, bestRank := "", 4
	for dir := range skillDirs {
		rank := 4
		switch {
		case dir == "skills/"+slug:
			rank = 0
		case dir != "" && path.Base(dir) == slug:
			rank = 1
		case normalize(frontMatterName(all[join(dir, "SKILL.md")].data)) == norm:
			rank = 2
		case dir != "" && normalize(path.Base(dir)) == norm:
			rank = 3
		}
		if rank == 4 {
			continue
		}
		shallower := strings.Count(dir, "/") < strings.Count(best, "/")
		if rank < bestRank || (rank == bestRank && (shallower || (strings.Count(dir, "/") == strings.Count(best, "/") && dir < best))) {
			best, bestRank = dir, rank
		}
	}
	return best, bestRank != 4
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
