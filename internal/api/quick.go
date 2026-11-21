package api

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

// quickRequest reads what a bookmarklet asks for: action like adds the skill
// if need be and likes it; save is the bookmarklet's Add, which like the
// Skills page's Add sets no stage, so a new skill is Active and one already in
// the library keeps its state (Restore is the way back from Archived). Old
// bookmarks still send "used", which does the same.
func (s *server) quickRequest(r *http.Request, q url.Values) (action string, rf ref.Ref, code int, msg string) {
	switch q.Get("action") {
	case "like":
		action = "like"
	case "save", "used":
		action = "add"
	default:
		return "", rf, 400, "Unknown action"
	}
	rf, err := s.parseSkillInput(r, q.Get("url"))
	if err != nil {
		return "", rf, 400, "Not a skills.sh skill page"
	}
	return action, rf, 200, ""
}

// quickConfirm is what a bookmarklet opens: a page that asks before it adds
// anything. A GET changes nothing, so another website that opens it in the
// visitor's browser adds nothing; the page's button posts back to /quick.
func (s *server) quickConfirm(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	action, rf, code, msg := s.quickRequest(r, q)
	if code != 200 {
		quickPage(w, code, msg)
		return
	}
	target := "/quick"
	if t := q.Get("t"); t != "" {
		target += "?t=" + url.QueryEscape(t)
	}
	title, button := "Add "+rf.Slug+" to your library?", "Add "+rf.Slug
	if action == "like" {
		title, button = "Like "+rf.Slug+"?", "Like "+rf.Slug
	}
	form := `<form method="post" action="` + html.EscapeString(target) + `"><input type="hidden" name="action" value="` + html.EscapeString(q.Get("action")) + `"><input type="hidden" name="url" value="` + html.EscapeString(q.Get("url")) + `">` +
		`<p>If it isn't in your library yet, it goes on every machine that follows your library.</p><button type="submit" autofocus>` + html.EscapeString(button) + `</button> <a class="quiet" href="/">Not now</a></form>`
	quickShell(w, 200, html.EscapeString(title), form, false)
}

// quickOrigin reports whether a POST to /quick came from this reet's own
// page: browsers say where a request came from, and only the confirm page
// (same origin) may submit.
func quickOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
}

func (s *server) quick(w http.ResponseWriter, r *http.Request) {
	if !quickOrigin(r) {
		quickPage(w, http.StatusForbidden, "That request didn't come from this reet's own page")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		quickPage(w, 400, "Unknown action")
		return
	}
	actionName, rf, code, msg := s.quickRequest(r, r.PostForm)
	if code != 200 {
		quickPage(w, code, msg)
		return
	}
	yes := true
	var p store.Patch
	if actionName == "like" {
		p.Liked = &yes
	}
	old, found, err := s.st.GetFor(s.uid(r), rf.ID)
	if err == nil && found {
		// A row whose skill was deleted and waits for its purge is added
		// afresh: AddFor brings it back.
		found, err = s.st.LiveLibrarySkillFor(s.uid(r), "", rf.ID)
	}
	if err != nil {
		quickPage(w, 500, "Could not save")
		return
	}
	already := found && (actionName == "add" || old.Liked)
	sk, err := s.st.AddFor(s.uid(r), rf, p)
	if errors.Is(err, store.ErrSkillNameTaken) {
		quickPage(w, 409, "A skill with that name is already in your library")
		return
	}
	if err != nil {
		quickPage(w, 500, "Could not save")
		return
	}
	s.enqueue(rf)
	if !found {
		s.kickSkillCopies(s.uid(r))
	}
	s.event(s.uid(r), "quick", actionName, sk.ID, map[string]any{"name": sk.Name, "already": already})
	switch {
	case actionName == "like" && already:
		msg = "Already liked: " + sk.Name
	case actionName == "like":
		msg = "Liked: " + sk.Name
	case already:
		msg = sk.Name + " is already in your library"
	default:
		msg = "Added " + sk.Name
	}
	quickPage(w, 200, msg)
}

func quickPage(w http.ResponseWriter, code int, msg string) {
	quickShell(w, code, html.EscapeString(msg), `<p>This window will close automatically. You can also return to your library.</p><a class="button" href="/">Open your library</a>`, true)
}

// quickShell writes the bookmarklet's small page: title and body are HTML,
// already escaped.
func quickShell(w http.ResponseWriter, code int, title, body string, closes bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	script := ""
	if closes {
		script = `<script>setTimeout(function(){window.close()},1500)</script>`
	}
	fmt.Fprintf(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Bookmarklet · reet.sh</title>
<style>
:root{--bg:#f7f6f2;--card:#fffdf9;--ink:#1d1c1a;--muted:#5f5c54;--line:#e4e1d8;--accent:#c2410c;--accent-ink:#fff;color-scheme:light dark}
@media(prefers-color-scheme:dark){:root{--bg:#151413;--card:#1d1c1a;--ink:#efece6;--muted:#b2aca2;--line:#49453e;--accent:#fb923c;--accent-ink:#1a1208}}
*{box-sizing:border-box}body{margin:0;min-height:100dvh;padding:16px;display:grid;place-items:center;background:var(--bg);color:var(--ink);font:16px/1.55 system-ui,sans-serif}
main{width:100%%;max-width:440px;padding:24px;border:1px solid var(--line);border-radius:16px;background:var(--card)}
.eyebrow{margin:18px 0 0;color:var(--accent);font-size:14px;font-weight:700;letter-spacing:.08em;text-transform:uppercase}
h1{margin:12px 0;font:500 24px/1.25 Georgia,serif;letter-spacing:-.02em;overflow-wrap:anywhere}p{color:var(--muted);font-size:14px}
.button,button{display:inline-flex;align-items:center;justify-content:center;min-height:44px;margin-top:12px;padding:10px 16px;border:0;border-radius:8px;background:var(--accent);color:var(--accent-ink);font:600 14px/1.2 system-ui,sans-serif;text-decoration:none;cursor:pointer;overflow-wrap:anywhere}
.quiet{display:inline-flex;align-items:center;min-height:44px;margin-top:12px;padding:10px 12px;color:var(--ink);font-size:14px}
a:focus-visible,button:focus-visible{outline:2px solid var(--accent);outline-offset:4px}
</style></head><body><main><img src="/favicon.svg" width="40" height="40" alt=""><p class="eyebrow">reet.sh · Bookmarklet</p><h1>%s</h1>%s</main>%s</body></html>`, title, body, script)
}
