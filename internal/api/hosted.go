package api

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) registerHostedRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/hosting", s.hostedList)
	mux.HandleFunc("POST /api/v2/hosting/username", s.requireWriter(s.hostedUsername))
	mux.HandleFunc("PUT /api/v2/hosting/skills/{slug}", s.requireWriter(s.hostedSave))
	mux.HandleFunc("POST /api/v2/hosting/skills/{slug}/rename", s.requireWriter(s.hostedRename))
	mux.HandleFunc("DELETE /api/v2/hosting/skills/{slug}", s.requireWriter(s.hostedDelete))
	mux.HandleFunc("GET /{username}/{slug}", s.hostedPublic)
	mux.HandleFunc("GET /{username}/{slug}/{$}", s.hostedPublic)
	mux.HandleFunc("GET /{username}/{slug}/SKILL.md", s.hostedFile)
	mux.HandleFunc("GET /{username}/{slug}/.well-known/agent-skills/index.json", s.hostedIndex)
	mux.HandleFunc("GET /{username}/{slug}/.well-known/agent-skills/{name}/{file...}", s.hostedFile)
}

// WithHostedPages routes public skill URLs alongside the SPA without capturing
// existing application routes or exposing the management API without auth.
func WithHostedPages(api, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 2 {
			validUser := store.ValidHostedName(parts[0]) && !store.ReservedUsername(parts[0])
			validName := store.ValidHostedName(parts[1])
			if validUser && validName {
				api.ServeHTTP(w, r)
				return
			}
		}
		fallback.ServeHTTP(w, r)
	})
}

func (s *server) hostedList(w http.ResponseWriter, r *http.Request) {
	username, err := s.st.PublishingUsername(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	skills, err := s.st.HostedSkillsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	profile, err := s.st.ProfileFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"username": username, "prefix": profile.HostedPrefix, "skills": skills})
}

func (s *server) hostedUsername(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
	}
	if !decode(w, r, &in) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(in.Username))
	if err := s.st.ClaimPublishingUsername(s.uid(r), name); err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, store.ErrNamespaceTaken) || errors.Is(err, store.ErrNamespaceSet) {
			code = http.StatusConflict
		}
		writeErr(w, code, err.Error())
		return
	}
	s.eventRequest(r, "publishing-username", "", map[string]any{"username": name})
	s.hostedList(w, r)
}

func (s *server) hostedSave(w http.ResponseWriter, r *http.Request) {
	var sk store.HostedSkill
	if !decode(w, r, &sk) {
		return
	}
	sk.Slug = r.PathValue("slug")
	if err := store.ValidateHostedSkill(&sk); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	username, err := s.st.PublishingUsername(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if username == "" {
		writeErr(w, 400, "claim a publishing username first")
		return
	}
	profile, err := s.st.ProfileFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	existed, err := s.st.HostedSkillExists(s.uid(r), sk.Slug)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	newSkill := !existed && profile.HostedPrefix != ""
	if newSkill && !strings.HasPrefix(sk.Slug, profile.HostedPrefix) {
		writeErr(w, 400, "skill names must start with "+profile.HostedPrefix+" — change the prefix in Account → Profile")
		return
	}
	saved, err := s.st.SaveHostedSkill(s.uid(r), sk)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "hosted-save", "", map[string]any{"name": saved.Slug, "published": saved.Published})
	if saved.Published {
		s.addHostedSkillToShelf(r, username, saved)
	} else {
		s.setHostedSkillVisible(r, username, saved.Slug, false)
	}
	writeJSON(w, 200, saved)
}

// addHostedSkillToShelf keeps a published reet skill in the shared catalogue
// and puts it on the author's shelf in To test so they can try what they ship.
func (s *server) addHostedSkillToShelf(r *http.Request, username string, skill store.HostedSkill) {
	rf, err := ref.ParseAt(requestOrigin(r)+"/"+username+"/"+skill.Slug, r.Host)
	if err != nil {
		return
	}
	test := true
	patch := store.Patch{Bookmarked: &test}
	if _, exists, err := s.st.GetFor(s.uid(r), rf.ID); err != nil {
		return
	} else if exists {
		patch = store.Patch{}
	}
	if _, err := s.st.UpsertFor(s.uid(r), rf, patch); err != nil {
		return
	}
	_ = s.st.SetHostedSkillVisible(rf.ID, true)
	_ = s.st.SetMeta(rf.ID, skill.Slug, skill.Description, 0, 0, nil)
}

