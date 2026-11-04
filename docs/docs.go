// Package docs renders the public documentation that ships inside the reet binary.
package docs

import (
	"bytes"
	"embed"
	"html"
	"io/fs"
	"regexp"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

//go:embed *.md
var markdown embed.FS

//go:embed images
var images embed.FS

// Assets is the docs image tree, rooted at "images".
func Assets() fs.FS {
	sub, err := fs.Sub(images, "images")
	if err != nil {
		return nil
	}
	return sub
}

// Page is one rendered documentation page.
type Page struct {
	Slug        string
	File        string
	Title       string
	Description string
	Section     string
	Order       int
	HTML        string
}

type entry struct {
	file    string
	section string
	order   int
}

// The manifest is deliberately a short, public subset of docs/. Design notes, research, handoffs and verification
// logs stay in the repository but are not published as user documentation.
var manifest = []entry{
	{"binary-installation.md", "Get started", 10},
	{"add-machine-in-context.md", "Get started", 20},
	{"cli-home.md", "Get started", 30},
	{"setup-discovery.md", "Get started", 40},
	{"control-centre.md", "Machines and agents", 10},
	{"agent-software.md", "Machines and agents", 20},
	{"agent-models.md", "Machines and agents", 30},
	{"start-agent-session.md", "Machines and agents", 40},
	{"terminal-workspace.md", "Machines and agents", 50},
	{"terminal-launch.md", "Machines and agents", 60},
	{"terminal-worktrees.md", "Machines and agents", 70},
	{"folder-browse.md", "Machines and agents", 80},
	{"skills-machines-ux.md", "Skills and sync", 10},
	{"controller-sync.md", "Skills and sync", 20},
	{"environment-sync.md", "Skills and sync", 30},
	{"mcp.md", "Skills and sync", 40},
	{"browser-skill-import.md", "Skills and sync", 50},
	{"machine-skill-import.md", "Skills and sync", 60},
	{"machine-content-observations.md", "Skills and sync", 70},
	{"sharing.md", "Skills and sync", 80},
	{"projects.md", "Work", 10},
	{"session-board.md", "Work", 20},
	{"backups.md", "Operations", 10},
	{"single-sign-on.md", "Operations", 20},
	{"email.md", "Operations", 30},
	{"notifications.md", "Operations", 40},
	{"support.md", "Operations", 50},
	{"remote-machine-operations.md", "Operations", 60},
	{"device-checks.md", "Operations", 70},
	{"branch-pruning.md", "Operations", 80},
	{"terminal-command-history.md", "Operations", 90},
	{"terminal-controller-commands.md", "Operations", 100},
	{"run-claude-code-on-a-vps.md", "Guides", 10},
	{"sync-skills-between-machines.md", "Guides", 20},
	{"self-host-mcp-server.md", "Guides", 30},
	{"compare-skills-sh.md", "Compare", 10},
	{"cli-reference.md", "Reference", 10},
	{"roadmap.md", "Reference", 20},
}

var (
	once   sync.Once
	pages  []Page
	bySlug map[string]Page
)

var (
	firstH1RE    = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	firstParaRE  = regexp.MustCompile(`(?m)^(?:[^\n#![\]()<>-][^\n]*)\n`)
	markdownRE   = regexp.MustCompile(`[#*_` + "`" + `>\[\]()]`)
	spacesRE     = regexp.MustCompile(`\s+`)
	linkAttrRE   = regexp.MustCompile(`(href|src)="([^"]*)"`)
	srcsetAttrRE = regexp.MustCompile(`srcset="([^"]*)"`)
)

const repoRoot = "https://github.com/eyupio/reet.sh/blob/main/"

// Pages returns every public documentation page in navigation order.
func Pages() []Page {
	load()
	out := make([]Page, len(pages))
	copy(out, pages)
	return out
}

// Get returns one page by slug.
func Get(slug string) (Page, bool) {
	load()
	page, ok := bySlug[slug]
	return page, ok
}

func load() {
	once.Do(func() {
		md := goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		)
		bySlug = map[string]Page{}
		known := map[string]bool{}
		for _, item := range manifest {
			known[strings.TrimSuffix(item.file, ".md")] = true
		}
		order := make([]Page, 0, len(manifest))
		for _, item := range manifest {
			raw, err := markdown.ReadFile(item.file)
			if err != nil {
				continue
			}
			page := Page{
				Slug:    strings.TrimSuffix(item.file, ".md"),
				File:    item.file,
				Section: item.section,
				Order:   item.order,
			}
			page.Title, page.Description = titleAndDescription(string(raw), page.Slug)
			var buf bytes.Buffer
			if err := md.Convert(raw, &buf); err != nil {
				continue
			}
			page.HTML = rewriteLinks(buf.String(), known)
			order = append(order, page)
			bySlug[page.Slug] = page
		}
		// Manifest order groups by section, then by the order within it.
		sections := []string{"Get started", "Machines and agents", "Skills and sync", "Work", "Operations", "Guides", "Compare", "Reference"}
		for _, section := range sections {
			for _, page := range order {
				if page.Section == section {
					pages = append(pages, page)
				}
			}
		}
	})
}

