// Package meta fetches best-effort skill metadata from skills.sh.
package meta

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/eyupio/reet.sh/internal/ref"
)

// Meta is whatever could be learned about a skill; zero values mean "unknown".
type Meta struct {
	Name        string
	Description string
	Installs    int64
}

// Fetcher reads metadata from the official API (when Token is set) or the public page.
type Fetcher struct {
	Client            *http.Client
	PageBase, APIBase string
	Token             string
}

// Default returns a Fetcher pointed at skills.sh with an 8 second timeout.
func Default(token string) Fetcher {
	return Fetcher{
		Client:   &http.Client{Timeout: 8 * time.Second},
		PageBase: "https://www.skills.sh/",
		APIBase:  "https://skills.sh",
		Token:    token,
	}
}

const maxBody = 2 << 20

// Fetch never fails: on any problem it returns whatever it has, possibly the zero Meta.
func (f Fetcher) Fetch(ctx context.Context, r ref.Ref) Meta {
	var m Meta
	if f.Token != "" && f.APIBase != "" {
		m = f.fromAPI(ctx, r)
	}
	if m.Description == "" && f.PageBase != "" {
		if d := f.fromPage(ctx, r); d != "" {
			m.Description = d
		}
	}
	return m
}

func (f Fetcher) get(ctx context.Context, url, token string) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	c := f.Client
	if c == nil {
		c = &http.Client{Timeout: 8 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	return b, err == nil
}

func (f Fetcher) fromAPI(ctx context.Context, r ref.Ref) Meta {
	b, ok := f.get(ctx, strings.TrimRight(f.APIBase, "/")+"/api/v1/skills/"+r.ID, f.Token)
	if !ok {
		return Meta{}
	}
	var v struct {
		Installs int64 `json:"installs"`
		Files    []struct {
			Path     string `json:"path"`
			Contents string `json:"contents"`
		} `json:"files"`
	}
	if json.Unmarshal(b, &v) != nil {
		return Meta{}
	}
	m := Meta{Installs: v.Installs}
	for _, file := range v.Files {
		if strings.EqualFold(file.Path, "SKILL.md") {
			m.Name, m.Description = frontmatter(file.Contents)
			break
		}
	}
	return m
}

func frontmatter(md string) (name, desc string) {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	if !strings.HasPrefix(md, "---\n") {
		return
	}
	end := strings.Index(md[4:], "\n---")
	if end < 0 {
		return
	}
	for _, line := range strings.Split(md[4:4+end], "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "name":
			name = v
		case "description":
			desc = v
		}
	}
	return
}

func (f Fetcher) fromPage(ctx context.Context, r ref.Ref) string {
	path := r.ID
	if r.SourceType == "well-known" {
		path = "site/" + path
	}
	b, ok := f.get(ctx, strings.TrimRight(f.PageBase, "/")+"/"+path, "")
	if !ok {
		return ""
	}
	z := html.NewTokenizer(strings.NewReader(string(b)))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			if tok.Data != "meta" {
				continue
			}
			var key, content string
			for _, a := range tok.Attr {
				switch a.Key {
				case "name", "property":
					key = a.Val
				case "content":
					content = a.Val
				}
			}
			if (key == "description" || key == "og:description") && strings.TrimSpace(content) != "" {
				return strings.TrimSpace(content)
			}
		}
	}
}
