// Package ref parses skills.sh references into stable skill ids.
package ref

import (
	"errors"
	"net/url"
	"strings"
)

// Ref identifies one skill listed on skills.sh.
type Ref struct {
	ID, Source, Slug, SourceType, URL string
}

const base = "https://www.skills.sh/"

// Parse accepts a skills.sh URL or a bare slug (owner/repo/skill or domain/skill).
func Parse(input string) (Ref, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return Ref{}, errors.New("empty input")
	}
	var path string
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return Ref{}, errors.New("invalid URL")
		}
		if h := strings.ToLower(u.Hostname()); h != "skills.sh" && h != "www.skills.sh" {
			return Ref{}, errors.New("not a skills.sh URL")
		}
		path = u.Path
	} else {
		path = s
		for _, p := range []string{"www.skills.sh/", "skills.sh/"} {
			if strings.HasPrefix(strings.ToLower(path), p) {
				path = path[len(p):]
				break
			}
		}
		if i := strings.IndexAny(path, "?#"); i >= 0 {
			path = path[:i]
		}
	}
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	switch {
	case len(parts) >= 3 && parts[0] == "site":
		return wellKnown(parts[1], parts[2]), nil
	case len(parts) >= 3:
		src := parts[0] + "/" + parts[1]
		id := src + "/" + parts[2]
		return Ref{ID: id, Source: src, Slug: parts[2], SourceType: "github", URL: base + id}, nil
	case len(parts) == 2 && strings.Contains(parts[0], "."):
		return wellKnown(parts[0], parts[1]), nil
	}
	return Ref{}, errors.New("expected owner/repo/skill or domain/skill")
}

func wellKnown(domain, slug string) Ref {
	id := domain + "/" + slug
	return Ref{ID: id, Source: domain, Slug: slug, SourceType: "well-known", URL: base + "site/" + id}
}