func (s *server) setHostedSkillVisible(r *http.Request, username, slug string, visible bool) {
	rf, err := ref.ParseAt(requestOrigin(r)+"/"+username+"/"+slug, r.Host)
	if err != nil {
		return
	}
	_ = s.st.SetHostedSkillVisible(rf.ID, visible)
}

func (s *server) hostedRename(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	slug := r.PathValue("slug")
	name := strings.ToLower(strings.TrimSpace(in.Name))
	if !store.ValidHostedName(name) {
		writeErr(w, 400, "skill names must use lowercase letters, numbers and single hyphens (up to 64 characters)")
		return
	}
	profile, err := s.st.ProfileFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if profile.HostedPrefix != "" && !strings.HasPrefix(name, profile.HostedPrefix) {
		writeErr(w, 400, "skill names must start with "+profile.HostedPrefix+" — change the prefix in Account → Profile")
		return
	}
	username, err := s.st.PublishingUsername(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if username == "" {
		writeErr(w, 400, "claim a publishing username first")
		return
	}
	oldRef, oldErr := ref.ParseAt(requestOrigin(r)+"/"+username+"/"+slug, r.Host)
	newRef, newErr := ref.ParseAt(requestOrigin(r)+"/"+username+"/"+name, r.Host)
	if oldErr != nil || newErr != nil {
		writeErr(w, 400, "that skill name cannot be used")
		return
	}
	renamed, err := s.st.RenameHostedSkill(s.uid(r), slug, name, oldRef, newRef)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "skill not found")
		return
	}
	if errors.Is(err, store.ErrHostedNameTaken) {
		writeErr(w, 409, err.Error())
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if renamed.Published {
		_ = s.st.SetMeta(newRef.ID, renamed.Slug, renamed.Description, 0, 0, nil)
	}
	s.eventRequest(r, "hosted-rename", "", map[string]any{"from": slug, "to": renamed.Slug})
	writeJSON(w, 200, renamed)
}

