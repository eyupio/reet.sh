// Package meta fetches best-effort skill metadata from skills.sh.
package meta

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/eyupio/reet.sh/internal/ref"
)

// Meta is whatever could be learned about a skill; zero values mean "unknown".
type Meta struct {
	Name         string
	Description  string
	Installs     int64
	InstallsWeek int64
	Trend        []int64
}

// Empty reports whether nothing was learned.
func (m Meta) Empty() bool {
	return m.Name == "" && m.Description == "" && m.Installs == 0 && m.InstallsWeek == 0 && len(m.Trend) == 0
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
	if (m.Name == "" || m.Description == "" || m.Installs == 0) && f.PageBase != "" {
		p := f.fromPage(ctx, r)
		if m.Name == "" {
			m.Name = p.Name
		}
		if m.Description == "" {
			m.Description = p.Description
		}
		if m.Installs == 0 {
			m.Installs = p.Installs
		}
		if m.InstallsWeek == 0 {
			m.InstallsWeek = p.InstallsWeek
		}
		if len(m.Trend) == 0 {
			m.Trend = p.Trend
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

func (f Fetcher) fromPage(ctx context.Context, r ref.Ref) Meta {
	path := r.ID
	if r.SourceType == "well-known" {
		path = "site/" + path
	}
	b, ok := f.get(ctx, strings.TrimRight(f.PageBase, "/")+"/"+path, "")
	if !ok {
		return Meta{}
	}
	z := html.NewTokenizer(strings.NewReader(string(b)))
	var m Meta
	var metaDesc, ogName string
	var ld strings.Builder
	inLD := false
loop:
	for {
		switch z.Next() {
		case html.ErrorToken:
			break loop
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			switch tok.Data {
			case "meta":
				var key, content string
				for _, a := range tok.Attr {
					switch a.Key {
					case "name", "property":
						key = a.Val
					case "content":
						content = a.Val
					}
				}
				switch key {
				case "description", "og:description":
					if metaDesc == "" && strings.TrimSpace(content) != "" {
						metaDesc = strings.TrimSpace(content)
					}
				case "og:title":
					if ogName == "" {
						ogName = nameFromTitle(content)
					}
				}
			case "script":
				if strings.EqualFold(attr(tok, "type"), "application/ld+json") {
					inLD = true
					ld.Reset()
				}
			case "svg":
				if label := attr(tok, "aria-label"); strings.HasPrefix(label, "Weekly installs:") {
					m.Trend = parseTrend(label)
					if n := len(m.Trend); n > 0 {
						m.InstallsWeek = m.Trend[n-1]
					}
				}
			}
		case html.TextToken:
			if inLD {
				ld.Write(z.Text())
			}
		case html.EndTagToken:
			if inLD && z.Token().Data == "script" {
				mergeLD(&m, ld.String())
				inLD = false
			}
		}
	}
	if m.Name == "" {
		m.Name = ogName
	}
	if m.Description == "" {
		m.Description = metaDesc
	}
	return m
}

func attr(tok html.Token, key string) string {
	for _, a := range tok.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func nameFromTitle(title string) string {
	title = strings.TrimSpace(title)
	if name, _, ok := strings.Cut(title, " — "); ok {
		return strings.TrimSpace(name)
	}
	return ""
}

func mergeLD(m *Meta, raw string) {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return
	}
	node, ok := findSoftwareApp(v)
	if !ok {
		return
	}
	if m.Name == "" {
		if name, ok := node["name"].(string); ok {
			m.Name = strings.TrimSpace(name)
		}
	}
	if m.Description == "" {
		if desc, ok := node["description"].(string); ok {
			m.Description = strings.TrimSpace(desc)
		}
	}
	if m.Installs == 0 {
		if stat, ok := node["interactionStatistic"].(map[string]any); ok {
			if n, ok := number(stat["userInteractionCount"]); ok {
				m.Installs = int64(n)
			}
		}
	}
}

func findSoftwareApp(v any) (map[string]any, bool) {
	switch node := v.(type) {
	case map[string]any:
		if isSoftwareApp(node) {
			return node, true
		}
		for _, child := range node {
			if found, ok := findSoftwareApp(child); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range node {
			if found, ok := findSoftwareApp(child); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func isSoftwareApp(node map[string]any) bool {
	if _, ok := node["interactionStatistic"]; ok {
		return true
	}
	switch typ := node["@type"].(type) {
	case string:
		return typ == "SoftwareApplication"
	case []any:
		for _, t := range typ {
			if s, ok := t.(string); ok && s == "SoftwareApplication" {
				return true
			}
		}
	}
	return false
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.ReplaceAll(n, ",", ""), 64)
		return f, err == nil
	}
	return 0, false
}

func parseTrend(label string) []int64 {
	_, series, ok := strings.Cut(label, ":")
	if !ok {
		return nil
	}
	var out []int64
	for _, part := range strings.Split(series, ", ") {
		part = strings.TrimSpace(strings.ReplaceAll(part, ",", ""))
		if part == "" || part == "…" {
			continue
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}
