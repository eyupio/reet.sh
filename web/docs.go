package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/docs"
)

// isDocsPath is true for /docs and everything below it. The docs are a normal, crawlable site, not SPA routes.
func isDocsPath(path string) bool {
	return path == "/docs" || strings.HasPrefix(path, "/docs/")
}

func serveDocs(w http.ResponseWriter, r *http.Request, origin string) bool {
	if !isDocsPath(r.URL.Path) {
		return false
	}
	path := r.URL.Path
	if path == "/docs" {
		http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
		return true
	}
	if strings.HasPrefix(path, "/docs/images/") {
		serveDocsImage(w, r)
		return true
	}
	if !strings.HasSuffix(path, "/") {
		http.Redirect(w, r, path+"/", http.StatusMovedPermanently)
		return true
	}
	slug := strings.Trim(strings.TrimPrefix(path, "/docs/"), "/")
	if slug == "" {
		writeDocsIndex(w, r, origin)
		return true
	}
	if strings.Contains(slug, "/") {
		writeDocsNotFound(w, r, origin)
		return true
	}
	page, ok := docs.Get(slug)
	if !ok {
		writeDocsNotFound(w, r, origin)
		return true
	}
	writeDocsPage(w, r, page, origin)
	return true
}

func serveDocsImage(w http.ResponseWriter, r *http.Request) {
	assets := docs.Assets()
	if assets == nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/docs/images/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	clone := r.Clone(r.Context())
	clone.URL.Path = "/" + name
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.FileServerFS(assets).ServeHTTP(w, clone)
}

type docsNavSection struct {
	Name  string
	Pages []docs.Page
}

func docsSitemapPaths() []string {
	paths := []string{"/docs/"}
	for _, page := range docs.Pages() {
		paths = append(paths, "/docs/"+page.Slug+"/")
	}
	return paths
}

func docsNav() []docsNavSection {
	pages := docs.Pages()
	sections := []docsNavSection{}
	for _, page := range pages {
		if len(sections) == 0 || sections[len(sections)-1].Name != page.Section {
			sections = append(sections, docsNavSection{Name: page.Section})
		}
		sections[len(sections)-1].Pages = append(sections[len(sections)-1].Pages, page)
	}
	return sections
}

func docsNeighbours(slug string) (prev, next *docs.Page) {
	pages := docs.Pages()
	for i := range pages {
		if pages[i].Slug == slug {
			if i > 0 {
				prev = &pages[i-1]
			}
			if i+1 < len(pages) {
				next = &pages[i+1]
			}
			return prev, next
		}
	}
	return nil, nil
}

type docsPageData struct {
	Origin      string
	Title       string
	Description string
	Canonical   string
	Eyebrow     string
	Current     string
	Heading     string
	Page        docs.Page
	Nav         []docsNavSection
	Prev        *docs.Page
	Next        *docs.Page
	Body        template.HTML
	Structured  template.JS
}