func (s *server) hostedDelete(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	skillID := ""
	if username, err := s.st.PublishingUsername(s.uid(r)); err == nil && username != "" {
		if rf, err := ref.ParseAt(requestOrigin(r)+"/"+username+"/"+slug, r.Host); err == nil {
			skillID = rf.ID
		}
	}
	err := s.st.DeleteHostedSkill(s.uid(r), slug, skillID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "skill not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "hosted-delete", "", map[string]any{"name": r.PathValue("slug")})
	writeJSON(w, 200, map[string]bool{"deleted": true})
}

func (s *server) publicHosted(w http.ResponseWriter, r *http.Request) (store.HostedSkill, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	sk, err := s.st.PublishedHostedSkill(r.PathValue("username"), r.PathValue("slug"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return sk, false
	}
	if err != nil {
		http.Error(w, "could not load skill", 500)
		return sk, false
	}
	return sk, true
}

func (s *server) hostedIndex(w http.ResponseWriter, r *http.Request) {
	sk, ok := s.publicHosted(w, r)
	if !ok {
		return
	}
	files := []string{"SKILL.md"}
	for name := range sk.Files {
		files = append(files, name)
	}
	sort.Strings(files)
	writeJSON(w, 200, map[string]any{"skills": []any{map[string]any{"name": sk.Slug, "description": sk.Description, "files": files}}})
}

func (s *server) hostedFile(w http.ResponseWriter, r *http.Request) {
	sk, ok := s.publicHosted(w, r)
	if !ok {
		return
	}
	name := r.PathValue("file")
	if name == "" {
		name = "SKILL.md"
	} else if r.PathValue("name") != sk.Slug {
		http.NotFound(w, r)
		return
	}
	content := []byte(sk.Content)
	if name != "SKILL.md" {
		var exists bool
		content, exists = sk.Files[name]
		if !exists {
			http.NotFound(w, r)
			return
		}
	}
	// Supporting files may contain HTML or scripts. Always download them as data.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="skill-file"`)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Write(content)
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

var hostedPage = template.Must(template.New("hosted").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Skill.Slug}} by {{.Username}} · reet.sh</title><meta name="description" content="{{.Skill.Description}}"><meta name="robots" content="index,follow"><link rel="canonical" href="{{.Origin}}{{.Path}}" />
<meta property="og:type" content="article"><meta property="og:site_name" content="reet.sh"><meta property="og:title" content="{{.Skill.Slug}} by {{.Username}} · reet.sh"><meta property="og:description" content="{{.Skill.Description}}"><meta property="og:url" content="{{.Origin}}{{.Path}}"><meta property="og:image" content="{{.Origin}}/og-image.png"><meta property="og:image:alt" content="reet.sh: Your AI agents. Your machines. One control room." />
<meta name="twitter:card" content="summary_large_image"><meta name="twitter:title" content="{{.Skill.Slug}} by {{.Username}} · reet.sh"><meta name="twitter:description" content="{{.Skill.Description}}"><meta name="twitter:image" content="{{.Origin}}/og-image.png">
<script type="application/ld+json">{{.StructuredData}}</script><style>
:root{color-scheme:light dark;font-family:system-ui,sans-serif}body{max-width:850px;margin:0 auto;padding:32px 20px;line-height:1.6}a{color:inherit}header{display:flex;justify-content:space-between;gap:16px}h1{overflow-wrap:anywhere;margin-bottom:8px}.muted{opacity:.7}pre{padding:20px;background:light-dark(#f4f3ee,#222);border-radius:12px;white-space:pre-wrap;overflow-wrap:anywhere}code{font-size:.95em}li{overflow-wrap:anywhere}h2{margin-top:32px}nav[aria-label=Breadcrumb]{font-size:.85rem;opacity:.8;margin:0 0 18px}nav[aria-label=Breadcrumb] span{overflow-wrap:anywhere}</style></head>
<body><header><a href="/">reet.sh</a><a href="/hosting">Host your own skills</a></header><main><nav aria-label="Breadcrumb"><a href="/">reet.sh</a> / <a href="/community">Community</a> / <span>{{.Username}}</span> / <span>{{.Skill.Slug}}</span></nav><p class="muted">Published by {{.Username}} · updated {{.Updated}}</p><h1>{{.Skill.Slug}}</h1><p>{{.Skill.Description}}</p><h2>Install this skill</h2><pre><code>{{.Command}}</code></pre><p><a href="{{.Path}}/SKILL.md">Download SKILL.md</a></p><h2>Skill contents</h2><pre>{{.Skill.Content}}</pre>{{if .Files}}<h2>Supporting files</h2><ul>{{range .Files}}<li><a href="{{$.Path}}/.well-known/agent-skills/{{$.Skill.Slug}}/{{.}}">{{.}}</a></li>{{end}}</ul>{{end}}<p><a href="/community">Browse the public community</a> · <a href="https://github.com/eyupio/reet.sh">Documentation and source</a></p></main></body></html>`))

func (s *server) hostedPublic(w http.ResponseWriter, r *http.Request) {
	sk, ok := s.publicHosted(w, r)
	if !ok {
		return
	}
	files := []string{}
	for name := range sk.Files {
		files = append(files, name)
	}
	sort.Strings(files)
	p := "/" + r.PathValue("username") + "/" + sk.Slug
	origin := requestOrigin(r)
	updated := time.UnixMilli(sk.UpdatedAt).UTC().Format(time.RFC3339)
	structured, err := json.Marshal(map[string]any{
		"@context": "https://schema.org",
		"@graph": []any{
			map[string]any{
				"@type":          "SoftwareSourceCode",
				"name":           sk.Slug,
				"description":    sk.Description,
				"url":            origin + p,
				"codeRepository": origin + p + "/SKILL.md",
				"dateModified":   updated,
				"author":         map[string]any{"@type": "Person", "name": r.PathValue("username")},
				"isPartOf":       map[string]any{"@type": "WebSite", "name": "reet.sh", "url": origin + "/"},
			},
			map[string]any{
				"@type": "BreadcrumbList",
				"itemListElement": []any{
					map[string]any{"@type": "ListItem", "position": 1, "name": "reet.sh", "item": origin + "/"},
					map[string]any{"@type": "ListItem", "position": 2, "name": "Community", "item": origin + "/community"},
					map[string]any{"@type": "ListItem", "position": 3, "name": sk.Slug, "item": origin + p},
				},
			},
		},
	})
	if err != nil {
		http.Error(w, "could not render skill", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	_ = hostedPage.Execute(w, map[string]any{"Skill": sk, "Username": r.PathValue("username"), "Path": p, "Files": files, "Command": "npx skills add " + origin + p, "Origin": origin, "Updated": updated, "StructuredData": template.JS(structured)})
}

func (s *server) parseSkillInput(r *http.Request, input string) (ref.Ref, error) {
	rf, err := ref.ParseAt(input, r.Host)
	if err == nil && rf.SourceType == "hosted" {
		_, username, _ := strings.Cut(rf.Source, "/")
		if _, err := s.st.PublishedHostedSkill(username, rf.Slug); err != nil {
			return ref.Ref{}, errors.New("published skill not found")
		}
	}
	return rf, err
}