func titleAndDescription(raw, fallback string) (string, string) {
	title := fallback
	if m := firstH1RE.FindStringSubmatch(raw); m != nil {
		title = strings.TrimSpace(m[1])
	}
	description := ""
	if m := firstParaRE.FindString(raw); m != "" {
		description = stripMarkdown(m)
	}
	if description == "" {
		description = title + " — documentation for reet.sh."
	}
	if len(description) > 155 {
		description = strings.TrimSpace(description[:152])
		if i := strings.LastIndex(description, " "); i > 80 {
			description = description[:i]
		}
		description += "…"
	}
	return title, description
}

func stripMarkdown(s string) string {
	s = strings.TrimSpace(s)
	s = markdownRE.ReplaceAllString(s, "")
	s = spacesRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func rewriteLinks(body string, known map[string]bool) string {
	body = linkAttrRE.ReplaceAllStringFunc(body, func(attr string) string {
		m := linkAttrRE.FindStringSubmatch(attr)
		if len(m) != 3 {
			return attr
		}
		name, target := m[1], m[2]
		if strings.HasPrefix(target, "http") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "/") {
			return attr
		}
		if name == "src" {
			if strings.HasPrefix(target, "images/") {
				return `src="/docs/` + target + `"`
			}
			return attr
		}
		return `href="` + docsTarget(target, known) + `"`
	})
	body = srcsetAttrRE.ReplaceAllStringFunc(body, func(attr string) string {
		m := srcsetAttrRE.FindStringSubmatch(attr)
		if len(m) != 2 {
			return attr
		}
		parts := strings.Split(m[1], ",")
		for i, part := range parts {
			fields := strings.Fields(strings.TrimSpace(part))
			if len(fields) == 0 || !strings.HasPrefix(fields[0], "images/") {
				continue
			}
			fields[0] = "/docs/" + fields[0]
			parts[i] = strings.Join(fields, " ")
		}
		return `srcset="` + strings.Join(parts, ", ") + `"`
	})
	return body
}

func docsTarget(target string, known map[string]bool) string {
	path, fragment := target, ""
	if i := strings.Index(target, "#"); i >= 0 {
		path, fragment = target[:i], target[i:]
	}
	if strings.HasPrefix(path, "images/") {
		return "/docs/" + path + fragment
	}
	if strings.HasSuffix(path, ".md") {
		base := path[strings.LastIndex(path, "/")+1:]
		slug := strings.TrimSuffix(base, ".md")
		if path == base && slug != "" && known[slug] {
			return "/docs/" + slug + "/" + fragment
		}
		if strings.HasPrefix(path, "../") {
			return repoRoot + strings.TrimPrefix(path, "../") + fragment
		}
		return repoRoot + "docs/" + path + fragment
	}
	return html.EscapeString(target)
}
