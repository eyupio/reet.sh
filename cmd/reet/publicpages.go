package main

import (
	"errors"
	"fmt"
	"html"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/web"
)

// publicPageFor renders the public community and public lists for crawlers that do not run JavaScript. The browser
// still gets the SPA; this is the readable copy that replaces the landing fallback on those routes.
func publicPageFor(st *store.Store) func(string) (web.PublicPage, bool) {
	return func(path string) (web.PublicPage, bool) {
		switch {
		case path == "/community":
			return communityPage(st), true
		case strings.HasPrefix(path, "/l/"):
			return publicListPage(st, path), true
		}
		return web.PublicPage{}, false
	}
}

func communityPage(st *store.Store) web.PublicPage {
	page := web.PublicPage{
		Path:        "/community",
		Title:       "Public community · reet.sh",
		Description: "Publicly published lists and their skills from the reet.sh community.",
	}
	lists, skills, err := st.Community()
	if err != nil {
		page.Title = "Public community · reet.sh"
		page.Status = 500
		page.HTML = `<main class="crawl-fallback"><p class="eyebrow">Public community</p><h1>Public community</h1><p>The community could not be read just now.</p><p><a href="/">Open your workspace</a></p></main>`
		return page
	}
	var b strings.Builder
	b.WriteString(`<main class="crawl-fallback"><p class="eyebrow">Public community</p><h1>Public community</h1><p class="lede">Publicly published lists and their skills. Explore without an account; personal shelves, teams and selected-people lists stay private.</p>`)
	if len(lists) > 0 {
		b.WriteString("<h2>Lists</h2><ul>")
		for _, list := range lists {
			b.WriteString(`<li><a href="/l/` + html.EscapeString(list.Slug) + `">` + html.EscapeString(list.Name) + `</a>`)
			if list.Description != "" {
				b.WriteString(" — " + html.EscapeString(list.Description))
			}
			b.WriteString(fmt.Sprintf(" (%d skills)</li>", list.SkillCount))
		}
		b.WriteString("</ul>")
	}
	if len(skills) > 0 {
		b.WriteString("<h2>Skills</h2><ul>")
		for _, skill := range skills {
			b.WriteString("<li>" + html.EscapeString(skill.Name))
			if skill.Source != "" {
				b.WriteString(" — <code>" + html.EscapeString(skill.Source) + "</code>")
			}
			if skill.Description != "" {
				b.WriteString(" — " + html.EscapeString(skill.Description))
			}
			b.WriteString("</li>")
		}
		b.WriteString("</ul>")
	}
	if len(lists) == 0 && len(skills) == 0 {
		b.WriteString("<p>No lists have been published yet.</p>")
	}
	b.WriteString(`<p><a href="/">Open your workspace</a></p></main>`)
	page.HTML = b.String()
	return page
}

func publicListPage(st *store.Store, path string) web.PublicPage {
	slug := strings.Trim(strings.TrimPrefix(path, "/l/"), "/")
	missing := web.PublicPage{
		Path:        "/l/" + slug,
		Title:       "That link is not available · reet.sh",
		Description: "The list may be private, deleted, or link sharing may be turned off.",
		HTML:        `<main class="crawl-fallback"><p class="eyebrow">Shared list</p><h1>That link is not available.</h1><p>The list may be private, deleted, or link sharing may be turned off.</p><p><a href="/">Go to reet.sh</a></p></main>`,
		Status:      404,
	}
	if slug == "" {
		return missing
	}
	detail, err := st.PublicListFor(slug)
	if errors.Is(err, store.ErrNotFound) {
		return missing
	}
	if err != nil {
		page := missing
		page.Title = "Shared list · reet.sh"
		page.Description = "A shared list of skills on reet.sh."
		page.Status = 500
		page.HTML = `<main class="crawl-fallback"><p class="eyebrow">Shared list</p><h1>Shared list</h1><p>The list could not be read just now.</p><p><a href="/">Go to reet.sh</a></p></main>`
		return page
	}
	if detail.Visibility == "link" {
		if enabled, _ := st.GetSetting("link_lists_enabled"); enabled != "1" {
			return missing
		}
	}
	description := strings.TrimSpace(detail.Description)
	if description == "" {
		description = fmt.Sprintf("A public list of %d skills on reet.sh.", len(detail.Skills))
	}
	page := web.PublicPage{
		Path:        "/l/" + detail.Slug,
		Title:       detail.Name + " · reet.sh",
		Description: description,
	}
	var b strings.Builder
	b.WriteString(`<main class="crawl-fallback"><p class="eyebrow">Shared list</p><h1>` + html.EscapeString(detail.Name) + `</h1>`)
	if detail.Description != "" {
		b.WriteString(`<p class="lede">` + html.EscapeString(detail.Description) + `</p>`)
	}
	if len(detail.Skills) > 0 {
		b.WriteString("<h2>Skills</h2><ul>")
		for _, skill := range detail.Skills {
			b.WriteString("<li><strong>" + html.EscapeString(skill.Name) + "</strong>")
			if skill.Source != "" {
				b.WriteString(" — <code>" + html.EscapeString(skill.Source) + "</code>")
			}
			if skill.Description != "" {
				b.WriteString(" — " + html.EscapeString(skill.Description))
			}
			b.WriteString("</li>")
		}
		b.WriteString("</ul>")
	}
	b.WriteString(`<p>Open this list in reet.sh to copy the install commands. Only the skills and list notes are shown; member names, emails and personal notes stay private.</p><p><a href="/community">Browse the public community</a> · <a href="/">Open your workspace</a></p></main>`)
	page.HTML = b.String()
	return page
}

// publicURLsFor keeps the sitemap in step with the community and the public lists it contains.
func publicURLsFor(st *store.Store) func() []string {
	return func() []string {
		urls := []string{"/community"}
		lists, _, err := st.Community()
		if err != nil {
			return urls
		}
		for _, list := range lists {
			if list.Slug != "" {
				urls = append(urls, "/l/"+list.Slug)
			}
		}
		return urls
	}
}