func writeDocsPage(w http.ResponseWriter, r *http.Request, page docs.Page, origin string) {
	canonical := origin + "/docs/" + page.Slug + "/"
	prev, next := docsNeighbours(page.Slug)
	structured, _ := json.Marshal(map[string]any{
		"@context": "https://schema.org",
		"@graph": []any{
			map[string]any{
				"@type":            "TechArticle",
				"headline":         page.Title,
				"description":      page.Description,
				"url":              canonical,
				"mainEntityOfPage": canonical,
				"isPartOf":         map[string]any{"@type": "WebSite", "name": "reet.sh", "url": origin + "/"},
				"publisher":        map[string]any{"@type": "Organization", "name": "EyUp.io", "url": "https://eyup.io"},
			},
			map[string]any{
				"@type": "BreadcrumbList",
				"itemListElement": []any{
					map[string]any{"@type": "ListItem", "position": 1, "name": "reet.sh", "item": origin + "/"},
					map[string]any{"@type": "ListItem", "position": 2, "name": "Docs", "item": origin + "/docs/"},
					map[string]any{"@type": "ListItem", "position": 3, "name": page.Title, "item": canonical},
				},
			},
		},
	})
	data := docsPageData{
		Origin:      origin,
		Title:       page.Title + " · reet.sh Docs",
		Description: page.Description,
		Canonical:   canonical,
		Eyebrow:     "Docs / " + page.Section,
		Current:     page.Slug,
		Heading:     page.Title,
		Page:        page,
		Nav:         docsNav(),
		Prev:        prev,
		Next:        next,
		Body:        template.HTML(page.HTML),
		Structured:  template.JS(structured),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = docsPageTemplate.Execute(w, data)
}

func writeDocsIndex(w http.ResponseWriter, r *http.Request, origin string) {
	canonical := origin + "/docs/"
	structured, _ := json.Marshal(map[string]any{
		"@context":    "https://schema.org",
		"@type":       "CollectionPage",
		"name":        "reet.sh documentation",
		"description": "Install, connect machines, run agents, sync skills and operate a self-hosted reet.sh instance.",
		"url":         canonical,
		"isPartOf":    map[string]any{"@type": "WebSite", "name": "reet.sh", "url": origin + "/"},
	})
	data := docsIndexData{
		Origin:      origin,
		Title:       "Documentation · reet.sh",
		Description: "Install, connect machines, run agents, sync skills and operate a self-hosted reet.sh instance.",
		Canonical:   canonical,
		Nav:         docsNav(),
		Structured:  template.JS(structured),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = docsIndexTemplate.Execute(w, data)
}

func writeDocsNotFound(w http.ResponseWriter, r *http.Request, origin string) {
	data := docsPageData{
		Origin:      origin,
		Title:       "Page not found · reet.sh Docs",
		Description: "That documentation page was not found.",
		Canonical:   origin + "/docs/",
		Eyebrow:     "Docs",
		Heading:     "Page not found",
		Nav:         docsNav(),
		Body:        template.HTML(`<p>That documentation page was not found. <a href="/docs/">Browse the documentation</a> or <a href="/community">visit the public community</a>.</p>`),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(http.StatusNotFound)
	_ = docsPageTemplate.Execute(w, data)
}

type docsIndexData struct {
	Origin      string
	Title       string
	Description string
	Canonical   string
	Current     string
	Nav         []docsNavSection
	Structured  template.JS
}

var docsHead = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark"><title>{{.Title}}</title><meta name="description" content="{{.Description}}"><meta name="robots" content="index,follow"><link rel="canonical" href="{{.Canonical}}"><meta property="og:type" content="article"><meta property="og:site_name" content="reet.sh"><meta property="og:title" content="{{.Title}}"><meta property="og:description" content="{{.Description}}"><meta property="og:url" content="{{.Canonical}}"><meta property="og:image" content="{{.Origin}}/og-image.png"><meta name="twitter:card" content="summary_large_image"><meta name="twitter:title" content="{{.Title}}"><meta name="twitter:description" content="{{.Description}}"><meta name="twitter:image" content="{{.Origin}}/og-image.png"><script type="application/ld+json">{{.Structured}}</script><style>
:root{color-scheme:light dark;--bg:#f7f6f2;--card:#fffdf9;--ink:#1d1c1a;--muted:#5f5c54;--line:#dedbd2;--accent:#c2410c;--mono:ui-monospace,Consolas,monospace;--sans:system-ui,-apple-system,'Segoe UI',sans-serif;--serif:Georgia,serif}
@media(prefers-color-scheme:dark){:root{--bg:#1a1612;--card:#29221d;--ink:#f1ebe4;--muted:#b7aea3;--line:#3a332c;--accent:#ff8d63}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:16px/1.7 var(--sans)}a{color:var(--accent)}a:hover{text-decoration-thickness:2px}.skip{position:absolute;left:-9999px}.skip:focus{left:12px;top:12px;background:var(--card);padding:8px 12px;z-index:9}
header.top{border-bottom:1px solid var(--line);background:var(--card)}header.top .inner{max-width:1120px;margin:0 auto;padding:16px 20px;display:flex;align-items:center;gap:20px;flex-wrap:wrap}.brand{font:600 1.15rem var(--serif);color:var(--ink);text-decoration:none}.top nav{display:flex;gap:16px;flex-wrap:wrap;margin-left:auto;font-size:.9rem}
.layout{max-width:1120px;margin:0 auto;display:grid;grid-template-columns:250px minmax(0,1fr);gap:40px;padding:28px 20px 64px}aside nav{position:sticky;top:16px;font-size:.86rem}aside .section{font-size:.68rem;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);margin:18px 0 6px}aside ul{list-style:none;margin:0;padding:0;display:grid;gap:3px}aside li a{display:block;padding:4px 8px;border-radius:6px;text-decoration:none;color:var(--ink)}aside li a:hover{background:var(--card)}aside li a[aria-current=page]{background:var(--card);box-shadow:inset 3px 0 0 var(--accent);font-weight:600}
main{min-width:0}.breadcrumb{font-size:.8rem;color:var(--muted);margin:0 0 14px}.eyebrow{color:var(--accent);font-size:.68rem;font-weight:700;letter-spacing:.12em;text-transform:uppercase;margin:0 0 6px}h1{font:500 clamp(1.8rem,4vw,2.7rem)/1.15 var(--serif);letter-spacing:-.03em;margin:0 0 16px}.content h2{font:600 1.4rem/1.25 var(--serif);margin:34px 0 10px}.content h3{font-size:1.05rem;margin:26px 0 6px}.content p,.content li{max-width:72ch}.content img{max-width:100%;height:auto;border:1px solid var(--line);border-radius:10px}.content pre{padding:14px 16px;overflow:auto;border:1px solid var(--line);border-radius:10px;background:var(--card);font:13px/1.6 var(--mono)}.content code{font-family:var(--mono);font-size:.9em}.content table{width:100%;border-collapse:collapse;display:block;overflow:auto}.content th,.content td{border:1px solid var(--line);padding:8px 10px;text-align:left;vertical-align:top}.content blockquote{margin:18px 0;padding:2px 16px;border-left:3px solid var(--accent);color:var(--muted)}.content details{border:1px solid var(--line);border-radius:10px;padding:10px 14px;margin:14px 0;background:var(--card)}
.pager{display:flex;justify-content:space-between;gap:16px;border-top:1px solid var(--line);margin-top:42px;padding-top:18px;font-size:.88rem}.pager a{text-decoration:none}.pager span{color:var(--muted)}
footer.site{border-top:1px solid var(--line);color:var(--muted);font-size:.82rem}footer.site .inner{max-width:1120px;margin:0 auto;padding:18px 20px;display:flex;gap:16px;flex-wrap:wrap;justify-content:space-between}footer.site a{color:var(--muted)}
.index-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px;margin-top:22px}.index-grid section{border:1px solid var(--line);border-radius:12px;background:var(--card);padding:18px}.index-grid h2{font:600 1.1rem var(--serif);margin:0 0 10px}.index-grid ul{list-style:none;margin:0;padding:0;display:grid;gap:10px}.index-grid a{font-weight:600;text-decoration:none}.index-grid p{margin:2px 0 0;color:var(--muted);font-size:.82rem;line-height:1.5}
@media(max-width:850px){.layout{grid-template-columns:1fr;gap:18px}aside nav{position:static;border-bottom:1px solid var(--line);padding-bottom:14px}.index-grid{grid-template-columns:1fr}}
</style></head><body><a class="skip" href="#main">Skip to content</a><header class="top"><div class="inner"><a class="brand" href="/">reet.sh</a><nav aria-label="Site"><a href="/docs/">Docs</a><a href="/community">Community</a><a href="https://github.com/eyupio/reet.sh">GitHub</a><a href="/">Open workspace</a></nav></div></header><div class="layout"><aside><nav aria-label="Documentation">{{range .Nav}}<p class="section">{{.Name}}</p><ul>{{range .Pages}}<li><a href="/docs/{{.Slug}}/" {{if eq .Slug $.Current}}aria-current="page"{{end}}>{{.Title}}</a></li>{{end}}</ul>{{end}}</nav></aside><main id="main">`

var docsFoot = `</main></div><footer class="site"><div class="inner"><span>reet.sh documentation · AGPL-3.0</span><span><a href="https://github.com/eyupio/reet.sh">Source</a> · <a href="/community">Community</a> · <a href="https://eyup.io">EyUp.io</a></span></div></footer></body></html>`

var docsPageTemplate = template.Must(template.New("docs").Parse(docsHead + `<p class="eyebrow">{{.Eyebrow}}</p><h1>{{.Heading}}</h1><article class="content">{{.Body}}</article><nav class="pager" aria-label="Previous and next">{{if .Prev}}<a href="/docs/{{.Prev.Slug}}/">← {{.Prev.Title}}</a>{{else}}<span></span>{{end}}{{if .Next}}<a href="/docs/{{.Next.Slug}}/">{{.Next.Title}} →</a>{{else}}<span></span>{{end}}</nav>` + docsFoot))

var docsIndexTemplate = template.Must(template.New("docs-index").Parse(docsHead + `<p class="eyebrow">Documentation</p><h1>Everything you need to run reet.sh.</h1><p>Install the binary, connect machines, run agents, sync skills and setups, and operate the instance.</p><div class="index-grid">{{range .Nav}}<section><h2>{{.Name}}</h2><ul>{{range .Pages}}<li><a href="/docs/{{.Slug}}/">{{.Title}}</a><p>{{.Description}}</p></li>{{end}}</ul></section>{{end}}</div>` + docsFoot))
