// Package web embeds the built Svelte SPA.
package web

import (
	"bytes"
	"embed"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// extraTypes are the types the served files need that Go's built-in table lacks. Without them a type comes from the host's
// mime.types, which a slim image does not have: the web app manifest would be text/plain and the self-hosted fonts untyped.
var extraTypes = [][2]string{
	{".webmanifest", "application/manifest+json"},
	{".woff2", "font/woff2"},
}

// registerTypes hands each extra type to add (mime.AddExtensionType when the package loads), so a test can see the
// registration itself rather than whatever the host's mime.types happens to say.
func registerTypes(add func(ext, typ string) error) {
	for _, t := range extraTypes {
		_ = add(t[0], t[1])
	}
}

func init() { registerTypes(mime.AddExtensionType) }

//go:embed all:dist
var dist embed.FS

const (
	originToken   = "{{ORIGIN}}"
	fallbackStart = "<!--seo-fallback:start-->"
	fallbackEnd   = "<!--seo-fallback:end-->"
)

// Handler serves the SPA, falling back to index.html for client-side routes.
// The crawler files are answered here so the SPA fallback cannot shadow them
// with an HTML page, and index.html is served with the request origin applied.
// baseURL is the configured public origin (REET_PUBLIC_URL); when empty the
// origin is derived from the request.
func Handler(baseURL string) http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("web: embedded dist is missing: " + err.Error())
	}
	files := http.FileServerFS(sub)
	index, _ := fs.ReadFile(sub, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			writeRobots(w, origin(r, baseURL))
			return
		case "/sitemap.xml":
			writeSitemap(w, origin(r, baseURL))
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" || p == "index.html" {
			serveIndex(w, index, r.URL.Path, origin(r, baseURL))
			return
		}
		if _, err := fs.Stat(sub, p); err == nil {
			files.ServeHTTP(w, r)
			return
		}
		if index == nil {
			http.Error(w, "reet.sh UI is not built; run `npm run build` in web/", http.StatusNotFound)
			return
		}
		serveIndex(w, index, r.URL.Path, origin(r, baseURL))
	})
}

// serveIndex serves the built index.html with the origin substituted. The
// static crawler fallback is only kept on landing routes; every other route is
// an app screen and does not need the marketing copy.
func serveIndex(w http.ResponseWriter, index []byte, path, origin string) {
	if index == nil {
		http.Error(w, "reet.sh UI is not built; run `npm run build` in web/", http.StatusNotFound)
		return
	}
	body := bytes.ReplaceAll(index, []byte(originToken), []byte(html.EscapeString(origin)))
	if path == "/" || path == "/welcome" {
		body = bytes.ReplaceAll(body, []byte(fallbackStart), nil)
		body = bytes.ReplaceAll(body, []byte(fallbackEnd), nil)
	} else {
		body = stripFallback(body)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

func stripFallback(body []byte) []byte {
	start := bytes.Index(body, []byte(fallbackStart))
	end := bytes.Index(body, []byte(fallbackEnd))
	if start < 0 || end < start {
		return body
	}
	out := make([]byte, 0, len(body))
	out = append(out, body[:start]...)
	out = append(out, body[end+len(fallbackEnd):]...)
	return out
}

// origin is the absolute origin used in robots.txt, sitemap.xml and the
// origin-tagged tags in index.html.
func origin(r *http.Request, baseURL string) string {
	if base := strings.TrimRight(strings.TrimSpace(baseURL), "/"); base != "" {
		if u, err := url.Parse(base); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
			return u.Scheme + "://" + u.Host
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := r.Host
	if !validHost(host) {
		host = "localhost"
	}
	return scheme + "://" + host
}

// validHost keeps untrusted Host values out of the generated HTML and XML. Go's
// server already rejects malformed hosts, but proxies can pass arbitrary values.
func validHost(host string) bool {
	if host == "" || len(host) > 255 {
		return false
	}
	for _, c := range host {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.' || c == '-' || c == ':' || c == '[' || c == ']' || c == '_':
		default:
			return false
		}
	}
	return true
}

const robotsTxt = `# reet.sh — self-hosted control room for AI agents
# Search engines and AI search crawlers may index the public pages.
# Model-training crawlers are not restricted by default; add your own
# User-agent blocks if this instance should opt out of training.

User-agent: *
Allow: /
Disallow: /api/
Disallow: /quick

User-agent: OAI-SearchBot
Allow: /

User-agent: Claude-SearchBot
Allow: /

User-agent: PerplexityBot
Allow: /

User-agent: Googlebot
Allow: /

Sitemap: %s/sitemap.xml
`

func writeRobots(w http.ResponseWriter, origin string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(strings.Replace(robotsTxt, "%s", html.EscapeString(origin), 1)))
}

const sitemapXML = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>%s/</loc>
  </url>
</urlset>
`

func writeSitemap(w http.ResponseWriter, origin string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(strings.Replace(sitemapXML, "%s", html.EscapeString(origin), 1)))
}
