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

// The boot shell in index.html holds the page's shape while the bundle loads. Only the variant that matches the route is
// served, so a landing request never holds an app skeleton and an app request never holds a landing hero.
var bootZones = [][2]string{
	{"<!--boot-welcome:start-->", "<!--boot-welcome:end-->"},
	{"<!--boot-app:start-->", "<!--boot-app:end-->"},
	{"<!--boot-auth:start-->", "<!--boot-auth:end-->"},
}

const (
	bootWelcome = iota
	bootApp
	bootAuth
)

var bootNames = [...]string{"welcome", "app", "auth"}

// bootVariant picks the shell that matches what the app is about to draw: the landing hero for an anonymous landing
// route, a centred card for the entry screens, and the page shell for everything else. A signed-in request to "/" is
// Home, not the landing page.
func bootVariant(path string, signedIn bool) int {
	if !signedIn && (path == "/" || path == "/welcome") {
		return bootWelcome
	}
	switch {
	case path == "/signin", path == "/signup", path == "/setup", path == "/reset", strings.HasPrefix(path, "/join"):
		return bootAuth
	}
	return bootApp
}

// Option configures Handler.
type Option func(*config)

type config struct {
	signedIn func(*http.Request) bool
}

// WithSignedIn tells Handler how to recognise a request that already carries a
// signed-in session. Such a visitor is shown the app shell, not the static
// landing page, so serving them the crawler fallback would flash the logged-out
// page before the SPA mounts.
func WithSignedIn(fn func(*http.Request) bool) Option {
	return func(c *config) { c.signedIn = fn }
}

// Handler serves the SPA, falling back to index.html for client-side routes.
// The crawler files are answered here so the SPA fallback cannot shadow them
// with an HTML page, and index.html is served with the request origin applied.
// baseURL is the configured public origin (REET_PUBLIC_URL); when empty the
// origin is derived from the request.
func Handler(baseURL string, opts ...Option) http.Handler {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
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
			serveIndex(w, index, r.URL.Path, origin(r, baseURL), cfg.signedIn != nil && cfg.signedIn(r))
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
		serveIndex(w, index, r.URL.Path, origin(r, baseURL), cfg.signedIn != nil && cfg.signedIn(r))
	})
}

// serveIndex serves the built index.html with the origin substituted. The
// static crawler fallback is only kept on landing routes for visitors who are
// not already signed in; every other route is an app screen, and a signed-in
// visitor is going straight to the app, so both would otherwise paint the
// logged-out landing page before the SPA mounts.
func serveIndex(w http.ResponseWriter, index []byte, path, origin string, signedIn bool) {
	if index == nil {
		http.Error(w, "reet.sh UI is not built; run `npm run build` in web/", http.StatusNotFound)
		return
	}
	body := bytes.ReplaceAll(index, []byte(originToken), []byte(html.EscapeString(origin)))
	if !signedIn && (path == "/" || path == "/welcome") {
		body = bytes.ReplaceAll(body, []byte(fallbackStart), nil)
		body = bytes.ReplaceAll(body, []byte(fallbackEnd), nil)
	} else {
		body = stripFallback(body)
	}
	variant := bootVariant(path, signedIn)
	body = selectBoot(body, variant)
	// The chosen shell is on the document before the head script runs, so a cookie-signed-in visitor on "/" gets the app
	// shell rather than the landing hero the browser alone cannot know to skip.
	body = bytes.Replace(body, []byte(`<html lang="en">`), []byte(`<html lang="en" data-boot="`+bootNames[variant]+`">`), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

func stripFallback(body []byte) []byte {
	return stripBetween(body, []byte(fallbackStart), []byte(fallbackEnd))
}

// selectBoot keeps one boot shell and removes the other two, markers included, so the served page never carries a
// variant that does not match its route.
func selectBoot(body []byte, keep int) []byte {
	for i, zone := range bootZones {
		start, end := []byte(zone[0]), []byte(zone[1])
		if i == keep {
			body = bytes.ReplaceAll(body, start, nil)
			body = bytes.ReplaceAll(body, end, nil)
			continue
		}
		body = stripBetween(body, start, end)
	}
	return body
}

// stripBetween removes the range from start through end, markers included. A missing or reversed pair leaves the body
// unchanged, so an index built without a zone still serves.
func stripBetween(body, start, end []byte) []byte {
	i := bytes.Index(body, start)
	if i < 0 {
		return body
	}
	j := bytes.Index(body[i:], end)
	if j < 0 {
		return body
	}
	j += i + len(end)
	out := make([]byte, 0, len(body))
	out = append(out, body[:i]...)
	out = append(out, body[j:]...)
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
