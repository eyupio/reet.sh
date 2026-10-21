package meta

import (
	"context"
	"fmt"
	"html"
	"strings"

	xhtml "golang.org/x/net/html"

	"github.com/eyupio/reet.sh/internal/ref"
)

// Topic is one curated skills.sh topic and the skills currently on its page.
type Topic struct {
	Slug   string
	Name   string
	Skills []string
}

// Topics reads the topic index and each topic page.
func (f Fetcher) Topics(ctx context.Context) ([]Topic, error) {
	base := strings.TrimRight(f.PageBase, "/")
	b, ok := f.get(ctx, base+"/topic", "")
	if !ok {
		return nil, fmt.Errorf("topic index unavailable")
	}
	var out []Topic
	for _, slug := range topicSlugs(b) {
		page, ok := f.get(ctx, base+"/topic/"+slug, "")
		if !ok {
			continue
		}
		name := topicName(page)
		if name == "" {
			name = slug
		}
		out = append(out, Topic{Slug: slug, Name: name, Skills: topicSkills(page)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no topics found")
	}
	return out, nil
}

func topicSlugs(b []byte) []string {
	z := xhtml.NewTokenizer(strings.NewReader(string(b)))
	seen := map[string]bool{}
	var out []string
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return out
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			tok := z.Token()
			if tok.Data != "a" {
				continue
			}
			href := attr(tok, "href")
			slug, ok := strings.CutPrefix(href, "/topic/")
			slug = strings.Trim(slug, "/")
			if !ok || slug == "" || strings.Contains(slug, "/") || seen[slug] {
				continue
			}
			seen[slug] = true
			out = append(out, slug)
			if len(out) >= 20 {
				return out
			}
		}
	}
}

func topicName(b []byte) string {
	z := xhtml.NewTokenizer(strings.NewReader(string(b)))
	inTitle := false
	var title strings.Builder
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			name := strings.TrimSpace(html.UnescapeString(title.String()))
			if before, _, ok := strings.Cut(name, " — "); ok {
				name = before
			}
			return strings.TrimSpace(strings.TrimSuffix(name, " skills"))
		case xhtml.StartTagToken:
			if z.Token().Data == "title" {
				inTitle = true
			}
		case xhtml.TextToken:
			if inTitle {
				title.Write(z.Text())
			}
		case xhtml.EndTagToken:
			if z.Token().Data == "title" {
				inTitle = false
			}
		}
	}
}

func topicSkills(b []byte) []string {
	z := xhtml.NewTokenizer(strings.NewReader(string(b)))
	seen := map[string]bool{}
	var out []string
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return out
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			tok := z.Token()
			if tok.Data != "a" {
				continue
			}
			href := attr(tok, "href")
			if !strings.HasPrefix(href, "/") {
				continue
			}
			r, err := ref.Parse("https://www.skills.sh" + href)
			if err != nil || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, r.ID)
		}
	}
}
