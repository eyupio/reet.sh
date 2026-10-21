// Package ref parses skills.sh references into stable skill ids.
package ref

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// Ref identifies one skill listed on skills.sh.
type Ref struct {
	ID, Source, Slug, SourceType, URL string
}

const base = "https://www.skills.sh/"

// Parse accepts a skills.sh URL, a bare slug, or an npx skills add command.
func Parse(input string) (Ref, error) { return ParseAt(input, "") }

// ParseAt also accepts skill URLs hosted on the current instance.
func ParseAt(input, host string) (Ref, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return Ref{}, errors.New("empty input")
	}
	if fields := strings.Fields(s); fields[0] == "npx" {
		return parseInstallCommand(fields, host)
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return Ref{}, errors.New("expected a skills.sh URL, slug, or npx skills add command")
	}
	var path string
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return Ref{}, errors.New("invalid URL")
		}
		if hosted, ok := hostedURL(u, host); ok {
			return hosted, nil
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

var commandPart = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]*$`)

// parseInstallCommand reads arguments only; pasted commands are never executed.
func parseInstallCommand(args []string, host string) (Ref, error) {
	invalid := errors.New("expected npx skills add <hosted skill URL> or npx skills add <GitHub URL or owner/repo> --skill <skill-name>")
	// Sources and skill names cannot contain spaces, so quoted arguments can be
	// handled without interpreting shell substitutions or escape sequences.
	for i, arg := range args {
		if len(arg) >= 2 && (arg[0] == '\'' || arg[0] == '"') && arg[len(arg)-1] == arg[0] {
			args[i] = arg[1 : len(arg)-1]
		}
	}
	args = args[1:]
	if len(args) > 0 && (args[0] == "-y" || args[0] == "--yes") {
		args = args[1:]
	}
	if len(args) < 3 || (args[0] != "skills" && args[0] != "skills@latest") || args[1] != "add" {
		return Ref{}, invalid
	}
	source := args[2]
	var slug string
	for i := 3; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-g" || arg == "--global" || arg == "-y" || arg == "--yes":
		case arg == "--skill" && slug == "" && i+1 < len(args):
			i++
			slug = args[i]
		case strings.HasPrefix(arg, "--skill=") && slug == "":
			slug = strings.TrimPrefix(arg, "--skill=")
		default:
			return Ref{}, invalid
		}
	}
	if u, err := url.Parse(source); err == nil {
		if hosted, ok := hostedURL(u, host); ok {
			if slug != "" && slug != hosted.Slug {
				return Ref{}, invalid
			}
			return hosted, nil
		}
	}
	if !commandPart.MatchString(slug) {
		return Ref{}, invalid
	}
	if strings.Contains(source, "://") {
		u, err := url.Parse(source)
		if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return Ref{}, invalid
		}
		source = strings.TrimPrefix(u.Path, "/")
	}
	source = strings.TrimSuffix(strings.TrimSuffix(source, "/"), ".git")
	parts := strings.Split(source, "/")
	if len(parts) != 2 || !commandPart.MatchString(parts[0]) || !commandPart.MatchString(parts[1]) {
		return Ref{}, invalid
	}
	id := source + "/" + slug
	return Ref{ID: id, Source: source, Slug: slug, SourceType: "github", URL: base + id}, nil
}

func wellKnown(domain, slug string) Ref {
	id := domain + "/" + slug
	return Ref{ID: id, Source: domain, Slug: slug, SourceType: "well-known", URL: base + "site/" + id}
}

func hostedURL(u *url.URL, host string) (Ref, bool) {
	if host == "" || !strings.EqualFold(u.Host, host) || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Ref{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || !commandPart.MatchString(parts[0]) || !commandPart.MatchString(parts[1]) {
		return Ref{}, false
	}
	source := strings.ToLower(u.Host) + "/" + parts[0]
	id := "hosted/" + source + "/" + parts[1]
	return Ref{ID: id, Source: source, Slug: parts[1], SourceType: "hosted", URL: u.Scheme + "://" + source + "/" + parts[1]}, true
}
